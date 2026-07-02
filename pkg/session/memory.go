package session

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/wly2lcl/basework/pkg/llm"
)

// sessionData 是内部存储结构，包含会话信息和事件列表。
type sessionData struct {
	info   *Info
	events []Event
	seq    int64 // 该会话的事件序列计数器
}

// MemoryStore 是一个内存会话存储实现，使用 map + RWMutex 保证并发安全。
type MemoryStore struct {
	mu       sync.RWMutex
	sessions map[string]*sessionData
}

// NewMemoryStore 创建一个新的 MemoryStore。
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		sessions: make(map[string]*sessionData),
	}
}

// AppendEvent 向指定会话追加事件，自动递增 Seq。
func (s *MemoryStore) AppendEvent(event Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sd, ok := s.sessions[event.SessionID]
	if !ok {
		return fmt.Errorf("session: 会话 %s 不存在", event.SessionID)
	}

	sd.seq++
	event.Seq = sd.seq
	if event.ID == "" {
		event.ID = newID()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}

	sd.events = append(sd.events, event)
	sd.info.UpdatedAt = time.Now().UTC()
	return nil
}

// Events 根据过滤条件返回事件列表。
func (s *MemoryStore) Events(filter EventFilter) ([]Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sd, ok := s.sessions[filter.SessionID]
	if !ok {
		return nil, fmt.Errorf("session: 会话 %s 不存在", filter.SessionID)
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
func (s *MemoryStore) Create(opts CreateOpts) (*Info, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := newID()
	now := time.Now().UTC()
	info := &Info{
		ID:        id,
		Title:     opts.Title,
		CreatedAt: now,
		UpdatedAt: now,
	}

	s.sessions[id] = &sessionData{
		info:   info,
		events: make([]Event, 0),
		seq:    0,
	}
	return info, nil
}

// Get 根据 ID 获取会话信息。
func (s *MemoryStore) Get(id string) (*Info, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sd, ok := s.sessions[id]
	if !ok {
		return nil, fmt.Errorf("session: 会话 %s 不存在", id)
	}
	// 返回副本避免外部修改
	cp := *sd.info
	return &cp, nil
}

// List 列举会话，支持过滤和分页。
func (s *MemoryStore) List(filter ListFilter) ([]*Info, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var all []*Info
	for _, sd := range s.sessions {
		if !filter.AfterTime.IsZero() && sd.info.CreatedAt.Before(filter.AfterTime) {
			continue
		}
		cp := *sd.info
		all = append(all, &cp)
	}

	// 按 CreatedAt 降序排列
	sort.Slice(all, func(i, j int) bool {
		return all[i].CreatedAt.After(all[j].CreatedAt)
	})

	// 分页
	if filter.Offset > 0 && filter.Offset < len(all) {
		all = all[filter.Offset:]
	}
	if filter.Limit > 0 && len(all) > filter.Limit {
		all = all[:filter.Limit]
	}
	return all, nil
}

// Delete 删除一个会话。
func (s *MemoryStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sessions[id]; !ok {
		return fmt.Errorf("session: 会话 %s 不存在", id)
	}
	delete(s.sessions, id)
	return nil
}

// Messages 投影当前会话的所有事件为 LLM 消息列表。
func (s *MemoryStore) Messages() ([]llm.ChatMessage, error) {
	// Messages 需要 SessionID，这里通过存储设计无法确定
	// 调用者应使用 Events() + ProjectMessages() 方式
	return nil, fmt.Errorf("session: MemoryStore.Messages() 不支持无 SessionID 调用")
}

// newID 生成 16 字节随机 hex 字符串作为 ID。
func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("session: 生成随机 ID 失败: %v", err))
	}
	return hex.EncodeToString(b)
}
