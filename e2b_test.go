package e2b

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"
)

// === Connection Tests ===

func TestGetHostDebug(t *testing.T) {
	cfg := &ConnectionConfig{Debug: true}
	got := cfg.GetHost("abc", "e2b.app", 8080)
	want := "localhost:8080"
	if got != want {
		t.Errorf("GetHost(Debug=true) = %q, want %q", got, want)
	}
}

func TestGetHostProduction(t *testing.T) {
	cfg := &ConnectionConfig{}
	got := cfg.GetHost("sandbox-123", "e2b.app", 8080)
	want := "8080-sandbox-123.e2b.app"
	if got != want {
		t.Errorf("GetHost = %q, want %q", got, want)
	}
}

func TestGetSandboxURLDefault(t *testing.T) {
	cfg := &ConnectionConfig{}
	got := cfg.GetSandboxURL("sandbox-123", "e2b.app")
	want := "https://49983-sandbox-123.e2b.app"
	if got != want {
		t.Errorf("GetSandboxURL = %q, want %q", got, want)
	}
}

func TestGetSandboxURLDebug(t *testing.T) {
	cfg := &ConnectionConfig{Debug: true}
	got := cfg.GetSandboxURL("sandbox-123", "e2b.app")
	want := "http://localhost:49983"
	if got != want {
		t.Errorf("GetSandboxURL(Debug) = %q, want %q", got, want)
	}
}

func TestGetSandboxURLOverride(t *testing.T) {
	cfg := &ConnectionConfig{SandboxURL: "http://custom:1234"}
	got := cfg.GetSandboxURL("abc", "e2b.app")
	want := "http://custom:1234"
	if got != want {
		t.Errorf("GetSandboxURL(override) = %q, want %q", got, want)
	}
}

func TestVersionLessThan(t *testing.T) {
	tests := []struct {
		a, b [3]int
		want bool
	}{
		{[3]int{0, 1, 0}, [3]int{0, 1, 2}, true},
		{[3]int{0, 1, 2}, [3]int{0, 1, 2}, false},
		{[3]int{0, 1, 3}, [3]int{0, 1, 2}, false},
		{[3]int{0, 0, 9}, [3]int{0, 1, 0}, true},
		{[3]int{1, 0, 0}, [3]int{0, 9, 9}, false},
	}
	for _, tt := range tests {
		got := versionLessThan(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("versionLessThan(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestParseEnvdVersion(t *testing.T) {
	tests := []struct {
		input string
		want  [3]int
	}{
		{"0.1.2", [3]int{0, 1, 2}},
		{"1.0.0", [3]int{1, 0, 0}},
		{"0.1.5", [3]int{0, 1, 5}},
		{"", [3]int{0, 0, 0}},
		{"abc", [3]int{0, 0, 0}},
	}
	for _, tt := range tests {
		got := parseEnvdVersion(tt.input)
		if got != tt.want {
			t.Errorf("parseEnvdVersion(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

// === Error Tests ===

func TestMapHTTPErrorNotFound(t *testing.T) {
	err := mapHTTPError(404, "sandbox not found")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("mapHTTPError(404) should match ErrNotFound, got %T", err)
	}
}

func TestMapHTTPErrorAuth(t *testing.T) {
	err := mapHTTPError(401, "invalid key")
	if !errors.Is(err, ErrAuth) {
		t.Errorf("mapHTTPError(401) should match ErrAuth, got %T", err)
	}
}

func TestMapHTTPErrorRateLimit(t *testing.T) {
	err := mapHTTPError(429, "rate limited")
	if !errors.Is(err, ErrRateLimit) {
		t.Errorf("mapHTTPError(429) should match ErrRateLimit, got %T", err)
	}
}

func TestMapHTTPErrorTimeout(t *testing.T) {
	err := mapHTTPError(502, "")
	if !errors.Is(err, ErrTimeout) {
		t.Errorf("mapHTTPError(502) should match ErrTimeout, got %T", err)
	}
}

func TestErrorsAs(t *testing.T) {
	err := mapHTTPError(507, "no space")
	var target *NotEnoughSpaceError
	if !errors.As(err, &target) {
		t.Errorf("mapHTTPError(507) should be extractable as *NotEnoughSpaceError")
	}
	if target.Message != "no space" {
		t.Errorf("expected message 'no space', got %q", target.Message)
	}
}

func TestCommandExitError(t *testing.T) {
	err := &CommandExitError{
		Stdout:   "out",
		Stderr:   "err",
		ExitCode: 1,
		Message:  "failed",
	}
	if err.Error() != "command exited with code 1: failed" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestSandboxErrorUnwrap(t *testing.T) {
	cause := errors.New("root cause")
	err := &SandboxError{Message: "wrapper", Cause: cause}
	if !errors.Is(err, cause) {
		t.Error("SandboxError.Unwrap should expose cause")
	}
}

// === Signature Tests ===

func TestGetSignatureBasic(t *testing.T) {
	sig, exp, err := getSignature("/test.txt", "read", "user", "token123", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exp != nil {
		t.Error("expected nil expiration")
	}
	if sig == "" || sig[:3] != "v1_" {
		t.Errorf("signature should start with 'v1_', got %q", sig)
	}
}

func TestGetSignatureWithExpiration(t *testing.T) {
	expSec := 3600
	sig, exp, err := getSignature("/test.txt", "read", "user", "token123", &expSec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exp == nil {
		t.Fatal("expected non-nil expiration")
	}
	if *exp <= 0 {
		t.Error("expiration should be positive")
	}
	if sig[:3] != "v1_" {
		t.Errorf("signature should start with 'v1_', got %q", sig)
	}
}

func TestGetSignatureNoAccessToken(t *testing.T) {
	_, _, err := getSignature("/test.txt", "read", "user", "", nil)
	if err == nil {
		t.Error("expected error when access token is empty")
	}
}

func TestGetSignatureDeterministic(t *testing.T) {
	sig1, _, _ := getSignature("/a.txt", "read", "u", "tok", nil)
	sig2, _, _ := getSignature("/a.txt", "read", "u", "tok", nil)
	if sig1 != sig2 {
		t.Error("signature should be deterministic for same inputs")
	}
}

func TestGetSignatureDifferentInputs(t *testing.T) {
	sig1, _, _ := getSignature("/a.txt", "read", "u", "tok", nil)
	sig2, _, _ := getSignature("/b.txt", "read", "u", "tok", nil)
	if sig1 == sig2 {
		t.Error("different paths should produce different signatures")
	}
}

// === Models Tests ===

func TestPaginatorEmpty(t *testing.T) {
	p := newPaginator[string](10, func(ctx context.Context, token string, limit int) ([]string, string, error) {
		return nil, "", nil
	})
	if !p.HasNext() {
		t.Error("new paginator should have next")
	}
	items, err := p.NextItems(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
	if p.HasNext() {
		t.Error("should not have next after empty result")
	}
}

func TestPaginatorMultiplePages(t *testing.T) {
	callCount := 0
	p := newPaginator[int](2, func(ctx context.Context, token string, limit int) ([]int, string, error) {
		callCount++
		switch callCount {
		case 1:
			return []int{1, 2}, "page2", nil
		case 2:
			return []int{3}, "", nil
		default:
			return nil, "", nil
		}
	})

	all, err := p.All(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 items, got %d", len(all))
	}
	if callCount != 2 {
		t.Errorf("expected 2 fetch calls, got %d", callCount)
	}
}

func TestResultFormats(t *testing.T) {
	text := "hello"
	png := "base64data"
	r := &Result{
		Text: &text,
		PNG:  &png,
	}
	formats := r.Formats()
	if len(formats) != 2 {
		t.Errorf("expected 2 formats, got %d: %v", len(formats), formats)
	}
}

func TestExecutionText(t *testing.T) {
	mainText := "42"
	exec := &Execution{
		Results: []Result{
			{IsMainResult: false, Text: nil},
			{IsMainResult: true, Text: &mainText},
		},
	}
	if exec.Text() != "42" {
		t.Errorf("expected '42', got %q", exec.Text())
	}
}

func TestExecutionTextEmpty(t *testing.T) {
	exec := &Execution{}
	if exec.Text() != "" {
		t.Errorf("expected empty string, got %q", exec.Text())
	}
}

// === Options Tests ===

func TestClientOptionDefaults(t *testing.T) {
	cfg := &clientConfig{
		domain:         DefaultDomain,
		apiURL:         DefaultAPIURL,
		requestTimeout: DefaultRequestTimeout,
	}
	WithAPIKey("sk-test")(cfg)
	WithDebug(true)(cfg)
	if cfg.apiKey != "sk-test" {
		t.Errorf("expected apiKey 'sk-test', got %q", cfg.apiKey)
	}
	if !cfg.debug {
		t.Error("expected debug=true")
	}
}

func TestSandboxOptionDefaults(t *testing.T) {
	cfg := &sandboxConfig{}
	applySandboxDefaults(cfg)
	if cfg.template != DefaultTemplate {
		t.Errorf("expected template %q, got %q", DefaultTemplate, cfg.template)
	}
	if cfg.timeout != DefaultSandboxTimeout {
		t.Errorf("expected timeout %d, got %d", DefaultSandboxTimeout, cfg.timeout)
	}
}

func TestSandboxOptionOverrides(t *testing.T) {
	cfg := &sandboxConfig{}
	WithTemplate("custom")(cfg)
	WithTimeout(600)(cfg)
	WithEnvVars(map[string]string{"K": "V"})(cfg)
	applySandboxDefaults(cfg)
	if cfg.template != "custom" {
		t.Errorf("expected template 'custom', got %q", cfg.template)
	}
	if cfg.timeout != 600 {
		t.Errorf("expected timeout 600, got %d", cfg.timeout)
	}
	if cfg.envVars["K"] != "V" {
		t.Error("envVars not set")
	}
}

// === NewClient Tests ===

func TestNewClientNoAPIKey(t *testing.T) {
	t.Setenv("E2B_API_KEY", "")
	_, err := NewClient()
	if err == nil {
		t.Error("expected error when no API key provided")
	}
}

func TestNewClientFromEnv(t *testing.T) {
	t.Setenv("E2B_API_KEY", "sk-env-test")
	client, err := NewClient()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.config.APIKey != "sk-env-test" {
		t.Errorf("expected API key from env, got %q", client.config.APIKey)
	}
}

func TestNewClientWithOption(t *testing.T) {
	client, err := NewClient(WithAPIKey("sk-option"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.config.APIKey != "sk-option" {
		t.Errorf("expected API key 'sk-option', got %q", client.config.APIKey)
	}
}

// === buildAuthHeader Tests ===

func TestBuildAuthHeaderEmpty(t *testing.T) {
	h := buildAuthHeader("")
	if h != nil {
		t.Errorf("expected nil for empty user, got %v", h)
	}
}

func TestBuildAuthHeaderUser(t *testing.T) {
	h := buildAuthHeader("user")
	if h == nil {
		t.Fatal("expected non-nil header")
	}
	// Python: base64("user:") = "dXNlcjo="
	want := "Basic dXNlcjo="
	if h["Authorization"] != want {
		t.Errorf("Authorization header: got %q, want %q", h["Authorization"], want)
	}
}

func TestBuildAuthHeaderRoot(t *testing.T) {
	h := buildAuthHeader("root")
	if h == nil {
		t.Fatal("expected non-nil header")
	}
	// base64("root:") = "cm9vdDo="
	want := "Basic cm9vdDo="
	if h["Authorization"] != want {
		t.Errorf("Authorization header: got %q, want %q", h["Authorization"], want)
	}
}

// === Sandbox resolveUsername Tests ===

func TestResolveUsernameExplicit(t *testing.T) {
	s := &Sandbox{envdVersion: [3]int{0, 1, 0}}
	got := s.resolveUsername("alice")
	if got != "alice" {
		t.Errorf("expected 'alice', got %q", got)
	}
}

func TestResolveUsernameOldEnvd(t *testing.T) {
	s := &Sandbox{envdVersion: [3]int{0, 1, 0}}
	got := s.resolveUsername("")
	if got != "user" {
		t.Errorf("expected 'user' for old envd, got %q", got)
	}
}

func TestResolveUsernameNewEnvd(t *testing.T) {
	// envdVersionDefaultUser = 0.4.0，0.4.0 及以上才不注入默认 user
	s := &Sandbox{envdVersion: [3]int{0, 4, 0}}
	got := s.resolveUsername("")
	if got != "" {
		t.Errorf("expected empty for new envd (>= 0.4.0), got %q", got)
	}
}

func TestResolveUsernameNewEnvdOld(t *testing.T) {
	// 0.1.5 < 0.4.0，仍应返回 "user"
	s := &Sandbox{envdVersion: [3]int{0, 1, 5}}
	got := s.resolveUsername("")
	if got != "user" {
		t.Errorf("expected 'user' for envd < 0.4.0, got %q", got)
	}
}

func TestSupportsStdin(t *testing.T) {
	// envdVersionStdin = 0.3.0
	s1 := &Sandbox{envdVersion: [3]int{0, 2, 9}}
	if s1.supportsStdin() {
		t.Error("0.2.9 should not support stdin (< 0.3.0)")
	}
	s2 := &Sandbox{envdVersion: [3]int{0, 3, 0}}
	if !s2.supportsStdin() {
		t.Error("0.3.0 should support stdin")
	}
	s3 := &Sandbox{envdVersion: [3]int{0, 1, 2}}
	if s3.supportsStdin() {
		t.Error("0.1.2 should NOT support stdin with new constant (>= 0.3.0 required)")
	}
}

func TestSupportsRecursiveWatch(t *testing.T) {
	s1 := &Sandbox{envdVersion: [3]int{0, 1, 3}}
	if s1.supportsRecursiveWatch() {
		t.Error("0.1.3 should not support recursive watch")
	}
	s2 := &Sandbox{envdVersion: [3]int{0, 1, 4}}
	if !s2.supportsRecursiveWatch() {
		t.Error("0.1.4 should support recursive watch")
	}
}

// === mapHTTPError 补充测试 ===

func TestMapHTTPErrorBadRequest(t *testing.T) {
	err := mapHTTPError(400, "bad request")
	var target *InvalidArgumentError
	if !errors.As(err, &target) {
		t.Errorf("mapHTTPError(400) should be *InvalidArgumentError, got %T", err)
	}
}

func TestMapHTTPErrorForbidden(t *testing.T) {
	err := mapHTTPError(403, "forbidden")
	var target *ForbiddenError
	if !errors.As(err, &target) {
		t.Errorf("mapHTTPError(403) should be *ForbiddenError, got %T", err)
	}
}

func TestMapHTTPErrorDefault(t *testing.T) {
	err := mapHTTPError(500, "internal error")
	var target *SandboxError
	if !errors.As(err, &target) {
		t.Errorf("mapHTTPError(500) should be *SandboxError, got %T", err)
	}
	if target.Message != "HTTP 500: internal error" {
		t.Errorf("unexpected message: %q", target.Message)
	}
}

// === getRequestTimeout 测试 ===

func TestGetRequestTimeoutDefault(t *testing.T) {
	cfg := &ConnectionConfig{}
	got := cfg.getRequestTimeout(nil)
	if got != DefaultRequestTimeout {
		t.Errorf("expected %v, got %v", DefaultRequestTimeout, got)
	}
}

func TestGetRequestTimeoutFromConfig(t *testing.T) {
	cfg := &ConnectionConfig{RequestTimeout: 30 * time.Second}
	got := cfg.getRequestTimeout(nil)
	if got != 30*time.Second {
		t.Errorf("expected 30s, got %v", got)
	}
}

func TestGetRequestTimeoutOverride(t *testing.T) {
	cfg := &ConnectionConfig{RequestTimeout: 30 * time.Second}
	override := 10 * time.Second
	got := cfg.getRequestTimeout(&override)
	if got != 10*time.Second {
		t.Errorf("expected 10s override, got %v", got)
	}
}

// === ConnectError 测试 ===

func TestConnectErrorFormat(t *testing.T) {
	err := &ConnectError{Code: "not_found", Message: "resource missing"}
	want := "connect rpc error [not_found]: resource missing"
	if err.Error() != want {
		t.Errorf("expected %q, got %q", want, err.Error())
	}
}

// === SandboxError.Error 测试 ===

func TestSandboxErrorMessage(t *testing.T) {
	err := &SandboxError{Message: "something broke"}
	if err.Error() != "something broke" {
		t.Errorf("unexpected: %q", err.Error())
	}
}

// === OutputMessage 测试 ===

func TestOutputMessageString(t *testing.T) {
	m := OutputMessage{Line: "hello world", Timestamp: 12345, Error: false}
	if m.String() != "hello world" {
		t.Errorf("expected 'hello world', got %q", m.String())
	}
}

func TestOutputMessageStringError(t *testing.T) {
	m := OutputMessage{Line: "error line", Error: true}
	if m.String() != "error line" {
		t.Errorf("expected 'error line', got %q", m.String())
	}
}

// === NewGitHubMCPConfig 测试 ===

func TestNewGitHubMCPConfigBasic(t *testing.T) {
	config := NewGitHubMCPConfig(map[string]GitHubMCPServerConfig{
		"github/owner/repo": {
			RunCmd:     "npx run",
			InstallCmd: "npm install",
			Envs:       map[string]string{"TOKEN": "abc"},
		},
	})
	if len(config) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(config))
	}
	entry, ok := config["github/owner/repo"]
	if !ok {
		t.Fatal("missing key 'github/owner/repo'")
	}
	typed, ok := entry.(GitHubMCPServerConfig)
	if !ok {
		t.Fatalf("expected GitHubMCPServerConfig, got %T", entry)
	}
	if typed.RunCmd != "npx run" {
		t.Errorf("RunCmd: got %q", typed.RunCmd)
	}
}

func TestNewGitHubMCPConfigEmptyMap(t *testing.T) {
	config := NewGitHubMCPConfig(map[string]GitHubMCPServerConfig{})
	if len(config) != 0 {
		t.Errorf("expected empty config, got %d entries", len(config))
	}
}

// === 命令选项测试 ===

func TestCommandOptions(t *testing.T) {
	cfg := &commandConfig{}
	WithCwd("/tmp")(cfg)
	WithUser("root")(cfg)
	WithCommandEnvVars(map[string]string{"K": "V"})(cfg)
	WithCommandTimeout(60)(cfg)
	WithStdin(true)(cfg)

	d := 5 * time.Second
	WithCommandRequestTimeout(d)(cfg)

	var stdoutCalled, stderrCalled bool
	WithOnStdout(func(s string) { stdoutCalled = true })(cfg)
	WithOnStderr(func(s string) { stderrCalled = true })(cfg)

	if cfg.cwd != "/tmp" {
		t.Errorf("cwd: got %q", cfg.cwd)
	}
	if cfg.user != "root" {
		t.Errorf("user: got %q", cfg.user)
	}
	if cfg.envVars["K"] != "V" {
		t.Error("envVars not set")
	}
	if cfg.timeout != 60 {
		t.Errorf("timeout: got %d", cfg.timeout)
	}
	if !cfg.stdin {
		t.Error("stdin should be true")
	}
	if cfg.requestTimeout == nil || *cfg.requestTimeout != d {
		t.Error("requestTimeout not set")
	}
	cfg.onStdout("test")
	cfg.onStderr("test")
	if !stdoutCalled {
		t.Error("onStdout not called")
	}
	if !stderrCalled {
		t.Error("onStderr not called")
	}
}

// === 沙箱选项补充测试 ===

func TestSandboxOptionsExtended(t *testing.T) {
	cfg := &sandboxConfig{}
	WithMetadata(map[string]string{"k": "v"})(cfg)
	WithAutoPause(true)(cfg)
	WithSecure(true)(cfg)
	WithAllowInternetAccess(false)(cfg)
	WithNetwork(NetworkOpts{AllowOut: []string{"10.0.0.0/8"}})(cfg)
	WithAutoResume(AutoResumePolicyOn)(cfg)
	WithVolumeMounts([]VolumeMount{{Name: "vol1", Path: "/data"}})(cfg)
	WithMCP(MCPConfig{"key": "value"})(cfg)

	if cfg.metadata["k"] != "v" {
		t.Error("metadata not set")
	}
	if cfg.autoPause == nil || !*cfg.autoPause {
		t.Error("autoPause should be *true")
	}
	if cfg.secure == nil || !*cfg.secure {
		t.Error("secure should be *true")
	}
	if cfg.allowInternetAccess == nil || *cfg.allowInternetAccess {
		t.Error("allowInternetAccess should be *false")
	}
	if cfg.network == nil || len(cfg.network.AllowOut) != 1 {
		t.Error("network not set correctly")
	}
	if cfg.autoResume == nil || *cfg.autoResume != AutoResumePolicyOn {
		t.Error("autoResume not set")
	}
	if len(cfg.volumeMounts) != 1 || cfg.volumeMounts[0].Name != "vol1" {
		t.Error("volumeMounts not set")
	}
	if cfg.mcp["key"] != "value" {
		t.Error("mcp not set")
	}
}

// === 客户端选项测试 ===

func TestClientOptionsExtended(t *testing.T) {
	cfg := &clientConfig{}
	WithDomain("custom.app")(cfg)
	WithAPIURL("https://api.custom.app")(cfg)
	WithHTTPClient(&http.Client{})(cfg)
	WithRequestTimeout(30 * time.Second)(cfg)
	WithAccessToken("at-123")(cfg)
	WithSandboxURL("http://sbx:1234")(cfg)

	if cfg.domain != "custom.app" {
		t.Errorf("domain: got %q", cfg.domain)
	}
	if cfg.apiURL != "https://api.custom.app" {
		t.Errorf("apiURL: got %q", cfg.apiURL)
	}
	if cfg.httpClient == nil {
		t.Error("httpClient not set")
	}
	if cfg.requestTimeout != 30*time.Second {
		t.Errorf("requestTimeout: got %v", cfg.requestTimeout)
	}
	if cfg.accessToken != "at-123" {
		t.Errorf("accessToken: got %q", cfg.accessToken)
	}
	if cfg.sandboxURL != "http://sbx:1234" {
		t.Errorf("sandboxURL: got %q", cfg.sandboxURL)
	}
}

// === 代码执行选项测试 ===

func TestRunCodeOptions(t *testing.T) {
	cfg := &runCodeConfig{}
	WithLanguage("python")(cfg)
	WithCodeContext(&CodeContext{ID: "ctx1"})(cfg)
	WithCodeEnvVars(map[string]string{"A": "B"})(cfg)
	WithCodeTimeout(30.0)(cfg)

	var resultCalled, errorCalled bool
	WithOnResult(func(r Result) { resultCalled = true })(cfg)
	WithOnError(func(e ExecutionError) { errorCalled = true })(cfg)
	WithOnCodeStdout(func(m OutputMessage) {})(cfg)
	WithOnCodeStderr(func(m OutputMessage) {})(cfg)

	if cfg.language != "python" {
		t.Errorf("language: got %q", cfg.language)
	}
	if cfg.codeContext == nil || cfg.codeContext.ID != "ctx1" {
		t.Error("codeContext not set")
	}
	if cfg.envVars["A"] != "B" {
		t.Error("envVars not set")
	}
	if cfg.timeout != 30.0 {
		t.Errorf("timeout: got %f", cfg.timeout)
	}
	if cfg.onStdout == nil || cfg.onStderr == nil {
		t.Error("callbacks not set")
	}
	cfg.onResult(Result{})
	cfg.onError(ExecutionError{})
	if !resultCalled {
		t.Error("onResult not called")
	}
	if !errorCalled {
		t.Error("onError not called")
	}
}

// === 文件系统选项测试 ===

func TestFilesystemOptions(t *testing.T) {
	cfg := &filesystemConfig{}
	WithFsUser("admin")(cfg)
	d := 10 * time.Second
	WithFsRequestTimeout(d)(cfg)
	WithDepth(3)(cfg)
	WithRecursive(true)(cfg)

	if cfg.user != "admin" {
		t.Errorf("user: got %q", cfg.user)
	}
	if cfg.requestTimeout == nil || *cfg.requestTimeout != d {
		t.Error("requestTimeout not set")
	}
	if cfg.depth != 3 {
		t.Errorf("depth: got %d", cfg.depth)
	}
	if !cfg.recursive {
		t.Error("recursive should be true")
	}
}

// === 文件 URL 选项测试 ===

func TestFileURLOptions(t *testing.T) {
	cfg := &fileURLConfig{}
	WithFileUser("user1")(cfg)
	WithSignatureExpiration(3600)(cfg)

	if cfg.user != "user1" {
		t.Errorf("user: got %q", cfg.user)
	}
	if cfg.expiration != 3600 {
		t.Errorf("expiration: got %d", cfg.expiration)
	}
}

// === 错误 Is() 方法测试 ===

func TestErrorIsMethodsCoverage(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"InvalidArgument", &InvalidArgumentError{SandboxError{Message: "test"}}},
		{"ForbiddenError", &ForbiddenError{SandboxError{Message: "test"}}},
		{"BuildError", &BuildError{SandboxError: SandboxError{Message: "test"}}},
		{"FileUploadError", &FileUploadError{SandboxError{Message: "test"}}},
		{"TemplateError", &TemplateError{SandboxError{Message: "test"}}},
		{"GitAuthError", &GitAuthError{SandboxError{Message: "test"}}},
		{"GitUpstreamError", &GitUpstreamError{SandboxError{Message: "test"}}},
	}
	for _, tt := range tests {
		if !errors.Is(tt.err, tt.err) {
			t.Errorf("%s: errors.Is should match itself", tt.name)
		}
	}
}

// === processDataEvent.UnmarshalJSON 测试 ===

func TestProcessDataEventUnmarshalJSON(t *testing.T) {
	// "hello" 的 Base64 编码为 "aGVsbG8="，"error" 的为 "ZXJyb3I="
	input := `{"stdout":"aGVsbG8=","stderr":"ZXJyb3I="}`
	var event processDataEvent
	if err := json.Unmarshal([]byte(input), &event); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if string(event.Stdout) != "hello" {
		t.Errorf("Stdout: got %q, want 'hello'", string(event.Stdout))
	}
	if string(event.Stderr) != "error" {
		t.Errorf("Stderr: got %q, want 'error'", string(event.Stderr))
	}
}

func TestProcessDataEventUnmarshalJSONEmpty(t *testing.T) {
	input := `{}`
	var event processDataEvent
	if err := json.Unmarshal([]byte(input), &event); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if event.Stdout != nil {
		t.Error("Stdout should be nil for empty input")
	}
	if event.Stderr != nil {
		t.Error("Stderr should be nil for empty input")
	}
}

// === sendInputRequest / proto JSON 结构测试 ===

func TestSendInputRequestStdinJSON(t *testing.T) {
	// 与 Python SDK 对齐：应生成嵌套 process 字段和 input.stdin 字段
	req := newSendStdinRequest(42, []byte("hello"))
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var decoded map[string]interface{}
	json.Unmarshal(data, &decoded)

	// 验证嵌套 process 字段
	process, ok := decoded["process"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected nested process field, got: %v", decoded)
	}
	if int(process["pid"].(float64)) != 42 {
		t.Errorf("process.pid: got %v, want 42", process["pid"])
	}
	// 验证 input.stdin 字段（base64 编码）
	input, ok := decoded["input"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected nested input field, got: %v", decoded)
	}
	// "hello" 的 base64 编码 = "aGVsbG8="
	if input["stdin"] != "aGVsbG8=" {
		t.Errorf("input.stdin: got %q, want 'aGVsbG8='", input["stdin"])
	}
}

func TestSendPtyRequestJSON(t *testing.T) {
	// PTY 输入应使用 input.pty 字段
	req := newSendPtyRequest(99, []byte("data"))
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var decoded map[string]interface{}
	json.Unmarshal(data, &decoded)

	process, ok := decoded["process"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected nested process field, got: %v", decoded)
	}
	if int(process["pid"].(float64)) != 99 {
		t.Errorf("process.pid: got %v, want 99", process["pid"])
	}
	input, ok := decoded["input"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected nested input field, got: %v", decoded)
	}
	// "data" base64 = "ZGF0YQ=="
	if input["pty"] != "ZGF0YQ==" {
		t.Errorf("input.pty: got %q, want 'ZGF0YQ=='", input["pty"])
	}
	if input["stdin"] != nil {
		t.Error("input.stdin should be absent for PTY request")
	}
}

func TestSendSignalRequestJSON(t *testing.T) {
	// 验证嵌套 process 字段
	req := sendSignalRequest{Process: processSelector{PID: 10}, Signal: "SIGNAL_SIGKILL"}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var decoded map[string]interface{}
	json.Unmarshal(data, &decoded)
	process, ok := decoded["process"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected nested process field")
	}
	if int(process["pid"].(float64)) != 10 {
		t.Errorf("process.pid: got %v, want 10", process["pid"])
	}
	if decoded["signal"] != "SIGNAL_SIGKILL" {
		t.Errorf("signal: got %v", decoded["signal"])
	}
}

// === Result.Formats 扩展测试 ===

func TestResultFormatsAll(t *testing.T) {
	s := "x"
	r := &Result{
		Text:       &s,
		HTML:       &s,
		Markdown:   &s,
		SVG:        &s,
		PNG:        &s,
		JPEG:       &s,
		PDF:        &s,
		LaTeX:      &s,
		JSON:       map[string]interface{}{"a": 1},
		JavaScript: &s,
		Chart:      &Chart{Type: ChartTypeLine},
	}
	formats := r.Formats()
	if len(formats) != 11 {
		t.Errorf("expected 11 formats, got %d: %v", len(formats), formats)
	}
}

func TestResultFormatsEmpty(t *testing.T) {
	r := &Result{}
	formats := r.Formats()
	if len(formats) != 0 {
		t.Errorf("expected 0 formats, got %d", len(formats))
	}
}

// === Execution.Text 边界情况 ===

func TestExecutionTextNoMainResult(t *testing.T) {
	text := "42"
	exec := &Execution{
		Results: []Result{
			{IsMainResult: false, Text: &text},
		},
	}
	if exec.Text() != "" {
		t.Errorf("expected empty for non-main result, got %q", exec.Text())
	}
}

func TestExecutionTextNilText(t *testing.T) {
	exec := &Execution{
		Results: []Result{
			{IsMainResult: true, Text: nil},
		},
	}
	if exec.Text() != "" {
		t.Errorf("expected empty for nil text, got %q", exec.Text())
	}
}
