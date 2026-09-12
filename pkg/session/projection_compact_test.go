package session

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/wly2lcl/basework/pkg/llm"
)

// ev 构造一个事件，data 为 nil 时序列化为 null。
func ev(seq int64, typ EventType, data interface{}) Event {
	var raw []byte
	if data != nil {
		encoded, err := EncodeData(data)
		if err != nil {
			panic(err)
		}
		raw = encoded
	} else {
		raw = []byte("null")
	}
	return Event{
		ID:        fmt.Sprintf("e%d", seq),
		SessionID: "s",
		Type:      typ,
		Data:      raw,
		Seq:       seq,
	}
}

// compactTestLog 构造一份带压缩事件的会话日志。
//
// 不加压缩时的投影结果（含各消息来源事件 Seq）：
//
//	msgs[0] user      "a"   seq 1
//	msgs[1] assistant "x"   seq 3
//	msgs[2] user      "b"   seq 4
//	msgs[3] assistant "y"   seq 6
func compactTestLog(cd *CompactedData) []Event {
	events := []Event{
		ev(1, EventPrompted, &PromptedData{Content: "a"}),
		ev(2, EventTextDelta, &TextDeltaData{Delta: "x"}),
		ev(3, EventTextEnded, nil),
		ev(4, EventPrompted, &PromptedData{Content: "b"}),
		ev(5, EventTextDelta, &TextDeltaData{Delta: "y"}),
		ev(6, EventTextEnded, nil),
	}
	if cd != nil {
		events = append(events, ev(7, EventCompacted, cd))
	}
	return events
}

// TestProjectionWithoutCompaction 基线：无压缩时的投影与来源 Seq。
func TestProjectionWithoutCompaction(t *testing.T) {
	msgs, seqs := ProjectMessagesWithSeq(compactTestLog(nil))
	if len(msgs) != 4 {
		t.Fatalf("消息数 = %d，期望 4", len(msgs))
	}
	wantSeqs := []int64{1, 3, 4, 6}
	if !reflect.DeepEqual(seqs, wantSeqs) {
		t.Errorf("来源 Seq = %v，期望 %v", seqs, wantSeqs)
	}
}

// TestProjectionStableAcrossSchema 关键验收项：同一份日志，
// v0 数据（只有 KeepFrom）与 v1 数据（有 TruncatedSeq）必须投影出相同结果。
// 否则迁移本身就会改变历史语义。
func TestProjectionStableAcrossSchema(t *testing.T) {
	v0 := ProjectMessages(compactTestLog(&CompactedData{KeepFrom: 2}))
	v1 := ProjectMessages(compactTestLog(&CompactedData{TruncatedSeq: 3, KeepFrom: 2}))

	if !reflect.DeepEqual(v0, v1) {
		t.Fatalf("v0 与 v1 投影结果不一致：\nv0 = %+v\nv1 = %+v", v0, v1)
	}
	if len(v1) != 2 {
		t.Fatalf("保留消息数 = %d，期望 2", len(v1))
	}
	if got := v1[0].Content[0].Text; got != "b" {
		t.Errorf("首条保留消息 = %q，期望 \"b\"", got)
	}
}

// TestProjectionTruncatedSeqWinsOverStaleKeepFrom 稳定锚点优先于位置索引。
//
// 这里故意把 KeepFrom 写成过期的值（1），模拟"用旧投影规则算出的下标"。
// 锚点是事件序号，不受消息条数变化影响，因此应当无视这个错误下标。
func TestProjectionTruncatedSeqWinsOverStaleKeepFrom(t *testing.T) {
	// 仅用 KeepFrom=1 会保留 3 条
	stale := ProjectMessages(compactTestLog(&CompactedData{KeepFrom: 1}))
	if len(stale) != 3 {
		t.Fatalf("仅 KeepFrom 时保留 %d 条，期望 3（用于对照）", len(stale))
	}

	// 锚点正确时，即使 KeepFrom 是过期值，也应按事件序号截断
	anchored := ProjectMessages(compactTestLog(&CompactedData{TruncatedSeq: 3, KeepFrom: 1}))
	if len(anchored) != 2 {
		t.Fatalf("有锚点时保留 %d 条，期望 2（锚点应覆盖过期的 KeepFrom）", len(anchored))
	}
	if got := anchored[0].Content[0].Text; got != "b" {
		t.Errorf("首条保留消息 = %q，期望 \"b\"", got)
	}
}

// TestProjectionSummaryInserted 摘要必须插回历史头部，否则压缩等于静默丢消息。
func TestProjectionSummaryInserted(t *testing.T) {
	cd := &CompactedData{Summary: "我们讨论了 a 和 b", TruncatedSeq: 3, KeepFrom: 2}
	msgs := ProjectMessages(compactTestLog(cd))

	if len(msgs) != 3 {
		t.Fatalf("消息数 = %d，期望 3（1 条摘要 + 2 条保留）", len(msgs))
	}
	first := msgs[0]
	if first.Role != "user" {
		t.Errorf("摘要消息角色 = %q，期望 user", first.Role)
	}
	if len(first.Content) == 0 || first.Content[0].Type != "text" {
		t.Fatalf("摘要消息内容类型异常: %+v", first.Content)
	}
	want := SummaryMessagePrefix + "我们讨论了 a 和 b"
	if got := first.Content[0].Text; got != want {
		t.Errorf("摘要消息文本 = %q，期望 %q", got, want)
	}

	// 摘要必须排在被保留的历史之前
	if got := msgs[1].Content[0].Text; got != "b" {
		t.Errorf("摘要之后的首条消息 = %q，期望 \"b\"", got)
	}
}

// TestProjectionNoSummaryNoInsert 摘要为空时不得凭空插入消息。
func TestProjectionNoSummaryNoInsert(t *testing.T) {
	msgs := ProjectMessages(compactTestLog(&CompactedData{TruncatedSeq: 3, KeepFrom: 2}))
	for i, m := range msgs {
		if len(m.Content) > 0 && m.Content[0].Type == "text" &&
			len(m.Content[0].Text) >= len(SummaryMessagePrefix) &&
			m.Content[0].Text[:len(SummaryMessagePrefix)] == SummaryMessagePrefix {
			t.Errorf("第 %d 条消息出现了摘要前缀，但 Summary 为空: %q", i, m.Content[0].Text)
		}
	}
}

// TestProjectionFallsBackWhenSeqMissing 事件未设 Seq 时（seqs 全 0），
// 锚点无法定位，必须退回 KeepFrom，而不是把历史清空。
func TestProjectionFallsBackWhenSeqMissing(t *testing.T) {
	// Seq 全为 0
	events := []Event{
		ev(0, EventPrompted, &PromptedData{Content: "a"}),
		ev(0, EventTextDelta, &TextDeltaData{Delta: "x"}),
		ev(0, EventTextEnded, nil),
		ev(0, EventPrompted, &PromptedData{Content: "b"}),
		ev(0, EventCompacted, &CompactedData{TruncatedSeq: 99, KeepFrom: 2}),
	}
	msgs := ProjectMessages(events)
	// 压缩前的投影是 [user a, assistant x, user b]，共 3 条；
	// KeepFrom=2 应保留 1 条，而不是因为锚点失效把历史清空。
	if len(msgs) != 1 {
		t.Fatalf("消息数 = %d，期望 1（应退回 KeepFrom 而不是清空历史）", len(msgs))
	}
	if got := msgs[0].Content[0].Text; got != "b" {
		t.Errorf("首条消息 = %q，期望 \"b\"", got)
	}
}

// TestProjectionSecondCompactionSupersedesFirst 后一次压缩应取代前一次的摘要，
// 不能出现两份摘要叠加。
func TestProjectionSecondCompactionSupersedesFirst(t *testing.T) {
	events := compactTestLog(&CompactedData{Summary: "旧摘要", TruncatedSeq: 3, KeepFrom: 2})
	// 继续追加，然后再次压缩，把"旧摘要 + 之后的对话"一起压掉
	events = append(events,
		ev(8, EventPrompted, &PromptedData{Content: "c"}),
		ev(9, EventTextDelta, &TextDeltaData{Delta: "z"}),
		ev(10, EventTextEnded, nil),
	)
	// 上一次压缩后 msgs = [user(旧摘要, seq=锚点3), user b(seq4), assistant y(seq6)]
	// 再追加 c/z 后共 5 条，第二次压缩以 TruncatedSeq=6 截断 → 只剩 [c, z]，
	// 旧的摘要应当与更早的历史一起被丢弃。
	events = append(events, ev(11, EventCompacted, &CompactedData{
		Summary:      "新摘要",
		TruncatedSeq: 6,
		KeepFrom:     3,
	}))

	msgs := ProjectMessages(events)

	var summaries []string
	for _, m := range msgs {
		if len(m.Content) > 0 && m.Content[0].Type == "text" &&
			len(m.Content[0].Text) >= len(SummaryMessagePrefix) &&
			m.Content[0].Text[:len(SummaryMessagePrefix)] == SummaryMessagePrefix {
			summaries = append(summaries, m.Content[0].Text)
		}
	}
	if len(summaries) != 1 {
		t.Fatalf("摘要条数 = %d，期望 1（后一次压缩应取代前一次）：%v", len(summaries), summaries)
	}
	if want := SummaryMessagePrefix + "新摘要"; summaries[0] != want {
		t.Errorf("摘要 = %q，期望 %q", summaries[0], want)
	}
}

// TestProjectionCompactionSnapshotReplaysExactMessages 记录的快照应按压缩器
// 的完整输出重放，包括任意保留和重排，而不是再次套用截断逻辑。
func TestProjectionCompactionSnapshotReplaysExactMessages(t *testing.T) {
	snapshot := []llm.ChatMessage{
		{
			Role: llm.RoleAssistant,
			Content: []llm.ContentPart{
				{Type: llm.ContentTypeText, Text: "保留的后段"},
				{Type: llm.ContentTypeImage, ImageURL: "https://example.test/image.png"},
			},
			ToolCalls: []llm.ToolCall{{ID: "call-1", Name: "lookup", ArgsJSON: `{"q":"x"}`}},
		},
		{
			Role:    llm.RoleUser,
			Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "被选择并移动到末尾的早期消息"}},
		},
	}
	events := compactTestLog(nil)
	events = append(events, ev(7, EventCompacted, &CompactedData{
		Summary:      "旧路径不应额外插入的摘要",
		TruncatedSeq: 6,
		KeepFrom:     99,
		Snapshot:     snapshot,
	}))

	got, seqs := ProjectMessagesWithSeq(events)
	if !reflect.DeepEqual(got, snapshot) {
		t.Fatalf("投影未精确重放快照：\ngot  = %#v\nwant = %#v", got, snapshot)
	}
	if want := []int64{7, 7}; !reflect.DeepEqual(seqs, want) {
		t.Fatalf("快照来源 Seq = %v，期望 %v", seqs, want)
	}
}

// TestProjectionEmptyCompactionSnapshotClearsHistory 验证显式 [] 与缺失快照不同：
// [] 表示压缩器要求清空，而不是走旧的 KeepFrom 回退逻辑。
func TestProjectionEmptyCompactionSnapshotClearsHistory(t *testing.T) {
	events := compactTestLog(nil)
	events = append(events, ev(7, EventCompacted, &CompactedData{
		Summary:  "不应插入",
		KeepFrom: 1,
		Snapshot: []llm.ChatMessage{},
	}))

	got, seqs := ProjectMessagesWithSeq(events)
	if len(got) != 0 || len(seqs) != 0 {
		t.Fatalf("空快照后仍有历史：messages=%#v seqs=%v", got, seqs)
	}
}

// TestProjectionLegacyCompactionWithoutSnapshot 保证没有 snapshot 字段的老事件
// 仍使用 Summary、TruncatedSeq 和 KeepFrom 的既有兼容逻辑。
func TestProjectionLegacyCompactionWithoutSnapshot(t *testing.T) {
	legacy := ev(7, EventCompacted, nil)
	legacy.Data = []byte(`{"summary":"legacy summary","truncated_seq":3,"keep_from":1}`)
	events := append(compactTestLog(nil), legacy)

	got, seqs := ProjectMessagesWithSeq(events)
	if len(got) != 3 {
		t.Fatalf("旧格式投影消息数 = %d，期望 3（摘要 + 两条锚点后的历史）", len(got))
	}
	if got[0].Content[0].Text != SummaryMessagePrefix+"legacy summary" {
		t.Fatalf("旧格式摘要 = %#v", got[0])
	}
	if got[1].Content[0].Text != "b" || got[2].Content[0].Text != "y" {
		t.Fatalf("旧格式锚点投影不符：%#v", got[1:])
	}
	if want := []int64{3, 4, 6}; !reflect.DeepEqual(seqs, want) {
		t.Fatalf("旧格式来源 Seq = %v，期望 %v", seqs, want)
	}
}

// TestProjectionConsecutiveCompactionSnapshots 使用完整快照连续压缩时，后一份
// 输出应完全取代前一份，不留下前一份摘要或消息。
func TestProjectionConsecutiveCompactionSnapshots(t *testing.T) {
	first := []llm.ChatMessage{{
		Role:    llm.RoleUser,
		Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "first snapshot"}},
	}}
	second := []llm.ChatMessage{
		{Role: llm.RoleAssistant, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "second snapshot A"}}},
		{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "second snapshot B"}}},
	}
	events := []Event{
		ev(1, EventPrompted, &PromptedData{Content: "original history"}),
		ev(2, EventCompacted, &CompactedData{Summary: "first summary", Snapshot: first}),
		ev(3, EventPrompted, &PromptedData{Content: "message after first compaction"}),
		ev(4, EventCompacted, &CompactedData{Summary: "second summary", Snapshot: second}),
	}

	got, seqs := ProjectMessagesWithSeq(events)
	if !reflect.DeepEqual(got, second) {
		t.Fatalf("连续压缩未由后一个快照完整替换：\ngot  = %#v\nwant = %#v", got, second)
	}
	if want := []int64{4, 4}; !reflect.DeepEqual(seqs, want) {
		t.Fatalf("最后快照来源 Seq = %v，期望 %v", seqs, want)
	}
}
