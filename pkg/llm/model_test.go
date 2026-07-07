package llm

import (
	"context"
	"testing"
)

// mockModel 是用于测试 Model 接口的模拟实现
type mockModel struct {
	id       string
	caps     map[Capability]bool
	genResp  *Response
	genErr   error
	streamCh chan StreamEvent
}

func (m *mockModel) ID() string { return m.id }

func (m *mockModel) Generate(_ context.Context, _ *Request) (*Response, error) {
	return m.genResp, m.genErr
}

func (m *mockModel) Stream(_ context.Context, _ *Request) (<-chan StreamEvent, error) {
	return m.streamCh, nil
}

func (m *mockModel) Supports(cap Capability) bool {
	return m.caps[cap]
}

func TestModelInterface_ID(t *testing.T) {
	m := &mockModel{id: "gpt-4"}
	if m.ID() != "gpt-4" {
		t.Errorf("期望 ID=gpt-4, 得到 %s", m.ID())
	}
}

func TestModelInterface_Generate(t *testing.T) {
	expected := &Response{
		Message: ChatMessage{
			Role: RoleAssistant,
			Content: []ContentPart{
				{Type: ContentTypeText, Text: "你好"},
			},
		},
		Usage: Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	}
	m := &mockModel{genResp: expected}
	resp, err := m.Generate(context.Background(), &Request{})
	if err != nil {
		t.Fatalf("Generate 返回错误: %v", err)
	}
	if resp.Message.Role != RoleAssistant {
		t.Errorf("期望 Role=%q", RoleAssistant)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("期望 TotalTokens=15, 得到 %d", resp.Usage.TotalTokens)
	}
}

func TestModelInterface_Stream(t *testing.T) {
	ch := make(chan StreamEvent, 2)
	ch <- StreamEvent{Type: StreamEventText, Delta: "He"}
	ch <- StreamEvent{Type: StreamEventDone}
	close(ch)

	m := &mockModel{streamCh: ch}
	got, err := m.Stream(context.Background(), &Request{})
	if err != nil {
		t.Fatalf("Stream 返回错误: %v", err)
	}
	var events []StreamEvent
	for ev := range got {
		events = append(events, ev)
	}
	if len(events) != 2 {
		t.Fatalf("期望 2 个事件, 得到 %d", len(events))
	}
	if events[0].Type != StreamEventText || events[0].Delta != "He" {
		t.Errorf("第一个事件不匹配: %+v", events[0])
	}
	if events[1].Type != StreamEventDone {
		t.Errorf("第二个事件期望 done, 得到 %s", events[1].Type)
	}
}

func TestModelInterface_Supports(t *testing.T) {
	m := &mockModel{
		caps: map[Capability]bool{
			CapTools:     true,
			CapStreaming: true,
			CapVision:    false,
			CapJSON:      false,
		},
	}
	tests := []struct {
		cap  Capability
		want bool
	}{
		{CapTools, true},
		{CapStreaming, true},
		{CapVision, false},
		{CapJSON, false},
	}
	for _, tt := range tests {
		got := m.Supports(tt.cap)
		if got != tt.want {
			t.Errorf("Supports(%q) = %v, 期望 %v", tt.cap, got, tt.want)
		}
	}
}
