//go:build sqlite

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/wly2lcl/basework/internal/permission"
)

// permissionCmd 表示 permission 子命令
var permissionCmd = &cobra.Command{
	Use:   "permission",
	Short: "管理权限规则",
	Long:  `管理工具的权限规则，支持查看、添加、删除和审计。`,
}

// permissionListCmd 表示 permission list 子命令
var permissionListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出所有权限规则",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPermissionList()
	},
}

// permissionAddCmd 表示 permission add 子命令
var permissionAddCmd = &cobra.Command{
	Use:   "add",
	Short: "添加权限规则",
	Long: `添加一条新的权限规则。
示例:
  basework permission add --type allow --pattern "read_*" --scope global
  basework permission add --type deny --pattern "write_file" --scope session`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPermissionAdd()
	},
}

// permissionRemoveCmd 表示 permission remove 子命令
var permissionRemoveCmd = &cobra.Command{
	Use:   "remove",
	Short: "删除权限规则",
	Long:  `按 ID 删除一条权限规则。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPermissionRemove()
	},
}

// permissionAuditCmd 表示 permission audit 子命令
var permissionAuditCmd = &cobra.Command{
	Use:   "audit",
	Short: "查看审计日志",
	Long:  `查看权限审计日志，可按 session、工具名和时间范围过滤。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPermissionAudit()
	},
}

// permissionExportCmd 表示 permission export 子命令
var permissionExportCmd = &cobra.Command{
	Use:   "export",
	Short: "导出权限规则",
	Long:  `将权限规则导出为 JSON 文件。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPermissionExport()
	},
}

// permissionImportCmd 表示 permission import 子命令
var permissionImportCmd = &cobra.Command{
	Use:   "import",
	Short: "导入权限规则",
	Long:  `从 JSON 文件导入权限规则。`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPermissionImport(args[0])
	},
}

var (
	permissionType    string
	permissionPattern string
	permissionScope   string
	permissionSession string
	permissionProject string
	permissionID      string
	permissionOutput  string
	auditSession      string
	auditTool         string
	auditDays         int
)

func init() {
	// 在 sqlite build tag 启用时注册 permission 子命令
	rootCmd.AddCommand(permissionCmd)

	permissionCmd.AddCommand(permissionListCmd)
	permissionCmd.AddCommand(permissionAddCmd)
	permissionCmd.AddCommand(permissionRemoveCmd)
	permissionCmd.AddCommand(permissionAuditCmd)
	permissionCmd.AddCommand(permissionExportCmd)
	permissionCmd.AddCommand(permissionImportCmd)

	permissionAddCmd.Flags().StringVar(&permissionType, "type", "", "规则类型: allow|deny|ask")
	permissionAddCmd.Flags().StringVar(&permissionPattern, "pattern", "", "工具名或路径模式（支持 glob）")
	permissionAddCmd.Flags().StringVar(&permissionScope, "scope", "global", "作用域: global|session|project")
	permissionAddCmd.Flags().StringVar(&permissionSession, "session", "", "session 作用域关联的会话 ID")
	permissionAddCmd.Flags().StringVar(&permissionProject, "project", "", "project 作用域关联的工作区/项目 ID")
	permissionAddCmd.MarkFlagRequired("type")
	permissionAddCmd.MarkFlagRequired("pattern")

	permissionRemoveCmd.Flags().StringVar(&permissionID, "id", "", "规则 ID")
	permissionRemoveCmd.MarkFlagRequired("id")

	permissionAuditCmd.Flags().StringVar(&auditSession, "session", "", "按会话 ID 过滤")
	permissionAuditCmd.Flags().StringVar(&auditTool, "tool", "", "按工具名过滤")
	permissionAuditCmd.Flags().IntVar(&auditDays, "days", 7, "查看最近几天的记录")

	permissionExportCmd.Flags().StringVar(&permissionOutput, "output", "", "输出文件路径（默认 stdout）")
}

// getPermissionDBPath 获取权限数据库路径。
func getPermissionDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "basework_permissions.db")
	}
	return filepath.Join(home, ".basework", "permissions.db")
}

// openPermissionStore 打开权限存储。
func openPermissionStore() (*permission.SQLiteStore, error) {
	dbPath := getPermissionDBPath()
	return permission.NewSQLiteStore(dbPath)
}

// runPermissionList 列出所有权限规则。
func runPermissionList() error {
	store, err := openPermissionStore()
	if err != nil {
		return fmt.Errorf("打开权限存储失败: %w", err)
	}
	defer store.Close()

	rules, err := store.List()
	if err != nil {
		return fmt.Errorf("列举规则失败: %w", err)
	}

	if len(rules) == 0 {
		fmt.Println("没有找到权限规则")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "ID\t类型\t模式\t作用域\t会话\t项目\t来源\t创建时间")
	fmt.Fprintln(w, "--\t----\t----\t------\t----\t----\t----\t--------")
	for _, r := range rules {
		shortID := r.ID
		if len(shortID) > 12 {
			shortID = shortID[:12]
		}
		sessionID := r.SessionID
		projectID := r.ProjectID
		if len(sessionID) > 12 {
			sessionID = sessionID[:12]
		}
		if len(projectID) > 12 {
			projectID = projectID[:12]
		}
		if sessionID == "" {
			sessionID = "-"
		}
		if projectID == "" {
			projectID = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			shortID,
			r.RuleType,
			r.Pattern,
			r.Scope,
			sessionID,
			projectID,
			r.Source,
			r.CreatedAt.Format("2006-01-02 15:04"),
		)
	}
	return w.Flush()
}

// runPermissionAdd 添加权限规则。
func runPermissionAdd() error {
	// 验证规则类型
	validTypes := map[string]bool{"allow": true, "deny": true, "ask": true}
	if !validTypes[permissionType] {
		return fmt.Errorf("无效的规则类型: %q (有效: allow, deny, ask)", permissionType)
	}
	scope := strings.ToLower(strings.TrimSpace(permissionScope))
	if scope != "global" && scope != "session" && scope != "project" {
		return fmt.Errorf("无效的作用域: %q (有效: global, session, project)", permissionScope)
	}
	if scope == "session" && strings.TrimSpace(permissionSession) == "" {
		return fmt.Errorf("session 作用域必须提供 --session")
	}
	if scope == "project" && strings.TrimSpace(permissionProject) == "" {
		return fmt.Errorf("project 作用域必须提供 --project")
	}

	store, err := openPermissionStore()
	if err != nil {
		return fmt.Errorf("打开权限存储失败: %w", err)
	}
	defer store.Close()

	rule := &permission.StoredRule{
		RuleType:  permissionType,
		Pattern:   permissionPattern,
		Scope:     scope,
		SessionID: strings.TrimSpace(permissionSession),
		ProjectID: strings.TrimSpace(permissionProject),
		Source:    "user",
	}

	if err := store.Create(rule); err != nil {
		return fmt.Errorf("创建规则失败: %w", err)
	}

	fmt.Printf("✅ 已创建权限规则: %s\n", rule.ID)
	fmt.Printf("   类型: %s, 模式: %s, 作用域: %s\n", rule.RuleType, rule.Pattern, rule.Scope)
	return nil
}

// runPermissionRemove 删除权限规则。
func runPermissionRemove() error {
	store, err := openPermissionStore()
	if err != nil {
		return fmt.Errorf("打开权限存储失败: %w", err)
	}
	defer store.Close()

	if err := store.Delete(permissionID); err != nil {
		return fmt.Errorf("删除规则失败: %w", err)
	}

	fmt.Printf("✅ 已删除权限规则: %s\n", permissionID)
	return nil
}

// runPermissionAudit 查看审计日志。
func runPermissionAudit() error {
	dbPath := getPermissionDBPath()
	store, err := permission.NewSQLiteStore(dbPath)
	if err != nil {
		return fmt.Errorf("打开数据库失败: %w", err)
	}
	defer store.Close()

	logger := permission.NewAuditLoggerWithDB(store.GetDB())
	defer logger.Close()

	// 计算起始时间
	end := time.Now().UTC()
	start := end.AddDate(0, 0, -auditDays)

	records, err := logger.Query(auditSession, auditTool, start, end, 100)
	if err != nil {
		return fmt.Errorf("查询审计日志失败: %w", err)
	}

	if len(records) == 0 {
		fmt.Println("没有找到匹配的审计记录")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "时间\t会话\t项目\t工具\t决策\t规则ID")
	fmt.Fprintln(w, "----\t----\t----\t----\t----\t------")
	for _, r := range records {
		sessionID := r.SessionID
		projectID := r.ProjectID
		if len(sessionID) > 12 {
			sessionID = sessionID[:12]
		}
		if len(projectID) > 12 {
			projectID = projectID[:12]
		}
		if sessionID == "" {
			sessionID = "-"
		}
		if projectID == "" {
			projectID = "-"
		}
		ruleID := r.RuleID
		if len(ruleID) > 12 {
			ruleID = ruleID[:12]
		}
		if ruleID == "" {
			ruleID = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			r.Timestamp.Format("2006-01-02 15:04:05"),
			sessionID,
			projectID,
			r.ToolName,
			r.Decision,
			ruleID,
		)
	}
	return w.Flush()
}

// runPermissionExport 导出权限规则。
func runPermissionExport() error {
	store, err := openPermissionStore()
	if err != nil {
		return fmt.Errorf("打开权限存储失败: %w", err)
	}
	defer store.Close()

	data, err := permission.ExportRules(store)
	if err != nil {
		return fmt.Errorf("导出规则失败: %w", err)
	}

	if permissionOutput == "" {
		// 输出到 stdout
		fmt.Println(string(data))
		return nil
	}

	if err := os.WriteFile(permissionOutput, data, 0644); err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}

	fmt.Printf("✅ 已导出 %d 条规则到 %s\n", len(data), permissionOutput)
	return nil
}

// runPermissionImport 导入权限规则。
func runPermissionImport(filePath string) error {
	store, err := openPermissionStore()
	if err != nil {
		return fmt.Errorf("打开权限存储失败: %w", err)
	}
	defer store.Close()

	// 打开文件
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("打开文件失败: %w", err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return fmt.Errorf("读取文件失败: %w", err)
	}

	imported, err := permission.ImportRules(store, data)
	if err != nil {
		return fmt.Errorf("导入规则失败: %w", err)
	}

	// 验证导入结果
	if err := permission.VerifyImport(store, imported); err != nil {
		fmt.Fprintf(os.Stderr, "⚠ 导入验证警告: %v\n", err)
	}

	fmt.Printf("✅ 已导入 %d 条权限规则\n", imported)
	return nil
}

// formatJSON 格式化 JSON 输出。
func formatJSON(v interface{}) string {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(data)
}
