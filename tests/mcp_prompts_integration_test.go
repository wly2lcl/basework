// Package tests 包含 basework 项目的集成测试。
package tests

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/wly2lcl/basework/pkg/mcp"
)

// ---------------------------------------------------------------------------
// Mock MCP Prompt Server — 通过子进程方式实现
// ---------------------------------------------------------------------------

// TestMCPPromptIntegrationMockHelper 是一个特殊的测试函数，
// 通过子进程方式充当提示模板模拟服务器。
// 通过 GO_MCP_PROMPT_INTEGRATION_MOCK=1 环境变量激活。
func TestMCPPromptIntegrationMockHelper(t *testing.T) {
	if os.Getenv("GO_MCP_PROMPT_INTEGRATION_MOCK") != "1" {
		t.Skip("not a mock helper process")
	}
	runPromptIntegrationMock()
}

func runPromptIntegrationMock() {
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
			result = json.RawMessage(`{"protocolVersion":"2024-11-05","serverInfo":{"name":"prompt-int-mock","version":"1.0.0"},"capabilities":{"prompts":{}}}`)
		case "tools/list":
			result = json.RawMessage(`{"tools":[]}`)
		case "resources/list":
			result = json.RawMessage(`{"resources":[]}`)
		case "prompts/list":
			result = json.RawMessage(`{"prompts":[
				{"name":"code-review","description":"Review Go code for best practices","arguments":[{"name":"code","description":"Go code to review","required":true}]},
				{"name":"explain","description":"Explain a concept","arguments":[{"name":"topic","description":"Topic to explain","required":true},{"name":"level","description":"Detail level: beginner/intermediate/expert"}]},
				{"name":"summarize","description":"Summarize text","arguments":[{"name":"text","description":"Text to summarize","required":true}]}
			]}`)
		case "prompts/get":
			var params struct {
				Name string `json:"name"`
			}
			if msg.Params != nil {
				json.Unmarshal(msg.Params, &params)
			}
			switch params.Name {
			case "code-review":
				result = json.RawMessage(`{"messages":[{"role":"user","content":{"type":"text","text":"Please review the following Go code for best practices, error handling, and performance."}}]}`)
			case "explain":
				result = json.RawMessage(`{"messages":[{"role":"user","content":{"type":"text","text":"Explain this topic in detail."}}]}`)
			default:
				sendErrorMock(os.Stdout, *msg.ID, -32602, "prompt not found")
				continue
			}
		default:
			sendErrorMock(os.Stdout, *msg.ID, -32601, "method not found")
			continue
		}

		sendResponseMock(os.Stdout, *msg.ID, result)
	}
}

// promptIntegrationTestConfig 返回适用于提示集成测试的 ServerConfig。
func promptIntegrationTestConfig() mcp.ServerConfig {
	return mcp.ServerConfig{
		Command: os.Args[0],
		Args:    []string{"-test.run=TestMCPPromptIntegrationMockHelper$"},
		Env:     map[string]string{"GO_MCP_PROMPT_INTEGRATION_MOCK": "1"},
	}
}

// ---------------------------------------------------------------------------
// Task 6.4: MCP 提示模板集成测试
// ---------------------------------------------------------------------------

// TestIntegration_MCPPrompts_ListPrompts 测试列出提示模板。
func TestIntegration_MCPPrompts_ListPrompts(t *testing.T) {
	mgr := mcp.NewManager()
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := mgr.Connect(ctx, "prompt-server", promptIntegrationTestConfig()); err != nil {
		t.Fatalf("Connect 失败: %v", err)
	}

	prompts, err := mgr.Prompts("prompt-server")
	if err != nil {
		t.Fatalf("Prompts() 失败: %v", err)
	}

	if len(prompts) != 3 {
		t.Fatalf("期望 3 个 prompt, 得到 %d", len(prompts))
	}

	// 验证第一个 prompt 的元数据
	if prompts[0].Name != "code-review" {
		t.Errorf("期望 Name='code-review', 得到 %q", prompts[0].Name)
	}
	if prompts[0].Description != "Review Go code for best practices" {
		t.Errorf("期望 Description='Review Go code for best practices', 得到 %q", prompts[0].Description)
	}
	if len(prompts[0].Arguments) != 1 {
		t.Fatalf("期望 1 个参数, 得到 %d", len(prompts[0].Arguments))
	}
	if !prompts[0].Arguments[0].Required {
		t.Error("期望 code 参数为 required")
	}

	// 验证第二个 prompt 的参数
	if len(prompts[1].Arguments) != 2 {
		t.Fatalf("期望 2 个参数, 得到 %d", len(prompts[1].Arguments))
	}
	if prompts[1].Arguments[0].Name != "topic" {
		t.Errorf("期望参数 name='topic', 得到 %q", prompts[1].Arguments[0].Name)
	}

	t.Logf("成功列出 %d 个提示模板", len(prompts))
}

// TestIntegration_MCPPrompts_GetPrompt 测试获取提示模板内容。
func TestIntegration_MCPPrompts_GetPrompt(t *testing.T) {
	mgr := mcp.NewManager()
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := mgr.Connect(ctx, "prompt-server", promptIntegrationTestConfig()); err != nil {
		t.Fatalf("Connect 失败: %v", err)
	}

	result, err := mgr.GetPrompt(ctx, "prompt-server", "code-review")
	if err != nil {
		t.Fatalf("GetPrompt 失败: %v", err)
	}

	if len(result.Messages) != 1 {
		t.Fatalf("期望 1 条消息, 得到 %d", len(result.Messages))
	}
	if result.Messages[0].Role != "user" {
		t.Errorf("期望 role='user', 得到 %q", result.Messages[0].Role)
	}
	if result.Truncated {
		t.Error("期望不截断")
	}

	t.Logf("成功获取提示模板 'code-review': %d 条消息", len(result.Messages))
}

// TestIntegration_MCPPrompts_GetPromptNotFound 测试获取不存在的提示模板。
func TestIntegration_MCPPrompts_GetPromptNotFound(t *testing.T) {
	mgr := mcp.NewManager()
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := mgr.Connect(ctx, "prompt-server", promptIntegrationTestConfig()); err != nil {
		t.Fatalf("Connect 失败: %v", err)
	}

	_, err := mgr.GetPrompt(ctx, "prompt-server", "nonexistent-prompt")
	if err == nil {
		t.Fatal("期望不存在的提示模板返回错误")
	}
	if !strings.Contains(err.Error(), "prompt not found") {
		t.Errorf("期望包含 'prompt not found', 得到 %v", err)
	}
}

// TestIntegration_MCPPrompts_ServerNotFound 测试不存在的服务器。
func TestIntegration_MCPPrompts_ServerNotFound(t *testing.T) {
	mgr := mcp.NewManager()
	defer mgr.Close()

	_, err := mgr.Prompts("nonexistent-server")
	if err == nil {
		t.Fatal("期望不存在的服务器返回错误")
	}

	_, err = mgr.GetPrompt(context.Background(), "nonexistent-server", "greeting")
	if err == nil {
		t.Fatal("期望不存在的服务器返回错误")
	}
}

// TestIntegration_MCPPrompts_MultipleServers 测试多服务器提示模板分组。
func TestIntegration_MCPPrompts_MultipleServers(t *testing.T) {
	mgr := mcp.NewManager()
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := mgr.Connect(ctx, "srv-a", promptIntegrationTestConfig()); err != nil {
		t.Fatalf("Connect srv-a: %v", err)
	}
	if err := mgr.Connect(ctx, "srv-b", promptIntegrationTestConfig()); err != nil {
		t.Fatalf("Connect srv-b: %v", err)
	}

	pA, err := mgr.Prompts("srv-a")
	if err != nil {
		t.Fatalf("Prompts srv-a: %v", err)
	}
	pB, err := mgr.Prompts("srv-b")
	if err != nil {
		t.Fatalf("Prompts srv-b: %v", err)
	}

	if len(pA) != 3 || len(pB) != 3 {
		t.Errorf("期望每个服务器 3 个 prompt, 得到 srv-a=%d, srv-b=%d", len(pA), len(pB))
	}
	t.Logf("多服务器提示测试通过: srv-a(%d), srv-b(%d)", len(pA), len(pB))
}

// TestIntegration_MCPPrompts_SizeLimit 测试提示模板大小限制。
func TestIntegration_MCPPrompts_SizeLimit(t *testing.T) {
	mgr := mcp.NewManager()
	defer mgr.Close()
	mgr.SetMaxPromptSize(1024) // 1KB 限制

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := mgr.Connect(ctx, "prompt-server", promptIntegrationTestConfig()); err != nil {
		t.Fatalf("Connect 失败: %v", err)
	}

	result, err := mgr.GetPrompt(ctx, "prompt-server", "code-review")
	if err != nil {
		t.Fatalf("GetPrompt 失败: %v", err)
	}

	// 小 prompt 不应被截断
	if result.Truncated {
		t.Error("预期 1KB 限制下小 prompt 不应截断")
	}
	t.Logf("提示模板大小限制测试通过: truncated=%v", result.Truncated)
}

// TestIntegration_MCPPrompts_ContextCancel 测试 context 取消。
func TestIntegration_MCPPrompts_ContextCancel(t *testing.T) {
	mgr := mcp.NewManager()
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := mgr.Connect(ctx, "prompt-server", promptIntegrationTestConfig()); err != nil {
		t.Fatalf("Connect 失败: %v", err)
	}

	cancelCtx, cancelFn := context.WithCancel(context.Background())
	cancelFn() // 立即取消

	_, err := mgr.GetPrompt(cancelCtx, "prompt-server", "code-review")
	if err == nil {
		t.Fatal("期望已取消的 context 返回错误")
	}
	t.Logf("Context 取消测试通过: %v", err)
}