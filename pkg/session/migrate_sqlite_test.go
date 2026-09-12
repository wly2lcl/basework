//go:build sqlite

package session

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// userVersion 读取数据库的 PRAGMA user_version。
func userVersion(t *testing.T, db *sql.DB) int {
	t.Helper()
	var v int
	if err := db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		t.Fatalf("读取 user_version 失败: %v", err)
	}
	return v
}

// TestSQLiteUserVersionUpgrade 新建库直接为当前版本；遗留库（user_version=0）
// 打开时被迁移到当前版本。
func TestSQLiteUserVersionUpgrade(t *testing.T) {
	t.Run("新库即为当前版本", func(t *testing.T) {
		dbPath := filepath.Join(t.TempDir(), "sessions.db")
		store, err := NewSQLiteStore(dbPath)
		if err != nil {
			t.Fatalf("创建 store 失败: %v", err)
		}
		defer store.Close()

		if got := userVersion(t, store.GetDB()); got != SchemaVersion {
			t.Errorf("user_version = %d，期望 %d", got, SchemaVersion)
		}
	})

	t.Run("遗留库被迁移到当前版本", func(t *testing.T) {
		dbPath := filepath.Join(t.TempDir(), "legacy.db")

		// 造一个 v0 遗留库：有库文件、user_version 为 0、无表。
		raw, err := sql.Open("sqlite", dbPath)
		if err != nil {
			t.Fatalf("打开原始库失败: %v", err)
		}
		if got := userVersion(t, raw); got != 0 {
			t.Fatalf("预设的 user_version = %d，期望 0", got)
		}
		raw.Close()

		store, err := NewSQLiteStore(dbPath)
		if err != nil {
			t.Fatalf("打开遗留库失败: %v", err)
		}
		defer store.Close()

		if got := userVersion(t, store.GetDB()); got != SchemaVersion {
			t.Errorf("迁移后 user_version = %d，期望 %d", got, SchemaVersion)
		}
	})
}

// TestSQLiteSchemaTooNewRejected 库版本高于程序支持版本时，打开必须失败。
func TestSQLiteSchemaTooNewRejected(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "future.db")

	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("打开原始库失败: %v", err)
	}
	if _, err := raw.Exec("PRAGMA user_version = 999"); err != nil {
		t.Fatalf("设置 user_version 失败: %v", err)
	}
	raw.Close()

	store, err := NewSQLiteStore(dbPath)
	if err == nil {
		store.Close()
		t.Fatal("打开未来版本库应失败，实际成功")
	}
	if !errors.Is(err, ErrSchemaTooNew) {
		t.Fatalf("错误 = %v，期望 ErrSchemaTooNew", err)
	}
}
