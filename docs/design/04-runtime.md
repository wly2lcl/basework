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

