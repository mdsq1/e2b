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

	sbx, err := client.CreateSandbox(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer sbx.Kill(ctx)

	repoPath := "/home/user/myrepo"

	// === 初始化仓库 ===
	fmt.Println("=== Git Init ===")
	err = sbx.Git.Init(ctx, repoPath, e2b.WithGitInitialBranch("main"))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Initialized git repo at", repoPath)

	// === 配置 ===
	fmt.Println("=== Git Config ===")
	_ = sbx.Git.SetConfig(ctx, "user.name", "Test User", e2b.WithGitConfigScope("local"), e2b.WithGitPath(repoPath))
	_ = sbx.Git.SetConfig(ctx, "user.email", "test@example.com", e2b.WithGitConfigScope("local"), e2b.WithGitPath(repoPath))

	name, _ := sbx.Git.GetConfig(ctx, "user.name", e2b.WithGitConfigScope("local"), e2b.WithGitPath(repoPath))
	email, _ := sbx.Git.GetConfig(ctx, "user.email", e2b.WithGitConfigScope("local"), e2b.WithGitPath(repoPath))
	fmt.Printf("Config: name=%s email=%s\n", name, email)

	// === 创建文件并提交 ===
	fmt.Println("=== Add & Commit ===")
	_, _ = sbx.Files.Write(ctx, repoPath+"/README.md", "# My Project\n\nHello!")

	err = sbx.Git.Add(ctx, repoPath)
	if err != nil {
		log.Fatal(err)
	}

	err = sbx.Git.Commit(ctx, repoPath, "Initial commit",
		e2b.WithGitAuthorName("Test User"),
		e2b.WithGitAuthorEmail("test@example.com"),
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Committed initial files")

	// === 查看状态 ===
	fmt.Println("=== Git Status ===")
	status, err := sbx.Git.Status(ctx, repoPath)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Clean: %v, HasChanges: %v\n", status.IsClean(), status.HasChanges())

	// === 分支操作 ===
	fmt.Println("=== Branches ===")
	branches, err := sbx.Git.Branches(ctx, repoPath)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Current: %s, All: %v\n", branches.Current, branches.Branches)

	// 创建并切换分支
	err = sbx.Git.CreateBranch(ctx, repoPath, "feature")
	if err != nil {
		log.Fatal(err)
	}
	err = sbx.Git.CheckoutBranch(ctx, repoPath, "feature")
	if err != nil {
		log.Fatal(err)
	}

	branches, _ = sbx.Git.Branches(ctx, repoPath)
	fmt.Printf("After checkout - Current: %s, All: %v\n", branches.Current, branches.Branches)

	// 在 feature 分支上做修改
	_, _ = sbx.Files.Write(ctx, repoPath+"/feature.txt", "new feature")
	_ = sbx.Git.Add(ctx, repoPath)
	_ = sbx.Git.Commit(ctx, repoPath, "Add feature",
		e2b.WithGitAuthorName("Test User"),
		e2b.WithGitAuthorEmail("test@example.com"),
	)

	// 切换回主分支并删除 feature 分支
	_ = sbx.Git.CheckoutBranch(ctx, repoPath, "main")
	_ = sbx.Git.DeleteBranch(ctx, repoPath, "feature")
	branches, _ = sbx.Git.Branches(ctx, repoPath)
	fmt.Printf("After delete - All: %v\n", branches.Branches)

	// === 克隆（公开仓库） ===
	fmt.Println("=== Git Clone ===")
	err = sbx.Git.Clone(ctx, "https://github.com/octocat/Hello-World.git",
		e2b.WithGitPath("/home/user/hello-world"),
		e2b.WithGitDepth(1),
	)
	if err != nil {
		log.Fatal(err)
	}

	entries, _ := sbx.Files.List(ctx, "/home/user/hello-world")
	fmt.Printf("Cloned repo has %d entries\n", len(entries))

	fmt.Println("Done!")
}
