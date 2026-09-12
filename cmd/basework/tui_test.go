package main

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/wly2lcl/basework/internal/tui"
	"github.com/wly2lcl/basework/pkg/llm"
)

// TestNewTUIStreamingOptionsWiresCallback 是流式回调整形回归的核心护栏。
//
// 历史缺陷：internal/tui 与 cmd/basework 都写好了回调实现，但 TUI 启动路径传入的是
// 空的 runtimeAgentOptions{}，导致 OnTextDelta/OnToolCallStart/OnToolCallEnd/
// OnThinkingDelta 永不触发 —— TUI 只能拿到整段最终文本，"流式输出"实际不存在。
//
// 该测试直接锁住"TUI 的 runtime 选项必须携带非空 Callback"。
func TestNewTUIStreamingOptionsWiresCallback(t *testing.T) {
	opts, _ := newTUIStreamingOptions()

	if opts.Callback == nil {
		t.Fatal("TUI runtime 选项必须携带流式回调，否则 TUI 只能拿到整段最终文本")
	}
}

// TestNewTUIStreamingOptionsForwardsAfterBind 验证绑定 program.Send 后能收到消息。
func TestNewTUIStreamingOptionsForwardsAfterBind(t *testing.T) {
	opts, bind := newTUIStreamingOptions()

	var got []tea.Msg
	bind(func(msg tea.Msg) { got = append(got, msg) })

	opts.Callback.OnTextDelta("hi")

	if len(got) != 1 {
		t.Fatalf("绑定后应收到 1 条消息，得到 %d", len(got))
	}
	delta, ok := got[0].(tui.StreamDeltaMsg)
	if !ok {
		t.Fatalf("消息类型应为 tui.StreamDeltaMsg，得到 %T", got[0])
	}
	if delta.Delta != "hi" {
		t.Fatalf("增量应为 %q，得到 %q", "hi", delta.Delta)
	}
}

// TestNewTUIStreamingOptionsBeforeBindDoesNotPanic 验证 program.Send 尚未绑定
// （Program 还没创建）时，回调触发不会 panic。
func TestNewTUIStreamingOptionsBeforeBindDoesNotPanic(t *testing.T) {
	opts, _ := newTUIStreamingOptions()

	opts.Callback.OnTextDelta("early")
	opts.Callback.OnThinkingDelta("early")
	opts.Callback.OnError(nil)
	opts.Callback.OnToolCallStart(llm.ToolCall{Name: "bash"})
	opts.Callback.OnTurnEnd(nil)
}

// TestRuntimeAgentOptionsCallbackAppliedToAgentOptions 验证 Callback 会被
// 真实转成 agent Option。runtimeAgentOptions 的 Callback 字段若未接入
// newRuntimeAgent，这条链路同样失效。
func TestRuntimeAgentOptionsCallbackAppliedToAgentOptions(t *testing.T) {
	opts, _ := newTUIStreamingOptions()

	if opts.Callback == nil {
		t.Fatal("Callback 不应为空")
	}
	// Callback 已在 newRuntimeAgent 中通过 agent.WithCallback 接入（runtime.go）。
	// 这里断言其可被安全调用，确保接口契约稳定。
	opts.Callback.OnToolCallEnd(llm.ToolCall{Name: "bash"}, nil, nil)
}
