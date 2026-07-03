package loopdetect

import (
	"errors"
	"strings"
	"testing"

	"github.com/wly2lcl/basework/pkg/llm"
)

// 辅助函数：快速构建 ChatMessage
func assistantMsg(text string) llm.ChatMessage {
	return llm.ChatMessage{
		Role: llm.RoleAssistant,
		Content: []llm.ContentPart{
			{Type: llm.ContentTypeText, Text: text},
		},
	}
}

func userMsg(text string) llm.ChatMessage {
	return llm.ChatMessage{
		Role: llm.RoleUser,
		Content: []llm.ContentPart{
			{Type: llm.ContentTypeText, Text: text},
		},
	}
}

// ---------- CheckRepeatedContent 测试 ----------

func TestCheckRepeatedContent_检测重复(t *testing.T) {
	msgs := []llm.ChatMessage{
		assistantMsg("Hello"),
		assistantMsg("Hello"),
		assistantMsg("Hello"),
	}
	loop, count := CheckRepeatedContent(msgs, 3)
	if !loop {
		t.Error("连续3次相同内容应检测为循环")
	}
	if count < 3 {
		t.Errorf("重复次数期望 >= 3, 得到 %d", count)
	}
}

func TestCheckRepeatedContent_未达阈值(t *testing.T) {
	// 只有2次重复，阈值3
	msgs := []llm.ChatMessage{
		assistantMsg("Hello"),
		assistantMsg("Hello"),
	}
	loop, count := CheckRepeatedContent(msgs, 3)
	if loop {
		t.Error("2次重复未达阈值3，不应检测为循环")
	}
	if count != 0 {
		t.Errorf("不足阈值时 count 应为0, 得到 %d", count)
	}
}

func TestCheckRepeatedContent_自定义阈值(t *testing.T) {
	msgs := []llm.ChatMessage{
		assistantMsg("Hi"),
		assistantMsg("Hi"),
	}
	loop, _ := CheckRepeatedContent(msgs, 2)
	if !loop {
		t.Error("阈值设为2时，2次重复应检测为循环")
	}
}

func TestCheckRepeatedContent_忽略非assistant消息(t *testing.T) {
	msgs := []llm.ChatMessage{
		userMsg("Hello"),
		assistantMsg("Hello"),
		userMsg("Hello"),
		assistantMsg("Hello"),
		assistantMsg("Hello"),
	}
	// assistant 消息只有3条连续的 "Hello"
	loop, count := CheckRepeatedContent(msgs, 3)
	if !loop {
		t.Error("应忽略非assistant消息，连续3条assistant消息重复应检测为循环")
	}
	if count < 3 {
		t.Errorf("重复次数期望 >= 3, 得到 %d", count)
	}
}

func TestCheckRepeatedContent_不同内容非循环(t *testing.T) {
	msgs := []llm.ChatMessage{
		assistantMsg("内容A"),
		assistantMsg("内容B"),
		assistantMsg("内容C"),
	}
	loop, _ := CheckRepeatedContent(msgs, 3)
	if loop {
		t.Error("不同内容不应检测为循环")
	}
}

func TestCheckRepeatedContent_忽略空白差异(t *testing.T) {
	msgs := []llm.ChatMessage{
		assistantMsg("  Hello   World  "),
		assistantMsg("Hello World"),
		assistantMsg("  Hello  World  "),
	}
	loop, count := CheckRepeatedContent(msgs, 3)
	if !loop {
		t.Error("仅空白差异的内容应视为相同")
	}
	if count < 3 {
		t.Errorf("重复次数期望 >= 3, 得到 %d", count)
	}
}

func TestCheckRepeatedContent_阈值设为0使用默认值(t *testing.T) {
	msgs := []llm.ChatMessage{
		assistantMsg("X"),
		assistantMsg("X"),
		assistantMsg("X"),
	}
	loop, _ := CheckRepeatedContent(msgs, 0) // 使用默认阈值3
	if !loop {
		t.Error("阈值0应使用默认值3，3次重复应检测为循环")
	}
}

func TestCheckRepeatedContent_消息不足阈值(t *testing.T) {
	msgs := []llm.ChatMessage{
		assistantMsg("A"),
	}
	loop, count := CheckRepeatedContent(msgs, 3)
	if loop {
		t.Error("只有1条消息时不应检测为循环")
	}
	if count != 0 {
		t.Errorf("消息不足阈值时 count 应为0, 得到 %d", count)
	}
}

// ---------- CheckToolLoop 测试 ----------

func TestCheckToolLoop_检测重复(t *testing.T) {
	calls := []ToolCall{
		{ToolName: "read_file", Args: map[string]interface{}{"path": "test.txt"}},
		{ToolName: "read_file", Args: map[string]interface{}{"path": "test.txt"}},
		{ToolName: "read_file", Args: map[string]interface{}{"path": "test.txt"}},
		{ToolName: "read_file", Args: map[string]interface{}{"path": "test.txt"}},
		{ToolName: "read_file", Args: map[string]interface{}{"path": "test.txt"}},
	}
	loop, count := CheckToolLoop(calls, 5)
	if !loop {
		t.Error("连续5次相同工具调用应检测为循环")
	}
	if count < 5 {
		t.Errorf("重复次数期望 >= 5, 得到 %d", count)
	}
}

func TestCheckToolLoop_不同参数非循环(t *testing.T) {
	calls := []ToolCall{
		{ToolName: "read_file", Args: map[string]interface{}{"path": "a.txt"}},
		{ToolName: "read_file", Args: map[string]interface{}{"path": "b.txt"}},
		{ToolName: "read_file", Args: map[string]interface{}{"path": "c.txt"}},
		{ToolName: "read_file", Args: map[string]interface{}{"path": "d.txt"}},
		{ToolName: "read_file", Args: map[string]interface{}{"path": "e.txt"}},
	}
	loop, _ := CheckToolLoop(calls, 5)
	if loop {
		t.Error("不同参数不应检测为工具循环")
	}
}

func TestCheckToolLoop_不同工具名非循环(t *testing.T) {
	calls := []ToolCall{
		{ToolName: "read_file", Args: map[string]interface{}{"path": "x"}},
		{ToolName: "write_file", Args: map[string]interface{}{"path": "x"}},
		{ToolName: "delete_file", Args: map[string]interface{}{"path": "x"}},
	}
	loop, _ := CheckToolLoop(calls, 3)
	if loop {
		t.Error("不同工具名不应检测为循环")
	}
}

func TestCheckToolLoop_自定义阈值(t *testing.T) {
	calls := []ToolCall{
		{ToolName: "tool", Args: map[string]interface{}{"x": "1"}},
		{ToolName: "tool", Args: map[string]interface{}{"x": "1"}},
		{ToolName: "tool", Args: map[string]interface{}{"x": "1"}},
	}
	loop, _ := CheckToolLoop(calls, 3)
	if !loop {
		t.Error("阈值3时3次重复应检测为循环")
	}
}

func TestCheckToolLoop_阈值0使用默认值(t *testing.T) {
	calls := []ToolCall{
		{ToolName: "tool", Args: nil},
		{ToolName: "tool", Args: nil},
		{ToolName: "tool", Args: nil},
		{ToolName: "tool", Args: nil},
		{ToolName: "tool", Args: nil},
	}
	loop, _ := CheckToolLoop(calls, 0) // 使用默认阈值5
	if !loop {
		t.Error("阈值0应使用默认值5，5次重复应检测为循环")
	}
}

func TestCheckToolLoop_调用次数不足阈值(t *testing.T) {
	calls := []ToolCall{
		{ToolName: "tool", Args: nil},
		{ToolName: "tool", Args: nil},
	}
	loop, count := CheckToolLoop(calls, 5)
	if loop {
		t.Error("调用次数不足阈值不应检测为循环")
	}
	if count != 0 {
		t.Errorf("不足阈值时 count 应为0, 得到 %d", count)
	}
}

func TestCheckToolLoop_参数顺序不影响比较(t *testing.T) {
	calls := []ToolCall{
		{ToolName: "tool", Args: map[string]interface{}{"a": "1", "b": "2"}},
		{ToolName: "tool", Args: map[string]interface{}{"b": "2", "a": "1"}},
		{ToolName: "tool", Args: map[string]interface{}{"a": "1", "b": "2"}},
		{ToolName: "tool", Args: map[string]interface{}{"b": "2", "a": "1"}},
		{ToolName: "tool", Args: map[string]interface{}{"a": "1", "b": "2"}},
	}
	loop, _ := CheckToolLoop(calls, 5)
	if !loop {
		t.Error("参数键顺序不同但值相同应视为相同")
	}
}

// ---------- CheckLoopPattern 测试 ----------

func TestCheckLoopPattern_默认模式匹配(t *testing.T) {
	tests := []struct {
		text       string
		wantMatch  bool
		wantPrefix string
	}{
		{"I'll try again", true, "I'll try again"},
		{"Let me retry the operation", true, "Let me retry"},
		{"Let me try again", true, "Let me try again"},
		{"让我再试一次", true, "让我再试一次"},
		{"我再次尝试连接", true, "我再次尝试"},
		{"正常内容，没有问题", false, ""},
		{"", false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.text[:min(len(tt.text), 20)], func(t *testing.T) {
			matched, pattern := CheckLoopPattern(tt.text, nil)
			if matched != tt.wantMatch {
				t.Errorf("CheckLoopPattern(%q) matched = %v, 期望 %v", tt.text, matched, tt.wantMatch)
			}
			if tt.wantMatch && !strings.Contains(pattern, tt.wantPrefix) {
				t.Errorf("匹配到的模式 %q 应包含 %q", pattern, tt.wantPrefix)
			}
		})
	}
}

func TestCheckLoopPattern_大小写不敏感(t *testing.T) {
	matched, pattern := CheckLoopPattern("I'LL TRY AGAIN", nil)
	if !matched {
		t.Error("大写 'I'LL TRY AGAIN' 也应匹配")
	}
	if pattern != "I'll try again" {
		t.Errorf("匹配到的模式应为原始大小写, 得到 %q", pattern)
	}
}

func TestCheckLoopPattern_自定义模式(t *testing.T) {
	customPatterns := []string{"custom_error", "another_pattern"}
	tests := []struct {
		text      string
		wantMatch bool
	}{
		{"custom_error occurred", true},
		{"another_pattern here", true},
		{"default pattern not match", false},
		{"I'll try again", false}, // 自定义模式时不用默认模式
	}
	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			matched, _ := CheckLoopPattern(tt.text, customPatterns)
			if matched != tt.wantMatch {
				t.Errorf("CheckLoopPattern(%q, custom) = %v, 期望 %v", tt.text, matched, tt.wantMatch)
			}
		})
	}
}

func TestCheckLoopPattern_空模式列表使用默认(t *testing.T) {
	matched, _ := CheckLoopPattern("I'll try again", []string{})
	if !matched {
		t.Error("空模式列表应使用默认模式")
	}
}

func TestCheckLoopPattern_空文本(t *testing.T) {
	matched, _ := CheckLoopPattern("", nil)
	if matched {
		t.Error("空文本不应匹配任何模式")
	}
}

// ---------- Fuse 测试 ----------

func TestFuse_无信号(t *testing.T) {
	cfg := FusionConfig{RepeatedThreshold: 3, ToolLoopThreshold: 5}
	result, err := Fuse(false, false, false, cfg)
	if err != nil {
		t.Fatalf("Fuse 不应返回错误: %v", err)
	}
	if result.IsLoop {
		t.Error("无信号时 IsLoop 应为 false")
	}
	if result.Confidence != 0 {
		t.Errorf("无信号时 Confidence 应为 0, 得到 %.1f", result.Confidence)
	}
}

func TestFuse_多信号高置信度(t *testing.T) {
	cfg := FusionConfig{RepeatedThreshold: 3, ToolLoopThreshold: 5}
	result, err := Fuse(true, true, false, cfg)
	if err != nil {
		t.Fatalf("Fuse 不应返回错误: %v", err)
	}
	if !result.IsLoop {
		t.Error("多信号时 IsLoop 应为 true")
	}
	if result.Confidence != 0.9 {
		t.Errorf("多信号置信度期望 0.9, 得到 %.1f", result.Confidence)
	}
	if result.Signal == "" {
		t.Error("多信号时 Signal 不应为空")
	}
}

func TestFuse_三信号(t *testing.T) {
	cfg := FusionConfig{RepeatedThreshold: 3, ToolLoopThreshold: 5}
	result, err := Fuse(true, true, true, cfg)
	if err != nil {
		t.Fatalf("Fuse 不应返回错误: %v", err)
	}
	if !result.IsLoop {
		t.Error("三信号时 IsLoop 应为 true")
	}
	if result.Confidence != 0.9 {
		t.Errorf("三信号置信度期望 0.9, 得到 %.1f", result.Confidence)
	}
}

func TestFuse_单信号中置信度(t *testing.T) {
	cfg := FusionConfig{RepeatedThreshold: 3, ToolLoopThreshold: 5}
	result, err := Fuse(true, false, false, cfg)
	if err != nil {
		t.Fatalf("Fuse 不应返回错误: %v", err)
	}
	if !result.IsLoop {
		t.Error("单信号时 IsLoop 应为 true")
	}
	if result.Confidence != 0.7 {
		t.Errorf("默认阈值下单信号置信度期望 0.7, 得到 %.1f", result.Confidence)
	}
}

func TestFuse_单信号模式匹配(t *testing.T) {
	cfg := FusionConfig{}
	result, err := Fuse(false, false, true, cfg)
	if err != nil {
		t.Fatalf("Fuse 不应返回错误: %v", err)
	}
	if !result.IsLoop {
		t.Error("模式匹配信号时 IsLoop 应为 true")
	}
	if result.Confidence != 0.7 {
		t.Errorf("模式匹配单信号置信度期望 0.7, 得到 %.1f", result.Confidence)
	}
}

func TestFuse_重复阈值等于高置信度阈值(t *testing.T) {
	cfg := FusionConfig{RepeatedThreshold: 6, ToolLoopThreshold: 5, HighConfidenceThreshold: 6}
	result, err := Fuse(true, false, false, cfg)
	if err != nil {
		t.Fatalf("Fuse 不应返回错误: %v", err)
	}
	if !result.IsLoop {
		t.Error("单信号 IsLoop 应为 true")
	}
	if result.Confidence != 0.85 {
		t.Errorf("阈值>=高置信度阈值时置信度期望 0.85, 得到 %.1f", result.Confidence)
	}
}

func TestFuse_工具循环单信号(t *testing.T) {
	cfg := FusionConfig{RepeatedThreshold: 3, ToolLoopThreshold: 4}
	result, err := Fuse(false, true, false, cfg)
	if err != nil {
		t.Fatalf("Fuse 不应返回错误: %v", err)
	}
	if !result.IsLoop {
		t.Error("单信号 IsLoop 应为 true")
	}
	if result.Signal != "工具循环检测（单信号）" {
		t.Errorf("Signal 期望 %q, 得到 %q", "工具循环检测（单信号）", result.Signal)
	}
}

// ---------- Response 测试 ----------

func TestParseResponseStrategy(t *testing.T) {
	tests := []struct {
		input   string
		want    ResponseStrategy
		wantErr bool
	}{
		{"warn", StrategyWarn, false},
		{"interrupt", StrategyInterrupt, false},
		{"prompt", StrategyPrompt, false},
		{"unknown", StrategyWarn, true},
		{"", StrategyWarn, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseResponseStrategy(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseResponseStrategy(%q) 错误 = %v, wantErr = %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ParseResponseStrategy(%q) = %v, 期望 %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestResponseStrategy_String(t *testing.T) {
	tests := []struct {
		s    ResponseStrategy
		want string
	}{
		{StrategyWarn, "warn"},
		{StrategyInterrupt, "interrupt"},
		{StrategyPrompt, "prompt"},
		{ResponseStrategy(99), "unknown(99)"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.s.String(); got != tt.want {
				t.Errorf("String() = %q, 期望 %q", got, tt.want)
			}
		})
	}
}

func TestHandleLoop_Warn(t *testing.T) {
	result := &DetectionResult{
		IsLoop:     true,
		Signal:     "测试信号",
		Confidence: 0.7,
		Details:    "测试详情",
	}
	err := HandleLoop(result, StrategyWarn)
	if err != nil {
		t.Errorf("Warn策略应返回 nil 错误, 得到 %v", err)
	}
}

func TestHandleLoop_Interrupt(t *testing.T) {
	result := &DetectionResult{
		IsLoop:     true,
		Signal:     "测试信号",
		Confidence: 0.9,
		Details:    "测试详情",
	}
	err := HandleLoop(result, StrategyInterrupt)
	if err == nil {
		t.Fatal("Interrupt策略应返回错误")
	}
	var loopErr *LoopInterruptError
	if !errors.As(err, &loopErr) {
		t.Errorf("错误应为 *LoopInterruptError 类型, 得到 %T", err)
	}
	if loopErr.Signal != "测试信号" {
		t.Errorf("LoopInterruptError.Signal = %q, 期望 %q", loopErr.Signal, "测试信号")
	}
}

func TestHandleLoop_Prompt无回调(t *testing.T) {
	result := &DetectionResult{
		IsLoop:     true,
		Signal:     "测试信号",
		Confidence: 0.7,
	}
	err := HandleLoop(result, StrategyPrompt)
	if err == nil {
		t.Error("无 PromptFunc 时 HandleLoop 应返回错误")
	}
}

func TestHandleLoop_NilResult(t *testing.T) {
	err := HandleLoop(nil, StrategyWarn)
	if err == nil {
		t.Error("nil result 时应返回错误")
	}
}

func TestHandleLoopWithPrompt_用户继续(t *testing.T) {
	result := &DetectionResult{
		IsLoop:     true,
		Signal:     "测试",
		Confidence: 0.7,
		Details:    "详情",
	}
	err := HandleLoopWithPrompt(result, func(detail string) (bool, error) {
		return true, nil // 用户选择继续
	})
	if err != nil {
		t.Errorf("用户继续时应返回 nil, 得到 %v", err)
	}
}

func TestHandleLoopWithPrompt_用户停止(t *testing.T) {
	result := &DetectionResult{
		IsLoop:     true,
		Signal:     "测试",
		Confidence: 0.9,
		Details:    "详情",
	}
	err := HandleLoopWithPrompt(result, func(detail string) (bool, error) {
		return false, nil // 用户选择停止
	})
	if err == nil {
		t.Fatal("用户停止时应返回错误")
	}
	var loopErr *LoopInterruptError
	if !errors.As(err, &loopErr) {
		t.Errorf("错误应为 *LoopInterruptError 类型")
	}
}

func TestHandleLoopWithPrompt_NilResult(t *testing.T) {
	err := HandleLoopWithPrompt(nil, func(detail string) (bool, error) {
		return true, nil
	})
	if err == nil {
		t.Error("nil result 时应返回错误")
	}
}

func TestHandleLoopWithPrompt_NilPromptFunc(t *testing.T) {
	err := HandleLoopWithPrompt(&DetectionResult{}, nil)
	if err == nil {
		t.Error("nil PromptFunc 时应返回错误")
	}
}

func TestLoopInterruptError_Error(t *testing.T) {
	err := &LoopInterruptError{
		Signal:     "测试信号",
		Confidence: 0.9,
		Details:    "测试详情",
	}
	msg := err.Error()
	if !strings.Contains(msg, "测试信号") {
		t.Errorf("Error() 应包含信号描述, 得到 %q", msg)
	}
}

// ---------- Detector.Check 完整流程测试 ----------

func TestDetector_Check_未启用(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = false
	d := NewDetector(cfg)

	result, err := d.Check(nil, nil)
	if err != nil {
		t.Fatalf("未启用时不应返回错误: %v", err)
	}
	if result.IsLoop {
		t.Error("未启用时 IsLoop 应为 false")
	}
}

func TestDetector_Check_重复内容检测(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.RepeatedThreshold = 3
	cfg.ResponseStrategy = "warn"
	d := NewDetector(cfg)

	msgs := []llm.ChatMessage{
		assistantMsg("重复内容"),
		assistantMsg("重复内容"),
		assistantMsg("重复内容"),
	}

	result, err := d.Check(msgs, nil)
	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}
	if !result.IsLoop {
		t.Error("重复内容应检测为循环")
	}
	if result.Confidence < 0.5 {
		t.Errorf("置信度应 >= 0.5, 得到 %.1f", result.Confidence)
	}
}

func TestDetector_Check_工具循环检测(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.ToolLoopThreshold = 3
	cfg.RepeatedThreshold = 10 // 避免重复内容误触发
	cfg.ResponseStrategy = "warn"
	d := NewDetector(cfg)

	calls := []ToolCall{
		{ToolName: "read", Args: map[string]interface{}{"x": "1"}},
		{ToolName: "read", Args: map[string]interface{}{"x": "1"}},
		{ToolName: "read", Args: map[string]interface{}{"x": "1"}},
	}

	result, err := d.Check(nil, calls)
	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}
	if !result.IsLoop {
		t.Error("工具循环应检测为循环")
	}
}

func TestDetector_Check_模式匹配检测(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.RepeatedThreshold = 10 // 避免重复内容误触发
	cfg.ToolLoopThreshold = 10
	cfg.ResponseStrategy = "warn"
	d := NewDetector(cfg)

	msgs := []llm.ChatMessage{
		assistantMsg("I'll try again"),
	}

	result, err := d.Check(msgs, nil)
	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}
	if !result.IsLoop {
		t.Error("循环模式应检测为循环")
	}
}

func TestDetector_Check_中断策略(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.RepeatedThreshold = 2
	cfg.ResponseStrategy = "interrupt"
	d := NewDetector(cfg)

	msgs := []llm.ChatMessage{
		assistantMsg("Oops"),
		assistantMsg("Oops"),
	}

	_, err := d.Check(msgs, nil)
	if err == nil {
		t.Error("Interrupt 策略应返回错误")
	}
	var loopErr *LoopInterruptError
	if !errors.As(err, &loopErr) {
		t.Errorf("错误应为 *LoopInterruptError, 得到 %T", err)
	}
}

func TestDetector_Check_Prompt策略用户继续(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.RepeatedThreshold = 2
	cfg.ResponseStrategy = "prompt"
	d := NewDetectorWithPrompt(cfg, func(detail string) (bool, error) {
		return true, nil // 用户继续
	})

	msgs := []llm.ChatMessage{
		assistantMsg("Loop"),
		assistantMsg("Loop"),
	}

	result, err := d.Check(msgs, nil)
	if err != nil {
		t.Fatalf("用户继续时不应返回错误: %v", err)
	}
	if !result.IsLoop {
		t.Error("仍应检测为循环")
	}
}

func TestDetector_Check_Prompt策略用户停止(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.RepeatedThreshold = 2
	cfg.ResponseStrategy = "prompt"
	d := NewDetectorWithPrompt(cfg, func(detail string) (bool, error) {
		return false, nil // 用户停止
	})

	msgs := []llm.ChatMessage{
		assistantMsg("Loop"),
		assistantMsg("Loop"),
	}

	_, err := d.Check(msgs, nil)
	if err == nil {
		t.Error("用户停止时应返回错误")
	}
}

func TestDetector_Check_多信号融合(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.RepeatedThreshold = 3
	cfg.ToolLoopThreshold = 3
	cfg.ResponseStrategy = "warn"
	d := NewDetector(cfg)

	msgs := []llm.ChatMessage{
		assistantMsg("内容"),
		assistantMsg("内容"),
		assistantMsg("内容"),
	}
	calls := []ToolCall{
		{ToolName: "tool", Args: map[string]interface{}{"x": "1"}},
		{ToolName: "tool", Args: map[string]interface{}{"x": "1"}},
		{ToolName: "tool", Args: map[string]interface{}{"x": "1"}},
	}

	result, err := d.Check(msgs, calls)
	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}
	if !result.IsLoop {
		t.Error("多信号应检测为循环")
	}
	if result.Confidence < 0.8 {
		t.Errorf("多信号置信度应高, 得到 %.1f", result.Confidence)
	}
	if result.Details == "" || result.Details == "未检测到循环" {
		t.Errorf("详情应包含具体信息, 得到 %q", result.Details)
	}
}

func TestDetector_Check_无循环(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.RepeatedThreshold = 3
	cfg.ResponseStrategy = "warn"
	d := NewDetector(cfg)

	msgs := []llm.ChatMessage{
		assistantMsg("内容A"),
		assistantMsg("内容B"),
		assistantMsg("内容C"),
	}

	result, err := d.Check(msgs, nil)
	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}
	if result.IsLoop {
		t.Error("无循环时 IsLoop 应为 false")
	}
}

func TestDetector_Check_自定义模式(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.ResponseStrategy = "warn"
	cfg.CustomPatterns = []string{"custom_error", "retry_limit"}
	cfg.RepeatedThreshold = 10
	cfg.ToolLoopThreshold = 10
	d := NewDetector(cfg)

	msgs := []llm.ChatMessage{
		assistantMsg("custom_error happened"),
	}

	result, err := d.Check(msgs, nil)
	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}
	if !result.IsLoop {
		t.Error("自定义模式应检测为循环")
	}
}

// ---------- 辅助函数测试 ----------

func TestGetLastAssistantText(t *testing.T) {
	msgs := []llm.ChatMessage{
		userMsg("你好"),
		assistantMsg("第一条助手消息"),
		userMsg("继续"),
		assistantMsg("最后一条助手消息"),
	}
	text := getLastAssistantText(msgs)
	if text != "最后一条助手消息" {
		t.Errorf("期望 %q, 得到 %q", "最后一条助手消息", text)
	}
}

func TestGetLastAssistantText_无assistant消息(t *testing.T) {
	msgs := []llm.ChatMessage{
		userMsg("只有用户消息"),
	}
	text := getLastAssistantText(msgs)
	if text != "" {
		t.Errorf("无assistant消息应返回空字符串, 得到 %q", text)
	}
}

func TestExtractText(t *testing.T) {
	parts := []llm.ContentPart{
		{Type: llm.ContentTypeImage, ImageURL: "img.png"},
		{Type: llm.ContentTypeText, Text: "文本内容"},
	}
	text := extractText(parts)
	if text != "文本内容" {
		t.Errorf("期望 %q, 得到 %q", "文本内容", text)
	}
}

func TestBuildDetails(t *testing.T) {
	tests := []struct {
		name   string
		parts  []string
		want   string
	}{
		{"全部为空", []string{"", ""}, "未检测到循环"},
		{"单个非空", []string{"", "重复内容", ""}, "重复内容"},
		{"多个非空", []string{"重复内容", "工具循环"}, "重复内容; 工具循环"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildDetails(tt.parts...)
			if got != tt.want {
				t.Errorf("buildDetails() = %q, 期望 %q", got, tt.want)
			}
		})
	}
}

// ---------- DefaultConfig 测试 ----------

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.Enabled {
		t.Error("默认应启用")
	}
	if cfg.RepeatedThreshold != 3 {
		t.Errorf("RepeatedThreshold 默认应为 3, 得到 %d", cfg.RepeatedThreshold)
	}
	if cfg.ToolLoopThreshold != 5 {
		t.Errorf("ToolLoopThreshold 默认应为 5, 得到 %d", cfg.ToolLoopThreshold)
	}
	if cfg.ResponseStrategy != "warn" {
		t.Errorf("ResponseStrategy 默认应为 warn, 得到 %q", cfg.ResponseStrategy)
	}
}

// 辅助函数
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}