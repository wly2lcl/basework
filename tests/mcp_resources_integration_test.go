// Package tests 包含 basework 项目的集成测试。
package tests

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

	"github.com/wly2lcl/basework/pkg/mcp"
)

// ---------------------------------------------------------------------------
// Mock MCP Resource Server — 通过子进程方式实现
// ---------------------------------------------------------------------------

// TestMCPResourceIntegrationMockHelper 是一个特殊的测试函数，
// 通过子进程方式充当资源模拟服务器。
// 通过 GO_MCP_RESOURCE_INTEGRATION_MOCK=1 环境变量激活。
func TestMCPResourceIntegrationMockHelper(t *testing.T) {
	if os.Getenv("GO_MCP_RESOURCE_INTEGRATION_MOCK") != "1" {
		t.Skip("not a mock helper process")
	}
	runResourceIntegrationMock()
}

func runResourceIntegrationMock() {
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
			result = json.RawMessage(`{"protocolVersion":"2024-11-05","serverInfo":{"name":"resource-int-mock","version":"1.0.0"},"capabilities":{"resources":{}}}`)
		case "tools/list":
			result = json.RawMessage(`{"tools":[]}`)
		case "resources/list":
			result = json.RawMessage(`{"resources":[
				{"uri":"file:///project/README.md","name":"Project README","description":"Project documentation","mimeType":"text/markdown"},
				{"uri":"file:///project/config.json","name":"Configuration","mimeType":"application/json","size":2048},
				{"uri":"file:///project/src/main.go","name":"Main source","mimeType":"text/x-go","size":4096}
			]}`)
		case "resources/read":
			var params struct {
				URI string `json:"uri"`
			}
			if msg.Params != nil {
				json.Unmarshal(msg.Params, &params)
			}
			switch params.URI {
			case "file:///project/README.md":
				result = json.RawMessage(`{"contents":[{"uri":"file:///project/README.md","mimeType":"text/markdown","text":"# Project\n\nThis is a test project."}]}`)
			case "file:///project/config.json":
				result = json.RawMessage(`{"contents":[{"uri":"file:///project/config.json","mimeType":"application/json","text":"{\"name\":\"test\",\"version\":\"1.0.0\"}"}]}`)
			default:
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

// resourceIntegrationTestConfig 返回适用于资源集成测试的 ServerConfig。
func resourceIntegrationTestConfig() mcp.ServerConfig {
	return mcp.ServerConfig{
		Command: os.Args[0],
		Args:    []string{"-test.run=TestMCPResourceIntegrationMockHelper$"},
		Env:     map[string]string{"GO_MCP_RESOURCE_INTEGRATION_MOCK": "1"},
	}
}

// ---------------------------------------------------------------------------
// 辅助函数（从 mcp/transport_test.go 复制）
// ---------------------------------------------------------------------------

func readHeadersMock(reader *bufio.Reader) (int, error) {
	var contentLength int
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return 0, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length:") {
			n, err := fmt.Sscanf(line, "Content-Length: %d", &contentLength)
			if err == nil && n == 1 {
				// already set
			}
		}
	}
	return contentLength, nil
}

func sendResponseMock(w io.Writer, id int, result json.RawMessage) {
	resp := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	}
	data, _ := json.Marshal(resp)
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	w.Write([]byte(header))
	w.Write(data)
}

func sendErrorMock(w io.Writer, id, code int, message string) {
	resp := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
		},
	}
	data, _ := json.Marshal(resp)
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	w.Write([]byte(header))
	w.Write(data)
}

// ---------------------------------------------------------------------------
// Task 6.3: MCP 资源集成测试
// ---------------------------------------------------------------------------

// TestIntegration_MCPResources_ListResources 测试列出 MCP 资源。
func TestIntegration_MCPResources_ListResources(t *testing.T) {
	mgr := mcp.NewManager()
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := mgr.Connect(ctx, "resource-server", resourceIntegrationTestConfig()); err != nil {
		t.Fatalf("Connect 失败: %v", err)
	}

	resources, err := mgr.Resources("resource-server")
	if err != nil {
		t.Fatalf("Resources() 失败: %v", err)
	}

	if len(resources) != 3 {
		t.Fatalf("期望 3 个资源, 得到 %d", len(resources))
	}

	// 验证资源列表
	expectedURIs := []string{
		"file:///project/README.md",
		"file:///project/config.json",
		"file:///project/src/main.go",
	}
	for i, uri := range expectedURIs {
		if resources[i].URI != uri {
			t.Errorf("资源 %d 期望 URI=%q, 得到 %q", i, uri, resources[i].URI)
		}
	}

	t.Logf("成功列出 %d 个资源", len(resources))
}

// TestIntegration_MCPResources_ReadResource 测试读取资源内容。
func TestIntegration_MCPResources_ReadResource(t *testing.T) {
	mgr := mcp.NewManager()
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := mgr.Connect(ctx, "resource-server", resourceIntegrationTestConfig()); err != nil {
		t.Fatalf("Connect 失败: %v", err)
	}

	result, err := mgr.ReadResource(ctx, "resource-server", "file:///project/README.md")
	if err != nil {
		t.Fatalf("ReadResource 失败: %v", err)
	}

	if len(result.Contents) != 1 {
		t.Fatalf("期望 1 个 content item, 得到 %d", len(result.Contents))
	}

	expectedText := "# Project\n\nThis is a test project."
	if result.Contents[0].Text != expectedText {
		t.Errorf("期望文本=%q, 得到 %q", expectedText, result.Contents[0].Text)
	}
	if result.Truncated {
		t.Error("期望不截断")
	}

	t.Logf("成功读取资源: %s (%d bytes)", result.Contents[0].URI, len(result.Contents[0].Text))
}

// TestIntegration_MCPResources_ReadResourceNotFound 测试读取不存在的资源。
func TestIntegration_MCPResources_ReadResourceNotFound(t *testing.T) {
	mgr := mcp.NewManager()
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := mgr.Connect(ctx, "resource-server", resourceIntegrationTestConfig()); err != nil {
		t.Fatalf("Connect 失败: %v", err)
	}

	_, err := mgr.ReadResource(ctx, "resource-server", "file:///project/nonexistent.txt")
	if err == nil {
		t.Fatal("期望读取不存在的资源返回错误")
	}
	if !strings.Contains(err.Error(), "resource not found") {
		t.Errorf("期望包含 'resource not found', 得到 %v", err)
	}
}

// TestIntegration_MCPResources_ServerNotFound 测试不存在的服务器。
func TestIntegration_MCPResources_ServerNotFound(t *testing.T) {
	mgr := mcp.NewManager()
	defer mgr.Close()

	_, err := mgr.Resources("nonexistent-server")
	if err == nil {
		t.Fatal("期望不存在的服务器返回错误")
	}

	_, err = mgr.ReadResource(context.Background(), "nonexistent-server", "file:///test/doc.txt")
	if err == nil {
		t.Fatal("期望不存在的服务器返回错误")
	}
}

// TestIntegration_MCPResources_MultipleServers 测试多服务器资源分组。
func TestIntegration_MCPResources_MultipleServers(t *testing.T) {
	mgr := mcp.NewManager()
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := mgr.Connect(ctx, "srv-a", resourceIntegrationTestConfig()); err != nil {
		t.Fatalf("Connect srv-a: %v", err)
	}
	if err := mgr.Connect(ctx, "srv-b", resourceIntegrationTestConfig()); err != nil {
		t.Fatalf("Connect srv-b: %v", err)
	}

	resA, err := mgr.Resources("srv-a")
	if err != nil {
		t.Fatalf("Resources srv-a: %v", err)
	}
	resB, err := mgr.Resources("srv-b")
	if err != nil {
		t.Fatalf("Resources srv-b: %v", err)
	}

	if len(resA) != 3 || len(resB) != 3 {
		t.Errorf("期望每个服务器 3 个资源, 得到 srv-a=%d, srv-b=%d", len(resA), len(resB))
	}
	t.Logf("多服务器资源测试通过: srv-a(%d), srv-b(%d)", len(resA), len(resB))
}

// TestIntegration_MCPResources_SizeLimit 测试资源大小限制。
func TestIntegration_MCPResources_SizeLimit(t *testing.T) {
	mgr := mcp.NewManager()
	defer mgr.Close()
	mgr.SetMaxResourceSize(100) // 100 字节限制

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := mgr.Connect(ctx, "resource-server", resourceIntegrationTestConfig()); err != nil {
		t.Fatalf("Connect 失败: %v", err)
	}

	result, err := mgr.ReadResource(ctx, "resource-server", "file:///project/config.json")
	if err != nil {
		t.Fatalf("ReadResource 失败: %v", err)
	}

	if len(result.Contents) > 0 && len(result.Contents[0].Text) > 100 {
		t.Errorf("期望截断后不超过 100 字节, 得到 %d 字节", len(result.Contents[0].Text))
	}
	t.Logf("资源大小限制测试通过: truncated=%v", result.Truncated)
}