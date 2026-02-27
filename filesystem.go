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

	"github.com/safe-compose/e2b-go/internal/connectrpc"
)

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
	if user != "" {
		uploadURL += "?username=" + url.QueryEscape(user)
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
	Type          string `json:"type"`                    // "FILE_TYPE_FILE" 或 "FILE_TYPE_DIRECTORY"
	Size          string `json:"size"`                    // JSON 中 int64 以字符串表示
	Permissions   string `json:"permissions"`
	Mode          int    `json:"mode"`
	Owner         string `json:"owner"`
	Group         string `json:"group"`
	ModifiedTime  string `json:"modifiedTime"`            // RFC 3339 格式
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

// createWatcherRequest 是创建目录监视器的 RPC 请求。
type createWatcherRequest struct {
	Path      string `json:"path"`
	Recursive bool   `json:"recursive,omitempty"`
}

// createWatcherResponse 是创建目录监视器的 RPC 响应。
type createWatcherResponse struct {
	WatcherID string `json:"watcherId"`
}

// getWatcherEventsRequest 是获取监视器事件的 RPC 请求。
type getWatcherEventsRequest struct {
	WatcherID string `json:"watcherId"`
}

// filesystemEventRaw 是原始文件系统事件结构。
type filesystemEventRaw struct {
	Name string `json:"name"`
	Type string `json:"type"` // "EVENT_TYPE_CREATE" etc.
}

// getWatcherEventsResponse 是获取监视器事件的 RPC 响应。
type getWatcherEventsResponse struct {
	Events []filesystemEventRaw `json:"events"`
}

// removeWatcherRequest 是移除目录监视器的 RPC 请求。
type removeWatcherRequest struct {
	WatcherID string `json:"watcherId"`
}

// List 列出目录中的条目。
func (f *Filesystem) List(ctx context.Context, path string, opts ...FilesystemOption) ([]EntryInfo, error) {
	cfg := f.applyOpts(opts)
	depth := cfg.depth
	if depth <= 0 {
		depth = 1
	}

	req := listDirRequest{Path: path, Depth: depth}
	var resp listDirResponse
	err := f.rpc.CallUnary(ctx, filesystemServiceName, "ListDir", req, &resp)
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
	req := makeDirRequest{Path: path}
	err := f.rpc.CallUnary(ctx, filesystemServiceName, "MakeDir", req, nil)
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
	req := statRequest{Path: path}
	var resp statResponse
	err := f.rpc.CallUnary(ctx, filesystemServiceName, "Stat", req, &resp)
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
	req := statRequest{Path: path}
	var resp statResponse
	err := f.rpc.CallUnary(ctx, filesystemServiceName, "Stat", req, &resp)
	if err != nil {
		return nil, &SandboxError{Message: fmt.Sprintf("failed to stat path: %v", err), Cause: err}
	}
	info := resp.Entry.toEntryInfo()
	return &info, nil
}

// Remove 删除文件或目录。
func (f *Filesystem) Remove(ctx context.Context, path string, opts ...FilesystemOption) error {
	req := removeRequest{Path: path}
	err := f.rpc.CallUnary(ctx, filesystemServiceName, "Remove", req, nil)
	if err != nil {
		return &SandboxError{Message: fmt.Sprintf("failed to remove path: %v", err), Cause: err}
	}
	return nil
}

// Rename 移动或重命名文件或目录。
func (f *Filesystem) Rename(ctx context.Context, oldPath, newPath string, opts ...FilesystemOption) (*EntryInfo, error) {
	req := moveRequest{Source: oldPath, Destination: newPath}
	var resp moveResponse
	err := f.rpc.CallUnary(ctx, filesystemServiceName, "Move", req, &resp)
	if err != nil {
		return nil, &SandboxError{Message: fmt.Sprintf("failed to rename: %v", err), Cause: err}
	}
	info := resp.Entry.toEntryInfo()
	return &info, nil
}

// --- 目录监听（轮询模式） ---

// WatchDir 创建目录监视器并返回用于轮询事件的句柄。
func (f *Filesystem) WatchDir(ctx context.Context, path string, opts ...FilesystemOption) (*WatchHandle, error) {
	cfg := f.applyOpts(opts)

	if cfg.recursive && !f.sandbox.supportsRecursiveWatch() {
		return nil, &SandboxError{Message: "recursive watch requires envd >= 0.1.4"}
	}

	req := createWatcherRequest{Path: path, Recursive: cfg.recursive}
	var resp createWatcherResponse
	err := f.rpc.CallUnary(ctx, filesystemServiceName, "CreateWatcher", req, &resp)
	if err != nil {
		return nil, &SandboxError{Message: fmt.Sprintf("failed to create watcher: %v", err), Cause: err}
	}

	return &WatchHandle{
		watcherID: resp.WatcherID,
		rpc:       f.rpc,
		closed:    false,
	}, nil
}

// applyOpts 应用文件系统选项并返回配置。
func (f *Filesystem) applyOpts(opts []FilesystemOption) *filesystemConfig {
	cfg := &filesystemConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// WatchHandle 表示使用轮询模式的文件系统目录监视器句柄。
type WatchHandle struct {
	watcherID string             // 监视器 ID
	rpc       *connectrpc.Client // RPC 客户端
	mu        sync.Mutex         // 保护 closed 字段
	closed    bool               // 是否已关闭
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

// GetNewEvents 返回自上次调用以来的新文件系统事件。
func (w *WatchHandle) GetNewEvents(ctx context.Context) ([]FilesystemEvent, error) {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil, &SandboxError{Message: "the watcher is already stopped"}
	}
	w.mu.Unlock()

	req := getWatcherEventsRequest{WatcherID: w.watcherID}
	var resp getWatcherEventsResponse
	err := w.rpc.CallUnary(ctx, filesystemServiceName, "GetWatcherEvents", req, &resp)
	if err != nil {
		return nil, &SandboxError{Message: fmt.Sprintf("failed to get watcher events: %v", err), Cause: err}
	}

	events := make([]FilesystemEvent, len(resp.Events))
	for i, e := range resp.Events {
		events[i] = FilesystemEvent{
			Name: e.Name,
			Type: mapEventType(e.Type),
		}
	}
	return events, nil
}

// Stop 移除目录监视器。
func (w *WatchHandle) Stop(ctx context.Context) error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil
	}
	w.closed = true
	w.mu.Unlock()

	req := removeWatcherRequest{WatcherID: w.watcherID}
	err := w.rpc.CallUnary(ctx, filesystemServiceName, "RemoveWatcher", req, nil)
	if err != nil {
		return &SandboxError{Message: fmt.Sprintf("failed to remove watcher: %v", err), Cause: err}
	}
	return nil
}
