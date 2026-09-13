//go:build sqlite

// Package main 是 basework CLI 的入口点
package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/wly2lcl/basework/pkg/session"
	_ "modernc.org/sqlite"
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
	Long: `扫描会话目录下的所有 JSONL 会话文件，并导入到 SQLite 数据库中。
默认源目录为 ~/.local/share/basework/sessions/（规范目录）；
若规范目录不存在而历史目录 ~/.basework/sessions/ 存在，则回退到历史目录。
已存在的会话（按 ID 判断）将被跳过。输出迁移报告。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMigrateSessions()
	},
}

// migrateWalCmd 表示 migrate --to-wal 子命令
var migrateWalCmd = &cobra.Command{
	Use:   "to-wal",
	Short: "将会话数据库迁移到 WAL 模式",
	Long: `将 SQLite 会话数据库从 DELETE 模式迁移到 WAL (Write-Ahead Logging) 模式。
WAL 模式支持并发读写，显著提升性能。
迁移前会自动备份数据库。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMigrateToWAL()
	},
}

// migrateRollbackCmd 表示 migrate --rollback 子命令
var migrateRollbackCmd = &cobra.Command{
	Use:   "rollback",
	Short: "回滚 WAL 模式迁移",
	Long: `从 WAL 模式回滚到 DELETE 模式。
如果存在备份文件，会从备份恢复。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMigrateRollback()
	},
}

var (
	migrateSQLitePath string
)

func init() {
	migrateCmd.AddCommand(migrateSessionsCmd)
	migrateCmd.AddCommand(migrateWalCmd)
	migrateCmd.AddCommand(migrateRollbackCmd)
	migrateSessionsCmd.Flags().StringVar(&migrateSQLitePath, "sqlite-path", "", "SQLite 数据库路径（默认为会话规范目录下的 sessions.db）")
}

// resolveMigrateSQLitePath 返回迁移相关的 SQLite 数据库路径。
//
// 默认值与会话规范目录保持一致。旧版本硬编码 ~/.basework/sessions，
// 与 agent 实际写入位置不符，导致迁移读一处、写另一处。
func resolveMigrateSQLitePath() string {
	if migrateSQLitePath != "" {
		return migrateSQLitePath
	}
	return filepath.Join(resolveSessionDir(), "sessions.db")
}

// runMigrateSessions 执行会话从 JSONL 到 SQLite 的迁移。
func runMigrateSessions() error {
	// 1. 确定 JSONL 源目录
	//
	// 用 resolveSessionDir() 而非 getSessionDir()：仅有历史目录
	// (~/.basework/sessions) 的用户也应能迁移，而不是直接报「目录不存在」。
	sessionDir := resolveSessionDir()

	// 2. 确定 SQLite 目标路径
	sqlitePath := resolveMigrateSQLitePath()

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

	// 通过 JSONLStore 读取源事件，而不是自己按行 json.Unmarshal。
	//
	// 自建解析会绕过 readEvents 的两件事：文件头识别与**格式版本判定 + 相邻迁移链**。
	// 实测后果：一个 v99 的会话文件会被"成功"导入当前库——未来版本的数据越过
	// 「读到更高版本即拒绝」的契约进入了本地存储（见 SHIP-002 记录）。
	// 走存储读取还顺带保证：低版本事件先升级到当前版本再落库。
	srcStore, err := session.NewJSONLStore(sessionDir)
	if err != nil {
		return fmt.Errorf("打开源会话存储失败: %w", err)
	}

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

		events, err := srcStore.Events(session.EventFilter{SessionID: sessionID})
		if err != nil {
			fmt.Printf("  ❌ 读取 %s 失败: %v\n", filename, err)
			failed++
			continue
		}
		if len(events) == 0 {
			fmt.Printf("  ⏭ 跳过 %s（无事件）\n", filename)
			skipped++
			continue
		}

		// 先按**原 ID** 创建会话。
		//
		// 必须保留原 ID：事件里带的 session_id 就是原 ID。若这里另发新 ID，
		// 每条事件的外键都对不上，逐条失败后被跳过，最终"报告成功、零条落库"。
		if _, err := sqliteStore.Create(session.CreateOpts{
			ID:    sessionID,
			Title: fmt.Sprintf("迁移会话 %s", shortSessionID(sessionID)),
		}); err != nil {
			fmt.Printf("  ❌ 创建会话 %s 失败: %v\n", sessionID, err)
			failed++
			continue
		}

		// 导入事件
		var eventCount, eventFailed int
		for _, event := range events {
			if event.SessionID == "" {
				event.SessionID = sessionID
			}
			if err := sqliteStore.AppendEvent(event); err != nil {
				fmt.Printf("  ⚠ 导入事件失败（已跳过）: %v\n", err)
				eventFailed++
				continue
			}
			eventCount++
		}

		// 「零条导入」不能算成功：这正是本次验证踩到的坑——事件全部失败被跳过，
		// 输出却是 ✅，用户以为迁移完成而数据一条没进来。
		if eventCount == 0 {
			fmt.Printf("  ❌ 导入 %s 失败：没有事件成功落库（失败 %d 行）\n",
				shortSessionID(sessionID), eventFailed)
			failed++
			continue
		}

		fmt.Printf("  ✅ 导入 %s（%d 个事件", shortSessionID(sessionID), eventCount)
		if eventFailed > 0 {
			fmt.Printf("，跳过 %d 行", eventFailed)
		}
		fmt.Println("）")
		imported++
	}

	// 7. 打印迁移报告
	fmt.Println("\n=== 迁移报告 ===")
	fmt.Printf("  ✅ 成功: %d\n", imported)
	fmt.Printf("  ⏭ 跳过: %d\n", skipped)
	fmt.Printf("  ❌ 失败: %d\n", failed)
	fmt.Printf("  📁 目标数据库: %s\n", sqlitePath)
	fmt.Printf("  📂 源目录: %s\n", sessionDir)

	// 有失败就必须以非零退出码结束。
	//
	// 报告里写「❌ 失败: 1」却返回 0，脚本与 CI 只会看到「成功」——
	// 这正是「隐藏错误」的形态：人看到了，自动化没看到。版本过高的会话文件
	// 就落在这一类（必须在写任何数据之前就拒绝）。
	if failed > 0 {
		return fmt.Errorf("会话迁移未全部成功：%d 个失败", failed)
	}

	return nil
}

// runMigrateToWAL 将会话数据库迁移到 WAL 模式。
func runMigrateToWAL() error {
	// 确定 SQLite 数据库路径
	sqlitePath := resolveMigrateSQLitePath()

	// 检查数据库是否存在
	if _, err := os.Stat(sqlitePath); os.IsNotExist(err) {
		return fmt.Errorf("数据库 %s 不存在", sqlitePath)
	}

	// 1. 备份数据库
	backupPath := sqlitePath + ".backup-" + fmt.Sprintf("%d", os.Getpid())
	fmt.Printf("📦 备份数据库到 %s\n", backupPath)

	backupData, err := os.ReadFile(sqlitePath)
	if err != nil {
		return fmt.Errorf("读取数据库失败: %w", err)
	}

	if err := os.WriteFile(backupPath, backupData, 0644); err != nil {
		return fmt.Errorf("创建备份失败: %w", err)
	}

	// 2. 打开数据库并切换到 WAL 模式
	db, err := sql.Open("sqlite", sqlitePath)
	if err != nil {
		return fmt.Errorf("打开数据库失败: %w", err)
	}
	defer db.Close()

	// 3. 检查当前模式
	var currentMode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&currentMode); err != nil {
		return fmt.Errorf("检查当前模式失败: %w", err)
	}

	if currentMode == "wal" {
		fmt.Println("✅ 数据库已经是 WAL 模式")
		// 删除备份
		os.Remove(backupPath)
		return nil
	}

	fmt.Printf("🔄 当前模式: %s，迁移到 WAL...\n", currentMode)

	// 4. 切换到 WAL 模式
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		// 迁移失败，恢复备份
		fmt.Printf("❌ 迁移失败，恢复备份...\n")
		os.WriteFile(sqlitePath, backupData, 0644)
		os.Remove(backupPath)
		return fmt.Errorf("切换到 WAL 模式失败: %w", err)
	}

	// 5. 验证迁移成功
	var newMode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&newMode); err != nil {
		return fmt.Errorf("验证模式失败: %w", err)
	}

	if newMode != "wal" {
		fmt.Printf("❌ 迁移失败，恢复备份...\n")
		os.WriteFile(sqlitePath, backupData, 0644)
		os.Remove(backupPath)
		return fmt.Errorf("验证失败：期望 wal，实际 %s", newMode)
	}

	fmt.Printf("✅ 成功迁移到 WAL 模式\n")
	fmt.Printf("📦 备份保留在: %s\n", backupPath)
	fmt.Printf("💡 如果确认迁移成功，可以手动删除备份文件\n")

	return nil
}

// runMigrateRollback 从 WAL 模式回滚到 DELETE 模式。
func runMigrateRollback() error {
	// 确定 SQLite 数据库路径
	sqlitePath := resolveMigrateSQLitePath()

	// 1. 查找最新的备份文件
	backupPattern := sqlitePath + ".backup-*"
	matches, err := filepath.Glob(backupPattern)
	if err != nil || len(matches) == 0 {
		return fmt.Errorf("未找到备份文件（模式: %s）", backupPattern)
	}

	// 使用最新的备份
	latestBackup := matches[len(matches)-1]
	fmt.Printf("📦 找到备份文件: %s\n", latestBackup)

	// 2. 从备份恢复
	backupData, err := os.ReadFile(latestBackup)
	if err != nil {
		return fmt.Errorf("读取备份失败: %w", err)
	}

	if err := os.WriteFile(sqlitePath, backupData, 0644); err != nil {
		return fmt.Errorf("恢复备份失败: %w", err)
	}

	// 3. 删除 WAL 相关文件
	walFile := sqlitePath + "-wal"
	shmFile := sqlitePath + "-shm"
	os.Remove(walFile)
	os.Remove(shmFile)

	// 4. 验证恢复
	db, err := sql.Open("sqlite", sqlitePath)
	if err != nil {
		return fmt.Errorf("打开数据库失败: %w", err)
	}
	defer db.Close()

	var mode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		return fmt.Errorf("验证模式失败: %w", err)
	}

	fmt.Printf("✅ 成功回滚到 %s 模式\n", mode)
	fmt.Printf("🗑 已删除 WAL 文件\n")

	return nil
}
