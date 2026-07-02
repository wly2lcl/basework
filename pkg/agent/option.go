package agent

import (
	"github.com/wly2lcl/basework/pkg/hook"
	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/session"
	"github.com/wly2lcl/basework/pkg/tool"
)

// Option 是 agent 配置选项函数
type Option func(*config)

type config struct {
	model        llm.Model
	tools        []tool.Tool
	registry     *tool.Registry
	systemPrompt string
	session      session.Store
	hooks        []hook.Hook
	maxSteps     int
	plugins      []Plugin
	observer     Observer
	callback     Callback
}

// WithModel 设置 LLM 模型
func WithModel(m llm.Model) Option {
	return func(c *config) { c.model = m }
}

// WithTools 追加工具
func WithTools(t ...tool.Tool) Option {
	return func(c *config) { c.tools = append(c.tools, t...) }
}

// WithToolRegistry 设置工具注册表（覆盖 WithTools 注册的默认 registry）
func WithToolRegistry(r *tool.Registry) Option {
	return func(c *config) { c.registry = r }
}

// WithSystemPrompt 设置系统提示词
func WithSystemPrompt(prompt string) Option {
	return func(c *config) { c.systemPrompt = prompt }
}

// WithSession 设置会话存储
func WithSession(s session.Store) Option {
	return func(c *config) { c.session = s }
}

// WithHook 追加生命周期钩子
func WithHook(h ...hook.Hook) Option {
	return func(c *config) { c.hooks = append(c.hooks, h...) }
}

// WithMaxSteps 设置最大执行步数
func WithMaxSteps(n int) Option {
	return func(c *config) { c.maxSteps = n }
}

// WithPlugin 追加插件
func WithPlugin(p ...Plugin) Option {
	return func(c *config) { c.plugins = append(c.plugins, p...) }
}

// WithObserver 设置运行观测器
func WithObserver(o Observer) Option {
	return func(c *config) { c.observer = o }
}

// WithCallback 设置生命周期回调
func WithCallback(cb Callback) Option {
	return func(c *config) { c.callback = cb }
}
