package pkg_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/wly2lcl/basework/pkg/hook"
	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/tool"
)

// TestIntegration_FullFlow 端到端集成测试：
// Registry → Materialize → Request → Hook 权限检查 → Settle 执行 → 结果验证
func TestIntegration_FullFlow(t *testing.T) {
	// 1. 创建 Registry，注册 EchoTool 和 AddTool
	reg := tool.NewRegistry()
	if err := reg.Register(&tool.EchoTool{}); err != nil {
		t.Fatalf("注册 EchoTool 失败: %v", err)
	}
	if err := reg.Register(&tool.AddTool{}); err != nil {
		t.Fatalf("注册 AddTool 失败: %v", err)
	}

	// 2. 创建 Hook Chain，添加 PermissionHook（Allow 所有工具）
	permHook := hook.NewPermissionHook(
		[]hook.Rule{
			{Action: "*", Resource: "*", Effect: hook.Allow},
		},
		nil,
	)
	chain := hook.NewChain()
	chain.Add(permHook)

	// 3. 使用 Materialize 生成工具定义列表，供 LLM 使用
	defs := reg.Materialize()
	if len(defs) != 2 {
		t.Fatalf("Materialize 应返回 2 个工具定义，得到 %d", len(defs))
	}
	// 验证工具定义名称
	names := make(map[string]bool)
	for _, d := range defs {
		names[d.Name] = true
	}
	if !names["echo"] || !names["add"] {
		t.Fatal("Materialize 应包含 echo 和 add")
	}

	// 4. 构建 llm.Request（包含用户消息 + 工具定义）
	req := &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "请帮我算 1+2"}}},
		},
		Tools:     defs,
		MaxTokens: 100,
	}
	if len(req.Tools) != 2 {
		t.Fatalf("Request 应包含 2 个工具定义，得到 %d", len(req.Tools))
	}

	// 5. 模拟 LLM 返回工具调用
	toolCall := llm.ToolCall{
		ID:       "call_echo_1",
		Name:     "echo",
		ArgsJSON: `{"message":"hello from integration"}`,
	}

	// 6. 通过 Chain.RunBeforeTool 检查权限（PermissionHook Allow 应通过）
	allowedCall, err := chain.RunBeforeTool(toolCall)
	if err != nil {
		t.Fatalf("RunBeforeTool 权限检查应通过: %v", err)
	}
	if allowedCall == nil {
		t.Fatal("RunBeforeTool 应返回非 nil ToolCall")
	}
	if allowedCall.Name != "echo" {
		t.Fatalf("ToolCall 名称应保持为 echo，得到 %s", allowedCall.Name)
	}

	// 7. 通过 Registry.Settle 执行工具
	result, err := reg.Settle(context.Background(), toolCall)
	if err != nil {
		t.Fatalf("Settle 执行失败: %v", err)
	}
	if result == nil {
		t.Fatal("Settle 应返回非 nil Result")
	}

	// 8. 验证 tool.Result 内容正确
	if result.Content != "hello from integration" {
		t.Fatalf("EchoTool 结果应为 'hello from integration'，得到 %q", result.Content)
	}
	if result.IsError {
		t.Fatal("EchoTool 执行不应标记为错误")
	}

	// 9. 通过 Chain.RunAfterTool 观察结果
	var observedCall llm.ToolCall
	var observedResult *tool.Result
	var observedErr error

	observer := &hook.FuncHook{
		AfterToolFn: func(call llm.ToolCall, result *tool.Result, err error) {
			observedCall = call
			observedResult = result
			observedErr = err
		},
	}
	chain.Add(observer)
	chain.RunAfterTool(toolCall, result, nil)

	if observedCall.Name != "echo" {
		t.Fatalf("AfterTool 观察到的调用名称应为 echo，得到 %s", observedCall.Name)
	}
	if observedResult.Content != "hello from integration" {
		t.Fatalf("AfterTool 观察到的结果应为 'hello from integration'，得到 %q", observedResult.Content)
	}
	if observedErr != nil {
		t.Fatalf("AfterTool 观察到的错误应为 nil，得到 %v", observedErr)
	}
}

// TestIntegration_AddTool 测试 AddTool 通过 Settle 执行的完整流程
func TestIntegration_AddTool(t *testing.T) {
	reg := tool.NewRegistry()
	if err := reg.Register(&tool.AddTool{}); err != nil {
		t.Fatalf("注册 AddTool 失败: %v", err)
	}

	chain := hook.NewChain()
	chain.Add(hook.NewPermissionHook(
		[]hook.Rule{{Action: "*", Resource: "*", Effect: hook.Allow}},
		nil,
	))

	call := llm.ToolCall{
		ID:       "call_add_1",
		Name:     "add",
		ArgsJSON: `{"a":1.5,"b":2.5}`,
	}

	// 权限检查
	_, err := chain.RunBeforeTool(call)
	if err != nil {
		t.Fatalf("权限检查应通过: %v", err)
	}

	// 执行
	result, err := reg.Settle(context.Background(), call)
	if err != nil {
		t.Fatalf("Settle 执行失败: %v", err)
	}
	if result.Content != "4" {
		t.Fatalf("1.5+2.5 应等于 4，得到 %q", result.Content)
	}
}

// TestIntegration_BrokerEventBus 测试 Broker[llm.ToolCall] 事件总线
func TestIntegration_BrokerEventBus(t *testing.T) {
	broker := hook.NewBroker[llm.ToolCall]()
	defer broker.Close()

	// 订阅工具调用事件
	ch := broker.Subscribe()

	// 发布事件
	event := llm.ToolCall{
		ID:       "call_broker_1",
		Name:     "echo",
		ArgsJSON: `{"message":"event"}`,
	}
	broker.Publish(event)

	// 验证订阅者收到事件
	received := <-ch
	if received.Name != "echo" {
		t.Fatalf("订阅者应收到 echo 事件，得到 %s", received.Name)
	}
	if received.ArgsJSON != `{"message":"event"}` {
		t.Fatalf("ArgsJSON 不匹配，得到 %s", received.ArgsJSON)
	}
}

// TestIntegration_PermissionDeny 测试 PermissionHook Deny 规则阻止工具执行
func TestIntegration_PermissionDeny(t *testing.T) {
	reg := tool.NewRegistry()
	if err := reg.Register(&tool.EchoTool{}); err != nil {
		t.Fatalf("注册 EchoTool 失败: %v", err)
	}

	// Deny 所有工具
	permHook := hook.NewPermissionHook(
		[]hook.Rule{
			{Action: "*", Resource: "*", Effect: hook.Deny},
		},
		nil,
	)
	chain := hook.NewChain()
	chain.Add(permHook)

	call := llm.ToolCall{
		ID:       "call_deny_1",
		Name:     "echo",
		ArgsJSON: `{"message":"should be denied"}`,
	}

	// 权限检查应拒绝
	_, err := chain.RunBeforeTool(call)
	if err == nil {
		t.Fatal("Deny 规则下 RunBeforeTool 应返回 error")
	}

	// 确认 Settle 仍能执行（权限检查在 Hook 层，不影响 Registry 直接调用）
	result, err := reg.Settle(context.Background(), call)
	if err != nil {
		t.Fatalf("Registry.Settle 直接调用应成功: %v", err)
	}
	if result.Content != "should be denied" {
		t.Fatalf("结果应为 'should be denied'，得到 %q", result.Content)
	}
}

// TestIntegration_MaterializeWithRequest 测试 Materialize→Request 的完整链路
func TestIntegration_MaterializeWithRequest(t *testing.T) {
	reg := tool.NewRegistry()
	if err := reg.Register(&tool.EchoTool{}); err != nil {
		t.Fatalf("注册 EchoTool 失败: %v", err)
	}
	if err := reg.Register(&tool.AddTool{}); err != nil {
		t.Fatalf("注册 AddTool 失败: %v", err)
	}

	// Materialize
	defs := reg.Materialize()

	// 构建 Request
	req := &llm.Request{
		Messages: []llm.ChatMessage{
			{Role: llm.RoleSystem, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "你是一个助手"}}},
			{Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: "echo hello"}}},
		},
		Tools: defs,
	}

	// 将 ToolDefinition 转成 JSON 验证参数 schema
	for _, def := range req.Tools {
		if def.Parameters == nil {
			t.Fatalf("工具 %s 的 Parameters 不应为 nil", def.Name)
		}
		var schema map[string]any
		if err := json.Unmarshal(def.Parameters, &schema); err != nil {
			t.Fatalf("工具 %s 的 Parameters 不是合法 JSON: %v", def.Name, err)
		}
		if schema["type"] != "object" {
			t.Fatalf("工具 %s 的 Parameters type 应为 object", def.Name)
		}
	}
}

// TestIntegration_DisabledToolInMaterialize 测试禁用工具在 Materialize 中被排除
func TestIntegration_DisabledToolInMaterialize(t *testing.T) {
	reg := tool.NewRegistry()
	_ = reg.Register(&tool.EchoTool{})
	_ = reg.Register(&tool.AddTool{})

	// 禁用 echo
	reg.Disable("echo")

	defs := reg.Materialize()
	if len(defs) != 1 {
		t.Fatalf("禁用后应返回 1 个定义，得到 %d", len(defs))
	}
	if defs[0].Name != "add" {
		t.Fatalf("剩余工具应为 add，得到 %s", defs[0].Name)
	}

	// 用 Materialize 结果构建 Request
	req := &llm.Request{Tools: defs}
	if len(req.Tools) != 1 {
		t.Fatalf("Request.Tools 应包含 1 个定义，得到 %d", len(req.Tools))
	}
}

// TestIntegration_ChainBeforeToolAfterSettle 测试完整编排：
// Hook 权限检查 → Settle 执行 → AfterTool 观察
func TestIntegration_ChainBeforeToolAfterSettle(t *testing.T) {
	reg := tool.NewRegistry()
	_ = reg.Register(&tool.EchoTool{})

	// 收集执行过程中 AfterTool 调用的结果
	var afterCall llm.ToolCall
	var afterResult *tool.Result
	var afterErr error

	chain := hook.NewChain()
	chain.Add(
		hook.NewPermissionHook(
			[]hook.Rule{{Action: "*", Resource: "*", Effect: hook.Allow}},
			nil,
		),
		&hook.FuncHook{
			AfterToolFn: func(call llm.ToolCall, result *tool.Result, err error) {
				afterCall = call
				afterResult = result
				afterErr = err
			},
		},
	)

	call := llm.ToolCall{
		ID:       "call_full_1",
		Name:     "echo",
		ArgsJSON: `{"message":"full integration"}`,
	}

	// BeforeTool 权限检查
	_, err := chain.RunBeforeTool(call)
	if err != nil {
		t.Fatalf("BeforeTool 权限检查应通过: %v", err)
	}

	// Settle 执行
	result, err := reg.Settle(context.Background(), call)
	if err != nil {
		t.Fatalf("Settle 执行失败: %v", err)
	}

	// AfterTool 观察
	chain.RunAfterTool(call, result, nil)

	if afterCall.Name != "echo" {
		t.Fatalf("AfterTool 调用名应为 echo，得到 %s", afterCall.Name)
	}
	if afterResult.Content != "full integration" {
		t.Fatalf("AfterTool 结果应为 'full integration'，得到 %q", afterResult.Content)
	}
	if afterErr != nil {
		t.Fatalf("AfterTool 错误应为 nil，得到 %v", afterErr)
	}
}