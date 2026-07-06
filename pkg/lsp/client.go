package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ClientState 表示 LSP 客户端的当前状态。
type ClientState int32

const (
	// StateUnstarted 表示客户端尚未启动。
	StateUnstarted ClientState = iota
	// StateStarting 表示客户端正在启动中。
	StateStarting
	// StateReady 表示客户端已就绪，可以处理请求。
	StateReady
	// StateError 表示客户端处于错误状态。
	StateError
)

// Client 管理一个 LSP 语言服务器进程的完整生命周期。
type Client struct {
	name    string
	command string
	args    []string
	env     []string
	cwd     string

	conn *Conn
	cmd  *exec.Cmd
	state atomic.Int32

	startMu   sync.Mutex
	startErr  error

	// 已打开文件列表（URI → 是否已打开）
	openFiles map[string]bool
	filesMu   sync.Mutex

	// 诊断缓存（URI → []Diagnostic）
	diagnostics map[string][]Diagnostic
	diagVersion uint64
	diagMu      sync.RWMutex
}

// publishDiagnosticsParams 是 textDocument/publishDiagnostics 通知的参数。
type publishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// DocumentSymbol 表示 LSP DocumentSymbol（层次化结构）。
type DocumentSymbol struct {
	Name     string           `json:"name"`
	Detail   string           `json:"detail,omitempty"`
	Kind     SymbolKind       `json:"kind"`
	Range    Range            `json:"range"`
	Children []DocumentSymbol `json:"children,omitempty"`
}

// initializeParams 是 initialize 请求的参数。
type initializeParams struct {
	ProcessID    *int         `json:"processId"`
	RootURI      string       `json:"rootUri"`
	Capabilities capabilities `json:"capabilities"`
}

// capabilities 表示客户端能力声明。
type capabilities struct {
	TextDocument textDocumentClientCapabilities `json:"textDocument"`
	Workspace    workspaceClientCapabilities    `json:"workspace"`
}

type textDocumentClientCapabilities struct {
	Definition         *definitionCap     `json:"definition,omitempty"`
	References         map[string]bool    `json:"references,omitempty"`
	Hover              *hoverCap          `json:"hover,omitempty"`
	DocumentSymbol     *documentSymbolCap `json:"documentSymbol,omitempty"`
	PublishDiagnostics map[string]bool    `json:"publishDiagnostics,omitempty"`
}

type definitionCap struct {
	LinkSupport *bool `json:"linkSupport,omitempty"`
}

type hoverCap struct {
	ContentFormat []string `json:"contentFormat,omitempty"`
}

type documentSymbolCap struct {
	HierarchicalDocumentSymbolSupport *bool `json:"hierarchicalDocumentSymbolSupport,omitempty"`
}

type workspaceClientCapabilities struct {
	WorkspaceFolders *bool `json:"workspaceFolders,omitempty"`
}

// textDocumentID 表示一个文本文档的 URI 标识。
type textDocumentID struct {
	URI string `json:"uri"`
}

// textDocumentPositionParams 是文本文档 + 位置的参数结构。
type textDocumentPositionParams struct {
	TextDocument textDocumentID `json:"textDocument"`
	Position     Position       `json:"position"`
}

// textDocumentParams 是仅含文本文档引用的参数结构。
type textDocumentParams struct {
	TextDocument textDocumentID `json:"textDocument"`
}

// didOpenParams 是 textDocument/didOpen 通知的参数。
type didOpenParams struct {
	TextDocument struct {
		URI        string `json:"uri"`
		LanguageID string `json:"languageId"`
		Version    int    `json:"version"`
		Text       string `json:"text"`
	} `json:"textDocument"`
}

// didChangeParams 是 textDocument/didChange 通知的参数。
type didChangeParams struct {
	TextDocument struct {
		URI     string `json:"uri"`
		Version int    `json:"version"`
	} `json:"textDocument"`
	ContentChanges []struct {
		Text string `json:"text"`
	} `json:"contentChanges"`
}

// didCloseParams 是 textDocument/didClose 通知的参数。
type didCloseParams struct {
	TextDocument textDocumentID `json:"textDocument"`
}

// referenceContext 是 textDocument/references 请求的上下文。
type referenceContext struct {
	IncludeDeclaration bool `json:"includeDeclaration"`
}

// referencesParams 是 textDocument/references 请求的参数。
type referencesParams struct {
	TextDocument textDocumentID  `json:"textDocument"`
	Position     Position         `json:"position"`
	Context      referenceContext `json:"context"`
}

// NewClient 创建一个新的 LSP 客户端，初始状态为 Unstarted。
func NewClient(name, command string, args []string, env []string) *Client {
	return &Client{
		name:        name,
		command:     command,
		args:        args,
		env:         env,
		openFiles:   make(map[string]bool),
		diagnostics: make(map[string][]Diagnostic),
	}
}

// Start 启动 LSP 语言服务器并完成初始化握手。
//   - 如果客户端已就绪，直接返回 nil。
//   - 如果客户端处于错误状态，返回上一次启动的错误。
//   - 使用 startMu 保证并发安全。
//   - 依次执行：创建进程、建立 Conn、发送 initialize 请求、发送 initialized 通知。
func (c *Client) Start(ctx context.Context, workspacePath string) error {
	c.startMu.Lock()
	defer c.startMu.Unlock()

	state := ClientState(c.state.Load())
	switch state {
	case StateReady:
		return nil
	case StateError:
		if c.startErr != nil {
			return c.startErr
		}
		return fmt.Errorf("client in error state")
	case StateUnstarted:
		// 继续执行
	default:
		return nil
	}

	c.state.Store(int32(StateStarting))

	// 创建语言服务器进程
	cmd := exec.Command(c.command, c.args...)
	if len(c.env) > 0 {
		cmd.Env = append(os.Environ(), c.env...)
	}
	if c.cwd != "" {
		cmd.Dir = c.cwd
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		c.state.Store(int32(StateError))
		c.startErr = fmt.Errorf("stdin pipe: %w", err)
		return c.startErr
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		c.state.Store(int32(StateError))
		c.startErr = fmt.Errorf("stdout pipe: %w", err)
		return c.startErr
	}

	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		c.state.Store(int32(StateError))
		c.startErr = fmt.Errorf("start process: %w", err)
		return c.startErr
	}

	c.cmd = cmd
	conn := NewConn(stdin, stdout)

	if err := c.startWithConn(ctx, conn, workspacePath); err != nil {
		c.cmd = nil
		c.state.Store(int32(StateError))
		c.startErr = err
		return err
	}

	c.state.Store(int32(StateReady))
	return nil
}

// startWithConn 使用已建立的连接执行 LSP 初始化握手。
// 不负责状态转换——由调用方管理。
func (c *Client) startWithConn(ctx context.Context, conn *Conn, workspacePath string) error {
	c.conn = conn

	// 注册诊断通知处理器
	c.conn.OnNotification("textDocument/publishDiagnostics", c.handlePublishDiagnostics)

	// 启动连接的读取循环
	c.conn.Start()

	// 发送 initialize 请求
	trueVal := true
	initParams := initializeParams{
		ProcessID: nil,
		RootURI:   c.fileToURI(workspacePath),
		Capabilities: capabilities{
			TextDocument: textDocumentClientCapabilities{
				Definition:         &definitionCap{LinkSupport: &trueVal},
				References:         map[string]bool{},
				Hover:              &hoverCap{ContentFormat: []string{"plaintext"}},
				DocumentSymbol:     &documentSymbolCap{HierarchicalDocumentSymbolSupport: &trueVal},
				PublishDiagnostics: map[string]bool{},
			},
			Workspace: workspaceClientCapabilities{
				WorkspaceFolders: &trueVal,
			},
		},
	}

	if _, err := c.conn.Send(ctx, "initialize", initParams); err != nil {
		c.conn.Close()
		return fmt.Errorf("initialize request failed: %w", err)
	}

	// 发送 initialized 通知
	if err := c.conn.Notify("initialized", struct{}{}); err != nil {
		c.conn.Close()
		return fmt.Errorf("initialized notification failed: %w", err)
	}

	return nil
}

// Stop 停止 LSP 客户端。
//   - 如果客户端未就绪，直接返回 nil。
//   - 发送 shutdown 请求（5 秒超时）。
//   - 发送 exit 通知。
//   - 等待进程退出（最多 5 秒），超时则杀死进程。
//   - 关闭连接，将状态重置为 Unstarted。
func (c *Client) Stop() error {
	if ClientState(c.state.Load()) != StateReady {
		return nil
	}

	// 发送 shutdown 请求
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if c.conn != nil {
		_, _ = c.conn.Send(shutdownCtx, "shutdown", nil)

		// 发送 exit 通知
		_ = c.conn.Notify("exit", nil)
	}

	// 等待进程退出
	if c.cmd != nil && c.cmd.Process != nil {
		done := make(chan error, 1)
		go func() {
			done <- c.cmd.Wait()
		}()

		select {
		case <-done:
			// 进程正常退出
		case <-time.After(5 * time.Second):
			// 超时，杀死进程
			_ = c.cmd.Process.Kill()
			<-done
		}
	}

	// 关闭连接
	if c.conn != nil {
		_ = c.conn.Close()
	}

	c.state.Store(int32(StateUnstarted))
	return nil
}

// Ready 返回客户端是否已就绪。
func (c *Client) Ready() bool {
	return ClientState(c.state.Load()) == StateReady
}

// OpenFile 发送 textDocument/didOpen 通知打开文件。
// 如果文件已经打开，则不重复发送。
func (c *Client) OpenFile(file string) error {
	uri := c.fileToURI(file)

	c.filesMu.Lock()
	if c.openFiles[uri] {
		c.filesMu.Unlock()
		return nil
	}
	c.openFiles[uri] = true
	c.filesMu.Unlock()

	// 读取文件内容
	content, err := os.ReadFile(file)
	if err != nil {
		c.filesMu.Lock()
		delete(c.openFiles, uri)
		c.filesMu.Unlock()
		return fmt.Errorf("read file: %w", err)
	}

	params := didOpenParams{}
	params.TextDocument.URI = uri
	params.TextDocument.LanguageID = c.languageID(file)
	params.TextDocument.Version = 1
	params.TextDocument.Text = string(content)

	return c.conn.Notify("textDocument/didOpen", params)
}

// ChangeFile 发送 textDocument/didChange 通知更新文件内容。
// 文件必须已通过 OpenFile 打开。
func (c *Client) ChangeFile(file, content string) error {
	uri := c.fileToURI(file)

	c.filesMu.Lock()
	if !c.openFiles[uri] {
		c.filesMu.Unlock()
		return fmt.Errorf("file not opened: %s", file)
	}
	c.filesMu.Unlock()

	params := didChangeParams{}
	params.TextDocument.URI = uri
	params.TextDocument.Version = 1
	params.ContentChanges = []struct {
		Text string `json:"text"`
	}{
		{Text: content},
	}

	return c.conn.Notify("textDocument/didChange", params)
}

// CloseFile 发送 textDocument/didClose 通知关闭文件。
func (c *Client) CloseFile(file string) error {
	uri := c.fileToURI(file)

	c.filesMu.Lock()
	if !c.openFiles[uri] {
		c.filesMu.Unlock()
		return nil
	}
	delete(c.openFiles, uri)
	c.filesMu.Unlock()

	params := didCloseParams{}
	params.TextDocument.URI = uri

	return c.conn.Notify("textDocument/didClose", params)
}

// ensureOpen 确保文件已打开，如果未打开则自动调用 OpenFile。
func (c *Client) ensureOpen(file string) {
	uri := c.fileToURI(file)

	c.filesMu.Lock()
	alreadyOpen := c.openFiles[uri]
	c.filesMu.Unlock()

	if !alreadyOpen {
		_ = c.OpenFile(file)
	}
}

// handlePublishDiagnostics 处理 textDocument/publishDiagnostics 通知。
func (c *Client) handlePublishDiagnostics(params json.RawMessage) {
	var p publishDiagnosticsParams
	if err := json.Unmarshal(params, &p); err != nil {
		return
	}

	c.diagMu.Lock()
	defer c.diagMu.Unlock()

	c.diagnostics[p.URI] = p.Diagnostics
	c.diagVersion++
}

// withTimeout 为 LSP 调用添加 10s 超时
func (c *Client) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, 10*time.Second)
}

// Diagnostics 返回指定文件的缓存诊断信息和版本号。
func (c *Client) Diagnostics(file string) ([]Diagnostic, uint64) {
	uri := c.fileToURI(file)

	c.diagMu.RLock()
	defer c.diagMu.RUnlock()

	diags := c.diagnostics[uri]
	version := c.diagVersion

	result := make([]Diagnostic, len(diags))
	copy(result, diags)
	return result, version
}

// Definition 查询光标位置处的符号定义位置。
func (c *Client) Definition(ctx context.Context, file string, pos Position) ([]Location, error) {
	c.ensureOpen(file)

	lspCtx, cancel := c.withTimeout(ctx)
	defer cancel()

	raw, err := c.conn.Send(lspCtx, "textDocument/definition", textDocumentPositionParams{
		TextDocument: textDocumentID{URI: c.fileToURI(file)},
		Position:     pos,
	})
	if err != nil {
		return nil, err
	}

	return parseLocations(raw)
}

// References 查询光标位置处符号的所有引用位置。
func (c *Client) References(ctx context.Context, file string, pos Position) ([]Location, error) {
	c.ensureOpen(file)

	lspCtx, cancel := c.withTimeout(ctx)
	defer cancel()

	raw, err := c.conn.Send(lspCtx, "textDocument/references", referencesParams{
		TextDocument: textDocumentID{URI: c.fileToURI(file)},
		Position:     pos,
		Context:      referenceContext{IncludeDeclaration: true},
	})
	if err != nil {
		return nil, err
	}

	return parseLocations(raw)
}

// Hover 查询光标位置处的悬停信息。
func (c *Client) Hover(ctx context.Context, file string, pos Position) (string, error) {
	c.ensureOpen(file)

	lspCtx, cancel := c.withTimeout(ctx)
	defer cancel()

	raw, err := c.conn.Send(lspCtx, "textDocument/hover", textDocumentPositionParams{
		TextDocument: textDocumentID{URI: c.fileToURI(file)},
		Position:     pos,
	})
	if err != nil {
		return "", err
	}
	if raw == nil {
		return "", nil
	}

	// 解析 hover 结果
	// LSP hover 结果格式：{"contents": MarkupContent | string | array}
	var result struct {
		Contents json.RawMessage `json:"contents"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", err
	}

	return extractHoverText(result.Contents), nil
}

// DocumentSymbols 查询文件中所有符号。
func (c *Client) DocumentSymbols(ctx context.Context, file string) ([]SymbolInfo, error) {
	c.ensureOpen(file)

	lspCtx, cancel := c.withTimeout(ctx)
	defer cancel()

	raw, err := c.conn.Send(lspCtx, "textDocument/documentSymbol", textDocumentParams{
		TextDocument: textDocumentID{URI: c.fileToURI(file)},
	})
	if err != nil {
		return nil, err
	}

	return parseSymbols(raw, c.fileToURI(file))
}

// WorkspaceSymbols 在工作区中搜索符号。
func (c *Client) WorkspaceSymbols(ctx context.Context, query string) ([]SymbolInfo, error) {
	lspCtx, cancel := c.withTimeout(ctx)
	defer cancel()

	raw, err := c.conn.Send(lspCtx, "workspace/symbol", map[string]string{"query": query})
	if err != nil {
		return nil, err
	}

	return parseSymbols(raw, "")
}

// parseLocations 解析 JSON-RPC 结果为 Location 列表。
// LSP 中一些端点的结果可能是单个 Location、[]Location 或 null。
func parseLocations(raw json.RawMessage) ([]Location, error) {
	if raw == nil {
		return nil, nil
	}

	// 尝试解析为数组
	var locations []Location
	if err := json.Unmarshal(raw, &locations); err == nil {
		return locations, nil
	}

	// 尝试解析为单个 Location
	var loc Location
	if err := json.Unmarshal(raw, &loc); err != nil {
		return nil, err
	}
	return []Location{loc}, nil
}

// extractHoverText 从 hover 结果中提取文本内容。
func extractHoverText(raw json.RawMessage) string {
	if raw == nil {
		return ""
	}

	// 尝试 MarkupContent 格式：{"kind": "plaintext", "value": "..."} 或 {"kind": "markdown", "value": "..."}
	var mc struct {
		Kind  string `json:"kind"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(raw, &mc); err == nil && mc.Value != "" {
		return mc.Value
	}

	// 尝试纯字符串格式
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}

	return ""
}

// parseSymbols 解析 JSON-RPC 结果为 SymbolInfo 列表。
// 支持 SymbolInformation（平坦）和 DocumentSymbol（层次化）两种格式。
func parseSymbols(raw json.RawMessage, uri string) ([]SymbolInfo, error) {
	if raw == nil {
		return nil, nil
	}

	// 先解析为 raw 数组，检查第一个元素是否有 "location" 字段
	var rawItems []json.RawMessage
	if err := json.Unmarshal(raw, &rawItems); err != nil {
		return nil, err
	}

	if len(rawItems) == 0 {
		return nil, nil
	}

	// 检查第一个元素是否有 location 字段 → SymbolInformation 格式
	var firstCheck struct {
		Location *Location `json:"location"`
	}
	if err := json.Unmarshal(rawItems[0], &firstCheck); err == nil && firstCheck.Location != nil &&
		firstCheck.Location.URI != "" {
		// SymbolInformation 格式
		var symbols []SymbolInfo
		if err := json.Unmarshal(raw, &symbols); err != nil {
			return nil, err
		}
		return symbols, nil
	}

	// DocumentSymbol 格式（层次化）
	var docSymbols []DocumentSymbol
	if err := json.Unmarshal(raw, &docSymbols); err != nil {
		return nil, err
	}

	return flattenDocumentSymbols(docSymbols, uri), nil
}

// flattenDocumentSymbols 将层次化的 DocumentSymbol 展平为 SymbolInfo 列表。
func flattenDocumentSymbols(symbols []DocumentSymbol, uri string) []SymbolInfo {
	var result []SymbolInfo
	for _, s := range symbols {
		info := SymbolInfo{
			Name: s.Name,
			Kind: s.Kind,
			Location: Location{
				URI:   uri,
				Range: s.Range,
			},
		}
		result = append(result, info)
		result = append(result, flattenDocumentSymbols(s.Children, uri)...)
	}
	return result
}

// fileToURI 将文件路径转换为 file:// URI。
func (c *Client) fileToURI(path string) string {
	// 确保是绝对路径
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}
	// 统一使用正斜杠
	absPath = filepath.ToSlash(absPath)
	return "file://" + absPath
}

// uriToFile 将 file:// URI 转换回文件路径。
func (c *Client) uriToFile(uri string) string {
	if !strings.HasPrefix(uri, "file://") {
		return uri
	}
	path := strings.TrimPrefix(uri, "file://")
	// 使用系统路径分隔符
	return filepath.FromSlash(path)
}

// languageID 根据文件扩展名猜测 LSP 语言标识符。
func (c *Client) languageID(file string) string {
	ext := strings.ToLower(filepath.Ext(file))
	switch ext {
	case ".go":
		return "go"
	case ".ts":
		return "typescript"
	case ".tsx":
		return "typescriptreact"
	case ".js":
		return "javascript"
	case ".jsx":
		return "javascriptreact"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".rb":
		return "ruby"
	case ".c":
		return "c"
	case ".cpp":
		return "cpp"
	case ".h":
		return "c"
	case ".hpp":
		return "cpp"
	case ".cs":
		return "csharp"
	case ".swift":
		return "swift"
	case ".kt":
		return "kotlin"
	case ".scala":
		return "scala"
	case ".php":
		return "php"
	case ".html":
		return "html"
	case ".css":
		return "css"
	case ".scss":
		return "scss"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".md":
		return "markdown"
	case ".sql":
		return "sql"
	case ".sh":
		return "shellscript"
	case ".go.mod":
		return "go.mod"
	default:
		return ""
	}
}