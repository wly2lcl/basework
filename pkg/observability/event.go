package observability

import "time"

// 事件类型常量
const (
	EventAgentStart = "agent.start" // Agent 开始执行
	EventAgentEnd   = "agent.end"   // Agent 执行结束
	EventAgentError = "agent.error" // Agent 执行出错

	EventToolStart = "tool.start" // 工具开始执行
	EventToolEnd   = "tool.end"   // 工具执行结束
	EventToolError = "tool.error" // 工具执行出错

	EventLLMCallStart = "llm.call.start" // LLM 调用开始
	EventLLMCallEnd   = "llm.call.end"   // LLM 调用结束

	EventToolTimeout = "tool.timeout" // 工具执行超时
)

// Event 是可观测性事件的通用结构
type Event struct {
	// Type 事件类型
	Type string `json:"type"`
	// Timestamp 事件发生时间
	Timestamp time.Time `json:"timestamp"`
	// Data 事件附带的数据
	Data map[string]interface{} `json:"data"`
}

// NewEvent 创建事件
func NewEvent(eventType string, data map[string]interface{}) Event {
	return Event{
		Type:      eventType,
		Timestamp: time.Now().UTC(),
		Data:      data,
	}
}
