package agent

import (
	"context"

	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/tool"
)

// Compactor 上下文压缩接口
type Compactor interface {
	ShouldCompact(history []llm.ChatMessage, maxTokens int) bool
	Compact(history []llm.ChatMessage) ([]llm.ChatMessage, error)
}

// CompactReport 是一次压缩的完整结果。
type CompactReport struct {
	// Messages 是压缩后的消息列表。
	Messages []llm.ChatMessage
	// Summary 是被压缩掉的旧消息的摘要文本；为空表示本次压缩未产出摘要
	// （例如滑动窗口策略直接丢弃旧消息）。
	Summary string
}

// CompactReporter 是 Compactor 的**可选**扩展：除压缩结果外还报告摘要。
//
// 之所以用可选接口而不是直接给 Compactor 加方法：Go 的接口是结构化满足的，
// 实现方多提供一个方法即可升级，未实现者仍按旧行为工作（压缩事件里不带摘要，
// 与历史行为一致）。这样已经在用自定义 Compactor 的嵌入方不会被破坏。
//
// 配套约定：Agent 在压缩后会把 Summary 持久化进 Compacted 事件。这是必须的——
// 发给模型的请求由事件日志投影而来，只把摘要留在内存里等于没生成。
type CompactReporter interface {
	CompactWithReport(history []llm.ChatMessage) (CompactReport, error)
}

// LoopDetector 循环检测接口
type LoopDetector interface {
	Check(messages []llm.ChatMessage, toolCalls []LoopToolCall) (*LoopDetectResult, error)
}

// LoopToolCall 供循环检测使用的工具调用记录
type LoopToolCall struct {
	ToolName string
	Args     map[string]interface{}
}

// LoopDetectResult 循环检测结果
type LoopDetectResult struct {
	IsLoop     bool
	Signal     string
	Confidence float64
	Details    string
}

// PermissionChecker 权限检查接口
type PermissionChecker interface {
	// Check 检查工具执行是否被允许
	Check(ctx context.Context, toolName string, args map[string]interface{}) (bool, error)
	// CheckPath 检查路径是否被允许
	CheckPath(path string) error
}

// EventPublisher 事件发布接口
type EventPublisher interface {
	PublishEvent(eventType string, data map[string]interface{})
}

// SubAgentRunner 子代理运行接口
type SubAgentRunner interface {
	// Run 执行子代理任务，返回结果字符串
	Run(ctx context.Context, task string) (string, error)
	// Enabled 返回子代理是否启用
	Enabled() bool
}

// ToolFactory 工具工厂，用于注册内置工具和自定义工具
type ToolFactory func(registry *tool.Registry)

// compile-time checks for interface compliance in internal packages
var _ EventPublisher = (*nopEventPublisher)(nil)

// nopEventPublisher 空操作事件发布器
type nopEventPublisher struct{}

func (nopEventPublisher) PublishEvent(string, map[string]interface{}) {}
