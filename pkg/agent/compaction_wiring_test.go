package agent

import (
	"errors"
	"testing"

	"github.com/wly2lcl/basework/pkg/llm"
)

// fakeCompactor 只实现 Compactor，不实现 CompactReporter——代表"已有的自定义压缩器"。
type fakeCompactor struct {
	dropped int
	err     error
}

func (f *fakeCompactor) ShouldCompact([]llm.ChatMessage, int) bool { return true }

func (f *fakeCompactor) Compact(h []llm.ChatMessage) ([]llm.ChatMessage, error) {
	if f.err != nil {
		return nil, f.err
	}
	return h[:len(h)-f.dropped], nil
}

// fakeReportingCompactor 额外实现 CompactReporter，代表"升级后的压缩器"。
type fakeReportingCompactor struct {
	fakeCompactor
	summary string
}

func (f *fakeReportingCompactor) CompactWithReport(h []llm.ChatMessage) (CompactReport, error) {
	if f.err != nil {
		return CompactReport{}, f.err
	}
	return CompactReport{Messages: h[:len(h)-f.dropped], Summary: f.summary}, nil
}

func threeMessages() []llm.ChatMessage {
	return []llm.ChatMessage{
		{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "1"}}},
		{Role: llm.RoleAssistant, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "2"}}},
		{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "3"}}},
	}
}

// TestRunCompactionWithoutReporter 未实现 CompactReporter 时按旧行为工作：
// 压缩照常进行，摘要为空。这保证已有嵌入方不会被破坏。
func TestRunCompactionWithoutReporter(t *testing.T) {
	a := &AgentLoop{compactor: &fakeCompactor{dropped: 1}}

	msgs, summary, err := a.runCompaction(threeMessages())
	if err != nil {
		t.Fatalf("意外错误: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("压缩后消息数 = %d，期望 2", len(msgs))
	}
	if summary != "" {
		t.Errorf("摘要 = %q，期望为空（该压缩器不上报摘要）", summary)
	}
}

// TestRunCompactionWithReporter 实现 CompactReporter 时摘要必须透传出来。
// 这条链路断了，压缩产生的摘要就永远落不了盘。
func TestRunCompactionWithReporter(t *testing.T) {
	a := &AgentLoop{compactor: &fakeReportingCompactor{
		fakeCompactor: fakeCompactor{dropped: 1},
		summary:       "被压缩掉的内容摘要",
	}}

	msgs, summary, err := a.runCompaction(threeMessages())
	if err != nil {
		t.Fatalf("意外错误: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("压缩后消息数 = %d，期望 2", len(msgs))
	}
	if summary != "被压缩掉的内容摘要" {
		t.Errorf("摘要 = %q，期望透传 %q", summary, "被压缩掉的内容摘要")
	}
}

// TestRunCompactionPropagatesError 压缩失败必须向上传播，不得静默当作未压缩。
func TestRunCompactionPropagatesError(t *testing.T) {
	wantErr := errors.New("boom")

	for name, c := range map[string]Compactor{
		"普通压缩器":  &fakeCompactor{dropped: 1, err: wantErr},
		"带报告压缩器": &fakeReportingCompactor{fakeCompactor: fakeCompactor{dropped: 1, err: wantErr}},
	} {
		t.Run(name, func(t *testing.T) {
			a := &AgentLoop{compactor: c}
			if _, _, err := a.runCompaction(threeMessages()); !errors.Is(err, wantErr) {
				t.Errorf("错误 = %v，期望 %v", err, wantErr)
			}
		})
	}
}

// TestEngineCompactInterfaceCompliance 编译期已保证，这里做个运行期兜底断言：
// 压缩器应当同时满足 Compactor，实现方可以选择性满足 CompactReporter。
func TestCompactorInterfaces(t *testing.T) {
	var _ Compactor = (*fakeCompactor)(nil)
	var _ Compactor = (*fakeReportingCompactor)(nil)
	var _ CompactReporter = (*fakeReportingCompactor)(nil)

	// 仅实现 Compactor 的类型不应被误判为 CompactReporter。
	if _, ok := any(&fakeCompactor{}).(CompactReporter); ok {
		t.Error("fakeCompactor 不应满足 CompactReporter")
	}
	if _, ok := any(&fakeReportingCompactor{}).(CompactReporter); !ok {
		t.Error("fakeReportingCompactor 应满足 CompactReporter")
	}
}
