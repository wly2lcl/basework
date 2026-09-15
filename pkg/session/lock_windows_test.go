//go:build windows && sqlite

package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Windows applies LockFileEx locks to the handle's byte range. The
// coordination handle must therefore be separate from the data file; keeping
// the data file locked would make an ordinary writer fail with a sharing
// violation.
func TestFileLock_AllowsDataFileAccessWhileHeld(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.jsonl")
	lock := NewFileLock(path)
	if err := lock.Lock(time.Second); err != nil {
		t.Fatalf("Lock: %v", err)
	}
	defer lock.Unlock()

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatalf("打开数据文件: %v", err)
	}
	if _, err := f.WriteString("{}\n"); err != nil {
		_ = f.Close()
		t.Fatalf("写入数据文件: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("关闭数据文件: %v", err)
	}
}

func TestFileLock_ForceUnlockRemovesCoordinationFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.jsonl")
	lock := NewFileLock(path)
	if err := lock.Lock(time.Second); err != nil {
		t.Fatalf("Lock: %v", err)
	}
	if err := lock.ForceUnlock(); err != nil {
		t.Fatalf("ForceUnlock: %v", err)
	}
	if _, err := os.Stat(path + ".lock"); !os.IsNotExist(err) {
		t.Fatalf("协调锁文件应被删除，stat=%v", err)
	}
}
