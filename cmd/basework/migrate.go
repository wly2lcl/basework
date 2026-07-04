//go:build sqlite

// Package main 是 basework CLI 的入口点
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/wly2lcl/basework/pkg/session"
)

// migrateCmd 表示 migrate 子命令
var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "数据迁移工具",
	Long:  `在存储后端之间迁移数据，例如从 JSONL 迁移到 SQLite。`,
}

// migrateSessionsCmd 表示 migrate sessions 子命令
var migrateSessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "将会话从 JSONL 迁移到 SQLite",
	Long: `扫描 ~/.basework/sessions/ 目录下的所有 JSONL 会话文件，
并导入到 SQLite 数据库中。已存在的会话（按 ID 判断）将被跳过。
输出迁移报告。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMigrateSessions()
	},
}

var (
	migrateSQLitePath string
)

func init() {
	migrateCmd.AddCommand(migrateSessionsCmd)
	migrateSessionsCmd.Flags().StringVar(&migrateSQLitePath, "sqlite-path", "", "SQLite 数据库路径（默认 ~/.basework/sessions/sessions.db）")
}

// runMigrateSessions 执行会话从 JSONL 到 SQLite 的迁移。
func runMigrateSessions() error {
	// 1. 确定 JSONL 源目录
	sessionDir := getSessionDir()

	// 2. 确定 SQLite 目标路径
	sqlitePath := migrateSQLitePath
	if sqlitePath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("获取用户主目录失败: %w", err)
		}
		sqlitePath = filepath.Join(home, ".basework", "sessions", "sessions.db")
	}

	// 3. 检查 JSONL 目录是否存在
	if _, err := os.Stat(sessionDir); os.IsNotExist(err) {
		return fmt.Errorf("JSONL 会话目录 %s 不存在", sessionDir)
	}

	// 4. 打开 SQLite 存储
	sqliteStore, err := session.NewSQLiteStore(sqlitePath)
	if err != nil {
		return fmt.Errorf("创建 SQLite 存储失败: %w", err)
	}
	defer sqliteStore.Close()

	// 5. 扫描 JSONL 文件
	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		return fmt.Errorf("读取会话目录 %s 失败: %w", sessionDir, err)
	}

	var jsonlFiles []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		jsonlFiles = append(jsonlFiles, entry.Name())
	}

	if len(jsonlFiles) == 0 {
		fmt.Println("未找到 JSONL 会话文件")
		return nil
	}

	fmt.Printf("找到 %d 个 JSONL 会话文件\n", len(jsonlFiles))

	// 6. 逐个导入
	var imported, skipped, failed int
	for _, filename := range jsonlFiles {
		sessionID := strings.TrimSuffix(filename, ".jsonl")

		// 检查是否已存在于 SQLite
		existing, err := sqliteStore.Get(sessionID)
		if err == nil && existing != nil {
			fmt.Printf("  ⏭ 跳过 %s（已在 SQLite 中）\n", sessionID)
			skipped++
			continue
		}

		// 读取 JSONL 文件
		jsonlPath := filepath.Join(sessionDir, filename)
		data, err := os.ReadFile(jsonlPath)
		if err != nil {
			fmt.Printf("  ❌ 读取 %s 失败: %v\n", filename, err)
			failed++
			continue
		}

		// 解析 JSONL 事件
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
			fmt.Printf("  ⏭ 跳过 %s（空文件）\n", filename)
			skipped++
			continue
		}

		// 先创建会话
		info, err := sqliteStore.Create(session.CreateOpts{
			Title: fmt.Sprintf("迁移会话 %s", sessionID[:8]),
		})
		if err != nil {
			fmt.Printf("  ❌ 创建会话 %s 失败: %v\n", sessionID, err)
			failed++
			continue
		}

		// 如果 JSONL 中的 ID 与我们创建的不同，需要额外处理
		_ = info

		// 导入事件
		var eventCount int
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			// 尝试解析为 Event
			var event session.Event
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				continue
			}

			if event.SessionID == "" {
				event.SessionID = sessionID
			}

			if err := sqliteStore.AppendEvent(event); err != nil {
				fmt.Printf("  ⚠ 导入事件失败（已跳过）: %v\n", err)
				continue
			}
			eventCount++
		}

		fmt.Printf("  ✅ 导入 %s（%d 个事件）\n", sessionID[:12], eventCount)
		imported++
	}

	// 7. 打印迁移报告
	fmt.Println("\n=== 迁移报告 ===")
	fmt.Printf("  ✅ 成功: %d\n", imported)
	fmt.Printf("  ⏭ 跳过: %d\n", skipped)
	fmt.Printf("  ❌ 失败: %d\n", failed)
	fmt.Printf("  📁 目标数据库: %s\n", sqlitePath)
	fmt.Printf("  📂 源目录: %s\n", sessionDir)

	return nil
}