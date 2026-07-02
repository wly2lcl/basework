// Package session 提供事件溯源会话层，用于记录和管理 LLM 对话事件。
package session

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/wly2lcl/basework/pkg/llm"
)

// EventType 表示事件的类型。
type EventType string

const (
	// 用户输入事件
	EventPrompted EventType = "prompted"
	// 文本流式输出开始
	EventTextStarted EventType = "text.started"
	// 文本流式输出增量
	EventTextDelta EventType = "text.delta"
	// 文本流式输出结束
	EventTextEnded EventType = "text.ended"
	// 工具被调用
	EventToolCalled EventType = "tool.called"
	// 工具执行成功
	EventToolSuccess EventType = "tool.success"
	// 工具执行失败
	EventToolFailed EventType = "tool.failed"
	// 对话轮次开始
	EventTurnStarted EventType = "turn.started"
	// 对话轮次结束
	EventTurnEnded EventType = "turn.ended"
	// 对话轮次失败
	EventTurnFailed EventType = "turn.failed"
	// 事件压缩
	EventCompacted EventType = "compacted"
	// Agent 切换
	EventAgentSwitched EventType = "agent.switched"
)

// Event 表示一个溯源事件。
type Event struct {
	ID        string          `json:"id"`
	SessionID string          `json:"session_id"`
	Type      EventType       `json:"type"`
	Data      json.RawMessage `json:"data"`
	Seq       int64           `json:"seq"`
	CreatedAt time.Time       `json:"created_at"`
}

// PromptedData 表示用户提示数据。
type PromptedData struct {
	Content string `json:"content"`
}

// TextDeltaData 表示文本增量数据。
type TextDeltaData struct {
	Delta string `json:"delta"`
}

// ToolCalledData 表示工具调用数据。
type ToolCalledData struct {
	ToolCall llm.ToolCall `json:"tool_call"`
}

// ToolSuccessData 表示工具执行成功数据。
type ToolSuccessData struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
}

// ToolFailedData 表示工具执行失败数据。
type ToolFailedData struct {
	ToolCallID string `json:"tool_call_id"`
	Error      string `json:"error"`
}

// TurnStartedData 表示轮次开始数据。
type TurnStartedData struct {
	Step int `json:"step"`
}

// TurnEndedData 表示轮次结束数据（含 token 用量）。
type TurnEndedData struct {
	Usage llm.Usage `json:"usage"`
}

// CompactedData 表示事件压缩数据。
type CompactedData struct {
	Summary      string `json:"summary"`
	TruncatedSeq int64  `json:"truncated_seq"`
}

// EncodeData 将事件数据编码为 json.RawMessage。
// 支持的 data 类型：*PromptedData, *TextDeltaData, *ToolCalledData, *ToolSuccessData,
// *ToolFailedData, *TurnStartedData, *TurnEndedData, *CompactedData。
func EncodeData(data interface{}) (json.RawMessage, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("session: 编码事件数据失败: %w", err)
	}
	return json.RawMessage(b), nil
}

// DecodeData 根据 Event.Type 解码 Event.Data 为对应的结构化数据。
func DecodeData(event Event) (interface{}, error) {
	if len(event.Data) == 0 {
		return nil, nil
	}
	switch event.Type {
	case EventPrompted:
		var d PromptedData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return nil, fmt.Errorf("session: 解码 PromptedData 失败: %w", err)
		}
		return &d, nil
	case EventTextDelta:
		var d TextDeltaData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return nil, fmt.Errorf("session: 解码 TextDeltaData 失败: %w", err)
		}
		return &d, nil
	case EventToolCalled:
		var d ToolCalledData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return nil, fmt.Errorf("session: 解码 ToolCalledData 失败: %w", err)
		}
		return &d, nil
	case EventToolSuccess:
		var d ToolSuccessData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return nil, fmt.Errorf("session: 解码 ToolSuccessData 失败: %w", err)
		}
		return &d, nil
	case EventToolFailed:
		var d ToolFailedData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return nil, fmt.Errorf("session: 解码 ToolFailedData 失败: %w", err)
		}
		return &d, nil
	case EventTurnStarted:
		var d TurnStartedData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return nil, fmt.Errorf("session: 解码 TurnStartedData 失败: %w", err)
		}
		return &d, nil
	case EventTurnEnded:
		var d TurnEndedData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return nil, fmt.Errorf("session: 解码 TurnEndedData 失败: %w", err)
		}
		return &d, nil
	case EventCompacted:
		var d CompactedData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return nil, fmt.Errorf("session: 解码 CompactedData 失败: %w", err)
		}
		return &d, nil
	default:
		return nil, fmt.Errorf("session: 不支持的事件类型: %s", event.Type)
	}
}

// EventFilter 用于过滤事件查询。
type EventFilter struct {
	SessionID string      `json:"session_id,omitempty"`
	Types     []EventType `json:"types,omitempty"`
	AfterSeq  int64       `json:"after_seq,omitempty"`
	Limit     int         `json:"limit,omitempty"`
}
