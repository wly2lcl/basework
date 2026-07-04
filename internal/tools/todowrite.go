package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/wly2lcl/basework/pkg/tool"
)

// Todo 表示单个任务项
type Todo struct {
	Text   string `json:"text"`
	Status string `json:"status"` // "pending" 或 "completed"
}

// todoStore 是全局的任务列表存储（会话级，内存 map）
var (
	todoMu    sync.Mutex
	todoStore = make(map[string][]Todo) // key 是会话 ID
)

// TodoWriteTool 实现 todowrite 工具，用于管理任务列表
type TodoWriteTool struct{}

// NewTodoWriteTool 创建 todowrite 工具
func NewTodoWriteTool() *TodoWriteTool {
	return &TodoWriteTool{}
}

// Name 返回工具名称
func (t *TodoWriteTool) Name() string {
	return "todowrite"
}

// Description 返回工具描述
func (t *TodoWriteTool) Description() string {
	return "创建和管理任务列表，支持标记完成状态"
}

// Parameters 返回工具参数 JSON Schema
func (t *TodoWriteTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"todos": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"text": { "type": "string", "description": "任务描述" },
						"status": {
							"type": "string",
							"enum": ["pending", "completed"],
							"description": "任务状态：pending（待办）或 completed（已完成）",
							"default": "pending"
						}
					},
					"required": ["text"]
				},
				"description": "任务列表"
			}
		},
		"required": ["todos"]
	}`)
}

// todoWriteParams 工具参数结构
type todoWriteParams struct {
	Todos []Todo `json:"todos"`
}

// Execute 执行 todowrite 工具
func (t *TodoWriteTool) Execute(ctx context.Context, args json.RawMessage) (*tool.Result, error) {
	var params todoWriteParams
	if err := json.Unmarshal(args, &params); err != nil {
		return &tool.Result{
			Content: fmt.Sprintf("参数解析失败: %v", err),
			IsError: true,
		}, nil
	}

	if len(params.Todos) == 0 {
		return &tool.Result{
			Content: "任务列表为空",
			IsError: true,
		}, nil
	}

	// 使用固定 key 存储（简化实现）
	sessionID := "default"

	todoMu.Lock()
	todoStore[sessionID] = params.Todos
	todoMu.Unlock()

	// 渲染任务列表
	var buf strings.Builder
	buf.WriteString("📋 任务列表:\n\n")

	for i, todo := range params.Todos {
		status := "○"
		if todo.Status == "completed" {
			status = "✓"
		}
		buf.WriteString(fmt.Sprintf("%s %d. %s\n", status, i+1, todo.Text))
	}

	completed := 0
	for _, todo := range params.Todos {
		if todo.Status == "completed" {
			completed++
		}
	}
	buf.WriteString(fmt.Sprintf("\n总计: %d | 已完成: %d | 待办: %d", len(params.Todos), completed, len(params.Todos)-completed))

	return &tool.Result{
		Content: buf.String(),
	}, nil
}

// GetTodos 返回指定会话的任务列表（用于测试）
func GetTodos(sessionID string) []Todo {
	todoMu.Lock()
	defer todoMu.Unlock()
	todos := todoStore[sessionID]
	result := make([]Todo, len(todos))
	copy(result, todos)
	return result
}

// ClearTodos 清空指定会话的任务列表（用于测试）
func ClearTodos(sessionID string) {
	todoMu.Lock()
	defer todoMu.Unlock()
	delete(todoStore, sessionID)
}