// Package dialog 测试
//
// 输入对话框与主输入框有一份同源缺陷（同一段按字节过滤的代码）：
// 中文与空格被静默丢弃，退格按字节删中文。这里锁住修复后的行为。
package dialog

import (
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

func typeRune(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

// TestInputDialog_AcceptsCJKAndSpace 中文与空格都要能输入。
func TestInputDialog_AcceptsCJKAndSpace(t *testing.T) {
	d := NewInput("标题", "提示", "", nil, nil)
	const want = "计划 名称"
	for _, r := range want {
		d.Update(typeRune(r))
	}
	if got := d.Value(); got != want {
		t.Fatalf("Value() = %q，期望 %q（中文/空格被丢弃）", got, want)
	}
	if !utf8.ValidString(d.Value()) {
		t.Fatal("输入内容不是合法 UTF-8")
	}
}

// TestInputDialog_DefaultValueCursorIsRuneBased 默认值含中文时，
// 光标必须停在 rune 末尾，退格删掉一个完整汉字。
func TestInputDialog_DefaultValueCursorIsRuneBased(t *testing.T) {
	d := NewInput("标题", "提示", "中文", nil, nil)
	d.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if got := d.Value(); got != "中" {
		t.Fatalf("退格后 Value() = %q，期望 %q（退格只删了 1 字节）", got, "中")
	}
	if !utf8.ValidString(d.Value()) {
		t.Fatal("退格后不是合法 UTF-8")
	}
}

// TestInputDialog_PasteInsertsWholeText 一次按键带多个字符。
func TestInputDialog_PasteInsertsWholeText(t *testing.T) {
	d := NewInput("标题", "提示", "", nil, nil)
	d.Update(tea.KeyPressMsg{Code: 'x', Text: "粘贴内容"})
	if got := d.Value(); got != "粘贴内容" {
		t.Fatalf("Value() = %q，期望 %q", got, "粘贴内容")
	}
}

// TestInputDialog_ControlKeysNotInserted 组合键不得被当成文本插入。
func TestInputDialog_ControlKeysNotInserted(t *testing.T) {
	d := NewInput("标题", "提示", "", nil, nil)
	d.Update(typeRune('a'))
	d.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if got := d.Value(); got != "a" {
		t.Fatalf("Value() = %q，期望 %q（组合键被当成文本插入）", got, "a")
	}
}
