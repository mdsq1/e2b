package e2b

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// 端口和默认配置常量。
const (
	EnvdPort    = 49983 // envd 服务端口
	JupyterPort = 49999 // Jupyter 服务端口

	DefaultDomain                  = "aisandbox.qihoo.net"             // 默认域名
	DefaultAPIURL                  = "https://api.aisandbox.qihoo.net" // 默认 API 地址
	DefaultDebugAPIURL             = "http://localhost:3000"           // 调试模式下的默认 API 地址
	DefaultRequestTimeout          = 60 * time.Second                  // 默认请求超时时间
	DefaultSandboxTimeout          = 300                               // 默认沙箱超时时间（秒）
	DefaultTemplate                = "base"                            // 默认沙箱模板
	DefaultCodeInterpreterTemplate = "code-interpreter-v1"             // 默认代码解释器模板

	// SDKVersion 是 Go SDK 的版本号。
	SDKVersion = "0.1.0"

	// AllTraffic 表示所有网络流量的 CIDR 范围。
	AllTraffic = "0.0.0.0/0"
)

// envd 功能所需的最低版本号。
var (
	envdVersionStdin          = [3]int{0, 1, 2} // 支持标准输入的最低版本
	envdVersionRecursiveWatch = [3]int{0, 1, 4} // 支持递归监听的最低版本
	envdVersionDefaultUser    = [3]int{0, 1, 5} // 支持默认用户的最低版本
)

// ConnectionConfig 包含与 E2B 服务建立连接所需的配置信息。
type ConnectionConfig struct {
	APIKey         string            // API 密钥
	Domain         string            // 服务域名
	APIURL         string            // API 地址
	Debug          bool              // 是否开启调试模式
	RequestTimeout time.Duration     // 请求超时时间
	Headers        map[string]string // 自定义请求头
	SandboxURL     string            // 沙箱服务地址
	AccessToken    string            // 访问令牌
	Logger         Logger            // 自定义日志输出
}

// GetHost 根据沙箱 ID、域名和端口号构建主机地址。
// 在调试模式下返回 localhost 地址。
func (c *ConnectionConfig) GetHost(sandboxID, sandboxDomain string, port int) string {
	if c.Debug {
		return fmt.Sprintf("localhost:%d", port)
	}
	return fmt.Sprintf("%d-%s.%s", port, sandboxID, sandboxDomain)
}

// GetSandboxURL 构建沙箱的完整 URL 地址。
// 如果已配置 SandboxURL 则直接返回；否则根据调试模式选择 http 或 https 协议。
func (c *ConnectionConfig) GetSandboxURL(sandboxID, sandboxDomain string) string {
	if c.SandboxURL != "" {
		return c.SandboxURL
	}
	scheme := "https"
	if c.Debug {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s", scheme, c.GetHost(sandboxID, sandboxDomain, EnvdPort))
}

// getRequestTimeout 获取请求超时时间。
// 优先使用覆盖值，其次使用配置值，最后使用默认值。
func (c *ConnectionConfig) getRequestTimeout(override *time.Duration) time.Duration {
	if override != nil {
		return *override
	}
	if c.RequestTimeout > 0 {
		return c.RequestTimeout
	}
	return DefaultRequestTimeout
}

// versionLessThan 比较两个语义化版本号 a 和 b，如果 a < b 则返回 true。
func versionLessThan(a, b [3]int) bool {
	if a[0] != b[0] {
		return a[0] < b[0]
	}
	if a[1] != b[1] {
		return a[1] < b[1]
	}
	return a[2] < b[2]
}

// parseEnvdVersion 将版本字符串（如 "0.1.2"）解析为三元素整数数组 [主版本, 次版本, 补丁版本]。
func parseEnvdVersion(version string) [3]int {
	var v [3]int
	parts := strings.Split(version, ".")
	for i := 0; i < 3 && i < len(parts); i++ {
		n, err := strconv.Atoi(parts[i])
		if err == nil {
			v[i] = n
		}
	}
	return v
}
