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
// Configurable mock MCP 服务器 — 通过 GO_MCP_CAPS 环境变量控制能力宣告
// ---------------------------------------------------------------------------

// TestCapabilityMockHelper 是一个特殊的测试函数，通过子进程方式充当可配置能力的 MCP 模拟服务器。
// 通过 GO_MCP_CAPS 环境变量控制宣告的能力，可选值：tools, resources, prompts, all, none。
func TestCapabilityMockHelper(t *testing.T) {
	if os.Getenv("GO_MCP_CAPS") == "" {
		t.Skip("not a capability mock helper process")
	}
	runCapabilityMockServer()
}

// runCapabilityMockServer 根据 GO_MCP_CAPS 环境变量控制能力宣告的 MCP 模拟服务器。
func runCapabilityMockServer() {
	caps := parseCapsEnv(os.Getenv("GO_MCP_CAPS"))

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
			result = buildCapsInitResponse(caps)
		case "tools/list":
			if !caps.Tools {
				sendErrorMock(os.Stdout, *msg.ID, -32601, "method not found")
				continue
			}
			result = json.RawMessage(`{"tools":[{"name":"cap_tool","description":"Capability tool","inputSchema":{"type":"object","properties":{"input":{"type":"string"}}}}]}`)
		case "tools/call":
			result = json.RawMessage(`{"content":[{"type":"text","text":"capability tool result"}],"isError":false}`)
		case "resources/list":
			if !caps.Resources {
				sendErrorMock(os.Stdout, *msg.ID, -32601, "method not found")
				continue
			}
			result = json.RawMessage(`{"resources":[{"uri":"cap://resource","name":"Cap Resource","description":"A capability resource"}]}`)
		case "prompts/list":
			if !caps.Prompts {
				sendErrorMock(os.Stdout, *msg.ID, -32601, "method not found")
				continue
			}
			result = json.RawMessage(`{"prompts":[{"name":"cap_prompt","description":"A capability prompt"}]}`)
		default:
			sendErrorMock(os.Stdout, *msg.ID, -32601, "method not found")
			continue
		}

		sendResponseMock(os.Stdout, *msg.ID, result)
	}
}

// parseCapsEnv 解析 GO_MCP_CAPS 环境变量，返回能力集合。
// 支持逗号分隔值："tools","resources","prompts","all","none"。
func parseCapsEnv(val string) ServerCapabilities {
	var caps ServerCapabilities
	parts := strings.Split(val, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		switch p {
		case "tools":
			caps.Tools = true
		case "resources":
			caps.Resources = true
		case "prompts":
			caps.Prompts = true
		case "all":
			caps.Tools = true
			caps.Resources = true
			caps.Prompts = true
		case "none":
			// 全部为 false
		}
	}
	return caps
}

// buildCapsInitResponse 根据能力集合构建 initialize 响应 JSON。
func buildCapsInitResponse(caps ServerCapabilities) json.RawMessage {
	capFields := ""
	if caps.Tools {
		capFields += `"tools":{},`
	}
	if caps.Resources {
		capFields += `"resources":{},`
	}
	if caps.Prompts {
		capFields += `"prompts":{},`
	}
	if len(capFields) > 0 {
		capFields = capFields[:len(capFields)-1] // 去掉末尾逗号
	}
	return json.RawMessage(fmt.Sprintf(`{"protocolVersion":"2024-11-05","capabilities":{%s}}`, capFields))
}

// capabilityTestTransport 创建一个使用可配置能力 mock 子进程的 StdioTransport。
func capabilityTestTransport(caps string) *StdioTransport {
	return NewStdioTransport(
		os.Args[0],
		[]string{"-test.run=TestCapabilityMockHelper$"},
		map[string]string{"GO_MCP_CAPS": caps},
	)
}

// capabilityTestConfig 返回适用于能力测试的 ServerConfig。
func capabilityTestConfig(caps string) ServerConfig {
	return ServerConfig{
		Command: os.Args[0],
		Args:    []string{"-test.run=TestCapabilityMockHelper$"},
		Env:     map[string]string{"GO_MCP_CAPS": caps},
	}
}

// ---------------------------------------------------------------------------
// 能力协商单元测试
// ---------------------------------------------------------------------------

// TestCapabilityToolsOnly 测试服务器仅宣告 tools 能力 — 仅 tools/list 被调用。
func TestCapabilityToolsOnly(t *testing.T) {
	mgr := NewManager()
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := mgr.Connect(ctx, "test-server", capabilityTestConfig("tools")); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	server, ok := mgr.GetServer("test-server")
	if !ok {
		t.Fatal("expected server to be registered")
	}

	// 验证 capabilities
	if !server.Capabilities.Tools {
		t.Error("expected Tools capability to be true")
	}
	if server.Capabilities.Resources {
		t.Error("expected Resources capability to be false")
	}
	if server.Capabilities.Prompts {
		t.Error("expected Prompts capability to be false")
	}

	// 验证仅 tools 被发现
	if len(server.Tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(server.Tools))
	}
	if server.Tools[0].Name != "cap_tool" {
		t.Errorf("expected tool 'cap_tool', got %q", server.Tools[0].Name)
	}

	// 验证 resources 和 prompts 未被发现
	if len(server.Resources) != 0 {
		t.Errorf("expected 0 resources, got %d", len(server.Resources))
	}
	if len(server.Prompts) != 0 {
		t.Errorf("expected 0 prompts, got %d", len(server.Prompts))
	}
}

// TestCapabilityAll 测试服务器宣告所有能力 — 所有发现方法均被调用。
func TestCapabilityAll(t *testing.T) {
	mgr := NewManager()
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := mgr.Connect(ctx, "test-server", capabilityTestConfig("all")); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	server, ok := mgr.GetServer("test-server")
	if !ok {
		t.Fatal("expected server to be registered")
	}

	// 验证所有能力均为 true
	if !server.Capabilities.Tools {
		t.Error("expected Tools capability to be true")
	}
	if !server.Capabilities.Resources {
		t.Error("expected Resources capability to be true")
	}
	if !server.Capabilities.Prompts {
		t.Error("expected Prompts capability to be true")
	}

	// 验证所有能力均被发现
	if len(server.Tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(server.Tools))
	}
	if len(server.Resources) != 1 {
		t.Errorf("expected 1 resource, got %d", len(server.Resources))
	}
	if len(server.Prompts) != 1 {
		t.Errorf("expected 1 prompt, got %d", len(server.Prompts))
	}

	if server.Tools[0].Name != "cap_tool" {
		t.Errorf("expected tool 'cap_tool', got %q", server.Tools[0].Name)
	}
	if server.Resources[0].URI != "cap://resource" {
		t.Errorf("expected resource URI 'cap://resource', got %q", server.Resources[0].URI)
	}
	if server.Prompts[0].Name != "cap_prompt" {
		t.Errorf("expected prompt 'cap_prompt', got %q", server.Prompts[0].Name)
	}
}

// TestCapabilityNone 测试服务器不宣告任何能力 — 无发现方法被调用。
func TestCapabilityNone(t *testing.T) {
	mgr := NewManager()
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := mgr.Connect(ctx, "test-server", capabilityTestConfig("none")); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	server, ok := mgr.GetServer("test-server")
	if !ok {
		t.Fatal("expected server to be registered")
	}

	// 验证所有能力均为 false
	if server.Capabilities.Tools {
		t.Error("expected Tools capability to be false")
	}
	if server.Capabilities.Resources {
		t.Error("expected Resources capability to be false")
	}
	if server.Capabilities.Prompts {
		t.Error("expected Prompts capability to be false")
	}

	// 验证无任何能力被发现
	if len(server.Tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(server.Tools))
	}
	if len(server.Resources) != 0 {
		t.Errorf("expected 0 resources, got %d", len(server.Resources))
	}
	if len(server.Prompts) != 0 {
		t.Errorf("expected 0 prompts, got %d", len(server.Prompts))
	}
}

// TestCapabilityResourcesOnly 测试服务器仅宣告 resources 能力。
func TestCapabilityResourcesOnly(t *testing.T) {
	mgr := NewManager()
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := mgr.Connect(ctx, "test-server", capabilityTestConfig("resources")); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	server, ok := mgr.GetServer("test-server")
	if !ok {
		t.Fatal("expected server to be registered")
	}

	if server.Capabilities.Tools {
		t.Error("expected Tools capability to be false")
	}
	if !server.Capabilities.Resources {
		t.Error("expected Resources capability to be true")
	}
	if server.Capabilities.Prompts {
		t.Error("expected Prompts capability to be false")
	}

	if len(server.Tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(server.Tools))
	}
	if len(server.Resources) != 1 {
		t.Errorf("expected 1 resource, got %d", len(server.Resources))
	}
	if len(server.Prompts) != 0 {
		t.Errorf("expected 0 prompts, got %d", len(server.Prompts))
	}
}

// TestCapabilityPromptsOnly 测试服务器仅宣告 prompts 能力。
func TestCapabilityPromptsOnly(t *testing.T) {
	mgr := NewManager()
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := mgr.Connect(ctx, "test-server", capabilityTestConfig("prompts")); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	server, ok := mgr.GetServer("test-server")
	if !ok {
		t.Fatal("expected server to be registered")
	}

	if server.Capabilities.Tools {
		t.Error("expected Tools capability to be false")
	}
	if server.Capabilities.Resources {
		t.Error("expected Resources capability to be false")
	}
	if !server.Capabilities.Prompts {
		t.Error("expected Prompts capability to be true")
	}

	if len(server.Tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(server.Tools))
	}
	if len(server.Resources) != 0 {
		t.Errorf("expected 0 resources, got %d", len(server.Resources))
	}
	if len(server.Prompts) != 1 {
		t.Errorf("expected 1 prompt, got %d", len(server.Prompts))
	}
}

// TestCapabilityReconnect 测试重连后 capabilities 被正确保留并影响重新发现。
func TestCapabilityReconnect(t *testing.T) {
	mgr := NewManager()
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// 连接一个仅支持 tools 的服务器
	if err := mgr.Connect(ctx, "test-server", capabilityTestConfig("tools")); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	server, ok := mgr.GetServer("test-server")
	if !ok {
		t.Fatal("expected server to be registered")
	}

	// 验证初始发现
	if !server.Capabilities.Tools {
		t.Error("expected Tools capability to be true")
	}
	if server.Capabilities.Resources {
		t.Error("expected Resources capability to be false")
	}
	if server.Capabilities.Prompts {
		t.Error("expected Prompts capability to be false")
	}
	if len(server.Tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(server.Tools))
	}

	// 关闭 transport 并重连
	server.Transport.Close()

	reconnectCtx, reconnectCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer reconnectCancel()

	if err := mgr.Reconnect(reconnectCtx, "test-server"); err != nil {
		t.Fatalf("Reconnect failed: %v", err)
	}

	// 验证重连后 capabilities 和发现结果正确
	if !server.Capabilities.Tools {
		t.Error("expected Tools capability to be true after reconnect")
	}
	if server.Capabilities.Resources {
		t.Error("expected Resources capability to be false after reconnect")
	}
	if server.Capabilities.Prompts {
		t.Error("expected Prompts capability to be false after reconnect")
	}
	if len(server.Tools) != 1 {
		t.Errorf("expected 1 tool after reconnect, got %d", len(server.Tools))
	}
	if len(server.Resources) != 0 {
		t.Errorf("expected 0 resources after reconnect, got %d", len(server.Resources))
	}
	if len(server.Prompts) != 0 {
		t.Errorf("expected 0 prompts after reconnect, got %d", len(server.Prompts))
	}

	// 验证工具仍可调用
	callResult, err := mgr.CallTool(ctx, "test-server", "cap_tool", nil)
	if err != nil {
		t.Fatalf("CallTool after reconnect failed: %v", err)
	}
	if callResult.Content != "capability tool result" {
		t.Errorf("expected 'capability tool result', got %q", callResult.Content)
	}
}

// TestCapabilityReconnectPreservesNone 测试重连 preserves 无能力场景。
func TestCapabilityReconnectPreservesNone(t *testing.T) {
	mgr := NewManager()
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := mgr.Connect(ctx, "test-server", capabilityTestConfig("none")); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	server, ok := mgr.GetServer("test-server")
	if !ok {
		t.Fatal("expected server to be registered")
	}

	// 验证初始无能力
	if server.Capabilities.Tools || server.Capabilities.Resources || server.Capabilities.Prompts {
		t.Error("expected all capabilities to be false")
	}

	// 关闭 transport 并重连
	server.Transport.Close()

	reconnectCtx, reconnectCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer reconnectCancel()

	if err := mgr.Reconnect(reconnectCtx, "test-server"); err != nil {
		t.Fatalf("Reconnect failed: %v", err)
	}

	// 验证重连后仍无能力
	if server.Capabilities.Tools || server.Capabilities.Resources || server.Capabilities.Prompts {
		t.Error("expected all capabilities to be false after reconnect")
	}
	if len(server.Tools) != 0 || len(server.Resources) != 0 || len(server.Prompts) != 0 {
		t.Error("expected no discovered items after reconnect with no capabilities")
	}
}

// TestParseCapabilities 测试 parseCapabilities 函数解析不同的初始化响应。
func TestParseCapabilities(t *testing.T) {
	tests := []struct {
		name     string
		response string
		want     ServerCapabilities
	}{
		{
			name:     "all capabilities",
			response: `{"protocolVersion":"2024-11-05","capabilities":{"tools":{},"resources":{},"prompts":{}}}`,
			want:     ServerCapabilities{Tools: true, Resources: true, Prompts: true},
		},
		{
			name:     "tools only",
			response: `{"protocolVersion":"2024-11-05","capabilities":{"tools":{}}}`,
			want:     ServerCapabilities{Tools: true},
		},
		{
			name:     "no capabilities",
			response: `{"protocolVersion":"2024-11-05","capabilities":{}}`,
			want:     ServerCapabilities{},
		},
		{
			name:     "no capabilities field",
			response: `{"protocolVersion":"2024-11-05"}`,
			want:     ServerCapabilities{},
		},
		{
			name:     "resources and prompts",
			response: `{"protocolVersion":"2024-11-05","capabilities":{"resources":{},"prompts":{}}}`,
			want:     ServerCapabilities{Resources: true, Prompts: true},
		},
		{
			name:     "prompts only",
			response: `{"protocolVersion":"2024-11-05","capabilities":{"prompts":{}}}`,
			want:     ServerCapabilities{Prompts: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var caps ServerCapabilities
			err := parseCapabilities(json.RawMessage(tt.response), &caps)
			if err != nil {
				t.Fatalf("parseCapabilities() error = %v", err)
			}
			if caps.Tools != tt.want.Tools {
				t.Errorf("Tools = %v, want %v", caps.Tools, tt.want.Tools)
			}
			if caps.Resources != tt.want.Resources {
				t.Errorf("Resources = %v, want %v", caps.Resources, tt.want.Resources)
			}
			if caps.Prompts != tt.want.Prompts {
				t.Errorf("Prompts = %v, want %v", caps.Prompts, tt.want.Prompts)
			}
		})
	}
}

// TestParseCapabilitiesError 测试 parseCapabilities 对无效 JSON 返回错误。
func TestParseCapabilitiesError(t *testing.T) {
	var caps ServerCapabilities
	err := parseCapabilities(json.RawMessage(`invalid`), &caps)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// TestCapabilityConcurrent 测试并发能力连接安全。
func TestCapabilityConcurrent(t *testing.T) {
	mgr := NewManager()
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 并发连接不同能力组合的服务器
	errCh := make(chan error, 4)
	names := []string{"tools-only", "all", "none", "prompts-only"}
	caps := []string{"tools", "all", "none", "prompts"}

	for i, name := range names {
		go func(name, cap string) {
			cfg := capabilityTestConfig(cap)
			if err := mgr.Connect(ctx, name, cfg); err != nil {
				errCh <- fmt.Errorf("Connect(%q): %w", name, err)
			}
			errCh <- nil
		}(name, caps[i])
	}

	for range names {
		if err := <-errCh; err != nil {
			t.Fatal(err)
		}
	}

	// 验证各服务器能力正确
	servers := mgr.GetServers()
	if len(servers) != 4 {
		t.Fatalf("expected 4 servers, got %d", len(servers))
	}

	if s := servers["tools-only"]; s != nil {
		if !s.Capabilities.Tools || s.Capabilities.Resources || s.Capabilities.Prompts {
			t.Error("tools-only: unexpected capabilities")
		}
		if len(s.Tools) != 1 || len(s.Resources) != 0 || len(s.Prompts) != 0 {
			t.Error("tools-only: unexpected discovered items")
		}
	}
	if s := servers["all"]; s != nil {
		if !s.Capabilities.Tools || !s.Capabilities.Resources || !s.Capabilities.Prompts {
			t.Error("all: unexpected capabilities")
		}
		if len(s.Tools) != 1 || len(s.Resources) != 1 || len(s.Prompts) != 1 {
			t.Error("all: unexpected discovered items")
		}
	}
	if s := servers["none"]; s != nil {
		if s.Capabilities.Tools || s.Capabilities.Resources || s.Capabilities.Prompts {
			t.Error("none: unexpected capabilities")
		}
		if len(s.Tools) != 0 || len(s.Resources) != 0 || len(s.Prompts) != 0 {
			t.Error("none: unexpected discovered items")
		}
	}
	if s := servers["prompts-only"]; s != nil {
		if s.Capabilities.Tools || s.Capabilities.Resources || !s.Capabilities.Prompts {
			t.Error("prompts-only: unexpected capabilities")
		}
		if len(s.Tools) != 0 || len(s.Resources) != 0 || len(s.Prompts) != 1 {
			t.Error("prompts-only: unexpected discovered items")
		}
	}
}
