# 嵌入指南

本文档介绍如何将 basework 作为 Go 库嵌入到你的应用中。

---

## 安装依赖

```bash
go get github.com/wly2lcl/basework
```

确保 Go 版本 >= 1.26。

---

## 配置 Provider

basework 支持以下 LLM Provider：

| Provider     | 类型值              | 默认端点 |
|-------------|---------------------|----------|
| OpenAI      | `"openai"`          | `https://api.openai.com/v1` |
| Anthropic   | `"anthropic"`       | `https://api.anthropic.com` |
| Gemini      | `"gemini"`          | `https://generativelanguage.googleapis.com` |
| DeepSeek    | `"deepseek"`        | `https://api.deepseek.com/v1` |
| Groq        | `"groq"`            | `https://api.groq.com/openai/v1` |
| Together    | `"together"`        | `https://api.together.xyz/v1` |
| OpenRouter  | `"openrouter"`      | `https://openrouter.ai/api/v1` |
| xAI         | `"xai"`             | `https://api.x.ai/v1` |
| Mistral     | `"mistral"`         | `https://api.mistral.ai/v1` |
| OpenAI 兼容 | `"openai-compat"`   | 自定义 |
| OpenCode Zen | `"opencode"`        | `https://opencode.ai/zen/v1` |
| Amazon Bedrock | `"bedrock"`       | AWS Converse API |
| Azure OpenAI | `"azure"`           | `https://{resource}.openai.azure.com` |
| GitHub Copilot | `"copilot"`       | `https://api.githubcopilot.com` |
| Ollama | `"ollama"`           | `http://localhost:11434/v1` |

### OpenAI

```go
import "github.com/wly2lcl/basework/pkg/provider"

model, err := provider.Create(provider.Config{
    Type:    "openai",
    APIKey:  os.Getenv("OPENAI_API_KEY"),
    ModelID: "gpt-4",
})
```

### Anthropic

```go
model, err := provider.Create(provider.Config{
    Type:    "anthropic",
    APIKey:  os.Getenv("ANTHROPIC_API_KEY"),
    ModelID: "claude-3-5-sonnet-20241022",
})
```

### Gemini

```go
model, err := provider.Create(provider.Config{
    Type:    "gemini",
    APIKey:  os.Getenv("GOOGLE_API_KEY"),
    ModelID: "gemini-2.0-flash",
})
```

### OpenAI 兼容（DeepSeek、Groq 等）

```go
model, err := provider.Create(provider.Config{
    Type:    "deepseek",       // 或 "groq"、"openai-compat" 等
    APIKey:  os.Getenv("DEEPSEEK_API_KEY"),
    ModelID: "deepseek-chat",
})
```

使用 `"openai-compat"` 类型时需指定自定义端点：

```go
model, err := provider.Create(provider.Config{
    Type:    "openai-compat",
    APIKey:  os.Getenv("CUSTOM_API_KEY"),
    BaseURL: "https://your-api.example.com/v1",
    ModelID: "your-model",
})
```

---

## 创建 Agent

`agent.New()` 使用函数式选项模式进行配置：

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    "github.com/wly2lcl/basework/pkg/agent"
    "github.com/wly2lcl/basework/pkg/provider"
    "github.com/wly2lcl/basework/pkg/tool/builtin"
)

func main() {
    // 1. 创建 LLM 模型
    model, err := provider.Create(provider.Config{
        Type:    "openai",
        APIKey:  os.Getenv("OPENAI_API_KEY"),
        ModelID: "gpt-4",
    })
    if err != nil {
        log.Fatal(err)
    }

    // 2. 创建 agent
    a, err := agent.New(
        agent.WithModel(model),
        agent.WithSystemPrompt("你是一个有帮助的编程助手。"),
        agent.WithTools(builtin.All()...),       // 注册内置工具
        agent.WithMaxSteps(25),                   // 最大执行步数
    )
    if err != nil {
        log.Fatal(err)
    }
    defer a.Close()

    // 3. 发送消息
    resp, err := a.HandleMessage(context.Background(), "创建一个 hello.go 文件")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(resp.Message.Content[0].Text)
}
```

### 可用选项

| 选项函数 | 说明 |
|---------|------|
| `WithModel(m)` | 设置 LLM 模型（必需） |
| `WithTools(t...)` | 注册工具 |
| `WithToolRegistry(r)` | 设置工具注册表（覆盖 WithTools） |
| `WithSystemPrompt(p)` | 设置系统提示词 |
| `WithSession(s)` | 设置会话存储 |
| `WithHook(h...)` | 注册生命周期钩子 |
| `WithMaxSteps(n)` | 设置最大执行步数（默认 25） |
| `WithPlugin(p...)` | 注册插件 |
| `WithObserver(o)` | 设置运行观测器 |
| `WithCallback(cb)` | 设置生命周期回调 |

---

> 当前 `examples/embed` 引用了 `internal/runtime`，只适用于仓库内部；外部 Go module 应直接使用 `pkg/agent` 等公共接口。外部模块的可运行示例由 QA-001 补齐。

## 处理会话

basework 使用事件溯源（Event Sourcing）管理会话。创建带会话能力的 agent：

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    "github.com/wly2lcl/basework/pkg/agent"
    "github.com/wly2lcl/basework/pkg/provider"
    "github.com/wly2lcl/basework/pkg/session"
)

func main() {
    model, _ := provider.Create(provider.Config{
        Type:   "anthropic",
        APIKey: os.Getenv("ANTHROPIC_API_KEY"),
    })

    // 创建 JSONL 持久化会话存储
    store, err := session.NewJSONLStore("sessions/my-session.jsonl")
    if err != nil {
        log.Fatal(err)
    }
    defer store.Close()

    a, err := agent.New(
        agent.WithModel(model),
        agent.WithSession(store),
        agent.WithSystemPrompt("你是一个有帮助的助手。"),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer a.Close()

    // 多轮对话自动保存到会话存储
    resp1, _ := a.HandleMessage(context.Background(), "我的名字是 Alice")
    fmt.Println(resp1.Message.Content[0].Text)

    resp2, _ := a.HandleMessage(context.Background(), "我叫什么名字？")
    fmt.Println(resp2.Message.Content[0].Text) // 会记住 Alice
}
```

### Session Store 接口

```go
type Store interface {
    AppendEvent(event Event) error
    Events(filter EventFilter) ([]Event, error)
    Create(opts CreateOpts) (*Info, error)
    Get(id string) (*Info, error)
    List(filter ListFilter) ([]*Info, error)
    Delete(id string) error
    Messages() ([]llm.ChatMessage, error)
}
```

### 内存会话存储（测试用）

```go
import "github.com/wly2lcl/basework/pkg/session"

store := session.NewMemoryStore()
```

### JSONL 文件存储（持久化）

```go
store, err := session.NewJSONLStore("path/to/session.jsonl")
```

---

## 错误处理

```go
resp, err := a.HandleMessage(ctx, input)
if err != nil {
    switch {
    case errors.Is(err, agent.ErrMaxStepsExceeded):
        // agent 超过最大执行步数
        log.Println("任务过于复杂，需要更多步数")
    default:
        // 其他错误（网络、API 等）
        log.Printf("消息处理失败: %v", err)
    }
    return
}

// 检查工具调用结果中的错误
for _, tc := range resp.ToolCalls {
    if tc.Err != nil {
        log.Printf("工具 %s 执行失败: %v", tc.Call.Name, tc.Err)
    }
}

// 查看 token 用量
log.Printf("Token 用量: %+v", resp.Usage)
```

---

## 高级配置

### 自定义 Hook

Hook 可以拦截和修改 agent 生命周期的各个阶段：

```go
import (
    "github.com/wly2lcl/basework/pkg/hook"
    "github.com/wly2lcl/basework/pkg/llm"
    "github.com/wly2lcl/basework/pkg/tool"
)

// 审计日志 Hook
type AuditHook struct {
    hook.NopHook // 嵌入空实现，只需覆盖关心的方法
}

func (h *AuditHook) BeforeTool(call llm.ToolCall) (*llm.ToolCall, error) {
    log.Printf("工具调用: %s(%s)", call.Name, call.ArgsJSON)
    return &call, nil
}

// 注册到 agent
a, err := agent.New(
    agent.WithModel(model),
    agent.WithHook(&AuditHook{}),
)
```

### LSP 代码智能集成

```go
import "github.com/wly2lcl/basework/pkg/lsp"

// 创建 LSP Manager
mgr := lsp.NewManager(lsp.Config{
    Servers: map[string]lsp.ServerConfig{
        "go": {Command: "gopls"},
        "typescript": {
            Command: "typescript-language-server",
            Args:    []string{"--stdio"},
        },
    },
})

// 启动
mgr.Start(ctx, "/path/to/workspace")

// 使用
defs, _ := mgr.Definition(ctx, "main.go", lsp.Position{Line: 10, Character: 5})
hover, _ := mgr.Hover(ctx, "main.go", lsp.Position{Line: 10, Character: 5})
diags, _ := mgr.Diagnostics("main.go")
```

### MCP 外部工具集成

```go
import "github.com/wly2lcl/basework/pkg/mcp"

// 创建 MCP Manager
mgr := mcp.NewManager()

// 连接到 MCP 服务器（stdio 传输）
err := mgr.Connect(ctx, "github", mcp.ServerConfig{
    Command: "npx",
    Args:    []string{"-y", "@modelcontextprotocol/server-github"},
    Env:     map[string]string{"GITHUB_TOKEN": os.Getenv("GITHUB_TOKEN")},
})

// 获取 MCP 暴露的工具列表
tools := mgr.Tools()

// 调用 MCP 工具
result, err := mgr.CallTool(ctx, "github", "search_repositories", argsJSON)
```

---

## 完整示例：HTTP Server + basework Agent

```go
package main

import (
    "context"
    "encoding/json"
    "log"
    "net/http"
    "os"

    "github.com/wly2lcl/basework/pkg/agent"
    "github.com/wly2lcl/basework/pkg/provider"
    "github.com/wly2lcl/basework/pkg/tool/builtin"
)

type ChatRequest struct {
    Message string `json:"message"`
}

type ChatResponse struct {
    Reply   string `json:"reply"`
    Tokens  int    `json:"tokens"`
}

var chatAgent agent.Agent

func init() {
    model, err := provider.Create(provider.Config{
        Type:    "anthropic",
        APIKey:  os.Getenv("ANTHROPIC_API_KEY"),
        ModelID: "claude-3-5-sonnet-20241022",
    })
    if err != nil {
        log.Fatal(err)
    }

    chatAgent, err = agent.New(
        agent.WithModel(model),
        agent.WithTools(builtin.All()...),
        agent.WithSystemPrompt("你是一个有帮助的编程助手。"),
        agent.WithMaxSteps(25),
    )
    if err != nil {
        log.Fatal(err)
    }
}

func handleChat(w http.ResponseWriter, r *http.Request) {
    var req ChatRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    resp, err := chatAgent.HandleMessage(r.Context(), req.Message)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    json.NewEncoder(w).Encode(ChatResponse{
        Reply:  resp.Message.Content[0].Text,
        Tokens: resp.Usage.TotalTokens,
    })
}

func main() {
    defer chatAgent.Close()

    http.HandleFunc("/chat", handleChat)
    log.Println("Server started on :8080")
    log.Fatal(http.ListenAndServe(":8080", nil))
}
```

---

## 相关文档

- [配置参考](configuration.md) — 配置文件和环境变量
- [扩展指南](extending.md) — Hook、Plugin、Skill 扩展
- [设计文档](../DESIGN.md) — 架构和接口定义