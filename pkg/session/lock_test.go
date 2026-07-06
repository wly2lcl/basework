//go:build sqlite && !windows

package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileLock_LockUnlock(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "test.lock")

	lock := NewFileLock(lockPath)

	// 获取锁
	err := lock.Lock(1 * time.Second)
	if err != nil {
		t.Fatalf("Lock 失败: %v", err)
	}

	// 验证锁文件存在
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Fatal("锁文件未创建")
	}

	// 释放锁
	err = lock.Unlock()
	if err != nil {
		t.Fatalf("Unlock 失败: %v", err)
	}
}

func TestFileLock_Timeout(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "test.lock")

	// 第一个锁
	lock1 := NewFileLock(lockPath)
	err := lock1.Lock(1 * time.Second)
	if err != nil {
		t.Fatalf("Lock1 失败: %v", err)
	}
	defer lock1.Unlock()

	// 第二个锁应该超时
	lock2 := NewFileLock(lockPath)
	err = lock2.Lock(500 * time.Millisecond)
	if err == nil {
		t.Fatal("期望超时错误")
	}
	if err.Error() == "" {
		t.Fatal("期望错误消息")
	}
}

func TestFileLock_ForceUnlock(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "test.lock")

	lock := NewFileLock(lockPath)
	
	// 获取锁
	err := lock.Lock(1 * time.Second)
	if err != nil {
		t.Fatalf("Lock 失败: %v", err)
	}

	// 强制解锁
	err = lock.ForceUnlock()
	if err != nil {
		t.Fatalf("ForceUnlock 失败: %v", err)
	}

	// 验证锁文件被删除
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatal("锁文件应该被删除")
	}
}
