package subagent

import "github.com/google/uuid"

// AgentType 表示子代理的类型
type AgentType string

const (
	// TypeGeneral 通用子代理，拥有完整工具集
	TypeGeneral AgentType = "general"
	// TypeReadonly 只读子代理，仅拥有只读工具集
	TypeReadonly AgentType = "readonly"
)

// Task 表示一个子代理任务
type Task struct {
	// ID 任务唯一标识
	ID string `json:"id"`
	// Description 任务描述
	Description string `json:"description"`
	// AgentType 子代理类型
	AgentType AgentType `json:"agent_type"`
	// Context 上下文信息，传递给子代理
	Context map[string]interface{} `json:"context,omitempty"`
}

// NewTask 创建新的子代理任务
func NewTask(description string, agentType AgentType, context map[string]interface{}) *Task {
	return &Task{
		ID:          uuid.New().String(),
		Description: description,
		AgentType:   agentType,
		Context:     context,
	}
}

// Result 表示子代理执行结果
type Result struct {
	// ID 对应 Task.ID
	ID string `json:"id"`
	// Success 是否执行成功
	Success bool `json:"success"`
	// Output 输出内容
	Output string `json:"output"`
	// Error 错误信息（当 Success 为 false 时）
	Error string `json:"error,omitempty"`
	// TokenUsage token 使用量
	TokenUsage TokenUsage `json:"token_usage"`
	// Cost 执行成本
	Cost float64 `json:"cost"`
}

// TokenUsage 记录 token 使用量
type TokenUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}