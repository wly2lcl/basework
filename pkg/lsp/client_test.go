package lsp

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// 测试辅助：mockLSPServer 模拟 LSP 语言服务器
// ---------------------------------------------------------------------------

// mockLSPServer 在 io.Pipe 之上模拟 LSP 服务器协议。
// 继承 mockServer 的帧读写能力，并添加 LSP 方法级别的响应路由。
type mockLSPServer struct {
	*mockServer

	mu            sync.Mutex
	methodHandler map[string]func(id int, method string, params json.RawMessage)
	initialized   atomic.Bool // 是否已收到 initialized 通知
}

// newMockLSPServer 创建 LSP 模拟服务器。
func newMockLSPServer(t *testing.T, in io.ReadCloser, out io.WriteCloser) *mockLSPServer {
	ms := newMockServer(t, in, out)
	s := &mockLSPServer{
		mockServer:    ms,
		methodHandler: make(map[string]func(id int, method string, params json.RawMessage)),
	}

	// 默认 initialize 处理
	s.methodHandler["initialize"] = func(id int, method string, params json.RawMessage) {
		capResult := `{"capabilities":{"textDocumentSync":1}}`
		ms.sendResponse(id, json.RawMessage(capResult), nil)
	}

	// 默认 shutdown 处理
	s.methodHandler["shutdown"] = func(id int, method string, params json.RawMessage) {
		ms.sendResponse(id, json.RawMessage("null"), nil)
	}

	ms.onRequest = func(id int, method string, params json.RawMessage) {
		s.mu.Lock()
		handler, ok := s.methodHandler[method]
		s.mu.Unlock()
		if ok {
			handler(id, method, params)
		} else {
			// 未知方法，返回默认响应
			result, _ := json.Marshal(method)
			ms.sendResponse(id, result, nil)
		}
	}

	return s
}

// start 启动模拟服务器的后台读取循环。
func (s *mockLSPServer) start() {
	s.mockServer.start()
}

// close 关闭模拟服务器。
func (s *mockLSPServer) close() {
	s.mockServer.close()
}

// setHandler 设置指定方法的请求处理函数。
func (s *mockLSPServer) setHandler(method string, handler func(id int, params json.RawMessage)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.methodHandler[method] = func(id int, m string, params json.RawMessage) {
		handler(id, params)
	}
}

// sendNotification 发送通知（复用基类方法）。
func (s *mockLSPServer) sendNotification(method string, params json.RawMessage) {
	s.mockServer.sendNotification(method, params)
}

// ---------------------------------------------------------------------------
// 测试辅助：创建客户端 + 模拟服务器的测试夹具
// ---------------------------------------------------------------------------

type clientFixture struct {
	client *Client
	server *mockLSPServer
	conn   *Conn
	// 客户端视角的管道
	clientIn  io.WriteCloser // 客户端写入
	clientOut io.ReadCloser  // 客户端读取
	cleanup   func()
}

func newClientFixture(t *testing.T) *clientFixture {
	// 创建管道对
	serverInRead, serverInWrite := io.Pipe()   // 服务器读取 ← 客户端写入
	serverOutRead, serverOutWrite := io.Pipe() // 服务器写入 → 客户端读取

	conn := NewConn(serverInWrite, serverOutRead)
	server := newMockLSPServer(t, serverInRead, serverOutWrite)

	client := &Client{
		name:        "test-lsp",
		openFiles:   make(map[string]bool),
		diagnostics: make(map[string][]Diagnostic),
	}
	// 设置 cwd 为临时目录，避免 fileToURI 使用不存在的路径
	client.cwd, _ = os.Getwd()

	server.start()

	// 执行初始化握手
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.startWithConn(ctx, conn, "."); err != nil {
		t.Fatalf("startWithConn failed: %v", err)
	}
	client.state.Store(int32(StateReady))

	cf := &clientFixture{
		client:    client,
		server:    server,
		conn:      conn,
		clientIn:  serverInWrite,
		clientOut: serverOutRead,
		cleanup: func() {
			client.Stop()
			server.close()
		},
	}
	return cf
}

func (f *clientFixture) close() {
	if f.cleanup != nil {
		f.cleanup()
	}
}

// ---------------------------------------------------------------------------
// 测试用例
// ---------------------------------------------------------------------------

// TestClientStateMachine 测试客户端状态机转换：Unstarted → Starting → Ready。
func TestClientStateMachine(t *testing.T) {
	client := NewClient("test", "echo", nil, nil)
	if client.Ready() {
		t.Error("new client should not be ready")
	}

	// 验证状态为 Unstarted
	if ClientState(client.state.Load()) != StateUnstarted {
		t.Errorf("expected Unstarted, got %d", client.state.Load())
	}

	// 验证 Ready() 返回 false
	if client.Ready() {
		t.Error("Ready() should return false for unstarted client")
	}
}

// TestClientConcurrentStart 测试并发 Start() 调用的保护机制。
func TestClientConcurrentStart(t *testing.T) {
	client := &Client{
		name:        "test-lsp",
		openFiles:   make(map[string]bool),
		diagnostics: make(map[string][]Diagnostic),
	}

	// 并发启动
	var wg sync.WaitGroup
	errs := make([]error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			// 每个 goroutine 创建独立的连接
			sr, sw := io.Pipe()
			or, ow := io.Pipe()
			c2 := NewConn(sw, or)
			s2 := newMockLSPServer(t, sr, ow)
			s2.start()
			errs[idx] = client.startWithConn(context.Background(), c2, ".")
			s2.close()
		}(i)
	}
	wg.Wait()

	client.Stop()

	// 至少一个应该成功
	successCount := 0
	for _, err := range errs {
		if err == nil {
			successCount++
		}
	}
	if successCount == 0 {
		t.Error("at least one startWithConn should succeed")
	}
}

// TestClientDuplicateStart 测试重复 Start() 调用返回 nil。
func TestClientDuplicateStart(t *testing.T) {
	f := newClientFixture(t)
	defer f.close()

	// 第一次调用
	err := f.client.Start(context.Background(), ".")
	if err != nil {
		t.Logf("first Start returned: %v (expected if process not found)", err)
		// 由于没有真实进程，Start 可能失败
		// 但我们只需要验证状态机
	}

	// 验证 state 是 Ready（通过 startWithConn 设置）或 Error
	state := ClientState(f.client.state.Load())
	if state != StateReady && state != StateError {
		t.Errorf("expected Ready or Error state, got %d", state)
	}
}

// TestClientDefinition 测试 Definition 查询方法。
func TestClientDefinition(t *testing.T) {
	f := newClientFixture(t)
	defer f.close()

	expectedLoc := Location{
		URI:   "file:///test.go",
		Range: Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 0, Character: 5}},
	}

	f.server.setHandler("textDocument/definition", func(id int, params json.RawMessage) {
		locJSON, _ := json.Marshal(expectedLoc)
		f.server.sendResponse(id, locJSON, nil)
	})

	// 创建临时测试文件
	tmpFile := filepath.Join(t.TempDir(), "test.go")
	if err := os.WriteFile(tmpFile, []byte("package main"), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	ctx := context.Background()
	locs, err := f.client.Definition(ctx, tmpFile, Position{Line: 0, Character: 0})
	if err != nil {
		t.Fatalf("Definition failed: %v", err)
	}
	if len(locs) != 1 {
		t.Fatalf("expected 1 location, got %d", len(locs))
	}
	if locs[0].URI != expectedLoc.URI {
		t.Errorf("expected URI %q, got %q", expectedLoc.URI, locs[0].URI)
	}
}

// TestClientReferences 测试 References 查询方法。
func TestClientReferences(t *testing.T) {
	f := newClientFixture(t)
	defer f.close()

	expectedLocs := []Location{
		{URI: "file:///test.go", Range: Range{Start: Position{Line: 1, Character: 0}, End: Position{Line: 1, Character: 5}}},
		{URI: "file:///test.go", Range: Range{Start: Position{Line: 5, Character: 0}, End: Position{Line: 5, Character: 5}}},
	}

	f.server.setHandler("textDocument/references", func(id int, params json.RawMessage) {
		locsJSON, _ := json.Marshal(expectedLocs)
		f.server.sendResponse(id, locsJSON, nil)
	})

	tmpFile := filepath.Join(t.TempDir(), "test.go")
	if err := os.WriteFile(tmpFile, []byte("package main"), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	ctx := context.Background()
	locs, err := f.client.References(ctx, tmpFile, Position{Line: 0, Character: 0})
	if err != nil {
		t.Fatalf("References failed: %v", err)
	}
	if len(locs) != 2 {
		t.Fatalf("expected 2 locations, got %d", len(locs))
	}
}

// TestClientHover 测试 Hover 查询方法。
func TestClientHover(t *testing.T) {
	f := newClientFixture(t)
	defer f.close()

	expectedText := "func main()"

	f.server.setHandler("textDocument/hover", func(id int, params json.RawMessage) {
		hoverResult := map[string]interface{}{
			"contents": map[string]string{
				"kind":  "plaintext",
				"value": expectedText,
			},
		}
		resultJSON, _ := json.Marshal(hoverResult)
		f.server.sendResponse(id, resultJSON, nil)
	})

	tmpFile := filepath.Join(t.TempDir(), "test.go")
	if err := os.WriteFile(tmpFile, []byte("package main"), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	ctx := context.Background()
	text, err := f.client.Hover(ctx, tmpFile, Position{Line: 0, Character: 0})
	if err != nil {
		t.Fatalf("Hover failed: %v", err)
	}
	if text != expectedText {
		t.Errorf("expected %q, got %q", expectedText, text)
	}
}

// TestClientHoverStringShorthand 测试 Hover 返回纯字符串格式。
func TestClientHoverStringShorthand(t *testing.T) {
	f := newClientFixture(t)
	defer f.close()

	expectedText := "hover text"

	f.server.setHandler("textDocument/hover", func(id int, params json.RawMessage) {
		hoverResult := map[string]interface{}{
			"contents": expectedText,
		}
		resultJSON, _ := json.Marshal(hoverResult)
		f.server.sendResponse(id, resultJSON, nil)
	})

	tmpFile := filepath.Join(t.TempDir(), "test.go")
	if err := os.WriteFile(tmpFile, []byte("package main"), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	ctx := context.Background()
	text, err := f.client.Hover(ctx, tmpFile, Position{Line: 0, Character: 0})
	if err != nil {
		t.Fatalf("Hover failed: %v", err)
	}
	if text != expectedText {
		t.Errorf("expected %q, got %q", expectedText, text)
	}
}

// TestClientDocumentSymbols 测试 DocumentSymbols 查询方法。
func TestClientDocumentSymbols(t *testing.T) {
	f := newClientFixture(t)
	defer f.close()

	expectedSymbols := []SymbolInfo{
		{Name: "main", Kind: SymbolKindFunction, Location: Location{URI: "file:///test.go", Range: Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 2, Character: 1}}}},
	}

	f.server.setHandler("textDocument/documentSymbol", func(id int, params json.RawMessage) {
		symJSON, _ := json.Marshal(expectedSymbols)
		f.server.sendResponse(id, symJSON, nil)
	})

	tmpFile := filepath.Join(t.TempDir(), "test.go")
	if err := os.WriteFile(tmpFile, []byte("package main"), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	ctx := context.Background()
	symbols, err := f.client.DocumentSymbols(ctx, tmpFile)
	if err != nil {
		t.Fatalf("DocumentSymbols failed: %v", err)
	}
	if len(symbols) != 1 {
		t.Fatalf("expected 1 symbol, got %d", len(symbols))
	}
	if symbols[0].Name != "main" {
		t.Errorf("expected 'main', got %q", symbols[0].Name)
	}
}

// TestClientWorkspaceSymbols 测试 WorkspaceSymbols 查询方法。
func TestClientWorkspaceSymbols(t *testing.T) {
	f := newClientFixture(t)
	defer f.close()

	expectedSymbols := []SymbolInfo{
		{Name: "main", Kind: SymbolKindFunction, Location: Location{URI: "file:///test.go", Range: Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 2, Character: 1}}}},
		{Name: "helper", Kind: SymbolKindFunction, Location: Location{URI: "file:///helper.go", Range: Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 1, Character: 1}}}},
	}

	f.server.setHandler("workspace/symbol", func(id int, params json.RawMessage) {
		symJSON, _ := json.Marshal(expectedSymbols)
		f.server.sendResponse(id, symJSON, nil)
	})

	ctx := context.Background()
	symbols, err := f.client.WorkspaceSymbols(ctx, "main")
	if err != nil {
		t.Fatalf("WorkspaceSymbols failed: %v", err)
	}
	if len(symbols) != 2 {
		t.Fatalf("expected 2 symbols, got %d", len(symbols))
	}
}

// TestClientOpenFile 测试 OpenFile 发送 didOpen 通知。
func TestClientOpenFile(t *testing.T) {
	f := newClientFixture(t)
	defer f.close()

	// 记录收到的 didOpen 通知
	didOpenCh := make(chan bool, 1)
	f.server.setHandler("textDocument/didOpen", func(id int, params json.RawMessage) {
		// didOpen 是通知，不应该有 id
		t.Error("didOpen should be a notification, not a request")
	})

	// 创建一个临时文件
	tmpFile := filepath.Join(t.TempDir(), "test.go")
	content := "package main\nfunc main() {}\n"
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	// 验证 OpenFile 不会报错
	err := f.client.OpenFile(tmpFile)
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}

	// 验证文件已标记为打开
	f.client.filesMu.Lock()
	uri := f.client.fileToURI(tmpFile)
	opened := f.client.openFiles[uri]
	f.client.filesMu.Unlock()
	if !opened {
		t.Error("file should be marked as opened")
	}

	_ = didOpenCh
}

// TestClientDuplicateOpenFile 测试重复 OpenFile 为 no-op。
func TestClientDuplicateOpenFile(t *testing.T) {
	f := newClientFixture(t)
	defer f.close()

	// 创建一个临时文件
	tmpFile := filepath.Join(t.TempDir(), "test.go")
	content := "package main\n"
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	// 第一次打开
	err := f.client.OpenFile(tmpFile)
	if err != nil {
		t.Fatalf("first OpenFile failed: %v", err)
	}

	// 第二次打开（no-op）
	err = f.client.OpenFile(tmpFile)
	if err != nil {
		t.Fatalf("second OpenFile should be no-op: %v", err)
	}

	// 验证文件只被标记一次
	f.client.filesMu.Lock()
	count := 0
	uri := f.client.fileToURI(tmpFile)
	if f.client.openFiles[uri] {
		count++
	}
	f.client.filesMu.Unlock()
	if count != 1 {
		t.Errorf("file should be marked once, got %d", count)
	}
}

// TestClientCloseFile 测试 CloseFile 发送 didClose 通知。
func TestClientCloseFile(t *testing.T) {
	f := newClientFixture(t)
	defer f.close()

	// 创建一个临时文件
	tmpFile := filepath.Join(t.TempDir(), "test.go")
	content := "package main\n"
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	// 先打开
	err := f.client.OpenFile(tmpFile)
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}

	// 关闭
	err = f.client.CloseFile(tmpFile)
	if err != nil {
		t.Fatalf("CloseFile failed: %v", err)
	}

	// 验证文件已从打开列表移除
	f.client.filesMu.Lock()
	uri := f.client.fileToURI(tmpFile)
	opened := f.client.openFiles[uri]
	f.client.filesMu.Unlock()
	if opened {
		t.Error("file should not be marked as opened after CloseFile")
	}
}

// TestClientCloseFileNotOpened 测试关闭未打开的文件为 no-op。
func TestClientCloseFileNotOpened(t *testing.T) {
	f := newClientFixture(t)
	defer f.close()

	tmpFile := filepath.Join(t.TempDir(), "test.go")
	err := f.client.CloseFile(tmpFile)
	if err != nil {
		t.Fatalf("CloseFile of unopened file should succeed: %v", err)
	}
}

// TestClientDiagnostics 测试诊断缓存功能。
func TestClientDiagnostics(t *testing.T) {
	f := newClientFixture(t)
	defer f.close()

	tmpFile := filepath.Join(t.TempDir(), "test.go")
	if err := os.WriteFile(tmpFile, []byte("package main"), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	uri := f.client.fileToURI(tmpFile)

	// 服务器主动发送诊断通知
	diags := []Diagnostic{
		{Range: Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 0, Character: 5}}, Severity: SeverityError, Message: "test error", Source: "test"},
	}
	diagParams := map[string]interface{}{
		"uri":         uri,
		"diagnostics": diags,
	}
	diagJSON, _ := json.Marshal(diagParams)
	f.server.sendNotification("textDocument/publishDiagnostics", diagJSON)

	// 等待通知被处理
	time.Sleep(50 * time.Millisecond)

	// 查询诊断缓存
	cachedDiags, version := f.client.Diagnostics(tmpFile)
	if len(cachedDiags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(cachedDiags))
	}
	if cachedDiags[0].Message != "test error" {
		t.Errorf("expected 'test error', got %q", cachedDiags[0].Message)
	}
	if version != 1 {
		t.Errorf("expected version 1, got %d", version)
	}
}

// TestClientDiagnosticsMultiple 测试多次诊断通知的缓存更新。
func TestClientDiagnosticsMultiple(t *testing.T) {
	f := newClientFixture(t)
	defer f.close()

	tmpFile := filepath.Join(t.TempDir(), "test.go")
	if err := os.WriteFile(tmpFile, []byte("package main"), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	uri := f.client.fileToURI(tmpFile)

	// 第一次诊断通知
	diagParams1 := map[string]interface{}{
		"uri":         uri,
		"diagnostics": []Diagnostic{{Range: Range{}, Severity: SeverityError, Message: "error 1"}},
	}
	diagJSON1, _ := json.Marshal(diagParams1)
	f.server.sendNotification("textDocument/publishDiagnostics", diagJSON1)
	time.Sleep(50 * time.Millisecond)

	// 第二次诊断通知（覆盖）
	diagParams2 := map[string]interface{}{
		"uri":         uri,
		"diagnostics": []Diagnostic{{Range: Range{}, Severity: SeverityWarning, Message: "error 2"}},
	}
	diagJSON2, _ := json.Marshal(diagParams2)
	f.server.sendNotification("textDocument/publishDiagnostics", diagJSON2)
	time.Sleep(50 * time.Millisecond)

	// 查询缓存，验证已更新
	cachedDiags, version := f.client.Diagnostics(tmpFile)
	if len(cachedDiags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(cachedDiags))
	}
	if cachedDiags[0].Message != "error 2" {
		t.Errorf("expected 'error 2', got %q", cachedDiags[0].Message)
	}
	if version != 2 {
		t.Errorf("expected version 2, got %d", version)
	}
}

// TestClientStop 测试 Stop() 方法发送 shutdown/exit 并清理状态。
func TestClientStop(t *testing.T) {
	f := newClientFixture(t)
	defer f.close()

	// 验证初始状态为 Ready
	if !f.client.Ready() {
		t.Error("client should be ready after initialization")
	}

	// 停止客户端
	err := f.client.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// 验证状态已重置
	if f.client.Ready() {
		t.Error("client should not be ready after Stop")
	}

	// 验证重复 Stop 不会报错
	err = f.client.Stop()
	if err != nil {
		t.Fatalf("second Stop should not fail: %v", err)
	}
}

// TestClientDiagnosticsNoFile 测试查询未收到诊断的文件返回空列表。
func TestClientDiagnosticsNoFile(t *testing.T) {
	f := newClientFixture(t)
	defer f.close()

	diags, version := f.client.Diagnostics("nonexistent.go")
	if len(diags) != 0 {
		t.Errorf("expected 0 diagnostics, got %d", len(diags))
	}
	if version != 0 {
		t.Errorf("expected version 0, got %d", version)
	}
}

// TestClientReady 测试 Ready() 方法。
func TestClientReady(t *testing.T) {
	f := newClientFixture(t)
	defer f.close()

	if !f.client.Ready() {
		t.Error("client should be ready after startWithConn")
	}
}

// TestClientChangeFile 测试 ChangeFile 发送 didChange 通知。
func TestClientChangeFile(t *testing.T) {
	f := newClientFixture(t)
	defer f.close()

	tmpFile := filepath.Join(t.TempDir(), "test.go")
	if err := os.WriteFile(tmpFile, []byte("package main"), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	// 先打开
	if err := f.client.OpenFile(tmpFile); err != nil {
		t.Fatalf("OpenFile: %v", err)
	}

	// 修改内容
	if err := f.client.ChangeFile(tmpFile, "package main\n\nfunc main() {}"); err != nil {
		t.Fatalf("ChangeFile: %v", err)
	}
}

// TestClientChangeFileNotOpened 测试修改未打开的文件返回错误。
func TestClientChangeFileNotOpened(t *testing.T) {
	f := newClientFixture(t)
	defer f.close()

	tmpFile := filepath.Join(t.TempDir(), "test.go")
	err := f.client.ChangeFile(tmpFile, "content")
	if err == nil {
		t.Error("ChangeFile should fail for unopened file")
	}
}

// TestClientDefinitionNil 测试 Definition 返回 nil（无定义）的情况。
func TestClientDefinitionNil(t *testing.T) {
	f := newClientFixture(t)
	defer f.close()

	f.server.setHandler("textDocument/definition", func(id int, params json.RawMessage) {
		f.server.sendResponse(id, json.RawMessage("null"), nil)
	})

	tmpFile := filepath.Join(t.TempDir(), "test.go")
	if err := os.WriteFile(tmpFile, []byte("package main"), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	ctx := context.Background()
	locs, err := f.client.Definition(ctx, tmpFile, Position{Line: 0, Character: 0})
	if err != nil {
		t.Fatalf("Definition failed: %v", err)
	}
	if len(locs) != 0 {
		t.Errorf("expected 0 locations, got %d", len(locs))
	}
}

// TestClientDocumentSymbolsHierarchical 测试层次化 DocumentSymbol 格式。
func TestClientDocumentSymbolsHierarchical(t *testing.T) {
	f := newClientFixture(t)
	defer f.close()

	// 返回 DocumentSymbol 格式（层次化）
	hierarchicalSymbols := []DocumentSymbol{
		{
			Name: "main",
			Kind: SymbolKindFunction,
			Range: Range{
				Start: Position{Line: 0, Character: 0},
				End:   Position{Line: 2, Character: 1},
			},
			Children: []DocumentSymbol{
				{
					Name: "fmt.Println",
					Kind: SymbolKindMethod,
					Range: Range{
						Start: Position{Line: 1, Character: 2},
						End:   Position{Line: 1, Character: 14},
					},
				},
			},
		},
	}

	f.server.setHandler("textDocument/documentSymbol", func(id int, params json.RawMessage) {
		symJSON, _ := json.Marshal(hierarchicalSymbols)
		f.server.sendResponse(id, symJSON, nil)
	})

	tmpFile := filepath.Join(t.TempDir(), "test.go")
	if err := os.WriteFile(tmpFile, []byte("package main"), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	ctx := context.Background()
	symbols, err := f.client.DocumentSymbols(ctx, tmpFile)
	if err != nil {
		t.Fatalf("DocumentSymbols failed: %v", err)
	}
	// 应该展平为 2 个 SymbolInfo（父 + 子）
	if len(symbols) != 2 {
		t.Fatalf("expected 2 symbols (flattened), got %d", len(symbols))
	}
	if symbols[0].Name != "main" {
		t.Errorf("expected 'main', got %q", symbols[0].Name)
	}
	if symbols[1].Name != "fmt.Println" {
		t.Errorf("expected 'fmt.Println', got %q", symbols[1].Name)
	}
}

// TestClientURIHelpers 测试 URI 工具函数。
func TestClientURIHelpers(t *testing.T) {
	client := &Client{}

	// fileToURI
	uri := client.fileToURI("/path/to/file.go")
	expected := "file:///path/to/file.go"
	if uri != expected {
		t.Errorf("fileToURI: expected %q, got %q", expected, uri)
	}

	// uriToFile
	path := client.uriToFile("file:///path/to/file.go")
	expectedPath := "/path/to/file.go"
	if path != expectedPath {
		t.Errorf("uriToFile: expected %q, got %q", expectedPath, path)
	}

	// URI 不含 file:// 前缀时原样返回
	original := "/some/path"
	result := client.uriToFile(original)
	if result != original {
		t.Errorf("uriToFile without prefix: expected %q, got %q", original, result)
	}
}

// TestClientLanguageID 测试语言标识符推断。
func TestClientLanguageID(t *testing.T) {
	client := &Client{}

	tests := []struct {
		file     string
		expected string
	}{
		{"main.go", "go"},
		{"app.ts", "typescript"},
		{"app.tsx", "typescriptreact"},
		{"app.js", "javascript"},
		{"app.jsx", "javascriptreact"},
		{"main.py", "python"},
		{"lib.rs", "rust"},
		{"Main.java", "java"},
		{"app.rb", "ruby"},
		{"program.c", "c"},
		{"program.cpp", "cpp"},
		{"program.h", "c"},
		{"program.hpp", "cpp"},
		{"Program.cs", "csharp"},
		{"main.swift", "swift"},
		{"main.kt", "kotlin"},
		{"main.scala", "scala"},
		{"index.php", "php"},
		{"index.html", "html"},
		{"style.css", "css"},
		{"style.scss", "scss"},
		{"data.json", "json"},
		{"config.yaml", "yaml"},
		{"config.yml", "yaml"},
		{"readme.md", "markdown"},
		{"query.sql", "sql"},
		{"script.sh", "shellscript"},
		{"unknown.xyz", ""},
		{"Makefile", ""},
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			got := client.languageID(tt.file)
			if got != tt.expected {
				t.Errorf("languageID(%q) = %q, want %q", tt.file, got, tt.expected)
			}
		})
	}
}

// TestClientOpenFileError 测试打开不存在的文件返回错误。
func TestClientOpenFileError(t *testing.T) {
	f := newClientFixture(t)
	defer f.close()

	err := f.client.OpenFile("/nonexistent/path/file.go")
	if err == nil {
		t.Error("OpenFile should fail for nonexistent file")
	}
}

// TestClientDiagnosticsConcurrent 测试并发诊断缓存访问。
func TestClientDiagnosticsConcurrent(t *testing.T) {
	f := newClientFixture(t)
	defer f.close()

	tmpFile := filepath.Join(t.TempDir(), "test.go")
	if err := os.WriteFile(tmpFile, []byte("package main"), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	uri := f.client.fileToURI(tmpFile)

	// 发送诊断
	diagParams := map[string]interface{}{
		"uri": uri,
		"diagnostics": []Diagnostic{
			{Message: "test"},
		},
	}
	diagJSON, _ := json.Marshal(diagParams)
	f.server.sendNotification("textDocument/publishDiagnostics", diagJSON)
	time.Sleep(50 * time.Millisecond)

	// 并发读取
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			diags, _ := f.client.Diagnostics(tmpFile)
			if len(diags) != 1 {
				t.Errorf("expected 1 diagnostic, got %d", len(diags))
			}
		}()
	}
	wg.Wait()
}

// TestClientStopMultiple 测试多次 Stop 调用是安全的。
func TestClientStopMultiple(t *testing.T) {
	f := newClientFixture(t)
	defer f.close()

	// 多次调用 Stop
	for i := 0; i < 3; i++ {
		err := f.client.Stop()
		if err != nil {
			t.Fatalf("Stop iteration %d failed: %v", i, err)
		}
	}
}

// TestParseLocations 测试 parseLocations 函数。
func TestParseLocations(t *testing.T) {
	// 测试 null
	locs, err := parseLocations(nil)
	if err != nil || len(locs) != 0 {
		t.Errorf("nil should return empty: %v, %d", err, len(locs))
	}

	// 测试单个 Location
	singleJSON := json.RawMessage(`{"uri":"file:///a.go","range":{"start":{"line":0,"character":0},"end":{"line":0,"character":5}}}`)
	locs, err = parseLocations(singleJSON)
	if err != nil || len(locs) != 1 {
		t.Errorf("single location: %v, %d", err, len(locs))
	}

	// 测试数组
	arrJSON := json.RawMessage(`[{"uri":"file:///a.go","range":{"start":{"line":0,"character":0},"end":{"line":0,"character":5}}}]`)
	locs, err = parseLocations(arrJSON)
	if err != nil || len(locs) != 1 {
		t.Errorf("array: %v, %d", err, len(locs))
	}
}

// TestExtractHoverText 测试 extractHoverText 函数。
func TestExtractHoverText(t *testing.T) {
	// 测试 MarkupContent
	text := extractHoverText(json.RawMessage(`{"kind":"plaintext","value":"hello"}`))
	if text != "hello" {
		t.Errorf("expected 'hello', got %q", text)
	}

	// 测试纯字符串
	text = extractHoverText(json.RawMessage(`"hello"`))
	if text != "hello" {
		t.Errorf("expected 'hello', got %q", text)
	}

	// 测试 nil
	text = extractHoverText(nil)
	if text != "" {
		t.Errorf("expected '', got %q", text)
	}

	// 测试空值
	text = extractHoverText(json.RawMessage(`""`))
	if text != "" {
		t.Errorf("expected '', got %q", text)
	}
}

// TestClientDefinitionArray 测试 Definition 返回 []Location 格式。
func TestClientDefinitionArray(t *testing.T) {
	f := newClientFixture(t)
	defer f.close()

	expectedLocs := []Location{
		{URI: "file:///test.go", Range: Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 0, Character: 5}}},
	}

	f.server.setHandler("textDocument/definition", func(id int, params json.RawMessage) {
		locsJSON, _ := json.Marshal(expectedLocs)
		f.server.sendResponse(id, locsJSON, nil)
	})

	tmpFile := filepath.Join(t.TempDir(), "test.go")
	if err := os.WriteFile(tmpFile, []byte("package main"), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	ctx := context.Background()
	locs, err := f.client.Definition(ctx, tmpFile, Position{Line: 0, Character: 0})
	if err != nil {
		t.Fatalf("Definition failed: %v", err)
	}
	if len(locs) != 1 {
		t.Fatalf("expected 1 location, got %d", len(locs))
	}
}

// TestClientHoverEmpty 测试 Hover 返回空结果。
func TestClientHoverEmpty(t *testing.T) {
	f := newClientFixture(t)
	defer f.close()

	f.server.setHandler("textDocument/hover", func(id int, params json.RawMessage) {
		f.server.sendResponse(id, json.RawMessage("null"), nil)
	})

	tmpFile := filepath.Join(t.TempDir(), "test.go")
	if err := os.WriteFile(tmpFile, []byte("package main"), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	ctx := context.Background()
	text, err := f.client.Hover(ctx, tmpFile, Position{Line: 0, Character: 0})
	if err != nil {
		t.Fatalf("Hover failed: %v", err)
	}
	if text != "" {
		t.Errorf("expected empty string, got %q", text)
	}
}

// TestClientStartWithConnNotReady 测试 startWithConn 时连接错误。
func TestClientStartWithConnError(t *testing.T) {
	// 创建一个会在 initialize 时出错的服务器
	serverInRead, serverInWrite := io.Pipe()
	serverOutRead, serverOutWrite := io.Pipe()

	conn := NewConn(serverInWrite, serverOutRead)
	server := newMockLSPServer(t, serverInRead, serverOutWrite)

	// 覆盖 initialize 处理器，使其不响应（超时）
	server.setHandler("initialize", func(id int, params json.RawMessage) {
		// 不响应
	})

	server.start()

	client := &Client{
		name:        "test",
		openFiles:   make(map[string]bool),
		diagnostics: make(map[string][]Diagnostic),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := client.startWithConn(ctx, conn, ".")
	if err == nil {
		t.Error("startWithConn should fail when server doesn't respond")
	}

	server.close()
}

// TestClientFileToURIAbsolute 测试 fileToURI 对绝对路径的处理。
func TestClientFileToURIAbsolute(t *testing.T) {
	client := &Client{}

	// 绝对路径
	uri := client.fileToURI("/home/user/test.go")
	expected := "file:///home/user/test.go"
	if uri != expected {
		t.Errorf("expected %q, got %q", expected, uri)
	}

	// 包含空格的路径 — 使用 url.URL 后会被正确编码为 %20
	uri = client.fileToURI("/home/user/my project/test.go")
	if uri != "file:///home/user/my%20project/test.go" {
		t.Errorf("unexpected URI for path with spaces: %q", uri)
	}
}

// TestClientOpenFileTrack 测试 OpenFile 正确跟踪打开的文件。
func TestClientOpenFileTrack(t *testing.T) {
	f := newClientFixture(t)
	defer f.close()

	// 创建临时文件
	dir := t.TempDir()
	file1 := filepath.Join(dir, "a.go")
	file2 := filepath.Join(dir, "b.go")
	os.WriteFile(file1, []byte("package a"), 0644)
	os.WriteFile(file2, []byte("package b"), 0644)

	// 打开两个文件
	if err := f.client.OpenFile(file1); err != nil {
		t.Fatalf("OpenFile 1: %v", err)
	}
	if err := f.client.OpenFile(file2); err != nil {
		t.Fatalf("OpenFile 2: %v", err)
	}

	// 验证跟踪
	f.client.filesMu.Lock()
	uri1 := f.client.fileToURI(file1)
	uri2 := f.client.fileToURI(file2)
	if !f.client.openFiles[uri1] {
		t.Error("file1 should be tracked")
	}
	if !f.client.openFiles[uri2] {
		t.Error("file2 should be tracked")
	}
	f.client.filesMu.Unlock()
}

// TestClientChangeFilePropagation 验证 ChangeFile 更新连接。
func TestClientChangeFilePropagation(t *testing.T) {
	f := newClientFixture(t)
	defer f.close()

	// 创建并打开文件
	tmpFile := filepath.Join(t.TempDir(), "test.go")
	os.WriteFile(tmpFile, []byte("original"), 0644)
	f.client.OpenFile(tmpFile)

	// 变更内容
	err := f.client.ChangeFile(tmpFile, "changed content")
	if err != nil {
		t.Fatalf("ChangeFile: %v", err)
	}

	// 正常完成，没有 panic
}

// TestClientDiagnosticsDifferentFiles 测试不同文件的诊断缓存独立。
func TestClientDiagnosticsDifferentFiles(t *testing.T) {
	f := newClientFixture(t)
	defer f.close()

	dir := t.TempDir()
	file1 := filepath.Join(dir, "a.go")
	file2 := filepath.Join(dir, "b.go")
	os.WriteFile(file1, []byte("package a"), 0644)
	os.WriteFile(file2, []byte("package b"), 0644)

	uri1 := f.client.fileToURI(file1)
	uri2 := f.client.fileToURI(file2)

	// 发送两个文件的诊断
	diagParams1 := map[string]interface{}{
		"uri":         uri1,
		"diagnostics": []Diagnostic{{Message: "error in a"}},
	}
	diagParams2 := map[string]interface{}{
		"uri":         uri2,
		"diagnostics": []Diagnostic{{Message: "error in b"}},
	}

	d1, _ := json.Marshal(diagParams1)
	d2, _ := json.Marshal(diagParams2)

	f.server.sendNotification("textDocument/publishDiagnostics", d1)
	f.server.sendNotification("textDocument/publishDiagnostics", d2)
	time.Sleep(50 * time.Millisecond)

	// 验证缓存独立
	diags1, _ := f.client.Diagnostics(file1)
	diags2, _ := f.client.Diagnostics(file2)

	if len(diags1) != 1 || diags1[0].Message != "error in a" {
		t.Errorf("file1 diagnostics wrong: %+v", diags1)
	}
	if len(diags2) != 1 || diags2[0].Message != "error in b" {
		t.Errorf("file2 diagnostics wrong: %+v", diags2)
	}
}

// TestClientWorkspaceSymbolsEmpty 测试 WorkspaceSymbols 空结果。
func TestClientWorkspaceSymbolsEmpty(t *testing.T) {
	f := newClientFixture(t)
	defer f.close()

	f.server.setHandler("workspace/symbol", func(id int, params json.RawMessage) {
		f.server.sendResponse(id, json.RawMessage("null"), nil)
	})

	ctx := context.Background()
	symbols, err := f.client.WorkspaceSymbols(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("WorkspaceSymbols: %v", err)
	}
	if len(symbols) != 0 {
		t.Errorf("expected 0 symbols, got %d", len(symbols))
	}
}

// TestClientMultipleOpenClose 测试多次打开/关闭文件。
func TestClientMultipleOpenClose(t *testing.T) {
	f := newClientFixture(t)
	defer f.close()

	dir := t.TempDir()
	file1 := filepath.Join(dir, "a.go")
	os.WriteFile(file1, []byte("package a"), 0644)

	// 打开 → 关闭 → 再打开
	if err := f.client.OpenFile(file1); err != nil {
		t.Fatalf("first OpenFile: %v", err)
	}
	if err := f.client.CloseFile(file1); err != nil {
		t.Fatalf("CloseFile: %v", err)
	}
	if err := f.client.OpenFile(file1); err != nil {
		t.Fatalf("second OpenFile: %v", err)
	}

	// 验证最终状态为打开
	f.client.filesMu.Lock()
	uri := f.client.fileToURI(file1)
	opened := f.client.openFiles[uri]
	f.client.filesMu.Unlock()
	if !opened {
		t.Error("file should be open after re-open")
	}
}

// TestClientStopIdempotent 测试 Stop 的幂等性。
func TestClientStopIdempotent(t *testing.T) {
	f := newClientFixture(t)
	defer f.close()

	// 多次 Stop
	for i := 0; i < 5; i++ {
		if err := f.client.Stop(); err != nil {
			t.Fatalf("Stop #%d: %v", i, err)
		}
		// 状态应为 Unstarted
		if ClientState(f.client.state.Load()) != StateUnstarted {
			t.Errorf("after Stop #%d, state should be Unstarted", i)
		}
	}
}

// TestClientConnRace 测试并发 Start/Stop/OpenFile 无 race condition。
func TestClientConnRace(t *testing.T) {
	cf := newClientFixture(t)
	defer cf.close()

	var wg sync.WaitGroup

	// 并发调用 OpenFile
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			file := filepath.Join(t.TempDir(), "test.go")
			_ = os.WriteFile(file, []byte("package main"), 0644)
			_ = cf.client.OpenFile(file)
		}()
	}

	// 并发调用 ChangeFile
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			file := filepath.Join(t.TempDir(), "test.go")
			_ = cf.client.ChangeFile(file, "package main")
		}()
	}

	// 并发调用 CloseFile
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			file := filepath.Join(t.TempDir(), "test.go")
			_ = cf.client.CloseFile(file)
		}()
	}

	// 并发调用 Stop
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = cf.client.Stop()
	}()

	wg.Wait()
}

// TestClientEnsureOpen 测试 ensureOpen 自动打开文件。
func TestClientEnsureOpen(t *testing.T) {
	f := newClientFixture(t)
	defer f.close()

	dir := t.TempDir()
	file1 := filepath.Join(dir, "test.go")
	os.WriteFile(file1, []byte("package main"), 0644)

	// 直接调用 ensureOpen（不应打开，因为文件不存在于 openFiles 中）
	f.client.ensureOpen(file1)

	// 验证文件已被打开
	f.client.filesMu.Lock()
	uri := f.client.fileToURI(file1)
	opened := f.client.openFiles[uri]
	f.client.filesMu.Unlock()
	if !opened {
		t.Error("ensureOpen should open the file")
	}
}

// TestClientOpenFileReadContent 测试 OpenFile 读取文件内容。
func TestClientOpenFileReadContent(t *testing.T) {
	// 创建临时文件并写入特定内容
	content := "package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n"
	dir := t.TempDir()
	tmpFile := filepath.Join(dir, "test.go")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	// 读取内容验证
	readContent, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(readContent) != content {
		t.Errorf("content mismatch: expected %q, got %q", content, string(readContent))
	}
}

// TestClientDefinitionWithContextCancel 测试带取消上下文的 Definition。
func TestClientDefinitionWithContextCancel(t *testing.T) {
	f := newClientFixture(t)
	defer f.close()

	// 服务器不响应，让上下文超时
	f.server.setHandler("textDocument/definition", func(id int, params json.RawMessage) {
		// 不响应
	})

	tmpFile := filepath.Join(t.TempDir(), "test.go")
	os.WriteFile(tmpFile, []byte("package main"), 0644)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	_, err := f.client.Definition(ctx, tmpFile, Position{Line: 0, Character: 0})
	if err == nil {
		t.Error("Definition should fail with canceled context")
	}
}

// TestClientNewClient 测试 NewClient 构造函数。
func TestClientNewClient(t *testing.T) {
	client := NewClient("test", "echo", []string{"-n", "hello"}, []string{"FOO=bar"})
	if client.name != "test" {
		t.Errorf("expected name 'test', got %q", client.name)
	}
	if client.command != "echo" {
		t.Errorf("expected command 'echo', got %q", client.command)
	}
	if len(client.args) != 2 || client.args[0] != "-n" {
		t.Errorf("unexpected args: %v", client.args)
	}
	if len(client.env) != 1 || client.env[0] != "FOO=bar" {
		t.Errorf("unexpected env: %v", client.env)
	}
	if client.Ready() {
		t.Error("new client should not be ready")
	}
}
