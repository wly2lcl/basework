# Basework 任务清单

> 分阶段实施计划，每个任务包含详情、依赖、验收标准

---

## 依赖关系总览

```
Phase 1: pkg/llm/          ← 无依赖，最先做
    │
    ├──→ Phase 2: pkg/hook/      ← 依赖 pkg/llm
    ├──→ Phase 3: pkg/session/   ← 依赖 pkg/llm
    └──→ Phase 4: pkg/tool/      ← 依赖 pkg/llm
              │
              ↓
         Phase 5: pkg/agent/     ← 依赖 llm + hook + session + tool
              │
              ├──→ Phase 6: pkg/provider/     ← 依赖 pkg/llm（可与 Phase 5 并行）
              ├──→ Phase 7: pkg/lsp/          ← 依赖 pkg/llm + pkg/tool（可并行）
              └──→ Phase 8: pkg/mcp/          ← 依赖 pkg/llm + pkg/tool（可并行）
                        │
                        ↓
                   Phase 9: pkg/memory/       ← 依赖 pkg/agent（build tag: memory）
                        │
                        ↓
                   Phase 10: pkg/config/ + pkg/skill/  ← 依赖 pkg/agent
                        │
                        ↓
                   Phase 11: cmd/basework/    ← 依赖所有 pkg
                        │
                        ↓
                   Phase 12: 集成测试 + 文档
```

Phase 2/3/4 可并行。Phase 6/7/8 可并行（均依赖 Phase 1-4）。

---

## Phase 1: 统一类型系统 (`pkg/llm/`)

**目标**：建立整个框架的类型基础，消除多套类型的转换开销。

**预计行数**：~400 行

### Task 1.1: 定义核心类型 (`pkg/llm/types.go`)

**文件**：`pkg/llm/types.go`

**内容**：
- `Role` 类型（system/user/assistant/tool）
- `ContentType` 类型（text/image）
- `ContentPart` 结构体（Type + Text + ImageURL）
- `ChatMessage` 结构体（Role + Content + Name + ToolCalls + ToolCallID）
- `ToolCall` 结构体（ID + Name + ArgsJSON）
- `ToolCallDelta` 结构体（流式工具调用增量）
- `ToolResult` 结构体（ToolCallID + Content + IsError）
- `ToolDefinition` 结构体（Name + Description + Parameters）

**验收标准**：
- [ ] 所有类型可 JSON 序列化/反序列化
- [ ] `ChatMessage` 支持纯文本和多模态两种模式
- [ ] `ToolCall.ArgsJSON` 保留原始 JSON（避免中间转换损失）
- [ ] 有基本单元测试覆盖序列化往返

### Task 1.2: 定义 Model 接口 (`pkg/llm/model.go`)

**文件**：`pkg/llm/model.go`

**内容**：
- `Model` 接口：`ID()`, `Generate()`, `Stream()`, `Supports()`
- `Request` 结构体（Messages + Tools + MaxTokens + Temperature + Stop + Extra）
- `Response` 结构体（Message + Usage + FinishReason）
- `StreamEvent` 结构体（Type + Delta + ToolCall + Usage + Error）
- `StreamEventType` 常量
- `Usage` 结构体（PromptTokens + CompletionTokens + TotalTokens）
- `Capability` 类型（tools/vision/streaming/json_mode）
- `Usage.Add()` 方法（累加用量）

**验收标准**：
- [ ] `Model` 接口足够简单（4 个方法）
- [ ] `Stream` 返回 `<-chan StreamEvent`（channel 模式，非回调）
- [ ] `Request.Extra` 支持 provider 特定选项透传
- [ ] `Capability` 可扩展（string 类型而非 enum）

### Task 1.3: 定义错误类型 (`pkg/llm/error.go`)

**文件**：`pkg/llm/error.go`

**内容**：
- `Error` 结构体（Type + Message + StatusCode + ProviderError）
- `ErrorType` 常量：`RateLimit`, `ContextOverflow`, `Auth`, `Network`, `ModelNotFound`, `Internal`
- `IsContextOverflow(err) bool` 辅助函数（agent 需要检测上下文溢出）
- `IsRateLimit(err) bool` 辅助函数

**验收标准**：
- [ ] 错误类型可被 `errors.Is`/`errors.As` 匹配
- [ ] `ContextOverflow` 可被 agent loop 检测并触发压缩

---

## Phase 2: Hook + PubSub (`pkg/hook/`)

**目标**：建立统一的事件总线和生命周期钩子系统。

**预计行数**：~300 行

**依赖**：`pkg/llm`

### Task 2.1: 实现 PubSub Broker (`pkg/hook/pubsub.go`)

**文件**：`pkg/hook/pubsub.go`

**内容**：
- `Broker[T any]` 泛型结构体
- `Subscribe() <-chan T` — 返回专属 channel
- `Unsubscribe(<-chan T)` — 取消订阅
- `Publish(T)` — 发布事件到所有订阅者
- `Close()` — 关闭所有 channel
- 内部使用 `sync.RWMutex` 保护订阅者列表
- 非阻塞发布（满 channel 跳过 + 日志警告）

**参考**：crush 的 `internal/pubsub/pubsub.go`

**验收标准**：
- [ ] 并发安全：多 goroutine 同时 Subscribe/Publish 不 panic
- [ ] 非阻塞发布：慢消费者不阻塞发布者
- [ ] Unsubscribe 后不再收到事件
- [ ] Close 后所有 channel 关闭
- [ ] 有并发压力测试

### Task 2.2: 定义 Hook 接口 (`pkg/hook/hook.go`)

**文件**：`pkg/hook/hook.go`

**内容**：
- `Hook` 接口：`BeforeLLM`, `AfterLLM`, `BeforeTool`, `AfterTool`
- `NopHook` 空实现（可嵌入）
- `FuncHook` 函数式实现（可选字段）
- `Chain` 结构体：管理多个 Hook 的链式调用
- `Chain.Add(hooks ...Hook)`
- `Chain.RunBeforeLLM(messages) (messages, error)` — 依次调用
- `Chain.RunBeforeTool(call) (call, error)` — 依次调用，任一拒绝即终止
- `Chain.RunAfterLLM(resp, err)` — 依次调用（只读）
- `Chain.RunAfterTool(call, result, err)` — 依次调用（只读）

**验收标准**：
- [ ] `NopHook` 所有方法为空操作
- [ ] `Chain` 中 `BeforeTool` 返回 error 时后续 Hook 不执行
- [ ] `Chain` 中 `AfterTool` 即使有 error 也继续调用所有 Hook
- [ ] `FuncHook` 未设置的字段自动 fallback 到 nop

### Task 2.3: 实现 PermissionHook (`pkg/hook/permission.go`)

**文件**：`pkg/hook/permission.go`

**内容**：
- `Rule` 结构体（Action + Resource + Effect）
- `Effect` 类型：Allow, Deny, Ask
- `PermissionHook` 结构体（rules + onAsk 回调）
- `BeforeTool` 实现：匹配规则 → allow/deny/ask
- `NewPermissionHook(rules, onAsk)` 构造函数
- `WildcardMatch(pattern, s string) bool` 通配符匹配辅助

**参考**：opencode 的 `packages/core/src/permission.ts`（三值评估模型）

**验收标准**：
- [ ] 规则匹配支持通配符（`tool:*`, `tool:write:/etc/**`）
- [ ] 最后匹配的规则生效（`findLast` 语义）
- [ ] `Ask` 效果调用 `onAsk` 回调并阻塞等待结果
- [ ] `Deny` 返回包含 action + resource 信息的 error

---

## Phase 3: 事件溯源 Session (`pkg/session/`)

**目标**：实现基于事件溯源的会话存储，支持可重放、可导出的会话历史。

**预计行数**：~500 行

**依赖**：`pkg/llm`

### Task 3.1: 定义事件类型 (`pkg/session/event.go`)

**文件**：`pkg/session/event.go`

**内容**：
- `EventType` 字符串类型 + 常量（参考 DESIGN.md 3.3.1）
- `Event` 结构体（ID + SessionID + Type + Data + Seq + CreatedAt）
- 事件数据子类型：
  - `PromptedData`（Content string）
  - `TextDeltaData`（Delta string）
  - `ToolCalledData`（ToolCall llm.ToolCall）
  - `ToolSuccessData`（ToolCallID + Content）
  - `ToolFailedData`（ToolCallID + Error）
  - `TurnStartedData`（Step int）
  - `TurnEndedData`（Usage llm.Usage）
  - `CompactedData`（Summary string, TruncatedSeq int64）
- `Event.Encode() ([]byte, error)` — JSON 序列化
- `DecodeEvent(data []byte) (*Event, error)` — JSON 反序列化
- `EventFilter` 结构体（SessionID + Types + AfterSeq + Limit）

**参考**：opencode 的 `packages/core/src/session/event.ts`

**验收标准**：
- [ ] 所有事件数据子类型可 JSON 序列化往返
- [ ] `EventType` 常量覆盖完整生命周期
- [ ] `EventFilter` 支持按类型、序列号、数量过滤

### Task 3.2: 定义 Store 接口 + 内存后端 (`pkg/session/store.go`, `pkg/session/memory.go`)

**文件**：`pkg/session/store.go`, `pkg/session/memory.go`

**内容**：
- `Store` 接口（参考 DESIGN.md 3.3.2）
- `Info` 结构体（ID + Title + MessageCount + Usage + CreatedAt + UpdatedAt）
- `CreateOpts` 结构体（Title + Metadata）
- `ListFilter` 结构体（Limit + Offset + AfterTime）
- `MemoryStore` 实现：
  - 内部 `map[string]*sessionData`（每个 session 一个事件切片）
  - `sync.RWMutex` 保护
  - 实现所有 `Store` 方法

**验收标准**：
- [ ] `MemoryStore` 并发安全
- [ ] `AppendEvent` 自动递增 Seq
- [ ] `Messages()` 正确投影事件为 ChatMessage 序列
- [ ] `Events(filter)` 支持所有过滤条件

### Task 3.3: JSONL 持久化后端 (`pkg/session/jsonl.go`)

**文件**：`pkg/session/jsonl.go`

**内容**：
- `JSONLStore` 结构体
- 每个 session 一个 `.jsonl` 文件
- `AppendEvent` → 追加写入一行 JSON
- `Events` → 逐行读取 + 过滤
- 原子写入（写 tmp 文件 → rename）
- 文件路径：`{base_dir}/{session_id}.jsonl`

**验收标准**：
- [ ] 崩溃后不丢失已追加的事件
- [ ] 大 session（10000+ 事件）查询性能可接受
- [ ] 文件路径安全（session ID 清理）

### Task 3.4: 事件投影 (`pkg/session/projection.go`)

**文件**：`pkg/session/projection.go`

**内容**：
- `ProjectMessages(events []Event) []llm.ChatMessage`
- 处理规则：
  - `Prompted` → user message
  - `TextDelta` → 累积到当前 assistant message
  - `TextEnded` → 完成当前 assistant message
  - `ToolCalled` → assistant message 的 ToolCall
  - `ToolSuccess/Failed` → tool message
  - `Compacted` → 替换之前的消息为压缩摘要
- 投影缓存（避免每次重算）

**验收标准**：
- [ ] 空事件列表返回空消息
- [ ] 多轮对话正确投影
- [ ] Compacted 事件正确截断历史
- [ ] 有完整的投影测试（覆盖所有事件类型组合）

---

## Phase 4: 工具系统 (`pkg/tool/`)

**目标**：精简工具接口和注册表，去除 facade 层和冗余的 toolloop。

**预计行数**：~400 行（不含内置工具实现）

**依赖**：`pkg/llm`

### Task 4.1: 定义 Tool 接口 + Registry (`pkg/tool/tool.go`, `pkg/tool/registry.go`)

**文件**：`pkg/tool/tool.go`, `pkg/tool/registry.go`

**内容**：
- `Tool` 接口（Name + Description + Parameters + Execute）
- `Result` 结构体（Content + IsError + Images）
- `Registry` 结构体：
  - `Register(tools ...Tool)`
  - `Unregister(names ...string)`
  - `Clone() *Registry`
  - `Materialize() []llm.ToolDefinition`
  - `Settle(ctx, call llm.ToolCall) (*Result, error)`
  - `Version() int64`（变更追踪）
  - `Disable(name string)` / `Enable(name string)`
- `TTL` 支持：工具可设置过期时间，过期后从 Materialize 结果隐藏

**参考**：openwork 的 `pkg/tools/registry.go`（保留核心，去掉 facade）

**验收标准**：
- [ ] `Clone()` 深拷贝，修改副本不影响原注册表
- [ ] `Materialize()` 只返回已启用且未过期的工具
- [ ] `Settle()` panic 恢复，返回 error 而非 crash
- [ ] `Version()` 在每次 Register/Unregister/Disable/Enable 后递增

### Task 4.2: 内置工具实现 (`pkg/tool/builtin/`)

**文件**：`pkg/tool/builtin/{bash,read,write,edit,grep,glob}.go`

**内容**：每个工具实现 `Tool` 接口：

| 工具 | 核心逻辑 | 预计行数 |
|------|----------|---------|
| `bash` | 执行 shell 命令，捕获 stdout/stderr | ~80 |
| `read` | 读取文件（支持 offset/limit） | ~60 |
| `write` | 写入文件（原子写入） | ~50 |
| `edit` | 精确字符串替换 | ~80 |
| `grep` | 正则搜索文件内容 | ~70 |
| `glob` | 文件模式匹配 | ~50 |

**参考**：openwork 的内置工具实现（简化版）

**验收标准**：
- [ ] 每个工具独立可测试
- [ ] `bash` 支持超时控制
- [ ] `edit` 处理多个匹配的情况（报错而非静默选择）
- [ ] `write` 自动创建父目录

---

## Phase 5: Agent 核心 (`pkg/agent/`)

**目标**：从 openwork 搬迁核心循环并精简，整合 TurnD/Instance 接口和四阶段管道。

**预计行数**：~800 行

**依赖**：`pkg/llm`, `pkg/tool`, `pkg/hook`, `pkg/session`

### Task 5.1: 定义 Agent 接口 + 选项 (`pkg/agent/agent.go`, `pkg/agent/option.go`)

**文件**：`pkg/agent/agent.go`, `pkg/agent/option.go`

**内容**：
- `Agent` 接口：`HandleMessage`, `HandleMessages`, `Close`
- `Response` 结构体
- `Option` 函数式选项类型
- 所有 `With*` 选项函数

**验收标准**：
- [ ] `New(opts ...Option) (Agent, error)` 构造函数
- [ ] 选项有合理默认值（MaxSteps=25, Session=MemoryStore）
- [ ] 缺少 Model 选项时返回明确错误

### Task 5.2: 实现 TurnD / Instance 接口 (`pkg/agent/turn.go`)

**文件**：`pkg/agent/turn.go`

**内容**：
- `TurnD` 接口（精简版 ~12 方法）
- `Instance` 接口（~5 方法）
- `agentTurnD` 内部实现（包装 agentLoop 字段）
- `agentInstance` 内部实现

**参考**：openwork 的 `internal/agentturn/coord.go`（保留接口设计，精简方法数）

**验收标准**：
- [ ] `TurnD` 与 `pkg/agent` 零循环依赖
- [ ] 接口方法数 < 15（openwork 原设计 24 个，精简到必要集合）

### Task 5.3: 实现四阶段 Pipeline (`pkg/agent/pipeline.go`)

**文件**：`pkg/agent/pipeline.go`

**内容**：
- `Pipeline` 结构体
- `setupTurn(ctx)` → 组装 messages + tools
- `callLLM(ctx, messages, tools)` → 流式调用 + hook 集成
- `executeTools(ctx, toolCalls)` → 权限检查 + 执行 + hook
- `finalize(ctx, response, results)` → 持久化事件 + 判断续传

**关键流程**：
```
setupTurn:
  1. session.Messages() → 获取历史
  2. prepend system prompt
  3. drain steering messages → 追加到末尾
  4. registry.Materialize() → 工具定义
  5. 如果 shouldCompact → 调用压缩

callLLM:
  1. hook.Chain.RunBeforeLLM(messages)
  2. model.Stream(ctx, request)
  3. 消费 stream → 发布事件到 session
  4. hook.Chain.RunAfterLLM(response, err)
  5. 如果 ContextOverflow → 触发压缩 + 重试

executeTools:
  1. 并行或串行遍历 tool calls
  2. hook.Chain.RunBeforeTool(call) → 可修改/拒绝
  3. registry.Settle(ctx, call)
  4. hook.Chain.RunAfterTool(call, result, err)
  5. 追加事件到 session

finalize:
  1. 持久化 assistant message 事件
  2. 更新 usage
  3. 返回 TurnResult (hasToolCalls, message, usage)
```

**验收标准**：
- [ ] Pipeline 可独立测试（mock Model + Tool + Session）
- [ ] `callLLM` 正确处理流式事件
- [ ] `executeTools` 中单个工具失败不影响其他工具
- [ ] `ContextOverflow` 检测 → 压缩 → 重试（最多 1 次）

### Task 5.4: 实现 AgentLoop (`pkg/agent/loop.go`)

**文件**：`pkg/agent/loop.go`

**内容**：
- `AgentLoop` 结构体（实现 `Agent` 接口）
- `HandleMessage` 主循环（最多 MaxSteps 轮）
- `Close` 优雅关闭（drain active turns）
- Plugin 初始化/关闭
- Observer 适配

**验收标准**：
- [ ] 基础对话（无工具）正常工作
- [ ] 工具调用循环正常终止
- [ ] MaxSteps 超限返回明确错误
- [ ] `Close()` 等待活跃 turn 完成后返回
- [ ] Plugin 按顺序 Init，逆序 Shutdown

### Task 5.5: 流式回调 (`pkg/agent/callback.go`)

**文件**：`pkg/agent/callback.go`

**内容**：
- `Callback` 接口：`OnTextDelta`, `OnToolCallStart`, `OnToolCallEnd`, `OnThinkingDelta`, `OnTurnEnd`, `OnError`
- `NopCallback` 空实现
- `CallbackFuncs` 函数式便捷构造（可选字段）
- `WithCallback(cb Callback) Option`
- `WithCallbackFunc(funcs CallbackFuncs) Option`

**验收标准**：
- [ ] `NopCallback` 所有方法为空操作
- [ ] `CallbackFuncs` 未设置字段自动 fallback 到 nop
- [ ] Pipeline 中流式 delta 同时回调消费者和追加到 session

### Task 5.6: Steering 机制 (`pkg/agent/steering.go`)

**文件**：`pkg/agent/steering.go`

**内容**：
- `DeliveryMode` 类型：`Steer`（立即打断）, `Queue`（排队等待）
- `InjectMessage(ctx, content, mode) error`
- 内部 `steeringCh` + `pendingCh` 通道
- Cancel 序列号机制（参考 crush 的 `acceptSeq`）
- Pipeline `setupTurn` 阶段检查 `steeringCh`
- Pipeline `executeTools` 阶段检查 interrupt 标记

**参考**：opencode 的 steer/queue 交付语义 + crush 的 cancel 序列号

**验收标准**：
- [ ] `Steer` 模式在当前工具执行完后中断
- [ ] `Queue` 模式等待当前 turn 完成后注入
- [ ] Cancel 序列号精确控制，不误取消后续请求
- [ ] 并发注入安全

### Task 5.7: 上下文压缩 (`pkg/agent/compact.go`)

**文件**：`pkg/agent/compact.go`

**内容**：
- `CompactConfig` 结构体（TriggerThreshold, KeepRecent, Strategy）
- `CompactStrategy` 类型：`StrategySummary`, `StrategyTruncate`
- `ShouldCompact(usage, maxTokens) bool` — 检查是否触发压缩
- `CompactSummary(ctx, model, messages) (string, error)` — LLM 生成摘要
- Pipeline `setupTurn` 集成：检查 → 压缩 → 追加 EventCompacted

**验收标准**：
- [ ] Token 占比 > 80%（默认阈值）时触发压缩
- [ ] 保留最近 KeepRecent 条消息不压缩
- [ ] 摘要策略正确调用 LLM 生成摘要
- [ ] 压缩事件正确追加到 session

### Task 5.8: 精简版子代理 (`pkg/agent/subturn.go`)

**文件**：`pkg/agent/subturn.go`

**内容**：
- `SubTurnConfig` 精简版（~8 字段）：
  - `Prompt string`
  - `Tools []tool.Tool`（可选，覆盖父 agent 工具）
  - `Model llm.Model`（可选，覆盖父 agent 模型）
  - `MaxSteps int`
  - `Async bool`（异步执行）
  - `Budget *int64`（token 预算共享）
- `RunSubTurn(ctx, config) (*Response, error)`
- 内部创建临时 AgentLoop 执行

**做减法**：从 openwork 的 22 字段 SubTurnConfig 精简到 8 字段，去掉 RestrictionConfig。

**验收标准**：
- [ ] 子代理可使用不同模型/工具
- [ ] 异步模式返回 channel 而非阻塞
- [ ] Token 预算共享正确扣减

### Task 5.9: Plugin 接口 (`pkg/agent/plugin.go`)

**文件**：`pkg/agent/plugin.go`

**内容**：
- `Plugin` 接口：`Name()`, `Init(agent Agent) error`, `Shutdown(ctx) error`
- 保留 openwork 的简洁设计（~50 行）

**验收标准**：
- [ ] Plugin 可注册额外工具
- [ ] Plugin Init 失败时 agent 创建失败

### Task 5.10: Observer 适配层 (`pkg/agent/observer.go`)

**文件**：`pkg/agent/observer.go`

**内容**：
- `Observer` 接口（向后兼容）：`OnToolCall`, `OnLLMRequest`, `OnError`, `OnTurnStart`, `OnTurnEnd`
- `observerHook` 内部结构体（将 Observer 适配为 Hook）
- `WithObserver(o Observer) Option`

**验收标准**：
- [ ] 旧 Observer 代码无需修改即可工作
- [ ] Observer 内部通过 Hook 机制实现

---

## Phase 6: Provider 工厂 (`pkg/provider/`)

**目标**：简化 provider 创建，40+ case 折叠为 4 分支。

**预计行数**：~400 行

**依赖**：`pkg/llm`

**可与 Phase 5 并行**

### Task 6.1: Provider 工厂 (`pkg/provider/factory.go`)

**文件**：`pkg/provider/factory.go`

**内容**：
- `Config` 结构体（Type + APIKey + BaseURL + ModelID + Options）
- `Create(Config) (llm.Model, error)` — 4 分支 switch
- `protocolMeta` 映射表（提供商名 → 默认 BaseURL + 是否允许空 API key）

**验收标准**：
- [ ] 未配置 API key 时给出清晰错误
- [ ] 自定义 BaseURL 覆盖默认值
- [ ] 未知 type 默认走 openai-compat

### Task 6.2: OpenAI 实现 (`pkg/provider/openai.go`)

**文件**：`pkg/provider/openai.go`

**内容**：
- `openAIModel` 结构体（实现 `llm.Model`）
- Chat Completions API 调用
- 流式 SSE 解析
- 工具调用解析
- 错误映射（429 → RateLimit, 401 → Auth, etc.）

**验收标准**：
- [ ] 同步和流式调用正确
- [ ] 工具调用（function calling）正确解析
- [ ] ContextOverflow 错误正确映射

### Task 6.3: Anthropic 实现 (`pkg/provider/anthropic.go`)

**文件**：`pkg/provider/anthropic.go`

**内容**：
- Messages API 调用
- Extended Thinking 支持
- 流式 SSE 解析
- 工具调用解析

### Task 6.4: Gemini 实现 (`pkg/provider/gemini.go`)

**文件**：`pkg/provider/gemini.go`

**内容**：
- GenerateContent / StreamGenerateContent
- Function calling
- Thinking 支持

### Task 6.5: OpenAI 兼容实现 (`pkg/provider/compat.go`)

**文件**：`pkg/provider/compat.go`

**内容**：
- 复用 `openai.go` 的大部分逻辑
- 差异处理（部分提供商的 quirks）
- 配置驱动（BaseURL + 可选 header 覆盖）

**验收标准**：
- [ ] DeepSeek / Groq / Together / OpenRouter 等通过配置 BaseURL 即可使用
- [ ] 无需为每个兼容提供商写代码

---

## Phase 7: LSP 集成 (`pkg/lsp/`)

**目标**：代码智能 — 让 agent 像 IDE 一样理解代码。

**预计行数**：~500 行

**依赖**：`pkg/llm`, `pkg/tool`

**可与 Phase 5 并行**

### Task 7.1: LSP 类型 + Manager (`pkg/lsp/manager.go`, `pkg/lsp/types.go`)

**文件**：`pkg/lsp/types.go`, `pkg/lsp/manager.go`

**内容**：
- `Location`, `Range`, `Position`, `Diagnostic`, `SymbolInfo` 类型
- `Manager` 结构体：管理多个 LSP 客户端生命周期
- `Config` / `ServerConfig` 配置类型
- `Start(ctx, workspacePath)` — 懒启动
- `Stop()` — 停止所有客户端
- `Definition`, `References`, `Hover`, `Diagnostics`, `DocumentSymbols`, `WorkspaceSymbols` 方法

**参考**：crush 的 `internal/lsp/`（简化版，只保留 6 种操作）

**验收标准**：
- [ ] Manager 并发安全
- [ ] 未配置的 LSP 返回空结果而非错误
- [ ] 客户端状态机（Unstarted → Starting → Ready/Error）正确

### Task 7.2: LSP Client (`pkg/lsp/client.go`)

**文件**：`pkg/lsp/client.go`

**内容**：
- `Client` 结构体：包装单个 LSP 服务器连接
- 使用 `golang.org/x/tools/gopls` 协议（通过 JSON-RPC）
- 文件打开/关闭/变更通知
- 诊断缓存（版本化，参考 crush 的 `VersionedMap`）

**验收标准**：
- [ ] 支持 gopls, typescript-language-server, pyright
- [ ] 诊断缓存版本化，无变化时跳过重算
- [ ] 服务器崩溃后自动重启

### Task 7.3: LSP 自动检测 + 工具暴露 (`pkg/lsp/auto.go`, `pkg/lsp/tool.go`)

**文件**：`pkg/lsp/auto.go`, `pkg/lsp/tool.go`

**内容**：
- `DefaultServers` 映射表（go→gopls, ts→typescript-language-server, python→pyright）
- `Detect(workspacePath)` — 根据工作区文件自动检测
- `Tools(manager) []tool.Tool` — 将 6 种 LSP 操作暴露为 agent 工具

**验收标准**：
- [ ] Go 项目自动检测 gopls
- [ ] TS/JS 项目自动检测 typescript-language-server
- [ ] LSP 未安装时不注册工具（优雅降级）

---

## Phase 8: MCP 协议支持 (`pkg/mcp/`)

**目标**：支持 Model Context Protocol，允许 agent 使用外部 MCP 服务器提供的工具。

**预计行数**：~400 行

**依赖**：`pkg/llm`, `pkg/tool`

**可与 Phase 5、Phase 7 并行**

### Task 8.1: MCP Manager + Transport (`pkg/mcp/manager.go`, `pkg/mcp/transport.go`)

**文件**：`pkg/mcp/manager.go`, `pkg/mcp/transport.go`

**内容**：
- `Manager` 结构体：管理多个 MCP 服务器连接
- `ServerConnection` 结构体（Name + Tools + Session）
- `Transport` 接口 + `StdioTransport` + `HTTPTransport` 实现
- `Connect(ctx, name, cfg) error` — 连接到 MCP 服务器
- `Disconnect(name) error`
- `Tools() []tool.Tool` — 返回适配后的工具
- `Close() error`
- 使用 `github.com/modelcontextprotocol/go-sdk/mcp`

**参考**：openwork 的 `pkg/mcp/manager.go`（简化版）

**验收标准**：
- [ ] stdio 传输：启动子进程，通过 stdin/stdout 通信
- [ ] http 传输：HTTP + SSE 与远程服务器通信
- [ ] 单个服务器连接失败不影响其他服务器
- [ ] 工具名加 `mcp_{server}_` 前缀

### Task 8.2: MCP 工具适配 (`pkg/mcp/adapter.go`)

**文件**：`pkg/mcp/adapter.go`

**内容**：
- `mcpToolAdapter` 实现 `tool.Tool` 接口
- Name → `mcp_{server}_{tool_name}`
- Execute → 调用 MCP 服务器的 tools/call
- 错误处理 + 超时

**验收标准**：
- [ ] MCP 工具可注册到 ToolRegistry
- [ ] 工具调用正确传递参数和返回结果
- [ ] 服务器断开时工具调用返回明确错误

---

## Phase 9: Memory 系统 (`pkg/memory/`)

**目标**：可选的持久化记忆模块，支持上下文检索和自动知识积累。

**预计行数**：~400 行

**依赖**：`pkg/llm`, `pkg/agent`

**Build Tag**: `memory`

### Task 9.1: Memory 类型 + Store (`pkg/memory/types.go`, `pkg/memory/store.go`)

**文件**：`pkg/memory/types.go`, `pkg/memory/store.go`

**内容**：
- `MemoryLayer` 类型：user, project, local, auto
- `Entry` 结构体（Layer + Content + Tags + CreatedAt）
- `Store` 接口：Write, Read, Search, Clear, List
- `FileStore` 实现（基于 AGENTS.md 文件）

**验收标准**：
- [ ] 四层记忆正确映射到文件路径
- [ ] 文件不存在时自动创建
- [ ] Markdown 格式人可读

### Task 9.2: 短期记忆引擎 (`pkg/memory/engine.go`, `pkg/memory/fts.go`)

**文件**：`pkg/memory/engine.go`, `pkg/memory/fts.go`（build tag: memory）

**内容**：
- `Engine` 结构体：压缩 + 检索 + 组装
- `Compress(ctx, messages)` — 智能压缩对话历史
- `Retrieve(ctx, currentMessages, limit)` — 根据上下文检索相关记忆
- `Assemble(systemPrompt, memories, messages)` — 组装最终上下文
- `FTSIndex` — FTS5 全文搜索（build tag: memory）
- `engine_stub.go` — 无 build tag 时的存根

**验收标准**：
- [ ] 无 `memory` build tag 时 Engine 是空操作
- [ ] 有 `memory` build tag 时 FTS5 搜索正常工作
- [ ] Assemble 正确组装 系统提示 + 记忆 + 对话

---

## Phase 10: 外围层 (`pkg/config/` + `pkg/skill/`)

**目标**：CoW 配置 + Markdown Skill 系统。

**预计行数**：~300 行

**依赖**：`pkg/agent`

### Task 10.1: Config 系统 (`pkg/config/config.go`)

**文件**：`pkg/config/config.go`

**内容**：
- `Config` 结构体（~15 字段，通过约定派生更多）
- `Store` 结构体（CoW 模式）
- `Load(path string) (*Store, error)` — 从 JSON 加载
- `Get() *Config` — 返回不可变快照
- `Mutate(fn func(*Config))` — 原子修改 + 自动保存
- 配置发现：当前目录 → 上级目录 → `~/.config/basework/`

**做减法**：无热重载。配置文件变更后需显式 `Reload()`。

**验收标准**：
- [ ] 并发安全：多 goroutine 读不阻塞，写互斥
- [ ] 配置文件不存在时使用默认值
- [ ] `Mutate` 原子性：修改要么全生效要么全不生效

### Task 10.2: Skill 系统 (`pkg/skill/skill.go`, `pkg/skill/loader.go`)

**文件**：`pkg/skill/skill.go`, `pkg/skill/loader.go`

**内容**：
- `Skill` 结构体（Name + Description + Instructions + FilePath + Builtin + Metadata）
- `Loader` 结构体：
  - `NewLoader(paths ...string) *Loader`
  - `Discover() error` — 扫描目录查找 SKILL.md
  - `Active() []*Skill`
  - `Get(name string) *Skill`
  - `ToPromptXML() string` — 生成 system prompt 注入片段
- SKILL.md 解析（YAML frontmatter + markdown body）
- 同名去重（用户 skill 覆盖 builtin）

**参考**：crush 的 `internal/skills/`（完整借鉴 markdown 标准）

**验收标准**：
- [ ] SKILL.md 格式正确解析
- [ ] 多路径搜索，用户路径优先
- [ ] `ToPromptXML()` 输出格式正确
- [ ] 无效 SKILL.md 有清晰错误信息

---

## Phase 11: CLI 参考实现 (`cmd/basework/`)

**目标**：提供一个最小可用的 CLI 工具，验证框架的可嵌入性。

**预计行数**：~300 行

**依赖**：所有 pkg

### Task 11.1: CLI 入口 (`cmd/basework/main.go`)

**文件**：`cmd/basework/main.go`

**内容**：
- `main()` — cobra 命令行框架
- `agent` 子命令 — 启动交互式 agent 会话
- `model list` 子命令 — 列出可用模型
- `session list/clear` 子命令 — 会话管理
- 简单 REPL 循环（readline + 流式输出）

**验收标准**：
- [ ] `basework agent -m "hello"` 可直接返回响应
- [ ] `basework agent` 进入交互模式
- [ ] 流式输出正确显示
- [ ] Ctrl+C 优雅退出

### Task 11.2: 配置初始化 (`cmd/basework/onboard.go`)

**文件**：`cmd/basework/onboard.go`

**内容**：
- 交互式配置 API key
- 检测已有环境变量
- 写入配置文件

**验收标准**：
- [ ] 自动检测 `ANTHROPIC_API_KEY`, `OPENAI_API_KEY` 等环境变量
- [ ] 配置文件写入正确路径

---

## Phase 12: 集成测试 + 文档

**目标**：端到端验证 + 项目文档。

### Task 12.1: 集成测试

**内容**：
- 基础对话测试（mock provider）
- 工具调用循环测试
- Session 事件溯源 + 投影测试
- 多 provider 集成测试（需要 API key）

### Task 12.2: 项目文档

**内容**：
- `README.md` — 项目介绍 + 快速开始 + 嵌入示例
- `docs/guides/embedder-guide.md` — 嵌入指南
- `docs/guides/configuration.md` — 配置参考
- `docs/guides/extending.md` — Hook/Plugin/Skill 扩展指南

---

## 任务统计

| Phase | 预计行数 | 任务数 | 依赖 |
|-------|---------|--------|------|
| Phase 1: pkg/llm/ | ~400 | 3 | 无 |
| Phase 2: pkg/hook/ | ~300 | 3 | Phase 1 |
| Phase 3: pkg/session/ | ~500 | 4 | Phase 1 |
| Phase 4: pkg/tool/ | ~400 + builtin | 2 | Phase 1 |
| Phase 5: pkg/agent/ | ~800 | 10 | Phase 1-4 |
| Phase 6: pkg/provider/ | ~400 | 5 | Phase 1 |
| Phase 7: pkg/lsp/ | ~300 | 3 | Phase 1, 4 |
| Phase 8: pkg/mcp/ | ~300 | 2 | Phase 1, 4 |
| Phase 9: pkg/memory/ | ~300 | 2 | Phase 5 |
| Phase 10: config + skill | ~300 | 2 | Phase 5 |
| Phase 11: cmd/ | ~300 | 2 | Phase 5-10 |
| Phase 12: 测试 + 文档 | ~500 | 2 | Phase 11 |
| **合计** | **~4800** | **42** | |

**预计核心代码量**：~4800 行（对比 openwork 当前 ~15000+ 行，减 ~68%）

---

## 执行顺序建议

```
Week 1: Phase 1 (llm) + Phase 2 (hook) + Phase 3 (session) — 并行
Week 2: Phase 4 (tool) + Phase 6 (provider) — 并行
Week 3: Phase 5 (agent 核心 + streaming + steering + compaction) — 串行
Week 4: Phase 7 (lsp) + Phase 8 (mcp) — 并行
Week 5: Phase 9 (memory) + Phase 10 (config+skill) — 并行
Week 6: Phase 11 (CLI) + Phase 12 (测试+文档)
```
