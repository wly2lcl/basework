## 8. 补充设计：LSP 集成

> 借鉴 crush 的 LSP 原生集成，让 agent 像 IDE 一样理解代码。

### 8.1 架构

```
pkg/lsp/
├── manager.go        # Manager: 管理多个 LSP 客户端的生命周期
├── client.go         # Client: 包装单个 LSP 服务器连接
├── types.go          # 统一类型定义（Location, Diagnostic, Symbol...）
├── auto.go           # 自动检测（根据文件类型 + 根标记选择 LSP 命令）
└── tool.go           # 将 LSP 功能暴露为 agent Tool
```

### 8.2 核心接口

```go
// pkg/lsp/types.go
package lsp

// Location 表示代码位置
type Location struct {
    URI   string // 文件路径
    Range Range
}

type Range struct {
    Start Position
    End   Position
}

type Position struct {
    Line      int
    Character int
}

// Diagnostic 表示诊断信息
type Diagnostic struct {
    Range    Range
    Severity Severity // Error, Warning, Info, Hint
    Message  string
    Source   string   // 来源（如 "gopls", "tsserver"）
}

// SymbolInformation 表示符号信息
type SymbolInfo struct {
    Name     string
    Kind     SymbolKind // Function, Class, Variable, etc.
    Location Location
}
```

```go
// pkg/lsp/manager.go
package lsp

// Manager 管理多个 LSP 客户端
type Manager struct {
    clients map[string]*Client  // name → client
    mu      sync.RWMutex
    config  Config
}

// Config 是 LSP 配置
type Config struct {
    Servers map[string]ServerConfig // "go" → {Command: "gopls"}
}

type ServerConfig struct {
    Command string
    Args    []string
    Env     map[string]string
}

// Start 懒启动 LSP（按文件类型匹配）
func (m *Manager) Start(ctx context.Context, workspacePath string) error

// Stop 停止所有 LSP 客户端
func (m *Manager) Stop() error

// Definition 跳转到定义
func (m *Manager) Definition(ctx context.Context, file string, pos Position) ([]Location, error)

// References 查找引用
func (m *Manager) References(ctx context.Context, file string, pos Position) ([]Location, error)

// Hover 获取悬停信息
func (m *Manager) Hover(ctx context.Context, file string, pos Position) (string, error)

// Diagnostics 获取诊断
func (m *Manager) Diagnostics(file string) []Diagnostic

// DocumentSymbols 获取文档符号
func (m *Manager) DocumentSymbols(ctx context.Context, file string) ([]SymbolInfo, error)

// WorkspaceSymbols 搜索工作区符号
func (m *Manager) WorkspaceSymbols(ctx context.Context, query string) ([]SymbolInfo, error)
```

### 8.3 自动检测

```go
// pkg/lsp/auto.go
package lsp

// DefaultServers 默认 LSP 服务器映射
var DefaultServers = map[string]ServerConfig{
    "go":         {Command: "gopls"},
    "typescript": {Command: "typescript-language-server", Args: []string{"--stdio"}},
    "javascript": {Command: "typescript-language-server", Args: []string{"--stdio"}},
    "python":     {Command: "pyright-langserver", Args: []string{"--stdio"}},
}

// Detect 根据工作区文件自动检测需要的 LSP
func Detect(workspacePath string) map[string]ServerConfig
```

### 8.4 LSP 作为 Tool

LSP 功能暴露为 agent 工具，让 LLM 可以主动查询代码信息：

```go
// pkg/lsp/tool.go
package lsp

// Tools 返回所有 LSP 工具
func Tools(m *Manager) []tool.Tool {
    return []tool.Tool{
        &definitionTool{manager: m},
        &referencesTool{manager: m},
        &hoverTool{manager: m},
        &diagnosticsTool{manager: m},
        &documentSymbolsTool{manager: m},
        &workspaceSymbolsTool{manager: m},
    }
}
```

### 8.5 设计决策

- **懒启动**：LSP 在首次需要时才启动，避免冷启动开销
- **诊断缓存**：使用版本号避免 UI 重复计算（参考 crush 的 `VersionedMap`）
- **状态机**：每个 Client 有 `Unstarted → Starting → Ready/Error` 状态
- **可选模块**：LSP 需要安装对应的 language server，未安装时优雅降级（不注册工具）
- **不做**：不实现完整的 LSP 协议，只暴露 agent 需要的 6 种操作

---

## 9. 补充设计：MCP 协议支持

> Model Context Protocol 是 AI agent 的外部工具扩展标准，opencode 和 crush 都支持。

### 9.1 架构

```
pkg/mcp/
├── manager.go        # Manager: 管理多个 MCP 服务器连接
├── transport.go      # 传输层（stdio + http）
├── adapter.go        # 将 MCP 工具适配为 tool.Tool
└── config.go         # MCP 配置类型
```

### 9.2 核心接口

```go
// pkg/mcp/config.go
package mcp

// Config 是 MCP 配置
type Config struct {
    Servers map[string]ServerConfig
}

type ServerConfig struct {
    // stdio 传输
    Command string            `json:"command,omitempty"`
    Args    []string          `json:"args,omitempty"`
    Env     map[string]string `json:"env,omitempty"`
    
    // http 传输
    URL     string            `json:"url,omitempty"`
    Headers map[string]string `json:"headers,omitempty"`
    
    // 通用
    Enabled bool `json:"enabled"`
}
```

```go
// pkg/mcp/manager.go
package mcp

// Manager 管理多个 MCP 服务器连接
type Manager struct {
    servers map[string]*ServerConnection
    mu      sync.RWMutex
}

// ServerConnection 表示到 MCP 服务器的连接
type ServerConnection struct {
    Name   string
    Tools  []mcp.Tool    // 服务器提供的工具
    client *mcp.ClientSession
}

// Connect 连接到 MCP 服务器
func (m *Manager) Connect(ctx context.Context, name string, cfg ServerConfig) error

// Disconnect 断开连接
func (m *Manager) Disconnect(name string) error

// Tools 返回所有 MCP 工具（适配为 tool.Tool）
func (m *Manager) Tools() []tool.Tool

// Close 关闭所有连接
func (m *Manager) Close() error
```

### 9.3 传输层

```go
// pkg/mcp/transport.go
package mcp

// Transport 是 MCP 传输接口
type Transport interface {
    Connect(ctx context.Context) error
    Send(msg []byte) error
    Receive() (<-chan []byte, error)
    Close() error
}

// StdioTransport 通过 stdin/stdout 与子进程通信
type StdioTransport struct {
    command string
    args    []string
    env     map[string]string
    cmd     *exec.Cmd
}

// HTTPTransport 通过 HTTP + SSE 与远程服务器通信
type HTTPTransport struct {
    url     string
    headers map[string]string
}
```

### 9.4 工具适配

MCP 工具自动转换为 `tool.Tool`：

```go
// pkg/mcp/adapter.go
package mcp

// mcpToolAdapter 将 MCP 工具适配为 tool.Tool
type mcpToolAdapter struct {
    server   string       // 服务器名
    tool     mcp.Tool     // MCP 工具定义
    session  *mcp.ClientSession
}

func (a *mcpToolAdapter) Name() string        { return "mcp_" + a.server + "_" + a.tool.Name }
func (a *mcpToolAdapter) Description() string  { return a.tool.Description }
func (a *mcpToolAdapter) Parameters() json.RawMessage { return a.tool.InputSchema }
func (a *mcpToolAdapter) Execute(ctx context.Context, args json.RawMessage) (*tool.Result, error) {
    // 调用 MCP 服务器的 tools/call
    result, err := a.session.CallTool(ctx, a.tool.Name, args)
    // ...
}
```

### 9.5 设计决策

- **两种传输**：只支持 `stdio`（本地子进程）和 `http`（远程服务器）。SSE 传输暂不实现，需要时再添加。
- **工具名前缀**：MCP 工具名加 `mcp_{server}_` 前缀，避免与内置工具冲突
- **容错**：单个 MCP 服务器连接失败不影响其他服务器和 agent 运行
- **懒连接**：MCP 服务器在首次需要时才连接
- **使用官方 SDK**：使用 `github.com/modelcontextprotocol/go-sdk/mcp`

---

