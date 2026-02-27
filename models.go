package e2b

import (
	"context"
	"time"
)

// SandboxState 表示沙箱的运行状态。
type SandboxState string

// 沙箱状态常量。
const (
	SandboxStateRunning SandboxState = "running" // 运行中
	SandboxStatePaused  SandboxState = "paused"  // 已暂停
)

// SandboxInfo 包含沙箱的详细信息。
type SandboxInfo struct {
	SandboxID   string            `json:"sandboxID"`   // 沙箱 ID
	TemplateID  string            `json:"templateID"`  // 模板 ID
	Name        string            `json:"alias"`       // 沙箱别名
	Metadata    map[string]string `json:"metadata"`    // 元数据
	StartedAt   time.Time         `json:"startedAt"`   // 启动时间
	EndAt       time.Time         `json:"endAt"`       // 结束时间
	State       SandboxState      `json:"state"`       // 运行状态
	CPUCount    int               `json:"cpuCount"`    // CPU 核数
	MemoryMB    int               `json:"memoryMB"`    // 内存大小（MB）
	EnvdVersion string            `json:"envdVersion"` // envd 版本
}

// SandboxMetrics 包含沙箱的资源使用指标。
type SandboxMetrics struct {
	CPUCount   int       `json:"cpuCount"`   // CPU 核数
	CPUUsedPct float64   `json:"cpuUsedPct"` // CPU 使用率（百分比）
	DiskTotal  int64     `json:"diskTotal"`  // 磁盘总量（字节）
	DiskUsed   int64     `json:"diskUsed"`   // 磁盘已用（字节）
	MemTotal   int64     `json:"memTotal"`   // 内存总量（字节）
	MemUsed    int64     `json:"memUsed"`    // 内存已用（字节）
	Timestamp  time.Time `json:"timestamp"` // 时间戳
}

// SnapshotInfo 包含快照的信息。
type SnapshotInfo struct {
	SnapshotID string `json:"snapshotID"` // 快照 ID
	// Deprecated: Names 在最新 API 中不再使用，请使用 SnapshotID。
	Names []string `json:"names,omitempty"`
}

// CommandResult 是已完成命令执行的结果。
type CommandResult struct {
	Stdout   string `json:"stdout"`          // 标准输出
	Stderr   string `json:"stderr"`          // 标准错误
	ExitCode int    `json:"exitCode"`        // 退出码
	Error    string `json:"error,omitempty"` // 错误信息
}

// EntryType 表示文件系统条目的类型。
type EntryType string

// 文件系统条目类型常量。
const (
	EntryTypeFile EntryType = "file" // 文件
	EntryTypeDir  EntryType = "dir"  // 目录
)

// EntryInfo 包含文件或目录的详细信息。
type EntryInfo struct {
	Name          string    `json:"name"`                     // 名称
	Path          string    `json:"path"`                     // 路径
	Type          EntryType `json:"type"`                     // 类型（文件/目录）
	Size          int64     `json:"size"`                     // 大小（字节）
	Permissions   string    `json:"permissions"`              // 权限字符串
	Mode          uint32    `json:"mode"`                     // 权限模式
	Owner         string    `json:"owner"`                    // 所有者
	Group         string    `json:"group"`                    // 所属组
	ModifiedTime  time.Time `json:"modifiedTime"`             // 修改时间
	SymlinkTarget *string   `json:"symlinkTarget,omitempty"` // 符号链接目标
}

// WriteInfo 包含已写入文件的信息。
type WriteInfo struct {
	Name string     `json:"name"` // 文件名
	Type *EntryType `json:"type"` // 条目类型
	Path string     `json:"path"` // 文件路径
}

// WriteEntry 表示要写入的文件。
type WriteEntry struct {
	Path string      // 文件路径
	Data interface{} // 文件数据，支持 string | []byte | io.Reader
}

// ProcessInfo 包含正在运行的进程信息。
// 注意：protobuf 的 ProcessInfo 包含嵌套的 ProcessConfig{cmd,args,envs,cwd}。
// Go SDK 将其扁平化为单一结构体；内部解析时从 config 子对象中提取。
type ProcessInfo struct {
	PID  int               `json:"pid"`  // 进程 ID
	Tag  string            `json:"tag"`  // 进程标签
	Cmd  string            `json:"cmd"`  // 命令
	Args []string          `json:"args"` // 参数列表
	Envs map[string]string `json:"envs"` // 环境变量
	Cwd  string            `json:"cwd"`  // 工作目录
}

// PtySize 表示伪终端的尺寸。
type PtySize struct {
	Rows int `json:"rows"` // 行数
	Cols int `json:"cols"` // 列数
}

// OutputMessage 表示代码执行的单行输出。
type OutputMessage struct {
	Line      string `json:"line"`      // 输出行内容
	Timestamp int64  `json:"timestamp"` // 时间戳
	Error     bool   `json:"error"`     // 是否为错误输出
}

// String 返回输出行的字符串表示。
func (m OutputMessage) String() string { return m.Line }

// SandboxQuery 包含列出沙箱时的过滤条件。
type SandboxQuery struct {
	Metadata map[string]string // 按元数据过滤
	State    []SandboxState    // 按状态过滤
}

// Paginator 提供对 API 结果的惰性分页功能。
// 非协程安全；设计为单协程使用。
type Paginator[T any] struct {
	hasNext   bool                                                                   // 是否还有下一页
	nextToken string                                                                 // 下一页令牌
	limit     int                                                                    // 每页数量限制
	fetchFunc func(ctx context.Context, token string, limit int) ([]T, string, error) // 获取数据的函数
}

// newPaginator 创建一个新的分页器。
func newPaginator[T any](limit int, fetchFunc func(ctx context.Context, token string, limit int) ([]T, string, error)) *Paginator[T] {
	return &Paginator[T]{
		hasNext:   true,
		nextToken: "",
		limit:     limit,
		fetchFunc: fetchFunc,
	}
}

// HasNext 返回是否还有更多页面可获取。
func (p *Paginator[T]) HasNext() bool { return p.hasNext }

// NextItems 获取下一页的数据项。
func (p *Paginator[T]) NextItems(ctx context.Context) ([]T, error) {
	if !p.hasNext {
		return nil, nil
	}
	items, nextToken, err := p.fetchFunc(ctx, p.nextToken, p.limit)
	if err != nil {
		return nil, err
	}
	p.nextToken = nextToken
	p.hasNext = nextToken != ""
	return items, nil
}

// All 获取所有页面的数据项。
func (p *Paginator[T]) All(ctx context.Context) ([]T, error) {
	var all []T
	for p.HasNext() {
		items, err := p.NextItems(ctx)
		if err != nil {
			return all, err
		}
		all = append(all, items...)
	}
	return all, nil
}

// NetworkOpts 包含沙箱的网络配置选项。
type NetworkOpts struct {
	AllowOut           []string `json:"allowOut,omitempty"`           // 允许的出站 CIDR 列表
	DenyOut            []string `json:"denyOut,omitempty"`            // 拒绝的出站 CIDR 列表
	AllowPublicTraffic *bool    `json:"allowPublicTraffic,omitempty"` // 是否允许公共流量
	MaskRequestHost    string   `json:"maskRequestHost,omitempty"`    // 请求主机掩码
}

// AutoResumePolicy 控制暂停沙箱的自动恢复行为。
type AutoResumePolicy string

// 自动恢复策略常量。
const (
	AutoResumePolicyOff AutoResumePolicy = "off" // 关闭自动恢复
	AutoResumePolicyOn  AutoResumePolicy = "on"  // 开启自动恢复
)

// VolumeMount 描述要挂载到沙箱中的卷。
type VolumeMount struct {
	Name string `json:"name"` // 卷名称
	Path string `json:"path"` // 挂载路径
}

// MCPConfig 以任意 JSON 形式保存 MCP 服务器配置。
type MCPConfig map[string]any

// GitHubMCPServerConfig 是 GitHub MCP 服务器的类型化配置。
type GitHubMCPServerConfig struct {
	RunCmd     string            `json:"run_cmd"`              // 运行命令
	InstallCmd string            `json:"install_cmd,omitempty"` // 安装命令
	Envs       map[string]string `json:"envs,omitempty"`        // 环境变量
}

// NewGitHubMCPConfig 创建包含 GitHub MCP 服务器条目的 MCPConfig。
// 键的格式应为 "github/owner/repo"。
func NewGitHubMCPConfig(servers map[string]GitHubMCPServerConfig) MCPConfig {
	config := MCPConfig{}
	for k, v := range servers {
		config[k] = v
	}
	return config
}

// FilesystemEventType 表示文件系统变更事件的类型。
type FilesystemEventType string

// 文件系统事件类型常量。
const (
	EventTypeCreate FilesystemEventType = "create" // 创建
	EventTypeWrite  FilesystemEventType = "write"  // 写入
	EventTypeRemove FilesystemEventType = "remove" // 删除
	EventTypeRename FilesystemEventType = "rename" // 重命名
	EventTypeChmod  FilesystemEventType = "chmod"  // 权限变更
)

// FilesystemEvent 表示一个文件系统变更事件。
type FilesystemEvent struct {
	Name string              // 文件/目录名称
	Type FilesystemEventType // 事件类型
}

// Execution 是在代码解释器中运行代码的结果。
type Execution struct {
	Results        []Result        `json:"results"`                  // 执行结果列表
	Logs           ExecutionLogs   `json:"logs"`                     // 执行日志
	Error          *ExecutionError `json:"error,omitempty"`          // 执行错误
	ExecutionCount *int            `json:"executionCount,omitempty"` // 执行计数
}

// Text 返回第一个主要结果的文本表示。
func (e *Execution) Text() string {
	for _, r := range e.Results {
		if r.IsMainResult && r.Text != nil {
			return *r.Text
		}
	}
	return ""
}

// ExecutionLogs 包含代码执行的标准输出和标准错误。
type ExecutionLogs struct {
	Stdout []string `json:"stdout"` // 标准输出行列表
	Stderr []string `json:"stderr"` // 标准错误行列表
}

// ExecutionError 描述代码执行期间的错误。
type ExecutionError struct {
	Name      string `json:"name"`      // 错误名称
	Value     string `json:"value"`     // 错误值
	Traceback string `json:"traceback"` // 错误堆栈跟踪
}

// Result 表示单个执行结果（类似于 Jupyter 单元格输出）。
type Result struct {
	Text         *string                `json:"text,omitempty"`       // 纯文本输出
	HTML         *string                `json:"html,omitempty"`       // HTML 输出
	Markdown     *string                `json:"markdown,omitempty"`   // Markdown 输出
	SVG          *string                `json:"svg,omitempty"`        // SVG 图形输出
	PNG          *string                `json:"png,omitempty"`        // PNG 图片（Base64）
	JPEG         *string                `json:"jpeg,omitempty"`       // JPEG 图片（Base64）
	PDF          *string                `json:"pdf,omitempty"`        // PDF 输出（Base64）
	LaTeX        *string                `json:"latex,omitempty"`      // LaTeX 输出
	JSON         map[string]interface{} `json:"json,omitempty"`       // JSON 输出
	JavaScript   *string                `json:"javascript,omitempty"` // JavaScript 输出
	Data         map[string]interface{} `json:"data,omitempty"`       // 原始数据
	Chart        *Chart                 `json:"chart,omitempty"`      // 图表数据
	IsMainResult bool                   `json:"is_main_result"`       // 是否为主要结果
	Extra        map[string]interface{} `json:"extra,omitempty"`      // 额外数据
}

// Formats 返回所有非空输出格式的名称列表。
func (r *Result) Formats() []string {
	var formats []string
	if r.Text != nil {
		formats = append(formats, "text")
	}
	if r.HTML != nil {
		formats = append(formats, "html")
	}
	if r.Markdown != nil {
		formats = append(formats, "markdown")
	}
	if r.SVG != nil {
		formats = append(formats, "svg")
	}
	if r.PNG != nil {
		formats = append(formats, "png")
	}
	if r.JPEG != nil {
		formats = append(formats, "jpeg")
	}
	if r.PDF != nil {
		formats = append(formats, "pdf")
	}
	if r.LaTeX != nil {
		formats = append(formats, "latex")
	}
	if r.JSON != nil {
		formats = append(formats, "json")
	}
	if r.JavaScript != nil {
		formats = append(formats, "javascript")
	}
	if r.Chart != nil {
		formats = append(formats, "chart")
	}
	return formats
}

// CodeContext 表示有状态的代码执行上下文。
type CodeContext struct {
	ID       string `json:"id"`       // 上下文 ID
	Language string `json:"language"` // 编程语言
	Cwd      string `json:"cwd"`      // 工作目录
}

// Chart 表示结构化的图表数据。
type Chart struct {
	Type     ChartType              `json:"type"`              // 图表类型
	Title    string                 `json:"title"`             // 图表标题
	Elements []interface{}          `json:"elements"`          // 图表元素列表
	XLabel   string                 `json:"x_label,omitempty"` // X 轴标签
	YLabel   string                 `json:"y_label,omitempty"` // Y 轴标签
	RawData  map[string]interface{} `json:"-"`                 // 原始数据（不序列化）
}

// ChartType 表示图表的类型。
type ChartType string

// 图表类型常量。
const (
	ChartTypeLine          ChartType = "line"           // 折线图
	ChartTypeScatter       ChartType = "scatter"        // 散点图
	ChartTypeBar           ChartType = "bar"            // 柱状图
	ChartTypePie           ChartType = "pie"            // 饼图
	ChartTypeBoxAndWhisker ChartType = "box_and_whisker" // 箱线图
	ChartTypeSuperChart    ChartType = "superchart"      // 超级图表
	ChartTypeUnknown       ChartType = "unknown"         // 未知类型
)
