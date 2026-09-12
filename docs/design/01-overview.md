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
| **可选复杂度** | Build Tag 控制可选模块（memory, sqlite） |

### 非目标

- 不做桌面应用
- 不做 Web UI
- 不做 IDE 插件（但可以被 IDE 插件嵌入）
- `pkg/` 核心不内置 TUI；终端产品能力放在 `cmd/basework` 和 `internal/tui`

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

### 2.2 下一阶段核心能力演进

2026-07-09 与 crush 对比后，basework 的后续设计重点从“补齐模块”转向“提升真实编程任务执行质量”。新增能力必须遵守两条边界：

1. **不污染 `pkg/` 可嵌入核心**：后台服务、TUI 状态、权限弹窗、diff view 等产品能力放在 `internal/` 和 `cmd/basework/`。
2. **不牺牲工具能力换取模型兼容**：免费模型或兼容模型可以降级提示、切换模型或暴露能力限制，但不能静默禁用工具调用。

#### 2.2.1 Provider 与 Tool Call 稳定性

不同 provider 对流式 tool call 的语义并不一致：

- 有的 provider 发送增量 args delta
- 有的 provider 在 complete event 中发送完整 args
- 有的 OpenAI-compatible provider 会混用增量和最终完整参数
- 有的模型不支持 tool call 或需要特殊协议适配

设计要求：

- `pkg/agent/pipeline.go` 负责归一化 tool call 事件，避免 provider 层差异泄漏到 agent loop。
- `pkg/llm` 的模型能力应能表达 `tools/streaming/vision/json/requires_api_key/free` 等信息。
- `cmd/basework model list/use` 应展示模型能力，避免用户选择后才在运行时失败。
- OpenCode 免费模型等无 key 场景必须保留工具能力；如果模型本身不支持工具，应在选择阶段提示或自动切换。

#### 2.2.2 Tool Job 生命周期

当前工具调用以同步执行为主。后续需要引入 job 生命周期，用于 shell 长命令、流式输出、取消和历史输出查询。

建议状态机：

```
Queued → Running → Succeeded
                 → Failed
                 → Canceled
                 → TimedOut
```

设计要求：

- job manager 属于 `internal/` 产品层，不进入 `pkg/tool` 核心接口。
- `pkg/tool` 继续保持简单同步接口；产品层通过适配器包装后台任务能力。
- session 记录 job id、命令、输出摘要、退出码、开始/结束时间和最终状态。
- TUI 通过事件总线观察 job 状态，不直接轮询工具内部实现。

#### 2.2.3 可靠编辑与 Diff

写入类工具必须从“直接改文件”演进为“可审计的编辑事务”。

设计要求：

- `edit/apply_patch/multi_edit/write` 等写入类工具输出 touched files 和结构化 diff。
- 权限 ask 模式下，写入前可展示 diff 并等待用户确认。
- 编辑失败必须区分匹配失败、文件已变化、权限拒绝、部分应用失败等情况。
- session 记录编辑摘要，TUI 负责渲染 diff，不在工具层生成终端样式字符串。

#### 2.2.4 Workspace Context

Agent 需要知道自己读过、改过、搜索过和运行过什么，而不是只依赖原始消息历史。

设计要求：

- session 增加 workspace projection：read files、written files、searched patterns、executed commands、diagnostics。
- environment 注入高价值摘要：当前分支、git status、最近 touched files、最近失败命令。
- 子代理可继承只读 workspace context，但不继承父代理写权限。
- context compaction 优先保留 workspace 摘要、最新 diff、失败测试和用户明确目标。

#### 2.2.5 Backend / Server / Client 分层

为支持后台任务、多会话、取消运行、TUI 状态同步，后续需要最小 backend 分层。

```
cmd/basework
    │
    ├── CLI/TUI client
    │       │
    │       ▼
    ├── internal/client  ───────► internal/server
    │                               │
    │                               ▼
    │                         internal/backend
    │                               │
    │                               ▼
    │                         pkg/agent + pkg/session + tools
```

设计要求：

- 单进程 embedded 用法继续直接使用 `pkg/agent`，不强制依赖 server。
- TUI 不直接持有复杂 runtime 状态，只订阅 backend 事件并发送用户动作。
- proto 第一阶段只覆盖 send message、cancel run、list sessions、subscribe events、permission response。
- backend 可以先作为同进程服务实现，待稳定后再开放 socket/server 模式。

#### 2.2.6 TUI 产品化边界

TUI 后续重点不是装饰，而是让核心能力可见、可控、可恢复。

设计要求：

- chat、input、status、tool view、dialogs、session/model selector 拆分为独立子模型。
- 工具调用展示等待权限、运行中、成功、失败、取消等状态。
- diff view、权限确认、模型选择、会话切换、通知和补全都消费 backend 事件或结构化数据。
- TUI 不解析工具原始文本来判断状态，所有状态必须来自 agent/backend 事件。

### 2.3 层间依赖规则

- 每层只能依赖下方层，不可反向依赖
- L0 (Foundation) 无外部依赖，只有标准库 + LLM SDK
- L1 (Agent) 依赖 L0，定义核心循环
- L2 (Session) 依赖 L0，独立于 L1（可单独使用）
- L3 (Extension) 内部有两类：
  - **轻量扩展**：Config、Skill、LSP、MCP — 只依赖 L0（+ L4 的 Tool 接口），不依赖 L1
  - **重量扩展**：Memory — 依赖 L0 + L1（需要 agent 上下文）
- L4 (Application) 依赖所有层

### 2.4 数据流

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
| `sqlite` | 关 | SQLite 会话存储（替换 JSONL） |

无 `slim`、`bedrock`、`isolation`、`tui`、`otel` 标签。TUI 和 observability 是终端产品代码路径的一部分，不再通过 build tag 控制。SQLite/FTS5 相关能力需要时显式开启。

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
│   ├── tools/                 增强工具（web_fetch、web_search 等）
│   └── tui/                   终端 UI（Bubble Tea 已实现）
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

### D9: 增强工具为什么放在 `internal/tools/` 而非 `pkg/tool/builtin/`？

内置工具（bash/read/write/edit/glob/grep）是框架核心，必须嵌入方可用。增强工具（web_fetch/web_search/todowrite/apply_patch/question）是终端产品功能，嵌入方可以按需实现自己的版本。

**方案**：增强工具放在 `internal/tools/`，通过 `cmd/basework` 注册到 ToolRegistry。嵌入方可以自行决定是否使用。

### D10: TUI 为什么用 Bubble Tea 而非其他框架？

Bubble Tea 是 Go 生态中最成熟的 TUI 框架，借鉴了 Elm 架构。相比其他选择：
- **tview**：更重，布局系统复杂
- **termui**：面向仪表盘，不适合聊天应用
- **Bubble Tea**：组件化、消息驱动、与 Go 并发模型契合

### D11: SQLite 会话存储为什么用 build tag 控制？

SQLite 通过 `modernc.org/sqlite`（纯 Go）实现，但仍增加了二进制大小和启动时间。用 `sqlite` build tag 控制：
- 默认使用 JSONLStore（零额外依赖）
- 需要 SQLite 时显式启用 `go build -tags sqlite`
- 存储接口一致，切换对上层透明

### D12: Bedrock 为什么不依赖 AWS SDK？

AWS SDK Go v2 约 15MB，其中大部分是 basework 不需要的服务。Bedrock Provider 只需要 SigV4 签名和 Converse API，使用原生 HTTP 实现：
- 原生 SigV4 实现约 80 行代码
- 无额外依赖，避免依赖膨胀
- 测试友好（可 mock HTTP 端点）

### D13: Copilot 为什么用 OAuth 设备授权而非 API Key？

GitHub Copilot 不提供 API Key 认证方式，只支持 OAuth 设备授权流程。这是 CLI 应用的标准 OAuth 模式：
- 用户无需在浏览器中输入敏感凭证
- 设备码流程适合无 GUI 的终端环境
- Token 持久化到文件，避免重复认证

---

