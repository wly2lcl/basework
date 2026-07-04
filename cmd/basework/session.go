// Package main 是 basework CLI 的入口点
package main

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/wly2lcl/basework/pkg/session"
)

// sessionCmd 表示 session 子命令
var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "管理会话",
	Long:  `列出和管理 Agent 会话。`,
}

// sessionListCmd 表示 session list 子命令
var sessionListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出所有会话",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSessionList()
	},
}

// sessionClearCmd 表示 session clear 子命令
var sessionClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "清除所有会话",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSessionClear()
	},
}

var (
	showTrackedFiles bool
)

func init() {
	sessionCmd.AddCommand(sessionListCmd)
	sessionCmd.AddCommand(sessionClearCmd)
	sessionListCmd.Flags().BoolVar(&showTrackedFiles, "tracked-files", false, "显示每个会话追踪的文件数量")
}

// openSessionStore 打开会话存储
func openSessionStore() (*session.JSONLStore, error) {
	dir := getSessionDir()
	return session.NewJSONLStore(dir)
}

// runSessionList 列出所有会话
func runSessionList() error {
	s, err := openSessionStore()
	if err != nil {
		return fmt.Errorf("打开会话存储失败: %w", err)
	}

	infos, err := s.List(session.ListFilter{Limit: 100})
	if err != nil {
		return fmt.Errorf("列举会话失败: %w", err)
	}

	if len(infos) == 0 {
		fmt.Println("没有找到会话")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	if showTrackedFiles {
		fmt.Fprintln(w, "ID\t标题\t创建时间\t消息数")
		fmt.Fprintln(w, "--\t----\t--------\t------")
	} else {
		fmt.Fprintln(w, "ID\t标题\t消息数\t创建时间")
		fmt.Fprintln(w, "--\t----\t--------\t--------")
	}
	for _, info := range infos {
		shortID := info.ID
		if len(shortID) > 12 {
			shortID = shortID[:12]
		}
		title := info.Title
		if title == "" {
			title = "(无标题)"
		}
		if showTrackedFiles {
			fmt.Fprintf(w, "%s\t%s\t%s\t%d\n",
				shortID,
				title,
				info.CreatedAt.Format(time.RFC3339),
				info.MessageCount,
			)
		} else {
			fmt.Fprintf(w, "%s\t%s\t%d\t%s\n",
				shortID,
				title,
				info.MessageCount,
				info.CreatedAt.Format(time.RFC3339),
			)
		}
	}
	return w.Flush()
}

// runSessionClear 清除所有会话
func runSessionClear() error {
	s, err := openSessionStore()
	if err != nil {
		return fmt.Errorf("打开会话存储失败: %w", err)
	}

	infos, err := s.List(session.ListFilter{Limit: 1000})
	if err != nil {
		return fmt.Errorf("列举会话失败: %w", err)
	}

	if len(infos) == 0 {
		fmt.Println("没有需要清除的会话")
		return nil
	}

	for _, info := range infos {
		if err := s.Delete(info.ID); err != nil {
			fmt.Fprintf(os.Stderr, "删除会话 %s 失败: %v\n", info.ID, err)
		}
	}
	fmt.Printf("已清除 %d 个会话\n", len(infos))
	return nil
}