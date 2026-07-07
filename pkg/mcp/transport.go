// Package mcp 提供 MCP（Model Context Protocol）的传输层实现。
// 支持 stdio（子进程 stdin/stdout）和 HTTP 两种传输方式，
// 均基于 JSON-RPC 2.0 协议 + Content-Length 分帧。
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ---------------------------------------------------------------------------
// Transport 接口
// ---------------------------------------------------------------------------

// Transport 是 MCP 传输层接口。
// 实现需要处理 JSON-RPC 2.0 消息的发送和接收。
type Transport interface {
	// Connect 建立连接。
	Connect(ctx context.Context) error

	// Call 发送 JSON-RPC 请求并等待响应。返回 result 字段的原始 JSON。
	Call(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error)

	// Notify 发送 JSON-RPC 通知（不等待响应）。
	Notify(method string, params json.RawMessage) error

	// Close 关闭传输层，释放资源。
	Close() error
}

// ---------------------------------------------------------------------------
// JSON-RPC 消息类型
// ---------------------------------------------------------------------------

// rpcRequest 表示 JSON-RPC 请求。
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// rpcResponse 表示 JSON-RPC 响应。
type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// rpcNotification 表示 JSON-RPC 通知（无 ID）。
type rpcNotification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// rpcError 表示 JSON-RPC 响应中的错误。
type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *rpcError) Error() string {
	return fmt.Sprintf("MCP error %d: %s", e.Code, e.Message)
}

// rawMessage 用于解析收到的通用 JSON-RPC 消息。
type rawMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int            `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// pendingResponse 承载一个等待中的请求的响应结果。
type pendingResponse struct {
	result json.RawMessage
	err    error
}

// ---------------------------------------------------------------------------
// StdioTransport — 基于子进程 stdin/stdout 的 JSON-RPC 传输
// ---------------------------------------------------------------------------

// StdioTransport 通过启动子进程并经由 stdin/stdout 通信实现 Transport 接口。
// 使用 Content-Length 分帧格式，与 LSP 协议兼容。
type StdioTransport struct {
	command string
	args    []string
	env     map[string]string

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser

	reader    *bufio.Reader
	writeMu   sync.Mutex
	pendingMu sync.Mutex
	pending   map[int]chan pendingResponse
	nextID    atomic.Int64

	started    atomic.Bool
	readLoopWg sync.WaitGroup // 追踪 readLoop goroutine，供 Reset 等待
	done       chan struct{}
	closeOnce  sync.Once
	closed     atomic.Bool
}

// NewStdioTransport 创建一个新的 StdioTransport。
func NewStdioTransport(command string, args []string, env map[string]string) *StdioTransport {
	return &StdioTransport{
		command: command,
		args:    args,
		env:     env,
		pending: make(map[int]chan pendingResponse),
		done:    make(chan struct{}),
	}
}

// Connect 启动子进程并建立 JSON-RPC 连接。
func (t *StdioTransport) Connect(ctx context.Context) error {
	if t.started.Load() {
		return nil
	}

	cmd := exec.CommandContext(ctx, t.command, t.args...)
	cmd.Env = os.Environ()
	for k, v := range t.env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		return fmt.Errorf("start process: %w", err)
	}

	t.cmd = cmd
	t.stdin = stdin
	t.stdout = stdout
	t.reader = bufio.NewReader(stdout)
	t.started.Store(true)

	t.readLoopWg.Add(1)
	go t.readLoop()
	return nil
}

// Call 发送 JSON-RPC 请求并等待响应。
func (t *StdioTransport) Call(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	id := int(t.nextID.Add(1))

	ch := make(chan pendingResponse, 1)

	t.pendingMu.Lock()
	t.pending[id] = ch
	t.pendingMu.Unlock()

	defer func() {
		t.pendingMu.Lock()
		delete(t.pending, id)
		t.pendingMu.Unlock()
	}()

	req := rpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}
	if err := t.write(req); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp := <-ch:
		return resp.result, resp.err
	}
}

// Notify 发送 JSON-RPC 通知（不等待响应）。
func (t *StdioTransport) Notify(method string, params json.RawMessage) error {
	notif := rpcNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	return t.write(notif)
}

// Reset 杀死旧进程并重置状态，使 transport 可以被重新 Connect。
// 用于重连场景：清理旧进程后重新调用 Connect 启动新进程。
func (t *StdioTransport) Reset() {
	if !t.started.Load() {
		return
	}

	// 取消等待中的请求
	t.cancelPending(fmt.Errorf("transport reset"))

	// 关闭 stdin
	if t.stdin != nil {
		t.stdin.Close()
	}

	// 关闭 stdout（这会导致 readLoop 退出）
	if t.stdout != nil {
		t.stdout.Close()
	}

	// 等待 readLoop goroutine 退出，避免后续 Connect 与旧 readLoop 产生 data race
	t.readLoopWg.Wait()

	// 杀死旧进程
	if t.cmd != nil && t.cmd.Process != nil {
		t.cmd.Process.Kill()
		t.cmd.Wait() // 忽略等待错误
	}

	// 重置状态——不置 nil 指针（避免与 readLoop 的 data race），
	// 下一次 Connect 会覆盖所有字段。
	t.started.Store(false)
}

// Close 关闭传输层，取消所有等待中的请求。
func (t *StdioTransport) Close() error {
	var err error
	t.closeOnce.Do(func() {
		t.closed.Store(true)

		t.cancelPending(fmt.Errorf("connection closed"))

		if t.stdin != nil {
			if cerr := t.stdin.Close(); cerr != nil {
				err = cerr
			}
		}
		if t.stdout != nil {
			if cerr := t.stdout.Close(); cerr != nil {
				err = cerr
			}
		}
		if t.cmd != nil && t.cmd.Process != nil {
			_ = t.cmd.Wait()
		}

		close(t.done)
	})
	return err
}

// write 将消息序列化为 JSON 并以 Content-Length 帧格式写入 stdin。
func (t *StdioTransport) write(msg interface{}) error {
	if t.stdin == nil {
		return fmt.Errorf("StdioTransport: not connected")
	}

	t.writeMu.Lock()
	defer t.writeMu.Unlock()

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	if _, err := t.stdin.Write([]byte(header)); err != nil {
		return err
	}
	if _, err := t.stdin.Write(data); err != nil {
		return err
	}
	return nil
}

// cancelPending 取消所有等待中的请求，向其发送指定的错误。
func (t *StdioTransport) cancelPending(err error) {
	t.pendingMu.Lock()
	defer t.pendingMu.Unlock()
	for id, ch := range t.pending {
		select {
		case ch <- pendingResponse{err: err}:
		default:
		}
		delete(t.pending, id)
	}
}

// readLoop 循环从 stdout 读取消息并分发。
func (t *StdioTransport) readLoop() {
	defer t.readLoopWg.Done()
	defer func() {
		if !t.closed.Load() {
			t.cancelPending(fmt.Errorf("connection closed"))
		}
	}()

	for {
		contentLength, err := t.readHeaders()
		if err != nil {
			return
		}
		if contentLength <= 0 {
			continue
		}

		body := make([]byte, contentLength)
		if _, err := io.ReadFull(t.reader, body); err != nil {
			return
		}

		t.dispatchMessage(body)
	}
}

// readHeaders 读取 Content-Length 头部，返回消息体长度。
func (t *StdioTransport) readHeaders() (int, error) {
	var contentLength int
	for {
		line, err := t.reader.ReadString('\n')
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

// dispatchMessage 解析并分发一条 JSON-RPC 消息。
func (t *StdioTransport) dispatchMessage(body []byte) {
	var msg rawMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return
	}

	switch {
	case msg.ID != nil && msg.Method != "":
		// 服务器发来的请求，回复 null
		resp := rpcResponse{
			JSONRPC: "2.0",
			ID:      *msg.ID,
			Result:  json.RawMessage("null"),
		}
		_ = t.write(resp)

	case msg.ID != nil:
		// 对我们之前请求的响应
		t.dispatchResponse(*msg.ID, msg.Result, msg.Error)

	default:
		// 通知，目前忽略
	}
}

// dispatchResponse 将响应分发给对应的等待者。
func (t *StdioTransport) dispatchResponse(id int, result json.RawMessage, rpcErr *rpcError) {
	pr := pendingResponse{result: result}
	if rpcErr != nil {
		pr.err = rpcErr
	}

	t.pendingMu.Lock()
	ch, ok := t.pending[id]
	if ok {
		delete(t.pending, id)
	}
	t.pendingMu.Unlock()

	if ok {
		select {
		case ch <- pr:
		default:
		}
	}
}

// ---------------------------------------------------------------------------
// HTTPTransport — 基于 HTTP POST 的 JSON-RPC 传输
// ---------------------------------------------------------------------------

// HTTPTransport 通过 HTTP POST 发送 JSON-RPC 消息实现 Transport 接口。
type HTTPTransport struct {
	url     string
	headers map[string]string
	client  *http.Client
	nextID  atomic.Int64
}

// NewHTTPTransport 创建一个新的 HTTPTransport。
func NewHTTPTransport(url string, headers map[string]string) *HTTPTransport {
	return &HTTPTransport{
		url:     url,
		headers: headers,
	}
}

// Connect 验证 URL 并创建 HTTP 客户端。
func (t *HTTPTransport) Connect(ctx context.Context) error {
	if t.url == "" {
		return fmt.Errorf("MCP HTTP transport: empty URL")
	}
	t.client = &http.Client{
		Timeout: 5 * time.Minute,
	}
	return nil
}

// Call 发送 JSON-RPC 请求并等待响应。
func (t *HTTPTransport) Call(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	if t.client == nil {
		return nil, fmt.Errorf("MCP HTTP transport: not connected")
	}

	reqBody := rpcRequest{
		JSONRPC: "2.0",
		ID:      int(t.nextID.Add(1)),
		Method:  method,
		Params:  params,
	}
	bodyData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url, bytes.NewReader(bodyData))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range t.headers {
		req.Header.Set(k, v)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("MCP HTTP transport: unexpected status %d", resp.StatusCode)
	}

	var rpcResp rpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, rpcResp.Error
	}
	return rpcResp.Result, nil
}

// Notify 发送 JSON-RPC 通知（不等待响应）。
func (t *HTTPTransport) Notify(method string, params json.RawMessage) error {
	if t.client == nil {
		return fmt.Errorf("MCP HTTP transport: not connected")
	}

	notifBody := rpcNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	bodyData, err := json.Marshal(notifBody)
	if err != nil {
		return fmt.Errorf("marshal notification: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, t.url, bytes.NewReader(bodyData))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range t.headers {
		req.Header.Set(k, v)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	resp.Body.Close()
	return nil
}

// Close 关闭空闲连接（幂等）。
func (t *HTTPTransport) Close() error {
	if t.client != nil {
		t.client.CloseIdleConnections()
	}
	return nil
}
