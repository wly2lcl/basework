package agent

import (
	"context"
	"testing"

	"github.com/wly2lcl/basework/pkg/llm"
)

func TestShouldCompact_Triggered(t *testing.T) {
	usage := llm.Usage{TotalTokens: 90}
	cfg := CompactConfig{
		MaxTokens:        100,
		TriggerThreshold: 0.8,
	}
	if !ShouldCompact(usage, cfg) {
		t.Error("90/100 > 0.8 应触发压缩")
	}
}

func TestShouldCompact_NotTriggered(t *testing.T) {
	usage := llm.Usage{TotalTokens: 70}
	cfg := CompactConfig{
		MaxTokens:        100,
		TriggerThreshold: 0.8,
	}
	if ShouldCompact(usage, cfg) {
		t.Error("70/100 <= 0.8 不应触发压缩")
	}
}

func TestShouldCompact_DefaultThreshold(t *testing.T) {
	usage := llm.Usage{TotalTokens: 90}
	cfg := CompactConfig{
		MaxTokens: 100,
		// TriggerThreshold 默认 0.8
	}
	if !ShouldCompact(usage, cfg) {
		t.Error("90/100 > 0.8（默认阈值）应触发压缩")
	}
}

func TestShouldCompact_ZeroMaxTokens(t *testing.T) {
	usage := llm.Usage{TotalTokens: 100}
	cfg := CompactConfig{
		MaxTokens:        0,
		TriggerThreshold: 0.8,
	}
	if ShouldCompact(usage, cfg) {
		t.Error("MaxTokens=0 不应触发压缩")
	}
}

func TestShouldCompact_Boundary(t *testing.T) {
	usage := llm.Usage{TotalTokens: 80}
	cfg := CompactConfig{
		MaxTokens:        100,
		TriggerThreshold: 0.8,
	}
	if ShouldCompact(usage, cfg) {
		t.Error("80/100 == 0.8，不大于阈值，不应触发压缩")
	}
}

func TestTruncateMessages_LessThanKeep(t *testing.T) {
	msgs := []llm.ChatMessage{
		{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "msg1"}}},
		{Role: llm.RoleAssistant, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "resp1"}}},
	}

	result := TruncateMessages(msgs, 10)
	if len(result) != 2 {
		t.Errorf("期望 2 条, 得到 %d", len(result))
	}
}

func TestTruncateMessages_MoreThanKeep(t *testing.T) {
	msgs := make([]llm.ChatMessage, 20)
	for i := 0; i < 20; i++ {
		msgs[i] = llm.ChatMessage{
			Role:    llm.RoleUser,
			Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "msg"}},
		}
	}

	result := TruncateMessages(msgs, 10)
	if len(result) != 10 {
		t.Errorf("期望 10 条, 得到 %d", len(result))
	}

	// 验证保留的是最后 10 条（内容相同）
	for i := 0; i < 10; i++ {
		if result[i].Role != msgs[10+i].Role ||
			len(result[i].Content) != len(msgs[10+i].Content) {
			t.Errorf("期望保留最后 10 条消息")
			break
		}
	}
}

func TestTruncateMessages_DefaultKeep(t *testing.T) {
	msgs := make([]llm.ChatMessage, 20)
	for i := 0; i < 20; i++ {
		msgs[i] = llm.ChatMessage{
			Role:    llm.RoleUser,
			Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "msg"}},
		}
	}

	result := TruncateMessages(msgs, -1)
	if len(result) != 10 {
		t.Errorf("keepRecent<=0 时默认保留 10 条, 得到 %d", len(result))
	}
}

func TestCompactSummary_Mock(t *testing.T) {
	// 使用 mock model 测试 CompactSummary
	model := &mockModel{
		responses: []llm.Response{
			{
				Message: llm.ChatMessage{
					Role:    llm.RoleAssistant,
					Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "这是对话摘要"}},
				},
				Usage: llm.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
			},
		},
	}

	messages := []llm.ChatMessage{
		{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "你好"}}},
		{Role: llm.RoleAssistant, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "你好！需要什么帮助？"}}},
	}

	summary, err := CompactSummary(context.Background(), model, messages)
	if err != nil {
		t.Fatalf("CompactSummary 返回错误: %v", err)
	}
	if summary != "这是对话摘要" {
		t.Errorf("期望摘要='这是对话摘要', 得到 '%s'", summary)
	}
}

func TestCompactSummary_NilModel(t *testing.T) {
	_, err := CompactSummary(context.Background(), nil, nil)
	if err == nil {
		t.Error("model 为 nil 时应返回错误")
	}
}
