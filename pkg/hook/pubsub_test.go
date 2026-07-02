package hook

import (
	"sync"
	"testing"
	"time"
)

// TestSubscribePublish 测试订阅后能收到发布的事件
func TestSubscribePublish(t *testing.T) {
	broker := NewBroker[string]()
	defer broker.Close()

	ch := broker.Subscribe()
	broker.Publish("hello")

	select {
	case msg := <-ch:
		if msg != "hello" {
			t.Errorf("expected 'hello', got '%s'", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

// TestMultipleSubscribers 测试多订阅者都收到事件
func TestMultipleSubscribers(t *testing.T) {
	broker := NewBroker[int]()
	defer broker.Close()

	ch1 := broker.Subscribe()
	ch2 := broker.Subscribe()

	broker.Publish(42)

	// 两个订阅者都应收到
	for i, ch := range []<-chan int{ch1, ch2} {
		select {
		case msg := <-ch:
			if msg != 42 {
				t.Errorf("subscriber %d: expected 42, got %d", i, msg)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d timeout", i)
		}
	}
}

// TestUnsubscribe 测试取消订阅后不再收到事件
func TestUnsubscribe(t *testing.T) {
	broker := NewBroker[string]()
	defer broker.Close()

	ch := broker.Subscribe()
	broker.Unsubscribe(ch)

	// 取消后发布消息
	broker.Publish("should not be received")

	// channel 应被关闭
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected channel to be closed after unsubscribe")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for channel close")
	}
}

// TestClose 测试 Close 后所有 channel 关闭
func TestClose(t *testing.T) {
	broker := NewBroker[int]()
	ch1 := broker.Subscribe()
	ch2 := broker.Subscribe()

	broker.Close()

	// 所有 channel 应被关闭
	for _, ch := range []<-chan int{ch1, ch2} {
		if _, ok := <-ch; ok {
			t.Error("expected channel to be closed")
		}
	}
}

// TestPublishNonBlocking 测试慢订阅者不会阻塞其他订阅者
func TestPublishNonBlocking(t *testing.T) {
	broker := NewBroker[string]()
	defer broker.Close()

	// 慢订阅者：只订阅不消费，很快会填满 buffer
	broker.Subscribe()

	// 正常订阅者
	ch := broker.Subscribe()

	// 并发发布大量消息，不应阻塞
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			broker.Publish("event")
		}
	}()

	// 正常订阅者应能收到一些消息
	received := 0
	done := make(chan struct{})
	go func() {
		for range ch {
			received++
		}
		close(done)
	}()

	wg.Wait()
	// 给足够时间处理已发送的消息
	time.Sleep(100 * time.Millisecond)

	// 验证没有死锁，且收到至少一条消息
	if received == 0 {
		t.Error("expected at least one event to be received")
	}
}