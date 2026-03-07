package e2b

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mdsq1/e2b/internal/connectrpc"
)

// watchDirRequest 是 WatchDir 流式 RPC 的请求结构。
type watchDirRequest struct {
	Path      string `json:"path"`
	Recursive bool   `json:"recursive,omitempty"`
}

// watchDirStartEvent 是 WatchDir 流的首个事件（start 类型）。
type watchDirStartEvent struct{}

// watchDirFilesystemEvent 是 WatchDir 流的文件系统事件。
type watchDirFilesystemEvent struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// watchDirResponse 是 WatchDir 流式 RPC 的响应帧，与 Python SDK WatchDirResponse 对齐。
type watchDirResponse struct {
	Start      *watchDirStartEvent      `json:"start,omitempty"`
	Filesystem *watchDirFilesystemEvent `json:"filesystem,omitempty"`
}

// filesystemServiceName 是文件系统 RPC 服务的名称。
const filesystemServiceName = "e2b.filesystem.v1.FilesystemService"

// Filesystem 提供沙箱中的文件操作功能。
type Filesystem struct {
	sandbox    *Sandbox           // 所属沙箱实例
	rpc        *connectrpc.Client // RPC 客户端
	httpClient *http.Client       // HTTP 客户端
}

// newFilesystem 创建一个新的文件系统模块。
func newFilesystem(sbx *Sandbox, rpc *connectrpc.Client, httpClient *http.Client) *Filesystem {
	return &Filesystem{sandbox: sbx, rpc: rpc, httpClient: httpClient}
}

// --- HTTP 文件操作 ---

// Read 以文本形式读取文件内容。
func (f *Filesystem) Read(ctx context.Context, path string, opts ...FilesystemOption) (string, error) {
	data, err := f.ReadBytes(ctx, path, opts...)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ReadBytes 以原始字节形式读取文件内容。
func (f *Filesystem) ReadBytes(ctx context.Context, path string, opts ...FilesystemOption) ([]byte, error) {
	rc, err := f.ReadStream(ctx, path, opts...)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// ReadStream 读取文件并以流的形式返回内容。
func (f *Filesystem) ReadStream(ctx context.Context, path string, opts ...FilesystemOption) (io.ReadCloser, error) {
	cfg := f.applyOpts(opts)
	user := f.sandbox.resolveUsername(cfg.user)

	requestURL := f.sandbox.envdAPIURL + "/files?path=" + url.QueryEscape(path)
	if user != "" {
		requestURL += "&username=" + url.QueryEscape(user)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, &SandboxError{Message: fmt.Sprintf("failed to create request: %v", err), Cause: err}
	}
	f.sandbox.setSandboxHeaders(req)

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, &SandboxError{Message: fmt.Sprintf("read request failed: %v", err), Cause: err}
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return nil, mapHTTPError(resp.StatusCode, string(body))
	}

	return resp.Body, nil
}

// Write 将数据写入文件。
func (f *Filesystem) Write(ctx context.Context, path string, data interface{}, opts ...FilesystemOption) (*WriteInfo, error) {
	infos, err := f.WriteFiles(ctx, []WriteEntry{{Path: path, Data: data}}, opts...)
	if err != nil {
		return nil, err
	}
	if len(infos) > 0 {
		return &infos[0], nil
	}
	return &WriteInfo{Path: path}, nil
}

// WriteFiles 使用多部分上传同时写入多个文件。
func (f *Filesystem) WriteFiles(ctx context.Context, files []WriteEntry, opts ...FilesystemOption) ([]WriteInfo, error) {
	cfg := f.applyOpts(opts)
	user := f.sandbox.resolveUsername(cfg.user)

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	for _, entry := range files {
		part, err := writer.CreateFormFile("file", entry.Path)
		if err != nil {
			return nil, &SandboxError{Message: fmt.Sprintf("failed to create form file: %v", err), Cause: err}
		}

		switch v := entry.Data.(type) {
		case string:
			if _, err := part.Write([]byte(v)); err != nil {
				return nil, &SandboxError{Message: fmt.Sprintf("failed to write data: %v", err), Cause: err}
			}
		case []byte:
			if _, err := part.Write(v); err != nil {
				return nil, &SandboxError{Message: fmt.Sprintf("failed to write data: %v", err), Cause: err}
			}
		case io.Reader:
			if _, err := io.Copy(part, v); err != nil {
				return nil, &SandboxError{Message: fmt.Sprintf("failed to copy data: %v", err), Cause: err}
			}
		default:
			return nil, &SandboxError{Message: fmt.Sprintf("unsupported data type: %T", entry.Data)}
		}
	}
	writer.Close()

	uploadURL := f.sandbox.envdAPIURL + "/files"
	sep := "?"
	// 与 Python SDK 对齐：单文件上传时在查询参数中也传入 path，确保服务端正确定位目标路径。
	if len(files) == 1 {
		uploadURL += sep + "path=" + url.QueryEscape(files[0].Path)
		sep = "&"
	}
	if user != "" {
		uploadURL += sep + "username=" + url.QueryEscape(user)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, &buf)
	if err != nil {
		return nil, &SandboxError{Message: fmt.Sprintf("failed to create request: %v", err), Cause: err}
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	f.sandbox.setSandboxHeaders(req)

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, &SandboxError{Message: fmt.Sprintf("write request failed: %v", err), Cause: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, mapHTTPError(resp.StatusCode, string(body))
	}

	var infos []WriteInfo
	if err := json.NewDecoder(resp.Body).Decode(&infos); err != nil {
		// 某些 envd 版本在成功时返回空响应体
		infos = make([]WriteInfo, len(files))
		for i, entry := range files {
			infos[i] = WriteInfo{Path: entry.Path}
		}
	}
	return infos, nil
}

// WriteStream 从 Reader 读取数据并写入文件。
func (f *Filesystem) WriteStream(ctx context.Context, path string, reader io.Reader, opts ...FilesystemOption) (*WriteInfo, error) {
	return f.Write(ctx, path, reader, opts...)
}

// --- RPC 操作 ---

// 文件系统 RPC 请求/响应类型

// listDirRequest 是列出目录的 RPC 请求。
type listDirRequest struct {
	Path  string `json:"path"`
	Depth int    `json:"depth,omitempty"`
}

// listDirResponse 是列出目录的 RPC 响应。
type listDirResponse struct {
	Entries []entryInfoRaw `json:"entries"`
}

// entryInfoRaw 是原始文件条目信息结构（匹配 protobuf JSON 编码）。
type entryInfoRaw struct {
	Name          string `json:"name"`
	Path          string `json:"path"`
	Type          string `json:"type"` // "FILE_TYPE_FILE" 或 "FILE_TYPE_DIRECTORY"
	Size          string `json:"size"` // JSON 中 int64 以字符串表示
	Permissions   string `json:"permissions"`
	Mode          int    `json:"mode"`
	Owner         string `json:"owner"`
	Group         string `json:"group"`
	ModifiedTime  string `json:"modifiedTime"` // RFC 3339 格式
	SymlinkTarget string `json:"symlinkTarget,omitempty"`
}

// toEntryInfo 将原始文件条目信息转换为 EntryInfo 结构。
func (r *entryInfoRaw) toEntryInfo() EntryInfo {
	info := EntryInfo{
		Name:        r.Name,
		Path:        r.Path,
		Type:        mapFileType(r.Type),
		Permissions: r.Permissions,
		Mode:        uint32(r.Mode),
		Owner:       r.Owner,
		Group:       r.Group,
	}
	if r.Size != "" {
		info.Size, _ = strconv.ParseInt(r.Size, 10, 64)
	}
	if r.ModifiedTime != "" {
		info.ModifiedTime, _ = time.Parse(time.RFC3339Nano, r.ModifiedTime)
	}
	if r.SymlinkTarget != "" {
		info.SymlinkTarget = &r.SymlinkTarget
	}
	return info
}

// mapFileType 将 protobuf 文件类型字符串映射为 EntryType。
func mapFileType(ft string) EntryType {
	switch ft {
	case "FILE_TYPE_FILE":
		return EntryTypeFile
	case "FILE_TYPE_DIRECTORY":
		return EntryTypeDir
	default:
		return EntryTypeFile
	}
}

// statRequest 是获取文件信息的 RPC 请求。
type statRequest struct {
	Path string `json:"path"`
}

// statResponse 是获取文件信息的 RPC 响应。
type statResponse struct {
	Entry entryInfoRaw `json:"entry"`
}

// makeDirRequest 是创建目录的 RPC 请求。
type makeDirRequest struct {
	Path string `json:"path"`
}

// removeRequest 是删除文件/目录的 RPC 请求。
type removeRequest struct {
	Path string `json:"path"`
}

// moveRequest 是移动/重命名文件的 RPC 请求。
type moveRequest struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

// moveResponse 是移动/重命名文件的 RPC 响应。
type moveResponse struct {
	Entry entryInfoRaw `json:"entry"`
}

// List 列出目录中的条目。
func (f *Filesystem) List(ctx context.Context, path string, opts ...FilesystemOption) ([]EntryInfo, error) {
	cfg := f.applyOpts(opts)
	depth := cfg.depth
	if depth <= 0 {
		depth = 1
	}
	user := f.sandbox.resolveUsername(cfg.user)
	authHeader := buildAuthHeader(user)

	req := listDirRequest{Path: path, Depth: depth}
	var resp listDirResponse
	err := f.rpc.CallUnary(ctx, filesystemServiceName, "ListDir", req, &resp, authHeader)
	if err != nil {
		return nil, &SandboxError{Message: fmt.Sprintf("failed to list directory: %v", err), Cause: err}
	}

	entries := make([]EntryInfo, len(resp.Entries))
	for i, e := range resp.Entries {
		entries[i] = e.toEntryInfo()
	}
	return entries, nil
}

// MakeDir 创建目录。创建成功返回 true，目录已存在返回 false。
func (f *Filesystem) MakeDir(ctx context.Context, path string, opts ...FilesystemOption) (bool, error) {
	cfg := f.applyOpts(opts)
	user := f.sandbox.resolveUsername(cfg.user)
	authHeader := buildAuthHeader(user)

	req := makeDirRequest{Path: path}
	err := f.rpc.CallUnary(ctx, filesystemServiceName, "MakeDir", req, nil, authHeader)
	if err != nil {
		if connErr, ok := err.(*connectrpc.Error); ok && connErr.Code == ConnectCodeAlreadyExists {
			return false, nil
		}
		return false, &SandboxError{Message: fmt.Sprintf("failed to create directory: %v", err), Cause: err}
	}
	return true, nil
}

// Exists 检查路径是否存在。
func (f *Filesystem) Exists(ctx context.Context, path string, opts ...FilesystemOption) (bool, error) {
	cfg := f.applyOpts(opts)
	user := f.sandbox.resolveUsername(cfg.user)
	authHeader := buildAuthHeader(user)

	req := statRequest{Path: path}
	var resp statResponse
	err := f.rpc.CallUnary(ctx, filesystemServiceName, "Stat", req, &resp, authHeader)
	if err != nil {
		if connErr, ok := err.(*connectrpc.Error); ok && connErr.Code == ConnectCodeNotFound {
			return false, nil
		}
		return false, &SandboxError{Message: fmt.Sprintf("failed to stat path: %v", err), Cause: err}
	}
	return true, nil
}

// GetInfo 返回文件或目录的详细信息。
func (f *Filesystem) GetInfo(ctx context.Context, path string, opts ...FilesystemOption) (*EntryInfo, error) {
	cfg := f.applyOpts(opts)
	user := f.sandbox.resolveUsername(cfg.user)
	authHeader := buildAuthHeader(user)

	req := statRequest{Path: path}
	var resp statResponse
	err := f.rpc.CallUnary(ctx, filesystemServiceName, "Stat", req, &resp, authHeader)
	if err != nil {
		return nil, &SandboxError{Message: fmt.Sprintf("failed to stat path: %v", err), Cause: err}
	}
	info := resp.Entry.toEntryInfo()
	return &info, nil
}

// Remove 删除文件或目录。
func (f *Filesystem) Remove(ctx context.Context, path string, opts ...FilesystemOption) error {
	cfg := f.applyOpts(opts)
	user := f.sandbox.resolveUsername(cfg.user)
	authHeader := buildAuthHeader(user)

	req := removeRequest{Path: path}
	err := f.rpc.CallUnary(ctx, filesystemServiceName, "Remove", req, nil, authHeader)
	if err != nil {
		return &SandboxError{Message: fmt.Sprintf("failed to remove path: %v", err), Cause: err}
	}
	return nil
}

// Rename 移动或重命名文件或目录。
func (f *Filesystem) Rename(ctx context.Context, oldPath, newPath string, opts ...FilesystemOption) (*EntryInfo, error) {
	cfg := f.applyOpts(opts)
	user := f.sandbox.resolveUsername(cfg.user)
	authHeader := buildAuthHeader(user)

	req := moveRequest{Source: oldPath, Destination: newPath}
	var resp moveResponse
	err := f.rpc.CallUnary(ctx, filesystemServiceName, "Move", req, &resp, authHeader)
	if err != nil {
		return nil, &SandboxError{Message: fmt.Sprintf("failed to rename: %v", err), Cause: err}
	}
	info := resp.Entry.toEntryInfo()
	return &info, nil
}

// --- 目录监听（流式模式，与 Python SDK 对齐） ---

// WatchDir 使用流式 WatchDir RPC 监听目录变更事件。
// onEvent 在每个文件系统事件到达时被调用（在后台 goroutine 中执行）。
// 与 Python SDK 对齐：使用 WatchDir 流式 RPC，而非轮询 CreateWatcher/GetWatcherEvents。
func (f *Filesystem) WatchDir(ctx context.Context, path string, onEvent func(FilesystemEvent), opts ...FilesystemOption) (*WatchHandle, error) {
	cfg := f.applyOpts(opts)

	if cfg.recursive && !f.sandbox.supportsRecursiveWatch() {
		return nil, &SandboxError{Message: "recursive watch requires envd >= 0.1.4"}
	}

	user := f.sandbox.resolveUsername(cfg.user)
	authHeader := buildAuthHeader(user)

	timeoutMs := 0
	if cfg.watchTimeout > 0 {
		timeoutMs = cfg.watchTimeout * 1000
	}

	req := watchDirRequest{Path: path, Recursive: cfg.recursive}

	watchCtx, cancel := context.WithCancel(ctx)
	stream, err := f.rpc.CallServerStream(watchCtx, filesystemServiceName, "WatchDir", req, timeoutMs, authHeader)
	if err != nil {
		cancel()
		return nil, &SandboxError{Message: fmt.Sprintf("failed to start watch: %v", err), Cause: err}
	}

	// 读取首个事件（start 事件）
	var firstEvent watchDirResponse
	if err := stream.Next(&firstEvent); err != nil {
		stream.Close()
		cancel()
		return nil, &SandboxError{Message: fmt.Sprintf("failed to read watch start event: %v", err), Cause: err}
	}
	if firstEvent.Start == nil {
		stream.Close()
		cancel()
		return nil, &SandboxError{Message: "expected start event from WatchDir stream"}
	}

	handle := &WatchHandle{
		cancel: cancel,
		done:   make(chan struct{}),
	}

	go func() {
		defer close(handle.done)
		defer stream.Close()
		defer cancel()

		for {
			var event watchDirResponse
			err := stream.Next(&event)
			if err != nil {
				if err != io.EOF {
					handle.mu.Lock()
					handle.err = &SandboxError{Message: fmt.Sprintf("watch stream error: %v", err), Cause: err}
					handle.mu.Unlock()
					if cfg.onExit != nil {
						cfg.onExit(handle.err)
					}
				}
				return
			}

			if event.Filesystem != nil {
				fsEvent := FilesystemEvent{
					Name: event.Filesystem.Name,
					Type: mapEventType(event.Filesystem.Type),
				}
				onEvent(fsEvent)
			}
		}
	}()

	return handle, nil
}

// applyOpts 应用文件系统选项并返回配置。
func (f *Filesystem) applyOpts(opts []FilesystemOption) *filesystemConfig {
	cfg := &filesystemConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// WatchHandle 表示一个正在运行的目录监听句柄（流式模式）。
type WatchHandle struct {
	cancel context.CancelFunc // 取消流式 RPC 的函数
	done   chan struct{}      // 后台 goroutine 结束信号
	mu     sync.Mutex         // 保护 err 字段
	err    error              // 监听过程中的错误
}

// Stop 停止目录监听，取消底层流式连接。
func (w *WatchHandle) Stop() {
	w.cancel()
}

// Wait 阻塞等待监听 goroutine 结束，返回监听过程中发生的错误（若有）。
func (w *WatchHandle) Wait(ctx context.Context) error {
	select {
	case <-w.done:
		w.mu.Lock()
		defer w.mu.Unlock()
		return w.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// mapEventType 将 protobuf 事件类型字符串映射为 FilesystemEventType。
func mapEventType(raw string) FilesystemEventType {
	switch raw {
	case "EVENT_TYPE_CREATE":
		return EventTypeCreate
	case "EVENT_TYPE_WRITE":
		return EventTypeWrite
	case "EVENT_TYPE_REMOVE":
		return EventTypeRemove
	case "EVENT_TYPE_RENAME":
		return EventTypeRename
	case "EVENT_TYPE_CHMOD":
		return EventTypeChmod
	default:
		return FilesystemEventType(strings.ToLower(strings.TrimPrefix(raw, "EVENT_TYPE_")))
	}
}
