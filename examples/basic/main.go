package main

import (
	"context"
	"fmt"
	"log"
	"os"

	e2b "github.com/safe-compose/e2b-go"
)

func main() {
	ctx := context.Background()

	client, err := e2b.NewClient(e2b.WithAPIKey(os.Getenv("E2B_API_KEY")))
	if err != nil {
		log.Fatal(err)
	}

	// 创建沙箱
	sbx, err := client.CreateSandbox(ctx,
		e2b.WithTimeout(300),
		e2b.WithEnvVars(map[string]string{"MY_VAR": "hello"}),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer sbx.Kill(ctx)

	fmt.Println("Sandbox created:", sbx.ID)

	// 执行命令
	result, err := sbx.Commands.Run(ctx, "echo $MY_VAR")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Command output:", result.Stdout)

	// 写入并读取文件
	_, err = sbx.Files.Write(ctx, "/tmp/test.txt", "Hello from Go SDK!")
	if err != nil {
		log.Fatal(err)
	}

	content, err := sbx.Files.Read(ctx, "/tmp/test.txt")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("File content:", content)

	// 列出目录内容
	entries, err := sbx.Files.List(ctx, "/tmp")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Found %d entries in /tmp\n", len(entries))
	for _, e := range entries {
		fmt.Printf("  %s (%s)\n", e.Name, e.Type)
	}

	// 后台命令
	handle, err := sbx.Commands.Start(ctx, "sleep 1 && echo done")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Background process started, PID:", handle.PID)

	bgResult, err := handle.Wait(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Background result:", bgResult.Stdout)

	fmt.Println("Done!")
}
