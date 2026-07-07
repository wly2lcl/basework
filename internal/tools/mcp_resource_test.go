// Package tools 测试
package tools

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/wly2lcl/basework/pkg/mcp"
)

// ---------------------------------------------------------------------------
// MCPReadTool 单元测试
// ---------------------------------------------------------------------------

// TestMCPReadToolName 测试工具名称。
func TestMCPReadToolName(t *testing.T) {
	tool := NewMCPReadTool(mcp.NewManager())
	defer tool.manager.Close()

	if tool.Name() != "mcp_read" {
		t.Errorf("expected 'mcp_read', got %q", tool.Name())
	}
}

// TestMCPReadToolDescription 测试工具描述非空。
func TestMCPReadToolDescription(t *testing.T) {
	tool := NewMCPReadTool(mcp.NewManager())
	defer tool.manager.Close()

	if tool.Description() == "" {
		t.Error("expected non-empty description")
	}
}

// TestMCPReadToolParameters 测试工具参数 Schema 包含 resource 和 server 字段。
func TestMCPReadToolParameters(t *testing.T) {
	tool := NewMCPReadTool(mcp.NewManager())
	defer tool.manager.Close()

	params := tool.Parameters()

	var schema struct {
		Required   []string `json:"required"`
		Properties map[string]struct {
			Type string `json:"type"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(params, &schema); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}

	if len(schema.Required) != 1 || schema.Required[0] != "resource" {
		t.Errorf("expected required ['resource'], got %v", schema.Required)
	}
	if _, ok := schema.Properties["resource"]; !ok {
		t.Error("expected 'resource' property")
	}
	if _, ok := schema.Properties["server"]; !ok {
		t.Error("expected 'server' property")
	}
}

// TestMCPReadToolExecuteMissingResource 测试缺少 resource 参数返回错误。
func TestMCPReadToolExecuteMissingResource(t *testing.T) {
	tool := NewMCPReadTool(mcp.NewManager())
	defer tool.manager.Close()

	result, err := tool.Execute(nil, []byte(`{}`))
	if err != nil {
		t.Fatalf("Execute should not return error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError for missing resource")
	}
	if !strings.Contains(result.Content, "resource") {
		t.Errorf("expected error about 'resource', got %q", result.Content)
	}
}
