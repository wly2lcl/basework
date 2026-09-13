package session

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func factsFixture(t *testing.T) *WorkspaceFacts {
	t.Helper()
	w := NewWorkspaceFacts("ws")
	base := time.Date(2026, 9, 12, 9, 0, 0, 0, time.UTC)
	w.Add(Fact{Kind: FactFileModified, Key: FileFactKey(FactFileModified, "a.go"), Path: "a.go", Ref: "plan-a", Source: "edit_files", FirstSeen: base.Format(time.RFC3339)})
	w.Add(Fact{Kind: FactFileRead, Key: FileFactKey(FactFileRead, "b.go"), Path: "b.go", Ref: "sess-1", Source: "read", FirstSeen: base.Add(-time.Hour).Format(time.RFC3339)})
	w.Add(Fact{Kind: FactCommand, Key: CommandFactKey("job-1"), Ref: "job-1", Source: "job", FirstSeen: base.Format(time.RFC3339)})
	return w
}

func TestSummarizeFacts_IncludesSourceAndOrder(t *testing.T) {
	w := factsFixture(t)
	sum := SummarizeFacts(w, FactsSummaryOptions{})
	if sum.Text == "" || sum.Included != 3 {
		t.Fatalf("应包含全部 3 条: %+v", sum)
	}
	if !strings.Contains(sum.Text, "[file_modified] a.go") {
		t.Errorf("缺修改事实: %s", sum.Text)
	}
	if !strings.Contains(sum.Text, "来源 edit_files") {
		t.Errorf("每行必须带来源（可追踪）: %s", sum.Text)
	}
	// 相关性排序：modified 行在 read 行之前
	if strings.Index(sum.Text, "a.go") > strings.Index(sum.Text, "b.go") {
		t.Errorf("modified 应排在 read 之前: %s", sum.Text)
	}
}

func TestSummarizeFacts_BudgetTruncates(t *testing.T) {
	w := NewWorkspaceFacts("ws")
	for i := 0; i < 50; i++ {
		p := "file" + string(rune('a'+i)) + ".go"
		w.Add(Fact{Kind: FactFileModified, Key: FileFactKey(FactFileModified, p), Path: p, Ref: "p", Source: "edit_files"})
	}
	sum := SummarizeFacts(w, FactsSummaryOptions{Budget: 300})
	if !sum.Truncated {
		t.Fatal("预算内放不下应截断")
	}
	if sum.Included >= 50 {
		t.Fatalf("截断后不应全量包含: %d", sum.Included)
	}
	if len(sum.Text) > 300+200 { // 截断说明行有额外开销
		t.Fatalf("摘要明显超预算: %d 字节", len(sum.Text))
	}
}

func TestSummarizeFacts_StaleDetection(t *testing.T) {
	w := factsFixture(t)
	h1, h2 := "hash-1", "hash-2"
	tracker := NewStaleTracker()
	// 第一次构建建立基线。
	SummarizeFacts(w, FactsSummaryOptions{
		Stale: tracker,
		Hash: func(path string) (string, error) {
			if path == "a.go" {
				return h1, nil
			}
			return "", errors.New("skip")
		},
	})
	// 第二次构建（共享 tracker）：a.go 的哈希已变，应标过期。
	sum2 := SummarizeFacts(w, FactsSummaryOptions{
		Stale: tracker,
		Hash: func(path string) (string, error) {
			if path == "a.go" {
				return h2, nil
			}
			return "", errors.New("skip")
		},
	})
	if sum2.Stale != 1 {
		t.Fatalf("文件变化后应标 1 条过期: %+v", sum2)
	}
	if !strings.Contains(sum2.Text, "[已过期:文件随后又变化") {
		t.Errorf("过期事实应有标识: %s", sum2.Text)
	}
}

func TestSummarizeFacts_UnreadableNotStale(t *testing.T) {
	w := factsFixture(t)
	sum := SummarizeFacts(w, FactsSummaryOptions{
		Hash: func(string) (string, error) {
			return "", errors.New("protected: 拒绝")
		},
	})
	if sum.Stale != 0 {
		t.Fatalf("读不了的文件不得判过期: %+v", sum)
	}
	if sum.Unreadable == 0 {
		t.Fatal("读不了的文件应计入未读取")
	}
	if !strings.Contains(sum.Text, "[未读取]") {
		t.Errorf("应标注未读取: %s", sum.Text)
	}
}

func TestSummarizeFacts_EmptyFacts(t *testing.T) {
	w := NewWorkspaceFacts("ws")
	sum := SummarizeFacts(w, FactsSummaryOptions{})
	if sum.Text != "" || sum.Total != 0 {
		t.Fatalf("空事实应得空摘要: %+v", sum)
	}
}

func TestSummarizeFacts_NoRepoScan_Structural(t *testing.T) {
	// 结构性保证：SummarizeFacts 只消费传入的事实集合，没有任何
	// 目录遍历入口。这里验证即使工作区目录不存在也能正常生成摘要。
	w := factsFixture(t)
	sum := SummarizeFacts(w, FactsSummaryOptions{Hash: func(string) (string, error) {
		return "", errors.New("no fs access")
	}})
	if sum.Included == 0 {
		t.Fatal("无文件系统访问也应能生成摘要")
	}
}
