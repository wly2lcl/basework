package llm

// StreamEventType 表示流式事件的类型
type StreamEventType string

const (
	StreamEventText     StreamEventType = "text"
	StreamEventToolCall StreamEventType = "tool_call"
	StreamEventUsage    StreamEventType = "usage"
	StreamEventDone     StreamEventType = "done"
)

// StreamEvent 是流式事件
type StreamEvent struct {
	Type     StreamEventType
	Delta    string         // 文本增量
	ToolCall *ToolCallDelta // 工具调用增量
	Usage    *Usage         // 用量（通常在最后）
	Error    error
}

// ToolCallDelta 工具调用增量
type ToolCallDelta struct {
	Index    int
	ID       string
	Name     string
	ArgsJSON string
	Complete bool // 该 tool call 是否完整
}
