package provider

import (
	"fmt"
	"sync"

	"github.com/wly2lcl/basework/pkg/llm"
)

// ProviderPlugin 插件 Provider 接口
// 实现者需要提供 llm.Model 的全部能力
type ProviderPlugin interface {
	// ID 返回插件唯一标识
	ID() string
	// Create 根据配置创建 Model 实例
	Create(cfg Config) (llm.Model, error)
}

// PluginRegistry 插件注册表
type PluginRegistry struct {
	mu      sync.RWMutex
	plugins map[string]ProviderPlugin
}

var globalPluginRegistry = &PluginRegistry{
	plugins: make(map[string]ProviderPlugin),
}

// RegisterPlugin 注册 Provider 插件
func RegisterPlugin(plugin ProviderPlugin) error {
	globalPluginRegistry.mu.Lock()
	defer globalPluginRegistry.mu.Unlock()
	if _, exists := globalPluginRegistry.plugins[plugin.ID()]; exists {
		return fmt.Errorf("provider plugin %q already registered", plugin.ID())
	}
	globalPluginRegistry.plugins[plugin.ID()] = plugin
	return nil
}

// GetPlugin 获取已注册的插件
func GetPlugin(id string) (ProviderPlugin, bool) {
	globalPluginRegistry.mu.RLock()
	defer globalPluginRegistry.mu.RUnlock()
	plugin, ok := globalPluginRegistry.plugins[id]
	return plugin, ok
}

// ListPlugins 列出所有已注册插件名称
func ListPlugins() []string {
	globalPluginRegistry.mu.RLock()
	defer globalPluginRegistry.mu.RUnlock()
	names := make([]string, 0, len(globalPluginRegistry.plugins))
	for id := range globalPluginRegistry.plugins {
		names = append(names, id)
	}
	return names
}

// CreateFromPlugin 使用插件创建 Provider
func CreateFromPlugin(pluginID string, cfg Config) (llm.Model, error) {
	plugin, ok := GetPlugin(pluginID)
	if !ok {
		return nil, fmt.Errorf("provider plugin %q not found", pluginID)
	}
	return plugin.Create(cfg)
}
