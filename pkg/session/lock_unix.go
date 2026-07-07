//go:build !windows

package session

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

// FileLock 实现基于 flock 的文件锁（Unix 平台）。
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
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
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

	err := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	l.file.Close()
	l.file = nil

	if err != nil {
		return fmt.Errorf("session: 释放锁失败: %w", err)
	}
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
