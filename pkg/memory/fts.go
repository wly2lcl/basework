//go:build memory

package memory

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// FTSIndex 提供基于 SQLite FTS5 的全文搜索索引。
// 使用 BM25 排序算法，支持 CJK 文本（通过 LIKE 回退）。
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

	// 创建主表（存储原始数据，用于 JOIN 筛选和 LIKE 回退）
	createSQL := `CREATE TABLE IF NOT EXISTS memories (
		id TEXT PRIMARY KEY,
		content TEXT NOT NULL,
		session_id TEXT NOT NULL DEFAULT ''
	)`
	if _, err := db.Exec(createSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("create table: %w", err)
	}

	// 创建 FTS5 虚拟表
	// 注意：modernc.org/sqlite 的 FTS5 实现不支持 delete/delete-by-rowid 命令，
	// 因此使用 JOIN 方式过滤已被覆盖的旧条目。
	ftsSQL := `CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(
		id, content, session_id,
		tokenize='unicode61'
	)`
	if _, err := db.Exec(ftsSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("create fts5 table: %w", err)
	}

	// 回填已有数据到 FTS 索引（兼容旧数据库升级）
	_, err = db.Exec(`INSERT OR IGNORE INTO memories_fts(id, content, session_id) SELECT id, content, session_id FROM memories`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("backfill fts: %w", err)
	}

	return &FTSIndex{db: db}, nil
}

// Index 将一段内容按 ID 加入索引。
// 如果 ID 已存在，则覆盖更新。
// 使用先删除主表旧行、再插入新行的方式，确保 FTS 索引通过 JOIN 过滤旧数据。
func (f *FTSIndex) Index(id, content string) error {
	// 删除主表旧行（FTS5 旧条目成为孤儿，被 JOIN 自动过滤）
	if _, err := f.db.Exec("DELETE FROM memories WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete old entry: %w", err)
	}

	// 插入主表新行
	if _, err := f.db.Exec(
		`INSERT INTO memories (id, content, session_id) VALUES (?, ?, '')`,
		id, content,
	); err != nil {
		return fmt.Errorf("insert entry: %w", err)
	}

	// 插入 FTS5 索引
	if _, err := f.db.Exec(
		`INSERT INTO memories_fts(id, content, session_id) VALUES (?, ?, '')`,
		id, content,
	); err != nil {
		return fmt.Errorf("index fts: %w", err)
	}

	return nil
}

// Search 在索引中搜索匹配的内容。
// 优先使用 FTS5 MATCH + BM25 排序（适用于 ASCII 等可分词文本），
// 回退到 LIKE 子串匹配（适用于 CJK 等 FTS5 无法分词的文本）。
// 返回匹配条目的 ID 列表，按相关性排序。
func (f *FTSIndex) Search(query string, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 10
	}

	if query == "" {
		return nil, nil
	}

	// 尝试 FTS5 MATCH 搜索（BM25 排序）
	// JOIN 通过 id 列自动过滤已被覆盖的旧条目
	rows, err := f.db.Query(
		`SELECT m.id FROM memories m
		JOIN memories_fts fts ON m.id = fts.id AND m.content = fts.content
		WHERE memories_fts MATCH ?
		ORDER BY bm25(memories_fts)
		LIMIT ?`,
		query, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("fts5 search: %w", err)
	}

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan result: %w", err)
		}
		ids = append(ids, id)
	}
	rows.Close()

	if len(ids) > 0 {
		return ids, nil
	}

	// 回退到 LIKE 子串匹配（支持 CJK 等 FTS5 无法分词的文本）
	likeQuery := "%" + query + "%"
	rows2, err := f.db.Query(
		"SELECT id FROM memories WHERE content LIKE ? LIMIT ?",
		likeQuery, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("like search: %w", err)
	}
	defer rows2.Close()

	for rows2.Next() {
		var id string
		if err := rows2.Scan(&id); err != nil {
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