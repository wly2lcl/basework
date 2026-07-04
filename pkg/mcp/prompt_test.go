package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Prompt Mock MCP 服务器 — 通过子进程方式实现
// ---------------------------------------------------------------------------

// TestPromptMockHelper 是一个特殊的测试函数，通过子进程方式充当提示模拟服务器。
// 通过 GO_PROMPT_MOCK=1 环境变量激活。
func TestPromptMockHelper(t *testing.T) {
	if os.Getenv("GO_PROMPT_MOCK") != "1" {
		t.Skip("not a mock helper process")
	}
	runPromptMockServer()
}

func runPromptMockServer() {
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
			result = json.RawMessage(`{"protocolVersion":"2024-11-05","serverInfo":{"name":"prompt-mock","version":"1.0.0"},"capabilities":{"prompts":{}}}`)
		case "tools/list":
			result = json.RawMessage(`{"tools":[]}`)
		case "resources/list":
			result = json.RawMessage(`{"resources":[]}`)
		case "prompts/list":
			result = json.RawMessage(`{"prompts":[{"name":"greeting","description":"Generate a greeting message","arguments":[{"name":"name","description":"Your name","required":true}]},{"name":"translate","description":"Translate text","arguments":[{"name":"text","description":"Text to translate","required":true},{"name":"lang","description":"Target language"}]}]}`)
		case "prompts/get":
			var params struct {
				Name string `json:"name"`
			}
			if msg.Params != nil {
				json.Unmarshal(msg.Params, &params)
			}
			if params.Name == "greeting" {
				result = json.RawMessage(`{"messages":[{"role":"user","content":{"type":"text","text":"Please greet the user!"}}]}`)
			} else if params.Name == "translate" {
				result = json.RawMessage(`{"messages":[{"role":"user","content":{"type":"text","text":"Translate the following text"}}]}`)
			} else {
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

// promptTestTransport 创建一个使用 Prompt mock 子进程的 StdioTransport。
func promptTestTransport() *StdioTransport {
	return NewStdioTransport(
		os.Args[0],
		[]string{"-test.run=TestPromptMockHelper$"},
		map[string]string{"GO_PROMPT_MOCK": "1"},
	)
}

// promptTestConfig 返回适用于提示测试的 ServerConfig。
func promptTestConfig() ServerConfig {
	return ServerConfig{
		Command: os.Args[0],
		Args:    []string{"-test.run=TestPromptMockHelper$"},
		Env:     map[string]string{"GO_PROMPT_MOCK": "1"},
	}
}

// setupManagerWithPromptMock 创建一个 Manager 并连接一个提示 mock 服务器。
func setupManagerWithPromptMock(t *testing.T, name string) *Manager {
	t.Helper()
	mgr := NewManager()
	if err := mgr.Connect(context.Background(), name, promptTestConfig()); err != nil {
		t.Fatalf("Connect(%q): %v", name, err)
	}
	return mgr
}

// ---------------------------------------------------------------------------
// Prompt 单元测试
// ---------------------------------------------------------------------------

// TestPromptsList 测试 prompts/list 成功获取提示模板列表。
func TestPromptsList(t *testing.T) {
	mgr := setupManagerWithPromptMock(t, "prompt-server")
	defer mgr.Close()

	prompts, err := mgr.Prompts("prompt-server")
	if err != nil {
		t.Fatalf("Prompts() failed: %v", err)
	}

	if len(prompts) != 2 {
		t.Fatalf("expected 2 prompts, got %d", len(prompts))
	}
	if prompts[0].Name != "greeting" {
		t.Errorf("expected Name 'greeting', got %q", prompts[0].Name)
	}
	if prompts[0].Description != "Generate a greeting message" {
		t.Errorf("expected description 'Generate a greeting message', got %q", prompts[0].Description)
	}
	if len(prompts[0].Arguments) != 1 {
		t.Errorf("expected 1 argument, got %d", len(prompts[0].Arguments))
	}
	if prompts[0].Arguments[0].Name != "name" {
		t.Errorf("expected argument name 'name', got %q", prompts[0].Arguments[0].Name)
	}
	if !prompts[0].Arguments[0].Required {
		t.Error("expected argument to be required")
	}
}

// TestPromptsListNotFound 测试不支持的服务器 graceful degradation。
func TestPromptsListNotFound(t *testing.T) {
	mgr := setupManagerWithMock(t, "basic-server") // 标准 mock 不支持 prompts/list
	defer mgr.Close()

	server, ok := mgr.GetServer("basic-server")
	if !ok {
		t.Fatal("expected server to be registered")
	}
	if len(server.Prompts) != 0 {
		t.Errorf("expected 0 prompts for unsupporting server, got %d", len(server.Prompts))
	}
}

// TestPromptsListMultiple 测试多服务器提示模板分组。
func TestPromptsListMultiple(t *testing.T) {
	mgr := NewManager()
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := mgr.Connect(ctx, "srv1", promptTestConfig()); err != nil {
		t.Fatalf("Connect srv1: %v", err)
	}
	if err := mgr.Connect(ctx, "srv2", promptTestConfig()); err != nil {
		t.Fatalf("Connect srv2: %v", err)
	}

	p1, err := mgr.Prompts("srv1")
	if err != nil {
		t.Fatalf("Prompts srv1: %v", err)
	}
	p2, err := mgr.Prompts("srv2")
	if err != nil {
		t.Fatalf("Prompts srv2: %v", err)
	}

	if len(p1) != 2 || len(p2) != 2 {
		t.Errorf("expected 2 prompts each, got %d and %d", len(p1), len(p2))
	}
}

// TestGetPrompt 测试 prompts/get 获取提示模板内容。
func TestGetPrompt(t *testing.T) {
	mgr := setupManagerWithPromptMock(t, "prompt-server")
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := mgr.GetPrompt(ctx, "prompt-server", "greeting")
	if err != nil {
		t.Fatalf("GetPrompt failed: %v", err)
	}

	if len(result.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result.Messages))
	}
	if result.Messages[0].Role != "user" {
		t.Errorf("expected role 'user', got %q", result.Messages[0].Role)
	}
}

// TestGetPromptNotFound 测试获取不存在的提示模板返回错误。
func TestGetPromptNotFound(t *testing.T) {
	mgr := setupManagerWithPromptMock(t, "prompt-server")
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := mgr.GetPrompt(ctx, "prompt-server", "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent prompt")
	}
	if !strings.Contains(err.Error(), "prompt not found") {
		t.Errorf("expected 'prompt not found' in error, got %v", err)
	}
}

// TestGetPromptServerNotFound 测试服务器不存在时返回错误。
func TestGetPromptServerNotFound(t *testing.T) {
	mgr := NewManager()
	defer mgr.Close()

	ctx := context.Background()
	_, err := mgr.GetPrompt(ctx, "nonexistent", "greeting")
	if err == nil {
		t.Fatal("expected error for nonexistent server")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got %v", err)
	}
}

// TestGetPromptAfterClose 测试 Manager 关闭后获取提示模板返回错误。
func TestGetPromptAfterClose(t *testing.T) {
	mgr := setupManagerWithPromptMock(t, "prompt-server")
	mgr.Close()

	ctx := context.Background()
	_, err := mgr.GetPrompt(ctx, "prompt-server", "greeting")
	if err == nil {
		t.Fatal("expected error after close")
	}
	if !strings.Contains(err.Error(), "manager is closed") {
		t.Errorf("expected 'manager is closed', got %v", err)
	}
}

// TestGetPromptContextCancel 测试 context 取消。
func TestGetPromptContextCancel(t *testing.T) {
	mgr := setupManagerWithPromptMock(t, "prompt-server")
	defer mgr.Close()

	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := mgr.GetPrompt(cancelCtx, "prompt-server", "greeting")
	if err == nil {
		t.Fatal("expected error with cancelled context")
	}
}

// TestGetPromptSizeLimitNormal 测试正常大小提示模板（不截断）。
func TestGetPromptSizeLimitNormal(t *testing.T) {
	mgr := setupManagerWithPromptMock(t, "prompt-server")
	defer mgr.Close()
	mgr.SetMaxPromptSize(10 * 1024 * 1024) // 10MB

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := mgr.GetPrompt(ctx, "prompt-server", "greeting")
	if err != nil {
		t.Fatalf("GetPrompt failed: %v", err)
	}

	if result.Truncated {
		t.Error("expected no truncation for small prompt")
	}
}

// TestGetPromptSizeLimitDisabled 测试大小限制为 0（不限制）。
func TestGetPromptSizeLimitDisabled(t *testing.T) {
	mgr := setupManagerWithPromptMock(t, "prompt-server")
	defer mgr.Close()
	mgr.SetMaxPromptSize(0) // 不限制

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := mgr.GetPrompt(ctx, "prompt-server", "greeting")
	if err != nil {
		t.Fatalf("GetPrompt failed: %v", err)
	}

	if result.Truncated {
		t.Error("expected no truncation when limit is 0")
	}
}

// TestApplyPromptContentLimit 测试 applyPromptContentLimit 函数。
func TestApplyPromptContentLimit(t *testing.T) {
	// nil safe
	result := applyPromptContentLimit(nil, 1024)
	if result != nil {
		t.Error("expected nil for nil input")
	}

	// 不限制
	result2 := applyPromptContentLimit(&GetPromptResult{Messages: []PromptMessage{{Role: "user"}}}, 0)
	if result2 == nil || result2.Truncated {
		t.Error("expected no truncation when limit is 0")
	}
}

// TestGetPromptCrossServer 测试跨服务器 prompt 路由。
func TestGetPromptCrossServer(t *testing.T) {
	mgr := setupManagerWithPromptMock(t, "srv-a")
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := mgr.GetPrompt(ctx, "srv-a", "greeting")
	if err != nil {
		t.Fatalf("GetPrompt failed: %v", err)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result.Messages))
	}
	if result.Messages[0].Role != "user" {
		t.Errorf("expected role 'user', got %q", result.Messages[0].Role)
	}
}