package benchmark

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/wly2lcl/basework/pkg/session"
)

// BenchmarkSessionCreate 测试 MemoryStore.Create 的吞吐量。
func BenchmarkSessionCreate(b *testing.B) {
	store := session.NewMemoryStore()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := store.Create(session.CreateOpts{Title: "基准测试会话"})
		if err != nil {
			b.Fatalf("Create 失败: %v", err)
		}
	}
}

// BenchmarkSessionAppendEvent 测试 MemoryStore.AppendEvent 的写入吞吐量。
func BenchmarkSessionAppendEvent(b *testing.B) {
	store := session.NewMemoryStore()
	info, err := store.Create(session.CreateOpts{Title: "基准测试"})
	if err != nil {
		b.Fatalf("Create 失败: %v", err)
	}

	eventData := mustMarshalSession(b, map[string]string{"content": "测试消息"})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		err := store.AppendEvent(session.Event{
			SessionID: info.ID,
			Type:      session.EventPrompted,
			Data:      eventData,
		})
		if err != nil {
			b.Fatalf("AppendEvent 失败: %v", err)
		}
	}
}

// BenchmarkSessionRead 测试 MemoryStore.Events 的读取性能。
// 预先写入大量事件后，测量带过滤条件的事件查询性能。
func BenchmarkSessionRead(b *testing.B) {
	store := session.NewMemoryStore()
	info, err := store.Create(session.CreateOpts{Title: "基准测试"})
	if err != nil {
		b.Fatalf("Create 失败: %v", err)
	}

	// 预填充 1000 条事件
	const eventCount = 1000
	eventData := mustMarshalSession(b, map[string]string{"content": "测试"})
	for i := 0; i < eventCount; i++ {
		eventType := session.EventPrompted
		if i%2 == 0 {
			eventType = session.EventTextDelta
		}
		err := store.AppendEvent(session.Event{
			SessionID: info.ID,
			Type:      eventType,
			Data:      eventData,
		})
		if err != nil {
			b.Fatalf("AppendEvent 失败: %v", err)
		}
	}

	// 测试不同查询模式
	b.Run("AllEvents", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := store.Events(session.EventFilter{SessionID: info.ID})
			if err != nil {
				b.Fatalf("Events 失败: %v", err)
			}
		}
	})

	b.Run("FilteredByType", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := store.Events(session.EventFilter{
				SessionID: info.ID,
				Types:     []session.EventType{session.EventPrompted},
			})
			if err != nil {
				b.Fatalf("Events 失败: %v", err)
			}
		}
	})

	b.Run("WithLimit", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := store.Events(session.EventFilter{
				SessionID: info.ID,
				Limit:     50,
			})
			if err != nil {
				b.Fatalf("Events 失败: %v", err)
			}
		}
	})

	b.Run("AfterSeq", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := store.Events(session.EventFilter{
				SessionID: info.ID,
				AfterSeq:  500,
			})
			if err != nil {
				b.Fatalf("Events 失败: %v", err)
			}
		}
	})
}

// BenchmarkSessionList 测试 MemoryStore.List 的列举性能。
// 预先创建大量会话后，测量分页列举的性能。
func BenchmarkSessionList(b *testing.B) {
	store := session.NewMemoryStore()

	// 预创建 100 个会话
	const sessionCount = 100
	for i := 0; i < sessionCount; i++ {
		_, err := store.Create(session.CreateOpts{
			Title: fmt.Sprintf("会话 %d", i),
		})
		if err != nil {
			b.Fatalf("Create 失败: %v", err)
		}
	}

	b.Run("ListAll", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := store.List(session.ListFilter{})
			if err != nil {
				b.Fatalf("List 失败: %v", err)
			}
		}
	})

	b.Run("ListWithLimit10", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := store.List(session.ListFilter{Limit: 10})
			if err != nil {
				b.Fatalf("List 失败: %v", err)
			}
		}
	})

	b.Run("ListWithOffset", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := store.List(session.ListFilter{Offset: 50, Limit: 10})
			if err != nil {
				b.Fatalf("List 失败: %v", err)
			}
		}
	})
}

// BenchmarkSessionGet 测试 MemoryStore.Get 的读取性能。
// 预先创建会话后，测量按 ID 查询的性能。
func BenchmarkSessionGet(b *testing.B) {
	store := session.NewMemoryStore()
	info, err := store.Create(session.CreateOpts{Title: "测试会话"})
	if err != nil {
		b.Fatalf("Create 失败: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := store.Get(info.ID)
		if err != nil {
			b.Fatalf("Get 失败: %v", err)
		}
	}
}

// BenchmarkSessionDelete 测试 MemoryStore.Delete 的删除性能。
// 预先创建会话后，测量删除操作的性能。
func BenchmarkSessionDelete(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		store := session.NewMemoryStore()
		info, err := store.Create(session.CreateOpts{Title: "待删除"})
		if err != nil {
			b.Fatalf("Create 失败: %v", err)
		}
		b.StartTimer()

		err = store.Delete(info.ID)
		if err != nil {
			b.Fatalf("Delete 失败: %v", err)
		}
	}
}

// BenchmarkSessionConcurrent 测试 MemoryStore 的并发读写性能。
// 多个 goroutine 同时进行事件追加和会话列举。
func BenchmarkSessionConcurrent(b *testing.B) {
	store := session.NewMemoryStore()
	info, err := store.Create(session.CreateOpts{Title: "并发基准"})
	if err != nil {
		b.Fatalf("Create 失败: %v", err)
	}

	writerCount := 4
	readerCount := 4
	eventData := mustMarshalSession(b, map[string]string{"delta": "concurrent"})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		var wg sync.WaitGroup
		errCh := make(chan error, writerCount+readerCount)

		// 写入 goroutines
		for w := 0; w < writerCount; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				err := store.AppendEvent(session.Event{
					SessionID: info.ID,
					Type:      session.EventTextDelta,
					Data:      eventData,
				})
				if err != nil {
					errCh <- err
				}
			}()
		}

		// 读取 goroutines
		for r := 0; r < readerCount; r++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := store.List(session.ListFilter{Limit: 10})
				if err != nil {
					errCh <- err
				}
				_, err = store.Get(info.ID)
				if err != nil {
					errCh <- err
				}
			}()
		}

		wg.Wait()
		close(errCh)

		for err := range errCh {
			if err != nil {
				b.Errorf("并发操作失败: %v", err)
			}
		}
	}
}

// BenchmarkSessionConcurrent_DifferentLoad 测试不同并发度下的性能。
func BenchmarkSessionConcurrent_DifferentLoad(b *testing.B) {
	loadLevels := []struct {
		name     string
		writers  int
		readers  int
	}{
		{"Light_2W1R", 2, 1},
		{"Medium_4W4R", 4, 4},
		{"Heavy_8W8R", 8, 8},
	}

	for _, load := range loadLevels {
		b.Run(load.name, func(b *testing.B) {
			store := session.NewMemoryStore()
			info, err := store.Create(session.CreateOpts{Title: "并发负载测试"})
			if err != nil {
				b.Fatalf("Create 失败: %v", err)
			}

			eventData := mustMarshalSession(b, map[string]string{"delta": "concurrent"})

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				var wg sync.WaitGroup

				for w := 0; w < load.writers; w++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						_ = store.AppendEvent(session.Event{
							SessionID: info.ID,
							Type:      session.EventTextDelta,
							Data:      eventData,
						})
					}()
				}

				for r := 0; r < load.readers; r++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						_, _ = store.List(session.ListFilter{Limit: 10})
					}()
				}

				wg.Wait()
			}
		})
	}
}

// 辅助函数
func mustMarshalSession(b *testing.B, v any) json.RawMessage {
	b.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		b.Fatalf("json.Marshal 失败: %v", err)
	}
	return data
}