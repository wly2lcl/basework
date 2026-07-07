package session

import (
	"encoding/json"

	"github.com/wly2lcl/basework/pkg/llm"
)

// ProjectMessages 将事件序列投影为 LLM 消息列表。
//
// 投影规则：
//   - Prompted → user message（Content 为单个文本 ContentPart）
//   - TextDelta → 累积到当前 assistant message 的文本内容
//   - TextEnded → 完成当前 assistant message（后续 delta 将开始新消息）
//   - ToolCalled → 追加 ToolCall 到当前 assistant message
//   - ToolSuccess → tool message（Role: "tool", ToolCallID 匹配）
//   - ToolFailed → tool message（Role: "tool", ToolCallID 匹配, IsError: true）
//   - ToolCalled 后紧接着的 ToolSuccess/ToolFailed 匹配对应的 ToolCallID
//   - Compacted → 截断 TruncatedSeq 之前的事件，替换为 system message 摘要
func ProjectMessages(events []Event) []llm.ChatMessage {
	if len(events) == 0 {
		return nil
	}

	// 找到最近的 Compacted 事件的截断点
	var truncateBefore int64
	for _, e := range events {
		if e.Type == EventCompacted {
			var data CompactedData
			if err := json.Unmarshal(e.Data, &data); err == nil && data.TruncatedSeq > truncateBefore {
				truncateBefore = data.TruncatedSeq
			}
		}
	}

	var msgs []llm.ChatMessage
	var currentAssistant *llm.ChatMessage // 正在累积的 assistant 消息

	// 记录已匹配的 ToolCalled/ToolSuccess 对（ToolCallID → Name）
	pendingToolCalls := make(map[string]string)

	// 累积文本内容的辅助函数
	flushAssistant := func() {
		if currentAssistant != nil {
			msgs = append(msgs, *currentAssistant)
			currentAssistant = nil
		}
	}

	for _, event := range events {
		// Compacted 事件替换为 system message 摘要，并跳过 truncateBefore 之前的事件
		if event.Type == EventCompacted {
			var data CompactedData
			if err := json.Unmarshal(event.Data, &data); err != nil {
				continue
			}
			summary := data.Summary
			if summary == "" {
				summary = "对话历史已被压缩"
			}
			msgs = append(msgs, llm.ChatMessage{
				Role:    llm.RoleSystem,
				Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: summary}},
			})
			truncateBefore = data.TruncatedSeq
			continue
		}
		// 跳过被截断的事件
		if truncateBefore > 0 && event.Seq > 0 && event.Seq <= truncateBefore {
			continue
		}

		switch event.Type {
		case EventPrompted:
			flushAssistant()
			var data PromptedData
			if err := json.Unmarshal(event.Data, &data); err != nil {
				continue
			}
			msgs = append(msgs, llm.ChatMessage{
				Role:    llm.RoleUser,
				Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: data.Content}},
			})

		case EventTextDelta:
			if currentAssistant == nil {
				currentAssistant = &llm.ChatMessage{
					Role:    llm.RoleAssistant,
					Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: ""}},
				}
			}
			var data TextDeltaData
			if err := json.Unmarshal(event.Data, &data); err != nil {
				continue
			}
			if len(currentAssistant.Content) > 0 && currentAssistant.Content[0].Type == llm.ContentTypeText {
				currentAssistant.Content[0].Text += data.Delta
			} else {
				currentAssistant.Content = append(currentAssistant.Content, llm.ContentPart{Type: llm.ContentTypeText, Text: data.Delta})
			}

		case EventTextEnded:
			if currentAssistant == nil {
				currentAssistant = &llm.ChatMessage{
					Role:    llm.RoleAssistant,
					Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: ""}},
				}
			}
			// TextEnded 无数据负载，关闭当前 assistant 消息
			flushAssistant()

		case EventToolCalled:
			var data ToolCalledData
			if err := json.Unmarshal(event.Data, &data); err != nil {
				continue
			}
			if currentAssistant == nil {
				currentAssistant = &llm.ChatMessage{
					Role:    llm.RoleAssistant,
					Content: []llm.ContentPart{},
				}
			}
			currentAssistant.ToolCalls = append(currentAssistant.ToolCalls, data.ToolCall)
			pendingToolCalls[data.ToolCall.ID] = data.ToolCall.Name

		case EventToolSuccess:
			flushAssistant()
			var data ToolSuccessData
			if err := json.Unmarshal(event.Data, &data); err != nil {
				continue
			}
			msg := llm.ChatMessage{
				Role:       llm.RoleTool,
				ToolCallID: data.ToolCallID,
				Content:    []llm.ContentPart{{Type: llm.ContentTypeText, Text: data.Content}},
			}
			// 设置 Name 为对应 tool call 的名称（如果有匹配的 pendingToolCall）
			if name, ok := pendingToolCalls[data.ToolCallID]; ok {
				msg.Name = name
			}
			delete(pendingToolCalls, data.ToolCallID)
			msgs = append(msgs, msg)

		case EventToolFailed:
			flushAssistant()
			var data ToolFailedData
			if err := json.Unmarshal(event.Data, &data); err != nil {
				continue
			}
			msg := llm.ChatMessage{
				Role:       llm.RoleTool,
				ToolCallID: data.ToolCallID,
				Content:    []llm.ContentPart{{Type: llm.ContentTypeText, Text: data.Error}},
			}
			if name, ok := pendingToolCalls[data.ToolCallID]; ok {
				msg.Name = name
			}
			delete(pendingToolCalls, data.ToolCallID)
			msgs = append(msgs, msg)

		case EventTurnStarted, EventTurnEnded, EventTurnFailed, EventAgentSwitched:
			flushAssistant()
			// 这些事件不直接投影为消息，仅用于辅助状态管理
		}
	}

	// 刷新最后的 assistant 消息
	flushAssistant()

	return msgs
}