package agent

import (
	"errors"
	"testing"

	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/tool"
)

func TestNopCallback_AllMethodsNoop(t *testing.T) {
	cb := NopCallback{}
	cb.OnTextDelta("hello")
	cb.OnToolCallStart(llm.ToolCall{ID: "call_1"})
	cb.OnToolCallEnd(llm.ToolCall{}, &tool.Result{}, nil)
	cb.OnThinkingDelta("thinking")
	cb.OnTurnEnd(&Response{})
	cb.OnError(errors.New("test error"))
	// 没有 panic 即通过
}

func TestCallbackFuncs_OnTextDelta(t *testing.T) {
	var got string
	cb := &CallbackFuncs{
		TextDelta: func(d string) { got = d },
	}
	cb.OnTextDelta("hello")
	if got != "hello" {
		t.Errorf("期望 delta=hello, 得到 %s", got)
	}

	// 未设置字段不 panic
	cb2 := &CallbackFuncs{}
	cb2.OnTextDelta("world")
}

func TestCallbackFuncs_OnToolCallStart(t *testing.T) {
	var got llm.ToolCall
	cb := &CallbackFuncs{
		ToolCallStart: func(c llm.ToolCall) { got = c },
	}
	expected := llm.ToolCall{ID: "call_1", Name: "get_weather"}
	cb.OnToolCallStart(expected)
	if got.ID != expected.ID || got.Name != expected.Name {
		t.Errorf("期望 %+v, 得到 %+v", expected, got)
	}
}

func TestCallbackFuncs_OnToolCallEnd(t *testing.T) {
	var (
		gotCall   llm.ToolCall
		gotResult *tool.Result
		gotErr    error
	)
	cb := &CallbackFuncs{
		ToolCallEnd: func(c llm.ToolCall, r *tool.Result, e error) {
			gotCall = c
			gotResult = r
			gotErr = e
		},
	}
	expectedCall := llm.ToolCall{ID: "call_1"}
	expectedResult := &tool.Result{Content: "ok"}
	expectedErr := errors.New("failed")
	cb.OnToolCallEnd(expectedCall, expectedResult, expectedErr)
	if gotCall.ID != expectedCall.ID {
		t.Errorf("期望 call ID=%s, 得到 %s", expectedCall.ID, gotCall.ID)
	}
	if gotResult.Content != expectedResult.Content {
		t.Errorf("期望 result=%s, 得到 %s", expectedResult.Content, gotResult.Content)
	}
	if gotErr.Error() != expectedErr.Error() {
		t.Errorf("期望 err=%v, 得到 %v", expectedErr, gotErr)
	}
}

func TestCallbackFuncs_OnThinkingDelta(t *testing.T) {
	var got string
	cb := &CallbackFuncs{
		ThinkingDelta: func(d string) { got = d },
	}
	cb.OnThinkingDelta("thinking...")
	if got != "thinking..." {
		t.Errorf("期望 delta=thinking..., 得到 %s", got)
	}
}

func TestCallbackFuncs_OnTurnEnd(t *testing.T) {
	var got *Response
	cb := &CallbackFuncs{
		TurnEnd: func(r *Response) { got = r },
	}
	expected := &Response{SessionID: "sess_1"}
	cb.OnTurnEnd(expected)
	if got.SessionID != expected.SessionID {
		t.Errorf("期望 SessionID=%s, 得到 %s", expected.SessionID, got.SessionID)
	}
}

func TestCallbackFuncs_OnError(t *testing.T) {
	var got error
	cb := &CallbackFuncs{
		Error: func(e error) { got = e },
	}
	expected := errors.New("some error")
	cb.OnError(expected)
	if got.Error() != expected.Error() {
		t.Errorf("期望 err=%v, 得到 %v", expected, got)
	}
}
