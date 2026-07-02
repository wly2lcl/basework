package session

import (
	"encoding/json"
	"testing"

	"github.com/wly2lcl/basework/pkg/llm"
)

func TestProjectMessages_Empty(t *testing.T) {
	msgs := ProjectMessages(nil)
	if msgs != nil {
		t.Fatalf("期望 nil，得到 %v", msgs)
	}

	msgs = ProjectMessages([]Event{})
	if msgs != nil {
		t.Fatalf("期望 nil，得到 %v", msgs)
	}
}

func TestProjectMessages_SingleTurn(t *testing.T) {
	events := []Event{
		{Seq: 1, Type: EventPrompted, Data: rawJSON(t, PromptedData{Content: "你好"})},
		{Seq: 2, Type: EventTextDelta, Data: rawJSON(t, TextDeltaData{Delta: "你好"})},
		{Seq: 3, Type: EventTextDelta, Data: rawJSON(t, TextDeltaData{Delta: "，世界"})},
		{Seq: 4, Type: EventTextEnded},
	}

	msgs := ProjectMessages(events)
	if len(msgs) != 2 {
		t.Fatalf("期望 2 条消息，得到 %d", len(msgs))
	}

	// 用户消息
	if msgs[0].Role != llm.RoleUser {
		t.Errorf("msgs[0].Role = %q, 期望 %q", msgs[0].Role, llm.RoleUser)
	}
	if len(msgs[0].Content) != 1 {
		t.Fatalf("msgs[0].Content 长度 = %d, 期望 1", len(msgs[0].Content))
	}
	if msgs[0].Content[0].Text != "你好" {
		t.Errorf("msgs[0].Content[0].Text = %q, 期望 %q", msgs[0].Content[0].Text, "你好")
	}

	// 助手消息
	if msgs[1].Role != llm.RoleAssistant {
		t.Errorf("msgs[1].Role = %q, 期望 %q", msgs[1].Role, llm.RoleAssistant)
	}
	if len(msgs[1].Content) != 1 {
		t.Fatalf("msgs[1].Content 长度 = %d, 期望 1", len(msgs[1].Content))
	}
	if msgs[1].Content[0].Text != "你好，世界" {
		t.Errorf("msgs[1].Content[0].Text = %q, 期望 %q", msgs[1].Content[0].Text, "你好，世界")
	}
}

func TestProjectMessages_ToolCall(t *testing.T) {
	events := []Event{
		{Seq: 1, Type: EventPrompted, Data: rawJSON(t, PromptedData{Content: "查天气"})},
		{Seq: 2, Type: EventToolCalled, Data: rawJSON(t, ToolCalledData{
			ToolCall: llm.ToolCall{ID: "call_1", Name: "get_weather", ArgsJSON: `{"city":"北京"}`},
		})},
		{Seq: 3, Type: EventToolSuccess, Data: rawJSON(t, ToolSuccessData{ToolCallID: "call_1", Content: "晴，25°C"})},
		{Seq: 4, Type: EventTextDelta, Data: rawJSON(t, TextDeltaData{Delta: "北京今天"})},
		{Seq: 5, Type: EventTextDelta, Data: rawJSON(t, TextDeltaData{Delta: "天气晴朗"})},
		{Seq: 6, Type: EventTextEnded},
	}

	msgs := ProjectMessages(events)
	if len(msgs) != 4 {
		t.Fatalf("期望 4 条消息，得到 %d: %+v", len(msgs), msgs)
	}

	// 用户消息
	if msgs[0].Role != llm.RoleUser {
		t.Errorf("msgs[0].Role = %q, 期望 %q", msgs[0].Role, llm.RoleUser)
	}

	// assistant 消息（含 tool call）
	if msgs[1].Role != llm.RoleAssistant {
		t.Errorf("msgs[1].Role = %q, 期望 %q", msgs[1].Role, llm.RoleAssistant)
	}
	if len(msgs[1].ToolCalls) != 1 {
		t.Fatalf("msgs[1].ToolCalls 长度 = %d, 期望 1", len(msgs[1].ToolCalls))
	}
	if msgs[1].ToolCalls[0].ID != "call_1" {
		t.Errorf("ToolCall.ID = %q, 期望 %q", msgs[1].ToolCalls[0].ID, "call_1")
	}

	// tool 消息
	if msgs[2].Role != llm.RoleTool {
		t.Errorf("msgs[2].Role = %q, 期望 %q", msgs[2].Role, llm.RoleTool)
	}
	if msgs[2].ToolCallID != "call_1" {
		t.Errorf("msgs[2].ToolCallID = %q, 期望 %q", msgs[2].ToolCallID, "call_1")
	}
	if len(msgs[2].Content) > 0 && msgs[2].Content[0].Text != "晴，25°C" {
		t.Errorf("tool 消息文本 = %q, 期望 %q", msgs[2].Content[0].Text, "晴，25°C")
	}

	// 第二条助理消息
	if msgs[3].Role != llm.RoleAssistant {
		t.Errorf("msgs[3].Role = %q, 期望 %q", msgs[3].Role, llm.RoleAssistant)
	}
	if len(msgs[3].Content) > 0 && msgs[3].Content[0].Text != "北京今天天气晴朗" {
		t.Errorf("msgs[3].Content[0].Text = %q, 期望 %q", msgs[3].Content[0].Text, "北京今天天气晴朗")
	}
}

func TestProjectMessages_ToolCallFailed(t *testing.T) {
	events := []Event{
		{Seq: 1, Type: EventToolCalled, Data: rawJSON(t, ToolCalledData{
			ToolCall: llm.ToolCall{ID: "call_1", Name: "get_data"},
		})},
		{Seq: 2, Type: EventToolFailed, Data: rawJSON(t, ToolFailedData{ToolCallID: "call_1", Error: "连接超时"})},
	}

	msgs := ProjectMessages(events)
	if len(msgs) != 2 {
		t.Fatalf("期望 2 条消息，得到 %d", len(msgs))
	}
	if msgs[0].Role != llm.RoleAssistant {
		t.Errorf("msgs[0].Role = %q, 期望 %q", msgs[0].Role, llm.RoleAssistant)
	}
	if msgs[1].Role != llm.RoleTool {
		t.Errorf("msgs[1].Role = %q, 期望 %q", msgs[1].Role, llm.RoleTool)
	}
}

func TestProjectMessages_MultipleTurns(t *testing.T) {
	events := []Event{
		{Seq: 1, Type: EventPrompted, Data: rawJSON(t, PromptedData{Content: "第一轮"})},
		{Seq: 2, Type: EventTextDelta, Data: rawJSON(t, TextDeltaData{Delta: "回复1"})},
		{Seq: 3, Type: EventTextEnded},
		{Seq: 4, Type: EventPrompted, Data: rawJSON(t, PromptedData{Content: "第二轮"})},
		{Seq: 5, Type: EventTextDelta, Data: rawJSON(t, TextDeltaData{Delta: "回复2"})},
		{Seq: 6, Type: EventTextEnded},
	}

	msgs := ProjectMessages(events)
	if len(msgs) != 4 {
		t.Fatalf("期望 4 条消息，得到 %d", len(msgs))
	}
	if msgs[0].Role != llm.RoleUser || msgs[1].Role != llm.RoleAssistant {
		t.Errorf("前两条消息角色不对: %q, %q", msgs[0].Role, msgs[1].Role)
	}
	if msgs[2].Role != llm.RoleUser || msgs[3].Role != llm.RoleAssistant {
		t.Errorf("后两条消息角色不对: %q, %q", msgs[2].Role, msgs[3].Role)
	}
}

func TestProjectMessages_TurnEventsSkipped(t *testing.T) {
	events := []Event{
		{Seq: 1, Type: EventTurnStarted, Data: rawJSON(t, TurnStartedData{Step: 1})},
		{Seq: 2, Type: EventPrompted, Data: rawJSON(t, PromptedData{Content: "hi"})},
		{Seq: 3, Type: EventTextDelta, Data: rawJSON(t, TextDeltaData{Delta: "hello"})},
		{Seq: 4, Type: EventTextEnded},
		{Seq: 5, Type: EventTurnEnded, Data: rawJSON(t, TurnEndedData{Usage: llm.Usage{TotalTokens: 10}})},
	}

	msgs := ProjectMessages(events)
	// TurnStarted/TurnEnded 不应该产生消息
	if len(msgs) != 2 {
		t.Fatalf("期望 2 条消息（跳过 turn 事件），得到 %d", len(msgs))
	}
}

func TestProjectMessages_Compacted(t *testing.T) {
	events := []Event{
		{Seq: 1, Type: EventPrompted, Data: rawJSON(t, PromptedData{Content: "旧消息1"})},
		{Seq: 2, Type: EventTextDelta, Data: rawJSON(t, TextDeltaData{Delta: "旧回复1"})},
		{Seq: 3, Type: EventTextEnded},
		{Seq: 4, Type: EventCompacted, Data: rawJSON(t, CompactedData{Summary: "历史摘要：用户问好", TruncatedSeq: 3})},
		{Seq: 5, Type: EventPrompted, Data: rawJSON(t, PromptedData{Content: "新消息"})},
		{Seq: 6, Type: EventTextDelta, Data: rawJSON(t, TextDeltaData{Delta: "新回复"})},
		{Seq: 7, Type: EventTextEnded},
	}

	msgs := ProjectMessages(events)
	// 预期：system（摘要）+ user（新消息）+ assistant（新回复）
	if len(msgs) != 3 {
		t.Fatalf("期望 3 条消息，得到 %d: %+v", len(msgs), msgs)
	}

	if msgs[0].Role != llm.RoleSystem {
		t.Errorf("msgs[0].Role = %q, 期望 %q", msgs[0].Role, llm.RoleSystem)
	}
	if msgs[0].Content[0].Text != "历史摘要：用户问好" {
		t.Errorf("msgs[0].Content[0].Text = %q, 期望 %q", msgs[0].Content[0].Text, "历史摘要：用户问好")
	}

	if msgs[1].Role != llm.RoleUser {
		t.Errorf("msgs[1].Role = %q, 期望 %q", msgs[1].Role, llm.RoleUser)
	}
	if msgs[1].Content[0].Text != "新消息" {
		t.Errorf("msgs[1].Content[0].Text = %q, 期望 %q", msgs[1].Content[0].Text, "新消息")
	}

	if msgs[2].Role != llm.RoleAssistant {
		t.Errorf("msgs[2].Role = %q, 期望 %q", msgs[2].Role, llm.RoleAssistant)
	}
}

func TestProjectMessages_MultipleToolCalls(t *testing.T) {
	events := []Event{
		{Seq: 1, Type: EventPrompted, Data: rawJSON(t, PromptedData{Content: "多个工具"})},
		{Seq: 2, Type: EventToolCalled, Data: rawJSON(t, ToolCalledData{
			ToolCall: llm.ToolCall{ID: "call_a", Name: "tool_a"},
		})},
		{Seq: 3, Type: EventToolCalled, Data: rawJSON(t, ToolCalledData{
			ToolCall: llm.ToolCall{ID: "call_b", Name: "tool_b"},
		})},
		{Seq: 4, Type: EventToolSuccess, Data: rawJSON(t, ToolSuccessData{ToolCallID: "call_a", Content: "结果A"})},
		{Seq: 5, Type: EventToolFailed, Data: rawJSON(t, ToolFailedData{ToolCallID: "call_b", Error: "失败B"})},
		{Seq: 6, Type: EventTextDelta, Data: rawJSON(t, TextDeltaData{Delta: "完成"})},
		{Seq: 7, Type: EventTextEnded},
	}

	msgs := ProjectMessages(events)
	// 预期：user + assistant(2 tool calls) + tool(success) + tool(failed) + assistant(result)
	if len(msgs) != 5 {
		t.Fatalf("期望 5 条消息，得到 %d", len(msgs))
	}

	// assistant 消息应该在 index 1
	if msgs[1].Role != llm.RoleAssistant {
		t.Errorf("msgs[1].Role = %q, 期望 %q", msgs[1].Role, llm.RoleAssistant)
	}
	if len(msgs[1].ToolCalls) != 2 {
		t.Fatalf("msgs[1].ToolCalls 长度 = %d, 期望 2", len(msgs[1].ToolCalls))
	}

	// tool success at index 2
	if msgs[2].Role != llm.RoleTool || msgs[2].ToolCallID != "call_a" {
		t.Errorf("msgs[2] 应为 tool/call_a, 得到 role=%q id=%q", msgs[2].Role, msgs[2].ToolCallID)
	}

	// tool failed at index 3
	if msgs[3].Role != llm.RoleTool || msgs[3].ToolCallID != "call_b" {
		t.Errorf("msgs[3] 应为 tool/call_b, 得到 role=%q id=%q", msgs[3].Role, msgs[3].ToolCallID)
	}
}

func TestProjectMessages_InvalidData(t *testing.T) {
	// Data 为空或解析失败时，投影应跳过该事件而不崩溃
	events := []Event{
		{Seq: 1, Type: EventPrompted, Data: json.RawMessage(`invalid json`)},
		{Seq: 2, Type: EventTextDelta, Data: json.RawMessage(`{"delta":"ok"}`)},
		{Seq: 3, Type: EventTextEnded},
	}

	msgs := ProjectMessages(events)
	// 无效的 Prompted 事件被跳过，只剩下 assistant 消息
	if len(msgs) != 1 {
		t.Fatalf("期望 1 条消息（无效事件被跳过），得到 %d", len(msgs))
	}
	if msgs[0].Role != llm.RoleAssistant {
		t.Errorf("msgs[0].Role = %q, 期望 %q", msgs[0].Role, llm.RoleAssistant)
	}
}

func TestProjectMessages_DeltaWithoutEnd(t *testing.T) {
	events := []Event{
		{Seq: 1, Type: EventPrompted, Data: rawJSON(t, PromptedData{Content: "hi"})},
		{Seq: 2, Type: EventTextDelta, Data: rawJSON(t, TextDeltaData{Delta: "part1"})},
		{Seq: 3, Type: EventTextDelta, Data: rawJSON(t, TextDeltaData{Delta: "part2"})},
		// 没有 TextEnded，但后面有 Prompted，应自动 flush
		{Seq: 4, Type: EventPrompted, Data: rawJSON(t, PromptedData{Content: "next"})},
	}

	msgs := ProjectMessages(events)
	if len(msgs) != 3 {
		t.Fatalf("期望 3 条消息，得到 %d", len(msgs))
	}
	if msgs[1].Role != llm.RoleAssistant {
		t.Errorf("msgs[1].Role = %q, 期望 %q", msgs[1].Role, llm.RoleAssistant)
	}
	if msgs[1].Content[0].Text != "part1part2" {
		t.Errorf("累积文本 = %q, 期望 %q", msgs[1].Content[0].Text, "part1part2")
	}
}