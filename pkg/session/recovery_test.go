//go:build sqlite

package session

import (
	"path/filepath"
	"testing"
)

func TestRecovery_CheckIntegrity(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore 失败: %v", err)
	}
	defer s.Close()

	// 创建一些数据
	_, err = s.Create(CreateOpts{Title: "测试会话"})
	if err != nil {
		t.Fatalf("Create 失败: %v", err)
	}

	// 检查完整性
	status, err := s.CheckIntegrity()
	if err != nil {
		t.Fatalf("CheckIntegrity 失败: %v", err)
	}

	if status.Corrupted {
		t.Errorf("期望数据库完整，但检测到损坏: %v", status.Error)
	}

	if status.SessionID == "" {
		t.Error("期望非空 SessionID")
	}

	if status.CheckedAt.IsZero() {
		t.Error("期望非零检查时间")
	}
}

func TestRecovery_ListCorruptedSessions(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建几个完整的数据库
	for i := 0; i < 3; i++ {
		dbPath := filepath.Join(tmpDir, "test"+string(rune('0'+i))+".db")
		s, err := NewSQLiteStore(dbPath)
		if err != nil {
			t.Fatalf("NewSQLiteStore 失败: %v", err)
		}
		s.Create(CreateOpts{Title: "测试"})
		s.Close()
	}

	// 列举损坏的会话
	corrupted, err := ListCorruptedSessions(tmpDir)
	if err != nil {
		t.Fatalf("ListCorruptedSessions 失败: %v", err)
	}

	// 应该没有损坏的会话
	if len(corrupted) != 0 {
		t.Errorf("期望 0 个损坏的会话，但找到 %d 个", len(corrupted))
	}
}
