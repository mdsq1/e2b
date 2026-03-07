package e2b

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

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
// 与 Python SDK 对齐：返回 (true, nil) 成功，(false, nil) 表示沙箱未找到。
func (s *Sandbox) Kill(ctx context.Context) (bool, error) {
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

// envd 版本阈值（与 Python SDK 对齐）。
var (
	envdVersionMetrics     = [3]int{0, 1, 5} // 支持 metrics API 的最低版本（Python: 0.1.5）
	envdVersionDiskMetrics = [3]int{0, 2, 4} // 支持磁盘 metrics 的最低版本（Python: 0.2.4）
)

// GetMetrics 获取当前沙箱的资源使用指标。
// 与 Python SDK 对齐：支持可选的 start/end 时间范围过滤，以及 envd 版本检查。
func (s *Sandbox) GetMetrics(ctx context.Context, opts ...MetricsOption) ([]SandboxMetrics, error) {
	// 与 Python SDK 对齐：envd < 0.1.5 不支持 metrics，直接报错
	if versionLessThan(s.envdVersion, envdVersionMetrics) {
		return nil, &SandboxError{Message: "metrics are not supported in this version of the sandbox, please rebuild your template"}
	}
	// 与 Python SDK 对齐：envd < 0.2.4 不支持磁盘 metrics，记录告警
	if versionLessThan(s.envdVersion, envdVersionDiskMetrics) {
		log.Println("[E2B] Disk metrics are not supported in this version of the sandbox, please rebuild the template to get disk metrics.")
	}

	cfg := &metricsConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	path := "/sandboxes/" + s.ID + "/metrics"
	sep := "?"
	if !cfg.start.IsZero() {
		path += sep + "start=" + fmt.Sprintf("%d", cfg.start.Unix())
		sep = "&"
	}
	if !cfg.end.IsZero() {
		path += sep + "end=" + fmt.Sprintf("%d", cfg.end.Unix())
	}

	// 与 Python SDK 对齐：API 直接返回 JSON 数组，而非 {"metrics": [...]}
	var metrics []SandboxMetrics
	err := s.client.doRequest(ctx, http.MethodGet, path, nil, &metrics)
	if err != nil {
		return nil, err
	}
	if metrics == nil {
		return []SandboxMetrics{}, nil
	}
	return metrics, nil
}

// metricsConfig 包含 GetMetrics 的可选参数。
type metricsConfig struct {
	start time.Time
	end   time.Time
}

// MetricsOption 是 GetMetrics 的函数选项。
type MetricsOption func(*metricsConfig)

// WithMetricsStart 设置指标查询的开始时间。
func WithMetricsStart(t time.Time) MetricsOption {
	return func(c *metricsConfig) { c.start = t }
}

// WithMetricsEnd 设置指标查询的结束时间。
func WithMetricsEnd(t time.Time) MetricsOption {
	return func(c *metricsConfig) { c.end = t }
}

// Pause 暂停当前沙箱。
func (s *Sandbox) Pause(ctx context.Context) error {
	return s.client.PauseSandbox(ctx, s.ID)
}

// IsRunning 通过健康检查请求判断沙箱是否正在运行。
// 与 Python SDK 对齐：
//   - 502 Bad Gateway → 返回 (false, nil)
//   - 超时 → 返回 (false, TimeoutError)
//   - 其他任何 HTTP 响应 → 返回 (true, nil)
//   - 网络错误（非超时） → 返回 (false, err)
func (s *Sandbox) IsRunning(ctx context.Context) (bool, error) {
	healthURL := s.envdAPIURL + "/health"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return false, &SandboxError{Message: fmt.Sprintf("failed to create health check request: %v", err), Cause: err}
	}
	s.setSandboxHeaders(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		// 检查是否为超时错误（context deadline exceeded 或 client timeout）
		if ctx.Err() != nil {
			return false, &TimeoutError{SandboxError: SandboxError{Message: "health check timed out", Cause: ctx.Err()}}
		}
		// 其他网络错误也可能是超时（http.Client timeout）
		if isTimeoutError(err) {
			return false, &TimeoutError{SandboxError: SandboxError{Message: "health check timed out", Cause: err}}
		}
		return false, &SandboxError{Message: fmt.Sprintf("health check request failed: %v", err), Cause: err}
	}
	defer resp.Body.Close()
	// 502 表示代理层认为沙箱不可达
	if resp.StatusCode == http.StatusBadGateway {
		return false, nil
	}
	return true, nil
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

// buildSignedURL 构建文件操作 URL。
// 与 Python SDK 对齐：有 access token 时生成带签名的 URL，否则返回无签名 URL。
func (s *Sandbox) buildSignedURL(path, operation string, cfg *fileURLConfig) string {
	user := s.resolveUsername(cfg.user)
	u := s.envdAPIURL + "/files"
	params := url.Values{}
	params.Set("path", path)
	if user != "" {
		params.Set("username", user)
	}

	// 与 Python SDK 对齐：仅在有 access token 时生成签名
	if s.envdAccessToken != "" {
		var expSec *int
		if cfg.expiration > 0 {
			expSec = &cfg.expiration
		}
		sig, exp, err := getSignature(path, operation, user, s.envdAccessToken, expSec)
		if err == nil {
			params.Set("signature", sig)
			if exp != nil {
				params.Set("signature_expiration", fmt.Sprintf("%d", *exp))
			}
		}
	}

	return u + "?" + params.Encode()
}

// GetMCPURL 返回当前沙箱的 MCP 端点 URL。
// 与 Python SDK 对齐：使用 MCPPort (50005) 而非 EnvdPort。
func (s *Sandbox) GetMCPURL() string {
	host := s.GetHost(MCPPort)
	scheme := "https"
	if s.client.config.Debug {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s/mcp", scheme, host)
}

// GetMCPToken 从沙箱文件系统中读取 MCP 令牌。
// 与 Python SDK 对齐：使用 user="root" 读取令牌文件（该文件归 root 所有）。
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
	token, err := s.Files.Read(ctx, "/etc/mcp-gateway/.token", WithFsUser("root"))
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

// buildAuthHeader 根据已解析的用户名构建 Authorization: Basic 头。
// 与 Python SDK 的 authentication_header() 对齐：对 "user:" 进行 Base64 编码。
// user 应为 resolveUsername() 的返回值；若为空则返回 nil。
func buildAuthHeader(user string) map[string]string {
	if user == "" {
		return nil
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(user + ":"))
	return map[string]string{
		"Authorization": "Basic " + encoded,
	}
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
			"E2B-Traffic-Access-Token": s.trafficAccessToken,
			"E2b-Sandbox-Id":           s.ID,
			"E2b-Sandbox-Port":         fmt.Sprintf("%d", EnvdPort),
			"Keepalive-Ping-Interval":  fmt.Sprintf("%d", KeepalivePingIntervalSec), // 与 Python SDK 对齐: 50s
		},
	}
}

// setSandboxHeaders 为 HTTP 请求设置沙箱相关的认证和标识头。
func (s *Sandbox) setSandboxHeaders(req *http.Request) {
	s.setSandboxHeadersWithPort(req, EnvdPort)
}

// setSandboxHeadersWithPort 为 HTTP 请求设置沙箱相关的认证和标识头，支持自定义端口。
func (s *Sandbox) setSandboxHeadersWithPort(req *http.Request, port int) {
	if s.envdAccessToken != "" {
		req.Header.Set("X-Access-Token", s.envdAccessToken)
	} else if s.client.config.APIKey != "" {
		// 360 沙盒网关：envdAccessToken 为空时，fallback 到 API Key 认证
		req.Header.Set("X-API-Key", s.client.config.APIKey)
	}
	if s.trafficAccessToken != "" {
		req.Header.Set("E2B-Traffic-Access-Token", s.trafficAccessToken)
	}
	req.Header.Set("E2b-Sandbox-Id", s.ID)
	req.Header.Set("E2b-Sandbox-Port", fmt.Sprintf("%d", port))
}
