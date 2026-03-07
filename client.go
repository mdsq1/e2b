package e2b

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Client 是 E2B 管理平面 API 客户端。
type Client struct {
	config     ConnectionConfig // 连接配置
	httpClient *http.Client     // HTTP 客户端
}

// logf 输出日志，优先使用自定义 logger，否则 fallback 到标准 log
func (c *Client) logf(format string, args ...interface{}) {
	if c.config.Logger != nil {
		c.config.Logger.Printf(format, args...)
	} else {
		log.Printf(format, args...)
	}
}

// NewClient 创建一个新的 E2B API 客户端。
// API 密钥从参数或 E2B_API_KEY 环境变量中读取。
func NewClient(opts ...ClientOption) (*Client, error) {
	cfg := &clientConfig{
		domain:         DefaultDomain,
		apiURL:         DefaultAPIURL,
		requestTimeout: DefaultRequestTimeout,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	apiKey := cfg.apiKey
	if apiKey == "" {
		apiKey = os.Getenv("E2B_API_KEY")
	}
	if apiKey == "" {
		return nil, &AuthenticationError{SandboxError{Message: "API key is required. Set it via WithAPIKey() or E2B_API_KEY environment variable."}}
	}

	// 从环境变量 E2B_DOMAIN 读取域名配置
	domain := cfg.domain
	if domain == DefaultDomain {
		if envDomain := os.Getenv("E2B_DOMAIN"); envDomain != "" {
			domain = envDomain
		}
	}

	// 从环境变量 E2B_API_URL 读取 API 地址（未显式设置时从域名派生）
	apiURL := cfg.apiURL
	if apiURL == DefaultAPIURL {
		if envAPIURL := os.Getenv("E2B_API_URL"); envAPIURL != "" {
			apiURL = envAPIURL
		} else if domain != DefaultDomain {
			apiURL = "https://api." + domain
		}
	}

	// 从环境变量 E2B_DEBUG 读取调试模式配置
	debug := cfg.debug
	if !debug {
		if envDebug := os.Getenv("E2B_DEBUG"); strings.EqualFold(envDebug, "true") || envDebug == "1" {
			debug = true
		}
	}

	// 调试模式下，如果未设置自定义 URL，则使用调试 API 地址
	if debug && apiURL == DefaultAPIURL {
		apiURL = DefaultDebugAPIURL
	}

	// 从环境变量 E2B_ACCESS_TOKEN 读取访问令牌
	accessToken := cfg.accessToken
	if accessToken == "" {
		accessToken = os.Getenv("E2B_ACCESS_TOKEN")
	}

	// 从环境变量 E2B_SANDBOX_URL 读取沙箱地址
	sandboxURL := cfg.sandboxURL
	if sandboxURL == "" {
		sandboxURL = os.Getenv("E2B_SANDBOX_URL")
	}

	httpClient := cfg.httpClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: cfg.requestTimeout,
		}
	}

	headers := map[string]string{
		"User-Agent": "e2b-go-sdk/" + SDKVersion,
	}

	return &Client{
		config: ConnectionConfig{
			APIKey:         apiKey,
			Domain:         domain,
			APIURL:         apiURL,
			Debug:          debug,
			RequestTimeout: cfg.requestTimeout,
			AccessToken:    accessToken,
			SandboxURL:     sandboxURL,
			Headers:        headers,
			Logger:         cfg.logger,
		},
		httpClient: httpClient,
	}, nil
}

// doRequest 执行 HTTP 请求，将请求体序列化为 JSON，并将响应反序列化到 result 中。
func (c *Client) doRequest(ctx context.Context, method, path string, body, result any) error {
	_, err := c.doRequestWithHeaders(ctx, method, path, body, result)
	return err
}

// doRequestWithHeaders 与 doRequest 类似，但同时返回响应头。
func (c *Client) doRequestWithHeaders(ctx context.Context, method, path string, body, result any) (http.Header, error) {
	var bodyReader io.Reader
	var requestBody string
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, &SandboxError{Message: fmt.Sprintf("failed to marshal request body: %v", err), Cause: err}
		}
		bodyReader = bytes.NewReader(data)
		requestBody = string(data)
	}

	url := c.config.APIURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, &SandboxError{Message: fmt.Sprintf("failed to create request: %v", err), Cause: err}
	}

	req.Header.Set("X-API-Key", c.config.APIKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range c.config.Headers {
		req.Header.Set(k, v)
	}

	// 输出 HTTP 请求详情（调试用）
	if c.config.Debug {
		c.logf("[E2B HTTP] %s %s", method, url)
		c.logf("[E2B HTTP] Headers: %v", req.Header)
		if requestBody != "" {
			c.logf("[E2B HTTP] Body: %s", requestBody)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &SandboxError{Message: fmt.Sprintf("request failed: %v", err), Cause: err}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &SandboxError{Message: fmt.Sprintf("failed to read response body: %v", err), Cause: err}
	}

	// 输出 HTTP 响应详情（调试用）
	if c.config.Debug {
		c.logf("[E2B HTTP] Status: %d", resp.StatusCode)
		c.logf("[E2B HTTP] Headers: %v", resp.Header)
		c.logf("[E2B HTTP] Body: %s", string(respBody))
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, mapHTTPError(resp.StatusCode, string(respBody))
	}

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return nil, &SandboxError{Message: fmt.Sprintf("failed to unmarshal response: %v", err), Cause: err}
		}
	}

	return resp.Header, nil
}

// KillSandbox 根据 ID 销毁沙箱。
// 与 Python SDK 对齐：返回 (true, nil) 销毁成功，(false, nil) 表示未找到。
func (c *Client) KillSandbox(ctx context.Context, sandboxID string) (bool, error) {
	err := c.doRequest(ctx, http.MethodDelete, "/sandboxes/"+sandboxID, nil, nil)
	if err != nil {
		if _, ok := err.(*NotFoundError); ok {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// SetSandboxTimeout 根据 ID 设置沙箱的超时时间。
func (c *Client) SetSandboxTimeout(ctx context.Context, sandboxID string, timeoutSeconds int) error {
	body := map[string]int{"timeout": timeoutSeconds}
	return c.doRequest(ctx, http.MethodPost, "/sandboxes/"+sandboxID+"/timeout", body, nil)
}

// GetSandboxInfo 根据 ID 获取沙箱信息。
func (c *Client) GetSandboxInfo(ctx context.Context, sandboxID string) (*SandboxInfo, error) {
	var info SandboxInfo
	err := c.doRequest(ctx, http.MethodGet, "/sandboxes/"+sandboxID, nil, &info)
	if err != nil {
		return nil, err
	}
	return &info, nil
}

// PauseSandbox 根据 ID 暂停沙箱。
// 与 Python SDK 对齐：409 Conflict 表示沙箱已暂停，静默成功返回 nil。
func (c *Client) PauseSandbox(ctx context.Context, sandboxID string) error {
	err := c.doRequest(ctx, http.MethodPost, "/sandboxes/"+sandboxID+"/pause", nil, nil)
	if err != nil {
		if isConflictError(err) {
			return nil // 已暂停状态，静默成功
		}
		return err
	}
	return nil
}

// ListSandboxes 返回用于列出沙箱的分页器。
func (c *Client) ListSandboxes(ctx context.Context, query *SandboxQuery) *Paginator[SandboxInfo] {
	return newPaginator[SandboxInfo](0, func(ctx context.Context, token string, limit int) ([]SandboxInfo, string, error) {
		path := "/v2/sandboxes"
		sep := "?"
		if token != "" {
			path += sep + "nextToken=" + url.QueryEscape(token)
			sep = "&"
		}
		if limit > 0 {
			path += sep + "limit=" + fmt.Sprintf("%d", limit)
			sep = "&"
		}
		if query != nil {
			for k, v := range query.Metadata {
				path += sep + "metadata." + url.QueryEscape(k) + "=" + url.QueryEscape(v)
				sep = "&"
			}
			for _, s := range query.State {
				path += sep + "state=" + url.QueryEscape(string(s))
				sep = "&"
			}
		}

		var sandboxes []SandboxInfo
		headers, err := c.doRequestWithHeaders(ctx, http.MethodGet, path, nil, &sandboxes)
		if err != nil {
			return nil, "", err
		}

		nextToken := ""
		if headers != nil {
			nextToken = headers.Get("x-next-token")
		}

		return sandboxes, nextToken, nil
	})
}

// ListSnapshots 返回用于列出快照的分页器。
func (c *Client) ListSnapshots(ctx context.Context, sandboxID *string) *Paginator[SnapshotInfo] {
	return newPaginator[SnapshotInfo](0, func(ctx context.Context, token string, limit int) ([]SnapshotInfo, string, error) {
		path := "/snapshots"
		sep := "?"
		if sandboxID != nil {
			path += sep + "sandbox_id=" + url.QueryEscape(*sandboxID)
			sep = "&"
		}
		if token != "" {
			path += sep + "next_token=" + url.QueryEscape(token)
			sep = "&"
		}
		if limit > 0 {
			path += sep + "limit=" + fmt.Sprintf("%d", limit)
		}

		var snapshots []SnapshotInfo
		headers, err := c.doRequestWithHeaders(ctx, http.MethodGet, path, nil, &snapshots)
		if err != nil {
			return nil, "", err
		}

		nextToken := ""
		if headers != nil {
			nextToken = headers.Get("x-next-token")
		}

		return snapshots, nextToken, nil
	})
}

// DeleteSnapshot 根据 ID 删除快照。
func (c *Client) DeleteSnapshot(ctx context.Context, snapshotID string) error {
	return c.doRequest(ctx, http.MethodDelete, "/templates/"+snapshotID, nil, nil)
}

// createSandboxRequest 是创建沙箱的请求体。
type createSandboxRequest struct {
	TemplateID          string            `json:"templateID"`
	Timeout             int               `json:"timeout,omitempty"`
	Metadata            map[string]string `json:"metadata,omitempty"`
	EnvVars             map[string]string `json:"envVars,omitempty"`
	AutoPause           *bool             `json:"autoPause,omitempty"`
	Secure              *bool             `json:"secure,omitempty"`
	AllowInternetAccess *bool             `json:"allowInternetAccess,omitempty"`
	Network             *NetworkOpts      `json:"network,omitempty"`
	AutoResume          *AutoResumePolicy `json:"autoResume,omitempty"`
	VolumeMounts        []VolumeMount     `json:"volumeMounts,omitempty"`
	MCP                 MCPConfig         `json:"mcp,omitempty"`
}

// createSandboxResponse 是创建或连接沙箱时的响应体。
type createSandboxResponse struct {
	SandboxID          string `json:"sandboxID"`
	ClientID           string `json:"clientID"` // 客户端标识符（短 ID），不是域名
	Domain             string `json:"domain"`   // 沙盒域名（如 aisandbox.qihoo.net），可能为空
	EnvdVersion        string `json:"envdVersion"`
	EnvdAccessToken    string `json:"envdAccessToken"`
	TrafficAccessToken string `json:"trafficAccessToken"`
}

// connectSandboxResponse 是连接到现有沙箱时的响应体。
type connectSandboxResponse struct {
	EnvdVersion        string `json:"envdVersion"`
	EnvdAccessToken    string `json:"envdAccessToken"`
	TrafficAccessToken string `json:"trafficAccessToken"`
	Domain             string `json:"domain"` // 沙箱域名，与 createSandboxResponse 对齐
}

// snapshotResponse 是创建快照时的响应体。
type snapshotResponse struct {
	SnapshotID string `json:"snapshotID"`
}

// CreateSandbox 创建一个新的沙箱并返回已连接的 Sandbox 实例。
func (c *Client) CreateSandbox(ctx context.Context, opts ...SandboxOption) (*Sandbox, error) {
	cfg := &sandboxConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	applySandboxDefaults(cfg)

	reqBody := createSandboxRequest{
		TemplateID:          cfg.template,
		Timeout:             cfg.timeout,
		Metadata:            cfg.metadata,
		EnvVars:             cfg.envVars,
		AutoPause:           cfg.autoPause,
		Secure:              cfg.secure,
		AllowInternetAccess: cfg.allowInternetAccess,
		Network:             cfg.network,
		AutoResume:          cfg.autoResume,
		VolumeMounts:        cfg.volumeMounts,
		MCP:                 cfg.mcp,
	}

	var resp createSandboxResponse
	err := c.doRequest(ctx, http.MethodPost, "/sandboxes", reqBody, &resp)
	if err != nil {
		return nil, err
	}

	// 与 Python SDK 对齐：envd < 0.1.0 表示模板过旧，需先 kill 再报错
	if ev := parseEnvdVersion(resp.EnvdVersion); versionLessThan(ev, [3]int{0, 1, 0}) {
		_ = c.doRequest(ctx, http.MethodDelete, "/sandboxes/"+resp.SandboxID, nil, nil)
		return nil, &TemplateError{SandboxError{Message: "you need to update the template. Run `e2b template build` in the template directory."}}
	}

	// domain 字段为沙盒域名；若 API 未返回或返回无效值则 fallback 到 config.Domain
	sandboxDomain := resp.Domain
	if sandboxDomain == "" || !strings.Contains(sandboxDomain, ".") {
		sandboxDomain = c.config.Domain
	}
	sbx := c.newSandbox(resp.SandboxID, sandboxDomain, resp.EnvdVersion, resp.EnvdAccessToken, resp.TrafficAccessToken)

	// 与 Python SDK 对齐：如果指定了 MCP 配置，在沙箱内启动 mcp-gateway 进程。
	if len(cfg.mcp) > 0 {
		token, err := generateToken()
		if err != nil {
			_ = c.doRequest(ctx, http.MethodDelete, "/sandboxes/"+resp.SandboxID, nil, nil)
			return nil, &SandboxError{Message: fmt.Sprintf("failed to generate MCP token: %v", err), Cause: err}
		}
		mcpJSON, err := json.Marshal(cfg.mcp)
		if err != nil {
			_ = c.doRequest(ctx, http.MethodDelete, "/sandboxes/"+resp.SandboxID, nil, nil)
			return nil, &SandboxError{Message: fmt.Sprintf("failed to marshal MCP config: %v", err), Cause: err}
		}
		result, err := sbx.Commands.Run(ctx,
			fmt.Sprintf("mcp-gateway --config '%s'", string(mcpJSON)),
			WithUser("root"),
			WithCommandEnvVars(map[string]string{"GATEWAY_ACCESS_TOKEN": token}),
		)
		if err != nil {
			_ = c.doRequest(ctx, http.MethodDelete, "/sandboxes/"+resp.SandboxID, nil, nil)
			return nil, &SandboxError{Message: fmt.Sprintf("failed to start MCP gateway: %v", err), Cause: err}
		}
		if result.ExitCode != 0 {
			_ = c.doRequest(ctx, http.MethodDelete, "/sandboxes/"+resp.SandboxID, nil, nil)
			return nil, &SandboxError{Message: fmt.Sprintf("failed to start MCP gateway: %s", result.Stderr)}
		}
		sbx.mcpToken = token
	}

	return sbx, nil
}

// ConnectSandbox 根据 ID 连接到一个已存在的沙箱。
// 与 Python SDK 对齐：支持 timeout 参数，发送 body {"timeout": N} 到连接接口。
func (c *Client) ConnectSandbox(ctx context.Context, sandboxID string, timeoutSeconds ...int) (*Sandbox, error) {
	type connectBody struct {
		Timeout int `json:"timeout,omitempty"`
	}
	body := &connectBody{}
	if len(timeoutSeconds) > 0 && timeoutSeconds[0] > 0 {
		body.Timeout = timeoutSeconds[0]
	} else {
		body.Timeout = DefaultSandboxTimeout // 与 Python 对齐：默认 300 秒
	}

	var resp connectSandboxResponse
	err := c.doRequest(ctx, http.MethodPost, "/sandboxes/"+sandboxID+"/connect", body, &resp)
	if err != nil {
		return nil, err
	}

	// 使用响应中的域名；若缺失或不合法则 fallback 到配置的域名。
	sandboxDomain := resp.Domain
	if sandboxDomain == "" || !strings.Contains(sandboxDomain, ".") {
		sandboxDomain = c.config.Domain
	}

	sbx := c.newSandbox(sandboxID, sandboxDomain, resp.EnvdVersion, resp.EnvdAccessToken, resp.TrafficAccessToken)
	return sbx, nil
}

// generateToken 生成一个随机的 16 字节十六进制令牌（与 Python uuid.uuid4() 等效）。
func generateToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// newSandbox 创建一个新的 Sandbox 实例并初始化其子模块。
func (c *Client) newSandbox(sandboxID, sandboxDomain, envdVersion, envdAccessToken, trafficAccessToken string) *Sandbox {
	sbx := &Sandbox{
		ID:                 sandboxID,
		SandboxDomain:      sandboxDomain,
		client:             c,
		envdAccessToken:    envdAccessToken,
		trafficAccessToken: trafficAccessToken,
		envdVersion:        parseEnvdVersion(envdVersion),
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
			},
		},
	}
	sbx.envdAPIURL = c.config.GetSandboxURL(sandboxID, sandboxDomain)

	// 初始化用于 envd 通信的 Connect RPC 客户端
	rpcClient := sbx.newConnectRPCClient()

	// 初始化子模块
	sbx.Commands = newCommands(sbx, rpcClient)
	sbx.Files = newFilesystem(sbx, rpcClient, sbx.httpClient)
	sbx.Pty = newPty(sbx, rpcClient)
	sbx.Git = &Git{commands: sbx.Commands}

	return sbx
}
