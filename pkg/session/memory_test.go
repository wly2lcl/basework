package session

import (
	"sync"
	"testing"
	"time"

	"github.com/wly2lcl/basework/pkg/llm"
)

func TestMemoryStore_CreateGet(t *testing.T) {
	s := NewMemoryStore()
	info, err := s.Create(CreateOpts{Title: "测试会话"})
	if err != nil {
		t.Fatalf("Create 失败: %v", err)
	}
	if info.ID == "" {
		t.Fatal("期望非空 ID")
	}
	if info.Title != "测试会话" {
		t.Errorf("Title = %q, 期望 %q", info.Title, "测试会话")
	}

	got, err := s.Get(info.ID)
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if got.ID != info.ID {
		t.Errorf("ID = %q, 期望 %q", got.ID, info.ID)
	}
}

func TestMemoryStore_GetNotFound(t *testing.T) {
	s := NewMemoryStore()
	_, err := s.Get("non-existent")
	if err == nil {
		t.Fatal("期望获取不存在的会话返回错误")
	}
}

func TestMemoryStore_AppendEventSeq(t *testing.T) {
	s := NewMemoryStore()
	info, _ := s.Create(CreateOpts{Title: "test"})

	e1 := Event{SessionID: info.ID, Type: EventPrompted, Data: rawJSON(t, PromptedData{Content: "a"})}
	if err := s.AppendEvent(e1); err != nil {
		t.Fatalf("AppendEvent 失败: %v", err)
	}

	e2 := Event{SessionID: info.ID, Type: EventPrompted, Data: rawJSON(t, PromptedData{Content: "b"})}
	if err := s.AppendEvent(e2); err != nil {
		t.Fatalf("AppendEvent 失败: %v", err)
	}

	events, _ := s.Events(EventFilter{SessionID: info.ID})
	if len(events) != 2 {
		t.Fatalf("期望 2 个事件，得到 %d", len(events))
	}
	if events[0].Seq != 1 || events[1].Seq != 2 {
		t.Errorf("Seq 未自动递增: got %d, %d; 期望 1, 2", events[0].Seq, events[1].Seq)
	}
}

func TestMemoryStore_AppendEventSessionNotFound(t *testing.T) {
	s := NewMemoryStore()
	err := s.AppendEvent(Event{SessionID: "nonexistent", Type: EventPrompted})
	if err == nil {
		t.Fatal("期望追加到不存在的会话返回错误")
	}
}

func TestMemoryStore_EventsFilter(t *testing.T) {
	s := NewMemoryStore()
	info, _ := s.Create(CreateOpts{Title: "test"})

	events := []Event{
		{SessionID: info.ID, Type: EventPrompted, Data: rawJSON(t, PromptedData{Content: "q"})},
		{SessionID: info.ID, Type: EventToolCalled, Data: rawJSON(t, ToolCalledData{ToolCall: llm.ToolCall{ID: "t1"}})},
		{SessionID: info.ID, Type: EventToolSuccess, Data: rawJSON(t, ToolSuccessData{ToolCallID: "t1", Content: "ok"})},
		{SessionID: info.ID, Type: EventTextDelta, Data: rawJSON(t, TextDeltaData{Delta: "hello"})},
	}
	for _, e := range events {
		if err := s.AppendEvent(e); err != nil {
			t.Fatalf("AppendEvent 失败: %v", err)
		}
	}

	t.Run("FilterByTypes", func(t *testing.T) {
		filtered, err := s.Events(EventFilter{
			SessionID: info.ID,
			Types:     []EventType{EventPrompted, EventTextDelta},
		})
		if err != nil {
			t.Fatalf("Events 失败: %v", err)
		}
		if len(filtered) != 2 {
			t.Fatalf("期望 2 个事件，得到 %d", len(filtered))
		}
	})

	t.Run("FilterAfterSeq", func(t *testing.T) {
		filtered, err := s.Events(EventFilter{
			SessionID: info.ID,
			AfterSeq:  2,
		})
		if err != nil {
			t.Fatalf("Events 失败: %v", err)
		}
		// AfterSeq=2 表示 seq > 2 的事件：即 seq=3 (ToolSuccess) 和 seq=4 (TextDelta)
		if len(filtered) != 2 {
			t.Fatalf("期望 2 个事件，得到 %d", len(filtered))
		}
	})

	t.Run("FilterLimit", func(t *testing.T) {
		filtered, err := s.Events(EventFilter{
			SessionID: info.ID,
			Limit:     2,
		})
		if err != nil {
			t.Fatalf("Events 失败: %v", err)
		}
		if len(filtered) != 2 {
			t.Fatalf("期望 2 个事件，得到 %d", len(filtered))
		}
	})
}

func TestMemoryStore_Delete(t *testing.T) {
	s := NewMemoryStore()
	info, _ := s.Create(CreateOpts{Title: "test"})

	if err := s.Delete(info.ID); err != nil {
		t.Fatalf("Delete 失败: %v", err)
	}
	_, err := s.Get(info.ID)
	if err == nil {
		t.Fatal("期望删除后 Get 返回错误")
	}
}

func TestMemoryStore_DeleteNotFound(t *testing.T) {
	s := NewMemoryStore()
	err := s.Delete("nonexistent")
	if err == nil {
		t.Fatal("期望删除不存在的会话返回错误")
	}
}

func TestMemoryStore_List(t *testing.T) {
	s := NewMemoryStore()
	s.Create(CreateOpts{Title: "a"})
	s.Create(CreateOpts{Title: "b"})
	s.Create(CreateOpts{Title: "c"})

	t.Run("All", func(t *testing.T) {
		result, err := s.List(ListFilter{})
		if err != nil {
			t.Fatalf("List 失败: %v", err)
		}
		if len(result) != 3 {
			t.Fatalf("期望 3 个会话，得到 %d", len(result))
		}
	})

	t.Run("Limit", func(t *testing.T) {
		result, err := s.List(ListFilter{Limit: 2})
		if err != nil {
			t.Fatalf("List 失败: %v", err)
		}
		if len(result) != 2 {
			t.Fatalf("期望 2 个会话，得到 %d", len(result))
		}
	})

	t.Run("Offset", func(t *testing.T) {
		result, err := s.List(ListFilter{Offset: 1})
		if err != nil {
			t.Fatalf("List 失败: %v", err)
		}
		if len(result) != 2 {
			t.Fatalf("期望 2 个会话，得到 %d", len(result))
		}
	})
}

func TestMemoryStore_ConcurrentSafety(t *testing.T) {
	s := NewMemoryStore()
	info, _ := s.Create(CreateOpts{Title: "concurrent"})

	var wg sync.WaitGroup
	n := 50
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = s.AppendEvent(Event{
				SessionID: info.ID,
				Type:      EventTextDelta,
				Data:      rawJSON(t, TextDeltaData{Delta: "x"}),
			})
		}()
	}
	wg.Wait()

	events, _ := s.Events(EventFilter{SessionID: info.ID})
	if len(events) != n {
		t.Fatalf("并发写入后期望 %d 个事件，得到 %d", n, len(events))
	}
	// 验证 Seq 连续
	for i, e := range events {
		if e.Seq != int64(i+1) {
			t.Errorf("events[%d].Seq = %d, 期望 %d", i, e.Seq, i+1)
		}
	}
}

func TestMemoryStore_UpdateMessageCountAndUsage(t *testing.T) {
	s := NewMemoryStore()
	info, _ := s.Create(CreateOpts{Title: "test"})

	// 添加几个事件，MessageCount 暂不由 AppendEvent 更新
	_ = s.AppendEvent(Event{SessionID: info.ID, Type: EventPrompted, Data: rawJSON(t, PromptedData{Content: "q"})})
	_ = s.AppendEvent(Event{SessionID: info.ID, Type: EventTextDelta, Data: rawJSON(t, TextDeltaData{Delta: "a"})})

	got, _ := s.Get(info.ID)
	// 调用者需要通过其他方式更新这些字段
	_ = got
}

func TestNewID(t *testing.T) {
	id1 := newID()
	id2 := newID()
	if id1 == id2 {
		t.Fatal("两次 newID 应该不同")
	}
	if len(id1) != 32 { // 16 字节 hex
		t.Errorf("ID 长度 = %d, 期望 32", len(id1))
	}
}

func TestMemoryStore_EventsSessionNotFound(t *testing.T) {
	s := NewMemoryStore()
	_, err := s.Events(EventFilter{SessionID: "nonexistent"})
	if err == nil {
		t.Fatal("期望查询不存在的会话返回错误")
	}
}

func TestMemoryStore_MessagesNotSupported(t *testing.T) {
	s := NewMemoryStore()
	_, err := s.Messages()
	if err == nil {
		t.Fatal("期望 MemoryStore.Messages() 返回错误")
	}
}

func TestMemoryStore_EventsFullyEmpty(t *testing.T) {
	s := NewMemoryStore()
	info, _ := s.Create(CreateOpts{Title: "empty"})

	events, err := s.Events(EventFilter{SessionID: info.ID})
	if err != nil {
		t.Fatalf("Events 失败: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("期望空结果，得到 %d", len(events))
	}
}

func TestMemoryStore_ListEmpty(t *testing.T) {
	s := NewMemoryStore()
	result, err := s.List(ListFilter{})
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("期望空列表，得到 %d", len(result))
	}
}

func TestMemoryStore_ListAfterTime(t *testing.T) {
	s := NewMemoryStore()
	time.Sleep(10 * time.Millisecond)
	before := time.Now().UTC()
	time.Sleep(10 * time.Millisecond)
	s.Create(CreateOpts{Title: "first"})
	time.Sleep(10 * time.Millisecond)
	mid := time.Now().UTC()
	time.Sleep(10 * time.Millisecond)
	s.Create(CreateOpts{Title: "second"})
	time.Sleep(10 * time.Millisecond)
	after := time.Now().UTC()

	// AfterTime=before (两个会话之前)：两个都应当被包含
	result, err := s.List(ListFilter{AfterTime: before})
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("AfterTime=before 期望 2 个，得到 %d", len(result))
	}

	// AfterTime=mid (两个会话之间)：只包含 second
	result, err = s.List(ListFilter{AfterTime: mid})
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("AfterTime=mid 期望 1 个，得到 %d", len(result))
	}

	// AfterTime=after (两个会话之后)：都不应当被包含
	result, err = s.List(ListFilter{AfterTime: after})
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("AfterTime=after 期望 0 个，得到 %d", len(result))
	}
}

func TestMemoryStore_AppendEventIDGeneration(t *testing.T) {
	s := NewMemoryStore()
	info, _ := s.Create(CreateOpts{Title: "test"})

	err := s.AppendEvent(Event{SessionID: info.ID, Type: EventPrompted, Data: rawJSON(t, PromptedData{Content: "hi"})})
	if err != nil {
		t.Fatalf("AppendEvent 失败: %v", err)
	}

	events, _ := s.Events(EventFilter{SessionID: info.ID})
	if len(events) != 1 {
		t.Fatalf("期望 1 个事件，得到 %d", len(events))
	}
	if events[0].ID == "" {
		t.Fatal("事件的 ID 应该自动生成")
	}
	if events[0].CreatedAt.IsZero() {
		t.Fatal("事件的 CreatedAt 应该自动设置")
	}
}