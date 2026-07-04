# Basework

Go 语言 AI Agent 框架与终端产品。既是可嵌入的 Go 库，也是功能完整的终端 AI 编程助手。

```go
import "github.com/wly2lcl/basework/pkg/agent"
```

## 特性

- **双模式** — 嵌入式框架 + 独立终端产品
- **最小核心** — Agent loop + provider + tool + session < 3000 行
- **可嵌入** — `agent.New(WithModel(...), WithTools(...))` 即可使用
- **可扩展** — Hook + Plugin + Skill 三层扩展体系
- **统一类型** — 一套 `ChatMessage`/`ToolCall` 类型贯穿始终
- **可观测** — 事件溯源 Session + PubSub 事件总线 + 结构化日志 + 成本追踪
- **可选复杂度** — Build Tag 控制可选模块（memory, tui, otel）
- **上下文压缩** — 自动摘要、滑动窗口、选择性保留
- **重试机制** — 指数退避、错误分类、可恢复错误自动重试
- **权限系统** — 规则引擎（allow/deny/ask）、YOLO 模式、命令黑名单
- **子代理** — 任务委托、隔离子会话、成本传播
- **循环检测** — SHA-256 签名 + 模式匹配，防止工具调用死循环
- **OAuth 2.0** — PKCE 流程、令牌刷新、凭证安全存储
- **Prompt 缓存** — 自动注入 `cache_control` 标记，减少 50-90% 重复 token 计费（Anthropic/OpenAI/Gemini）
- **命令黑名单** — 12+ 内置危险命令模式 + 自定义扩展，支持交互/YOLO 权限模式
- **10+ LLM 提供商** — OpenAI、Anthropic、Gemini 及所有 OpenAI 兼容 API
- **免费模型** — 默认使用 OpenCode Zen 的 `big-pickle`（免费）
- **LSP 集成** — 通过 Language Server Protocol 获取代码智能（Go、TypeScript、Python）
- **MCP 支持** — Model Context Protocol 外部工具服务器、资源读取、提示模板获取
- **MCP 增强** — 自动重连（指数退避）、Shell 变量展开（`$HOME`/`${VAR}`）
- **会话持久化** — 事件溯源，支持回放
- **会话增强** — SQLite 存储、文件追踪、自动标题、会话队列
- **终端 UI** — Bubble Tea 构建的完整 TUI
- **增强工具** — web_fetch、web_search、todowrite、apply_patch、question
- **15+ LLM 提供商** — 含 Amazon Bedrock、Azure、GitHub Copilot、Ollama

## 安装

```bash
go get github.com/wly2lcl/basework
```

或从源码构建：

```bash
git clone https://github.com/wly2lcl/basework.git
cd basework
make build
```

## 快速开始（CLI）

```bash
# 交互式 agent 会话
basework agent

# 单次提问
basework agent -m "写一个 Go 反转字符串的函数"

# 列出可用模型
basework model list
```

## 快速开始（嵌入使用）

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/wly2lcl/basework/pkg/agent"
    "github.com/wly2lcl/basework/pkg/provider"
    "github.com/wly2lcl/basework/pkg/tool/builtin"
)

func main() {
    // 创建 LLM 提供商
    model, err := provider.Create(provider.Config{
        Type:   "anthropic",
        APIKey: "sk-...",
    })
    if err != nil {
        log.Fatal(err)
    }

    // 创建 agent 并注册工具
    a, err := agent.New(
        agent.WithModel(model),
        agent.WithTools(
            builtin.NewBashTool(),
            builtin.NewReadTool(),
            builtin.NewWriteTool(),
            builtin.NewEditTool(),
            builtin.NewGrepTool(),
            builtin.NewGlobTool(),
        ),
        agent.WithSystemPrompt("你是一个有帮助的编程助手。"),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer a.Close()

    // 发送消息
    resp, err := a.HandleMessage(context.Background(), "创建一个 hello.go 文件")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(resp.Message.Content[0].Text)
}
```

## 架构

```
┌─────────────────────────────────────────────────────────┐
│  cmd/basework/ — CLI 入口（cobra + TUI）                 │
├─────────────────────────────────────────────────────────┤
│  internal/ — 终端产品专用逻辑                            │
│  ├── compaction/     上下文压缩                          │
│  ├── retry/          重试机制                            │
│  ├── permission/     权限系统                            │
│  ├── subagent/       子代理系统                          │
│  ├── loopdetect/     循环检测                            │
│  ├── observability/  可观测性（日志 + 成本追踪）         │
│  ├── oauth/          OAuth 2.0 认证                      │
│  ├── tools/          增强工具（web_fetch/search/todo…）  │
│  └── tui/            终端 UI（Bubble Tea 已实现）         │
├─────────────────────────────────────────────────────────┤
│  pkg/ — 核心框架（可嵌入，稳定 API）                      │
│  ├── llm/          类型系统 + 错误分类                   │
│  ├── tool/         工具接口 + 8 个内置工具               │
│  ├── hook/         Hook 系统 + PubSub                   │
│  ├── session/      会话管理 + 事件溯源                   │
│  ├── agent/        Agent 循环 + 流式处理                 │
│  ├── provider/     Provider 工厂（15+ 个 Provider）      │
│  ├── lsp/          LSP 集成                              │
│  ├── mcp/          MCP 集成                              │
│  ├── memory/       记忆系统（FTS5, build tag）           │
│  ├── config/       配置管理（CoW 模式）                  │
│  └── skill/        技能加载                              │
└─────────────────────────────────────────────────────────┘
```

### 核心包

| 包 | 说明 |
|---|------|
| `pkg/llm` | 统一类型：`ChatMessage`、`ToolCall`、`Model` 接口 |
| `pkg/tool` | Tool 接口 + Registry（支持 TTL） |
| `pkg/hook` | Hook 生命周期 + PubSub 事件总线 + 权限 |
| `pkg/provider` | Provider 工厂：OpenAI、Anthropic、Gemini、OpenCode Zen、Bedrock、Azure、Copilot、Ollama、OpenAI 兼容 |
| `pkg/agent` | Agent 循环、Pipeline、函数式选项 |
| `pkg/session` | 事件溯源会话存储 |
| `pkg/lsp` | LSP 代码智能集成 |
| `pkg/mcp` | MCP 协议外部工具服务器 |
| `pkg/memory` | 可选持久化记忆（build tag: `memory`） |
| `pkg/config` | Copy-on-Write 配置管理 |
| `pkg/skill` | 基于 Markdown 的 Skill 系统 |

## 扩展点

### Hook（生命周期钩子）

拦截 agent 生命周期事件：

```go
type MyHook struct{}

func (h *MyHook) BeforeTool(call llm.ToolCall) (*llm.ToolCall, error) {
    // 修改或拒绝工具调用
    if call.Name == "bash" && strings.Contains(call.ArgsJSON, "rm -rf") {
        return nil, fmt.Errorf("危险命令已拒绝")
    }
    return &call, nil
}

func (h *MyHook) AfterTool(call llm.ToolCall, result *tool.Result, err error) {
    // 观察工具执行结果
    log.Printf("工具 %s 已执行", call.Name)
}

// 嵌入 NopHook 获得默认空实现，只需实现你关心的方法
```

### Plugin（插件）

在初始化时扩展 agent 能力：

```go
type MyPlugin struct{}

func (p *MyPlugin) Name() string { return "my-plugin" }

func (p *MyPlugin) Init(a agent.Agent) error {
    // 注册额外的工具、Hook 等
    return nil
}

func (p *MyPlugin) Shutdown(ctx context.Context) error {
    return nil
}
```

### Skill（技能）

基于 Markdown 的技能文件，YAML frontmatter 定义元数据：

```markdown
---
name: code-reviewer
description: "代码审查最佳实践"
---

## Instructions

审查代码时关注：
1. 正确性和边界情况
2. 性能影响
3. 安全性
4. 代码风格和可读性
```

## 配置

配置文件：`basework.json` 或 `.basework/basework.json`

```json
{
  "model": "claude-3-5-sonnet",
  "provider": "anthropic",
  "api_key": "sk-...",
  "max_steps": 25,
  "system_prompt": "你是一个有帮助的助手。",
  "tools": {
    "bash": true,
    "write": true
  },
  "lsp": {
    "go": { "command": "gopls" },
    "typescript": { "command": "typescript-language-server", "args": ["--stdio"] }
  },
  "mcp": {
    "github": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-github"],
      "env": { "GITHUB_TOKEN": "..." }
    }
  }
}
```

## Build Tag

| Tag | 默认 | 说明 |
|-----|------|------|
| `memory` | 关 | SQLite 持久化记忆 + FTS5 全文搜索 |
| `sqlite` | 关 | SQLite 会话存储（替换 JSONL） |
| `otel` | 关 | OpenTelemetry 追踪导出 |

```bash
# 启用记忆模块构建
go build -tags memory ./...

# 启用 SQLite 会话存储
go build -tags sqlite ./...

# 启用 OpenTelemetry 追踪
go build -tags otel ./...
```

## 环境要求

- Go 1.26+

## 文档

### 快速开始
- [嵌入指南](docs/guides/embedder-guide.md) — 将 basework 嵌入 Go 应用
- [CLI 使用指南](docs/guides/cli-guide.md) — CLI 完整命令参考
- [Provider 配置](docs/guides/provider-guide.md) — Provider 配置、模型选择、故障排查
- [配置参考](docs/guides/configuration.md) — 配置文件、环境变量

### 功能指南
- [权限系统](docs/guides/permission-guide.md) — 权限规则、交互提示、YOLO 模式（Phase 15）
- [子代理](docs/guides/subagent-guide.md) — 任务委托、隔离子会话、成本追踪（Phase 16）
- [终端 UI](docs/guides/tui-guide.md) — TUI 启动、快捷键、主题配置（Phase 18）

### 常见问题
- [FAQ](docs/FAQ.md) — 常见问题解答

### 架构与设计
- [架构概览](ARCHITECTURE.md) — 系统架构、模块依赖、API 兼容性
- [设计文档](docs/DESIGN.md) — 架构设计、接口定义、设计决策
- [项目状态](docs/STATUS.md) — 已完成功能、待完成功能、对比分析

### 开发指南
- [扩展指南](docs/guides/extending.md) — Hook、Plugin、Skill、自定义 Provider/Tool
- [迁移指南](docs/guides/migration.md) — 版本升级、JSONL → SQLite 迁移
- [贡献指南](CONTRIBUTING.md) — 如何贡献代码

### 项目管理
- [任务清单](docs/TASKS.md) — Phase 1-25 完整任务列表
- [路线图](ROADMAP.md) — 高层路线图
- [变更日志](CHANGELOG.md) — 版本变更记录
- [安全策略](SECURITY.md) — 漏洞报告、安全更新
- [行为准则](CODE_OF_CONDUCT.md) — 社区行为准则

## 许可证

MIT
