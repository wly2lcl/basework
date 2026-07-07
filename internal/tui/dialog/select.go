// Package dialog 提供对话框系统
package dialog

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"strings"
)

// 选择对话框样式
var (
	styleSelectItem = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Padding(0, 1)

	styleSelectSelected = lipgloss.NewStyle().
				Foreground(lipgloss.Color("0")).
				Background(lipgloss.Color("63")).
				Padding(0, 1)

	styleDialogHint = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			Italic(true)
)

// SelectDialog 选择对话框
type SelectDialog struct {
	baseDialog
	items    []string
	selected int
	onSelect func(string)
	onCancel func()
}

// NewSelect 创建选择对话框
func NewSelect(title, message string, items []string, onSelect func(string), onCancel func()) *SelectDialog {
	return &SelectDialog{
		baseDialog: baseDialog{
			title:   title,
			message: message,
		},
		items:    items,
		selected: 0,
		onSelect: onSelect,
		onCancel: onCancel,
	}
}

// Update 处理按键消息
func (d *SelectDialog) Update(msg tea.Msg) (Dialog, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc":
			if d.onCancel != nil {
				d.onCancel()
			}
			return d, nil

		case "enter":
			if d.selected >= 0 && d.selected < len(d.items) {
				if d.onSelect != nil {
					d.onSelect(d.items[d.selected])
				}
			}
			return d, nil

		case "up":
			if d.selected > 0 {
				d.selected--
			}
			return d, nil

		case "down":
			if d.selected < len(d.items)-1 {
				d.selected++
			}
			return d, nil

		case "home":
			d.selected = 0
			return d, nil

		case "end":
			d.selected = len(d.items) - 1
			return d, nil
		}
	}
	return d, nil
}

// View 渲染选择对话框
func (d *SelectDialog) View(width int) string {
	var buf strings.Builder
	buf.WriteString(styleDialogTitle.Render(d.title))
	buf.WriteString("\n\n")

	if d.message != "" {
		buf.WriteString(styleDialogMsg.Render(d.message))
		buf.WriteString("\n\n")
	}

	// 显示项目列表
	maxItems := 10
	displayItems := d.items
	start := 0

	if len(displayItems) > maxItems {
		start = d.selected - maxItems/2
		if start < 0 {
			start = 0
		}
		if start+maxItems > len(displayItems) {
			start = len(displayItems) - maxItems
		}
		displayItems = displayItems[start : start+maxItems]
	}

	for i, item := range displayItems {
		actualIdx := start + i
		if actualIdx == d.selected {
			buf.WriteString(styleSelectSelected.Render("→ " + item))
		} else {
			buf.WriteString(styleSelectItem.Render("  " + item))
		}
		buf.WriteString("\n")
	}

	buf.WriteString(styleDialogHint.Render("↑↓ 导航 Enter 选择 Esc 取消"))

	content := buf.String()
	dialogWidth := width - 8
	if dialogWidth < 40 {
		dialogWidth = 40
	}

	return styleDialogBox.Width(dialogWidth).Render(content)
}
