package observability

import (
	"fmt"
	"os"
	"runtime/debug"
	"sync"
)

// EventHandler 是事件处理函数的类型
type EventHandler func(event Event)

// EventBus 是事件总线，支持发布-订阅模式。
// 事件处理采用异步方式，发布者不会阻塞。
// 并发 goroutine 上限为 100，超出时发布者会阻塞等待。
type EventBus struct {
	mu          sync.RWMutex
	subscribers map[string][]EventHandler
	sem         chan struct{}
}

// NewEventBus 创建事件总线
func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[string][]EventHandler),
		sem:         make(chan struct{}, 100),
	}
}

// Subscribe 订阅指定类型的事件。
// eventType 为事件类型，handler 为事件处理函数。
func (eb *EventBus) Subscribe(eventType string, handler EventHandler) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	eb.subscribers[eventType] = append(eb.subscribers[eventType], handler)
}

// Publish 发布事件，所有订阅了该事件类型的处理函数将异步执行。
// 并发 goroutine 数量受 semaphore channel 限制（上限 100），
// 超出时发布者会阻塞等待已有 handler 完成。
func (eb *EventBus) Publish(event Event) {
	eb.mu.RLock()
	handlers := eb.subscribers[event.Type]
	eb.mu.RUnlock()

	if len(handlers) == 0 {
		return
	}

	// 异步处理事件，避免阻塞发布者
	for _, handler := range handlers {
		h := handler // 捕获变量
		eb.sem <- struct{}{} // 获取信号量，超限时阻塞
		go func() {
			defer func() { <-eb.sem }() // 释放信号量
			defer func() {
				if r := recover(); r != nil {
					fmt.Fprintf(os.Stderr, "EventBus handler panic: %v\n%s\n", r, debug.Stack())
				}
			}()
			h(event)
		}()
	}
}

// UnsubscribeAll 取消指定事件类型的所有订阅
func (eb *EventBus) UnsubscribeAll(eventType string) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	delete(eb.subscribers, eventType)
}