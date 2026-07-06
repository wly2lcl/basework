# 配置参考

本文档介绍 basework 的配置系统，包括配置文件格式、搜索路径、环境变量和完整示例。

---

## 配置文件格式

配置文件使用 **JSON** 格式。basework 的 `config.Store` 通过 Copy-on-Write 机制实现无锁读取，支持原子更新和持久化。

### 字段说明

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `provider` | string | `"openai"` | LLM 提供商类型 |
| `model` | string | `"gpt-4"` | 模型名称 |
| `temperature` | float | `0.7` | 生成温度 (0~2) |
| `max_tokens` | int | `4096` | 最大生成 token 数 |
| `system_prompt` | string | `""` | 系统提示词 |
| `top_p` | float | `1.0` | Top-P 采样 |
| `frequency_penalty` | float | `0.0` | 频率惩罚 |
| `presence_penalty` | float | `0.0` | 存在惩罚 |
| `stop_sequences` | string[] | `null` | 停止序列 |
| `max_iterations` | int | `10` | 最大迭代次数 |
| `timeout` | int | `60` | API 超时时间（秒） |
| `verbose` | bool | `false` | 详细日志 |
| `mcp_configs` | object | `null` | MCP 服务器配置 |
| `max_tool_calls` | int | `20` | 每轮最大工具调用数 |
| `max_context_tokens` | int | `128000` | 最大上下文 token 数 |

---

## 配置文件路径和优先级

配置文件搜索顺序（按优先级从高到低）：

1. **当前目录** — `./config.json`
2. **项目级配置** — `.basework/config.json`（当前工作目录下）
3. **父目录** — 逐级向上搜索
4. **用户目录** — `~/.config/basework/config.json`

均不存在时，使用默认配置，保存路径为 `~/.config/basework/config.json`。

### 搜索逻辑（代码实现）

```go
// 搜索顺序：当前目录 → 父目录（逐级向上）→ ~/.config/basework/config.json
func Discover() (string, error)
```

---

## Provider 配置

Provider 直接通过 Go 代码中的 `provider.Create()` 配置，也可以通过环境变量指定 API key。

### 支持的 Provider

| 类型值 | 说明 | 默认端点 |
|--------|------|----------|
| `openai` | OpenAI API | `https://api.openai.com/v1` |
| `anthropic` | Anthropic Claude | `https://api.anthropic.com` |
| `gemini` | Google Gemini | `https://generativelanguage.googleapis.com` |
| `openai-compat` | OpenAI 兼容 API | 自定义 |
| `deepseek` | DeepSeek | `https://api.deepseek.com/v1` |
| `groq` | Groq | `https://api.groq.com/openai/v1` |
| `together` | Together AI | `https://api.together.xyz/v1` |
| `openrouter` | OpenRouter | `https://openrouter.ai/api/v1` |
| `xai` | xAI | `https://api.x.ai/v1` |
| `mistral` | Mistral AI | `https://api.mistral.ai/v1` |
| `opencode` | OpenCode Zen | `https://api.opencode.ai/v1` |
| `bedrock` | Amazon Bedrock | AWS Converse API |
| `azure` | Azure OpenAI | `https://{resource}.openai.azure.com` |
| `copilot` | GitHub Copilot | `https://api.githubcopilot.com` |
| `ollama` | Ollama | `http://localhost:11434/v1` |

### 代码配置参数

```go
type Config struct {
    Type    string         // provider 类型
    APIKey  string         // API 密钥
    BaseURL string         // 自定义端点（可选）
    ModelID string         // 模型 ID
    Options map[string]any // provider 特定选项（可选）
}
```

---

## 环境变量

basework 自动识别以下环境变量：

| 环境变量 | 对应 Provider |
|---------|---------------|
| `ANTHROPIC_API_KEY` | Anthropic Claude |
| `OPENAI_API_KEY` | OpenAI |
| `GOOGLE_API_KEY` | Google Gemini |

CLI 的 `basework init` 命令会自动检测这些环境变量。

---

## 配置示例

### 最小配置

```json
{
  "provider": "anthropic",
  "model": "claude-3-5-sonnet-20241022",
  "temperature": 0.7,
  "max_tokens": 4096
}
```

### 多 Provider 配置

```json
{
  "provider": "openai",
  "model": "gpt-4",
  "temperature": 0.5,
  "max_tokens": 8192,
  "system_prompt": "你是一个专业的代码审查助手。",
  "stop_sequences": ["```"],
  "max_iterations": 15,
  "timeout": 120
}
```

### 启用 LSP 和 MCP

```json
{
  "provider": "anthropic",
  "model": "claude-3-5-sonnet-20241022",
  "system_prompt": "你是一个全栈开发助手，擅长 Go 和 TypeScript。",
  "max_iterations": 25,
  "max_tool_calls": 30,
  "max_context_tokens": 200000,
  "mcp_configs": {
    "github": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-github"],
      "env": {
        "GITHUB_TOKEN": "ghp_..."
      }
    },
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/workspace"]
    }
  }
}
```

### 配置持久化（代码中）

```go
import "github.com/wly2lcl/basework/pkg/config"

// 发现配置文件路径
path, err := config.Discover()

// 加载配置
store, err := config.Load(path)

// 读取配置
cfg := store.Get()
fmt.Println(cfg.Provider, cfg.Model)

// 修改配置
store.Mutate(func(c *config.Config) {
    c.Temperature = 0.8
    c.MaxTokens = 2048
})

// 保存到文件
store.Save()
```

---

## 相关文档

- [嵌入指南](embedder-guide.md) — 如何将 basework 嵌入到应用中
- [扩展指南](extending.md) — Hook、Plugin、Skill 扩展
- [设计文档](../DESIGN.md) — 架构和接口定义

---

## 数据库配置

### `database.mode`

SQLite 日志模式，影响并发读写性能。

| 值 | 说明 |
|---|---|
| `wal` | Write-Ahead Logging，支持并发读写（默认） |
| `delete` | 传统模式，写入时阻塞读取 |

```json
{
  "database": {
    "mode": "wal"
  }
}
```

## 会话压缩配置

### `session.compression.enabled`

是否启用长会话消息压缩（默认 `true`）。当会话消息数超过 1000 条时自动触发 Snappy 压缩。

```json
{
  "session": {
    "compression": {
      "enabled": true
    }
  }
}
```