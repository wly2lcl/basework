package agent

import (
	"context"
	"testing"

	"github.com/wly2lcl/basework/pkg/llm"
)

func TestAgentLoop_HandleMessage_Basic(t *testing.T) {
	model := &mockModel{
		responses: []llm.Response{
			{
				Message: llm.ChatMessage{
					Role:    llm.RoleAssistant,
					Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "你好！"}},
				},
				Usage: llm.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
			},
		},
	}

	a, err := New(WithModel(model))
	if err != nil {
		t.Fatalf("New 返回错误: %v", err)
	}
	defer a.Close()

	resp, err := a.HandleMessage(context.Background(), "你好")
	if err != nil {
		t.Fatalf("HandleMessage 返回错误: %v", err)
	}

	if resp == nil {
		t.Fatal("期望非 nil response")
	}

	text := ""
	for _, part := range resp.Message.Content {
		if part.Type == llm.ContentTypeText {
			text += part.Text
		}
	}
	if text != "你好！" {
		t.Errorf("期望文本='你好！', 得到 '%s'", text)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("期望 TotalTokens=15, 得到 %d", resp.Usage.TotalTokens)
	}
	if resp.SessionID == "" {
		t.Error("期望非空 SessionID")
	}
}

func TestAgentLoop_HandleMessage_ToolLoop(t *testing.T) {
	model := &mockModel{
		responses: []llm.Response{
			{
				Message: llm.ChatMessage{
					Role: llm.RoleAssistant,
					ToolCalls: []llm.ToolCall{
						{ID: "call_1", Name: "test_tool", ArgsJSON: `{}`},
					},
				},
				Usage: llm.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
			},
			{
				Message: llm.ChatMessage{
					Role:    llm.RoleAssistant,
					Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "完成"}},
				},
				Usage: llm.Usage{PromptTokens: 20, CompletionTokens: 10, TotalTokens: 30},
			},
		},
	}

	a, err := New(
		WithModel(model),
		WithTools(&mockTool{name: "test_tool"}),
	)
	if err != nil {
		t.Fatalf("New 返回错误: %v", err)
	}
	defer a.Close()

	resp, err := a.HandleMessage(context.Background(), "执行工具")
	if err != nil {
		t.Fatalf("HandleMessage 返回错误: %v", err)
	}

	if resp == nil {
		t.Fatal("期望非 nil response")
	}

	text := ""
	for _, part := range resp.Message.Content {
		if part.Type == llm.ContentTypeText {
			text += part.Text
		}
	}
	if text != "完成" {
		t.Errorf("期望最终文本='完成', 得到 '%s'", text)
	}

	if len(resp.ToolCalls) != 1 {
		t.Errorf("期望 1 条工具调用记录, 得到 %d", len(resp.ToolCalls))
	}
	if resp.Usage.TotalTokens != 30 {
		t.Errorf("期望 TotalTokens=30, 得到 %d", resp.Usage.TotalTokens)
	}
}

func TestAgentLoop_HandleMessage_MaxStepsExceeded(t *testing.T) {
	model := &mockModel{
		responses: []llm.Response{
			{
				Message: llm.ChatMessage{
					Role: llm.RoleAssistant,
					ToolCalls: []llm.ToolCall{
						{ID: "call_1", Name: "always_tool", ArgsJSON: `{}`},
					},
				},
			},
		},
	}

	a, err := New(
		WithModel(model),
		WithTools(&mockTool{name: "always_tool"}),
		WithMaxSteps(3),
	)
	if err != nil {
		t.Fatalf("New 返回错误: %v", err)
	}
	defer a.Close()

	_, err = a.HandleMessage(context.Background(), "循环测试")
	if err != ErrMaxStepsExceeded {
		t.Errorf("期望 ErrMaxStepsExceeded, 得到 %v", err)
	}
}

func TestAgentLoop_HandleMessages(t *testing.T) {
	model := &mockModel{
		responses: []llm.Response{
			{
				Message: llm.ChatMessage{
					Role:    llm.RoleAssistant,
					Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "这是回答"}},
				},
				Usage: llm.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
			},
		},
	}

	a, err := New(WithModel(model))
	if err != nil {
		t.Fatalf("New 返回错误: %v", err)
	}
	defer a.Close()

	messages := []llm.ChatMessage{
		{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "问题1"}}},
		{Role: llm.RoleAssistant, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "回答1"}}},
		{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "问题2"}}},
	}

	resp, err := a.HandleMessages(context.Background(), messages)
	if err != nil {
		t.Fatalf("HandleMessages 返回错误: %v", err)
	}

	if resp == nil {
		t.Fatal("期望非 nil response")
	}

	text := ""
	for _, part := range resp.Message.Content {
		if part.Type == llm.ContentTypeText {
			text += part.Text
		}
	}
	if text != "这是回答" {
		t.Errorf("期望文本='这是回答', 得到 '%s'", text)
	}
}

func TestAgentLoop_Close_Idempotent(t *testing.T) {
	model := &mockModel{}
	a, err := New(WithModel(model))
	if err != nil {
		t.Fatalf("New 返回错误: %v", err)
	}

	if err := a.Close(); err != nil {
		t.Errorf("第一次 Close 返回错误: %v", err)
	}

	// 重复关闭不应返回错误
	if err := a.Close(); err != nil {
		t.Errorf("第二次 Close 返回错误: %v", err)
	}
}

func TestAgentLoop_Close_ThenHandleMessage(t *testing.T) {
	model := &mockModel{}
	a, err := New(WithModel(model))
	if err != nil {
		t.Fatalf("New 返回错误: %v", err)
	}

	_ = a.Close()

	resp, err := a.HandleMessage(context.Background(), "test")
	if err != nil {
		t.Fatalf("关闭后 HandleMessage 不应返回错误: %v", err)
	}
	if resp != nil {
		t.Error("关闭后 HandleMessage 应返回 nil response")
	}
}

func TestAgentLoop_CallbackIntegration(t *testing.T) {
	var (
		gotTextDelta string
		gotTurnEnd   bool
	)

	model := &mockModel{
		responses: []llm.Response{
			{
				Message: llm.ChatMessage{
					Role:    llm.RoleAssistant,
					Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "你好"}},
				},
				Usage: llm.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
			},
		},
	}

	a, err := New(
		WithModel(model),
		WithCallback(&CallbackFuncs{
			TextDelta: func(d string) { gotTextDelta = d },
			TurnEnd:   func(r *Response) { gotTurnEnd = true },
		}),
	)
	if err != nil {
		t.Fatalf("New 返回错误: %v", err)
	}
	defer a.Close()

	resp, err := a.HandleMessage(context.Background(), "你好")
	if err != nil {
		t.Fatalf("HandleMessage 返回错误: %v", err)
	}

	if resp == nil {
		t.Fatal("期望非 nil response")
	}

	if gotTextDelta != "你好" {
		t.Errorf("期望 TextDelta='你好', 得到 '%s'", gotTextDelta)
	}
	if !gotTurnEnd {
		t.Error("期望 TurnEnd 被调用")
	}
}