# Provider 配置指南

本文档介绍 basework 支持的 LLM 提供商、配置方式及故障排查方法。

---

## 概述

basework 支持 15+ LLM 提供商，通过 `pkg/provider` 统一接口访问。每个 Provider 只需提供类型标识和 API Key 即可使用，无需额外的 SDK 依赖。

所有支持的 Provider 分为三类：

- **原生支持** — 使用各自原生协议（OpenAI chat/completions API、Anthropic Messages API、Gemini API）
- **OpenAI 兼容** — 使用 OpenAI 协议，仅更换端点和认证信息
- **✅ 已实现** — 后续新增的 Provider

---

## 支持的 Provider

### 原生支持

| Provider | 协议 | 端点 | 环境变量 |
|----------|------|------|----------|
| OpenAI | openai-chat | `https://api.openai.com/v1` | `OPENAI_API_KEY` |
| Anthropic | anthropic-messages | `https://api.anthropic.com` | `ANTHROPIC_API_KEY` |
| Gemini | gemini | `https://generativelanguage.googleapis.com` | `GEMINI_API_KEY` |

### OpenAI 兼容

以下 Provider 使用 OpenAI 协议，只需替换端点和 API Key：

| Provider | 类型值 | 端点 | 环境变量 |
|----------|--------|------|----------|
| DeepSeek | `"deepseek"` | `https://api.deepseek.com/v1` | `DEEPSEEK_API_KEY` |
| Groq | `"groq"` | `https://api.groq.com/openai/v1` | `GROQ_API_KEY` |
| Together | `"together"` | `https://api.together.xyz/v1` | `TOGETHER_API_KEY` |
| OpenRouter | `"openrouter"` | `https://openrouter.ai/api/v1` | `OPENROUTER_API_KEY` |
| xAI | `"xai"` | `https://api.x.ai/v1` | `XAI_API_KEY` |
| Mistral | `"mistral"` | `https://api.mistral.ai/v1` | `MISTRAL_API_KEY` |

### ✅ 已实现

| Provider | 类型值 | 端点 | 环境变量 |
|----------|--------|------|----------|
| OpenCode Zen | `"opencode"` | `https://opencode.ai/zen/v1` | `OPENCODE_API_KEY`（兼容 `OG_API_KEY`） |
| Amazon Bedrock | `"bedrock"` | AWS Converse API | AWS 凭证 |
| Azure OpenAI | `"azure"` | `https://{resource}.openai.azure.com` | `AZURE_OPENAI_API_KEY` |
| GitHub Copilot | `"copilot"` | `https://api.githubcopilot.com` | OAuth 认证 |
| Ollama | `"ollama"` | `http://localhost:11434/v1` | 无（本地） |

---

## 配置方式

Provider 支持三种配置方式，优先级从高到低为：代码中配置 > 配置文件 > 环境变量。

### 环境变量（推荐）

```bash
# 基础配置
export ANTHROPIC_API_KEY=sk-ant-...
export OPENAI_API_KEY=sk-...

# OpenAI 兼容 Provider
export DEEPSEEK_API_KEY=sk-...
export GROQ_API_KEY=gsk_...
export TOGETHER_API_KEY=...
```

环境变量方式适合 CI/CD、容器化部署和多环境管理。配置文件名不会泄露到版本控制中。

### 配置文件

```json
{
  "provider": "opencode",
  "model": "big-pickle",
  "opencode": {
    "api_key": "..."
  }
}
```

配置文件使用顶层 `provider` 和 `model` 选择当前 Provider，并通过 provider 专属块补充凭证或端点配置：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `provider` | string | 是 | Provider 类型（见支持列表） |
| `model` | string | 否 | 模型名称，不填使用默认模型 |
| `opencode.api_key` | string | 否 | OpenCode Zen API Key |
| `azure.api_key` | string | 否 | Azure OpenAI API Key |
| `bedrock.access_key` / `bedrock.secret_key` | string | 否 | Amazon Bedrock 凭证 |

### 代码中配置

```go
import "github.com/wly2lcl/basework/pkg/provider"

model, err := provider.Create(provider.Config{
    Type:    "anthropic",
    APIKey:  "sk-ant-...",
    BaseURL: "",
    ModelID: "claude-sonnet-4-20250514",
})
if err != nil {
    log.Fatal(err)
}
```

---

## 自定义端点

嵌入式使用时，可通过 `provider.Config.BaseURL` 设置自定义端点，适用于代理、本地模型或企业网关：

```go
model, err := provider.Create(provider.Config{
    Type:    "openai-compat",
    APIKey:  "sk-...",
    BaseURL: "https://my-proxy.example.com/v1",
    ModelID: "my-custom-model",
})
```

注意事项：

- 自定义端点必须包含 `/v1` 后缀（大多数 OpenAI 兼容 API 要求）
- `base_url` 不以 `/` 结尾
- 如果 Provider 有专用类型（如 `"deepseek"`），优先使用专用类型而非 `"openai-compat"`，因为专用类型会自动处理协议差异

---

## 模型选择

### 默认模型

```json
{
  "provider": "opencode",
  "model": "big-pickle"
}
```

| 配置 | 用途 |
|------|------|
| `default` | 主要对话模型，用于复杂推理和代码生成 |
| `small` | 轻量任务模型，用于摘要、标题生成、快速验证 |

### 列出可用模型

```bash
basework model list
basework model list --free
basework model use mimo-v2.5-free
```

`model list` 列出已知模型；`--free` 只显示无需 API key 的免费模型；`model use` 会把选中的模型写入当前配置。

---

## Provider 专属配置

```json
{
  "provider": "azure",
  "model": "gpt-4o",
  "azure": {
    "resource": "my-resource",
    "deployment": "gpt-4o",
    "api_key": "...",
    "api_version": "2024-02-01"
  }
}
```

当前 CLI runtime 使用单个 active provider；主备切换可在嵌入式场景中通过自定义 `llm.Model` 或上层调度实现。

---

## 故障排查

### 认证错误

| 错误信息 | 可能原因 | 解决方法 |
|----------|----------|----------|
| `401 Unauthorized` | API Key 无效 | 检查 Key 是否正确，是否过期 |
| `403 Forbidden` | API Key 无权限 | 确认 Key 的模型访问权限 |
| `auth: api_key required` | 未配置 API Key | 设置环境变量或配置文件中的 `api_key` |

检查步骤：

```bash
# 检查环境变量是否设置
echo $ANTHROPIC_API_KEY

# 检查配置文件
cat ~/.basework/config.json | grep api_key

# 测试认证（以 OpenAI 为例）
curl -H "Authorization: Bearer $OPENAI_API_KEY" https://api.openai.com/v1/models
```

### 端点错误

| 错误信息 | 可能原因 | 解决方法 |
|----------|----------|----------|
| `connection refused` | 端点不可达 | 检查 `base_url` 拼写，确认网络连通性 |
| `404 Not Found` | 路径错误 | 确认 `base_url` 包含 `/v1` 后缀 |
| `timeout` | 网络超时 | 检查代理设置（`HTTP_PROXY`/`HTTPS_PROXY`） |

检查步骤：

```bash
# 测试网络连通性
curl -I https://api.openai.com/v1/models

# 检查代理
echo $HTTP_PROXY
echo $HTTPS_PROXY

# 测试 DNS 解析
nslookup api.openai.com
```

### 模型错误

| 错误信息 | 可能原因 | 解决方法 |
|----------|----------|----------|
| `model not found` | `model_id` 拼写错误 | 运行 `basework model list` 查看可用模型 |
| `model not supported` | 模型不在 Provider 支持列表中 | 查看 Provider 官方文档确认模型名称 |
| `context length exceeded` | 输入超出模型上下文限制 | 启用 `compaction` 或减少输入长度 |

### 通用排查流程

```
1. 验证 API Key → curl 测试认证
2. 验证网络 → curl 测试端点
3. 验证模型 → basework model list
4. 查看日志 → 设置 verbose: true
5. 最小化复现 → 用 provider.Create() 单步测试
```

---

## 成本优化建议

### 模型分级使用

```json
{
  "model": {
    "default": "claude-sonnet-4-20250514",
    "small": "gpt-4o-mini",
    "cheap": "deepseek-chat"
  }
}
```

| 任务类型 | 推荐模型 | 说明 |
|----------|----------|------|
| 复杂推理、代码生成 | Claude Sonnet 4 / GPT-4o | 高质量，成本较高 |
| 摘要、标题、分类 | GPT-4o-mini / DeepSeek | 低成本，速度更快 |
| 测试、开发调试 | big-pickle / deepseek-v4-flash-free | 免费，适合快速迭代 |

### 使用限制

```json
{
  "budget": {
    "monthly_limit_usd": 50,
    "max_per_session_usd": 2
  }
}
```

- 使用免费模型（big-pickle, deepseek-v4-flash-free）进行开发和测试
- 小模型用于摘要/标题生成等轻量任务
- 设置月度使用限额，避免意外超支
- 监控 API 调用量，定期审查使用模式

---

> **参考**：嵌入指南见 [嵌入指南](./embedder-guide.md)，迁移信息见 [迁移指南](./migration.md)。
