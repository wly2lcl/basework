package mcp

import (
	"context"
	"encoding/json"

	"github.com/wly2lcl/basework/pkg/tool"
)

// mcpToolAdapter 将 MCP 工具适配为 tool.Tool 接口。
type mcpToolAdapter struct {
	server  string   // 服务器名
	mcpTool mcpTool  // MCP 工具定义
	manager *Manager // 用于调用 CallTool
}

func (a *mcpToolAdapter) Name() string {
	return "mcp_" + a.server + "_" + a.mcpTool.Name
}

func (a *mcpToolAdapter) Description() string {
	return a.mcpTool.Description
}

func (a *mcpToolAdapter) Parameters() json.RawMessage {
	if a.mcpTool.InputSchema != nil {
		return a.mcpTool.InputSchema
	}
	return json.RawMessage(`{"type":"object","properties":{}}`)
}

func (a *mcpToolAdapter) Execute(ctx context.Context, args json.RawMessage) (*tool.Result, error) {
	return a.manager.CallTool(ctx, a.server, a.mcpTool.Name, args)
}
