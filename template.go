package e2b

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// === 指令类型 ===

// InstructionType 表示模板构建指令类型。
type InstructionType string

const (
	InstructionCopy    InstructionType = "COPY"    // 复制文件指令
	InstructionRun     InstructionType = "RUN"     // 运行命令指令
	InstructionEnv     InstructionType = "ENV"     // 设置环境变量指令
	InstructionWorkdir InstructionType = "WORKDIR" // 设置工作目录指令
	InstructionUser    InstructionType = "USER"    // 设置用户指令
)

// === 构建状态 ===

// TemplateBuildStatus 表示模板构建的状态。
type TemplateBuildStatus string

const (
	BuildStatusBuilding TemplateBuildStatus = "building" // 正在构建
	BuildStatusWaiting  TemplateBuildStatus = "waiting"  // 等待中
	BuildStatusReady    TemplateBuildStatus = "ready"    // 已就绪
	BuildStatusError    TemplateBuildStatus = "error"    // 构建错误
)

// === 数据类型 ===

// BuildInfo 包含已构建模板的信息。
type BuildInfo struct {
	TemplateID string   `json:"templateId"`     // 模板 ID
	BuildID    string   `json:"buildId"`        // 构建 ID
	Name       string   `json:"name"`           // 模板名称
	Tags       []string `json:"tags,omitempty"` // 标签列表
}

// BuildStatusResponse 包含构建状态查询的响应。
type BuildStatusResponse struct {
	BuildID    string              `json:"buildId"`              // 构建 ID
	TemplateID string              `json:"templateId"`           // 模板 ID
	Status     TemplateBuildStatus `json:"status"`               // 构建状态
	Logs       []string            `json:"logs"`                 // 日志行
	LogEntries []LogEntry          `json:"logEntries,omitempty"` // 结构化日志条目
	Reason     *BuildStatusReason  `json:"reason,omitempty"`     // 状态原因
}

// BuildStatusReason 包含构建状态的原因信息。
type BuildStatusReason struct {
	Message    string     `json:"message"`              // 原因消息
	Step       string     `json:"step,omitempty"`       // 失败步骤
	LogEntries []LogEntry `json:"logEntries,omitempty"` // 相关日志条目
}

// RegistryConfig 是容器镜像仓库的认证配置。
type RegistryConfig struct {
	Type               string `json:"type"`                         // 仓库类型："registry"、"aws"、"gcp"
	Username           string `json:"username,omitempty"`           // Docker Registry 用户名
	Password           string `json:"password,omitempty"`           // Docker Registry 密码
	AWSAccessKeyID     string `json:"awsAccessKeyId,omitempty"`     // AWS ECR 访问密钥 ID
	AWSSecretAccessKey string `json:"awsSecretAccessKey,omitempty"` // AWS ECR 秘密访问密钥
	AWSRegion          string `json:"awsRegion,omitempty"`          // AWS ECR 区域
	ServiceAccountJSON string `json:"serviceAccountJson,omitempty"` // GCP Artifact Registry 服务账户 JSON
}

// LogEntry 表示一条结构化的构建日志条目。
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"` // 时间戳
	Level     string    `json:"level"`     // 日志级别："info"、"error"、"warn"、"debug"
	Message   string    `json:"message"`   // 日志消息
}

// String 返回日志消息文本。
func (e LogEntry) String() string { return e.Message }

// Instruction 表示单个模板构建指令。
type Instruction struct {
	Type            InstructionType `json:"type"`                      // 指令类型
	Args            []string        `json:"args"`                      // 指令参数
	Force           bool            `json:"force,omitempty"`           // 是否强制执行（跳过缓存）
	ForceUpload     *bool           `json:"forceUpload,omitempty"`     // 是否强制上传
	ResolveSymlinks *bool           `json:"resolveSymlinks,omitempty"` // 是否解析符号链接
	FilesHash       string          `json:"filesHash,omitempty"`       // 文件哈希值
}

// templateData 是发送到构建 API 的序列化格式。
type templateData struct {
	FromImage         string          `json:"fromImage,omitempty"`         // 基础 Docker 镜像
	FromTemplate      string          `json:"fromTemplate,omitempty"`      // 基础 E2B 模板
	FromImageRegistry *RegistryConfig `json:"fromImageRegistry,omitempty"` // 镜像仓库认证配置
	StartCmd          string          `json:"startCmd,omitempty"`          // 启动命令
	ReadyCmd          string          `json:"readyCmd,omitempty"`          // 就绪检查命令
	Steps             []Instruction   `json:"steps"`                       // 构建步骤列表
	Force             bool            `json:"force"`                       // 是否强制重新构建
}

// === 就绪检查命令 ===

// ReadyCmd 封装一个 shell 命令，用于检查沙箱在启动命令执行后是否就绪。
type ReadyCmd struct {
	cmd string // shell 命令
}

// String 返回底层的 shell 命令。
func (r ReadyCmd) String() string { return r.cmd }

// WaitForPort 创建一个就绪检查，等待指定端口开始监听。
func WaitForPort(port int) ReadyCmd {
	return ReadyCmd{cmd: fmt.Sprintf("ss -tuln | grep :%d", port)}
}

// WaitForURL 创建一个就绪检查，等待 URL 返回指定的状态码。
func WaitForURL(url string, statusCode int) ReadyCmd {
	return ReadyCmd{cmd: fmt.Sprintf(`curl -s -o /dev/null -w "%%{http_code}" %s | grep -q "%d"`, url, statusCode)}
}

// WaitForProcess 创建一个就绪检查，等待指定进程名称出现。
func WaitForProcess(processName string) ReadyCmd {
	return ReadyCmd{cmd: fmt.Sprintf("pgrep %s > /dev/null", processName)}
}

// WaitForFile 创建一个就绪检查，等待指定文件存在。
func WaitForFile(filename string) ReadyCmd {
	return ReadyCmd{cmd: fmt.Sprintf("[ -f %s ]", filename)}
}

// WaitForTimeout 创建一个就绪检查，等待固定时间（毫秒）。
// 最小等待时间为 1 秒。
func WaitForTimeout(timeoutMs int) ReadyCmd {
	if timeoutMs < 1000 {
		timeoutMs = 1000
	}
	seconds := float64(timeoutMs) / 1000.0
	return ReadyCmd{cmd: fmt.Sprintf("sleep %.3f", seconds)}
}

// === 镜像仓库选项 ===

// registryConfig 包含镜像仓库认证的内部配置。
type registryConfig struct {
	username string // 用户名
	password string // 密码
}

// RegistryOption 用于配置 FromImage 的 Docker 镜像仓库认证。
type RegistryOption func(*registryConfig)

// WithRegistryAuth 设置 Docker 镜像仓库的认证凭据。
func WithRegistryAuth(username, password string) RegistryOption {
	return func(c *registryConfig) {
		c.username = username
		c.password = password
	}
}

// applyRegistryOpts 应用镜像仓库选项并返回配置。
func applyRegistryOpts(opts []RegistryOption) *registryConfig {
	cfg := &registryConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// === 模板构建器 ===

// TemplateBuilder 使用流式 API 构建 E2B 沙箱模板。
// 所有修改方法都返回构建器本身，支持链式调用。
type TemplateBuilder struct {
	baseImage       string          // 基础 Docker 镜像
	baseTemplate    string          // 基础 E2B 模板
	registryConfig  *RegistryConfig // 镜像仓库认证配置
	startCmd        string          // 沙箱启动命令
	readyCmd        string          // 就绪检查命令
	force           bool            // 是否强制重新构建
	forceNextLayer  bool            // 下一层是否跳过缓存
	instructions    []Instruction   // 构建指令列表
	fileContextPath string          // 本地文件上下文路径
	ignorePatterns  []string        // 排除的 glob 模式
	err             error           // 验证中的延迟错误
}

// TemplateBuilderOption 在创建时配置 TemplateBuilder。
type TemplateBuilderOption func(*TemplateBuilder)

// WithFileContextPath 设置 COPY 指令的本地文件上下文路径。
func WithFileContextPath(path string) TemplateBuilderOption {
	return func(t *TemplateBuilder) { t.fileContextPath = path }
}

// WithIgnorePatterns 设置从文件上下文中排除的 glob 模式。
func WithIgnorePatterns(patterns []string) TemplateBuilderOption {
	return func(t *TemplateBuilder) { t.ignorePatterns = patterns }
}

// NewTemplate 使用默认基础镜像创建一个新的模板构建器。
func NewTemplate(opts ...TemplateBuilderOption) *TemplateBuilder {
	t := &TemplateBuilder{
		baseImage: "e2bdev/base:latest",
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// === FROM 方法 ===

// FromImage 设置模板的基础 Docker 镜像。
func (t *TemplateBuilder) FromImage(image string, opts ...RegistryOption) *TemplateBuilder {
	t.baseImage = image
	t.baseTemplate = ""
	cfg := applyRegistryOpts(opts)
	if cfg.username != "" && cfg.password != "" {
		t.registryConfig = &RegistryConfig{Type: "registry", Username: cfg.username, Password: cfg.password}
	} else {
		t.registryConfig = nil
	}
	if t.forceNextLayer {
		t.force = true
	}
	return t
}

// FromTemplate 设置模板的基础模板（在另一个 E2B 模板上构建）。
func (t *TemplateBuilder) FromTemplate(template string) *TemplateBuilder {
	t.baseTemplate = template
	t.baseImage = ""
	t.registryConfig = nil
	if t.forceNextLayer {
		t.force = true
	}
	return t
}

// FromImageWithAuth 设置带有显式镜像仓库认证的基础 Docker 镜像。
func (t *TemplateBuilder) FromImageWithAuth(image, username, password string) *TemplateBuilder {
	return t.FromImage(image, WithRegistryAuth(username, password))
}

// FromPythonImage 设置 Python 基础镜像。
// version 默认为 "3"（例如 "3"、"3.12"、"3.12-slim"）。
func (t *TemplateBuilder) FromPythonImage(version string) *TemplateBuilder {
	if version == "" {
		version = "3"
	}
	return t.FromImage("python:" + version)
}

// FromNodeImage 设置 Node.js 基础镜像。
// variant 默认为 "lts"（例如 "lts"、"20"、"22-slim"）。
func (t *TemplateBuilder) FromNodeImage(variant string) *TemplateBuilder {
	if variant == "" {
		variant = "lts"
	}
	return t.FromImage("node:" + variant)
}

// FromBunImage 设置 Bun 基础镜像。
// variant 默认为 "latest"（例如 "latest"、"1.1"）。
func (t *TemplateBuilder) FromBunImage(variant string) *TemplateBuilder {
	if variant == "" {
		variant = "latest"
	}
	return t.FromImage("oven/bun:" + variant)
}

// FromDebianImage 设置 Debian 基础镜像。
// variant 默认为 "stable"（例如 "stable"、"bookworm"、"12"）。
func (t *TemplateBuilder) FromDebianImage(variant string) *TemplateBuilder {
	if variant == "" {
		variant = "stable"
	}
	return t.FromImage("debian:" + variant)
}

// FromUbuntuImage 设置 Ubuntu 基础镜像。
// variant 默认为 "latest"（例如 "latest"、"22.04"、"noble"）。
func (t *TemplateBuilder) FromUbuntuImage(variant string) *TemplateBuilder {
	if variant == "" {
		variant = "latest"
	}
	return t.FromImage("ubuntu:" + variant)
}

// FromBaseImage 重置为默认的 E2B 基础镜像。
func (t *TemplateBuilder) FromBaseImage() *TemplateBuilder {
	return t.FromImage("e2bdev/base:latest")
}

// FromAWSRegistry 从 AWS ECR 镜像仓库设置基础镜像。
func (t *TemplateBuilder) FromAWSRegistry(image, accessKeyID, secretAccessKey, region string) *TemplateBuilder {
	t.baseImage = image
	t.baseTemplate = ""
	t.registryConfig = &RegistryConfig{
		Type:               "aws",
		AWSAccessKeyID:     accessKeyID,
		AWSSecretAccessKey: secretAccessKey,
		AWSRegion:          region,
	}
	if t.forceNextLayer {
		t.force = true
	}
	return t
}

// FromGCPRegistry 从 GCP Artifact Registry 设置基础镜像。
func (t *TemplateBuilder) FromGCPRegistry(image, serviceAccountJSON string) *TemplateBuilder {
	t.baseImage = image
	t.baseTemplate = ""
	t.registryConfig = &RegistryConfig{Type: "gcp", ServiceAccountJSON: serviceAccountJSON}
	if t.forceNextLayer {
		t.force = true
	}
	return t
}

// === 路径验证 ===

// validateRelativePath 检查 src 是否为相对路径且未逃逸上下文目录。
// 防止路径遍历攻击。
func validateRelativePath(src string) error {
	// filepath.IsAbs は Windows では "/" を絶対パスと認識しないため、
	// Unix スタイルの絶対パス（"/" で始まる）も明示的に拒否する。
	if filepath.IsAbs(src) || strings.HasPrefix(src, "/") {
		return fmt.Errorf("invalid source path %q: absolute paths are not allowed, use a relative path within the context directory", src)
	}
	normalized := filepath.Clean(src)
	if normalized == ".." || strings.HasPrefix(normalized, ".."+string(filepath.Separator)) || strings.HasPrefix(normalized, "../") {
		return fmt.Errorf("invalid source path %q: path escapes the context directory, the path must stay within the context directory", src)
	}
	return nil
}

// === 复制选项 ===

// CopyOption 配置 COPY 指令。
type CopyOption func(*copyConfig)

// copyConfig 包含 COPY 指令的内部配置。
type copyConfig struct {
	user            string // 执行用户
	mode            int    // 文件权限模式
	forceUpload     *bool  // 是否强制上传
	resolveSymlinks *bool  // 是否解析符号链接
}

// WithCopyUser 设置 COPY 指令的执行用户。
func WithCopyUser(user string) CopyOption {
	return func(c *copyConfig) { c.user = user }
}

// WithCopyMode 设置 COPY 指令的文件权限模式。
func WithCopyMode(mode int) CopyOption {
	return func(c *copyConfig) { c.mode = mode }
}

// WithCopyForceUpload 强制重新上传，即使已缓存。
func WithCopyForceUpload(force bool) CopyOption {
	return func(c *copyConfig) { c.forceUpload = &force }
}

// WithCopyResolveSymlinks 控制 COPY 期间是否解析符号链接。
func WithCopyResolveSymlinks(resolve bool) CopyOption {
	return func(c *copyConfig) { c.resolveSymlinks = &resolve }
}

// applyCopyOpts 应用复制选项并返回配置。
func applyCopyOpts(opts []CopyOption) *copyConfig {
	cfg := &copyConfig{}
	for _, o := range opts {
		o(cfg)
	}
	return cfg
}

// === 构建指令 ===

// addInstruction 添加一条指令并重置 forceNextLayer。
func (t *TemplateBuilder) addInstruction(inst Instruction) {
	inst.Force = t.forceNextLayer
	t.instructions = append(t.instructions, inst)
	t.forceNextLayer = false
}

// RunCmd 添加一条 RUN 指令，在构建期间执行 shell 命令。
func (t *TemplateBuilder) RunCmd(command string) *TemplateBuilder {
	t.addInstruction(Instruction{
		Type: InstructionRun, Args: []string{command},
	})
	return t
}

// RunCmds 添加一条 RUN 指令，多个命令用 && 连接。
func (t *TemplateBuilder) RunCmds(commands []string) *TemplateBuilder {
	return t.RunCmd(strings.Join(commands, " && "))
}

// RunCmdAsUser 添加一条以特定用户身份运行的 RUN 指令。
func (t *TemplateBuilder) RunCmdAsUser(command, user string) *TemplateBuilder {
	args := []string{command}
	if user != "" {
		args = append(args, user)
	}
	t.addInstruction(Instruction{
		Type: InstructionRun, Args: args,
	})
	return t
}

// RunCmdsAsUser 添加一条以特定用户身份运行的 RUN 指令，多个命令用 && 连接。
func (t *TemplateBuilder) RunCmdsAsUser(commands []string, user string) *TemplateBuilder {
	return t.RunCmdAsUser(strings.Join(commands, " && "), user)
}

// Copy 添加一条 COPY 指令，将本地上下文中的文件复制到模板中。
// src 相对于文件上下文路径；dest 是模板中的绝对路径。
// 接受可选的 CopyOption 配置用户、权限、强制上传和符号链接解析。
func (t *TemplateBuilder) Copy(src, dest string, opts ...CopyOption) *TemplateBuilder {
	if err := validateRelativePath(src); err != nil {
		if t.err == nil {
			t.err = err
		}
		return t
	}
	cfg := applyCopyOpts(opts)
	modeStr := ""
	if cfg.mode > 0 {
		modeStr = fmt.Sprintf("%04o", cfg.mode)
	}
	inst := Instruction{
		Type:            InstructionCopy,
		Args:            []string{src, dest, cfg.user, modeStr},
		ForceUpload:     cfg.forceUpload,
		ResolveSymlinks: cfg.resolveSymlinks,
	}
	t.addInstruction(inst)
	return t
}

// CopyWithOptions 添加带有用户和权限选项的 COPY 指令。
// 已弃用：请改用 Copy 配合 WithCopyUser 和 WithCopyMode。
func (t *TemplateBuilder) CopyWithOptions(src, dest, user string, mode int) *TemplateBuilder {
	return t.Copy(src, dest, WithCopyUser(user), WithCopyMode(mode))
}

// CopyItem 定义 CopyItems 的单个文件复制操作。
type CopyItem struct {
	Src             string // 源路径（相对路径）
	Dest            string // 目标路径（绝对路径）
	User            string // 执行用户
	Mode            int    // 文件权限模式
	ForceUpload     *bool  // 是否强制上传
	ResolveSymlinks *bool  // 是否解析符号链接
}

// CopyItems 从 CopyItem 列表添加多条 COPY 指令。
func (t *TemplateBuilder) CopyItems(items []CopyItem) *TemplateBuilder {
	for _, item := range items {
		var opts []CopyOption
		if item.User != "" {
			opts = append(opts, WithCopyUser(item.User))
		}
		if item.Mode > 0 {
			opts = append(opts, WithCopyMode(item.Mode))
		}
		if item.ForceUpload != nil {
			opts = append(opts, WithCopyForceUpload(*item.ForceUpload))
		}
		if item.ResolveSymlinks != nil {
			opts = append(opts, WithCopyResolveSymlinks(*item.ResolveSymlinks))
		}
		t.Copy(item.Src, item.Dest, opts...)
	}
	return t
}

// SetEnvs 添加一条 ENV 指令来设置环境变量。
// 如果 envs 为空则立即返回（无操作）。
func (t *TemplateBuilder) SetEnvs(envs map[string]string) *TemplateBuilder {
	if len(envs) == 0 {
		return t
	}
	keys := make([]string, 0, len(envs))
	for k := range envs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	args := make([]string, 0, len(envs)*2)
	for _, k := range keys {
		args = append(args, k, envs[k])
	}
	t.addInstruction(Instruction{
		Type: InstructionEnv, Args: args,
	})
	return t
}

// SetWorkdir 添加一条 WORKDIR 指令来设置工作目录。
func (t *TemplateBuilder) SetWorkdir(workdir string) *TemplateBuilder {
	t.addInstruction(Instruction{
		Type: InstructionWorkdir, Args: []string{workdir},
	})
	return t
}

// SetUser 添加一条 USER 指令来设置后续命令的执行用户。
func (t *TemplateBuilder) SetUser(user string) *TemplateBuilder {
	t.addInstruction(Instruction{
		Type: InstructionUser, Args: []string{user},
	})
	return t
}

// === 包管理器安装选项 ===

// PipInstallOption 配置 PipInstall 行为。
type PipInstallOption func(*pipInstallConfig)

// pipInstallConfig 包含 pip 安装的内部配置。
type pipInstallConfig struct {
	global bool // 是否全局安装，默认为 true
}

// WithPipGlobal 设置是否全局安装（以 root 身份）。默认为 true。
func WithPipGlobal(global bool) PipInstallOption {
	return func(c *pipInstallConfig) { c.global = global }
}

// PipInstall 添加一条 RUN 指令，通过 pip 安装 Python 包。
// 默认为全局安装（以 root 身份运行）。使用 WithPipGlobal(false) 进行用户安装。
// 如果未指定包，则运行 "pip install ."（从当前目录安装）。
func (t *TemplateBuilder) PipInstall(packages []string, opts ...PipInstallOption) *TemplateBuilder {
	cfg := &pipInstallConfig{global: true}
	for _, o := range opts {
		o(cfg)
	}
	cmd := "pip install"
	if !cfg.global {
		cmd += " --user"
	}
	if len(packages) > 0 {
		cmd += " " + strings.Join(packages, " ")
	} else {
		cmd += " ."
	}
	if cfg.global {
		return t.RunCmdAsUser(cmd, "root")
	}
	return t.RunCmd(cmd)
}

// NpmInstallOption 配置 NpmInstall 行为。
type NpmInstallOption func(*npmInstallConfig)

// npmInstallConfig 包含 npm 安装的内部配置。
type npmInstallConfig struct {
	global bool // 是否全局安装
	dev    bool // 是否作为开发依赖安装
}

// WithNpmGlobal 设置是否全局安装。
func WithNpmGlobal(global bool) NpmInstallOption {
	return func(c *npmInstallConfig) { c.global = global }
}

// WithNpmDev 设置是否作为开发依赖安装。
func WithNpmDev(dev bool) NpmInstallOption {
	return func(c *npmInstallConfig) { c.dev = dev }
}

// NpmInstall 添加一条 RUN 指令，通过 npm 安装 Node.js 包。
// 如果未指定包，则运行 "npm install"（从 package.json 安装）。
func (t *TemplateBuilder) NpmInstall(packages []string, opts ...NpmInstallOption) *TemplateBuilder {
	cfg := &npmInstallConfig{}
	for _, o := range opts {
		o(cfg)
	}
	cmd := "npm install"
	if cfg.global {
		cmd += " -g"
	}
	if cfg.dev {
		cmd += " --save-dev"
	}
	if len(packages) > 0 {
		cmd += " " + strings.Join(packages, " ")
	}
	if cfg.global {
		return t.RunCmdAsUser(cmd, "root")
	}
	return t.RunCmd(cmd)
}

// BunInstallOption 配置 BunInstall 行为。
type BunInstallOption func(*bunInstallConfig)

// bunInstallConfig 包含 bun 安装的内部配置。
type bunInstallConfig struct {
	global bool // 是否全局安装
	dev    bool // 是否作为开发依赖安装
}

// WithBunGlobal 设置是否全局安装。
func WithBunGlobal(global bool) BunInstallOption {
	return func(c *bunInstallConfig) { c.global = global }
}

// WithBunDev 设置是否作为开发依赖安装。
func WithBunDev(dev bool) BunInstallOption {
	return func(c *bunInstallConfig) { c.dev = dev }
}

// BunInstall 添加一条 RUN 指令，通过 bun 安装包。
// 如果未指定包，则运行 "bun install"。
func (t *TemplateBuilder) BunInstall(packages []string, opts ...BunInstallOption) *TemplateBuilder {
	cfg := &bunInstallConfig{}
	for _, o := range opts {
		o(cfg)
	}
	cmd := "bun install"
	if cfg.global {
		cmd += " -g"
	}
	if cfg.dev {
		cmd += " --dev"
	}
	if len(packages) > 0 {
		cmd += " " + strings.Join(packages, " ")
	}
	if cfg.global {
		return t.RunCmdAsUser(cmd, "root")
	}
	return t.RunCmd(cmd)
}

// AptInstallOption 配置 AptInstall 行为。
type AptInstallOption func(*aptInstallConfig)

// aptInstallConfig 包含 apt 安装的内部配置。
type aptInstallConfig struct {
	noInstallRecommends bool // 是否禁用推荐包安装
}

// WithAptNoInstallRecommends 禁用推荐包的安装。
func WithAptNoInstallRecommends(noRec bool) AptInstallOption {
	return func(c *aptInstallConfig) { c.noInstallRecommends = noRec }
}

// AptInstall 添加一条 RUN 指令，通过 apt-get 安装 Debian/Ubuntu 包。
func (t *TemplateBuilder) AptInstall(packages []string, opts ...AptInstallOption) *TemplateBuilder {
	if len(packages) == 0 {
		return t
	}
	cfg := &aptInstallConfig{}
	for _, o := range opts {
		o(cfg)
	}
	installCmd := "DEBIAN_FRONTEND=noninteractive DEBCONF_NOWARNINGS=yes apt-get install -y"
	if cfg.noInstallRecommends {
		installCmd += " --no-install-recommends"
	}
	installCmd += " " + strings.Join(packages, " ")
	return t.RunCmdsAsUser([]string{"apt-get update", installCmd}, "root")
}

// === 便捷方法选项 ===

// ConvenienceOption 配置 Remove、Rename、MakeDir、MakeSymlink 等便捷方法。
type ConvenienceOption func(*convenienceConfig)

// convenienceConfig 包含便捷方法的内部配置。
type convenienceConfig struct {
	user string // 执行用户
}

// WithConvenienceUser 设置便捷方法命令的执行用户。
func WithConvenienceUser(user string) ConvenienceOption {
	return func(c *convenienceConfig) { c.user = user }
}

// applyConvenienceOpts 应用便捷方法选项并返回配置。
func applyConvenienceOpts(opts []ConvenienceOption) *convenienceConfig {
	cfg := &convenienceConfig{}
	for _, o := range opts {
		o(cfg)
	}
	return cfg
}

// === 便捷方法 ===

// Remove 添加一条 RUN 指令来删除文件或目录。
// 接受可选的 WithConvenienceUser 来以特定用户身份运行。
func (t *TemplateBuilder) Remove(path string, force, recursive bool, opts ...ConvenienceOption) *TemplateBuilder {
	cmd := "rm"
	if recursive {
		cmd += " -r"
	}
	if force {
		cmd += " -f"
	}
	cmd += " " + shellQuote(path)
	cfg := applyConvenienceOpts(opts)
	if cfg.user != "" {
		return t.RunCmdAsUser(cmd, cfg.user)
	}
	return t.RunCmd(cmd)
}

// RemoveMultiple 添加一条 RUN 指令来删除多个路径。
// 接受可选的 WithConvenienceUser 来以特定用户身份运行。
func (t *TemplateBuilder) RemoveMultiple(paths []string, force, recursive bool, opts ...ConvenienceOption) *TemplateBuilder {
	if len(paths) == 0 {
		return t
	}
	cmd := "rm"
	if recursive {
		cmd += " -r"
	}
	if force {
		cmd += " -f"
	}
	for _, p := range paths {
		cmd += " " + shellQuote(p)
	}
	cfg := applyConvenienceOpts(opts)
	if cfg.user != "" {
		return t.RunCmdAsUser(cmd, cfg.user)
	}
	return t.RunCmd(cmd)
}

// Rename 添加一条 RUN 指令来重命名/移动文件或目录。
// 接受可选的 WithConvenienceUser 来以特定用户身份运行。
func (t *TemplateBuilder) Rename(src, dest string, force bool, opts ...ConvenienceOption) *TemplateBuilder {
	cmd := "mv"
	if force {
		cmd += " -f"
	}
	cmd += " " + shellQuote(src) + " " + shellQuote(dest)
	cfg := applyConvenienceOpts(opts)
	if cfg.user != "" {
		return t.RunCmdAsUser(cmd, cfg.user)
	}
	return t.RunCmd(cmd)
}

// MakeDir 添加一条 RUN 指令，使用 mkdir -p 创建目录。
// 接受可选的 WithConvenienceUser 来以特定用户身份运行。
func (t *TemplateBuilder) MakeDir(path string, mode int, opts ...ConvenienceOption) *TemplateBuilder {
	cmd := "mkdir -p"
	if mode > 0 {
		cmd += fmt.Sprintf(" -m %04o", mode)
	}
	cmd += " " + shellQuote(path)
	cfg := applyConvenienceOpts(opts)
	if cfg.user != "" {
		return t.RunCmdAsUser(cmd, cfg.user)
	}
	return t.RunCmd(cmd)
}

// MakeDirs 创建多个目录。
// 接受可选的 WithConvenienceUser 来以特定用户身份运行。
func (t *TemplateBuilder) MakeDirs(paths []string, mode int, opts ...ConvenienceOption) *TemplateBuilder {
	if len(paths) == 0 {
		return t
	}
	cmd := "mkdir -p"
	if mode > 0 {
		cmd += fmt.Sprintf(" -m %04o", mode)
	}
	for _, p := range paths {
		cmd += " " + shellQuote(p)
	}
	cfg := applyConvenienceOpts(opts)
	if cfg.user != "" {
		return t.RunCmdAsUser(cmd, cfg.user)
	}
	return t.RunCmd(cmd)
}

// MakeSymlink 添加一条 RUN 指令来创建符号链接。
// 接受可选的 WithConvenienceUser 来以特定用户身份运行。
func (t *TemplateBuilder) MakeSymlink(src, dest string, force bool, opts ...ConvenienceOption) *TemplateBuilder {
	cmd := "ln -s"
	if force {
		cmd += " -f"
	}
	cmd += " " + shellQuote(src) + " " + shellQuote(dest)
	cfg := applyConvenienceOpts(opts)
	if cfg.user != "" {
		return t.RunCmdAsUser(cmd, cfg.user)
	}
	return t.RunCmd(cmd)
}

// GitClone 添加一条 RUN 指令，在构建期间克隆 git 仓库。
func (t *TemplateBuilder) GitClone(url string, opts ...GitCloneOption) *TemplateBuilder {
	cfg := applyGitCloneOpts(opts)
	cmd := "git clone " + shellQuote(url)
	if cfg.branch != "" {
		cmd += " --branch " + shellQuote(cfg.branch) + " --single-branch"
	}
	if cfg.depth > 0 {
		cmd += fmt.Sprintf(" --depth %d", cfg.depth)
	}
	if cfg.path != "" {
		cmd += " " + shellQuote(cfg.path)
	}
	if cfg.user != "" {
		return t.RunCmdAsUser(cmd, cfg.user)
	}
	return t.RunCmd(cmd)
}

// GitCloneOption 配置 GitClone 调用。
type GitCloneOption func(*gitCloneConfig)

// gitCloneConfig 包含 git 克隆的内部配置。
type gitCloneConfig struct {
	path   string // 目标路径
	branch string // 分支名称
	depth  int    // 克隆深度
	user   string // 执行用户
}

// WithGitClonePath 设置克隆的目标路径。
func WithGitClonePath(path string) GitCloneOption {
	return func(c *gitCloneConfig) { c.path = path }
}

// WithGitCloneBranch 设置要克隆的分支。
func WithGitCloneBranch(branch string) GitCloneOption {
	return func(c *gitCloneConfig) { c.branch = branch }
}

// WithGitCloneDepth 设置克隆深度（浅克隆）。
func WithGitCloneDepth(depth int) GitCloneOption {
	return func(c *gitCloneConfig) { c.depth = depth }
}

// WithGitCloneUser 设置 git clone 命令的执行用户。
func WithGitCloneUser(user string) GitCloneOption {
	return func(c *gitCloneConfig) { c.user = user }
}

// applyGitCloneOpts 应用 git 克隆选项并返回配置。
func applyGitCloneOpts(opts []GitCloneOption) *gitCloneConfig {
	cfg := &gitCloneConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// AddMCPServer 添加一条 RUN 指令，通过 mcp-gateway 安装 MCP 服务器。
// 需要预装了 mcp-gateway 的基础模板（例如 "mcp-gateway"）。
// 如果基础模板不是 "mcp-gateway"，则返回延迟错误。
func (t *TemplateBuilder) AddMCPServer(servers ...string) *TemplateBuilder {
	if t.baseTemplate != "mcp-gateway" {
		if t.err == nil {
			t.err = fmt.Errorf("MCP servers can only be added to mcp-gateway template, got base template %q", t.baseTemplate)
		}
		return t
	}
	cmd := "mcp-gateway pull " + strings.Join(servers, " ")
	return t.RunCmdAsUser(cmd, "root")
}

// BetaDevContainerPrebuild 添加一条 RUN 指令来预构建 devcontainer。
// 需要基础模板为 "devcontainer"。
func (t *TemplateBuilder) BetaDevContainerPrebuild(devcontainerDirectory string) *TemplateBuilder {
	if t.baseTemplate != "devcontainer" {
		if t.err == nil {
			t.err = fmt.Errorf("devcontainers can only be used with the devcontainer template, got base template %q", t.baseTemplate)
		}
		return t
	}
	cmd := "devcontainer build --workspace-folder " + shellQuote(devcontainerDirectory)
	return t.RunCmdAsUser(cmd, "root")
}

// BetaSetDevContainerStart 设置 devcontainer 沙箱的启动命令。
// 需要基础模板为 "devcontainer"。
// 设置启动命令和就绪检查，应作为链式调用的最后一个方法。
func (t *TemplateBuilder) BetaSetDevContainerStart(devcontainerDirectory string) *TemplateBuilder {
	if t.baseTemplate != "devcontainer" {
		if t.err == nil {
			t.err = fmt.Errorf("devcontainers can only be used with the devcontainer template, got base template %q", t.baseTemplate)
		}
		return t
	}
	dir := shellQuote(devcontainerDirectory)
	startCmd := fmt.Sprintf(
		"sudo devcontainer up --workspace-folder %s && sudo /prepare-exec.sh %s | sudo tee /devcontainer.sh > /dev/null && sudo chmod +x /devcontainer.sh && sudo touch /devcontainer.up",
		dir, dir,
	)
	return t.SetStartCmd(startCmd, WaitForFile("/devcontainer.up"))
}

// SkipCache 标记下一条指令跳过构建缓存。
func (t *TemplateBuilder) SkipCache() *TemplateBuilder {
	t.forceNextLayer = true
	return t
}

// SetStartCmd 设置沙箱的启动命令和就绪检查。
func (t *TemplateBuilder) SetStartCmd(startCmd string, readyCmd ReadyCmd) *TemplateBuilder {
	t.startCmd = startCmd
	t.readyCmd = readyCmd.cmd
	return t
}

// SetStartCmdRaw 使用原始 shell 字符串设置启动命令和就绪检查。
func (t *TemplateBuilder) SetStartCmdRaw(startCmd string, readyCmd string) *TemplateBuilder {
	t.startCmd = startCmd
	t.readyCmd = readyCmd
	return t
}

// SetReadyCmd 仅设置就绪检查命令。
func (t *TemplateBuilder) SetReadyCmd(readyCmd ReadyCmd) *TemplateBuilder {
	t.readyCmd = readyCmd.cmd
	return t
}

// SetReadyCmdRaw 从原始 shell 字符串设置就绪检查命令。
func (t *TemplateBuilder) SetReadyCmdRaw(readyCmd string) *TemplateBuilder {
	t.readyCmd = readyCmd
	return t
}

// === 序列化 ===

// serialize 将模板构建器序列化为 API 请求数据。
func (t *TemplateBuilder) serialize() templateData {
	data := templateData{
		Steps: t.instructions,
		Force: t.force,
	}
	if t.baseImage != "" {
		data.FromImage = t.baseImage
	}
	if t.baseTemplate != "" {
		data.FromTemplate = t.baseTemplate
	}
	if t.registryConfig != nil {
		data.FromImageRegistry = t.registryConfig
	}
	if t.startCmd != "" {
		data.StartCmd = t.startCmd
	}
	if t.readyCmd != "" {
		data.ReadyCmd = t.readyCmd
	}
	return data
}

// ToJSON 将模板序列化为 JSON 字符串。
func (t *TemplateBuilder) ToJSON() (string, error) {
	data := t.serialize()
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ToDockerfile 将模板转换为 Dockerfile 格式。
// 基于其他 E2B 模板的模板无法转换。
func (t *TemplateBuilder) ToDockerfile() (string, error) {
	if t.baseTemplate != "" {
		return "", fmt.Errorf("cannot convert template built from another template to Dockerfile")
	}
	if t.baseImage == "" {
		return "", fmt.Errorf("no base image specified for template")
	}

	var buf strings.Builder
	buf.WriteString("FROM " + t.baseImage + "\n")

	for _, inst := range t.instructions {
		switch inst.Type {
		case InstructionRun:
			if len(inst.Args) > 0 {
				buf.WriteString("RUN " + inst.Args[0] + "\n")
			}
		case InstructionCopy:
			if len(inst.Args) >= 2 {
				buf.WriteString("COPY " + inst.Args[0] + " " + inst.Args[1] + "\n")
			}
		case InstructionEnv:
			var pairs []string
			for i := 0; i+1 < len(inst.Args); i += 2 {
				pairs = append(pairs, inst.Args[i]+"="+inst.Args[i+1])
			}
			if len(pairs) > 0 {
				buf.WriteString("ENV " + strings.Join(pairs, " ") + "\n")
			}
		case InstructionWorkdir:
			if len(inst.Args) > 0 {
				buf.WriteString("WORKDIR " + inst.Args[0] + "\n")
			}
		case InstructionUser:
			if len(inst.Args) > 0 {
				buf.WriteString("USER " + inst.Args[0] + "\n")
			}
		default:
			buf.WriteString(string(inst.Type) + " " + strings.Join(inst.Args, " ") + "\n")
		}
	}

	if t.startCmd != "" {
		buf.WriteString("ENTRYPOINT " + t.startCmd + "\n")
	}

	return buf.String(), nil
}

// === 构建选项 ===

// buildConfig 包含模板构建的内部配置。
type buildConfig struct {
	tags        []string       // 模板标签
	cpuCount    int            // CPU 数量
	memoryMB    int            // 内存限制（MB）
	skipCache   bool           // 是否跳过缓存
	onBuildLogs func(LogEntry) // 构建日志回调函数
}

// BuildOption 配置模板构建行为。
type BuildOption func(*buildConfig)

// WithBuildTags 设置构建模板的标签。
func WithBuildTags(tags []string) BuildOption {
	return func(c *buildConfig) { c.tags = tags }
}

// WithBuildCPUCount 设置构建的 CPU 数量。
func WithBuildCPUCount(count int) BuildOption {
	return func(c *buildConfig) { c.cpuCount = count }
}

// WithBuildMemoryMB 设置构建的内存限制（MB）。
func WithBuildMemoryMB(mb int) BuildOption {
	return func(c *buildConfig) { c.memoryMB = mb }
}

// WithBuildSkipCache 强制跳过构建缓存。
func WithBuildSkipCache(skip bool) BuildOption {
	return func(c *buildConfig) { c.skipCache = skip }
}

// WithOnBuildLogs 设置接收结构化构建日志条目的回调函数。
func WithOnBuildLogs(fn func(LogEntry)) BuildOption {
	return func(c *buildConfig) { c.onBuildLogs = fn }
}

// DefaultBuildLogger 返回一个日志回调函数，将格式化的日志条目
// 带时间戳和日志级别打印到 os.Stderr。
func DefaultBuildLogger() func(LogEntry) {
	start := time.Now()
	return func(entry LogEntry) {
		elapsed := time.Since(start).Seconds()
		ts := entry.Timestamp.Format("15:04:05")
		fmt.Fprintf(os.Stderr, "%5.1fs | %s %-5s %s\n",
			elapsed, ts, strings.ToUpper(entry.Level), entry.Message)
	}
}

// DefaultBuildLoggerWithLevel 返回一个按最低级别过滤的日志回调函数。
// 有效级别："debug"、"info"、"warn"、"error"。未知级别默认为 "info"。
func DefaultBuildLoggerWithLevel(minLevel string) func(LogEntry) {
	levelOrder := map[string]int{"debug": 0, "info": 1, "warn": 2, "error": 3}
	minOrd, ok := levelOrder[minLevel]
	if !ok {
		minOrd = levelOrder["info"]
	}
	inner := DefaultBuildLogger()
	return func(entry LogEntry) {
		entryOrd, ok := levelOrder[entry.Level]
		if !ok {
			entryOrd = levelOrder["info"]
		}
		if entryOrd >= minOrd {
			inner(entry)
		}
	}
}

// applyBuildOpts 应用构建选项并返回配置。
func applyBuildOpts(opts []BuildOption) *buildConfig {
	cfg := &buildConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// === 构建 API 请求/响应类型 ===

// buildTemplateRequest 是发起模板构建的请求体。
type buildTemplateRequest struct {
	Alias     string       `json:"alias,omitempty"`    // 模板名称/别名
	BuildDesc templateData `json:"buildDescription"`   // 构建描述
	CPUCount  int          `json:"cpuCount,omitempty"` // CPU 数量
	MemoryMB  int          `json:"memoryMB,omitempty"` // 内存限制（MB）
	StartCmd  string       `json:"startCmd,omitempty"` // 启动命令
	Tags      []string     `json:"tags,omitempty"`     // 标签列表
}

// buildTemplateResponse 是发起模板构建的响应。
type buildTemplateResponse struct {
	TemplateID string `json:"templateID"` // 模板 ID
	BuildID    string `json:"buildID"`    // 构建 ID
}

// buildStatusAPIResponse 是构建状态的原始 API 响应。
type buildStatusAPIResponse struct {
	TemplateID string              `json:"templateID"`           // 模板 ID
	BuildID    string              `json:"buildID"`              // 构建 ID
	Status     TemplateBuildStatus `json:"status"`               // 构建状态
	Logs       []string            `json:"logs"`                 // 日志行
	LogEntries []LogEntry          `json:"logEntries,omitempty"` // 结构化日志条目
	Reason     *BuildStatusReason  `json:"reason,omitempty"`     // 状态原因
}

// === 构建 API 方法 ===

// BuildTemplate 发起模板构建并返回构建信息。
// template 参数通过 TemplateBuilder 定义构建指令。
// 使用 WithBuildTags、WithBuildCPUCount、WithBuildMemoryMB 和 WithOnBuildLogs 进行配置。
func (c *Client) BuildTemplate(ctx context.Context, template *TemplateBuilder, name string, opts ...BuildOption) (*BuildInfo, error) {
	if template.err != nil {
		return nil, &TemplateError{SandboxError{Message: template.err.Error(), Cause: template.err}}
	}

	cfg := applyBuildOpts(opts)

	data := template.serialize()

	if cfg.skipCache {
		data.Force = true
	}

	reqBody := buildTemplateRequest{
		Alias:     name,
		BuildDesc: data,
		CPUCount:  cfg.cpuCount,
		MemoryMB:  cfg.memoryMB,
		Tags:      cfg.tags,
	}

	var resp buildTemplateResponse
	err := c.doRequest(ctx, "POST", "/templates", reqBody, &resp)
	if err != nil {
		return nil, err
	}

	info := &BuildInfo{
		TemplateID: resp.TemplateID,
		BuildID:    resp.BuildID,
		Name:       name,
		Tags:       cfg.tags,
	}

	// 如果设置了 onBuildLogs，则轮询日志直到构建完成
	if cfg.onBuildLogs != nil {
		_, err := c.WaitForBuild(ctx, resp.TemplateID, resp.BuildID, WithOnBuildLogs(cfg.onBuildLogs))
		if err != nil {
			return info, err
		}
	}

	return info, nil
}

// GetBuildStatus 查询模板构建的当前状态。
// logsOffset 控制日志条目的分页（0 = 从开头）。
func (c *Client) GetBuildStatus(ctx context.Context, templateID, buildID string, logsOffset int) (*BuildStatusResponse, error) {
	path := fmt.Sprintf("/templates/%s/builds/%s/status?logsOffset=%d",
		templateID, buildID, logsOffset)

	var resp buildStatusAPIResponse
	err := c.doRequest(ctx, "GET", path, nil, &resp)
	if err != nil {
		return nil, err
	}

	return &BuildStatusResponse{
		BuildID:    resp.BuildID,
		TemplateID: resp.TemplateID,
		Status:     resp.Status,
		Logs:       resp.Logs,
		LogEntries: resp.LogEntries,
		Reason:     resp.Reason,
	}, nil
}

// WaitForBuild 轮询构建状态直到构建完成或失败。
// 返回最终构建状态。如果提供了 WithOnBuildLogs，日志行
// 会在出现时流式传递给回调函数。
func (c *Client) WaitForBuild(ctx context.Context, templateID, buildID string, opts ...BuildOption) (*BuildStatusResponse, error) {
	cfg := applyBuildOpts(opts)

	logsOffset := 0
	pollInterval := 200 * time.Millisecond
	const maxPollInterval = 2 * time.Second

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		status, err := c.GetBuildStatus(ctx, templateID, buildID, logsOffset)
		if err != nil {
			return nil, err
		}

		// 如果设置了回调，流式传递新的日志条目
		if cfg.onBuildLogs != nil {
			for _, entry := range status.LogEntries {
				cfg.onBuildLogs(entry)
			}
		}
		logsOffset += len(status.LogEntries)

		switch status.Status {
		case BuildStatusReady:
			return status, nil
		case BuildStatusError:
			msg := "template build failed"
			if status.Reason != nil {
				msg = status.Reason.Message
			}
			return status, &BuildError{
				SandboxError: SandboxError{Message: msg},
				BuildID:      buildID,
				TemplateID:   templateID,
			}
		case BuildStatusBuilding, BuildStatusWaiting:
			// 继续轮询
		default:
			return status, fmt.Errorf("unknown build status: %s", status.Status)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pollInterval):
		}

		// 指数退避：将间隔翻倍，上限为 maxPollInterval
		if pollInterval < maxPollInterval {
			pollInterval *= 2
			if pollInterval > maxPollInterval {
				pollInterval = maxPollInterval
			}
		}
	}
}

// === 模板管理 API ===

// TemplateTagInfo 包含已分配模板标签的信息。
type TemplateTagInfo struct {
	BuildID string   `json:"buildId"` // 构建 ID
	Tags    []string `json:"tags"`    // 标签列表
}

// TemplateExists 检查指定名称的模板是否存在。
func (c *Client) TemplateExists(ctx context.Context, name string) (bool, error) {
	path := "/templates/aliases/" + name
	err := c.doRequest(ctx, "GET", path, nil, nil)
	if err != nil {
		var notFound *NotFoundError
		if errors.As(err, &notFound) {
			return false, nil
		}
		// 403 表示存在但调用者不是所有者
		var forbidden *ForbiddenError
		if errors.As(err, &forbidden) {
			return true, nil
		}
		return false, err
	}
	return true, nil
}

// AssignTemplateTags 将标签分配给已有的模板构建。
// targetName 应为 'name:tag' 格式。
func (c *Client) AssignTemplateTags(ctx context.Context, targetName string, tags []string) (*TemplateTagInfo, error) {
	reqBody := struct {
		Target string   `json:"target"`
		Tags   []string `json:"tags"`
	}{
		Target: targetName,
		Tags:   tags,
	}
	var resp TemplateTagInfo
	err := c.doRequest(ctx, "POST", "/templates/tags", reqBody, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// RemoveTemplateTags 从模板中移除标签。
func (c *Client) RemoveTemplateTags(ctx context.Context, name string, tags []string) error {
	reqBody := struct {
		Name string   `json:"name"`
		Tags []string `json:"tags"`
	}{
		Name: name,
		Tags: tags,
	}
	return c.doRequest(ctx, "DELETE", "/templates/tags", reqBody, nil)
}
