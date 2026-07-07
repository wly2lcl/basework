// Package dialog 提供对话框系统
package dialog

import (
	tea "charm.land/bubbletea/v2"
	"strings"
)

// Manager 对话框堆栈管理器
type Manager struct {
	stack []Dialog
}

// NewManager 创建对话框管理器
func NewManager() *Manager {
	return &Manager{
		stack: make([]Dialog, 0),
	}
}

// Open 打开一个新对话框（压入栈顶）
func (m *Manager) Open(d Dialog) {
	m.stack = append(m.stack, d)
}

// Close 关闭栈顶对话框
func (m *Manager) Close() {
	if len(m.stack) > 0 {
		m.stack = m.stack[:len(m.stack)-1]
	}
}

// Top 返回栈顶对话框
func (m *Manager) Top() Dialog {
	if len(m.stack) == 0 {
		return nil
	}
	return m.stack[len(m.stack)-1]
}

// HasDialog 检查是否有活跃对话框
func (m *Manager) HasDialog() bool {
	return len(m.stack) > 0
}

// IsModal 检查当前对话框是否为模态
func (m *Manager) IsModal() bool {
	if d := m.Top(); d != nil {
		return d.IsModal()
	}
	return false
}

// Depth 返回对话框数量
func (m *Manager) Depth() int {
	return len(m.stack)
}

// Update 将消息发送给栈顶对话框
func (m *Manager) Update(msg tea.Msg) tea.Cmd {
	d := m.Top()
	if d == nil {
		return nil
	}

	updated, cmd := d.Update(msg)

	// 替换栈顶
	if len(m.stack) > 0 {
		m.stack[len(m.stack)-1] = updated
	}

	return cmd
}

// View 渲染所有对话框的叠加视图
// 非栈顶对话框通过半透明效果叠加
func (m *Manager) View(width, height int) string {
	if len(m.stack) == 0 {
		return ""
	}

	// 只渲染栈顶对话框
	top := m.Top()
	return top.View(width)
}

// Clear 关闭所有对话框
func (m *Manager) Clear() {
	m.stack = make([]Dialog, 0)
}

// CloseAll 关闭所有对话框直到指定深度
func (m *Manager) CloseAll() {
	m.Clear()
}

// CloseUntilDepth 关闭对话框直到栈大小降到指定深度
func (m *Manager) CloseUntilDepth(depth int) {
	if depth < 0 {
		depth = 0
	}
	if len(m.stack) > depth {
		m.stack = m.stack[:depth]
	}
}

// Strings 返回调试用的对话框列表
func (m *Manager) Strings() string {
	names := make([]string, 0, len(m.stack))
	for _, d := range m.stack {
		switch d.(type) {
		case *ConfirmDialog:
			names = append(names, "Confirm")
		case *InputDialog:
			names = append(names, "Input")
		case *SelectDialog:
			names = append(names, "Select")
		default:
			names = append(names, "Unknown")
		}
	}
	return strings.Join(names, " > ")
}
