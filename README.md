# E2B Go SDK

[E2B](https://e2b.dev) 云沙箱平台的 Go SDK。用于创建、管理和操作面向 AI 智能体的沙箱环境。

**零外部依赖** — 完全基于 Go 标准库构建。

## 安装

```bash
go get github.com/mdsq1/e2b
```

## 快速开始

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	e2b "github.com/mdsq1/e2b"
)

func main() {
	ctx := context.Background()

	client, err := e2b.NewClient(e2b.WithAPIKey(os.Getenv("E2B_API_KEY")))
	if err != nil {
		log.Fatal(err)
	}

	// 创建沙箱
	sbx, err := client.CreateSandbox(ctx, e2b.WithTimeout(300))
	if err != nil {
		log.Fatal(err)
	}
	defer sbx.Kill(ctx)

	// 执行命令
	result, err := sbx.Commands.Run(ctx, "echo 'Hello from E2B!'")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result.Stdout)

	// 读写文件
	_, _ = sbx.Files.Write(ctx, "/tmp/hello.txt", "Hello, World!")
	content, _ := sbx.Files.Read(ctx, "/tmp/hello.txt")
	fmt.Println(content)
}
```

## 功能特性

### 沙箱生命周期

```go
// 创建沙箱（支持多种选项）
sbx, _ := client.CreateSandbox(ctx,
	e2b.WithTemplate("base"),
	e2b.WithTimeout(600),
	e2b.WithMetadata(map[string]string{"project": "demo"}),
	e2b.WithEnvVars(map[string]string{"MY_VAR": "value"}),
	e2b.WithAutoPause(true),
	e2b.WithSecure(true),
	e2b.WithAllowInternetAccess(true),
)

// 连接到已有沙箱
sbx, _ = client.ConnectSandbox(ctx, "sandbox-id")

// 沙箱信息与指标
info, _ := sbx.GetInfo(ctx)
metrics, _ := sbx.GetMetrics(ctx)
running := sbx.IsRunning(ctx)

// 超时管理
sbx.SetTimeout(ctx, 600)

// 暂停
sbx.Pause(ctx)

// 快照
snapshot, _ := sbx.CreateSnapshot(ctx)

// 销毁
sbx.Kill(ctx)

// 列出沙箱（分页）
paginator := client.ListSandboxes(ctx, nil)
sandboxes, _ := paginator.All(ctx)

// 列出 / 删除快照
snaps := client.ListSnapshots(ctx, nil)
client.DeleteSnapshot(ctx, "snapshot-id")

// 端口转发 & MCP
host := sbx.GetHost(8080)
mcpURL := sbx.GetMCPURL()
mcpToken, _ := sbx.GetMCPToken(ctx)
```

### 命令执行

```go
// 前台执行（阻塞）
result, err := sbx.Commands.Run(ctx, "ls -la",
	e2b.WithCwd("/home/user"),
	e2b.WithCommandTimeout(30),
	e2b.WithUser("root"),
	e2b.WithCommandEnvVars(map[string]string{"FOO": "bar"}),
)

// 后台执行（非阻塞）
handle, _ := sbx.Commands.Start(ctx, "sleep 10")
// ... 执行其他操作 ...
result, _ = handle.Wait(ctx)

// 流式输出
result, _ = sbx.Commands.Run(ctx, "cat /var/log/syslog",
	e2b.WithOnStdout(func(line string) { fmt.Print(line) }),
	e2b.WithOnStderr(func(line string) { fmt.Fprint(os.Stderr, line) }),
)

// 标准输入支持
handle, _ = sbx.Commands.Start(ctx, "cat", e2b.WithStdin(true))
handle.SendStdin(ctx, "hello\n")

// 连接到已有进程
handle, _ = sbx.Commands.Connect(ctx, pid)

// 列出 / 终止进程
processes, _ := sbx.Commands.List(ctx)
sbx.Commands.Kill(ctx, processes[0].PID)
```

### 文件系统

```go
// 读取文件（文本、字节、流）
content, _ := sbx.Files.Read(ctx, "/etc/hostname")
data, _ := sbx.Files.ReadBytes(ctx, "/tmp/binary")
rc, _ := sbx.Files.ReadStream(ctx, "/tmp/large-file")
defer rc.Close()

// 写入文件（字符串、字节、io.Reader）
_, _ = sbx.Files.Write(ctx, "/tmp/data.json", `{"key": "value"}`)
_, _ = sbx.Files.Write(ctx, "/tmp/binary", []byte{0x00, 0x01})
_, _ = sbx.Files.WriteStream(ctx, "/tmp/file", reader)

// 批量写入
_, _ = sbx.Files.WriteFiles(ctx, []e2b.WriteEntry{
	{Path: "/tmp/a.txt", Data: "aaa"},
	{Path: "/tmp/b.txt", Data: "bbb"},
})

// 目录操作
entries, _ := sbx.Files.List(ctx, "/home/user")
sbx.Files.MakeDir(ctx, "/tmp/newdir")
sbx.Files.Remove(ctx, "/tmp/oldfile")
sbx.Files.Rename(ctx, "/tmp/old", "/tmp/new")

// 检查文件是否存在 & 获取文件信息
exists, _ := sbx.Files.Exists(ctx, "/tmp/file.txt")
info, _ := sbx.Files.GetInfo(ctx, "/tmp/file.txt")

// 监听目录变更（轮询）
watcher, _ := sbx.Files.WatchDir(ctx, "/tmp", e2b.WithRecursive(true))
defer watcher.Stop(ctx)
events, _ := watcher.GetNewEvents(ctx)

// 签名 URL（上传/下载）
downloadURL := sbx.DownloadURL("/tmp/file.txt")
uploadURL := sbx.UploadURL("/tmp/upload.txt")
```

### 代码解释器

```go
ci, _ := client.CreateCodeInterpreter(ctx)
defer ci.Kill(ctx)

exec, _ := ci.RunCode(ctx, `
import numpy as np
print(np.mean([1, 2, 3, 4, 5]))
`)
fmt.Println(exec.Text()) // "3.0"

// 流式回调
exec, _ = ci.RunCode(ctx, "print('hello')",
	e2b.WithOnCodeStdout(func(msg e2b.OutputMessage) {
		fmt.Print(msg.Line)
	}),
	e2b.WithOnCodeStderr(func(msg e2b.OutputMessage) {
		fmt.Fprint(os.Stderr, msg.Line)
	}),
	e2b.WithOnResult(func(r e2b.Result) {
		fmt.Println("结果:", r.Formats())
	}),
)

// 有状态上下文
codeCtx, _ := ci.CreateCodeContext(ctx)
ci.RunCode(ctx, "x = 42", e2b.WithCodeContext(codeCtx))
exec, _ = ci.RunCode(ctx, "print(x)", e2b.WithCodeContext(codeCtx))

// 上下文管理
contexts, _ := ci.ListCodeContexts(ctx)
ci.RestartCodeContext(ctx, codeCtx)
ci.RemoveCodeContext(ctx, codeCtx.ID)
```

### PTY（伪终端）

```go
ptyHandle, _ := sbx.Pty.Create(ctx, e2b.PtySize{Cols: 80, Rows: 24})
defer ptyHandle.Kill(ctx)

// 读取输出
go func() {
	for event := range ptyHandle.Events() {
		fmt.Print(string(event.Data))
	}
}()

// 发送输入
ptyHandle.SendStdin(ctx, []byte("ls\n"))

// 调整窗口大小
ptyHandle.Resize(ctx, e2b.PtySize{Cols: 120, Rows: 40})
```

### Git 操作

```go
// 克隆
sbx.Git.Clone(ctx, "https://github.com/user/repo.git",
	e2b.WithGitPath("/home/user/repo"),
	e2b.WithGitAuth("username", "github_token"),
	e2b.WithGitDepth(1),
)

// 初始化 + 提交
sbx.Git.Init(ctx, "/home/user/project", e2b.WithGitBare(true))
sbx.Git.Add(ctx, "/home/user/project")
sbx.Git.Commit(ctx, "/home/user/project", "Initial commit",
	e2b.WithGitAuthorName("Alice"),
	e2b.WithGitAuthorEmail("alice@example.com"),
	e2b.WithGitAllowEmpty(true),
)

// 分支管理
branches, _ := sbx.Git.Branches(ctx, "/home/user/repo")
sbx.Git.CreateBranch(ctx, "/home/user/repo", "feature")  // 创建并切换
sbx.Git.CheckoutBranch(ctx, "/home/user/repo", "main")
sbx.Git.DeleteBranch(ctx, "/home/user/repo", "old-branch")

// 推送 / 拉取（带认证）
sbx.Git.Push(ctx, "/home/user/repo",
	e2b.WithGitAuth("username", "token"),
	e2b.WithGitSetUpstream(true),
	e2b.WithGitForce(true),
)
sbx.Git.Pull(ctx, "/home/user/repo",
	e2b.WithGitRemote("origin"),
	e2b.WithGitBranch("main"),
)

// 状态、重置、恢复
status, _ := sbx.Git.Status(ctx, "/home/user/repo")
fmt.Println(status.CurrentBranch, status.IsClean(), status.StagedCount())
sbx.Git.Reset(ctx, "/home/user/repo", e2b.WithGitResetMode("hard"))
sbx.Git.Restore(ctx, "/home/user/repo", []string{"file.txt"}, e2b.WithGitStaged(true))

// 远程仓库 & 配置
sbx.Git.RemoteAdd(ctx, "/home/user/repo", "upstream", "https://...",
	e2b.WithGitFetch(true), e2b.WithGitOverwrite(true))
url, _ := sbx.Git.RemoteGet(ctx, "/home/user/repo", "origin")
sbx.Git.SetConfig(ctx, "user.name", "Alice", e2b.WithGitConfigScope("global"))
val, _ := sbx.Git.GetConfig(ctx, "user.email")
sbx.Git.ConfigureUser(ctx, "Alice", "alice@example.com")

// 危险操作：在沙箱中持久化凭据
sbx.Git.DangerouslyAuthenticate(ctx, "username", "token")
```

### 模板构建器

```go
// 以编程方式构建自定义模板
tmpl := e2b.NewTemplate(e2b.WithFileContextPath("./my-app")).
	FromPythonImage("3.12").
	AptInstall([]string{"curl", "git"}).
	PipInstall([]string{"flask", "numpy"}).
	Copy("app.py", "/home/user/app.py").
	SetEnvs(map[string]string{"FLASK_APP": "app.py"}).
	SetStartCmd("python -m flask run --host=0.0.0.0", e2b.WaitForPort(5000))

// 构建并部署
buildInfo, _ := client.Build(ctx, tmpl, nil)

// 其他基础镜像
tmpl.FromNodeImage("20")
tmpl.FromBunImage("latest")
tmpl.FromTemplate("my-existing-template")
tmpl.FromImage("my-registry.com/image:tag", e2b.WithRegistryAuth("user", "pass"))
tmpl.FromAWSRegistry("ecr-image", "key-id", "secret", "us-east-1")
tmpl.FromGCPRegistry("gcr-image", serviceAccountJSON)

// 便捷方法
tmpl.MakeDir("/app", 0o755)
tmpl.Remove("/tmp/old", true, true)
tmpl.Rename("/old", "/new", false)
tmpl.MakeSymlink("/usr/bin/python3", "/usr/bin/python", true)
tmpl.GitClone("https://github.com/user/repo", e2b.WithGitCloneDepth(1))
tmpl.NpmInstall([]string{"express"}, e2b.WithNpmGlobal(true))
tmpl.BunInstall(nil)
tmpl.AptInstall([]string{"vim"}, e2b.WithAptNoInstallRecommends(true))
tmpl.SkipCache().RunCmd("echo 'always runs'")
```

## 配置

```go
client, _ := e2b.NewClient(
	e2b.WithAPIKey("sk-..."),              // 或设置 E2B_API_KEY 环境变量
	e2b.WithDomain("custom.e2b.app"),      // 或设置 E2B_DOMAIN 环境变量（默认: e2b.app）
	e2b.WithAPIURL("https://api.custom"),  // 或设置 E2B_API_URL 环境变量
	e2b.WithRequestTimeout(30*time.Second),
	e2b.WithDebug(true),                   // 或设置 E2B_DEBUG 环境变量，使用 localhost
	e2b.WithAccessToken("token"),          // 或设置 E2B_ACCESS_TOKEN 环境变量
	e2b.WithHTTPClient(customHTTPClient),  // 自定义 *http.Client
)
```

### 沙箱创建选项

```go
sbx, _ := client.CreateSandbox(ctx,
	e2b.WithTemplate("base"),
	e2b.WithTimeout(600),                  // 秒
	e2b.WithMetadata(map[string]string{"key": "val"}),
	e2b.WithEnvVars(map[string]string{"VAR": "val"}),
	e2b.WithAutoPause(true),
	e2b.WithSecure(true),
	e2b.WithAllowInternetAccess(true),
	e2b.WithNetwork(e2b.NetworkOpts{AllowOut: []string{e2b.AllTraffic}}),
	e2b.WithAutoResume(e2b.AutoResumePolicyOn),
	e2b.WithVolumeMounts([]e2b.VolumeMount{{Name: "data", Path: "/data"}}),
	e2b.WithMCP(e2b.MCPConfig{"server": map[string]any{"run_cmd": "..."}}),
)
```

## 错误处理

```go
result, err := sbx.Commands.Run(ctx, "exit 1")
if err != nil {
	var exitErr *e2b.CommandExitError
	if errors.As(err, &exitErr) {
		fmt.Printf("退出码: %d\n标准错误: %s\n", exitErr.ExitCode, exitErr.Stderr)
	}
}

// 哨兵错误（配合 errors.Is() 使用）
if errors.Is(err, e2b.ErrNotFound)  { /* 沙箱未找到 */ }
if errors.Is(err, e2b.ErrAuth)      { /* 认证失败 */ }
if errors.Is(err, e2b.ErrRateLimit) { /* 请求被限流 */ }
if errors.Is(err, e2b.ErrTimeout)   { /* 操作超时 */ }

// Git 特定错误
var authErr *e2b.GitAuthError       // Git 认证失败
var upErr   *e2b.GitUpstreamError   // 未配置上游分支

// 模板特定错误
var buildErr *e2b.BuildError        // 模板构建失败
var tmplErr  *e2b.TemplateError     // 模板管理错误
```

## Python SDK 对比

| 功能           | Python SDK                               | Go SDK                             |
| -------------- | ---------------------------------------- | ---------------------------------- |
| **创建沙箱**   | `Sandbox.create()`                       | `client.CreateSandbox(ctx)`        |
| **连接沙箱**   | `Sandbox.connect(id)`                    | `client.ConnectSandbox(ctx, id)`   |
| **销毁沙箱**   | `sbx.kill()`                             | `sbx.Kill(ctx)`                    |
| **暂停沙箱**   | `sbx.pause()`                            | `sbx.Pause(ctx)`                   |
| **创建快照**   | `sbx.snapshot()`                         | `sbx.CreateSnapshot(ctx)`          |
| **设置超时**   | `sbx.set_timeout(secs)`                  | `sbx.SetTimeout(ctx, secs)`        |
| **获取信息**   | `sbx.get_info()`                         | `sbx.GetInfo(ctx)`                 |
| **执行命令**   | `sbx.commands.run(cmd)`                  | `sbx.Commands.Run(ctx, cmd)`       |
| **后台命令**   | `sbx.commands.run(cmd, background=True)` | `sbx.Commands.Start(ctx, cmd)`     |
| **连接进程**   | `sbx.commands.connect(pid)`              | `sbx.Commands.Connect(ctx, pid)`   |
| **读取文件**   | `sbx.files.read(path)`                   | `sbx.Files.Read(ctx, path)`        |
| **写入文件**   | `sbx.files.write(path, data)`            | `sbx.Files.Write(ctx, path, data)` |
| **列出目录**   | `sbx.files.list(path)`                   | `sbx.Files.List(ctx, path)`        |
| **监听目录**   | `sbx.files.watch_dir(path)`              | `sbx.Files.WatchDir(ctx, path)`    |
| **执行代码**   | `ci.run_code(code)`                      | `ci.RunCode(ctx, code)`            |
| **代码上下文** | `ci.create_code_context()`               | `ci.CreateCodeContext(ctx)`        |
| **Git 克隆**   | `sbx.git.clone(url)`                     | `sbx.Git.Clone(ctx, url)`          |
| **Git 提交**   | `sbx.git.commit(path, msg)`              | `sbx.Git.Commit(ctx, path, msg)`   |
| **Git 推送**   | `sbx.git.push(path)`                     | `sbx.Git.Push(ctx, path)`          |
| **Git 拉取**   | `sbx.git.pull(path)`                     | `sbx.Git.Pull(ctx, path)`          |
| **Git 状态**   | `sbx.git.status(path)`                   | `sbx.Git.Status(ctx, path)`        |
| **构建模板**   | `Sandbox.build(tmpl)`                    | `client.Build(ctx, tmpl, logger)`  |
| **参数风格**   | 关键字参数 (`timeout=300`)               | 函数式选项 (`WithTimeout(300)`)    |
| **错误处理**   | 异常                                     | `error` 返回值 + `errors.Is/As`    |
| **外部依赖**   | httpx, protobuf 等                       | 零依赖（仅标准库）                 |

## 许可证

MIT
