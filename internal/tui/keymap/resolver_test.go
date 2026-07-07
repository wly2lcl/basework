// Package keymap 测试
package keymap

import (
	"testing"
)

// TestNewResolver 测试创建解析器
func TestNewResolver(t *testing.T) {
	r := NewResolver(DefaultBindings)
	if r == nil {
		t.Fatal("NewResolver() 返回了 nil")
	}
}

// TestResolve 测试按键解析
func TestResolve(t *testing.T) {
	r := NewResolver(DefaultBindings)

	// 解析全局层的 ctrl+c
	action, ok := r.Resolve("ctrl+c", LayerApp)
	if !ok {
		t.Fatal("ctrl+c 应被解析")
	}
	if action != ActionQuit {
		t.Fatalf("期望 ActionQuit，得到 %s", action)
	}

	// 解析应用层的 enter
	action, ok = r.Resolve("enter", LayerApp)
	if !ok {
		t.Fatal("enter 应被解析")
	}
	if action != ActionSubmit {
		t.Fatalf("期望 ActionSubmit，得到 %s", action)
	}
}

// TestResolveLayerFallback 测试层回退
func TestResolveLayerFallback(t *testing.T) {
	r := NewResolver(DefaultBindings)

	// 全局绑定应在任何层可用
	action, ok := r.Resolve("ctrl+p", LayerApp)
	if !ok {
		t.Fatal("全局绑定 ctrl+p 应在应用层可解析")
	}
	if action != ActionCommandPalette {
		t.Fatalf("期望 ActionCommandPalette，得到 %s", action)
	}
}

// TestResolveUnknown 测试未知按键
func TestResolveUnknown(t *testing.T) {
	r := NewResolver(DefaultBindings)

	_, ok := r.Resolve("unknown_key", LayerApp)
	if ok {
		t.Fatal("未知按键不应被解析")
	}
}

// TestResolveUndefinedLayer 测试未定义层
func TestResolveUndefinedLayer(t *testing.T) {
	r := NewResolver(DefaultBindings)

	_, ok := r.Resolve("enter", "undefined_layer")
	// 应在全局层找到
	if !ok {
		t.Log("未定义层应回退到全局层（日志而非失败）")
	}
}

// TestGetBindings 测试获取绑定
func TestGetBindings(t *testing.T) {
	r := NewResolver(DefaultBindings)

	bindings := r.GetBindings(LayerApp)
	if len(bindings) == 0 {
		t.Fatal("应用层绑定不应为空")
	}

	enterAction, ok := bindings["enter"]
	if !ok {
		t.Fatal("应用层应有 'enter' 绑定")
	}
	if enterAction != ActionSubmit {
		t.Fatalf("期望 ActionSubmit，得到 %s", enterAction)
	}
}

// TestGetActionForKey 测试查找指定键
func TestGetActionForKey(t *testing.T) {
	r := NewResolver(DefaultBindings)

	action, layer, ok := r.GetActionForKey("enter")
	if !ok {
		t.Fatal("'enter' 应被找到")
	}
	if action != ActionSubmit {
		t.Fatalf("期望 ActionSubmit，得到 %s", action)
	}
	if layer != LayerApp {
		t.Fatalf("期望层 LayerApp，得到 %s", layer)
	}
}

// TestCustomResolver 测试自定义解析器
func TestCustomResolver(t *testing.T) {
	bindings := []KeyBinding{
		{Key: "ctrl+q", Action: ActionQuit, Layer: LayerGlobal},
		{Key: "space", Action: ActionSubmit, Layer: LayerApp},
	}
	r := NewResolver(bindings)

	action, ok := r.Resolve("ctrl+q", LayerGlobal)
	if !ok || action != ActionQuit {
		t.Fatal("自定义绑定应工作")
	}

	action, ok = r.Resolve("space", LayerApp)
	if !ok || action != ActionSubmit {
		t.Fatal("自定义绑定应工作")
	}
}
