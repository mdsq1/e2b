package e2b

import "fmt"

// SandboxError 是所有沙箱操作的基础错误类型。
type SandboxError struct {
	Message string // 错误信息
	Cause   error  // 原始错误
}

// Error 返回错误信息字符串。
func (e *SandboxError) Error() string { return e.Message }

// Unwrap 返回被包装的原始错误。
func (e *SandboxError) Unwrap() error { return e.Cause }

// 预定义的哨兵错误变量，可用于 errors.Is() 进行错误类型判断。
var (
	ErrTimeout   = &TimeoutError{SandboxError: SandboxError{Message: "timeout"}}                      // 超时错误
	ErrNotFound  = &NotFoundError{SandboxError: SandboxError{Message: "not found"}}                   // 资源未找到错误
	ErrAuth      = &AuthenticationError{SandboxError: SandboxError{Message: "authentication failed"}} // 认证失败错误
	ErrRateLimit = &RateLimitError{SandboxError: SandboxError{Message: "rate limit exceeded"}}        // 速率限制错误
)

// TimeoutError 表示操作超时错误。
type TimeoutError struct {
	SandboxError
}

// Is 实现 errors.Is 接口，用于判断目标错误是否为 TimeoutError 类型。
func (e *TimeoutError) Is(target error) bool {
	_, ok := target.(*TimeoutError)
	return ok
}

// NotFoundError 表示请求的资源未找到（HTTP 404）。
type NotFoundError struct {
	SandboxError
}

// Is 实现 errors.Is 接口，用于判断目标错误是否为 NotFoundError 类型。
func (e *NotFoundError) Is(target error) bool {
	_, ok := target.(*NotFoundError)
	return ok
}

// AuthenticationError 表示认证失败错误（HTTP 401）。
type AuthenticationError struct {
	SandboxError
}

// Is 实现 errors.Is 接口，用于判断目标错误是否为 AuthenticationError 类型。
func (e *AuthenticationError) Is(target error) bool {
	_, ok := target.(*AuthenticationError)
	return ok
}

// InvalidArgumentError 表示无效参数错误（HTTP 400）。
type InvalidArgumentError struct {
	SandboxError
}

// Is 实现 errors.Is 接口，用于判断目标错误是否为 InvalidArgumentError 类型。
func (e *InvalidArgumentError) Is(target error) bool {
	_, ok := target.(*InvalidArgumentError)
	return ok
}

// NotEnoughSpaceError 表示磁盘空间不足错误（HTTP 507）。
type NotEnoughSpaceError struct {
	SandboxError
}

// Is 实现 errors.Is 接口，用于判断目标错误是否为 NotEnoughSpaceError 类型。
func (e *NotEnoughSpaceError) Is(target error) bool {
	_, ok := target.(*NotEnoughSpaceError)
	return ok
}

// RateLimitError 表示请求速率超限错误（HTTP 429）。
type RateLimitError struct {
	SandboxError
}

// Is 实现 errors.Is 接口，用于判断目标错误是否为 RateLimitError 类型。
func (e *RateLimitError) Is(target error) bool {
	_, ok := target.(*RateLimitError)
	return ok
}

// CommandExitError 在命令以非零退出码退出时返回。
type CommandExitError struct {
	Stdout   string // 标准输出内容
	Stderr   string // 标准错误内容
	ExitCode int    // 退出码
	Message  string // 错误信息
}

// Error 返回格式化的错误信息，包含退出码和错误消息。
func (e *CommandExitError) Error() string {
	return fmt.Sprintf("command exited with code %d: %s", e.ExitCode, e.Message)
}

// GitAuthError 在 Git 认证失败时返回。
type GitAuthError struct {
	SandboxError
}

// Is 实现 errors.Is 接口，用于判断目标错误是否为 GitAuthError 类型。
func (e *GitAuthError) Is(target error) bool {
	_, ok := target.(*GitAuthError)
	return ok
}

// GitUpstreamError 在 Git 上游分支缺失时返回。
type GitUpstreamError struct {
	SandboxError
}

// Is 实现 errors.Is 接口，用于判断目标错误是否为 GitUpstreamError 类型。
func (e *GitUpstreamError) Is(target error) bool {
	_, ok := target.(*GitUpstreamError)
	return ok
}

// BuildError 在模板构建失败时返回。
type BuildError struct {
	SandboxError
	BuildID    string // 构建 ID
	TemplateID string // 模板 ID
}

// Is 实现 errors.Is 接口，用于判断目标错误是否为 BuildError 类型。
func (e *BuildError) Is(target error) bool {
	_, ok := target.(*BuildError)
	return ok
}

// FileUploadError 在模板构建期间文件上传失败时返回。
type FileUploadError struct {
	SandboxError
}

// Is 实现 errors.Is 接口，用于判断目标错误是否为 FileUploadError 类型。
func (e *FileUploadError) Is(target error) bool {
	_, ok := target.(*FileUploadError)
	return ok
}

// TemplateError 在模板管理操作失败时返回。
type TemplateError struct {
	SandboxError
}

// Is 实现 errors.Is 接口，用于判断目标错误是否为 TemplateError 类型。
func (e *TemplateError) Is(target error) bool {
	_, ok := target.(*TemplateError)
	return ok
}

// ForbiddenError 在调用者缺少权限时返回（HTTP 403）。
type ForbiddenError struct {
	SandboxError
}

// Is 实现 errors.Is 接口，用于判断目标错误是否为 ForbiddenError 类型。
func (e *ForbiddenError) Is(target error) bool {
	_, ok := target.(*ForbiddenError)
	return ok
}

// ConnectError 在 Connect RPC 协议错误时返回。
type ConnectError struct {
	Code    string // 错误码
	Message string // 错误信息
}

// Error 返回格式化的 Connect RPC 错误信息。
func (e *ConnectError) Error() string {
	return fmt.Sprintf("connect rpc error [%s]: %s", e.Code, e.Message)
}

// Connect RPC 错误码常量。
const (
	ConnectCodeNotFound      = "not_found"      // 资源未找到
	ConnectCodeAlreadyExists = "already_exists" // 资源已存在
)

// mapHTTPError 将 HTTP 状态码映射为对应的错误类型。
func mapHTTPError(statusCode int, body string) error {
	switch statusCode {
	case 400:
		return &InvalidArgumentError{SandboxError{Message: body, Cause: nil}}
	case 401:
		return &AuthenticationError{SandboxError{Message: body, Cause: nil}}
	case 403:
		return &ForbiddenError{SandboxError{Message: body, Cause: nil}}
	case 404:
		return &NotFoundError{SandboxError{Message: body, Cause: nil}}
	case 429:
		return &RateLimitError{SandboxError{Message: body, Cause: nil}}
	case 502:
		return &TimeoutError{SandboxError{Message: "sandbox is likely not running", Cause: nil}}
	case 507:
		return &NotEnoughSpaceError{SandboxError{Message: body, Cause: nil}}
	default:
		return &SandboxError{Message: fmt.Sprintf("HTTP %d: %s", statusCode, body)}
	}
}
