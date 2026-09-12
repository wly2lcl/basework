## 26. 终端产品模块设计（`internal/`）

> `internal/` 下的模块服务于终端产品（`cmd/basework/`），不暴露给嵌入方。
> 它们依赖 `pkg/` 层的接口，但 `pkg/` 层对它们无感知。

### 26.1 层间关系

```
cmd/basework/ ← 编排所有 internal 模块
    │
    ├── internal/compaction/      → pkg/agent (TurnD.ShouldCompact)
    ├── internal/retry/           → pkg/llm (错误分类)
    ├── internal/permission/      → pkg/hook (PermissionHook)
    ├── internal/subagent/        → pkg/agent (Agent 实例管理)
    ├── internal/loopdetect/      → pkg/hook (AfterTool hook)
    ├── internal/observability/   → pkg/hook (AfterLLM/AfterTool hooks)
    └── internal/oauth/           → pkg/provider (token 注入)
```

### 26.2 上下文压缩 (`internal/compaction/`)

```
compaction/
├── engine.go           # 压缩引擎：触发判断 + 策略路由
├── strategy.go         # 压缩策略接口
├── summarization.go    # LLM 摘要策略
├── selective.go        # 选择性保留策略（保留关键工具结果）
├── sliding_window.go   # 滑动窗口策略（仅保留最近 N 轮）
└── token.go            # Token 估算
```

**架构决策**：
- **策略模式**：支持 `summarize`（LLM 摘要）、`truncate`（直接截断）、`sliding_window`（滑动窗口）
- **触发机制**：在 `SetupTurn` 阶段通过 `EstimateTokens` 估算，超过阈值时触发
- **事件溯源集成**：压缩产生 `session.EventCompacted` 事件，投影时截断旧消息
- **与 pkg/agent 的关系**：`pkg/agent.CompactConfig` 定义配置，`internal/compaction` 实现策略

### 26.3 重试机制 (`internal/retry/`)

```
retry/
├── retry.go            # 重试循环 + 指数退避
├── backoff.go          # 退避计算
├── classify.go         # 错误分类（可重试/不可重试）
```

**架构决策**：
- **错误分类驱动**：`classify.go` 将 `llm.Error` 分类为可重试（rate_limit, overloaded, network）和不可重试
- **指数退避**：默认初始 1s，最大 30s，退避因子 2.0，加 jitter
- **Retry-After 支持**：解析 HTTP 响应头的 `Retry-After`，优先使用服务器指定的等待时间
- **与 pkg/agent 的关系**：`pkg/agent.RetryConfig` 定义配置，`internal/retry` 供 `AgentLoop` 调用

### 26.4 权限系统 (`internal/permission/`)

```
permission/
├── checker.go          # 权限检查器
├── rule.go             # 规则定义与匹配
├── mode.go             # 模式管理（interactive/yolo/deny-all）
└── cache.go            # 用户选择缓存
```

**架构决策**：
- **Hook 模式**（参考 §3.1.4）：权限通过 `BeforeTool` hook 实现，不独立建系统
- **三值模式**：`interactive`（交互确认）、`yolo`（全部允许）、`deny-all`（全部拒绝）
- **规则引擎**：通配符匹配（`tool:write:/etc/**`），首次匹配优先
- **用户选择缓存**：同一工具同一路径的选择可缓存，避免重复询问
- **与 pkg/hook 的关系**：`pkg/hook.PermissionHook` 定义规则结构，`internal/permission` 提供 CLI 交互层

### 26.5 子代理系统 (`internal/subagent/`)

```
subagent/
├── coordinator.go      # 子代理协调器
├── progress.go         # 进度报告
├── cost.go             # 成本累积与传播
├── tool.go             # task 工具实现
└── types.go            # 类型定义
```

**架构决策**：
- **新 Agent 实例**：每个子代理创建独立的 `pkg/agent.Agent` 实例，共享 Model 和 ToolRegistry 的基础配置
- **隔离子会话**：子代理使用独立的 `session.Store`（通过 `Session.Fork`），不污染父会话
- **成本传播**：子代理的 token 消耗异步累加到父会话的 Usage 中
- **进度流式回传**：子代理的输出通过 `Callback` 流式回传给父 agent
- **只读代理**：通过 `pkg/agent.WithAgent("plan")` 创建只读子代理（无写工具权限）

### 26.6 循环检测 (`internal/loopdetect/`)

```
loopdetect/
├── detector.go         # 循环检测器（主入口）
├── repeated.go         # 连续相同调用检测
├── pattern.go          # 重复模式识别
├── tool_loop.go        # 工具调用循环检测
├── fusion.go           # 相似调用融合
└── response.go         # 类型定义
```

**架构决策**：
- **双重检测**：SHA-256 签名追踪（检测完全相同的调用序列）+ 模式匹配（检测变体循环）
- **窗口机制**：默认追踪最近 10 步调用，超过 5 次相似调用触发中断
- **工具融合**：对连续相同参数的重复工具调用（如反复 `read` 同一文件），融合为一次
- **集成方式**：通过 `AfterTool` hook 注入检测逻辑，对主循环无侵入
- **自动中断**：检测到循环后，自动跳过同类调用并向 LLM 返回警告消息

### 26.7 可观测性 (`internal/observability/`)

```
observability/
├── config.go           # 配置定义
├── logger.go           # 结构化日志
├── bus.go              # 事件总线适配
├── event.go            # observability 事件类型
├── subscriber.go       # 事件订阅者
├── tokens.go           # Token 计数与费用估算
└── cost.go             # 成本追踪
```

**架构决策**：
- **标准库优先**：使用 `log/slog` 为默认日志后端，无额外依赖
- **事件驱动**：订阅 `pkg/hook.Broker` 事件，将日志写入文件
- **日志级别**：debug/info/warn/error，通过配置控制输出级别
- **成本估算**：基于 Provider 返回的 Usage + 预设的每千 token 单价估算费用
- **追踪扩展可选**：默认使用 `log/slog` 和事件总线；如需 OpenTelemetry，可在上层通过 Hook/EventBus 适配

### 26.8 OAuth 2.0 (`internal/oauth/`)

```
oauth/
├── config.go           # 配置定义
├── flow.go             # PKCE 认证流程
├── pkce.go             # PKCE 码挑战生成
├── token.go            # 令牌管理与刷新
├── store.go            # 凭证安全存储
├── provider.go         # Provider 特定配置
└── callback.go         # 本地回调服务器
```

**架构决策**：
- **PKCE 流程**：Authorization Code + PKCE，无需 client_secret，适合 CLI 应用
- **本地回调服务器**：启动临时 HTTP 服务器接收 callback code（监听 localhost）
- **令牌自动刷新**：检测 `401/403` 响应时自动用 refresh_token 刷新 access_token
- **安全存储**：凭证加密存储在 `~/.config/basework/auth/`，文件权限 0600
- **Provider 配置**：内置 GitHub、Google 等常见 Provider 的端点配置

### 26.9 增强工具系统 (`internal/tools/`)

```
tools/
├── webfetch.go         # web_fetch 工具：获取网页内容，支持 text/markdown/html 格式
├── websearch.go        # web_search 工具：网络搜索（Tavily/Exa 后端）
├── todowrite.go        # todowrite 工具：任务列表管理
├── applypatch.go       # apply_patch 工具：结构化补丁应用
├── question.go         # question 工具：向用户提问交互
├── *_test.go           # 对应测试文件
```

**架构决策**：
- **统一接口**：所有工具实现 `pkg/tool.Tool` 接口，与内置工具（bash/read/write）使用相同注册机制
- **不依赖框架核心**：`internal/tools/` 只依赖 `pkg/tool` 接口，不依赖 `pkg/agent`
- **通过 `cmd/basework` 编排**：CLI 入口负责创建 ToolRegistry 并注册这些工具
- **无额外依赖**：web_fetch 使用标准库 `net/http` + `golang.org/x/net/html` 解析 HTML

#### web_fetch 工具

```go
// internal/tools/webfetch.go
// 获取指定 URL 的内容并提取文本信息
// 参数: url (required), format (text|markdown|html, default: text)
// 限制: 100KB 响应大小, 30s 超时
// HTML 解析: 标准库 html.Parse，块级元素自动加换行
```

#### web_search 工具

```go
// internal/tools/websearch.go
// 搜索网络信息，支持 Tavily 和 Exa 搜索后端
// 参数: query (required), num_results (1-20, default: 5)
// 配置: 通过 config.tools.web_search.backend/api_key 控制
// 结果统一为 SearchResult{Title, URL, Summary}
```

#### todowrite 工具

```go
// internal/tools/todowrite.go
// 任务列表管理，纯内存存储（会话级）
// 参数: todos (required) [{text, status}]
// 状态: "pending" | "completed"
// 渲染: 带完成统计的格式化列表
```

#### apply_patch 工具

```go
// internal/tools/applypatch.go
// 应用结构化补丁到文件系统
// 格式: *** Begin Patch / *** End Patch 包裹
// 操作: Add File（创建新文件）、Delete File（删除文件）、Update File（更新文件内容）
// 限制: 工作目录约束，防止越界操作
```

#### question 工具

```go
// internal/tools/question.go
// 向用户提出问题并提供选项供选择
// 参数: questions (required) [{text, options}]
// 交互: stdin/stdout 读取用户选择
// YOLO 模式: 自动选择第一个选项
// Mock 支持: 通过可替换的 stdin/stdout 方便测试
```

### 26.10 终端 UI (`internal/tui/`)

```
tui/
├── app.go            # TUI 主应用（Bubble Tea Model 接口）
├── callback.go       # agent.Callback → tea.Msg 适配器（流式/工具事件入口）
├── input.go          # 输入区组件（多行、历史、Tab 补全）
├── message.go        # 消息渲染组件（Glamour Markdown 渲染）
├── streaming.go      # 流式输出组件（逐字输出、spinner 动画）
├── statusbar.go      # 状态栏组件（模型名、token、MCP 状态）
├── theme/            # 主题系统（Phase 29）
├── keymap/           # 键盘绑定（Phase 29）
├── command/          # 斜杠命令面板（Phase 29）
├── dialog/           # 对话框系统（Phase 29）
└── plugin/           # TUI 插件插槽（Phase 30）
```

**Bubble Tea 架构**：

```
App (Bubble Tea Model)
  ├── InputView     — 输入区（tea.Model 子组件）
  ├── StatusBarView — 状态栏（纯渲染组件）
  ├── StreamingView — 流式输出（纯渲染组件）
  └── MessageView   — 消息渲染器（非 tea 组件，纯函数）
```

**App 主结构体**：

```go
// App 是 TUI 应用的主结构体，实现了 Bubble Tea Model 接口
type App struct {
    Messages  []Message       // 消息历史
    Input     *InputView      // 输入子组件
    StatusBar *StatusBarView  // 状态栏子组件
    Streaming *StreamingView  // 流式子组件
    Width     int             // 终端宽度
    Height    int             // 终端高度
    IsStreaming bool          // 是否正在流式输出
}
```

**消息类型定义**：

```go
// 事件循环内消息（由 App.Update 内部产生 / 消费）
type UserInputMsg struct { Text string }
type AgentResponseMsg struct { Text string }
type ToolCallMsg struct {
    ToolName string
    Args     string
    Result   string
    IsError  bool
}
type ErrorMsg struct{ Err error }

// 流式消息（由 internal/tui/callback.go 投递，见 26.10.1）
type StreamDeltaMsg   struct{ Delta string }
type ThinkingDeltaMsg struct{ Delta string }
type ToolStartMsg     struct{ Name string }
type ToolEndMsg       struct{ Name, Args, Result string; IsError bool }
```

**组件通信模式**：

- 按键事件 → `App.Update()` → 分发到子组件
- 用户按 Enter → `App.Update()` 返回 `UserInputMsg`
- **Agent 事件 → `agent.Callback` → `tea.Msg` → `tea.Program.Send` → `App.Update()`**
  状态变更（消息列表、流式缓冲、工具进度）一律在事件循环内完成
- 渲染只读状态：`App.View()` 与 `App.Update()` 由 Bubble Tea 并发调度，
  因此**任何**非事件循环的 goroutine 都不得直接改 `App` 或其子组件

#### 26.10.1 回调桥接（`internal/tui/callback.go`）

`agent.Callback` 在 `tea.Cmd` 所在的 goroutine 中被调用（`UserInputMsg` 分支返回的
闭包 → `InputHandler` → `HandleMessage` → pipeline → `Callback`），而 Bubble Tea 事件
循环会并发调用 `App.Update()` 与 `App.View()`。**回调若直接调 `app.AddToolCall(...)`、
`app.UpdateStreamingText(...)` 就是明确的数据竞态。**

正确做法是回调只投递消息、不改状态：

```go
type agentCallback struct{ send func(tea.Msg) }

func NewAgentCallback(send func(tea.Msg)) agent.Callback

// OnTextDelta     → StreamDeltaMsg
// OnThinkingDelta → ThinkingDeltaMsg
// OnToolCallStart → ToolStartMsg
// OnToolCallEnd   → ToolEndMsg
// OnError         → ErrorMsg
// OnTurnEnd       → 空实现（本轮最终文本由 AgentResponseMsg 分支统一提交，
//                   在此重复提交会与流式累积文本重复入列表）
```

`cmd/basework/tui.go` 负责接线。这里存在构造顺序依赖：runtime 需要回调，回调需要
`tea.Program.Send`，而 program 需要 App。因此采用 **post-binding**：

```go
app := tui.NewApp(model, provider, "")
opts, bindSend := newTUIStreamingOptions()  // send 暂为 nil，emit 时判空
rt, _ := newRuntimeAgent(cfg, opts)
program := tea.NewProgram(app)
bindSend(program.Send)                      // 此时才真正接上
```

> 历史缺陷：该桥接曾长期未接线（`runtimeAgentOptions.Callback` 恒为 nil），
> 导致 TUI 只能拿到整段最终文本，流式增量 / thinking / 工具进度全部失效。
> 回归测试见 `cmd/basework/tui_test.go` 与 `internal/tui/callback_test.go`。

**输入区组件（InputView）**：

```go
type InputView struct {
    lines      []string    // 多行输入缓冲区
    history    []string    // 输入历史（上下键导航）
    completions []string   // Tab 补全候选
    prompt     string      // 提示符
}
```

**状态栏组件（StatusBarView）**：

```go
type StatusBarView struct {
    ModelName        string  // 当前模型名
    Provider         string  // Provider 名称
    SessionID        string  // 会话 ID
    PromptTokens     int     // 输入 token 数
    CompletionTokens int     // 输出 token 数
    TotalTokens      int     // 总 token 数
    MCPConnected     bool    // MCP 连接状态
    IsBusy           bool    // 忙/闲状态
}
```

**架构决策**：
- **Bubble Tea v2**：使用最新版 Bubble Tea 框架，基于 tea.Model 接口
- **子组件模式**：输入/状态栏/流式三个子组件各司其职
- **消息驱动桥接**：`cmd/basework/tui.go` 只做「构造 App → 构造回调 → 建 Program →
  绑定 `Send`」的接线；agent 事件一律以 `tea.Msg` 进入事件循环，**不在回调里改 Model**
  （详见 26.10.1）
- **Glamour Markdown**：使用 `charm.land/glamour/v2` 渲染 Markdown 响应（终端样式一致）
- **Lip Gloss 样式**：使用 `charm.land/lipgloss/v2` 定义颜色和样式，支持主题
- **无需 CGO**：所有依赖纯 Go

### 26.11 会话增强 (`pkg/session/`)

**SQLite Store（build tag: sqlite）**

```
pkg/session/
├── sqlite.go       # SQLiteStore 实现（build tag: sqlite）
├── filetrack.go    # FileTracker 文件追踪
└── queue.go        # 会话消息队列
```

**SQLite 表结构**：

```sql
-- 会话表
CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL DEFAULT '',
    model TEXT NOT NULL DEFAULT '',
    provider TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    tracked_files TEXT NOT NULL DEFAULT '[]'  -- JSON 文件追踪记录
);

-- 事件表（事件溯源的核心表）
CREATE TABLE events (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    type TEXT NOT NULL,
    data TEXT NOT NULL DEFAULT '',
    seq INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

-- 消息表（平铺，便于 SQL 查询）
CREATE TABLE messages (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    role TEXT NOT NULL,
    content TEXT NOT NULL DEFAULT '',
    timestamp TEXT NOT NULL,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

-- 工具调用表
CREATE TABLE tool_calls (
    id TEXT PRIMARY KEY,
    message_id TEXT NOT NULL,
    tool_name TEXT NOT NULL,
    arguments TEXT NOT NULL DEFAULT '{}',
    FOREIGN KEY (message_id) REFERENCES messages(id) ON DELETE CASCADE
);

-- 工具结果表
CREATE TABLE tool_results (
    id TEXT PRIMARY KEY,
    tool_call_id TEXT NOT NULL,
    content TEXT NOT NULL DEFAULT '',
    is_error INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (tool_call_id) REFERENCES tool_calls(id) ON DELETE CASCADE
);
```

**自动标题（AutoTitle）**：

```go
// pkg/config/config.go
type AutoTitleConfig struct {
    Enabled bool   `json:"enabled"`
    Model   string `json:"model"` // 用于生成标题的小模型（空则用主模型）
}
```

**会话队列（Queue）**：

```go
// pkg/session/queue.go
// Queue 是一个线程安全的 FIFO 消息队列
type Queue struct {
    mu       sync.Mutex
    messages []Message
    maxSize  int  // 0 表示不限制
}

func (q *Queue) Enqueue(msg Message) bool   // 入队（队列满时返回 false）
func (q *Queue) Dequeue() Message           // 出队（FIFO）
func (q *Queue) Cancel(msgID string) bool   // 按 ID 取消消息
func (q *Queue) Len() int                   // 当前队列大小
func (q *Queue) IsEmpty() bool              // 是否为空
```

**文件追踪（FileTracker）**：

```go
// pkg/session/filetrack.go
// FileTracker 追踪会话中访问或修改的文件
type FileTracker struct {
    readFiles  map[string]bool  // 读取的文件
    writeFiles map[string]bool  // 写入的文件
    editFiles  map[string]bool  // 编辑的文件
    allFiles   map[string]bool  // 所有文件（去重）
}

func (ft *FileTracker) TrackRead(path string)
func (ft *FileTracker) TrackWrite(path string)
func (ft *FileTracker) TrackEdit(path string)
func (ft *FileTracker) GetTrackedFiles() []string
func (ft *FileTracker) GetModifiedFiles() []string  // 写入+编辑的文件
```

**架构决策**：
- **Build Tag 控制**：SQLiteStore 通过 `sqlite` build tag 控制，不启用时使用 JSONLStore 作为默认
- **双写策略**：事件溯源事件同时写入原始 events 表和平铺到关系表（messages/tool_calls/tool_results），兼顾可重放性和 SQL 查询效率
- **WAL 模式**：使用 SQLite WAL 模式提升并发读写性能
- **文件追踪持久化**：tracked_files 字段存储 JSON 数组到 sessions 表，会话间隔离

### 26.12 Provider 扩展（`pkg/provider/`）

**新增 5 个 Provider**：

| Provider | 文件名 | 协议 | 认证方式 | 默认端点 |
|----------|--------|------|---------|---------|
| OpenCode Zen | `opencode.go` | OpenAI 兼容 | API Key | `https://opencode.ai/zen/v1` |
| Amazon Bedrock | `bedrock.go` | AWS Converse API | AWS SigV4 签名 | `https://bedrock-runtime.{region}.amazonaws.com` |
| Azure OpenAI | `azure.go` | OpenAI 兼容 | API Key (`api-key` 头) | `https://{resource}.openai.azure.com` |
| GitHub Copilot | `copilot.go` | OpenAI 兼容 | OAuth 设备授权 | `https://api.githubcopilot.com` |
| Ollama | `ollama.go` | OpenAI 兼容 | 无需认证 | `http://localhost:11434` |

**OpenCode Zen Provider**：

```go
// pkg/provider/opencode.go
// 基于 OpenAI 兼容协议，复用 compatModel 的 HTTP 客户端
// 免费模型: big-pickle, deepseek-v4-flash-free, mimo-v2.5-free
// 默认模型: big-pickle
type OpenCodeProvider struct {
    model   *compatModel
    apiKey  string
    modelID string
}
```

**Amazon Bedrock Provider**：

```go
// pkg/provider/bedrock.go
// 使用原生 HTTP + AWS SigV4 签名（不依赖 AWS SDK）
// 支持模型: Claude 3.5 Sonnet, Claude 3 Opus, Llama 3.1, Mistral Large
// 通过 AWS Converse API 非流式 + Converse Stream API 流式
type BedrockProvider struct {
    accessKey string
    secretKey string
    region    string
    modelID   string
    // 内置 SigV4 签名器
}
```

**Azure OpenAI Provider**：

```go
// pkg/provider/azure.go
// 端点格式: https://{resource}.openai.azure.com/openai/deployments/{deployment}
// 认证: api-key 请求头
// 复用 openai.go 的请求构建和流式解析函数
type AzureProvider struct {
    resource    string
    deployment  string
    apiKey      string
    apiVersion  string
    modelID     string
}
```

**GitHub Copilot Provider**：

```go
// pkg/provider/copilot.go
// 端点: https://api.githubcopilot.com/chat/completions
// 认证: OAuth 设备授权流程（用户访问 github.com 输入 code）
// token 持久化到 ~/.config/basework/copilot_token.json
// 支持模型: gpt-4, gpt-4o, claude-3.5-sonnet, gemini-2.0-flash
type CopilotProvider struct {
    model     string
    token     *oauth2.Token
    tokenPath string
    // 内置设备授权流程 (Authenticate 方法)
}
```

**Ollama Provider**：

```go
// pkg/provider/ollama.go
// 端点: http://localhost:11434/v1/chat/completions
// 无需认证
// 自动发现: 调用 /api/tags 获取本地模型列表
// 未指定模型时自动选择第一个可用模型
type OllamaProvider struct {
    endpoint string
    modelID  string
    models   []string      // 自动发现的本地模型
}
```

**Factory 集成**：

```go
// pkg/provider/factory.go — 新增 5 分支
var protocols = map[string]protocolMeta{
    // ... 原有 provider ...
    "opencode": {defaultBaseURL: "https://opencode.ai/zen/v1", allowEmptyKey: false},
    "bedrock":  {defaultBaseURL: "", allowEmptyKey: true},
    "azure":    {defaultBaseURL: "", allowEmptyKey: false},
    "copilot":  {defaultBaseURL: "", allowEmptyKey: true},
    "ollama":   {defaultBaseURL: "http://localhost:11434", allowEmptyKey: true},
}

// Create 方法新增 case 分支
switch cfg.Type {
case "bedrock":  return newBedrock(baseURL, cfg.APIKey, cfg.ModelID, cfg.Options)
case "azure":    return newAzure(baseURL, cfg.APIKey, cfg.ModelID, cfg.Options)
case "copilot":  return newCopilot(baseURL, cfg.APIKey, cfg.ModelID, cfg.Options)
case "opencode": return newOpenCode(baseURL, cfg.APIKey, cfg.ModelID, cfg.Options)
case "ollama":   return newOllama(baseURL, cfg.APIKey, cfg.ModelID, cfg.Options)
}
```

**架构决策**：
- **无 AWS SDK 依赖**：Bedrock 使用原生 HTTP 实现 SigV4 签名，避免 15MB SDK 依赖
- **代码复用**：OpenCode、Ollama 走 openai-compat 路径；Azure 复用 OpenAI 请求构建和流式解析
- **不同认证策略**：每个 Provider 的认证方式不同（API Key / SigV4 / OAuth / 无认证），各自独立实现
- **allowEmptyKey 标记**：Bedrock（用 access_key/secret_key）、Copilot（OAuth）、Ollama（无认证）允许空 API Key
- **本地模型自动发现**：Ollama 通过 `/api/tags` 端点自动获取可用模型列表

### 26.13 Prompt 缓存设计（`pkg/provider/cache.go`）

**文件结构**：
```
pkg/provider/
├── cache.go       # 缓存标记逻辑
└── cache_test.go  # 缓存标记单元测试
```

**设计目标**：
- Anthropic Provider 自动标记 system + user 消息前 1-2 块为 `cache_control: {"type": "ephemeral"}`
- OpenAI/Gemini Provider 添加 `cache_control` 字段支持
- 缓存标记在 Provider 层完成，对上层透明

**缓存标记策略**：

| Provider | 标记对象 | 规则 |
|----------|---------|------|
| Anthropic | system + 第一条 user 消息 | system 转为带 `cache_control` 的对象数组；user 前 2 个 text block 标记 |
| OpenAI | 第一条 user 消息 | 前 2 个 text parts 添加 `cache_control` |
| Gemini | 第一条 user 消息 | 前 2 个 user contents 添加缓存标记 |

**标记规则**：
- 仅标记 `text` 类型的内容块，`tool_result`、`image`、`image_url` 不标记
- 仅第一条 user 消息被标记，后续轮次的 user 消息跳过
- system 消息转为 `[{type: "text", text: "...", cache_control: {type: "ephemeral"}}]`
- 字符串类型的 content 自动转为 block 数组再标记

**核心函数**：

```go
// 检查缓存是否启用
func shouldCachePrompt(cfg *config.Config) bool

// system 标记
func markSystemContentForCache(systemText string) []map[string]any

// user content blocks 标记（前 maxBlocks 个 text block）
func markUserContentBlocksForCache(blocks []map[string]any, maxBlocks int)

// Anthropic 消息序列完整标记
func markAnthropicMessagesForCache(system string, messages []map[string]any, cacheEnabled bool) ([]map[string]any, string, []map[string]any)

// OpenAI 格式标记
func markOpenAIContentForCache(parts []llm.ContentPart) []map[string]any
```

**配置选项**（`config.PromptCacheConfig`）：
```go
type PromptCacheConfig struct {
    Enabled bool `json:"enabled"` // 是否启用（默认 true）
}
```

**Provider 差异**：
- **Anthropic**：system 消息转为带 cache_control 的对象数组；user content 支持字符串和 block 数组两种格式
- **OpenAI**：user content 始终为 block 数组，免疫 `image_url` 类型
- **Gemini**：使用 `maxUserContentsToCache` 常量控制标记数量

**架构决策**：
- **Provider 侧缓存**：不维护本地 LRU 缓存状态，Provider 侧自动管理（ephemeral 缓存 5 分钟 TTL）
- **保守策略**：仅在 system + 第一条 user 消息的前 1-2 块标记，避免标记过多导致性能下降
- **默认启用**：缓存默认开启，可通过 `config.yaml` 关闭

### 26.14 命令黑名单设计（`pkg/tool/builtin/blacklist.go`）

**文件结构**：
```
pkg/tool/builtin/
├── blacklist.go      # 黑名单检查逻辑
├── blacklist_test.go # 黑名单单元测试
└── bash.go           # BashTool（集成黑名单检查）
```

**设计目标**：
- 在 `BashTool.Execute` 中拦截危险 shell 命令
- 12+ 内置高危模式，支持用户自定义扩展
- 与权限系统集成（YOLO/Interactive/Default 模式）

**默认黑名单模式（12 个）**：

| # | 模式 | 正则 | 示例 |
|---|------|------|------|
| 1 | 删根 | `rm\s+-rf\s+/\*?$` | `rm -rf /`, `rm -rf /*` |
| 2 | 格式化 | `mkfs` | `mkfs.ext4 /dev/sda1` |
| 3 | 设备读取 | `dd\s+if=/dev/` | `dd if=/dev/zero` |
| 4 | 权限清空 | `chmod.*000\s+.*/` | `chmod 000 /etc/passwd` |
| 5 | Fork 炸弹 | `:\(\)\{.*:\|:.*\};:` | `:(){ :|:& };:` |
| 6 | 管道到 Shell | `\|\s*(sh|bash|zsh)\s*$` | `curl ... \| sh` |
| 7 | 远程下载执行 | `(wget|curl)\s+.*\|\s*(sh|bash)` | `wget ... \| bash` |
| 8 | 直接写入磁盘 | `>/dev/(sd[a-z]\|nvme\|hd[a-z])` | `echo >/dev/sda1` |
| 9 | 创建交换分区 | `mkswap\s+/dev/` | `mkswap /dev/sda1` |
| 10 | 系统关机 | `^(halt\|poweroff\|reboot\|shutdown)\s*$` | `halt`, `reboot` |
| 11 | 设备写入 | `dd\s+of=/dev/` | `dd of=/dev/sda` |
| 12 | 重定向到磁盘 | `>\s*/dev/sd[a-z]` | `cat data > /dev/sdb` |

**权限集成**：

```go
type BashTool struct {
    PermissionMode  string   // "default" | "interactive" | "yolo"
    BlockedCommands []string // 用户自定义正则模式
}
```

| 模式 | 黑名单行为 |
|------|-----------|
| `default` | 直接拒绝，返回错误信息 |
| `interactive` | 返回确认提示（由调用方处理确认逻辑） |
| `yolo` | 跳过黑名单检查 |

**核心函数**：

```go
// CheckBlacklist 检查命令是否匹配黑名单模式
func CheckBlacklist(cmd string, extraPatterns []string) (matched bool, matchedPattern string, err error)
```

**架构决策**：
- **正则匹配**：足够灵活（支持 `rm\s+-rf\s+/` 等模式），无需 AST 解析
- **单点检查**：在 `BashTool.Execute` 中集中检查，所有 bash 调用必经之路
- **用户可扩展**：通过 `config.yaml` 的 `blocked_commands` 添加自定义模式
- **内置 + 自定义分层**：内置模式始终生效，自定义模式用于补充

### 26.15 MCP 增强设计（`pkg/mcp/`）

**文件结构**：
```
pkg/mcp/
├── resource.go         # 资源列出/读取（resources/list, resources/read）
├── resource_test.go    # 资源单元测试
├── prompt.go           # 提示模板获取（prompts/list, prompts/get）
├── prompt_test.go      # 提示模板单元测试
├── reconnect.go        # 自动重连（指数退避）
├── reconnect_test.go   # 重连单元测试
├── config_expand.go    # Shell 变量展开
├── config_expand_test.go # 变量展开单元测试
├── manager.go          # Manager（集成资源/提示/重连）
└── manager_test.go     # Manager 单元测试
```

**设计目标**：
- 实现 MCP resources 协议（`resources/list`, `resources/read`），暴露为 `mcp_read` 工具
- 实现 MCP prompts 协议（`prompts/list`, `prompts/get`），暴露为 `mcp_prompt` 工具
- 实现 ping 失败时的指数退避自动重连
- 实现 MCP 配置中的 Shell 变量展开

#### 26.15.1 资源协议

**协议映射**：
| MCP 方法 | 内部函数 | Agent 工具 |
|----------|---------|-----------|
| `resources/list` | `Manager.Resources(serverName)` | — |
| `resources/read` | `Manager.ReadResource(ctx, serverName, uri)` | `mcp_read` |

**数据结构**：
```go
type mcpResource struct {
    URI         string `json:"uri"`
    Name        string `json:"name"`
    Description string `json:"description,omitempty"`
    MimeType    string `json:"mimeType,omitempty"`
    Size        int    `json:"size,omitempty"`
}

type ResourceContent struct {
    URI      string `json:"uri"`
    MimeType string `json:"mimeType,omitempty"`
    Text     string `json:"text,omitempty"`
    Blob     string `json:"blob,omitempty"`
}

type ReadResourceResult struct {
    Contents  []ResourceContent `json:"contents"`
    Truncated bool              `json:"truncated,omitempty"`
}
```

**大小限制**：默认 10MB，可通过 `SetMaxResourceSize()` 配置（0 表示不限制）

#### 26.15.2 提示模板协议

**协议映射**：
| MCP 方法 | 内部函数 | Agent 工具 |
|----------|---------|-----------|
| `prompts/list` | `Manager.Prompts(serverName)` | — |
| `prompts/get` | `Manager.GetPrompt(ctx, serverName, name)` | `mcp_prompt` |

**数据结构**：
```go
type mcpPrompt struct {
    Name        string         `json:"name"`
    Description string         `json:"description,omitempty"`
    Arguments   []mcpPromptArg `json:"arguments,omitempty"`
}

type PromptMessage struct {
    Role    string          `json:"role"`
    Content json.RawMessage `json:"content"`
}

type GetPromptResult struct {
    Messages  []PromptMessage `json:"messages"`
    Truncated bool            `json:"truncated,omitempty"`
}
```

#### 26.15.3 自动重连

**重连策略**：指数退避重试（1s, 2s, 4s, 8s...），最大重试 3 次（可配置）

**状态机**：
```
Available → Reconnecting → Available（成功）
                         → Unavailable（超过最大重试）
```

**核心函数**：
```go
// Reconnect 执行指数退避重连
func (m *Manager) Reconnect(ctx context.Context, serverName string) error

// 状态管理
func (s *ServerConnection) setStatus(status ServerStatus)
func (s *ServerConnection) Status() ServerStatus
func (s *ServerConnection) IsAvailable() bool
```

**配置**：
```go
type ServerConfig struct {
    MaxRetries int `json:"max_retries,omitempty"` // 默认 3
}
```

#### 26.15.4 Shell 变量展开

**展开时机**：在 `ServerConfig.LoadConfig()` 中一次性展开（启动时）

**展开字段**：`command`, `args`, `env` 三个字段

**语法支持**：
| 语法 | 示例 | 说明 |
|------|------|------|
| `$VAR` | `$HOME/bin/tool` | 简单变量 |
| `${VAR}` | `${HOME}/config.yaml` | 带花括号的变量 |

**核心函数**：
```go
// expandConfig 对 ServerConfig 中的 command/args/env 执行 Shell 变量展开
func expandConfig(cfg *ServerConfig) *ServerConfig
```

**架构决策**：
- **降级设计**：`resources/list` 和 `prompts/list` 失败时不中断连接，仅返回空列表
- **非破坏性**：现有 MCP 配置无需修改，环境变量展开是可选的
- **隔离性**：一个服务器 unavailable 不影响其他服务器
