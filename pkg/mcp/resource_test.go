package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Resource Mock MCP 服务器 — 通过子进程方式实现
// ---------------------------------------------------------------------------

// TestResourceMockHelper 是一个特殊的测试函数，通过子进程方式充当资源模拟服务器。
// 通过 GO_RESOURCE_MOCK=1 环境变量激活。
func TestResourceMockHelper(t *testing.T) {
	if os.Getenv("GO_RESOURCE_MOCK") != "1" {
		t.Skip("not a mock helper process")
	}
	runResourceMockServer()
}

func runResourceMockServer() {
	reader := bufio.NewReader(os.Stdin)
	for {
		contentLength, err := readHeadersMock(reader)
		if err != nil {
			return
		}
		if contentLength <= 0 {
			continue
		}

		body := make([]byte, contentLength)
		if _, err := io.ReadFull(reader, body); err != nil {
			return
		}

		var msg struct {
			ID     *int            `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(body, &msg); err != nil {
			continue
		}

		if msg.ID == nil {
			continue
		}

		var result json.RawMessage
		switch msg.Method {
		case "initialize":
			result = json.RawMessage(`{"protocolVersion":"2024-11-05","serverInfo":{"name":"resource-mock","version":"1.0.0"},"capabilities":{"resources":{}}}`)
		case "tools/list":
			result = json.RawMessage(`{"tools":[]}`)
		case "resources/list":
			result = json.RawMessage(`{"resources":[{"uri":"file:///test/doc.txt","name":"Test Document","description":"A test resource","mimeType":"text/plain"},{"uri":"file:///test/data.json","name":"Test Data","mimeType":"application/json","size":1024}]}`)
		case "resources/read":
			var params struct {
				URI string `json:"uri"`
			}
			if msg.Params != nil {
				json.Unmarshal(msg.Params, &params)
			}
			if params.URI == "file:///test/doc.txt" {
				result = json.RawMessage(`{"contents":[{"uri":"file:///test/doc.txt","mimeType":"text/plain","text":"Hello, this is test content."}]}`)
			} else if params.URI == "file:///test/large.txt" {
				largeText := strings.Repeat("A", 20*1024*1024) // 20MB
				result = []byte(fmt.Sprintf(`{"contents":[{"uri":"file:///test/large.txt","mimeType":"text/plain","text":%q}]}`, largeText))
			} else {
				sendErrorMock(os.Stdout, *msg.ID, -32602, "resource not found")
				continue
			}
		case "prompts/list":
			result = json.RawMessage(`{"prompts":[]}`)
		default:
			sendErrorMock(os.Stdout, *msg.ID, -32601, "method not found")
			continue
		}

		sendResponseMock(os.Stdout, *msg.ID, result)
	}
}

// resourceTestTransport 创建一个使用 Resource mock 子进程的 StdioTransport。
func resourceTestTransport() *StdioTransport {
	return NewStdioTransport(
		os.Args[0],
		[]string{"-test.run=TestResourceMockHelper$"},
		map[string]string{"GO_RESOURCE_MOCK": "1"},
	)
}

// resourceTestConfig 返回适用于资源测试的 ServerConfig。
func resourceTestConfig() ServerConfig {
	return ServerConfig{
		Command: os.Args[0],
		Args:    []string{"-test.run=TestResourceMockHelper$"},
		Env:     map[string]string{"GO_RESOURCE_MOCK": "1"},
	}
}

// setupManagerWithResourceMock 创建一个 Manager 并连接一个资源 mock 服务器。
func setupManagerWithResourceMock(t *testing.T, name string) *Manager {
	t.Helper()
	mgr := NewManager()
	if err := mgr.Connect(context.Background(), name, resourceTestConfig()); err != nil {
		t.Fatalf("Connect(%q): %v", name, err)
	}
	return mgr
}

// ---------------------------------------------------------------------------
// Resource 单元测试
// ---------------------------------------------------------------------------

// TestResourcesList 测试 resources/list 成功获取资源列表。
func TestResourcesList(t *testing.T) {
	mgr := setupManagerWithResourceMock(t, "resource-server")
	defer mgr.Close()

	resources, err := mgr.Resources("resource-server")
	if err != nil {
		t.Fatalf("Resources() failed: %v", err)
	}

	if len(resources) != 2 {
		t.Fatalf("expected 2 resources, got %d", len(resources))
	}
	if resources[0].URI != "file:///test/doc.txt" {
		t.Errorf("expected URI 'file:///test/doc.txt', got %q", resources[0].URI)
	}
	if resources[0].Name != "Test Document" {
		t.Errorf("expected Name 'Test Document', got %q", resources[0].Name)
	}
	if resources[0].MimeType != "text/plain" {
		t.Errorf("expected MimeType 'text/plain', got %q", resources[0].MimeType)
	}
}

// TestResourcesListNotFound 测试不支持的服务器 graceful degradation。
func TestResourcesListNotFound(t *testing.T) {
	mgr := setupManagerWithMock(t, "basic-server") // 标准 mock 不支持 resources/list
	defer mgr.Close()

	// Connect 应该成功（降级）
	server, ok := mgr.GetServer("basic-server")
	if !ok {
		t.Fatal("expected server to be registered")
	}
	// 资源列表应为空
	if len(server.Resources) != 0 {
		t.Errorf("expected 0 resources for unsupporting server, got %d", len(server.Resources))
	}
}

// TestResourcesListMultiple 测试多服务器资源分组。
func TestResourcesListMultiple(t *testing.T) {
	mgr := NewManager()
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := mgr.Connect(ctx, "srv1", resourceTestConfig()); err != nil {
		t.Fatalf("Connect srv1: %v", err)
	}
	if err := mgr.Connect(ctx, "srv2", resourceTestConfig()); err != nil {
		t.Fatalf("Connect srv2: %v", err)
	}

	res1, err := mgr.Resources("srv1")
	if err != nil {
		t.Fatalf("Resources srv1: %v", err)
	}
	res2, err := mgr.Resources("srv2")
	if err != nil {
		t.Fatalf("Resources srv2: %v", err)
	}

	if len(res1) != 2 || len(res2) != 2 {
		t.Errorf("expected 2 resources each, got %d and %d", len(res1), len(res2))
	}
}

// TestReadResource 测试 resources/read 读取资源内容。
func TestReadResource(t *testing.T) {
	mgr := setupManagerWithResourceMock(t, "resource-server")
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := mgr.ReadResource(ctx, "resource-server", "file:///test/doc.txt")
	if err != nil {
		t.Fatalf("ReadResource failed: %v", err)
	}

	if len(result.Contents) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(result.Contents))
	}
	if result.Contents[0].Text != "Hello, this is test content." {
		t.Errorf("expected 'Hello, this is test content.', got %q", result.Contents[0].Text)
	}
	if result.Truncated {
		t.Error("expected not truncated")
	}
}

// TestReadResourceNotFound 测试读取不存在的资源返回错误。
func TestReadResourceNotFound(t *testing.T) {
	mgr := setupManagerWithResourceMock(t, "resource-server")
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := mgr.ReadResource(ctx, "resource-server", "file:///test/nonexistent.txt")
	if err == nil {
		t.Fatal("expected error for nonexistent resource")
	}
	if !strings.Contains(err.Error(), "resource not found") {
		t.Errorf("expected 'resource not found' in error, got %v", err)
	}
}

// TestReadResourceServerNotFound 测试服务器不存在时返回错误。
func TestReadResourceServerNotFound(t *testing.T) {
	mgr := NewManager()
	defer mgr.Close()

	ctx := context.Background()
	_, err := mgr.ReadResource(ctx, "nonexistent", "file:///test/doc.txt")
	if err == nil {
		t.Fatal("expected error for nonexistent server")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got %v", err)
	}
}

// TestReadResourceAfterClose 测试 Manager 关闭后读取资源返回错误。
func TestReadResourceAfterClose(t *testing.T) {
	mgr := setupManagerWithResourceMock(t, "resource-server")
	mgr.Close()

	ctx := context.Background()
	_, err := mgr.ReadResource(ctx, "resource-server", "file:///test/doc.txt")
	if err == nil {
		t.Fatal("expected error after close")
	}
	if !strings.Contains(err.Error(), "manager is closed") {
		t.Errorf("expected 'manager is closed', got %v", err)
	}
}

// TestReadResourceContextCancel 测试 context 取消。
func TestReadResourceContextCancel(t *testing.T) {
	mgr := setupManagerWithResourceMock(t, "resource-server")
	defer mgr.Close()

	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := mgr.ReadResource(cancelCtx, "resource-server", "file:///test/doc.txt")
	if err == nil {
		t.Fatal("expected error with cancelled context")
	}
}

// TestReadResourceSizeLimitNormal 测试正常大小资源（不截断）。
func TestReadResourceSizeLimitNormal(t *testing.T) {
	mgr := setupManagerWithResourceMock(t, "resource-server")
	defer mgr.Close()
	mgr.SetMaxResourceSize(10 * 1024 * 1024) // 10MB

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := mgr.ReadResource(ctx, "resource-server", "file:///test/doc.txt")
	if err != nil {
		t.Fatalf("ReadResource failed: %v", err)
	}

	if result.Truncated {
		t.Error("expected no truncation for small content")
	}
	if result.Contents[0].Text != "Hello, this is test content." {
		t.Errorf("expected full content, got truncated")
	}
}

// TestReadResourceSizeLimitOverflow 测试超大资源截断。
func TestReadResourceSizeLimitOverflow(t *testing.T) {
	mgr := setupManagerWithResourceMock(t, "resource-server")
	defer mgr.Close()
	mgr.SetMaxResourceSize(1024) // 1KB 限制

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := mgr.ReadResource(ctx, "resource-server", "file:///test/large.txt")
	if err != nil {
		t.Fatalf("ReadResource failed: %v", err)
	}

	if !result.Truncated {
		t.Error("expected truncation for large content")
	}
	if len(result.Contents[0].Text) > 1024 {
		t.Errorf("expected truncated content <= 1024 bytes, got %d", len(result.Contents[0].Text))
	}
}

// TestReadResourceSizeLimitDisabled 测试大小限制为 0（不限制）。
func TestReadResourceSizeLimitDisabled(t *testing.T) {
	mgr := setupManagerWithResourceMock(t, "resource-server")
	defer mgr.Close()
	mgr.SetMaxResourceSize(0) // 不限制

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := mgr.ReadResource(ctx, "resource-server", "file:///test/large.txt")
	if err != nil {
		t.Fatalf("ReadResource failed: %v", err)
	}

	if result.Truncated {
		t.Error("expected no truncation when limit is 0")
	}
}

// TestApplyResourceContentLimitNil 测试 applyResourceContentLimit nil 安全。
func TestApplyResourceContentLimitNil(t *testing.T) {
	result := applyResourceContentLimit(nil, 1024)
	if result != nil {
		t.Error("expected nil for nil input")
	}
}

// TestReadResourceCrossServer 测试跨服务器 URI 路由。
func TestReadResourceCrossServer(t *testing.T) {
	// 使用 mock 资源服务器验证读取
	mgr := setupManagerWithResourceMock(t, "srv-a")
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := mgr.ReadResource(ctx, "srv-a", "file:///test/doc.txt")
	if err != nil {
		t.Fatalf("ReadResource failed: %v", err)
	}
	if len(result.Contents) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(result.Contents))
	}
	if result.Contents[0].Text != "Hello, this is test content." {
		t.Errorf("unexpected content: %q", result.Contents[0].Text)
	}
}
