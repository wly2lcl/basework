# Basework

Go 语言通用 AI Agent 编程框架。构建能使用工具、调用多模型 LLM、通过插件扩展的 AI Agent —— 可嵌入 CLI、Web 服务或自定义应用。

```go
import "github.com/wly2lcl/basework/pkg/agent"
```

## 特性

- **最小核心** — Agent loop + provider + tool + session < 3000 行
- **可嵌入** — `agent.New(WithModel(...), WithTools(...))` 即可使用
- **可扩展** — Hook + Plugin + Skill 三层扩展体系
- **统一类型** — 一套 `ChatMessage`/`ToolCall` 类型贯穿始终
- **可观测** — 事件溯源 Session + PubSub 事件总线
- **可选复杂度** — Build Tag 控制可选模块（memory, tui）
- **30+ LLM 提供商** — OpenAI、Anthropic、Gemini 及所有 OpenAI 兼容 API
- **LSP 集成** — 通过 Language Server Protocol 获取代码智能（Go、TypeScript、Python）
- **MCP 支持** — Model Context Protocol 外部工具服务器
- **会话持久化** — 事件溯源，支持回放

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
┌──────────────────────────────────────────────────────┐
│  L4: Application (应用层)                              │
│  CLI / TUI / 嵌入 / 自定义 Channel                    │
├──────────────────────────────────────────────────────┤
│  L3: Extension (扩展层)                               │
│  Config + Skill + LSP + MCP + Memory                 │
├──────────────────────────────────────────────────────┤
│  L2: Session (会话层)                                  │
│  Event Sourcing Store + Projection                    │
├──────────────────────────────────────────────────────┤
│  L1: Agent Loop (核心层)                               │
│  Pipeline + TurnD/Instance + Steering + Streaming    │
├──────────────────────────────────────────────────────┤
│  L0: Foundation (基础层)                               │
│  LLM Types + Provider + Tool + Hook/PubSub            │
└──────────────────────────────────────────────────────┘
```

### 核心包

| 包 | 说明 |
|---|------|
| `pkg/llm` | 统一类型：`ChatMessage`、`ToolCall`、`Model` 接口 |
| `pkg/tool` | Tool 接口 + Registry（支持 TTL） |
| `pkg/hook` | Hook 生命周期 + PubSub 事件总线 + 权限 |
| `pkg/provider` | Provider 工厂：OpenAI、Anthropic、Gemini、OpenAI 兼容 |
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

```bash
# 启用记忆模块构建
go build -tags memory ./...
```

## 环境要求

- Go 1.26+

## 文档

- [设计文档](docs/DESIGN.md) — 架构、接口定义、设计决策
- [任务清单](docs/TASKS.md) — 实施阶段和任务详情

## 许可证

MIT
