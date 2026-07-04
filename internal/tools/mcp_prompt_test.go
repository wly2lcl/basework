// Package tools 测试
package tools

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/wly2lcl/basework/pkg/mcp"
)

// ---------------------------------------------------------------------------
// MCPPromptTool 单元测试
// ---------------------------------------------------------------------------

// TestMCPPromptToolName 测试工具名称。
func TestMCPPromptToolName(t *testing.T) {
	tool := NewMCPPromptTool(mcp.NewManager())
	defer tool.manager.Close()

	if tool.Name() != "mcp_prompt" {
		t.Errorf("expected 'mcp_prompt', got %q", tool.Name())
	}
}

// TestMCPPromptToolDescription 测试工具描述非空。
func TestMCPPromptToolDescription(t *testing.T) {
	tool := NewMCPPromptTool(mcp.NewManager())
	defer tool.manager.Close()

	if tool.Description() == "" {
		t.Error("expected non-empty description")
	}
}

// TestMCPPromptToolParameters 测试工具参数 Schema 包含 prompt 和 server 字段。
func TestMCPPromptToolParameters(t *testing.T) {
	tool := NewMCPPromptTool(mcp.NewManager())
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

	if len(schema.Required) != 1 || schema.Required[0] != "prompt" {
		t.Errorf("expected required ['prompt'], got %v", schema.Required)
	}
	if _, ok := schema.Properties["prompt"]; !ok {
		t.Error("expected 'prompt' property")
	}
	if _, ok := schema.Properties["server"]; !ok {
		t.Error("expected 'server' property")
	}
}

// TestMCPPromptToolExecuteMissingPrompt 测试缺少 prompt 参数返回错误。
func TestMCPPromptToolExecuteMissingPrompt(t *testing.T) {
	tool := NewMCPPromptTool(mcp.NewManager())
	defer tool.manager.Close()

	result, err := tool.Execute(nil, []byte(`{}`))
	if err != nil {
		t.Fatalf("Execute should not return error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError for missing prompt")
	}
	if !strings.Contains(result.Content, "prompt") {
		t.Errorf("expected error about 'prompt', got %q", result.Content)
	}
}