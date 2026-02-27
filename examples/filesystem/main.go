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

	// === 写入和读取 ===
	fmt.Println("=== Write & Read ===")
	_, err = sbx.Files.Write(ctx, "/tmp/hello.txt", "Hello, World!")
	if err != nil {
		log.Fatal(err)
	}

	content, err := sbx.Files.Read(ctx, "/tmp/hello.txt")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Content:", content)

	// === 写入字节 ===
	fmt.Println("=== Write Bytes ===")
	_, err = sbx.Files.Write(ctx, "/tmp/data.bin", []byte{0x48, 0x65, 0x6c, 0x6c, 0x6f})
	if err != nil {
		log.Fatal(err)
	}
	data, err := sbx.Files.ReadBytes(ctx, "/tmp/data.bin")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Bytes: %v (%s)\n", data, string(data))

	// === 创建目录 ===
	fmt.Println("=== MakeDir ===")
	created, err := sbx.Files.MakeDir(ctx, "/tmp/mydir")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Created: %v\n", created)

	// 目录已存在
	created2, err := sbx.Files.MakeDir(ctx, "/tmp/mydir")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Created again: %v (expected false)\n", created2)

	// === 判断是否存在 ===
	fmt.Println("=== Exists ===")
	exists, _ := sbx.Files.Exists(ctx, "/tmp/mydir")
	fmt.Printf("/tmp/mydir exists: %v\n", exists)
	exists, _ = sbx.Files.Exists(ctx, "/tmp/nonexistent")
	fmt.Printf("/tmp/nonexistent exists: %v\n", exists)

	// === 列出目录 ===
	fmt.Println("=== List ===")
	entries, err := sbx.Files.List(ctx, "/tmp")
	if err != nil {
		log.Fatal(err)
	}
	for _, e := range entries {
		fmt.Printf("  %s [%s] %d bytes\n", e.Name, e.Type, e.Size)
	}

	// === 获取文件信息 ===
	fmt.Println("=== GetInfo ===")
	info, err := sbx.Files.GetInfo(ctx, "/tmp/hello.txt")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Name: %s, Type: %s, Size: %d\n", info.Name, info.Type, info.Size)

	// === 重命名 ===
	fmt.Println("=== Rename ===")
	_, err = sbx.Files.Rename(ctx, "/tmp/hello.txt", "/tmp/renamed.txt")
	if err != nil {
		log.Fatal(err)
	}
	exists, _ = sbx.Files.Exists(ctx, "/tmp/hello.txt")
	fmt.Printf("Old path exists: %v (expected false)\n", exists)
	exists, _ = sbx.Files.Exists(ctx, "/tmp/renamed.txt")
	fmt.Printf("New path exists: %v (expected true)\n", exists)

	// === 删除 ===
	fmt.Println("=== Remove ===")
	err = sbx.Files.Remove(ctx, "/tmp/renamed.txt")
	if err != nil {
		log.Fatal(err)
	}
	exists, _ = sbx.Files.Exists(ctx, "/tmp/renamed.txt")
	fmt.Printf("After remove exists: %v (expected false)\n", exists)

	// === 监听目录 ===
	fmt.Println("=== WatchDir ===")
	watcher, err := sbx.Files.WatchDir(ctx, "/tmp/mydir")
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Stop(ctx)

	// 创建文件以触发事件
	_, _ = sbx.Files.Write(ctx, "/tmp/mydir/watched.txt", "trigger")

	events, err := watcher.GetNewEvents(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Watch events: %d\n", len(events))
	for _, ev := range events {
		fmt.Printf("  %s: %s\n", ev.Type, ev.Name)
	}

	// === 签名 URL ===
	fmt.Println("=== Signed URLs ===")
	dlURL := sbx.DownloadURL("/tmp/data.bin")
	ulURL := sbx.UploadURL("/tmp/upload.txt")
	fmt.Println("Download URL:", dlURL[:50]+"...")
	fmt.Println("Upload URL:", ulURL[:50]+"...")

	fmt.Println("Done!")
}
