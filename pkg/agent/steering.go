package agent

import (
	"context"
	"errors"
	"sync"

	"github.com/wly2lcl/basework/pkg/llm"
)

// DeliveryMode 消息交付模式
type DeliveryMode int

const (
	// Steer 立即打断模式：当前工具完成后中断，注入消息
	Steer DeliveryMode = iota
	// Queue 排队模式：当前 turn 完成后，在下次 setupTurn 时注入
	Queue
)

// steeringMsg 是 steering 消息的内部表示
type steeringMsg struct {
	content string
	mode    DeliveryMode
	seq     int64
}

// SteeringManager 管理消息注入
type SteeringManager struct {
	mu           sync.Mutex
	steerCh      chan steeringMsg // Steer 模式通道
	queueCh      chan steeringMsg // Queue 模式通道
	pending      []steeringMsg    // Drain 后尚未成功写入会话事件的消息
	seqGen       int64            // 序列号生成器
	canceledSeqs map[int64]bool   // 已取消的序列号
}

// NewSteeringManager 创建 SteeringManager
func NewSteeringManager() *SteeringManager {
	return &SteeringManager{
		steerCh:      make(chan steeringMsg, 10),
		queueCh:      make(chan steeringMsg, 10),
		canceledSeqs: make(map[int64]bool),
	}
}

// InjectMessage 注入一条 steering 消息
// ctx 用于取消操作；content 是消息内容；mode 指定交付模式
func (sm *SteeringManager) InjectMessage(_ context.Context, content string, mode DeliveryMode) error {
	sm.mu.Lock()
	sm.seqGen++
	seq := sm.seqGen
	sm.mu.Unlock()

	msg := steeringMsg{content: content, mode: mode, seq: seq}

	switch mode {
	case Steer:
		select {
		case sm.steerCh <- msg:
		default:
			return errors.New("steering: Steer 通道已满")
		}
	case Queue:
		select {
		case sm.queueCh <- msg:
		default:
			return errors.New("steering: Queue 通道已满")
		}
	default:
		return errors.New("steering: 未知的 DeliveryMode")
	}

	return nil
}

// Drain 清空当前 pending 的 steering 消息，返回所有未被 cancel 的消息
// 在 setupTurn 阶段调用
func (sm *SteeringManager) Drain() []string {
	pending := sm.drainPending()
	msgs := make([]string, len(pending))
	for i, msg := range pending {
		msgs[i] = msg.content
	}
	return msgs
}

// drainPending 取出待交付 steering，保留 seq 以便事件写入失败后恢复。
func (sm *SteeringManager) drainPending() []steeringMsg {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	msgs := make([]steeringMsg, 0, len(sm.pending))
	appendActive := func(msg steeringMsg) {
		if !sm.canceledSeqs[msg.seq] {
			msgs = append(msgs, msg)
		}
		delete(sm.canceledSeqs, msg.seq)
	}
	for _, msg := range sm.pending {
		appendActive(msg)
	}
	sm.pending = nil

	// 先 drain Steer 通道
drainSteer:
	for {
		select {
		case msg := <-sm.steerCh:
			appendActive(msg)
		default:
			break drainSteer
		}
	}

	// 再 drain Queue 通道
	for {
		select {
		case msg := <-sm.queueCh:
			appendActive(msg)
		default:
			return msgs
		}
	}
}

// restorePending 把未能写入事件日志的消息放回队首，下一次 turn 可重试。
func (sm *SteeringManager) restorePending(msgs []steeringMsg) {
	if len(msgs) == 0 {
		return
	}
	sm.mu.Lock()
	defer sm.mu.Unlock()
	restored := make([]steeringMsg, 0, len(msgs)+len(sm.pending))
	restored = append(restored, msgs...)
	restored = append(restored, sm.pending...)
	sm.pending = restored
}

// Cancel 取消指定序列号的消息
func (sm *SteeringManager) Cancel(seq int64) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.canceledSeqs[seq] = true
}

// steeringToChatMessages 将 steering 消息转换为 LLM system 消息列表
func steeringToChatMessages(msgs []string) []llm.ChatMessage {
	chatMsgs := make([]llm.ChatMessage, 0, len(msgs))
	for _, m := range msgs {
		chatMsgs = append(chatMsgs, llm.ChatMessage{
			Role: llm.RoleSystem,
			Content: []llm.ContentPart{
				{Type: llm.ContentTypeText, Text: m},
			},
		})
	}
	return chatMsgs
}
