package session

import (
	"sync"
)

// Message 表示队列中的一条消息。
type Message struct {
	ID      string
	Content string
}

// Queue 是一个线程安全的 FIFO 消息队列。
// 用于在 Agent 忙时暂存用户输入，按入队顺序依次处理。
type Queue struct {
	mu       sync.Mutex
	messages []Message
	maxSize  int
}

// NewQueue 创建一个新的 Queue。
// maxSize 为 0 表示不限制大小。
func NewQueue(maxSize int) *Queue {
	return &Queue{
		messages: make([]Message, 0),
		maxSize:  maxSize,
	}
}

// Enqueue 将消息加入队列尾部。
// 如果队列已满（maxSize > 0 且达到上限），返回 false。
func (q *Queue) Enqueue(msg Message) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.maxSize > 0 && len(q.messages) >= q.maxSize {
		return false
	}
	q.messages = append(q.messages, msg)
	return true
}

// Dequeue 从队列头部取出下一条消息（FIFO）。
// 如果队列为空，返回空 Message。
func (q *Queue) Dequeue() Message {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.messages) == 0 {
		return Message{}
	}
	msg := q.messages[0]
	q.messages = q.messages[1:]
	return msg
}

// Cancel 根据消息 ID 从队列中移除指定的消息。
// 如果消息不存在，返回 false。
func (q *Queue) Cancel(msgID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	for i, msg := range q.messages {
		if msg.ID == msgID {
			q.messages = append(q.messages[:i], q.messages[i+1:]...)
			return true
		}
	}
	return false
}

// Len 返回队列中的消息数量。
func (q *Queue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	return len(q.messages)
}

// IsEmpty 返回队列是否为空。
func (q *Queue) IsEmpty() bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	return len(q.messages) == 0
}