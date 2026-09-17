//go:build sqlite

package permission

import (
	"database/sql"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// SQLiteStore 是基于 SQLite 的权限规则持久化存储实现。
// 使用 modernc.org/sqlite（纯 Go，无 CGO）。
// 通过 build tag "sqlite" 控制编译。
type SQLiteStore struct {
	mu   sync.RWMutex
	db   *sql.DB
	path string
}

// NewSQLiteStore 创建一个新的 SQLiteStore。
// dbPath 是 SQLite 数据库文件的路径。
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("permission: 创建目录 %s 失败: %w", dir, err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("permission: 打开 SQLite 数据库失败: %w", err)
	}

	// 设置连接参数
	db.SetMaxOpenConns(3)
	db.SetMaxIdleConns(1)

	s := &SQLiteStore{
		db:   db,
		path: dbPath,
	}

	if err := s.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("permission: 初始化表结构失败: %w", err)
	}

	return s, nil
}

// initSchema 创建数据库表结构。
func (s *SQLiteStore) initSchema() error {
	schema := `
	PRAGMA journal_mode=WAL;
	PRAGMA foreign_keys=ON;

	CREATE TABLE IF NOT EXISTS permissions (
		id TEXT PRIMARY KEY,
		rule_type TEXT NOT NULL,
		pattern TEXT NOT NULL,
		scope TEXT NOT NULL DEFAULT 'global',
		session_id TEXT DEFAULT '',
		project_id TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		source TEXT DEFAULT 'user'
	);

	CREATE INDEX IF NOT EXISTS idx_permissions_scope ON permissions(scope);
	CREATE INDEX IF NOT EXISTS idx_permissions_pattern ON permissions(pattern);

	CREATE TABLE IF NOT EXISTS permission_audit (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT DEFAULT '',
		project_id TEXT DEFAULT '',
		tool_name TEXT NOT NULL,
		rule_id TEXT DEFAULT '',
		decision TEXT NOT NULL,
		context TEXT DEFAULT '{}',
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_audit_session ON permission_audit(session_id);
	CREATE INDEX IF NOT EXISTS idx_audit_tool ON permission_audit(tool_name);
	CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON permission_audit(timestamp);
	`
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}
	// Older databases predate project_id on audit rows. Add it before creating
	// the index; existing rows safely read as the empty, unbound project.
	if err := ensureColumn(s.db, "permission_audit", "project_id", "TEXT DEFAULT ''"); err != nil {
		return err
	}
	_, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_audit_project ON permission_audit(project_id)`)
	return err
}

func ensureColumn(db *sql.DB, table, column, definition string) error {
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return err
	}
	defer rows.Close()
	var found bool
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull, pk int
		var defaultValue interface{}
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if name == column {
			found = true
			break
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if found {
		return nil
	}
	_, err = db.Exec("ALTER TABLE " + table + " ADD COLUMN " + column + " " + definition)
	return err
}

// Close 关闭数据库连接。
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// Create 创建权限规则。
func (s *SQLiteStore) Create(rule *StoredRule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if rule.ID == "" {
		rule.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	if rule.CreatedAt.IsZero() {
		rule.CreatedAt = now
	}
	if rule.UpdatedAt.IsZero() {
		rule.UpdatedAt = now
	}
	if rule.Scope == "" {
		rule.Scope = "global"
	}
	if rule.Source == "" {
		rule.Source = "user"
	}

	_, err := s.db.Exec(
		`INSERT INTO permissions (id, rule_type, pattern, scope, session_id, project_id, created_at, updated_at, source)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rule.ID, rule.RuleType, rule.Pattern, rule.Scope,
		rule.SessionID, rule.ProjectID,
		rule.CreatedAt.Format(time.RFC3339), rule.UpdatedAt.Format(time.RFC3339),
		rule.Source,
	)
	if err != nil {
		return fmt.Errorf("permission: 创建规则失败: %w", err)
	}
	return nil
}

// Get 按 ID 获取权限规则。
func (s *SQLiteStore) Get(id string) (*StoredRule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	row := s.db.QueryRow(
		`SELECT id, rule_type, pattern, scope, session_id, project_id, created_at, updated_at, source
		FROM permissions WHERE id = ?`, id,
	)

	rule, err := scanRule(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("permission: 规则 %s 不存在", id)
	}
	if err != nil {
		return nil, fmt.Errorf("permission: 获取规则失败: %w", err)
	}
	return rule, nil
}

// Update 更新权限规则。
func (s *SQLiteStore) Update(rule *StoredRule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	rule.UpdatedAt = time.Now().UTC()
	result, err := s.db.Exec(
		`UPDATE permissions SET rule_type=?, pattern=?, scope=?, session_id=?, project_id=?, updated_at=?, source=?
		WHERE id=?`,
		rule.RuleType, rule.Pattern, rule.Scope, rule.SessionID, rule.ProjectID,
		rule.UpdatedAt.Format(time.RFC3339), rule.Source, rule.ID,
	)
	if err != nil {
		return fmt.Errorf("permission: 更新规则失败: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("permission: 规则 %s 不存在", rule.ID)
	}
	return nil
}

// Delete 删除权限规则。
func (s *SQLiteStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec(`DELETE FROM permissions WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("permission: 删除规则失败: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("permission: 规则 %s 不存在", id)
	}
	return nil
}

// List 列出所有权限规则。
func (s *SQLiteStore) List() ([]StoredRule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(
		`SELECT id, rule_type, pattern, scope, session_id, project_id, created_at, updated_at, source
		FROM permissions ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("permission: 列举规则失败: %w", err)
	}
	defer rows.Close()

	var result []StoredRule
	for rows.Next() {
		rule, err := scanRule(rows)
		if err != nil {
			continue
		}
		result = append(result, *rule)
	}
	if result == nil {
		result = []StoredRule{}
	}
	return result, nil
}

// FindByPattern 按模式查找匹配的权限规则。
// 使用 path.Match 匹配工具名称。
func (s *SQLiteStore) FindByPattern(toolName string, args map[string]interface{}) (*StoredRule, error) {
	return s.FindByPatternInContext(toolName, args, ScopeContext{})
}

// FindByPatternInContext finds the highest-specificity rule that matches both
// the tool/arguments and the current session/project context.
func (s *SQLiteStore) FindByPatternInContext(toolName string, args map[string]interface{}, context ScopeContext) (*StoredRule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(
		`SELECT id, rule_type, pattern, scope, session_id, project_id, created_at, updated_at, source
		FROM permissions ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("permission: 查询规则失败: %w", err)
	}
	defer rows.Close()

	context = context.normalized()
	argsStr := formatArgs(args)
	var best *StoredRule
	bestRank := int(^uint(0) >> 1)
	for rows.Next() {
		rule, err := scanRule(rows)
		if err != nil {
			continue
		}
		if !RuleApplies(*rule, context) {
			continue
		}
		// 使用 path.Match 匹配 pattern
		// pattern 格式：tool_pattern 或 tool_pattern:arg_pattern
		parts := strings.SplitN(rule.Pattern, ":", 2)
		toolPattern := parts[0]

		matched, err := path.Match(toolPattern, toolName)
		if err != nil || !matched {
			continue
		}

		if len(parts) > 1 && parts[1] != "" {
			argPattern := parts[1]
			matched, err = path.Match(argPattern, argsStr)
			if err != nil || !matched {
				continue
			}
		}

		rank := ScopeRank(rule.Scope)
		if best == nil || rank < bestRank || (rank == bestRank && rule.CreatedAt.Before(best.CreatedAt)) {
			best = rule
			bestRank = rank
		}
	}
	return best, nil
}

// scanRule 从 Scanner 扫描一行数据到 StoredRule。
func scanRule(row interface {
	Scan(dest ...interface{}) error
}) (*StoredRule, error) {
	var rule StoredRule
	var createdAt, updatedAt string
	err := row.Scan(
		&rule.ID, &rule.RuleType, &rule.Pattern, &rule.Scope,
		&rule.SessionID, &rule.ProjectID, &createdAt, &updatedAt, &rule.Source,
	)
	if err != nil {
		return nil, err
	}
	rule.CreatedAt = parseTime(createdAt)
	rule.UpdatedAt = parseTime(updatedAt)
	rule.Scope = RuleScope(rule)
	return &rule, nil
}

// parseTime 解析 RFC3339 格式的时间字符串。
func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// GetDB 返回底层的数据库连接（用于审计日志等操作）。
func (s *SQLiteStore) GetDB() *sql.DB {
	return s.db
}

// Path 返回数据库文件路径。
func (s *SQLiteStore) Path() string {
	return s.path
}
