package observability

import (
	"bytes"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestEventBus_GoroutineLimit 验证并发 goroutine 上限为 100
func TestEventBus_GoroutineLimit(t *testing.T) {
	bus := NewEventBus()

	// 注册一个会阻塞的 handler
	blocked := make(chan struct{})
	release := make(chan struct{})
	var active int32

	bus.Subscribe("test.event", func(event Event) {
		atomic.AddInt32(&active, 1)
		<-blocked // 首次调用会阻塞等待
		<-release
		atomic.AddInt32(&active, -1)
	})

	// 第一次发布，handler 进入阻塞
	bus.Publish(NewEvent("test.event", nil))

	// 等待 handler 开始执行
	time.Sleep(50 * time.Millisecond)

	// 获取初始活跃数
	initial := atomic.LoadInt32(&active)
	if initial != 1 {
		t.Fatalf("期望 1 个活跃 handler，得到 %d", initial)
	}

	// 发布 99 个事件，触发 99 个新 goroutine（总计 100）
	for i := 0; i < 99; i++ {
		bus.Publish(NewEvent("test.event", nil))
	}

	// 等待 goroutine 启动
	time.Sleep(100 * time.Millisecond)

	// 此时应该有 100 个活跃 goroutine
	current := atomic.LoadInt32(&active)
	if current != 100 {
		t.Fatalf("期望 100 个活跃 goroutine（上限），得到 %d", current)
	}

	// 再发布 50 个事件，这些应被信号量阻塞
	publishDone := make(chan struct{})
	go func() {
		for i := 0; i < 50; i++ {
			bus.Publish(NewEvent("test.event", nil))
		}
		close(publishDone)
	}()

	// 确认 Publish 被阻塞（50ms 内不应完成）
	select {
	case <-publishDone:
		current = atomic.LoadInt32(&active)
		if current > 100 {
			t.Fatalf("活跃 goroutine 超过上限 100: %d", current)
		}
	case <-time.After(50 * time.Millisecond):
		// 正常：Publish 被信号量阻塞
		current = atomic.LoadInt32(&active)
		if current > 100 {
			t.Fatalf("活跃 goroutine 超过上限 100: %d", current)
		}
	}

	// 释放所有 handler
	close(blocked)
	time.Sleep(50 * time.Millisecond)
	close(release)

	// 等待所有 goroutine 完成
	time.Sleep(200 * time.Millisecond)

	// 验证 publishDone 已关闭（所有 50 个额外事件都发布了）
	select {
	case <-publishDone:
		// 正常
	default:
		t.Fatal("发布 50 个额外事件超时")
	}
}

// TestEventBus_StacktraceOnPanic 验证 panic 时输出完整 stack trace
func TestEventBus_StacktraceOnPanic(t *testing.T) {
	bus := NewEventBus()

	// 使用 os.Pipe 捕获 stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("创建 pipe 失败: %v", err)
	}

	origStderr := os.Stderr
	os.Stderr = w

	bus.Subscribe("test.event", func(event Event) {
		panic("模拟 panic 测试")
	})

	bus.Publish(NewEvent("test.event", nil))

	// 等待 handler 执行
	time.Sleep(100 * time.Millisecond)

	// 关闭写入端并恢复 stderr
	w.Close()
	os.Stderr = origStderr

	// 读取捕获的输出
	var buf bytes.Buffer
	io.Copy(&buf, r)
	r.Close()

	stderr := buf.String()

	// 验证输出包含 panic 消息
	if !strings.Contains(stderr, "EventBus handler panic: 模拟 panic 测试") {
		t.Errorf("stderr 应包含 panic 消息，得到:\n%s", stderr)
	}

	// 验证包含 debug.Stack() 输出的 goroutine 信息
	if !strings.Contains(stderr, "goroutine ") {
		t.Errorf("stderr 应包含 goroutine stack trace，得到:\n%s", stderr)
	}

	// 验证包含此测试函数的文件名（stack trace 应从 handler 处展开）
	if !strings.Contains(stderr, "bus_test.go") {
		t.Errorf("stack trace 应包含本测试文件，得到:\n%s", stderr)
	}
}

// TestEventBus_SemaphoreFull_DropsEvent 验证信号量满时 Publish 不会阻塞
func TestEventBus_SemaphoreFull_DropsEvent(t *testing.T) {
	bus := NewEventBus()

	// 注册一个会阻塞的 handler（等待 release 信号）
	release := make(chan struct{})
	bus.Subscribe("test.event", func(event Event) {
		<-release
	})

	// 发布 100 个事件填满信号量
	for i := 0; i < 100; i++ {
		bus.Publish(NewEvent("test.event", nil))
	}

	// 等待所有 goroutine 获取信号量
	time.Sleep(100 * time.Millisecond)

	// 验证信号量已满：再发布一个事件不应阻塞
	done := make(chan struct{})
	go func() {
		bus.Publish(NewEvent("test.event", nil))
		close(done)
	}()

	select {
	case <-done:
		// 正常：Publish 没有阻塞
	case <-time.After(time.Second):
		t.Fatal("信号量满时 Publish 阻塞了，期望 non-blocking drop")
	}

	// 清理，防止 goroutine 泄漏
	close(release)
	time.Sleep(100 * time.Millisecond)
}

// TestEventBus_SemaphoreFull_DropsEvent_Log 验证信号量满时输出日志
func TestEventBus_SemaphoreFull_DropsEvent_Log(t *testing.T) {
	// 用 log.SetOutput 捕获日志输出，避免 stderr 重定向的并发问题
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	bus := NewEventBus()

	// 注册一个会阻塞的 handler
	release := make(chan struct{})
	bus.Subscribe("test.event", func(event Event) {
		<-release
	})

	// 填满信号量
	for i := 0; i < 100; i++ {
		bus.Publish(NewEvent("test.event", nil))
	}
	time.Sleep(100 * time.Millisecond)

	// 发布额外事件（应被 drop 并记录日志）
	bus.Publish(NewEvent("test.event", nil))
	time.Sleep(50 * time.Millisecond)

	output := buf.String()
	if !strings.Contains(output, "EventBus handler dropped") {
		t.Errorf("日志应包含 drop 消息，得到:\n%s", output)
	}

	close(release)
	time.Sleep(100 * time.Millisecond)
}
func TestEventBus_GoroutineLimitRecover(t *testing.T) {
	bus := NewEventBus()

	var mu sync.Mutex
	normalCount := 0

	// 一个会 panic 的 handler
	bus.Subscribe("test.event", func(event Event) {
		panic("模拟 panic")
	})

	// 一个正常 handler
	bus.Subscribe("test.event", func(event Event) {
		mu.Lock()
		normalCount++
		mu.Unlock()
	})

	// 发布事件，验证不崩溃
	done := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Publish 本身不应该 panic, 得到: %v", r)
			}
			close(done)
		}()
		bus.Publish(NewEvent("test.event", nil))
	}()

	select {
	case <-done:
		// 正常
	case <-time.After(time.Second):
		t.Fatal("Publish 超时")
	}

	// 等待异步 handler 执行完毕
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	if normalCount != 1 {
		t.Errorf("正常 handler 应执行 1 次，得到 %d", normalCount)
	}
	mu.Unlock()
}
