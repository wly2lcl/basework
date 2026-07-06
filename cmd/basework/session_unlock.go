//go:build sqlite

package main

import (
	"fmt"
	"os"

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
	// 确定锁文件路径
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("获取用户主目录失败: %w", err)
	}

	lockPath := fmt.Sprintf("%s/.basework/sessions/%s.lock", home, sessionID)

	// 创建锁并强制解锁
	lock := session.NewFileLock(lockPath)
	if err := lock.ForceUnlock(); err != nil {
		return fmt.Errorf("强制解锁失败: %w", err)
	}

	fmt.Printf("✅ 已强制解锁会话 %s\n", sessionID[:12])
	fmt.Printf("🗑 已删除锁文件: %s\n", lockPath)
	fmt.Printf("⚠️  警告：如果其他进程正在写入，可能导致数据损坏\n")

	return nil
}
