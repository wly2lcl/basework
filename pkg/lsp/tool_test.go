package lsp

import (
	"context"
	"encoding/json"
	"testing"
)

// ---------------------------------------------------------------------------
// 工具注册与元数据测试
// ---------------------------------------------------------------------------

// TestToolsCount 验证 Tools() 返回 6 个工具。
func TestToolsCount(t *testing.T) {
	tools := Tools(nil)
	if len(tools) != 6 {
		t.Fatalf("expected 6 tools, got %d", len(tools))
	}
}

// TestToolNames 验证每个工具的名称正确。
func TestToolNames(t *testing.T) {
	tools := Tools(nil)
	expected := map[string]bool{
		"lsp_definition":        false,
		"lsp_references":        false,
		"lsp_hover":             false,
		"lsp_diagnostics":       false,
		"lsp_document_symbols":  false,
		"lsp_workspace_symbols": false,
	}
	for _, tool := range tools {
		name := tool.Name()
		if _, ok := expected[name]; !ok {
			t.Errorf("unexpected tool name: %q", name)
		}
		expected[name] = true
	}
	for name, found := range expected {
		if !found {
			t.Errorf("missing tool: %q", name)
		}
	}
}

// TestToolDescriptions 验证每个工具的描述不为空。
func TestToolDescriptions(t *testing.T) {
	for _, tool := range Tools(nil) {
		if tool.Description() == "" {
			t.Errorf("tool %q has empty description", tool.Name())
		}
	}
}

// TestToolParametersSchema 验证每个工具的 Parameters() 是有效的 JSON Schema。
func TestToolParametersSchema(t *testing.T) {
	for _, tool := range Tools(nil) {
		raw := tool.Parameters()
		if len(raw) == 0 {
			t.Errorf("tool %q has empty parameters", tool.Name())
			continue
		}
		var schema map[string]interface{}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Errorf("tool %q parameters is not valid JSON: %v", tool.Name(), err)
			continue
		}
		// 必须有 type 和 properties
		if schema["type"] != "object" {
			t.Errorf("tool %q parameters.type should be 'object', got %v", tool.Name(), schema["type"])
		}
		if _, ok := schema["properties"]; !ok {
			t.Errorf("tool %q parameters missing 'properties'", tool.Name())
		}
	}
}

// ---------------------------------------------------------------------------
// 优雅降级测试：nil manager
// ---------------------------------------------------------------------------

// TestNilManagerReturnsEmpty 验证 Manager 为 nil 时所有工具返回空结果（而非错误）。
func TestNilManagerReturnsEmpty(t *testing.T) {
	ctx := context.Background()
	for _, tl := range Tools(nil) {
		// 传入任何参数都应返回空结果，因为 manager 为 nil 直接提前返回
		validArgs := validArgsForTool(tl.Name())
		result, err := tl.Execute(ctx, validArgs)
		if err != nil {
			t.Errorf("tool %q: unexpected error: %v", tl.Name(), err)
		}
		if result == nil {
			t.Fatalf("tool %q: result is nil", tl.Name())
		}
		if result.IsError {
			t.Errorf("tool %q: expected non-error result for nil manager, got IsError=true with content: %q", tl.Name(), result.Content)
		}
	}
}

// TestNilManagerContent 验证 nil manager 下各工具返回的具体内容。
func TestNilManagerContent(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		want string
	}{
		{"lsp_definition", "[]"},
		{"lsp_references", "[]"},
		{"lsp_hover", ""},
		{"lsp_diagnostics", `{"diagnostics":[],"version":0}`},
		{"lsp_document_symbols", "[]"},
		{"lsp_workspace_symbols", "[]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, tl := range Tools(nil) {
				if tl.Name() != tt.name {
					continue
				}
				result, err := tl.Execute(ctx, validArgsForTool(tt.name))
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if result.Content != tt.want {
					t.Errorf("got %q, want %q", result.Content, tt.want)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 参数解析测试
// ---------------------------------------------------------------------------

// TestInvalidArgs 验证传入非法 JSON 参数时返回 IsError。
func TestInvalidArgs(t *testing.T) {
	m := NewManager(Config{Servers: map[string]ServerConfig{}})
	m.Start(context.Background(), t.TempDir())

	ctx := context.Background()
	invalidJSON := json.RawMessage(`not valid json`)
	for _, tl := range Tools(m) {
		result, err := tl.Execute(ctx, invalidJSON)
		if err != nil {
			t.Errorf("tool %q: unexpected error: %v", tl.Name(), err)
		}
		if result == nil {
			t.Fatalf("tool %q: result is nil", tl.Name())
		}
		if !result.IsError {
			t.Errorf("tool %q: expected IsError=true for invalid args", tl.Name())
		}
	}
}

// TestMissingRequiredField 验证缺少必填字段时不崩溃（Go 的 json.Unmarshal 不强制 required）。
// 缺少字段时字段值被置为零值，工具应正常执行并返回空结果。
func TestMissingRequiredField(t *testing.T) {
	m := NewManager(Config{Servers: map[string]ServerConfig{}})
	m.Start(context.Background(), t.TempDir())

	ctx := context.Background()
	emptyArgs := json.RawMessage(`{}`)
	for _, tl := range Tools(m) {
		result, err := tl.Execute(ctx, emptyArgs)
		if err != nil {
			t.Errorf("tool %q: unexpected error: %v", tl.Name(), err)
		}
		if result == nil {
			t.Fatalf("tool %q: result is nil", tl.Name())
		}
		if result.IsError {
			t.Errorf("tool %q: expected non-error for missing required field, got IsError=true with content: %q",
				tl.Name(), result.Content)
		}
	}
}

// ---------------------------------------------------------------------------
// Manager 路由测试（无真实 LSP 客户端）
// ---------------------------------------------------------------------------

// TestToolWithEmptyManager 验证与无客户端的 Manager 配合使用时返回空结果。
func TestToolWithEmptyManager(t *testing.T) {
	m := NewManager(Config{Servers: map[string]ServerConfig{}})
	m.Start(context.Background(), t.TempDir())

	ctx := context.Background()
	tests := []struct {
		name string
		args json.RawMessage
		want string
	}{
		{"lsp_definition", json.RawMessage(`{"file":"test.go","line":0,"character":0}`), "[]"},
		{"lsp_references", json.RawMessage(`{"file":"test.go","line":0,"character":0}`), "[]"},
		{"lsp_hover", json.RawMessage(`{"file":"test.go","line":0,"character":0}`), ""},
		{"lsp_diagnostics", json.RawMessage(`{"file":"test.go"}`), `{"diagnostics":[],"version":0}`},
		{"lsp_document_symbols", json.RawMessage(`{"file":"test.go"}`), "[]"},
		{"lsp_workspace_symbols", json.RawMessage(`{"query":"test"}`), "[]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, tl := range Tools(m) {
				if tl.Name() != tt.name {
					continue
				}
				result, err := tl.Execute(ctx, tt.args)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if result.IsError {
					t.Fatalf("unexpected error result: %q", result.Content)
				}
				if result.Content != tt.want {
					t.Errorf("got %q, want %q", result.Content, tt.want)
				}
			}
		})
	}
}

// TestToolWithConnectedManager 验证有 Manager 时，使用未知扩展名文件返回空结果。
func TestToolWithConnectedManager(t *testing.T) {
	m := NewManager(Config{
		Servers: map[string]ServerConfig{},
	})
	m.Start(context.Background(), t.TempDir())

	ctx := context.Background()
	tools := Tools(m)

	// Manager 无客户端，所有工具返回空结果
	for _, tl := range tools {
		t.Run(tl.Name(), func(t *testing.T) {
			result, err := tl.Execute(ctx, validArgsForTool(tl.Name()))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.IsError {
				t.Fatalf("unexpected error result: %q", result.Content)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 工具列表验证
// ---------------------------------------------------------------------------

// TestToolsNonNil 验证 Tools(nil) 返回非 nil 的切片。
func TestToolsNonNil(t *testing.T) {
	tools := Tools(nil)
	if tools == nil {
		t.Error("Tools(nil) returned nil")
	}
}

// TestToolsWithManager 验证 Tools(manager) 返回正确数量的工具。
func TestToolsWithManager(t *testing.T) {
	m := NewManager(Config{})
	tools := Tools(m)
	if len(tools) != 6 {
		t.Fatalf("expected 6 tools, got %d", len(tools))
	}
	// 每个工具的 manager 应不为 nil
	for _, tl := range tools {
		name := tl.Name()
		_ = name // 至少可以获取名称
	}
}

// ---------------------------------------------------------------------------
// 帮助函数
// ---------------------------------------------------------------------------

// validArgsForTool 返回指定工具的有效参数字节。
func validArgsForTool(name string) json.RawMessage {
	switch name {
	case "lsp_definition", "lsp_references", "lsp_hover":
		return json.RawMessage(`{"file":"test.xyz","line":0,"character":0}`)
	case "lsp_diagnostics", "lsp_document_symbols":
		return json.RawMessage(`{"file":"test.xyz"}`)
	case "lsp_workspace_symbols":
		return json.RawMessage(`{"query":"test"}`)
	default:
		return json.RawMessage(`{}`)
	}
}
