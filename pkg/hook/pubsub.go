package hook

import "sync"

// Broker 泛型事件总线
type Broker[T any] struct {
	subs []chan T
	mu   sync.RWMutex
	done bool
}

// NewBroker 创建新的事件总线
func NewBroker[T any]() *Broker[T] {
	return &Broker[T]{}
}

// Subscribe 订阅事件，返回带缓冲的 channel (buffer=64)
func (b *Broker[T]) Subscribe() <-chan T {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan T, 64)
	b.subs = append(b.subs, ch)
	return ch
}

// Unsubscribe 取消订阅，移除并关闭 channel
func (b *Broker[T]) Unsubscribe(ch <-chan T) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for i, s := range b.subs {
		if s == ch {
			b.subs = append(b.subs[:i], b.subs[i+1:]...)
			close(s)
			return
		}
	}
}

// Publish 非阻塞发送事件到所有订阅者
func (b *Broker[T]) Publish(event T) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, s := range b.subs {
		select {
		case s <- event:
		default:
			// 非阻塞：慢订阅者跳过
		}
	}
}

// Close 关闭所有 channel
func (b *Broker[T]) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, s := range b.subs {
		close(s)
	}
	b.subs = nil
	b.done = true
}
