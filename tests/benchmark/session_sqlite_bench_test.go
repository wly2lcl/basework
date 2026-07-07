//go:build sqlite

package benchmark

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/wly2lcl/basework/pkg/session"
)

// BenchmarkSQLiteStoreCreate 测试 SQLiteStore.Create 的吞吐量。
func BenchmarkSQLiteStoreCreate(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		dbPath := filepath.Join(b.TempDir(), fmt.Sprintf("bench_create_%d.db", i))
		s, err := session.NewSQLiteStore(dbPath)
		if err != nil {
			b.Fatalf("NewSQLiteStore 失败: %v", err)
		}
		b.StartTimer()

		_, err = s.Create(session.CreateOpts{Title: "基准测试"})
		if err != nil {
			b.Fatalf("Create 失败: %v", err)
		}

		b.StopTimer()
		s.Close()
	}
}

// BenchmarkSQLiteStore_AppendEvent 测试 SQLiteStore.AppendEvent 的写入吞吐量。
func BenchmarkSQLiteStore_AppendEvent(b *testing.B) {
	b.StopTimer()
	dbPath := filepath.Join(b.TempDir(), "bench_append.db")
	s, err := session.NewSQLiteStore(dbPath)
	if err != nil {
		b.Fatalf("NewSQLiteStore 失败: %v", err)
	}
	defer s.Close()

	info, err := s.Create(session.CreateOpts{Title: "基准测试"})
	if err != nil {
		b.Fatalf("Create 失败: %v", err)
	}

	eventData := mustMarshalSessionSQLite(b, map[string]string{"content": "测试消息"})
	b.StartTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		err := s.AppendEvent(session.Event{
			SessionID: info.ID,
			Type:      session.EventPrompted,
			Data:      eventData,
		})
		if err != nil {
			b.Fatalf("AppendEvent 失败: %v", err)
		}
	}
}

// BenchmarkSQLiteStore_Read 测试 SQLiteStore.Events 的读取性能。
func BenchmarkSQLiteStore_Read(b *testing.B) {
	b.StopTimer()
	dbPath := filepath.Join(b.TempDir(), "bench_read.db")
	s, err := session.NewSQLiteStore(dbPath)
	if err != nil {
		b.Fatalf("NewSQLiteStore 失败: %v", err)
	}
	defer s.Close()

	info, err := s.Create(session.CreateOpts{Title: "基准测试"})
	if err != nil {
		b.Fatalf("Create 失败: %v", err)
	}

	// 预填充 1000 条事件
	eventData := mustMarshalSessionSQLite(b, map[string]string{"content": "测试"})
	for i := 0; i < 1000; i++ {
		err := s.AppendEvent(session.Event{
			SessionID: info.ID,
			Type:      session.EventTextDelta,
			Data:      eventData,
		})
		if err != nil {
			b.Fatalf("AppendEvent 失败: %v", err)
		}
	}

	b.StartTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := s.Events(session.EventFilter{SessionID: info.ID})
		if err != nil {
			b.Fatalf("Events 失败: %v", err)
		}
	}
}

// BenchmarkSQLiteStore_Write_100Events 测试写入 100 个事件的性能
// 与 pkg/session/bench_test.go 中的 BenchmarkWALMode_Write 类似。
func BenchmarkSQLiteStore_Write_100Events(b *testing.B) {
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		dbPath := filepath.Join(b.TempDir(), "bench_100_write.db")
		s, err := session.NewSQLiteStore(dbPath)
		if err != nil {
			b.Fatalf("NewSQLiteStore 失败: %v", err)
		}

		info, err := s.Create(session.CreateOpts{Title: "基准测试"})
		if err != nil {
			b.Fatalf("Create 失败: %v", err)
		}

		eventData := mustMarshalSessionSQLite(b, map[string]string{
			"content": "benchmark message for WAL mode write performance testing",
		})
		b.StartTimer()

		const eventsPerIteration = 100
		for j := 0; j < eventsPerIteration; j++ {
			err := s.AppendEvent(session.Event{
				SessionID: info.ID,
				Type:      session.EventPrompted,
				Data:      eventData,
			})
			if err != nil {
				b.Fatalf("AppendEvent 失败: %v", err)
			}
		}

		b.StopTimer()
		s.Close()
	}
}

// BenchmarkSQLiteStore_ConcurrentWrite 测试不同并发度下 SQLiteStore 的写入性能。
func BenchmarkSQLiteStore_ConcurrentWrite(b *testing.B) {
	concurrencyLevels := []int{1, 2, 4, 8}

	for _, concurrency := range concurrencyLevels {
		b.Run(fmt.Sprintf("Concurrency_%d", concurrency), func(b *testing.B) {
			b.StopTimer()
			dbPath := filepath.Join(b.TempDir(), "bench_conc_write.db")
			s, err := session.NewSQLiteStore(dbPath)
			if err != nil {
				b.Fatalf("NewSQLiteStore 失败: %v", err)
			}
			defer s.Close()

			info, err := s.Create(session.CreateOpts{Title: "并发写入基准"})
			if err != nil {
				b.Fatalf("Create 失败: %v", err)
			}

			eventData := mustMarshalSessionSQLite(b, map[string]string{"delta": "concurrent"})
			b.StartTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				var wg sync.WaitGroup
				for j := 0; j < concurrency; j++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						_ = s.AppendEvent(session.Event{
							SessionID: info.ID,
							Type:      session.EventTextDelta,
							Data:      eventData,
						})
					}()
				}
				wg.Wait()
			}
		})
	}
}

// 辅助函数
func mustMarshalSessionSQLite(b *testing.B, v any) json.RawMessage {
	b.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		b.Fatalf("json.Marshal 失败: %v", err)
	}
	return data
}
