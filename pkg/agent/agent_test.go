package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/tool"
)

func TestNew_MissingModel(t *testing.T) {
	_, err := New()
	if err == nil {
		t.Fatal("期望缺少 model 时返回错误")
	}
}

func TestNew_WithModel(t *testing.T) {
	model := &mockModel{}
	agent, err := New(WithModel(model))
	if err != nil {
		t.Fatalf("New 返回错误: %v", err)
	}
	defer agent.Close()

	if agent == nil {
		t.Fatal("期望非 nil agent")
	}
}

func TestWithMaxContextTokensWiresIntoAgentLoop(t *testing.T) {
	a, err := New(WithModel(&mockModel{}), WithMaxContextTokens(37))
	if err != nil {
		t.Fatalf("New 返回错误: %v", err)
	}
	defer a.Close()

	loop, ok := a.(*AgentLoop)
	if !ok {
		t.Fatalf("New 返回 %T，期望 *AgentLoop", a)
	}
	if loop.cfg.maxContextTokens != 37 {
		t.Fatalf("maxContextTokens = %d，期望 37", loop.cfg.maxContextTokens)
	}

	// <= 0 不应把默认预算意外改成无预算，避免旧调用方行为变化。
	b, err := New(WithModel(&mockModel{}), WithMaxContextTokens(0))
	if err != nil {
		t.Fatalf("New 默认预算返回错误: %v", err)
	}
	defer b.Close()
	defaultLoop := b.(*AgentLoop)
	if defaultLoop.cfg.maxContextTokens != 128000 {
		t.Fatalf("非正预算 = %d，期望默认 128000", defaultLoop.cfg.maxContextTokens)
	}
}

func TestNew_DefaultMaxSteps(t *testing.T) {
	model := &mockModel{}
	a, err := New(WithModel(model))
	if err != nil {
		t.Fatalf("New 返回错误: %v", err)
	}
	defer a.Close()

	loop, ok := a.(*AgentLoop)
	if !ok {
		t.Fatal("期望 AgentLoop 类型")
	}
	if loop.cfg.maxSteps != 25 {
		t.Errorf("期望默认 maxSteps=25, 得到 %d", loop.cfg.maxSteps)
	}
}

func TestNew_WithMaxSteps(t *testing.T) {
	model := &mockModel{}
	a, err := New(WithModel(model), WithMaxSteps(10))
	if err != nil {
		t.Fatalf("New 返回错误: %v", err)
	}
	defer a.Close()

	loop, ok := a.(*AgentLoop)
	if !ok {
		t.Fatal("期望 AgentLoop 类型")
	}
	if loop.cfg.maxSteps != 10 {
		t.Errorf("期望 maxSteps=10, 得到 %d", loop.cfg.maxSteps)
	}
}

func TestNew_WithTools(t *testing.T) {
	model := &mockModel{}
	tool1 := &mockTool{name: "tool1"}
	tool2 := &mockTool{name: "tool2"}

	a, err := New(WithModel(model), WithTools(tool1, tool2))
	if err != nil {
		t.Fatalf("New 返回错误: %v", err)
	}
	defer a.Close()

	loop, ok := a.(*AgentLoop)
	if !ok {
		t.Fatal("期望 AgentLoop 类型")
	}

	// 验证工具已注册
	if loop.cfg.registry.Get("tool1") == nil {
		t.Error("期望 tool1 已注册")
	}
	if loop.cfg.registry.Get("tool2") == nil {
		t.Error("期望 tool2 已注册")
	}
}

func TestNew_WithSubAgentRunnerRegistersTool(t *testing.T) {
	model := &mockModel{}
	runner := &mockSubAgentRunner{enabled: true, output: "child result"}

	a, err := New(WithModel(model), WithSubAgentRunner(runner))
	if err != nil {
		t.Fatalf("New 返回错误: %v", err)
	}
	defer a.Close()

	st := findTool(a.Tools(), "sub_agent")
	if st == nil {
		t.Fatal("期望自动注册 sub_agent 工具")
	}

	result, err := st.Execute(context.Background(), json.RawMessage(`{"description":"review code"}`))
	if err != nil {
		t.Fatalf("sub_agent Execute 返回错误: %v", err)
	}
	if result.Content != "child result" {
		t.Fatalf("期望子代理输出 child result，得到 %q", result.Content)
	}
	if runner.lastTask != "review code" {
		t.Fatalf("期望任务透传给 runner，得到 %q", runner.lastTask)
	}
}

func TestNew_WithSubAgentRunnerDoesNotOverrideExplicitTool(t *testing.T) {
	model := &mockModel{}
	explicit := &mockTool{name: "sub_agent"}

	a, err := New(
		WithModel(model),
		WithTools(explicit),
		WithSubAgentRunner(&mockSubAgentRunner{enabled: true, output: "child result"}),
	)
	if err != nil {
		t.Fatalf("New 返回错误: %v", err)
	}
	defer a.Close()

	if got := findTool(a.Tools(), "sub_agent"); got != explicit {
		t.Fatal("显式注册的 sub_agent 工具不应被覆盖")
	}
}

func TestNew_WithSystemPrompt(t *testing.T) {
	model := &mockModel{}
	a, err := New(WithModel(model), WithSystemPrompt("你是一个助手"))
	if err != nil {
		t.Fatalf("New 返回错误: %v", err)
	}
	defer a.Close()

	loop, ok := a.(*AgentLoop)
	if !ok {
		t.Fatal("期望 AgentLoop 类型")
	}
	if loop.cfg.systemPrompt != "你是一个助手" {
		t.Errorf("期望 systemPrompt='你是一个助手', 得到 '%s'", loop.cfg.systemPrompt)
	}
}

func findTool(tools []tool.Tool, name string) tool.Tool {
	for _, t := range tools {
		if t.Name() == name {
			return t
		}
	}
	return nil
}

type mockSubAgentRunner struct {
	enabled  bool
	output   string
	lastTask string
}

func (r *mockSubAgentRunner) Run(_ context.Context, task string) (string, error) {
	r.lastTask = task
	return r.output, nil
}

func (r *mockSubAgentRunner) Enabled() bool { return r.enabled }

var _ SubAgentRunner = (*mockSubAgentRunner)(nil)

func TestNew_WithCallback(t *testing.T) {
	model := &mockModel{}
	cb := &NopCallback{}
	a, err := New(WithModel(model), WithCallback(cb))
	if err != nil {
		t.Fatalf("New 返回错误: %v", err)
	}
	defer a.Close()

	loop, ok := a.(*AgentLoop)
	if !ok {
		t.Fatal("期望 AgentLoop 类型")
	}
	if loop.callback == nil {
		t.Error("期望 callback 不为 nil")
	}
}

func TestAgent_HandleMessage_Basic(t *testing.T) {
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

	resp, err := a.HandleMessage(nil, "你好")
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

func TestAgent_HandleMessage_WithToolLoop(t *testing.T) {
	model := &mockModel{
		responses: []llm.Response{
			{
				// 第一轮：返回 tool call
				Message: llm.ChatMessage{
					Role: llm.RoleAssistant,
					ToolCalls: []llm.ToolCall{
						{ID: "call_1", Name: "test_tool", ArgsJSON: `{}`},
					},
				},
				Usage: llm.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
			},
			{
				// 第二轮：返回纯文本（工具执行后）
				Message: llm.ChatMessage{
					Role:    llm.RoleAssistant,
					Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "工具执行完成"}},
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

	resp, err := a.HandleMessage(nil, "执行工具")
	if err != nil {
		t.Fatalf("HandleMessage 返回错误: %v", err)
	}

	if resp == nil {
		t.Fatal("期望非 nil response")
	}

	// 最终消息应为第二轮文本
	text := ""
	for _, part := range resp.Message.Content {
		if part.Type == llm.ContentTypeText {
			text += part.Text
		}
	}
	if text != "工具执行完成" {
		t.Errorf("期望最终文本='工具执行完成', 得到 '%s'", text)
	}

	// 应有工具调用记录
	if len(resp.ToolCalls) != 1 {
		t.Errorf("期望 1 条工具调用记录, 得到 %d", len(resp.ToolCalls))
	}

	// 用量应为最后一次的
	if resp.Usage.TotalTokens != 30 {
		t.Errorf("期望 TotalTokens=30, 得到 %d", resp.Usage.TotalTokens)
	}
}

func TestAgent_HandleMessage_MaxStepsExceeded(t *testing.T) {
	// 模型总是返回 tool call，导致无限循环
	model := &mockModel{
		responses: []llm.Response{
			{
				Message: llm.ChatMessage{
					Role: llm.RoleAssistant,
					ToolCalls: []llm.ToolCall{
						{ID: "call_1", Name: "test_tool", ArgsJSON: `{}`},
					},
				},
			},
		},
	}

	a, err := New(
		WithModel(model),
		WithTools(&mockTool{name: "test_tool"}),
		WithMaxSteps(3),
	)
	if err != nil {
		t.Fatalf("New 返回错误: %v", err)
	}
	defer a.Close()

	_, err = a.HandleMessage(nil, "循环测试")
	if err != ErrMaxStepsExceeded {
		t.Errorf("期望 ErrMaxStepsExceeded, 得到 %v", err)
	}
}

func TestAgent_Close(t *testing.T) {
	model := &mockModel{}
	a, err := New(WithModel(model))
	if err != nil {
		t.Fatalf("New 返回错误: %v", err)
	}

	if err := a.Close(); err != nil {
		t.Errorf("Close 返回错误: %v", err)
	}

	// 重复关闭不应 panic
	if err := a.Close(); err != nil {
		t.Errorf("重复 Close 返回错误: %v", err)
	}
}
