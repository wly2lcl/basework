package subagent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/wly2lcl/basework/pkg/tool"
)

// SubAgentTool 实现 tool.Tool 接口，作为子代理系统的入口工具
type SubAgentTool struct {
	coordinator *Coordinator
}

// NewSubAgentTool 创建子代理工具
func NewSubAgentTool(coordinator *Coordinator) *SubAgentTool {
	return &SubAgentTool{coordinator: coordinator}
}

// Name 返回工具名称
func (s *SubAgentTool) Name() string {
	return "sub_agent"
}

// Description 返回工具描述
func (s *SubAgentTool) Description() string {
	return "创建并执行子代理任务，支持 general（完整工具集）和 readonly（只读工具集）两种类型"
}

// Parameters 返回工具参数 JSON Schema
func (s *SubAgentTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"description": {
				"type": "string",
				"description": "子代理任务描述"
			},
			"agent_type": {
				"type": "string",
				"enum": ["general", "readonly"],
				"description": "子代理类型：general（完整工具集）或 readonly（只读工具集）",
				"default": "general"
			},
			"context": {
				"type": "object",
				"description": "传递给子代理的上下文信息",
				"additionalProperties": true
			}
		},
		"required": ["description"]
	}`)
}

// subAgentParams 工具参数结构
type subAgentParams struct {
	Description string                 `json:"description"`
	AgentType   string                 `json:"agent_type"`
	Context     map[string]interface{} `json:"context,omitempty"`
}

// Execute 执行子代理工具
func (s *SubAgentTool) Execute(ctx context.Context, args json.RawMessage) (*tool.Result, error) {
	if s.coordinator == nil {
		return &tool.Result{
			Content: "子代理协调器未初始化",
			IsError: true,
		}, nil
	}

	if !s.coordinator.Config.Enabled {
		return &tool.Result{
			Content: "子代理系统已禁用",
			IsError: true,
		}, nil
	}

	var params subAgentParams
	if err := json.Unmarshal(args, &params); err != nil {
		return &tool.Result{
			Content: fmt.Sprintf("参数解析失败: %v", err),
			IsError: true,
		}, nil
	}

	// 确定子代理类型
	agentType := AgentType(params.AgentType)
	if agentType == "" {
		agentType = AgentType(s.coordinator.Config.DefaultType)
	}

	// 创建任务
	task, err := s.coordinator.CreateTask(params.Description, agentType, params.Context)
	if err != nil {
		return &tool.Result{
			Content: fmt.Sprintf("创建子代理任务失败: %v", err),
			IsError: true,
		}, nil
	}

	// 执行任务
	result, err := s.coordinator.ExecuteTask(ctx, task)
	if err != nil {
		return &tool.Result{
			Content: fmt.Sprintf("执行子代理任务失败: %v", err),
			IsError: true,
		}, nil
	}

	// 序列化结果为 JSON 返回
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return &tool.Result{
			Content: fmt.Sprintf("序列化结果失败: %v", err),
			IsError: true,
		}, nil
	}

	if !result.Success {
		return &tool.Result{
			Content: string(resultJSON),
			IsError: true,
		}, nil
	}

	return &tool.Result{
		Content: string(resultJSON),
	}, nil
}
