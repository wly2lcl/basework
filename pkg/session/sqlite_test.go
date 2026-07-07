//go:build sqlite

package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wly2lcl/basework/pkg/llm"
)

func TestSQLiteStore_CreateGet(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore 失败: %v", err)
	}
	defer s.Close()

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

func TestSQLiteStore_CreateAndList(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore 失败: %v", err)
	}
	defer s.Close()

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
}

func TestSQLiteStore_Delete(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore 失败: %v", err)
	}
	defer s.Close()

	info, _ := s.Create(CreateOpts{Title: "test"})

	if err := s.Delete(info.ID); err != nil {
		t.Fatalf("Delete 失败: %v", err)
	}

	_, err = s.Get(info.ID)
	if err == nil {
		t.Fatal("删除后 Get 应返回错误")
	}
}

func TestSQLiteStore_DeleteNotFound(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore 失败: %v", err)
	}
	defer s.Close()

	err = s.Delete("nonexistent")
	if err == nil {
		t.Fatal("删除不存在的会话应返回错误")
	}
}

func TestSQLiteStore_GetNotFound(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore 失败: %v", err)
	}
	defer s.Close()

	_, err = s.Get("nonexistent")
	if err == nil {
		t.Fatal("获取不存在的会话应返回错误")
	}
}

func TestSQLiteStore_AppendPromptedEvent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore 失败: %v", err)
	}
	defer s.Close()

	info, _ := s.Create(CreateOpts{Title: "test"})

	err = s.AppendEvent(Event{
		SessionID: info.ID,
		Type:      EventPrompted,
		Data:      rawJSON(t, PromptedData{Content: "你好"}),
	})
	if err != nil {
		t.Fatalf("AppendEvent 失败: %v", err)
	}

	// 验证消息已插入
	msgs, err := s.MessagesForSession(info.ID)
	if err != nil {
		t.Fatalf("MessagesForSession 失败: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("期望 1 条消息，得到 %d", len(msgs))
	}
	if string(msgs[0].Role) != "user" {
		t.Errorf("Role = %q, 期望 user", msgs[0].Role)
	}
}

func TestSQLiteStore_AppendEventInvalidSession(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore 失败: %v", err)
	}
	defer s.Close()

	err = s.AppendEvent(Event{
		SessionID: "nonexistent",
		Type:      EventPrompted,
		Data:      rawJSON(t, PromptedData{Content: "hi"}),
	})
	if err == nil {
		t.Fatal("追加到不存在的会话应返回错误")
	}
}

func TestSQLiteStore_EventsWorks(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore 失败: %v", err)
	}
	defer s.Close()

	info, _ := s.Create(CreateOpts{Title: "test"})
	s.AppendEvent(Event{
		SessionID: info.ID, Type: EventPrompted,
		Data: rawJSON(t, PromptedData{Content: "hi"}),
	})
	s.AppendEvent(Event{
		SessionID: info.ID, Type: EventTextDelta,
		Data: rawJSON(t, TextDeltaData{Delta: "hello"}),
	})

	events, err := s.Events(EventFilter{SessionID: info.ID})
	if err != nil {
		t.Fatalf("Events 失败: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("期望 2 个事件，得到 %d", len(events))
	}
}

func TestSQLiteStore_MessagesNotSupported(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore 失败: %v", err)
	}
	defer s.Close()

	_, err = s.Messages()
	if err == nil {
		t.Fatal("SQLiteStore.Messages() 应返回错误")
	}
}

func TestSQLiteStore_ForeignKeysEnabled(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore 失败: %v", err)
	}
	defer s.Close()

	var fkEnabled int
	err = s.GetDB().QueryRow("PRAGMA foreign_keys").Scan(&fkEnabled)
	if err != nil {
		t.Fatalf("查询 PRAGMA foreign_keys 失败: %v", err)
	}
	if fkEnabled != 1 {
		t.Fatalf("foreign_keys 应为 1，得到 %d", fkEnabled)
	}
}

func TestSQLiteStore_DeleteCascadeViaForeignKey(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore 失败: %v", err)
	}
	defer s.Close()

	// 创建会话
	info, err := s.Create(CreateOpts{Title: "cascade-test"})
	if err != nil {
		t.Fatalf("Create 失败: %v", err)
	}

	// 插入一条 message（通过 events→ProjectMessages 或直接 SQL）
	// 使用直接 SQL 插入 messages 表以验证级联
	db := s.GetDB()
	_, err = db.Exec(
		`INSERT INTO messages (id, session_id, role, content, timestamp) VALUES (?, ?, ?, ?, ?)`,
		"msg_1", info.ID, "user", "你好", "2024-01-01T00:00:00Z",
	)
	if err != nil {
		t.Fatalf("插入 message 失败: %v", err)
	}

	// 验证 message 存在
	var msgCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM messages WHERE session_id = ?`, info.ID).Scan(&msgCount)
	if err != nil {
		t.Fatalf("查询 messages 失败: %v", err)
	}
	if msgCount != 1 {
		t.Fatalf("期望 1 条 message，得到 %d", msgCount)
	}

	// 删除会话（通过 Delete 方法）
	if err := s.Delete(info.ID); err != nil {
		t.Fatalf("Delete 失败: %v", err)
	}

	// 验证 message 也被级联删除
	err = db.QueryRow(`SELECT COUNT(*) FROM messages WHERE session_id = ?`, info.ID).Scan(&msgCount)
	if err != nil {
		t.Fatalf("查询 messages 失败: %v", err)
	}
	if msgCount != 0 {
		t.Errorf("级联删除后 message 应为 0，得到 %d", msgCount)
	}
}

func TestSQLiteStore_DeleteRemovesAllData(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore 失败: %v", err)
	}
	defer s.Close()

	// 创建会话并追加事件和消息
	info, err := s.Create(CreateOpts{Title: "full-delete-test"})
	if err != nil {
		t.Fatalf("Create 失败: %v", err)
	}

	// 追加事件（会触发 messages 写入）
	err = s.AppendEvent(Event{
		SessionID: info.ID,
		Type:      EventPrompted,
		Data:      rawJSON(t, PromptedData{Content: "测试"}),
	})
	if err != nil {
		t.Fatalf("AppendEvent 失败: %v", err)
	}

	// 验证 events 表有数据
	db := s.GetDB()
	var eventCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM events WHERE session_id = ?`, info.ID).Scan(&eventCount)
	if err != nil {
		t.Fatalf("查询 events 失败: %v", err)
	}
	if eventCount != 1 {
		t.Fatalf("期望 1 个 event，得到 %d", eventCount)
	}

	// 删除会话
	if err := s.Delete(info.ID); err != nil {
		t.Fatalf("Delete 失败: %v", err)
	}

	// 验证所有关联数据都被删除
	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id = ?`, info.ID).Scan(&count)
	if err != nil {
		t.Fatalf("查询 sessions 失败: %v", err)
	}
	if count != 0 {
		t.Errorf("删除后 sessions 应为 0，得到 %d", count)
	}

	err = db.QueryRow(`SELECT COUNT(*) FROM events WHERE session_id = ?`, info.ID).Scan(&count)
	if err != nil {
		t.Fatalf("查询 events 失败: %v", err)
	}
	if count != 0 {
		t.Errorf("删除后 events 应为 0，得到 %d", count)
	}

	err = db.QueryRow(`SELECT COUNT(*) FROM messages WHERE session_id = ?`, info.ID).Scan(&count)
	if err != nil {
		t.Fatalf("查询 messages 失败: %v", err)
	}
	if count != 0 {
		t.Errorf("删除后 messages 应为 0，得到 %d", count)
	}
}

func TestSQLiteStore_ListLimit(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore 失败: %v", err)
	}
	defer s.Close()

	s.Create(CreateOpts{Title: "a"})
	s.Create(CreateOpts{Title: "b"})
	s.Create(CreateOpts{Title: "c"})

	limited, err := s.List(ListFilter{Limit: 2})
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if len(limited) != 2 {
		t.Fatalf("期望 2 个会话，得到 %d", len(limited))
	}
}

func TestSQLiteStore_DatabaseFileCreated(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "nested", "subdir", "session.db")
	s, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore 失败: %v", err)
	}
	defer s.Close()

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatal("数据库文件未被自动创建")
	}
}

func TestSQLiteStore_AppendDeltaAndToolCall(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore 失败: %v", err)
	}
	defer s.Close()

	info, _ := s.Create(CreateOpts{Title: "test"})

	// 追加多种事件
	s.AppendEvent(Event{
		SessionID: info.ID, Type: EventPrompted,
		Data: rawJSON(t, PromptedData{Content: "查天气"}),
	})
	s.AppendEvent(Event{
		SessionID: info.ID, Type: EventToolCalled,
		Data: rawJSON(t, ToolCalledData{
			ToolCall: llm.ToolCall{ID: "call_1", Name: "get_weather", ArgsJSON: `{"city":"北京"}`},
		}),
	})
	s.AppendEvent(Event{
		SessionID: info.ID, Type: EventToolSuccess,
		Data: rawJSON(t, ToolSuccessData{ToolCallID: "call_1", Content: "晴"}),
	})
	s.AppendEvent(Event{
		SessionID: info.ID, Type: EventTextDelta,
		Data: rawJSON(t, TextDeltaData{Delta: "北京晴天"}),
	})

	msgs, err := s.MessagesForSession(info.ID)
	if err != nil {
		t.Fatalf("MessagesForSession 失败: %v", err)
	}

	if len(msgs) != 4 {
		t.Fatalf("期望 4 条消息，得到 %d", len(msgs))
	}

	// 验证 user 消息
	if string(msgs[0].Role) != "user" {
		t.Errorf("msgs[0] 应为 user, 得到 %s", msgs[0].Role)
	}

	// 验证 tool call 存在
	toolCallFound := false
	for _, m := range msgs {
		if len(m.ToolCalls) > 0 && m.ToolCalls[0].Name == "get_weather" {
			toolCallFound = true
			break
		}
	}
	if !toolCallFound {
		t.Error("未找到 tool call 消息")
	}

	// 验证 tool result 存在
	toolResultFound := false
	for _, m := range msgs {
		if m.Role == "tool" && m.ToolCallID == "call_1" {
			toolResultFound = true
			break
		}
	}
	if !toolResultFound {
		t.Error("未找到 tool result 消息")
	}
}