//go:build sqlite

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/wly2lcl/basework/pkg/session"
)

var (
	checkIntegrity bool
)

// sessionStatusCmd 表示 session status 子命令
var sessionStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "显示会话状态和健康信息",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSessionStatus()
	},
}

func init() {
	sessionCmd.AddCommand(sessionStatusCmd)
	sessionStatusCmd.Flags().BoolVar(&checkIntegrity, "check-integrity", false, "检查所有会话的完整性")
}

// runSessionStatus 显示会话状态
func runSessionStatus() error {
	// 与 agent 实际写入位置保持一致（旧版本此处硬编码 ~/.basework/sessions，
	// 导致 status 读取的目录与真实会话目录不同）。
	sessionDir := resolveSessionDir()

	if checkIntegrity {
		return checkAllSessionsIntegrity(sessionDir)
	}

	// 显示会话列表和基本信息
	s, err := session.NewSQLiteStore(filepath.Join(sessionDir, "sessions.db"))
	if err != nil {
		return fmt.Errorf("打开会话存储失败: %w", err)
	}
	defer s.Close()

	infos, err := s.List(session.ListFilter{Limit: 100})
	if err != nil {
		return fmt.Errorf("列举会话失败: %w", err)
	}

	if len(infos) == 0 {
		fmt.Println("没有找到会话")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "ID\t标题\t消息数\t创建时间\t最后更新")
	fmt.Fprintln(w, "--\t----\t------\t--------\t--------")

	for _, info := range infos {
		shortID := shortSessionID(info.ID)
		title := info.Title
		if title == "" {
			title = "(无标题)"
		}
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\n",
			shortID,
			title,
			info.MessageCount,
			info.CreatedAt.Format(time.RFC3339),
			info.UpdatedAt.Format(time.RFC3339),
		)
	}

	return w.Flush()
}

// checkAllSessionsIntegrity 检查所有会话的完整性
func checkAllSessionsIntegrity(sessionDir string) error {
	fmt.Println("🔍 检查所有会话完整性...")
	fmt.Println()

	corrupted, err := session.ListCorruptedSessions(sessionDir)
	if err != nil {
		return fmt.Errorf("检查失败: %w", err)
	}

	if len(corrupted) == 0 {
		fmt.Println("✅ 所有会话完整，无损坏")
		return nil
	}

	fmt.Printf("❌ 发现 %d 个损坏的会话:\n", len(corrupted))
	fmt.Println()

	for _, status := range corrupted {
		fmt.Printf("  会话: %s\n", status.SessionID)
		fmt.Printf("  检查时间: %s\n", status.CheckedAt.Format(time.RFC3339))
		if status.Error != nil {
			fmt.Printf("  错误: %v\n", status.Error)
		}
		fmt.Println()
	}

	return nil
}
