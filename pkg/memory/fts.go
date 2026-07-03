//go:build memory

package memory

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// FTSIndex 提供基于 SQLite 的全文搜索索引。
// 使用 LIKE 子串匹配，支持所有语言（包括 CJK）。
type FTSIndex struct {
	db *sql.DB
}

// NewFTSIndex 创建或打开一个搜索索引数据库。
// dbPath 是 SQLite 数据库文件路径，":memory:" 表示纯内存数据库。
func NewFTSIndex(dbPath string) (*FTSIndex, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open fts db: %w", err)
	}

	// 启用 WAL 模式以提升并发性能
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	// 创建索引表
	createSQL := `CREATE TABLE IF NOT EXISTS memories (
		id TEXT PRIMARY KEY,
		content TEXT NOT NULL
	)`
	if _, err := db.Exec(createSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("create table: %w", err)
	}

	return &FTSIndex{db: db}, nil
}

// Index 将一段内容按 ID 加入索引。
// 如果 ID 已存在，则覆盖更新。
func (f *FTSIndex) Index(id, content string) error {
	_, err := f.db.Exec(
		`INSERT OR REPLACE INTO memories (id, content) VALUES (?, ?)`,
		id, content,
	)
	if err != nil {
		return fmt.Errorf("index entry: %w", err)
	}
	return nil
}

// Search 在索引中搜索匹配的内容。
// 使用 LIKE 子串匹配，支持所有语言。
// 返回匹配条目的 ID 列表，按内容长度排序。
func (f *FTSIndex) Search(query string, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 10
	}

	if query == "" {
		return nil, nil
	}

	likeQuery := "%" + query + "%"
	rows, err := f.db.Query(
		`SELECT id FROM memories WHERE content LIKE ? ORDER BY length(content) LIMIT ?`,
		likeQuery, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan result: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// Close 关闭索引数据库连接。
func (f *FTSIndex) Close() error {
	return f.db.Close()
}