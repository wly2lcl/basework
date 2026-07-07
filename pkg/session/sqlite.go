//go:build sqlite

package session

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/wly2lcl/basework/pkg/llm"
	_ "modernc.org/sqlite"
)

// SQLiteStore 是基于 SQLite 的会话存储实现。
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
		return nil, fmt.Errorf("session: 创建目录 %s 失败: %w", dir, err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("session: 打开 SQLite 数据库失败: %w", err)
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
		return nil, fmt.Errorf("session: 初始化表结构失败: %w", err)
	}

	return s, nil
}

// initSchema 创建数据库表结构。
func (s *SQLiteStore) initSchema() error {
	// 单独设置 PRAGMA（多语句 Exec 中 PRAGMA 可能不生效）
	if _, err := s.db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return fmt.Errorf("session: 设置 WAL 模式失败: %w", err)
	}
	if _, err := s.db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		return fmt.Errorf("session: 启用外键约束失败: %w", err)
	}

	schema := `
	CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL DEFAULT '',
		model TEXT NOT NULL DEFAULT '',
		provider TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		tracked_files TEXT NOT NULL DEFAULT '[]'
	);

	CREATE TABLE IF NOT EXISTS events (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		type TEXT NOT NULL,
		data TEXT NOT NULL DEFAULT '',
		seq INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL,
		FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS messages (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		role TEXT NOT NULL,
		content TEXT NOT NULL DEFAULT '',
		timestamp TEXT NOT NULL,
		FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS tool_calls (
		id TEXT PRIMARY KEY,
		message_id TEXT NOT NULL,
		tool_name TEXT NOT NULL,
		arguments TEXT NOT NULL DEFAULT '{}',
		FOREIGN KEY (message_id) REFERENCES messages(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS tool_results (
		id TEXT PRIMARY KEY,
		tool_call_id TEXT NOT NULL,
		content TEXT NOT NULL DEFAULT '',
		is_error INTEGER NOT NULL DEFAULT 0,
		FOREIGN KEY (tool_call_id) REFERENCES tool_calls(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_events_session_id ON events(session_id);
	CREATE INDEX IF NOT EXISTS idx_events_session_seq ON events(session_id, seq);
	CREATE INDEX IF NOT EXISTS idx_messages_session_id ON messages(session_id);
	CREATE INDEX IF NOT EXISTS idx_tool_calls_message_id ON tool_calls(message_id);
	CREATE INDEX IF NOT EXISTS idx_tool_results_tool_call_id ON tool_results(tool_call_id);
	`
	_, err := s.db.Exec(schema)
	return err
}

// Close 关闭数据库连接。
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// Create 创建一个新的会话。
func (s *SQLiteStore) Create(opts CreateOpts) (*Info, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := newID()
	now := nowUTC()
	info := &Info{
		ID:        id,
		Title:     opts.Title,
		CreatedAt: now,
		UpdatedAt: now,
	}

	_, err := s.db.Exec(
		`INSERT INTO sessions (id, title, model, provider, created_at, updated_at, tracked_files) VALUES (?, ?, '', '', ?, ?, '[]')`,
		id, opts.Title, now.Format(time.RFC3339), now.Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("session: 创建会话失败: %w", err)
	}

	return info, nil
}

// Get 根据 ID 获取会话信息。
func (s *SQLiteStore) Get(id string) (*Info, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	row := s.db.QueryRow(
		`SELECT id, title, model, provider, created_at, updated_at FROM sessions WHERE id = ?`,
		id,
	)

	var info Info
	var createdAt, updatedAt string
	err := row.Scan(&info.ID, &info.Title, &info.Model, &info.Provider, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session: 会话 %s 不存在", id)
	}
	if err != nil {
		return nil, fmt.Errorf("session: 获取会话失败: %w", err)
	}

	info.CreatedAt = parseTime(createdAt)
	info.UpdatedAt = parseTime(updatedAt)

	// 统计消息数
	var msgCount int
	err = s.db.QueryRow(`SELECT COUNT(*) FROM messages WHERE session_id = ?`, id).Scan(&msgCount)
	if err == nil {
		info.MessageCount = msgCount
	}

	return &info, nil
}

// List 列举会话，支持过滤和分页。
func (s *SQLiteStore) List(filter ListFilter) ([]*Info, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `SELECT s.id, s.title, s.model, s.provider, s.created_at, s.updated_at,
		COALESCE((SELECT COUNT(*) FROM messages WHERE session_id = s.id), 0) as msg_count
		FROM sessions s`
	var args []interface{}

	if !filter.AfterTime.IsZero() {
		query += ` WHERE s.created_at >= ?`
		args = append(args, filter.AfterTime.Format(time.RFC3339))
	}

	query += ` ORDER BY s.created_at DESC`

	if filter.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, filter.Limit)
	}
	if filter.Offset > 0 {
		query += ` OFFSET ?`
		args = append(args, filter.Offset)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("session: 列举会话失败: %w", err)
	}
	defer rows.Close()

	var result []*Info
	for rows.Next() {
		var info Info
		var createdAt, updatedAt string
		err := rows.Scan(&info.ID, &info.Title, &info.Model, &info.Provider, &createdAt, &updatedAt, &info.MessageCount)
		if err != nil {
			continue
		}
		info.CreatedAt = parseTime(createdAt)
		info.UpdatedAt = parseTime(updatedAt)
		result = append(result, &info)
	}

	return result, nil
}

// Delete 删除一个会话及其关联的数据。
func (s *SQLiteStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 先检查会话是否存在
	var exists bool
	err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM sessions WHERE id = ?)`, id).Scan(&exists)
	if err != nil {
		return fmt.Errorf("session: 检查会话失败: %w", err)
	}
	if !exists {
		return fmt.Errorf("session: 会话 %s 不存在", id)
	}

	// 使用事务确保原子性
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("session: 开始事务失败: %w", err)
	}
	defer tx.Rollback() // 如果 Commit 没调用，自动 Rollback

	// 手动级联删除
	_, err = tx.Exec(`DELETE FROM tool_results WHERE tool_call_id IN (SELECT id FROM tool_calls WHERE message_id IN (SELECT id FROM messages WHERE session_id = ?))`, id)
	if err != nil {
		return fmt.Errorf("session: 删除 tool_results 失败: %w", err)
	}
	_, err = tx.Exec(`DELETE FROM tool_calls WHERE message_id IN (SELECT id FROM messages WHERE session_id = ?)`, id)
	if err != nil {
		return fmt.Errorf("session: 删除 tool_calls 失败: %w", err)
	}
	_, err = tx.Exec(`DELETE FROM messages WHERE session_id = ?`, id)
	if err != nil {
		return fmt.Errorf("session: 删除 messages 失败: %w", err)
	}
	_, err = tx.Exec(`DELETE FROM events WHERE session_id = ?`, id)
	if err != nil {
		return fmt.Errorf("session: 删除 events 失败: %w", err)
	}
	_, err = tx.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("session: 删除会话失败: %w", err)
	}

	return tx.Commit()
}

// AppendEvent 向会话追加一个事件。
// 存储原始事件到 events 表，同时平铺到关系表以便 SQL 查询。
func (s *SQLiteStore) AppendEvent(event Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 验证会话存在
	var exists bool
	err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM sessions WHERE id = ?)`, event.SessionID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("session: 检查会话失败: %w", err)
	}
	if !exists {
		return fmt.Errorf("session: 会话 %s 不存在", event.SessionID)
	}

	if event.ID == "" {
		event.ID = newID()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = nowUTC()
	}

	// 获取当前最大 seq
	var maxSeq int64
	s.db.QueryRow(`SELECT COALESCE(MAX(seq), 0) FROM events WHERE session_id = ?`, event.SessionID).Scan(&maxSeq)
	event.Seq = maxSeq + 1

	// 存储原始事件到 events 表
	eventData, _ := json.Marshal(event.Data)
	_, err = s.db.Exec(
		`INSERT INTO events (id, session_id, type, data, seq, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		event.ID, event.SessionID, string(event.Type), string(eventData), event.Seq, event.CreatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("session: 存储事件失败: %w", err)
	}

	// 更新会话的更新时间
	_, err = s.db.Exec(`UPDATE sessions SET updated_at = ? WHERE id = ?`,
		nowUTC().Format(time.RFC3339), event.SessionID)
	return err
}

// Events 根据过滤条件查询事件列表。
func (s *SQLiteStore) Events(filter EventFilter) ([]Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 验证会话存在
	var exists bool
	err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM sessions WHERE id = ?)`, filter.SessionID).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("session: 检查会话失败: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("session: 会话 %s 不存在", filter.SessionID)
	}

	query := `SELECT id, session_id, type, data, seq, created_at FROM events WHERE session_id = ?`
	var args []interface{}
	args = append(args, filter.SessionID)

	if filter.AfterSeq > 0 {
		query += ` AND seq > ?`
		args = append(args, filter.AfterSeq)
	}

	query += ` ORDER BY seq ASC`

	if filter.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, filter.Limit)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("session: 查询事件失败: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		var typeStr, dataStr, createdAt string
		err := rows.Scan(&e.ID, &e.SessionID, &typeStr, &dataStr, &e.Seq, &createdAt)
		if err != nil {
			continue
		}
		e.Type = EventType(typeStr)
		e.Data = json.RawMessage(dataStr)
		e.CreatedAt = parseTime(createdAt)
		events = append(events, e)
	}

	// 如果有 Type 过滤，在内存中过滤
	if len(filter.Types) > 0 {
		var filtered []Event
		for _, e := range events {
			for _, t := range filter.Types {
				if e.Type == t {
					filtered = append(filtered, e)
					break
				}
			}
		}
		events = filtered
	}

	return events, nil
}

// Messages 返回 LLM 消息列表（需要外部提供 SessionID）。
func (s *SQLiteStore) Messages() ([]llm.ChatMessage, error) {
	return nil, fmt.Errorf("session: SQLiteStore.Messages() 不支持无 SessionID 调用，请使用 MessagesForSession()")
}

// MessagesForSession 根据 SessionID 从 events 表投影出 LLM 消息列表。
func (s *SQLiteStore) MessagesForSession(sessionID string) ([]llm.ChatMessage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 从 events 表读取所有事件
	rows, err := s.db.Query(
		`SELECT id, session_id, type, data, seq, created_at FROM events WHERE session_id = ? ORDER BY seq ASC`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("session: 读取事件失败: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		var typeStr, dataStr, createdAt string
		if err := rows.Scan(&e.ID, &e.SessionID, &typeStr, &dataStr, &e.Seq, &createdAt); err != nil {
			continue
		}
		e.Type = EventType(typeStr)
		e.Data = json.RawMessage(dataStr)
		e.CreatedAt = parseTime(createdAt)
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("session: 遍历事件失败: %w", err)
	}

	return ProjectMessages(events), nil
}

// parseTime 解析 RFC3339 格式的时间字符串。
func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// GetDB 返回底层的数据库连接（用于迁移等操作）。
func (s *SQLiteStore) GetDB() *sql.DB {
	return s.db
}

// Path 返回数据库文件路径。
func (s *SQLiteStore) Path() string {
	return s.path
}

// sortByCreatedAtDesc 按 CreatedAt 降序排列。
func sortByCreatedAtDesc(items []*Info) {
	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
}
