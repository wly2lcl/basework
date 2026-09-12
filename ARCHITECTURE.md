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
> 该约束由测试 `TestPkgDoesNotImportInternal`（`tests/pkg_no_internal_test.go`，
> 用 `go/parser` 静态扫描 `pkg/` 的 import）强制守护，已接入 `make check-arch`
> 与 CI `quality` job。

`pkg/` 需要接收产品层实现（如路径检查、事件发布）时，做法是**在 `pkg` 内定义最小
接口**，由 `internal/` 的具体类型结构化满足，而不是反向 import。现有示例：

| `pkg` 内接口 | `internal/` 实现方 | 注入点 |
|---|---|---|
| `pkg/tool/builtin.PathChecker` | `*permission.PathChecker` | `builtin.SetPathChecker` |
| `pkg/tool/builtin.EventPublisher` | `*observability.EventBusAdapter` | `builtin.SetEventBus` |

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
| 标准库优先 | 核心依赖 | HTTP/JSON/SSE 自实现；`pkg/` 层第三方依赖仅限明确批准的例外（见下） |
| `modernc.org/sqlite` | 持久化 | 纯 Go SQLite，build tag 控制 |
| `golang.org/x/image` | WebP 解码 + 图片缩放 | `pkg/llm/image.go`（多模态图片输入），见下方例外说明 |
| cobra | CLI 框架 | 子命令、标志解析 |
| Bubble Tea + Lip Gloss + Glamour | TUI | Phase 18 引入，可独立于 `pkg/` 使用 |

## 关键技术决策

### 为什么用标准库优先？

核心框架（`pkg/` 层）尽量不依赖第三方库，确保最小依赖和长期可维护性。第三方依赖主要
出现在终端产品（`internal/`、`cmd/`）或可选模块（`pkg/memory/`）中。

**已批准的例外**（唯一一个）：`golang.org/x/image`，用于 `pkg/llm/image.go` 的 WebP
解码与图片缩放（多模态输入）。理由：标准库无 WebP 解码器，自实现不现实；该包由 Go
官方团队维护，无传递依赖负担。新增其他 `pkg/` 层第三方依赖需要在此登记。

### 为什么用事件溯源？

会话存储采用事件溯源架构（Event Sourcing），每个事件不可变追加，支持回放、投影和审计。相比状态快照，事件溯源更灵活，可支持多种查询模式。

### 为什么用 Option 模式？

所有配置通过函数式 Option 模式传入，保证接口签名稳定，同时支持无限扩展。新增功能只需新增 Option 函数，无需修改已有接口。