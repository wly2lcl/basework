//go:build sqlite

package main

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/wly2lcl/basework/pkg/session"
)

// sessionUnlockCmd 表示 session unlock 子命令
var sessionUnlockCmd = &cobra.Command{
	Use:   "unlock [session-id]",
	Short: "强制解锁会话",
	Long:  `强制删除会话锁文件，用于死锁恢复。`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSessionUnlock(args[0])
	},
}

func init() {
	sessionCmd.AddCommand(sessionUnlockCmd)
}

// runSessionUnlock 强制解锁会话
func runSessionUnlock(sessionID string) error {
	// 锁文件必须与 agent 实际写入位置一致。旧版本硬编码 ~/.basework/sessions，
	// 导致 unlock 删除的并不是真实锁文件。
	lockPath := filepath.Join(resolveSessionDir(), sessionID+".lock")

	// 创建锁并强制解锁
	lock := session.NewFileLock(lockPath)
	if err := lock.ForceUnlock(); err != nil {
		return fmt.Errorf("强制解锁失败: %w", err)
	}

	fmt.Printf("✅ 已强制解锁会话 %s\n", shortSessionID(sessionID))
	fmt.Printf("🗑 已删除锁文件: %s\n", lockPath)
	fmt.Printf("⚠️  警告：如果其他进程正在写入，可能导致数据损坏\n")

	return nil
}
