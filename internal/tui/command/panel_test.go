// Package command 测试
package command

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestNewPanelModel 测试创建面板
func TestNewPanelModel(t *testing.T) {
	r := NewRegistry()
	pm := NewPanelModel(r)
	if pm == nil {
		t.Fatal("NewPanelModel() 返回了 nil")
	}
	if pm.IsVisible() {
		t.Fatal("新面板不应可见")
	}
}

// TestPanelModelOpenClose 测试打开/关闭
func TestPanelModelOpenClose(t *testing.T) {
	r := NewRegistry()
	pm := NewPanelModel(r)

	pm.Open()
	if !pm.IsVisible() {
		t.Fatal("Open 后面板应可见")
	}

	pm.Close()
	if pm.IsVisible() {
		t.Fatal("Close 后面板应不可见")
	}
}

// TestPanelModelView 测试渲染
func TestPanelModelView(t *testing.T) {
	r := NewRegistry()
	r.Register(&Command{Name: "help", Description: "帮助", Handler: func(args string) error { return nil }})
	pm := NewPanelModel(r)

	// 关闭状态渲染为空
	view := pm.View(80)
	if view != "" {
		t.Fatal("关闭状态应返回空字符串")
	}

	// 打开状态有内容
	pm.Open()
	view = pm.View(80)
	if view == "" {
		t.Fatal("打开状态不应返回空")
	}
}

// TestPanelModelKeyboardNavigation 测试键盘导航
func TestPanelModelKeyboardNavigation(t *testing.T) {
	r := NewRegistry()
	r.Register(&Command{Name: "help", Handler: func(args string) error { return nil }})
	r.Register(&Command{Name: "clear", Handler: func(args string) error { return nil }})
	r.Register(&Command{Name: "theme", Handler: func(args string) error { return nil }})
	pm := NewPanelModel(r)

	pm.Open()
	if len(pm.matches) != 3 {
		t.Fatalf("期望 3 个匹配项，得到 %d", len(pm.matches))
	}

	// 模拟向下键
	pm.Update(tea.KeyPressMsg(tea.Key{Text: "down"}))
	if pm.selected != 1 {
		t.Fatalf("向下后 selected 应为 1，得到 %d", pm.selected)
	}

	// 模拟向上键
	pm.Update(tea.KeyPressMsg(tea.Key{Text: "up"}))
	if pm.selected != 0 {
		t.Fatalf("向上后 selected 应为 0，得到 %d", pm.selected)
	}

	// 模拟 Esc 关闭
	pm.Update(tea.KeyPressMsg(tea.Key{Text: "esc"}))
	if pm.IsVisible() {
		t.Fatal("Esc 后应关闭面板")
	}
}

// TestPanelModelFiltering 测试过滤
func TestPanelModelFiltering(t *testing.T) {
	r := NewRegistry()
	r.Register(&Command{Name: "help", Handler: func(args string) error { return nil }})
	r.Register(&Command{Name: "clear", Handler: func(args string) error { return nil }})
	pm := NewPanelModel(r)

	pm.Open()
	// 输入查询
	pm.SetQuery("he")
	if len(pm.matches) != 1 || pm.matches[0].Name != "help" {
		t.Fatalf("查询 'he' 应匹配 'help'，得到 %d 个结果", len(pm.matches))
	}
}

// TestPanelModelTabComplete 测试 Tab 补全
func TestPanelModelTabComplete(t *testing.T) {
	r := NewRegistry()
	r.Register(&Command{Name: "help", Handler: func(args string) error { return nil }})
	pm := NewPanelModel(r)

	pm.Open()
	// 输入 'h' 然后 Tab 应补全为 'help'
	pm.Update(tea.KeyPressMsg(tea.Key{Text: "h"}))
	pm.Update(tea.KeyPressMsg(tea.Key{Text: "tab"}))
	if pm.Query() != "help " {
		t.Fatalf("Tab 补全后续期为 'help '，得到 %q", pm.Query())
	}
}

// TestPanelModelExecuteOnEnter 测试 Enter 执行命令
func TestPanelModelExecuteOnEnter(t *testing.T) {
	r := NewRegistry()
	r.Register(&Command{
		Name: "test",
		Handler: func(args string) error {
			return nil
		},
	})
	pm := NewPanelModel(r)

	pm.Open()
	pm.SetQuery("test")
	_, cmd := pm.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter, Text: "enter"}))

	if cmd == nil {
		t.Fatal("Enter 应产生命令")
	}

	// 验证命令类型
	resultMsg := cmd()
	if _, ok := resultMsg.(ExecuteCommandMsg); !ok {
		t.Fatalf("期望 ExecuteCommandMsg，得到 %T", resultMsg)
	}
}