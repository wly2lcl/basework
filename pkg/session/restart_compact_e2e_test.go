package session

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/wly2lcl/basework/pkg/llm"
)

// ---- CTX-003：重启与压缩后的项目恢复（端到端，真实 JSONLStore） ----

// buildRestartScenario 构造一个「读改跑测试」会话的事件序列：
// system prompt、两轮 steering、工具调用配对（成功+失败）、文件编辑
// （写入+冲突）、轮次失败、两次带快照的压缩。
// 通过真实 AppendEvent 写入（Seq 由存储分配），返回会话 ID。
func buildRestartScenario(t *testing.T, s *JSONLStore, sessionID string) {
	t.Helper()
	append := func(typ EventType, data any) {
		t.Helper()
		enc, err := EncodeData(data)
		if err != nil {
			t.Fatalf("编码 %s: %v", typ, err)
		}
		if err := s.AppendEvent(Event{SessionID: sessionID, Type: typ, Data: enc}); err != nil {
			t.Fatalf("追加 %s: %v", typ, err)
		}
	}

	append(EventSystemPromptSet, &SystemPromptSetData{Content: "你是测试助手", Hash: "h1"})
	append(EventSteered, &SteeredData{Messages: []string{"第一轮 steering：用简短回答"}})
	append(EventPrompted, &PromptedData{Content: "读一下 a.go"})
	append(EventToolCalled, &ToolCalledData{ToolCall: llm.ToolCall{ID: "call-1", Name: "read", ArgsJSON: `{"path":"a.go"}`}})
	append(EventToolSuccess, &ToolSuccessData{ToolCallID: "call-1", Content: "package main"})

	// 第二轮：带 steering、失败的工具调用与文件冲突。
	append(EventSteered, &SteeredData{Messages: []string{"第二轮 steering：注意错误处理"}})
	append(EventPrompted, &PromptedData{Content: "改一下 a.go 和 b.go"})
	append(EventFileEdited, &FileEditedData{
		Phase:  "committed",
		PlanID: "plan-1",
		Files: []FileEditRecord{
			{Path: "a.go", Op: "replace", State: "written"},
			{Path: "b.go", Op: "replace", State: "conflict", Err: "edits: 基线不一致: 文件已被外部修改"},
		},
		Summary: "提交结果：共 2 个文件，已写 1，未写 1",
	})
	append(EventToolCalled, &ToolCalledData{ToolCall: llm.ToolCall{ID: "call-2", Name: "bash", ArgsJSON: `{"command":"go test ./..."}`}})
	append(EventToolFailed, &ToolFailedData{ToolCallID: "call-2", Error: "exit status 1: FAIL pkg/x"})

	// 第一次压缩：保留失败原因与最近消息，丢掉最早的读取。
	snapshot1 := []llm.ChatMessage{
		{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "读一下 a.go"}}},
		{Role: llm.RoleTool, ToolCallID: "call-1", Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "package main"}}},
		{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "改一下 a.go 和 b.go"}}},
	}
	append(EventCompacted, &CompactedData{Summary: "压缩1：读改测试进行中", Snapshot: snapshot1})

	// 第二次压缩：只留失败上下文。
	snapshot2 := []llm.ChatMessage{
		{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "改一下 a.go 和 b.go"}}},
		{Role: llm.RoleTool, ToolCallID: "call-2", Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "exit status 1: FAIL pkg/x"}}},
	}
	append(EventCompacted, &CompactedData{Summary: "压缩2：只剩失败上下文", Snapshot: snapshot2})
}

// TestRestart_ProjectsAfterTwoCompactions 两次压缩后重启（关店重开）恢复：
// 消息不重复、不丢失；工具调用配对完整；最近变更与失败原因保留。
func TestRestart_ProjectsAfterTwoCompactions(t *testing.T) {
	dir := t.TempDir()

	store, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("打开存储: %v", err)
	}
	info, err := store.Create(CreateOpts{Title: "restart"})
	if err != nil {
		t.Fatalf("创建会话: %v", err)
	}
	sid := info.ID
	buildRestartScenario(t, store, sid)

	// —— 重启：全新打开 ——
	// 「重启」= 用同一个目录重新构造存储实例（无进程内状态、无文件锁，
	// 新实例等价于新进程从磁盘读取）。
	reopened, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("重开存储: %v", err)
	}
	events, err := reopened.Events(EventFilter{SessionID: sid})
	if err != nil {
		t.Fatalf("读事件: %v", err)
	}

	msgs := ProjectMessages(events)

	// 第二次压缩的快照是最终历史：恰好 2 条（用户消息 + 失败工具结果），
	// 不重复、不丢失（若快照替换失效会看到累计历史而非 2 条）。
	if len(msgs) != 2 {
		t.Fatalf("两次压缩后应恰为快照的 2 条，得到 %d 条: %+v", len(msgs), msgs)
	}
	if msgs[0].Role != llm.RoleUser || msgs[1].Role != llm.RoleTool || msgs[1].ToolCallID != "call-2" {
		t.Fatalf("快照内容不符: %+v", msgs)
	}

	// 失败原因保留：call-2 的错误文本在历史里。
	toolText := msgs[1].Content[0].Text
	if !strings.Contains(toolText, "FAIL pkg/x") {
		t.Fatalf("失败原因应保留: %q", toolText)
	}
}

// TestRestart_ToolPairingBeforeCompaction 重启后、压缩前的历史段中
// 工具调用必须配对（assistant.tool_calls ↔ tool result），文件冲突
// 状态保留（file.edited 的 conflict 记录可在事件流中找到）。
func TestRestart_ToolPairingBeforeCompaction(t *testing.T) {
	dir := t.TempDir()

	store, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("打开存储: %v", err)
	}
	info, err := store.Create(CreateOpts{Title: "pairing"})
	if err != nil {
		t.Fatalf("创建会话: %v", err)
	}
	sid := info.ID
	buildRestartScenario(t, store, sid)

	// 重启：同目录新实例。
	reopened, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("重开存储: %v", err)
	}
	events, err := reopened.Events(EventFilter{SessionID: sid})
	if err != nil {
		t.Fatalf("读事件: %v", err)
	}

	// 事件层核对：每个 tool.called 都有同 ID 的 success/failed 配对。
	called := map[string]bool{}
	resolved := map[string]bool{}
	var conflictSeen bool
	for _, ev := range events {
		switch ev.Type {
		case EventToolCalled:
			var d ToolCalledData
			if err := json.Unmarshal(ev.Data, &d); err == nil {
				called[d.ToolCall.ID] = true
			}
		case EventToolSuccess:
			var d ToolSuccessData
			if err := json.Unmarshal(ev.Data, &d); err == nil {
				resolved[d.ToolCallID] = true
			}
		case EventToolFailed:
			var d ToolFailedData
			if err := json.Unmarshal(ev.Data, &d); err == nil {
				resolved[d.ToolCallID] = true
			}
		case EventFileEdited:
			var d FileEditedData
			if err := json.Unmarshal(ev.Data, &d); err == nil {
				for _, f := range d.Files {
					if f.State == "conflict" && f.Path == "b.go" {
						conflictSeen = true
					}
				}
			}
		}
	}
	for id := range called {
		if !resolved[id] {
			t.Fatalf("工具调用 %s 重启后失去配对结果", id)
		}
	}
	if !conflictSeen {
		t.Fatal("文件冲突状态（b.go conflict）重启后应保留")
	}

	// 投影层核对第一次压缩前的段：assistant(tool_calls) 与 tool 成对相邻。
	msgs := ProjectMessages(events[:firstCompactionSeq(events)-1])
	paired := map[string]bool{}
	for _, m := range msgs {
		for _, tc := range m.ToolCalls {
			paired[tc.ID] = false
		}
		if m.Role == llm.RoleTool && m.ToolCallID != "" {
			paired[m.ToolCallID] = true
		}
	}
	for id, ok := range paired {
		if !ok {
			t.Fatalf("压缩前历史段中工具调用 %s 无配对结果", id)
		}
	}
}

func firstCompactionSeq(events []Event) int64 {
	for _, ev := range events {
		if ev.Type == EventCompacted {
			return ev.Seq
		}
	}
	return -1
}

// TestRestart_SnapshotSizeDelta 记录完整快照带来的日志体积增量（任务卡第 5 步）。
// 快照把「被压缩掉的历史」从摘要一行变成全量消息，体积增长是已知代价；
// 本测试量化它，供证据文档引用。
func TestRestart_SnapshotSizeDelta(t *testing.T) {
	history := []llm.ChatMessage{
		{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: strings.Repeat("历史消息内容。", 50)}}},
		{Role: llm.RoleAssistant, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: strings.Repeat("助手回复内容。", 50)}}},
	}

	newEnc, err := EncodeData(&CompactedData{Summary: "摘要", Snapshot: history})
	if err != nil {
		t.Fatal(err)
	}
	legacyEnc, err := EncodeData(&CompactedData{Summary: "摘要", TruncatedSeq: 2, KeepFrom: 1})
	if err != nil {
		t.Fatal(err)
	}

	newSize := len(newEnc)
	legacySize := len(legacyEnc)
	payload := 0
	for _, m := range history {
		for _, p := range m.Content {
			payload += len(p.Text)
		}
	}
	t.Logf("快照事件 %d 字节，旧格式事件 %d 字节，消息负载 %d 字节（增量 ≈ %.1f%%）",
		newSize, legacySize, payload, float64(newSize-legacySize)/float64(legacySize)*100)
	if !bytes.Contains(newEnc, []byte("历史消息内容。")) {
		t.Fatal("快照应包含完整消息内容")
	}
	if newSize <= legacySize {
		t.Fatal("快照事件理应大于旧格式事件（记录增量本身，不构成失败）")
	}
}
