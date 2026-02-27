package e2b

import (
	"net/http"
	"time"
)

// === 客户端选项 ===

// clientConfig 包含客户端的内部配置参数。
type clientConfig struct {
	apiKey         string        // API 密钥
	domain         string        // 服务域名
	apiURL         string        // API 地址
	httpClient     *http.Client  // 自定义 HTTP 客户端
	requestTimeout time.Duration // 请求超时时间
	debug          bool          // 调试模式
	accessToken    string        // 访问令牌
	sandboxURL     string        // 沙箱服务地址
}

// ClientOption 是用于配置客户端的函数选项类型。
type ClientOption func(*clientConfig)

// WithAPIKey 设置 API 密钥。
func WithAPIKey(key string) ClientOption {
	return func(c *clientConfig) { c.apiKey = key }
}

// WithDomain 设置服务域名。
func WithDomain(domain string) ClientOption {
	return func(c *clientConfig) { c.domain = domain }
}

// WithAPIURL 设置 API 地址。
func WithAPIURL(url string) ClientOption {
	return func(c *clientConfig) { c.apiURL = url }
}

// WithHTTPClient 设置自定义 HTTP 客户端。
func WithHTTPClient(client *http.Client) ClientOption {
	return func(c *clientConfig) { c.httpClient = client }
}

// WithRequestTimeout 设置请求超时时间。
func WithRequestTimeout(d time.Duration) ClientOption {
	return func(c *clientConfig) { c.requestTimeout = d }
}

// WithDebug 设置是否启用调试模式。
func WithDebug(debug bool) ClientOption {
	return func(c *clientConfig) { c.debug = debug }
}

// WithAccessToken 设置访问令牌。
func WithAccessToken(token string) ClientOption {
	return func(c *clientConfig) { c.accessToken = token }
}

// WithSandboxURL 设置沙箱服务地址。
func WithSandboxURL(url string) ClientOption {
	return func(c *clientConfig) { c.sandboxURL = url }
}

// === 沙箱选项（创建时使用） ===

// sandboxConfig 包含沙箱创建的内部配置参数。
type sandboxConfig struct {
	template            string            // 模板名称
	timeout             int               // 超时时间（秒）
	metadata            map[string]string // 元数据
	envVars             map[string]string // 环境变量
	autoPause           *bool             // 是否自动暂停
	secure              *bool             // 是否启用安全模式
	allowInternetAccess *bool             // 是否允许互联网访问
	network             *NetworkOpts      // 网络配置
	autoResume          *AutoResumePolicy // 自动恢复策略
	volumeMounts        []VolumeMount     // 卷挂载列表
	mcp                 MCPConfig         // MCP 配置
}

// SandboxOption 是用于配置沙箱创建的函数选项类型。
type SandboxOption func(*sandboxConfig)

// WithTemplate 设置沙箱使用的模板。
func WithTemplate(template string) SandboxOption {
	return func(c *sandboxConfig) { c.template = template }
}

// WithTimeout 设置沙箱超时时间（秒）。
func WithTimeout(seconds int) SandboxOption {
	return func(c *sandboxConfig) { c.timeout = seconds }
}

// WithMetadata 设置沙箱的元数据。
func WithMetadata(metadata map[string]string) SandboxOption {
	return func(c *sandboxConfig) { c.metadata = metadata }
}

// WithEnvVars 设置沙箱的环境变量。
func WithEnvVars(envs map[string]string) SandboxOption {
	return func(c *sandboxConfig) { c.envVars = envs }
}

// WithAutoPause 设置是否启用自动暂停。
func WithAutoPause(autoPause bool) SandboxOption {
	return func(c *sandboxConfig) { c.autoPause = &autoPause }
}

// WithSecure 设置是否启用安全模式。
func WithSecure(secure bool) SandboxOption {
	return func(c *sandboxConfig) { c.secure = &secure }
}

// WithAllowInternetAccess 设置是否允许互联网访问。
func WithAllowInternetAccess(allow bool) SandboxOption {
	return func(c *sandboxConfig) { c.allowInternetAccess = &allow }
}

// WithNetwork 设置网络配置选项。
func WithNetwork(opts NetworkOpts) SandboxOption {
	return func(c *sandboxConfig) { c.network = &opts }
}

// WithAutoResume 设置自动恢复策略。
func WithAutoResume(policy AutoResumePolicy) SandboxOption {
	return func(c *sandboxConfig) { c.autoResume = &policy }
}

// WithVolumeMounts 设置卷挂载列表。
func WithVolumeMounts(mounts []VolumeMount) SandboxOption {
	return func(c *sandboxConfig) { c.volumeMounts = mounts }
}

// WithMCP 设置 MCP 配置。
func WithMCP(config MCPConfig) SandboxOption {
	return func(c *sandboxConfig) { c.mcp = config }
}

// applySandboxDefaults 为未设置的沙箱配置应用默认值。
func applySandboxDefaults(c *sandboxConfig) {
	if c.template == "" {
		c.template = DefaultTemplate
	}
	if c.timeout == 0 {
		c.timeout = DefaultSandboxTimeout
	}
}

// === 命令选项 ===

// commandConfig 包含命令执行的内部配置参数。
type commandConfig struct {
	cwd            string            // 工作目录
	user           string            // 执行用户
	envVars        map[string]string // 环境变量
	onStdout       func(string)      // 标准输出回调
	onStderr       func(string)      // 标准错误回调
	timeout        int               // 命令超时时间（秒）
	requestTimeout *time.Duration    // 请求超时时间
	stdin          bool              // 是否启用标准输入
}

// CommandOption 是用于配置命令执行的函数选项类型。
type CommandOption func(*commandConfig)

// WithCwd 设置命令执行的工作目录。
func WithCwd(cwd string) CommandOption {
	return func(c *commandConfig) { c.cwd = cwd }
}

// WithUser 设置命令执行的用户。
func WithUser(user string) CommandOption {
	return func(c *commandConfig) { c.user = user }
}

// WithCommandEnvVars 设置命令执行的环境变量。
func WithCommandEnvVars(envs map[string]string) CommandOption {
	return func(c *commandConfig) { c.envVars = envs }
}

// WithOnStdout 设置标准输出的回调函数。
func WithOnStdout(fn func(string)) CommandOption {
	return func(c *commandConfig) { c.onStdout = fn }
}

// WithOnStderr 设置标准错误的回调函数。
func WithOnStderr(fn func(string)) CommandOption {
	return func(c *commandConfig) { c.onStderr = fn }
}

// WithCommandTimeout 设置命令的超时时间（秒）。
func WithCommandTimeout(seconds int) CommandOption {
	return func(c *commandConfig) { c.timeout = seconds }
}

// WithCommandRequestTimeout 设置命令请求的超时时间。
func WithCommandRequestTimeout(d time.Duration) CommandOption {
	return func(c *commandConfig) { c.requestTimeout = &d }
}

// WithStdin 设置是否启用标准输入。
func WithStdin(enabled bool) CommandOption {
	return func(c *commandConfig) { c.stdin = enabled }
}

// === 代码执行选项 ===

// runCodeConfig 包含代码执行的内部配置参数。
type runCodeConfig struct {
	language     string                // 编程语言
	codeContext  *CodeContext          // 代码执行上下文
	envVars      map[string]string     // 环境变量
	timeout      float64               // 超时时间（秒）
	onStdout     func(OutputMessage)   // 标准输出回调
	onStderr     func(OutputMessage)   // 标准错误回调
	onResult     func(Result)          // 结果回调
	onError      func(ExecutionError)  // 错误回调
}

// RunCodeOption 是用于配置代码执行的函数选项类型。
type RunCodeOption func(*runCodeConfig)

// WithLanguage 设置代码执行使用的编程语言。
func WithLanguage(lang string) RunCodeOption {
	return func(c *runCodeConfig) { c.language = lang }
}

// WithCodeContext 设置代码执行上下文。
func WithCodeContext(ctx *CodeContext) RunCodeOption {
	return func(c *runCodeConfig) { c.codeContext = ctx }
}

// WithCodeEnvVars 设置代码执行的环境变量。
func WithCodeEnvVars(envs map[string]string) RunCodeOption {
	return func(c *runCodeConfig) { c.envVars = envs }
}

// WithCodeTimeout 设置代码执行的超时时间（秒）。
func WithCodeTimeout(seconds float64) RunCodeOption {
	return func(c *runCodeConfig) { c.timeout = seconds }
}

// WithOnCodeStdout 设置代码执行标准输出的回调函数。
func WithOnCodeStdout(fn func(OutputMessage)) RunCodeOption {
	return func(c *runCodeConfig) { c.onStdout = fn }
}

// WithOnCodeStderr 设置代码执行标准错误的回调函数。
func WithOnCodeStderr(fn func(OutputMessage)) RunCodeOption {
	return func(c *runCodeConfig) { c.onStderr = fn }
}

// WithOnResult 设置代码执行结果的回调函数。
func WithOnResult(fn func(Result)) RunCodeOption {
	return func(c *runCodeConfig) { c.onResult = fn }
}

// WithOnError 设置代码执行错误的回调函数。
func WithOnError(fn func(ExecutionError)) RunCodeOption {
	return func(c *runCodeConfig) { c.onError = fn }
}

// === 文件 URL 选项 ===

// fileURLConfig 包含文件 URL 生成的内部配置参数。
type fileURLConfig struct {
	user       string // 用户名
	expiration int    // 签名过期时间（秒）
}

// FileURLOption 是用于配置文件 URL 生成的函数选项类型。
type FileURLOption func(*fileURLConfig)

// WithFileUser 设置文件操作的用户名。
func WithFileUser(user string) FileURLOption {
	return func(c *fileURLConfig) { c.user = user }
}

// WithSignatureExpiration 设置签名的过期时间（秒）。
func WithSignatureExpiration(seconds int) FileURLOption {
	return func(c *fileURLConfig) { c.expiration = seconds }
}

// === 文件系统选项 ===

// filesystemConfig 包含文件系统操作的内部配置参数。
type filesystemConfig struct {
	user           string         // 用户名
	requestTimeout *time.Duration // 请求超时时间
	depth          int            // 目录列表深度
	recursive      bool           // 是否递归操作
}

// FilesystemOption 是用于配置文件系统操作的函数选项类型。
type FilesystemOption func(*filesystemConfig)

// WithFsUser 设置文件系统操作的用户名。
func WithFsUser(user string) FilesystemOption {
	return func(c *filesystemConfig) { c.user = user }
}

// WithFsRequestTimeout 设置文件系统请求的超时时间。
func WithFsRequestTimeout(d time.Duration) FilesystemOption {
	return func(c *filesystemConfig) { c.requestTimeout = &d }
}

// WithDepth 设置目录列表的深度。
func WithDepth(depth int) FilesystemOption {
	return func(c *filesystemConfig) { c.depth = depth }
}

// WithRecursive 设置是否递归操作目录。
func WithRecursive(recursive bool) FilesystemOption {
	return func(c *filesystemConfig) { c.recursive = recursive }
}
