# Basework 架构概览

## 系统架构

```
┌─────────────────────────────────────────────────────────┐
│  cmd/basework/ — CLI 入口（cobra + TUI）                 │
├─────────────────────────────────────────────────────────┤
│  internal/ — 终端产品专用逻辑                            │
│  ├── tui/          终端 UI（Bubble Tea）                 │
│  ├── compaction/   上下文压缩                            │
│  ├── retry/        重试机制                              │
│  ├── permission/   权限系统                              │
│  ├── subagent/     子代理                                │
│  ├── loopdetect/   循环检测                              │
│  ├── observability/ 可观测性                             │
│  ├── cache/        Prompt 缓存                          │
│  ├── oauth/        OAuth 认证                           │
│  └── tools/        增强工具                              │
├─────────────────────────────────────────────────────────┤
│  pkg/ — 核心框架（可嵌入，稳定 API）                      │
│  ├── llm/          类型系统 + 错误分类                   │
│  ├── tool/         工具接口 + 注册表 + 6 个内置工具      │
│  ├── hook/         Hook 系统 + PubSub                   │
│  ├── session/      会话管理 + 事件溯源                   │
│  ├── agent/        Agent 循环 + 流式处理                 │
│  ├── provider/     Provider 工厂（15+ 个 Provider）       │
│  ├── lsp/          LSP 集成                              │
│  ├── mcp/          MCP 集成                              │
│  ├── memory/       记忆系统（FTS5, build tag）           │
│  ├── config/       配置管理（CoW 模式）                  │
│  └── skill/        技能加载                              │
└─────────────────────────────────────────────────────────┘
```

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
| 标准库优先 | 核心依赖 | HTTP/JSON/SSE 自实现，无第三方依赖 |
| `modernc.org/sqlite` | 持久化 | 纯 Go SQLite，build tag 控制 |
| cobra | CLI 框架 | 子命令、标志解析 |
| Bubble Tea + Lip Gloss + Glamour | TUI | Phase 18 引入，build tag 控制 |

## 关键技术决策

### 为什么用标准库优先？

核心框架（`pkg/` 层）不依赖任何第三方库，确保最小依赖和长期可维护性。第三方依赖仅在终端产品（`internal/`、`cmd/`）或可选模块（`pkg/memory/`）中使用。

### 为什么用事件溯源？

会话存储采用事件溯源架构（Event Sourcing），每个事件不可变追加，支持回放、投影和审计。相比状态快照，事件溯源更灵活，可支持多种查询模式。

### 为什么用 Option 模式？

所有配置通过函数式 Option 模式传入，保证接口签名稳定，同时支持无限扩展。新增功能只需新增 Option 函数，无需修改已有接口。