package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/wly2lcl/basework/pkg/llm"
)

// 本文件是流式工具调用协议的回归测试，覆盖 REL-001 的四类场景：
// 增量参数、最终完整参数、交错调用、空文本，以及断流与重试不重复执行。
//
// 协议约定（与 pkg/llm.ToolCallDelta 的文档一致）：
//   - Complete=false 时 ArgsJSON 是"自上次同 index 事件以来的参数片段"，消费方按序拼接；
//   - Complete=true 时 ArgsJSON 是完整参数，是唯一权威值；
//   - 同一响应内，Complete 事件按 index 升序发出，与运行次数无关。

const (
	fragOpen   = `{"city":`
	fragValue  = `"Beijing"`
	fragParams = fragOpen + fragValue + `}`
)

// sseLine 写入一行 SSE data 事件。
func sseLine(w http.ResponseWriter, payload string) {
	fmt.Fprintf(w, "data: %s\n\n", payload)
}

// newStreamServer 启动一个返回固定 SSE 脚本的测试服务。
func newStreamServer(t *testing.T, script func(w http.ResponseWriter)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		script(w)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// streamAll 消费全部事件，遇到错误立即失败。
func streamAll(t *testing.T, m llm.Model) []llm.StreamEvent {
	t.Helper()
	ch, err := m.Stream(t.Context(), &llm.Request{
		Messages: []llm.ChatMessage{{
			Role:    llm.RoleUser,
			Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "go"}},
		}},
	})
	if err != nil {
		t.Fatalf("Stream 返回错误: %v", err)
	}
	var events []llm.StreamEvent
	for evt := range ch {
		if evt.Error != nil {
			t.Fatalf("流中出现错误: %v", evt.Error)
		}
		events = append(events, evt)
	}
	return events
}

// TestStream_ToolCallArgsDeltaIsIncremental 验证增量事件的 ArgsJSON 是参数片段，
// 拼接后等于 Complete 事件给出的完整参数（参数既不丢失也不重复拼接）。
func TestStream_ToolCallArgsDeltaIsIncremental(t *testing.T) {
	srv := newStreamServer(t, func(w http.ResponseWriter) {
		sseLine(w, `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"get_weather","arguments":""}}]}}]}`)
		sseLine(w, `{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"city\":"}}]}}]}`)
		sseLine(w, `{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"Beijing\""}}]}}]}`)
		sseLine(w, `{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"}"}}]}}]}`)
		sseLine(w, `{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`)
		sseLine(w, `[DONE]`)
	})

	m, err := newOpenAI(srv.URL, "test-key", "gpt-4", nil)
	if err != nil {
		t.Fatalf("newOpenAI: %v", err)
	}

	var fragments []string
	var complete *llm.ToolCallDelta
	for _, evt := range streamAll(t, m) {
		if evt.Type != llm.StreamEventToolCall || evt.ToolCall == nil {
			continue
		}
		if evt.ToolCall.Complete {
			complete = evt.ToolCall
			continue
		}
		fragments = append(fragments, evt.ToolCall.ArgsJSON)
	}

	// 服务端共发出 4 个参数片段（首个片段为空，与 id/name 同帧到达）。
	wantFragments := []string{"", fragOpen, fragValue, "}"}
	if len(fragments) != len(wantFragments) {
		t.Fatalf("期望 %d 个增量参数片段，得到 %d 个: %q", len(wantFragments), len(fragments), fragments)
	}
	for i, want := range wantFragments {
		if fragments[i] != want {
			t.Errorf("片段 %d 不是增量值: 期望 %q, 实际 %q（完整列表 %q）", i, want, fragments[i], fragments)
		}
	}
	// 每个增量事件只能携带本片段，不能携带累计值——否则消费方拼接会重复。
	if got := strings.Join(fragments, ""); got != fragParams {
		t.Errorf("增量片段拼接结果错误\n  期望 %q\n  实际 %q\n  片段 %q", fragParams, got, fragments)
	}

	if complete == nil {
		t.Fatal("缺少 Complete=true 的工具调用事件")
	}
	if complete.ArgsJSON != fragParams {
		t.Errorf("完整参数错误: 期望 %q, 实际 %q", fragParams, complete.ArgsJSON)
	}
	if complete.ID != "call_1" || complete.Name != "get_weather" {
		t.Errorf("完整事件的 ID/Name 错误: ID=%q Name=%q", complete.ID, complete.Name)
	}
}

// TestStream_InterleavedToolCallsKeepIdentityAndOrder 验证交错到达的多个工具调用
// 各自保留 ID/参数对应关系，且 Complete 事件顺序与 index 升序一致、可复现。
func TestStream_InterleavedToolCallsKeepIdentityAndOrder(t *testing.T) {
	const rounds = 50
	srv := newStreamServer(t, func(w http.ResponseWriter) {
		// 三个调用交错到达：0、1、2 依次开场，随后 0、1、2 依次补参数。
		sseLine(w, `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"tool_a","arguments":"{\"x\":1}"}}]}}]}`)
		sseLine(w, `{"choices":[{"delta":{"tool_calls":[{"index":1,"id":"call_b","type":"function","function":{"name":"tool_b","arguments":"{\"y\":2}"}}]}}]}`)
		sseLine(w, `{"choices":[{"delta":{"tool_calls":[{"index":2,"id":"call_c","type":"function","function":{"name":"tool_c","arguments":"{\"z\":3}"}}]}}]}`)
		sseLine(w, `{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":""}}]}}]}`)
		sseLine(w, `{"choices":[{"delta":{"tool_calls":[{"index":1,"function":{"arguments":""}}]}}]}`)
		sseLine(w, `{"choices":[{"delta":{"tool_calls":[{"index":2,"function":{"arguments":""}}]}}]}`)
		sseLine(w, `{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`)
		sseLine(w, `[DONE]`)
	})

	m, err := newOpenAI(srv.URL, "test-key", "gpt-4", nil)
	if err != nil {
		t.Fatalf("newOpenAI: %v", err)
	}

	wantIDs := []string{"call_a", "call_b", "call_c"}
	wantArgs := []string{`{"x":1}`, `{"y":2}`, `{"z":3}`}

	for round := 0; round < rounds; round++ {
		var ids, args []string
		for _, evt := range streamAll(t, m) {
			if evt.Type != llm.StreamEventToolCall || evt.ToolCall == nil || !evt.ToolCall.Complete {
				continue
			}
			ids = append(ids, evt.ToolCall.ID)
			args = append(args, evt.ToolCall.ArgsJSON)
		}

		if strings.Join(ids, ",") != strings.Join(wantIDs, ",") {
			t.Fatalf("第 %d 轮 Complete 事件顺序不确定: 期望 %v, 实际 %v", round, wantIDs, ids)
		}
		for i := range wantArgs {
			if args[i] != wantArgs[i] {
				t.Fatalf("第 %d 轮 index=%d 的参数与 ID 对应错误: ID=%s 参数=%s", round, i, ids[i], args[i])
			}
		}
		if len(ids) != len(wantIDs) {
			t.Fatalf("第 %d 轮工具调用数量错误: 期望 %d, 实际 %d", round, len(wantIDs), len(ids))
		}
	}
}

// TestStream_EmptyTextDeltaProducesNoTextEvent 验证空文本增量不产生文本事件。
func TestStream_EmptyTextDeltaProducesNoTextEvent(t *testing.T) {
	srv := newStreamServer(t, func(w http.ResponseWriter) {
		sseLine(w, `{"choices":[{"delta":{"role":"assistant","content":""}}]}`)
		sseLine(w, `{"choices":[{"delta":{"content":""}}]}`)
		sseLine(w, `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"noop","arguments":"{}"}}]}}]}`)
		sseLine(w, `{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`)
		sseLine(w, `[DONE]`)
	})

	m, err := newOpenAI(srv.URL, "test-key", "gpt-4", nil)
	if err != nil {
		t.Fatalf("newOpenAI: %v", err)
	}

	var textDeltas []string
	var toolCalls int
	for _, evt := range streamAll(t, m) {
		switch evt.Type {
		case llm.StreamEventText:
			textDeltas = append(textDeltas, evt.Delta)
		case llm.StreamEventToolCall:
			if evt.ToolCall != nil && evt.ToolCall.Complete {
				toolCalls++
			}
		}
	}

	if len(textDeltas) != 0 {
		t.Errorf("空文本增量不应产生文本事件，实际得到 %q", textDeltas)
	}
	if toolCalls != 1 {
		t.Errorf("期望恰好 1 个完整工具调用，得到 %d", toolCalls)
	}
}

// TestStream_TruncatedStreamFlushesExactlyOnce 验证断流（无 finish_reason）时
// 残留工具调用被补齐为完整事件，且只发出一次。
func TestStream_TruncatedStreamFlushesExactlyOnce(t *testing.T) {
	srv := newStreamServer(t, func(w http.ResponseWriter) {
		sseLine(w, `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"get_weather","arguments":""}}]}}]}`)
		sseLine(w, `{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"city\":"}}]}}]}`)
		sseLine(w, `{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"Beijing\"}"}}]}}]}`)
		// 断流：没有 finish_reason，直接 [DONE]
		sseLine(w, `[DONE]`)
	})

	m, err := newOpenAI(srv.URL, "test-key", "gpt-4", nil)
	if err != nil {
		t.Fatalf("newOpenAI: %v", err)
	}

	var completes []*llm.ToolCallDelta
	for _, evt := range streamAll(t, m) {
		if evt.Type == llm.StreamEventToolCall && evt.ToolCall != nil && evt.ToolCall.Complete {
			completes = append(completes, evt.ToolCall)
		}
	}

	if len(completes) != 1 {
		t.Fatalf("断流后应恰好补齐 1 个完整工具调用，得到 %d", len(completes))
	}
	if completes[0].ArgsJSON != fragParams {
		t.Errorf("补齐的参数错误: 期望 %q, 实际 %q", fragParams, completes[0].ArgsJSON)
	}
	if completes[0].ID != "call_1" {
		t.Errorf("补齐的 ID 错误: %q", completes[0].ID)
	}
}

// TestStream_NoRetryOnStreamingRequest 验证流式请求不做隐式重试：
// 失败状态码只请求一次，因此不存在"重试导致有副作用执行重复"的路径。
func TestStream_NoRetryOnStreamingRequest(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var hits int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&hits, 1)
				w.WriteHeader(status)
				fmt.Fprint(w, `{"error":{"message":"transient"}}`)
			}))
			defer srv.Close()

			m, err := newOpenAI(srv.URL, "test-key", "gpt-4", nil)
			if err != nil {
				t.Fatalf("newOpenAI: %v", err)
			}

			ch, err := m.Stream(t.Context(), &llm.Request{
				Messages: []llm.ChatMessage{{
					Role:    llm.RoleUser,
					Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "go"}},
				}},
			})
			if err == nil {
				for range ch {
				}
				t.Fatalf("状态码 %d 时 Stream 应返回错误", status)
			}
			if got := atomic.LoadInt32(&hits); got != 1 {
				t.Errorf("流式请求不应重试，期望 1 次请求，实际 %d 次", got)
			}
		})
	}
}
