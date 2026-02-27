package e2b

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sync"

	"github.com/mdsq1/e2b/internal/connectrpc"
)

// Sandbox 表示一个正在运行的 E2B 沙箱。
type Sandbox struct {
	ID            string // 沙箱唯一标识符
	SandboxDomain string // 沙箱域名

	// 子模块（在 Connect RPC 客户端可用后初始化）
	Commands *Commands   // 命令执行模块
	Files    *Filesystem // 文件系统模块
	Pty      *Pty        // 伪终端模块
	Git      *Git        // Git 操作模块

	client             *Client      // 管理平面 API 客户端
	envdAPIURL         string       // envd 服务 API 地址
	envdAccessToken    string       // envd 访问令牌
	trafficAccessToken string       // 流量访问令牌
	envdVersion        [3]int       // envd 版本号
	httpClient         *http.Client // HTTP 客户端
	mcpToken           string       // MCP 令牌缓存
	mcpTokenMu         sync.Mutex   // 保护 mcpToken 的并发访问
}

// Kill 销毁当前沙箱。
func (s *Sandbox) Kill(ctx context.Context) error {
	return s.client.KillSandbox(ctx, s.ID)
}

// SetTimeout 设置沙箱的生命周期超时时间（秒）。
func (s *Sandbox) SetTimeout(ctx context.Context, timeoutSeconds int) error {
	return s.client.SetSandboxTimeout(ctx, s.ID, timeoutSeconds)
}

// GetInfo 获取当前沙箱的信息。
func (s *Sandbox) GetInfo(ctx context.Context) (*SandboxInfo, error) {
	return s.client.GetSandboxInfo(ctx, s.ID)
}

// GetMetrics 获取当前沙箱的资源使用指标。
func (s *Sandbox) GetMetrics(ctx context.Context) ([]SandboxMetrics, error) {
	var resp metricsResponse
	err := s.client.doRequest(ctx, http.MethodGet, "/sandboxes/"+s.ID+"/metrics", nil, &resp)
	if err != nil {
		return nil, err
	}
	return resp.Metrics, nil
}

// Pause 暂停当前沙箱。
func (s *Sandbox) Pause(ctx context.Context) error {
	return s.client.PauseSandbox(ctx, s.ID)
}

// IsRunning 通过健康检查请求判断沙箱是否正在运行。
func (s *Sandbox) IsRunning(ctx context.Context) bool {
	healthURL := s.envdAPIURL + "/health"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return false
	}
	s.setSandboxHeaders(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// GetHost 返回当前沙箱指定端口的主机名。
func (s *Sandbox) GetHost(port int) string {
	return s.client.config.GetHost(s.ID, s.SandboxDomain, port)
}

// CreateSnapshot 创建当前沙箱的快照。
func (s *Sandbox) CreateSnapshot(ctx context.Context) (*SnapshotInfo, error) {
	var resp snapshotResponse
	err := s.client.doRequest(ctx, http.MethodPost, "/sandboxes/"+s.ID+"/snapshots", nil, &resp)
	if err != nil {
		return nil, err
	}
	return &SnapshotInfo{SnapshotID: resp.SnapshotID}, nil
}

// DownloadURL 返回用于从沙箱下载文件的签名 URL。
func (s *Sandbox) DownloadURL(path string, opts ...FileURLOption) string {
	cfg := &fileURLConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	return s.buildSignedURL(path, "read", cfg)
}

// UploadURL 返回用于向沙箱上传文件的签名 URL。
func (s *Sandbox) UploadURL(path string, opts ...FileURLOption) string {
	cfg := &fileURLConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	return s.buildSignedURL(path, "write", cfg)
}

// buildSignedURL 构建带签名的文件操作 URL。
func (s *Sandbox) buildSignedURL(path, operation string, cfg *fileURLConfig) string {
	user := s.resolveUsername(cfg.user)

	var expSec *int
	if cfg.expiration > 0 {
		expSec = &cfg.expiration
	}

	sig, exp, err := getSignature(path, operation, user, s.envdAccessToken, expSec)
	if err != nil {
		return ""
	}

	u := s.envdAPIURL + "/files"
	params := url.Values{}
	params.Set("path", path)
	params.Set("signature", sig)
	if user != "" {
		params.Set("username", user)
	}
	if exp != nil {
		params.Set("signature_expiration", fmt.Sprintf("%d", *exp))
	}

	return u + "?" + params.Encode()
}

// GetMCPURL 返回当前沙箱的 MCP 端点 URL。
func (s *Sandbox) GetMCPURL() string {
	host := s.GetHost(EnvdPort)
	scheme := "https"
	if s.client.config.Debug {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s/mcp", scheme, host)
}

// GetMCPToken 从沙箱文件系统中读取 MCP 令牌。
// 此方法需要 Files 子模块已初始化。
// 该方法是并发安全的，成功读取后会缓存令牌，失败时允许重试。
func (s *Sandbox) GetMCPToken(ctx context.Context) (string, error) {
	s.mcpTokenMu.Lock()
	defer s.mcpTokenMu.Unlock()

	if s.mcpToken != "" {
		return s.mcpToken, nil
	}
	if s.Files == nil {
		return "", &SandboxError{Message: "filesystem module not initialized"}
	}
	token, err := s.Files.Read(ctx, "/etc/mcp-gateway/.token")
	if err != nil {
		return "", err
	}
	s.mcpToken = token
	return token, nil
}

// resolveUsername 解析用户名，如果未指定且 envd 版本较低则默认使用 "user"。
func (s *Sandbox) resolveUsername(user string) string {
	if user != "" {
		return user
	}
	if versionLessThan(s.envdVersion, envdVersionDefaultUser) {
		return "user"
	}
	return ""
}

// supportsStdin 检查当前 envd 版本是否支持标准输入。
func (s *Sandbox) supportsStdin() bool {
	return !versionLessThan(s.envdVersion, envdVersionStdin)
}

// supportsRecursiveWatch 检查当前 envd 版本是否支持递归目录监听。
func (s *Sandbox) supportsRecursiveWatch() bool {
	return !versionLessThan(s.envdVersion, envdVersionRecursiveWatch)
}

// newConnectRPCClient 创建用于与 envd 通信的 Connect RPC 客户端。
func (s *Sandbox) newConnectRPCClient() *connectrpc.Client {
	return &connectrpc.Client{
		BaseURL:    s.envdAPIURL,
		HTTPClient: s.httpClient,
		Headers: map[string]string{
			"X-Access-Token":           s.envdAccessToken,
			"E2B-Traffic-Access-Token":  s.trafficAccessToken,
			"E2b-Sandbox-Id":           s.ID,
			"E2b-Sandbox-Port":         fmt.Sprintf("%d", EnvdPort),
			"Keepalive-Ping-Interval":  "55",
		},
	}
}

// setSandboxHeaders 为 HTTP 请求设置沙箱相关的认证和标识头。
func (s *Sandbox) setSandboxHeaders(req *http.Request) {
	if s.envdAccessToken != "" {
		req.Header.Set("X-Access-Token", s.envdAccessToken)
	}
	if s.trafficAccessToken != "" {
		req.Header.Set("E2B-Traffic-Access-Token", s.trafficAccessToken)
	}
	req.Header.Set("E2b-Sandbox-Id", s.ID)
	req.Header.Set("E2b-Sandbox-Port", fmt.Sprintf("%d", EnvdPort))
}
