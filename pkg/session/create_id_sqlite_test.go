//go:build sqlite

package session

import (
	"path/filepath"
	"testing"
)

// TestSQLiteCreateOptsID_Preserved 覆盖迁移路径真正依赖的那一个后端：
// 迁移要按原 ID 建会话，事件的外键才找得到归属。
func TestSQLiteCreateOptsID_Preserved(t *testing.T) {
	const want = "fixed-session-id-sqlite"

	dbPath := filepath.Join(t.TempDir(), "sessions.db")
	s, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore 失败: %v", err)
	}
	defer s.Close()

	info, err := s.Create(CreateOpts{ID: want, Title: "保留 ID"})
	if err != nil {
		t.Fatalf("Create 失败: %v", err)
	}
	if info.ID != want {
		t.Fatalf("ID = %q，期望 %q", info.ID, want)
	}

	got, err := s.Get(want)
	if err != nil {
		t.Fatalf("按原 ID 取不到会话: %v", err)
	}
	if got.ID != want {
		t.Fatalf("Get 返回 ID = %q，期望 %q", got.ID, want)
	}

	// 事件外键必须能落在该会话上：这正是修复前失败的动作。
	event := Event{
		SessionID: want,
		Type:      EventPrompted,
		Data:      []byte(`{"content":"升级前写入的内容"}`),
	}
	if err := s.AppendEvent(event); err != nil {
		t.Fatalf("按原 ID 追加事件失败（外键对不上）: %v", err)
	}

	events, err := s.Events(EventFilter{SessionID: want})
	if err != nil {
		t.Fatalf("读取事件失败: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("事件数 = %d，期望 1", len(events))
	}
}

func TestSQLiteCreateOptsID_RejectsInvalidAndDuplicate(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "sessions.db")
	s, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore 失败: %v", err)
	}
	defer s.Close()

	if _, err := s.Create(CreateOpts{ID: "../evil"}); err == nil {
		t.Error("SQLiteStore 接受了非法 ID")
	}

	const id = "dup"
	if _, err := s.Create(CreateOpts{ID: id}); err != nil {
		t.Fatalf("首次创建失败: %v", err)
	}
	if _, err := s.Create(CreateOpts{ID: id}); err == nil {
		t.Error("重复 ID 未报错")
	}
}
