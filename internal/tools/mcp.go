// Package tools 实现 basework 的内置工具集合。
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/wly2lcl/basework/pkg/mcp"
	"github.com/wly2lcl/basework/pkg/tool"
)

// MCPReadTool 实现 mcp_read 工具，用于读取 MCP 资源内容。
type MCPReadTool struct {
	manager *mcp.Manager
}

// NewMCPReadTool 创建 mcp_read 工具。
func NewMCPReadTool(manager *mcp.Manager) *MCPReadTool {
	return &MCPReadTool{manager: manager}
}

// Name 返回工具名称。
func (w *MCPReadTool) Name() string {
	return "mcp_read"
}

// Description 返回工具描述。
func (w *MCPReadTool) Description() string {
	return "读取 MCP 服务器的资源内容，需要提供资源 URI"
}

// Parameters 返回工具参数 JSON Schema。
func (w *MCPReadTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"resource": {
				"type": "string",
				"description": "资源 URI"
			},
			"server": {
				"type": "string",
				"description": "MCP 服务器名称（可选，不指定时自动查找）"
			}
		},
		"required": ["resource"]
	}`)
}

// mcpReadParams 工具参数结构。
type mcpReadParams struct {
	Resource string `json:"resource"`
	Server   string `json:"server,omitempty"`
}

// Execute 执行 mcp_read 工具。
func (w *MCPReadTool) Execute(ctx context.Context, args json.RawMessage) (*tool.Result, error) {
	var params mcpReadParams
	if err := json.Unmarshal(args, &params); err != nil {
		return &tool.Result{
			Content: fmt.Sprintf("参数解析失败: %v", err),
			IsError: true,
		}, nil
	}

	if params.Resource == "" {
		return &tool.Result{
			Content: "参数 'resource' 不能为空，需要提供资源 URI",
			IsError: true,
		}, nil
	}

	// 确定目标服务器
	var serverName string
	if params.Server != "" {
		serverName = params.Server
	} else {
		// 自动查找包含该资源的服务器
		serverName = w.findServerByURI(params.Resource)
		if serverName == "" {
			return &tool.Result{
				Content: fmt.Sprintf("未找到包含资源 URI %q 的服务器", params.Resource),
				IsError: true,
			}, nil
		}
	}

	result, err := w.manager.ReadResource(ctx, serverName, params.Resource)
	if err != nil {
		return &tool.Result{
			Content: fmt.Sprintf("读取资源失败: %v", err),
			IsError: true,
		}, nil
	}

	contentBytes, err := json.Marshal(result)
	if err != nil {
		return &tool.Result{
			Content: fmt.Sprintf("序列化结果失败: %v", err),
			IsError: true,
		}, nil
	}

	return &tool.Result{
		Content: string(contentBytes),
	}, nil
}

// findServerByURI 根据 URI 前缀查找匹配的服务器。
func (w *MCPReadTool) findServerByURI(uri string) string {
	for _, server := range w.manager.GetServers() {
		resources, err := w.manager.Resources(server.Name)
		if err != nil {
			continue
		}
		for _, r := range resources {
			if r.URI == uri {
				return server.Name
			}
		}
		// 也检查 URI 前缀匹配
		for _, r := range resources {
			if len(uri) >= len(r.URI) && uri[:len(r.URI)] == r.URI {
				return server.Name
			}
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// MCPPromptTool
// ---------------------------------------------------------------------------

// MCPPromptTool 实现 mcp_prompt 工具，用于获取 MCP 提示模板内容。
type MCPPromptTool struct {
	manager *mcp.Manager
}

// NewMCPPromptTool 创建 mcp_prompt 工具。
func NewMCPPromptTool(manager *mcp.Manager) *MCPPromptTool {
	return &MCPPromptTool{manager: manager}
}

// Name 返回工具名称。
func (p *MCPPromptTool) Name() string {
	return "mcp_prompt"
}

// Description 返回工具描述。
func (p *MCPPromptTool) Description() string {
	return "获取 MCP 服务器的提示模板内容，需要提供 prompt 名称"
}

// Parameters 返回工具参数 JSON Schema。
func (p *MCPPromptTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"prompt": {
				"type": "string",
				"description": "提示模板名称"
			},
			"server": {
				"type": "string",
				"description": "MCP 服务器名称（可选，不指定时自动查找）"
			}
		},
		"required": ["prompt"]
	}`)
}

// mcpPromptParams 工具参数结构。
type mcpPromptParams struct {
	Prompt string `json:"prompt"`
	Server string `json:"server,omitempty"`
}

// Execute 执行 mcp_prompt 工具。
func (p *MCPPromptTool) Execute(ctx context.Context, args json.RawMessage) (*tool.Result, error) {
	var params mcpPromptParams
	if err := json.Unmarshal(args, &params); err != nil {
		return &tool.Result{
			Content: fmt.Sprintf("参数解析失败: %v", err),
			IsError: true,
		}, nil
	}

	if params.Prompt == "" {
		return &tool.Result{
			Content: "参数 'prompt' 不能为空，需要提供提示模板名称",
			IsError: true,
		}, nil
	}

	// 确定目标服务器
	var serverName string
	if params.Server != "" {
		serverName = params.Server
	} else {
		serverName = p.findServerByPrompt(params.Prompt)
		if serverName == "" {
			return &tool.Result{
				Content: fmt.Sprintf("未找到包含提示模板 %q 的服务器", params.Prompt),
				IsError: true,
			}, nil
		}
	}

	result, err := p.manager.GetPrompt(ctx, serverName, params.Prompt)
	if err != nil {
		return &tool.Result{
			Content: fmt.Sprintf("获取提示模板失败: %v", err),
			IsError: true,
		}, nil
	}

	contentBytes, err := json.Marshal(result)
	if err != nil {
		return &tool.Result{
			Content: fmt.Sprintf("序列化结果失败: %v", err),
			IsError: true,
		}, nil
	}

	return &tool.Result{
		Content: string(contentBytes),
	}, nil
}

// findServerByPrompt 根据 prompt 名称查找匹配的服务器。
func (p *MCPPromptTool) findServerByPrompt(name string) string {
	for _, server := range p.manager.GetServers() {
		prompts, err := p.manager.Prompts(server.Name)
		if err != nil {
			continue
		}
		for _, pr := range prompts {
			if pr.Name == name {
				return server.Name
			}
		}
	}
	return ""
}
