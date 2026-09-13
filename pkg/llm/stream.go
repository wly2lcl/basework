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

// ToolCallDelta 是一次工具调用在流式响应中的单个事件。
//
// ArgsJSON 的语义由 Complete 决定，这是所有 Provider 必须遵守的归一化约定
// （归一化发生在 Provider 层，消费方不负责猜测 Provider 的方言）：
//
//   - Complete=false：ArgsJSON 是"自上次同 Index 事件以来的参数增量片段"。
//     消费方按 Index 分组、按到达顺序拼接即可还原完整参数；不能发出已经发过的前缀，
//     否则按序拼接的消费方会重复拼接。
//   - Complete=true：ArgsJSON 是完整参数，是唯一权威值；消费方必须用它覆盖拼接结果。
//
// 同一个 Index 在一次响应内只应出现一次 Complete=true，事件顺序按 Index 升序可复现。
type ToolCallDelta struct {
	Index    int
	ID       string
	Name     string
	ArgsJSON string
	Complete bool // 该 tool call 是否完整
}
