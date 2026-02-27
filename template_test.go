package e2b

import (
	"encoding/json"
	"strings"
	"testing"
)

// === ReadyCmd tests ===

func TestWaitForPort(t *testing.T) {
	rc := WaitForPort(8080)
	if !strings.Contains(rc.String(), ":8080") {
		t.Errorf("expected port 8080 in cmd, got %q", rc.String())
	}
}

func TestWaitForURL(t *testing.T) {
	rc := WaitForURL("http://localhost:3000", 200)
	s := rc.String()
	if !strings.Contains(s, "localhost:3000") {
		t.Errorf("expected URL in cmd, got %q", s)
	}
	if !strings.Contains(s, "200") {
		t.Errorf("expected status code 200 in cmd, got %q", s)
	}
}

func TestWaitForProcess(t *testing.T) {
	rc := WaitForProcess("node")
	if !strings.Contains(rc.String(), "node") {
		t.Errorf("expected process name in cmd, got %q", rc.String())
	}
}

func TestWaitForFile(t *testing.T) {
	rc := WaitForFile("/tmp/ready")
	if !strings.Contains(rc.String(), "/tmp/ready") {
		t.Errorf("expected file path in cmd, got %q", rc.String())
	}
}

func TestWaitForTimeout(t *testing.T) {
	rc := WaitForTimeout(5000)
	if !strings.Contains(rc.String(), "sleep 5.000") {
		t.Errorf("expected sleep 5.000 in cmd, got %q", rc.String())
	}
}

func TestWaitForTimeout_MinOneSecond(t *testing.T) {
	rc := WaitForTimeout(100)
	if !strings.Contains(rc.String(), "sleep 1.000") {
		t.Errorf("expected minimum sleep 1.000, got %q", rc.String())
	}
}

// === TemplateBuilder tests ===

func TestNewTemplate_DefaultImage(t *testing.T) {
	tb := NewTemplate()
	data := tb.serialize()
	if data.FromImage != "e2bdev/base:latest" {
		t.Errorf("default image: got %q, want 'e2bdev/base:latest'", data.FromImage)
	}
}

func TestTemplateBuilder_FromImage(t *testing.T) {
	tb := NewTemplate().FromImage("python:3.12")
	data := tb.serialize()
	if data.FromImage != "python:3.12" {
		t.Errorf("FromImage: got %q", data.FromImage)
	}
	if data.FromTemplate != "" {
		t.Errorf("FromTemplate should be empty, got %q", data.FromTemplate)
	}
}

func TestTemplateBuilder_FromTemplate(t *testing.T) {
	tb := NewTemplate().FromTemplate("my-template")
	data := tb.serialize()
	if data.FromTemplate != "my-template" {
		t.Errorf("FromTemplate: got %q", data.FromTemplate)
	}
	if data.FromImage != "" {
		t.Errorf("FromImage should be empty, got %q", data.FromImage)
	}
}

func TestTemplateBuilder_FromImageWithRegistry(t *testing.T) {
	tb := NewTemplate().FromImage("registry.example.com/myimg:latest",
		WithRegistryAuth("user", "pass"))
	data := tb.serialize()
	if data.FromImageRegistry == nil {
		t.Fatal("expected registry config")
	}
	if data.FromImageRegistry.Type != "registry" {
		t.Errorf("registry type: got %q", data.FromImageRegistry.Type)
	}
	if data.FromImageRegistry.Username != "user" {
		t.Errorf("username: got %q", data.FromImageRegistry.Username)
	}
}

func TestTemplateBuilder_FromPythonImage(t *testing.T) {
	tb := NewTemplate().FromPythonImage("3.12-slim")
	data := tb.serialize()
	if data.FromImage != "python:3.12-slim" {
		t.Errorf("got %q", data.FromImage)
	}
}

func TestTemplateBuilder_FromNodeImage(t *testing.T) {
	tb := NewTemplate().FromNodeImage("20-slim")
	data := tb.serialize()
	if data.FromImage != "node:20-slim" {
		t.Errorf("got %q", data.FromImage)
	}
}

func TestTemplateBuilder_FromBunImage(t *testing.T) {
	tb := NewTemplate().FromBunImage("1.0")
	data := tb.serialize()
	if data.FromImage != "oven/bun:1.0" {
		t.Errorf("got %q", data.FromImage)
	}
}

func TestTemplateBuilder_FromDebianImage(t *testing.T) {
	tb := NewTemplate().FromDebianImage("bookworm-slim")
	data := tb.serialize()
	if data.FromImage != "debian:bookworm-slim" {
		t.Errorf("got %q", data.FromImage)
	}
}

func TestTemplateBuilder_FromUbuntuImage(t *testing.T) {
	tb := NewTemplate().FromUbuntuImage("22.04")
	data := tb.serialize()
	if data.FromImage != "ubuntu:22.04" {
		t.Errorf("got %q", data.FromImage)
	}
}

func TestTemplateBuilder_FromBaseImage(t *testing.T) {
	tb := NewTemplate().FromPythonImage("3.12").FromBaseImage()
	data := tb.serialize()
	if data.FromImage != "e2bdev/base:latest" {
		t.Errorf("got %q", data.FromImage)
	}
}

func TestTemplateBuilder_FromAWSRegistry(t *testing.T) {
	tb := NewTemplate().FromAWSRegistry("123456.dkr.ecr.us-east-1.amazonaws.com/myimg", "AKID", "SECRET", "us-east-1")
	data := tb.serialize()
	if data.FromImageRegistry == nil {
		t.Fatal("expected registry config")
	}
	if data.FromImageRegistry.Type != "aws" {
		t.Errorf("type: got %q", data.FromImageRegistry.Type)
	}
	if data.FromImageRegistry.AWSAccessKeyID != "AKID" {
		t.Errorf("access key: got %q", data.FromImageRegistry.AWSAccessKeyID)
	}
}

func TestTemplateBuilder_FromGCPRegistry(t *testing.T) {
	tb := NewTemplate().FromGCPRegistry("us-docker.pkg.dev/project/repo/img", `{"type":"service_account"}`)
	data := tb.serialize()
	if data.FromImageRegistry == nil {
		t.Fatal("expected registry config")
	}
	if data.FromImageRegistry.Type != "gcp" {
		t.Errorf("type: got %q", data.FromImageRegistry.Type)
	}
}

// === Build Instructions ===

func TestTemplateBuilder_RunCmd(t *testing.T) {
	tb := NewTemplate().RunCmd("echo hello")
	data := tb.serialize()
	if len(data.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(data.Steps))
	}
	if data.Steps[0].Type != InstructionRun {
		t.Errorf("type: got %q", data.Steps[0].Type)
	}
	if data.Steps[0].Args[0] != "echo hello" {
		t.Errorf("args: got %v", data.Steps[0].Args)
	}
}

func TestTemplateBuilder_Copy(t *testing.T) {
	tb := NewTemplate().Copy("app.py", "/app/")
	data := tb.serialize()
	if len(data.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(data.Steps))
	}
	s := data.Steps[0]
	if s.Type != InstructionCopy {
		t.Errorf("type: got %q", s.Type)
	}
	if s.Args[0] != "app.py" || s.Args[1] != "/app/" {
		t.Errorf("args: got %v", s.Args)
	}
}

func TestTemplateBuilder_CopyWithOptions(t *testing.T) {
	tb := NewTemplate().CopyWithOptions("app.py", "/app/", "root", 0o755)
	data := tb.serialize()
	s := data.Steps[0]
	if s.Args[2] != "root" {
		t.Errorf("user: got %q", s.Args[2])
	}
	if s.Args[3] != "0755" {
		t.Errorf("mode: got %q", s.Args[3])
	}
}

func TestTemplateBuilder_SetEnvs(t *testing.T) {
	tb := NewTemplate().SetEnvs(map[string]string{"KEY": "val"})
	data := tb.serialize()
	s := data.Steps[0]
	if s.Type != InstructionEnv {
		t.Errorf("type: got %q", s.Type)
	}
	if len(s.Args) != 2 {
		t.Errorf("args length: got %d", len(s.Args))
	}
}

func TestTemplateBuilder_SetWorkdir(t *testing.T) {
	tb := NewTemplate().SetWorkdir("/app")
	data := tb.serialize()
	if data.Steps[0].Type != InstructionWorkdir {
		t.Errorf("type: got %q", data.Steps[0].Type)
	}
}

func TestTemplateBuilder_SetUser(t *testing.T) {
	tb := NewTemplate().SetUser("nobody")
	data := tb.serialize()
	if data.Steps[0].Type != InstructionUser {
		t.Errorf("type: got %q", data.Steps[0].Type)
	}
}

func TestTemplateBuilder_PipInstall(t *testing.T) {
	tb := NewTemplate().PipInstall([]string{"flask", "requests"})
	data := tb.serialize()
	if !strings.Contains(data.Steps[0].Args[0], "pip install flask requests") {
		t.Errorf("got %q", data.Steps[0].Args[0])
	}
}

func TestTemplateBuilder_PipInstallNoArgs(t *testing.T) {
	tb := NewTemplate().PipInstall(nil)
	data := tb.serialize()
	if !strings.Contains(data.Steps[0].Args[0], "pip install .") {
		t.Errorf("got %q", data.Steps[0].Args[0])
	}
}

func TestTemplateBuilder_NpmInstall(t *testing.T) {
	tb := NewTemplate().NpmInstall([]string{"express"})
	data := tb.serialize()
	if !strings.Contains(data.Steps[0].Args[0], "npm install express") {
		t.Errorf("got %q", data.Steps[0].Args[0])
	}
}

func TestTemplateBuilder_BunInstall(t *testing.T) {
	tb := NewTemplate().BunInstall([]string{"hono"})
	data := tb.serialize()
	if !strings.Contains(data.Steps[0].Args[0], "bun install hono") {
		t.Errorf("got %q", data.Steps[0].Args[0])
	}
}

func TestTemplateBuilder_AptInstall(t *testing.T) {
	tb := NewTemplate().AptInstall([]string{"curl", "git"})
	data := tb.serialize()
	cmd := data.Steps[0].Args[0]
	if !strings.Contains(cmd, "apt-get install -y curl git") {
		t.Errorf("got %q", cmd)
	}
	if !strings.Contains(cmd, "DEBIAN_FRONTEND=noninteractive") {
		t.Errorf("missing DEBIAN_FRONTEND, got %q", cmd)
	}
}

func TestTemplateBuilder_SkipCache(t *testing.T) {
	tb := NewTemplate().SkipCache().RunCmd("echo 1")
	data := tb.serialize()
	if !data.Steps[0].Force {
		t.Error("step after SkipCache should have Force=true")
	}
}

func TestTemplateBuilder_SkipCacheResetsAfterOneInstruction(t *testing.T) {
	tb := NewTemplate().SkipCache().RunCmd("echo 1").RunCmd("echo 2")
	data := tb.serialize()
	if !data.Steps[0].Force {
		t.Error("first step after SkipCache should have Force=true")
	}
	if data.Steps[1].Force {
		t.Error("second step should NOT have Force=true (SkipCache should reset)")
	}
}

func TestTemplateBuilder_SetEnvsDeterministicOrder(t *testing.T) {
	envs := map[string]string{"Z_KEY": "z", "A_KEY": "a", "M_KEY": "m"}
	tb := NewTemplate().SetEnvs(envs)
	data := tb.serialize()
	args := data.Steps[0].Args
	// Keys should be sorted: A_KEY, M_KEY, Z_KEY
	if len(args) != 6 {
		t.Fatalf("expected 6 args, got %d", len(args))
	}
	if args[0] != "A_KEY" || args[2] != "M_KEY" || args[4] != "Z_KEY" {
		t.Errorf("env keys not sorted: got %v", args)
	}
}

func TestTemplateBuilder_SetReadyCmdRaw(t *testing.T) {
	tb := NewTemplate().SetReadyCmdRaw("sleep 3")
	data := tb.serialize()
	if data.ReadyCmd != "sleep 3" {
		t.Errorf("ReadyCmd: got %q", data.ReadyCmd)
	}
}

func TestTemplateBuilder_SetStartCmd(t *testing.T) {
	tb := NewTemplate().SetStartCmd("node server.js", WaitForPort(3000))
	data := tb.serialize()
	if data.StartCmd != "node server.js" {
		t.Errorf("StartCmd: got %q", data.StartCmd)
	}
	if data.ReadyCmd == "" {
		t.Error("ReadyCmd should not be empty")
	}
}

func TestTemplateBuilder_SetStartCmdRaw(t *testing.T) {
	tb := NewTemplate().SetStartCmdRaw("python app.py", "sleep 2")
	data := tb.serialize()
	if data.ReadyCmd != "sleep 2" {
		t.Errorf("ReadyCmd: got %q", data.ReadyCmd)
	}
}

func TestTemplateBuilder_SetReadyCmd(t *testing.T) {
	tb := NewTemplate().SetReadyCmd(WaitForFile("/tmp/ready"))
	data := tb.serialize()
	if !strings.Contains(data.ReadyCmd, "/tmp/ready") {
		t.Errorf("ReadyCmd: got %q", data.ReadyCmd)
	}
}

// === Chaining ===

func TestTemplateBuilder_Chaining(t *testing.T) {
	tb := NewTemplate().
		FromPythonImage("3.12").
		SetWorkdir("/app").
		Copy("requirements.txt", "/app/").
		RunCmd("pip install -r requirements.txt").
		Copy(".", "/app/").
		SetEnvs(map[string]string{"PORT": "8080"}).
		SetStartCmd("python app.py", WaitForPort(8080))

	data := tb.serialize()
	if data.FromImage != "python:3.12" {
		t.Errorf("FromImage: got %q", data.FromImage)
	}
	// WORKDIR + COPY + RUN + COPY + ENV = 5 steps
	if len(data.Steps) != 5 {
		t.Errorf("expected 5 steps, got %d", len(data.Steps))
	}
	if data.StartCmd != "python app.py" {
		t.Errorf("StartCmd: got %q", data.StartCmd)
	}
}

// === Serialization ===

func TestTemplateBuilder_ToJSON(t *testing.T) {
	tb := NewTemplate().
		FromPythonImage("3.12").
		RunCmd("pip install flask")

	jsonStr, err := tb.ToJSON()
	if err != nil {
		t.Fatal(err)
	}

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if data["fromImage"] != "python:3.12" {
		t.Errorf("fromImage: got %v", data["fromImage"])
	}
}

func TestTemplateBuilder_ToDockerfile(t *testing.T) {
	tb := NewTemplate().
		FromPythonImage("3.12").
		SetWorkdir("/app").
		Copy("requirements.txt", "/app/").
		RunCmd("pip install -r requirements.txt").
		SetEnvs(map[string]string{"PORT": "8080"}).
		SetUser("appuser").
		SetStartCmd("python app.py", WaitForPort(8080))

	df, err := tb.ToDockerfile()
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(df, "FROM python:3.12\n") {
		t.Errorf("missing FROM, got:\n%s", df)
	}
	if !strings.Contains(df, "WORKDIR /app") {
		t.Errorf("missing WORKDIR, got:\n%s", df)
	}
	if !strings.Contains(df, "COPY requirements.txt /app/") {
		t.Errorf("missing COPY, got:\n%s", df)
	}
	if !strings.Contains(df, "RUN pip install -r requirements.txt") {
		t.Errorf("missing RUN, got:\n%s", df)
	}
	if !strings.Contains(df, "ENV PORT=8080") {
		t.Errorf("missing ENV, got:\n%s", df)
	}
	if !strings.Contains(df, "USER appuser") {
		t.Errorf("missing USER, got:\n%s", df)
	}
	if !strings.Contains(df, "ENTRYPOINT python app.py") {
		t.Errorf("missing ENTRYPOINT, got:\n%s", df)
	}
}

func TestTemplateBuilder_ToDockerfile_CannotConvertFromTemplate(t *testing.T) {
	tb := NewTemplate().FromTemplate("my-template")
	_, err := tb.ToDockerfile()
	if err == nil {
		t.Error("expected error when converting template-based builder to Dockerfile")
	}
}

func TestTemplateBuilder_ToDockerfile_NoBaseImage(t *testing.T) {
	tb := &TemplateBuilder{}
	_, err := tb.ToDockerfile()
	if err == nil {
		t.Error("expected error when no base image")
	}
}

// === Build Options ===

func TestBuildOptions(t *testing.T) {
	cfg := &buildConfig{}
	WithBuildTags([]string{"v1", "latest"})(cfg)
	WithBuildCPUCount(4)(cfg)
	WithBuildMemoryMB(2048)(cfg)
	WithBuildSkipCache(true)(cfg)

	called := false
	WithOnBuildLogs(func(e LogEntry) { called = true })(cfg)

	if len(cfg.tags) != 2 {
		t.Errorf("tags: got %v", cfg.tags)
	}
	if cfg.cpuCount != 4 {
		t.Errorf("cpuCount: got %d", cfg.cpuCount)
	}
	if cfg.memoryMB != 2048 {
		t.Errorf("memoryMB: got %d", cfg.memoryMB)
	}
	if !cfg.skipCache {
		t.Error("skipCache should be true")
	}
	if cfg.onBuildLogs == nil {
		t.Error("onBuildLogs should not be nil")
	}
	cfg.onBuildLogs(LogEntry{Message: "test"})
	if !called {
		t.Error("onBuildLogs callback not called")
	}
}

// === MCP helper ===

func TestNewGitHubMCPConfig(t *testing.T) {
	cfg := NewGitHubMCPConfig(map[string]GitHubMCPServerConfig{
		"github/owner/repo": {
			RunCmd:     "npx @owner/repo",
			InstallCmd: "npm install @owner/repo",
			Envs:       map[string]string{"TOKEN": "xxx"},
		},
	})

	entry, ok := cfg["github/owner/repo"]
	if !ok {
		t.Fatal("expected entry for github/owner/repo")
	}
	srv, ok := entry.(GitHubMCPServerConfig)
	if !ok {
		t.Fatalf("expected GitHubMCPServerConfig, got %T", entry)
	}
	if srv.RunCmd != "npx @owner/repo" {
		t.Errorf("RunCmd: got %q", srv.RunCmd)
	}
}

// === TemplateBuilderOption ===

func TestNewTemplate_WithFileContextPath(t *testing.T) {
	tb := NewTemplate(WithFileContextPath("/tmp/ctx"))
	if tb.fileContextPath != "/tmp/ctx" {
		t.Errorf("fileContextPath: got %q", tb.fileContextPath)
	}
}

func TestNewTemplate_WithIgnorePatterns(t *testing.T) {
	tb := NewTemplate(WithIgnorePatterns([]string{"*.pyc", "__pycache__"}))
	if len(tb.ignorePatterns) != 2 {
		t.Fatalf("ignorePatterns: got %d", len(tb.ignorePatterns))
	}
	if tb.ignorePatterns[0] != "*.pyc" {
		t.Errorf("ignorePatterns[0]: got %q", tb.ignorePatterns[0])
	}
}

func TestNewTemplate_CombinedOptions(t *testing.T) {
	tb := NewTemplate(
		WithFileContextPath("/app"),
		WithIgnorePatterns([]string{"node_modules"}),
	)
	if tb.fileContextPath != "/app" {
		t.Errorf("fileContextPath: got %q", tb.fileContextPath)
	}
	if len(tb.ignorePatterns) != 1 {
		t.Errorf("ignorePatterns: got %d", len(tb.ignorePatterns))
	}
}

// === Instruction ForceUpload ===

func TestInstruction_ForceUpload(t *testing.T) {
	boolTrue := true
	inst := Instruction{
		Type:        InstructionCopy,
		Args:        []string{"src", "dest"},
		ForceUpload: &boolTrue,
	}
	if inst.ForceUpload == nil || !*inst.ForceUpload {
		t.Error("ForceUpload should be true")
	}

	// nil ForceUpload should omit from JSON
	inst2 := Instruction{Type: InstructionRun, Args: []string{"echo"}}
	if inst2.ForceUpload != nil {
		t.Error("ForceUpload should be nil by default")
	}
}

// === Build API types ===

func TestBuildTemplateRequest_Serialization(t *testing.T) {
	tb := NewTemplate().FromPythonImage("3.12").RunCmd("pip install flask")
	data := tb.serialize()

	req := buildTemplateRequest{
		Alias:     "my-template",
		BuildDesc: data,
		CPUCount:  2,
		MemoryMB:  1024,
		Tags:      []string{"v1"},
	}

	if req.Alias != "my-template" {
		t.Errorf("Alias: got %q", req.Alias)
	}
	if req.BuildDesc.FromImage != "python:3.12" {
		t.Errorf("FromImage: got %q", req.BuildDesc.FromImage)
	}
	if len(req.BuildDesc.Steps) != 1 {
		t.Errorf("Steps: got %d", len(req.BuildDesc.Steps))
	}
	if req.CPUCount != 2 {
		t.Errorf("CPUCount: got %d", req.CPUCount)
	}
}

func TestBuildStatusResponse_Types(t *testing.T) {
	resp := BuildStatusResponse{
		BuildID:    "build-123",
		TemplateID: "tmpl-456",
		Status:     BuildStatusBuilding,
		Logs:       []string{"Step 1/3", "Step 2/3"},
		Reason:     nil,
	}
	if resp.Status != BuildStatusBuilding {
		t.Errorf("Status: got %q", resp.Status)
	}
	if len(resp.Logs) != 2 {
		t.Errorf("Logs: got %d", len(resp.Logs))
	}

	// With reason
	resp2 := BuildStatusResponse{
		Status: BuildStatusError,
		Reason: &BuildStatusReason{Message: "build failed", Step: "RUN pip install"},
	}
	if resp2.Reason.Message != "build failed" {
		t.Errorf("Reason.Message: got %q", resp2.Reason.Message)
	}
}

// === §2.2 RunCmds / RunCmdAsUser / RunCmdsAsUser ===

func TestTemplateBuilder_RunCmds(t *testing.T) {
	tb := NewTemplate().RunCmds([]string{"echo 1", "echo 2", "echo 3"})
	data := tb.serialize()
	if data.Steps[0].Args[0] != "echo 1 && echo 2 && echo 3" {
		t.Errorf("got %q", data.Steps[0].Args[0])
	}
}

func TestTemplateBuilder_RunCmdAsUser(t *testing.T) {
	tb := NewTemplate().RunCmdAsUser("apt update", "root")
	data := tb.serialize()
	if len(data.Steps[0].Args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(data.Steps[0].Args))
	}
	if data.Steps[0].Args[1] != "root" {
		t.Errorf("user: got %q", data.Steps[0].Args[1])
	}
}

func TestTemplateBuilder_RunCmdsAsUser(t *testing.T) {
	tb := NewTemplate().RunCmdsAsUser([]string{"apt update", "apt install -y vim"}, "root")
	data := tb.serialize()
	if !strings.Contains(data.Steps[0].Args[0], "apt update && apt install -y vim") {
		t.Errorf("got %q", data.Steps[0].Args[0])
	}
	if data.Steps[0].Args[1] != "root" {
		t.Errorf("user: got %q", data.Steps[0].Args[1])
	}
}

// === §2.3 CopyOption ===

func TestTemplateBuilder_CopyWithCopyOptions(t *testing.T) {
	tb := NewTemplate().Copy("src", "/dest",
		WithCopyUser("root"),
		WithCopyMode(0o755),
		WithCopyForceUpload(true),
		WithCopyResolveSymlinks(false),
	)
	data := tb.serialize()
	s := data.Steps[0]
	if s.Args[2] != "root" {
		t.Errorf("user: got %q", s.Args[2])
	}
	if s.Args[3] != "0755" {
		t.Errorf("mode: got %q", s.Args[3])
	}
	if s.ForceUpload == nil || !*s.ForceUpload {
		t.Error("ForceUpload should be true")
	}
	if s.ResolveSymlinks == nil || *s.ResolveSymlinks {
		t.Error("ResolveSymlinks should be false")
	}
}

func TestTemplateBuilder_CopyItems(t *testing.T) {
	forceUpload := true
	tb := NewTemplate().CopyItems([]CopyItem{
		{Src: "a.py", Dest: "/app/a.py"},
		{Src: "b.py", Dest: "/app/b.py", User: "root", ForceUpload: &forceUpload},
	})
	data := tb.serialize()
	if len(data.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(data.Steps))
	}
	if data.Steps[1].Args[2] != "root" {
		t.Errorf("second step user: got %q", data.Steps[1].Args[2])
	}
	if data.Steps[1].ForceUpload == nil || !*data.Steps[1].ForceUpload {
		t.Error("second step ForceUpload should be true")
	}
}

// === §2.11 validateRelativePath ===

func TestValidateRelativePath(t *testing.T) {
	tests := []struct {
		src     string
		wantErr bool
	}{
		{"app.py", false},
		{"src/main.go", false},
		{"./config.yaml", false},
		{"/etc/passwd", true},
		{"../secret", true},
		{"foo/../../bar", true},
	}
	for _, tt := range tests {
		err := validateRelativePath(tt.src)
		if (err != nil) != tt.wantErr {
			t.Errorf("validateRelativePath(%q): err=%v, wantErr=%v", tt.src, err, tt.wantErr)
		}
	}
}

func TestTemplateBuilder_Copy_RejectsAbsolutePath(t *testing.T) {
	tb := NewTemplate().Copy("/etc/passwd", "/app/")
	if tb.err == nil {
		t.Error("expected error for absolute source path")
	}
}

func TestTemplateBuilder_Copy_RejectsTraversal(t *testing.T) {
	tb := NewTemplate().Copy("../secret", "/app/")
	if tb.err == nil {
		t.Error("expected error for path traversal")
	}
}

// === §2.16 SetEnvs empty guard ===

func TestTemplateBuilder_SetEnvs_EmptyMap(t *testing.T) {
	tb := NewTemplate().SetEnvs(map[string]string{})
	data := tb.serialize()
	if len(data.Steps) != 0 {
		t.Errorf("expected 0 steps for empty envs, got %d", len(data.Steps))
	}
}

// === §2.1 Convenience Methods ===

func TestTemplateBuilder_Remove(t *testing.T) {
	tb := NewTemplate().Remove("/tmp/foo", true, true)
	data := tb.serialize()
	if data.Steps[0].Args[0] != "rm -r -f '/tmp/foo'" {
		t.Errorf("got %q", data.Steps[0].Args[0])
	}
}

func TestTemplateBuilder_RemoveMultiple(t *testing.T) {
	tb := NewTemplate().RemoveMultiple([]string{"/a", "/b"}, true, false)
	data := tb.serialize()
	if data.Steps[0].Args[0] != "rm -f '/a' '/b'" {
		t.Errorf("got %q", data.Steps[0].Args[0])
	}
}

func TestTemplateBuilder_RemoveMultiple_Empty(t *testing.T) {
	tb := NewTemplate().RemoveMultiple(nil, true, false)
	data := tb.serialize()
	if len(data.Steps) != 0 {
		t.Errorf("expected 0 steps for empty paths, got %d", len(data.Steps))
	}
}

func TestTemplateBuilder_Rename(t *testing.T) {
	tb := NewTemplate().Rename("/a", "/b", true)
	data := tb.serialize()
	if data.Steps[0].Args[0] != "mv -f '/a' '/b'" {
		t.Errorf("got %q", data.Steps[0].Args[0])
	}
}

func TestTemplateBuilder_MakeDir(t *testing.T) {
	tb := NewTemplate().MakeDir("/app/data", 0o755)
	data := tb.serialize()
	if data.Steps[0].Args[0] != "mkdir -p -m 0755 '/app/data'" {
		t.Errorf("got %q", data.Steps[0].Args[0])
	}
}

func TestTemplateBuilder_MakeDirs(t *testing.T) {
	tb := NewTemplate().MakeDirs([]string{"/a", "/b"}, 0)
	data := tb.serialize()
	if data.Steps[0].Args[0] != "mkdir -p '/a' '/b'" {
		t.Errorf("got %q", data.Steps[0].Args[0])
	}
}

func TestTemplateBuilder_MakeDirs_Empty(t *testing.T) {
	tb := NewTemplate().MakeDirs(nil, 0)
	data := tb.serialize()
	if len(data.Steps) != 0 {
		t.Errorf("expected 0 steps for empty paths, got %d", len(data.Steps))
	}
}

func TestTemplateBuilder_MakeSymlink(t *testing.T) {
	tb := NewTemplate().MakeSymlink("/src", "/link", false)
	data := tb.serialize()
	if data.Steps[0].Args[0] != "ln -s '/src' '/link'" {
		t.Errorf("got %q", data.Steps[0].Args[0])
	}
}

func TestTemplateBuilder_GitClone(t *testing.T) {
	tb := NewTemplate().GitClone("https://github.com/user/repo",
		WithGitCloneBranch("main"),
		WithGitCloneDepth(1),
		WithGitClonePath("/app"),
	)
	data := tb.serialize()
	cmd := data.Steps[0].Args[0]
	if !strings.Contains(cmd, "git clone 'https://github.com/user/repo'") {
		t.Errorf("missing clone URL, got %q", cmd)
	}
	if !strings.Contains(cmd, "--branch 'main' --single-branch") {
		t.Errorf("missing branch, got %q", cmd)
	}
	if !strings.Contains(cmd, "--depth 1") {
		t.Errorf("missing depth, got %q", cmd)
	}
	if !strings.Contains(cmd, "/app") {
		t.Errorf("missing path, got %q", cmd)
	}
}

func TestTemplateBuilder_GitClone_WithUser(t *testing.T) {
	tb := NewTemplate().GitClone("https://github.com/user/repo",
		WithGitCloneUser("appuser"),
	)
	data := tb.serialize()
	if len(data.Steps[0].Args) < 2 || data.Steps[0].Args[1] != "appuser" {
		t.Errorf("expected user=appuser, got args=%v", data.Steps[0].Args)
	}
}

func TestTemplateBuilder_AddMCPServer(t *testing.T) {
	tb := NewTemplate().FromTemplate("mcp-gateway").AddMCPServer("server1", "server2")
	if tb.err != nil {
		t.Fatalf("unexpected error: %v", tb.err)
	}
	data := tb.serialize()
	if !strings.Contains(data.Steps[0].Args[0], "mcp-gateway pull server1 server2") {
		t.Errorf("got %q", data.Steps[0].Args[0])
	}
	if data.Steps[0].Args[1] != "root" {
		t.Errorf("user: got %q", data.Steps[0].Args[1])
	}
}

func TestTemplateBuilder_AddMCPServer_WrongBaseTemplate(t *testing.T) {
	tb := NewTemplate().AddMCPServer("server1")
	if tb.err == nil {
		t.Error("expected error when base template is not mcp-gateway")
	}
	if !strings.Contains(tb.err.Error(), "mcp-gateway") {
		t.Errorf("error message should mention mcp-gateway, got %q", tb.err.Error())
	}
}

func TestTemplateBuilder_AddMCPServer_FromImageNotAllowed(t *testing.T) {
	tb := NewTemplate().FromImage("python:3.12").AddMCPServer("server1")
	if tb.err == nil {
		t.Error("expected error when using FromImage instead of FromTemplate mcp-gateway")
	}
}

// === BetaDevContainerPrebuild / BetaSetDevContainerStart ===

func TestTemplateBuilder_BetaDevContainerPrebuild(t *testing.T) {
	tb := NewTemplate().FromTemplate("devcontainer").BetaDevContainerPrebuild("/workspaces/myapp")
	if tb.err != nil {
		t.Fatalf("unexpected error: %v", tb.err)
	}
	data := tb.serialize()
	if len(data.Steps) == 0 {
		t.Fatal("expected at least 1 step")
	}
	cmd := data.Steps[0].Args[0]
	if !strings.Contains(cmd, "devcontainer build --workspace-folder") {
		t.Errorf("expected devcontainer build command, got %q", cmd)
	}
	if !strings.Contains(cmd, "/workspaces/myapp") {
		t.Errorf("expected workspace path, got %q", cmd)
	}
	// Should run as root
	if len(data.Steps[0].Args) < 2 || data.Steps[0].Args[1] != "root" {
		t.Errorf("expected root user, got %v", data.Steps[0].Args)
	}
}

func TestTemplateBuilder_BetaDevContainerPrebuild_WrongTemplate(t *testing.T) {
	tb := NewTemplate().BetaDevContainerPrebuild("/workspaces/myapp")
	if tb.err == nil {
		t.Error("expected error when base template is not devcontainer")
	}
	if !strings.Contains(tb.err.Error(), "devcontainer") {
		t.Errorf("error should mention devcontainer, got %q", tb.err.Error())
	}
}

func TestTemplateBuilder_BetaSetDevContainerStart(t *testing.T) {
	tb := NewTemplate().FromTemplate("devcontainer").BetaSetDevContainerStart("/workspaces/myapp")
	if tb.err != nil {
		t.Fatalf("unexpected error: %v", tb.err)
	}
	data := tb.serialize()
	if !strings.Contains(data.StartCmd, "devcontainer up --workspace-folder") {
		t.Errorf("expected devcontainer up in StartCmd, got %q", data.StartCmd)
	}
	if !strings.Contains(data.StartCmd, "/workspaces/myapp") {
		t.Errorf("expected workspace path in StartCmd, got %q", data.StartCmd)
	}
	if !strings.Contains(data.ReadyCmd, "/devcontainer.up") {
		t.Errorf("expected /devcontainer.up in ReadyCmd, got %q", data.ReadyCmd)
	}
}

func TestTemplateBuilder_BetaSetDevContainerStart_WrongTemplate(t *testing.T) {
	tb := NewTemplate().BetaSetDevContainerStart("/workspaces/myapp")
	if tb.err == nil {
		t.Error("expected error when base template is not devcontainer")
	}
}

// === Convenience methods with WithConvenienceUser ===

func TestTemplateBuilder_Remove_WithUser(t *testing.T) {
	tb := NewTemplate().Remove("/tmp/foo", true, true, WithConvenienceUser("root"))
	data := tb.serialize()
	if len(data.Steps[0].Args) < 2 || data.Steps[0].Args[1] != "root" {
		t.Errorf("expected root user, got %v", data.Steps[0].Args)
	}
}

func TestTemplateBuilder_Remove_WithoutUser(t *testing.T) {
	tb := NewTemplate().Remove("/tmp/foo", true, true)
	data := tb.serialize()
	if len(data.Steps[0].Args) > 1 {
		t.Errorf("expected no user arg, got %v", data.Steps[0].Args)
	}
}

func TestTemplateBuilder_RemoveMultiple_WithUser(t *testing.T) {
	tb := NewTemplate().RemoveMultiple([]string{"/a", "/b"}, true, false, WithConvenienceUser("root"))
	data := tb.serialize()
	if len(data.Steps[0].Args) < 2 || data.Steps[0].Args[1] != "root" {
		t.Errorf("expected root user, got %v", data.Steps[0].Args)
	}
}

func TestTemplateBuilder_Rename_WithUser(t *testing.T) {
	tb := NewTemplate().Rename("/a", "/b", true, WithConvenienceUser("appuser"))
	data := tb.serialize()
	if len(data.Steps[0].Args) < 2 || data.Steps[0].Args[1] != "appuser" {
		t.Errorf("expected appuser, got %v", data.Steps[0].Args)
	}
}

func TestTemplateBuilder_MakeDir_WithUser(t *testing.T) {
	tb := NewTemplate().MakeDir("/app/data", 0o755, WithConvenienceUser("root"))
	data := tb.serialize()
	if len(data.Steps[0].Args) < 2 || data.Steps[0].Args[1] != "root" {
		t.Errorf("expected root user, got %v", data.Steps[0].Args)
	}
}

func TestTemplateBuilder_MakeDirs_WithUser(t *testing.T) {
	tb := NewTemplate().MakeDirs([]string{"/a", "/b"}, 0, WithConvenienceUser("root"))
	data := tb.serialize()
	if len(data.Steps[0].Args) < 2 || data.Steps[0].Args[1] != "root" {
		t.Errorf("expected root user, got %v", data.Steps[0].Args)
	}
}

func TestTemplateBuilder_MakeSymlink_WithUser(t *testing.T) {
	tb := NewTemplate().MakeSymlink("/src", "/link", false, WithConvenienceUser("appuser"))
	data := tb.serialize()
	if len(data.Steps[0].Args) < 2 || data.Steps[0].Args[1] != "appuser" {
		t.Errorf("expected appuser, got %v", data.Steps[0].Args)
	}
}

// === §2.4 Package Manager Options ===

func TestTemplateBuilder_PipInstall_UserMode(t *testing.T) {
	tb := NewTemplate().PipInstall([]string{"flask"}, WithPipGlobal(false))
	data := tb.serialize()
	cmd := data.Steps[0].Args[0]
	if !strings.Contains(cmd, "--user") {
		t.Errorf("expected --user flag, got %q", cmd)
	}
	// Should NOT have user arg (not running as root)
	if len(data.Steps[0].Args) > 1 {
		t.Errorf("user mode should not set user arg, got %v", data.Steps[0].Args)
	}
}

func TestTemplateBuilder_PipInstall_GlobalRunsAsRoot(t *testing.T) {
	tb := NewTemplate().PipInstall([]string{"flask"})
	data := tb.serialize()
	if len(data.Steps[0].Args) < 2 || data.Steps[0].Args[1] != "root" {
		t.Errorf("global pip install should run as root, got %v", data.Steps[0].Args)
	}
}

func TestTemplateBuilder_NpmInstall_Global(t *testing.T) {
	tb := NewTemplate().NpmInstall([]string{"typescript"}, WithNpmGlobal(true))
	data := tb.serialize()
	cmd := data.Steps[0].Args[0]
	if !strings.Contains(cmd, "npm install -g typescript") {
		t.Errorf("got %q", cmd)
	}
}

func TestTemplateBuilder_NpmInstall_Dev(t *testing.T) {
	tb := NewTemplate().NpmInstall([]string{"jest"}, WithNpmDev(true))
	data := tb.serialize()
	cmd := data.Steps[0].Args[0]
	if !strings.Contains(cmd, "--save-dev") {
		t.Errorf("got %q", cmd)
	}
}

func TestTemplateBuilder_BunInstall_GlobalDev(t *testing.T) {
	tb := NewTemplate().BunInstall([]string{"elysia"}, WithBunGlobal(true), WithBunDev(true))
	data := tb.serialize()
	cmd := data.Steps[0].Args[0]
	if !strings.Contains(cmd, "-g") || !strings.Contains(cmd, "--dev") {
		t.Errorf("got %q", cmd)
	}
}

func TestTemplateBuilder_AptInstall_NoRecommends(t *testing.T) {
	tb := NewTemplate().AptInstall([]string{"vim"}, WithAptNoInstallRecommends(true))
	data := tb.serialize()
	cmd := data.Steps[0].Args[0]
	if !strings.Contains(cmd, "--no-install-recommends") {
		t.Errorf("got %q", cmd)
	}
}

// === §2.10 FromImageWithAuth ===

func TestTemplateBuilder_FromImageWithAuth(t *testing.T) {
	tb := NewTemplate().FromImageWithAuth("registry.example.com/img:latest", "user", "pass")
	data := tb.serialize()
	if data.FromImage != "registry.example.com/img:latest" {
		t.Errorf("image: got %q", data.FromImage)
	}
	if data.FromImageRegistry == nil {
		t.Fatal("expected registry config")
	}
	if data.FromImageRegistry.Username != "user" || data.FromImageRegistry.Password != "pass" {
		t.Errorf("auth: got %+v", data.FromImageRegistry)
	}
}

// === §2.18 Language-specific image defaults ===

func TestTemplateBuilder_FromPythonImage_Default(t *testing.T) {
	tb := NewTemplate().FromPythonImage("")
	data := tb.serialize()
	if data.FromImage != "python:3" {
		t.Errorf("got %q, want python:3", data.FromImage)
	}
}

func TestTemplateBuilder_FromNodeImage_Default(t *testing.T) {
	tb := NewTemplate().FromNodeImage("")
	data := tb.serialize()
	if data.FromImage != "node:lts" {
		t.Errorf("got %q, want node:lts", data.FromImage)
	}
}

func TestTemplateBuilder_FromBunImage_Default(t *testing.T) {
	tb := NewTemplate().FromBunImage("")
	data := tb.serialize()
	if data.FromImage != "oven/bun:latest" {
		t.Errorf("got %q, want oven/bun:latest", data.FromImage)
	}
}

func TestTemplateBuilder_FromDebianImage_Default(t *testing.T) {
	tb := NewTemplate().FromDebianImage("")
	data := tb.serialize()
	if data.FromImage != "debian:stable" {
		t.Errorf("got %q, want debian:stable", data.FromImage)
	}
}

func TestTemplateBuilder_FromUbuntuImage_Default(t *testing.T) {
	tb := NewTemplate().FromUbuntuImage("")
	data := tb.serialize()
	if data.FromImage != "ubuntu:latest" {
		t.Errorf("got %q, want ubuntu:latest", data.FromImage)
	}
}

// === §2.8 LogEntry ===

func TestLogEntry_String(t *testing.T) {
	e := LogEntry{Message: "hello world", Level: "info"}
	if e.String() != "hello world" {
		t.Errorf("got %q", e.String())
	}
}

// === §2.5 FromDockerfile ===

func TestTemplateBuilder_FromDockerfile(t *testing.T) {
	dockerfile := `FROM python:3.12
WORKDIR /app
COPY requirements.txt /app/
RUN pip install -r requirements.txt
ENV PORT=8080
USER appuser
ENTRYPOINT python app.py`

	tb := NewTemplate().FromDockerfile(dockerfile)
	data := tb.serialize()

	if data.FromImage != "python:3.12" {
		t.Errorf("FromImage: got %q", data.FromImage)
	}
	if data.StartCmd != "python app.py" {
		t.Errorf("StartCmd: got %q", data.StartCmd)
	}
	// WORKDIR + COPY + RUN + ENV + USER = 5 steps
	if len(data.Steps) != 5 {
		t.Errorf("expected 5 steps, got %d", len(data.Steps))
	}
}

func TestTemplateBuilder_FromDockerfile_CMD(t *testing.T) {
	dockerfile := "FROM node:20\nCMD node server.js\n"
	tb := NewTemplate().FromDockerfile(dockerfile)
	data := tb.serialize()
	if data.StartCmd != "node server.js" {
		t.Errorf("StartCmd: got %q", data.StartCmd)
	}
}

// === §2.17 Instruction ResolveSymlinks ===

func TestInstruction_ResolveSymlinks(t *testing.T) {
	val := false
	inst := Instruction{
		Type:            InstructionCopy,
		Args:            []string{"src", "dest"},
		ResolveSymlinks: &val,
	}
	if inst.ResolveSymlinks == nil || *inst.ResolveSymlinks {
		t.Error("ResolveSymlinks should be false")
	}
}

// === §3.1 Error Types ===

func TestBuildError_Is(t *testing.T) {
	err := &BuildError{SandboxError: SandboxError{Message: "fail"}, BuildID: "b1", TemplateID: "t1"}
	if err.Error() != "fail" {
		t.Errorf("Error(): got %q", err.Error())
	}
	if err.BuildID != "b1" || err.TemplateID != "t1" {
		t.Error("BuildID/TemplateID mismatch")
	}
}

func TestFileUploadError_Is(t *testing.T) {
	err := &FileUploadError{SandboxError{Message: "upload fail"}}
	if err.Error() != "upload fail" {
		t.Errorf("Error(): got %q", err.Error())
	}
}

func TestTemplateError_Is(t *testing.T) {
	err := &TemplateError{SandboxError{Message: "tmpl fail"}}
	if err.Error() != "tmpl fail" {
		t.Errorf("Error(): got %q", err.Error())
	}
}

// === ForbiddenError ===

func TestForbiddenError_Is(t *testing.T) {
	err := &ForbiddenError{SandboxError{Message: "forbidden"}}
	if err.Error() != "forbidden" {
		t.Errorf("Error(): got %q", err.Error())
	}
}

// === parseEnvLine quoted values ===

func TestParseEnvLine_SimpleKV(t *testing.T) {
	envs := parseEnvLine("PORT=8080")
	if envs["PORT"] != "8080" {
		t.Errorf("PORT: got %q", envs["PORT"])
	}
}

func TestParseEnvLine_QuotedValue(t *testing.T) {
	envs := parseEnvLine(`MY_VAR="hello world"`)
	if envs["MY_VAR"] != "hello world" {
		t.Errorf("MY_VAR: got %q", envs["MY_VAR"])
	}
}

func TestParseEnvLine_SingleQuoted(t *testing.T) {
	envs := parseEnvLine("MY_VAR='hello world'")
	if envs["MY_VAR"] != "hello world" {
		t.Errorf("MY_VAR: got %q", envs["MY_VAR"])
	}
}

func TestParseEnvLine_MultipleKV(t *testing.T) {
	envs := parseEnvLine(`A=1 B="two" C=3`)
	if envs["A"] != "1" || envs["B"] != "two" || envs["C"] != "3" {
		t.Errorf("got %v", envs)
	}
}

func TestParseEnvLine_LegacyFormat(t *testing.T) {
	envs := parseEnvLine("MY_KEY my value")
	if envs["MY_KEY"] != "my value" {
		t.Errorf("MY_KEY: got %q", envs["MY_KEY"])
	}
}

// === shouldIgnore with ** patterns ===

func TestShouldIgnore_SimpleGlob(t *testing.T) {
	if !shouldIgnore("foo.pyc", []string{"*.pyc"}) {
		t.Error("expected *.pyc to match foo.pyc")
	}
	if shouldIgnore("foo.py", []string{"*.pyc"}) {
		t.Error("expected *.pyc to NOT match foo.py")
	}
}

func TestShouldIgnore_BasenameMatch(t *testing.T) {
	if !shouldIgnore("dir/sub/foo.pyc", []string{"*.pyc"}) {
		t.Error("expected *.pyc to match dir/sub/foo.pyc via basename")
	}
}

func TestShouldIgnore_DoubleStarPrefix(t *testing.T) {
	if !shouldIgnore("a/b/__pycache__", []string{"**/__pycache__"}) {
		t.Error("expected **/__pycache__ to match a/b/__pycache__")
	}
	if !shouldIgnore("__pycache__", []string{"**/__pycache__"}) {
		t.Error("expected **/__pycache__ to match __pycache__ at root")
	}
}

func TestShouldIgnore_DoubleStarSuffix(t *testing.T) {
	if !shouldIgnore("node_modules/foo/bar.js", []string{"node_modules/**"}) {
		t.Error("expected node_modules/** to match node_modules/foo/bar.js")
	}
	if shouldIgnore("src/app.js", []string{"node_modules/**"}) {
		t.Error("expected node_modules/** to NOT match src/app.js")
	}
}

// === AptInstall empty guard ===

func TestTemplateBuilder_AptInstall_Empty(t *testing.T) {
	tb := NewTemplate().AptInstall(nil)
	data := tb.serialize()
	if len(data.Steps) != 0 {
		t.Errorf("expected 0 steps for empty packages, got %d", len(data.Steps))
	}
}

// === Shell quoting with spaces ===

func TestTemplateBuilder_Remove_PathWithSpaces(t *testing.T) {
	tb := NewTemplate().Remove("/tmp/my file", false, false)
	data := tb.serialize()
	expected := "rm '/tmp/my file'"
	if data.Steps[0].Args[0] != expected {
		t.Errorf("got %q, want %q", data.Steps[0].Args[0], expected)
	}
}

// === DefaultBuildLoggerWithLevel unknown level ===

func TestDefaultBuildLoggerWithLevel_UnknownLevel(t *testing.T) {
	// Should not panic and should default to info level
	logger := DefaultBuildLoggerWithLevel("unknown_level")
	if logger == nil {
		t.Error("logger should not be nil")
	}
}

// === Constants ===

func TestAllTrafficConstant(t *testing.T) {
	if AllTraffic != "0.0.0.0/0" {
		t.Errorf("AllTraffic: got %q", AllTraffic)
	}
}

func TestSDKVersion(t *testing.T) {
	if SDKVersion == "" {
		t.Error("SDKVersion should not be empty")
	}
}
