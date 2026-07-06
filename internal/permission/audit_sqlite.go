//go:build sqlite

package permission

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// sqliteAuditLogger 是 SQLite 后端的审计日志记录器实现。
type sqliteAuditLogger struct {
	db     *sql.DB
	batch  []AuditRecord
	mu     sync.Mutex
	ticker *time.Ticker
	done   chan struct{}
	once   sync.Once
}

// NewAuditLoggerWithDB 创建一个 SQLite 后端的审计日志记录器。
// 使用异步批量写入，每 1 秒或积累 100 条时写入。
func NewAuditLoggerWithDB(db *sql.DB) *AuditLogger {
	impl := &sqliteAuditLogger{
		db:     db,
		batch:  make([]AuditRecord, 0, 100),
		ticker: time.NewTicker(1 * time.Second),
		done:   make(chan struct{}),
	}
	go impl.flushLoop()

	return &AuditLogger{
		recorder: impl.Record,
		querier:  impl.Query,
		cleaner:  impl.Cleanup,
		flusher:  impl.flush,
		closer:   impl.Close,
	}
}

// Record 记录审计事件（异步，先缓冲后批量写入）。
func (a *sqliteAuditLogger) Record(record AuditRecord) {
	if record.Timestamp.IsZero() {
		record.Timestamp = time.Now().UTC()
	}

	a.mu.Lock()
	a.batch = append(a.batch, record)
	shouldFlush := len(a.batch) >= 100
	a.mu.Unlock()

	if shouldFlush {
		a.flush()
	}
}

// flushLoop 定时刷新缓冲中的审计记录。
func (a *sqliteAuditLogger) flushLoop() {
	for {
		select {
		case <-a.ticker.C:
			a.flush()
		case <-a.done:
			return
		}
	}
}

// flush 将缓冲中的审计记录批量写入数据库。
func (a *sqliteAuditLogger) flush() {
	a.mu.Lock()
	if len(a.batch) == 0 {
		a.mu.Unlock()
		return
	}
	batch := a.batch
	a.batch = make([]AuditRecord, 0, 100)
	a.mu.Unlock()

	if err := a.batchInsert(batch); err != nil {
		// 写入失败时重新放回缓冲
		a.mu.Lock()
		a.batch = append(batch, a.batch...)
		a.mu.Unlock()
	}
}

// batchInsert 批量插入审计记录。
func (a *sqliteAuditLogger) batchInsert(records []AuditRecord) error {
	tx, err := a.db.Begin()
	if err != nil {
		return fmt.Errorf("audit: 开始事务失败: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(
		`INSERT INTO permission_audit (session_id, tool_name, rule_id, decision, context, timestamp)
		VALUES (?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return fmt.Errorf("audit: 准备语句失败: %w", err)
	}
	defer stmt.Close()

	for _, r := range records {
		_, err := stmt.Exec(r.SessionID, r.ToolName, r.RuleID, r.Decision, r.Context, r.Timestamp.Format(time.RFC3339))
		if err != nil {
			return fmt.Errorf("audit: 插入记录失败: %w", err)
		}
	}

	return tx.Commit()
}

// Query 查询审计日志。
func (a *sqliteAuditLogger) Query(sessionID string, toolName string, start, end time.Time, limit int) ([]AuditRecord, error) {
	query := `SELECT id, session_id, tool_name, rule_id, decision, context, timestamp FROM permission_audit WHERE 1=1`
	var args []interface{}

	if sessionID != "" {
		query += ` AND session_id = ?`
		args = append(args, sessionID)
	}
	if toolName != "" {
		query += ` AND tool_name = ?`
		args = append(args, toolName)
	}
	if !start.IsZero() {
		query += ` AND timestamp >= ?`
		args = append(args, start.Format(time.RFC3339))
	}
	if !end.IsZero() {
		query += ` AND timestamp <= ?`
		args = append(args, end.Format(time.RFC3339))
	}

	query += ` ORDER BY timestamp DESC`

	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}

	rows, err := a.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("audit: 查询审计日志失败: %w", err)
	}
	defer rows.Close()

	var result []AuditRecord
	for rows.Next() {
		var r AuditRecord
		var ts string
		err := rows.Scan(&r.ID, &r.SessionID, &r.ToolName, &r.RuleID, &r.Decision, &r.Context, &ts)
		if err != nil {
			continue
		}
		r.Timestamp = parseAuditTime(ts)
		result = append(result, r)
	}
	if result == nil {
		result = []AuditRecord{}
	}
	return result, nil
}

// Cleanup 清理过期记录。
func (a *sqliteAuditLogger) Cleanup(retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		return 0, fmt.Errorf("audit: retentionDays 必须大于 0")
	}

	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)
	result, err := a.db.Exec(`DELETE FROM permission_audit WHERE timestamp < ?`, cutoff.Format(time.RFC3339))
	if err != nil {
		return 0, fmt.Errorf("audit: 清理审计日志失败: %w", err)
	}
	return result.RowsAffected()
}

// Close 关闭审计日志记录器。
func (a *sqliteAuditLogger) Close() error {
	a.once.Do(func() {
		a.ticker.Stop()
		close(a.done)
	})
	// 关闭前刷新缓冲
	a.flush()
	return nil
}

// parseAuditTime 解析审计日志表中的时间字符串。
func parseAuditTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		// 尝试 SQLite 默认格式
		t, err = time.Parse("2006-01-02 15:04:05", s)
		if err != nil {
			return time.Time{}
		}
	}
	return t
}