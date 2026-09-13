// Package tui 测试
//
// 本文件锁住一个真实缺陷（SHIP-003 自动化 TUI 验收发现）：
// 输入框按**字节**过滤可打印字符——
//
//	if msg.String() != "" && len(msg.String()) == 1 {
//	    r := rune(msg.String()[0])
//	    if r >= 32 && r <= 126 { ... }
//	}
//
// 而 uv.Key.String() 对可打印键返回 Key.Text，并且**特意排除空格**
// （空格回落成 "space"）。于是：
//   - 中文（UTF-8 三字节）→ len != 1 → 静默丢弃；
//   - 空格 → String()=="space"（5 字节）→ 静默丢弃；
//   - 光标位置按字节推进 → 中文后面插入/退格会切出半个字符。
//
// 对一个中文产品这是致命的：用户敲「修复 calc.go」只会得到「calc.go」。
// 现有单测全用 ASCII 输入，所以一直没暴露。
package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

// typeRune 构造一次「用户键入该字符」的按键消息（与终端解码层一致：填 Text）。
func typeRune(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

// TestInputView_AcceptsCJK 中文必须能输入。
func TestInputView_AcceptsCJK(t *testing.T) {
	iv := NewInputView()
	for _, r := range "修复缺陷" {
		iv.Update(typeRune(r))
	}
	if got := iv.Text(); got != "修复缺陷" {
		t.Fatalf("Text() = %q，期望 %q（中文被丢弃）", got, "修复缺陷")
	}
}

// TestInputView_AcceptsSpace 空格必须能输入。
//
// 空格在 uv 里走 KeySpace：Key.Text 是 " "，但 Key.String() 返回 "space"。
func TestInputView_AcceptsSpace(t *testing.T) {
	iv := NewInputView()
	iv.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	if got := iv.Text(); got != " " {
		t.Fatalf("Text() = %q，期望单个空格（空格被丢弃）", got)
	}
}

// TestInputView_MixedCJKASCIIAndSpace 中英混排 + 空格（真实提示词的形态）。
func TestInputView_MixedCJKASCIIAndSpace(t *testing.T) {
	iv := NewInputView()
	const want = "请修复 calc.go 里 Add 的缺陷"
	for _, r := range want {
		iv.Update(typeRune(r))
	}
	if got := iv.Text(); got != want {
		t.Fatalf("Text() = %q，期望 %q", got, want)
	}
	if !utf8.ValidString(iv.Text()) {
		t.Fatal("输入内容不是合法 UTF-8")
	}
}

// TestInputView_CursorIsRuneBased 光标必须按 rune 走：中文行里左移/右移/退格
// 都不能切出非法 UTF-8。
func TestInputView_CursorIsRuneBased(t *testing.T) {
	iv := NewInputView()
	for _, r := range "中文ab" {
		iv.Update(typeRune(r))
	}
	// 光标在行尾（4 个 rune）。
	if got := iv.Text(); got != "中文ab" {
		t.Fatalf("初始 Text() = %q", got)
	}

	// 左移两格 → 停在 '文' 与 'a' 之间。
	iv.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	iv.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	iv.Update(typeRune('X'))
	if got := iv.Text(); got != "中文Xab" {
		t.Fatalf("中插入后 Text() = %q，期望 %q（光标不是 rune 索引）", got, "中文Xab")
	}
	if !utf8.ValidString(iv.Text()) {
		t.Fatal("插入后不是合法 UTF-8（按字节切片切坏了中文）")
	}

	// 退格删掉刚插入的 X（光标此时停在 '文' 与 'a' 之间）。
	iv.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if got := iv.Text(); got != "中文ab" {
		t.Fatalf("退格后 Text() = %q，期望 %q", got, "中文ab")
	}

	// 光标仍在 '文' 与 'a' 之间，再退格必须整体删掉一个汉字（3 字节）。
	iv.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if got := iv.Text(); got != "中ab" {
		t.Fatalf("删汉字后 Text() = %q，期望 %q（退格只删了 1 字节）", got, "中ab")
	}
	if !utf8.ValidString(iv.Text()) {
		t.Fatal("退格后不是合法 UTF-8")
	}

	// 反向：右移到行尾后追加，中文整体不动。
	iv.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	iv.Update(typeRune('!'))
	if got := iv.Text(); got != "中ab!" {
		t.Fatalf("End 后追加 Text() = %q，期望 %q", got, "中ab!")
	}
}

// TestInputView_PasteInsertsWholeText 一次按键可能带多个字符（粘贴/输入法上屏）。
func TestInputView_PasteInsertsWholeText(t *testing.T) {
	iv := NewInputView()
	iv.Update(tea.KeyPressMsg{Code: 'x', Text: "中文粘贴"})
	if got := iv.Text(); got != "中文粘贴" {
		t.Fatalf("Text() = %q，期望 %q（多字符输入被截断）", got, "中文粘贴")
	}
}

// TestInputView_ControlKeysNotInserted 组合键不得当成文本插进输入框。
func TestInputView_ControlKeysNotInserted(t *testing.T) {
	iv := NewInputView()
	iv.Update(typeRune('a'))
	// ctrl+c 不带 Text，只有 Code + ModCtrl：不能被当作 'c'。
	iv.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if got := iv.Text(); got != "a" {
		t.Fatalf("Text() = %q，期望 %q（组合键被当成文本插入）", got, "a")
	}
}

// TestApp_Update_CJKReachesInput 端到端一层：按键经 App.Update 落到输入框。
func TestApp_Update_CJKReachesInput(t *testing.T) {
	app := NewApp("m", "p", "s")
	app.Width, app.Height = 80, 24
	for _, r := range "跑测试 确认" {
		app.Update(typeRune(r))
	}
	if got := app.Input.Text(); got != "跑测试 确认" {
		t.Fatalf("Input.Text() = %q，期望 %q", got, "跑测试 确认")
	}
}

// TestInputView_TabCompletionKeepsCJKBeforeCursor 补全替换末尾词时，
// 光标前有中文也不能切错位置。
func TestInputView_TabCompletionKeepsCJKBeforeCursor(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "calc_go.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("准备补全目标失败: %v", err)
	}
	t.Chdir(dir)

	iv := NewInputView()
	for _, r := range "看 calc" {
		iv.Update(typeRune(r))
	}
	iv.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // 打开补全
	iv.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // 选中第一项
	got := iv.Text()
	if !strings.Contains(got, "calc_go.txt") {
		t.Fatalf("补全后 Text() = %q，期望包含 calc_go.txt", got)
	}
	if !strings.HasPrefix(got, "看 ") {
		t.Fatalf("补全把光标前的中文弄丢了：%q", got)
	}
	if !utf8.ValidString(got) {
		t.Fatal("补全后不是合法 UTF-8")
	}
}
