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
	plugins         []Plugin
	observer        Observer
	callback        Callback
	maxContextTokens int

	// 集成模块接口
	compactor       Compactor
	permChecker     PermissionChecker
	loopDetector    LoopDetector
	subAgentRunner  SubAgentRunner
	eventBus        EventPublisher
	obsEnabled      bool

	// toolFactory 注册内置工具（允许内建 wiring）
	toolFactory ToolFactory

	// steeringManager 转向管理器
	steeringManager *SteeringManager
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

// WithCompactor 设置上下文压缩引擎
func WithCompactor(c Compactor) Option {
	return func(cfg *config) { cfg.compactor = c }
}

// WithPermissionChecker 设置权限检查器
func WithPermissionChecker(pc PermissionChecker) Option {
	return func(cfg *config) { cfg.permChecker = pc }
}

// WithLoopDetector 设置循环检测器
func WithLoopDetector(d LoopDetector) Option {
	return func(cfg *config) { cfg.loopDetector = d }
}

// WithSubAgentRunner 设置子代理运行器
func WithSubAgentRunner(sr SubAgentRunner) Option {
	return func(cfg *config) { cfg.subAgentRunner = sr }
}

// WithEventBus 设置可观测性事件总线
func WithEventBus(eb EventPublisher) Option {
	return func(cfg *config) { cfg.eventBus = eb; cfg.obsEnabled = eb != nil }
}

// WithToolFactory 设置工具工厂，用于注册内置工具和自定义工具
func WithToolFactory(f ToolFactory) Option {
	return func(cfg *config) { cfg.toolFactory = f }
}

// WithSteeringManager 设置转向管理器，用于在对话中注入系统消息
func WithSteeringManager(mgr *SteeringManager) Option {
	return func(cfg *config) { cfg.steeringManager = mgr }
}
