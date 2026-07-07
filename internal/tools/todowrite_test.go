package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestTodoWriteTool_Name(t *testing.T) {
	tool := NewTodoWriteTool("test")
	if tool.Name() != "todowrite" {
		t.Errorf("期望 Name='todowrite', 得到 '%s'", tool.Name())
	}
}

func TestTodoWriteTool_Description(t *testing.T) {
	tool := NewTodoWriteTool("test")
	if tool.Description() == "" {
		t.Error("期望 Description 非空")
	}
}

func TestTodoWriteTool_Parameters(t *testing.T) {
	tool := NewTodoWriteTool("test")
	params := tool.Parameters()
	if len(params) == 0 {
		t.Error("期望 Parameters 非空")
	}
}

func TestTodoWriteTool_Execute_EmptyTodos(t *testing.T) {
	tool := NewTodoWriteTool("test")
	result, err := tool.Execute(context.Background(), []byte(`{"todos": []}`))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result.IsError {
		t.Error("期望 IsError=true")
	}
}

func TestTodoWriteTool_Execute_InvalidParams(t *testing.T) {
	tool := NewTodoWriteTool("test")
	result, err := tool.Execute(context.Background(), []byte(`not json`))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if !result.IsError {
		t.Error("期望 IsError=true")
	}
}

func TestTodoWriteTool_Execute_CreateTodos(t *testing.T) {
	// 先清理
	ClearTodos("test")

	tool := NewTodoWriteTool("test")
	result, err := tool.Execute(context.Background(), []byte(`{
		"todos": [
			{"text": "任务1", "status": "pending"},
			{"text": "任务2", "status": "completed"},
			{"text": "任务3"}
		]
	}`))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if result.IsError {
		t.Fatalf("期望 IsError=false, 得到: %s", result.Content)
	}

	// 验证输出包含 ✓ 和 ○
	if !strings.Contains(result.Content, "✓") {
		t.Errorf("期望包含 ✓ 标记, 得到: %s", result.Content)
	}
	if !strings.Contains(result.Content, "○") {
		t.Errorf("期望包含 ○ 标记, 得到: %s", result.Content)
	}

	// 验证统计信息
	if !strings.Contains(result.Content, "总计: 3") {
		t.Errorf("期望包含 '总计: 3', 得到: %s", result.Content)
	}
	if !strings.Contains(result.Content, "已完成: 1") {
		t.Errorf("期望包含 '已完成: 1', 得到: %s", result.Content)
	}
	if !strings.Contains(result.Content, "待办: 2") {
		t.Errorf("期望包含 '待办: 2', 得到: %s", result.Content)
	}
}

func TestTodoWriteTool_Execute_AllCompleted(t *testing.T) {
	ClearTodos("test")

	tool := NewTodoWriteTool("test")
	result, err := tool.Execute(context.Background(), []byte(`{
		"todos": [
			{"text": "完成", "status": "completed"},
			{"text": "搞定", "status": "completed"}
		]
	}`))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if result.IsError {
		t.Fatalf("期望 IsError=false, 得到: %s", result.Content)
	}

	if !strings.Contains(result.Content, "✓") {
		t.Error("期望所有任务都标记为 ✓")
	}
	if strings.Contains(result.Content, "○") {
		t.Error("不应有 ○ 标记")
	}
}

func TestTodoWriteTool_Execute_AllPending(t *testing.T) {
	ClearTodos("test")

	tool := NewTodoWriteTool("test")
	result, err := tool.Execute(context.Background(), []byte(`{
		"todos": [
			{"text": "待办1"},
			{"text": "待办2"}
		]
	}`))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	if result.IsError {
		t.Fatalf("期望 IsError=false, 得到: %s", result.Content)
	}

	if !strings.Contains(result.Content, "○") {
		t.Error("期望所有任务都标记为 ○")
	}
}

func TestTodoWriteTool_RenderFormat(t *testing.T) {
	ClearTodos("test")

	tool := NewTodoWriteTool("test")
	result, err := tool.Execute(context.Background(), []byte(`{
		"todos": [
			{"text": "任务 A", "status": "pending"}
		]
	}`))
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}

	// 验证格式：✓/○ 编号. 描述
	if !strings.Contains(result.Content, "○ 1.") {
		t.Errorf("期望渲染格式为 '○ 1. 任务 A', 得到: %s", result.Content)
	}
	if !strings.Contains(result.Content, "任务 A") {
		t.Errorf("期望包含任务描述, 得到: %s", result.Content)
	}
}

func TestTodoWriteTool_StoreAndGet(t *testing.T) {
	ClearTodos("test")

	tool := NewTodoWriteTool("test")
	tool.Execute(context.Background(), []byte(`{
		"todos": [
			{"text": "存储测试", "status": "pending"}
		]
	}`))

	todos := GetTodos("test")
	if len(todos) != 1 {
		t.Fatalf("期望 1 个任务, 得到 %d", len(todos))
	}
	if todos[0].Text != "存储测试" {
		t.Errorf("期望 Text='存储测试', 得到 '%s'", todos[0].Text)
	}
	if todos[0].Status != "pending" {
		t.Errorf("期望 Status='pending', 得到 '%s'", todos[0].Status)
	}
}

func TestTodoWriteTool_OverwriteExisting(t *testing.T) {
	ClearTodos("test")

	tool := NewTodoWriteTool("test")
	// 先写入一批
	tool.Execute(context.Background(), []byte(`{
		"todos": [{"text": "旧任务", "status": "pending"}]
	}`))

	// 覆盖写入新批次
	tool.Execute(context.Background(), []byte(`{
		"todos": [{"text": "新任务", "status": "completed"}]
	}`))

	todos := GetTodos("test")
	if len(todos) != 1 {
		t.Fatalf("期望 1 个任务, 得到 %d", len(todos))
	}
	if todos[0].Text != "新任务" {
		t.Errorf("期望 Text='新任务', 得到 '%s'", todos[0].Text)
	}
}

func TestTodo_ParametersSchema(t *testing.T) {
	tool := NewTodoWriteTool("test")
	params := tool.Parameters()

	var schema map[string]interface{}
	if err := json.Unmarshal(params, &schema); err != nil {
		t.Fatalf("解析 Parameters 失败: %v", err)
	}

	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("期望 properties 字段")
	}

	todosProp, ok := props["todos"]
	if !ok {
		t.Fatal("期望 todos 参数")
	}

	todosMap, ok := todosProp.(map[string]interface{})
	if !ok {
		t.Fatal("期望 todos 为 object")
	}

	if todosMap["type"] != "array" {
		t.Errorf("期望 todos 类型为 array, 得到 %v", todosMap["type"])
	}
}
