package benchmark

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/tool"
	"github.com/wly2lcl/basework/pkg/tool/builtin"
)

// BenchmarkToolRegistration 测试 Registry 的工具注册和查找性能。
func BenchmarkToolRegistration(b *testing.B) {
	reg := tool.NewRegistry()

	// 注册所有内置工具
	for _, t := range builtin.All() {
		if err := reg.Register(t); err != nil {
			b.Fatalf("Register 失败: %v", err)
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = reg.Get("bash")
	}
}

// BenchmarkToolMaterialize 测试 Registry.Materialize 生成工具定义列表的性能。
func BenchmarkToolMaterialize(b *testing.B) {
	reg := tool.NewRegistry()
	for _, t := range builtin.All() {
		if err := reg.Register(t); err != nil {
			b.Fatalf("Register 失败: %v", err)
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		defs := reg.Materialize()
		_ = defs
	}
}

// BenchmarkToolClone 测试 Registry.Clone 的性能。
func BenchmarkToolClone(b *testing.B) {
	reg := tool.NewRegistry()
	for _, t := range builtin.All() {
		if err := reg.Register(t); err != nil {
			b.Fatalf("Register 失败: %v", err)
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		clone := reg.Clone()
		_ = clone
	}
}

// BenchmarkBashTool_Echo 测试 BashTool 执行简单 echo 命令的性能。
func BenchmarkBashTool_Echo(b *testing.B) {
	bt := &builtin.BashTool{PermissionMode: "yolo"}
	args := json.RawMessage(`{"command":"echo hello"}`)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := bt.Execute(context.Background(), args)
		if err != nil {
			b.Fatalf("Execute 失败: %v", err)
		}
	}
}

// BenchmarkReadTool 测试 ReadTool 读取小文件的性能。
func BenchmarkReadTool(b *testing.B) {
	dir := b.TempDir()
	path := filepath.Join(dir, "test.txt")
	content := "line1\nline2\nline3\nline4\nline5\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		b.Fatal(err)
	}

	rt := &builtin.ReadTool{}
	args := mustMarshalTool(b, map[string]any{"path": path})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := rt.Execute(context.Background(), args)
		if err != nil {
			b.Fatalf("Execute 失败: %v", err)
		}
	}
}

// BenchmarkReadTool_LargeFile 测试 ReadTool 读取大文件的性能。
func BenchmarkReadTool_LargeFile(b *testing.B) {
	dir := b.TempDir()
	path := filepath.Join(dir, "large.txt")

	// 生成 10000 行数据
	var content []byte
	for i := 0; i < 10000; i++ {
		content = append(content, fmt.Sprintf("line-%d: 这是一行测试数据，用于基准测试读取大文件的性能\n", i)...)
	}
	if err := os.WriteFile(path, content, 0644); err != nil {
		b.Fatal(err)
	}

	rt := &builtin.ReadTool{}

	b.ResetTimer()
	b.ReportAllocs()
	b.SetBytes(int64(len(content)))

	for i := 0; i < b.N; i++ {
		_, err := rt.Execute(context.Background(), mustMarshalTool(b, map[string]any{"path": path}))
		if err != nil {
			b.Fatalf("Execute 失败: %v", err)
		}
	}
}

// BenchmarkWriteTool 测试 WriteTool 写入小文件的性能。
func BenchmarkWriteTool(b *testing.B) {
	dir := b.TempDir()
	wt := &builtin.WriteTool{}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		path := filepath.Join(dir, fmt.Sprintf("file_%d.txt", i))
		_, err := wt.Execute(context.Background(), mustMarshalTool(b, map[string]any{
			"path":    path,
			"content": "hello world",
		}))
		if err != nil {
			b.Fatalf("Execute 失败: %v", err)
		}
	}
}

// BenchmarkEditTool 测试 EditTool 执行字符串替换的性能。
func BenchmarkEditTool(b *testing.B) {
	dir := b.TempDir()
	path := filepath.Join(dir, "edit.txt")
	original := "hello world, hello universe, hello everyone"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		b.Fatal(err)
	}

	et := &builtin.EditTool{}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// 每次重新写入原内容（因为 edit 会修改文件）
		if err := os.WriteFile(path, []byte(original), 0644); err != nil {
			b.Fatal(err)
		}
		_, err := et.Execute(context.Background(), mustMarshalTool(b, map[string]any{
			"path": path,
			"old":  "world",
			"new":  "there",
		}))
		if err != nil {
			b.Fatalf("Execute 失败: %v", err)
		}
	}
}

// BenchmarkToolSettle 测试 Registry.Settle 的完整工具调用链路性能。
func BenchmarkToolSettle(b *testing.B) {
	reg := tool.NewRegistry()
	bt := &builtin.BashTool{PermissionMode: "yolo"}
	if err := reg.Register(bt); err != nil {
		b.Fatalf("Register 失败: %v", err)
	}

	call := llm.ToolCall{
		Name:     "bash",
		ArgsJSON: `{"command":"echo hello"}`,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := reg.Settle(context.Background(), call)
		if err != nil {
			b.Fatalf("Settle 失败: %v", err)
		}
	}
}

// BenchmarkToolExecution_DifferentTools 对比不同工具的执行性能。
func BenchmarkToolExecution_DifferentTools(b *testing.B) {
	dir := b.TempDir()

	// 准备文件供 read/edit 使用
	readPath := filepath.Join(dir, "read.txt")
	readContent := "line1\nline2\nline3\n"
	if err := os.WriteFile(readPath, []byte(readContent), 0644); err != nil {
		b.Fatal(err)
	}

	editPath := filepath.Join(dir, "edit.txt")
	editContent := "hello world"
	if err := os.WriteFile(editPath, []byte(editContent), 0644); err != nil {
		b.Fatal(err)
	}

	tools := []struct {
		name string
		exec func() (*tool.Result, error)
	}{
		{
			name: "Bash_Echo",
			exec: func() (*tool.Result, error) {
				bt := &builtin.BashTool{PermissionMode: "yolo"}
				return bt.Execute(context.Background(), json.RawMessage(`{"command":"echo hello"}`))
			},
		},
		{
			name: "Read_Small",
			exec: func() (*tool.Result, error) {
				rt := &builtin.ReadTool{}
				return rt.Execute(context.Background(), mustMarshalTool(b, map[string]any{"path": readPath}))
			},
		},
		{
			name: "Write_Small",
			exec: func() (*tool.Result, error) {
				wt := &builtin.WriteTool{}
				wPath := filepath.Join(dir, fmt.Sprintf("write_%d.txt", rand.Intn(100000)))
				return wt.Execute(context.Background(), mustMarshalTool(b, map[string]any{
					"path":    wPath,
					"content": "hello world",
				}))
			},
		},
	}

	for _, tt := range tools {
		b.Run(tt.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, err := tt.exec()
				if err != nil {
					b.Fatalf("%s 失败: %v", tt.name, err)
				}
			}
		})
	}
}

// BenchmarkToolRegistry_GetNonExistent 测试 Registry.Get 查找不存在的工具的性能。
func BenchmarkToolRegistry_GetNonExistent(b *testing.B) {
	reg := tool.NewRegistry()
	for _, t := range builtin.All() {
		if err := reg.Register(t); err != nil {
			b.Fatalf("Register 失败: %v", err)
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = reg.Get("nonexistent_tool")
	}
}

// 辅助函数
func mustMarshalTool(b *testing.B, v any) json.RawMessage {
	b.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		b.Fatalf("json.Marshal 失败: %v", err)
	}
	return data
}