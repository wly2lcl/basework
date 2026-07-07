package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// JSON-RPC 标准错误码
const (
	ErrCodeParse           = -32700
	ErrCodeInvalidRequest  = -32600
	ErrCodeMethodNotFound  = -32601
	ErrCodeInvalidParams   = -32602
	ErrCodeInternal        = -32603
	ErrCodeServerNotInit   = -32002
	ErrCodeUnknown         = -32001
	ErrCodeRequestCanceled = -32800
	ErrCodeContentModified = -32801
)

// ResponseError 表示 JSON-RPC 响应中的错误。
type ResponseError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *ResponseError) Error() string {
	return fmt.Sprintf("JSON-RPC error %d: %s", e.Code, e.Message)
}

// Message 是 JSON-RPC 2.0 消息的基类型。
type Message struct {
	JSONRPC string `json:"jsonrpc"`
}

// Request 表示 JSON-RPC 请求。
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response 表示 JSON-RPC 响应。
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *ResponseError  `json:"error,omitempty"`
}

// Notification 表示 JSON-RPC 通知（无 ID）。
type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// rawMessage 用于解析收到的通用 JSON-RPC 消息。
type rawMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int            `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *ResponseError  `json:"error,omitempty"`
}

// pendingResponse 承载一个等待中的请求的响应结果。
type pendingResponse struct {
	result json.RawMessage
	err    error
}

// Conn 管理一条基于 stdio 的 JSON-RPC 2.0 连接。
type Conn struct {
	stdin  io.WriteCloser
	stdout io.ReadCloser

	reader    *bufio.Reader
	writeMu   sync.Mutex
	pendingMu sync.Mutex
	pending   map[int]chan pendingResponse
	nextID    atomic.Int64

	notifyMu sync.RWMutex
	handlers map[string]func(json.RawMessage)

	started   atomic.Bool
	done      chan struct{}
	closeOnce sync.Once

	// closed 标记连接已关闭，避免重复操作
	closed atomic.Bool
}

// NewConn 创建一个新的 JSON-RPC 连接。
func NewConn(stdin io.WriteCloser, stdout io.ReadCloser) *Conn {
	return &Conn{
		stdin:    stdin,
		stdout:   stdout,
		reader:   bufio.NewReader(stdout),
		pending:  make(map[int]chan pendingResponse),
		handlers: make(map[string]func(json.RawMessage)),
		done:     make(chan struct{}),
	}
}

// Start 启动后台读取协程来处理收到的消息。
func (c *Conn) Start() {
	if c.started.Load() {
		return
	}
	c.started.Store(true)
	go c.readLoop()
}

// Send 发送一个请求并等待响应。ctx 用于控制超时和取消。
func (c *Conn) Send(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	id := int(c.nextID.Add(1))

	var paramsRaw json.RawMessage
	if params != nil {
		var err error
		paramsRaw, err = json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("marshal params: %w", err)
		}
	}

	// 创建带缓冲的 channel，避免发送方阻塞
	ch := make(chan pendingResponse, 1)

	c.pendingMu.Lock()
	c.pending[id] = ch
	c.pendingMu.Unlock()

	// 函数退出时从 pending 映射中清理
	defer func() {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
	}()

	// 发送请求
	req := Request{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  paramsRaw,
	}
	if err := c.write(req); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}

	// 等待响应或上下文取消
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp := <-ch:
		return resp.result, resp.err
	}
}

// Notify 发送一个通知（不期待响应）。
func (c *Conn) Notify(method string, params interface{}) error {
	var paramsRaw json.RawMessage
	if params != nil {
		var err error
		paramsRaw, err = json.Marshal(params)
		if err != nil {
			return fmt.Errorf("marshal params: %w", err)
		}
	}

	notif := Notification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  paramsRaw,
	}
	return c.write(notif)
}

// Close 关闭连接，取消所有等待中的请求。
func (c *Conn) Close() error {
	var err error
	c.closeOnce.Do(func() {
		c.closed.Store(true)

		// 取消所有等待中的请求
		c.cancelPending(fmt.Errorf("connection closed"))

		// 关闭 stdin（向服务器发送 EOF）
		if cerr := c.stdin.Close(); cerr != nil {
			err = cerr
		}

		// 关闭 stdout
		if cerr := c.stdout.Close(); cerr != nil {
			err = cerr
		}

		close(c.done)
	})
	return err
}

// Done 返回一个 channel，当连接关闭时该 channel 被关闭。
func (c *Conn) Done() <-chan struct{} {
	return c.done
}

// OnNotification 注册一个通知处理方法。
func (c *Conn) OnNotification(method string, handler func(json.RawMessage)) {
	c.notifyMu.Lock()
	defer c.notifyMu.Unlock()
	c.handlers[method] = handler
}

// write 将消息序列化为 JSON 并以 Content-Length 帧格式写入。
func (c *Conn) write(msg interface{}) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	if _, err := c.stdin.Write([]byte(header)); err != nil {
		return err
	}
	if _, err := c.stdin.Write(data); err != nil {
		return err
	}
	return nil
}

// cancelPending 取消所有等待中的请求，向其发送指定的错误。
func (c *Conn) cancelPending(err error) {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	for id, ch := range c.pending {
		select {
		case ch <- pendingResponse{err: err}:
		default:
		}
		delete(c.pending, id)
	}
}

// readLoop 循环从 stdout 读取消息并分发。
func (c *Conn) readLoop() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("LSP readLoop panic: %v\n%s", r, debug.Stack())
			_ = c.Close()
		}
	}()

	defer func() {
		if !c.closed.Load() {
			c.cancelPending(fmt.Errorf("connection closed"))
		}
	}()

	for {
		contentLength, err := c.readHeaders()
		if err != nil {
			return
		}
		if contentLength <= 0 {
			continue
		}

		body := make([]byte, contentLength)
		if _, err := io.ReadFull(c.reader, body); err != nil {
			return
		}

		c.dispatchMessage(body)
	}
}

// readHeaders 读取 Content-Length 头部，返回消息体长度。
func (c *Conn) readHeaders() (int, error) {
	var contentLength int
	for {
		line, err := c.reader.ReadString('\n')
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
func (c *Conn) dispatchMessage(body []byte) {
	var msg rawMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return // 忽略无法解析的消息
	}

	switch {
	case msg.ID != nil && msg.Method != "":
		// 服务器发来的请求（如 workspace/configuration），回复 null
		resp := Response{
			JSONRPC: "2.0",
			ID:      *msg.ID,
			Result:  json.RawMessage("null"),
		}
		c.write(resp)

	case msg.ID != nil:
		// 对我们之前请求的响应
		c.dispatchResponse(*msg.ID, msg.Result, msg.Error)

	default:
		// 通知
		c.dispatchNotification(msg.Method, msg.Params)
	}
}

// dispatchResponse 将响应分发给对应的等待者。
func (c *Conn) dispatchResponse(id int, result json.RawMessage, rpcErr *ResponseError) {
	pr := pendingResponse{result: result}
	if rpcErr != nil {
		pr.err = rpcErr
	}

	c.pendingMu.Lock()
	ch, ok := c.pending[id]
	if ok {
		delete(c.pending, id)
	}
	c.pendingMu.Unlock()

	if ok {
		select {
		case ch <- pr:
		default:
		}
	}
}

// dispatchNotification 将通知分发给已注册的处理函数。
func (c *Conn) dispatchNotification(method string, params json.RawMessage) {
	c.notifyMu.RLock()
	handler, ok := c.handlers[method]
	c.notifyMu.RUnlock()
	if ok {
		handler(params)
	}
}
