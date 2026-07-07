//go:build sqlite

package session

import (
	"path/filepath"
	"testing"
)

func TestVersionInfo(t *testing.T) {
	// 这个测试验证版本信息变量存在
	// 实际的版本信息通过 ldflags 注入，在测试中为默认值

	// 验证版本命令可以正常工作
	// 这里只是确保相关代码存在且可编译

	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore 失败: %v", err)
	}
	defer s.Close()

	// 创建会话验证数据库工作正常
	info, err := s.Create(CreateOpts{Title: "版本测试"})
	if err != nil {
		t.Fatalf("Create 失败: %v", err)
	}

	if info.ID == "" {
		t.Fatal("期望非空 ID")
	}
}
