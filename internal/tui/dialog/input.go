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
		cursor:   len(defaultValue),
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
			if d.cursor > 0 {
				d.value = d.value[:d.cursor-1] + d.value[d.cursor:]
				d.cursor--
			}
			return d, nil

		case "delete":
			if d.cursor < len(d.value) {
				d.value = d.value[:d.cursor] + d.value[d.cursor+1:]
			}
			return d, nil

		case "left":
			if d.cursor > 0 {
				d.cursor--
			}
			return d, nil

		case "right":
			if d.cursor < len(d.value) {
				d.cursor++
			}
			return d, nil

		case "home":
			d.cursor = 0
			return d, nil

		case "end":
			d.cursor = len(d.value)
			return d, nil

		default:
			if msg.String() != "" && len(msg.String()) == 1 {
				r := rune(msg.String()[0])
				if r >= 32 && r <= 126 {
					d.value = d.value[:d.cursor] + string(r) + d.value[d.cursor:]
					d.cursor++
				}
			}
			return d, nil
		}
	}
	return d, nil
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
