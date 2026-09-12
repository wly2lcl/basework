package session

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/wly2lcl/basework/pkg/llm"
)

func TestEventJSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	events := []struct {
		name  string
		event Event
	}{
		{
			name: "PromptedData",
			event: Event{
				ID: "evt-1", SessionID: "sess-1", Type: EventPrompted, Seq: 1, CreatedAt: now,
				Data: rawJSON(t, PromptedData{Content: "Hello"}),
			},
		},
		{
			name: "TextDeltaData",
			event: Event{
				ID: "evt-2", SessionID: "sess-1", Type: EventTextDelta, Seq: 2, CreatedAt: now,
				Data: rawJSON(t, TextDeltaData{Delta: " world"}),
			},
		},
		{
			name: "ToolCalledData",
			event: Event{
				ID: "evt-3", SessionID: "sess-1", Type: EventToolCalled, Seq: 3, CreatedAt: now,
				Data: rawJSON(t, ToolCalledData{ToolCall: llm.ToolCall{ID: "tc1", Name: "get_weather", ArgsJSON: `{"city":"beijing"}`}}),
			},
		},
		{
			name: "ToolSuccessData",
			event: Event{
				ID: "evt-4", SessionID: "sess-1", Type: EventToolSuccess, Seq: 4, CreatedAt: now,
				Data: rawJSON(t, ToolSuccessData{ToolCallID: "tc1", Content: "sunny"}),
			},
		},
		{
			name: "ToolFailedData",
			event: Event{
				ID: "evt-5", SessionID: "sess-1", Type: EventToolFailed, Seq: 5, CreatedAt: now,
				Data: rawJSON(t, ToolFailedData{ToolCallID: "tc1", Error: "timeout"}),
			},
		},
		{
			name: "TurnStartedData",
			event: Event{
				ID: "evt-6", SessionID: "sess-1", Type: EventTurnStarted, Seq: 6, CreatedAt: now,
				Data: rawJSON(t, TurnStartedData{Step: 1}),
			},
		},
		{
			name: "TurnEndedData",
			event: Event{
				ID: "evt-7", SessionID: "sess-1", Type: EventTurnEnded, Seq: 7, CreatedAt: now,
				Data: rawJSON(t, TurnEndedData{Usage: llm.Usage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 30}}),
			},
		},
		{
			name: "CompactedData",
			event: Event{
				ID: "evt-8", SessionID: "sess-1", Type: EventCompacted, Seq: 8, CreatedAt: now,
				Data: rawJSON(t, CompactedData{Summary: "summarized", TruncatedSeq: 5}),
			},
		},
	}

	for _, tc := range events {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(tc.event)
			if err != nil {
				t.Fatalf("JSON 序列化失败: %v", err)
			}
			var got Event
			if err := json.Unmarshal(b, &got); err != nil {
				t.Fatalf("JSON 反序列化失败: %v", err)
			}
			if got.ID != tc.event.ID || got.Type != tc.event.Type || got.Seq != tc.event.Seq {
				t.Errorf("字段不匹配: got %+v, want %+v", got, tc.event)
			}
		})
	}
}

func TestEncodeDecodeData(t *testing.T) {
	tests := []struct {
		name string
		typ  EventType
		data interface{}
	}{
		{"Prompted", EventPrompted, &PromptedData{Content: "hi"}},
		{"TextDelta", EventTextDelta, &TextDeltaData{Delta: " hello"}},
		{"ToolCalled", EventToolCalled, &ToolCalledData{ToolCall: llm.ToolCall{ID: "tc1", Name: "fn", ArgsJSON: "{}"}}},
		{"ToolSuccess", EventToolSuccess, &ToolSuccessData{ToolCallID: "tc1", Content: "ok"}},
		{"ToolFailed", EventToolFailed, &ToolFailedData{ToolCallID: "tc1", Error: "err"}},
		{"TurnStarted", EventTurnStarted, &TurnStartedData{Step: 1}},
		{"TurnEnded", EventTurnEnded, &TurnEndedData{Usage: llm.Usage{PromptTokens: 1}}},
		{"Compacted", EventCompacted, &CompactedData{Summary: "s", TruncatedSeq: 3}},
		{"SystemPromptSet", EventSystemPromptSet, &SystemPromptSetData{Content: "你是助手", Hash: "abc"}},
		{"Steered", EventSteered, &SteeredData{Messages: []string{"用中文", "简短些"}}},
		{"RequestBuilt", EventRequestBuilt, &RequestBuiltData{
			MsgCount:  3,
			ToolCount: 2,
			Hash:      "deadbeef",
			Sources: []RequestSource{
				{Kind: "system_prompt", Count: 1, Seq: 1, Hash: "abc"},
				{Kind: "history", Count: 2},
			},
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := EncodeData(tc.data)
			if err != nil {
				t.Fatalf("EncodeData 失败: %v", err)
			}
			event := Event{Type: tc.typ, Data: raw}
			got, err := DecodeData(event)
			if err != nil {
				t.Fatalf("DecodeData 失败: %v", err)
			}
			if got == nil {
				t.Fatal("DecodeData 返回 nil")
			}
		})
	}
}

func TestDecodeData_UnknownType(t *testing.T) {
	raw := rawJSON(t, PromptedData{Content: "x"})
	event := Event{Type: "unknown.type", Data: raw}
	_, err := DecodeData(event)
	if err == nil {
		t.Fatal("预期未知类型返回错误，但得到 nil")
	}
}

func TestDecodeData_EmptyData(t *testing.T) {
	event := Event{Type: EventPrompted}
	got, err := DecodeData(event)
	if err != nil {
		t.Fatalf("空 Data 解码失败: %v", err)
	}
	if got != nil {
		t.Fatalf("预期 nil，得到 %v", got)
	}
}

func TestEventFilterFields(t *testing.T) {
	f := EventFilter{
		SessionID: "sess-1",
		Types:     []EventType{EventPrompted, EventTextDelta},
		AfterSeq:  10,
		Limit:     5,
	}
	if f.SessionID != "sess-1" || len(f.Types) != 2 || f.AfterSeq != 10 || f.Limit != 5 {
		t.Errorf("EventFilter 字段不匹配: %+v", f)
	}
}

// rawJSON 辅助函数，将 v 编码为 json.RawMessage。
func rawJSON(t *testing.T, v interface{}) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("编码失败: %v", err)
	}
	return json.RawMessage(b)
}
