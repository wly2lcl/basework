package builtin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAll_ReturnsAllTools(t *testing.T) {
	tools := All()
	if len(tools) != 6 {
		t.Fatalf("期望 6 个工具，得到 %d", len(tools))
	}

	names := make(map[string]bool)
	for _, tt := range tools {
		if names[tt.Name()] {
			t.Fatalf("工具名重复: %s", tt.Name())
		}
		names[tt.Name()] = true
	}

	expected := []string{"bash", "read", "write", "edit", "grep", "glob"}
	for _, n := range expected {
		if !names[n] {
			t.Fatalf("缺少工具: %s", n)
		}
	}
}

// --- BashTool 测试 ---

func TestBashTool_Echo(t *testing.T) {
	b := &BashTool{}
	result, err := b.Execute(context.Background(), json.RawMessage(`{"command":"echo hello"}`))
	if err != nil {
		t.Fatalf("Execute 应成功: %v", err)
	}
	if result.IsError {
		t.Fatalf("不应标记为错误: %s", result.Content)
	}
	if result.Content != "hello\n" {
		t.Fatalf("期望 'hello\\n'，得到 %q", result.Content)
	}
}

func TestBashTool_ErrorExitCode(t *testing.T) {
	b := &BashTool{}
	result, err := b.Execute(context.Background(), json.RawMessage(`{"command":"exit 42"}`))
	if err != nil {
		t.Fatalf("Execute 不应返回 error（应返回 Result.IsError）: %v", err)
	}
	if !result.IsError {
		t.Fatal("应标记为错误")
	}
	if !contains(result.Content, "退出码 42") {
		t.Fatalf("内容应包含退出码 42，得到: %s", result.Content)
	}
}

func TestBashTool_Timeout(t *testing.T) {
	b := &BashTool{}
	// 极短 timeout 应该导致超时
	result, err := b.Execute(context.Background(), json.RawMessage(`{"command":"sleep 10","timeout":1}`))
	if err != nil {
		t.Fatalf("Execute 不应返回 error（应返回 Result.IsError）: %v", err)
	}
	if !result.IsError {
		t.Fatal("超时应标记为错误")
	}
}

func TestBashTool_EmptyCommand(t *testing.T) {
	b := &BashTool{}
	result, err := b.Execute(context.Background(), json.RawMessage(`{"command":""}`))
	if err != nil {
		t.Fatalf("Execute 不应返回 error（应返回 Result.IsError）: %v", err)
	}
	if !result.IsError {
		t.Fatal("空命令应标记为错误")
	}
}

func TestBashTool_InvalidArgs(t *testing.T) {
	b := &BashTool{}
	result, err := b.Execute(context.Background(), json.RawMessage(`invalid`))
	if err != nil {
		t.Fatalf("Execute 不应返回 error（应返回 Result.IsError）: %v", err)
	}
	if !result.IsError {
		t.Fatal("参数解析失败应标记为错误")
	}
}

// --- ReadTool 测试 ---

func TestReadTool_Normal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	content := "line1\nline2\nline3\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	r := &ReadTool{}
	result, err := r.Execute(context.Background(), mustMarshal(t, map[string]any{
		"path": path,
	}))
	if err != nil {
		t.Fatalf("Execute 应成功: %v", err)
	}
	if result.IsError {
		t.Fatalf("不应标记为错误: %s", result.Content)
	}
	expected := "1: line1\n2: line2\n3: line3\n"
	if result.Content != expected {
		t.Fatalf("期望 %q，得到 %q", expected, result.Content)
	}
}

func TestReadTool_OffsetLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	content := "line1\nline2\nline3\nline4\nline5\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	r := &ReadTool{}
	result, err := r.Execute(context.Background(), mustMarshal(t, map[string]any{
		"path":   path,
		"offset": 2,
		"limit":  3,
	}))
	if err != nil {
		t.Fatalf("Execute 应成功: %v", err)
	}
	if result.IsError {
		t.Fatalf("不应标记为错误: %s", result.Content)
	}
	expected := "2: line2\n3: line3\n4: line4\n"
	if result.Content != expected {
		t.Fatalf("期望 %q，得到 %q", expected, result.Content)
	}
}

func TestReadTool_FileNotFound(t *testing.T) {
	r := &ReadTool{}
	result, err := r.Execute(context.Background(), mustMarshal(t, map[string]any{
		"path": "/nonexistent/path/file.txt",
	}))
	if err != nil {
		t.Fatalf("Execute 不应返回 error（应返回 Result.IsError）: %v", err)
	}
	if !result.IsError {
		t.Fatal("文件不存在应标记为错误")
	}
}

func TestReadTool_InvalidArgs(t *testing.T) {
	r := &ReadTool{}
	result, err := r.Execute(context.Background(), json.RawMessage(`invalid`))
	if err != nil {
		t.Fatalf("Execute 不应返回 error（应返回 Result.IsError）: %v", err)
	}
	if !result.IsError {
		t.Fatal("参数解析失败应标记为错误")
	}
}

// --- WriteTool 测试 ---

func TestWriteTool_NewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "newfile.txt")

	w := &WriteTool{}
	result, err := w.Execute(context.Background(), mustMarshal(t, map[string]any{
		"path":    path,
		"content": "hello world",
	}))
	if err != nil {
		t.Fatalf("Execute 应成功: %v", err)
	}
	if result.IsError {
		t.Fatalf("不应标记为错误: %s", result.Content)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello world" {
		t.Fatalf("文件内容应为 'hello world'，得到 %q", string(data))
	}
}

func TestWriteTool_AutoCreateDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "nested", "file.txt")

	w := &WriteTool{}
	result, err := w.Execute(context.Background(), mustMarshal(t, map[string]any{
		"path":    path,
		"content": "auto created dirs",
	}))
	if err != nil {
		t.Fatalf("Execute 应成功: %v", err)
	}
	if result.IsError {
		t.Fatalf("不应标记为错误: %s", result.Content)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "auto created dirs" {
		t.Fatalf("文件内容不匹配，得到 %q", string(data))
	}
}

func TestWriteTool_InvalidArgs(t *testing.T) {
	w := &WriteTool{}
	result, err := w.Execute(context.Background(), json.RawMessage(`invalid`))
	if err != nil {
		t.Fatalf("Execute 不应返回 error（应返回 Result.IsError）: %v", err)
	}
	if !result.IsError {
		t.Fatal("参数解析失败应标记为错误")
	}
}

// --- EditTool 测试 ---

func TestEditTool_SingleReplace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edit.txt")
	content := "hello world, hello universe"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	e := &EditTool{}
	result, err := e.Execute(context.Background(), mustMarshal(t, map[string]any{
		"path": path,
		"old":  "world",
		"new":  "there",
	}))
	if err != nil {
		t.Fatalf("Execute 应成功: %v", err)
	}
	if result.IsError {
		t.Fatalf("不应标记为错误: %s", result.Content)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	expected := "hello there, hello universe"
	if string(data) != expected {
		t.Fatalf("期望 %q，得到 %q", expected, string(data))
	}
}

func TestEditTool_MultipleMatches(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edit.txt")
	content := "hello hello hello"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	e := &EditTool{}
	result, err := e.Execute(context.Background(), mustMarshal(t, map[string]any{
		"path": path,
		"old":  "hello",
		"new":  "hi",
	}))
	if err != nil {
		t.Fatalf("Execute 不应返回 error（应返回 Result.IsError）: %v", err)
	}
	if !result.IsError {
		t.Fatal("多次匹配应标记为错误")
	}
	if !contains(result.Content, "3 处匹配") {
		t.Fatalf("内容应提示匹配数量，得到: %s", result.Content)
	}
}

func TestEditTool_NoMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edit.txt")
	content := "hello world"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	e := &EditTool{}
	result, err := e.Execute(context.Background(), mustMarshal(t, map[string]any{
		"path": path,
		"old":  "nonexistent",
		"new":  "replacement",
	}))
	if err != nil {
		t.Fatalf("Execute 不应返回 error（应返回 Result.IsError）: %v", err)
	}
	if !result.IsError {
		t.Fatal("未找到匹配应标记为错误")
	}
}

func TestEditTool_FileNotFound(t *testing.T) {
	e := &EditTool{}
	result, err := e.Execute(context.Background(), mustMarshal(t, map[string]any{
		"path": "/nonexistent/file.txt",
		"old":  "foo",
		"new":  "bar",
	}))
	if err != nil {
		t.Fatalf("Execute 不应返回 error（应返回 Result.IsError）: %v", err)
	}
	if !result.IsError {
		t.Fatal("文件不存在应标记为错误")
	}
}

func TestEditTool_EmptyOld(t *testing.T) {
	e := &EditTool{}
	result, err := e.Execute(context.Background(), mustMarshal(t, map[string]any{
		"path": "/some/path",
		"old":  "",
		"new":  "bar",
	}))
	if err != nil {
		t.Fatalf("Execute 不应返回 error（应返回 Result.IsError）: %v", err)
	}
	if !result.IsError {
		t.Fatal("空 old 应标记为错误")
	}
}

// --- GrepTool 测试 ---

func TestGrepTool_FindMatch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\nfunc main() {}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.go"), []byte("package foo\nfunc foo() {}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "readme.md"), []byte("# Hello"), 0644); err != nil {
		t.Fatal(err)
	}

	g := &GrepTool{}
	result, err := g.Execute(context.Background(), mustMarshal(t, map[string]any{
		"pattern": "func",
		"path":    dir,
	}))
	if err != nil {
		t.Fatalf("Execute 应成功: %v", err)
	}
	if result.IsError {
		t.Fatalf("不应标记为错误: %s", result.Content)
	}
	if !contains(result.Content, "a.go") || !contains(result.Content, "b.go") {
		t.Fatalf("应包含两个 go 文件的匹配，得到: %s", result.Content)
	}
	// readme.md 不应匹配
	if contains(result.Content, "readme.md") {
		t.Fatalf("不应包含 readme.md 的匹配")
	}
}

func TestGrepTool_IncludeFilter(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.py"), []byte("import os"), 0644); err != nil {
		t.Fatal(err)
	}

	g := &GrepTool{}
	result, err := g.Execute(context.Background(), mustMarshal(t, map[string]any{
		"pattern": "package",
		"path":    dir,
		"include": "*.go",
	}))
	if err != nil {
		t.Fatalf("Execute 应成功: %v", err)
	}
	if result.IsError {
		t.Fatalf("不应标记为错误: %s", result.Content)
	}
	if !contains(result.Content, "a.go") {
		t.Fatalf("应包含 a.go 的匹配")
	}
	if contains(result.Content, "a.py") {
		t.Fatalf("不应包含 a.py 的匹配")
	}
}

func TestGrepTool_NoMatch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	g := &GrepTool{}
	result, err := g.Execute(context.Background(), mustMarshal(t, map[string]any{
		"pattern": "zzzzzz",
		"path":    dir,
	}))
	if err != nil {
		t.Fatalf("Execute 应成功: %v", err)
	}
	if result.IsError {
		t.Fatalf("无匹配不应标记为错误: %s", result.Content)
	}
	if result.Content != "" {
		t.Fatalf("无匹配应返回空内容，得到: %s", result.Content)
	}
}

func TestGrepTool_InvalidRegex(t *testing.T) {
	g := &GrepTool{}
	result, err := g.Execute(context.Background(), mustMarshal(t, map[string]any{
		"pattern": `[invalid`,
		"path":    ".",
	}))
	if err != nil {
		t.Fatalf("Execute 不应返回 error（应返回 Result.IsError）: %v", err)
	}
	if !result.IsError {
		t.Fatal("无效正则应标记为错误")
	}
}

// --- GlobTool 测试 ---

func TestGlobTool_Match(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte{}, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.go"), []byte{}, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "c.py"), []byte{}, 0644); err != nil {
		t.Fatal(err)
	}

	g := &GlobTool{}
	result, err := g.Execute(context.Background(), mustMarshal(t, map[string]any{
		"pattern": "*.go",
		"path":    dir,
	}))
	if err != nil {
		t.Fatalf("Execute 应成功: %v", err)
	}
	if result.IsError {
		t.Fatalf("不应标记为错误: %s", result.Content)
	}
	if !contains(result.Content, "a.go") || !contains(result.Content, "b.go") {
		t.Fatalf("应包含 a.go 和 b.go，得到: %s", result.Content)
	}
	if contains(result.Content, "c.py") {
		t.Fatalf("不应包含 c.py")
	}
}

func TestGlobTool_NoMatch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte{}, 0644); err != nil {
		t.Fatal(err)
	}

	g := &GlobTool{}
	result, err := g.Execute(context.Background(), mustMarshal(t, map[string]any{
		"pattern": "*.go",
		"path":    dir,
	}))
	if err != nil {
		t.Fatalf("Execute 应成功: %v", err)
	}
	if result.IsError {
		t.Fatalf("无匹配不应标记为错误: %s", result.Content)
	}
	if result.Content != "" {
		t.Fatalf("无匹配应返回空内容，得到: %s", result.Content)
	}
}

func TestGlobTool_SkipHiddenDirs(t *testing.T) {
	dir := t.TempDir()
	hiddenDir := filepath.Join(dir, ".hidden")
	if err := os.MkdirAll(hiddenDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hiddenDir, "a.go"), []byte{}, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.go"), []byte{}, 0644); err != nil {
		t.Fatal(err)
	}

	g := &GlobTool{}
	result, err := g.Execute(context.Background(), mustMarshal(t, map[string]any{
		"pattern": "*.go",
		"path":    dir,
	}))
	if err != nil {
		t.Fatalf("Execute 应成功: %v", err)
	}
	if contains(result.Content, ".hidden") {
		t.Fatalf("应跳过隐藏目录，得到: %s", result.Content)
	}
	if !contains(result.Content, "b.go") {
		t.Fatalf("应包含 b.go")
	}
}

// --- 辅助函数 ---

func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal 失败: %v", err)
	}
	return data
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsStr(s, substr)
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
