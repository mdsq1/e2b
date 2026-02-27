package e2b

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// Git 通过 shell 命令在沙箱中提供 git 操作功能。
type Git struct {
	commands *Commands // 命令执行模块
}

// === Git 选项 ===

// gitConfig 包含 git 操作的内部配置参数。
type gitConfig struct {
	path          string
	branch        string
	depth         int
	auth          *gitAuth
	initialBranch string
	files         []string
	authorName    string
	authorEmail   string
	resetMode     string
	staged        bool
	force         bool
	setUpstream   bool

	// V2 新增字段
	allowEmpty                  bool              // 是否允许空提交
	bare                        bool              // 是否为裸仓库
	configScope                 string            // 配置作用域："global"、"local"、"system"
	target                      string            // 重置目标
	paths                       []string          // 重置路径列表
	worktree                    *bool             // restore --worktree 选项
	source                      string            // restore --source 选项
	remote                      string            // push/pull 远程名称
	dangerouslyStoreCredentials bool              // 是否在克隆后存储凭据
	gitHost                     string            // DangerouslyAuthenticate 的主机名
	gitProtocol                 string            // DangerouslyAuthenticate 的协议

	// V3 新增字段
	fetch      bool              // RemoteAdd：添加后是否获取
	overwrite  bool              // RemoteAdd：是否覆盖已有远程
	envVars    map[string]string // 通用：git 命令的额外环境变量
	cmdUser    string            // 通用：执行命令的系统用户
	cmdCwd     string            // 通用：工作目录覆盖
	cmdTimeout int               // 通用：命令超时时间（秒）
}

// gitAuth 包含 git 认证信息。
type gitAuth struct {
	Username string // 用户名
	Token    string // 令牌/密码
}

// GitOption 是用于配置 git 操作的函数选项类型。
type GitOption func(*gitConfig)

// WithGitPath 设置仓库路径。
func WithGitPath(path string) GitOption {
	return func(c *gitConfig) { c.path = path }
}

// WithGitBranch 设置分支名称。
func WithGitBranch(branch string) GitOption {
	return func(c *gitConfig) { c.branch = branch }
}

// WithGitDepth 设置克隆深度。
func WithGitDepth(depth int) GitOption {
	return func(c *gitConfig) { c.depth = depth }
}

// WithGitAuth 设置 git 认证凭据（用户名和令牌）。
func WithGitAuth(username, token string) GitOption {
	return func(c *gitConfig) { c.auth = &gitAuth{Username: username, Token: token} }
}

// WithGitInitialBranch 设置初始化仓库时的默认分支名。
func WithGitInitialBranch(branch string) GitOption {
	return func(c *gitConfig) { c.initialBranch = branch }
}

// WithGitFiles 设置要操作的文件列表。
func WithGitFiles(files []string) GitOption {
	return func(c *gitConfig) { c.files = files }
}

// WithGitAuthorName 设置提交作者名称。
func WithGitAuthorName(name string) GitOption {
	return func(c *gitConfig) { c.authorName = name }
}

// WithGitAuthorEmail 设置提交作者邮箱。
func WithGitAuthorEmail(email string) GitOption {
	return func(c *gitConfig) { c.authorEmail = email }
}

// WithGitResetMode 设置重置模式（soft、mixed、hard、merge、keep）。
func WithGitResetMode(mode string) GitOption {
	return func(c *gitConfig) { c.resetMode = mode }
}

// WithGitStaged 设置是否针对暂存区操作。
func WithGitStaged(staged bool) GitOption {
	return func(c *gitConfig) { c.staged = staged }
}

// WithGitForce 设置是否强制执行操作。
func WithGitForce(force bool) GitOption {
	return func(c *gitConfig) { c.force = force }
}

// WithGitSetUpstream 设置是否设置上游分支。
func WithGitSetUpstream(set bool) GitOption {
	return func(c *gitConfig) { c.setUpstream = set }
}

// WithGitAllowEmpty 设置是否允许空提交。
func WithGitAllowEmpty(allow bool) GitOption {
	return func(c *gitConfig) { c.allowEmpty = allow }
}

// WithGitBare 设置是否创建裸仓库。
func WithGitBare(bare bool) GitOption {
	return func(c *gitConfig) { c.bare = bare }
}

// WithGitConfigScope 设置 git 配置的作用域："global"、"local" 或 "system"。
// 当作用域为 "local" 时，必须通过 WithGitPath 提供仓库路径。
func WithGitConfigScope(scope string) GitOption {
	return func(c *gitConfig) { c.configScope = scope }
}

// WithGitTarget 设置重置的目标引用。
func WithGitTarget(target string) GitOption {
	return func(c *gitConfig) { c.target = target }
}

// WithGitPaths 设置重置的路径列表。
func WithGitPaths(paths []string) GitOption {
	return func(c *gitConfig) { c.paths = paths }
}

// WithGitWorktree 设置是否恢复工作树文件。
func WithGitWorktree(worktree bool) GitOption {
	return func(c *gitConfig) { c.worktree = &worktree }
}

// WithGitSource 设置恢复文件的来源引用。
func WithGitSource(source string) GitOption {
	return func(c *gitConfig) { c.source = source }
}

// WithGitRemote 设置远程仓库名称。
func WithGitRemote(remote string) GitOption {
	return func(c *gitConfig) { c.remote = remote }
}

// WithGitDangerouslyStoreCredentials 控制克隆后是否在远程 URL 中存储凭据。
// 默认为 false，即凭据会被清除。
func WithGitDangerouslyStoreCredentials(store bool) GitOption {
	return func(c *gitConfig) { c.dangerouslyStoreCredentials = store }
}

// WithGitHost 设置 git 主机名（用于凭据存储）。
func WithGitHost(host string) GitOption {
	return func(c *gitConfig) { c.gitHost = host }
}

// WithGitProtocol 设置 git 协议（用于凭据存储）。
func WithGitProtocol(protocol string) GitOption {
	return func(c *gitConfig) { c.gitProtocol = protocol }
}

// WithGitFetch 设置添加远程后是否获取。
func WithGitFetch(fetch bool) GitOption {
	return func(c *gitConfig) { c.fetch = fetch }
}

// WithGitOverwrite 设置是否覆盖已有的远程 URL。
func WithGitOverwrite(overwrite bool) GitOption {
	return func(c *gitConfig) { c.overwrite = overwrite }
}

// WithGitEnvVars 设置 git 命令的额外环境变量。
func WithGitEnvVars(envs map[string]string) GitOption {
	return func(c *gitConfig) { c.envVars = envs }
}

// WithGitCmdUser 设置运行 git 命令的系统用户。
func WithGitCmdUser(user string) GitOption {
	return func(c *gitConfig) { c.cmdUser = user }
}

// WithGitCmdCwd 设置 git 命令的工作目录。
func WithGitCmdCwd(cwd string) GitOption {
	return func(c *gitConfig) { c.cmdCwd = cwd }
}

// WithGitCmdTimeout 设置 git 命令的超时时间（秒）。
func WithGitCmdTimeout(seconds int) GitOption {
	return func(c *gitConfig) { c.cmdTimeout = seconds }
}

// applyGitOpts 应用 git 选项并返回配置。
func applyGitOpts(opts []GitOption) *gitConfig {
	cfg := &gitConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// === Git 操作 ===

// Clone 克隆一个 git 仓库。
// 当通过 WithGitAuth 提供凭据时，克隆后会清理远程 URL 中的凭据，
// 除非设置了 WithGitDangerouslyStoreCredentials(true)。
func (g *Git) Clone(ctx context.Context, repoURL string, opts ...GitOption) error {
	cfg := applyGitOpts(opts)

	cloneURL := repoURL
	if cfg.auth != nil {
		cloneURL = g.injectAuth(repoURL, cfg.auth)
	}
	sanitizedURL := stripCredentials(cloneURL)
	shouldStrip := !cfg.dangerouslyStoreCredentials && sanitizedURL != cloneURL

	repoPath := cfg.path
	if shouldStrip && repoPath == "" {
		repoPath = deriveRepoDirFromURL(repoURL)
	}
	if shouldStrip && repoPath == "" {
		return &InvalidArgumentError{SandboxError{Message: "a destination path is required when using credentials without storing them"}}
	}

	cmd := "git clone"
	if cfg.branch != "" {
		cmd += " --branch " + shellQuote(cfg.branch) + " --single-branch"
	}
	if cfg.depth > 0 {
		cmd += fmt.Sprintf(" --depth %d", cfg.depth)
	}
	cmd += " " + shellQuote(cloneURL)
	// 始终显式传递目标路径，以便凭据清理可以找到它
	if repoPath != "" {
		cmd += " " + shellQuote(repoPath)
	}

	_, err := g.run(ctx, cmd, cfg)
	if err != nil {
		return err
	}

	// 克隆后从远程 URL 中清除凭据
	if shouldStrip && repoPath != "" {
		setURLCmd := fmt.Sprintf("git -C %s remote set-url origin %s", shellQuote(repoPath), shellQuote(sanitizedURL))
		g.commands.Run(ctx, setURLCmd, WithCommandEnvVars(map[string]string{"GIT_TERMINAL_PROMPT": "0"})) // best-effort
	}

	return nil
}

// Init 初始化一个新的 git 仓库。
// 支持 WithGitInitialBranch 和 WithGitBare 选项。
func (g *Git) Init(ctx context.Context, path string, opts ...GitOption) error {
	cfg := applyGitOpts(opts)

	cmd := "git init"
	if cfg.bare {
		cmd += " --bare"
	}
	if cfg.initialBranch != "" {
		cmd += " --initial-branch=" + shellQuote(cfg.initialBranch)
	}
	cmd += " " + shellQuote(path)

	_, err := g.run(ctx, cmd, cfg)
	return err
}

// Add 将文件暂存以准备提交。
// 当通过 WithGitFiles 提供特定文件时，仅暂存这些文件。
// 否则，暂存所有更改（git add --all）。
func (g *Git) Add(ctx context.Context, path string, opts ...GitOption) error {
	cfg := applyGitOpts(opts)

	var cmd string
	if len(cfg.files) > 0 {
		files := make([]string, len(cfg.files))
		for i, f := range cfg.files {
			files[i] = shellQuote(f)
		}
		cmd = fmt.Sprintf("git -C %s add %s", shellQuote(path), strings.Join(files, " "))
	} else {
		cmd = fmt.Sprintf("git -C %s add --all", shellQuote(path))
	}

	_, err := g.run(ctx, cmd, cfg)
	return err
}

// Commit 创建一个 git 提交。
func (g *Git) Commit(ctx context.Context, path, message string, opts ...GitOption) error {
	cfg := applyGitOpts(opts)

	if cfg.envVars == nil {
		cfg.envVars = make(map[string]string)
	}
	if cfg.authorName != "" {
		cfg.envVars["GIT_AUTHOR_NAME"] = cfg.authorName
		cfg.envVars["GIT_COMMITTER_NAME"] = cfg.authorName
	}
	if cfg.authorEmail != "" {
		cfg.envVars["GIT_AUTHOR_EMAIL"] = cfg.authorEmail
		cfg.envVars["GIT_COMMITTER_EMAIL"] = cfg.authorEmail
	}

	cmd := fmt.Sprintf("git -C %s commit -m %s", shellQuote(path), shellQuote(message))
	if cfg.allowEmpty {
		cmd += " --allow-empty"
	}

	_, err := g.run(ctx, cmd, cfg)
	return err
}

// Push 将提交推送到远程仓库。
// 支持 WithGitAuth 通过临时修改远程 URL 注入凭据。
func (g *Git) Push(ctx context.Context, path string, opts ...GitOption) error {
	cfg := applyGitOpts(opts)

	if err := validateGitAuth(cfg.auth); err != nil {
		return err
	}

	if cfg.auth != nil && cfg.auth.Username != "" && cfg.auth.Token != "" {
		remote := g.resolveRemoteName(ctx, path, cfg)
		return g.withRemoteCredentials(ctx, path, remote, cfg.auth, func() error {
			return g.pushInternal(ctx, path, cfg)
		})
	}

	return g.pushInternal(ctx, path, cfg)
}

// Pull 从远程仓库拉取更新。
// 支持 WithGitAuth 进行临时凭据注入、WithGitRemote 和 WithGitBranch 选项。
func (g *Git) Pull(ctx context.Context, path string, opts ...GitOption) error {
	cfg := applyGitOpts(opts)

	if err := validateGitAuth(cfg.auth); err != nil {
		return err
	}

	// 当没有显式指定远程和分支时，检查上游分支
	if cfg.remote == "" && cfg.branch == "" {
		if !g.hasUpstream(ctx, path) {
			return &GitUpstreamError{SandboxError{Message: "git pull failed: no upstream branch configured. Pass remote and branch explicitly via WithGitRemote and WithGitBranch."}}
		}
	}

	if cfg.auth != nil && cfg.auth.Username != "" && cfg.auth.Token != "" {
		remote := g.resolveRemoteName(ctx, path, cfg)
		return g.withRemoteCredentials(ctx, path, remote, cfg.auth, func() error {
			return g.pullInternal(ctx, path, cfg)
		})
	}

	return g.pullInternal(ctx, path, cfg)
}

// GitBranches 包含分支列表信息。
type GitBranches struct {
	Current  string   // 当前分支名称
	Branches []string // 所有分支名称
}

// Branches 列出所有分支并识别当前分支。
func (g *Git) Branches(ctx context.Context, path string) (*GitBranches, error) {
	fmtStr := "%(refname:short)\t%(HEAD)"
	cmd := fmt.Sprintf("git -C %s branch --format=%s", shellQuote(path), shellQuote(fmtStr))
	result, err := g.commands.Run(ctx, cmd, WithCommandEnvVars(map[string]string{"GIT_TERMINAL_PROMPT": "0"}))
	if err != nil {
		return nil, g.wrapGitError(err)
	}

	info := &GitBranches{}
	for _, line := range strings.Split(result.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		name := parts[0]
		info.Branches = append(info.Branches, name)
		if len(parts) > 1 && parts[1] == "*" {
			info.Current = name
		}
	}
	return info, nil
}

// CreateBranch 创建一个新分支并切换到该分支（类似 git checkout -b）。
func (g *Git) CreateBranch(ctx context.Context, path, name string, opts ...GitOption) error {
	cfg := applyGitOpts(opts)
	cmd := fmt.Sprintf("git -C %s checkout -b %s", shellQuote(path), shellQuote(name))
	_, err := g.run(ctx, cmd, cfg)
	return err
}

// CheckoutBranch 切换到指定分支。
func (g *Git) CheckoutBranch(ctx context.Context, path, name string, opts ...GitOption) error {
	cfg := applyGitOpts(opts)
	cmd := fmt.Sprintf("git -C %s checkout %s", shellQuote(path), shellQuote(name))
	_, err := g.run(ctx, cmd, cfg)
	return err
}

// DeleteBranch 删除一个分支。
// 默认使用安全删除（-d）。使用 WithGitForce(true) 进行强制删除（-D）。
func (g *Git) DeleteBranch(ctx context.Context, path, name string, opts ...GitOption) error {
	cfg := applyGitOpts(opts)
	flag := "-d"
	if cfg.force {
		flag = "-D"
	}
	cmd := fmt.Sprintf("git -C %s branch %s %s", shellQuote(path), flag, shellQuote(name))
	_, err := g.run(ctx, cmd, cfg)
	return err
}

// GitFileStatus 表示 git 仓库中单个文件的状态。
type GitFileStatus struct {
	Name              string // 相对于仓库根目录的路径
	Status            string // 标准化状态："modified"、"added"、"deleted"、"renamed"、"copied"、"untracked"、"conflict"、"typechange"、"unknown"
	IndexStatus       string // porcelain 输出中的索引状态字符
	WorkingTreeStatus string // porcelain 输出中的工作树状态字符
	Staged            bool   // 更改是否已暂存
	RenamedFrom       string // 重命名前的原始路径，否则为空
}

// GitStatus 包含全面的 git 仓库状态信息。
type GitStatus struct {
	CurrentBranch string          // 当前分支名称，可能为空
	Upstream      string          // 上游分支名称，可能为空
	Ahead         int             // 领先上游的提交数
	Behind        int             // 落后上游的提交数
	Detached      bool            // HEAD 是否处于分离状态
	FileStatus    []GitFileStatus // 所有文件状态
}

// IsClean 当没有更改时返回 true。
func (s *GitStatus) IsClean() bool { return len(s.FileStatus) == 0 }

// HasChanges 当有任何更改时返回 true。
func (s *GitStatus) HasChanges() bool { return len(s.FileStatus) > 0 }

// HasStaged 当有已暂存的更改时返回 true。
func (s *GitStatus) HasStaged() bool {
	for _, f := range s.FileStatus {
		if f.Staged {
			return true
		}
	}
	return false
}

// HasUntracked 当有未跟踪的文件时返回 true。
func (s *GitStatus) HasUntracked() bool {
	for _, f := range s.FileStatus {
		if f.Status == "untracked" {
			return true
		}
	}
	return false
}

// HasConflicts 当有合并冲突时返回 true。
func (s *GitStatus) HasConflicts() bool {
	for _, f := range s.FileStatus {
		if f.Status == "conflict" {
			return true
		}
	}
	return false
}

// TotalCount 返回已更改文件的总数。
func (s *GitStatus) TotalCount() int { return len(s.FileStatus) }

// StagedCount 返回已暂存文件的数量。
func (s *GitStatus) StagedCount() int {
	n := 0
	for _, f := range s.FileStatus {
		if f.Staged {
			n++
		}
	}
	return n
}

// UnstagedCount 返回未暂存文件的数量（包括未跟踪的文件）。
func (s *GitStatus) UnstagedCount() int {
	n := 0
	for _, f := range s.FileStatus {
		if !f.Staged {
			n++
		}
	}
	return n
}

// UntrackedCount 返回未跟踪文件的数量。
func (s *GitStatus) UntrackedCount() int {
	n := 0
	for _, f := range s.FileStatus {
		if f.Status == "untracked" {
			n++
		}
	}
	return n
}

// ConflictCount 返回冲突文件的数量。
func (s *GitStatus) ConflictCount() int {
	n := 0
	for _, f := range s.FileStatus {
		if f.Status == "conflict" {
			n++
		}
	}
	return n
}

// Status 返回包含全面信息的工作树状态。
func (g *Git) Status(ctx context.Context, path string) (*GitStatus, error) {
	cmd := fmt.Sprintf("git -C %s status --porcelain=1 -b", shellQuote(path))
	result, err := g.commands.Run(ctx, cmd, WithCommandEnvVars(map[string]string{"GIT_TERMINAL_PROMPT": "0"}))
	if err != nil {
		return nil, g.wrapGitError(err)
	}
	return parseGitStatus(result.Stdout), nil
}

// Reset 将当前 HEAD 重置到指定状态。
// 支持 WithGitResetMode、WithGitTarget 和 WithGitPaths 选项。
func (g *Git) Reset(ctx context.Context, path string, opts ...GitOption) error {
	cfg := applyGitOpts(opts)

	if cfg.resetMode != "" {
		allowed := map[string]bool{"soft": true, "mixed": true, "hard": true, "merge": true, "keep": true}
		if !allowed[cfg.resetMode] {
			return &InvalidArgumentError{SandboxError{Message: "Reset mode must be one of: soft, mixed, hard, merge, keep."}}
		}
	}

	cmd := fmt.Sprintf("git -C %s reset", shellQuote(path))
	if cfg.resetMode != "" {
		cmd += " --" + cfg.resetMode
	}
	if cfg.target != "" {
		cmd += " " + shellQuote(cfg.target)
	}
	if len(cfg.paths) > 0 {
		cmd += " --"
		for _, p := range cfg.paths {
			cmd += " " + shellQuote(p)
		}
	}

	_, err := g.run(ctx, cmd, cfg)
	return err
}

// Restore 恢复工作树文件。
// 支持 WithGitStaged、WithGitWorktree 和 WithGitSource 选项。
func (g *Git) Restore(ctx context.Context, path string, files []string, opts ...GitOption) error {
	cfg := applyGitOpts(opts)

	if len(files) == 0 {
		return &InvalidArgumentError{SandboxError{Message: "at least one file path is required for restore"}}
	}

	// Resolve worktree/staged flags:
	//   staged=false, worktree=nil  → worktree only (default)
	//   staged=true,  worktree=nil  → staged only
	//   staged=true,  worktree=true → both
	//   staged=false, worktree=false → error
	resolvedWorktree := false
	resolvedStaged := cfg.staged

	if cfg.worktree != nil {
		resolvedWorktree = *cfg.worktree
	} else if !cfg.staged {
		resolvedWorktree = true
	}

	if !resolvedStaged && !resolvedWorktree {
		return &InvalidArgumentError{SandboxError{Message: "at least one of staged or worktree must be true"}}
	}

	cmd := fmt.Sprintf("git -C %s restore", shellQuote(path))
	if resolvedWorktree {
		cmd += " --worktree"
	}
	if resolvedStaged {
		cmd += " --staged"
	}
	if cfg.source != "" {
		cmd += " --source " + shellQuote(cfg.source)
	}
	cmd += " --"
	for _, f := range files {
		cmd += " " + shellQuote(f)
	}

	_, err := g.run(ctx, cmd, cfg)
	return err
}

// RemoteAdd 添加一个远程仓库。
// 支持 WithGitFetch（添加后获取）和 WithGitOverwrite（覆盖已有远程 URL）选项。
func (g *Git) RemoteAdd(ctx context.Context, path, name, remoteURL string, opts ...GitOption) error {
	cfg := applyGitOpts(opts)

	if cfg.overwrite {
		// Try set-url first; if remote doesn't exist, fall through to add
		setCmd := fmt.Sprintf("git -C %s remote set-url %s %s", shellQuote(path), shellQuote(name), shellQuote(remoteURL))
		_, err := g.run(ctx, setCmd, cfg)
		if err != nil {
			// Remote doesn't exist, add it
			addCmd := fmt.Sprintf("git -C %s remote add %s %s", shellQuote(path), shellQuote(name), shellQuote(remoteURL))
			_, err = g.run(ctx, addCmd, cfg)
			if err != nil {
				return err
			}
		}
	} else {
		cmd := fmt.Sprintf("git -C %s remote add %s %s", shellQuote(path), shellQuote(name), shellQuote(remoteURL))
		_, err := g.run(ctx, cmd, cfg)
		if err != nil {
			return err
		}
	}

	if cfg.fetch {
		fetchCmd := fmt.Sprintf("git -C %s fetch %s", shellQuote(path), shellQuote(name))
		_, err := g.run(ctx, fetchCmd, cfg)
		if err != nil {
			return err
		}
	}

	return nil
}

// RemoteGet 获取远程仓库的 URL。
func (g *Git) RemoteGet(ctx context.Context, path, name string) (string, error) {
	cmd := fmt.Sprintf("git -C %s remote get-url %s", shellQuote(path), shellQuote(name))
	result, err := g.commands.Run(ctx, cmd, WithCommandEnvVars(map[string]string{"GIT_TERMINAL_PROMPT": "0"}))
	if err != nil {
		return "", g.wrapGitError(err)
	}
	return strings.TrimSpace(result.Stdout), nil
}

// SetConfig 设置一个 git 配置值。
// 支持 WithGitConfigScope（"global"、"local"、"system"）和 WithGitPath（"local" 作用域时必需）选项。
func (g *Git) SetConfig(ctx context.Context, key, value string, opts ...GitOption) error {
	cfg := applyGitOpts(opts)

	scopeFlag, repoPath, err := resolveConfigScope(cfg.configScope, cfg.path)
	if err != nil {
		return err
	}

	var cmd string
	if repoPath != "" {
		cmd = fmt.Sprintf("git -C %s config %s %s %s", shellQuote(repoPath), scopeFlag, shellQuote(key), shellQuote(value))
	} else {
		cmd = fmt.Sprintf("git config %s %s %s", scopeFlag, shellQuote(key), shellQuote(value))
	}

	_, err = g.run(ctx, cmd, cfg)
	return err
}

// GetConfig 获取一个 git 配置值。
// 当键不存在时返回空字符串（而非错误）。
// 支持 WithGitConfigScope（"global"、"local"、"system"）和 WithGitPath（"local" 作用域时必需）选项。
func (g *Git) GetConfig(ctx context.Context, key string, opts ...GitOption) (string, error) {
	cfg := applyGitOpts(opts)

	scopeFlag, repoPath, err := resolveConfigScope(cfg.configScope, cfg.path)
	if err != nil {
		return "", err
	}

	var cmd string
	if repoPath != "" {
		cmd = fmt.Sprintf("git -C %s config %s %s", shellQuote(repoPath), scopeFlag, shellQuote(key))
	} else {
		cmd = fmt.Sprintf("git config %s %s", scopeFlag, shellQuote(key))
	}

	result, err := g.run(ctx, cmd, cfg)
	if err != nil {
		// 键未找到时返回空字符串而非错误
		exitErr, ok := err.(*CommandExitError)
		if ok && exitErr.ExitCode == 1 {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(result.Stdout), nil
}

// DangerouslyAuthenticate 使用 credential helper store 全局存储 git 凭据。
// 凭据将在沙箱中持久化，可能被代理访问。
// 请尽可能使用短期凭据。
func (g *Git) DangerouslyAuthenticate(ctx context.Context, username, password string, opts ...GitOption) error {
	cfg := applyGitOpts(opts)

	if username == "" || password == "" {
		return &InvalidArgumentError{SandboxError{Message: "both username and password are required to authenticate git"}}
	}

	// 全局设置 credential.helper = store
	err := g.SetConfig(ctx, "credential.helper", "store", WithGitConfigScope("global"))
	if err != nil {
		return err
	}

	// 通过 git credential approve 存储凭据
	host := "github.com"
	protocol := "https"
	if cfg.gitHost != "" {
		host = cfg.gitHost
	}
	if cfg.gitProtocol != "" {
		protocol = cfg.gitProtocol
	}

	credentialInput := fmt.Sprintf("protocol=%s\nhost=%s\nusername=%s\npassword=%s\n", protocol, host, username, password)
	cmd := fmt.Sprintf("git credential approve <<'__E2B_CRED_EOF__'\n%s__E2B_CRED_EOF__", credentialInput)

	_, err = g.commands.Run(ctx, cmd, WithCommandEnvVars(map[string]string{"GIT_TERMINAL_PROMPT": "0"}))
	return err
}

// ConfigureUser 设置 git user.name 和 user.email 配置。
// 支持 WithGitConfigScope 和 WithGitPath 选项。
func (g *Git) ConfigureUser(ctx context.Context, name, email string, opts ...GitOption) error {
	if name == "" || email == "" {
		return &InvalidArgumentError{SandboxError{Message: "both name and email are required"}}
	}

	err := g.SetConfig(ctx, "user.name", name, opts...)
	if err != nil {
		return err
	}
	return g.SetConfig(ctx, "user.email", email, opts...)
}

// === 内部辅助函数 ===

// run 执行 git 命令并处理错误包装。
func (g *Git) run(ctx context.Context, cmd string, cfg *gitConfig) (*CommandResult, error) {
	envs := map[string]string{"GIT_TERMINAL_PROMPT": "0"}
	if cfg != nil {
		for k, v := range cfg.envVars {
			envs[k] = v
		}
	}
	opts := []CommandOption{WithCommandEnvVars(envs)}
	if cfg != nil {
		if cfg.cmdUser != "" {
			opts = append(opts, WithUser(cfg.cmdUser))
		}
		if cfg.cmdCwd != "" {
			opts = append(opts, WithCwd(cfg.cmdCwd))
		}
		if cfg.cmdTimeout > 0 {
			opts = append(opts, WithCommandTimeout(cfg.cmdTimeout))
		}
	}
	result, err := g.commands.Run(ctx, cmd, opts...)
	if err != nil {
		return result, g.wrapGitError(err)
	}
	return result, nil
}

// withRemoteCredentials 临时在远程 URL 上设置凭据，执行操作，
// 然后无论操作结果如何都恢复原始 URL。
func (g *Git) withRemoteCredentials(ctx context.Context, path, remote string, auth *gitAuth, op func() error) error {
	originalURL, err := g.RemoteGet(ctx, path, remote)
	if err != nil {
		return err
	}

	credURL := g.injectAuth(originalURL, auth)
	setURLCmd := fmt.Sprintf("git -C %s remote set-url %s %s", shellQuote(path), shellQuote(remote), shellQuote(credURL))
	_, err = g.commands.Run(ctx, setURLCmd, WithCommandEnvVars(map[string]string{"GIT_TERMINAL_PROMPT": "0"}))
	if err != nil {
		return err
	}

	opErr := op()

	// 恢复原始 URL（尽力而为，即使操作失败）
	restoreCmd := fmt.Sprintf("git -C %s remote set-url %s %s", shellQuote(path), shellQuote(remote), shellQuote(originalURL))
	_, restoreErr := g.commands.Run(ctx, restoreCmd, WithCommandEnvVars(map[string]string{"GIT_TERMINAL_PROMPT": "0"}))

	if opErr != nil {
		return opErr // already wrapped by run() inside pushInternal/pullInternal
	}
	if restoreErr != nil {
		return restoreErr
	}
	return nil
}

// pushInternal 执行 git push 的内部实现。
func (g *Git) pushInternal(ctx context.Context, path string, cfg *gitConfig) error {
	cmd := fmt.Sprintf("git -C %s push", shellQuote(path))
	if cfg.force {
		cmd += " --force"
	}
	// set_upstream：如果设置了分支，默认启用 setUpstream（与 Python 行为一致）
	setUpstream := cfg.setUpstream
	if cfg.branch != "" && !cfg.setUpstream {
		// 指定分支时，自动启用 set-upstream
		setUpstream = true
	}
	if setUpstream {
		remote := cfg.remote
		if remote == "" {
			remote = "origin"
		}
		branch := cfg.branch
		if branch == "" {
			// 自动检测当前分支
			branch = g.currentBranch(ctx, path)
		}
		if branch != "" {
			cmd += " --set-upstream " + shellQuote(remote) + " " + shellQuote(branch)
		}
	}
	_, err := g.run(ctx, cmd, cfg)
	return err
}

// pullInternal 执行 git pull 的内部实现。
func (g *Git) pullInternal(ctx context.Context, path string, cfg *gitConfig) error {
	cmd := fmt.Sprintf("git -C %s pull", shellQuote(path))
	if cfg.remote != "" {
		cmd += " " + shellQuote(cfg.remote)
	}
	if cfg.branch != "" {
		cmd += " " + shellQuote(cfg.branch)
	}
	_, err := g.run(ctx, cmd, cfg)
	return err
}

// hasUpstream 检查当前分支是否有上游跟踪分支。
func (g *Git) hasUpstream(ctx context.Context, path string) bool {
	cmd := fmt.Sprintf("git -C %s rev-parse --abbrev-ref --symbolic-full-name @{u}", shellQuote(path))
	result, err := g.commands.Run(ctx, cmd, WithCommandEnvVars(map[string]string{"GIT_TERMINAL_PROMPT": "0"}))
	if err != nil {
		return false
	}
	return strings.TrimSpace(result.Stdout) != ""
}

// currentBranch 返回当前分支名称，如果处于分离状态或未知则返回空字符串。
func (g *Git) currentBranch(ctx context.Context, path string) string {
	cmd := fmt.Sprintf("git -C %s rev-parse --abbrev-ref HEAD", shellQuote(path))
	result, err := g.commands.Run(ctx, cmd, WithCommandEnvVars(map[string]string{"GIT_TERMINAL_PROMPT": "0"}))
	if err != nil {
		return ""
	}
	branch := strings.TrimSpace(result.Stdout)
	if branch == "HEAD" {
		return "" // detached HEAD
	}
	return branch
}

// resolveRemoteName 解析 push/pull 操作的远程名称。
// 返回显式配置的远程名称，或从当前分支的上游检测，
// 回退到 "origin"。
func (g *Git) resolveRemoteName(ctx context.Context, path string, cfg *gitConfig) string {
	if cfg.remote != "" {
		return cfg.remote
	}
	// 尝试从当前分支的远程跟踪中检测
	cmd := fmt.Sprintf("git -C %s config branch.%s.remote", shellQuote(path), shellQuote(g.currentBranch(ctx, path)))
	result, err := g.commands.Run(ctx, cmd, WithCommandEnvVars(map[string]string{"GIT_TERMINAL_PROMPT": "0"}))
	if err == nil {
		if remote := strings.TrimSpace(result.Stdout); remote != "" {
			return remote
		}
	}
	return "origin"
}

// validateGitAuth 验证认证凭据是否完整。
func validateGitAuth(auth *gitAuth) error {
	if auth == nil {
		return nil
	}
	if auth.Token != "" && auth.Username == "" {
		return &InvalidArgumentError{SandboxError{Message: "git username is required when password/token is provided"}}
	}
	return nil
}

// wrapGitError 将命令错误包装为特定的 git 错误类型。
func (g *Git) wrapGitError(err error) error {
	if err == nil {
		return nil
	}
	exitErr, ok := err.(*CommandExitError)
	if !ok {
		return err
	}
	message := strings.ToLower(exitErr.Stderr + "\n" + exitErr.Stdout)

	authSnippets := []string{
		"authentication failed",
		"terminal prompts disabled",
		"could not read username",
		"invalid username or password",
		"access denied",
		"permission denied",
		"not authorized",
	}
	for _, snippet := range authSnippets {
		if strings.Contains(message, snippet) {
			return &GitAuthError{SandboxError{Message: exitErr.Stderr, Cause: exitErr}}
		}
	}

	upstreamSnippets := []string{
		"has no upstream branch",
		"no upstream branch",
		"no upstream configured",
		"no tracking information for the current branch",
		"no tracking information",
		"set the remote as upstream",
		"set the upstream branch",
		"please specify which branch you want to merge with",
	}
	for _, snippet := range upstreamSnippets {
		if strings.Contains(message, snippet) {
			return &GitUpstreamError{SandboxError{Message: exitErr.Stderr, Cause: exitErr}}
		}
	}

	return err
}

// injectAuth 将认证信息注入到仓库 URL 中。
func (g *Git) injectAuth(repoURL string, auth *gitAuth) string {
	if auth == nil {
		return repoURL
	}
	parsed, err := url.Parse(repoURL)
	if err != nil {
		return repoURL
	}
	parsed.User = url.UserPassword(auth.Username, auth.Token)
	return parsed.String()
}

// resolveConfigScope 将配置作用域名称映射为 git 标志和仓库路径。
func resolveConfigScope(scope, path string) (flag string, repoPath string, err error) {
	switch strings.ToLower(scope) {
	case "", "global":
		return "--global", "", nil
	case "local":
		if path == "" {
			return "", "", &InvalidArgumentError{SandboxError{Message: "repository path is required when scope is local"}}
		}
		return "--local", path, nil
	case "system":
		return "--system", "", nil
	default:
		return "", "", &InvalidArgumentError{SandboxError{Message: fmt.Sprintf("invalid config scope: %s (must be global, local, or system)", scope)}}
	}
}

// stripCredentials 从 URL 中移除用户名和密码。
func stripCredentials(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	if parsed.User == nil {
		return rawURL
	}
	parsed.User = nil
	return parsed.String()
}

// deriveRepoDirFromURL 从 git URL 中推导仓库目录名称。
func deriveRepoDirFromURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ""
	}
	path := strings.TrimRight(parsed.Path, "/")
	if path == "" {
		return ""
	}
	parts := strings.Split(path, "/")
	last := parts[len(parts)-1]
	if last == "" {
		return ""
	}
	return strings.TrimSuffix(last, ".git")
}

// normalizeFileStatus 将 porcelain 状态字符映射为标准化状态字符串。
func normalizeFileStatus(c byte) string {
	switch c {
	case 'M':
		return "modified"
	case 'A':
		return "added"
	case 'D':
		return "deleted"
	case 'R':
		return "renamed"
	case 'C':
		return "copied"
	case 'U':
		return "conflict"
	case 'T':
		return "typechange"
	case '?':
		return "untracked"
	case '!':
		return "ignored"
	default:
		return "unknown"
	}
}

// deriveStatus 从索引和工作树状态字符中选取主要状态。
func deriveStatus(index, wt byte) string {
	if index == 'U' || wt == 'U' || (index == 'A' && wt == 'A') || (index == 'D' && wt == 'D') {
		return "conflict"
	}
	// 如果索引状态有意义的值，优先使用索引状态作为主要状态
	if index != ' ' && index != '?' {
		return normalizeFileStatus(index)
	}
	return normalizeFileStatus(wt)
}

// parseGitStatus 将 `git status --porcelain=1 -b` 的输出解析为 GitStatus 结构。
func parseGitStatus(output string) *GitStatus {
	status := &GitStatus{}
	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			continue
		}
		// 分支头部行
		if strings.HasPrefix(line, "## ") {
			parseBranchHeader(line[3:], status)
			continue
		}
		// 文件状态行：XY path 或 XY old -> new
		if len(line) < 4 {
			continue
		}
		indexStatus := line[0]
		wtStatus := line[1]
		rest := line[3:]

		fs := GitFileStatus{
			IndexStatus:       string(indexStatus),
			WorkingTreeStatus: string(wtStatus),
			Staged:            indexStatus != ' ' && indexStatus != '?',
		}

		// 处理重命名："old -> new"
		if indexStatus == 'R' || wtStatus == 'R' || indexStatus == 'C' || wtStatus == 'C' {
			if idx := strings.Index(rest, " -> "); idx >= 0 {
				fs.RenamedFrom = rest[:idx]
				fs.Name = rest[idx+4:]
			} else {
				fs.Name = rest
			}
		} else {
			fs.Name = rest
		}

		fs.Status = deriveStatus(indexStatus, wtStatus)
		status.FileStatus = append(status.FileStatus, fs)
	}
	return status
}

// parseBranchHeader 解析 "## branch...upstream [ahead N, behind M]" 行。
func parseBranchHeader(header string, status *GitStatus) {
	// 处理分离的 HEAD
	if strings.HasPrefix(header, "HEAD (no branch)") || strings.Contains(header, "(detached") {
		status.Detached = true
		return
	}

	// 处理 "No commits yet on branch" 或 "Initial commit on branch"
	if strings.HasPrefix(header, "No commits yet on ") {
		status.CurrentBranch = strings.TrimPrefix(header, "No commits yet on ")
		return
	}
	if strings.HasPrefix(header, "Initial commit on ") {
		status.CurrentBranch = strings.TrimPrefix(header, "Initial commit on ")
		return
	}

	// 从方括号中提取 ahead/behind 信息
	aheadBehind := ""
	main := header
	if lbIdx := strings.Index(header, " ["); lbIdx >= 0 {
		rbIdx := strings.Index(header[lbIdx:], "]")
		if rbIdx >= 0 {
			aheadBehind = header[lbIdx+2 : lbIdx+rbIdx]
			main = header[:lbIdx]
		}
	}

	// 解析 branch...upstream
	if dotIdx := strings.Index(main, "..."); dotIdx >= 0 {
		status.CurrentBranch = main[:dotIdx]
		status.Upstream = main[dotIdx+3:]
	} else {
		status.CurrentBranch = main
	}

	// 解析 ahead/behind 计数
	if aheadBehind != "" {
		for _, part := range strings.Split(aheadBehind, ", ") {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "ahead ") {
				fmt.Sscanf(part, "ahead %d", &status.Ahead)
			} else if strings.HasPrefix(part, "behind ") {
				fmt.Sscanf(part, "behind %d", &status.Behind)
			}
		}
	}
}

// shellQuote 对字符串进行 shell 安全转义。
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
