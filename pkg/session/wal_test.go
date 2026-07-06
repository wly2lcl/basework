//go:build sqlite

package session

import (
	"path/filepath"
	"testing"
)

func TestSQLiteStore_WALMode(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore 失败: %v", err)
	}
	defer s.Close()

	// 验证 WAL 模式已启用
	var journalMode string
	err = s.db.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	if err != nil {
		t.Fatalf("查询 journal_mode 失败: %v", err)
	}

	if journalMode != "wal" {
		t.Errorf("期望 WAL 模式，得到 %s", journalMode)
	}
}

func TestSQLiteStore_WALModePerformance(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore 失败: %v", err)
	}
	defer s.Close()

	// 创建会话
	info, err := s.Create(CreateOpts{Title: "性能测试"})
	if err != nil {
		t.Fatalf("Create 失败: %v", err)
	}

	// 写入多个事件
	for i := 0; i < 100; i++ {
		event := Event{
			SessionID: info.ID,
			Type:      "test",
			Data:      []byte(`{"index":1}`),
		}
		err = s.AppendEvent(event)
		if err != nil {
			t.Fatalf("AppendEvent 失败: %v", err)
		}
	}

	// 读取事件
	events, err := s.Events(EventFilter{SessionID: info.ID})
	if err != nil {
		t.Fatalf("Events 失败: %v", err)
	}

	if len(events) != 100 {
		t.Errorf("期望 100 个事件，得到 %d", len(events))
	}
}
