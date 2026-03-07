package e2b

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/mdsq1/e2b/internal/connectrpc"
)

// processServiceName 是进程管理 RPC 服务的名称。
const processServiceName = "e2b.process.v1.ProcessService"

// Commands 提供沙箱中的命令执行功能。
type Commands struct {
	sandbox *Sandbox           // 所属沙箱实例
	rpc     *connectrpc.Client // RPC 客户端
}

// newCommands 创建一个新的命令执行模块。
func newCommands(sbx *Sandbox, rpc *connectrpc.Client) *Commands {
	return &Commands{sandbox: sbx, rpc: rpc}
}

// RPC 请求/响应类型（内部使用，匹配 protobuf JSON 编码）

// processConfig 包含进程的配置信息。
type processConfig struct {
	Cmd  string            `json:"cmd"`
	Args []string          `json:"args"`
	Envs map[string]string `json:"envs,omitempty"`
	Cwd  string            `json:"cwd,omitempty"`
}

// startRequest 是启动进程的 RPC 请求。
type startRequest struct {
	Process processConfig `json:"process"`
	Pty     *ptyConfig    `json:"pty,omitempty"`
	Tag     string        `json:"tag,omitempty"`
	Stdin   bool          `json:"stdin,omitempty"`
}

// ptyConfig 包含伪终端的配置。
type ptyConfig struct {
	Size *ptySizeMsg `json:"size,omitempty"`
}

// ptySizeMsg 包含伪终端的尺寸信息。
type ptySizeMsg struct {
	Cols int `json:"cols"`
	Rows int `json:"rows"`
}

// connectRequest 是连接已有进程的 RPC 请求（与 protobuf ConnectRequest 对齐）。
type connectRequest struct {
	Process processSelector `json:"process"`
}

// processEvent 是进程事件的包装结构。
type processEvent struct {
	Event processEventData `json:"event"`
}

// processEventData 包含进程事件的具体数据。
type processEventData struct {
	Start     *processStartEvent `json:"start,omitempty"`
	Data      *processDataEvent  `json:"data,omitempty"`
	End       *processEndEvent   `json:"end,omitempty"`
	Keepalive *struct{}          `json:"keepalive,omitempty"`
}

// processStartEvent 表示进程启动事件。
type processStartEvent struct {
	PID int `json:"pid"`
}

// processDataEvent 表示进程数据输出事件。
type processDataEvent struct {
	Stdout []byte `json:"stdout,omitempty"` // 标准输出（JSON 中为 base64 编码）
	Stderr []byte `json:"stderr,omitempty"` // 标准错误（JSON 中为 base64 编码）
	Pty    []byte `json:"pty,omitempty"`    // 伪终端输出（JSON 中为 base64 编码）
}

// UnmarshalJSON 自定义反序列化，处理 base64 编码的字节字段。
func (d *processDataEvent) UnmarshalJSON(data []byte) error {
	var raw struct {
		Stdout string `json:"stdout,omitempty"`
		Stderr string `json:"stderr,omitempty"`
		Pty    string `json:"pty,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw.Stdout != "" {
		b, err := base64.StdEncoding.DecodeString(raw.Stdout)
		if err != nil {
			return err
		}
		d.Stdout = b
	}
	if raw.Stderr != "" {
		b, err := base64.StdEncoding.DecodeString(raw.Stderr)
		if err != nil {
			return err
		}
		d.Stderr = b
	}
	if raw.Pty != "" {
		b, err := base64.StdEncoding.DecodeString(raw.Pty)
		if err != nil {
			return err
		}
		d.Pty = b
	}
	return nil
}

// processEndEvent 表示进程结束事件。
type processEndEvent struct {
	ExitCode int    `json:"exitCode"`        // 退出码
	Error    string `json:"error,omitempty"` // 错误信息
}

// listProcessesResponse 是列出进程的 RPC 响应。
type listProcessesResponse struct {
	Processes []processInfoRaw `json:"processes"`
}

// processInfoRaw 是原始进程信息结构（含嵌套配置）。
type processInfoRaw struct {
	PID    int           `json:"pid"`
	Tag    string        `json:"tag"`
	Config processConfig `json:"config"`
}

// processSelector 表示进程选择器，与 protobuf ProcessSelector 对齐。
// 与 Python SDK 对齐：所有进程操作请求都使用嵌套的 process 字段。
type processSelector struct {
	PID int `json:"pid"`
}

// processInputData 表示进程输入数据，支持 stdin 和 pty 两种字段。
// stdin 用于普通命令标准输入，pty 用于伪终端输入。
type processInputData struct {
	Stdin string `json:"stdin,omitempty"` // base64 编码，用于普通命令
	Pty   string `json:"pty,omitempty"`   // base64 编码，用于 PTY
}

// sendSignalRequest 是发送信号的 RPC 请求（与 protobuf SendSignalRequest 对齐）。
type sendSignalRequest struct {
	Process processSelector `json:"process"`
	Signal  string          `json:"signal"`
}

// sendInputRequest 是发送输入数据的 RPC 请求（与 protobuf SendInputRequest 对齐）。
type sendInputRequest struct {
	Process processSelector  `json:"process"`
	Input   processInputData `json:"input"`
}

// newSendStdinRequest 构造用于普通命令 stdin 的发送输入请求。
func newSendStdinRequest(pid int, data []byte) sendInputRequest {
	return sendInputRequest{
		Process: processSelector{PID: pid},
		Input:   processInputData{Stdin: base64.StdEncoding.EncodeToString(data)},
	}
}

// newSendPtyRequest 构造用于 PTY 的发送输入请求。
func newSendPtyRequest(pid int, data []byte) sendInputRequest {
	return sendInputRequest{
		Process: processSelector{PID: pid},
		Input:   processInputData{Pty: base64.StdEncoding.EncodeToString(data)},
	}
}

// closeStdinRequest 是关闭标准输入的 RPC 请求（与 protobuf CloseStdinRequest 对齐）。
type closeStdinRequest struct {
	Process processSelector `json:"process"`
}

// Run 在前台执行命令，阻塞直到命令完成并返回结果。
func (c *Commands) Run(ctx context.Context, cmd string, opts ...CommandOption) (*CommandResult, error) {
	cfg := c.applyOpts(opts)
	stream, pid, err := c.startStream(ctx, cmd, cfg, false)
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	return c.consumeStream(stream, pid, cfg, nil, nil, nil)
}

// Start 在后台启动命令并立即返回命令句柄。
func (c *Commands) Start(ctx context.Context, cmd string, opts ...CommandOption) (*CommandHandle, error) {
	cfg := c.applyOpts(opts)
	stream, pid, err := c.startStream(ctx, cmd, cfg, false)
	if err != nil {
		return nil, err
	}

	handle := &CommandHandle{
		PID:     pid,
		done:    make(chan struct{}),
		sandbox: c.sandbox,
		rpc:     c.rpc,
	}

	go func() {
		defer stream.Close()
		result, streamErr := c.consumeStream(stream, pid, cfg, &handle.stdout, &handle.stderr, &handle.mu)
		handle.mu.Lock()
		handle.result = result
		handle.err = streamErr
		handle.mu.Unlock()
		close(handle.done)
	}()

	return handle, nil
}

// Connect 通过 PID 连接到已在运行的进程。
func (c *Commands) Connect(ctx context.Context, pid int, opts ...CommandOption) (*CommandHandle, error) {
	cfg := c.applyOpts(opts)

	req := connectRequest{Process: processSelector{PID: pid}}
	timeoutMs := 0
	if cfg.timeout > 0 {
		timeoutMs = cfg.timeout * 1000
	}

	stream, err := c.rpc.CallServerStream(ctx, processServiceName, "Connect", req, timeoutMs)
	if err != nil {
		return nil, &SandboxError{Message: fmt.Sprintf("failed to connect to process %d: %v", pid, err), Cause: err}
	}

	handle := &CommandHandle{
		PID:     pid,
		done:    make(chan struct{}),
		sandbox: c.sandbox,
		rpc:     c.rpc,
	}

	go func() {
		defer stream.Close()
		result, streamErr := c.consumeStream(stream, pid, cfg, &handle.stdout, &handle.stderr, &handle.mu)
		handle.mu.Lock()
		handle.result = result
		handle.err = streamErr
		handle.mu.Unlock()
		close(handle.done)
	}()

	return handle, nil
}

// List 返回沙箱中所有正在运行的进程。
func (c *Commands) List(ctx context.Context) ([]ProcessInfo, error) {
	var resp listProcessesResponse
	err := c.rpc.CallUnary(ctx, processServiceName, "List", struct{}{}, &resp)
	if err != nil {
		return nil, &SandboxError{Message: fmt.Sprintf("failed to list processes: %v", err), Cause: err}
	}

	processes := make([]ProcessInfo, len(resp.Processes))
	for i, p := range resp.Processes {
		processes[i] = ProcessInfo{
			PID:  p.PID,
			Tag:  p.Tag,
			Cmd:  p.Config.Cmd,
			Args: p.Config.Args,
			Envs: p.Config.Envs,
			Cwd:  p.Config.Cwd,
		}
	}
	return processes, nil
}

// Kill 向进程发送 SIGKILL 信号。
// 与 Python SDK 对齐：进程不存在（not_found）时返回 (false, nil)。
func (c *Commands) Kill(ctx context.Context, pid int) (bool, error) {
	req := sendSignalRequest{Process: processSelector{PID: pid}, Signal: "SIGNAL_SIGKILL"}
	err := c.rpc.CallUnary(ctx, processServiceName, "SendSignal", req, nil)
	if err != nil {
		if connErr, ok := err.(*connectrpc.Error); ok && connErr.Code == ConnectCodeNotFound {
			return false, nil
		}
		return false, &SandboxError{Message: fmt.Sprintf("failed to kill process %d: %v", pid, err), Cause: err}
	}
	return true, nil
}

// SendStdin 向进程的标准输入发送数据。
func (c *Commands) SendStdin(ctx context.Context, pid int, data string) error {
	req := newSendStdinRequest(pid, []byte(data))
	err := c.rpc.CallUnary(ctx, processServiceName, "SendInput", req, nil)
	if err != nil {
		return &SandboxError{Message: fmt.Sprintf("failed to send stdin to process %d: %v", pid, err), Cause: err}
	}
	return nil
}

// applyOpts 应用命令选项并返回配置。
func (c *Commands) applyOpts(opts []CommandOption) *commandConfig {
	cfg := &commandConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// startStream 启动命令并返回事件流和进程 PID。
func (c *Commands) startStream(ctx context.Context, cmd string, cfg *commandConfig, isPty bool) (*connectrpc.StreamReader, int, error) {
	user := c.sandbox.resolveUsername(cfg.user)

	// 与 Python SDK 对齐：不强制设置默认 cwd，由服务端决定（通常为用户 home 目录）。
	req := startRequest{
		Process: processConfig{
			Cmd:  "/bin/bash",
			Args: []string{"-l", "-c", cmd},
			Envs: cfg.envVars,
			Cwd:  cfg.cwd,
		},
		Stdin: cfg.stdin && c.sandbox.supportsStdin(),
	}

	// 与 Python SDK 对齐：默认超时 60s；显式 WithCommandTimeout(0) 表示无限制。
	timeout := cfg.timeout
	if !cfg.timeoutSet {
		timeout = DefaultCommandTimeout
	}
	timeoutMs := 0
	if timeout > 0 {
		timeoutMs = timeout * 1000
	}

	// 与 Python SDK 对齐：通过 Authorization: Basic 头传递用户身份，
	// 而不是用 su 命令切换用户（Python: authentication_header(version, user)）。
	authHeader := buildAuthHeader(user)

	stream, err := c.rpc.CallServerStream(ctx, processServiceName, "Start", req, timeoutMs, authHeader)
	if err != nil {
		return nil, 0, &SandboxError{Message: fmt.Sprintf("failed to start command: %v", err), Cause: err}
	}

	// 读取第一个事件以获取 PID
	var firstEvent processEvent
	if err := stream.Next(&firstEvent); err != nil {
		stream.Close()
		return nil, 0, &SandboxError{Message: fmt.Sprintf("failed to read start event: %v", err), Cause: err}
	}

	pid := 0
	if firstEvent.Event.Start != nil {
		pid = firstEvent.Event.Start.PID
	}

	return stream, pid, nil
}

// consumeStream 消费进程事件流，收集输出并返回命令结果。
// stdout 和 stderr 是外部传入的缓冲区，若为 nil 则内部创建。
// mu 可选，用于在写入 stdout/stderr 时加锁（用于并发场景）。
func (c *Commands) consumeStream(stream *connectrpc.StreamReader, pid int, cfg *commandConfig, stdout, stderr *strings.Builder, mu *sync.Mutex) (*CommandResult, error) {
	if stdout == nil {
		stdout = &strings.Builder{}
	}
	if stderr == nil {
		stderr = &strings.Builder{}
	}

	for {
		var event processEvent
		err := stream.Next(&event)
		if err == io.EOF {
			break
		}
		if err != nil {
			// 检查是否为 Connect 错误
			return nil, &SandboxError{Message: fmt.Sprintf("stream error: %v", err), Cause: err}
		}

		if event.Event.Start != nil && pid == 0 {
			pid = event.Event.Start.PID
		}

		if event.Event.Data != nil {
			if len(event.Event.Data.Stdout) > 0 {
				s := string(event.Event.Data.Stdout)
				if mu != nil {
					mu.Lock()
				}
				stdout.WriteString(s)
				if mu != nil {
					mu.Unlock()
				}
				if cfg.onStdout != nil {
					cfg.onStdout(s)
				}
			}
			if len(event.Event.Data.Stderr) > 0 {
				s := string(event.Event.Data.Stderr)
				if mu != nil {
					mu.Lock()
				}
				stderr.WriteString(s)
				if mu != nil {
					mu.Unlock()
				}
				if cfg.onStderr != nil {
					cfg.onStderr(s)
				}
			}
		}

		if event.Event.End != nil {
			result := &CommandResult{
				Stdout:   stdout.String(),
				Stderr:   stderr.String(),
				ExitCode: event.Event.End.ExitCode,
				Error:    event.Event.End.Error,
			}
			if result.ExitCode != 0 {
				return result, &CommandExitError{
					Stdout:   result.Stdout,
					Stderr:   result.Stderr,
					ExitCode: result.ExitCode,
					Message:  result.Error,
				}
			}
			return result, nil
		}
	}

	return &CommandResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}, nil
}

// CommandHandle 表示一个正在后台运行的命令句柄。
type CommandHandle struct {
	PID int // 进程 ID

	mu      sync.Mutex         // 互斥锁，保护并发访问
	stdout  strings.Builder    // 标准输出缓冲
	stderr  strings.Builder    // 标准错误缓冲
	done    chan struct{}      // 命令完成信号通道
	result  *CommandResult     // 命令执行结果
	err     error              // 执行错误
	sandbox *Sandbox           // 所属沙箱
	rpc     *connectrpc.Client // RPC 客户端
}

// Stdout 返回目前为止累积的标准输出。
func (h *CommandHandle) Stdout() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.result != nil {
		return h.result.Stdout
	}
	return h.stdout.String()
}

// Stderr 返回目前为止累积的标准错误输出。
func (h *CommandHandle) Stderr() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.result != nil {
		return h.result.Stderr
	}
	return h.stderr.String()
}

// Wait 阻塞等待命令完成并返回结果。
func (h *CommandHandle) Wait(ctx context.Context) (*CommandResult, error) {
	select {
	case <-h.done:
		h.mu.Lock()
		defer h.mu.Unlock()
		return h.result, h.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Kill 向命令进程发送 SIGKILL 信号。
func (h *CommandHandle) Kill(ctx context.Context) error {
	req := sendSignalRequest{Process: processSelector{PID: h.PID}, Signal: "SIGNAL_SIGKILL"}
	return h.rpc.CallUnary(ctx, processServiceName, "SendSignal", req, nil)
}

// SendStdin 向命令的标准输入发送数据。
func (h *CommandHandle) SendStdin(ctx context.Context, data string) error {
	req := newSendStdinRequest(h.PID, []byte(data))
	return h.rpc.CallUnary(ctx, processServiceName, "SendInput", req, nil)
}
