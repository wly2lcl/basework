package subagent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/wly2lcl/basework/pkg/tool"
)

// ===== Mock Agent =====

// mockAgent 实现 AgentTaskRunner 接口，用于测试
type mockAgent struct {
	handleMessageFn func(ctx context.Context, input string) (*TaskResult, error)
}

func (m *mockAgent) HandleMessage(ctx context.Context, input string) (*TaskResult, error) {
	if m.handleMessageFn != nil {
		return m.handleMessageFn(ctx, input)
	}
	return &TaskResult{
		Content:      "mock response: " + input,
		InputTokens:  10,
		OutputTokens: 20,
		TotalTokens:  30,
	}, nil
}

func (m *mockAgent) Close() error { return nil }

// newMockAgent 创建默认的 mock agent
func newMockAgent() *mockAgent {
	return &mockAgent{}
}

// newErrorMockAgent 创建返回错误的 mock agent
func newErrorMockAgent(errMsg string) *mockAgent {
	return &mockAgent{
		handleMessageFn: func(ctx context.Context, input string) (*TaskResult, error) {
			return nil, errors.New(errMsg)
		},
	}
}

// newTimeoutMockAgent 创建会超时的 mock agent
func newTimeoutMockAgent() *mockAgent {
	return &mockAgent{
		handleMessageFn: func(ctx context.Context, input string) (*TaskResult, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
}

// ===== AgentType 类型测试 =====

func TestAgentType_Constants(t *testing.T) {
	tests := []struct {
		name     string
		agentType AgentType
		want     string
	}{
		{"通用类型", TypeGeneral, "general"},
		{"只读类型", TypeReadonly, "readonly"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.agentType) != tt.want {
				t.Errorf("AgentType = %q, 期望 %q", tt.agentType, tt.want)
			}
		})
	}
}

func TestAgentType_IsValid(t *testing.T) {
	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{"general 有效", "general", true},
		{"readonly 有效", "readonly", true},
		{"空字符串无效", "", false},
		{"未知类型无效", "unknown", false},
		{"大小写敏感", "General", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			at := AgentType(tt.value)
			valid := at == TypeGeneral || at == TypeReadonly
			if valid != tt.valid {
				t.Errorf("AgentType(%q) 有效 = %v, 期望 %v", tt.value, valid, tt.valid)
			}
		})
	}
}

// ===== Task 创建测试 =====

func TestNewTask(t *testing.T) {
	ctx := map[string]interface{}{"key": "value"}
	task := NewTask("测试任务", TypeGeneral, ctx)

	if task.ID == "" {
		t.Error("Task.ID 不应为空")
	}
	if task.Description != "测试任务" {
		t.Errorf("Task.Description = %q, 期望 %q", task.Description, "测试任务")
	}
	if task.AgentType != TypeGeneral {
		t.Errorf("Task.AgentType = %q, 期望 %q", task.AgentType, TypeGeneral)
	}
	if task.Context["key"] != "value" {
		t.Errorf("Task.Context[\"key\"] = %v, 期望 %v", task.Context["key"], "value")
	}
}

func TestNewTask_NoContext(t *testing.T) {
	task := NewTask("无上下文任务", TypeReadonly, nil)
	if task.Context != nil {
		t.Error("未提供 context 时，Task.Context 应为 nil")
	}
}

// ===== CostTracker 测试 =====

func TestCostTracker_TrackUsage(t *testing.T) {
	ct := NewCostTracker()

	usage := TokenUsage{InputTokens: 100, OutputTokens: 50, TotalTokens: 150}
	ct.TrackUsage("task-1", usage, 0.002)

	cost := ct.GetTaskCost("task-1")
	if cost != 0.002 {
		t.Errorf("GetTaskCost = %f, 期望 %f", cost, 0.002)
	}

	taskUsage := ct.GetTaskUsage("task-1")
	if taskUsage.InputTokens != 100 || taskUsage.OutputTokens != 50 || taskUsage.TotalTokens != 150 {
		t.Errorf("GetTaskUsage = %+v, 期望 {InputTokens:100 OutputTokens:50 TotalTokens:150}", taskUsage)
	}
}

func TestCostTracker_GetTaskCost_NoRecords(t *testing.T) {
	ct := NewCostTracker()
	cost := ct.GetTaskCost("nonexistent")
	if cost != 0 {
		t.Errorf("不存在的任务应返回 0, 得到 %f", cost)
	}
}

func TestCostTracker_GetTaskUsage_NoRecords(t *testing.T) {
	ct := NewCostTracker()
	usage := ct.GetTaskUsage("nonexistent")
	if usage.InputTokens != 0 || usage.OutputTokens != 0 || usage.TotalTokens != 0 {
		t.Errorf("不存在的任务应返回空 TokenUsage, 得到 %+v", usage)
	}
}

func TestCostTracker_GetTotalCost(t *testing.T) {
	ct := NewCostTracker()

	ct.TrackUsage("task-1", TokenUsage{InputTokens: 100, OutputTokens: 50, TotalTokens: 150}, 0.001)
	ct.TrackUsage("task-2", TokenUsage{InputTokens: 200, OutputTokens: 100, TotalTokens: 300}, 0.003)
	ct.TrackUsage("task-1", TokenUsage{InputTokens: 50, OutputTokens: 25, TotalTokens: 75}, 0.0005)

	total := ct.GetTotalCost()
	if total <= 0 {
		t.Errorf("GetTotalCost = %f, 期望正值", total)
	}
}

func TestCostTracker_GetTotalUsage(t *testing.T) {
	ct := NewCostTracker()

	ct.TrackUsage("task-1", TokenUsage{InputTokens: 100, OutputTokens: 50, TotalTokens: 150}, 0)
	ct.TrackUsage("task-2", TokenUsage{InputTokens: 200, OutputTokens: 100, TotalTokens: 300}, 0)

	usage := ct.GetTotalUsage()
	if usage.InputTokens != 300 {
		t.Errorf("Total InputTokens = %d, 期望 %d", usage.InputTokens, 300)
	}
	if usage.OutputTokens != 150 {
		t.Errorf("Total OutputTokens = %d, 期望 %d", usage.OutputTokens, 150)
	}
	if usage.TotalTokens != 450 {
		t.Errorf("Total TotalTokens = %d, 期望 %d", usage.TotalTokens, 450)
	}
}

func TestCostTracker_IsOverLimit(t *testing.T) {
	ct := NewCostTracker()

	ct.TrackUsage("task-1", TokenUsage{InputTokens: 100, OutputTokens: 50, TotalTokens: 150}, 0.50)

	tests := []struct {
		name   string
		taskID string
		limit  float64
		over   bool
	}{
		{"未超限（成本低于限制）", "task-1", 1.0, false},
		{"刚好超限（成本等于限制）", "task-1", 0.50, true},
		{"超限（成本高于限制）", "task-1", 0.30, true},
		{"不存在的任务（成本为 0）", "nonexistent", 0.01, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ct.IsOverLimit(tt.taskID, tt.limit)
			if got != tt.over {
				t.Errorf("IsOverLimit(%q, %f) = %v, 期望 %v", tt.taskID, tt.limit, got, tt.over)
			}
		})
	}
}

func TestCostTracker_Reset(t *testing.T) {
	ct := NewCostTracker()
	ct.TrackUsage("task-1", TokenUsage{InputTokens: 100, OutputTokens: 50, TotalTokens: 150}, 0.001)

	ct.Reset()

	if ct.GetTotalCost() != 0 {
		t.Error("Reset 后总成本应为 0")
	}
	if ct.GetTaskCost("task-1") != 0 {
		t.Error("Reset 后任务成本应为 0")
	}
}

func TestCostTracker_Summary(t *testing.T) {
	ct := NewCostTracker()
	ct.TrackUsage("task-1", TokenUsage{InputTokens: 100, OutputTokens: 50, TotalTokens: 150}, 0.001234)

	summary := ct.Summary()
	if summary == "" {
		t.Error("Summary 不应为空字符串")
	}
}

func TestCostTracker_ConcurrentSafety(t *testing.T) {
	ct := NewCostTracker()
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(id int) {
			ct.TrackUsage("task", TokenUsage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2}, 0.001)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	cost := ct.GetTotalCost()
	if cost < 0.009 || cost > 0.011 {
		t.Errorf("并发写入后总成本 = %f, 期望 ~0.01", cost)
	}
}

// ===== Coordinator 测试 =====

func TestCoordinator_DefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.Enabled {
		t.Error("默认配置应启用子代理")
	}
	if cfg.DefaultType != string(TypeGeneral) {
		t.Errorf("默认类型 = %q, 期望 %q", cfg.DefaultType, TypeGeneral)
	}
	if cfg.CostLimit != 1.0 {
		t.Errorf("默认成本限制 = %f, 期望 %f", cfg.CostLimit, 1.0)
	}
	if cfg.MaxConcurrent != 5 {
		t.Errorf("默认最大并发 = %d, 期望 %d", cfg.MaxConcurrent, 5)
	}
}

func TestCoordinator_NewCoordinator(t *testing.T) {
	cfg := DefaultConfig()
	factory := func(ctx context.Context, task *Task) (AgentTaskRunner, error) {
		return newMockAgent(), nil
	}

	c := NewCoordinator(factory, cfg)
	if c.AgentFactory == nil {
		t.Error("AgentFactory 不应为 nil")
	}
	if c.CostTracker() == nil {
		t.Error("CostTracker 不应为 nil")
	}
	if c.ProgressReporter() == nil {
		t.Error("ProgressReporter 不应为 nil")
	}
}

func TestCoordinator_SetProgressReporter(t *testing.T) {
	cfg := DefaultConfig()
	factory := func(ctx context.Context, task *Task) (AgentTaskRunner, error) {
		return newMockAgent(), nil
	}
	c := NewCoordinator(factory, cfg)

	reporter := NewCallbackProgressReporter()
	c.SetProgressReporter(reporter)

	if c.ProgressReporter() != reporter {
		t.Error("SetProgressReporter 后未正确返回")
	}
}

func TestCoordinator_CreateTask(t *testing.T) {
	cfg := DefaultConfig()
	factory := func(ctx context.Context, task *Task) (AgentTaskRunner, error) {
		return newMockAgent(), nil
	}
	c := NewCoordinator(factory, cfg)

	task, err := c.CreateTask("测试任务", TypeGeneral, nil)
	if err != nil {
		t.Fatalf("CreateTask 失败: %v", err)
	}
	if task.Description != "测试任务" {
		t.Errorf("Description = %q, 期望 %q", task.Description, "测试任务")
	}
	if task.AgentType != TypeGeneral {
		t.Errorf("AgentType = %q, 期望 %q", task.AgentType, TypeGeneral)
	}
	if task.ID == "" {
		t.Error("Task.ID 不应为空")
	}
}

func TestCoordinator_CreateTask_Disabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = false
	c := NewCoordinator(nil, cfg)

	_, err := c.CreateTask("测试任务", TypeGeneral, nil)
	if err == nil {
		t.Error("系统禁用时 CreateTask 应返回错误")
	}
}

func TestCoordinator_CreateTask_EmptyDescription(t *testing.T) {
	cfg := DefaultConfig()
	c := NewCoordinator(nil, cfg)

	_, err := c.CreateTask("", TypeGeneral, nil)
	if err == nil {
		t.Error("空描述时 CreateTask 应返回错误")
	}
}

func TestCoordinator_CreateTask_InvalidType(t *testing.T) {
	cfg := DefaultConfig()
	c := NewCoordinator(nil, cfg)

	_, err := c.CreateTask("测试任务", "unknown", nil)
	if err == nil {
		t.Error("无效类型时 CreateTask 应返回错误")
	}
}

func TestCoordinator_CreateTask_DefaultType(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DefaultType = string(TypeReadonly)
	c := NewCoordinator(nil, cfg)

	task, err := c.CreateTask("测试任务", "", nil)
	if err != nil {
		t.Fatalf("CreateTask 失败: %v", err)
	}
	if task.AgentType != TypeReadonly {
		t.Errorf("应使用默认类型 readonly, 得到 %q", task.AgentType)
	}
}

func TestCoordinator_ExecuteTask(t *testing.T) {
	cfg := DefaultConfig()
	factory := func(ctx context.Context, task *Task) (AgentTaskRunner, error) {
		return newMockAgent(), nil
	}
	c := NewCoordinator(factory, cfg)

	task := NewTask("测试任务", TypeGeneral, nil)
	result, err := c.ExecuteTask(context.Background(), task)
	if err != nil {
		t.Fatalf("ExecuteTask 失败: %v", err)
	}
	if !result.Success {
		t.Errorf("执行应成功, 得到错误: %s", result.Error)
	}
	if result.ID != task.ID {
		t.Errorf("Result.ID = %q, 期望 %q", result.ID, task.ID)
	}
}

func TestCoordinator_ExecuteTask_Disabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = false
	c := NewCoordinator(nil, cfg)

	_, err := c.ExecuteTask(context.Background(), &Task{ID: "1"})
	if err == nil {
		t.Error("系统禁用时 ExecuteTask 应返回错误")
	}
}

func TestCoordinator_ExecuteTask_NoFactory(t *testing.T) {
	cfg := DefaultConfig()
	c := NewCoordinator(nil, cfg)

	_, err := c.ExecuteTask(context.Background(), &Task{ID: "1", Description: "test"})
	if err == nil {
		t.Error("未设置 AgentFactory 时 ExecuteTask 应返回错误")
	}
}

func TestCoordinator_ExecuteTask_FactoryError(t *testing.T) {
	cfg := DefaultConfig()
	factory := func(ctx context.Context, task *Task) (AgentTaskRunner, error) {
		return nil, assertAnError("factory error")
	}
	c := NewCoordinator(factory, cfg)

	task := NewTask("测试任务", TypeGeneral, nil)
	result, err := c.ExecuteTask(context.Background(), task)
	if err != nil {
		t.Fatalf("ExecuteTask 失败: %v", err)
	}
	if result.Success {
		t.Error("工厂错误时 Result.Success 应为 false")
	}
}

// assertAnError 返回一个 error 用于测试
func assertAnError(msg string) error {
	return &testError{msg: msg}
}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

func TestCoordinator_ExecuteTask_CostLimit(t *testing.T) {
	cfg := DefaultConfig()
	cfg.CostLimit = 0.001
	factory := func(ctx context.Context, task *Task) (AgentTaskRunner, error) {
		return newMockAgent(), nil
	}
	c := NewCoordinator(factory, cfg)

	// 先记录一些成本使其超限
	c.costTracker.TrackUsage("task-over-limit", TokenUsage{TotalTokens: 1000}, 0.002)

	task := NewTask("超限任务", TypeGeneral, nil)
	// 手动设置 ID 匹配已记录的成本
	task.ID = "task-over-limit"

	result, err := c.ExecuteTask(context.Background(), task)
	if err != nil {
		t.Fatalf("ExecuteTask 失败: %v", err)
	}
	if result.Success {
		t.Error("成本超限时 Result.Success 应为 false")
	}
	if result.Error == "" {
		t.Error("成本超限时应返回错误信息")
	}
}

func TestCoordinator_ExecuteTasks(t *testing.T) {
	cfg := DefaultConfig()
	factory := func(ctx context.Context, task *Task) (AgentTaskRunner, error) {
		return newMockAgent(), nil
	}
	c := NewCoordinator(factory, cfg)

	tasks := []*Task{
		NewTask("任务1", TypeGeneral, nil),
		NewTask("任务2", TypeReadonly, nil),
		NewTask("任务3", TypeGeneral, nil),
	}

	results, err := c.ExecuteTasks(context.Background(), tasks)
	if err != nil {
		t.Fatalf("ExecuteTasks 失败: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("结果数量 = %d, 期望 %d", len(results), 3)
	}
	for i, r := range results {
		if !r.Success {
			t.Errorf("任务 %d 执行失败: %s", i, r.Error)
		}
		if r.ID != tasks[i].ID {
			t.Errorf("结果 %d 的 ID 不匹配", i)
		}
	}
}

func TestCoordinator_ExecuteTasks_Empty(t *testing.T) {
	cfg := DefaultConfig()
	c := NewCoordinator(nil, cfg)

	results, err := c.ExecuteTasks(context.Background(), nil)
	if err != nil {
		t.Fatalf("ExecuteTasks 失败: %v", err)
	}
	if results != nil {
		t.Error("空任务列表应返回 nil")
	}
}

func TestCoordinator_ExecuteTasks_Disabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = false
	c := NewCoordinator(nil, cfg)

	_, err := c.ExecuteTasks(context.Background(), []*Task{{ID: "1"}})
	if err == nil {
		t.Error("系统禁用时 ExecuteTasks 应返回错误")
	}
}

// ===== executeAgentTask 集成测试 =====

func TestExecuteAgentTask_Success(t *testing.T) {
	// 验证 executeAgentTask 正确调用 HandleMessage 并映射结果
	cfg := DefaultConfig()
	factory := func(ctx context.Context, task *Task) (AgentTaskRunner, error) {
		return newMockAgent(), nil
	}
	c := NewCoordinator(factory, cfg)

	task := NewTask("集成测试任务", TypeGeneral, nil)
	agent, err := c.AgentFactory(context.Background(), task)
	if err != nil {
		t.Fatalf("创建 agent 失败: %v", err)
	}

	result, err := c.executeAgentTask(context.Background(), agent, task)
	if err != nil {
		t.Fatalf("executeAgentTask 失败: %v", err)
	}
	if !result.Success {
		t.Errorf("执行应成功, 得到错误: %s", result.Error)
	}
	if result.Output != "mock response: 集成测试任务" {
		t.Errorf("Output = %q, 期望 %q", result.Output, "mock response: 集成测试任务")
	}
	if result.TokenUsage.InputTokens != 10 {
		t.Errorf("InputTokens = %d, 期望 %d", result.TokenUsage.InputTokens, 10)
	}
	if result.TokenUsage.OutputTokens != 20 {
		t.Errorf("OutputTokens = %d, 期望 %d", result.TokenUsage.OutputTokens, 20)
	}
	if result.TokenUsage.TotalTokens != 30 {
		t.Errorf("TotalTokens = %d, 期望 %d", result.TokenUsage.TotalTokens, 30)
	}
}

func TestExecuteAgentTask_AgentError(t *testing.T) {
	// 验证 agent 返回错误时 executeAgentTask 正确传播
	cfg := DefaultConfig()
	factory := func(ctx context.Context, task *Task) (AgentTaskRunner, error) {
		return newErrorMockAgent("something went wrong"), nil
	}
	c := NewCoordinator(factory, cfg)

	task := NewTask("错误测试", TypeGeneral, nil)
	agent, err := c.AgentFactory(context.Background(), task)
	if err != nil {
		t.Fatalf("创建 agent 失败: %v", err)
	}

	_, err = c.executeAgentTask(context.Background(), agent, task)
	if err == nil {
		t.Fatal("期望错误，但没有返回")
	}
	if err.Error() != "子代理执行失败: something went wrong" {
		t.Errorf("错误信息 = %q, 期望包含 'something went wrong'", err.Error())
	}
}

func TestExecuteAgentTask_Timeout(t *testing.T) {
	// 验证超时场景：agent 在 context 超时后返回 DeadlineExceeded
	cfg := DefaultConfig()
	factory := func(ctx context.Context, task *Task) (AgentTaskRunner, error) {
		return newTimeoutMockAgent(), nil
	}
	c := NewCoordinator(factory, cfg)

	task := NewTask("超时测试", TypeGeneral, nil)
	agent, err := c.AgentFactory(context.Background(), task)
	if err != nil {
		t.Fatalf("创建 agent 失败: %v", err)
	}

	// 使用极短超时的 context 触发 DeadlineExceeded
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// 等待 context 超时
	<-ctx.Done()

	result, err := c.executeAgentTask(ctx, agent, task)
	if err != nil {
		t.Fatalf("executeAgentTask 应返回 result, 但返回了 error: %v", err)
	}
	if result.Success {
		t.Error("超时应返回 Success=false")
	}
	if result.Error == "" {
		t.Error("超时应返回错误信息")
	}
}

func TestExecuteAgentTask_OutputInExecuteTask(t *testing.T) {
	// 验证 ExecuteTask 整体链路中 Output 正确传递
	cfg := DefaultConfig()
	factory := func(ctx context.Context, task *Task) (AgentTaskRunner, error) {
		return newMockAgent(), nil
	}
	c := NewCoordinator(factory, cfg)

	task := NewTask("全链路测试任务", TypeGeneral, nil)
	result, err := c.ExecuteTask(context.Background(), task)
	if err != nil {
		t.Fatalf("ExecuteTask 失败: %v", err)
	}
	if !result.Success {
		t.Errorf("执行应成功, 得到错误: %s", result.Error)
	}
	if result.Output != "mock response: 全链路测试任务" {
		t.Errorf("Output = %q, 期望 %q", result.Output, "mock response: 全链路测试任务")
	}
	if result.TokenUsage.TotalTokens != 30 {
		t.Errorf("TotalTokens = %d, 期望 %d", result.TokenUsage.TotalTokens, 30)
	}
}

// ===== ProgressReporter 测试 =====

func TestNopProgressReporter(t *testing.T) {
	reporter := &NopProgressReporter{}
	task := &Task{ID: "1", Description: "test"}
	result := &Result{ID: "1", Success: true}

	// 确保不 panic
	reporter.OnStart(task)
	reporter.OnProgress(task, "50%")
	reporter.OnComplete(task, result)
}

func TestCallbackProgressReporter_OnStart(t *testing.T) {
	reporter := NewCallbackProgressReporter()

	var captured ProgressEvent
	reporter.SetOnStart(func(event ProgressEvent) {
		captured = event
	})

	task := &Task{ID: "task-1", Description: "测试任务", AgentType: TypeGeneral}
	reporter.OnStart(task)

	if captured.Type != "subagent.start" {
		t.Errorf("事件类型 = %q, 期望 %q", captured.Type, "subagent.start")
	}
	if captured.TaskID != "task-1" {
		t.Errorf("TaskID = %q, 期望 %q", captured.TaskID, "task-1")
	}
	if captured.Description != "测试任务" {
		t.Errorf("Description = %q, 期望 %q", captured.Description, "测试任务")
	}
	if captured.Timestamp.IsZero() {
		t.Error("Timestamp 不应为 zero")
	}
}

func TestCallbackProgressReporter_OnProgress(t *testing.T) {
	reporter := NewCallbackProgressReporter()

	var captured ProgressEvent
	reporter.SetOnProgress(func(event ProgressEvent) {
		captured = event
	})

	task := &Task{ID: "task-1", Description: "测试任务"}
	reporter.OnProgress(task, "50%")

	if captured.Type != "subagent.progress" {
		t.Errorf("事件类型 = %q, 期望 %q", captured.Type, "subagent.progress")
	}
	if captured.Progress != "50%" {
		t.Errorf("Progress = %q, 期望 %q", captured.Progress, "50%")
	}
}

func TestCallbackProgressReporter_OnComplete(t *testing.T) {
	reporter := NewCallbackProgressReporter()

	var captured ProgressEvent
	reporter.SetOnComplete(func(event ProgressEvent) {
		captured = event
	})

	task := &Task{ID: "task-id-123456", Description: "测试任务"}
	result := &Result{ID: "task-id-123456", Success: true, Output: "完成"}
	reporter.OnComplete(task, result)

	if captured.Type != "subagent.complete" {
		t.Errorf("事件类型 = %q, 期望 %q", captured.Type, "subagent.complete")
	}
	if captured.Result == nil {
		t.Fatal("Result 不应为 nil")
	}
	if !captured.Result.Success {
		t.Error("Result.Success 应为 true")
	}
}

func TestCallbackProgressReporter_NoCallbacks(t *testing.T) {
	reporter := NewCallbackProgressReporter()
	task := &Task{ID: "test-id-123456", Description: "test"}

	// 未设置回调时不应 panic
	reporter.OnStart(task)
	reporter.OnProgress(task, "50%")
	reporter.OnComplete(task, &Result{ID: "test-id-123456", Success: true})
}

// ===== SubAgentTool 测试 =====

func TestSubAgentTool_Name(t *testing.T) {
	cfg := DefaultConfig()
	c := NewCoordinator(nil, cfg)
	tool := NewSubAgentTool(c)

	if tool.Name() != "sub_agent" {
		t.Errorf("Name() = %q, 期望 %q", tool.Name(), "sub_agent")
	}
}

func TestSubAgentTool_Description(t *testing.T) {
	cfg := DefaultConfig()
	c := NewCoordinator(nil, cfg)
	tool := NewSubAgentTool(c)

	desc := tool.Description()
	if desc == "" {
		t.Error("Description 不应为空")
	}
}

func TestSubAgentTool_Parameters(t *testing.T) {
	cfg := DefaultConfig()
	c := NewCoordinator(nil, cfg)
	tool := NewSubAgentTool(c)

	params := tool.Parameters()
	if params == nil {
		t.Fatal("Parameters 不应为 nil")
	}

	// 验证 JSON Schema 结构
	var schema map[string]interface{}
	if err := json.Unmarshal(params, &schema); err != nil {
		t.Fatalf("Parameters JSON 解析失败: %v", err)
	}

	if schema["type"] != "object" {
		t.Errorf("schema.type = %v, 期望 'object'", schema["type"])
	}

	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("properties 应为 object")
	}

	if _, ok := props["description"]; !ok {
		t.Error("缺少 description 属性")
	}
	if _, ok := props["agent_type"]; !ok {
		t.Error("缺少 agent_type 属性")
	}

	required, ok := schema["required"].([]interface{})
	if !ok {
		t.Fatal("required 应为 array")
	}
	if len(required) != 1 || required[0] != "description" {
		t.Errorf("required = %v, 期望 [\"description\"]", required)
	}
}

func TestSubAgentTool_Execute(t *testing.T) {
	cfg := DefaultConfig()
	factory := func(ctx context.Context, task *Task) (AgentTaskRunner, error) {
		return newMockAgent(), nil
	}
	c := NewCoordinator(factory, cfg)
	st := NewSubAgentTool(c)

	args := json.RawMessage(`{"description": "测试任务", "agent_type": "general"}`)
	result, err := st.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute 失败: %v", err)
	}
	if result.IsError {
		t.Errorf("执行应成功, 得到错误: %s", result.Content)
	}

	// 验证返回 JSON 结构
	var res Result
	if err := json.Unmarshal([]byte(result.Content), &res); err != nil {
		t.Fatalf("结果 JSON 解析失败: %v", err)
	}
	if !res.Success {
		t.Error("Result.Success 应为 true")
	}
}

func TestSubAgentTool_Execute_ReadonlyType(t *testing.T) {
	cfg := DefaultConfig()
	factory := func(ctx context.Context, task *Task) (AgentTaskRunner, error) {
		if task.AgentType != TypeReadonly {
			t.Error("期望 readonly 类型")
		}
		return newMockAgent(), nil
	}
	c := NewCoordinator(factory, cfg)
	st := NewSubAgentTool(c)

	args := json.RawMessage(`{"description": "只读任务", "agent_type": "readonly"}`)
	result, err := st.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute 失败: %v", err)
	}
	if result.IsError {
		t.Errorf("执行应成功, 得到错误: %s", result.Content)
	}
}

func TestSubAgentTool_Execute_WithContext(t *testing.T) {
	cfg := DefaultConfig()
	factory := func(ctx context.Context, task *Task) (AgentTaskRunner, error) {
		if task.Context == nil {
			t.Error("Context 不应为 nil")
		}
		return newMockAgent(), nil
	}
	c := NewCoordinator(factory, cfg)
	st := NewSubAgentTool(c)

	args := json.RawMessage(`{"description": "上下文任务", "agent_type": "general", "context": {"key": "value"}}`)
	result, err := st.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute 失败: %v", err)
	}
	if result.IsError {
		t.Errorf("执行应成功, 得到错误: %s", result.Content)
	}
}

func TestSubAgentTool_Execute_NoCoordinator(t *testing.T) {
	st := &SubAgentTool{coordinator: nil}

	args := json.RawMessage(`{"description": "test"}`)
	result, err := st.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute 失败: %v", err)
	}
	if !result.IsError {
		t.Error("无 coordinator 时应返回错误")
	}
}

func TestSubAgentTool_Execute_Disabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = false
	c := NewCoordinator(nil, cfg)
	st := NewSubAgentTool(c)

	args := json.RawMessage(`{"description": "test"}`)
	result, err := st.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute 失败: %v", err)
	}
	if !result.IsError {
		t.Error("系统禁用时应返回错误")
	}
}

func TestSubAgentTool_Execute_InvalidJSON(t *testing.T) {
	cfg := DefaultConfig()
	c := NewCoordinator(nil, cfg)
	st := NewSubAgentTool(c)

	args := json.RawMessage(`invalid json`)
	result, err := st.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute 失败: %v", err)
	}
	if !result.IsError {
		t.Error("无效 JSON 时应返回错误")
	}
}

func TestSubAgentTool_Execute_MissingDescription(t *testing.T) {
	cfg := DefaultConfig()
	c := NewCoordinator(nil, cfg)
	st := NewSubAgentTool(c)

	args := json.RawMessage(`{"agent_type": "general"}`)
	result, err := st.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute 失败: %v", err)
	}
	if !result.IsError {
		t.Error("缺少 description 时应返回错误")
	}
}

func TestSubAgentTool_ImplementsToolInterface(t *testing.T) {
	cfg := DefaultConfig()
	c := NewCoordinator(nil, cfg)
	st := NewSubAgentTool(c)

	// 编译时检查接口实现
	var _ tool.Tool = st
}

// ===== Result 结构测试 =====

func TestResult_StructuredFields(t *testing.T) {
	result := &Result{
		ID:      "task-1",
		Success: true,
		Output:  "完成",
		TokenUsage: TokenUsage{
			InputTokens:  100,
			OutputTokens: 50,
			TotalTokens:  150,
		},
		Cost: 0.002,
	}

	if result.ID != "task-1" {
		t.Errorf("ID = %q, 期望 %q", result.ID, "task-1")
	}
	if !result.Success {
		t.Error("Success 应为 true")
	}
	if result.Cost != 0.002 {
		t.Errorf("Cost = %f, 期望 %f", result.Cost, 0.002)
	}
}

func TestResult_FailureResult(t *testing.T) {
	result := &Result{
		ID:      "task-1",
		Success: false,
		Error:   "执行失败",
	}

	if result.Success {
		t.Error("Success 应为 false")
	}
	if result.Error != "执行失败" {
		t.Errorf("Error = %q, 期望 %q", result.Error, "执行失败")
	}
}

// ===== ProgressEvent 测试 =====

func TestProgressEvent_Timestamp(t *testing.T) {
	now := time.Now()
	event := ProgressEvent{
		TaskID:    "task-1",
		Type:      "subagent.start",
		Timestamp: now,
	}

	if event.Timestamp.IsZero() {
		t.Error("Timestamp 不应为 zero")
	}
}

// ===== Coordinator.ExecuteTask 进度报告测试 =====

func TestCoordinator_ExecuteTask_ReportsProgress(t *testing.T) {
	cfg := DefaultConfig()
	factory := func(ctx context.Context, task *Task) (AgentTaskRunner, error) {
		return newMockAgent(), nil
	}
	c := NewCoordinator(factory, cfg)

	events := make([]string, 0)
	reporter := NewCallbackProgressReporter()
	reporter.SetOnStart(func(event ProgressEvent) {
		events = append(events, "start")
	})
	reporter.SetOnComplete(func(event ProgressEvent) {
		events = append(events, "complete")
	})
	c.SetProgressReporter(reporter)

	task := NewTask("测试任务", TypeGeneral, nil)
	_, err := c.ExecuteTask(context.Background(), task)
	if err != nil {
		t.Fatalf("ExecuteTask 失败: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("事件数量 = %d, 期望 2 (start, complete)", len(events))
	}
	if events[0] != "start" {
		t.Errorf("第一个事件应为 start, 得到 %s", events[0])
	}
	if events[1] != "complete" {
		t.Errorf("第二个事件应为 complete, 得到 %s", events[1])
	}
}