> 历史归档（2026-09-12）：保留迁移前正文与历史判断，链接已迁移。状态、代码片段、数量和待办可能过时；不得据此领取任务或判定完成。当前入口见 [文档中心](../../README.md)。

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
- `internal/tui/` — 终端产品 TUI（默认随 CLI 构建）
- 宿主应用可以完全不用这一层，直接使用 `pkg/agent`

---

