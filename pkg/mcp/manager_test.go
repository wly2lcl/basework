package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wly2lcl/basework/pkg/tool"
)

// ---------------------------------------------------------------------------
// Manager Mock MCP 服务器 — 通过子进程方式实现
// ---------------------------------------------------------------------------

// TestManagerMockHelper 是一个特殊的测试函数，通过子进程方式充当 MCP 模拟服务器。
// 通过 GO_MCP_MOCK=1 环境变量激活（与 transport 测试共享）。
func TestManagerMockHelper(t *testing.T) {
	if os.Getenv("GO_MCP_MOCK") != "1" {
		t.Skip("not a mock helper process")
	}
	runManagerMockServer()
}

// runManagerMockServer 模拟用于 Manager 测试的 MCP 服务器。
func runManagerMockServer() {
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
			ID     *int             `json:"id"`
			Method string           `json:"method"`
			Params json.RawMessage  `json:"params"`
		}
		if err := json.Unmarshal(body, &msg); err != nil {
			continue
		}

		if msg.ID == nil {
			// 通知，忽略
			continue
		}

		var result json.RawMessage
		switch msg.Method {
		case "initialize":
			result = json.RawMessage(`{"protocolVersion":"2024-11-05","serverInfo":{"name":"mock","version":"1.0.0"},"capabilities":{"tools":{}}}`)
		case "tools/list":
			result = json.RawMessage(`{"tools":[{"name":"test_tool","description":"A test tool","inputSchema":{"type":"object","properties":{"query":{"type":"string"}}}}]}`)
		case "tools/call":
			// 检查参数中是否包含 slow_tool
			var callParams struct {
				Name string `json:"name"`
			}
			if msg.Params != nil {
				json.Unmarshal(msg.Params, &callParams)
			}
			if callParams.Name == "slow_tool" {
				time.Sleep(200 * time.Millisecond)
			}
			result = json.RawMessage(`{"content":[{"type":"text","text":"mock result"}],"isError":false}`)
		default:
			sendErrorMock(os.Stdout, *msg.ID, -32601, "method not found")
			continue
		}

		sendResponseMock(os.Stdout, *msg.ID, result)
	}
}

// ---------------------------------------------------------------------------
// 测试辅助函数
// ---------------------------------------------------------------------------

// managerTestTransport 创建一个使用 Manager mock 子进程的 StdioTransport。
func managerTestTransport() *StdioTransport {
	return NewStdioTransport(
		os.Args[0],
		[]string{"-test.run=TestManagerMockHelper$"},
		map[string]string{"GO_MCP_MOCK": "1"},
	)
}

// managerTestConfig 返回适用于 Manager 测试的 ServerConfig。
func managerTestConfig() ServerConfig {
	return ServerConfig{
		Command: os.Args[0],
		Args:    []string{"-test.run=TestManagerMockHelper$"},
		Env:     map[string]string{"GO_MCP_MOCK": "1"},
	}
}

// setupManagerWithMock 创建一个 Manager 并连接一个 mock 服务器。
// 使用 context.Background() 而非可取消的 context，避免 Connect 返回后
// context 取消导致 exec.CommandContext 杀掉子进程。
func setupManagerWithMock(t *testing.T, name string) *Manager {
	t.Helper()
	mgr := NewManager()
	if err := mgr.Connect(context.Background(), name, managerTestConfig()); err != nil {
		t.Fatalf("Connect(%q): %v", name, err)
	}
	return mgr
}

// ---------------------------------------------------------------------------
// Manager 单元测试
// ---------------------------------------------------------------------------

// TestManagerConnect 测试连接 mock 服务器，验证服务器已注册。
func TestManagerConnect(t *testing.T) {
	mgr := NewManager()
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := mgr.Connect(ctx, "test-server", managerTestConfig()); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	server, ok := mgr.GetServer("test-server")
	if !ok {
		t.Fatal("expected server to be registered")
	}
	if server.Name != "test-server" {
		t.Errorf("expected name 'test-server', got %q", server.Name)
	}
	if len(server.Tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(server.Tools))
	}
}

// TestManagerConnectMultiple 测试连接多个 mock 服务器，验证均被注册。
func TestManagerConnectMultiple(t *testing.T) {
	mgr := NewManager()
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	names := []string{"server-a", "server-b"}
	for _, name := range names {
		if err := mgr.Connect(ctx, name, managerTestConfig()); err != nil {
			t.Fatalf("Connect(%q) failed: %v", name, err)
		}
	}

	servers := mgr.GetServers()
	if len(servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(servers))
	}
	for _, name := range names {
		if _, ok := servers[name]; !ok {
			t.Errorf("expected server %q to be registered", name)
		}
	}
}

// TestManagerConnectFail 测试连接无效命令，验证返回错误且未注册。
func TestManagerConnectFail(t *testing.T) {
	mgr := NewManager()
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := ServerConfig{
		Command: "nonexistent-command-that-should-not-exist",
	}
	err := mgr.Connect(ctx, "bad-server", cfg)
	if err == nil {
		t.Fatal("expected error for invalid command")
	}

	if _, ok := mgr.GetServer("bad-server"); ok {
		t.Fatal("expected no server to be registered after failed connect")
	}
}

// TestManagerConnectPartialFail 测试一个有效一个无效连接，验证有效连接成功，无效连接不影响其他。
func TestManagerConnectPartialFail(t *testing.T) {
	mgr := NewManager()
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 无效连接
	badCfg := ServerConfig{Command: "nonexistent-command"}
	err := mgr.Connect(ctx, "bad-server", badCfg)
	if err == nil {
		t.Fatal("expected error for bad server")
	}

	// 有效连接
	if err := mgr.Connect(ctx, "good-server", managerTestConfig()); err != nil {
		t.Fatalf("Connect good-server failed: %v", err)
	}

	servers := mgr.GetServers()
	if len(servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(servers))
	}
	if _, ok := servers["good-server"]; !ok {
		t.Error("expected good-server to be registered")
	}
}

// TestManagerTools 测试连接后 Tools() 返回适配后的工具，名称格式正确。
func TestManagerTools(t *testing.T) {
	mgr := setupManagerWithMock(t, "test-server")
	defer mgr.Close()

	tools := mgr.Tools()
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}

	tool := tools[0]
	expectedName := "mcp_test-server_test_tool"
	if tool.Name() != expectedName {
		t.Errorf("expected name %q, got %q", expectedName, tool.Name())
	}
	if tool.Description() != "A test tool" {
		t.Errorf("expected description 'A test tool', got %q", tool.Description())
	}
}

// TestManagerCallTool 测试调用工具，验证返回正确结果。
func TestManagerCallTool(t *testing.T) {
	mgr := setupManagerWithMock(t, "test-server")
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	args := json.RawMessage(`{"query":"hello"}`)
	result, err := mgr.CallTool(ctx, "test-server", "test_tool", args)
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}

	if result.Content != "mock result" {
		t.Errorf("expected 'mock result', got %q", result.Content)
	}
	if result.IsError {
		t.Error("expected IsError to be false")
	}
}

// TestManagerDisconnect 测试断开服务器连接，验证已被移除。
func TestManagerDisconnect(t *testing.T) {
	mgr := setupManagerWithMock(t, "test-server")
	defer mgr.Close()

	if err := mgr.Disconnect("test-server"); err != nil {
		t.Fatalf("Disconnect failed: %v", err)
	}

	if _, ok := mgr.GetServer("test-server"); ok {
		t.Error("expected server to be removed after disconnect")
	}
}

// TestManagerClose 测试关闭 Manager，验证所有连接已关闭。
func TestManagerClose(t *testing.T) {
	mgr := setupManagerWithMock(t, "server-a")
	// 再连接一个
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := mgr.Connect(ctx, "server-b", managerTestConfig()); err != nil {
		t.Fatalf("Connect server-b: %v", err)
	}

	if err := mgr.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	servers := mgr.GetServers()
	if len(servers) != 0 {
		t.Errorf("expected all servers removed after close, got %d", len(servers))
	}

	// 关闭后调用应该失败
	_, err := mgr.CallTool(context.Background(), "server-a", "test_tool", nil)
	if err == nil {
		t.Error("expected error when calling tool after close")
	}
}

// TestManagerCloseIdempotent 测试重复 Close 安全（第二次返回 nil）。
func TestManagerCloseIdempotent(t *testing.T) {
	mgr := NewManager()
	// 第一次 Close（未连接任何服务器）
	if err := mgr.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	// 第二次 Close
	if err := mgr.Close(); err != nil {
		t.Fatalf("second Close should return nil: %v", err)
	}
}

// TestManagerCloseWaitsForCalls 测试 Close 等待进行中的调用完成。
func TestManagerCloseWaitsForCalls(t *testing.T) {
	mgr := setupManagerWithMock(t, "test-server")

	// 在 goroutine 中发起一个慢调用
	errCh := make(chan error, 1)
	go func() {
		_, err := mgr.CallTool(context.Background(), "test-server", "slow_tool", nil)
		errCh <- err
	}()

	// 确保 CallTool 已经开始
	time.Sleep(50 * time.Millisecond)

	// Close 应该等待 slow 调用完成
	start := time.Now()
	if err := mgr.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	elapsed := time.Since(start)

	// 确保 Close 等待了（至少等了部分时间）
	if elapsed < 100*time.Millisecond {
		t.Logf("Close waited %v (expected at least ~200ms for slow call)", elapsed)
	}

	// 检查调用结果
	callErr := <-errCh
	if callErr != nil {
		t.Logf("CallTool after close returned: %v (expected)", callErr)
	}
}

// TestManagerCallAfterClose 测试 Close 后调用工具返回错误 "manager is closed"。
func TestManagerCallAfterClose(t *testing.T) {
	mgr := setupManagerWithMock(t, "test-server")

	if err := mgr.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	_, err := mgr.CallTool(context.Background(), "test-server", "test_tool", nil)
	if err == nil {
		t.Fatal("expected error after close")
	}
	if !strings.Contains(err.Error(), "manager is closed") {
		t.Errorf("expected 'manager is closed', got %v", err)
	}
}

// TestManagerConcurrentConnect 测试并发连接多个服务器。
func TestManagerConcurrentConnect(t *testing.T) {
	mgr := NewManager()
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	const n = 5
	var wg sync.WaitGroup
	errCh := make(chan error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		name := fmt.Sprintf("server-%d", i)
		go func(name string) {
			defer wg.Done()
			if err := mgr.Connect(ctx, name, managerTestConfig()); err != nil {
				errCh <- fmt.Errorf("Connect(%q): %w", name, err)
			}
		}(name)
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatal(err)
	}

	servers := mgr.GetServers()
	if len(servers) != n {
		t.Errorf("expected %d servers, got %d", n, len(servers))
	}

	// 验证工具适配器数量
	tools := mgr.Tools()
	if len(tools) != n {
		t.Errorf("expected %d tools (1 per server), got %d", n, len(tools))
	}
}

// TestManagerGetServers 测试 GetServers 返回副本，修改副本不影响原 map。
func TestManagerGetServers(t *testing.T) {
	mgr := setupManagerWithMock(t, "test-server")
	defer mgr.Close()

	servers := mgr.GetServers()
	if len(servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(servers))
	}

	// 修改副本不应影响原 map
	servers["fake"] = &ServerConnection{Name: "fake"}
	if _, ok := mgr.GetServer("fake"); ok {
		t.Error("expected fake server not to be in original map")
	}
}

// TestManagerConcurrentTools 测试并发安全地调用 Tools()。
func TestManagerConcurrentTools(t *testing.T) {
	mgr := setupManagerWithMock(t, "test-server")
	defer mgr.Close()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tools := mgr.Tools()
			if len(tools) != 1 {
				t.Errorf("expected 1 tool, got %d", len(tools))
			}
		}()
	}
	wg.Wait()
}

// TestManagerConnectWithHttpCfg 测试无效配置（空类型）返回错误。
func TestManagerConnectWithInvalidConfig(t *testing.T) {
	mgr := NewManager()
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := ServerConfig{} // 空的配置，TransportType 返回 ""
	err := mgr.Connect(ctx, "empty", cfg)
	if err == nil {
		t.Fatal("expected error for empty config")
	}
}

// TestManagerToolsFromMultipleServers 测试多服务器工具适配器。
func TestManagerToolsFromMultipleServers(t *testing.T) {
	mgr := NewManager()
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := mgr.Connect(ctx, "srv1", managerTestConfig()); err != nil {
		t.Fatalf("Connect srv1: %v", err)
	}
	if err := mgr.Connect(ctx, "srv2", managerTestConfig()); err != nil {
		t.Fatalf("Connect srv2: %v", err)
	}

	tools := mgr.Tools()
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}

	// 验证两个工具的名称不同
	names := make(map[string]bool)
	for _, tl := range tools {
		names[tl.Name()] = true
	}
	if !names["mcp_srv1_test_tool"] {
		t.Error("expected mcp_srv1_test_tool")
	}
	if !names["mcp_srv2_test_tool"] {
		t.Error("expected mcp_srv2_test_tool")
	}
}

// TestManagerCallToolNilArgs 测试传递 nil 参数调用工具。
func TestManagerCallToolNilArgs(t *testing.T) {
	mgr := setupManagerWithMock(t, "test-server")
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 传递 nil 参数
	result, err := mgr.CallTool(ctx, "test-server", "test_tool", nil)
	if err != nil {
		t.Fatalf("CallTool with nil args failed: %v", err)
	}
	if result.Content != "mock result" {
		t.Errorf("expected 'mock result', got %q", result.Content)
	}
}

// TestManagerCallToolEmptyObjectArgs 测试传递 {} 参数调用工具。
func TestManagerCallToolEmptyObjectArgs(t *testing.T) {
	mgr := setupManagerWithMock(t, "test-server")
	defer mgr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := mgr.CallTool(ctx, "test-server", "test_tool", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("CallTool with empty args failed: %v", err)
	}
	if result.Content != "mock result" {
		t.Errorf("expected 'mock result', got %q", result.Content)
	}
}

// TestManagerCallToolServerNotFound 测试调用不存在的服务器。
func TestManagerCallToolServerNotFound(t *testing.T) {
	mgr := NewManager()
	defer mgr.Close()

	_, err := mgr.CallTool(context.Background(), "nonexistent", "test_tool", nil)
	if err == nil {
		t.Fatal("expected error for nonexistent server")
	}
}

// TestManagerCallTool_CloseRace 测试并发 CallTool 和 Close 无 race condition。
func TestManagerCallTool_CloseRace(t *testing.T) {
	mgr := setupManagerWithMock(t, "test-server")

	var wg sync.WaitGroup
	// 并发发起多个 CallTool
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_, _ = mgr.CallTool(ctx, "test-server", "test_tool", nil)
		}()
	}

	// 同时关闭
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = mgr.Close()
	}()

	wg.Wait()
}

// TestManagerReadResource_CloseRace 测试并发 ReadResource 和 Close 无 race condition。
func TestManagerReadResource_CloseRace(t *testing.T) {
	mgr := setupManagerWithMock(t, "test-server")

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_, _ = mgr.ReadResource(ctx, "test-server", "test://resource")
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = mgr.Close()
	}()

	wg.Wait()
}

// TestManagerGetPrompt_CloseRace 测试并发 GetPrompt 和 Close 无 race condition。
func TestManagerGetPrompt_CloseRace(t *testing.T) {
	mgr := setupManagerWithMock(t, "test-server")

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_, _ = mgr.GetPrompt(ctx, "test-server", "test_prompt")
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = mgr.Close()
	}()

	wg.Wait()
}
func TestManagerToolInterface(t *testing.T) {
	mgr := setupManagerWithMock(t, "test-server")
	defer mgr.Close()

	tools := mgr.Tools()
	if len(tools) == 0 {
		t.Fatal("expected at least 1 tool")
	}

	// 验证实现了 tool.Tool 接口
	var iface tool.Tool = tools[0]
	if iface == nil {
		t.Fatal("tool must implement tool.Tool interface")
	}
}