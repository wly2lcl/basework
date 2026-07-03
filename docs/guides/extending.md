# 扩展指南

basework 提供多层扩展体系：**Hook**（生命周期钩子）、**Plugin**（插件）、**Skill**（技能）、**自定义 Provider** 和 **自定义 Tool**。本文档逐一介绍。

---

## Hook 扩展

Hook 允许你在 agent 生命周期的关键节点插入自定义逻辑——拦截、修改或记录 LLM 请求/响应和工具调用。

### Hook 接口

```go
type Hook interface {
    BeforeLLM(messages []llm.ChatMessage) ([]llm.ChatMessage, error)
    AfterLLM(resp *llm.Response, err error)
    BeforeTool(call llm.ToolCall) (*llm.ToolCall, error)
    AfterTool(call llm.ToolCall, result *tool.Result, err error)
}
```

| 方法 | 时机 | 用途 |
|------|------|------|
| `BeforeLLM` | 发送给 LLM 之前 | 修改消息、注入上下文、过滤敏感信息 |
| `AfterLLM` | LLM 响应之后 | 记录响应、统计用量 |
| `BeforeTool` | 执行工具之前 | 验证参数、拒绝危险操作 |
| `AfterTool` | 工具执行之后 | 记录结果、后处理 |

### 使用 NopHook 简化实现

嵌入 `hook.NopHook` 获得所有方法的默认空实现，只需覆盖你关心的方法：

```go
import (
    "github.com/wly2lcl/basework/pkg/hook"
    "github.com/wly2lcl/basework/pkg/llm"
)

// 安全拦截器：阻止危险命令
type SafetyHook struct {
    hook.NopHook
}

func (h *SafetyHook) BeforeTool(call llm.ToolCall) (*llm.ToolCall, error) {
    if call.Name == "bash" {
        // 检查是否包含危险命令
        if strings.Contains(call.ArgsJSON, "rm -rf") ||
           strings.Contains(call.ArgsJSON, "sudo") {
            return nil, fmt.Errorf("安全策略: 危险命令已拒绝")
        }
    }
    return &call, nil
}
```

### 使用 FuncHook 快速创建

无需定义新类型，直接用函数：

```go
import "github.com/wly2lcl/basework/pkg/hook"

rateLimitHook := &hook.FuncHook{
    BeforeLLMFn: func(messages []llm.ChatMessage) ([]llm.ChatMessage, error) {
        log.Printf("LLM 请求: %d 条消息", len(messages))
        return messages, nil
    },
    AfterToolFn: func(call llm.ToolCall, result *tool.Result, err error) {
        log.Printf("工具 %s 执行完成 (错误=%v)", call.Name, err)
    },
}
```

### 注册 Hook

```go
a, err := agent.New(
    agent.WithModel(model),
    agent.WithHook(&SafetyHook{}),
    agent.WithHook(rateLimitHook),
)
```

Hook 按注册顺序执行：`BeforeLLM` 和 `BeforeTool` 正向链式传递，`AfterLLM` 和 `AfterTool` 逆序执行。

---

## Plugin 扩展

Plugin 提供更完整的生命周期管理，适合需要初始化和清理的资源。

### Plugin 接口

```go
type Plugin interface {
    Name() string
    Initialize(ctx context.Context, agent Agent) error
    Shutdown(ctx context.Context) error
}
```

### 示例：数据库插件

```go
package main

import (
    "context"
    "database/sql"

    "github.com/wly2lcl/basework/pkg/agent"
)

type DBPlugin struct {
    db *sql.DB
}

func (p *DBPlugin) Name() string { return "database" }

func (p *DBPlugin) Initialize(ctx context.Context, a agent.Agent) error {
    var err error
    p.db, err = sql.Open("sqlite3", "./data.db")
    return err
}

func (p *DBPlugin) Shutdown(ctx context.Context) error {
    return p.db.Close()
}

// 注册插件
a, err := agent.New(
    agent.WithModel(model),
    agent.WithPlugin(&DBPlugin{}),
)
```

---

## Skill 扩展

Skill 是基于 Markdown 的技能定义文件，通过 YAML frontmatter 定义元数据。系统会自动将加载的技能注入到 system prompt 中。

### Skill 文件格式

```markdown
---
name: code-reviewer
description: "代码审查最佳实践"
tags: ["code-review", "best-practices"]
---

## Instructions

审查代码时关注：
1. 正确性和边界情况
2. 性能影响
3. 安全性
4. 代码风格和可读性
```

### 加载 Skill

```go
import "github.com/wly2lcl/basework/pkg/skill"

// 从文件加载技能
skills, err := skill.LoadFromFile("skills/code-reviewer.md")

// 加载目录下所有技能
skills, err := skill.LoadFromDir("skills/")

// 生成 XML 格式注入 system prompt
xml := skill.ToPromptXML(skills)
```

### Skill 结构

```go
type Skill struct {
    Name         string            // 技能名称
    Description  string            // 技能描述
    Instructions string            // 指令内容
    FilePath     string            // 文件路径
    Builtin      bool              // 是否为内置技能
    Metadata     map[string]string // 自定义元数据
}
```

### 在 agent 中使用 Skill

```go
// 加载技能并注入到 system prompt
skills, _ := skill.LoadFromDir("skills/")
skillXML := skill.ToPromptXML(skills)

a, _ := agent.New(
    agent.WithModel(model),
    agent.WithSystemPrompt(fmt.Sprintf(
        "你是一个有帮助的助手。\n\n可用技能：\n%s", skillXML,
    )),
)
```

---

## 自定义 Provider

实现 `llm.Model` 接口即可创建自定义 Provider，并通过 Provider 工厂注册。

### Model 接口

```go
type Model interface {
    ID() string
    Generate(ctx context.Context, req *Request) (*Response, error)
    Stream(ctx context.Context, req *Request) (<-chan StreamEvent, error)
    Supports(cap Capability) bool
}
```

### 示例：自定义本地 Provider

```go
package myprovider

import (
    "context"
    "fmt"

    "github.com/wly2lcl/basework/pkg/llm"
)

// LocalModel 使用本地推理引擎
type LocalModel struct {
    modelID string
    endpoint string
}

func (m *LocalModel) ID() string { return m.modelID }

func (m *LocalModel) Generate(ctx context.Context, req *llm.Request) (*llm.Response, error) {
    // 实现与本地推理服务的通信
    // ...

    return &llm.Response{
        Message: llm.ChatMessage{
            Role: llm.RoleAssistant,
            Content: []llm.ContentPart{
                {Type: llm.ContentTypeText, Text: responseText},
            },
        },
        Usage: llm.Usage{
            PromptTokens:     promptTokens,
            CompletionTokens: completionTokens,
        },
    }, nil
}

func (m *LocalModel) Stream(ctx context.Context, req *llm.Request) (<-chan llm.StreamEvent, error) {
    // 实现流式生成
    ch := make(chan llm.StreamEvent)
    go func() {
        defer close(ch)
        // 逐 token 发送事件
    }()
    return ch, nil
}

func (m *LocalModel) Supports(cap llm.Capability) bool {
    switch cap {
    case llm.CapTools, llm.CapStreaming:
        return true
    case llm.CapVision, llm.CapJSON:
        return false
    }
    return false
}

// 创建函数
func New(modelID, endpoint string) *LocalModel {
    return &LocalModel{modelID: modelID, endpoint: endpoint}
}
```

### 注册到 Factory（可选）

```go
import "github.com/wly2lcl/basework/pkg/provider"

// 在 init 中注册
func init() {
    provider.Register("local", func(cfg provider.Config) (llm.Model, error) {
        return myprovider.New(cfg.ModelID, cfg.BaseURL), nil
    })
}
```

---

## 自定义 Tool

实现 `tool.Tool` 接口即可创建自定义工具。

### Tool 接口

```go
type Tool interface {
    Name() string
    Description() string
    Parameters() json.RawMessage // JSON Schema
    Execute(ctx context.Context, args json.RawMessage) (*Result, error)
}
```

### 示例：天气查询工具

```go
package mytools

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"

    "github.com/wly2lcl/basework/pkg/tool"
)

type WeatherTool struct{}

func (t *WeatherTool) Name() string { return "weather" }

func (t *WeatherTool) Description() string {
    return "查询指定城市的天气"
}

func (t *WeatherTool) Parameters() json.RawMessage {
    return json.RawMessage(`{
        "type": "object",
        "properties": {
            "city": {
                "type": "string",
                "description": "城市名称"
            }
        },
        "required": ["city"]
    }`)
}

func (t *WeatherTool) Execute(ctx context.Context, args json.RawMessage) (*tool.Result, error) {
    var params struct {
        City string `json:"city"`
    }
    if err := json.Unmarshal(args, &params); err != nil {
        return &tool.Result{Content: "参数解析失败", IsError: true}, nil
    }

    // 调用天气 API
    resp, err := http.Get(fmt.Sprintf("https://api.weather.com/v1/%s", params.City))
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    return &tool.Result{Content: fmt.Sprintf("城市 %s 的天气数据已获取", params.City)}, nil
}
```

### 注册工具

```go
import (
    "github.com/wly2lcl/basework/pkg/agent"
    "github.com/wly2lcl/basework/pkg/tool/builtin"
    "github.com/your/mytools"
)

a, err := agent.New(
    agent.WithModel(model),
    agent.WithTools(
        builtin.All()...,             // 内置工具
        &mytools.WeatherTool{},        // 自定义工具
    ),
)
```

### 使用 Tool Registry

```go
import "github.com/wly2lcl/basework/pkg/tool"

registry := tool.NewRegistry()
registry.Register(&WeatherTool{})
registry.Register(&MyTool{})

// 禁用特定工具
registry.Disable("bash")

// 获取工具定义（给 LLM）
defs := registry.Materialize()

// 克隆 registry（子 agent 隔离）
clone := registry.Clone()
```

---

## 最佳实践

1. **Hook 优先** — 简单的拦截/记录用 Hook，复杂的生命周期管理用 Plugin
2. **NopHook 嵌入** — 实现 Hook 时嵌入 `hook.NopHook`，只覆盖需要的方法
3. **FuncHook 快捷** — 小型 Hook 使用 `hook.FuncHook` 避免定义新类型
4. **Tool 错误处理** — `Execute` 返回 `tool.Result` 时设置 `IsError: true` 让 LLM 感知错误
5. **Tool JSON Schema** — `Parameters()` 返回精确的 JSON Schema，帮助 LLM 正确传参
6. **Skill 粒度** — 每个技能聚焦一个领域，便于组合和复用
7. **Provider 优雅降级** — 自定义 Provider 遇到错误时返回 error，不要静默失败
8. **Plugin 清理** — 确保 `Shutdown` 正确释放资源，agent 的 `Close()` 会自动调用

---

## 相关文档

- [嵌入指南](embedder-guide.md) — 如何将 basework 嵌入到应用中
- [配置参考](configuration.md) — 配置文件和环境变量
- [设计文档](../DESIGN.md) — 架构和接口定义