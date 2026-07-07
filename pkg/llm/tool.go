package llm

import "encoding/json"

// ToolCall 表示一次工具调用请求
type ToolCall struct {
	ID       string
	Name     string
	ArgsJSON string // 原始 JSON 参数
}

// ToolResult 表示工具执行结果
type ToolResult struct {
	ToolCallID string
	Content    string
	IsError    bool
}

// ToolDefinition 是给 LLM 的工具描述（不含执行逻辑）
type ToolDefinition struct {
	Name        string
	Description string
	Parameters  json.RawMessage // JSON Schema
}
