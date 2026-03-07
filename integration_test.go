package e2b

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func skipWithoutAPIKey(t *testing.T) {
	t.Helper()
	if os.Getenv("E2B_API_KEY") == "" {
		t.Skip("E2B_API_KEY not set, skipping integration test")
	}
}

func newTestClient(t *testing.T) *Client {
	t.Helper()
	skipWithoutAPIKey(t)
	client, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func newTestSandbox(t *testing.T) (*Client, *Sandbox) {
	t.Helper()
	client := newTestClient(t)
	ctx := context.Background()
	sbx, err := client.CreateSandbox(ctx, WithTimeout(120))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sbx.Kill(context.Background()) })
	return client, sbx
}

// ===========================
// T1.9 管理面集成测试
// ===========================

func TestIntegration_CreateAndKillSandbox(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	sbx, err := client.CreateSandbox(ctx, WithTimeout(60))
	if err != nil {
		t.Fatal(err)
	}
	if sbx.ID == "" {
		t.Fatal("sandbox ID should not be empty")
	}

	info, err := sbx.GetInfo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if info.SandboxID != sbx.ID {
		t.Errorf("info.SandboxID=%q != sbx.ID=%q", info.SandboxID, sbx.ID)
	}

	_, err = sbx.Kill(ctx)
	if err != nil {
		t.Fatal(err)
	}
}

func TestIntegration_ConnectSandbox(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	sbx, err := client.CreateSandbox(ctx, WithTimeout(60))
	if err != nil {
		t.Fatal(err)
	}
	defer sbx.Kill(ctx)

	sbx2, err := client.ConnectSandbox(ctx, sbx.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sbx2.ID != sbx.ID {
		t.Errorf("connected sandbox ID mismatch: %q != %q", sbx2.ID, sbx.ID)
	}
}

func TestIntegration_ListSandboxes(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	sbx, err := client.CreateSandbox(ctx, WithTimeout(60), WithMetadata(map[string]string{"test_list": "yes"}))
	if err != nil {
		t.Fatal(err)
	}
	defer sbx.Kill(ctx)

	paginator := client.ListSandboxes(ctx, nil)
	sandboxes, err := paginator.All(ctx)
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, s := range sandboxes {
		if s.SandboxID == sbx.ID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("created sandbox %s not found in list (%d total)", sbx.ID, len(sandboxes))
	}
}

func TestIntegration_SetTimeout(t *testing.T) {
	_, sbx := newTestSandbox(t)
	ctx := context.Background()

	err := sbx.SetTimeout(ctx, 180)
	if err != nil {
		t.Fatal(err)
	}
}

func TestIntegration_PauseAndResume(t *testing.T) {
	client, sbx := newTestSandbox(t)
	ctx := context.Background()

	err := sbx.Pause(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Wait a moment for pause to take effect
	time.Sleep(2 * time.Second)

	sbx2, err := client.ConnectSandbox(ctx, sbx.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sbx2.ID != sbx.ID {
		t.Errorf("resumed sandbox ID mismatch")
	}
}

func TestIntegration_SignatureURL(t *testing.T) {
	_, sbx := newTestSandbox(t)

	dlURL := sbx.DownloadURL("/tmp/test.txt")
	if !strings.Contains(dlURL, "signature=v1_") {
		t.Errorf("download URL should contain signature, got: %s", dlURL)
	}
	if !strings.Contains(dlURL, "path=%2Ftmp%2Ftest.txt") && !strings.Contains(dlURL, "path=/tmp/test.txt") {
		t.Errorf("download URL should contain path, got: %s", dlURL)
	}

	ulURL := sbx.UploadURL("/tmp/upload.txt", WithSignatureExpiration(3600))
	if !strings.Contains(ulURL, "signature=v1_") {
		t.Errorf("upload URL should contain signature, got: %s", ulURL)
	}
	if !strings.Contains(ulURL, "signature_expiration=") {
		t.Errorf("upload URL should contain expiration, got: %s", ulURL)
	}
}

func TestIntegration_Snapshot(t *testing.T) {
	client, sbx := newTestSandbox(t)
	ctx := context.Background()

	snapshot, err := sbx.CreateSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.SnapshotID == "" {
		t.Fatal("snapshot ID should not be empty")
	}

	// List snapshots
	paginator := client.ListSnapshots(ctx, nil)
	snapshots, err := paginator.All(ctx)
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, s := range snapshots {
		if s.SnapshotID == snapshot.SnapshotID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("snapshot %s not found in list", snapshot.SnapshotID)
	}

	// Delete snapshot
	err = client.DeleteSnapshot(ctx, snapshot.SnapshotID)
	if err != nil {
		t.Fatal(err)
	}
}

// ===========================
// T2.3 Code Interpreter 集成测试
// ===========================

func TestIntegration_RunCodeBasic(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	ci, err := client.CreateCodeInterpreter(ctx, WithTimeout(120))
	if err != nil {
		t.Fatal(err)
	}
	defer ci.Kill(ctx)

	exec, err := ci.RunCode(ctx, `print("hello")`)
	if err != nil {
		t.Fatal(err)
	}

	if len(exec.Logs.Stdout) == 0 || !strings.Contains(exec.Logs.Stdout[0], "hello") {
		t.Errorf("expected stdout containing 'hello', got: %v", exec.Logs.Stdout)
	}
}

func TestIntegration_RunCodeChart(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	ci, err := client.CreateCodeInterpreter(ctx, WithTimeout(120))
	if err != nil {
		t.Fatal(err)
	}
	defer ci.Kill(ctx)

	exec, err := ci.RunCode(ctx, `
import matplotlib.pyplot as plt
plt.figure()
plt.plot([1,2,3], [4,5,6])
plt.show()
`)
	if err != nil {
		t.Fatal(err)
	}

	hasPNG := false
	for _, r := range exec.Results {
		if r.PNG != nil {
			hasPNG = true
			break
		}
	}
	if !hasPNG {
		t.Error("expected a PNG result from matplotlib plot")
	}
}

func TestIntegration_RunCodeError(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	ci, err := client.CreateCodeInterpreter(ctx, WithTimeout(120))
	if err != nil {
		t.Fatal(err)
	}
	defer ci.Kill(ctx)

	exec, err := ci.RunCode(ctx, `raise ValueError("test error")`)
	if err != nil {
		t.Fatal(err)
	}

	if exec.Error == nil {
		t.Fatal("expected execution error")
	}
	if exec.Error.Name != "ValueError" {
		t.Errorf("expected ValueError, got %q", exec.Error.Name)
	}
}

func TestIntegration_RunCodeStreaming(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	ci, err := client.CreateCodeInterpreter(ctx, WithTimeout(120))
	if err != nil {
		t.Fatal(err)
	}
	defer ci.Kill(ctx)

	var stdoutLines []string
	_, err = ci.RunCode(ctx, `
for i in range(3):
    print(f"line{i}")
`,
		WithOnCodeStdout(func(msg OutputMessage) {
			stdoutLines = append(stdoutLines, msg.Line)
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(stdoutLines) < 3 {
		t.Errorf("expected at least 3 stdout callbacks, got %d: %v", len(stdoutLines), stdoutLines)
	}
}

func TestIntegration_CodeContext(t *testing.T) {
	client := newTestClient(t)
	ctx := context.Background()

	ci, err := client.CreateCodeInterpreter(ctx, WithTimeout(120))
	if err != nil {
		t.Fatal(err)
	}
	defer ci.Kill(ctx)

	// Create context
	codeCtx, err := ci.CreateCodeContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if codeCtx.ID == "" {
		t.Fatal("context ID should not be empty")
	}

	// Set variable in context
	_, err = ci.RunCode(ctx, "x = 42", WithCodeContext(codeCtx))
	if err != nil {
		t.Fatal(err)
	}

	// Read variable from same context
	exec, err := ci.RunCode(ctx, "print(x)", WithCodeContext(codeCtx))
	if err != nil {
		t.Fatal(err)
	}
	if len(exec.Logs.Stdout) == 0 || !strings.Contains(exec.Logs.Stdout[0], "42") {
		t.Errorf("expected '42' in stdout, got: %v", exec.Logs.Stdout)
	}

	// List contexts
	contexts, err := ci.ListCodeContexts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(contexts) == 0 {
		t.Error("expected at least one context")
	}

	// Restart context
	err = ci.RestartCodeContext(ctx, codeCtx)
	if err != nil {
		t.Fatal(err)
	}

	// After restart, variable should be gone
	exec2, err := ci.RunCode(ctx, `
try:
    print(x)
except NameError:
    print("undefined")
`, WithCodeContext(codeCtx))
	if err != nil {
		t.Fatal(err)
	}
	if len(exec2.Logs.Stdout) == 0 || !strings.Contains(exec2.Logs.Stdout[0], "undefined") {
		t.Errorf("expected 'undefined' after restart, got: %v", exec2.Logs.Stdout)
	}

	// Remove context
	err = ci.RemoveCodeContext(ctx, codeCtx.ID)
	if err != nil {
		t.Fatal(err)
	}
}

// ===========================
// T3.6.1 命令执行集成测试
// ===========================

func TestIntegration_CommandRun(t *testing.T) {
	_, sbx := newTestSandbox(t)
	ctx := context.Background()

	result, err := sbx.Commands.Run(ctx, "echo hello")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Stdout, "hello") {
		t.Errorf("expected 'hello' in stdout, got: %q", result.Stdout)
	}
}

func TestIntegration_CommandRunError(t *testing.T) {
	_, sbx := newTestSandbox(t)
	ctx := context.Background()

	result, err := sbx.Commands.Run(ctx, "exit 42")
	if err == nil {
		t.Fatal("expected error for non-zero exit code")
	}
	var exitErr *CommandExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *CommandExitError, got %T: %v", err, err)
	}
	if exitErr.ExitCode != 42 {
		t.Errorf("expected exit code 42, got %d", exitErr.ExitCode)
	}
	if result == nil || result.ExitCode != 42 {
		t.Error("result should also have exit code 42")
	}
}

func TestIntegration_CommandStart(t *testing.T) {
	_, sbx := newTestSandbox(t)
	ctx := context.Background()

	handle, err := sbx.Commands.Start(ctx, "sleep 1 && echo background_done")
	if err != nil {
		t.Fatal(err)
	}
	if handle.PID <= 0 {
		t.Errorf("expected positive PID, got %d", handle.PID)
	}

	result, err := handle.Wait(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Stdout, "background_done") {
		t.Errorf("expected 'background_done' in stdout, got: %q", result.Stdout)
	}
}

func TestIntegration_CommandConnect(t *testing.T) {
	_, sbx := newTestSandbox(t)
	ctx := context.Background()

	handle, err := sbx.Commands.Start(ctx, "sleep 5 && echo connected_output")
	if err != nil {
		t.Fatal(err)
	}

	// Connect to same process
	handle2, err := sbx.Commands.Connect(ctx, handle.PID)
	if err != nil {
		t.Fatal(err)
	}

	result, err := handle2.Wait(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Stdout, "connected_output") {
		t.Errorf("expected 'connected_output' in stdout, got: %q", result.Stdout)
	}
}

func TestIntegration_CommandStdin(t *testing.T) {
	_, sbx := newTestSandbox(t)
	ctx := context.Background()

	if !sbx.supportsStdin() {
		t.Skip("envd version does not support stdin")
	}

	handle, err := sbx.Commands.Start(ctx, "head -n1", WithStdin(true))
	if err != nil {
		t.Fatal(err)
	}

	err = handle.SendStdin(ctx, "hello_from_stdin\n")
	if err != nil {
		t.Fatal(err)
	}

	result, err := handle.Wait(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Stdout, "hello_from_stdin") {
		t.Errorf("expected stdin echo, got: %q", result.Stdout)
	}
}

func TestIntegration_CommandList(t *testing.T) {
	_, sbx := newTestSandbox(t)
	ctx := context.Background()

	handle, err := sbx.Commands.Start(ctx, "sleep 30")
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Kill(ctx)

	processes, err := sbx.Commands.List(ctx)
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, p := range processes {
		if p.PID == handle.PID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("started process PID=%d not in list (total=%d)", handle.PID, len(processes))
	}
}

func TestIntegration_CommandKill(t *testing.T) {
	_, sbx := newTestSandbox(t)
	ctx := context.Background()

	handle, err := sbx.Commands.Start(ctx, "sleep 300")
	if err != nil {
		t.Fatal(err)
	}

	ok, err := sbx.Commands.Kill(ctx, handle.PID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("expected Kill to return true")
	}
}

func TestIntegration_CommandCallback(t *testing.T) {
	_, sbx := newTestSandbox(t)
	ctx := context.Background()

	var stdoutLines []string
	var stderrLines []string

	_, err := sbx.Commands.Run(ctx, `echo out1; echo err1 >&2; echo out2`,
		WithOnStdout(func(line string) {
			stdoutLines = append(stdoutLines, line)
		}),
		WithOnStderr(func(line string) {
			stderrLines = append(stderrLines, line)
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(stdoutLines) < 2 {
		t.Errorf("expected at least 2 stdout callbacks, got %d", len(stdoutLines))
	}
	if len(stderrLines) < 1 {
		t.Errorf("expected at least 1 stderr callback, got %d", len(stderrLines))
	}
}

// ===========================
// T3.6.2 文件操作集成测试
// ===========================

func TestIntegration_FileReadWrite(t *testing.T) {
	_, sbx := newTestSandbox(t)
	ctx := context.Background()

	_, err := sbx.Files.Write(ctx, "/tmp/test_rw.txt", "hello world")
	if err != nil {
		t.Fatal(err)
	}

	content, err := sbx.Files.Read(ctx, "/tmp/test_rw.txt")
	if err != nil {
		t.Fatal(err)
	}
	if content != "hello world" {
		t.Errorf("expected 'hello world', got %q", content)
	}
}

func TestIntegration_FileReadBytes(t *testing.T) {
	_, sbx := newTestSandbox(t)
	ctx := context.Background()

	data := []byte{0x00, 0x01, 0x02, 0xFF}
	_, err := sbx.Files.Write(ctx, "/tmp/test_bytes.bin", data)
	if err != nil {
		t.Fatal(err)
	}

	readData, err := sbx.Files.ReadBytes(ctx, "/tmp/test_bytes.bin")
	if err != nil {
		t.Fatal(err)
	}
	if len(readData) != len(data) {
		t.Errorf("expected %d bytes, got %d", len(data), len(readData))
	}
	for i := range data {
		if readData[i] != data[i] {
			t.Errorf("byte %d: expected %02x, got %02x", i, data[i], readData[i])
		}
	}
}

func TestIntegration_FileWriteFiles(t *testing.T) {
	_, sbx := newTestSandbox(t)
	ctx := context.Background()

	files := []WriteEntry{
		{Path: "/tmp/batch_a.txt", Data: "content A"},
		{Path: "/tmp/batch_b.txt", Data: "content B"},
	}
	_, err := sbx.Files.WriteFiles(ctx, files)
	if err != nil {
		t.Fatal(err)
	}

	a, err := sbx.Files.Read(ctx, "/tmp/batch_a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if a != "content A" {
		t.Errorf("expected 'content A', got %q", a)
	}

	b, err := sbx.Files.Read(ctx, "/tmp/batch_b.txt")
	if err != nil {
		t.Fatal(err)
	}
	if b != "content B" {
		t.Errorf("expected 'content B', got %q", b)
	}
}

func TestIntegration_FileList(t *testing.T) {
	_, sbx := newTestSandbox(t)
	ctx := context.Background()

	sbx.Files.MakeDir(ctx, "/tmp/listdir")
	sbx.Files.Write(ctx, "/tmp/listdir/file1.txt", "a")
	sbx.Files.Write(ctx, "/tmp/listdir/file2.txt", "b")

	entries, err := sbx.Files.List(ctx, "/tmp/listdir")
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) < 2 {
		t.Errorf("expected at least 2 entries, got %d", len(entries))
	}

	names := map[string]bool{}
	for _, e := range entries {
		names[e.Name] = true
	}
	if !names["file1.txt"] || !names["file2.txt"] {
		t.Errorf("expected file1.txt and file2.txt, got: %v", names)
	}
}

func TestIntegration_FileMakeDir(t *testing.T) {
	_, sbx := newTestSandbox(t)
	ctx := context.Background()

	created, err := sbx.Files.MakeDir(ctx, "/tmp/newdir_test")
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Error("expected created=true for new directory")
	}

	exists, err := sbx.Files.Exists(ctx, "/tmp/newdir_test")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Error("expected directory to exist")
	}
}

func TestIntegration_FileRemove(t *testing.T) {
	_, sbx := newTestSandbox(t)
	ctx := context.Background()

	sbx.Files.Write(ctx, "/tmp/to_remove.txt", "delete me")

	exists, _ := sbx.Files.Exists(ctx, "/tmp/to_remove.txt")
	if !exists {
		t.Fatal("file should exist before remove")
	}

	err := sbx.Files.Remove(ctx, "/tmp/to_remove.txt")
	if err != nil {
		t.Fatal(err)
	}

	exists, _ = sbx.Files.Exists(ctx, "/tmp/to_remove.txt")
	if exists {
		t.Error("file should not exist after remove")
	}
}

func TestIntegration_FileRename(t *testing.T) {
	_, sbx := newTestSandbox(t)
	ctx := context.Background()

	sbx.Files.Write(ctx, "/tmp/old_name.txt", "content")

	_, err := sbx.Files.Rename(ctx, "/tmp/old_name.txt", "/tmp/new_name.txt")
	if err != nil {
		t.Fatal(err)
	}

	exists, _ := sbx.Files.Exists(ctx, "/tmp/old_name.txt")
	if exists {
		t.Error("old path should not exist")
	}

	exists, _ = sbx.Files.Exists(ctx, "/tmp/new_name.txt")
	if !exists {
		t.Error("new path should exist")
	}

	content, _ := sbx.Files.Read(ctx, "/tmp/new_name.txt")
	if content != "content" {
		t.Errorf("expected 'content', got %q", content)
	}
}

func TestIntegration_FileGetInfo(t *testing.T) {
	_, sbx := newTestSandbox(t)
	ctx := context.Background()

	sbx.Files.Write(ctx, "/tmp/info_test.txt", "12345")

	info, err := sbx.Files.GetInfo(ctx, "/tmp/info_test.txt")
	if err != nil {
		t.Fatal(err)
	}

	if info.Name != "info_test.txt" {
		t.Errorf("expected name 'info_test.txt', got %q", info.Name)
	}
	if info.Type != EntryTypeFile {
		t.Errorf("expected type FILE, got %q", info.Type)
	}
	if info.Size <= 0 {
		t.Errorf("expected positive size, got %d", info.Size)
	}
}

func TestIntegration_FileWatchDir(t *testing.T) {
	_, sbx := newTestSandbox(t)
	ctx := context.Background()

	sbx.Files.MakeDir(ctx, "/tmp/watchdir_test")

	var receivedEvents []FilesystemEvent
	var mu sync.Mutex

	watcher, err := sbx.Files.WatchDir(ctx, "/tmp/watchdir_test", func(ev FilesystemEvent) {
		mu.Lock()
		receivedEvents = append(receivedEvents, ev)
		mu.Unlock()
	})
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.Stop()

	// Create a file to trigger event
	sbx.Files.Write(ctx, "/tmp/watchdir_test/trigger.txt", "data")

	// Wait briefly for events to arrive via streaming
	time.Sleep(1 * time.Second)
	watcher.Stop()

	mu.Lock()
	events := make([]FilesystemEvent, len(receivedEvents))
	copy(events, receivedEvents)
	mu.Unlock()

	if len(events) == 0 {
		t.Error("expected at least one filesystem event")
	}

	foundCreate := false
	for _, ev := range events {
		if ev.Type == EventTypeCreate {
			foundCreate = true
			break
		}
	}
	if !foundCreate {
		t.Errorf("expected CREATE event, got: %v", events)
	}
}

// ===========================
// T4.4 Git 集成测试
// ===========================

func TestIntegration_GitFullWorkflow(t *testing.T) {
	_, sbx := newTestSandbox(t)
	ctx := context.Background()

	repoPath := "/home/user/test_repo"

	// Init
	err := sbx.Git.Init(ctx, repoPath, WithGitInitialBranch("main"))
	if err != nil {
		t.Fatal("init:", err)
	}

	// Config
	sbx.Git.SetConfig(ctx, "user.name", "Test User", WithGitConfigScope("local"), WithGitPath(repoPath))
	sbx.Git.SetConfig(ctx, "user.email", "test@e2b.dev", WithGitConfigScope("local"), WithGitPath(repoPath))

	name, err := sbx.Git.GetConfig(ctx, "user.name", WithGitConfigScope("local"), WithGitPath(repoPath))
	if err != nil {
		t.Fatal("get config:", err)
	}
	if name != "Test User" {
		t.Errorf("expected 'Test User', got %q", name)
	}

	// Create file, add, commit
	sbx.Files.Write(ctx, repoPath+"/README.md", "# Test")
	err = sbx.Git.Add(ctx, repoPath)
	if err != nil {
		t.Fatal("add:", err)
	}

	err = sbx.Git.Commit(ctx, repoPath, "Initial commit",
		WithGitAuthorName("Test User"),
		WithGitAuthorEmail("test@e2b.dev"),
	)
	if err != nil {
		t.Fatal("commit:", err)
	}

	// Status (clean after commit)
	status, err := sbx.Git.Status(ctx, repoPath)
	if err != nil {
		t.Fatal("status:", err)
	}
	if status.HasChanges() {
		t.Errorf("expected clean status, got %d changes", len(status.FileStatus))
	}

	// Branches
	branches, err := sbx.Git.Branches(ctx, repoPath)
	if err != nil {
		t.Fatal("branches:", err)
	}
	if branches.Current != "main" {
		t.Errorf("expected current branch 'main', got %q", branches.Current)
	}
}

func TestIntegration_GitBranches(t *testing.T) {
	_, sbx := newTestSandbox(t)
	ctx := context.Background()

	repoPath := "/home/user/branch_test"
	sbx.Git.Init(ctx, repoPath, WithGitInitialBranch("main"))
	sbx.Git.SetConfig(ctx, "user.name", "Test", WithGitConfigScope("local"), WithGitPath(repoPath))
	sbx.Git.SetConfig(ctx, "user.email", "t@t.com", WithGitConfigScope("local"), WithGitPath(repoPath))
	sbx.Files.Write(ctx, repoPath+"/f.txt", "x")
	sbx.Git.Add(ctx, repoPath)
	sbx.Git.Commit(ctx, repoPath, "init", WithGitAuthorName("T"), WithGitAuthorEmail("t@t.com"))

	// Create branch
	err := sbx.Git.CreateBranch(ctx, repoPath, "feature")
	if err != nil {
		t.Fatal("create branch:", err)
	}

	// Checkout
	err = sbx.Git.CheckoutBranch(ctx, repoPath, "feature")
	if err != nil {
		t.Fatal("checkout:", err)
	}

	branches, _ := sbx.Git.Branches(ctx, repoPath)
	if branches.Current != "feature" {
		t.Errorf("expected current='feature', got %q", branches.Current)
	}
	if len(branches.Branches) != 2 {
		t.Errorf("expected 2 branches, got %d", len(branches.Branches))
	}

	// Delete
	sbx.Git.CheckoutBranch(ctx, repoPath, "main")
	err = sbx.Git.DeleteBranch(ctx, repoPath, "feature")
	if err != nil {
		t.Fatal("delete branch:", err)
	}

	branches, _ = sbx.Git.Branches(ctx, repoPath)
	if len(branches.Branches) != 1 {
		t.Errorf("expected 1 branch after delete, got %d", len(branches.Branches))
	}
}

func TestIntegration_GitResetRestore(t *testing.T) {
	_, sbx := newTestSandbox(t)
	ctx := context.Background()

	repoPath := "/home/user/reset_test"
	sbx.Git.Init(ctx, repoPath, WithGitInitialBranch("main"))
	sbx.Git.SetConfig(ctx, "user.name", "T", WithGitConfigScope("local"), WithGitPath(repoPath))
	sbx.Git.SetConfig(ctx, "user.email", "t@t.com", WithGitConfigScope("local"), WithGitPath(repoPath))
	sbx.Files.Write(ctx, repoPath+"/a.txt", "original")
	sbx.Git.Add(ctx, repoPath)
	sbx.Git.Commit(ctx, repoPath, "init", WithGitAuthorName("T"), WithGitAuthorEmail("t@t.com"))

	// Modify file and stage
	sbx.Files.Write(ctx, repoPath+"/a.txt", "modified")
	sbx.Git.Add(ctx, repoPath)

	// Reset (unstage)
	err := sbx.Git.Reset(ctx, repoPath)
	if err != nil {
		t.Fatal("reset:", err)
	}

	status, _ := sbx.Git.Status(ctx, repoPath)
	if status.IsClean() {
		t.Error("expected modified files after reset")
	}

	// Restore working tree
	err = sbx.Git.Restore(ctx, repoPath, []string{"a.txt"})
	if err != nil {
		t.Fatal("restore:", err)
	}

	content, _ := sbx.Files.Read(ctx, repoPath+"/a.txt")
	if strings.TrimSpace(content) != "original" {
		t.Errorf("expected 'original' after restore, got %q", content)
	}
}

func TestIntegration_GitClonePublic(t *testing.T) {
	_, sbx := newTestSandbox(t)
	ctx := context.Background()

	err := sbx.Git.Clone(ctx, "https://github.com/octocat/Hello-World.git",
		WithGitPath("/home/user/hello-world"),
		WithGitDepth(1),
	)
	if err != nil {
		t.Fatal("clone:", err)
	}

	exists, _ := sbx.Files.Exists(ctx, "/home/user/hello-world/README")
	if !exists {
		t.Error("expected README to exist after clone")
	}
}
