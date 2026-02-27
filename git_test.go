package e2b

import (
	"errors"
	"testing"
)

func TestShellQuote(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "'hello'"},
		{"hello world", "'hello world'"},
		{"it's", "'it'\\''s'"},
		{"", "''"},
	}
	for _, tt := range tests {
		got := shellQuote(tt.input)
		if got != tt.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestInjectAuth(t *testing.T) {
	g := &Git{}

	// No auth
	url := g.injectAuth("https://github.com/user/repo.git", nil)
	if url != "https://github.com/user/repo.git" {
		t.Errorf("expected unchanged URL, got %q", url)
	}

	// With auth
	auth := &gitAuth{Username: "user", Token: "tok123"}
	url = g.injectAuth("https://github.com/user/repo.git", auth)
	if url != "https://user:tok123@github.com/user/repo.git" {
		t.Errorf("unexpected auth URL: %q", url)
	}
}

func TestWrapGitErrorAuth(t *testing.T) {
	g := &Git{}
	err := g.wrapGitError(&CommandExitError{
		Stderr:   "fatal: Authentication failed for 'https://github.com/...'",
		ExitCode: 128,
	})
	var authErr *GitAuthError
	if !errors.As(err, &authErr) {
		t.Errorf("expected *GitAuthError, got %T", err)
	}
}

func TestWrapGitErrorUpstream(t *testing.T) {
	g := &Git{}
	err := g.wrapGitError(&CommandExitError{
		Stderr:   "fatal: The current branch main has no upstream branch.",
		ExitCode: 128,
	})
	var upErr *GitUpstreamError
	if !errors.As(err, &upErr) {
		t.Errorf("expected *GitUpstreamError, got %T", err)
	}
}

func TestWrapGitErrorCouldNotRead(t *testing.T) {
	g := &Git{}
	err := g.wrapGitError(&CommandExitError{
		Stderr:   "fatal: could not read Username for 'https://github.com'",
		ExitCode: 128,
	})
	var authErr *GitAuthError
	if !errors.As(err, &authErr) {
		t.Errorf("expected *GitAuthError for 'could not read', got %T", err)
	}
}

func TestWrapGitErrorPassthrough(t *testing.T) {
	g := &Git{}
	orig := &CommandExitError{Stderr: "some other error", ExitCode: 1}
	err := g.wrapGitError(orig)
	if err != orig {
		t.Errorf("expected original error passthrough, got %T", err)
	}
}

func TestWrapGitErrorNil(t *testing.T) {
	g := &Git{}
	if g.wrapGitError(nil) != nil {
		t.Error("expected nil for nil input")
	}
}

func TestWrapGitErrorNonCommandExit(t *testing.T) {
	g := &Git{}
	orig := &SandboxError{Message: "network"}
	err := g.wrapGitError(orig)
	if err != orig {
		t.Errorf("expected passthrough for non-CommandExitError")
	}
}

func TestGitOptionFunctions(t *testing.T) {
	cfg := &gitConfig{}
	WithGitPath("/repo")(cfg)
	WithGitBranch("main")(cfg)
	WithGitDepth(1)(cfg)
	WithGitAuth("user", "token")(cfg)
	WithGitInitialBranch("dev")(cfg)
	WithGitFiles([]string{"a.txt"})(cfg)
	WithGitAuthorName("Alice")(cfg)
	WithGitAuthorEmail("alice@example.com")(cfg)
	WithGitResetMode("hard")(cfg)
	WithGitStaged(true)(cfg)
	WithGitForce(true)(cfg)
	WithGitSetUpstream(true)(cfg)

	if cfg.path != "/repo" {
		t.Errorf("path: got %q", cfg.path)
	}
	if cfg.branch != "main" {
		t.Errorf("branch: got %q", cfg.branch)
	}
	if cfg.depth != 1 {
		t.Errorf("depth: got %d", cfg.depth)
	}
	if cfg.auth == nil || cfg.auth.Username != "user" || cfg.auth.Token != "token" {
		t.Error("auth not set correctly")
	}
	if cfg.initialBranch != "dev" {
		t.Errorf("initialBranch: got %q", cfg.initialBranch)
	}
	if len(cfg.files) != 1 || cfg.files[0] != "a.txt" {
		t.Errorf("files: got %v", cfg.files)
	}
	if cfg.authorName != "Alice" {
		t.Errorf("authorName: got %q", cfg.authorName)
	}
	if cfg.authorEmail != "alice@example.com" {
		t.Errorf("authorEmail: got %q", cfg.authorEmail)
	}
	if cfg.resetMode != "hard" {
		t.Errorf("resetMode: got %q", cfg.resetMode)
	}
	if !cfg.staged {
		t.Error("staged should be true")
	}
	if !cfg.force {
		t.Error("force should be true")
	}
	if !cfg.setUpstream {
		t.Error("setUpstream should be true")
	}
}

func TestNewGitOptionFunctions(t *testing.T) {
	cfg := &gitConfig{}
	WithGitAllowEmpty(true)(cfg)
	WithGitBare(true)(cfg)
	WithGitConfigScope("local")(cfg)
	WithGitTarget("HEAD~1")(cfg)
	WithGitPaths([]string{"a.txt", "b.txt"})(cfg)
	WithGitWorktree(true)(cfg)
	WithGitSource("HEAD")(cfg)
	WithGitRemote("upstream")(cfg)
	WithGitDangerouslyStoreCredentials(true)(cfg)
	WithGitHost("gitlab.com")(cfg)
	WithGitProtocol("https")(cfg)

	if !cfg.allowEmpty {
		t.Error("allowEmpty should be true")
	}
	if !cfg.bare {
		t.Error("bare should be true")
	}
	if cfg.configScope != "local" {
		t.Errorf("configScope: got %q", cfg.configScope)
	}
	if cfg.target != "HEAD~1" {
		t.Errorf("target: got %q", cfg.target)
	}
	if len(cfg.paths) != 2 {
		t.Errorf("paths: got %v", cfg.paths)
	}
	if cfg.worktree == nil || !*cfg.worktree {
		t.Error("worktree should be *true")
	}
	if cfg.source != "HEAD" {
		t.Errorf("source: got %q", cfg.source)
	}
	if cfg.remote != "upstream" {
		t.Errorf("remote: got %q", cfg.remote)
	}
	if !cfg.dangerouslyStoreCredentials {
		t.Error("dangerouslyStoreCredentials should be true")
	}
	if cfg.gitHost != "gitlab.com" {
		t.Errorf("gitHost: got %q", cfg.gitHost)
	}
	if cfg.gitProtocol != "https" {
		t.Errorf("gitProtocol: got %q", cfg.gitProtocol)
	}
}

// === parseGitStatus tests ===

func TestParseGitStatus_CleanRepo(t *testing.T) {
	output := "## main...origin/main\n"
	status := parseGitStatus(output)

	if status.CurrentBranch != "main" {
		t.Errorf("CurrentBranch: got %q, want %q", status.CurrentBranch, "main")
	}
	if status.Upstream != "origin/main" {
		t.Errorf("Upstream: got %q, want %q", status.Upstream, "origin/main")
	}
	if !status.IsClean() {
		t.Error("expected clean status")
	}
	if status.HasChanges() {
		t.Error("expected no changes")
	}
}

func TestParseGitStatus_ModifiedAndUntracked(t *testing.T) {
	output := "## main\n M file1.txt\n?? newfile.txt\n"
	status := parseGitStatus(output)

	if status.CurrentBranch != "main" {
		t.Errorf("CurrentBranch: got %q", status.CurrentBranch)
	}
	if len(status.FileStatus) != 2 {
		t.Fatalf("expected 2 files, got %d", len(status.FileStatus))
	}

	// file1.txt: modified in working tree, not staged
	f1 := status.FileStatus[0]
	if f1.Name != "file1.txt" {
		t.Errorf("f1.Name: got %q", f1.Name)
	}
	if f1.Status != "modified" {
		t.Errorf("f1.Status: got %q, want 'modified'", f1.Status)
	}
	if f1.Staged {
		t.Error("f1 should not be staged")
	}

	// newfile.txt: untracked
	f2 := status.FileStatus[1]
	if f2.Name != "newfile.txt" {
		t.Errorf("f2.Name: got %q", f2.Name)
	}
	if f2.Status != "untracked" {
		t.Errorf("f2.Status: got %q, want 'untracked'", f2.Status)
	}
	if f2.Staged {
		t.Error("f2 should not be staged")
	}

	if !status.HasUntracked() {
		t.Error("expected HasUntracked true")
	}
}

func TestParseGitStatus_StagedFiles(t *testing.T) {
	output := "## dev\nM  staged.txt\nA  new.txt\nD  deleted.txt\n"
	status := parseGitStatus(output)

	if len(status.FileStatus) != 3 {
		t.Fatalf("expected 3 files, got %d", len(status.FileStatus))
	}

	for _, f := range status.FileStatus {
		if !f.Staged {
			t.Errorf("file %q should be staged", f.Name)
		}
	}

	if !status.HasStaged() {
		t.Error("expected HasStaged true")
	}

	if status.FileStatus[0].Status != "modified" {
		t.Errorf("staged.txt status: got %q", status.FileStatus[0].Status)
	}
	if status.FileStatus[1].Status != "added" {
		t.Errorf("new.txt status: got %q", status.FileStatus[1].Status)
	}
	if status.FileStatus[2].Status != "deleted" {
		t.Errorf("deleted.txt status: got %q", status.FileStatus[2].Status)
	}
}

func TestParseGitStatus_Renamed(t *testing.T) {
	output := "## main\nR  old.txt -> new.txt\n"
	status := parseGitStatus(output)

	if len(status.FileStatus) != 1 {
		t.Fatalf("expected 1 file, got %d", len(status.FileStatus))
	}

	f := status.FileStatus[0]
	if f.Status != "renamed" {
		t.Errorf("status: got %q, want 'renamed'", f.Status)
	}
	if f.Name != "new.txt" {
		t.Errorf("Name: got %q, want 'new.txt'", f.Name)
	}
	if f.RenamedFrom != "old.txt" {
		t.Errorf("RenamedFrom: got %q, want 'old.txt'", f.RenamedFrom)
	}
	if !f.Staged {
		t.Error("renamed file should be staged")
	}
}

func TestParseGitStatus_Conflict(t *testing.T) {
	output := "## main\nUU conflict.txt\n"
	status := parseGitStatus(output)

	if len(status.FileStatus) != 1 {
		t.Fatalf("expected 1 file, got %d", len(status.FileStatus))
	}

	f := status.FileStatus[0]
	if f.Status != "conflict" {
		t.Errorf("status: got %q, want 'conflict'", f.Status)
	}
	if !status.HasConflicts() {
		t.Error("expected HasConflicts true")
	}
}

func TestParseGitStatus_AheadBehind(t *testing.T) {
	output := "## main...origin/main [ahead 3, behind 2]\n"
	status := parseGitStatus(output)

	if status.CurrentBranch != "main" {
		t.Errorf("CurrentBranch: got %q", status.CurrentBranch)
	}
	if status.Upstream != "origin/main" {
		t.Errorf("Upstream: got %q", status.Upstream)
	}
	if status.Ahead != 3 {
		t.Errorf("Ahead: got %d, want 3", status.Ahead)
	}
	if status.Behind != 2 {
		t.Errorf("Behind: got %d, want 2", status.Behind)
	}
}

func TestParseGitStatus_AheadOnly(t *testing.T) {
	output := "## feature...origin/feature [ahead 5]\n"
	status := parseGitStatus(output)

	if status.Ahead != 5 {
		t.Errorf("Ahead: got %d, want 5", status.Ahead)
	}
	if status.Behind != 0 {
		t.Errorf("Behind: got %d, want 0", status.Behind)
	}
}

func TestParseGitStatus_DetachedHead(t *testing.T) {
	output := "## HEAD (no branch)\n M file.txt\n"
	status := parseGitStatus(output)

	if !status.Detached {
		t.Error("expected Detached true")
	}
	if status.CurrentBranch != "" {
		t.Errorf("CurrentBranch should be empty, got %q", status.CurrentBranch)
	}
}

func TestParseGitStatus_NoCommitsYet(t *testing.T) {
	output := "## No commits yet on main\nA  file.txt\n"
	status := parseGitStatus(output)

	if status.CurrentBranch != "main" {
		t.Errorf("CurrentBranch: got %q, want 'main'", status.CurrentBranch)
	}
}

func TestParseGitStatus_InitialCommit(t *testing.T) {
	output := "## Initial commit on master\n"
	status := parseGitStatus(output)

	if status.CurrentBranch != "master" {
		t.Errorf("CurrentBranch: got %q, want 'master'", status.CurrentBranch)
	}
}

func TestParseGitStatus_NoUpstream(t *testing.T) {
	output := "## feature\n"
	status := parseGitStatus(output)

	if status.CurrentBranch != "feature" {
		t.Errorf("CurrentBranch: got %q", status.CurrentBranch)
	}
	if status.Upstream != "" {
		t.Errorf("Upstream should be empty, got %q", status.Upstream)
	}
}

func TestParseGitStatus_EmptyOutput(t *testing.T) {
	status := parseGitStatus("")
	if !status.IsClean() {
		t.Error("empty output should be clean")
	}
}

func TestParseGitStatus_BothAddedConflict(t *testing.T) {
	output := "## main\nAA both_added.txt\n"
	status := parseGitStatus(output)

	if len(status.FileStatus) != 1 {
		t.Fatalf("expected 1 file, got %d", len(status.FileStatus))
	}
	if status.FileStatus[0].Status != "conflict" {
		t.Errorf("AA should be conflict, got %q", status.FileStatus[0].Status)
	}
}

func TestParseGitStatus_BothDeletedConflict(t *testing.T) {
	output := "## main\nDD both_deleted.txt\n"
	status := parseGitStatus(output)

	if len(status.FileStatus) != 1 {
		t.Fatalf("expected 1 file, got %d", len(status.FileStatus))
	}
	if status.FileStatus[0].Status != "conflict" {
		t.Errorf("DD should be conflict, got %q", status.FileStatus[0].Status)
	}
}

// === resolveConfigScope tests ===

func TestResolveConfigScope(t *testing.T) {
	tests := []struct {
		scope   string
		path    string
		flag    string
		repo    string
		wantErr bool
	}{
		{"", "", "--global", "", false},
		{"global", "", "--global", "", false},
		{"local", "/repo", "--local", "/repo", false},
		{"local", "", "", "", true},
		{"system", "", "--system", "", false},
		{"invalid", "", "", "", true},
	}

	for _, tt := range tests {
		flag, repo, err := resolveConfigScope(tt.scope, tt.path)
		if (err != nil) != tt.wantErr {
			t.Errorf("resolveConfigScope(%q, %q): err=%v, wantErr=%v", tt.scope, tt.path, err, tt.wantErr)
			continue
		}
		if err != nil {
			continue
		}
		if flag != tt.flag {
			t.Errorf("resolveConfigScope(%q, %q): flag=%q, want %q", tt.scope, tt.path, flag, tt.flag)
		}
		if repo != tt.repo {
			t.Errorf("resolveConfigScope(%q, %q): repo=%q, want %q", tt.scope, tt.path, repo, tt.repo)
		}
	}
}

// === stripCredentials tests ===

func TestStripCredentials(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://github.com/user/repo.git", "https://github.com/user/repo.git"},
		{"https://user:token@github.com/user/repo.git", "https://github.com/user/repo.git"},
		{"not-a-url", "not-a-url"},
	}
	for _, tt := range tests {
		got := stripCredentials(tt.input)
		if got != tt.want {
			t.Errorf("stripCredentials(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// === deriveRepoDirFromURL tests ===

func TestDeriveRepoDirFromURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://github.com/user/my-repo.git", "my-repo"},
		{"https://github.com/user/my-repo", "my-repo"},
		{"https://github.com/user/my-repo/", "my-repo"},
		{"not-a-url", ""},
		{"ssh://git@github.com/user/repo.git", ""},
	}
	for _, tt := range tests {
		got := deriveRepoDirFromURL(tt.input)
		if got != tt.want {
			t.Errorf("deriveRepoDirFromURL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// === normalizeFileStatus tests ===

func TestNormalizeFileStatus(t *testing.T) {
	tests := []struct {
		c    byte
		want string
	}{
		{'M', "modified"},
		{'A', "added"},
		{'D', "deleted"},
		{'R', "renamed"},
		{'C', "copied"},
		{'U', "conflict"},
		{'T', "typechange"},
		{'?', "untracked"},
		{'!', "ignored"},
		{'X', "unknown"},
	}
	for _, tt := range tests {
		got := normalizeFileStatus(tt.c)
		if got != tt.want {
			t.Errorf("normalizeFileStatus(%c) = %q, want %q", tt.c, got, tt.want)
		}
	}
}

// === deriveStatus tests ===

func TestDeriveStatus(t *testing.T) {
	tests := []struct {
		index byte
		wt    byte
		want  string
	}{
		{'U', ' ', "conflict"},
		{' ', 'U', "conflict"},
		{'A', 'A', "conflict"},
		{'D', 'D', "conflict"},
		{'M', ' ', "modified"},
		{' ', 'M', "modified"},
		{'A', ' ', "added"},
		{'D', ' ', "deleted"},
		{'R', ' ', "renamed"},
		{'?', '?', "untracked"},
	}
	for _, tt := range tests {
		got := deriveStatus(tt.index, tt.wt)
		if got != tt.want {
			t.Errorf("deriveStatus(%c, %c) = %q, want %q", tt.index, tt.wt, got, tt.want)
		}
	}
}

// === GitStatus Count methods ===

func TestGitStatusCountMethods(t *testing.T) {
	status := &GitStatus{
		FileStatus: []GitFileStatus{
			{Name: "a.go", Status: "modified", Staged: true},
			{Name: "b.go", Status: "modified", Staged: false},
			{Name: "c.go", Status: "added", Staged: true},
			{Name: "d.go", Status: "untracked", Staged: false},
			{Name: "e.go", Status: "untracked", Staged: false},
			{Name: "f.go", Status: "conflict", Staged: false},
		},
	}

	if status.TotalCount() != 6 {
		t.Errorf("TotalCount: got %d, want 6", status.TotalCount())
	}
	if status.StagedCount() != 2 {
		t.Errorf("StagedCount: got %d, want 2", status.StagedCount())
	}
	if status.UnstagedCount() != 4 {
		t.Errorf("UnstagedCount: got %d, want 4 (modified + untracked + conflict)", status.UnstagedCount())
	}
	if status.UntrackedCount() != 2 {
		t.Errorf("UntrackedCount: got %d, want 2", status.UntrackedCount())
	}
	if status.ConflictCount() != 1 {
		t.Errorf("ConflictCount: got %d, want 1", status.ConflictCount())
	}

	empty := &GitStatus{}
	if empty.TotalCount() != 0 {
		t.Error("TotalCount on empty should be 0")
	}
}

// === Reset mode validation ===

func TestResetModeValidation(t *testing.T) {
	cfg := applyGitOpts([]GitOption{WithGitResetMode("invalid")})
	if cfg.resetMode != "invalid" {
		t.Fatalf("expected resetMode='invalid', got %q", cfg.resetMode)
	}
	// Validation happens inside Reset(), not in applyGitOpts.
	// We test the allowed set here via the option.
	validModes := []string{"soft", "mixed", "hard", "merge", "keep"}
	for _, m := range validModes {
		cfg := applyGitOpts([]GitOption{WithGitResetMode(m)})
		if cfg.resetMode != m {
			t.Errorf("expected resetMode=%q, got %q", m, cfg.resetMode)
		}
	}
}

// === wrapGitError enhanced snippet tests ===

func TestWrapGitErrorTerminalPrompts(t *testing.T) {
	g := &Git{}
	err := g.wrapGitError(&CommandExitError{
		Stderr:   "fatal: terminal prompts disabled",
		ExitCode: 128,
	})
	var authErr *GitAuthError
	if !errors.As(err, &authErr) {
		t.Errorf("expected *GitAuthError for 'terminal prompts disabled', got %T", err)
	}
}

func TestWrapGitErrorNoTrackingInfo(t *testing.T) {
	g := &Git{}
	err := g.wrapGitError(&CommandExitError{
		Stderr:   "There is no tracking information for the current branch.",
		ExitCode: 1,
	})
	var upErr *GitUpstreamError
	if !errors.As(err, &upErr) {
		t.Errorf("expected *GitUpstreamError for 'no tracking information', got %T", err)
	}
}

// === V3 Git 选项测试 ===

func TestV3GitOptionFunctions(t *testing.T) {
	cfg := &gitConfig{}
	WithGitFetch(true)(cfg)
	WithGitOverwrite(true)(cfg)
	WithGitEnvVars(map[string]string{"GIT_SSH": "/usr/bin/ssh"})(cfg)
	WithGitCmdUser("root")(cfg)
	WithGitCmdCwd("/tmp")(cfg)
	WithGitCmdTimeout(30)(cfg)

	if !cfg.fetch {
		t.Error("fetch should be true")
	}
	if !cfg.overwrite {
		t.Error("overwrite should be true")
	}
	if cfg.envVars["GIT_SSH"] != "/usr/bin/ssh" {
		t.Errorf("envVars: got %v", cfg.envVars)
	}
	if cfg.cmdUser != "root" {
		t.Errorf("cmdUser: got %q", cfg.cmdUser)
	}
	if cfg.cmdCwd != "/tmp" {
		t.Errorf("cmdCwd: got %q", cfg.cmdCwd)
	}
	if cfg.cmdTimeout != 30 {
		t.Errorf("cmdTimeout: got %d", cfg.cmdTimeout)
	}
}

// === validateGitAuth 测试 ===

func TestValidateGitAuthNil(t *testing.T) {
	if err := validateGitAuth(nil); err != nil {
		t.Errorf("expected nil for nil auth, got %v", err)
	}
}

func TestValidateGitAuthValid(t *testing.T) {
	auth := &gitAuth{Username: "user", Token: "tok"}
	if err := validateGitAuth(auth); err != nil {
		t.Errorf("expected nil for valid auth, got %v", err)
	}
}

func TestValidateGitAuthTokenWithoutUsername(t *testing.T) {
	auth := &gitAuth{Username: "", Token: "tok"}
	err := validateGitAuth(auth)
	if err == nil {
		t.Error("expected error when token is set without username")
	}
	var invErr *InvalidArgumentError
	if !errors.As(err, &invErr) {
		t.Errorf("expected *InvalidArgumentError, got %T", err)
	}
}

func TestValidateGitAuthUsernameOnly(t *testing.T) {
	auth := &gitAuth{Username: "user", Token: ""}
	if err := validateGitAuth(auth); err != nil {
		t.Errorf("expected nil for username-only auth, got %v", err)
	}
}

// === parseGitStatus 补充测试 ===

func TestParseGitStatus_BehindOnly(t *testing.T) {
	output := "## main...origin/main [behind 4]\n"
	status := parseGitStatus(output)

	if status.Ahead != 0 {
		t.Errorf("Ahead: got %d, want 0", status.Ahead)
	}
	if status.Behind != 4 {
		t.Errorf("Behind: got %d, want 4", status.Behind)
	}
}

func TestParseGitStatus_CopiedFile(t *testing.T) {
	output := "## main\nC  src.txt -> dst.txt\n"
	status := parseGitStatus(output)

	if len(status.FileStatus) != 1 {
		t.Fatalf("expected 1 file, got %d", len(status.FileStatus))
	}
	f := status.FileStatus[0]
	if f.Status != "copied" {
		t.Errorf("status: got %q, want 'copied'", f.Status)
	}
	if f.Name != "dst.txt" {
		t.Errorf("Name: got %q, want 'dst.txt'", f.Name)
	}
	if f.RenamedFrom != "src.txt" {
		t.Errorf("RenamedFrom: got %q, want 'src.txt'", f.RenamedFrom)
	}
	if !f.Staged {
		t.Error("copied file should be staged")
	}
}

func TestParseGitStatus_IgnoredFile(t *testing.T) {
	output := "## main\n!! ignored.log\n"
	status := parseGitStatus(output)

	if len(status.FileStatus) != 1 {
		t.Fatalf("expected 1 file, got %d", len(status.FileStatus))
	}
	if status.FileStatus[0].Status != "ignored" {
		t.Errorf("status: got %q, want 'ignored'", status.FileStatus[0].Status)
	}
}

func TestParseGitStatus_DetachedWithParens(t *testing.T) {
	output := "## HEAD (detached at abc1234)\n"
	status := parseGitStatus(output)

	if !status.Detached {
		t.Error("expected Detached true")
	}
}

func TestParseGitStatus_MultipleFiles(t *testing.T) {
	output := "## main\nM  a.go\n M b.go\nA  c.go\n?? d.go\nDD e.go\n"
	status := parseGitStatus(output)

	if len(status.FileStatus) != 5 {
		t.Fatalf("expected 5 files, got %d", len(status.FileStatus))
	}
	if !status.HasStaged() {
		t.Error("expected HasStaged true")
	}
	if !status.HasUntracked() {
		t.Error("expected HasUntracked true")
	}
	if !status.HasConflicts() {
		t.Error("expected HasConflicts true")
	}
	if status.IsClean() {
		t.Error("expected not clean")
	}
}

// === wrapGitError 补充认证错误片段测试 ===

func TestWrapGitErrorInvalidUsername(t *testing.T) {
	g := &Git{}
	err := g.wrapGitError(&CommandExitError{
		Stderr:   "fatal: Invalid username or password",
		ExitCode: 128,
	})
	var authErr *GitAuthError
	if !errors.As(err, &authErr) {
		t.Errorf("expected *GitAuthError for 'invalid username or password', got %T", err)
	}
}

func TestWrapGitErrorAccessDenied(t *testing.T) {
	g := &Git{}
	err := g.wrapGitError(&CommandExitError{
		Stderr:   "fatal: Access denied",
		ExitCode: 128,
	})
	var authErr *GitAuthError
	if !errors.As(err, &authErr) {
		t.Errorf("expected *GitAuthError for 'access denied', got %T", err)
	}
}

func TestWrapGitErrorPermissionDenied(t *testing.T) {
	g := &Git{}
	err := g.wrapGitError(&CommandExitError{
		Stderr:   "fatal: Permission denied (publickey)",
		ExitCode: 128,
	})
	var authErr *GitAuthError
	if !errors.As(err, &authErr) {
		t.Errorf("expected *GitAuthError for 'permission denied', got %T", err)
	}
}

func TestWrapGitErrorNotAuthorized(t *testing.T) {
	g := &Git{}
	err := g.wrapGitError(&CommandExitError{
		Stderr:   "remote: Not authorized",
		ExitCode: 128,
	})
	var authErr *GitAuthError
	if !errors.As(err, &authErr) {
		t.Errorf("expected *GitAuthError for 'not authorized', got %T", err)
	}
}

// === wrapGitError 补充上游错误片段测试 ===

func TestWrapGitErrorSetRemoteAsUpstream(t *testing.T) {
	g := &Git{}
	err := g.wrapGitError(&CommandExitError{
		Stderr:   "fatal: set the remote as upstream",
		ExitCode: 128,
	})
	var upErr *GitUpstreamError
	if !errors.As(err, &upErr) {
		t.Errorf("expected *GitUpstreamError for 'set the remote as upstream', got %T", err)
	}
}

func TestWrapGitErrorPleaseSpecifyBranch(t *testing.T) {
	g := &Git{}
	err := g.wrapGitError(&CommandExitError{
		Stderr:   "fatal: please specify which branch you want to merge with",
		ExitCode: 1,
	})
	var upErr *GitUpstreamError
	if !errors.As(err, &upErr) {
		t.Errorf("expected *GitUpstreamError for 'please specify which branch', got %T", err)
	}
}

// === injectAuth 边界情况 ===

func TestInjectAuthInvalidURL(t *testing.T) {
	g := &Git{}
	auth := &gitAuth{Username: "user", Token: "tok"}
	// url.Parse 很少失败，但无协议前缀的 URL 可能会有问题
	url := g.injectAuth("://invalid", auth)
	// 不应 panic，且应返回非空结果
	if url == "" {
		t.Error("expected non-empty result even for unusual URL")
	}
}

// === shellQuote 边界情况 ===

func TestShellQuoteSpecialChars(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello\nworld", "'hello\nworld'"},
		{"tab\there", "'tab\there'"},
		{"dollar$var", "'dollar$var'"},
		{"back`tick", "'back`tick'"},
		{"double\"quote", "'double\"quote'"},
	}
	for _, tt := range tests {
		got := shellQuote(tt.input)
		if got != tt.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
