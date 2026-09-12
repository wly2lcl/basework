// Package tui 测试
package tui

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/wly2lcl/basework/pkg/agent"
	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/tool"
)

// collectCallback 创建一个回调并把投递的消息收集到切片中。
func collectCallback() (*[]tea.Msg, agent.Callback) {
	got := &[]tea.Msg{}
	cb := NewAgentCallback(func(msg tea.Msg) {
		*got = append(*got, msg)
	})
	return got, cb
}

// TestAgentCallback_ImplementsCallback 保证返回类型满足 agent.Callback。
func TestAgentCallback_ImplementsCallback(t *testing.T) {
	_, cb := collectCallback()
	if cb == nil {
		t.Fatal("NewAgentCallback 返回 nil")
	}
}

// TestAgentCallback_OnTextDeltaEmitsStreamDeltaMsg 验证文本增量被翻译为 StreamDeltaMsg。
func TestAgentCallback_OnTextDeltaEmitsStreamDeltaMsg(t *testing.T) {
	got, cb := collectCallback()

	cb.OnTextDelta("Hello ")
	cb.OnTextDelta("World")

	if len(*got) != 2 {
		t.Fatalf("期望 2 条消息，得到 %d", len(*got))
	}
	for i, want := range []string{"Hello ", "World"} {
		msg, ok := (*got)[i].(StreamDeltaMsg)
		if !ok {
			t.Fatalf("第 %d 条消息类型应为 StreamDeltaMsg，得到 %T", i, (*got)[i])
		}
		if msg.Delta != want {
			t.Fatalf("第 %d 条增量应为 %q，得到 %q", i, want, msg.Delta)
		}
	}
}

// TestAgentCallback_OnThinkingDeltaEmitsMsg 验证思考增量。
func TestAgentCallback_OnThinkingDeltaEmitsMsg(t *testing.T) {
	got, cb := collectCallback()

	cb.OnThinkingDelta("let me think")

	if len(*got) != 1 {
		t.Fatalf("期望 1 条消息，得到 %d", len(*got))
	}
	msg, ok := (*got)[0].(ThinkingDeltaMsg)
	if !ok {
		t.Fatalf("消息类型应为 ThinkingDeltaMsg，得到 %T", (*got)[0])
	}
	if msg.Delta != "let me think" {
		t.Fatalf("增量不符，得到 %q", msg.Delta)
	}
}

// TestAgentCallback_OnToolCallStartEmitsMsg 验证工具开始事件。
func TestAgentCallback_OnToolCallStartEmitsMsg(t *testing.T) {
	got, cb := collectCallback()

	cb.OnToolCallStart(llm.ToolCall{ID: "1", Name: "bash", ArgsJSON: `{"cmd":"ls"}`})

	if len(*got) != 1 {
		t.Fatalf("期望 1 条消息，得到 %d", len(*got))
	}
	msg, ok := (*got)[0].(ToolStartMsg)
	if !ok {
		t.Fatalf("消息类型应为 ToolStartMsg，得到 %T", (*got)[0])
	}
	if msg.Name != "bash" {
		t.Fatalf("工具名应为 bash，得到 %q", msg.Name)
	}
}

// TestAgentCallback_OnToolCallEndSuccess 验证工具成功结束事件。
func TestAgentCallback_OnToolCallEndSuccess(t *testing.T) {
	got, cb := collectCallback()

	cb.OnToolCallEnd(
		llm.ToolCall{Name: "read", ArgsJSON: `{"path":"a.go"}`},
		&tool.Result{Content: "file body"},
		nil,
	)

	if len(*got) != 1 {
		t.Fatalf("期望 1 条消息，得到 %d", len(*got))
	}
	msg, ok := (*got)[0].(ToolEndMsg)
	if !ok {
		t.Fatalf("消息类型应为 ToolEndMsg，得到 %T", (*got)[0])
	}
	if msg.Name != "read" || msg.Result != "file body" || msg.IsError {
		t.Fatalf("ToolEndMsg 内容不符: %+v", msg)
	}
}

// TestAgentCallback_OnToolCallEndError 验证工具失败结束事件。
func TestAgentCallback_OnToolCallEndError(t *testing.T) {
	got, cb := collectCallback()

	cb.OnToolCallEnd(llm.ToolCall{Name: "bash"}, nil, errors.New("boom"))

	if len(*got) != 1 {
		t.Fatalf("期望 1 条消息，得到 %d", len(*got))
	}
	msg, ok := (*got)[0].(ToolEndMsg)
	if !ok {
		t.Fatalf("消息类型应为 ToolEndMsg，得到 %T", (*got)[0])
	}
	if !msg.IsError || msg.Result != "boom" {
		t.Fatalf("错误分支不符: %+v", msg)
	}
}

// TestAgentCallback_OnToolCallEndNilResult 验证 result 为 nil 且无错误时不 panic。
func TestAgentCallback_OnToolCallEndNilResult(t *testing.T) {
	got, cb := collectCallback()

	cb.OnToolCallEnd(llm.ToolCall{Name: "noop"}, nil, nil)

	if len(*got) != 1 {
		t.Fatalf("期望 1 条消息，得到 %d", len(*got))
	}
	msg := (*got)[0].(ToolEndMsg)
	if msg.IsError || msg.Result != "" {
		t.Fatalf("nil result 应产出空结果且非错误: %+v", msg)
	}
}

// TestAgentCallback_OnTurnEndEmitsNothing 是本 bug 的核心护栏：
// OnTurnEnd 不得提交消息，否则会与 AgentResponseMsg 的提交重复。
func TestAgentCallback_OnTurnEndEmitsNothing(t *testing.T) {
	got, cb := collectCallback()

	cb.OnTurnEnd(&agent.Response{
		Message: llm.ChatMessage{
			Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "final"}},
		},
	})
	cb.OnTurnEnd(nil)

	if len(*got) != 0 {
		t.Fatalf("OnTurnEnd 不应投递任何消息，实际投递 %d 条", len(*got))
	}
}

// TestAgentCallback_OnErrorEmitsErrorMsg 验证错误事件。
func TestAgentCallback_OnErrorEmitsErrorMsg(t *testing.T) {
	got, cb := collectCallback()

	cb.OnError(errors.New("network down"))

	if len(*got) != 1 {
		t.Fatalf("期望 1 条消息，得到 %d", len(*got))
	}
	msg, ok := (*got)[0].(ErrorMsg)
	if !ok {
		t.Fatalf("消息类型应为 ErrorMsg，得到 %T", (*got)[0])
	}
	if msg.Err == nil || msg.Err.Error() != "network down" {
		t.Fatalf("错误内容不符: %v", msg.Err)
	}
}

// TestAgentCallback_NilSendDoesNotPanic 验证 send 为 nil 时不 panic。
func TestAgentCallback_NilSendDoesNotPanic(t *testing.T) {
	cb := NewAgentCallback(nil)

	cb.OnTextDelta("x")
	cb.OnThinkingDelta("x")
	cb.OnToolCallStart(llm.ToolCall{Name: "bash"})
	cb.OnToolCallEnd(llm.ToolCall{Name: "bash"}, nil, nil)
	cb.OnError(errors.New("x"))
	cb.OnTurnEnd(nil)
}

// TestAppUpdate_StreamDeltaAccumulates 验证增量经事件循环累积到流式视图。
// 这是"OnTextDelta 触发后内容递增"的直接断言。
func TestAppUpdate_StreamDeltaAccumulates(t *testing.T) {
	app := NewApp("test-model", "test-provider", "")
	app.StartStreaming()

	app.Update(StreamDeltaMsg{Delta: "Hel"})
	if got := app.Streaming.FullText(); got != "Hel" {
		t.Fatalf("首次增量后期望 %q，得到 %q", "Hel", got)
	}

	app.Update(StreamDeltaMsg{Delta: "lo"})
	if got := app.Streaming.FullText(); got != "Hello" {
		t.Fatalf("二次增量后期望 %q，得到 %q", "Hello", got)
	}
}

// TestAppUpdate_ThinkingDeltaAccumulates 验证思考增量累积。
func TestAppUpdate_ThinkingDeltaAccumulates(t *testing.T) {
	app := NewApp("m", "p", "")
	app.StartStreaming()

	app.Update(ThinkingDeltaMsg{Delta: "step1 "})
	app.Update(ThinkingDeltaMsg{Delta: "step2"})

	if got := app.Streaming.thinkingText; got != "step1 step2" {
		t.Fatalf("思考文本期望 %q，得到 %q", "step1 step2", got)
	}
}

// TestAppUpdate_ToolLifecycle 验证工具开始/结束在事件循环内的状态流转。
func TestAppUpdate_ToolLifecycle(t *testing.T) {
	app := NewApp("m", "p", "")
	app.StartStreaming()

	before := len(app.Messages)

	app.Update(ToolStartMsg{Name: "bash"})
	if app.Streaming.toolInProgress != "bash" {
		t.Fatalf("工具开始时 toolInProgress 应为 bash，得到 %q", app.Streaming.toolInProgress)
	}

	app.Update(ToolEndMsg{Name: "bash", Args: `{"cmd":"ls"}`, Result: "ok"})
	if app.Streaming.toolInProgress != "" {
		t.Fatalf("工具结束后 toolInProgress 应清空，得到 %q", app.Streaming.toolInProgress)
	}
	if len(app.Messages) != before+1 {
		t.Fatalf("工具结束应追加 1 条消息，期望 %d，得到 %d", before+1, len(app.Messages))
	}
	last := app.Messages[len(app.Messages)-1]
	if last.Role != "tool" || last.ToolMsg == nil {
		t.Fatalf("最后一条消息应为 tool 角色，得到 %+v", last)
	}
	if last.ToolMsg.ToolName != "bash" || last.ToolMsg.Result != "ok" || last.ToolMsg.IsError {
		t.Fatalf("工具消息内容不符: %+v", last.ToolMsg)
	}
}

// TestAppUpdate_ToolEndErrorFlag 验证工具失败标记透传。
func TestAppUpdate_ToolEndErrorFlag(t *testing.T) {
	app := NewApp("m", "p", "")

	app.Update(ToolEndMsg{Name: "bash", Result: "failed", IsError: true})

	if len(app.Messages) != 1 {
		t.Fatalf("期望 1 条消息，得到 %d", len(app.Messages))
	}
	tm := app.Messages[0].ToolMsg
	if tm == nil || !tm.IsError || tm.Result != "failed" {
		t.Fatalf("错误标记未透传: %+v", tm)
	}
}

// TestAppUpdate_StreamDeltaWhileNotStreaming 验证非流式状态下增量不破坏消息列表。
func TestAppUpdate_StreamDeltaWhileNotStreaming(t *testing.T) {
	app := NewApp("m", "p", "")

	app.Update(StreamDeltaMsg{Delta: "stray"})

	if app.IsStreaming {
		t.Fatal("未收到 UserInputMsg 时不应进入流式状态")
	}
	if len(app.Messages) != 0 {
		t.Fatalf("增量不应写入消息列表，得到 %d 条", len(app.Messages))
	}
}
