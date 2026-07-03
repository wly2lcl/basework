package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// mcpToolAdapter 单元测试
// ---------------------------------------------------------------------------

// TestAdapterName 验证适配器名称格式 mcp_{server}_{tool}。
func TestAdapterName(t *testing.T) {
	adapter := &mcpToolAdapter{
		server: "my-server",
		mcpTool: mcpTool{
			Name: "my_tool",
		},
	}
	expected := "mcp_my-server_my_tool"
	if adapter.Name() != expected {
		t.Errorf("expected %q, got %q", expected, adapter.Name())
	}
}

// TestAdapterNameSpecialChars 验证名称中特殊字符的处理。
func TestAdapterNameSpecialChars(t *testing.T) {
	adapter := &mcpToolAdapter{
		server: "server-123",
		mcpTool: mcpTool{
			Name: "get_data",
		},
	}
	expected := "mcp_server-123_get_data"
	if adapter.Name() != expected {
		t.Errorf("expected %q, got %q", expected, adapter.Name())
	}
}

// TestAdapterDescription 验证描述透传。
func TestAdapterDescription(t *testing.T) {
	adapter := &mcpToolAdapter{
		mcpTool: mcpTool{
			Description: "A useful test tool",
		},
	}
	if adapter.Description() != "A useful test tool" {
		t.Errorf("expected 'A useful test tool', got %q", adapter.Description())
	}
}

// TestAdapterDescriptionEmpty 验证空描述。
func TestAdapterDescriptionEmpty(t *testing.T) {
	adapter := &mcpToolAdapter{
		mcpTool: mcpTool{},
	}
	if adapter.Description() != "" {
		t.Errorf("expected empty description, got %q", adapter.Description())
	}
}

// TestAdapterParameters 验证 inputSchema 透传。
func TestAdapterParameters(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`)
	adapter := &mcpToolAdapter{
		mcpTool: mcpTool{
			InputSchema: schema,
		},
	}
	params := adapter.Parameters()
	if string(params) != string(schema) {
		t.Errorf("expected %s, got %s", string(schema), string(params))
	}
}

// TestAdapterParametersNil 验证 InputSchema 为 nil 时返回默认 schema。
func TestAdapterParametersNil(t *testing.T) {
	adapter := &mcpToolAdapter{
		mcpTool: mcpTool{}, // InputSchema 为 nil
	}
	params := adapter.Parameters()
	expected := `{"type":"object","properties":{}}`
	if string(params) != expected {
		t.Errorf("expected %s, got %s", expected, string(params))
	}
}

// TestAdapterExecute 测试 Execute 通过 manager 调用工具。
func TestAdapterExecute(t *testing.T) {
	mgr := setupManagerWithMock(t, "test-server")
	defer mgr.Close()

	adapter := &mcpToolAdapter{
		server:  "test-server",
		mcpTool: mcpTool{Name: "test_tool"},
		manager: mgr,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	args := json.RawMessage(`{"query":"hello"}`)
	result, err := adapter.Execute(ctx, args)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.Content != "mock result" {
		t.Errorf("expected 'mock result', got %q", result.Content)
	}
	if result.IsError {
		t.Error("expected IsError to be false")
	}
}

// TestAdapterExecuteNilArgs 测试 Execute 传递 nil 参数。
func TestAdapterExecuteNilArgs(t *testing.T) {
	mgr := setupManagerWithMock(t, "test-server")
	defer mgr.Close()

	adapter := &mcpToolAdapter{
		server:  "test-server",
		mcpTool: mcpTool{Name: "test_tool"},
		manager: mgr,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := adapter.Execute(ctx, nil)
	if err != nil {
		t.Fatalf("Execute with nil args failed: %v", err)
	}

	if result.Content != "mock result" {
		t.Errorf("expected 'mock result', got %q", result.Content)
	}
}

// TestAdapterExecuteAfterClose 测试 Manager 关闭后 Execute 返回错误。
func TestAdapterExecuteAfterClose(t *testing.T) {
	mgr := setupManagerWithMock(t, "test-server")
	mgr.Close()

	adapter := &mcpToolAdapter{
		server:  "test-server",
		mcpTool: mcpTool{Name: "test_tool"},
		manager: mgr,
	}

	_, err := adapter.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error after manager close")
	}
	if !strings.Contains(err.Error(), "manager is closed") {
		t.Errorf("expected 'manager is closed', got %v", err)
	}
}

// TestAdapterExecuteServerNotFound 测试 Execute 调用不存在的服务器。
func TestAdapterExecuteServerNotFound(t *testing.T) {
	mgr := NewManager()
	defer mgr.Close()

	adapter := &mcpToolAdapter{
		server:  "nonexistent",
		mcpTool: mcpTool{Name: "test_tool"},
		manager: mgr,
	}

	_, err := adapter.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nonexistent server")
	}
}

// TestAdapterConcurrentExecute 测试并发 Execute 调用。
func TestAdapterConcurrentExecute(t *testing.T) {
	mgr := setupManagerWithMock(t, "test-server")
	defer mgr.Close()

	adapter := &mcpToolAdapter{
		server:  "test-server",
		mcpTool: mcpTool{Name: "test_tool"},
		manager: mgr,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	errCh := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_, err := adapter.Execute(ctx, json.RawMessage(`{"query":"test"}`))
			if err != nil {
				errCh <- err
			}
		}()
	}

	// 收集错误
	for i := 0; i < 10; i++ {
		select {
		case err := <-errCh:
			t.Fatalf("concurrent execute failed: %v", err)
		default:
		}
	}
}