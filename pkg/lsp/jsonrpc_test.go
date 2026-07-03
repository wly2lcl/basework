package lsp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// 测试辅助：mockServer 模拟 LSP 服务器端，通过 io.Pipe 与 Conn 通信
// ---------------------------------------------------------------------------

// mockServer 在后台协程中运行，从 in 读取请求并向 out 写入响应/通知。
type mockServer struct {
	t      *testing.T
	in     io.ReadCloser  // 服务器读取请求
	out    io.WriteCloser // 服务器写入响应
	reader *bufio.Reader
	closed atomic.Bool

	// 收到请求时调用的回调，便于测试自定义响应逻辑
	onRequest func(id int, method string, params json.RawMessage)
}

func newMockServer(t *testing.T, in io.ReadCloser, out io.WriteCloser) *mockServer {
	return &mockServer{
		t:      t,
		in:     in,
		out:    out,
		reader: bufio.NewReader(in),
	}
}

// start 启动服务器的后台读取循环。
// 默认行为：对每个请求回复一个包含 method 名的响应。
// 可通过 onRequest 定制行为。
func (s *mockServer) start() {
	go s.loop()
}

func (s *mockServer) loop() {
	defer s.in.Close()
	defer s.out.Close()

	for {
		contentLength, err := s.readHeaders()
		if err != nil {
			return
		}
		if contentLength <= 0 {
			continue
		}

		body := make([]byte, contentLength)
		if _, err := io.ReadFull(s.reader, body); err != nil {
			return
		}

		var msg struct {
			ID     *int             `json:"id"`
			Method string           `json:"method"`
			Params json.RawMessage  `json:"params"`
		}
		if err := json.Unmarshal(body, &msg); err != nil {
			s.t.Errorf("mock server: failed to unmarshal request: %v", err)
			continue
		}

		if msg.ID != nil {
			// 这是一个带 ID 的请求，需要回复
			if s.onRequest != nil {
				s.onRequest(*msg.ID, msg.Method, msg.Params)
			} else {
				// 默认回复：返回 method 名的字符串
				result, _ := json.Marshal(msg.Method)
				s.sendResponse(*msg.ID, result, nil)
			}
		}
	}
}

func (s *mockServer) readHeaders() (int, error) {
	var contentLength int
	for {
		line, err := s.reader.ReadString('\n')
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
				// 成功
			}
		}
	}
	return contentLength, nil
}

func (s *mockServer) sendResponse(id int, result json.RawMessage, rpcErr *ResponseError) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
		Error:   rpcErr,
	}
	data, _ := json.Marshal(resp)
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	s.out.Write([]byte(header))
	s.out.Write(data)
}

func (s *mockServer) sendNotification(method string, params json.RawMessage) {
	notif := Notification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	data, _ := json.Marshal(notif)
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	s.out.Write([]byte(header))
	s.out.Write(data)
}

func (s *mockServer) close() {
	s.closed.Store(true)
	s.in.Close()
	s.out.Close()
}

// ---------------------------------------------------------------------------
// 测试辅助：创建 Conn + mockServer 对
// ---------------------------------------------------------------------------

type connFixture struct {
	conn   *Conn
	server *mockServer
	// 客户端视角的管道
	clientIn  io.WriteCloser  // Conn 写 stdin
	clientOut io.ReadCloser   // Conn 读 stdout
}

func newConnFixture(t *testing.T) *connFixture {
	// 服务器读取、客户端写入
	serverInRead, serverInWrite := io.Pipe()
	// 服务器写入、客户端读取
	serverOutRead, serverOutWrite := io.Pipe()

	conn := NewConn(serverInWrite, serverOutRead)
	server := newMockServer(t, serverInRead, serverOutWrite)

	return &connFixture{
		conn:      conn,
		server:    server,
		clientIn:  serverInWrite,
		clientOut: serverOutRead,
	}
}

func (f *connFixture) start() {
	f.server.start()
	f.conn.Start()
}

func (f *connFixture) close() {
	f.conn.Close()
	f.server.close()
}

// ---------------------------------------------------------------------------
// 测试用例
// ---------------------------------------------------------------------------

// TestContentLengthFraming 测试 Content-Length 帧格式的发送和接收。
func TestContentLengthFraming(t *testing.T) {
	f := newConnFixture(t)
	defer f.close()

	// 自定义服务器：读取请求后回复
	received := make(chan string, 1)
	f.server.onRequest = func(id int, method string, params json.RawMessage) {
		received <- method
		result, _ := json.Marshal(method)
		f.server.sendResponse(id, result, nil)
	}
	f.start()

	// 发送请求
	ctx := context.Background()
	result, err := f.conn.Send(ctx, "test/method", nil)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	select {
	case method := <-received:
		if method != "test/method" {
			t.Errorf("expected method 'test/method', got %q", method)
		}
	default:
		t.Fatal("server did not receive request")
	}

	var got string
	if err := json.Unmarshal(result, &got); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if got != "test/method" {
		t.Errorf("expected result 'test/method', got %q", got)
	}
}

// TestConcurrentRequests 测试并发请求能否正确配对响应与请求 ID。
func TestConcurrentRequests(t *testing.T) {
	f := newConnFixture(t)
	defer f.close()

	var mu sync.Mutex
	requestIDs := make(map[int]string)

	f.server.onRequest = func(id int, method string, params json.RawMessage) {
		mu.Lock()
		requestIDs[id] = method
		mu.Unlock()
		// 回复包含 method 名
		result, _ := json.Marshal(method)
		f.server.sendResponse(id, result, nil)
	}
	f.start()

	ctx := context.Background()
	type result struct {
		method string
		err    error
	}

	const n = 10
	ch := make(chan result, n)
	for i := 0; i < n; i++ {
		method := fmt.Sprintf("method_%d", i)
		go func(m string) {
			res, err := f.conn.Send(ctx, m, nil)
			if err != nil {
				ch <- result{err: err}
				return
			}
			var got string
			if err := json.Unmarshal(res, &got); err != nil {
				ch <- result{err: err}
				return
			}
			ch <- result{method: got}
		}(method)
	}

	seen := make(map[string]int)
	for i := 0; i < n; i++ {
		r := <-ch
		if r.err != nil {
			t.Fatalf("concurrent request failed: %v", r.err)
		}
		seen[r.method]++
	}

	for i := 0; i < n; i++ {
		method := fmt.Sprintf("method_%d", i)
		if seen[method] != 1 {
			t.Errorf("method %q seen %d times, want 1", method, seen[method])
		}
	}
}

// TestNotificationDispatch 测试服务器发送通知后，客户端注册的处理函数被正确调用。
func TestNotificationDispatch(t *testing.T) {
	f := newConnFixture(t)
	defer f.close()

	f.start()

	// 注册通知处理器
	notifCh := make(chan string, 1)
	f.conn.OnNotification("textDocument/publishDiagnostics", func(params json.RawMessage) {
		var msg string
		json.Unmarshal(params, &msg)
		notifCh <- msg
	})

	// 服务器发送通知
	params, _ := json.Marshal("diagnostic message")
	f.server.sendNotification("textDocument/publishDiagnostics", params)

	select {
	case got := <-notifCh:
		if got != "diagnostic message" {
			t.Errorf("expected 'diagnostic message', got %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for notification")
	}
}

// TestRequestTimeout 测试请求在上下文超时时返回错误。
func TestRequestTimeout(t *testing.T) {
	f := newConnFixture(t)
	defer f.close()

	// 服务器不回复任何请求
	f.server.onRequest = func(id int, method string, params json.RawMessage) {
		// 不回复，模拟超时
	}
	f.start()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := f.conn.Send(ctx, "test/timeout", nil)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if err != context.DeadlineExceeded {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}
}

// TestServerError 测试服务器返回错误响应时，客户端能正确获取错误。
func TestServerError(t *testing.T) {
	f := newConnFixture(t)
	defer f.close()

	f.server.onRequest = func(id int, method string, params json.RawMessage) {
		errResp := &ResponseError{
			Code:    ErrCodeInternal,
			Message: "internal server error",
		}
		f.server.sendResponse(id, nil, errResp)
	}
	f.start()

	ctx := context.Background()
	_, err := f.conn.Send(ctx, "test/error", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var rpcErr *ResponseError
	if e, ok := err.(*ResponseError); ok {
		rpcErr = e
	} else {
		// 如果被 fmt.Errorf 包装，可能需要使用 errors.As
		t.Fatalf("expected *ResponseError, got %T: %v", err, err)
	}
	if rpcErr.Code != ErrCodeInternal {
		t.Errorf("expected code %d, got %d", ErrCodeInternal, rpcErr.Code)
	}
	if rpcErr.Message != "internal server error" {
		t.Errorf("expected message 'internal server error', got %q", rpcErr.Message)
	}
}

// TestCloseCleanup 测试关闭连接后，等待中的请求收到错误。
func TestCloseCleanup(t *testing.T) {
	f := newConnFixture(t)
	defer f.close()

	f.server.onRequest = func(id int, method string, params json.RawMessage) {
		// 不回复，保证请求在 Close 时仍在等待中
	}
	f.start()

	errCh := make(chan error, 1)
	go func() {
		ctx := context.Background()
		_, err := f.conn.Send(ctx, "test/close", nil)
		errCh <- err
	}()

	// 给 Send 一点时间发送请求并进入等待状态
	time.Sleep(50 * time.Millisecond)

	// 关闭连接
	f.conn.Close()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected error from closed connection, got nil")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for Send to return after Close")
	}
}

// TestEOFHandling 测试服务器断开连接后，等待中的请求收到错误。
func TestEOFHandling(t *testing.T) {
	f := newConnFixture(t)
	defer f.close()

	f.server.onRequest = func(id int, method string, params json.RawMessage) {
		// 不回复，直接关闭服务器连接
		f.server.close()
	}
	f.start()

	ctx := context.Background()
	_, err := f.conn.Send(ctx, "test/eof", nil)
	if err == nil {
		t.Fatal("expected error on EOF, got nil")
	}
}

// TestServerRequest 测试服务器发出的请求（如 workspace/configuration）被正确处理。
// 不使用 mockServer loop，避免与测试读取竞争同一个 reader。
func TestServerRequest(t *testing.T) {
	// 用 buffer 捕获客户端写出的数据
	var captured safeBuffer
	tee := &teeWriter{writers: []io.Writer{&captured}}

	serverToClientR, serverToClientW := io.Pipe()
	conn := NewConn(tee, serverToClientR)
	conn.Start()
	defer func() {
		conn.Close()
		serverToClientW.Close()
	}()

	// 发送服务器请求
	serverReq := Request{
		JSONRPC: "2.0",
		ID:      42,
		Method:  "workspace/configuration",
		Params:  json.RawMessage(`{"section": "editor"}`),
	}
	data, _ := json.Marshal(serverReq)
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	serverToClientW.Write([]byte(header))
	serverToClientW.Write(data)

	// 等待客户端响应写入
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		body := captured.String()
		if strings.Contains(body, `"id":42`) {
			if !strings.Contains(body, `"jsonrpc":"2.0"`) {
				t.Errorf("response should contain jsonrpc field")
			}
			if !strings.Contains(body, `"result":null`) {
				t.Errorf("response should contain null result, got: %s", body)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for server request response, captured: %s", captured.String())
}

// safeBuffer 是线程安全的 bytes.Buffer。
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// teeWriter 将写入分发到多个 writer，满足 io.WriteCloser。
type teeWriter struct {
	writers []io.Writer
}

func (t *teeWriter) Write(p []byte) (int, error) {
	for _, w := range t.writers {
		if _, err := w.Write(p); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

func (t *teeWriter) Close() error { return nil }

// TestNotifyWithoutParams 测试发送不带参数的通知。
func TestNotifyWithoutParams(t *testing.T) {
	f := newConnFixture(t)
	defer f.close()

	notifCh := make(chan struct{}, 1)
	f.conn.OnNotification("test/noParams", func(params json.RawMessage) {
		notifCh <- struct{}{}
	})
	f.start()

	// 服务器发送不带 params 的通知
	notif := Notification{
		JSONRPC: "2.0",
		Method:  "test/noParams",
	}
	data, _ := json.Marshal(notif)
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	f.server.out.Write([]byte(header))
	f.server.out.Write(data)

	select {
	case <-notifCh:
		// 成功
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for notification without params")
	}
}

// TestSendWithParams 测试带参数的请求和响应。
func TestSendWithParams(t *testing.T) {
	f := newConnFixture(t)
	defer f.close()

	type queryParams struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}

	f.server.onRequest = func(id int, method string, params json.RawMessage) {
		var q queryParams
		if err := json.Unmarshal(params, &q); err != nil {
			t.Errorf("unmarshal params: %v", err)
		}
		if q.Query != "hello" || q.Limit != 10 {
			t.Errorf("unexpected params: %+v", q)
		}
		result, _ := json.Marshal(map[string]string{"status": "ok"})
		f.server.sendResponse(id, result, nil)
	}
	f.start()

	ctx := context.Background()
	result, err := f.conn.Send(ctx, "test/withParams", queryParams{Query: "hello", Limit: 10})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	var resp map[string]string
	if err := json.Unmarshal(result, &resp); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status 'ok', got %q", resp["status"])
	}
}

// TestNotify 测试发送通知。
func TestNotify(t *testing.T) {
	// 手动创建管道，避免与 fixture 的 reader 冲突
	serverInRead, serverInWrite := io.Pipe()
	serverOutRead, serverOutWrite := io.Pipe()

	conn := NewConn(serverInWrite, serverOutRead)
	conn.Start()

	// 在发送通知之前启动读取协程（io.Pipe 是同步的，写需要对应读）
	type notifyResult struct {
		method string
		params json.RawMessage
		err    error
	}
	resultCh := make(chan notifyResult, 1)
	go func() {
		reader := bufio.NewReader(serverInRead)
		defer serverInRead.Close()
		defer serverOutWrite.Close()

		// 读取 Content-Length 头部
		var contentLength int
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				resultCh <- notifyResult{err: fmt.Errorf("read header: %w", err)}
				return
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

		if contentLength <= 0 {
			resultCh <- notifyResult{err: fmt.Errorf("invalid content length: %d", contentLength)}
			return
		}

		body := make([]byte, contentLength)
		if _, err := io.ReadFull(reader, body); err != nil {
			resultCh <- notifyResult{err: fmt.Errorf("read body: %w", err)}
			return
		}

		var parsed struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			resultCh <- notifyResult{err: fmt.Errorf("unmarshal: %w", err)}
			return
		}
		resultCh <- notifyResult{method: parsed.Method, params: parsed.Params}
	}()

	notifParams := map[string]string{"text": "hello"}
	err := conn.Notify("textDocument/didChange", notifParams)
	if err != nil {
		t.Fatalf("Notify failed: %v", err)
	}

	result := <-resultCh
	if result.err != nil {
		t.Fatalf("read result: %v", result.err)
	}
	if result.method != "textDocument/didChange" {
		t.Errorf("expected method 'textDocument/didChange', got %q", result.method)
	}
	if result.params == nil {
		t.Error("expected params in notification, got nil")
	}

	conn.Close()
}

// TestMultipleNotifications 测试同一 method 注册多个通知处理器（验证覆盖行为）。
func TestMultipleNotifications(t *testing.T) {
	f := newConnFixture(t)
	defer f.close()

	callCount := 0
	handler := func(params json.RawMessage) {
		callCount++
	}

	f.conn.OnNotification("test/event", handler)
	f.conn.OnNotification("test/event", handler) // 第二次注册覆盖第一次
	f.start()

	// Server sends notification
	params, _ := json.Marshal("data")
	f.server.sendNotification("test/event", params)

	time.Sleep(100 * time.Millisecond)
	if callCount != 1 {
		t.Errorf("handler should be called once, called %d times", callCount)
	}
}

// TestDoneChannel 测试 Close 后 Done 通道被关闭。
func TestDoneChannel(t *testing.T) {
	f := newConnFixture(t)
	f.start()

	done := f.conn.Done()
	select {
	case <-done:
		t.Fatal("Done channel should not be closed before Close")
	default:
	}

	f.conn.Close()

	select {
	case <-done:
		// 正确
	case <-time.After(time.Second):
		t.Fatal("Done channel not closed after Close")
	}
}