# Basework 设计文档

> AI Agent 编程框架 — 从 openwork 做减法，取 opencode 与 crush 之长

---

## 1. 定位与目标

### 定位

Basework 是一个 **Go 语言的可嵌入 AI Agent 框架**。与 opencode（终端产品）和 crush（TUI 产品）不同，basework 的核心价值是作为 **库** 被嵌入到各种应用中 — CLI 工具、Web 服务、IDE 插件、自定义 Agent 等。

### 设计目标

| 目标 | 说明 |
|------|------|
| **最小核心** | 核心 agent loop + provider + tool + session < 3000 行 |
| **可嵌入** | `agent.New(WithModel(...), WithTools(...))` 即可使用 |
| **可扩展** | Hook + Plugin + Skill 三层扩展点 |
| **类型安全** | 统一类型系统，一套 Message/ToolCall 贯穿始终 |
| **可观测** | 事件溯源 Session + PubSub 事件总线 |
| **可选复杂度** | Build Tag 控制可选模块（memory, bedrock, isolation） |

### 非目标

- 不做桌面应用
- 不做 Web UI
- 不做 IDE 插件（但可以被 IDE 插件嵌入）
- 不内置 TUI（但提供 `cmd/basework` CLI 参考实现）

---

## 2. 架构总览

### 2.1 分层架构

```
┌──────────────────────────────────────────────────────┐
│  L4: Application (应用层)                             │
│  CLI / TUI / Embedded / Custom Channel               │
├──────────────────────────────────────────────────────┤
│  L3: Extension (扩展层)                               │
│  Config + Skill + LSP + MCP + Memory                 │
├──────────────────────────────────────────────────────┤
│  L2: Session (会话层)                                 │
│  Event Sourcing Store + Projection                    │
├──────────────────────────────────────────────────────┤
│  L1: Agent Loop (核心层)                              │
│  Pipeline Coordinator + TurnD/Instance + Steering     │
│  + Streaming Callback + Compaction                    │
├──────────────────────────────────────────────────────┤
│  L0: Foundation (基础层)                              │
│  LLM Types + Provider + Tool + Hook/PubSub            │
└──────────────────────────────────────────────────────┘
```

### 2.2 层间依赖规则

- 每层只能依赖下方层，不可反向依赖
- L0 (Foundation) 无外部依赖，只有标准库 + LLM SDK
- L1 (Agent) 依赖 L0，定义核心循环
- L2 (Session) 依赖 L0，独立于 L1（可单独使用）
- L3 (Extension) 内部有两类：
  - **轻量扩展**：Config、Skill、LSP、MCP — 只依赖 L0（+ L4 的 Tool 接口），不依赖 L1
  - **重量扩展**：Memory — 依赖 L0 + L1（需要 agent 上下文）
- L4 (Application) 依赖所有层

### 2.3 数据流

```
User Input
    │
    ▼
┌─────────────────────────────────────────────────────────────┐
│  AgentLoop.HandleMessage(ctx, input)                        │
│                                                              │
│  1. session.AppendEvent(Prompted{Content: input})           │
│  2. pipeline.Run(ctx, turnData)                             │
│     ├─ SetupTurn: 组装历史 + system prompt + tools          │
│     ├─ CallLLM:  provider.Stream(ctx, messages, tools)     │
│     │   └─ 每个 delta → session.AppendEvent(TextDelta{})    │
│     ├─ ExecTools:                                           │
│     │   ├─ hook.BeforeTool(call) → 可修改/拒绝              │
│     │   ├─ tool.Execute(ctx, params)                        │
│     │   ├─ hook.AfterTool(call, result)                     │
│     │   └─ session.AppendEvent(ToolCalled{...})             │
│     └─ Finalize: 持久化 + 判断是否继续                      │
│  3. 如果有工具结果 → 回到 2                                  │
│  4. 返回最终响应                                             │
└─────────────────────────────────────────────────────────────┘
    │
    ▼
Response
```

---

## 3. 各层详细设计

### 3.1 L0: Foundation（基础层）

#### 3.1.1 统一类型系统 (`pkg/llm/`)

**设计决策**：整个框架只有一套消息类型，消除 openwork 中 3 套类型的转换开销。

```go
// pkg/llm/types.go
package llm

// Role 定义消息角色
type Role string

const (
    RoleSystem    Role = "system"
    RoleUser      Role = "user"
    RoleAssistant Role = "assistant"
    RoleTool      Role = "tool"
)

// ChatMessage 是 LLM 对话的基本单元
type ChatMessage struct {
    Role       Role
    Content    []ContentPart
    Name       string            // 可选，工具名或助手名
    ToolCalls  []ToolCall        // 助手请求的工具调用
    ToolCallID string            // 工具响应关联的 ID
}

// ContentPart 支持多模态内容
type ContentPart struct {
    Type     ContentType
    Text     string
    ImageURL string
    // 未来扩展: Audio, Video, File 等
}

type ContentType string

const (
    ContentTypeText  ContentType = "text"
    ContentTypeImage ContentType = "image"
)

// ToolCall 表示一次工具调用请求
type ToolCall struct {
    ID       string
    Name     string
    ArgsJSON string // 原始 JSON 参数
}

// ToolResult 表示工具执行结果
type ToolResult struct {
    ToolCallID string
    Content    string
    IsError    bool
}
```

**借鉴来源**：
- crush 的单一类型贯穿（`internal/message/` 统一定义）
- opencode 的 `ContentPart` 多模态设计（`session/message.ts`）

#### 3.1.2 Model 接口 (`pkg/llm/model.go`)

```go
// pkg/llm/model.go
package llm

// Model 是 LLM 提供商的统一抽象
type Model interface {
    // ID 返回模型唯一标识（如 "gpt-4o", "claude-3.5-sonnet"）
    ID() string
    
    // Generate 同步生成完整响应
    Generate(ctx context.Context, req *Request) (*Response, error)
    
    // Stream 流式生成响应
    Stream(ctx context.Context, req *Request) (<-chan StreamEvent, error)
    
    // Supports 检查模型是否支持特定能力
    Supports(cap Capability) bool
}

// Request 是 LLM 请求
type Request struct {
    Messages    []ChatMessage
    Tools       []ToolDefinition
    MaxTokens   int
    Temperature *float64
    Stop        []string
    // 模型特定选项（透传）
    Extra map[string]any
}

// Response 是 LLM 完整响应
type Response struct {
    Message    ChatMessage
    Usage      Usage
    FinishReason string
}

// StreamEvent 是流式事件
type StreamEvent struct {
    Type      StreamEventType
    Delta     string          // 文本增量
    ToolCall  *ToolCallDelta  // 工具调用增量
    Usage     *Usage          // 用量（通常在最后）
    Error     error
}

type StreamEventType string

const (
    StreamEventText     StreamEventType = "text"
    StreamEventToolCall StreamEventType = "tool_call"
    StreamEventUsage    StreamEventType = "usage"
    StreamEventDone     StreamEventType = "done"
)

// Usage 记录 token 用量
type Usage struct {
    PromptTokens     int
    CompletionTokens int
    TotalTokens      int
}

// Capability 表示模型能力
type Capability string

const (
    CapTools      Capability = "tools"
    CapVision     Capability = "vision"
    CapStreaming  Capability = "streaming"
    CapJSON       Capability = "json_mode"
)
```

**借鉴来源**：
- openwork 的 4 方法接口（简洁）
- opencode 的 model capabilities 设计

#### 3.1.3 Tool 接口 (`pkg/tool/`)

```go
// pkg/tool/tool.go
package tool

// Tool 是工具的统一接口
type Tool interface {
    Name() string
    Description() string
    Parameters() json.RawMessage // JSON Schema
    Execute(ctx context.Context, args json.RawMessage) (*Result, error)
}

// Result 是工具执行结果
type Result struct {
    Content string
    IsError bool
    // 可选：多媒体结果
    Images []string // base64 或 URL
}

// ToolDefinition 是给 LLM 的工具描述（不含执行逻辑）
type ToolDefinition struct {
    Name        string
    Description string
    Parameters  json.RawMessage // JSON Schema
}
```

```go
// pkg/tool/registry.go
package tool

// Registry 管理工具集合
type Registry struct {
    tools    map[string]Tool
    version  int64
    disabled map[string]bool
}

// Clone 创建副本（子 agent 作用域隔离）
func (r *Registry) Clone() *Registry

// Materialize 生成给 LLM 的工具定义列表
func (r *Registry) Materialize() []llm.ToolDefinition

// Settle 执行工具调用
func (r *Registry) Settle(ctx context.Context, call llm.ToolCall) (*Result, error)
```

**借鉴来源**：
- openwork 的 Tool 接口 + Registry（保留）
- opencode 的 `materialize` + `settle` 分离设计

#### 3.1.4 Hook + PubSub (`pkg/hook/`)

```go
// pkg/hook/pubsub.go
package hook

// Broker 是泛型事件总线（参考 crush 的 pubsub.Broker[T]）
type Broker[T any] struct {
    subs []chan T
    mu   sync.RWMutex
}

func (b *Broker[T]) Subscribe() <-chan T
func (b *Broker[T]) Unsubscribe(ch <-chan T)
func (b *Broker[T]) Publish(event T)
func (b *Broker[T]) Close()
```

```go
// pkg/hook/hook.go
package hook

// Hook 是生命周期钩子接口
type Hook interface {
    // BeforeLLM 在 LLM 调用前触发，可修改消息
    BeforeLLM(messages []llm.ChatMessage) ([]llm.ChatMessage, error)
    
    // AfterLLM 在 LLM 调用后触发，只读观察
    AfterLLM(resp *llm.Response, err error)
    
    // BeforeTool 在工具执行前触发，可修改/拒绝
    BeforeTool(call llm.ToolCall) (*llm.ToolCall, error)
    
    // AfterTool 在工具执行后触发，只读观察
    AfterTool(call llm.ToolCall, result *tool.Result, err error)
}

// NopHook 空实现（嵌入用）
type NopHook struct{}

// FuncHook 函数式 hook（便捷构造）
type FuncHook struct {
    BeforeLLMFn  func([]llm.ChatMessage) ([]llm.ChatMessage, error)
    AfterLLMFn   func(*llm.Response, error)
    BeforeToolFn func(llm.ToolCall) (*llm.ToolCall, error)
    AfterToolFn  func(llm.ToolCall, *tool.Result, error)
}
```

```go
// pkg/hook/permission.go
package hook

// PermissionHook 实现权限 = Hook 模式
// 通过 BeforeTool 返回 error 来拒绝工具调用
type PermissionHook struct {
    rules   []Rule
    onAsk   func(action, resource string) (bool, error) // 用户确认回调
}

type Rule struct {
    Action   string // "tool:write", "tool:bash" 等
    Resource string // 通配符匹配
    Effect   Effect // Allow, Deny, Ask
}

type Effect string

const (
    Allow Effect = "allow"
    Deny  Effect = "deny"
    Ask   Effect = "ask"
)
```

**借鉴来源**：
- crush 的 PubSub `Broker[T]` 模式
- opencode 的三值权限（allow/deny/ask）
- 设计创新：权限不再独立系统，而是通过 Hook 实现

#### 3.1.5 Provider 工厂 (`pkg/provider/`)

```go
// pkg/provider/factory.go
package provider

// Config 是 provider 创建配置
type Config struct {
    Type     string // "openai", "anthropic", "gemini", "openai-compat"
    APIKey   string
    BaseURL  string // 可选，自定义端点
    ModelID  string
    Options  map[string]any // provider 特定选项
}

// Create 根据配置创建 Model
func Create(cfg Config) (llm.Model, error) {
    switch cfg.Type {
    case "anthropic":
        return newAnthropic(cfg)
    case "gemini":
        return newGemini(cfg)
    case "openai":
        return newOpenAI(cfg)
    default:
        // 所有 OpenAI 兼容的统一处理
        return newOpenAICompat(cfg)
    }
}
```

**做减法**：
- openwork 的 40+ case switch → 4 个分支（anthropic, gemini, openai, openai-compat）
- OpenAI 兼容的提供商（deepseek, groq, together, etc.）全部走 `newOpenAICompat`，只配置不同 BaseURL

---

### 3.2 L1: Agent Loop（核心层）

#### 3.2.1 Agent 接口

```go
// pkg/agent/agent.go
package agent

// Agent 是核心 agent 接口
type Agent interface {
    // HandleMessage 发送消息并获取响应
    HandleMessage(ctx context.Context, input string) (*Response, error)
    
    // HandleMessages 发送多模态消息
    HandleMessages(ctx context.Context, messages []llm.ChatMessage) (*Response, error)
    
    // Close 优雅关闭
    Close() error
}

// Response 是 agent 响应
type Response struct {
    Message    llm.ChatMessage
    ToolCalls  []ToolCallRecord // 本次会话中的所有工具调用
    Usage      llm.Usage
    SessionID  string
}

// ToolCallRecord 记录一次工具调用
type ToolCallRecord struct {
    Call   llm.ToolCall
    Result *tool.Result
    Err    error
}
```

#### 3.2.2 函数式选项

```go
// pkg/agent/option.go
package agent

// Option 是 agent 配置选项
type Option func(*config)

func WithModel(m llm.Model) Option
func WithTools(t ...tool.Tool) Option
func WithToolRegistry(r *tool.Registry) Option
func WithSystemPrompt(prompt string) Option
func WithSession(s session.Store) Option
func WithHook(h ...hook.Hook) Option
func WithMaxSteps(n int) Option          // 最大工具循环次数，默认 25
func WithPlugin(p ...Plugin) Option
func WithObserver(o Observer) Option     // 向后兼容的观察接口
```

#### 3.2.3 TurnD / Instance 接口（保留 openwork 精华）

```go
// pkg/agent/turn.go
package agent

// TurnD 是 turn 执行所需的依赖接口（保留 openwork 设计）
type TurnD interface {
    // 配置
    SystemPrompt() string
    MaxSteps() int
    
    // 会话
    Session() session.Store
    History() ([]llm.ChatMessage, error)
    
    // Hook
    Hooks() *hook.Chain
    
    // 转向
    DrainSteering() []string
    
    // 工具
    ToolRegistry() *tool.Registry
    
    // 压缩
    ShouldCompact() bool
    Compact(ctx context.Context) error
}

// Instance 是 agent 实例属性接口
type Instance interface {
    ID() string
    Model() llm.Model
    TurnD() TurnD
}
```

#### 3.2.4 Pipeline 四阶段

```go
// pkg/agent/pipeline.go
package agent

// Pipeline 编排一个完整的 turn
type Pipeline struct {
    td TurnD
}

func (p *Pipeline) Run(ctx context.Context) (*TurnResult, error) {
    // Phase 1: Setup
    messages, tools, err := p.setupTurn(ctx)
    
    // Phase 2: CallLLM
    resp, stream, err := p.callLLM(ctx, messages, tools)
    
    // Phase 3: ExecuteTools
    results, err := p.executeTools(ctx, resp.ToolCalls)
    
    // Phase 4: Finalize
    return p.finalize(ctx, resp, results)
}
```

#### 3.2.5 AgentLoop 实现

```go
// pkg/agent/loop.go
package agent

// AgentLoop 是 Agent 的默认实现
type AgentLoop struct {
    instance Instance
    pipeline *Pipeline
    plugins  []Plugin
    observer Observer
    closeFn  func() error
}

func (a *AgentLoop) HandleMessage(ctx context.Context, input string) (*Response, error) {
    // 1. 追加用户消息到 session
    a.instance.TurnD().Session().AppendEvent(session.EventPrompted{Content: input})
    
    // 2. 工具循环
    var allToolCalls []ToolCallRecord
    var totalUsage llm.Usage
    
    for step := 0; step < a.instance.TurnD().MaxSteps(); step++ {
        result, err := a.pipeline.Run(ctx)
        if err != nil {
            return nil, err
        }
        
        allToolCalls = append(allToolCalls, result.ToolCalls...)
        totalUsage.Add(result.Usage)
        
        if !result.HasToolCalls {
            return &Response{
                Message:   result.Message,
                ToolCalls: allToolCalls,
                Usage:     totalUsage,
            }, nil
        }
    }
    
    return nil, ErrMaxStepsExceeded
}
```

**借鉴来源**：
- openwork 的 TurnD/Instance 接口解耦（核心保留）
- openwork 的四阶段 Pipeline（核心保留）
- crush 的 cancel 序列号机制（用于 steering 中断）

---

### 3.3 L2: Session（会话层）

#### 3.3.1 事件溯源设计

```go
// pkg/session/event.go
package session

// EventType 定义事件类型
type EventType string

const (
    // 用户输入
    EventPrompted EventType = "prompted"
    
    // 助手输出
    EventTextStarted EventType = "text.started"
    EventTextDelta   EventType = "text.delta"
    EventTextEnded   EventType = "text.ended"
    
    // 工具调用
    EventToolCalled   EventType = "tool.called"
    EventToolSuccess  EventType = "tool.success"
    EventToolFailed   EventType = "tool.failed"
    
    // Turn 生命周期
    EventTurnStarted EventType = "turn.started"
    EventTurnEnded   EventType = "turn.ended"
    EventTurnFailed  EventType = "turn.failed"
    
    // 元事件
    EventCompacted     EventType = "compacted"
    EventAgentSwitched EventType = "agent.switched"
    EventModelSwitched EventType = "model.switched"
)

// Event 是会话事件
type Event struct {
    ID        string
    SessionID string
    Type      EventType
    Data      json.RawMessage // 事件特定数据
    Seq       int64           // 序列号
    CreatedAt time.Time
}
```

#### 3.3.2 Store 接口

```go
// pkg/session/store.go
package session

// Store 是会话存储接口
type Store interface {
    // 事件追加（核心操作）
    AppendEvent(event Event) error
    
    // 事件查询
    Events(filter EventFilter) ([]Event, error)
    
    // 会话管理
    Create(opts CreateOpts) (*Info, error)
    Get(id string) (*Info, error)
    List(filter ListFilter) ([]*Info, error)
    Delete(id string) error
    
    // 投影（从事件重建消息）
    Messages() ([]llm.ChatMessage, error)
}

// Info 是会话元信息
type Info struct {
    ID           string
    Title        string
    MessageCount int
    Usage        llm.Usage
    CreatedAt    time.Time
    UpdatedAt    time.Time
}
```

#### 3.3.3 后端实现

- **MemoryStore**：纯内存，适合测试和短期会话
- **JSONLStore**：追加写入 JSONL 文件，适合简单持久化
- **SQLiteStore**：SQLite 存储，支持 FTS5 全文搜索（可选 build tag）

**借鉴来源**：
- opencode 的事件溯源 Session（完整借鉴）
- crush 的 SQLite + PubSub 模式（Store 实现参考）
- openwork 的可插拔后端接口（保留）

---

### 3.4 L3: Periphery（外围层）

#### 3.4.1 Config（Copy-on-Write）

```go
// pkg/config/config.go
package config

// Config 是不可变配置
type Config struct {
    Model      string            `json:"model"`
    APIKey     string            `json:"api_key"`
    Provider   string            `json:"provider"`
    BaseURL    string            `json:"base_url,omitempty"`
    MaxSteps   int               `json:"max_steps,omitempty"`
    SystemPrompt string          `json:"system_prompt,omitempty"`
    Tools      map[string]bool   `json:"tools,omitempty"`      // 工具开关
    Hooks      []HookConfig      `json:"hooks,omitempty"`
    Skills     []string          `json:"skills,omitempty"`     // skill 路径
    Memory     MemoryConfig      `json:"memory,omitempty"`
}

// Store 管理配置（Copy-on-Write）
type Store struct {
    mu      sync.RWMutex
    config  *Config
    path    string
}

func (s *Store) Get() *Config           // 返回不可变快照
func (s *Store) Mutate(fn func(*Config)) // 原子修改
func (s *Store) Load(path string) error
func (s *Store) Save() error
```

**借鉴来源**：crush 的 `ConfigStore`（`cloneForWrite()` + 原子交换）

#### 3.4.2 Skill（Markdown 标准）

```go
// pkg/skill/skill.go
package skill

// Skill 定义
type Skill struct {
    Name         string
    Description  string
    Instructions string   // markdown body
    FilePath     string
    Builtin      bool
    Metadata     map[string]string
}

// Loader 发现和加载 skill
type Loader struct {
    paths []string          // 搜索路径
    skills map[string]*Skill
}

func (l *Loader) Discover() error                    // 扫描目录发现 SKILL.md
func (l *Loader) Active() []*Skill                   // 返回活跃 skill
func (l *Loader) ToPromptXML() string                // 生成 system prompt 注入片段
```

**Skill 文件格式**（采用 crush 的 Agent Skills 标准）：

```markdown
---
name: my-skill
description: "做什么用的"
---

## Instructions

模型看到的具体指令内容...
```

**借鉴来源**：crush 的 `internal/skills/`（完整借鉴 markdown 标准）

#### 3.4.3 Permission（Hook 模式）

见 3.1.4 的 `PermissionHook`。不独立建系统，通过 Hook 实现。

---

### 3.5 L4: Application（应用层）

这一层不在 `pkg/` 中，而是作为参考实现：

- `cmd/basework/` — CLI 入口
- `internal/tui/` — 可选 TUI（build tag: `tui`）
- 宿主应用可以完全不用这一层，直接使用 `pkg/agent`

---

## 4. 与 openwork 的对比：保留与裁剪

| 组件 | 决策 | 理由 |
|------|------|------|
| 统一类型 (`pkg/llm`) | **新建** | 消除 3 套消息类型的转换开销 |
| TurnD/Instance 接口 | **保留** | openwork 最好的设计 |
| 四阶段 Pipeline | **保留** | 清晰的关注点分离 |
| Tool 接口 | **保留+精简** | 去掉 facade 层 |
| Plugin (54 行) | **保留** | 足够简单 |
| Observer | **简化为适配层** | 统一通过 Hook 系统 |
| Hook + EventBus + Observer | **统一为 Hook + PubSub** | 消除 3 套事件系统 |
| 权限系统 | **删除独立系统** | 通过 PermissionHook 实现 |
| Session | **事件溯源重构** | 参考 opencode |
| Config 热重载 | **删除** | 改为 CoW 不可变配置 |
| Skill 20 文件 | **精简到 2 文件** | 采用 crush markdown 标准 |
| SubTurn 22 字段 | **精简到 8 字段** | 去掉 RestrictionConfig |
| Seahorse build tag 矩阵 | **简化** | 去掉 slim，保留 seahorse 单一开关 |
| 40+ provider switch | **折叠为 4 分支** | OpenAI 兼容统一处理 |
| shared_facade.go 等 | **删除** | 纯样板代码 |
| toolloop.go RunToolLoop | **删除** | 子代理用完整协调器 |

---

## 5. Build Tag 策略

| Tag | 默认 | 说明 |
|-----|------|------|
| `memory` | 关 | SQLite 上下文管理（FTS5 全文搜索） |
| `tui` | 关 | TUI 界面（如未来添加） |

无 `slim`、`bedrock`、`isolation` 标签。所有可选模块默认关闭，需要时显式开启。

> **说明**：bedrock 和 isolation 在 openwork 中是可选模块，但在 basework 中不作为核心设计。
> 如需 AWS Bedrock，通过 `openai-compat` provider 配置 BaseURL 即可。
> 如需进程隔离，由宿主应用在 L4 实现，不属于框架职责。

---

## 6. 目录结构

> 见 §15 的完整目录结构（含 LSP、MCP 等补充设计后的最终版）。

---

## 7. 核心设计决策记录

### D1: 为什么选事件溯源而非消息追加？

opencode 的 session 完全基于事件溯源。每个状态变更都是持久化事件，消息是事件的投影。

**优点**：
- 完全可重放：任何时刻可以重建会话状态
- 审计友好：所有变更都有记录
- 可嵌入友好：宿主应用可以观察、导出、转换会话
- 流式友好：delta 事件天然适配流式输出

**代价**：
- 存储体积略大（但 delta 事件只持久化 Ended，不存 Delta）
- 查询需要投影（但可以用缓存加速）

### D2: 为什么删除独立权限系统？

opencode 有完整的三值权限系统（allow/deny/ask + Deferred + Saved Rules），约 300+ 行。

**替代方案**：`PermissionHook` 通过 `BeforeTool` hook 实现相同功能。
- `Allow` → 直接返回 nil
- `Deny` → 返回 error
- `Ask` → 调用 `onAsk` 回调阻塞等待用户确认

**优点**：
- 减少 300+ 行独立代码
- 权限逻辑可组合（多个 PermissionHook 链接）
- 宿主应用可以自定义权限策略（不局限于 allow/deny/ask）

### D3: 为什么统一 Hook + PubSub？

openwork 有 Observer + EventBus + HookManager 三套。

**统一方案**：
- PubSub (`Broker[T]`) 用于子系统间松耦合通信（session 变更、工具状态等）
- Hook 用于 agent loop 生命周期拦截（Before/After Tool/LLM）
- Observer 退化为 Hook 的薄适配层（向后兼容）

### D4: 为什么 Provider 只有 4 分支？

openwork 的 40+ case switch 中，35+ 个都是 OpenAI 兼容的 HTTP API，只是 BaseURL 和默认模型名不同。

**替代方案**：
- `anthropic` → 专用实现
- `gemini` → 专用实现
- `openai` → 专用实现
- 其他所有 → `openai-compat`（配置 BaseURL + 可能的差异微调）

用户通过配置而非代码来添加新提供商。

---

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

## 10. 补充设计：上下文压缩策略

> 当对话超出模型上下文窗口时，需要智能压缩。

### 10.1 触发条件

```go
// pkg/agent/compact.go
package agent

// CompactConfig 压缩配置
type CompactConfig struct {
    // TriggerThreshold 触发压缩的 token 占比（0.0-1.0）
    // 默认 0.8，即使用 80% 上下文窗口时触发
    TriggerThreshold float64
    
    // KeepRecent 保留最近的消息数（不压缩）
    KeepRecent int
    
    // Strategy 压缩策略
    Strategy CompactStrategy
}

type CompactStrategy string

const (
    // StrategySummary 用 LLM 生成旧消息摘要
    StrategySummary CompactStrategy = "summary"
    
    // StrategyTruncate 直接截断旧消息
    StrategyTruncate CompactStrategy = "truncate"
)
```

### 10.2 压缩流程

```
1. SetupTurn 阶段检查 token 用量
2. 如果 tokenCount > maxTokens * TriggerThreshold:
   a. 将最近 KeepRecent 条消息标记为"保留"
   b. 其余旧消息 → 压缩
   c. 压缩方式取决于 Strategy:
      - summary: 调用 LLM 生成摘要（使用专门的 compaction model）
      - truncate: 直接丢弃
   d. 替换旧消息为一条 Compacted 消息
   e. 追加 session.EventCompacted{Summary, TruncatedSeq}
3. 重试 LLM 调用
```

### 10.3 摘要压缩实现

```go
// CompactSummary 使用 LLM 生成旧消息摘要
func CompactSummary(ctx context.Context, model llm.Model, messages []llm.ChatMessage) (string, error) {
    prompt := `请将以下对话历史压缩为简洁的摘要，保留关键信息：
- 用户的目标和意图
- 已完成的操作和结果
- 当前正在进行的工作
- 重要的文件路径和代码引用

对话历史：
%s

请输出摘要：`
    
    // 构建压缩请求
    content := formatMessages(messages)
    req := &llm.Request{
        Messages: []llm.ChatMessage{
            {Role: llm.RoleUser, Content: []llm.ContentPart{{Type: llm.ContentTypeText, Text: fmt.Sprintf(prompt, content)}}},
        },
        MaxTokens: 1024,
    }
    
    resp, err := model.Generate(ctx, req)
    if err != nil {
        return "", err
    }
    
    return resp.Message.Content[0].Text, nil
}
```

### 10.4 设计决策

- **默认 summary 策略**：直接截断会丢失关键信息，LLM 摘要保留上下文
- **可配置阈值**：不同模型的上下文窗口不同，用比例而非绝对值
- **KeepRecent**：最近的几轮对话通常最重要，不压缩
- **事件溯源集成**：压缩产生 `EventCompacted`，投影时截断旧消息
- **不做**：不做 DAG 式多级摘要（openwork 的 seahorse 方案过于复杂）

---

## 11. 补充设计：Steering 机制

> 用户如何在 agent 运行中注入新指令。

### 11.1 两种模式

```go
// pkg/agent/steering.go
package agent

// DeliveryMode 定义消息交付语义
type DeliveryMode string

const (
    // Steer 立即抢占当前运行，注入新指令
    // 参考 opencode 的 steer 模式
    Steer DeliveryMode = "steer"
    
    // Queue 排队等待，当前 turn 完成后处理
    // 参考 opencode 的 queue 模式
    Queue DeliveryMode = "queue"
)
```

### 11.2 接口设计

```go
// InjectMessage 向运行中的 agent 注入消息
func (a *AgentLoop) InjectMessage(ctx context.Context, content string, mode DeliveryMode) error {
    switch mode {
    case Steer:
        // 1. 设置 interrupt 标记
        // 2. 当前工具执行完后中断
        // 3. 将新消息插入到历史头部
        // 4. 触发新一轮 turn
        a.steeringCh <- content
        
    case Queue:
        // 排队等待当前 turn 完成
        a.pendingCh <- content
    }
    return nil
}
```

### 11.3 Cancel 序列号（参考 crush）

```go
// 每个注入的请求分配单调递增的序列号
type steerRequest struct {
    seq     int64
    content string
}

// AgentLoop 维护 cancelMark
// cancel(seq) 只取消 seq ≤ mark 的请求，不影响后续排队请求
// 这解决了"取消但不取消后续"的精确控制问题
```

### 11.4 Pipeline 集成

```
Pipeline.Run() 中:
  setupTurn 阶段:
    1. 检查 steeringCh 是否有新消息
    2. 如果有，插入到消息列表末尾（作为 user message）
    3. 清空 steeringCh
  
  executeTools 阶段:
    1. 每个工具执行前检查 interrupt 标记
    2. 如果 interrupted → 跳过剩余工具，进入 finalize
    3. finalize 检查 pendingCh → 如果有排队消息，设置 needsContinuation
```

### 11.5 设计决策

- **两种模式足够**：steer（紧急打断）+ queue（耐心等待），覆盖所有场景
- **序列号取消**：比简单的 bool 标记更精确，避免误取消后续请求
- **工具粒度中断**：不是立即中断（可能破坏文件状态），而是在工具执行间检查

---

## 12. 补充设计：流式传播架构

> StreamEvent 如何从 provider → agent → 宿主应用。

### 12.1 流式数据流

```
Provider                  AgentLoop                Consumer
   │                         │                        │
   │  <-chan StreamEvent     │                        │
   ├────────────────────────→│                        │
   │  Text{Delta: "Hello"}   │  OnTextDelta("Hello")  │
   │                         ├───────────────────────→│
   │  Text{Delta: " world"} │  OnTextDelta(" world") │
   │                         ├───────────────────────→│
   │  ToolCall{...}         │  OnToolCall(call)       │
   │                         ├───────────────────────→│
   │  Done{}                │  OnTurnEnd(resp)        │
   │                         ├───────────────────────→│
```

### 12.2 消费者接口

```go
// pkg/agent/callback.go
package agent

// Callback 是宿主应用接收流式事件的接口
type Callback interface {
    // OnTextDelta 收到文本增量
    OnTextDelta(delta string)
    
    // OnToolCallStart 工具调用开始
    OnToolCallStart(call llm.ToolCall)
    
    // OnToolCallEnd 工具调用结束
    OnToolCallEnd(call llm.ToolCall, result *tool.Result, err error)
    
    // OnThinkingDelta 推理增量（如 Anthropic extended thinking）
    OnThinkingDelta(delta string)
    
    // OnTurnEnd 一个 turn 结束
    OnTurnEnd(resp *Response)
    
    // OnError 发生错误
    OnError(err error)
}

// NopCallback 空实现
type NopCallback struct{}
```

### 12.3 Agent 选项集成

```go
// WithCallback 设置流式回调
func WithCallback(cb Callback) Option

// WithCallbackFunc 函数式便捷构造
func WithCallbackFunc(opts CallbackFuncs) Option

type CallbackFuncs struct {
    OnTextDeltaFn     func(string)
    OnToolCallStartFn func(llm.ToolCall)
    OnToolCallEndFn   func(llm.ToolCall, *tool.Result, error)
    OnThinkingDeltaFn func(string)
    OnTurnEndFn       func(*Response)
    OnErrorFn         func(error)
}
```

### 12.4 Pipeline 中的流式处理

```go
func (p *Pipeline) callLLM(ctx context.Context, messages []llm.ChatMessage, tools []llm.ToolDefinition) (*llm.Response, error) {
    stream, err := model.Stream(ctx, &llm.Request{
        Messages: messages,
        Tools:    tools,
    })
    if err != nil {
        return nil, err
    }
    
    var fullText strings.Builder
    var toolCalls []llm.ToolCall
    
    for event := range stream {
        switch event.Type {
        case llm.StreamEventText:
            fullText.WriteString(event.Delta)
            p.callback.OnTextDelta(event.Delta)
            // 同时追加事件到 session
            p.session.AppendEvent(session.Event{
                Type: session.EventTextDelta,
                Data: encode(session.TextDeltaData{Delta: event.Delta}),
            })
            
        case llm.StreamEventToolCall:
            // 累积 tool call delta
            accumulateToolCall(&toolCalls, event.ToolCall)
            if event.ToolCall.IsComplete() {
                p.callback.OnToolCallStart(toolCalls[len(toolCalls)-1])
            }
            
        case llm.StreamEventDone:
            // 完成
        }
    }
    
    return &llm.Response{...}, nil
}
```

### 12.5 设计决策

- **回调模式**（而非 channel）：回调更简单，不需要消费者管理 goroutine 生命周期
- **同时写入 session**：流式 delta 同时回调消费者和追加到 session 事件
- **函数式便捷**：不需要实现完整 Callback 接口，只传需要的函数

---

## 13. 补充设计：Memory 系统

> 可选的持久化记忆模块，支持上下文检索和自动知识积累。

### 13.1 架构

```
pkg/memory/
├── engine.go           # 短期记忆引擎（压缩 + 检索 + 组装）
├── fts.go              # FTS5 全文搜索（build tag: memory）
├── store.go            # MemoryStore 接口
├── engine_stub.go      # 无 build tag 时的存根
└── types.go            # 类型定义
```

### 13.2 核心接口

```go
// pkg/memory/types.go
package memory

// MemoryLayer 记忆层级
type MemoryLayer string

const (
    // LayerUser 用户级记忆（跨项目）
    LayerUser MemoryLayer = "user"
    
    // LayerProject 项目级记忆
    LayerProject MemoryLayer = "project"
    
    // LayerLocal 本地记忆（.gitignore 中）
    LayerLocal MemoryLayer = "local"
    
    // LayerAuto 自动积累的记忆
    LayerAuto MemoryLayer = "auto"
)

// Entry 是一条记忆条目
type Entry struct {
    Layer   MemoryLayer
    Content string
    Tags    []string
    CreatedAt time.Time
}
```

```go
// pkg/memory/store.go
package memory

// Store 是记忆存储接口
type Store interface {
    // Write 写入记忆
    Write(layer MemoryLayer, content string, tags ...string) error
    
    // Read 读取指定层的所有记忆
    Read(layer MemoryLayer) ([]Entry, error)
    
    // Search 全文搜索记忆
    Search(query string, limit int) ([]Entry, error)
    
    // Clear 清空指定层
    Clear(layer MemoryLayer) error
    
    // List 列出所有记忆层
    List() map[MemoryLayer][]Entry
}
```

### 13.3 短期记忆引擎

```go
// pkg/memory/engine.go
package memory

// Engine 是短期记忆引擎（build tag: memory）
// 负责在会话过长时智能压缩和检索相关记忆
type Engine struct {
    store     Store
    fts       *FTSIndex   // FTS5 全文搜索
    maxTokens int
}

// Compress 压缩对话历史
// 保留最近 N 条消息，其余用摘要替换
func (e *Engine) Compress(ctx context.Context, messages []llm.ChatMessage) ([]llm.ChatMessage, error)

// Retrieve 根据当前上下文检索相关记忆
func (e *Engine) Retrieve(ctx context.Context, currentMessages []llm.ChatMessage, limit int) ([]Entry, error)

// Assemble 组装上下文：系统提示 + 相关记忆 + 对话历史
func (e *Engine) Assemble(systemPrompt string, memories []Entry, messages []llm.ChatMessage) []llm.ChatMessage
```

### 13.4 AGENTS.md 集成

记忆层映射到文件：

| Layer | 文件路径 | 说明 |
|-------|---------|------|
| `user` | `~/.config/basework/AGENTS.md` | 用户全局偏好 |
| `project` | `{workspace}/AGENTS.md` | 项目知识 |
| `local` | `{workspace}/.basework/AGENTS.md` | 本地私有（gitignore） |
| `auto` | `{workspace}/.basework/auto-memory.md` | 自动积累 |

### 13.5 设计决策

- **Build Tag 可选**：FTS5 需要 SQLite，通过 `memory` build tag 控制
- **存根模式**：无 build tag 时 `Engine` 是空操作，`Store` 退化为基础文件读写
- **文件优先**：记忆以 Markdown 文件存储，人可读可编辑
- **不做**：不做 DAG 式多级摘要、不做向量嵌入检索

---

## 14. 补充设计：次要缺口

### 14.1 Session Fork/Branch

```go
// pkg/session/store.go 扩展
type Store interface {
    // ... 原有方法 ...
    
    // Fork 从指定序列号创建分支会话
    Fork(fromSeq int64) (string, error) // 返回新 session ID
}

// Fork 实现：
// 1. 复制源 session 的事件直到 fromSeq
// 2. 创建新 session ID
// 3. 后续事件独立追加
```

用于并行探索：agent 在一个分支上尝试方案 A，另一个分支尝试方案 B。

### 14.2 多 Agent 路由

```go
// pkg/agent/registry.go
package agent

// AgentDef 定义一个 agent
type AgentDef struct {
    Name         string
    Description  string
    SystemPrompt string
    Tools        []tool.Tool      // 额外工具
    Permissions  []hook.Rule      // 权限规则
    Model        string           // 模型覆盖（可选）
}

// Registry 管理多个 agent 定义
type Registry struct {
    agents map[string]*AgentDef
}

// Select 根据名称选择 agent
func (r *Registry) Select(name string) (*AgentDef, error)

// List 列出所有可用 agent
func (r *Registry) List() []*AgentDef
```

内置 agent：
- **build**：默认，全权限，用于开发
- **plan**：只读，不允许写文件和执行命令，用于分析和规划

通过 `WithAgent(name)` 选项选择 agent，或通过 Agent Registry 动态切换。

### 14.3 Channel 消息分发

```go
// pkg/agent/channel.go
package agent

// Channel 是消息输入/输出通道接口
type Channel interface {
    // Send 发送消息到通道
    Send(msg llm.ChatMessage) error
    
    // Receive 从通道接收消息
    Receive(ctx context.Context) (llm.ChatMessage, error)
    
    // Close 关闭通道
    Close() error
}

// StdioChannel stdin/stdout 通道（CLI 用）
type StdioChannel struct{}

// CallbackChannel 回调通道（嵌入用）
type CallbackChannel struct {
    onMessage func(llm.ChatMessage)
}
```

宿主应用可以实现自定义 Channel（WebSocket、gRPC、消息队列等），将 agent 连接到任意通信协议。

---

## 15. 更新后的目录结构

```
basework/
├── cmd/basework/              # CLI 参考实现
├── pkg/
│   ├── llm/                   # L0: 统一类型
│   ├── tool/                  # L0: 工具系统
│   │   └── builtin/           # 内置工具
│   ├── hook/                  # L0: Hook + PubSub + Permission
│   ├── provider/              # L0: Provider 工厂
│   ├── agent/                 # L1: Agent 核心
│   │   ├── agent.go           # Agent interface
│   │   ├── loop.go            # AgentLoop
│   │   ├── pipeline.go        # 四阶段管道
│   │   ├── turn.go            # TurnD/Instance
│   │   ├── option.go          # 函数式选项
│   │   ├── callback.go        # 流式回调
│   │   ├── steering.go        # Steering 机制
│   │   ├── compact.go         # 上下文压缩
│   │   ├── plugin.go          # Plugin interface
│   │   ├── observer.go        # Observer 适配
│   │   ├── registry.go        # Agent Registry（多 agent）
│   │   ├── channel.go         # Channel 接口
│   │   └── subturn.go         # 子代理
│   ├── session/               # L2: 事件溯源会话
│   ├── lsp/                   # L3: LSP 集成
│   │   ├── manager.go
│   │   ├── client.go
│   │   ├── types.go
│   │   ├── auto.go
│   │   └── tool.go
│   ├── mcp/                   # L3: MCP 协议
│   │   ├── manager.go
│   │   ├── transport.go
│   │   ├── adapter.go
│   │   └── config.go
│   ├── memory/                # L3: 记忆系统（build tag: memory）
│   ├── config/                # L3: 配置
│   └── skill/                 # L3: Skill
├── internal/                  # 终端产品专用逻辑
│   ├── compaction/            上下文压缩
│   ├── retry/                 重试机制
│   ├── permission/            权限系统
│   ├── subagent/              子代理系统
│   ├── loopdetect/            循环检测
│   ├── observability/         可观测性（日志 + 成本）
│   ├── oauth/                 OAuth 2.0 认证
│   └── tui/                   终端 UI（计划中）
└── cmd/basework/              CLI 参考实现
```

---

## 16. 更新后的设计决策记录

### D5: LSP 为什么暴露为 Tool 而非内部服务？

crush 的 LSP 集成是内部服务，agent 直接调用。但 basework 作为可嵌入框架，应该让 LLM 自主决定何时使用 LSP。

**方案**：LSP 功能包装为 `tool.Tool`，注册到 ToolRegistry。LLM 通过工具调用使用 LSP。
**优点**：LLM 有自主权，框架不需要硬编码 LSP 使用逻辑。

### D6: MCP 为什么只支持 stdio + http？

SSE 传输在 opencode/crush 中使用较少，且实现复杂度高于 stdio/http。作为 v1 只支持两种最常用传输。需要 SSE 时再添加。

### D7: 上下文压缩为什么不用 DAG 结构？

openwork 的 seahorse 使用 SummaryNode DAG，支持多级摘要和检索。这非常优雅但也非常复杂（~4000 行）。

**替代方案**：简单的"保留最近 N 条 + LLM 摘要其余"策略，覆盖 95% 的场景。DAG 结构可以作为未来增强。

### D8: Streaming 为什么用回调而非 Channel？

Channel 模式需要消费者管理 goroutine 生命周期和 context 取消。回调模式更简单：
- 消费者只需实现接口方法
- 不需要 `go func()` + `select` 模式
- 错误处理更自然（方法内直接处理）

---

## 17. System Prompt 组合流程

> 最终的 system prompt 由多个来源拼接而成。

### 17.1 组合顺序

```
┌─────────────────────────────────────────────────────┐
│ 1. 基础 system prompt（用户配置或默认）               │
│    WithSystemPrompt("You are a helpful assistant...") │
├─────────────────────────────────────────────────────┤
│ 2. Agent 定义覆盖（如果选了非默认 agent）              │
│    AgentDef.SystemPrompt 追加/替换基础 prompt          │
├─────────────────────────────────────────────────────┤
│ 3. Skill 指令注入（XML 格式）                         │
│    <skills>                                          │
│      <skill name="xxx">instructions</skill>           │
│    </skills>                                         │
├─────────────────────────────────────────────────────┤
│ 4. Memory 上下文注入（检索到的相关记忆）                │
│    <memory>                                          │
│      <project>AGENTS.md 内容</project>                │
│      <auto>自动积累的记忆</auto>                       │
│    </memory>                                         │
├─────────────────────────────────────────────────────┤
│ 5. 工具约束提示（自动附加）                            │
│    "You have access to the following tools: ..."     │
│    "Always ask before destructive operations."       │
└─────────────────────────────────────────────────────┘
```

### 17.2 实现

```go
// pkg/agent/system_prompt.go
package agent

// ComposeSystemPrompt 组装最终 system prompt
func ComposeSystemPrompt(
    base string,                    // 用户配置的基础 prompt
    agentDef *AgentDef,             // agent 定义（可选）
    skills []*skill.Skill,          // 活跃 skill 列表
    memories []memory.Entry,        // 检索到的记忆
    toolDefs []llm.ToolDefinition,  // 可用工具列表
) string {
    var b strings.Builder
    
    // 1. 基础 prompt
    b.WriteString(base)
    
    // 2. Agent 覆盖
    if agentDef != nil && agentDef.SystemPrompt != "" {
        b.WriteString("\n\n")
        b.WriteString(agentDef.SystemPrompt)
    }
    
    // 3. Skill 注入
    if len(skills) > 0 {
        b.WriteString("\n\n")
        b.WriteString(skill.ToPromptXML(skills))
    }
    
    // 4. Memory 注入
    if len(memories) > 0 {
        b.WriteString("\n\n")
        b.WriteString(memory.FormatForPrompt(memories))
    }
    
    // 5. 工具约束
    b.WriteString("\n\n")
    b.WriteString(formatToolConstraints(toolDefs))
    
    return b.String()
}
```

### 17.3 缓存策略

System prompt 在每个 turn 的 `setupTurn` 阶段组装。由于 skill 和 memory 可能动态变化，**不缓存**，每次重新组装。组装开销很小（字符串拼接）。

---

## 18. 并发模型

> 明确框架各组件的并发安全保证。

### 18.1 并发安全矩阵

| 组件 | 并发安全？ | 说明 |
|------|-----------|------|
| `Agent` | **否** | 一个 Agent 实例同一时间只能有一个活跃的 HandleMessage 调用。多并发需创建多个 Agent 实例 |
| `AgentLoop.InjectMessage` | **是** | 可从任意 goroutine 调用，通过 channel 传递 |
| `tool.Registry` | **是** | 内部 sync.RWMutex 保护，支持并发读取和串行写入 |
| `session.Store` | **是** | MemoryStore 用 RWMutex，JSONL/SQLite 用文件锁 |
| `hook.Broker[T]` | **是** | 内部 RWMutex 保护订阅者列表 |
| `hook.Chain` | **是** | 只读结构体，创建后不可变 |
| `config.Store` | **是** | Copy-on-Write 模式，Get 无锁，Mutate 互斥 |
| `llm.Model` | **是** | Provider 内部维护 HTTP 连接池，支持并发请求 |
| `lsp.Manager` | **是** | 内部 RWMutex 保护客户端映射 |
| `mcp.Manager` | **是** | 内部 RWMutex 保护连接映射 |

### 18.2 HandleMessage 的非并发约束

```go
// AgentLoop 内部使用 mutex 防止并发 HandleMessage
type AgentLoop struct {
    runMu sync.Mutex  // 确保同一时间只有一个 turn 在运行
    // ...
}

func (a *AgentLoop) HandleMessage(ctx context.Context, input string) (*Response, error) {
    if !a.runMu.TryLock() {
        return nil, ErrAgentBusy  // 已有 turn 在运行
    }
    defer a.runMu.Unlock()
    // ...
}
```

### 18.3 多实例模式

如果需要并发处理多个对话，创建多个 Agent 实例：

```go
// 每个对话一个 Agent 实例
agent1, _ := agent.New(agent.WithModel(model), agent.WithSession(session1))
agent2, _ := agent.New(agent.WithModel(model), agent.WithSession(session2))

// 可以并发调用
go agent1.HandleMessage(ctx, "task 1")
go agent2.HandleMessage(ctx, "task 2")
```

多个 Agent 可以共享同一个 `llm.Model`（Provider 是并发安全的）和同一个 `tool.Registry`。

---

## 19. Token 计数策略

> 用于上下文压缩触发和 usage 统计。

### 19.1 计数方式

```go
// pkg/llm/tokens.go
package llm

// EstimateTokens 估算 token 数量
// 使用字符数近似：英文 ~4 字符/token，中文 ~2 字符/token
// 误差约 ±20%，仅用于触发压缩判断，不需要精确
func EstimateTokens(messages []ChatMessage) int {
    total := 0
    for _, msg := range messages {
        for _, part := range msg.Content {
            total += estimateString(part.Text)
        }
        // 工具定义开销（每个工具约 200-500 tokens）
        for _, tc := range msg.ToolCalls {
            total += 300 + len(tc.ArgsJSON)/4
        }
    }
    return total
}

func estimateString(s string) int {
    // 简单启发式：ASCII 按 4 字符/token，非 ASCII 按 2 字符/token
    ascii, nonASCII := 0, 0
    for _, r := range s {
        if r < 128 {
            ascii++
        } else {
            nonASCII++
        }
    }
    return ascii/4 + nonASCII/2
}
```

### 19.2 精确值来源

- **Provider 返回的 Usage**：每次 LLM 调用后，`Response.Usage` 包含精确 token 数
- **累积统计**：AgentLoop 在整个 session 中累积精确 Usage
- **压缩触发**：使用 `EstimateTokens`（近似值），避免每次精确计数

### 19.3 压缩触发检查

```go
func (p *Pipeline) shouldCompact(estimatedTokens int, maxContextTokens int) bool {
    threshold := p.config.CompactConfig.TriggerThreshold // 默认 0.8
    return float64(estimatedTokens) > float64(maxContextTokens)*threshold
}
```

---

## 20. 安全模型

> 工具执行的安全约束和输入/输出清洗。

### 20.1 工具安全层级

```
┌─────────────────────────────────────────────┐
│ L1: 权限检查（PermissionHook）               │
│      → 用户确认 allow/deny/ask               │
├─────────────────────────────────────────────┤
│ L2: 路径约束                                 │
│      → 文件操作限制在工作区内                  │
├─────────────────────────────────────────────┤
│ L3: 输出清洗                                 │
│      → 工具输出截断（防止上下文溢出）          │
├─────────────────────────────────────────────┤
│ L4: 沙箱（可选，build tag: isolation）        │
│      → 进程级隔离                             │
└─────────────────────────────────────────────┘
```

### 20.2 路径约束

```go
// pkg/tool/builtin/safety.go
package builtin

// ValidatePath 验证文件路径是否在工作区内
func ValidatePath(path string, workspace string) error {
    abs, err := filepath.Abs(path)
    if err != nil {
        return fmt.Errorf("invalid path: %w", err)
    }
    
    absWorkspace, err := filepath.Abs(workspace)
    if err != nil {
        return fmt.Errorf("invalid workspace: %w", err)
    }
    
    if !strings.HasPrefix(abs, absWorkspace) {
        return fmt.Errorf("path %q is outside workspace %q", path, workspace)
    }
    
    return nil
}
```

### 20.3 输出截断

```go
// pkg/tool/truncate.go
package tool

const MaxOutputSize = 100_000 // 字符，约 25k tokens

// TruncateOutput 截断过大的工具输出
func TruncateOutput(content string) string {
    if len(content) <= MaxOutputSize {
        return content
    }
    half := MaxOutputSize / 2
    return content[:half] + "\n\n[... truncated " + 
        strconv.Itoa(len(content)-MaxOutputSize) + 
        " characters ...]\n\n" + content[len(content)-half:]
}
```

### 20.4 Prompt Injection 防护

框架层面不做 prompt injection 检测（这是宿主应用的责任），但提供以下安全约束：
- 工具输出作为 `ToolResult` 返回，而非直接注入 user message
- LLM 不会将工具输出视为用户指令
- 宿主应用可通过 `BeforeTool` hook 检查/清洗工具参数

---

## 21. Logging 与可观测性

> 框架内部的日志和可观测性策略。

### 21.1 设计原则

- **框架不内置日志库** — 使用 Go 标准 `log/slog`
- **宿主应用控制日志级别** — 通过 `WithLogger` 选项注入
- **结构化日志** — 所有日志使用 key-value 格式

### 21.2 Logger 接口

```go
// pkg/agent/logger.go
package agent

// Logger 是框架日志接口
type Logger interface {
    Debug(msg string, args ...any)
    Info(msg string, args ...any)
    Warn(msg string, args ...any)
    Error(msg string, args ...any)
}

// WithLogger 注入自定义 logger
func WithLogger(l Logger) Option

// DefaultLogger 默认使用 slog.Default()
func DefaultLogger() Logger
```

### 21.3 关键日志点

| 事件 | 级别 | 内容 |
|------|------|------|
| Turn 开始 | Debug | session_id, step, model |
| LLM 调用 | Debug | model, token_count |
| LLM 响应 | Debug | finish_reason, usage |
| 工具调用 | Info | tool_name, args_summary |
| 工具结果 | Debug | tool_name, result_size |
| 权限拒绝 | Warn | tool_name, action |
| 上下文压缩 | Info | before_tokens, after_tokens |
| 错误 | Error | error_type, message |

### 21.4 OpenTelemetry（未来增强）

框架通过 Hook 系统的 `AfterLLM` 和 `AfterTool` 可以无侵入地集成 OpenTelemetry：
- 宿主应用注册一个 OTel Hook，自动采集 span
- 不需要框架内置 OTel 依赖

---

## 22. MCP/LSP 工具的权限集成

> 动态注册的工具同样经过 PermissionHook。

### 22.1 统一权限模型

所有工具（内置、MCP、LSP）在 `ToolRegistry.Settle()` 执行前，都经过 `hook.Chain.RunBeforeTool()`：

```
ToolRegistry.Settle(call)
    │
    ├── hook.Chain.RunBeforeTool(call)  ← 权限检查
    │   └── PermissionHook 匹配规则
    │       ├── "tool:bash" → allow
    │       ├── "tool:mcp_server_search" → ask
    │       └── "tool:write:/etc/**" → deny
    │
    └── tool.Execute(ctx, args)
```

### 22.2 权限规则命名约定

| 工具来源 | 规则格式 | 示例 |
|---------|---------|------|
| 内置工具 | `tool:{name}` | `tool:bash`, `tool:write` |
| 内置 + 资源 | `tool:{name}:{path}` | `tool:write:/etc/**` |
| MCP 工具 | `tool:mcp_{server}_{name}` | `tool:mcp_github_create_issue` |
| LSP 工具 | `tool:lsp_{operation}` | `tool:lsp_definition`, `tool:lsp_references` |

### 22.3 默认权限

```go
// 默认权限规则
var DefaultRules = []hook.Rule{
    // 只读工具默认允许
    {Action: "tool:read", Effect: hook.Allow},
    {Action: "tool:grep", Effect: hook.Allow},
    {Action: "tool:glob", Effect: hook.Allow},
    {Action: "tool:lsp_*", Effect: hook.Allow},
    
    // 写入工具默认需要确认
    {Action: "tool:write", Effect: hook.Ask},
    {Action: "tool:edit", Effect: hook.Ask},
    {Action: "tool:bash", Effect: hook.Ask},
    
    // MCP 工具默认需要确认
    {Action: "tool:mcp_*", Effect: hook.Ask},
}
```

---

## 23. 错误处理策略

> 统一的错误分类、重试和恢复机制。

### 17.1 错误分类

```go
// pkg/llm/error.go 中定义
type ErrorType string

const (
    // 可重试错误
    ErrRateLimit     ErrorType = "rate_limit"      // 429
    ErrOverloaded    ErrorType = "overloaded"       // 529/503
    ErrNetwork       ErrorType = "network"          // 连接超时/DNS
    
    // 需恢复的错误
    ErrContextOverflow ErrorType = "context_overflow" // 上下文超长
    ErrAuthExpired     ErrorType = "auth_expired"     // 401, 可刷新 token
    
    // 不可恢复错误
    ErrAuth        ErrorType = "auth"            // 401, 无法恢复
    ErrModelNotFound ErrorType = "model_not_found" // 404
    ErrInvalidRequest ErrorType = "invalid_request" // 400
    ErrInternal    ErrorType = "internal"         // 500
)
```

### 17.2 重试策略

```go
// pkg/agent/retry.go
package agent

// RetryConfig 重试配置
type RetryConfig struct {
    MaxRetries    int           // 最大重试次数，默认 3
    InitialDelay  time.Duration // 初始延迟，默认 1s
    MaxDelay      time.Duration // 最大延迟，默认 30s
    BackoffFactor float64       // 退避因子，默认 2.0
}

// shouldRetry 判断错误是否可重试
func shouldRetry(err error) bool {
    var llmErr *llm.Error
    if errors.As(err, &llmErr) {
        switch llmErr.Type {
        case llm.ErrRateLimit, llm.ErrOverloaded, llm.ErrNetwork:
            return true
        }
    }
    return false
}

// retryWithBackoff 指数退避重试
func retryWithBackoff(ctx context.Context, cfg RetryConfig, fn func() error) error
```

### 17.3 各层错误处理职责

| 层 | 职责 |
|---|------|
| **Provider** | HTTP 状态码 → `llm.ErrorType` 映射 |
| **Agent Loop** | 可重试错误 → 指数退避重试；ContextOverflow → 触发压缩后重试 |
| **Tool** | 工具执行 panic 恢复 → 返回 error；单个工具失败不影响其他工具 |
| **Session** | 事件写入失败 → 重试 1 次后返回 error（不丢数据） |
| **宿主应用** | 最终错误处理和用户提示 |

### 17.4 不可恢复错误处理

```
不可恢复错误流程:
1. Agent Loop 记录错误到 session (EventTurnFailed)
2. 调用 Callback.OnError(err)
3. 返回 error 给宿主应用
4. Agent 状态重置为 idle，可接收下一个消息
```

---

## 24. 优雅关闭流程

> Agent 关闭时的资源清理顺序。

### 18.1 关闭顺序

```
AgentLoop.Close()
    │
    ├── 1. 设置关闭标记 (atomic.Bool)
    │
    ├── 2. 停止接收新消息
    │   └── HandleMessage 返回 ErrAgentClosed
    │
    ├── 3. 等待活跃 turn 完成
    │   └── 给当前 turn 最多 30s 完成
    │   └── 超时后强制取消 (context cancel)
    │
    ├── 4. 关闭 steering/pending 通道
    │
    ├── 5. 逆序关闭 Plugin
    │   └── for i := len(plugins)-1; i >= 0; i-- { plugins[i].Shutdown(ctx) }
    │
    ├── 6. 关闭 Provider（释放 HTTP 连接池）
    │
    ├── 7. 关闭 MCP 连接
    │
    ├── 8. 关闭 LSP 客户端
    │
    ├── 9. Flush session（确保所有事件持久化）
    │
    └── 10. 关闭 PubSub Broker
```

### 18.2 Close 接口

```go
// AgentLoop.Close 实现
func (a *AgentLoop) Close() error {
    // 幂等：多次调用安全
    if !a.closed.CompareAndSwap(false, true) {
        return nil
    }
    
    // 1-2: 停止接收
    // 3: 等待活跃 turn
    done := make(chan struct{})
    go func() {
        a.activeWg.Wait()
        close(done)
    }()
    select {
    case <-done:
    case <-time.After(30 * time.Second):
        a.cancel() // 强制取消
        <-done
    }
    
    // 4-10: 逆序关闭资源
    // ...
    
    return nil
}
```

### 18.3 Context 传播

```
AgentLoop 持有 root context
    │
    ├── 所有 turn 使用 child context（可被 cancel 中断）
    ├── Plugin Init 使用独立 context
    ├── Tool Execute 使用 turn context（继承 turn 取消）
    └── Provider Stream 使用 turn context
```

Close() 取消 root context → 级联取消所有活跃操作。

---

## 25. 迁移策略

从 openwork 到 basework 的迁移路径（更新版）：

1. **Phase 1**：新建 `pkg/llm/` 统一类型（无依赖，可并行）
2. **Phase 2**：新建 `pkg/hook/` PubSub + Hook（依赖 pkg/llm）
3. **Phase 3**：新建 `pkg/session/` 事件溯源（依赖 pkg/llm）
4. **Phase 4**：新建 `pkg/tool/` 精简版（依赖 pkg/llm）
5. **Phase 5**：新建 `pkg/agent/` 核心循环 + streaming + steering + compaction（从 openwork 搬 + 精简）
6. **Phase 6**：新建 `pkg/provider/` 简化工厂
7. **Phase 7**：新建 `pkg/lsp/` LSP 集成
8. **Phase 8**：新建 `pkg/mcp/` MCP 协议支持
9. **Phase 9**：新建 `pkg/memory/` 记忆系统
10. **Phase 10**：新建 `pkg/config/` + `pkg/skill/`
11. **Phase 11**：新建 `cmd/basework/` CLI 参考实现
12. **Phase 12**：集成测试 + 文档
13. **Phase 13-25**：`internal/` 层新模块（见 §26）

---

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
- **OpenTelemetry 可选**：通过 build tag `otel` 启用 OTel 追踪（Hook 模式实现）

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
