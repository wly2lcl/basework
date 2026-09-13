package agent

import (
	"strings"
	"testing"

	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/session"
)

// TestBuildRequestMessages_RestartAfterCompactions（CTX-003）：
// 两次带快照的压缩 + 重启（全新从磁盘读事件）之后，重建的请求必须——
// 恰好一条 system prompt、全部已持久化 steering 按序重放（累积，非一次性
// 消费）、历史恰为最后一次压缩的快照（不重复不丢失）。
func TestBuildRequestMessages_RestartAfterCompactions(t *testing.T) {
	baseDir := t.TempDir()
	store, err := session.NewJSONLStore(baseDir)
	if err != nil {
		t.Fatalf("打开存储: %v", err)
	}
	info, err := store.Create(session.CreateOpts{Title: "ctx003"})
	if err != nil {
		t.Fatalf("创建会话: %v", err)
	}
	sid := info.ID

	add := func(typ session.EventType, data any) {
		t.Helper()
		enc, err := session.EncodeData(data)
		if err != nil {
			t.Fatalf("编码 %s: %v", typ, err)
		}
		if err := store.AppendEvent(session.Event{SessionID: sid, Type: typ, Data: enc}); err != nil {
			t.Fatalf("追加 %s: %v", typ, err)
		}
	}

	add(session.EventSystemPromptSet, &session.SystemPromptSetData{Content: "系统提示词", Hash: "h"})
	add(session.EventSteered, &session.SteeredData{Messages: []string{"steer-1"}})
	add(session.EventPrompted, &session.PromptedData{Content: "第一问"})
	add(session.EventSteered, &session.SteeredData{Messages: []string{"steer-2"}})
	add(session.EventPrompted, &session.PromptedData{Content: "第二问"})

	// 压缩 1：保留第一问。
	snap1 := []llm.ChatMessage{
		{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "第一问"}}},
	}
	add(session.EventCompacted, &session.CompactedData{Summary: "s1", Snapshot: snap1})

	// 压缩 2：保留第二问（快照重排/选择性保留的表达力）。
	snap2 := []llm.ChatMessage{
		{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "第二问"}}},
	}
	add(session.EventCompacted, &session.CompactedData{Summary: "s2", Snapshot: snap2})

	// —— 重启：全新实例读事件 ——
	reopened, err := session.NewJSONLStore(baseDir)
	if err != nil {
		t.Fatalf("重开存储: %v", err)
	}
	events, err := reopened.Events(session.EventFilter{SessionID: sid})
	if err != nil {
		t.Fatalf("读事件: %v", err)
	}

	// 请求级上下文（system prompt / steering）在压缩后必须全部还在：
	// 它们不进投影（投影只管历史），由 BuildRequestMessages 单独回填。
	msgs := BuildRequestMessages(events)

	if len(msgs) == 0 || msgs[0].Role != llm.RoleSystem {
		t.Fatalf("第一条应为 system prompt: %+v", msgs)
	}
	if got := msgs[0].Content[0].Text; got != "系统提示词" {
		t.Fatalf("system prompt 内容不符: %q", got)
	}
	// system prompt 只出现一次（steering 虽同为 system 角色，但内容可区分）。
	sysPromptCount := 0
	for _, m := range msgs {
		if m.Role == llm.RoleSystem && m.Content[0].Text == "系统提示词" {
			sysPromptCount++
		}
	}
	if sysPromptCount != 1 {
		t.Fatalf("system prompt 应恰 1 条，得到 %d", sysPromptCount)
	}

	// steering 全部重放（累积，非一次性消费）：steer-1 与 steer-2 都在，
	// 且按写入顺序（steering 以 system 角色注入，见 steeringToChatMessages）。
	var steered []string
	for _, m := range msgs {
		if m.Role == llm.RoleSystem && (strings.Contains(m.Content[0].Text, "steer-1") || strings.Contains(m.Content[0].Text, "steer-2")) {
			steered = append(steered, m.Content[0].Text)
		}
	}
	if len(steered) != 2 || !strings.Contains(steered[0], "steer-1") || !strings.Contains(steered[1], "steer-2") {
		t.Fatalf("steering 应全部按序重放: %v", steered)
	}

	// 历史 = 最后一次压缩的快照（第二问），不重复不丢失。
	history := msgs[len(msgs)-1:]
	if len(history) != 1 || history[0].Role != llm.RoleUser || !strings.Contains(history[0].Content[0].Text, "第二问") {
		t.Fatalf("压缩后历史应为最后快照: %+v", msgs)
	}
}
