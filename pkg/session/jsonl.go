package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/wly2lcl/basework/pkg/llm"
)

// safeIDPattern 限制 session ID 只允许 [a-zA-Z0-9_-]。
var safeIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// JSONLStore 是基于 JSONL 文件的会话存储实现。
// 每个会话对应一个 {base_dir}/{session_id}.jsonl 文件。
type JSONLStore struct {
	mu      sync.RWMutex // 保护文件操作
	cacheMu sync.RWMutex // 保护 cache 映射
	baseDir string
	cache   map[string]*sessionData // 内存缓存，加速读取
}

// NewJSONLStore 创建一个新的 JSONLStore。
// baseDir 是存储 JSONL 文件的目录，不存在时自动创建。
func NewJSONLStore(baseDir string) (*JSONLStore, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("session: 创建目录 %s 失败: %w", baseDir, err)
	}
	return &JSONLStore{
		baseDir: baseDir,
		cache:   make(map[string]*sessionData),
	}, nil
}

// safePath 校验 session ID 并返回安全的文件路径。
func (s *JSONLStore) safePath(sessionID string) (string, error) {
	if !safeIDPattern.MatchString(sessionID) {
		return "", fmt.Errorf("session: 非法的会话 ID: %q", sessionID)
	}
	return filepath.Join(s.baseDir, sessionID+".jsonl"), nil
}

// AppendEvent 向会话追加一个事件（原子写入）。
func (s *JSONLStore) AppendEvent(event Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path, err := s.safePath(event.SessionID)
	if err != nil {
		return err
	}

	sd, err := s.loadSession(event.SessionID)
	if err != nil {
		return err
	}

	sd.seq++
	event.Seq = sd.seq
	if event.ID == "" {
		event.ID = newID()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = nowUTC()
	}
	sd.events = append(sd.events, event)
	sd.info.UpdatedAt = nowUTC()

	// 更新缓存
	s.cacheMu.Lock()
	s.cache[event.SessionID] = sd
	s.cacheMu.Unlock()

	// 原子写入：写全部事件到 tmp 文件，再 rename
	tmpPath := path + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("session: 创建临时文件失败: %w", err)
	}
	enc := json.NewEncoder(f)
	for _, e := range sd.events {
		if err := enc.Encode(e); err != nil {
			f.Close()
			os.Remove(tmpPath)
			return fmt.Errorf("session: 写入事件失败: %w", err)
		}
	}
	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("session: 关闭临时文件失败: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("session: 重命名文件失败: %w", err)
	}

	return nil
}

// Events 根据过滤条件返回事件列表。
// 优先从缓存读取，缓存未命中时从文件加载。
func (s *JSONLStore) Events(filter EventFilter) ([]Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sd, err := s.loadSession(filter.SessionID)
	if err != nil {
		return nil, err
	}

	var result []Event
	for _, e := range sd.events {
		if filter.AfterSeq > 0 && e.Seq <= filter.AfterSeq {
			continue
		}
		if len(filter.Types) > 0 {
			match := false
			for _, t := range filter.Types {
				if e.Type == t {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		result = append(result, e)
	}

	if filter.Limit > 0 && len(result) > filter.Limit {
		result = result[:filter.Limit]
	}
	return result, nil
}

// Create 创建新会话。
func (s *JSONLStore) Create(opts CreateOpts) (*Info, error) {
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

	sd := &sessionData{
		info:   info,
		events: make([]Event, 0),
		seq:    0,
	}
	s.cacheMu.Lock()
	s.cache[id] = sd
	s.cacheMu.Unlock()

	// 创建空文件
	path := filepath.Join(s.baseDir, id+".jsonl")
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("session: 创建文件失败: %w", err)
	}
	f.Close()

	return info, nil
}

// Get 根据 ID 获取会话信息。
func (s *JSONLStore) Get(id string) (*Info, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sd, err := s.loadSession(id)
	if err != nil {
		return nil, err
	}
	cp := *sd.info
	return &cp, nil
}

// List 列举会话，支持过滤和分页。
func (s *JSONLStore) List(filter ListFilter) ([]*Info, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		return nil, fmt.Errorf("session: 读取目录 %s 失败: %w", s.baseDir, err)
	}

	var all []*Info
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".jsonl")
		if !safeIDPattern.MatchString(id) {
			continue
		}

		sd, err := s.loadSession(id)
		if err != nil {
			continue
		}

		if !filter.AfterTime.IsZero() && sd.info.CreatedAt.Before(filter.AfterTime) {
			continue
		}
		cp := *sd.info
		all = append(all, &cp)
	}

	// 按 CreatedAt 降序排列
	for i := 0; i < len(all); i++ {
		for j := i + 1; j < len(all); j++ {
			if all[i].CreatedAt.Before(all[j].CreatedAt) {
				all[i], all[j] = all[j], all[i]
			}
		}
	}

	if filter.Offset > 0 && filter.Offset < len(all) {
		all = all[filter.Offset:]
	}
	if filter.Limit > 0 && len(all) > filter.Limit {
		all = all[:filter.Limit]
	}
	return all, nil
}

// Delete 删除一个会话（文件 + 缓存）。
func (s *JSONLStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path, err := s.safePath(id)
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("session: 会话 %s 不存在", id)
		}
		return fmt.Errorf("session: 删除文件失败: %w", err)
	}

	s.cacheMu.Lock()
	delete(s.cache, id)
	s.cacheMu.Unlock()
	return nil
}

// Messages 投影当前会话的所有事件为 LLM 消息列表。
func (s *JSONLStore) Messages() ([]llm.ChatMessage, error) {
	// Messages 需要 SessionID，调用者应使用 Events() + ProjectMessages()
	return nil, fmt.Errorf("session: JSONLStore.Messages() 不支持无 SessionID 调用")
}

// loadSession 从缓存或文件加载会话数据。
// 调用者必须持有 mu 锁（读或写），本函数只操作 cacheMu。
func (s *JSONLStore) loadSession(id string) (*sessionData, error) {
	// 先检查缓存（cacheMu 读锁）
	s.cacheMu.RLock()
	sd, ok := s.cache[id]
	s.cacheMu.RUnlock()
	if ok {
		return sd, nil
	}

	// 从文件加载（调用者已持有 mu 锁保护文件操作）
	path, err := s.safePath(id)
	if err != nil {
		return nil, err
	}

	events, err := s.readEvents(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("session: 会话 %s 不存在", id)
		}
		return nil, err
	}

	var maxSeq int64
	for _, e := range events {
		if e.Seq > maxSeq {
			maxSeq = e.Seq
		}
	}

	// 重建 Info
	info := &Info{
		ID:        id,
		CreatedAt: nowUTC(),
		UpdatedAt: nowUTC(),
	}
	if len(events) > 0 {
		info.CreatedAt = events[0].CreatedAt
		info.UpdatedAt = events[len(events)-1].CreatedAt
	}

	sd = &sessionData{
		info:   info,
		events: events,
		seq:    maxSeq,
	}
	// 写入缓存（cacheMu 写锁）
	s.cacheMu.Lock()
	s.cache[id] = sd
	s.cacheMu.Unlock()
	return sd, nil
}

// readEvents 从 JSONL 文件读取所有事件。
func (s *JSONLStore) readEvents(path string) ([]Event, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var events []Event
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var e Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, fmt.Errorf("session: 解析事件失败: %w", err)
		}
		events = append(events, e)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("session: 读取文件失败: %w", err)
	}
	return events, nil
}

// nowUTC 返回当前 UTC 时间。
func nowUTC() time.Time {
	return time.Now().UTC()
}
