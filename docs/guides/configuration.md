# 配置参考

本文档介绍 basework 的配置系统，包括配置文件格式、搜索路径、环境变量和完整示例。

---

## 配置文件格式

配置文件使用 **JSON** 格式。basework 的 `config.Store` 通过 Copy-on-Write 机制实现无锁读取，支持原子更新和持久化。

### 字段说明

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `provider` | string | `"opencode"` | LLM 提供商类型 |
| `model` | string | `"big-pickle"` | 模型名称 |
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
| `oauth` | object | disabled | OAuth 认证配置 |

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
| `opencode` | OpenCode Zen | `https://opencode.ai/zen/v1` |
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
| `OPENCODE_API_KEY` | OpenCode Zen |
| `OG_API_KEY` | OpenCode Zen（兼容旧环境变量名） |

CLI 的 `basework init` 命令会自动检测这些环境变量。

---

## 配置示例

### 最小配置

```json
{
  "provider": "opencode",
  "model": "big-pickle",
  "temperature": 0.7,
  "max_tokens": 4096
}
```

如需把 OpenCode API Key 写入配置文件，可使用 provider 专属配置：

```json
{
  "provider": "opencode",
  "model": "big-pickle",
  "opencode": {
    "api_key": "..."
  }
}
```

### 多 Provider 配置

```json
{
  "provider": "openai",
  "model": "gpt-4o",
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

### OAuth 认证配置

`oauth.providers` 是 `basework auth login <provider>` 使用的 OAuth 端点配置，不是普通 LLM Provider API Key 配置。

```json
{
  "oauth": {
    "enabled": true,
    "storage_backend": "file",
    "callback_port": 8181,
    "providers": {
      "copilot": {
        "authorization_endpoint": "https://github.com/login/oauth/authorize",
        "token_endpoint": "https://github.com/login/oauth/access_token",
        "client_id": "...",
        "client_secret": "...",
        "scopes": ["read:user"],
        "redirect_uri": "http://127.0.0.1:8181/callback"
      }
    }
  }
}
```

当前 `storage_backend` 支持 `file`。`keychain` 是保留配置值，但系统密钥链后端尚未实现，启用时会明确报错。

---

## 相关文档

- [嵌入指南](embedder-guide.md) — 如何将 basework 嵌入到应用中
- [扩展指南](extending.md) — Hook、Plugin、Skill 扩展
- [安全配置指南](security.md) — 权限持久化、敏感路径保护、审计日志
- [性能分析指南](profiling.md) — pprof 集成、benchmark 套件
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

---

## 安全配置

### 敏感路径保护

敏感路径保护在工具执行前检查文件路径，防止 Agent 意外访问或修改敏感文件。

**保护级别**：

| 级别 | 说明 |
|------|------|
| `strict` | 禁止访问敏感路径（默认） |
| `warn` | 记录警告日志但允许访问 |
| `off` | 关闭路径保护 |

**配置示例**：

```json
{
  "security": {
    "protection_level": "strict",
    "permission_store": "sqlite",
    "audit_retention_days": 30,
    "sensitive_paths": {
      "block": ["/custom/secret/"],
      "allow": ["~/.ssh/config"]
    }
  }
}
```

- `protection_level` — 保护级别（`strict` / `warn` / `off`）
- `permission_store` — 权限规则存储后端（`sqlite` / `memory`）
- `audit_retention_days` — 审计日志保留天数（默认 30）
- `sensitive_paths.block` — 额外黑名单路径
- `sensitive_paths.allow` — 白名单路径（覆盖黑名单）

### 默认保护路径

以下路径默认受保护（strict 级别）：

| 路径 | 说明 |
|------|------|
| `.git/` | Git 仓库元数据 |
| `~/.ssh/` | SSH 密钥和配置 |
| `~/.aws/` | AWS 凭证 |
| `~/.gnupg/` | GPG 密钥 |
| `~/.config/basework/` | Basework 自身配置 |

---

## 工具执行超时

为每个工具设置独立的超时时间，防止长时间运行的命令阻塞 Agent。

```json
{
  "tools": {
    "timeout": {
      "default": 30,
      "overrides": {
        "bash": 60,
        "read": 10,
        "write": 15,
        "edit": 15,
        "grep": 10,
        "web_fetch": 30,
        "web_search": 20
      }
    }
  }
}
```

- `tools.timeout.default` — 全局默认超时（秒），0 表示不超时
- `tools.timeout.overrides` — 按工具名覆盖超时时间

---

## 性能分析

通过 pprof 集成，可以实时分析 Agent 运行时的 CPU、内存和 goroutine 状态。

```json
{
  "profiling": {
    "enabled": false,
    "host": "127.0.0.1",
    "port": 6060
  }
}
```

- `enabled` — 是否启用 pprof HTTP 端点（默认 false）
- `host` — 监听地址，默认仅本地（127.0.0.1）
- `port` — 监听端口（默认 6060）

启动后可通过 `http://127.0.0.1:6060/debug/pprof/` 访问 pprof 页面。

详细用法请参考 [性能分析指南](profiling.md)。

---

## 主题配置

TUI 主题系统，支持内置主题切换和自定义主题。

```json
{
  "theme": {
    "name": "dark",
    "custom_path": "~/.config/basework/themes/"
  }
}
```

- `theme.name` — 主题名称（`dark` / `light` / `dracula` / `monokai`，默认 `dark`）
- `theme.custom_path` — 自定义主题目录路径（可选）

详细用法请参考 [主题配置指南](theme.md)。

---

## 键盘绑定配置

键盘绑定配置文件路径，支持自定义快捷键。

```json
{
  "keybindings": {
    "path": "~/.config/basework/keybindings.json"
  }
}
```

- `keybindings.path` — 键盘绑定 JSON 配置文件路径（可选）

---

## 模板配置

模板系统配置，支持用户自定义模板和默认 Provider 选择。

```json
{
  "templates": {
    "custom_dir": ".basework/prompts/",
    "default_provider": "anthropic"
  }
}
```

- `templates.custom_dir` — 用户自定义模板目录（可选，指向 `.basework/prompts/`）
- `templates.default_provider` — 默认模板 Provider 名称（默认 `"default"`）

详细用法请参考 [模板系统指南](templates.md)。
