//go:build windows

package session

import (
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"
)

var (
	modkernel32    = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx = modkernel32.NewProc("LockFileEx")
)

// FileLock 实现基于 LockFileEx 的文件锁（Windows 平台）。
type FileLock struct {
	path string
	file *os.File
}

// NewFileLock 创建一个新的文件锁。
func NewFileLock(path string) *FileLock {
	return &FileLock{path: path}
}

// Lock 获取排他锁（非阻塞模式，带超时）。
func (l *FileLock) Lock(timeout time.Duration) error {
	file, err := os.OpenFile(l.path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("session: 打开锁文件失败: %w", err)
	}
	l.file = file

	deadline := time.Now().Add(timeout)
	for {
		// Windows LOCKFILE_FAIL_IMMEDIATELY = 0x00000001
		// LOCKFILE_EXCLUSIVE_LOCK = 0x00000002
		const LOCKFILE_FAIL_IMMEDIATELY = 0x00000001
		const LOCKFILE_EXCLUSIVE_LOCK = 0x00000002

		var overlapped syscall.Overlapped
		ret, _, _ := procLockFileEx.Call(
			file.Fd(),
			LOCKFILE_EXCLUSIVE_LOCK|LOCKFILE_FAIL_IMMEDIATELY,
			0, // dwFlags reserved
			1, // nNumberOfBytesToLockLow
			0, // nNumberOfBytesToLockHigh
			(uintptr)(unsafe.Pointer(&overlapped)),
		)

		if ret != 0 {
			return nil // 获取锁成功
		}

		// 检查是否超时
		if time.Now().After(deadline) {
			file.Close()
			return fmt.Errorf("session: 获取锁超时（%v）", timeout)
		}

		// 等待后重试
		time.Sleep(100 * time.Millisecond)
	}
}

// Unlock 释放锁。
func (l *FileLock) Unlock() error {
	if l.file == nil {
		return nil
	}

	// Windows 关闭文件时自动释放锁
	l.file.Close()
	l.file = nil
	return nil
}

// ForceUnlock 强制删除锁文件（用于死锁恢复）。
func (l *FileLock) ForceUnlock() error {
	if l.file != nil {
		if err := l.Unlock(); err != nil {
			return err
		}
	}

	err := os.Remove(l.path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("session: 强制解锁失败: %w", err)
	}
	return nil
}
