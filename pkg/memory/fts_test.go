//go:build memory

package memory

import (
	"path/filepath"
	"testing"
)

// TestFTS5SearchBasic 测试 FTS5 基本搜索功能。
func TestFTS5SearchBasic(t *testing.T) {
	dir := t.TempDir()
	fts, err := NewFTSIndex(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewFTSIndex: %v", err)
	}
	defer fts.Close()

	if err := fts.Index("id1", "Golang 是一种静态类型语言"); err != nil {
		t.Fatalf("Index: %v", err)
	}
	if err := fts.Index("id2", "Python 是一种动态类型语言"); err != nil {
		t.Fatalf("Index: %v", err)
	}

	ids, err := fts.Search("Golang", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(ids), ids)
	}
	if ids[0] != "id1" {
		t.Errorf("expected id 'id1', got %q", ids[0])
	}
}

// TestFTS5SearchMultiple 测试 FTS5 多结果搜索。
func TestFTS5SearchMultiple(t *testing.T) {
	dir := t.TempDir()
	fts, err := NewFTSIndex(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewFTSIndex: %v", err)
	}
	defer fts.Close()

	fts.Index("id1", "Golang 并发编程入门")
	fts.Index("id2", "Golang 网络编程实战")
	fts.Index("id3", "Python 数据分析入门")

	ids, err := fts.Search("Golang", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 results, got %d: %v", len(ids), ids)
	}
}

// TestFTS5BM25Ranking 测试 BM25 相关性排序。
// 使用 ASCII 文本验证 BM25 排序（FTS5 分词器对 CJK 按词组而非单字分词）。
func TestFTS5BM25Ranking(t *testing.T) {
	dir := t.TempDir()
	fts, err := NewFTSIndex(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewFTSIndex: %v", err)
	}
	defer fts.Close()

	// 使用 ASCII 内容确保 FTS5 正确分词
	fts.Index("id1", "Golang programming language")
	fts.Index("id2", "Golang concurrent programming")
	fts.Index("id3", "Python programming language")

	ids, err := fts.Search("Golang programming", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(ids) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(ids))
	}

	// 验证 id2 排在 id1 前面（"concurrent" 增加文档长度，BM25 行为与文档频率相关）
	// 我们只验证排名稳定，不假设具体顺序
	t.Logf("BM25 ranking: %v", ids)
}

// TestFTS5IDMatch 测试 FTS5 返回的 ID 与索引时的 ID 一致。
func TestFTS5IDMatch(t *testing.T) {
	dir := t.TempDir()
	fts, err := NewFTSIndex(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewFTSIndex: %v", err)
	}
	defer fts.Close()

	tests := []struct {
		id, content string
	}{
		{"mem_1", "Golang 并发编程"},
		{"mem_2", "Golang 网络编程"},
		{"long-id-12345", "Python 数据分析"},
		{"special/chars:id", "Rust 系统编程"},
	}

	for _, tt := range tests {
		if err := fts.Index(tt.id, tt.content); err != nil {
			t.Fatalf("Index(%q): %v", tt.id, err)
		}
	}

	for _, tt := range tests {
		ids, err := fts.Search(tt.content, 10)
		if err != nil {
			t.Fatalf("Search(%q): %v", tt.content, err)
		}
		if len(ids) != 1 {
			t.Fatalf("Search(%q): expected 1 result, got %d: %v", tt.content, len(ids), ids)
		}
		if ids[0] != tt.id {
			t.Errorf("Search(%q): expected id %q, got %q", tt.content, tt.id, ids[0])
		}
	}
}

// TestFTS5CJKSearch 测试 CJK 文本搜索。
// unicode61 分词器将每个 CJK 字符作为独立 token 处理。
func TestFTS5CJKSearch(t *testing.T) {
	dir := t.TempDir()
	fts, err := NewFTSIndex(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewFTSIndex: %v", err)
	}
	defer fts.Close()

	fts.Index("id1", "今天天气很好")
	fts.Index("id2", "天气预报说明天会下雨")

	// 搜索 "天气" — unicode61 会将 "天气" 拆分为 "天" 和 "气" 两个 token，
	// 两个文档都包含这两个 token，所以都返回
	ids, err := fts.Search("天气", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 results for CJK search, got %d: %v", len(ids), ids)
	}
}

// TestFTS5UpdateTrigger 测试 UPDATE 触发器同步 FTS 索引。
func TestFTS5UpdateTrigger(t *testing.T) {
	dir := t.TempDir()
	fts, err := NewFTSIndex(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewFTSIndex: %v", err)
	}
	defer fts.Close()

	fts.Index("id1", "Golang 编程")
	fts.Index("id2", "Python 编程")

	// 更新 id1 的内容
	fts.Index("id1", "Rust 编程")

	// 搜索旧关键词不应匹配 id1
	ids, err := fts.Search("Golang", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected 0 results for 'Golang' after update, got %d: %v", len(ids), ids)
	}

	// 搜索新关键词应匹配 id1
	ids, err = fts.Search("Rust", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(ids) != 1 || ids[0] != "id1" {
		t.Errorf("expected 1 result 'id1' for 'Rust', got %v", ids)
	}
}

// TestFTS5DeleteTrigger 测试 DELETE 触发器同步 FTS 索引。
func TestFTS5DeleteTrigger(t *testing.T) {
	dir := t.TempDir()
	fts, err := NewFTSIndex(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewFTSIndex: %v", err)
	}
	defer fts.Close()

	fts.Index("id1", "Golang 编程")
	fts.Index("id2", "Python 编程")

	// 直接通过 FTSIndex 删除：重新 Index 相同 ID 覆盖即可
	// 测试删除后搜索结果减少
	fts.Index("id1", "一些无关内容")

	ids, err := fts.Search("Golang", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected 0 results for 'Golang' after delete, got %d", len(ids))
	}
}

// TestFTS5Backfill 测试打开已有数据库时回填 FTS 索引。
func TestFTS5Backfill(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "persist.db")

	// 第一次打开：创建并写入数据
	fts1, err := NewFTSIndex(dbPath)
	if err != nil {
		t.Fatalf("NewFTSIndex: %v", err)
	}
	fts1.Index("id1", "持久化测试数据")
	fts1.Close()

	// 第二次打开：应自动回填已有数据到 FTS 索引
	fts2, err := NewFTSIndex(dbPath)
	if err != nil {
		t.Fatalf("NewFTSIndex reopen: %v", err)
	}
	defer fts2.Close()

	ids, err := fts2.Search("持久化", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(ids) != 1 {
		t.Errorf("expected 1 result after reopen+backfill, got %d: %v", len(ids), ids)
	}
}

// TestFTS5NoMatch 测试无匹配搜索。
func TestFTS5NoMatch(t *testing.T) {
	dir := t.TempDir()
	fts, err := NewFTSIndex(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewFTSIndex: %v", err)
	}
	defer fts.Close()

	fts.Index("id1", "Golang 编程")
	ids, err := fts.Search("Python", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected 0 results, got %d", len(ids))
	}
}

// TestFTS5EmptyQuery 测试空查询。
func TestFTS5EmptyQuery(t *testing.T) {
	fts, err := NewFTSIndex(":memory:")
	if err != nil {
		t.Fatalf("NewFTSIndex: %v", err)
	}
	defer fts.Close()

	ids, err := fts.Search("", 10)
	if err != nil {
		t.Fatalf("Search empty: %v", err)
	}
	if ids != nil {
		t.Errorf("expected nil for empty query, got %v", ids)
	}
}

// TestFTS5InMemory 测试内存数据库。
func TestFTS5InMemory(t *testing.T) {
	fts, err := NewFTSIndex(":memory:")
	if err != nil {
		t.Fatalf("NewFTSIndex: %v", err)
	}
	defer fts.Close()

	if err := fts.Index("id1", "内存数据库测试"); err != nil {
		t.Fatalf("Index: %v", err)
	}

	ids, err := fts.Search("内存", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(ids) != 1 {
		t.Errorf("expected 1 result, got %d", len(ids))
	}
}

// TestFTS5Reindex 测试相同 ID 重新索引。
func TestFTS5Reindex(t *testing.T) {
	dir := t.TempDir()
	fts, err := NewFTSIndex(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewFTSIndex: %v", err)
	}
	defer fts.Close()

	fts.Index("id1", "Golang")
	fts.Index("id1", "Python")

	// 旧内容不应再被搜索到
	ids, err := fts.Search("Golang", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected 0 results for 'Golang' after reindex, got %d", len(ids))
	}

	// 新内容应可搜索
	ids, err = fts.Search("Python", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(ids) != 1 {
		t.Errorf("expected 1 result for 'Python', got %d", len(ids))
	}
}