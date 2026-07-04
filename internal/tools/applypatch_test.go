package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyPatchTool_Name(t *testing.T) {
	tool := NewApplyPatchTool("")
	if tool.Name() != "apply_patch" {
		t.Errorf("期望 Name='apply_patch', 得到 '%s'", tool.Name())
	}
}

func TestApplyPatchTool_Description(t *testing.T) {
	tool := NewApplyPatchTool("")
	if tool.Description() == "" {
		t.Error("期望 Description 非空")
	}
}

func TestApplyPatchTool_Parameters(t *testing.T) {
	tool := NewApplyPatchTool("")
	params := tool.Parameters()
	if len(params) == 0 {
		t.Error("期望 Parameters 非空")
	}
}

func TestApplyPatchTool_Execute_EmptyPatch(t *testing.T) {
	tool := NewApplyPatchTool(t.TempDir())
	result, err := tool.Execute(context.Background(), []byte(`{"patch": ""}`))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result.IsError {
		t.Error("期望 IsError=true")
	}
}

func TestApplyPatchTool_Execute_InvalidParams(t *testing.T) {
	tool := NewApplyPatchTool("")
	result, err := tool.Execute(context.Background(), []byte(`not json`))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result.IsError {
		t.Error("期望 IsError=true")
	}
}

func TestApplyPatchTool_Execute_AddFile(t *testing.T) {
	workDir := t.TempDir()
	tool := NewApplyPatchTool(workDir)

	patch := "*** Begin Patch\n*** Add File: test.txt\n+Hello, World!\n+This is a new file.\n*** End Patch"

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]interface{}{
		"patch": patch,
	}))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if result.IsError {
		t.Fatalf("期望 IsError=false, 得到: %s", result.Content)
	}

	if !strings.Contains(result.Content, "添加文件") {
		t.Errorf("期望包含 '添加文件', 得到: %s", result.Content)
	}
	if !strings.Contains(result.Content, "成功 1") {
		t.Errorf("期望包含 '成功 1', 得到: %s", result.Content)
	}

	// 验证文件已创建
	content, err := os.ReadFile(filepath.Join(workDir, "test.txt"))
	if err != nil {
		t.Fatalf("读取文件失败: %v", err)
	}
	expected := "Hello, World!\nThis is a new file."
	if string(content) != expected {
		t.Errorf("期望内容 '%s', 得到 '%s'", expected, string(content))
	}
}

func TestApplyPatchTool_Execute_DeleteFile(t *testing.T) {
	workDir := t.TempDir()

	// 先创建要删除的文件
	srcFile := filepath.Join(workDir, "old.txt")
	if err := os.WriteFile(srcFile, []byte("old content"), 0644); err != nil {
		t.Fatalf("创建文件失败: %v", err)
	}

	tool := NewApplyPatchTool(workDir)
	patch := "*** Begin Patch\n*** Delete File: old.txt\n*** End Patch"

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]interface{}{
		"patch": patch,
	}))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if result.IsError {
		t.Fatalf("期望 IsError=false, 得到: %s", result.Content)
	}

	if !strings.Contains(result.Content, "删除文件") {
		t.Errorf("期望包含 '删除文件', 得到: %s", result.Content)
	}

	// 验证文件已删除
	if _, err := os.Stat(srcFile); !os.IsNotExist(err) {
		t.Error("期望文件已被删除")
	}
}

func TestApplyPatchTool_Execute_UpdateFile(t *testing.T) {
	workDir := t.TempDir()

	// 先创建要更新的文件
	srcFile := filepath.Join(workDir, "update.txt")
	if err := os.WriteFile(srcFile, []byte("old content"), 0644); err != nil {
		t.Fatalf("创建文件失败: %v", err)
	}

	tool := NewApplyPatchTool(workDir)
	patch := "*** Begin Patch\n*** Update File: update.txt\n+new content line 1\n+new content line 2\n*** End Patch"

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]interface{}{
		"patch": patch,
	}))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if result.IsError {
		t.Fatalf("期望 IsError=false, 得到: %s", result.Content)
	}

	if !strings.Contains(result.Content, "更新文件") {
		t.Errorf("期望包含 '更新文件', 得到: %s", result.Content)
	}

	// 验证文件内容已更新
	content, err := os.ReadFile(srcFile)
	if err != nil {
		t.Fatalf("读取文件失败: %v", err)
	}
	expected := "new content line 1\nnew content line 2"
	if string(content) != expected {
		t.Errorf("期望内容 '%s', 得到 '%s'", expected, string(content))
	}
}

func TestApplyPatchTool_Execute_DeleteNonExistentFile(t *testing.T) {
	workDir := t.TempDir()
	tool := NewApplyPatchTool(workDir)

	patch := "*** Begin Patch\n*** Delete File: nonexistent.txt\n*** End Patch"

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]interface{}{
		"patch": patch,
	}))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result.IsError {
		t.Error("期望 IsError=true（部分失败）")
	}
	if !strings.Contains(result.Content, "失败 1") {
		t.Errorf("期望包含 '失败 1', 得到: %s", result.Content)
	}
}

func TestApplyPatchTool_Execute_MultipleOperations(t *testing.T) {
	workDir := t.TempDir()

	// 先创建要更新的文件
	existingFile := filepath.Join(workDir, "existing.txt")
	if err := os.WriteFile(existingFile, []byte("old"), 0644); err != nil {
		t.Fatalf("创建文件失败: %v", err)
	}

	tool := NewApplyPatchTool(workDir)
	patch := "*** Begin Patch\n*** Add File: new.txt\n+new file content\n\n*** Update File: existing.txt\n+updated content\n\n*** Delete File: existing.txt\n\n*** Add File: dir/sub/file.txt\n+deeply nested file\n*** End Patch"

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]interface{}{
		"patch": patch,
	}))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}

	if !strings.Contains(result.Content, "成功") {
		t.Errorf("期望包含 '成功', 得到: %s", result.Content)
	}

	// new.txt 应存在
	if _, err := os.Stat(filepath.Join(workDir, "new.txt")); os.IsNotExist(err) {
		t.Error("期望 new.txt 存在")
	}

	// existing.txt 应被删除
	if _, err := os.Stat(existingFile); !os.IsNotExist(err) {
		t.Error("期望 existing.txt 已被删除")
	}

	// dir/sub/file.txt 应存在
	if _, err := os.Stat(filepath.Join(workDir, "dir/sub/file.txt")); os.IsNotExist(err) {
		t.Error("期望 dir/sub/file.txt 存在")
	}
}

func TestApplyPatchTool_Execute_NoMarkers(t *testing.T) {
	tool := NewApplyPatchTool(t.TempDir())
	result, err := tool.Execute(context.Background(), []byte(`{"patch": "no markers here"}`))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result.IsError {
		t.Error("期望 IsError=true")
	}
}

func TestApplyPatchTool_Execute_AddExistingFile(t *testing.T) {
	workDir := t.TempDir()

	// 先创建文件
	srcFile := filepath.Join(workDir, "exists.txt")
	if err := os.WriteFile(srcFile, []byte("content"), 0644); err != nil {
		t.Fatalf("创建文件失败: %v", err)
	}

	tool := NewApplyPatchTool(workDir)
	patch := "*** Begin Patch\n*** Add File: exists.txt\n+new content\n*** End Patch"

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]interface{}{
		"patch": patch,
	}))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result.IsError {
		t.Error("期望 IsError=true（文件已存在）")
	}
}

func TestParsePatch_Empty(t *testing.T) {
	_, err := parsePatch("")
	if err == nil {
		t.Error("期望解析空补丁时返回错误")
	}
}

func TestParsePatch_MissingStart(t *testing.T) {
	_, err := parsePatch("*** End Patch")
	if err == nil || !strings.Contains(err.Error(), "开始标记") {
		t.Errorf("期望'缺少开始标记'错误, 得到: %v", err)
	}
}

func TestParsePatch_MissingEnd(t *testing.T) {
	_, err := parsePatch("*** Begin Patch\ncontent")
	if err == nil || !strings.Contains(err.Error(), "结束标记") {
		t.Errorf("期望'缺少结束标记'错误, 得到: %v", err)
	}
}

func TestParsePatch_UnsupportedOperation(t *testing.T) {
	_, err := parsePatch("*** Begin Patch\n*** Unknown Op: test\n*** End Patch")
	if err == nil || !strings.Contains(err.Error(), "未知操作") {
		t.Errorf("期望'未知操作'错误, 得到: %v", err)
	}
}

func TestApplyPatchTool_UpdateNonExistentFile(t *testing.T) {
	workDir := t.TempDir()
	tool := NewApplyPatchTool(workDir)

	patch := "*** Begin Patch\n*** Update File: nonexistent.txt\n+content\n*** End Patch"

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]interface{}{
		"patch": patch,
	}))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result.IsError {
		t.Error("期望 IsError=true")
	}
}

func TestApplyPatchTool_WithNestedDirectory(t *testing.T) {
	workDir := t.TempDir()
	tool := NewApplyPatchTool(workDir)

	patch := "*** Begin Patch\n*** Add File: a/b/c/d/e.txt\n+deeply nested\n*** End Patch"

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]interface{}{
		"patch": patch,
	}))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if result.IsError {
		t.Fatalf("期望 IsError=false, 得到: %s", result.Content)
	}

	if _, err := os.Stat(filepath.Join(workDir, "a/b/c/d/e.txt")); os.IsNotExist(err) {
		t.Error("期望嵌套目录文件存在")
	}
}

// mustMarshal 将 v 序列化为 JSON，测试失败时终止
func mustMarshal(t *testing.T, v interface{}) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("JSON 序列化失败: %v", err)
	}
	return data
}