# Basework 架构概览

## 系统架构

```
┌─────────────────────────────────────────────────────────┐
│  cmd/basework/ — CLI 入口（cobra + 增强 REPL + TUI）     │
├─────────────────────────────────────────────────────────┤
│  internal/ — 终端产品专用逻辑                            │
│  ├── tui/            终端 UI（Bubble Tea）               │
│  ├── compaction/     上下文压缩                          │
│  ├── retry/          重试机制                            │
│  ├── permission/     权限系统                            │
│  ├── subagent/       子代理                              │
│  ├── loopdetect/     循环检测                            │
│  ├── observability/  可观测性（日志 + 成本追踪 + 事件总线）│
│  ├── oauth/          OAuth 认证                          │
│  └── tools/          增强工具                            │
├─────────────────────────────────────────────────────────┤
│  pkg/ — 核心框架（可嵌入，稳定 API）                      │
│  ├── llm/          类型系统 + 错误分类 + 图片处理         │
│  ├── tool/         工具接口 + 注册表 + 内置工具          │
│  ├── hook/         Hook 系统 + PubSub                   │
│  ├── session/      会话管理 + 事件溯源                   │
│  ├── agent/        Agent 循环 + 流式处理                 │
│  ├── provider/     Provider 工厂（15+ 个 Provider）       │
│  │                 + Prompt 缓存（cache.go）             │
│  ├── lsp/          LSP 集成                              │
│  ├── mcp/          MCP 集成                              │
│  ├── memory/       记忆系统（FTS5, build tag）           │
│  ├── config/       配置管理（CoW 模式）                  │
│  └── skill/        技能加载                              │
└─────────────────────────────────────────────────────────┘
```

> **分层硬约束**：`pkg/` **不得**依赖 `internal/`（含 `basework/internal/*`）。
> `pkg/` 是可独立嵌入、面向外部使用者的核心层；`internal/` 是终端产品专用实现，
> 反向依赖会让核心层无法独立发布。
>
> 该约束由 `tests/arch_test.go` 用 `go/parser` 静态扫描 import 强制守护，已接入
> `make check-arch` 与 CI `quality` job。同文件另有两条规则：
> `TestArchInternalDoesNotImportCmd`（`internal/` 不得依赖 `cmd/`）与
> `TestArchInjectionPointsRegistered`（`pkg` 内的包级注入点必须在白名单登记，
> 防止"全局状态"无声扩散）。

`pkg/` 需要接收产品层实现（如路径检查、事件发布）时，做法是**在 `pkg` 内定义最小
接口**，由 `internal/` 的具体类型结构化满足，而不是反向 import。现有示例：

| `pkg` 内接口 | `internal/` 实现方 | 注入点 |
|---|---|---|
| `pkg/tool/builtin.PathChecker` | `*permission.PathChecker` | `builtin.SetPathChecker` |
| `pkg/tool/builtin.EventPublisher` | `*observability.EventBusAdapter` | `builtin.SetEventBus` |
| `pkg/tool/builtin.TimeoutConfig` | 产品层配置 | `builtin.SetTimeoutConfig` |

> 这三个注入点是包级全局状态，进程内共享，由上面的白名单测试登记。

## 核心数据流

Agent Loop 的四阶段 Pipeline：

```
                  ┌──────────────┐
                  │  PrepareStep  │  ← 构建请求（消息 + 工具定义）
                  └──────┬───────┘
                         ↓
                  ┌──────────────┐
                  │   Execute     │  ← 调用 LLM（流式/非流式）
                  └──────┬───────┘
                         ↓
                  ┌──────────────┐
                  │ ProcessResp  │  ← 解析响应（文本 + 工具调用）
                  └──────┬───────┘
                         ↓
                  ┌──────────────┐
                  │ ExecuteTools │  ← 执行工具 → 结果注入消息 → 回到 1
                  └──────────────┘
```

### 分层架构

| 层 | 说明 | 包 |
|----|------|-----|
| L4 应用层 | CLI / TUI / 嵌入 / 自定义 Channel | `cmd/` |
| L3 扩展层 | Config + Skill + LSP + MCP + Memory | `pkg/` 扩展包 |
| L2 会话层 | 事件溯源 Store + Projection | `pkg/session/` |
| L1 核心层 | Pipeline + TurnD/Instance + Steering + Streaming | `pkg/agent/` |
| L0 基础层 | LLM Types + Provider + Tool + Hook/PubSub | `pkg/llm/` 等 |

## 模块依赖图

```
llm ← tool ← agent ← provider
llm ← hook ← agent
llm ← session ← agent
llm + tool ← lsp
llm + tool ← mcp
```

各模块依赖关系：

- `pkg/llm/` — 无依赖，基础类型
- `pkg/hook/` — 依赖 `pkg/llm`
- `pkg/session/` — 依赖 `pkg/llm`
- `pkg/tool/` — 依赖 `pkg/llm`
- `pkg/agent/` — 依赖 `pkg/llm` + `pkg/hook` + `pkg/session` + `pkg/tool`
- `pkg/provider/` — 依赖 `pkg/llm`（可与 agent 并行开发）
- `pkg/lsp/` — 依赖 `pkg/llm` + `pkg/tool`
- `pkg/mcp/` — 依赖 `pkg/llm` + `pkg/tool`
- `pkg/memory/` — 依赖 `pkg/agent`（build tag: `memory`）
- `pkg/config/` + `pkg/skill/` — 依赖 `pkg/agent`
- `cmd/basework/` — 依赖所有 `pkg/`
- `internal/*` — 依赖 `pkg/` 各模块

## API 兼容性分层

| 层级 | 包 | 兼容性承诺 |
|------|----|-----------|
| 核心 API | `pkg/llm/`, `pkg/tool/`, `pkg/session/`, `pkg/agent/` | **SemVer 严格兼容**：不删除/修改导出类型签名 |
| 扩展 API | `pkg/provider/`, `pkg/hook/`, `pkg/mcp/`, `pkg/lsp/` | **SemVer 次版本兼容**：可新增，不可删除 |
| 内部实现 | `internal/*` | **无兼容性承诺**：随时可重构 |

**例外（不计入兼容性承诺）**：`pkg/` 中形参或返回类型**直接引用 `internal/` 类型**的
导出符号。这类签名外部使用者无法构造（外部 module 不能 import `basework/internal/*`），
因此对外本来不可用；修正它们时的签名变更不算破坏性变更。

> 实际案例：`pkg/tool/builtin.SetEventBus` 原签名为
> `SetEventBus(*observability.EventBus)`，2026-09 的架构边界清理将其改为
> `SetEventBus(EventPublisher)`（接口在 `pkg` 内定义）。见 `CHANGELOG.md`。

### 扩展原则

```go
// ✅ 正确：通过 Option 模式扩展，不修改接口签名
func WithCompaction(cfg CompactionConfig) agent.Option { ... }

// ❌ 错误：修改 Agent 接口签名
type Agent interface {
    Run(ctx, req) Response        // 不可改
}
```

### 废弃策略

1. `// Deprecated: 使用 Xxx 替代。将在 Phase N+2 移除。`
2. 至少保留 2 个 Phase 的开发周期
3. 移除前在 `CHANGELOG.md` 中记录

## 扩展机制

### Hook（生命周期钩子）

拦截 agent 生命周期事件，支持 PreToolUse、PostToolUse、PreStep 等 Hook 点：

```go
type MyHook struct{ hook.NopHook }  // 嵌入 NopHook 获得默认空实现

func (h *MyHook) BeforeTool(call llm.ToolCall) (*llm.ToolCall, error) {
    // 修改或拒绝工具调用
    return &call, nil
}
```

### Plugin（插件）

在初始化时扩展 agent 能力，支持生命周期回调：

```go
type MyPlugin struct{}

func (p *MyPlugin) Name() string              { return "my-plugin" }
func (p *MyPlugin) Init(a agent.Agent) error  { return nil }
func (p *MyPlugin) Shutdown(ctx context.Context) error { return nil }
```

### Skill（技能）

基于 Markdown 的技能文件，YAML frontmatter 定义元数据，通过 `pkg/skill/` 加载注入指令：

```markdown
---
name: code-reviewer
description: "代码审查最佳实践"
---
## Instructions
审查代码时关注：正确性、性能、安全性、代码风格。
```

## 关键技术选型

| 技术 | 用途 | 说明 |
|------|------|------|
| Go 1.26+ | 开发语言 | 泛型、`log/slog`、`net/http` 增强 |
| 标准库优先 | 核心依赖 | HTTP/JSON/SSE 自实现；`pkg/` 层第三方依赖限定在下方清单内 |
| `modernc.org/sqlite` | 持久化 | 纯 Go SQLite，`sqlite` / `memory` build tag 控制 |
| `golang.org/x/image` | WebP 解码 + 图片缩放 | `pkg/llm/image.go`（多模态图片输入） |
| `golang.org/x/oauth2` | OAuth 设备授权 | `pkg/provider/copilot.go`（GitHub Copilot 登录） |
| `github.com/golang/snappy` | 事件压缩 | `pkg/session/compress.go`，`sqlite` build tag 控制 |
| cobra | CLI 框架 | 子命令、标志解析 |
| Bubble Tea + Lip Gloss + Glamour | TUI | Phase 18 引入，可独立于 `pkg/` 使用 |

## 关键技术决策

### 为什么用标准库优先？

核心框架（`pkg/` 层）尽量不依赖第三方库，确保最小依赖和长期可维护性。第三方依赖主要
出现在终端产品（`internal/`、`cmd/`）中。

**`pkg/` 层当前的全部第三方依赖**——权威清单由 `make deps` 生成在
[`docs/DEPGRAPH.md`](DEPGRAPH.md)，下表必须与其一致：

| 依赖 | 引入位置 | 构建约束 |
|------|---------|---------|
| `golang.org/x/image` | `pkg/llm` | 默认构建 |
| `golang.org/x/oauth2` | `pkg/provider` | 默认构建 |
| `modernc.org/sqlite` | `pkg/session`、`pkg/memory` | `sqlite` / `memory` |
| `github.com/golang/snappy` | `pkg/session` | `sqlite` |

白名单定义在 `scripts/gendeps/main.go` 的 `approvedPkgDeps`：新增 `pkg/` 层第三方依赖
必须同时改白名单与本表，否则 `make deps` 会以非零退出，CI 也会失败。

> 这张表来自一次纠错：本文档此前声称 `pkg/` 层「不依赖任何第三方库」、
> 后又称「唯一例外是 `golang.org/x/image`」，两次都与代码不符（实际有 4 个）。
> 结构性断言和数字一样会漂移，因此现在由依赖图生成器做机器核对。

**已知偏差（待评估）**：`golang.org/x/image` 与 `golang.org/x/oauth2` 没有 build tag
隔离，默认构建即引入；而 `modernc.org/sqlite`、`github.com/golang/snappy` 都以 tag
隔离。前两者分别只在「使用多模态输入」「使用 Copilot」时才需要，理论上同样可以按
tag 隔离或改为接口注入。记录见 [`docs/adr/0002`](adr/0002-decouple-pkg-from-internal.md)。

### 为什么用事件溯源？

会话存储采用事件溯源架构（Event Sourcing），每个事件不可变追加，支持回放、投影和审计。相比状态快照，事件溯源更灵活，可支持多种查询模式。

### 为什么请求必须能由日志重建？

光有事件溯源还不够：事件日志是历史的来源，但**请求**此前并不完全来自日志。
`pkg/agent/pipeline.go` 里曾有四条绕过日志的路径——配置里的 system prompt、
`Drain()` 之后即丢的 steering 消息、可任意改写消息的 hook、以及压缩截断。
前两条会让「模型看到的」与「日志记的」不一致，且事后无法解释模型为什么那样回答。

当前提供的重建能力（有条件成立）是：

```
request == BuildRequestMessages(events)     // 在没有改写请求的 hook 时
```

- system prompt 与 steering 落成 `system.prompt_set` / `steered` 事件；
- 每次请求尝试写 `request.built`（条数、哈希、来源）；写入失败只记录日志并继续；
- `agent.CheckRequestInvariant` 可随时校验上面这个等式。

`CheckRequestInvariant` 当前供诊断和测试使用，运行时未调用；Hook 改写以及 Provider 传输层变换不受该重建等式保证。严格审计是 REL-004 的未来任务，见 [任务看板](TASKS.md)。

### 为什么会话数据要带格式版本？

同一份事件日志在不同投影规则下会投影出**不同历史**。没有版本号时，这种错解是静默的。
因此 `pkg/session` 给事件与文件头都带版本（`SchemaVersion`），并维护**相邻迁移链**：
只允许 vN→vN+1 步进，读到更高版本直接返回 `ErrSchemaTooNew` 并拒绝读写。

注意版本号是**能力门槛**，不只是数据变换标记：即便新增事件不改变投影结果（如
`system.prompt_set`），旧版本程序也会忽略它、改用配置，从而构造出与写入方意图不同的
请求。这种情况同样要升版本。

### 为什么每个 pkg 包必须有集中契约？

可嵌入库需要说明用途、配置、扩展点、Model Experience 和 Known Limitations。
包契约统一存放在 [reference/pkg](reference/README.md)，源码目录不再放 README。
`scripts/doccheck` 从实际 Go 包扫描对应文档并检查五节，布局决定见 [ADR 0004](adr/0004-centralize-docs-and-task-tracking.md)。

### 为什么用 Option 模式？

所有配置通过函数式 Option 模式传入，保证接口签名稳定，同时支持无限扩展。新增功能只需新增 Option 函数，无需修改已有接口。