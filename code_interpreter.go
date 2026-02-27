package e2b

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// CodeInterpreter 嵌入 *Sandbox 并添加代码执行能力。
type CodeInterpreter struct {
	*Sandbox                      // 嵌入的沙箱实例
	jupyterURL  string            // Jupyter 服务 URL
	jupyterHTTP *http.Client      // Jupyter HTTP 客户端
}

// CreateCodeInterpreter 创建一个新的代码解释器沙箱。
func (c *Client) CreateCodeInterpreter(ctx context.Context, opts ...SandboxOption) (*CodeInterpreter, error) {
	// 将代码解释器模板作为默认模板前置
	allOpts := append([]SandboxOption{WithTemplate(DefaultCodeInterpreterTemplate)}, opts...)
	sbx, err := c.CreateSandbox(ctx, allOpts...)
	if err != nil {
		return nil, err
	}
	return newCodeInterpreter(sbx), nil
}

// newCodeInterpreter 根据已有沙箱创建代码解释器实例。
func newCodeInterpreter(sbx *Sandbox) *CodeInterpreter {
	scheme := "https"
	if sbx.client.config.Debug {
		scheme = "http"
	}
	host := sbx.client.config.GetHost(sbx.ID, sbx.SandboxDomain, JupyterPort)

	return &CodeInterpreter{
		Sandbox:    sbx,
		jupyterURL: fmt.Sprintf("%s://%s", scheme, host),
		jupyterHTTP: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

// executeRequest 是 /execute 端点的请求体。
type executeRequest struct {
	Code    string            `json:"code"`                // 要执行的代码
	Context *string           `json:"context_id,omitempty"` // 执行上下文 ID
	EnvVars map[string]string `json:"env_vars,omitempty"`  // 环境变量
	Timeout *float64          `json:"timeout,omitempty"`   // 超时时间（秒）
}

// streamEvent 表示 /execute 流式响应中的单行事件。
type streamEvent struct {
	Type string `json:"type"` // 事件类型

	// 标准输出/标准错误字段
	Text      string `json:"text,omitempty"`      // 输出文本
	Timestamp int64  `json:"timestamp,omitempty"` // 时间戳

	// 结果字段
	IsMainResult bool                   `json:"is_main_result,omitempty"` // 是否为主要结果
	ResultText   *string                `json:"text_result,omitempty"`    // 文本结果
	HTML         *string                `json:"html,omitempty"`           // HTML 输出
	Markdown     *string                `json:"markdown,omitempty"`       // Markdown 输出
	SVG          *string                `json:"svg,omitempty"`            // SVG 输出
	PNG          *string                `json:"png,omitempty"`            // PNG 输出
	JPEG         *string                `json:"jpeg,omitempty"`           // JPEG 输出
	PDF          *string                `json:"pdf,omitempty"`            // PDF 输出
	LaTeX        *string                `json:"latex,omitempty"`          // LaTeX 输出
	JSON         map[string]interface{} `json:"json,omitempty"`           // JSON 输出
	JavaScript   *string                `json:"javascript,omitempty"`     // JavaScript 输出
	Data         map[string]interface{} `json:"data,omitempty"`           // 原始数据
	Chart        *Chart                 `json:"chart,omitempty"`          // 图表数据
	Extra        map[string]interface{} `json:"extra,omitempty"`          // 额外数据

	// 错误字段
	Name      string `json:"name,omitempty"`      // 错误名称
	Value     string `json:"value,omitempty"`     // 错误值
	Traceback string `json:"traceback,omitempty"` // 错误堆栈

	// 执行计数
	ExecutionCount *int `json:"execution_count,omitempty"` // 执行次数
}

// RunCode 在代码解释器中执行代码并返回结果。
func (ci *CodeInterpreter) RunCode(ctx context.Context, code string, opts ...RunCodeOption) (*Execution, error) {
	cfg := &runCodeConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	reqBody := executeRequest{
		Code:    code,
		EnvVars: cfg.envVars,
	}
	if cfg.codeContext != nil {
		reqBody.Context = &cfg.codeContext.ID
	}
	if cfg.timeout > 0 {
		reqBody.Timeout = &cfg.timeout
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, &SandboxError{Message: fmt.Sprintf("failed to marshal request: %v", err), Cause: err}
	}

	url := ci.jupyterURL + "/execute"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, &SandboxError{Message: fmt.Sprintf("failed to create request: %v", err), Cause: err}
	}
	req.Header.Set("Content-Type", "application/json")
	ci.Sandbox.setSandboxHeaders(req)

	resp, err := ci.jupyterHTTP.Do(req)
	if err != nil {
		return nil, &SandboxError{Message: fmt.Sprintf("execute request failed: %v", err), Cause: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, mapHTTPError(resp.StatusCode, string(body))
	}

	exec := &Execution{
		Logs: ExecutionLogs{
			Stdout: []string{},
			Stderr: []string{},
		},
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var event streamEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}

		switch event.Type {
		case "stdout":
			exec.Logs.Stdout = append(exec.Logs.Stdout, event.Text)
			if cfg.onStdout != nil {
				cfg.onStdout(OutputMessage{
					Line:      event.Text,
					Timestamp: event.Timestamp,
					Error:     false,
				})
			}

		case "stderr":
			exec.Logs.Stderr = append(exec.Logs.Stderr, event.Text)
			if cfg.onStderr != nil {
				cfg.onStderr(OutputMessage{
					Line:      event.Text,
					Timestamp: event.Timestamp,
					Error:     true,
				})
			}

		case "result":
			result := ci.parseResult(&event)
			exec.Results = append(exec.Results, result)
			if cfg.onResult != nil {
				cfg.onResult(result)
			}

		case "error":
			execErr := ExecutionError{
				Name:      event.Name,
				Value:     event.Value,
				Traceback: event.Traceback,
			}
			exec.Error = &execErr
			if cfg.onError != nil {
				cfg.onError(execErr)
			}

		case "number_of_executions":
			exec.ExecutionCount = event.ExecutionCount
		}
	}

	if err := scanner.Err(); err != nil {
		return exec, &SandboxError{Message: fmt.Sprintf("error reading stream: %v", err), Cause: err}
	}

	return exec, nil
}

// parseResult 将流事件解析为执行结果。
func (ci *CodeInterpreter) parseResult(event *streamEvent) Result {
	r := Result{
		IsMainResult: event.IsMainResult,
		HTML:         event.HTML,
		Markdown:     event.Markdown,
		SVG:          event.SVG,
		PNG:          event.PNG,
		JPEG:         event.JPEG,
		PDF:          event.PDF,
		LaTeX:        event.LaTeX,
		JSON:         event.JSON,
		JavaScript:   event.JavaScript,
		Data:         event.Data,
		Chart:        event.Chart,
		Extra:        event.Extra,
	}
	// 结果类型的 text 字段来自事件 JSON 中的 "text" 键
	if event.Text != "" {
		text := event.Text
		r.Text = &text
	}
	return r
}

// CreateCodeContext 创建一个新的有状态代码执行上下文。
func (ci *CodeInterpreter) CreateCodeContext(ctx context.Context, opts ...RunCodeOption) (*CodeContext, error) {
	cfg := &runCodeConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	reqBody := map[string]string{}
	if cfg.language != "" {
		reqBody["language"] = cfg.language
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, &SandboxError{Message: fmt.Sprintf("failed to marshal request: %v", err), Cause: err}
	}

	url := ci.jupyterURL + "/contexts"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, &SandboxError{Message: fmt.Sprintf("failed to create request: %v", err), Cause: err}
	}
	req.Header.Set("Content-Type", "application/json")
	ci.Sandbox.setSandboxHeaders(req)

	resp, err := ci.jupyterHTTP.Do(req)
	if err != nil {
		return nil, &SandboxError{Message: fmt.Sprintf("request failed: %v", err), Cause: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, mapHTTPError(resp.StatusCode, string(body))
	}

	var codeCtx CodeContext
	if err := json.NewDecoder(resp.Body).Decode(&codeCtx); err != nil {
		return nil, &SandboxError{Message: fmt.Sprintf("failed to decode response: %v", err), Cause: err}
	}

	return &codeCtx, nil
}

// ListCodeContexts 列出所有代码执行上下文。
func (ci *CodeInterpreter) ListCodeContexts(ctx context.Context) ([]CodeContext, error) {
	url := ci.jupyterURL + "/contexts"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, &SandboxError{Message: fmt.Sprintf("failed to create request: %v", err), Cause: err}
	}
	ci.Sandbox.setSandboxHeaders(req)

	resp, err := ci.jupyterHTTP.Do(req)
	if err != nil {
		return nil, &SandboxError{Message: fmt.Sprintf("request failed: %v", err), Cause: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, mapHTTPError(resp.StatusCode, string(body))
	}

	var contexts []CodeContext
	if err := json.NewDecoder(resp.Body).Decode(&contexts); err != nil {
		return nil, &SandboxError{Message: fmt.Sprintf("failed to decode response: %v", err), Cause: err}
	}

	return contexts, nil
}

// RemoveCodeContext 移除一个代码执行上下文。
func (ci *CodeInterpreter) RemoveCodeContext(ctx context.Context, contextID string) error {
	url := ci.jupyterURL + "/contexts/" + contextID
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return &SandboxError{Message: fmt.Sprintf("failed to create request: %v", err), Cause: err}
	}
	ci.Sandbox.setSandboxHeaders(req)

	resp, err := ci.jupyterHTTP.Do(req)
	if err != nil {
		return &SandboxError{Message: fmt.Sprintf("request failed: %v", err), Cause: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return mapHTTPError(resp.StatusCode, string(body))
	}

	return nil
}

// RestartCodeContext 重启一个代码执行上下文。
func (ci *CodeInterpreter) RestartCodeContext(ctx context.Context, codeCtx *CodeContext) error {
	url := ci.jupyterURL + "/contexts/" + codeCtx.ID + "/restart"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return &SandboxError{Message: fmt.Sprintf("failed to create request: %v", err), Cause: err}
	}
	ci.Sandbox.setSandboxHeaders(req)

	resp, err := ci.jupyterHTTP.Do(req)
	if err != nil {
		return &SandboxError{Message: fmt.Sprintf("request failed: %v", err), Cause: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return mapHTTPError(resp.StatusCode, string(body))
	}

	return nil
}
