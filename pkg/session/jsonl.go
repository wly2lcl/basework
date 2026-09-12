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

// sessionSchemaName 是 JSONL 文件头的 `_schema` 标识值。
const sessionSchemaName = "basework.session"

// sessionHeader 是 JSONL 文件的首行，记录该文件的格式版本。
//
// 刻意不含时间戳：文件头每次重写都会重新序列化，若带时间戳则首行会无意义地
// 变动，既不可读也让 diff 失去意义。会话创建时间可由首个事件推得。
type sessionHeader struct {
	Schema string `json:"_schema"`
	V      int    `json:"v"`
}

// encodeHeader 返回当前格式版本的文件头字节（以换行结尾）。
func encodeHeader() ([]byte, error) {
	b, err := json.Marshal(sessionHeader{Schema: sessionSchemaName, V: SchemaVersion})
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// probeHeaderVersion 判断一行是否为文件头，并返回其格式版本。
// 非文件头（例如 v0 老文件的首条事件）返回 ok=false。
func probeHeaderVersion(line []byte) (version int, ok bool) {
	var probe struct {
		Schema *string `json:"_schema"`
		V      int     `json:"v"`
	}
	if err := json.Unmarshal(line, &probe); err != nil {
		return 0, false
	}
	if probe.Schema == nil || *probe.Schema != sessionSchemaName {
		return 0, false
	}
	return probe.V, true
}

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
	// 写入的文件头声明的是 SchemaVersion，因此每条事件都要与之对齐：
	// 历史事件在加载时已由 migrateEventLine 升级，此处统一盖章保证
	// 「文件头版本 == 内容版本」，避免出现头说 v1、内容还是 v0 的半迁移状态。
	event.SchemaVersion = SchemaVersion
	sd.events = append(sd.events, event)
	sd.info.UpdatedAt = nowUTC()

	// 更新缓存
	s.cacheMu.Lock()
	s.cache[event.SessionID] = sd
	s.cacheMu.Unlock()

	// 原子写入：写文件头 + 全部事件到 tmp 文件，再 rename
	tmpPath := path + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("session: 创建临时文件失败: %w", err)
	}
	header, err := encodeHeader()
	if err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("session: 生成文件头失败: %w", err)
	}
	if _, err := f.Write(header); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("session: 写入文件头失败: %w", err)
	}
	enc := json.NewEncoder(f)
	for i := range sd.events {
		sd.events[i].SchemaVersion = SchemaVersion
		if err := enc.Encode(sd.events[i]); err != nil {
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

	// 创建仅含文件头的文件。文件头声明格式版本，读取端据此判定是否需要迁移，
	// 也据此拒绝来自更高版本程序的数据。
	path := filepath.Join(s.baseDir, id+".jsonl")
	header, err := encodeHeader()
	if err != nil {
		return nil, fmt.Errorf("session: 生成文件头失败: %w", err)
	}
	if err := os.WriteFile(path, header, 0644); err != nil {
		return nil, fmt.Errorf("session: 创建文件失败: %w", err)
	}

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
//
// 处理三件事：
//  1. 首行文件头识别——含 `_schema` 即为头，跳过；否则视为 v0 老文件（首行即事件）。
//     判定是 O(1) 的（只看首行），读路径是热路径，不做全文件扫描。
//  2. 版本判定——文件头或单条事件的版本高于 SchemaVersion 时返回 ErrSchemaTooNew。
//  3. 逐条迁移——低版本事件经 migrate.go 的相邻迁移链升级到当前版本。
func (s *JSONLStore) readEvents(path string) ([]Event, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var events []Event
	scanner := bufio.NewScanner(f)
	// 单条事件可能携带很长的工具输出，默认 64KB 上限会直接报 token too long。
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	firstLine := true
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if firstLine {
			firstLine = false
			if v, ok := probeHeaderVersion([]byte(line)); ok {
				if v > SchemaVersion {
					return nil, fmt.Errorf("%w（文件 %s 为 v%d，当前支持 v%d）",
						ErrSchemaTooNew, filepath.Base(path), v, SchemaVersion)
				}
				continue
			}
			// 非文件头：这是 v0 老文件，首行本身就是事件，继续往下走。
		}

		migrated, err := migrateEventLine([]byte(line))
		if err != nil {
			return nil, fmt.Errorf("%w（文件 %s 第 %d 行）", err, filepath.Base(path), lineNo)
		}

		var e Event
		if err := json.Unmarshal(migrated, &e); err != nil {
			return nil, fmt.Errorf("session: 解析事件失败（第 %d 行）: %w", lineNo, err)
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
