// Package keymap 测试
package keymap

import (
	"testing"
)

// TestDefaultBindings 测试默认绑定
func TestDefaultBindings(t *testing.T) {
	if len(DefaultBindings) == 0 {
		t.Fatal("DefaultBindings 不应为空")
	}

	hasQuit := false
	for _, b := range DefaultBindings {
		if b.Action == ActionQuit {
			hasQuit = true
			break
		}
	}
	if !hasQuit {
		t.Fatal("默认绑定应有退出动作")
	}
}

// TestValidateBindingsNoConflict 测试无冲突
func TestValidateBindingsNoConflict(t *testing.T) {
	bindings := []KeyBinding{
		{Key: "ctrl+c", Action: ActionQuit, Layer: LayerGlobal},
		{Key: "enter", Action: ActionSubmit, Layer: LayerApp},
	}
	if err := ValidateBindings(bindings); err != nil {
		t.Fatalf("无冲突绑定应验证通过: %v", err)
	}
}

// TestValidateBindingsConflict 测试冲突检测
func TestValidateBindingsConflict(t *testing.T) {
	bindings := []KeyBinding{
		{Key: "ctrl+c", Action: ActionQuit, Layer: LayerGlobal},
		{Key: "ctrl+c", Action: ActionClear, Layer: LayerGlobal},
	}
	if err := ValidateBindings(bindings); err == nil {
		t.Fatal("冲突绑定应返回错误")
	}
}

// TestValidateBindingsSameLayerKey 测试同一层同一键
func TestValidateBindingsSameLayerKey(t *testing.T) {
	// 不同层的相同键不应冲突
	bindings := []KeyBinding{
		{Key: "ctrl+c", Action: ActionQuit, Layer: LayerGlobal},
		{Key: "ctrl+c", Action: ActionQuit, Layer: LayerExit},
	}
	if err := ValidateBindings(bindings); err != nil {
		t.Fatalf("不同层的相同键不应冲突: %v", err)
	}
}

// TestDescribeAction 测试操作描述
func TestDescribeAction(t *testing.T) {
	desc := DescribeAction(ActionQuit)
	if desc == "" {
		t.Fatal("ActionQuit 的描述不应为空")
	}

	desc = DescribeAction("unknown")
	if desc != "unknown" {
		t.Fatalf("未知操作用应返回自身，得到 %s", desc)
	}
}

// TestFormatKey 测试键格式化
func TestFormatKey(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"ctrl+c", "Ctrl+C"},
		{"enter", "Enter"},
		{"esc", "Esc"},
		{"up", "↑"},
		{"down", "↓"},
		{"left", "←"},
		{"right", "→"},
		{"tab", "Tab"},
		{"backspace", "Backspace"},
		{"shift+enter", "Shift+Enter"},
	}

	for _, tt := range tests {
		result := FormatKey(tt.input)
		if result != tt.expected {
			t.Fatalf("FormatKey(%q) = %q，期望 %q", tt.input, result, tt.expected)
		}
	}
}

// TestFormatKeyDefault 测试默认格式化
func TestFormatKeyDefault(t *testing.T) {
	result := FormatKey("ctrl+x")
	if result != "Ctrl+X" {
		t.Fatalf("期望 'Ctrl+X'，得到 %s", result)
	}
}
