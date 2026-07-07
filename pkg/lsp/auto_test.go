package lsp

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// Detect 测试
// ---------------------------------------------------------------------------

// TestDetectEmpty 测试空目录返回空 map。
func TestDetectEmpty(t *testing.T) {
	dir := t.TempDir()
	result := Detect(dir)
	if len(result) != 0 {
		t.Errorf("expected empty map for empty directory, got %v", result)
	}
}

// TestDetectGo 测试包含 .go 文件的目录。
func TestDetectGo(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	result := Detect(dir)

	_, hasGo := result["go"]
	_, lookPathErr := exec.LookPath("gopls")

	if lookPathErr == nil && !hasGo {
		t.Error("gopls is available in PATH but Detect didn't return 'go' config")
	}
	if lookPathErr != nil && hasGo {
		t.Error("gopls is not available in PATH but Detect returned 'go' config")
	}
}

// TestDetectTypeScript 测试包含 .ts 文件的目录。
func TestDetectTypeScript(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.ts"), []byte("const x = 1"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	result := Detect(dir)

	_, hasTS := result["typescript"]
	_, lookPathErr := exec.LookPath("typescript-language-server")

	if lookPathErr == nil && !hasTS {
		t.Error("typescript-language-server is available but Detect didn't return 'typescript' config")
	}
	if lookPathErr != nil && hasTS {
		t.Error("typescript-language-server is not available but Detect returned 'typescript' config")
	}
}

// TestDetectTypeScriptTSX 测试 .tsx 文件被检测为 typescript。
func TestDetectTypeScriptTSX(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "component.tsx"), []byte("const x = 1"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	result := Detect(dir)

	_, hasTS := result["typescript"]
	_, lookPathErr := exec.LookPath("typescript-language-server")

	if lookPathErr == nil && !hasTS {
		t.Error("typescript-language-server is available but .tsx not detected")
	}
}

// TestDetectJavaScript 测试包含 .js 文件的目录。
func TestDetectJavaScript(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.js"), []byte("const x = 1"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	result := Detect(dir)

	_, hasJS := result["javascript"]
	_, lookPathErr := exec.LookPath("typescript-language-server")

	if lookPathErr == nil && !hasJS {
		t.Error("typescript-language-server is available but Detect didn't return 'javascript' config")
	}
}

// TestDetectPython 测试包含 .py 文件的目录。
func TestDetectPython(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.py"), []byte("print('hello')"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	result := Detect(dir)

	_, hasPy := result["python"]
	_, lookPathErr := exec.LookPath("pyright-langserver")

	if lookPathErr == nil && !hasPy {
		t.Error("pyright-langserver is available but Detect didn't return 'python' config")
	}
	if lookPathErr != nil && hasPy {
		t.Error("pyright-langserver is not available but Detect returned 'python' config")
	}
}

// TestDetectMixed 测试混合文件的检测。
func TestDetectMixed(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(dir, "app.ts"), []byte("const x = 1"), 0644)

	result := Detect(dir)

	_, hasGo := result["go"]
	_, hasTS := result["typescript"]

	goAvailable := exec.Command("gopls", "version").Run() == nil
	tsAvailable := exec.Command("typescript-language-server", "--version").Run() == nil

	if goAvailable && !hasGo {
		t.Error("gopls available but not detected")
	}
	if tsAvailable && !hasTS {
		t.Error("typescript-language-server available but not detected")
	}
}

// TestDetectNoLSPCommand 测试 LSP 命令不可用时的行为。
// 创建一个包含 .xyz 文件的目录（ext 不在 extensionToLanguage 中），应返回空。
func TestDetectNoLSPCommand(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file.unknown"), []byte("content"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	result := Detect(dir)
	if len(result) != 0 {
		t.Errorf("expected empty map for unknown extension, got %v", result)
	}
}

// TestDetectNonExistentDirectory 测试目录不存在返回空 map。
func TestDetectNonExistentDirectory(t *testing.T) {
	result := Detect("/nonexistent/path/that/does/not/exist")
	if result != nil {
		t.Errorf("expected nil for non-existent directory, got %v", result)
	}
}

// TestDetectJSX 测试 .jsx 文件。
func TestDetectJSX(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "component.jsx"), []byte("const x = 1"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	result := Detect(dir)

	_, hasJS := result["javascript"]
	_, lookPathErr := exec.LookPath("typescript-language-server")

	if lookPathErr == nil && !hasJS {
		t.Error("typescript-language-server available but .jsx not detected as javascript")
	}
}

// TestDetectSubdirectoryOnly 测试只扫描直接子文件，不递归子目录。
func TestDetectSubdirectoryOnly(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// 在子目录中创建 .go 文件
	if err := os.WriteFile(filepath.Join(subDir, "main.go"), []byte("package main"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	result := Detect(dir)
	if len(result) != 0 {
		t.Errorf("expected empty map (only subdirectory has .go files), got %v", result)
	}
}
