package session

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// jsonlSyncBatchSize 控制追加日志的批量 fsync 间隔。O_APPEND 的每行写入
// 仍然是原子的；批量同步把系统调用成本从每条事件摊薄，同时把掉电时最多
// 未同步的事件数限制在一个小批次内。旧实现同样没有对临时文件显式 Sync，
// 因而这不会改变既有 API 的实际耐久性承诺。
const jsonlSyncBatchSize = 64

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

// AppendEvent 向会话追加一个事件。
//
// 当前版本使用 O_APPEND 写入单行，避免每次追加都重写整个会话文件。
// 文件锁保证多进程不会争用序号；单次写入和有界批量 Sync 保持追加顺序，并
// 限制掉电时尚未同步的尾部批次。读取端允许丢弃崩溃留下的最后一条不完整行，
// 中间损坏仍显式报错。
func (s *JSONLStore) AppendEvent(event Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path, err := s.safePath(event.SessionID)
	if err != nil {
		return err
	}

	// Unix 的 FileLock 会在打开锁文件时创建目标路径，因此先检查数据文件，
	// 避免对不存在的会话误创建一个空文件。
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("session: 会话 %s 不存在", event.SessionID)
		}
		return fmt.Errorf("session: 检查会话文件失败: %w", err)
	}
	fileLock := NewFileLock(path)
	if err := fileLock.Lock(5 * time.Second); err != nil {
		return fmt.Errorf("session: 获取会话追加锁失败: %w", err)
	}
	defer func() { _ = fileLock.Unlock() }()

	sd, err := s.loadSession(event.SessionID)
	if err != nil {
		return err
	}
	// 另一个进程可能在本实例缓存之后追加过事件。正常的单进程追加只需
	// stat，不会重新读取历史；检测到外部变化时才回读一次，保证 Seq 和
	// 本地缓存不会覆盖或重复外部事件。
	if err := s.refreshIfChanged(event.SessionID, path, sd); err != nil {
		return err
	}
	sd, err = s.loadSession(event.SessionID)
	if err != nil {
		return err
	}

	// v0/v1 文件首次追加时仍需一次性迁移到带当前头的格式；之后的追加
	// 都走增量路径。
	if sd.needsRewrite {
		if err := s.rewriteSession(path, sd.events); err != nil {
			return err
		}
		sd.needsRewrite = false
		sd.eventsSinceSync = 0
		if err := s.updateFileState(path, sd); err != nil {
			return err
		}
	}

	nextSeq := sd.seq + 1
	event.Seq = nextSeq
	if event.ID == "" {
		event.ID = newID()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = nowUTC()
	}
	updatedAt := nowUTC()
	// 写入的文件头声明的是 SchemaVersion，因此每条事件都要与之对齐：
	// 历史事件在加载时已由 migrateEventLine 升级，此处统一盖章保证
	// 「文件头版本 == 内容版本」，避免出现头说 v1、内容还是 v0 的半迁移状态。
	event.SchemaVersion = SchemaVersion
	line, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("session: 编码事件失败: %w", err)
	}
	line = append(line, '\n')
	forceSync := sd.eventsSinceSync+1 >= jsonlSyncBatchSize
	if err := appendJSONLine(path, line, forceSync); err != nil {
		return err
	}

	// 只有磁盘写入成功后才更新内存，避免失败写入污染缓存和 Seq。
	sd.seq = nextSeq
	sd.events = append(sd.events, event)
	sd.info.UpdatedAt = updatedAt
	if forceSync {
		sd.eventsSinceSync = 0
	} else {
		sd.eventsSinceSync++
	}
	if err := s.updateFileState(path, sd); err != nil {
		return err
	}

	// 更新缓存（sd 仍由 s.mu 保护）。
	s.cacheMu.Lock()
	s.cache[event.SessionID] = sd
	s.cacheMu.Unlock()

	return nil
}

// refreshIfChanged 只在发现文件由其他进程改动时重新读盘。单进程的热路径
// 只做一次 stat，避免把历史事件重新编码/读取，跨进程追加仍能保持 Seq 连续。
func (s *JSONLStore) refreshIfChanged(id, path string, cached *sessionData) error {
	stat, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("session: 会话 %s 不存在", id)
		}
		return fmt.Errorf("session: 检查会话文件失败: %w", err)
	}
	if cached.fileSize == stat.Size() && cached.fileModTime.Equal(stat.ModTime()) {
		return nil
	}

	events, needsRewrite, err := s.readEventsWithMeta(path)
	if err != nil {
		return err
	}
	info := cached.info
	if info == nil {
		info = &Info{ID: id, CreatedAt: nowUTC(), UpdatedAt: nowUTC()}
	}
	if len(events) > 0 {
		if info.CreatedAt.IsZero() {
			info.CreatedAt = events[0].CreatedAt
		}
		info.UpdatedAt = events[len(events)-1].CreatedAt
	}
	var maxSeq int64
	for _, event := range events {
		if event.Seq > maxSeq {
			maxSeq = event.Seq
		}
	}
	refreshed := &sessionData{
		info:         info,
		events:       events,
		seq:          maxSeq,
		fileSize:     stat.Size(),
		fileModTime:  stat.ModTime(),
		needsRewrite: needsRewrite,
	}
	s.cacheMu.Lock()
	s.cache[id] = refreshed
	s.cacheMu.Unlock()
	return nil
}

func (s *JSONLStore) updateFileState(path string, sd *sessionData) error {
	stat, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("session: 检查写入文件失败: %w", err)
	}
	sd.fileSize = stat.Size()
	sd.fileModTime = stat.ModTime()
	return nil
}

// rewriteSession 只用于旧格式首次追加时的迁移；当前格式的热路径不会调用它。
func (s *JSONLStore) rewriteSession(path string, events []Event) error {
	tmpPath := path + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("session: 创建迁移临时文件失败: %w", err)
	}
	cleanup := func() {
		_ = f.Close()
		_ = os.Remove(tmpPath)
	}
	header, err := encodeHeader()
	if err != nil {
		cleanup()
		return fmt.Errorf("session: 生成文件头失败: %w", err)
	}
	if _, err := f.Write(header); err != nil {
		cleanup()
		return fmt.Errorf("session: 写入文件头失败: %w", err)
	}
	enc := json.NewEncoder(f)
	for i := range events {
		events[i].SchemaVersion = SchemaVersion
		if err := enc.Encode(events[i]); err != nil {
			cleanup()
			return fmt.Errorf("session: 写入迁移事件失败: %w", err)
		}
	}
	if err := f.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("session: 同步迁移文件失败: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("session: 关闭迁移文件失败: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("session: 重命名迁移文件失败: %w", err)
	}
	return nil
}

// appendJSONLine 处理崩溃残留的最后一行后，以 O_APPEND 单次写入事件。
func appendJSONLine(path string, line []byte, forceSync bool) error {
	separator, err := prepareAppendTail(path)
	if err != nil {
		return err
	}
	payload := line
	if separator {
		payload = make([]byte, 0, len(line)+1)
		payload = append(payload, '\n')
		payload = append(payload, line...)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("session: 打开追加文件失败: %w", err)
	}
	if _, err := f.Write(payload); err != nil {
		_ = f.Close()
		return fmt.Errorf("session: 追加事件失败: %w", err)
	}
	if forceSync {
		if err := f.Sync(); err != nil {
			_ = f.Close()
			return fmt.Errorf("session: 同步追加事件失败: %w", err)
		}
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("session: 关闭追加文件失败: %w", err)
	}
	return nil
}

// prepareAppendTail 返回是否需要在新事件前补换行，并修剪崩溃留下的坏尾。
// 中间已有换行的坏 JSON 不在这里修复，交由 readEvents 明确报错。
func prepareAppendTail(path string) (bool, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0o600)
	if err != nil {
		return false, fmt.Errorf("session: 打开追加尾部失败: %w", err)
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return false, fmt.Errorf("session: 检查追加尾部失败: %w", err)
	}
	if stat.Size() == 0 {
		return false, fmt.Errorf("session: 会话文件为空")
	}
	last := []byte{0}
	if _, err := f.ReadAt(last, stat.Size()-1); err != nil {
		return false, fmt.Errorf("session: 读取追加尾部失败: %w", err)
	}
	if last[0] == '\n' {
		return false, nil
	}
	offset, err := lastNewlineOffset(f, stat.Size())
	if err != nil {
		return false, err
	}
	if stat.Size()-offset > 16*1024*1024 {
		return false, fmt.Errorf("session: 追加尾部超过单行大小限制")
	}
	tail := make([]byte, stat.Size()-offset)
	if _, err := f.ReadAt(tail, offset); err != nil {
		return false, fmt.Errorf("session: 读取追加尾部内容失败: %w", err)
	}
	trimmed := bytes.TrimSpace(tail)
	if json.Valid(trimmed) {
		return true, nil
	}
	if offset == 0 {
		return false, fmt.Errorf("session: 会话文件尾部损坏且无法安全修剪")
	}
	if err := f.Truncate(offset); err != nil {
		return false, fmt.Errorf("session: 修剪崩溃残尾失败: %w", err)
	}
	if err := f.Sync(); err != nil {
		return false, fmt.Errorf("session: 同步残尾修剪失败: %w", err)
	}
	return false, nil
}

func lastNewlineOffset(f *os.File, size int64) (int64, error) {
	const chunkSize int64 = 64 * 1024
	for end := size; end > 0; {
		start := end - chunkSize
		if start < 0 {
			start = 0
		}
		buf := make([]byte, end-start)
		if _, err := f.ReadAt(buf, start); err != nil && !errors.Is(err, io.EOF) {
			return 0, fmt.Errorf("session: 查找追加尾部失败: %w", err)
		}
		for i := len(buf) - 1; i >= 0; i-- {
			if buf[i] == '\n' {
				return start + int64(i) + 1, nil
			}
		}
		end = start
	}
	return 0, nil
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

	id, err := resolveCreateID(opts)
	if err != nil {
		return nil, err
	}
	// 已存在则不覆盖：指定 ID 的创建（迁移路径）一旦覆盖，就是静默的数据丢失。
	if _, statErr := os.Stat(filepath.Join(s.baseDir, id+".jsonl")); statErr == nil {
		return nil, fmt.Errorf("session: 会话 %s 已存在", id)
	}
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
	if err := s.updateFileState(path, sd); err != nil {
		return nil, err
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
			// 版本过高必须向上报错，不能像损坏文件那样跳过。
			//
			// 跳过会让 `session list` 输出「没有找到会话」——用户看到的是「数据没了」，
			// 而不是「数据来自更新的版本」。pkg/session 的契约写的是「读到更高版本
			// 返回 ErrSchemaTooNew，没有尽力解析的降级模式」，此处在列举路径上兑现它。
			// 其余加载失败（内容损坏等）保持跳过，不因单个坏文件让整次列举失败。
			if errors.Is(err, ErrSchemaTooNew) {
				return nil, err
			}
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

	events, needsRewrite, err := s.readEventsWithMeta(path)
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

	stat, statErr := os.Stat(path)
	if statErr != nil {
		return nil, fmt.Errorf("session: 检查会话文件失败: %w", statErr)
	}
	sd = &sessionData{
		info:         info,
		events:       events,
		seq:          maxSeq,
		fileSize:     stat.Size(),
		fileModTime:  stat.ModTime(),
		needsRewrite: needsRewrite,
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
	events, _, err := s.readEventsWithMeta(path)
	return events, err
}

func (s *JSONLStore) readEventsWithMeta(path string) ([]Event, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return nil, false, err
	}
	lastByte := byte('\n')
	if stat.Size() > 0 {
		var last [1]byte
		if _, err := f.ReadAt(last[:], stat.Size()-1); err != nil {
			return nil, false, fmt.Errorf("session: 读取文件尾部失败: %w", err)
		}
		lastByte = last[0]
	}

	var events []Event
	scanner := bufio.NewScanner(f)
	// 单条事件可能携带很长的工具输出，默认 64KB 上限会直接报 token too long。
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	firstLine := true
	lineNo := 0
	needsRewrite := false
	var pending string
	for scanner.Scan() {
		if pending != "" {
			lineNo++
			var err error
			events, needsRewrite, err = s.decodeEventLine(path, pending, lineNo, firstLine, events, needsRewrite, false)
			if err != nil {
				return nil, false, err
			}
			firstLine = false
		}
		pending = scanner.Text()
	}
	if err := scanner.Err(); err != nil {
		return nil, false, fmt.Errorf("session: 读取文件失败: %w", err)
	}
	if pending != "" {
		lineNo++
		finalIncomplete := stat.Size() > 0 && lastByte != '\n'
		var err error
		events, needsRewrite, err = s.decodeEventLine(path, pending, lineNo, firstLine, events, needsRewrite, finalIncomplete)
		if err != nil {
			return nil, false, err
		}
	}
	return events, needsRewrite, nil
}

func (s *JSONLStore) decodeEventLine(path, raw string, lineNo int, firstLine bool,
	events []Event, needsRewrite, finalIncomplete bool) ([]Event, bool, error) {
	line := strings.TrimSpace(raw)
	if line == "" {
		return events, needsRewrite, nil
	}
	if firstLine {
		if v, ok := probeHeaderVersion([]byte(line)); ok {
			if v > SchemaVersion {
				return nil, false, fmt.Errorf("%w（文件 %s 为 v%d，当前支持 v%d）",
					ErrSchemaTooNew, filepath.Base(path), v, SchemaVersion)
			}
			if v != SchemaVersion {
				needsRewrite = true
			}
			return events, needsRewrite, nil
		}
		// 非文件头：这是 v0 老文件，首行本身就是事件。
		needsRewrite = true
	}
	version, probeErr := probeEventSchema([]byte(line))
	if probeErr == nil && version < SchemaVersion {
		needsRewrite = true
	}
	migrated, err := migrateEventLine([]byte(line))
	if err != nil {
		if finalIncomplete && !errors.Is(err, ErrSchemaTooNew) {
			// 进程可能在最后一条事件写完换行前退出；保留前面的完整事件，
			// 下次追加时 prepareAppendTail 会在锁内修剪这条残尾。
			return events, needsRewrite, nil
		}
		return nil, false, fmt.Errorf("%w（文件 %s 第 %d 行）", err, filepath.Base(path), lineNo)
	}
	var e Event
	if err := json.Unmarshal(migrated, &e); err != nil {
		if finalIncomplete {
			return events, needsRewrite, nil
		}
		return nil, false, fmt.Errorf("session: 解析事件失败（第 %d 行）: %w", lineNo, err)
	}
	return append(events, e), needsRewrite, nil
}

// nowUTC 返回当前 UTC 时间。
func nowUTC() time.Time {
	return time.Now().UTC()
}
