// Package plugin 测试
package plugin

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestRegisterAndList 测试注册和列出插件
func TestRegisterAndList(t *testing.T) {
	// 清空
	Clear()

	p := &testPlugin{id: "test1", slot: SlotTop}
	Register(p)

	plugins := List()
	if len(plugins) != 1 {
		t.Fatalf("期望 1 个插件，得到 %d", len(plugins))
	}
}

// TestGet 测试获取插件
func TestGet(t *testing.T) {
	Clear()

	p := &testPlugin{id: "test-get", slot: SlotTop}
	Register(p)

	got, ok := Get("test-get")
	if !ok {
		t.Fatal("Get('test-get') 应返回 true")
	}
	if got.ID() != "test-get" {
		t.Fatalf("期望 ID 'test-get'，得到 %s", got.ID())
	}

	_, ok = Get("nonexistent")
	if ok {
		t.Fatal("Get('nonexistent') 应返回 false")
	}
}

// TestListBySlot 测试按插槽列出
func TestListBySlot(t *testing.T) {
	Clear()

	Register(&testPlugin{id: "top1", slot: SlotTop})
	Register(&testPlugin{id: "top2", slot: SlotTop})
	Register(&testPlugin{id: "bottom1", slot: SlotBottom})

	topPlugins := ListBySlot(SlotTop)
	if len(topPlugins) != 2 {
		t.Fatalf("期望 2 个顶层插件，得到 %d", len(topPlugins))
	}

	bottomPlugins := ListBySlot(SlotBottom)
	if len(bottomPlugins) != 1 {
		t.Fatalf("期望 1 个底层插件，得到 %d", len(bottomPlugins))
	}

	sidebarPlugins := ListBySlot(SlotSidebar)
	if len(sidebarPlugins) != 0 {
		t.Fatalf("期望 0 个侧边栏插件，得到 %d", len(sidebarPlugins))
	}
}

// TestUnregister 测试注销
func TestUnregister(t *testing.T) {
	Clear()

	Register(&testPlugin{id: "test", slot: SlotTop})
	Unregister("test")

	_, ok := Get("test")
	if ok {
		t.Fatal("注销后 Get 应返回 false")
	}
}

// TestClear 测试清空
func TestClear(t *testing.T) {
	Clear()

	Register(&testPlugin{id: "test", slot: SlotTop})
	Clear()

	if Count() != 0 {
		t.Fatalf("Clear 后期望 Count=0，得到 %d", Count())
	}
}

// TestCount 测试计数
func TestCount(t *testing.T) {
	Clear()

	if Count() != 0 {
		t.Fatalf("初始 Count 应为 0，得到 %d", Count())
	}

	Register(&testPlugin{id: "p1", slot: SlotTop})
	Register(&testPlugin{id: "p2", slot: SlotBottom})

	if Count() != 2 {
		t.Fatalf("期望 Count=2，得到 %d", Count())
	}
}

// TestBasePlugin 测试基类
func TestBasePlugin(t *testing.T) {
	bp := NewBasePlugin("base", SlotTop)
	if bp.ID() != "base" {
		t.Fatalf("期望 ID 'base'，得到 %s", bp.ID())
	}
	if bp.Slot() != SlotTop {
		t.Fatalf("期望 Slot SlotTop，得到 %d", bp.Slot())
	}

	cmd := bp.Init()
	if cmd != nil {
		t.Fatal("Init() 应返回 nil")
	}

	updated, cmd := bp.Update(nil)
	if cmd != nil {
		t.Fatal("Update(nil) 应返回 nil")
	}
	if updated.ID() != "base" {
		t.Fatal("Update 应返回自身")
	}

	view := bp.View(80, 24)
	if view != "" {
		t.Fatal("BasePlugin View 应返回空字符串")
	}
}

// TestSlotType 测试插槽类型
func TestSlotType(t *testing.T) {
	if SlotTop != 0 {
		t.Fatalf("SlotTop 应为 0，得到 %d", SlotTop)
	}
	if SlotBottom != 1 {
		t.Fatalf("SlotBottom 应为 1，得到 %d", SlotBottom)
	}
	if SlotSidebar != 2 {
		t.Fatalf("SlotSidebar 应为 2，得到 %d", SlotSidebar)
	}
}

// testPlugin 测试用插件实现
type testPlugin struct {
	id   string
	slot SlotType
}

func (p *testPlugin) ID() string                              { return p.id }
func (p *testPlugin) Slot() SlotType                          { return p.slot }
func (p *testPlugin) Init() tea.Cmd                           { return nil }
func (p *testPlugin) Update(msg tea.Msg) (TUIPlugin, tea.Cmd) { return p, nil }
func (p *testPlugin) View(width, height int) string           { return "test-plugin-view" }
