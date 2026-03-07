package e2b

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/mdsq1/e2b/internal/connectrpc"
)

// Pty 提供沙箱中的伪终端操作功能。
type Pty struct {
	sandbox *Sandbox           // 所属沙箱实例
	rpc     *connectrpc.Client // RPC 客户端
}

// newPty 创建一个新的伪终端模块。
func newPty(sbx *Sandbox, rpc *connectrpc.Client) *Pty {
	return &Pty{sandbox: sbx, rpc: rpc}
}

// PtyEvent 表示伪终端的输出事件。
type PtyEvent struct {
	Data []byte // 输出数据
}

// PtyHandle 表示一个正在运行的伪终端会话句柄。
type PtyHandle struct {
	PID    int                // 进程 ID
	events chan PtyEvent      // 输出事件通道
	done   chan struct{}      // 会话结束信号通道
	rpc    *connectrpc.Client // RPC 客户端
}

// Events 返回接收伪终端输出事件的通道。
func (h *PtyHandle) Events() <-chan PtyEvent {
	return h.events
}

// Wait 阻塞等待伪终端会话结束。
func (h *PtyHandle) Wait() {
	<-h.done
}

// Kill 向伪终端进程发送 SIGKILL 信号。
// 与 Python SDK 对齐：进程不存在时返回 (false, nil)。
func (h *PtyHandle) Kill(ctx context.Context) (bool, error) {
	req := sendSignalRequest{Process: processSelector{PID: h.PID}, Signal: "SIGNAL_SIGKILL"}
	err := h.rpc.CallUnary(ctx, processServiceName, "SendSignal", req, nil)
	if err != nil {
		if connErr, ok := err.(*connectrpc.Error); ok && connErr.Code == ConnectCodeNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// SendStdin 向伪终端发送数据（使用 pty 字段，与 Python SDK 对齐）。
func (h *PtyHandle) SendStdin(ctx context.Context, data []byte) error {
	req := newSendPtyRequest(h.PID, data)
	return h.rpc.CallUnary(ctx, processServiceName, "SendInput", req, nil)
}

// Resize 更改伪终端的终端尺寸。
func (h *PtyHandle) Resize(ctx context.Context, size PtySize) error {
	req := updateRequest{
		Process: processSelector{PID: h.PID},
		Pty: &ptyConfig{
			Size: &ptySizeMsg{Cols: size.Cols, Rows: size.Rows},
		},
	}
	return h.rpc.CallUnary(ctx, processServiceName, "Update", req, nil)
}

// updateRequest 是更新进程（如调整终端尺寸）的 RPC 请求（与 protobuf UpdateRequest 对齐）。
type updateRequest struct {
	Process processSelector `json:"process"`
	Pty     *ptyConfig      `json:"pty,omitempty"`
}

// Create 创建一个新的伪终端会话。
// 与 Python SDK 对齐：使用 bash [-i,-l] 参数，注入 TERM/LANG/LC_ALL 默认环境变量，通过 Authorization 头传递用户身份。
func (p *Pty) Create(ctx context.Context, size PtySize, opts ...CommandOption) (*PtyHandle, error) {
	cfg := &commandConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	user := p.sandbox.resolveUsername(cfg.user)
	authHeader := buildAuthHeader(user)

	// 与 Python SDK 对齐：设置终端所需的默认环境变量
	envs := map[string]string{
		"TERM":   "xterm-256color",
		"LANG":   "C.UTF-8",
		"LC_ALL": "C.UTF-8",
	}
	for k, v := range cfg.envVars {
		envs[k] = v
	}

	req := startRequest{
		Process: processConfig{
			Cmd:  "/bin/bash",
			Args: []string{"-i", "-l"}, // 与 Python SDK 对齐：交互式登录 shell
			Envs: envs,
			Cwd:  cfg.cwd,
		},
		Pty: &ptyConfig{
			Size: &ptySizeMsg{Cols: size.Cols, Rows: size.Rows},
		},
	}

	timeoutMs := 0
	if cfg.timeout > 0 {
		timeoutMs = cfg.timeout * 1000
	}

	stream, err := p.rpc.CallServerStream(ctx, processServiceName, "Start", req, timeoutMs, authHeader)
	if err != nil {
		return nil, &SandboxError{Message: fmt.Sprintf("failed to create PTY: %v", err), Cause: err}
	}

	// 读取第一个事件以获取 PID
	var firstEvent processEvent
	if err := stream.Next(&firstEvent); err != nil {
		stream.Close()
		return nil, &SandboxError{Message: fmt.Sprintf("failed to read PTY start event: %v", err), Cause: err}
	}

	pid := 0
	if firstEvent.Event.Start != nil {
		pid = firstEvent.Event.Start.PID
	}

	events := make(chan PtyEvent, 64)
	done := make(chan struct{})

	handle := &PtyHandle{
		PID:    pid,
		events: events,
		done:   done,
		rpc:    p.rpc,
	}

	var once sync.Once
	go func() {
		defer stream.Close()
		defer once.Do(func() { close(done) })
		defer close(events)

		for {
			var event processEvent
			err := stream.Next(&event)
			if err == io.EOF {
				return
			}
			if err != nil {
				return
			}

			if event.Event.Data != nil && len(event.Event.Data.Pty) > 0 {
				select {
				case events <- PtyEvent{Data: event.Event.Data.Pty}:
				default:
					// 通道已满时丢弃事件
				}
			}

			if event.Event.End != nil {
				return
			}
		}
	}()

	return handle, nil
}

// Kill 向 PTY 进程发送 SIGKILL 信号。
// 与 Python SDK 对齐：进程不存在时返回 (false, nil)。
func (p *Pty) Kill(ctx context.Context, pid int) (bool, error) {
	req := sendSignalRequest{Process: processSelector{PID: pid}, Signal: "SIGNAL_SIGKILL"}
	err := p.rpc.CallUnary(ctx, processServiceName, "SendSignal", req, nil)
	if err != nil {
		if connErr, ok := err.(*connectrpc.Error); ok && connErr.Code == ConnectCodeNotFound {
			return false, nil
		}
		return false, &SandboxError{Message: fmt.Sprintf("failed to kill pty %d: %v", pid, err), Cause: err}
	}
	return true, nil
}

// Connect 连接到已运行的 PTY 进程。
// 与 Python SDK 的 pty.connect() 对齐。
func (p *Pty) Connect(ctx context.Context, pid int, opts ...CommandOption) (*PtyHandle, error) {
	cfg := &commandConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	req := connectRequest{Process: processSelector{PID: pid}}
	timeoutMs := 0
	if cfg.timeout > 0 {
		timeoutMs = cfg.timeout * 1000
	}

	stream, err := p.rpc.CallServerStream(ctx, processServiceName, "Connect", req, timeoutMs)
	if err != nil {
		return nil, &SandboxError{Message: fmt.Sprintf("failed to connect to PTY %d: %v", pid, err), Cause: err}
	}

	// 读取首个事件以获取 PID
	var firstEvent processEvent
	if err := stream.Next(&firstEvent); err != nil {
		stream.Close()
		return nil, &SandboxError{Message: fmt.Sprintf("failed to read PTY connect start event: %v", err), Cause: err}
	}

	events := make(chan PtyEvent, 64)
	done := make(chan struct{})

	handle := &PtyHandle{
		PID:    pid,
		events: events,
		done:   done,
		rpc:    p.rpc,
	}

	var once sync.Once
	go func() {
		defer stream.Close()
		defer once.Do(func() { close(done) })
		defer close(events)

		for {
			var event processEvent
			err := stream.Next(&event)
			if err == io.EOF {
				return
			}
			if err != nil {
				return
			}

			if event.Event.Data != nil && len(event.Event.Data.Pty) > 0 {
				select {
				case events <- PtyEvent{Data: event.Event.Data.Pty}:
				default:
				}
			}

			if event.Event.End != nil {
				return
			}
		}
	}()

	return handle, nil
}

// Resize 通过 PID 更改伪终端的终端尺寸。
func (p *Pty) Resize(ctx context.Context, pid int, size PtySize) error {
	req := updateRequest{
		Process: processSelector{PID: pid},
		Pty: &ptyConfig{
			Size: &ptySizeMsg{Cols: size.Cols, Rows: size.Rows},
		},
	}
	return p.rpc.CallUnary(ctx, processServiceName, "Update", req, nil)
}

// SendStdin 通过 PID 向伪终端发送数据（使用 pty 字段，与 Python SDK 对齐）。
func (p *Pty) SendStdin(ctx context.Context, pid int, data []byte) error {
	req := newSendPtyRequest(pid, data)
	return p.rpc.CallUnary(ctx, processServiceName, "SendInput", req, nil)
}
