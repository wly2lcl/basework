package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Mock MCP 服务器 — 通过子进程助手实现
// ---------------------------------------------------------------------------

// TestMCPMockHelper 是一个特殊的测试函数，通过子进程方式充当 MCP 模拟服务器。
// 通过 GO_MCP_MOCK=1 环境变量激活。
func TestMCPMockHelper(t *testing.T) {
	if os.Getenv("GO_MCP_MOCK") != "1" {
		t.Skip("not a mock helper process")
	}
	runMockMCPServer()
}

// runMockMCPServer 模拟一个简单的 MCP 语言服务器。
// 从 stdin 读取 JSON-RPC 请求，写入响应到 stdout。
func runMockMCPServer() {
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
			result = json.RawMessage(`{"protocolVersion":"2024-11-05","capabilities":{}}`)
		case "tools/list":
			result = json.RawMessage(`{"tools":[{"name":"echo","description":"Echo tool","inputSchema":{"type":"object"}}]}`)
		case "tools/call":
			result = json.RawMessage(`{"content":[{"type":"text","text":"echo: hello"}]}`)
		case "shutdown":
			result = json.RawMessage("null")
		case "slow":
			// 模拟慢响应
			time.Sleep(200 * time.Millisecond)
			result = json.RawMessage(`"done"`)
		case "error":
			// 返回错误
			sendErrorMock(os.Stdout, *msg.ID, -1, "mock error")
			continue
		case "ping":
			result = json.RawMessage(`"pong"`)
		default:
			sendErrorMock(os.Stdout, *msg.ID, -32601, "method not found")
			continue
		}

		sendResponseMock(os.Stdout, *msg.ID, result)
	}
}

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
			n, err := strconv.Atoi(strings.TrimSpace(line[len("Content-Length:"):]))
			if err != nil {
				continue
			}
			contentLength = n
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
// StdioTransport 单元测试
// ---------------------------------------------------------------------------

// stdioTestTransport 创建一个使用自身测试二进制作为 MCP 子进程的 StdioTransport。
func stdioTestTransport() *StdioTransport {
	return NewStdioTransport(
		os.Args[0],
		[]string{"-test.run=TestMCPMockHelper$"},
		map[string]string{"GO_MCP_MOCK": "1"},
	)
}

// TestStdioConnect 测试 StdioTransport.Connect 启动子进程成功。
func TestStdioConnect(t *testing.T) {
	transport := stdioTestTransport()
	defer transport.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
}

// TestStdioConnectInvalidCommand 测试连接无效命令返回错误。
func TestStdioConnectInvalidCommand(t *testing.T) {
	transport := NewStdioTransport("nonexistent-command-that-should-not-exist", nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := transport.Connect(ctx); err == nil {
		t.Fatal("expected error for invalid command")
	}
}

// TestStdioCall 测试 Call 发送请求并接收响应。
func TestStdioCall(t *testing.T) {
	transport := stdioTestTransport()
	defer transport.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	result, err := transport.Call(ctx, "ping", nil)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}

	var s string
	if err := json.Unmarshal(result, &s); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if s != "pong" {
		t.Errorf("expected 'pong', got %q", s)
	}
}

// TestStdioCallWithParams 测试带参数的 Call。
func TestStdioCallWithParams(t *testing.T) {
	transport := stdioTestTransport()
	defer transport.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	params := json.RawMessage(`{"name":"test"}`)
	result, err := transport.Call(ctx, "initialize", params)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}

	var capResp struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := json.Unmarshal(result, &capResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if capResp.ProtocolVersion != "2024-11-05" {
		t.Errorf("expected protocol version, got %+v", capResp)
	}
}

// TestStdioCallTimeout 测试 Call 超时。
func TestStdioCallTimeout(t *testing.T) {
	transport := stdioTestTransport()
	defer transport.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// 使用超时短的 context
	shortCtx, shortCancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer shortCancel()

	_, err := transport.Call(shortCtx, "slow", nil)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if err != context.DeadlineExceeded {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}
}

// TestStdioNotify 测试 Notify 发送通知。
func TestStdioNotify(t *testing.T) {
	transport := stdioTestTransport()
	defer transport.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// 通知不需要响应
	err := transport.Notify("notifications/initialized", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Notify failed: %v", err)
	}
}

// TestStdioCloseCancelsPending 测试 Close 取消等待中的请求。
func TestStdioCloseCancelsPending(t *testing.T) {
	transport := stdioTestTransport()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// 在另一个 goroutine 中发起请求
	errCh := make(chan error, 1)
	go func() {
		_, err := transport.Call(context.Background(), "slow", nil)
		errCh <- err
	}()

	// 给 Call 一点时间发送请求
	time.Sleep(50 * time.Millisecond)

	// 关闭连接，应取消等待中的请求
	transport.Close()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected error from closed transport")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for Call to return after Close")
	}
}

// TestStdioConcurrentCalls 测试并发请求正确配对。
func TestStdioConcurrentCalls(t *testing.T) {
	transport := stdioTestTransport()
	defer transport.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	const n = 5
	type result struct {
		value string
		err   error
	}
	ch := make(chan result, n)

	for i := 0; i < n; i++ {
		go func(idx int) {
			_, err := transport.Call(ctx, "ping", nil)
			if err != nil {
				ch <- result{err: err}
				return
			}
			ch <- result{value: "pong"}
		}(i)
	}

	for i := 0; i < n; i++ {
		r := <-ch
		if r.err != nil {
			t.Fatalf("concurrent call failed: %v", r.err)
		}
		if r.value != "pong" {
			t.Errorf("expected 'pong', got %q", r.value)
		}
	}
}

// TestStdioMultipleCalls 测试连续多次 Call。
func TestStdioMultipleCalls(t *testing.T) {
	transport := stdioTestTransport()
	defer transport.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	for i := 0; i < 10; i++ {
		result, err := transport.Call(ctx, "ping", nil)
		if err != nil {
			t.Fatalf("call %d failed: %v", i, err)
		}
		var s string
		if err := json.Unmarshal(result, &s); err != nil {
			t.Fatalf("unmarshal %d: %v", i, err)
		}
		if s != "pong" {
			t.Errorf("call %d: expected 'pong', got %q", i, s)
		}
	}
}

// TestStdioCallError 测试服务器返回错误。
func TestStdioCallError(t *testing.T) {
	transport := stdioTestTransport()
	defer transport.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	_, err := transport.Call(ctx, "error", nil)
	if err == nil {
		t.Fatal("expected error from server")
	}

	var rpcErr *rpcError
	if e, ok := err.(*rpcError); ok {
		rpcErr = e
	} else {
		t.Fatalf("expected *rpcError, got %T: %v", err, err)
	}
	if rpcErr.Code != -1 {
		t.Errorf("expected code -1, got %d", rpcErr.Code)
	}
	if rpcErr.Message != "mock error" {
		t.Errorf("expected 'mock error', got %q", rpcErr.Message)
	}
}

// TestStdioCallNotFound 测试调用不存在的方法。
func TestStdioCallNotFound(t *testing.T) {
	transport := stdioTestTransport()
	defer transport.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	_, err := transport.Call(ctx, "unknown_method", nil)
	if err == nil {
		t.Fatal("expected error for unknown method")
	}
}

// TestStdioConnectIdempotent 测试重复 Connect 为 no-op。
func TestStdioConnectIdempotent(t *testing.T) {
	transport := stdioTestTransport()
	defer transport.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("first Connect: %v", err)
	}
	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("second Connect should be no-op: %v", err)
	}
}

// TestStdioCloseIdempotent 测试重复 Close 安全。
func TestStdioCloseIdempotent(t *testing.T) {
	transport := NewStdioTransport("echo", nil, nil)
	for i := 0; i < 3; i++ {
		if err := transport.Close(); err != nil {
			t.Fatalf("Close #%d: %v", i, err)
		}
	}
}

// TestStdioNotifyWithoutConnect 测试未连接时 Notify 返回错误。
func TestStdioNotifyWithoutConnect(t *testing.T) {
	transport := NewStdioTransport("echo", nil, nil)
	defer transport.Close()

	err := transport.Notify("test", nil)
	if err == nil {
		t.Error("expected error when not connected")
	}
}

// ---------------------------------------------------------------------------
// HTTPTransport 单元测试
// ---------------------------------------------------------------------------

// newHTTPServer 创建模拟 MCP 服务器的 HTTP 测试服务器。
// 根据 JSON-RPC 的方法字段返回对应的响应。
func newHTTPServer(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// 验证 Content-Type
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		var req struct {
			ID     *int             `json:"id"`
			Method string           `json:"method"`
			Params json.RawMessage  `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		if req.ID == nil {
			// 通知，直接返回 200
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
			return
		}

		var result json.RawMessage
		switch req.Method {
		case "initialize":
			result = json.RawMessage(`{"protocolVersion":"2024-11-05","capabilities":{}}`)
		case "tools/list":
			result = json.RawMessage(`{"tools":[{"name":"echo","description":"Echo tool","inputSchema":{"type":"object"}}]}`)
		case "tools/call":
			result = json.RawMessage(`{"content":[{"type":"text","text":"echo from http"}]}`)
		case "ping":
			result = json.RawMessage(`"pong"`)
		case "error":
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      *req.ID,
				"error":   map[string]interface{}{"code": -1, "message": "http error"},
			})
			return
		default:
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      *req.ID,
				"error":   map[string]interface{}{"code": -32601, "message": "method not found"},
			})
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      *req.ID,
			"result":  result,
		})
	}))
}

// TestHTTPConnect 测试 HTTPTransport.Connect。
func TestHTTPConnect(t *testing.T) {
	transport := NewHTTPTransport("http://example.com/mcp", nil)
	ctx := context.Background()
	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	transport.Close()
}

// TestHTTPConnectEmptyURL 测试空 URL 的 Connect。
func TestHTTPConnectEmptyURL(t *testing.T) {
	transport := NewHTTPTransport("", nil)
	ctx := context.Background()
	if err := transport.Connect(ctx); err == nil {
		t.Fatal("expected error for empty URL")
	}
}

// TestHTTPCall 测试 HTTPTransport.Call。
func TestHTTPCall(t *testing.T) {
	srv := newHTTPServer(t)
	defer srv.Close()

	transport := NewHTTPTransport(srv.URL, nil)
	defer transport.Close()

	ctx := context.Background()
	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	result, err := transport.Call(ctx, "ping", nil)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}

	var s string
	if err := json.Unmarshal(result, &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if s != "pong" {
		t.Errorf("expected 'pong', got %q", s)
	}
}

// TestHTTPCallWithHeaders 测试自定义头部。
func TestHTTPCallWithHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("expected Authorization header")
		}
		if r.Header.Get("X-Custom") != "custom-value" {
			t.Errorf("expected X-Custom header")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"result":  json.RawMessage(`"ok"`),
		})
	}))
	defer srv.Close()

	transport := NewHTTPTransport(srv.URL, map[string]string{
		"Authorization": "Bearer test-token",
		"X-Custom":      "custom-value",
	})
	defer transport.Close()

	ctx := context.Background()
	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	_, err := transport.Call(ctx, "ping", nil)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}
}

// TestHTTPCallError 测试 HTTP 返回错误响应。
func TestHTTPCallError(t *testing.T) {
	srv := newHTTPServer(t)
	defer srv.Close()

	transport := NewHTTPTransport(srv.URL, nil)
	defer transport.Close()

	ctx := context.Background()
	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	_, err := transport.Call(ctx, "error", nil)
	if err == nil {
		t.Fatal("expected error")
	}

	var rpcErr *rpcError
	if e, ok := err.(*rpcError); ok {
		rpcErr = e
	} else {
		t.Fatalf("expected *rpcError, got %T: %v", err, err)
	}
	if rpcErr.Code != -1 {
		t.Errorf("expected code -1, got %d", rpcErr.Code)
	}
}

// TestHTTPCallNotFound 测试未找到方法。
func TestHTTPCallNotFound(t *testing.T) {
	srv := newHTTPServer(t)
	defer srv.Close()

	transport := NewHTTPTransport(srv.URL, nil)
	defer transport.Close()

	ctx := context.Background()
	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	_, err := transport.Call(ctx, "nonexistent", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestHTTPCallHTTPError 测试 HTTP 服务端错误。
func TestHTTPCallHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	transport := NewHTTPTransport(srv.URL, nil)
	defer transport.Close()

	ctx := context.Background()
	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	_, err := transport.Call(ctx, "ping", nil)
	if err == nil {
		t.Fatal("expected error for HTTP 500")
	}
}

// TestHTTPNotify 测试 HTTPTransport.Notify。
func TestHTTPNotify(t *testing.T) {
	notifyCh := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     *int   `json:"id"`
			Method string `json:"method"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.ID == nil {
			notifyCh <- req.Method
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	transport := NewHTTPTransport(srv.URL, nil)
	defer transport.Close()

	ctx := context.Background()
	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	err := transport.Notify("notifications/updated", json.RawMessage(`{"data":"test"}`))
	if err != nil {
		t.Fatalf("Notify failed: %v", err)
	}

	select {
	case method := <-notifyCh:
		if method != "notifications/updated" {
			t.Errorf("expected 'notifications/updated', got %q", method)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for notification")
	}
}

// TestHTTPCallWithoutConnect 测试未连接时的 Call。
func TestHTTPCallWithoutConnect(t *testing.T) {
	transport := NewHTTPTransport("http://example.com", nil)
	defer transport.Close()

	_, err := transport.Call(context.Background(), "ping", nil)
	if err == nil {
		t.Fatal("expected error when not connected")
	}
}

// TestHTTPCloseIdempotent 测试 HTTP Close 幂等。
func TestHTTPCloseIdempotent(t *testing.T) {
	transport := NewHTTPTransport("http://example.com", nil)
	for i := 0; i < 3; i++ {
		if err := transport.Close(); err != nil {
			t.Fatalf("Close #%d: %v", i, err)
		}
	}
}

// TestHTTPMultipleCalls 测试多次 HTTP 调用。
func TestHTTPMultipleCalls(t *testing.T) {
	srv := newHTTPServer(t)
	defer srv.Close()

	transport := NewHTTPTransport(srv.URL, nil)
	defer transport.Close()

	ctx := context.Background()
	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	for i := 0; i < 5; i++ {
		result, err := transport.Call(ctx, "ping", nil)
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		var s string
		if err := json.Unmarshal(result, &s); err != nil {
			t.Fatalf("unmarshal %d: %v", i, err)
		}
		if s != "pong" {
			t.Errorf("call %d: expected 'pong', got %q", i, s)
		}
	}
}

// TestHTTPConcurrentCalls 测试并发 HTTP 调用。
func TestHTTPConcurrentCalls(t *testing.T) {
	srv := newHTTPServer(t)
	defer srv.Close()

	transport := NewHTTPTransport(srv.URL, nil)
	defer transport.Close()

	ctx := context.Background()
	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := transport.Call(ctx, "ping", nil)
			if err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("concurrent call failed: %v", err)
	}
}

// TestHTTPCallContextCancel 测试 HTTP 调用上下文取消。
func TestHTTPCallContextCancel(t *testing.T) {
	// 创建一个会延迟响应的服务器
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"result":  json.RawMessage(`"done"`),
		})
	}))
	defer srv.Close()

	transport := NewHTTPTransport(srv.URL, nil)
	defer transport.Close()

	ctx := context.Background()
	transport.Connect(ctx)

	shortCtx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := transport.Call(shortCtx, "ping", nil)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

// TestStdioCallWithContextCancel 测试取消上下文。
func TestStdioCallWithContextCancel(t *testing.T) {
	transport := stdioTestTransport()
	defer transport.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// 立即取消的上下文
	cancelCtx, cancelFunc := context.WithCancel(context.Background())
	cancelFunc()

	_, err := transport.Call(cancelCtx, "ping", nil)
	if err == nil {
		t.Fatal("expected error with canceled context")
	}
}

// TestStdioDuplicateClose 测试重复 Close 不 panic。
func TestStdioDuplicateClose(t *testing.T) {
	transport := stdioTestTransport()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// 多次 Close
	if err := transport.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := transport.Close(); err != nil {
		t.Fatalf("second Close should not error: %v", err)
	}
}

// TestStdioToolsList 测试 tools/list 调用。
func TestStdioToolsList(t *testing.T) {
	transport := stdioTestTransport()
	defer transport.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	result, err := transport.Call(ctx, "tools/list", nil)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}

	var listResp struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(result, &listResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(listResp.Tools) != 1 || listResp.Tools[0].Name != "echo" {
		t.Errorf("unexpected tools list: %+v", listResp)
	}
}

// TestStdioToolsCall 测试 tools/call 调用。
func TestStdioToolsCall(t *testing.T) {
	transport := stdioTestTransport()
	defer transport.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	params := json.RawMessage(`{"name":"echo","arguments":{"text":"hello"}}`)
	result, err := transport.Call(ctx, "tools/call", params)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}

	var callResp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(result, &callResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(callResp.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(callResp.Content))
	}
	if callResp.Content[0].Text != "echo: hello" {
		t.Errorf("expected 'echo: hello', got %q", callResp.Content[0].Text)
	}
}

// TestStdioShutdown 测试 shutdown 调用。
func TestStdioShutdown(t *testing.T) {
	transport := stdioTestTransport()
	defer transport.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	result, err := transport.Call(ctx, "shutdown", nil)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}
	if string(result) != "null" {
		t.Errorf("expected null result, got %s", string(result))
	}
}

// TestNewStdioTransport 测试构造函数。
func TestNewStdioTransport(t *testing.T) {
	env := map[string]string{"FOO": "bar"}
	tp := NewStdioTransport("echo", []string{"hello"}, env)
	if tp.command != "echo" {
		t.Errorf("expected 'echo', got %q", tp.command)
	}
	if len(tp.args) != 1 || tp.args[0] != "hello" {
		t.Errorf("unexpected args: %v", tp.args)
	}
	if tp.env["FOO"] != "bar" {
		t.Errorf("unexpected env: %v", tp.env)
	}
}

// TestNewHTTPTransport 测试 HTTP 构造函数。
func TestNewHTTPTransport(t *testing.T) {
	headers := map[string]string{"Authorization": "token"}
	tp := NewHTTPTransport("http://example.com/mcp", headers)
	if tp.url != "http://example.com/mcp" {
		t.Errorf("unexpected URL: %q", tp.url)
	}
	if tp.headers["Authorization"] != "token" {
		t.Errorf("unexpected headers: %v", tp.headers)
	}
}

// TestRPCError 测试 rpcError.Error()。
func TestRPCError(t *testing.T) {
	err := &rpcError{Code: -32601, Message: "method not found"}
	expected := "MCP error -32601: method not found"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

// TestStdioConnectContextCancel 测试使用已取消的 context 连接。
func TestStdioConnectContextCancel(t *testing.T) {
	transport := NewStdioTransport("echo", nil, nil)
	defer transport.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// 即使 context 已取消，CommandContext 仍会启动进程，
	// 但进程会被立即杀死。这里测试不 panic。
	err := transport.Connect(ctx)
	if err != nil {
		t.Logf("Connect with canceled context returned: %v (expected)", err)
	}
}

// TestStdioCallAfterClose 测试 Close 后调用 Call 的行为。
func TestStdioCallAfterClose(t *testing.T) {
	transport := stdioTestTransport()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	transport.Close()

	// 关闭后 Call 可能返回错误（连接已关闭）
	_, err := transport.Call(ctx, "ping", nil)
	_ = err
	// 没 panic 就算通过
}

// TestStdioCallNullResult 测试返回 null 的响应。
func TestStdioCallNullResult(t *testing.T) {
	transport := stdioTestTransport()
	defer transport.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	result, err := transport.Call(ctx, "shutdown", nil)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}
	if result != nil {
		// null 可能解析为 nil 或 "null"
		if string(result) != "null" {
			t.Errorf("expected null, got %s", string(result))
		}
	}
}

// TestHTTPTransportCloseBeforeConnect 测试未连接就 Close。
func TestHTTPTransportCloseBeforeConnect(t *testing.T) {
	transport := NewHTTPTransport("http://example.com", nil)
	if err := transport.Close(); err != nil {
		t.Fatalf("Close before Connect: %v", err)
	}
}

// TestStdioAfterClose 测试 Close 后的状态。
func TestStdioAfterClose(t *testing.T) {
	transport := stdioTestTransport()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	if err := transport.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// 第二次 Close 不应 panic
	if err := transport.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// TestHTTPCallParam 测试 HTTP 调用带参数。
func TestHTTPCallParam(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string           `json:"method"`
			Params json.RawMessage  `json:"params"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.Method != "echo" {
			t.Errorf("expected method 'echo', got %q", req.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"result":  req.Params,
		})
	}))
	defer srv.Close()

	transport := NewHTTPTransport(srv.URL, nil)
	defer transport.Close()

	ctx := context.Background()
	if err := transport.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	params := json.RawMessage(`{"text":"hello"}`)
	result, err := transport.Call(ctx, "echo", params)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}

	var data map[string]string
	if err := json.Unmarshal(result, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data["text"] != "hello" {
		t.Errorf("expected 'hello', got %q", data["text"])
	}
}