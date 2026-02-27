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

	sbx, err := client.CreateSandbox(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer sbx.Kill(ctx)

	// === 前台命令 ===
	fmt.Println("=== Foreground Command ===")
	result, err := sbx.Commands.Run(ctx, "echo 'Hello from E2B!'",
		e2b.WithCwd("/home/user"),
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Stdout:", result.Stdout)

	// === 流式输出 ===
	fmt.Println("=== Streaming Output ===")
	_, err = sbx.Commands.Run(ctx, "for i in 1 2 3; do echo \"line $i\"; done",
		e2b.WithOnStdout(func(line string) {
			fmt.Printf("  [stdout] %s", line)
		}),
		e2b.WithOnStderr(func(line string) {
			fmt.Printf("  [stderr] %s", line)
		}),
	)
	if err != nil {
		log.Fatal(err)
	}

	// === 后台命令 ===
	fmt.Println("\n=== Background Command ===")
	handle, err := sbx.Commands.Start(ctx, "sleep 1 && echo 'background done'")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Background PID:", handle.PID)

	bgResult, err := handle.Wait(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Background result:", bgResult.Stdout)

	// === 列出进程 ===
	fmt.Println("=== List Processes ===")
	processes, err := sbx.Commands.List(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Running processes: %d\n", len(processes))
	for _, p := range processes {
		fmt.Printf("  PID=%d CMD=%s\n", p.PID, p.Cmd)
	}

	// === 错误处理 ===
	fmt.Println("=== Error Handling ===")
	result, err = sbx.Commands.Run(ctx, "exit 42")
	if err != nil {
		fmt.Printf("Command failed (expected): %v\n", err)
		if result != nil {
			fmt.Printf("Exit code: %d\n", result.ExitCode)
		}
	}

	// === 标准输入 ===
	fmt.Println("=== Stdin ===")
	handle, err = sbx.Commands.Start(ctx, "cat", e2b.WithStdin(true))
	if err != nil {
		log.Fatal(err)
	}
	_ = handle.SendStdin(ctx, "hello from stdin\n")
	_ = handle.Kill(ctx)

	fmt.Println("Done!")
}
