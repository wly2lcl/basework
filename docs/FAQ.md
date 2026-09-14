# 常见问题解答 (FAQ)

## 一般问题

### basework 是什么？

basework 是一个基于 Go 语言构建的终端 AI 编程助手。它**既是框架也是终端产品**——作为框架，你可以将其嵌入到自己的 Go 项目中，通过 `agent.New` 和 `provider.Create` 等 API 构建自定义的 AI Agent 工作流；作为终端产品，它提供了完整的交互式 Agent 会话和 TUI 模式，可直接在命令行中使用。

### basework 和 opencode/crush 有什么区别？

basework 在设计上与 opencode 和 crush 有相似的目标——提供终端 AI 编程体验。但 basework 的侧重点在于**模块化和可嵌入性**：更清晰的 Provider 抽象层、完整的 Hook 系统、标准化的 Tool 接口，以及纯 Go 实现的零外部依赖核心。你可以把 basework 理解为一个 AI Agent 工具包，而不仅仅是一个 CLI 工具。

### 默认使用什么模型？

basework 默认使用 OpenCode Zen 的 **big-pickle 免费模型**。该模型本身免费，但运行时仍需要配置 OpenCode API Key：推荐使用 `OPENCODE_API_KEY`，也兼容旧环境变量名 `OG_API_KEY`。当需要更强大的模型时，可以随时切换到其他 Provider 的模型。

### 支持哪些 Provider？

basework 原生支持 **15+ 个 Provider**，同时兼容所有标准 OpenAI API 格式的 Provider：

| Provider | 原生支持 |
|----------|----------|
| Anthropic | ✅ |
| OpenAI | ✅ |
| Google Gemini | ✅ |
| Cohere | ✅ |
| Mistral AI | ✅ |
| Groq | ✅ |
| DeepSeek | ✅ |
| Together AI | ✅ |
| Azure OpenAI | ✅ |
| Ollama（本地） | ✅ |
| 其他兼容 OpenAI 的 Provider | 兼容 |

---

## 安装与配置

### 如何安装？

**方式一：go install（推荐）**

```bash
go install -tags "sqlite memory" github.com/wly2lcl/basework/cmd/basework@latest
```

**方式二：源码构建**

```bash
git clone https://github.com/wly2lcl/basework.git
cd basework
make build
```

### 如何配置 API Key？

basework 支持两种方式配置 API Key，优先级从上到下递减：

1. **环境变量**：`ANTHROPIC_API_KEY`、`OPENAI_API_KEY`、`GEMINI_API_KEY` 等
2. **配置文件**：`~/.config/basework/config.json`，通过 `basework init` 交互式生成

```json
{
  "provider": "opencode",
  "model": "big-pickle",
  "opencode": {
    "api_key": "..."
  }
}
```

### 配置文件在哪里？

全局配置文件位置：`~/.config/basework/config.json`

项目级配置（可选）：`.basework/config.json`（当前工作目录）

你可以通过环境变量 `BASEWORK_CONFIG` 或 `--config` 标志指定自定义路径。

### 如何切换模型？

使用 `basework model` 命令：

```bash
# 列出可用模型
basework model list

# 只显示免费模型
basework model list --free

# 切换当前模型
basework model use <model-id>
```

---

## 使用问题

### 如何启动交互式会话？

两种方式：

```bash
# Agent 模式（纯 CLI 交互）
basework agent

# TUI 模式（终端界面）
basework tui
```

### 如何使用特定模型？

当前 agent 没有 `--model` 参数。在配置顶层设置 `provider` 和 `model`，或用 `model list` 后的 `model use <model-id>` 选择已知模型；自定义端点放在 `providers.<provider>.base_url`。

```bash
basework --config /path/to/config.json config explain
basework --config /path/to/config.json agent -m "写一个函数"
```

详见 [CLI 指南](guides/cli-guide.md)。

### 如何查看可用工具？

basework 的工具系统是模块化的。你可以查阅项目文档了解内置工具列表。开发者也可以实现 `tool.Tool` 接口添加自定义工具。

### 如何使用 MCP 工具？

在配置文件中添加 MCP 服务器配置：

```json
{
  "mcp_servers": {
    "my-server": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "."]
    }
  }
}
```

basework 会自动加载并暴露 MCP 服务器提供的工具给 Agent。

### 如何使用 LSP？

basework 支持 LSP（Language Server Protocol）集成：

- **自动检测**：对于主流语言，basework 会自动检测项目中的 LSP 服务器
- **手动配置**：在配置文件中指定 LSP 设置

LSP 支持提供代码补全、诊断、跳转定义等 IDE 级能力。

---

## 开发问题

### 如何嵌入到 Go 项目？

```go
import (
    "github.com/wly2lcl/basework/pkg/agent"
    "github.com/wly2lcl/basework/pkg/provider"
)

func main() {
    p := provider.New(provider.Config{
        Type:     "anthropic",
        APIKey:   os.Getenv("ANTHROPIC_API_KEY"),
        Model:    "claude-3-5-sonnet-20241022",
        BaseURL:  "https://api.anthropic.com/v1",
    })
    
    model := llm.NewModel(llm.ModelConfig{
        Provider: "anthropic",
        Model:    "claude-3-5-sonnet-20241022",
        APIKey:   os.Getenv("ANTHROPIC_API_KEY"),
    })
    a := agent.New(
        agent.WithModel(model),
        agent.WithTools(tool.DefaultRegistry()),
    )
    
    resp, _ := a.Run(ctx, "写一个 Go 反转字符串函数")
    fmt.Println(resp)
}
```

### 如何添加自定义工具？

实现 `tool.Tool` 接口：

```go
import (
    "encoding/json"
    "github.com/wly2lcl/basework/pkg/tool"
)

type MyTool struct{}

func (t *MyTool) Name() string                           { return "my_tool" }
func (t *MyTool) Description() string                    { return "我的自定义工具" }
func (t *MyTool) Parameters() json.RawMessage            { return nil }
func (t *MyTool) Execute(ctx context.Context, args json.RawMessage) (*tool.Result, error) {
    // 工具逻辑
    return &tool.Result{Content: "结果"}, nil
}
```

然后在 Agent 配置中注册：

```go
a := agent.New(agent.Config{
    Provider: p,
    Tools:    []tool.Tool{&MyTool{}},
})
```

### 如何添加 Hook？

Hook 允许你在 Agent 生命周期的各个阶段插入自定义逻辑。实现 `hook.Hook` 接口：

```go
import "github.com/wly2lcl/basework/pkg/hook"

type MyHook struct{}

func (h *MyHook) OnBeforeRequest(ctx context.Context, req *agent.Request) error {
    // 请求发送前
    return nil
}

func (h *MyHook) OnAfterResponse(ctx context.Context, resp *agent.Response) error {
    // 响应返回后
    return nil
}
```

### 如何添加 Skill？

Skill 是 basework 的扩展机制，以 `SKILL.md` 文件形式存在。在项目根目录或指定目录下创建 `SKILL.md` 文件，basework 会自动加载并按 Skill 规则调整行为。

---

## 故障排查

### API Key 无效怎么办？

按以下步骤排查：

1. 检查环境变量是否正确设置：`echo $ANTHROPIC_API_KEY`
2. 检查配置文件：`basework config show`
3. 运行 `basework config validate` 验证配置
4. 确认 API Key 未过期，且账户有足够余额

### 模型不可用怎么办？

1. 确认 `model_id` 格式正确（`provider/model-name`）
2. 检查对应 Provider 的 API 状态
3. 使用 `basework model list` 查看可用模型
4. 尝试切换其他模型或 Provider

### 工具执行失败怎么办？

1. 检查工具的权限设置
2. 确认工具依赖的外部命令或路径可用
3. 检查文件系统权限
4. 使用 `--verbose` 模式获取详细错误信息

### 如何查看日志？

```bash
# 查看日志
basework logs

# 查看最近 50 行
basework logs --tail 50

# 实时跟踪
basework logs --follow
```

---

## 性能与成本

### 如何降低 API 成本？

- 使用 big-pickle 免费模型处理简单任务
- 对小任务使用小模型做摘要或预处理
- 合理控制上下文长度，避免不必要的 token 消耗
- 在配置中设置 `max_tokens` 限制每次请求的 token 使用量

### 上下文窗口满了怎么办？

basework 会在上下文接近满时触发自动压缩策略，智能保留关键信息、压缩或丢弃不重要的内容。你也可以手动重启会话以清空上下文。

### 如何限制 token 使用？

在配置文件中设置 `max_tokens` 字段：

```json
{
  "max_tokens": 4096
}
```

这将限制每次响应的最大 token 数。同时也可在 Provider 配置中设置更细粒度的限制。
