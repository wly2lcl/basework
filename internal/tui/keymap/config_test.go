// Package keymap 测试
package keymap

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadKeybindingsDefault 测试加载默认绑定
func TestLoadKeybindingsDefault(t *testing.T) {
	bindings, err := LoadKeybindings("/nonexistent/path/keymap.json")
	if err != nil {
		t.Fatalf("文件不存在时应使用默认: %v", err)
	}
	if len(bindings) == 0 {
		t.Fatal("默认绑定不应为空")
	}
}

// TestLoadKeybindings 测试从文件加载
func TestLoadKeybindings(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "keymap.json")

	content := `{
		"bindings": [
			{"key": "ctrl+x", "action": "quit", "layer": "global"}
		]
	}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("写入文件失败: %v", err)
	}

	bindings, err := LoadKeybindings(path)
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}

	// 应包含默认 + 自定义
	found := false
	for _, b := range bindings {
		if b.Key == "ctrl+x" && b.Action == ActionQuit && b.Layer == LayerGlobal {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("自定义绑定应被加载")
	}
}

// TestLoadKeybindingsInvalid 测试加载无效文件
func TestLoadKeybindingsInvalid(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "invalid.json")

	if err := os.WriteFile(path, []byte("invalid json"), 0644); err != nil {
		t.Fatalf("写入文件失败: %v", err)
	}

	_, err := LoadKeybindings(path)
	if err == nil {
		t.Fatal("无效 JSON 应返回错误")
	}
}

// TestLoadKeybindingsConflict 测试加载冲突绑定
func TestLoadKeybindingsConflict(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "conflict.json")

	content := `{
		"bindings": [
			{"key": "ctrl+c", "action": "quit", "layer": "global"},
			{"key": "ctrl+c", "action": "clear", "layer": "global"}
		]
	}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("写入文件失败: %v", err)
	}

	_, err := LoadKeybindings(path)
	if err == nil {
		t.Fatal("冲突绑定应返回错误")
	}
}

// TestSaveAndLoadKeybindings 测试保存和加载
func TestSaveAndLoadKeybindings(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test_save.json")

	bindings := []KeyBinding{
		{Key: "ctrl+x", Action: ActionQuit, Layer: LayerGlobal},
	}
	if err := SaveKeybindings(path, bindings); err != nil {
		t.Fatalf("保存失败: %v", err)
	}

	loaded, err := LoadKeybindings(path)
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}

	if len(loaded) == 0 {
		t.Fatal("加载的绑定不应为空")
	}
}
