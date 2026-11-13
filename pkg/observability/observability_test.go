package observability

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ===== Logger 测试 =====

func TestLogger_NewLogger(t *testing.T) {
	logger := NewLogger(LogLevelInfo, nil)
	if logger == nil {
		t.Fatal("NewLogger 不应返回 nil")
	}
	if logger.level != LogLevelInfo {
		t.Errorf("日志级别 = %d, 期望 %d", logger.level, LogLevelInfo)
	}
}

func TestLogger_LogLevelParsing(t *testing.T) {
	tests := []struct {
		input string
		want  LogLevel
	}{
		{"debug", LogLevelDebug},
		{"info", LogLevelInfo},
		{"warn", LogLevelWarn},
		{"error", LogLevelError},
		{"unknown", LogLevelInfo},
		{"", LogLevelInfo},
		{"INFO", LogLevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseLogLevel(tt.input)
			if got != tt.want {
				t.Errorf("ParseLogLevel(%q) = %d, 期望 %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestLogger_Debug(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(LogLevelDebug, &buf)
	logger.Debug("调试消息")

	var entry logEntry
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry); err != nil {
		t.Fatalf("JSON 解析失败: %v", err)
	}
	if entry.Level != "debug" {
		t.Errorf("级别 = %q, 期望 %q", entry.Level, "debug")
	}
	if entry.Message != "调试消息" {
		t.Errorf("消息 = %q, 期望 %q", entry.Message, "调试消息")
	}
}

func TestLogger_Info(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(LogLevelDebug, &buf)
	logger.Info("信息消息")

	var entry logEntry
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry); err != nil {
		t.Fatalf("JSON 解析失败: %v", err)
	}
	if entry.Level != "info" {
		t.Errorf("级别 = %q, 期望 %q", entry.Level, "info")
	}
	if entry.Message != "信息消息" {
		t.Errorf("消息 = %q, 期望 %q", entry.Message, "信息消息")
	}
}

func TestLogger_Warn(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(LogLevelDebug, &buf)
	logger.Warn("警告消息")

	var entry logEntry
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry); err != nil {
		t.Fatalf("JSON 解析失败: %v", err)
	}
	if entry.Level != "warn" {
		t.Errorf("级别 = %q, 期望 %q", entry.Level, "warn")
	}
}

func TestLogger_Error(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(LogLevelDebug, &buf)
	logger.Error("错误消息")

	var entry logEntry
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry); err != nil {
		t.Fatalf("JSON 解析失败: %v", err)
	}
	if entry.Level != "error" {
		t.Errorf("级别 = %q, 期望 %q", entry.Level, "error")
	}
}

func TestLogger_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(LogLevelDebug, &buf)
	logger.Info("测试JSON格式")

	var entry map[string]interface{}
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry); err != nil {
		t.Fatalf("JSON 解析失败: %v", err)
	}

	// 验证 timestamp 字段（ISO 8601 格式）
	ts, ok := entry["timestamp"].(string)
	if !ok {
		t.Fatal("缺少 timestamp 字段")
	}
	if _, err := time.Parse(time.RFC3339Nano, ts); err != nil {
		t.Errorf("timestamp 格式不是 RFC3339Nano: %v", err)
	}

	// 验证 level 字段
	if entry["level"] != "info" {
		t.Errorf("level = %v, 期望 'info'", entry["level"])
	}

	// 验证 message 字段
	if entry["message"] != "测试JSON格式" {
		t.Errorf("message = %v, 期望 '测试JSON格式'", entry["message"])
	}
}

func TestLogger_WithFields(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(LogLevelDebug, &buf)
	logger.Info("带字段的日志", map[string]interface{}{
		"agent_id": "agent-1",
		"tool":     "read_file",
		"duration": 1.5,
	})

	var entry logEntry
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry); err != nil {
		t.Fatalf("JSON 解析失败: %v", err)
	}

	if entry.Fields == nil {
		t.Fatal("Fields 不应为 nil")
	}
	if entry.Fields["agent_id"] != "agent-1" {
		t.Errorf("agent_id = %v, 期望 'agent-1'", entry.Fields["agent_id"])
	}
	if entry.Fields["tool"] != "read_file" {
		t.Errorf("tool = %v, 期望 'read_file'", entry.Fields["tool"])
	}
}

func TestLogger_LevelFiltering(t *testing.T) {
	tests := []struct {
		name      string
		setLevel  LogLevel
		logLevel  LogLevel
		shouldLog bool
	}{
		{"Info 级别记录 Info", LogLevelInfo, LogLevelInfo, true},
		{"Info 级别记录 Warn", LogLevelInfo, LogLevelWarn, true},
		{"Info 级别记录 Error", LogLevelInfo, LogLevelError, true},
		{"Info 级别不记录 Debug", LogLevelInfo, LogLevelDebug, false},
		{"Debug 级别记录 Debug", LogLevelDebug, LogLevelDebug, true},
		{"Warn 级别不记录 Info", LogLevelWarn, LogLevelInfo, false},
		{"Error 级别不记录 Warn", LogLevelError, LogLevelWarn, false},
		{"Error 级别记录 Error", LogLevelError, LogLevelError, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := NewLogger(tt.setLevel, &buf)
			logger.Log(tt.logLevel, "测试消息", nil)

			if tt.shouldLog && buf.Len() == 0 {
				t.Error("应该有日志输出，但缓冲区为空")
			}
			if !tt.shouldLog && buf.Len() > 0 {
				t.Error("不应该有日志输出，但缓冲区有内容")
			}
		})
	}
}

func TestLogger_SetLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(LogLevelError, &buf)

	// Error 级别，Debug 不应输出
	logger.Debug("不应出现")
	if buf.Len() > 0 {
		t.Error("Error 级别时 Debug 不应输出")
	}

	// 改为 debug 级别
	logger.SetLevel(LogLevelDebug)
	logger.Debug("应出现")
	if buf.Len() == 0 {
		t.Error("Debug 级别时 Debug 应输出")
	}
}

func TestLogger_SetOutput(t *testing.T) {
	var buf1, buf2 bytes.Buffer
	logger := NewLogger(LogLevelDebug, &buf1)

	logger.Info("输出到 buf1")
	if buf1.Len() == 0 {
		t.Error("buf1 应有内容")
	}

	logger.SetOutput(&buf2)
	logger.Info("输出到 buf2")
	if buf2.Len() == 0 {
		t.Error("buf2 应有内容")
	}
}

func TestLogger_LogEntry_NoFields(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(LogLevelDebug, &buf)
	logger.Info("无字段消息")

	var entry logEntry
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry); err != nil {
		t.Fatalf("JSON 解析失败: %v", err)
	}
	// 无字段时 Fields 应为 nil（omitempty），而不是空 map
	if entry.Fields != nil {
		t.Error("无字段时 Fields 应为 nil")
	}
}

func TestLogger_MultipleFieldMaps(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(LogLevelDebug, &buf)
	logger.Info("多 map 字段",
		map[string]interface{}{"key1": "val1"},
		map[string]interface{}{"key2": "val2"},
	)

	var entry logEntry
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry); err != nil {
		t.Fatalf("JSON 解析失败: %v", err)
	}
	if entry.Fields["key1"] != "val1" {
		t.Errorf("key1 = %v, 期望 'val1'", entry.Fields["key1"])
	}
	if entry.Fields["key2"] != "val2" {
		t.Errorf("key2 = %v, 期望 'val2'", entry.Fields["key2"])
	}
}

func TestLogger_TrailingNewline(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(LogLevelDebug, &buf)
	logger.Info("test")

	output := buf.String()
	if !strings.HasSuffix(output, "\n") {
		t.Error("日志输出应以换行符结尾")
	}
}

// ===== CostTracker 测试 =====

func TestCostTracker_RecordCall(t *testing.T) {
	ct := NewCostTracker()
	ct.RecordCall("gpt-4o", 100, 50, 0.00075)

	total := ct.GetTotalCost()
	if total <= 0 {
		t.Errorf("GetTotalCost = %f, 期望正值", total)
	}
}

func TestCostTracker_GetCallCost(t *testing.T) {
	ct := NewCostTracker()

	tests := []struct {
		name         string
		model        string
		inputTokens  int
		outputTokens int
		expected     float64
	}{
		{"gpt-4o (100 in, 50 out)", "gpt-4o", 100, 50, 0.0008},
		{"gpt-4o-mini (1000 in, 500 out)", "gpt-4o-mini", 1000, 500, 0.0005},
		{"deepseek-v3 (500 in, 200 out)", "deepseek-v3", 500, 200, 0.0004},
		{"未知模型使用默认定价", "unknown-model", 1000, 500, 0.0015},
		{"零 token 零成本", "gpt-4o", 0, 0, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ct.GetCallCost(tt.model, tt.inputTokens, tt.outputTokens)
			if got != tt.expected {
				t.Errorf("GetCallCost(%q, %d, %d) = %f, 期望 %f",
					tt.model, tt.inputTokens, tt.outputTokens, got, tt.expected)
			}
		})
	}
}

func TestCostTracker_GetTotalCost_MultipleCalls(t *testing.T) {
	ct := NewCostTracker()

	ct.RecordCall("gpt-4o", 100, 50, 0.00075)
	ct.RecordCall("gpt-4o-mini", 1000, 500, 0.00045)
	ct.RecordCall("gpt-4o", 200, 100, 0.00150)

	total := ct.GetTotalCost()
	expected := 0.00075 + 0.00045 + 0.00150
	if total != expected {
		t.Errorf("GetTotalCost = %f, 期望 %f", total, expected)
	}
}

func TestCostTracker_GetCostByModel(t *testing.T) {
	ct := NewCostTracker()

	ct.RecordCall("gpt-4o", 100, 50, 0.00075)
	ct.RecordCall("gpt-4o-mini", 1000, 500, 0.00045)
	ct.RecordCall("gpt-4o", 200, 100, 0.00150)

	cost4o := ct.GetCostByModel("gpt-4o")
	expected4o := 0.0023
	if cost4o != expected4o {
		t.Errorf("GetCostByModel('gpt-4o') = %f, 期望 %f", cost4o, expected4o)
	}

	costMini := ct.GetCostByModel("gpt-4o-mini")
	if costMini != 0.0005 {
		t.Errorf("GetCostByModel('gpt-4o-mini') = %f, 期望 %f", costMini, 0.0005)
	}

	// 不存在的模型
	costUnknown := ct.GetCostByModel("nonexistent")
	if costUnknown != 0 {
		t.Errorf("不存在的模型应返回 0, 得到 %f", costUnknown)
	}
}

func TestCostTracker_Reset(t *testing.T) {
	ct := NewCostTracker()
	ct.RecordCall("gpt-4o", 100, 50, 0.00075)

	ct.Reset()

	if ct.GetTotalCost() != 0 {
		t.Error("Reset 后总成本应为 0")
	}
	if ct.GetCostByModel("gpt-4o") != 0 {
		t.Error("Reset 后模型成本应为 0")
	}
}

func TestCostTracker_ConcurrentSafety(t *testing.T) {
	ct := NewCostTracker()
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ct.RecordCall("gpt-4o", 10, 5, 0.0001)
			ct.GetTotalCost()
			ct.GetCostByModel("gpt-4o")
		}()
	}
	wg.Wait()

	total := ct.GetTotalCost()
	if total != 0.002 {
		t.Errorf("20 次调用后总成本 = %f, 期望 %f", total, 0.002)
	}
}

// ===== TokenTracker 测试 =====

func TestTokenTracker_RecordUsage(t *testing.T) {
	tt := NewTokenTracker()
	tt.RecordUsage("gpt-4o", 100, 50)

	input, output, total := tt.GetTotalUsage()
	if input != 100 {
		t.Errorf("Input = %d, 期望 %d", input, 100)
	}
	if output != 50 {
		t.Errorf("Output = %d, 期望 %d", output, 50)
	}
	if total != 150 {
		t.Errorf("Total = %d, 期望 %d", total, 150)
	}
}

func TestTokenTracker_GetTotalUsage_MultipleCalls(t *testing.T) {
	tt := NewTokenTracker()

	tt.RecordUsage("gpt-4o", 100, 50)
	tt.RecordUsage("gpt-4o-mini", 200, 100)
	tt.RecordUsage("gpt-4o", 150, 75)

	input, output, total := tt.GetTotalUsage()
	if input != 450 {
		t.Errorf("Input = %d, 期望 %d", input, 450)
	}
	if output != 225 {
		t.Errorf("Output = %d, 期望 %d", output, 225)
	}
	if total != 675 {
		t.Errorf("Total = %d, 期望 %d", total, 675)
	}
}

func TestTokenTracker_GetUsageByModel(t *testing.T) {
	tt := NewTokenTracker()

	tt.RecordUsage("gpt-4o", 100, 50)
	tt.RecordUsage("gpt-4o-mini", 200, 100)
	tt.RecordUsage("gpt-4o", 150, 75)

	input, output, total := tt.GetUsageByModel("gpt-4o")
	if input != 250 {
		t.Errorf("Input = %d, 期望 %d", input, 250)
	}
	if output != 125 {
		t.Errorf("Output = %d, 期望 %d", output, 125)
	}
	if total != 375 {
		t.Errorf("Total = %d, 期望 %d", total, 375)
	}

	// 不存在的模型
	input, output, total = tt.GetUsageByModel("nonexistent")
	if input != 0 || output != 0 || total != 0 {
		t.Errorf("不存在的模型应返回 0, 得到 (%d, %d, %d)", input, output, total)
	}
}

func TestTokenTracker_Reset(t *testing.T) {
	tt := NewTokenTracker()
	tt.RecordUsage("gpt-4o", 100, 50)

	tt.Reset()

	input, output, total := tt.GetTotalUsage()
	if input != 0 || output != 0 || total != 0 {
		t.Errorf("Reset 后应全部为 0, 得到 (%d, %d, %d)", input, output, total)
	}
}

func TestTokenTracker_ConcurrentSafety(t *testing.T) {
	tt := NewTokenTracker()
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tt.RecordUsage("gpt-4o", 10, 5)
			tt.GetTotalUsage()
			tt.GetUsageByModel("gpt-4o")
		}()
	}
	wg.Wait()

	_, _, total := tt.GetTotalUsage()
	if total != 150 {
		t.Errorf("10 次调用后 total = %d, 期望 %d", total, 150)
	}
}

// ===== EventBus 测试 =====

func TestEventBus_NewEventBus(t *testing.T) {
	bus := NewEventBus()
	if bus == nil {
		t.Fatal("NewEventBus 不应返回 nil")
	}
}

func TestEventBus_SubscribeAndPublish(t *testing.T) {
	bus := NewEventBus()
	received := make(chan Event, 1)

	bus.Subscribe("test.event", func(event Event) {
		received <- event
	})

	event := NewEvent("test.event", map[string]interface{}{"key": "value"})
	bus.Publish(event)

	select {
	case e := <-received:
		if e.Type != "test.event" {
			t.Errorf("事件类型 = %q, 期望 %q", e.Type, "test.event")
		}
		if e.Data["key"] != "value" {
			t.Errorf("Data[\"key\"] = %v, 期望 %v", e.Data["key"], "value")
		}
		if e.Timestamp.IsZero() {
			t.Error("Timestamp 不应为 zero")
		}
	case <-time.After(time.Second):
		t.Fatal("超时等待事件接收")
	}
}

func TestEventBus_MultipleSubscribers(t *testing.T) {
	bus := NewEventBus()
	var mu sync.Mutex
	count := 0

	// 两个订阅者订阅同一事件
	bus.Subscribe("test.event", func(event Event) {
		mu.Lock()
		count++
		mu.Unlock()
	})
	bus.Subscribe("test.event", func(event Event) {
		mu.Lock()
		count++
		mu.Unlock()
	})

	bus.Publish(NewEvent("test.event", nil))

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	if count != 2 {
		t.Errorf("两个订阅者应各接收一次, 得到 %d", count)
	}
	mu.Unlock()
}

func TestEventBus_DifferentEventTypes(t *testing.T) {
	bus := NewEventBus()
	received := make(chan string, 2)

	bus.Subscribe("type.a", func(event Event) {
		received <- "a"
	})
	bus.Subscribe("type.b", func(event Event) {
		received <- "b"
	})

	bus.Publish(NewEvent("type.a", nil))
	bus.Publish(NewEvent("type.b", nil))

	types := make(map[string]bool)
	for i := 0; i < 2; i++ {
		select {
		case typ := <-received:
			types[typ] = true
		case <-time.After(time.Second):
			t.Fatal("超时等待事件接收")
		}
	}

	if !types["a"] || !types["b"] {
		t.Errorf("应收到两种类型事件, 得到 %v", types)
	}
}

func TestEventBus_UnsubscribeAll(t *testing.T) {
	bus := NewEventBus()
	var count atomic.Int32
	bus.Subscribe("test.event", func(event Event) {
		count.Add(1)
	})

	bus.Publish(NewEvent("test.event", nil))
	time.Sleep(50 * time.Millisecond)

	bus.UnsubscribeAll("test.event")

	// 取消订阅后再发布
	bus.Publish(NewEvent("test.event", nil))
	time.Sleep(50 * time.Millisecond)

	if count.Load() != 1 {
		t.Errorf("取消订阅后不应再接收事件, 得到 %d", count.Load())
	}
}

func TestEventBus_NoSubscribers(t *testing.T) {
	bus := NewEventBus()
	// 无订阅者时不应 panic
	bus.Publish(NewEvent("nonexistent", nil))
}

func TestEventBus_AsyncProcessing(t *testing.T) {
	bus := NewEventBus()
	blocked := make(chan struct{})
	started := make(chan struct{})

	bus.Subscribe("test.event", func(event Event) {
		close(started)
		<-blocked // 阻塞直到测试完成
	})

	publishDone := make(chan struct{})
	go func() {
		bus.Publish(NewEvent("test.event", nil))
		close(publishDone)
	}()

	// Publish 应在 handler 完成前返回（异步）
	select {
	case <-publishDone:
		// 正常：Publish 不阻塞
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Publish 应异步返回，不应阻塞")
	}

	close(blocked) // 释放 handler
}

func TestEventBus_EventTimestampInUTC(t *testing.T) {
	event := NewEvent("test.event", nil)
	// 检查时区是否为 UTC
	_, offset := event.Timestamp.Zone()
	if offset != 0 {
		t.Errorf("Timestamp 应为 UTC, 时区偏移 = %d", offset)
	}
}

// TestEventBus_HandlerPanic 验证 handler panic 不会崩溃进程，且其他 handler 不受影响
func TestEventBus_HandlerPanic(t *testing.T) {
	bus := NewEventBus()
	var wg sync.WaitGroup
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
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Publish 本身不应该 panic, 得到: %v", r)
			}
		}()
		bus.Publish(NewEvent("test.event", nil))
	}()

	wg.Wait()

	// 等待异步 handler 执行完毕
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	if normalCount != 1 {
		t.Errorf("正常 handler 应执行 1 次，得到 %d", normalCount)
	}
	mu.Unlock()
}

// TestEventBus_MultiplePanicHandlers 验证多个 handler panic 时进程仍然正常运行
func TestEventBus_MultiplePanicHandlers(t *testing.T) {
	bus := NewEventBus()

	// 三个 handler，全部会 panic
	for i := 0; i < 3; i++ {
		i := i
		bus.Subscribe("test.event", func(event Event) {
			panic(fmt.Sprintf("handler %d panic", i))
		})
	}

	// 发布事件不应崩溃
	done := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Publish 不应 panic: %v", r)
			}
			close(done)
		}()
		bus.Publish(NewEvent("test.event", nil))
	}()

	select {
	case <-done:
		// 正常返回
	case <-time.After(time.Second):
		t.Fatal("Publish 超时")
	}
}

// ===== Subscriber 测试 =====

func TestLogSubscriber_EventTypes(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(LogLevelDebug, &buf)
	sub := NewLogSubscriber(logger)

	types := sub.EventTypes()
	expectedTypes := []string{
		EventAgentStart,
		EventAgentEnd,
		EventAgentError,
		EventToolStart,
		EventToolEnd,
		EventToolError,
		EventLLMCallStart,
		EventLLMCallEnd,
	}

	if len(types) != len(expectedTypes) {
		t.Fatalf("事件类型数量 = %d, 期望 %d", len(types), len(expectedTypes))
	}
	for i, et := range types {
		if et != expectedTypes[i] {
			t.Errorf("事件类型[%d] = %q, 期望 %q", i, et, expectedTypes[i])
		}
	}
}

func TestLogSubscriber_Handle(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(LogLevelDebug, &buf)
	sub := NewLogSubscriber(logger)

	event := NewEvent(EventAgentStart, map[string]interface{}{
		"agent_id": "agent-1",
	})
	err := sub.Handle(event)
	if err != nil {
		t.Fatalf("Handle 失败: %v", err)
	}

	var entry logEntry
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry); err != nil {
		t.Fatalf("JSON 解析失败: %v", err)
	}
	if entry.Fields == nil {
		t.Fatal("Fields 不应为 nil")
	}
	if entry.Fields["event_type"] != EventAgentStart {
		t.Errorf("event_type = %v, 期望 %q", entry.Fields["event_type"], EventAgentStart)
	}
	if entry.Fields["agent_id"] != "agent-1" {
		t.Errorf("agent_id = %v, 期望 'agent-1'", entry.Fields["agent_id"])
	}
}

func TestMetricsSubscriber_EventTypes(t *testing.T) {
	ct := NewCostTracker()
	tt := NewTokenTracker()
	sub := NewMetricsSubscriber(ct, tt)

	types := sub.EventTypes()
	if len(types) != 1 || types[0] != EventLLMCallEnd {
		t.Errorf("EventTypes = %v, 期望 [%q]", types, EventLLMCallEnd)
	}
}

func TestMetricsSubscriber_Handle_LLMCallEnd(t *testing.T) {
	ct := NewCostTracker()
	tt := NewTokenTracker()
	sub := NewMetricsSubscriber(ct, tt)

	event := NewEvent(EventLLMCallEnd, map[string]interface{}{
		"model":         "gpt-4o",
		"input_tokens":  100,
		"output_tokens": 50,
		"cost":          0.00075,
	})

	err := sub.Handle(event)
	if err != nil {
		t.Fatalf("Handle 失败: %v", err)
	}

	// 验证 token 追踪
	input, output, total := tt.GetTotalUsage()
	if input != 100 {
		t.Errorf("Input = %d, 期望 %d", input, 100)
	}
	if output != 50 {
		t.Errorf("Output = %d, 期望 %d", output, 50)
	}
	if total != 150 {
		t.Errorf("Total = %d, 期望 %d", total, 150)
	}

	// 验证成本追踪
	cost := ct.GetTotalCost()
	if cost <= 0 {
		t.Errorf("Cost = %f, 期望正值", cost)
	}
}

func TestMetricsSubscriber_Handle_NoExplicitCost(t *testing.T) {
	ct := NewCostTracker()
	tt := NewTokenTracker()
	sub := NewMetricsSubscriber(ct, tt)

	// 不传 cost，应自动计算
	event := NewEvent(EventLLMCallEnd, map[string]interface{}{
		"model":         "gpt-4o",
		"input_tokens":  100,
		"output_tokens": 50,
	})

	err := sub.Handle(event)
	if err != nil {
		t.Fatalf("Handle 失败: %v", err)
	}

	cost := ct.GetTotalCost()
	if cost <= 0 {
		t.Errorf("Cost = %f, 期望正值", cost)
	}
}

func TestMetricsSubscriber_Handle_OtherEvents(t *testing.T) {
	ct := NewCostTracker()
	tt := NewTokenTracker()
	sub := NewMetricsSubscriber(ct, tt)

	// 非 LLMCallEnd 事件不应触发追踪
	event := NewEvent(EventAgentStart, nil)
	err := sub.Handle(event)
	if err != nil {
		t.Fatalf("Handle 失败: %v", err)
	}

	cost := ct.GetTotalCost()
	if cost != 0 {
		t.Error("非 LLMCallEnd 事件不应更新成本")
	}
}

func TestMetricsSubscriber_Handle_ModelTracking(t *testing.T) {
	ct := NewCostTracker()
	tt := NewTokenTracker()
	sub := NewMetricsSubscriber(ct, tt)

	// 两个不同模型的调用
	sub.Handle(NewEvent(EventLLMCallEnd, map[string]interface{}{
		"model":         "gpt-4o",
		"input_tokens":  100,
		"output_tokens": 50,
		"cost":          0.00075,
	}))
	sub.Handle(NewEvent(EventLLMCallEnd, map[string]interface{}{
		"model":         "gpt-4o-mini",
		"input_tokens":  1000,
		"output_tokens": 500,
		"cost":          0.00045,
	}))

	// 按模型查询
	cost4o := ct.GetCostByModel("gpt-4o")
	costMini := ct.GetCostByModel("gpt-4o-mini")
	if cost4o <= 0 {
		t.Errorf("gpt-4o 成本 = %f, 期望正值", cost4o)
	}
	if costMini <= 0 {
		t.Errorf("gpt-4o-mini 成本 = %f, 期望正值", costMini)
	}

	input4o, output4o, _ := tt.GetUsageByModel("gpt-4o")
	if input4o != 100 || output4o != 50 {
		t.Errorf("gpt-4o usage = (%d, %d), 期望 (100, 50)", input4o, output4o)
	}
}

// ===== SetupSubscribers 测试 =====

func TestSetupSubscribers_Disabled(t *testing.T) {
	cfg := Config{Enabled: false}
	bus := NewEventBus()
	logger := NewLogger(LogLevelInfo, nil)
	ct := NewCostTracker()
	tt := NewTokenTracker()

	// 禁用时不应 panic
	SetupSubscribers(cfg, bus, logger, ct, tt)
}

func TestSetupSubscribers_Enabled(t *testing.T) {
	cfg := Config{Enabled: true, LogLevel: "debug"}
	bus := NewEventBus()
	logger := NewLogger(LogLevelInfo, nil)
	ct := NewCostTracker()
	tt := NewTokenTracker()

	SetupSubscribers(cfg, bus, logger, ct, tt)

	// Logger 级别应更新
	if logger.level != LogLevelDebug {
		t.Errorf("Logger 级别 = %d, 期望 %d", logger.level, LogLevelDebug)
	}
}

// ===== Event 类型常量测试 =====

func TestEventConstants(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"EventAgentStart", EventAgentStart},
		{"EventAgentEnd", EventAgentEnd},
		{"EventAgentError", EventAgentError},
		{"EventToolStart", EventToolStart},
		{"EventToolEnd", EventToolEnd},
		{"EventToolError", EventToolError},
		{"EventLLMCallStart", EventLLMCallStart},
		{"EventLLMCallEnd", EventLLMCallEnd},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value == "" {
				t.Errorf("%s 应为非空字符串", tt.name)
			}
		})
	}
}

func TestNewEvent(t *testing.T) {
	event := NewEvent(EventAgentStart, map[string]interface{}{
		"agent_id": "agent-1",
		"time":     "2024-01-01",
	})

	if event.Type != EventAgentStart {
		t.Errorf("Type = %q, 期望 %q", event.Type, EventAgentStart)
	}
	if event.Data["agent_id"] != "agent-1" {
		t.Errorf("Data[\"agent_id\"] = %v, 期望 'agent-1'", event.Data["agent_id"])
	}
	if event.Timestamp.IsZero() {
		t.Error("Timestamp 不应为 zero")
	}
}

// ===== Subscriber 接口兼容性测试 =====

// TestSubscriberInterface 验证 LogSubscriber 和 MetricsSubscriber 实现 Subscriber 接口
func TestSubscriberInterface(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(LogLevelDebug, &buf)
	ct := NewCostTracker()
	tt := NewTokenTracker()

	// 编译时接口检查
	var logSub Subscriber = NewLogSubscriber(logger)
	var metricsSub Subscriber = NewMetricsSubscriber(ct, tt)

	_ = logSub
	_ = metricsSub
}

// ===== Subscriber 错误隔离测试 =====

func TestSubscriber_ErrorIsolation(t *testing.T) {
	bus := NewEventBus()
	var buf bytes.Buffer
	logger := NewLogger(LogLevelDebug, &buf)

	// 一个会记录事件的订阅者
	normalReceived := make(chan Event, 1)
	bus.Subscribe("test.event", func(event Event) {
		normalReceived <- event
	})

	// 发布事件，正常订阅者应能收到
	bus.Publish(NewEvent("test.event", nil))
	select {
	case <-normalReceived:
		// 正常收到
	case <-time.After(time.Second):
		t.Fatal("正常订阅者应收到事件")
	}

	_ = logger // 避免未使用变量错误
}

// ===== CostTracker + TokenTracker 集成测试 =====

func TestCostAndTokenTracking_Integration(t *testing.T) {
	ct := NewCostTracker()
	tt := NewTokenTracker()

	calls := []struct {
		model        string
		inputTokens  int
		outputTokens int
		cost         float64
	}{
		{"gpt-4o", 150, 80, 0.001175},
		{"gpt-4o-mini", 2000, 1000, 0.00090},
		{"gpt-4o", 50, 20, 0.000325},
	}

	for _, c := range calls {
		ct.RecordCall(c.model, c.inputTokens, c.outputTokens, c.cost)
		tt.RecordUsage(c.model, c.inputTokens, c.outputTokens)
	}

	// 验证总成本
	totalCost := ct.GetTotalCost()
	if totalCost <= 0 {
		t.Errorf("总成本 = %f, 期望正值", totalCost)
	}

	// 验证总 token
	input, output, total := tt.GetTotalUsage()
	if input != 2200 {
		t.Errorf("总输入 = %d, 期望 %d", input, 2200)
	}
	if output != 1100 {
		t.Errorf("总输出 = %d, 期望 %d", output, 1100)
	}
	if total != 3300 {
		t.Errorf("总 token = %d, 期望 %d", total, 3300)
	}

	// 验证按模型查询
	cost4o := ct.GetCostByModel("gpt-4o")
	if cost4o <= 0 {
		t.Errorf("gpt-4o 成本 = %f, 期望正值", cost4o)
	}

	input4o, output4o, _ := tt.GetUsageByModel("gpt-4o")
	if input4o != 200 || output4o != 100 {
		t.Errorf("gpt-4o usage = (%d, %d), 期望 (200, 100)", input4o, output4o)
	}
}

// ===== 工具函数测试 =====

func TestMergeFields_Nil(t *testing.T) {
	result := mergeFields()
	if result != nil {
		t.Error("无参数时应返回 nil")
	}
}

func TestMergeFields_Single(t *testing.T) {
	result := mergeFields(map[string]interface{}{"key": "val"})
	if result["key"] != "val" {
		t.Errorf("key = %v, 期望 'val'", result["key"])
	}
}

func TestMergeFields_Multiple(t *testing.T) {
	result := mergeFields(
		map[string]interface{}{"a": "1", "b": "2"},
		map[string]interface{}{"b": "3", "c": "4"},
	)
	if result["a"] != "1" {
		t.Errorf("a = %v, 期望 '1'", result["a"])
	}
	if result["b"] != "3" {
		t.Errorf("b (覆盖后) = %v, 期望 '3'", result["b"])
	}
	if result["c"] != "4" {
		t.Errorf("c = %v, 期望 '4'", result["c"])
	}
}
