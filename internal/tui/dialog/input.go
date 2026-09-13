// Package dialog 提供对话框系统
package dialog

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"strings"
)

// 输入对话框样式
var (
	styleInputLabel = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Bold(true)

	styleInputField = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("63")).
			Padding(0, 1)

	styleInputValue = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255"))
)

// InputDialog 输入对话框
type InputDialog struct {
	baseDialog
	value    string
	cursor   int
	onSubmit func(string)
	onCancel func()
}

// NewInput 创建输入对话框
func NewInput(title, message string, defaultValue string, onSubmit func(string), onCancel func()) *InputDialog {
	return &InputDialog{
		baseDialog: baseDialog{
			title:   title,
			message: message,
		},
		value:    defaultValue,
		cursor:   len([]rune(defaultValue)),
		onSubmit: onSubmit,
		onCancel: onCancel,
	}
}

// Value 返回当前输入值
func (d *InputDialog) Value() string {
	return d.value
}

// Update 处理按键消息
func (d *InputDialog) Update(msg tea.Msg) (Dialog, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc":
			if d.onCancel != nil {
				d.onCancel()
			}
			return d, nil

		case "enter":
			if d.onSubmit != nil {
				d.onSubmit(d.value)
			}
			return d, nil

		case "backspace":
			d.value, d.cursor = deleteRuneBefore(d.value, d.cursor)
			return d, nil

		case "delete":
			d.value = deleteRuneAt(d.value, d.cursor)
			return d, nil

		case "left":
			if d.cursor > 0 {
				d.cursor--
			}
			return d, nil

		case "right":
			if d.cursor < len([]rune(d.value)) {
				d.cursor++
			}
			return d, nil

		case "home":
			d.cursor = 0
			return d, nil

		case "end":
			d.cursor = len([]rune(d.value))
			return d, nil

		default:
			// 与主输入框同源缺陷：按字节长度过滤会静默丢弃中文与空格。
			// 详见 internal/tui/input.go 的 printableKeyText 注释。
			if text := printableKeyText(msg); text != "" {
				runes := []rune(d.value)
				pos := clampRunePos(d.cursor, len(runes))
				ins := []rune(text)
				out := make([]rune, 0, len(runes)+len(ins))
				out = append(out, runes[:pos]...)
				out = append(out, ins...)
				out = append(out, runes[pos:]...)
				d.value = string(out)
				d.cursor = pos + len(ins)
			}
			return d, nil
		}
	}
	return d, nil
}

// printableKeyText 取出一次按键所代表的文本；不是文本输入时返回 ""。
func printableKeyText(msg tea.KeyPressMsg) string {
	k := msg.Key()
	if k.Text != "" {
		return k.Text
	}
	if k.Code >= 32 && k.Code != 127 && msg.Keystroke() == string(k.Code) {
		return string(k.Code)
	}
	return ""
}

// clampRunePos 把 rune 索引夹到 [0, n]。
func clampRunePos(pos, n int) int {
	if pos < 0 {
		return 0
	}
	if pos > n {
		return n
	}
	return pos
}

// deleteRuneBefore 删除 rune 索引 pos 之前的字符，返回新串与新索引。
func deleteRuneBefore(s string, pos int) (string, int) {
	runes := []rune(s)
	if pos <= 0 || len(runes) == 0 {
		return s, clampRunePos(pos, len(runes))
	}
	p := clampRunePos(pos, len(runes))
	if p == 0 {
		return s, 0
	}
	out := make([]rune, 0, len(runes)-1)
	out = append(out, runes[:p-1]...)
	out = append(out, runes[p:]...)
	return string(out), p - 1
}

// deleteRuneAt 删除 rune 索引 pos 处的字符。
func deleteRuneAt(s string, pos int) string {
	runes := []rune(s)
	p := clampRunePos(pos, len(runes))
	if p >= len(runes) {
		return s
	}
	out := make([]rune, 0, len(runes)-1)
	out = append(out, runes[:p]...)
	out = append(out, runes[p+1:]...)
	return string(out)
}

// View 渲染输入对话框
func (d *InputDialog) View(width int) string {
	var buf strings.Builder
	buf.WriteString(styleDialogTitle.Render(d.title))
	buf.WriteString("\n\n")

	if d.message != "" {
		buf.WriteString(styleDialogMsg.Render(d.message))
		buf.WriteString("\n\n")
	}

	buf.WriteString(styleInputLabel.Render("输入:"))
	buf.WriteString("\n")

	// 输入字段
	inputWidth := width - 16
	if inputWidth < 20 {
		inputWidth = 20
	}
	inputContent := d.value
	if inputContent == "" {
		inputContent = " "
	}
	buf.WriteString(styleInputField.Width(inputWidth).Render(inputContent))

	buf.WriteString("\n\n")
	buf.WriteString(styleDialogHint.Render("Enter 确认  Esc 取消"))

	content := buf.String()
	dialogWidth := width - 8
	if dialogWidth < 40 {
		dialogWidth = 40
	}

	return styleDialogBox.Width(dialogWidth).Render(content)
}
