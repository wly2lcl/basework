package session

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// v0SessionJSONL 是一份「v0 格式」的会话文件：没有文件头，事件没有 `v` 字段。
// 模拟引入格式版本化之前落盘的历史数据。
const v0SessionJSONL = `{"id":"e1","session_id":"sess01","type":"prompted","data":{"content":"你好"},"seq":1,"created_at":"2026-01-01T00:00:00Z"}
{"id":"e2","session_id":"sess01","type":"text.delta","data":{"delta":"你"},"seq":2,"created_at":"2026-01-01T00:00:01Z"}
{"id":"e3","session_id":"sess01","type":"text.delta","data":{"delta":"好"},"seq":3,"created_at":"2026-01-01T00:00:02Z"}
`

// writeV0Session 在 dir 下写一份 v0 格式会话文件。
func writeV0Session(t *testing.T, dir, id, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, id+".jsonl"), []byte(content), 0o644); err != nil {
		t.Fatalf("写入 v0 会话文件失败: %v", err)
	}
}

// TestMigrateV0ToV1 v0 老文件必须可读，并被升级标记为当前版本。
func TestMigrateV0ToV1(t *testing.T) {
	dir := t.TempDir()
	writeV0Session(t, dir, "sess01", v0SessionJSONL)

	store, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("创建 store 失败: %v", err)
	}

	events, err := store.Events(EventFilter{SessionID: "sess01"})
	if err != nil {
		t.Fatalf("读取 v0 会话失败: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("事件数量 = %d，期望 3", len(events))
	}
	for i, e := range events {
		if e.SchemaVersion != SchemaVersion {
			t.Errorf("第 %d 条事件 SchemaVersion = %d，期望迁移到 %d", i, e.SchemaVersion, SchemaVersion)
		}
	}

	// 迁移后投影结果必须与 v0 语义一致（v0→v1 是纯版本戳，不改结构）。
	msgs := ProjectMessages(events)
	if len(msgs) != 2 {
		t.Fatalf("投影消息数 = %d，期望 2（1 条 user + 1 条 assistant）", len(msgs))
	}
	if msgs[0].Role != "user" {
		t.Errorf("首条消息角色 = %q，期望 user", msgs[0].Role)
	}
	if msgs[1].Role != "assistant" {
		t.Errorf("次条消息角色 = %q，期望 assistant", msgs[1].Role)
	}
}

// TestMigrateV0ToV1RewritesFileWithHeader 追加事件后文件应带上文件头，
// 且内容与文件头声明的版本一致（不允许出现"头说 v1、内容还是 v0"的半迁移状态）。
func TestMigrateV0ToV1RewritesFileWithHeader(t *testing.T) {
	dir := t.TempDir()
	writeV0Session(t, dir, "sess01", v0SessionJSONL)

	store, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("创建 store 失败: %v", err)
	}
	if _, err := store.Events(EventFilter{SessionID: "sess01"}); err != nil {
		t.Fatalf("读取 v0 会话失败: %v", err)
	}
	if err := store.AppendEvent(Event{
		SessionID: "sess01",
		Type:      EventTextEnded,
	}); err != nil {
		t.Fatalf("追加事件失败: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "sess01.jsonl"))
	if err != nil {
		t.Fatalf("读取文件失败: %v", err)
	}
	lines := splitNonEmptyLines(string(raw))
	if len(lines) != 5 {
		t.Fatalf("文件行数 = %d，期望 5（1 行文件头 + 原 3 条事件 + 新增 1 条）", len(lines))
	}

	// 首行是文件头
	var hdr sessionHeader
	if err := json.Unmarshal([]byte(lines[0]), &hdr); err != nil {
		t.Fatalf("解析文件头失败: %v", err)
	}
	if hdr.Schema != sessionSchemaName {
		t.Errorf("文件头 _schema = %q，期望 %q", hdr.Schema, sessionSchemaName)
	}
	if hdr.V != SchemaVersion {
		t.Errorf("文件头 v = %d，期望 %d", hdr.V, SchemaVersion)
	}

	// 每条事件都带当前版本号
	for i, line := range lines[1:] {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			t.Fatalf("解析第 %d 条事件失败: %v", i+1, err)
		}
		got, ok := obj["v"]
		if !ok {
			t.Errorf("第 %d 条事件缺少 v 字段", i+1)
			continue
		}
		if string(got) != strconv.Itoa(SchemaVersion) {
			t.Errorf("第 %d 条事件 v = %s，期望 %d", i+1, got, SchemaVersion)
		}
	}
}

// TestCreatedSessionHasHeader 新建会话即带文件头。
func TestCreatedSessionHasHeader(t *testing.T) {
	dir := t.TempDir()
	store, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("创建 store 失败: %v", err)
	}
	info, err := store.Create(CreateOpts{Title: "t"})
	if err != nil {
		t.Fatalf("创建会话失败: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, info.ID+".jsonl"))
	if err != nil {
		t.Fatalf("读取文件失败: %v", err)
	}
	if _, ok := probeHeaderVersion([]byte(strings.TrimSpace(string(raw)))); !ok {
		t.Fatalf("新建会话文件首行不是文件头: %q", string(raw))
	}
}

// TestSchemaTooNewRejected 未来版本必须被拒绝，且错误可被 errors.Is 判定。
func TestSchemaTooNewRejected(t *testing.T) {
	t.Run("文件头版本过高", func(t *testing.T) {
		dir := t.TempDir()
		content := `{"_schema":"basework.session","v":999}
{"id":"e1","session_id":"sess01","type":"prompted","data":{"content":"x"},"seq":1,"created_at":"2026-01-01T00:00:00Z"}
`
		writeV0Session(t, dir, "sess01", content)

		store, err := NewJSONLStore(dir)
		if err != nil {
			t.Fatalf("创建 store 失败: %v", err)
		}
		_, err = store.Events(EventFilter{SessionID: "sess01"})
		if !errors.Is(err, ErrSchemaTooNew) {
			t.Fatalf("错误 = %v，期望 ErrSchemaTooNew", err)
		}
	})

	t.Run("单条事件版本过高", func(t *testing.T) {
		dir := t.TempDir()
		content := `{"id":"e1","session_id":"sess01","type":"prompted","data":{"content":"x"},"seq":1,"created_at":"2026-01-01T00:00:00Z","v":999}
`
		writeV0Session(t, dir, "sess01", content)

		store, err := NewJSONLStore(dir)
		if err != nil {
			t.Fatalf("创建 store 失败: %v", err)
		}
		_, err = store.Events(EventFilter{SessionID: "sess01"})
		if !errors.Is(err, ErrSchemaTooNew) {
			t.Fatalf("错误 = %v，期望 ErrSchemaTooNew", err)
		}
	})

	t.Run("写入路径必须失败", func(t *testing.T) {
		dir := t.TempDir()
		content := `{"_schema":"basework.session","v":999}
`
		writeV0Session(t, dir, "sess01", content)

		store, err := NewJSONLStore(dir)
		if err != nil {
			t.Fatalf("创建 store 失败: %v", err)
		}
		err = store.AppendEvent(Event{SessionID: "sess01", Type: EventTextEnded})
		if !errors.Is(err, ErrSchemaTooNew) {
			t.Fatalf("追加事件的错误 = %v，期望 ErrSchemaTooNew（不得降级写入）", err)
		}
	})
}

// TestProbeHeaderVersion 文件头识别必须精确：普通事件行不能被误判为文件头。
func TestProbeHeaderVersion(t *testing.T) {
	cases := []struct {
		name string
		line string
		want bool
	}{
		{"文件头", `{"_schema":"basework.session","v":1}`, true},
		{"版本过高也识别为文件头", `{"_schema":"basework.session","v":9}`, true},
		{"普通事件", `{"id":"e1","type":"prompted","data":{},"seq":1}`, false},
		{"别的 schema 名", `{"_schema":"other","v":1}`, false},
		{"非法 JSON", `not json`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := probeHeaderVersion([]byte(tc.line))
			if ok != tc.want {
				t.Errorf("probeHeaderVersion(%q) ok = %v，期望 %v", tc.line, ok, tc.want)
			}
		})
	}
}

// TestMigrateV1ToV2 v1 日志必须能升到当前版本。迁移只补版本戳，
// 但不据此保证新投影器与旧程序产生相同的消息历史。
func TestMigrateV1ToV2(t *testing.T) {
	dir := t.TempDir()
	// 一份 v1 文件：带 v1 文件头，事件也盖章为 1。
	content := `{"_schema":"basework.session","v":1}
{"id":"e1","session_id":"sess2","type":"prompted","data":{"content":"你好"},"seq":1,"created_at":"2026-01-01T00:00:00Z","v":1}
{"id":"e2","session_id":"sess2","type":"text.delta","data":{"delta":"好"},"seq":2,"created_at":"2026-01-01T00:00:01Z","v":1}
`
	writeV0Session(t, dir, "sess2", content)

	store, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("创建 store 失败: %v", err)
	}

	events, err := store.Events(EventFilter{SessionID: "sess2"})
	if err != nil {
		t.Fatalf("读取 v1 会话失败: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("事件数量 = %d，期望 2", len(events))
	}
	for i, e := range events {
		if e.SchemaVersion != SchemaVersion {
			t.Errorf("第 %d 条事件 SchemaVersion = %d，期望 %d", i, e.SchemaVersion, SchemaVersion)
		}
	}

	// 再追加一条事件后，文件头必须升到 v2，不能停留在 v1。
	if err := store.AppendEvent(Event{SessionID: "sess2", Type: EventTextEnded}); err != nil {
		t.Fatalf("追加事件失败: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "sess2.jsonl"))
	if err != nil {
		t.Fatalf("读取文件失败: %v", err)
	}
	lines := splitNonEmptyLines(string(raw))
	var hdr sessionHeader
	if err := json.Unmarshal([]byte(lines[0]), &hdr); err != nil {
		t.Fatalf("解析文件头失败: %v", err)
	}
	if hdr.V != SchemaVersion {
		t.Errorf("重写后的文件头 v = %d，期望 %d", hdr.V, SchemaVersion)
	}
}

// TestMigrateV1CompactedPayloadUsesCurrentProjection 锁定 v1 压缩事件的兼容语义：
// 迁移只给事件补 v2 版本戳，不改 payload；当前投影器仍会读取旧 payload 中已有的
// Summary 和 TruncatedSeq，并据此插入摘要、截断历史。
func TestMigrateV1CompactedPayloadUsesCurrentProjection(t *testing.T) {
	dir := t.TempDir()
	content := `{"_schema":"basework.session","v":1}
{"id":"e1","session_id":"sess-compact","type":"prompted","data":{"content":"旧段"},"seq":1,"created_at":"2026-01-01T00:00:00Z","v":1}
{"id":"e2","session_id":"sess-compact","type":"text.delta","data":{"delta":"旧回复"},"seq":2,"created_at":"2026-01-01T00:00:01Z","v":1}
{"id":"e3","session_id":"sess-compact","type":"text.ended","data":null,"seq":3,"created_at":"2026-01-01T00:00:02Z","v":1}
{"id":"e4","session_id":"sess-compact","type":"prompted","data":{"content":"保留问题"},"seq":4,"created_at":"2026-01-01T00:00:03Z","v":1}
{"id":"e5","session_id":"sess-compact","type":"text.delta","data":{"delta":"回答"},"seq":5,"created_at":"2026-01-01T00:00:04Z","v":1}
{"id":"e6","session_id":"sess-compact","type":"text.ended","data":null,"seq":6,"created_at":"2026-01-01T00:00:05Z","v":1}
{"id":"e7","session_id":"sess-compact","type":"compacted","data":{"summary":"压缩前讨论了旧段","truncated_seq":3,"keep_from":1},"seq":7,"created_at":"2026-01-01T00:00:06Z","v":1}
`
	writeV0Session(t, dir, "sess-compact", content)

	store, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("创建 store 失败: %v", err)
	}
	events, err := store.Events(EventFilter{SessionID: "sess-compact"})
	if err != nil {
		t.Fatalf("读取 v1 会话失败: %v", err)
	}
	if len(events) != 7 {
		t.Fatalf("事件数量 = %d，期望 7", len(events))
	}
	if events[6].SchemaVersion != SchemaVersion {
		t.Fatalf("压缩事件 SchemaVersion = %d，期望迁移到 %d", events[6].SchemaVersion, SchemaVersion)
	}

	msgs, seqs := ProjectMessagesWithSeq(events)
	if len(msgs) != 3 {
		t.Fatalf("迁移后投影消息数 = %d，期望 3（摘要 + 两条保留消息）: %+v", len(msgs), msgs)
	}
	wantTexts := []string{SummaryMessagePrefix + "压缩前讨论了旧段", "保留问题", "回答"}
	for i, want := range wantTexts {
		if len(msgs[i].Content) == 0 || msgs[i].Content[0].Text != want {
			t.Errorf("第 %d 条消息内容 = %+v，期望文本 %q", i, msgs[i].Content, want)
		}
	}
	if want := []int64{3, 4, 6}; !reflect.DeepEqual(seqs, want) {
		t.Errorf("迁移后来源 Seq = %v，期望 %v", seqs, want)
	}
}

// TestAdjacentStepsEnforced 迁移链只允许相邻步进，不允许跳跃与回退。
func TestAdjacentStepsEnforced(t *testing.T) {
	full := []migration{{From: 0, To: 1}, {From: 1, To: 2}}

	t.Run("同版本返回空", func(t *testing.T) {
		steps, err := adjacentSteps(1, 1, full)
		if err != nil {
			t.Fatalf("意外错误: %v", err)
		}
		if len(steps) != 0 {
			t.Errorf("步骤数 = %d，期望 0", len(steps))
		}
	})

	t.Run("连续两跳", func(t *testing.T) {
		steps, err := adjacentSteps(0, 2, full)
		if err != nil {
			t.Fatalf("意外错误: %v", err)
		}
		if len(steps) != 2 {
			t.Errorf("步骤数 = %d，期望 2", len(steps))
		}
	})

	t.Run("缺步骤报错", func(t *testing.T) {
		if _, err := adjacentSteps(0, 2, []migration{{From: 0, To: 1}}); err == nil {
			t.Error("缺少 v1→v2 步骤时应报错")
		}
	})

	t.Run("非相邻步进报错", func(t *testing.T) {
		if _, err := adjacentSteps(0, 2, []migration{{From: 0, To: 2}}); err == nil {
			t.Error("v0→v2 跨越一步时应报错")
		}
	})

	t.Run("回退报错", func(t *testing.T) {
		if _, err := adjacentSteps(2, 1, full); err == nil {
			t.Error("回退版本时应报错")
		}
	})
}

// TestStampSchemaVersionPreservesFields 版本戳不得丢失其余字段。
func TestStampSchemaVersionPreservesFields(t *testing.T) {
	raw := []byte(`{"id":"e1","session_id":"s1","type":"prompted","data":{"content":"hi"},"seq":7}`)
	out, err := stampSchemaVersion(raw, 1)
	if err != nil {
		t.Fatalf("盖章失败: %v", err)
	}

	var got map[string]json.RawMessage
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("解析结果失败: %v", err)
	}
	if string(got["v"]) != "1" {
		t.Errorf("v = %s，期望 1", got["v"])
	}
	if string(got["id"]) != `"e1"` {
		t.Errorf("id = %s，期望 \"e1\"", got["id"])
	}
	if string(got["seq"]) != "7" {
		t.Errorf("seq = %s，期望 7", got["seq"])
	}
	if string(got["data"]) != `{"content":"hi"}` {
		t.Errorf("data = %s，期望 {\"content\":\"hi\"}", got["data"])
	}
}

// splitNonEmptyLines 按行切分并丢弃空行。
func splitNonEmptyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}
