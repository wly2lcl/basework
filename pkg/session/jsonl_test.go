package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestJSONLStore_CreateGet(t *testing.T) {
	dir := t.TempDir()
	s, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("NewJSONLStore 失败: %v", err)
	}

	info, err := s.Create(CreateOpts{Title: "测试会话"})
	if err != nil {
		t.Fatalf("Create 失败: %v", err)
	}
	if info.ID == "" {
		t.Fatal("期望非空 ID")
	}

	got, err := s.Get(info.ID)
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if got.ID != info.ID || got.Title != "测试会话" {
		t.Errorf("Get 结果不匹配: %+v", got)
	}
}

func TestJSONLStore_AppendAndRead(t *testing.T) {
	dir := t.TempDir()
	s, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("NewJSONLStore 失败: %v", err)
	}

	info, _ := s.Create(CreateOpts{Title: "test"})

	// 追加多个事件
	for i := 0; i < 3; i++ {
		err := s.AppendEvent(Event{
			SessionID: info.ID,
			Type:      EventTextDelta,
			Data:      rawJSON(t, TextDeltaData{Delta: "data"}),
		})
		if err != nil {
			t.Fatalf("AppendEvent 失败: %v", err)
		}
	}

	// 读取验证
	events, err := s.Events(EventFilter{SessionID: info.ID})
	if err != nil {
		t.Fatalf("Events 失败: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("期望 3 个事件，得到 %d", len(events))
	}
	for i, e := range events {
		if e.Seq != int64(i+1) {
			t.Errorf("events[%d].Seq = %d, 期望 %d", i, e.Seq, i+1)
		}
	}
}

func TestJSONLStore_EventsFilter(t *testing.T) {
	dir := t.TempDir()
	s, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("NewJSONLStore 失败: %v", err)
	}

	info, _ := s.Create(CreateOpts{Title: "test"})
	s.AppendEvent(Event{SessionID: info.ID, Type: EventPrompted, Data: rawJSON(t, PromptedData{Content: "q"})})
	s.AppendEvent(Event{SessionID: info.ID, Type: EventToolCalled, Data: rawJSON(t, ToolCalledData{})})
	s.AppendEvent(Event{SessionID: info.ID, Type: EventToolSuccess, Data: rawJSON(t, ToolSuccessData{})})

	t.Run("FilterByType", func(t *testing.T) {
		filtered, _ := s.Events(EventFilter{SessionID: info.ID, Types: []EventType{EventPrompted}})
		if len(filtered) != 1 {
			t.Fatalf("期望 1 个事件，得到 %d", len(filtered))
		}
	})

	t.Run("FilterAfterSeq", func(t *testing.T) {
		filtered, _ := s.Events(EventFilter{SessionID: info.ID, AfterSeq: 1})
		if len(filtered) != 2 {
			t.Fatalf("期望 2 个事件，得到 %d", len(filtered))
		}
	})

	t.Run("FilterLimit", func(t *testing.T) {
		filtered, _ := s.Events(EventFilter{SessionID: info.ID, Limit: 1})
		if len(filtered) != 1 {
			t.Fatalf("期望 1 个事件，得到 %d", len(filtered))
		}
	})
}

func TestJSONLStore_Delete(t *testing.T) {
	dir := t.TempDir()
	s, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("NewJSONLStore 失败: %v", err)
	}

	info, _ := s.Create(CreateOpts{Title: "test"})
	if err := s.Delete(info.ID); err != nil {
		t.Fatalf("Delete 失败: %v", err)
	}

	// 文件应该已删除
	path := filepath.Join(dir, info.ID+".jsonl")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("删除后文件仍然存在")
	}

	// 再次删除应返回错误
	if err := s.Delete(info.ID); err == nil {
		t.Fatal("期望删除已删除的会话返回错误")
	}
}

func TestJSONLStore_GetInvalid(t *testing.T) {
	dir := t.TempDir()
	s, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("NewJSONLStore 失败: %v", err)
	}

	_, err = s.Get("nonexistent")
	if err == nil {
		t.Fatal("期望获取不存在的会话返回错误")
	}
}

func TestJSONLStore_List(t *testing.T) {
	dir := t.TempDir()
	s, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("NewJSONLStore 失败: %v", err)
	}

	s.Create(CreateOpts{Title: "a"})
	s.Create(CreateOpts{Title: "b"})
	s.Create(CreateOpts{Title: "c"})

	all, err := s.List(ListFilter{})
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("期望 3 个会话，得到 %d", len(all))
	}

	limited, _ := s.List(ListFilter{Limit: 2})
	if len(limited) != 2 {
		t.Fatalf("期望 2 个会话，得到 %d", len(limited))
	}
}

func TestJSONLStore_PathSafety(t *testing.T) {
	dir := t.TempDir()
	s, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("NewJSONLStore 失败: %v", err)
	}

	// 非法 session ID
	info, err := s.Create(CreateOpts{Title: "test"})
	if err != nil {
		t.Fatalf("Create 失败: %v", err)
	}

	// 使用带有特殊字符的 ID 应失败
	err = s.AppendEvent(Event{
		SessionID: info.ID + "../../etc/passwd",
		Type:      EventPrompted,
	})
	if err == nil {
		t.Fatal("期望非法 session ID 返回错误")
	}

	// 文件名中不应包含路径分隔符
	path, err := s.safePath(info.ID)
	if err != nil {
		t.Fatalf("safePath 失败: %v", err)
	}
	if filepath.Base(path) != info.ID+".jsonl" {
		t.Errorf("文件名应为 %q, 得到 %q", info.ID+".jsonl", filepath.Base(path))
	}
}

func TestJSONLStore_NewStoreCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "subdir")
	s, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("NewJSONLStore 失败: %v", err)
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Fatal("目录未被自动创建")
	}
	_ = s
}

func TestJSONLStore_AppendToExistingFile(t *testing.T) {
	dir := t.TempDir()
	s, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("NewJSONLStore 失败: %v", err)
	}

	info, _ := s.Create(CreateOpts{Title: "test"})
	s.AppendEvent(Event{SessionID: info.ID, Type: EventPrompted, Data: rawJSON(t, PromptedData{Content: "a"})})

	// 创建新实例模拟重新加载
	s2, _ := NewJSONLStore(dir)
	events, err := s2.Events(EventFilter{SessionID: info.ID})
	if err != nil {
		t.Fatalf("新实例读取事件失败: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("新实例期望 1 个事件，得到 %d", len(events))
	}
}

func TestJSONLStore_MessagesNotSupported(t *testing.T) {
	dir := t.TempDir()
	s, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("NewJSONLStore 失败: %v", err)
	}
	_, err = s.Messages()
	if err == nil {
		t.Fatal("期望 JSONLStore.Messages() 返回错误")
	}
}

func TestJSONLStore_ConcurrentAppend(t *testing.T) {
	dir := t.TempDir()
	s, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("NewJSONLStore 失败: %v", err)
	}

	info, _ := s.Create(CreateOpts{Title: "concurrent"})

	done := make(chan struct{})
	n := 10
	for i := 0; i < n; i++ {
		go func() {
			s.AppendEvent(Event{
				SessionID: info.ID,
				Type:      EventTextDelta,
				Data:      rawJSON(t, TextDeltaData{Delta: "x"}),
			})
			done <- struct{}{}
		}()
	}
	for i := 0; i < n; i++ {
		<-done
	}

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

func TestJSONLStore_EventsEmptySession(t *testing.T) {
	dir := t.TempDir()
	s, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("NewJSONLStore 失败: %v", err)
	}

	info, _ := s.Create(CreateOpts{Title: "empty"})
	events, err := s.Events(EventFilter{SessionID: info.ID})
	if err != nil {
		t.Fatalf("Events 失败: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("期望空结果，得到 %d", len(events))
	}
}

func TestJSONLStore_EventsSessionNotFound(t *testing.T) {
	dir := t.TempDir()
	s, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("NewJSONLStore 失败: %v", err)
	}
	_, err = s.Events(EventFilter{SessionID: "nonexistent"})
	if err == nil {
		t.Fatal("期望查询不存在的会话返回错误")
	}
}

func TestJSONLStore_ListEmpty(t *testing.T) {
	dir := t.TempDir()
	s, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("NewJSONLStore 失败: %v", err)
	}
	result, err := s.List(ListFilter{})
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("期望空列表，得到 %d", len(result))
	}
}

func TestJSONLStore_EventsAfterDelete(t *testing.T) {
	dir := t.TempDir()
	s, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("NewJSONLStore 失败: %v", err)
	}

	info, _ := s.Create(CreateOpts{Title: "test"})
	s.AppendEvent(Event{SessionID: info.ID, Type: EventPrompted, Data: rawJSON(t, PromptedData{Content: "hi"})})
	s.Delete(info.ID)

	_, err = s.Events(EventFilter{SessionID: info.ID})
	if err == nil {
		t.Fatal("删除后查询事件应返回错误")
	}
}

func TestJSONLStore_SafePathRejectsInvalid(t *testing.T) {
	s := &JSONLStore{baseDir: "/tmp"}
	invalid := []string{
		"../etc/passwd",
		"a/b/c",
		"",
		"hello world",
		"a.b",
	}
	for _, id := range invalid {
		_, err := s.safePath(id)
		if err == nil {
			t.Errorf("safePath(%q) 应返回错误", id)
		}
	}

	valid := []string{"abc123", "ABC-DEF_ghi", "a", "123"}
	for _, id := range valid {
		_, err := s.safePath(id)
		if err != nil {
			t.Errorf("safePath(%q) 不应返回错误: %v", id, err)
		}
	}
}

func TestJSONLStore_ListOrder(t *testing.T) {
	dir := t.TempDir()
	s, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("NewJSONLStore 失败: %v", err)
	}

	s.Create(CreateOpts{Title: "first"})
	s.Create(CreateOpts{Title: "second"})
	s.Create(CreateOpts{Title: "third"})

	all, _ := s.List(ListFilter{})
	if len(all) != 3 {
		t.Fatalf("期望 3 个会话，得到 %d", len(all))
	}
	// 按 CreatedAt 降序排列，最新的在最前面
	if all[0].CreatedAt.Before(all[1].CreatedAt) || all[1].CreatedAt.Before(all[2].CreatedAt) {
		t.Error("List 未按 CreatedAt 降序排列")
	}
}

func TestJSONLStore_AppendTwiceReopenCheck(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewJSONLStore(dir)
	info, _ := s.Create(CreateOpts{Title: "test"})
	s.AppendEvent(Event{SessionID: info.ID, Type: EventPrompted, Data: rawJSON(t, PromptedData{Content: "a"})})

	// 重新打开
	s2, _ := NewJSONLStore(dir)
	events, _ := s2.Events(EventFilter{SessionID: info.ID})
	if len(events) != 1 {
		t.Fatalf("重新打开后期望 1 个事件，得到 %d", len(events))
	}

	// 追加新事件
	s2.AppendEvent(Event{SessionID: info.ID, Type: EventTextDelta, Data: rawJSON(t, TextDeltaData{Delta: "b"})})

	// 再次重新打开验证
	s3, _ := NewJSONLStore(dir)
	events, _ = s3.Events(EventFilter{SessionID: info.ID})
	if len(events) != 2 {
		t.Fatalf("再次重新打开后期望 2 个事件，得到 %d", len(events))
	}
}

func TestJSONLStore_RejectInvalidSessionID(t *testing.T) {
	dir := t.TempDir()
	s, err := NewJSONLStore(dir)
	if err != nil {
		t.Fatalf("NewJSONLStore 失败: %v", err)
	}

	// 创建时用的是 newID() 生成的合法 ID
	info, _ := s.Create(CreateOpts{Title: "test"})

	// 尝试用非法 ID 追加事件
	err = s.AppendEvent(Event{
		SessionID: "bad/id",
		Type:      EventPrompted,
	})
	if err == nil {
		t.Fatal("期望非法 ID 返回错误")
	}
	_ = info
}