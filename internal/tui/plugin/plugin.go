// Package plugin 提供 TUI 插件系统
package plugin

import tea "charm.land/bubbletea/v2"

// SlotType 定义插件插槽类型
type SlotType int

const (
	SlotTop     SlotType = iota // 状态栏上方
	SlotBottom                  // 输入框下方
	SlotSidebar                 // 侧边栏
)

// TUIPlugin 定义 TUI 插件接口
type TUIPlugin interface {
	// ID 返回插件唯一标识
	ID() string
	// Slot 返回插件渲染位置
	Slot() SlotType
	// Init 初始化插件
	Init() tea.Cmd
	// Update 处理消息
	Update(msg tea.Msg) (TUIPlugin, tea.Cmd)
	// View 渲染插件视图
	View(width, height int) string
}

// BasePlugin 提供插件基类
type BasePlugin struct {
	id   string
	slot SlotType
}

// NewBasePlugin 创建插件基类
func NewBasePlugin(id string, slot SlotType) BasePlugin {
	return BasePlugin{
		id:   id,
		slot: slot,
	}
}

// ID 返回插件标识
func (p *BasePlugin) ID() string {
	return p.id
}

// Slot 返回插件插槽
func (p *BasePlugin) Slot() SlotType {
	return p.slot
}

// Init 默认初始化
func (p *BasePlugin) Init() tea.Cmd {
	return nil
}

// Update 默认更新（不作任何处理）
func (p *BasePlugin) Update(msg tea.Msg) (TUIPlugin, tea.Cmd) {
	return p, nil
}

// View 默认视图（空字符串）
func (p *BasePlugin) View(width, height int) string {
	return ""
}
