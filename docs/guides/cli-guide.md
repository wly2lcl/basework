# basework CLI 使用指南

## 概述

basework CLI 提供完整的终端 AI 编程助手体验。通过命令行交互，你可以使用 AI Agent 完成代码生成、重构、调试、文档编写等开发任务。CLI 支持交互式会话模式和单次命令模式，适用于不同的使用场景。

## 安装

```bash
# 方式一：go install（推荐）
go install github.com/wly2lcl/basework/cmd/basework@latest

# 方式二：从源码构建
git clone https://github.com/wly2lcl/basework.git
cd basework && make build
```

安装完成后，验证是否成功：

```bash
basework --version
```

## 快速开始

```bash
# 初始化配置（首次使用）
basework init

# 启动交互式 Agent 会话
basework agent

# 单次提问（不进入交互模式）
basework agent -m "写一个 Go 反转字符串函数"

# 使用特定模型
basework agent -m "解释这段代码" --model anthropic/claude-sonnet-4-20250514

# 使用 TUI 模式（✅ 已实现）
basework tui
```

---

## 命令详解

### basework agent

启动交互式 Agent 会话，这是 basework 的核心功能。进入交互模式后，你可以连续提问，Agent 会保持对话上下文。

**参数：**

| 参数 | 类型 | 说明 |
|------|------|------|
| `-m, --message` | string | 单次消息（不进入交互模式） |
| `--model` | string | 指定模型（格式：`provider/model-id`） |
| `--no-stream` | — | 禁用流式输出 |
| `--config` | string | 配置文件路径（自定义） |
| `--verbose` | — | 详细输出（显示调试信息） |

**示例：**

```bash
# 交互模式 — 连续对话
basework agent

# 单次模式 — 执行后退出
basework agent -m "创建一个 Go 的 HTTP 服务器 hello.go"

# 指定模型
basework agent --model openai/gpt-4o -m "优化这段代码"

# 禁用流式输出
basework agent --no-stream

# 使用自定义配置文件
basework agent --config /path/to/config.json

# 调试模式
basework agent --verbose
```

### basework init

初始化配置文件。运行后会以交互方式引导你完成配置：

1. 检测环境变量中的 API Key（`ANTHROPIC_API_KEY`、`OPENAI_API_KEY` 等）
2. 选择默认 Provider 和模型
3. 生成 `~/.config/basework/config.json`

```bash
basework init
```

如果环境变量已设置，`init` 会自动识别并填入配置。

### basework model

模型管理命令。

```bash
# 列出所有可用模型
basework model list

# 只列出本地目录标为免费的模型
basework model list --free

# 切换当前模型
basework model use mimo-v2.5-free
```

`model list` 会显示已知模型的 Provider、模型 ID 和是否免费；`model use` 会把选中的模型写入配置文件。免费标记不保证免密钥或支持工具；认证要求和可用性应按实际端点验证。当前 CLI 的部分帮助文字尚把两者混用，修复列入 REL-002。

### basework session

会话管理命令，用于查看和管理历史对话。

```bash
# 列出所有会话
basework session list

# 清除所有会话
basework session clear

# 恢复指定会话（✅ 已实现）
basework session resume <session-id>

# 导出会话为文件（✅ 已实现）
basework session export <session-id>

# 搜索会话内容（✅ 已实现）
basework session search "关键词"
```

会话数据默认存储在 `~/.local/share/basework/sessions/` 目录下（数据目录，遵循 XDG：
配置放 `~/.config/basework/`，会话数据放 `$XDG_DATA_HOME`）。早期版本的
`~/.basework/sessions/` 作为只读回退仍可被读取，可用 `basework migrate sessions`
合并到规范目录。

### basework tui

启动 TUI（终端用户界面）模式，提供更丰富的交互体验（✅ 已实现）。

```bash
# 启动 TUI
basework tui

# 指定主题
basework tui --theme dark

# 恢复历史会话
basework tui --resume <session-id>
```

TUI 模式提供分屏布局、语法高亮、文件树等增强功能。

### basework permission

权限管理命令（✅ 已实现），管理 Agent 的文件系统、网络等操作权限。

```bash
# 列出所有权限规则
basework permission list

# 添加权限规则
basework permission add "allow read /home/user/project/*"

# 删除权限规则
basework permission remove "allow read /home/user/project/*"
```

### basework auth

认证管理命令（✅ 已实现），用于管理 OAuth 登录状态。

```bash
# OAuth 登录 Provider
basework auth login github

# 登出 Provider
basework auth logout github

# 查看认证状态
basework auth status
```

### basework config

配置管理命令。

```bash
# 验证配置文件是否正确
basework config validate

# 显示当前配置
basework config show
```

`config validate` 会检查：
- 配置文件格式是否正确（JSON）
- Provider 配置是否完整
- API Key 是否已设置
- 必填字段是否缺失

`config show` 会显示当前生效的完整配置（API Key 会被脱敏处理）。

### basework migrate

数据迁移命令（✅ 已实现），用于将旧格式数据迁移到新格式。

```bash
# 将会话数据从 JSONL 迁移到 SQLite
basework migrate sessions
```

### basework logs

日志查看命令（✅ 已实现），用于排查问题。

```bash
# 查看日志
basework logs

# 查看最近 50 行
basework logs --tail 50

# 实时跟踪日志输出
basework logs --follow
```

---

## 全局标志

以下标志适用于所有子命令：

| 标志 | 类型 | 说明 |
|------|------|------|
| `--config string` | string | 指定配置文件路径 |
| `--verbose` | — | 启用详细输出 |
| `--help` | — | 显示帮助信息 |
| `--version` | — | 显示版本信息 |

示例：

```bash
basework --config ~/custom-config.json agent
basework --verbose agent -m "hello"
basework --help
basework --version
```

---

## 环境变量

| 变量 | 说明 |
|------|------|
下表是**代码实际读取**的全部运行期环境变量（核实日期 2026-09-13）：

| 变量 | 说明 |
|------|------|
| `OPENAI_API_KEY` | OpenAI；也是各 OpenAI 兼容 provider（`openai-compat`、`deepseek`、`groq`、`together`、`openrouter`、`xai`、`mistral`）的回落来源 |
| `ANTHROPIC_API_KEY` | Anthropic API Key |
| `GOOGLE_API_KEY` | Google Gemini API Key |
| `OPENCODE_API_KEY` | OpenCode Zen API Key（推荐） |
| `OG_API_KEY` | OpenCode Zen API Key（兼容旧名称） |
| `AZURE_API_KEY` | Azure OpenAI API Key |
| `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` / `AWS_REGION` | Amazon Bedrock |
| `BASEWORK_PROVIDER` | 覆盖配置文件里的 `provider` |
| `BASEWORK_BASE_URL` | 覆盖生效 provider 的自定义端点 |
| `BASEWORK_AUDIT_MODE` | 请求审计模式：`compatible`（默认）/ `strict` |

环境变量优先级高于配置文件中的对应设置。

> 历史文档里出现过的 `COHERE_API_KEY`、`MISTRAL_API_KEY`、`GROQ_API_KEY`、
> `DEEPSEEK_API_KEY`、`TOGETHER_API_KEY`、`AZURE_OPENAI_API_KEY`、`BASEWORK_CONFIG`
> 与 `BASEWORK_LOG_LEVEL` **代码从未读取**，已从本表移除。对应的 provider 目前
> 走 `OPENAI_API_KEY` 回落，或只能在配置文件里写凭据。

---

## 配置文件

basework 按以下优先级查找配置文件：

1. `--config` 标志指定的路径（最高优先级）
2. 当前工作目录及其各级父目录下的 `config.json`
3. `~/.config/basework/config.json`（全局配置，默认路径）

**示例配置文件：**

```json
{
  "provider": "openai",
  "model": "gpt-4o-mini",
  "temperature": 0.7,
  "max_tokens": 4096,
  "providers": {
    "openai": {
      "base_url": "https://gateway.example.com/v1",
      "api_key": "sk-..."
    }
  },
  "mcp_configs": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "."]
    }
  }
}
```

字段完整清单以 [配置参考](configuration.md) 与
[`pkg/config` 包契约](../reference/pkg/config.md) 为准。几点容易踩的：

- **没有 `default_provider` / `default_model` / `mcp_servers` 这些字段**——模型与
  provider 分别用 `provider` / `model`，MCP 用 `mcp_configs`。
- **凭据不写 `${ANTHROPIC_API_KEY}` 这种展开语法**（未实现）。省略 `api_key`
  就会去读该 provider 的环境变量。
- **自定义端点**用 `providers.<provider 名>.base_url`，键按**生效 provider**取值；
  写完可以用 `basework config explain` 确认它真的生效、以及生效端点来自哪里。
  该命令只显示 URL 与「key 是否设置」，不会输出 key 本身。

---

## 快捷键（交互模式）

在 Agent 交互模式下的快捷键：

| 快捷键 | 说明 |
|--------|------|
| `Enter` | 发送消息 |
| `Ctrl+C` | 取消当前操作 |
| `Ctrl+D` | 退出会话 |
| `↑/↓` | 浏览历史命令 |
| `Tab` | 自动补全（命令、路径等） |

---

## 退出码

| 退出码 | 说明 |
|--------|------|
| 0 | 成功 |
| 1 | 一般错误（运行时错误、网络错误等） |
| 2 | 配置错误（配置文件格式错误、缺失字段等） |
| 3 | 认证错误（API Key 无效、OAuth 失败等） |

在脚本中使用 basework 时，可以通过检查退出码来判断执行结果：

```bash
basework agent -m "task" || {
    case $? in
        2) echo "请检查配置文件" ;;
        3) echo "请检查 API Key" ;;
        *) echo "执行失败" ;;
    esac
}
```

---

## 使用技巧

### 交互模式中切换模型

在交互会话中，无法动态切换模型。如需使用不同模型，请启动新的会话。

### 结合管道使用

单次模式可以与 Unix 管道结合：

```bash
cat main.go | basework agent -m "审查这段代码"
```

### 长任务处理

对于耗时较长的任务，建议使用交互模式以便持续观察进度。如果任务意外中断，后续可通过会话恢复功能继续（✅ 已实现）。
