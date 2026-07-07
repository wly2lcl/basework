// Package dialog 测试
package dialog

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestSelectDialogNoItems 测试空项目列表
func TestSelectDialogNoItems(t *testing.T) {
	d := NewSelect("选择", "消息", []string{}, nil, nil)

	// Enter 不应 panic
	_, cmd := d.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter, Text: "enter"}))
	_ = cmd
}

// TestSelectDialogViewMultiple 测试多项目渲染
func TestSelectDialogViewWithItems(t *testing.T) {
	items := make([]string, 20)
	for i := range items {
		items[i] = "项目"
	}
	d := NewSelect("选择", "消息", items, nil, nil)
	view := d.View(80)
	if view == "" {
		t.Fatal("View 不应为空")
	}
}

// TestInputDialogBackspaceDelete 测试输入框的退格和删除
func TestInputDialogBackspaceDelete(t *testing.T) {
	d := NewInput("标题", "提示", "hello", nil, nil)

	// 先移到末尾
	d.cursor = len(d.value)

	// 退格两次
	d.Update(tea.KeyPressMsg(tea.Key{Text: "backspace"}))
	d.Update(tea.KeyPressMsg(tea.Key{Text: "backspace"}))
	if d.value != "hel" {
		t.Fatalf("两次退格后期望 'hel'，得到 %s", d.value)
	}

	// 删除（光标在末尾，删除无效）
	d.Update(tea.KeyPressMsg(tea.Key{Text: "delete"}))
	if d.value != "hel" {
		t.Fatalf("末尾删除不应改变内容，得到 %s", d.value)
	}

	// 移到中间删除
	d.cursor = 1
	d.Update(tea.KeyPressMsg(tea.Key{Text: "delete"}))
	if d.value != "hl" {
		t.Fatalf("删除光标后字符后期望 'hl'，得到 %s", d.value)
	}
}

// TestInputDialogCursor 测试光标导航
func TestInputDialogCursor(t *testing.T) {
	d := NewInput("标题", "提示", "hello", nil, nil)
	d.cursor = 2

	// 左移
	d.Update(tea.KeyPressMsg(tea.Key{Text: "left"}))
	if d.cursor != 1 {
		t.Fatalf("左移后期望 cursor=1，得到 %d", d.cursor)
	}

	// 右移
	d.Update(tea.KeyPressMsg(tea.Key{Text: "right"}))
	if d.cursor != 2 {
		t.Fatalf("右移后期望 cursor=2，得到 %d", d.cursor)
	}

	// Home
	d.Update(tea.KeyPressMsg(tea.Key{Text: "home"}))
	if d.cursor != 0 {
		t.Fatalf("Home 后期望 cursor=0，得到 %d", d.cursor)
	}

	// End
	d.Update(tea.KeyPressMsg(tea.Key{Text: "end"}))
	if d.cursor != 5 {
		t.Fatalf("End 后期望 cursor=5，得到 %d", d.cursor)
	}
}

// TestInputDialogInsert 测试插入字符
func TestInputDialogInsert(t *testing.T) {
	d := NewInput("标题", "提示", "helo", nil, nil)
	d.cursor = 3

	d.Update(tea.KeyPressMsg(tea.Key{Text: "l"}))
	if d.value != "hello" {
		t.Fatalf("插入后期望 'hello'，得到 %s", d.value)
	}
}

// TestConfirmDialogRightEdge 测试右导航边界
func TestConfirmDialogRightEdge(t *testing.T) {
	d := NewConfirm("标题", "消息", nil, nil)
	d.selected = 1

	// 再右移不应越界
	d.Update(tea.KeyPressMsg(tea.Key{Text: "right"}))
	if d.selected != 1 {
		t.Fatalf("右边界时应保持 selected=1，得到 %d", d.selected)
	}
}
