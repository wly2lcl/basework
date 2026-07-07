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
