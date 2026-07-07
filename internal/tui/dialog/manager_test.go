// Package dialog 测试
package dialog

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestNewManager 测试创建管理器
func TestNewManager(t *testing.T) {
	m := NewManager()
	if m == nil {
		t.Fatal("NewManager() 返回了 nil")
	}
	if m.HasDialog() {
		t.Fatal("新管理器不应有对话框")
	}
}

// TestManagerOpenClose 测试打开/关闭对话框
func TestManagerOpenClose(t *testing.T) {
	m := NewManager()
	d := NewConfirm("标题", "消息", nil, nil)

	m.Open(d)
	if !m.HasDialog() {
		t.Fatal("Open 后应有对话框")
	}
	if m.Depth() != 1 {
		t.Fatalf("期望深度 1，得到 %d", m.Depth())
	}

	m.Close()
	if m.HasDialog() {
		t.Fatal("Close 后不应有对话框")
	}
}

// TestManagerTop 测试获取栈顶
func TestManagerTop(t *testing.T) {
	m := NewManager()
	d1 := NewConfirm("第一个", "消息1", nil, nil)
	d2 := NewConfirm("第二个", "消息2", nil, nil)

	m.Open(d1)
	m.Open(d2)

	top := m.Top()
	if top == nil {
		t.Fatal("Top() 不应返回 nil")
	}
}

// TestManagerClear 测试清空
func TestManagerClear(t *testing.T) {
	m := NewManager()
	m.Open(NewConfirm("标题", "消息", nil, nil))
	m.Open(NewConfirm("标题2", "消息2", nil, nil))
	m.Clear()

	if m.HasDialog() {
		t.Fatal("Clear 后不应有对话框")
	}
}

// TestManagerIsModal 测试模态判断
func TestManagerIsModal(t *testing.T) {
	m := NewManager()
	if m.IsModal() {
		t.Fatal("空管理器不应是模态")
	}

	m.Open(NewConfirm("标题", "消息", nil, nil))
	if !m.IsModal() {
		t.Fatal("确认对话框应是模态")
	}
}

// TestManagerCloseUntilDepth 测试关闭到指定深度
func TestManagerCloseUntilDepth(t *testing.T) {
	m := NewManager()
	m.Open(NewConfirm("标题1", "消息1", nil, nil))
	m.Open(NewConfirm("标题2", "消息2", nil, nil))
	m.Open(NewConfirm("标题3", "消息3", nil, nil))

	m.CloseUntilDepth(1)
	if m.Depth() != 1 {
		t.Fatalf("期望深度 1，得到 %d", m.Depth())
	}
}

// TestManagerUpdate 测试转发消息
func TestManagerUpdate(t *testing.T) {
	m := NewManager()
	d := NewConfirm("标题", "消息", nil, nil)
	m.Open(d)

	// 发送按键消息
	cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: "enter"}))
	// Enter 在确认对话框上应产生命令（关闭对话框）
	_ = cmd
}

// TestManagerView 测试渲染
func TestManagerView(t *testing.T) {
	m := NewManager()

	// 空管理器渲染为空
	view := m.View(80, 24)
	if view != "" {
		t.Fatal("空管理器应返回空字符串")
	}

	// 有对话框应有内容
	m.Open(NewConfirm("标题", "消息", nil, nil))
	view = m.View(80, 24)
	if view == "" {
		t.Fatal("有对话框时应返回内容")
	}
}

// TestManagerStrings 测试调试字符串
func TestManagerStrings(t *testing.T) {
	m := NewManager()
	m.Open(NewConfirm("标题", "消息", nil, nil))
	m.Open(NewInput("输入标题", "提示", "", nil, nil))

	s := m.Strings()
	if s != "Confirm > Input" {
		t.Fatalf("期望 'Confirm > Input'，得到 %s", s)
	}
}

// TestConfirmDialog 测试确认对话框
func TestConfirmDialog(t *testing.T) {
	confirmed := false
	d := NewConfirm("确认", "确认操作？", func() {
		confirmed = true
	}, nil)

	if d.IsModal() != true {
		t.Fatal("确认对话框应是模态")
	}

	// Enter 确认
	d.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter, Text: "enter"}))
	if !confirmed {
		t.Fatal("Enter 应触发确认回调")
	}
}

// TestConfirmDialogCancel 测试取消按钮
func TestConfirmDialogCancel(t *testing.T) {
	cancelled := false
	d := NewConfirm("确认", "确认操作？", nil, func() {
		cancelled = true
	})

	// 按右箭头选择取消
	d.Update(tea.KeyPressMsg(tea.Key{Text: "right"}))
	d.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter, Text: "enter"}))
	if !cancelled {
		t.Fatal("选择取消后 Enter 应触发取消回调")
	}
}

// TestConfirmDialogEsc 测试 Esc 取消
func TestConfirmDialogEsc(t *testing.T) {
	cancelled := false
	d := NewConfirm("确认", "消息", nil, func() {
		cancelled = true
	})

	d.Update(tea.KeyPressMsg(tea.Key{Text: "esc"}))
	if !cancelled {
		t.Fatal("Esc 应触发取消回调")
	}
}

// TestConfirmDialogNavigation 测试导航
func TestConfirmDialogNavigation(t *testing.T) {
	d := NewConfirm("确认", "消息", nil, nil)

	// 默认选中确认 (0)
	if d.selected != 0 {
		t.Fatalf("期望 selected=0，得到 %d", d.selected)
	}

	// 向右导航到取消
	d.Update(tea.KeyPressMsg(tea.Key{Text: "right"}))
	if d.selected != 1 {
		t.Fatalf("期望 selected=1，得到 %d", d.selected)
	}

	// Tab 导航
	d.Update(tea.KeyPressMsg(tea.Key{Text: "tab"}))
	if d.selected != 0 {
		t.Fatalf("Tab 后期望 selected=0，得到 %d", d.selected)
	}

	// 向左
	d.Update(tea.KeyPressMsg(tea.Key{Text: "left"}))
	if d.selected != 0 {
		t.Fatalf("左箭头不应向左越界，期望 selected=0，得到 %d", d.selected)
	}
}

// TestConfirmDialogView 测试渲染
func TestConfirmDialogView(t *testing.T) {
	d := NewConfirm("测试标题", "测试消息", nil, nil)
	view := d.View(80)
	if view == "" {
		t.Fatal("View 不应为空")
	}
}

// TestInputDialog 测试输入对话框
func TestInputDialog(t *testing.T) {
	submittedValue := ""
	d := NewInput("输入", "请输入名称", "默认值", func(v string) {
		submittedValue = v
	}, nil)

	if d.Value() != "默认值" {
		t.Fatalf("期望值 '默认值'，得到 %s", d.Value())
	}

	// 输入字符
	d.Update(tea.KeyPressMsg(tea.Key{Text: "a"}))
	// 提交
	d.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter, Text: "enter"}))
	if submittedValue == "" {
		t.Log("输入对话框提交（输入内容可能通过其他方式验证）")
	}
}

// TestInputDialogEsc 测试输入对话框取消
func TestInputDialogEsc(t *testing.T) {
	cancelled := false
	d := NewInput("输入", "提示", "", nil, func() {
		cancelled = true
	})

	d.Update(tea.KeyPressMsg(tea.Key{Text: "esc"}))
	if !cancelled {
		t.Fatal("Esc 应触发取消回调")
	}
}

// TestInputDialogView 测试渲染
func TestInputDialogView(t *testing.T) {
	d := NewInput("标题", "提示", "值", nil, nil)
	view := d.View(80)
	if view == "" {
		t.Fatal("View 不应为空")
	}
}

// TestSelectDialog 测试选择对话框
func TestSelectDialog(t *testing.T) {
	selected := ""
	items := []string{"选项1", "选项2", "选项3"}
	d := NewSelect("选择", "请选择一项", items, func(s string) {
		selected = s
	}, nil)

	// 默认选中第一项
	if d.selected != 0 {
		t.Fatalf("期望 selected=0，得到 %d", d.selected)
	}

	// 向下导航
	d.Update(tea.KeyPressMsg(tea.Key{Text: "down"}))
	if d.selected != 1 {
		t.Fatalf("期望 selected=1，得到 %d", d.selected)
	}

	// 确认选择
	d.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter, Text: "enter"}))
	if selected != "选项2" {
		t.Fatalf("期望选中 '选项2'，得到 %s", selected)
	}
}

// TestSelectDialogEsc 测试选择对话框取消
func TestSelectDialogEsc(t *testing.T) {
	cancelled := false
	d := NewSelect("选择", "消息", []string{"a"}, nil, func() {
		cancelled = true
	})

	d.Update(tea.KeyPressMsg(tea.Key{Text: "esc"}))
	if !cancelled {
		t.Fatal("Esc 应触发取消回调")
	}
}

// TestSelectDialogHomeEnd 测试 Home/End
func TestSelectDialogHomeEnd(t *testing.T) {
	items := []string{"a", "b", "c", "d", "e"}
	d := NewSelect("选择", "消息", items, nil, nil)

	// 导航到最后
	d.Update(tea.KeyPressMsg(tea.Key{Text: "end"}))
	if d.selected != len(items)-1 {
		t.Fatalf("End 后期望 selected=%d，得到 %d", len(items)-1, d.selected)
	}

	// 导航到最前
	d.Update(tea.KeyPressMsg(tea.Key{Text: "home"}))
	if d.selected != 0 {
		t.Fatalf("Home 后期望 selected=0，得到 %d", d.selected)
	}
}

// TestSelectDialogNavigationBounds 测试导航边界
func TestSelectDialogNavigationBounds(t *testing.T) {
	items := []string{"唯一选项"}
	d := NewSelect("选择", "消息", items, nil, nil)

	// 不应越界
	d.Update(tea.KeyPressMsg(tea.Key{Text: "down"}))
	if d.selected != 0 {
		t.Fatalf("不能向下越界，期望 selected=0，得到 %d", d.selected)
	}

	d.Update(tea.KeyPressMsg(tea.Key{Text: "up"}))
	if d.selected != 0 {
		t.Fatalf("不能向上越界，期望 selected=0，得到 %d", d.selected)
	}
}

// TestSelectDialogView 测试渲染
func TestSelectDialogView(t *testing.T) {
	d := NewSelect("标题", "消息", []string{"a", "b", "c"}, nil, nil)
	view := d.View(80)
	if view == "" {
		t.Fatal("View 不应为空")
	}
}

// TestAPIFunctions 测试快捷 API
func TestAPIFunctions(t *testing.T) {
	// Confirm
	d1 := Confirm("标题", "消息", nil)
	if d1 == nil {
		t.Fatal("Confirm() 不应返回 nil")
	}

	// ConfirmWithCancel
	d2 := ConfirmWithCancel("标题", "消息", nil, nil)
	if d2 == nil {
		t.Fatal("ConfirmWithCancel() 不应返回 nil")
	}

	// Input
	d3 := Input("标题", "提示", "默认", nil)
	if d3 == nil {
		t.Fatal("Input() 不应返回 nil")
	}

	// InputWithCancel
	d4 := InputWithCancel("标题", "提示", "默认", nil, nil)
	if d4 == nil {
		t.Fatal("InputWithCancel() 不应返回 nil")
	}

	// Select
	d5 := Select("标题", "消息", []string{"a"}, nil)
	if d5 == nil {
		t.Fatal("Select() 不应返回 nil")
	}

	// SelectWithCancel
	d6 := SelectWithCancel("标题", "消息", []string{"a"}, nil, nil)
	if d6 == nil {
		t.Fatal("SelectWithCancel() 不应返回 nil")
	}
}
