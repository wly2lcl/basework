package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/wly2lcl/basework/pkg/llm"
)

// flushProbeModel 每次 Stream 都返回同一份脚本：三个工具调用按 2、0、1 的顺序
// 只发增量、不发 Complete，用于验证 Pipeline 在流结束时自行补齐时的顺序可复现。
type flushProbeModel struct{}

func (flushProbeModel) ID() string { return "flush-probe" }

func (flushProbeModel) Generate(context.Context, *llm.Request) (*llm.Response, error) {
	return nil, errors.New("Generate 不应被调用")
}

func (flushProbeModel) Stream(context.Context, *llm.Request) (<-chan llm.StreamEvent, error) {
	script := []llm.StreamEvent{
		{Type: llm.StreamEventText, Delta: ""}, // 空文本增量不应产生任何内容
		toolCallDelta(2, "call_c", "tool_c", `{"z":3}`),
		toolCallDelta(0, "call_a", "tool_a", `{"x":1}`),
		toolCallDelta(1, "call_b", "tool_b", `{"y":2}`),
	}
	ch := make(chan llm.StreamEvent, len(script))
	for _, evt := range script {
		ch <- evt
	}
	close(ch)
	return ch, nil
}

func (flushProbeModel) Supports(llm.Capability) bool { return true }

func toolCallDelta(index int, id, name, args string) llm.StreamEvent {
	return llm.StreamEvent{
		Type: llm.StreamEventToolCall,
		ToolCall: &llm.ToolCallDelta{
			Index:    index,
			ID:       id,
			Name:     name,
			ArgsJSON: args,
			Complete: false,
		},
	}
}

// TestPipeline_EndFlushIsOrderedByToolCallIndex 验证流结束时 Pipeline 自行补齐的
// 工具调用按 index 升序、多次运行结果一致（map 迭代顺序随机是此前的不确定来源）。
func TestPipeline_EndFlushIsOrderedByToolCallIndex(t *testing.T) {
	const rounds = 50
	wantIDs := []string{"call_a", "call_b", "call_c"}
	wantArgs := []string{`{"x":1}`, `{"y":2}`, `{"z":3}`}

	for round := 0; round < rounds; round++ {
		td := setupTestTurnD(t, flushProbeModel{})
		p := NewPipeline(td)

		resp, err := p.callLLM(context.Background(), []llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "go"}}},
		}, nil)
		if err != nil {
			t.Fatalf("第 %d 轮 callLLM 返回错误: %v", round, err)
		}

		if len(resp.Message.ToolCalls) != len(wantIDs) {
			t.Fatalf("第 %d 轮工具调用数量错误: 期望 %d, 实际 %d", round, len(wantIDs), len(resp.Message.ToolCalls))
		}
		for i := range wantIDs {
			if resp.Message.ToolCalls[i].ID != wantIDs[i] {
				t.Fatalf("第 %d 轮第 %d 个工具调用 ID 错误: 期望 %s, 实际 %s",
					round, i, wantIDs[i], resp.Message.ToolCalls[i].ID)
			}
			if resp.Message.ToolCalls[i].ArgsJSON != wantArgs[i] {
				t.Fatalf("第 %d 轮 %s 的参数错误: 期望 %s, 实际 %s",
					round, wantIDs[i], wantArgs[i], resp.Message.ToolCalls[i].ArgsJSON)
			}
		}
	}
}
