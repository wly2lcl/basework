package session

import (
	"encoding/json"

	"github.com/wly2lcl/basework/pkg/llm"
)

// SummaryMessagePrefix 是压缩摘要插回历史时的固定前缀。
//
// 定义在这里而不是 internal/compaction，是为了让「内存中的压缩结果」与
// 「按事件日志投影出的历史」对摘要采用同一种标记方式。
const SummaryMessagePrefix = "之前的对话摘要："

// ProjectMessages 将事件序列投影为 LLM 消息列表。
// 投影规则见 ProjectMessagesWithSeq。
func ProjectMessages(events []Event) []llm.ChatMessage {
	msgs, _ := ProjectMessagesWithSeq(events)
	return msgs
}

// ProjectMessagesWithSeq 将事件序列投影为 LLM 消息列表，并返回每条消息的来源事件 Seq。
//
// 投影规则：
//   - Prompted → user message（Content 为单个文本 ContentPart）
//   - TextDelta → 累积到当前 assistant message 的文本内容
//   - TextEnded → 完成当前 assistant message（后续 delta 将开始新消息）
//   - ToolCalled → 追加 ToolCall 到当前 assistant message
//   - ToolSuccess → tool message（Role: "tool", ToolCallID 匹配）
//   - ToolFailed → tool message（Role: "tool", ToolCallID 匹配）
//   - Compacted → 截断历史，并把摘要插回列表头部
//   - SystemPromptSet / Steered / RequestBuilt → 不投影为消息（请求级上下文与审计记录，
//     由 agent.BuildRequestMessages 在组装请求时使用）
//
// 返回的 seqs[i] 是产出第 i 条消息的事件里 **Seq 最大者**（即「这条消息最后被哪条
// 事件更新」）。旧格式压缩摘要的 Seq 是截断边界 TruncatedSeq；精确快照中每条消息
// 的 Seq 是生成该快照的压缩事件 Seq。
// seqs 因此保证**非递减**，这是 compactKeepIndex 用「第一条大于锚点的消息」定位的前提。
//
// 它的用途是给压缩提供**稳定锚点**：
//
//	消息下标会随投影规则变化而漂移（新增一种事件类型就可能多出一条消息，
//	原来保留第 3 条的含义就变了），而事件 Seq 是写盘时就固定的。
//
// 用下标做锚点会导致「同一份日志在不同版本下投影出不同历史」，且无法察觉。
func ProjectMessagesWithSeq(events []Event) ([]llm.ChatMessage, []int64) {
	if len(events) == 0 {
		return nil, nil
	}

	var msgs []llm.ChatMessage
	var seqs []int64
	var currentAssistant *llm.ChatMessage // 正在累积的 assistant 消息
	var currentSeq int64                  // 当前 assistant 消息最后被哪条事件更新

	// 记录已匹配的 ToolCalled/ToolSuccess 对（ToolCallID → Name）
	pendingToolCalls := make(map[string]string)

	appendMsg := func(msg llm.ChatMessage, seq int64) {
		msgs = append(msgs, msg)
		seqs = append(seqs, seq)
	}

	// 累积文本内容的辅助函数
	flushAssistant := func() {
		if currentAssistant != nil {
			appendMsg(*currentAssistant, currentSeq)
			currentAssistant = nil
			currentSeq = 0
		}
	}

	for _, event := range events {
		switch event.Type {
		case EventCompacted:
			flushAssistant()
			var data CompactedData
			if err := json.Unmarshal(event.Data, &data); err != nil {
				continue
			}
			if data.Snapshot != nil {
				// 新格式压缩事件保存的是压缩器实际产出的完整消息序列。
				// 它可以保留任意消息并调整顺序，因此不能再尝试从旧历史
				// 用截断下标或 Seq 锚点近似重建。空快照同样有效，表示清空。
				msgs = append([]llm.ChatMessage(nil), data.Snapshot...)
				seqs = make([]int64, len(msgs))
				for i := range seqs {
					seqs[i] = event.Seq
				}
				continue
			}
			if keepFrom, ok := compactKeepIndex(data, msgs, seqs); ok {
				msgs = msgs[keepFrom:]
				seqs = seqs[keepFrom:]
			}
			// 把摘要插回列表头部。不插回的话，"压缩"就等于"静默丢消息"：
			// 模型看不到被丢弃的内容，也看不到任何替代说明。
			//
			// 摘要的来源 Seq 取 **截断边界**（TruncatedSeq）而不是压缩事件自身的
			// Seq：摘要代表的是被丢弃的那段历史，不是压缩这一刻。若用压缩事件的
			// Seq（它必然晚于其后保留的消息），seqs 会变成非单调序列，下一次压缩
			// 定位锚点时会错误命中这条摘要，于是把本该丢掉的整段历史又留下来。
			if data.Summary != "" {
				summaryMsg := llm.ChatMessage{
					Role:    llm.RoleUser,
					Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: SummaryMessagePrefix + data.Summary}},
				}
				msgs = append([]llm.ChatMessage{summaryMsg}, msgs...)
				seqs = append([]int64{data.TruncatedSeq}, seqs...)
			}

		case EventPrompted:
			flushAssistant()
			var data PromptedData
			if err := json.Unmarshal(event.Data, &data); err != nil {
				continue
			}
			appendMsg(llm.ChatMessage{
				Role:    llm.RoleUser,
				Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: data.Content}},
			}, event.Seq)

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
			currentSeq = event.Seq

		case EventTextEnded:
			if currentAssistant == nil {
				currentAssistant = &llm.ChatMessage{
					Role:    llm.RoleAssistant,
					Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: ""}},
				}
			}
			// TextEnded 无数据负载，关闭当前 assistant 消息
			currentSeq = event.Seq
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
			currentSeq = event.Seq
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
			appendMsg(msg, event.Seq)

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
			appendMsg(msg, event.Seq)

		case EventTurnStarted, EventTurnEnded, EventTurnFailed, EventAgentSwitched,
			EventSystemPromptSet, EventSteered, EventRequestBuilt:
			flushAssistant()
			// 这些事件不直接投影为消息，仅用于辅助状态管理。
			//
			// EventSystemPromptSet / EventSteered 是「请求级」上下文：它们会进入
			// 发给 provider 的请求，但不属于对话历史。把它们留在投影之外是有意的：
			//   - 消息列表保持非递减的 Seq（压缩锚点依赖这一点），若把 Session 级
			//     上下文插到列表头部，它的 Seq 必然大于其后所有历史，锚点定位会失效；
			//   - system 消息必须在列表最前，而压缩只截断尾部，两者叠加会让压缩
			//     有可能把 system prompt 一并丢掉。
			// 由 agent.BuildRequestMessages 负责把它们放到正确位置，请求因此仍是
			// 事件日志的纯函数（见该函数注释）。
		}
	}

	// 刷新最后的 assistant 消息
	flushAssistant()

	return msgs, seqs
}

// compactKeepIndex 决定压缩事件之后应保留的消息起点。
//
// 优先使用稳定锚点 TruncatedSeq（事件序号）：保留第一条来源 Seq 大于它的消息。
// 旧数据没有该字段，退回 KeepFrom（消息下标）——这条回退路径是 v0 兼容所必需。
//
// 另有一个防御分支：若 TruncatedSeq > 0 但所有消息的来源 Seq 都不大于它
// （例如事件未设 Seq，seqs 全为 0），稳定锚点无法定位，同样退回 KeepFrom。
// 宁可回到旧行为，也不能把历史清空。
func compactKeepIndex(data CompactedData, msgs []llm.ChatMessage, seqs []int64) (int, bool) {
	if data.TruncatedSeq > 0 {
		for i, s := range seqs {
			if s > data.TruncatedSeq {
				return i, true
			}
		}
	}
	if data.KeepFrom > 0 && data.KeepFrom < len(msgs) {
		return data.KeepFrom, true
	}
	return 0, false
}
