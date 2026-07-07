# Basework 任务清单

> 从可嵌入框架到独立终端产品的完整实施计划
> 
> Phase 1-12：核心框架（已完成 ✅）| Phase 13-25：终端产品化（已完成 ✅）| Phase 26-27：稳定性+CI/CD（已完成 ✅）| Phase 28：安全加固+性能基线（已完成 ✅）| Phase 29-30：TUI 增强 + 模板系统 + 多模态 + 插件生态（已完成 ✅）

---

## Phase 35: 主路径 Wiring 修复 + P0 优化（2026-07-07）

**背景**：审计发现多个模块已实现但未接入 CLI/TUI 主路径，导致终端产品能力与文档承诺不一致。第一轮目标是先恢复核心可用性：配置默认值可靠、`basework agent` 有基础编程工具、`basework tui` 能真实对话。

### Task 35.1: 配置默认值合并 ✅

**文件**：`pkg/config/config.go`, `pkg/config/config_test.go`

**内容**：
- `Load` 使用 `defaultConfig()` 初始化，再反序列化用户 JSON 覆盖
- `Reload` 使用同样语义，避免热重载丢失默认值
- 保留 `Validate()` 校验
- 增加局部配置回归测试

**验收标准**：
- [x] 只配置 `provider` 时，`model/max_iterations/tools/security` 等默认值仍保留
- [x] 嵌套配置只覆盖指定字段，不清空整个默认结构

### Task 35.2: 公共 Agent Runtime 创建器 ✅

**文件**：`cmd/basework/runtime.go`

**内容**：
- 统一 provider 创建、session 创建、基础工具注册
- 初始化工具超时配置
- 初始化敏感路径保护
- 根据配置映射 Bash 黑名单和 permission mode
- 为后续增强工具/MCP/LSP 接入预留集中入口

**验收标准**：
- [x] `cmd/basework agent` 和 `cmd/basework tui` 不再各自复制 agent 创建逻辑
- [x] 基础工具默认注册到 agent registry
- [x] 默认启用敏感路径 strict 保护

### Task 35.3: CLI Agent 基础工具接入 ✅

**文件**：`cmd/basework/agent.go`

**内容**：
- 改为复用 `newRuntimeAgent`
- `basework agent` 默认拥有 `bash/read/write/edit/grep/glob`
- verbose 输出 provider/model/tools 数量

**验收标准**：
- [x] `Agent.Tools()` 返回基础工具列表
- [x] Bash 默认仍受内置黑名单保护
- [x] 文件工具走敏感路径保护

### Task 35.4: TUI 接入 Agent ✅

**文件**：`cmd/basework/tui.go`, `internal/tui/app.go`, `internal/tui/app_test.go`

**内容**：
- TUI App 支持 `InputHandler`
- `UserInputMsg` 调用 agent handler
- `AgentResponseMsg` / `ErrorMsg` 更新消息区并停止 streaming 状态
- `cmd/basework tui` 设置 handler 调用 `Agent.HandleMessage`

**验收标准**：
- [x] TUI 输入不再只停留在 UI 内部
- [x] agent 回复会加入助手消息列表
- [x] 有单元测试覆盖 handler 调用链

### Task 35.5: 第二轮增强工具接入 ✅

**文件**：`cmd/basework/runtime.go`, `internal/tools/websearch.go`, `internal/tools/websearch_test.go`

**内容**：
- `runtimeTools` 默认注册 `web_fetch/web_search/todowrite/apply_patch/question`
- `web_search` 支持后端注入，单元测试不再访问真实 Tavily/Exa API
- `web_search` 对 nil config 安全返回配置错误，不再 panic
- 保留 Tavily/Exa 默认后端行为与参数上限处理

**验收标准**：
- [x] CLI/TUI 共享 runtime 默认具备增强工具
- [x] `web_search` nil config 不 panic
- [x] `web_search` 测试不依赖外网或真实 API key
- [x] `num_results` 默认值和最大值有回归测试

### Task 35.6: 第三轮 MCP/Skill/LSP 主路径接入 ✅

**文件**：`cmd/basework/runtime.go`, `cmd/basework/runtime_test.go`

**内容**：
- 从 `mcp_configs` 初始化 MCP manager，并注册 MCP 工具、资源读取、提示获取工具
- MCP 单个 server 连接失败只记录 warning，不阻断 CLI/TUI 启动
- 初始化 LSP manager 并注册 6 个 LSP 工具
- 扫描 `.basework/skills`、`~/.basework/skills`、`~/.config/basework/skills` 并注入 system prompt
- 使用 agent plugin 在退出时关闭 MCP/LSP 资源

**验收标准**：
- [x] runtime 工具列表包含增强工具和 LSP 工具
- [x] workspace Skill 可发现并注入 system prompt
- [x] MCP/LSP 生命周期由 agent 关闭流程托管
- [x] MCP 连接失败不会导致无关 CLI/TUI 启动失败

### Phase 35 后续任务（下一轮）

| 任务 | 优先级 | 状态 | 说明 |
|------|--------|------|------|
| 完整权限交互 | P1 | 🔲 待做 | REPL/TUI ask 流程、权限缓存、SQLite 审计 |
| CI/CD 加强 | P1 | 🔲 待做 | lint + platform matrix + release dry-run/tag 策略 |
| README 对齐 | P2 | 🔲 待做 | 默认 provider/model 与文档声明统一 |

---

## 依赖关系总览

### Phase 1-12：核心框架（已完成 ✅）

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

### Phase 13-25：终端产品化（已完成 ✅）

```
Phase 12 完成后
    │
    ├──→ Phase 13: internal/compaction/  ← 上下文压缩（依赖 pkg/agent）
    ├──→ Phase 14: internal/retry/       ← 重试机制（依赖 pkg/llm, pkg/provider）
    ├──→ Phase 15: internal/permission/  ← 权限系统（依赖 pkg/tool, pkg/agent）
    │         │
    │         ↓
    │    Phase 16: internal/subagent/    ← 子代理（依赖 permission）
    │
    ├──→ Phase 17: internal/tools/       ← 增强工具（依赖 pkg/tool）
    │
    ├──→ Phase 19: pkg/session/ (SQLite) ← 会话增强（依赖 pkg/session）
    ├──→ Phase 20: pkg/provider/ (扩展)  ← Provider 扩展（依赖 pkg/provider）
    │
    ├──→ Phase 21: internal/observability/ ← 可观测性（独立）
    ├──→ Phase 22: internal/loopdetect/  ← 循环检测（依赖 pkg/agent）
    ├──→ Phase 23: internal/cache/       ← Prompt 缓存（依赖 pkg/llm）
    ├──→ Phase 24: pkg/mcp/ (增强)       ← MCP 增强（依赖 pkg/mcp）
    ├──→ Phase 25: internal/oauth/       ← OAuth 认证（独立）
    │
    └──→ Phase 18: internal/tui/         ← 终端 UI（依赖 Phase 13-17）
```

Phase 13-17, 19-25 可并行开发。Phase 18 (TUI) 依赖 Phase 13-17 完成。

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

## 任务统计（Phase 1-12：已完成）

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

---

## 任务统计（Phase 13-25：终端产品化 - 已完成 ✅）

| Phase | 预计行数 | 任务数 | 依赖 | 优先级 |
|-------|---------|--------|------|--------|
| Phase 13: 上下文压缩 | ~600 | 5 | Phase 5 | P0 |
| Phase 14: 重试机制 | ~400 | 4 | Phase 1, 6 | P0 |
| Phase 15: 权限系统 | ~500 | 5 | Phase 4, 5 | P0 |
| Phase 16: 子代理 | ~500 | 4 | Phase 5, 15 | P0 |
| Phase 17: 增强工具 | ~600 | 5 | Phase 4, 5 | P0-P1 |
| Phase 18: 终端 UI | ~1200 | 6 | Phase 13-17 | P0 |
| Phase 19: 会话增强 | ~500 | 4 | Phase 3 | P0-P2 |
| Phase 20: Provider 扩展 | ~800 | 5 | Phase 6 | P0-P2 |
| Phase 21: 可观测性 | ~300 | 3 | Phase 5 | P0-P2 |
| Phase 22: 循环检测 | ~200 | 2 | Phase 5 | P1 |
| Phase 23: Prompt 缓存 | ~200 | 2 | Phase 1, 6 | P1 |
| Phase 24: MCP 增强 | ~400 | 4 | Phase 8 | P1-P2 |
| Phase 25: OAuth 认证 | ~400 | 3 | Phase 6 | P2 |
| **合计** | **~6600** | **52** | | |

---

## 总任务统计

| 阶段 | 行数 | 任务数 |
|------|------|--------|
| Phase 1-12（已完成） | ~4800 | 42 |
| Phase 13-25（已完成 ✅）| ~6600 | 52 |
| **总计** | **~11400** | **94** |

---

## 执行顺序建议

_✅ 已于 2026-07 完成_

```
Week 1-2: Phase 13 (压缩) + Phase 14 (重试) — 可并行
Week 3:   Phase 15 (权限) + Phase 22 (循环检测) — 可并行
Week 4:   Phase 16 (子代理) + Phase 17 (增强工具) — 可并行
Week 5:   Phase 19 (会话 SQLite) + Phase 20 (Provider 扩展) — 可并行
Week 6-8: Phase 18 (TUI) — 依赖 Phase 13-17
Week 9:   Phase 21 (可观测性) + Phase 23 (缓存) — 可并行
Week 10:  Phase 24 (MCP 增强) + Phase 25 (OAuth) — 可并行
```

---

## Phase 13: 上下文压缩 (`internal/compaction/`)

**目标**：当上下文窗口接近满时，自动压缩历史消息，保持 Agent 持续工作。

**预计行数**：~600 行

### Task 13.1: Token 估算器 (`internal/compaction/token.go`)

**文件**：`internal/compaction/token.go`

**内容**：
- `Estimate(messages []llm.ChatMessage) int` — 基于字符数/token 比的粗略估算
- `EstimateToolResult(content string) int` — 工具输出 token 估算
- `ContextWindowSize(modelID string) int` — 从 provider 元数据获取窗口大小
- 支持手动覆盖

**验收标准**：
- [ ] 估算误差 < 20%（与 tiktoken 对比）
- [ ] 覆盖常见模型的窗口大小
- [ ] 有单元测试

### Task 13.2: 压缩策略接口 (`internal/compaction/strategy.go`)

**文件**：`internal/compaction/strategy.go`

**内容**：
- `Strategy` 接口：`Compact(ctx, messages, budget) ([]llm.ChatMessage, error)`
- `TruncateStrategy` — 截断最早的消息，保留最近 N 轮
- `PruneToolOutputStrategy` — 修剪大型工具输出，替换为摘要
- `SummarizeStrategy` — 调用 LLM 生成历史摘要
- `CombinedStrategy` — 按顺序应用多个策略

**验收标准**：
- [ ] 策略接口清晰
- [ ] 3 种内置策略实现
- [ ] 组合策略支持链式调用

### Task 13.3: 自动压缩引擎 (`internal/compaction/engine.go`)

**文件**：`internal/compaction/engine.go`

**内容**：
- `Engine` 结构体 — 管理压缩流程
- `ShouldCompact(messages, modelID) bool` — 判断是否需要压缩
- `Compact(ctx, messages, modelID) ([]llm.ChatMessage, error)` — 执行压缩
- 配置：触发阈值（默认 80% 窗口）、保留最近轮次数（默认 2）
- 修剪保护：skill 工具输出不修剪

**验收标准**：
- [ ] 80% 阈值触发压缩
- [ ] 保留最近 2 轮对话
- [ ] 保护 skill 输出
- [ ] 有单元测试

### Task 13.4: 摘要提示模板 (`internal/compaction/prompt.go`)

**文件**：`internal/compaction/prompt.go`

**内容**：
- `BuildSummaryPrompt(messages) string` — 构建摘要请求
- 结构化模板：目标、约束、进度、关键决策、下一步、相关文件
- 支持多语言（中文/英文）

**验收标准**：
- [ ] 模板输出结构化摘要
- [ ] 有单元测试验证格式

### Task 13.5: Agent 集成 (`internal/compaction/hook.go`)

**文件**：`internal/compaction/hook.go`

**内容**：
- `CompactionHook` — 实现 `hook.Hook` 接口
- 在 `PreStep` 中检查上下文使用率
- 超阈值时自动压缩
- 压缩后注入摘要消息

**验收标准**：
- [ ] 自动触发压缩
- [ ] 压缩后 Agent 继续正常工作
- [ ] 集成测试验证

---

## Phase 14: 重试机制 (`internal/retry/`)

**目标**：Provider 请求失败时自动重试，处理速率限制和临时错误。

**预计行数**：~400 行

### Task 14.1: 错误分类 (`internal/retry/classify.go`)

**文件**：`internal/retry/classify.go`

**内容**：
- `ClassifyError(err) RetryDecision` — 分类错误为可重试/不可重试
- 可重试：5xx、速率限制、超时、连接错误
- 不可重试：401 认证、400 无效请求、上下文溢出、内容策略
- 利用 `pkg/llm` 已有的错误类型

**验收标准**：
- [ ] 正确分类各类错误
- [ ] 有单元测试

### Task 14.2: 指数退避调度器 (`internal/retry/backoff.go`)

**文件**：`internal/retry/backoff.go`

**内容**：
- `Backoff` 结构体 — 管理退避状态
- `NextDelay() time.Duration` — 计算下次延迟
- 初始延迟 2s、因子 2x、最大 30s
- 解析 `Retry-After` 头部（秒数 / HTTP 日期）
- 解析 `Retry-After-Ms` 头部
- 抖动（jitter）避免惊群

**验收标准**：
- [ ] 退避序列正确：2s, 4s, 8s, 16s, 30s, 30s...
- [ ] 正确解析 Retry-After
- [ ] 有单元测试

### Task 14.3: 重试包装器 (`internal/retry/wrapper.go`)

**文件**：`internal/retry/wrapper.go`

**内容**：
- `RetryModel` — 包装 `llm.Model` 接口
- `Generate(ctx, req) -> 自动重试`
- `Stream(ctx, req) -> 自动重试（流中断恢复）`
- 最大重试次数可配置（默认 5）
- 进度回调通知

**验收标准**：
- [ ] 包装后透明重试
- [ ] 流中断可恢复
- [ ] 超过最大次数返回错误
- [ ] 有单元测试

### Task 14.4: Provider 集成 (`internal/retry/integration.go`)

**文件**：`internal/retry/integration.go`

**内容**：
- `WrapProvider(model llm.Model, cfg Config) llm.Model` — 便捷函数
- 在 `pkg/provider/factory.go` 的 Create 流程中自动包装
- 配置项：`retry.max_attempts`, `retry.initial_delay`, `retry.max_delay`

**验收标准**：
- [ ] Provider 创建时自动包装
- [ ] 配置可自定义
- [ ] 有集成测试

---

## Phase 15: 权限系统 (`internal/permission/`)

**目标**：工具调用前检查权限，支持规则引擎和交互提示。

**预计行数**：~500 行

### Task 15.1: 权限规则 (`internal/permission/rules.go`)

**文件**：`internal/permission/rules.go`

**内容**：
- `Rule` 结构体：`{Action, Resource, Effect}`
- `Effect` 类型：`Allow`, `Deny`
- 通配符支持（`*` 匹配所有）
- `Ruleset` 结构体：有序规则列表
- `Evaluate(action, resource) Decision` — 按顺序匹配

**验收标准**：
- [ ] 规则匹配正确
- [ ] 通配符工作正常
- [ ] 有单元测试

### Task 15.2: 权限服务 (`internal/permission/service.go`)

**文件**：`internal/permission/service.go`

**内容**：
- `Service` 结构体 — 管理权限检查和会话状态
- `Check(ctx, tool, args) (Allowed, error)` — 检查工具调用权限
- 会话级缓存（"always" 授权）
- 全局规则 + 会话规则合并

**验收标准**：
- [ ] 权限检查正确
- [ ] 会话缓存工作正常
- [ ] 有单元测试

### Task 15.3: 交互提示 (`internal/permission/prompt.go`)

**文件**：`internal/permission/prompt.go`

**内容**：
- `PromptHandler` 接口 — 处理权限提示
- `TerminalPromptHandler` — 终端交互实现
- 提示选项：一次允许 / 永远允许 / 拒绝
- YOLO 模式：跳过所有提示

**验收标准**：
- [ ] 终端提示工作正常
- [ ] YOLO 模式跳过提示
- [ ] 有单元测试

### Task 15.4: Agent 集成 (`internal/permission/hook.go`)

**文件**：`internal/permission/hook.go`

**内容**：
- `PermissionHook` — 实现 `hook.Hook` 接口
- 在 `PreToolUse` 中检查权限
- 拒绝时返回错误给模型
- Hook 预审批：`PreToolUse` 返回 allow 时跳过提示

**验收标准**：
- [ ] 工具调用前检查权限
- [ ] 拒绝时正确反馈
- [ ] 有集成测试

### Task 15.5: 命令黑名单 (`internal/permission/bash_guard.go`)

**文件**：`internal/permission/bash_guard.go`

**内容**：
- `IsBlockedCommand(cmd string) bool` — 检查 bash 命令是否被禁止
- 禁止列表：`rm -rf /`, `mkfs`, `dd if=`, `:(){ :|:& };:` 等
- 支持自定义禁止列表（配置）

**验收标准**：
- [ ] 危险命令被拦截
- [ ] 正常命令不受影响
- [ ] 有单元测试

---

## Phase 16: 子代理 (`internal/subagent/`)

**目标**：支持启动隔离子代理处理复杂任务。

**预计行数**：~500 行

### Task 16.1: 子代理管理器 (`internal/subagent/manager.go`)

**文件**：`internal/subagent/manager.go`

**内容**：
- `Manager` 结构体 — 管理子代理生命周期
- `Spawn(ctx, task, options) (*Result, error)` — 启动子代理
- 隔离子会话（独立 session ID）
- 前台模式（等待结果）/ 后台模式（异步执行）

**验收标准**：
- [ ] 子代理在隔离子会话中运行
- [ ] 前台/后台模式工作正常
- [ ] 有单元测试

### Task 16.2: task 工具 (`internal/subagent/task_tool.go`)

**文件**：`internal/subagent/task_tool.go`

**内容**：
- `TaskTool` — 实现 `tool.Tool` 接口
- 参数：`description`, `prompt`, `subagent_type`
- 返回子代理执行结果
- 支持 task_id 恢复之前的子代理会话

**验收标准**：
- [ ] 工具定义正确
- [ ] 子代理执行并返回结果
- [ ] 有单元测试

### Task 16.3: 成本传播 (`internal/subagent/cost.go`)

**文件**：`internal/subagent/cost.go`

**内容**：
- 子代理 token 使用累加到父会话
- `CostAggregator` — 聚合多子代理成本
- 在 session 事件中记录总成本

**验收标准**：
- [ ] 成本正确累加
- [ ] 有单元测试

### Task 16.4: 只读代理 (`internal/subagent/readonly.go`)

**文件**：`internal/subagent/readonly.go`

**内容**：
- `ReadOnlyAgent` — 配置为只读工具集
- 禁用：write, edit, bash, multiedit
- 适用场景：信息搜索、代码分析

**验收标准**：
- [ ] 只写工具被禁用
- [ ] 只读工具正常工作
- [ ] 有单元测试

---

## Phase 17: 增强工具 (`pkg/tool/builtin/` + `internal/tools/`)

**目标**：添加更多实用工具。

**预计行数**：~600 行

### Task 17.1: web_fetch 工具 (`internal/tools/webfetch.go`)

**文件**：`internal/tools/webfetch.go`

**内容**：
- `WebFetchTool` — 实现 `tool.Tool` 接口
- 参数：`url`, `format` (text/markdown/html)
- HTTP GET + 内容提取（纯文本/markdown/html）
- 大小限制：100KB
- 超时：30s

**验收标准**：
- [ ] 正确获取 URL 内容
- [ ] 格式转换工作正常
- [ ] 大小限制生效
- [ ] 有单元测试

### Task 17.2: web_search 工具 (`internal/tools/websearch.go`)

**文件**：`internal/tools/websearch.go`

**内容**：
- `WebSearchTool` — 实现 `tool.Tool` 接口
- 参数：`query`, `num_results`
- 后端：Tavily API / Exa API（可配置）
- 返回搜索结果列表

**验收标准**：
- [ ] 搜索返回结果
- [ ] 可配置后端
- [ ] 有单元测试

### Task 17.3: todowrite 工具 (`internal/tools/todowrite.go`)

**文件**：`internal/tools/todowrite.go`

**内容**：
- `TodoWriteTool` — 实现 `tool.Tool` 接口
- 参数：`todos` (JSON 数组)
- 会话级任务列表存储
- 渲染输出：任务列表的文本表示

**验收标准**：
- [ ] 任务列表正确存储
- [ ] 渲染格式清晰
- [ ] 有单元测试

### Task 17.4: apply_patch 工具 (`internal/tools/applypatch.go`)

**文件**：`internal/tools/applypatch.go`

**内容**：
- `ApplyPatchTool` — 实现 `tool.Tool` 接口
- 参数：`patch` (结构化补丁文本)
- 补丁格式：`*** Begin Patch` / `*** End Patch`
- 支持：Add File / Delete File / Update File
- 顺序应用，部分失败时报告进度

**验收标准**：
- [ ] 补丁正确应用
- [ ] 部分失败时报告已应用/失败的文件
- [ ] 有单元测试

### Task 17.5: question 工具 (`internal/tools/question.go`)

**文件**：`internal/tools/question.go`

**内容**：
- `QuestionTool` — 实现 `tool.Tool` 接口
- 参数：`questions` (JSON 数组，含问题文本和选项)
- 通过权限系统交互提示用户
- 返回用户选择

**验收标准**：
- [ ] 正确显示问题和选项
- [ ] 返回用户选择
- [ ] 有单元测试

---

## Phase 18: 终端 UI (`internal/tui/`)

**目标**：基于 Bubble Tea 构建完整的终端界面。

**预计行数**：~1200 行
**依赖**：`charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`, `charm.land/glamour`

### Task 18.1: TUI 框架 (`internal/tui/app.go`)

**文件**：`internal/tui/app.go`

**内容**：
- `App` 结构体 — Bubble Tea 主应用
- `Model` 接口实现：`Init()`, `Update()`, `View()`
- 消息类型：`UserInputMsg`, `AgentResponseMsg`, `ToolCallMsg`, `ErrorMsg`
- 布局：顶部状态栏 + 中间消息区 + 底部输入区

**验收标准**：
- [ ] 基本界面渲染正确
- [ ] 键盘输入处理正常
- [ ] 可编译运行

### Task 18.2: 消息渲染 (`internal/tui/message.go`)

**文件**：`internal/tui/message.go`

**内容**：
- `MessageView` — 消息渲染组件
- 用户消息：简单文本
- 助手消息：Glamour markdown 渲染
- 工具调用：显示工具名 + 参数 + 结果
- 错误消息：红色高亮

**验收标准**：
- [ ] Markdown 正确渲染
- [ ] 工具调用显示清晰
- [ ] 可编译运行

### Task 18.3: 输入区 (`internal/tui/input.go`)

**文件**：`internal/tui/input.go`

**内容**：
- `InputView` — 输入区组件
- 多行输入支持
- 历史导航（上/下箭头）
- Tab 补全（文件路径、命令）
- Ctrl+C 取消 / Ctrl+D 退出

**验收标准**：
- [ ] 多行输入正常
- [ ] 历史记录工作正常
- [ ] 可编译运行

### Task 18.4: 流式渲染 (`internal/tui/streaming.go`)

**文件**：`internal/tui/streaming.go`

**内容**：
- `StreamingView` — 流式 token 渲染
- 逐字输出效果
- 工具调用进度指示
- 思考过程显示（reasoning tokens）

**验收标准**：
- [ ] 流式渲染流畅
- [ ] 工具进度可见
- [ ] 可编译运行

### Task 18.5: 状态栏 (`internal/tui/statusbar.go`)

**文件**：`internal/tui/statusbar.go`

**内容**：
- `StatusBarView` — 顶部状态栏
- 显示：模型名、Provider、token 使用、会话 ID
- MCP 连接状态指示
- 忙/闲状态

**验收标准**：
- [ ] 状态信息正确
- [ ] 实时更新
- [ ] 可编译运行

### Task 18.6: CLI 集成 (`cmd/basework/tui.go`)

**文件**：`cmd/basework/tui.go`

**内容**：
- `tui` 子命令 — 启动 TUI 模式
- 替换现有的简单 REPL
- 保留 `--no-tui` 标志回退到简单模式
- 集成 Agent + Session + Tool + Hook

**验收标准**：
- [ ] TUI 模式可启动
- [ ] 基本对话流程正常
- [ ] 可编译运行

---

## Phase 19: 会话增强 (`pkg/session/`)

**目标**：用 SQLite 替换 JSONL 后端，增加高级会话功能。

**预计行数**：~500 行

### Task 19.1: SQLite 存储 (`pkg/session/sqlite.go`)

**文件**：`pkg/session/sqlite.go`

**内容**：
- `SQLiteStore` — 实现 `session.Store` 接口
- 表结构：sessions, messages, tool_calls, tool_results
- 使用 `modernc.org/sqlite`（纯 Go，无 CGO）
- build tag `sqlite` 控制

**验收标准**：
- [ ] CRUD 操作正确
- [ ] 查询性能可接受
- [ ] 有单元测试

### Task 19.2: 自动标题生成 (`pkg/session/title.go`)

**文件**：`pkg/session/title.go`

**内容**：
- `GenerateTitle(ctx, model, firstMessage) (string, error)`
- 使用小型模型生成简短标题
- 在首条用户消息后自动触发
- 可配置禁用

**验收标准**：
- [ ] 标题生成正确
- [ ] 自动触发工作正常
- [ ] 有单元测试

### Task 19.3: 会话队列 (`pkg/session/queue.go`)

**文件**：`pkg/session/queue.go`

**内容**：
- `Queue` 结构体 — 管理排队提示
- 会话忙时新消息入队
- 按 FIFO 顺序处理
- 支持取消排队消息

**验收标准**：
- [ ] 排队机制正确
- [ ] FIFO 顺序处理
- [ ] 有单元测试

### Task 19.4: 文件追踪 (`pkg/session/filetrack.go`)

**文件**：`pkg/session/filetrack.go`

**内容**：
- `FileTracker` — 追踪会话中访问/修改的文件
- `TrackRead(path)`, `TrackWrite(path)`, `TrackEdit(path)`
- `GetTrackedFiles() []string`
- 在会话元数据中持久化

**验收标准**：
- [ ] 文件访问正确追踪
- [ ] 持久化到存储
- [ ] 有单元测试

---

## Phase 20: Provider 扩展 (`pkg/provider/`)

**目标**：添加更多 Provider 支持。

**预计行数**：~800 行

### Task 20.1: OpenCode Zen (`pkg/provider/opencode.go`)

**文件**：`pkg/provider/opencode.go`

**内容**：
- `OpenCodeProvider` — OpenCode Zen 平台集成
- 端点：`https://opencode.ai/zen/v1/chat/completions`
- 默认模型：`big-pickle`
- 免费模型列表：big-pickle, deepseek-v4-flash-free, mimo-v2.5-free 等
- OpenAI 兼容协议

**验收标准**：
- [ ] 可连接到 OpenCode Zen
- [ ] big-pickle 模型工作正常
- [ ] 有单元测试

### Task 20.2: Amazon Bedrock (`pkg/provider/bedrock.go`)

**文件**：`pkg/provider/bedrock.go`

**内容**：
- `BedrockProvider` — AWS Bedrock Converse API
- AWS 认证（access key + secret key + region）
- 模型 ID 映射（anthropic.claude-3-5-sonnet 等）
- 流式支持

**验收标准**：
- [ ] AWS 认证正确
- [ ] 消息收发正常
- [ ] 有单元测试

### Task 20.3: Azure OpenAI (`pkg/provider/azure.go`)

**文件**：`pkg/provider/azure.go`

**内容**：
- `AzureProvider` — Azure OpenAI 服务
- 端点格式：`https://{resource}.openai.azure.com/openai/deployments/{deployment}`
- API key 或 AD token 认证
- api-version 参数

**验收标准**：
- [ ] Azure 端点正确
- [ ] 认证工作正常
- [ ] 有单元测试

### Task 20.4: GitHub Copilot (`pkg/provider/copilot.go`)

**文件**：`pkg/provider/copilot.go`

**内容**：
- `CopilotProvider` — GitHub Copilot 集成
- OAuth 认证流程
- 模型：gpt-4, claude-3-sonnet 等
- 特殊的请求头处理

**验收标准**：
- [ ] OAuth 流程正确
- [ ] 消息收发正常
- [ ] 有单元测试

### Task 20.5: Ollama (`pkg/provider/ollama.go`)

**文件**：`pkg/provider/ollama.go`

**内容**：
- `OllamaProvider` — Ollama 本地模型
- 端点：`http://localhost:11434/v1/chat/completions`
- 自动发现本地模型（`/api/tags`）
- OpenAI 兼容协议

**验收标准**：
- [ ] 可连接到 Ollama
- [ ] 自动发现模型列表
- [ ] 有单元测试

---

## Phase 21: 可观测性 (`internal/observability/`)

**目标**：结构化日志和成本追踪。

**预计行数**：~300 行

### Task 21.1: 结构化日志 (`internal/observability/logging.go`)

**文件**：`internal/observability/logging.go`

**内容**：
- 基于 `log/slog` 的结构化日志
- 日志级别：DEBUG, INFO, WARN, ERROR
- 文件日志：`~/.basework/logs/basework.log`
- 可配置日志级别（环境变量 `BASEWORK_LOG_LEVEL`）

**验收标准**：
- [ ] 日志输出格式正确
- [ ] 文件日志工作正常
- [ ] 有单元测试

### Task 21.2: 成本追踪 (`internal/observability/cost.go`)

**文件**：`internal/observability/cost.go`

**内容**：
- `CostTracker` — 追踪 token 使用和费用
- 每个模型的定价表（每百万 token）
- `TrackUsage(modelID, promptTokens, completionTokens)`
- `GetTotalCost() float64`

**验收标准**：
- [ ] token 计数正确
- [ ] 费用计算准确
- [ ] 有单元测试

### Task 21.3: 事件总线 (`internal/observability/events.go`)

**文件**：`internal/observability/events.go`

**内容**：
- `EventBus` — 发布/订阅事件系统
- 事件类型：SessionCreated, MessageReceived, ToolCalled, ErrorOccurred
- 订阅者接收事件通知

**验收标准**：
- [ ] 事件发布/订阅正常
- [ ] 有单元测试

---

## Phase 22: 循环检测 (`internal/loopdetect/`)

**目标**：检测 Agent 陷入循环并自动中断。

**预计行数**：~200 行

### Task 22.1: 循环检测器 (`internal/loopdetect/detector.go`)

**文件**：`internal/loopdetect/detector.go`

**内容**：
- `Detector` 结构体 — 追踪工具调用签名
- SHA-256 哈希：tool_name + input + output
- 滑动窗口：最近 10 步
- 阈值：同一签名出现 > 5 次触发中断

**验收标准**：
- [ ] 正确检测重复调用
- [ ] 滑动窗口工作正常
- [ ] 有单元测试

### Task 22.2: Agent 集成 (`internal/loopdetect/hook.go`)

**文件**：`internal/loopdetect/hook.go`

**内容**：
- `LoopDetectHook` — 实现 `hook.Hook` 接口
- 在 `PostToolUse` 中记录调用签名
- 检测到循环时返回错误终止 Agent

**验收标准**：
- [ ] 循环时正确中断
- [ ] 正常调用不受影响
- [ ] 有集成测试

---

## Phase 23: Prompt 缓存 (`internal/cache/`)

**目标**：自动注入 Anthropic 缓存控制标记，减少 token 消耗。

**预计行数**：~200 行

### Task 23.1: 缓存策略 (`internal/cache/policy.go`)

**文件**：`internal/cache/policy.go`

**内容**：
- `Policy` 接口 — 决定哪些消息/工具添加缓存标记
- `AutoPolicy` — 自动在最后一个工具定义、最后系统消息、最新用户消息处注入
- `ExplicitPolicy` — 手动指定缓存位置
- 仅适用于 Anthropic 协议

**验收标准**：
- [ ] 缓存标记正确注入
- [ ] 仅对 Anthropic 生效
- [ ] 有单元测试

### Task 23.2: Provider 集成 (`internal/cache/integration.go`)

**文件**：`internal/cache/integration.go`

**内容**：
- 在 `pkg/provider/anthropic.go` 中集成缓存策略
- `CacheControl` 字段添加到消息和工具定义
- 配置：`cache.enabled`, `cache.policy`

**验收标准**：
- [ ] Anthropic 请求包含缓存标记
- [ ] 缓存命中时 token 减少
- [ ] 有单元测试

---

## Phase 24: MCP 增强 (`pkg/mcp/`)

**目标**：增强 MCP 客户端功能。

**预计行数**：~400 行

### Task 24.1: 资源支持 (`pkg/mcp/resources.go`)

**文件**：`pkg/mcp/resources.go`

**内容**：
- `ListResources(ctx) ([]Resource, error)` — 列出 MCP 资源
- `ReadResource(ctx, uri) (Content, error)` — 读取资源内容
- `list_mcp_resources` 工具
- `read_mcp_resource` 工具

**验收标准**：
- [ ] 资源列表正确获取
- [ ] 资源读取正确
- [ ] 有单元测试

### Task 24.2: 提示支持 (`pkg/mcp/prompts.go`)

**文件**：`pkg/mcp/prompts.go`

**内容**：
- `ListPrompts(ctx) ([]Prompt, error)` — 列出 MCP 提示
- `GetPrompt(ctx, name, args) (Messages, error)` — 获取提示消息
- 提示消息注入到会话中

**验收标准**：
- [ ] 提示列表正确
- [ ] 提示消息注入正确
- [ ] 有单元测试

### Task 24.3: 自动重连 (`pkg/mcp/reconnect.go`)

**文件**：`pkg/mcp/reconnect.go`

**内容**：
- `ReconnectManager` — 管理 MCP 连接重连
- Ping 失败时自动重建连接
- 最大重试次数、退避策略
- 状态机：Connected → Reconnecting → Connected/Error

**验收标准**：
- [ ] 断线后自动重连
- [ ] 重连状态正确
- [ ] 有单元测试

### Task 24.4: Shell 变量展开 (`pkg/mcp/expand.go`)

**文件**：`pkg/mcp/expand.go`

**内容**：
- `ExpandConfig(cfg MCPConfig) MCPConfig` — 展开配置中的变量
- 支持：`$VAR`, `${VAR}`, `${VAR:-default}`, `$(command)`
- 应用于 command, args, env, headers, url

**验收标准**：
- [ ] 变量正确展开
- [ ] 默认值工作正常
- [ ] 命令替换工作正常
- [ ] 有单元测试

---

## Phase 25: OAuth 认证 (`internal/oauth/`)

**目标**：支持 OAuth 2.0 认证流程。

**预计行数**：~400 行

### Task 25.1: OAuth 客户端 (`internal/oauth/client.go`)

**文件**：`internal/oauth/client.go`

**内容**：
- `OAuthClient` — 实现 OAuth 2.0 客户端
- PKCE 流程（code_verifier + code_challenge）
- 授权码交换
- 令牌刷新

**验收标准**：
- [ ] PKCE 流程正确
- [ ] 令牌获取正确
- [ ] 有单元测试

### Task 25.2: 令牌存储 (`internal/oauth/store.go`)

**文件**：`internal/oauth/store.go`

**内容**：
- `TokenStore` — 持久化 OAuth 令牌
- 存储位置：`~/.basework/oauth/tokens.json`
- 支持 access_token, refresh_token, expires_at
- 自动刷新过期令牌

**验收标准**：
- [ ] 令牌持久化正确
- [ ] 自动刷新工作正常
- [ ] 有单元测试

### Task 25.3: Provider 集成 (`internal/oauth/providers.go`)

**文件**：`internal/oauth/providers.go`

**内容**：
- GitHub Copilot OAuth 配置
- OpenCode Zen OAuth 配置
- `basework auth` CLI 命令 — 交互式登录

**验收标准**：
- [ ] Copilot OAuth 工作正常
- [ ] OpenCode Zen OAuth 工作正常
- [ ] CLI 登录流程正确
- [ ] 有集成测试

---

## Phase 26: 会话稳定性加固

**目标**：解决长会话稳定性、并发安全、压缩策略
**预计行数**：~2,000 行
**依赖**：无

### Task 26.1: Session 文件锁

**文件**：`pkg/session/lock.go`
**任务**：
1. 实现基于文件系统的分布式锁（参考 opencode flock.ts）
2. 支持锁获取、释放、续约
3. 支持超时检测和心跳机制
4. 防止多进程同时写入同一个 SQLite 数据库

**预计行数**：~400 行
**测试**：单元测试 + 并发写入测试

### Task 26.2: SQLite WAL 模式

**文件**：`pkg/session/sqlite.go`
**任务**：
1. 启用 SQLite WAL (Write-Ahead Logging) 模式
2. 配置连接池参数
3. 添加 WAL checkpoint 机制
4. 性能对比测试（WAL vs DELETE 模式）

**预计行数**：~200 行
**测试**：性能基准测试

### Task 26.3: 压缩双策略

**文件**：`internal/compaction/compaction.go`
**任务**：
1. 增加"修剪"(Prune)策略：释放工具输出空间
2. 可配置策略选择：`auto` | `summarize` | `prune`
3. 实现工具输出截断（>2000 字符）
4. 压缩后保留最近 N 轮对话（tail_turns）

**预计行数**：~600 行
**测试**：单元测试 + 集成测试

### Task 26.4: 长会话压力测试

**文件**：`tests/stress/long_session_test.go`
**任务**：
1. 100+ 轮对话稳定性测试
2. 内存占用监控
3. token 计数准确性验证
4. 压缩触发时机测试

**预计行数**：~300 行
**测试**：压力测试

### Task 26.5: 并发写入测试

**文件**：`tests/stress/concurrent_write_test.go`
**任务**：
1. 多进程并发写入测试
2. 文件锁竞争测试
3. 死锁检测
4. 数据一致性验证

**预计行数**：~200 行
**测试**：并发测试

---

## Phase 27: CI/CD + 发布流程

**目标**：建立自动化测试、构建、发布流水线
**预计行数**：~500 行（workflow 配置）
**依赖**：无

### Task 27.1: GitHub Actions 基础 workflow

**文件**：`.github/workflows/test.yml`
**任务**：
1. PR/push 触发测试
2. 多 Go 版本测试（1.21, 1.22, 1.23）
3. 代码覆盖率上传
4. lint 检查（golangci-lint）

**预计行数**：~100 行
**测试**：CI 验证

### Task 27.2: 交叉编译 workflow

**文件**：`.github/workflows/build.yml`
**任务**：
1. Linux/macOS/Windows 三平台编译
2. ARM64 支持
3. 二进制打包（tar.gz）
4. 上传到 GitHub Releases

**预计行数**：~150 行
**测试**：CI 验证

### Task 27.3: goreleaser 集成

**文件**：`.goreleaser.yml`
**任务**：
1. 配置 goreleaser
2. 自动生成 CHANGELOG
3. 多平台打包（brew, npm, scoop）
4. Docker 镜像构建

**预计行数**：~150 行
**测试**：发布测试

### Task 27.4: Homebrew 发布

**文件**：`.github/workflows/homebrew.yml` + `Formula/basework.rb`
**任务**：
1. 创建 Homebrew tap
2. 自动更新 Formula
3. `brew install basework` 支持
4. 版本同步

**预计行数**：~100 行
**测试**：安装测试

---

## Phase 28: 安全加固 + 性能基线（已完成 ✅）

**目标**：提升安全性、建立性能基准
**预计行数**：~1,500 行
**依赖**：无

### Task 28.1: 权限持久化

**文件**：`internal/permission/persist.go`
**任务**：
1. 权限规则存储到 SQLite
2. 跨会话权限保留
3. 权限迁移工具
4. 权限审计日志

**预计行数**：~400 行
**测试**：单元测试 + 集成测试

### Task 28.2: 敏感路径保护

**文件**：`internal/permission/paths.go`
**任务**：
1. 禁止访问 `.git/`, `~/.ssh/`, `~/.aws/` 等敏感路径
2. 路径白名单/黑名单机制
3. 工具执行前路径检查
4. 可配置保护级别

**预计行数**：~300 行
**测试**：单元测试

### Task 28.3: 工具执行超时

**文件**：`pkg/tool/builtin/timeout.go`
**任务**：
1. 默认 30s 超时机制
2. 可配置超时时间
3. 超时后优雅终止
4. 超时事件通知

**预计行数**：~200 行
**测试**：单元测试

### Task 28.4: pprof 集成

**文件**：`internal/observability/pprof.go`
**任务**：
1. HTTP pprof 端点
2. CPU/内存 profile 生成
3. goroutine 泄漏检测
4. 性能报告生成

**预计行数**：~200 行
**测试**：性能测试

### Task 28.5: benchmark 套件

**文件**：`tests/benchmark/`
**任务**：
1. token 计数性能基准
2. 流式响应延迟测试
3. 工具执行性能测试
4. 会话读写性能测试

**预计行数**：~400 行
**测试**：基准测试

---

## Phase 29: TUI 增强 + 模板系统（已完成 ✅）

**目标**：提升终端用户体验
**预计行数**：~3,000 行
**依赖**：Phase 18 TUI 基础

### Task 29.1: 主题系统

**文件**：`internal/tui/theme/`
**任务**：
1. 亮/暗主题切换
2. 自定义主题支持
3. 终端主题色自适应
4. 主题配置持久化

**预计行数**：~600 行
**测试**：单元测试

### Task 29.2: 命令面板

**文件**：`internal/tui/command.go`
**任务**：
1. `/` 斜杠命令触发
2. fuzzy 搜索匹配
3. 命令历史
4. 命令补全

**预计行数**：~500 行
**测试**：单元测试

### Task 29.3: 键盘绑定系统

**文件**：`internal/tui/keymap.go`
**任务**：
1. 三层绑定（全局/应用/退出）
2. 可配置快捷键
3. 冲突检测
4. 键盘绑定文档

**预计行数**：~400 行
**测试**：单元测试

### Task 29.4: 按模型分发模板

**文件**：`internal/agent/prompt.go`
**任务**：
1. 根据 Provider 选择系统提示模板
2. 内置模板：anthropic.txt, openai.txt, gemini.txt, default.txt
3. 模板渲染引擎
4. 环境变量替换

**预计行数**：~600 行
**测试**：单元测试

### Task 29.5: 用户自定义模板

**文件**：`internal/agent/template.go`
**任务**：
1. `.basework/prompts/` 目录扫描
2. 模板优先级：用户 > 内置
3. 模板热重载
4. 模板验证

**预计行数**：~400 行
**测试**：单元测试

### Task 29.6: 环境动态注入

**文件**：`internal/agent/environment.go`
**任务**：
1. 自动注入工作目录
2. Git 状态信息
3. 平台信息
4. 项目上下文

**预计行数**：~300 行
**测试**：单元测试

### Task 29.7: 对话框系统优化

**文件**：`internal/tui/dialog/`
**任务**：
1. 对话框管理器
2. 命令式 API
3. 对话框堆栈
4. 模态/非模态支持

**预计行数**：~500 行
**测试**：单元测试

---

## Phase 30: 多模态 + 插件生态（已完成 ✅）

**目标**：扩展能力和生态系统
**预计行数**：~2,500 行
**依赖**：无

### Task 30.1: 图片输入支持

**文件**：`pkg/llm/image.go`
**任务**：
1. 图片加载和验证
2. 自动 resize（最大 2000x2000）
3. base64 编码
4. JPEG 质量渐进压缩
5. Provider 适配（Anthropic/OpenAI/Gemini）

**预计行数**：~600 行
**测试**：单元测试

### Task 30.2: Hook 系统扩展

**文件**：`pkg/hook/extended.go`
**任务**：
1. PreStep, PostStep 钩子
2. OnToolError 钩子
3. OnCompaction 钩子
4. Hook 链执行

**预计行数**：~500 行
**测试**：单元测试

### Task 30.3: 提供商插件化

**文件**：`pkg/provider/plugin.go`
**任务**：
1. Provider 插件接口定义
2. 插件加载机制
3. 插件注册表
4. 插件示例

**预计行数**：~600 行
**测试**：单元测试

### Task 30.4: TUI 插件插槽（长期）

**文件**：`internal/tui/plugin/`
**任务**：
1. 插件运行时
2. UI 组件注册
3. 插槽机制（app_bottom, app）
4. 插件 SDK

**预计行数**：~800 行
**测试**：单元测试
