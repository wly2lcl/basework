// Package dialog 提供对话框系统
package dialog

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"strings"
)

// 对话框样式
var (
	styleDialogBox = lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(lipgloss.Color("63")).
			Padding(1, 2).
			Background(lipgloss.Color("236"))

	styleDialogTitle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("63")).
				Bold(true)

	styleDialogMsg = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255"))

	styleDialogSelected = lipgloss.NewStyle().
				Foreground(lipgloss.Color("0")).
				Background(lipgloss.Color("63")).
				Padding(0, 2)

	styleDialogUnselected = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245")).
				Padding(0, 2)
)

// ConfirmDialog 确认对话框
type ConfirmDialog struct {
	baseDialog
	OnConfirm func()
	OnCancel  func()
	selected  int // 0=确认, 1=取消
}

// NewConfirm 创建确认对话框
func NewConfirm(title, message string, onConfirm func(), onCancel func()) *ConfirmDialog {
	return &ConfirmDialog{
		baseDialog: baseDialog{
			title:   title,
			message: message,
		},
		OnConfirm: onConfirm,
		OnCancel:  onCancel,
		selected:  0,
	}
}

// Update 处理按键消息
func (d *ConfirmDialog) Update(msg tea.Msg) (Dialog, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc":
			if d.OnCancel != nil {
				d.OnCancel()
			}
			return d, nil

		case "enter":
			if d.selected == 0 {
				if d.OnConfirm != nil {
					d.OnConfirm()
				}
			} else {
				if d.OnCancel != nil {
					d.OnCancel()
				}
			}
			return d, nil

		case "left":
			if d.selected > 0 {
				d.selected--
			}
			return d, nil

		case "right":
			if d.selected < 1 {
				d.selected++
			}
			return d, nil

		case "tab":
			d.selected = (d.selected + 1) % 2
			return d, nil
		}
	}
	return d, nil
}

// View 渲染确认对话框
func (d *ConfirmDialog) View(width int) string {
	var buf strings.Builder
	buf.WriteString(styleDialogTitle.Render(d.title))
	buf.WriteString("\n\n")
	buf.WriteString(styleDialogMsg.Render(d.message))
	buf.WriteString("\n\n")

	// 按钮
	confirmBtn := styleDialogUnselected.Render("  确认  ")
	cancelBtn := styleDialogUnselected.Render("  取消  ")
	if d.selected == 0 {
		confirmBtn = styleDialogSelected.Render("  确认  ")
	} else {
		cancelBtn = styleDialogSelected.Render("  取消  ")
	}
	buf.WriteString(confirmBtn)
	buf.WriteString("  ")
	buf.WriteString(cancelBtn)

	// 计算对话框宽度
	content := buf.String()
	dialogWidth := width - 8
	if dialogWidth < 40 {
		dialogWidth = 40
	}

	return styleDialogBox.Width(dialogWidth).Render(content)
}