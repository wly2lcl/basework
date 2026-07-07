package session

import (
	"sync"
	"testing"
)

func TestJSONLStore_ConcurrentAccess(t *testing.T) {
	dir := t.TempDir()
	s, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("NewJSONLStore 失败: %v", err)
	}

	// 创建测试会话
	info, err := s.Create(CreateOpts{Title: "concurrent-test"})
	if err != nil {
		t.Fatalf("Create 失败: %v", err)
	}

	// 先写入一些事件供读取
	for i := 0; i < 5; i++ {
		err := s.AppendEvent(Event{
			SessionID: info.ID,
			Type:      EventTextDelta,
			Data:      rawJSON(t, TextDeltaData{Delta: "init data"}),
		})
		if err != nil {
			t.Fatalf("AppendEvent 初始化失败: %v", err)
		}
	}

	var wg sync.WaitGroup
	iterations := 100

	for g := 0; g < 10; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				switch gid % 3 {
				case 0: // Get
					_, err := s.Get(info.ID)
					if err != nil {
						t.Errorf("Get 失败: %v", err)
						return
					}
				case 1: // Events
					events, err := s.Events(EventFilter{SessionID: info.ID})
					if err != nil {
						t.Errorf("Events 失败: %v", err)
						return
					}
					// 验证事件数据完整性
					if len(events) < 5 {
						t.Errorf("Events 返回 %d 个事件，期望至少 5 个", len(events))
						return
					}
				case 2: // List
					all, err := s.List(ListFilter{})
					if err != nil {
						t.Errorf("List 失败: %v", err)
						return
					}
					if len(all) != 1 {
						t.Errorf("List 返回 %d 个会话，期望 1 个", len(all))
						return
					}
				}
			}
		}(g)
	}

	// 同时运行 AppendEvent 的 goroutine
	for g := 0; g < 3; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				err := s.AppendEvent(Event{
					SessionID: info.ID,
					Type:      EventTextDelta,
					Data:      rawJSON(t, TextDeltaData{Delta: "concurrent data"}),
				})
				if err != nil {
					t.Errorf("AppendEvent 失败: %v", err)
					return
				}
			}
		}()
	}

	wg.Wait()

	// 验证最终数据一致性
	events, err := s.Events(EventFilter{SessionID: info.ID})
	if err != nil {
		t.Fatalf("最终验证 Events 失败: %v", err)
	}

	expected := 5 + 3*iterations // 初始 + 并发追加
	if len(events) != expected {
		t.Fatalf("最终期望 %d 个事件，得到 %d", expected, len(events))
	}

	// 验证 Seq 连续且无重复
	seqMap := make(map[int64]bool)
	for _, e := range events {
		if seqMap[e.Seq] {
			t.Fatalf("发现重复 Seq: %d", e.Seq)
		}
		seqMap[e.Seq] = true
	}
	for i := 1; i <= expected; i++ {
		if !seqMap[int64(i)] {
			t.Fatalf("Seq %d 缺失", i)
		}
	}

	// 验证 Get/List 仍正常工作
	info2, err := s.Get(info.ID)
	if err != nil {
		t.Fatalf("并发后 Get 失败: %v", err)
	}
	if info2.ID != info.ID {
		t.Fatalf("Get 返回错误的 ID: %s", info2.ID)
	}

	all, err := s.List(ListFilter{})
	if err != nil {
		t.Fatalf("并发后 List 失败: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("并发后 List 返回 %d 个会话，期望 1 个", len(all))
	}
}

func TestJSONLStore_ConcurrentCacheHitRace(t *testing.T) {
	// 专门测试 Get 的缓存命中 + 缓存未命中 + 并发写入的竞态
	dir := t.TempDir()
	s, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("NewJSONLStore 失败: %v", err)
	}

	info, err := s.Create(CreateOpts{Title: "race-test"})
	if err != nil {
		t.Fatalf("Create 失败: %v", err)
	}

	// 写入一些事件
	for i := 0; i < 3; i++ {
		s.AppendEvent(Event{
			SessionID: info.ID,
			Type:      EventTextDelta,
			Data:      rawJSON(t, TextDeltaData{Delta: "data"}),
		})
	}

	var wg sync.WaitGroup

	// 多 goroutine 反复 Get（触发缓存读写）
	for g := 0; g < 5; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				_, err := s.Get(info.ID)
				if err != nil {
					t.Errorf("Get 失败: %v", err)
					return
				}
			}
		}()
	}

	// 同时 AppendEvent（触发缓存写入）
	for g := 0; g < 3; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 30; i++ {
				s.AppendEvent(Event{
					SessionID: info.ID,
					Type:      EventTextDelta,
					Data:      rawJSON(t, TextDeltaData{Delta: "more"}),
				})
			}
		}()
	}

	// 同时 List（遍历所有文件，触发 loadSession 和缓存写入）
	for g := 0; g < 3; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				_, err := s.List(ListFilter{})
				if err != nil {
					t.Errorf("List 失败: %v", err)
					return
				}
			}
		}()
	}

	// 同时 Events（从文件读取，不经过缓存）
	for g := 0; g < 3; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 30; i++ {
				_, err := s.Events(EventFilter{SessionID: info.ID})
				if err != nil {
					t.Errorf("Events 失败: %v", err)
					return
				}
			}
		}()
	}

	wg.Wait()
}
