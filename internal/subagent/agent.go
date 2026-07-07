package subagent

import "context"

// AgentTaskRunner 是子代理协调器所需的 agent 最小接口
// 用于避免与 pkg/agent 的循环依赖
type AgentTaskRunner interface {
	// HandleMessage 处理单条文本输入，返回处理结果
	HandleMessage(ctx context.Context, input string) (*TaskResult, error)
	// Close 释放资源
	Close() error
}

// TaskResult 是 agent 处理任务的结果
type TaskResult struct {
	// Content 输出文本内容
	Content string `json:"content"`
	// InputTokens 输入 token 数
	InputTokens int `json:"input_tokens"`
	// OutputTokens 输出 token 数
	OutputTokens int `json:"output_tokens"`
	// TotalTokens 总 token 数
	TotalTokens int `json:"total_tokens"`
}
