package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyPatch_Security_NormalPathSucceeds(t *testing.T) {
	workDir := t.TempDir()
	tool := NewApplyPatchTool(workDir)

	patch := "*** Begin Patch\n*** Add File: safe.txt\n+hello\n*** End Patch"
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]interface{}{
		"patch": patch,
	}))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if result.IsError {
		t.Fatalf("期望 IsError=false, 得到: %s", result.Content)
	}

	if _, err := os.Stat(filepath.Join(workDir, "safe.txt")); os.IsNotExist(err) {
		t.Error("期望 safe.txt 存在")
	}
}

func TestApplyPatch_Security_PathTraversalRejected(t *testing.T) {
	workDir := t.TempDir()
	tool := NewApplyPatchTool(workDir)

	patch := "*** Begin Patch\n*** Add File: ../../etc/crontab\n+malicious\n*** End Patch"
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]interface{}{
		"patch": patch,
	}))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result.IsError {
		t.Fatal("期望 IsError=true（路径遍历应被拦截）")
	}
	if !strings.Contains(result.Content, "路径校验失败") && !strings.Contains(result.Content, "敏感路径拦截") &&
		!strings.Contains(result.Content, "path traversal") {
		t.Errorf("期望包含拦截信息, 得到: %s", result.Content)
	}
	if !strings.Contains(result.Content, "失败 1") {
		t.Errorf("期望包含 '失败 1', 得到: %s", result.Content)
	}
}

func TestApplyPatch_Security_SensitiveGitConfigRejected(t *testing.T) {
	workDir := t.TempDir()
	tool := NewApplyPatchTool(workDir)

	patch := "*** Begin Patch\n*** Add File: .git/config\n+[core]\n+malicious\n*** End Patch"
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]interface{}{
		"patch": patch,
	}))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result.IsError {
		t.Fatal("期望 IsError=true（敏感路径 .git/config 应被拦截）")
	}
	if !strings.Contains(result.Content, "敏感路径拦截") {
		t.Errorf("期望包含 '敏感路径拦截', 得到: %s", result.Content)
	}
	if !strings.Contains(result.Content, "失败 1") {
		t.Errorf("期望包含 '失败 1', 得到: %s", result.Content)
	}
}

func TestApplyPatch_Security_SensitiveSSHRejected(t *testing.T) {
	workDir := t.TempDir()
	tool := NewApplyPatchTool(workDir)

	patch := "*** Begin Patch\n*** Add File: .ssh/authorized_keys\n+ssh-rsa malicious\n*** End Patch"
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]interface{}{
		"patch": patch,
	}))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result.IsError {
		t.Fatal("期望 IsError=true（敏感路径 .ssh/authorized_keys 应被拦截）")
	}
	if !strings.Contains(result.Content, "敏感路径拦截") {
		t.Errorf("期望包含 '敏感路径拦截', 得到: %s", result.Content)
	}
	if !strings.Contains(result.Content, "失败 1") {
		t.Errorf("期望包含 '失败 1', 得到: %s", result.Content)
	}
}

func TestSanitizePath_Normal(t *testing.T) {
	workDir := t.TempDir()
	path, err := sanitizePath(workDir, "foo/bar.txt")
	if err != nil {
		t.Fatalf("sanitizePath 返回错误: %v", err)
	}
	expected := filepath.Join(workDir, "foo/bar.txt")
	if path != expected {
		t.Errorf("期望 %q, 得到 %q", expected, path)
	}
}

func TestSanitizePath_Traversal(t *testing.T) {
	workDir := t.TempDir()
	_, err := sanitizePath(workDir, "../../etc/passwd")
	if err == nil {
		t.Fatal("期望路径遍历被拒绝")
	}
}

func TestSanitizePath_AbsolutePath(t *testing.T) {
	workDir := t.TempDir()
	_, err := sanitizePath(workDir, "/etc/passwd")
	if err == nil {
		t.Fatal("期望绝对路径被拒绝")
	}
}

func TestSanitizePath_WorkDirDot(t *testing.T) {
	// 当 workDir 为 "." 时，应能正常工作
	path, err := sanitizePath(".", "somefile.txt")
	if err != nil {
		t.Fatalf("sanitizePath 返回错误: %v", err)
	}
	abs, _ := filepath.Abs("somefile.txt")
	if path != abs {
		t.Errorf("期望 %q, 得到 %q", abs, path)
	}
}
