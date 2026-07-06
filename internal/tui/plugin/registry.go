// Package plugin 提供 TUI 插件系统
package plugin

// 全局插件注册表
var plugins = make(map[string]TUIPlugin)

// Register 注册一个 TUI 插件
func Register(p TUIPlugin) {
	plugins[p.ID()] = p
}

// List 返回所有已注册的插件
func List() []TUIPlugin {
	result := make([]TUIPlugin, 0, len(plugins))
	for _, p := range plugins {
		result = append(result, p)
	}
	return result
}

// Get 根据 ID 获取插件
func Get(id string) (TUIPlugin, bool) {
	p, ok := plugins[id]
	return p, ok
}

// ListBySlot 返回指定插槽的所有插件
func ListBySlot(slot SlotType) []TUIPlugin {
	var result []TUIPlugin
	for _, p := range plugins {
		if p.Slot() == slot {
			result = append(result, p)
		}
	}
	return result
}

// Unregister 注销插件
func Unregister(id string) {
	delete(plugins, id)
}

// Clear 清空所有插件
func Clear() {
	plugins = make(map[string]TUIPlugin)
}

// Count 返回插件数量
func Count() int {
	return len(plugins)
}