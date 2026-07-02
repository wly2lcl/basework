package agent

import "context"

// Plugin 是 agent 插件接口，支持在 agent 生命周期中执行初始化和清理
type Plugin interface {
	// Name 返回插件名称
	Name() string
	// Initialize 在 agent 启动时调用
	Initialize(ctx context.Context, agent Agent) error
	// Shutdown 在 agent 关闭时调用
	Shutdown(ctx context.Context) error
}