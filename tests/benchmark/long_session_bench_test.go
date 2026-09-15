package benchmark

// OPT-002 的可重复基准：固定事件/事实/job 数量，单独测量追加、投影、摘要、
// 历史列表和 TUI 输入状态转换。网络模型不在这些基准中，避免把外部等待混进数据。

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wly2lcl/basework/internal/jobs"
	"github.com/wly2lcl/basework/internal/tui"
	"github.com/wly2lcl/basework/pkg/session"
)

func benchmarkEventStore(b *testing.B, n int) (*session.JSONLStore, string) {
	b.Helper()
	storeDir := filepath.Join(b.TempDir(), fmt.Sprintf("projected-%d", n))
	store, err := session.NewJSONLStore(storeDir)
	if err != nil {
		b.Fatal(err)
	}
	info, err := store.Create(session.CreateOpts{Title: fmt.Sprintf("bench-%d", n)})
	if err != nil {
		b.Fatal(err)
	}
	// 预填充不计入投影计时。使用合法的 JSONL 事件直接写入，避免把
	// AppendEvent 的 O(n²) 原子重写成本重复混进每个投影样本的准备阶段。
	file, err := os.OpenFile(filepath.Join(storeDir, info.ID+".jsonl"), os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		b.Fatal(err)
	}
	enc := json.NewEncoder(file)
	data := json.RawMessage(`{"content":"fixed benchmark event payload"}`)
	for i := 0; i < n; i++ {
		if err := enc.Encode(session.Event{ID: fmt.Sprintf("bench-%d", i), SessionID: info.ID,
			Type: session.EventPrompted, Data: data, Seq: int64(i + 1),
			CreatedAt: time.Unix(int64(i), 0).UTC(), SchemaVersion: 1}); err != nil {
			file.Close()
			b.Fatal(err)
		}
	}
	if err := file.Close(); err != nil {
		b.Fatal(err)
	}
	projected, err := session.NewJSONLStore(storeDir)
	if err != nil {
		b.Fatal(err)
	}
	return projected, info.ID
}

func BenchmarkJSONLAppendEvents(b *testing.B) {
	for _, n := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("Events_%d", n), func(b *testing.B) {
			b.ReportAllocs()
			data := []byte(`{"content":"fixed benchmark event payload"}`)
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				store, err := session.NewJSONLStore(filepath.Join(b.TempDir(), fmt.Sprintf("append-%d", i)))
				if err != nil {
					b.Fatal(err)
				}
				info, err := store.Create(session.CreateOpts{Title: "append"})
				if err != nil {
					b.Fatal(err)
				}
				b.StartTimer()
				for j := 0; j < n; j++ {
					if err := store.AppendEvent(session.Event{SessionID: info.ID, Type: session.EventPrompted, Data: data}); err != nil {
						b.Fatal(err)
					}
				}
				b.StopTimer()
			}
		})
	}
}

func BenchmarkJSONLProjectEvents(b *testing.B) {
	for _, n := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("Events_%d", n), func(b *testing.B) {
			store, sessionID := benchmarkEventStore(b, n)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := store.Events(session.EventFilter{SessionID: sessionID}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkFactsSummaryRead(b *testing.B) {
	for _, n := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("Facts_%d", n), func(b *testing.B) {
			facts := session.NewWorkspaceFacts("bench-workspace")
			for i := 0; i < n; i++ {
				facts.Add(session.Fact{Kind: session.FactFileModified,
					Key:  session.FileFactKey(session.FactFileModified, fmt.Sprintf("pkg/file-%d.go", i)),
					Path: fmt.Sprintf("pkg/file-%d.go", i), Source: "benchmark"})
			}
			hash := func(string) (string, error) { return "fixed-hash", nil }
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = session.SummarizeFacts(facts, session.FactsSummaryOptions{Budget: 8 * 1024, Hash: hash})
			}
		})
	}
}

func BenchmarkJobsHistoryList(b *testing.B) {
	for _, n := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("Jobs_%d", n), func(b *testing.B) {
			manager := jobs.New(jobs.Options{})
			owner := "bench-owner"
			ids := make([]string, 0, n)
			for i := 0; i < n; i++ {
				job, err := manager.Start(owner, fmt.Sprintf("echo %d", i), func(context.Context) (int, error) { return 0, nil })
				if err != nil {
					b.Fatal(err)
				}
				ids = append(ids, job.ID)
			}
			for _, id := range ids {
				if _, err := manager.Await(context.Background(), owner, id); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = manager.List(owner)
			}
			manager.Close()
		})
	}
}

func BenchmarkTUIInputTransition(b *testing.B) {
	for i := 0; i < b.N; i++ {
		app := tui.NewApp("bench-model", "bench-provider", "bench-session")
		app.SetInputHandler(func(context.Context, string) (string, error) { return "ok", nil })
		_, _ = app.Update(tui.UserInputMsg{Text: "benchmark input"})
		app.Quit()
	}
}
