package permission

import (
	"encoding/json"
	"time"
)

// AuditRecord 审计记录。
type AuditRecord struct {
	ID        int64     `json:"id,omitempty"`
	SessionID string    `json:"session_id"`
	ToolName  string    `json:"tool_name"`
	RuleID    string    `json:"rule_id"`
	Decision  string    `json:"decision"` // "allowed" | "denied" | "asked"
	Context   string    `json:"context"`  // JSON
	Timestamp time.Time `json:"timestamp"`
}

// AuditLogger 是审计日志记录器。
// 无 build tag sqlite 时提供空操作实现;
// 有 build tag sqlite 时使用 NewAuditLoggerWithDB 创建 SQLite 后端实现。
type AuditLogger struct {
	recorder func(AuditRecord)
	querier  func(sessionID, toolName string, start, end time.Time, limit int) ([]AuditRecord, error)
	cleaner  func(retentionDays int) (int64, error)
	flusher  func()
	closer   func() error
}

// NewAuditLogger 创建一个空操作的审计日志记录器。
func NewAuditLogger() *AuditLogger {
	return &AuditLogger{
		recorder: func(AuditRecord) {},
		querier:  func(string, string, time.Time, time.Time, int) ([]AuditRecord, error) { return nil, nil },
		cleaner:  func(int) (int64, error) { return 0, nil },
		flusher:  func() {},
		closer:   func() error { return nil },
	}
}

// Record 记录审计事件。
func (a *AuditLogger) Record(record AuditRecord) {
	if a.recorder != nil {
		a.recorder(record)
	}
}

// Query 查询审计日志。
func (a *AuditLogger) Query(sessionID string, toolName string, start, end time.Time, limit int) ([]AuditRecord, error) {
	if a.querier != nil {
		return a.querier(sessionID, toolName, start, end, limit)
	}
	return nil, nil
}

// Cleanup 清理过期记录。
func (a *AuditLogger) Cleanup(retentionDays int) (int64, error) {
	if a.cleaner != nil {
		return a.cleaner(retentionDays)
	}
	return 0, nil
}

// Flush 强制刷新缓冲中的审计记录到数据库。
func (a *AuditLogger) Flush() {
	if a.flusher != nil {
		a.flusher()
	}
}

// Close 关闭审计日志记录器。
func (a *AuditLogger) Close() error {
	if a.closer != nil {
		return a.closer()
	}
	return nil
}

// FormatAuditContext 格式化审计上下文为 JSON 字符串。
func FormatAuditContext(args map[string]interface{}) string {
	if args == nil {
		return "{}"
	}
	data, err := json.Marshal(args)
	if err != nil {
		return "{}"
	}
	return string(data)
}