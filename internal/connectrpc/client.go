package connectrpc

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

// Client 是使用 JSON 编码的 Connect RPC 客户端。
type Client struct {
	BaseURL    string            // RPC 服务基础 URL
	HTTPClient *http.Client      // HTTP 客户端
	Headers    map[string]string // 自定义请求头
}

// CallUnary 执行一元 RPC 调用（一个请求，一个响应）。
// extraHeaders 为可选的每次请求附加头，例如 Authorization 用于用户认证。
func (c *Client) CallUnary(ctx context.Context, service, method string, req, resp any, extraHeaders ...map[string]string) error {
	url := fmt.Sprintf("%s/%s/%s", c.BaseURL, service, method)

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("connect: failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("connect: failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Connect-Protocol-Version", "1")
	c.setHeaders(httpReq)
	for _, h := range extraHeaders {
		for k, v := range h {
			httpReq.Header.Set(k, v)
		}
	}

	httpResp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("connect: request failed: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return fmt.Errorf("connect: failed to read response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return parseConnectError(httpResp.StatusCode, respBody)
	}

	if resp != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, resp); err != nil {
			return fmt.Errorf("connect: failed to unmarshal response: %w", err)
		}
	}

	return nil
}

// CallServerStream 执行服务端流式 RPC 调用。
// extraHeaders 为可选的每次请求附加头，例如 Authorization 用于用户认证。
func (c *Client) CallServerStream(ctx context.Context, service, method string, req any, timeoutMs int, extraHeaders ...map[string]string) (*StreamReader, error) {
	url := fmt.Sprintf("%s/%s/%s", c.BaseURL, service, method)

	jsonBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("connect: failed to marshal request: %w", err)
	}
	envelopedBody := EncodeEnvelope(jsonBody)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(envelopedBody))
	if err != nil {
		return nil, fmt.Errorf("connect: failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/connect+json")
	httpReq.Header.Set("Connect-Protocol-Version", "1")
	if timeoutMs > 0 {
		httpReq.Header.Set("Connect-Timeout-Ms", strconv.Itoa(timeoutMs))
	}
	c.setHeaders(httpReq)
	for _, h := range extraHeaders {
		for k, v := range h {
			httpReq.Header.Set(k, v)
		}
	}

	httpResp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("connect: request failed: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		defer httpResp.Body.Close()
		respBody, _ := io.ReadAll(httpResp.Body)
		return nil, parseConnectError(httpResp.StatusCode, respBody)
	}

	return &StreamReader{reader: httpResp.Body}, nil
}

// setHeaders 为请求设置自定义请求头。
func (c *Client) setHeaders(req *http.Request) {
	for k, v := range c.Headers {
		req.Header.Set(k, v)
	}
}

// EncodeEnvelope 将数据包装在 Connect 二进制信封中。
// 格式：[标志: 1 字节][长度: 4 字节大端序][数据: N 字节]
func EncodeEnvelope(data []byte) []byte {
	header := make([]byte, 5)
	header[0] = 0 // flags: no compression
	binary.BigEndian.PutUint32(header[1:5], uint32(len(data)))
	return append(header, data...)
}

// DecodeEnvelopeHeader 解码 5 字节的信封头部。
func DecodeEnvelopeHeader(header []byte) (flags byte, dataLen uint32) {
	return header[0], binary.BigEndian.Uint32(header[1:5])
}

// StreamReader 读取带有信封帧的 Connect RPC 流式响应。
type StreamReader struct {
	reader io.ReadCloser // 底层读取器
}

// Next 从流中读取下一条消息。
// 流正常结束时返回 io.EOF。
// 流尾部包含 Connect RPC 错误时返回 *Error。
func (s *StreamReader) Next(msg any) error {
	// 读取 5 字节头部
	header := make([]byte, 5)
	if _, err := io.ReadFull(s.reader, header); err != nil {
		return err // io.EOF = stream ended normally
	}
	flags, dataLen := DecodeEnvelopeHeader(header)

	// 读取消息体
	data := make([]byte, dataLen)
	if _, err := io.ReadFull(s.reader, data); err != nil {
		return fmt.Errorf("connect: failed to read envelope body: %w", err)
	}

	// 检查 end_stream 标志（第 1 位）
	if flags&0x02 != 0 {
		var trailer struct {
			Error *struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error,omitempty"`
		}
		if err := json.Unmarshal(data, &trailer); err == nil && trailer.Error != nil {
			return &Error{Code: trailer.Error.Code, Message: trailer.Error.Message}
		}
		return io.EOF
	}

	// 普通消息，JSON 解码
	return json.Unmarshal(data, msg)
}

// Close 关闭流读取器。
func (s *StreamReader) Close() error {
	return s.reader.Close()
}

// Error 表示 Connect RPC 协议错误。
type Error struct {
	Code    string // 错误码
	Message string // 错误信息
}

// Error 返回错误的字符串表示。
func (e *Error) Error() string {
	return fmt.Sprintf("connect rpc error [%s]: %s", e.Code, e.Message)
}

// parseConnectError 解析 Connect RPC 错误响应。
func parseConnectError(statusCode int, body []byte) error {
	var errResp struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Code != "" {
		return &Error{Code: errResp.Code, Message: errResp.Message}
	}
	return fmt.Errorf("connect: HTTP %d: %s", statusCode, string(body))
}
