package agent

import (
	"context"
	"fmt"
	"testing"

	"github.com/wly2lcl/basework/pkg/llm"
)

type promptCaptureModel struct {
	requests []*llm.Request
}

func (m *promptCaptureModel) ID() string { return "prompt-capture" }

func (m *promptCaptureModel) Supports(llm.Capability) bool { return true }

func (m *promptCaptureModel) Generate(context.Context, *llm.Request) (*llm.Response, error) {
	return nil, fmt.Errorf("Generate should not be called")
}

func (m *promptCaptureModel) Stream(_ context.Context, req *llm.Request) (<-chan llm.StreamEvent, error) {
	m.requests = append(m.requests, req)
	ch := make(chan llm.StreamEvent, 2)
	ch <- llm.StreamEvent{Type: llm.StreamEventText, Delta: "ok"}
	ch <- llm.StreamEvent{Type: llm.StreamEventDone}
	close(ch)
	return ch, nil
}

func TestSystemPromptProviderRefreshesBeforeEachRequest(t *testing.T) {
	model := &promptCaptureModel{}
	turn := 0
	agt, err := New(
		WithModel(model),
		WithSystemPromptProvider(func() string {
			turn++
			return fmt.Sprintf("dynamic-%d", turn)
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = agt.Close() })

	for i := 1; i <= 2; i++ {
		if _, err := agt.HandleMessage(context.Background(), "hello"); err != nil {
			t.Fatalf("turn %d: %v", i, err)
		}
	}
	if len(model.requests) != 2 {
		t.Fatalf("expected two requests, got %d", len(model.requests))
	}
	for i, req := range model.requests {
		if len(req.Messages) == 0 || req.Messages[0].Role != llm.RoleSystem {
			t.Fatalf("request %d missing system prompt: %#v", i+1, req.Messages)
		}
		want := fmt.Sprintf("dynamic-%d", i+1)
		if got := req.Messages[0].Content[0].Text; got != want {
			t.Fatalf("request %d system prompt = %q, want %q", i+1, got, want)
		}
	}
}
