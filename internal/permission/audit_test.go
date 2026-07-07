//go:build sqlite

package permission

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newTestAuditLogger 创建一个测试用的审计日志记录器。
func newTestAuditLogger(t *testing.T) (*AuditLogger, *SQLiteStore) {
	t.Helper()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test_audit.db")

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("创建 SQLiteStore 失败: %v", err)
	}

	logger := NewAuditLoggerWithDB(store.GetDB())

	t.Cleanup(func() {
		logger.Close()
		store.Close()
		os.Remove(dbPath)
	})

	return logger, store
}

func TestAuditLogger_Record(t *testing.T) {
	logger, _ := newTestAuditLogger(t)

	record := AuditRecord{
		SessionID: "sess-1",
		ToolName:  "read_file",
		RuleID:    "rule-1",
		Decision:  "allowed",
		Context:   `{"path":"test.txt"}`,
		Timestamp: time.Now().UTC(),
	}

	logger.Record(record)
	logger.Flush()

	// 查询验证
	results, err := logger.Query("sess-1", "", time.Time{}, time.Time{}, 10)
	if err != nil {
		t.Fatalf("Query 失败: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("期望 1 条记录，得到 %d 条", len(results))
	}

	if results[0].ToolName != "read_file" {
		t.Errorf("ToolName 期望 %q, 得到 %q", "read_file", results[0].ToolName)
	}
	if results[0].Decision != "allowed" {
		t.Errorf("Decision 期望 %q, 得到 %q", "allowed", results[0].Decision)
	}
	if results[0].SessionID != "sess-1" {
		t.Errorf("SessionID 期望 %q, 得到 %q", "sess-1", results[0].SessionID)
	}
}

func TestAuditLogger_Query_按工具名过滤(t *testing.T) {
	logger, _ := newTestAuditLogger(t)

	records := []AuditRecord{
		{SessionID: "sess-1", ToolName: "read_file", Decision: "allowed"},
		{SessionID: "sess-1", ToolName: "write_file", Decision: "denied"},
		{SessionID: "sess-2", ToolName: "read_file", Decision: "allowed"},
	}
	for _, r := range records {
		logger.Record(r)
	}
	logger.Flush()

	// 按工具名过滤
	results, err := logger.Query("", "read_file", time.Time{}, time.Time{}, 10)
	if err != nil {
		t.Fatalf("Query 失败: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("期望 2 条 read_file 记录，得到 %d 条", len(results))
	}

	// 按 session 过滤
	results, err = logger.Query("sess-2", "", time.Time{}, time.Time{}, 10)
	if err != nil {
		t.Fatalf("Query 失败: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("期望 1 条 sess-2 记录，得到 %d 条", len(results))
	}
}

func TestAuditLogger_Query_按时间范围过滤(t *testing.T) {
	logger, _ := newTestAuditLogger(t)

	// 创建三段时间不同的记录
	now := time.Now().UTC()
	records := []AuditRecord{
		{SessionID: "sess-1", ToolName: "tool_1", Decision: "allowed", Timestamp: now.Add(-2 * time.Hour)},
		{SessionID: "sess-1", ToolName: "tool_2", Decision: "denied", Timestamp: now.Add(-1 * time.Hour)},
		{SessionID: "sess-1", ToolName: "tool_3", Decision: "allowed", Timestamp: now},
	}
	for _, r := range records {
		logger.Record(r)
	}
	logger.Flush()

	// 查询最近 90 分钟内的记录
	start := now.Add(-90 * time.Minute)
	end := now.Add(1 * time.Minute)
	results, err := logger.Query("", "", start, end, 10)
	if err != nil {
		t.Fatalf("Query 失败: %v", err)
	}

	// 应该返回 tool_2 和 tool_3
	if len(results) != 2 {
		t.Fatalf("期望 2 条时间范围内记录，得到 %d 条", len(results))
	}
}

func TestAuditLogger_Cleanup(t *testing.T) {
	logger, _ := newTestAuditLogger(t)

	now := time.Now().UTC()

	// 创建旧记录（30 天前）和新记录
	oldRecord := AuditRecord{
		SessionID: "sess-old",
		ToolName:  "old_tool",
		Decision:  "allowed",
		Timestamp: now.Add(-30 * 24 * time.Hour),
	}
	newRecord := AuditRecord{
		SessionID: "sess-new",
		ToolName:  "new_tool",
		Decision:  "allowed",
		Timestamp: now,
	}

	logger.Record(oldRecord)
	logger.Record(newRecord)
	logger.Flush()

	// 清理 7 天前的记录
	deleted, err := logger.Cleanup(7)
	if err != nil {
		t.Fatalf("Cleanup 失败: %v", err)
	}

	if deleted != 1 {
		t.Errorf("期望删除 1 条记录，实际删除 %d 条", deleted)
	}

	// 验证旧记录已被清理
	results, err := logger.Query("", "", time.Time{}, time.Time{}, 10)
	if err != nil {
		t.Fatalf("Query 失败: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("期望 1 条记录，得到 %d 条", len(results))
	}
	if results[0].SessionID != "sess-new" {
		t.Errorf("剩余记录 SessionID 期望 %q, 得到 %q", "sess-new", results[0].SessionID)
	}
}

func TestAuditLogger_批量写入(t *testing.T) {
	logger, _ := newTestAuditLogger(t)

	// 批量写入超过 100 条记录，触发自动批量写入
	count := 120
	for i := 0; i < count; i++ {
		logger.Record(AuditRecord{
			SessionID: "sess-batch",
			ToolName:  "batch_tool",
			Decision:  "allowed",
		})
	}
	logger.Flush()

	results, err := logger.Query("sess-batch", "", time.Time{}, time.Time{}, 200)
	if err != nil {
		t.Fatalf("Query 失败: %v", err)
	}

	if len(results) != count {
		t.Errorf("期望 %d 条记录，得到 %d 条", count, len(results))
	}
}

func TestAuditLogger_Query_limit(t *testing.T) {
	logger, _ := newTestAuditLogger(t)

	for i := 0; i < 10; i++ {
		logger.Record(AuditRecord{
			ToolName: "tool", Decision: "allowed",
		})
	}
	logger.Flush()

	results, err := logger.Query("", "", time.Time{}, time.Time{}, 3)
	if err != nil {
		t.Fatalf("Query 失败: %v", err)
	}

	if len(results) > 3 {
		t.Errorf("Limit 3 应返回 <=3 条，实际 %d 条", len(results))
	}
}

func TestAuditLogger_Query_空结果(t *testing.T) {
	logger, _ := newTestAuditLogger(t)

	results, err := logger.Query("non-existent", "", time.Time{}, time.Time{}, 10)
	if err != nil {
		t.Fatalf("Query 失败: %v", err)
	}

	if results == nil {
		t.Error("空结果应返回空切片而非 nil")
	}
	if len(results) != 0 {
		t.Errorf("期望 0 条，得到 %d 条", len(results))
	}
}
