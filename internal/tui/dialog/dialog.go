// Package dialog 提供对话框系统
package dialog

import tea "charm.land/bubbletea/v2"

// Dialog 定义对话框接口
type Dialog interface {
	// Init 初始化对话框
	Init() tea.Cmd
	// Update 处理消息
	Update(msg tea.Msg) (Dialog, tea.Cmd)
	// View 渲染对话框视图（给定可用宽高）
	View(width int) string
	// IsModal 返回是否为模态对话框（模态时阻止下层交互）
	IsModal() bool
}

// baseDialog 对话框基类，提供通用功能
type baseDialog struct {
	title   string
	message string
	onClose func()
}

func (d *baseDialog) Init() tea.Cmd {
	return nil
}

func (d *baseDialog) IsModal() bool {
	return true
}
