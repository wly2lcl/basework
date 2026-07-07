package agent

import (
	"context"
	"testing"
	"time"

	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/tool"
)

func TestRunSubTurn_Sync(t *testing.T) {
	parentModel := &mockModel{
		responses: []llm.Response{
			{
				Message: llm.ChatMessage{
					Role:    llm.RoleAssistant,
					Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "子代理回复"}},
				},
				Usage: llm.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
			},
		},
	}

	parent, err := New(WithModel(parentModel))
	if err != nil {
		t.Fatalf("创建 parent agent 失败: %v", err)
	}
	defer parent.Close()

	cfg := SubTurnConfig{
		Prompt:   "子代理任务",
		Model:    parentModel,
		MaxSteps: 5,
	}

	result, err := RunSubTurn(context.Background(), parent, cfg)
	if err != nil {
		t.Fatalf("RunSubTurn 返回错误: %v", err)
	}
	if result.Err != nil {
		t.Fatalf("子代理执行错误: %v", result.Err)
	}
	if result.Response == nil {
		t.Fatal("期望非 nil Response")
	}

	text := ""
	for _, part := range result.Response.Message.Content {
		if part.Type == llm.ContentTypeText {
			text += part.Text
		}
	}
	if text != "子代理回复" {
		t.Errorf("期望文本='子代理回复', 得到 '%s'", text)
	}
}

func TestRunSubTurn_Async(t *testing.T) {
	subModel := &mockModel{
		responses: []llm.Response{
			{
				Message: llm.ChatMessage{
					Role:    llm.RoleAssistant,
					Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "异步回复"}},
				},
				Usage: llm.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
			},
		},
	}

	parent, err := New(WithModel(subModel))
	if err != nil {
		t.Fatalf("创建 parent agent 失败: %v", err)
	}
	defer parent.Close()

	cfg := SubTurnConfig{
		Prompt:   "异步任务",
		Model:    subModel,
		MaxSteps: 5,
		Async:    true,
	}

	result, err := RunSubTurn(context.Background(), parent, cfg)
	if err != nil {
		t.Fatalf("RunSubTurn 返回错误: %v", err)
	}

	// 异步模式，Result 需要等待 Done channel
	select {
	case <-result.Done:
		// 完成
	case <-time.After(5 * time.Second):
		t.Fatal("异步子代理 5 秒未完成")
	}

	if result.Err != nil {
		t.Fatalf("子代理执行错误: %v", result.Err)
	}
	if result.Response == nil {
		t.Fatal("期望非 nil Response")
	}

	text := ""
	for _, part := range result.Response.Message.Content {
		if part.Type == llm.ContentTypeText {
			text += part.Text
		}
	}
	if text != "异步回复" {
		t.Errorf("期望文本='异步回复', 得到 '%s'", text)
	}
}

func TestRunSubTurn_WithTools(t *testing.T) {
	subModel := &mockModel{
		responses: []llm.Response{
			{
				Message: llm.ChatMessage{
					Role: llm.RoleAssistant,
					ToolCalls: []llm.ToolCall{
						{ID: "call_1", Name: "sub_tool", ArgsJSON: `{}`},
					},
				},
			},
			{
				Message: llm.ChatMessage{
					Role:    llm.RoleAssistant,
					Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "工具执行完成"}},
				},
				Usage: llm.Usage{PromptTokens: 20, CompletionTokens: 10, TotalTokens: 30},
			},
		},
	}

	parent, err := New(WithModel(subModel))
	if err != nil {
		t.Fatalf("创建 parent agent 失败: %v", err)
	}
	defer parent.Close()

	cfg := SubTurnConfig{
		Prompt:   "执行工具",
		Model:    subModel,
		MaxSteps: 5,
		Tools:    []tool.Tool{&mockTool{name: "sub_tool"}},
	}

	result, err := RunSubTurn(context.Background(), parent, cfg)
	if err != nil {
		t.Fatalf("RunSubTurn 返回错误: %v", err)
	}
	if result.Err != nil {
		t.Fatalf("子代理执行错误: %v", result.Err)
	}
	if result.Response == nil {
		t.Fatal("期望非 nil Response")
	}

	text := ""
	for _, part := range result.Response.Message.Content {
		if part.Type == llm.ContentTypeText {
			text += part.Text
		}
	}
	if text != "工具执行完成" {
		t.Errorf("期望文本='工具执行完成', 得到 '%s'", text)
	}

	if len(result.Response.ToolCalls) != 1 {
		t.Errorf("期望 1 条工具调用记录, 得到 %d", len(result.Response.ToolCalls))
	}
}

func TestRunSubTurn_NilParent(t *testing.T) {
	_, err := RunSubTurn(context.Background(), nil, SubTurnConfig{})
	if err == nil {
		t.Error("parent 为 nil 时应返回错误")
	}
}
