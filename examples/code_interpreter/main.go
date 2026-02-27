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

	// 创建代码解释器沙箱
	ci, err := client.CreateCodeInterpreter(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer ci.Kill(ctx)

	fmt.Println("Code Interpreter created:", ci.ID)

	// 运行 Python 代码
	exec, err := ci.RunCode(ctx, `
import math
result = math.factorial(10)
print(f"10! = {result}")
result
`)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Stdout:", exec.Logs.Stdout)
	fmt.Println("Text result:", exec.Text())

	// 有状态执行（带上下文）
	codeCtx, err := ci.CreateCodeContext(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// 第一个单元格：定义变量
	_, err = ci.RunCode(ctx, "x = 42", e2b.WithCodeContext(codeCtx))
	if err != nil {
		log.Fatal(err)
	}

	// 第二个单元格：使用上一个单元格的变量
	exec2, err := ci.RunCode(ctx, "print(f'x = {x}')", e2b.WithCodeContext(codeCtx))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Stateful output:", exec2.Logs.Stdout)

	// 流式输出
	fmt.Println("\nStreaming execution:")
	_, err = ci.RunCode(ctx, `
for i in range(5):
    print(f"Step {i}")
`,
		e2b.WithOnCodeStdout(func(msg e2b.OutputMessage) {
			fmt.Printf("  [stream] %s", msg.Line)
		}),
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("\nDone!")
}
