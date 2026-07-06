# 模板系统指南

本文档介绍 basework 的模板系统，包括内置模板、自定义模板、模板变量和 Provider 路由。

---

## 概述

basework 的模板系统用于生成 Provider 感知的系统提示（System Prompt）。系统会根据当前使用的 LLM Provider 自动选择合适的模板，并注入环境上下文信息。

### 工作流程

```
Agent 初始化
    │
    ├── 检测 Provider (anthropic/openai/gemini/...)
    │
    ├── RouteForProvider() → 选择模板名称
    │
    ├── 检查用户自定义模板（优先级高）
    │   └── 不存在则使用内置模板
    │
    ├── 环境动态注入（工作目录/Git/平台）
    │
    └── RenderTemplate() → 最终系统提示
```

---

## 内置模板

basework 提供 4 个内置模板，每个模板针对特定 Provider 优化：

| 模板名 | 适用 Provider | 特点 |
|--------|-------------|------|
| `anthropic` | Anthropic Claude | 安全性强调，详细行为准则 |
| `openai` | OpenAI GPT / OpenCode Zen | JSON 模式提示，function calling 引导 |
| `gemini` | Google Gemini | 多模态能力提示，长上下文 |
| `default` | 其他 Provider | 通用简洁系统提示 |

### Provider 路由规则

Provider 到模板的自动映射：

| Provider 类型 | 路由到模板 |
|--------------|-----------|
| `anthropic` | `anthropic` |
| `openai` | `openai` |
| `gemini` | `gemini` |
| `opencode` | `openai`（兼容 OpenAI 协议） |
| `bedrock` | `anthropic`（常用 Claude 模型） |
| `azure` | `default` |
| `ollama` | `default` |
| `copilot` | `default` |
| 其他 | `default` |

### anthropic 模板

针对 Claude 系列优化，强调安全性和行为准则：

```
You are Claude, an AI coding assistant built by Anthropic, powered by {{.Model}}.
...
Always follow these principles:
1. Write correct, idiomatic, and well-documented code.
2. Prefer simple solutions over complex ones.
...
```

### openai 模板

针对 GPT 系列优化，强调 JSON 模式和 function calling：

```
You are GPT, an AI coding assistant powered by {{.Model}} ({{.Provider}}).
...
Capabilities:
- Structured output with JSON mode
- Function calling for external integrations
...
```

### gemini 模板

针对 Gemini 系列优化，强调多模态能力：

```
You are Gemini, an AI coding assistant powered by {{.Model}} ({{.Provider}}).
...
You can process both text and images...
Capabilities:
- Multimodal understanding (text + images)
- Long context window for large codebases
...
```

### default 模板

通用模板，适用于所有 Provider：

```
You are a helpful AI coding assistant powered by {{.Model}} ({{.Provider}}).
...
```

---

## 模板变量

模板支持以下变量，通过 `{{.VariableName}}` 语法引用：

| 变量名 | 类型 | 说明 | 示例值 |
|--------|------|------|--------|
| `{{.Model}}` | string | 当前使用的模型名称 | `claude-3-5-sonnet` |
| `{{.Provider}}` | string | LLM Provider 名称 | `anthropic` |
| `{{.WorkingDir}}` | string | 当前工作目录 | `/home/user/project` |
| `{{.Date}}` | string | 当前日期 | `2026-07-06` |
| `{{.Platform}}` | string | 操作系统 | `darwin`, `linux` |
| `{{.Arch}}` | string | CPU 架构 | `arm64`, `amd64` |
| `{{.Shell}}` | string | 默认 Shell | `zsh`, `bash` |
| `{{.Context}}` | string | 环境动态注入信息 | 见下文 |

### 环境动态注入

`{{.Context}}` 变量包含由 `ContextCollector` 并行收集的环境信息：

- **工作目录** — 当前工作目录路径
- **Git 状态** — 分支名、最新提交、是否 dirty
- **平台信息** — OS/架构/Shell
- **项目信息** — 检测到的项目类型（Go/Python/TS）和关键文件

示例 Context 输出：

```
Working directory: /home/user/project
Git branch: main (dirty)
Latest commit: a1b2c3d fix: update config
Platform: linux/amd64
Shell: bash
Project type: Go
Key files: Makefile, Dockerfile
```

---

## 自定义模板

用户可以通过 `.basework/prompts/` 目录自定义模板，覆盖内置模板。

### 目录结构

```
.basework/
├── prompts/
│   ├── anthropic.txt    # 覆盖 anthropic 内置模板
│   ├── my-custom.txt    # 自定义新模板
│   └── openai.txt       # 覆盖 openai 内置模板
└── basework.json
```

### 创建自定义模板

1. 创建模板目录：

```bash
mkdir -p .basework/prompts/
```

2. 创建模板文件（使用 `.txt` 扩展名）：

```bash
cat > .basework/prompts/my-custom.txt << 'EOF'
You are {{.Model}}, a specialized coding assistant.

Current context:
- Working in: {{.WorkingDir}}
- Platform: {{.Platform}}/{{.Arch}}
- Date: {{.Date}}

{{.Context}}

Special instructions:
1. Always write tests first.
2. Use idiomatic Go patterns.
3. Keep functions small and focused.
EOF
```

3. 配置使用自定义模板：

```json
{
  "templates": {
    "custom_dir": ".basework/prompts/"
  }
}
```

### 模板优先级

```
用户自定义模板 > 内置模板
```

如果用户在 `.basework/prompts/anthropic.txt` 中定义了同名模板，将优先使用自定义版本。

### 使用自定义模板名

通过 `templates.default_provider` 配置指定默认使用的模板名：

```json
{
  "templates": {
    "custom_dir": ".basework/prompts/",
    "default_provider": "my-custom"
  }
}
```

---

## 模板验证

basework 在加载模板时会自动使用 Go 的 `text/template` 引擎验证语法正确性：

- 语法错误（如未闭合的 `{{`）会在编译时报错
- 未定义的变量引用会在渲染时报错（`missingkey=error` 模式）
- 用户自定义模板扫描时自动验证，无效模板不会被加载

---

## 热重载

`UserTemplateManager` 支持热重载：修改 `.basework/prompts/` 目录下的模板文件后，
调用 `Reload()` 方法即可动态更新模板内容，无需重启 Agent 进程。

热重载检测策略：
1. 检查文件的修改时间（`ModTime`）
2. 仅加载有变更或新增的文件
3. 删除的文件从模板缓存中移除
4. 语法错误不会阻断已有模板的使用

---

## 代码使用示例

### 基本使用

```go
package main

import (
    "fmt"
    "github.com/wly2lcl/basework/pkg/agent"
)

func main() {
    // 根据 Provider 选择模板
    tmplName := agent.RouteForProvider("anthropic")

    // 加载内置模板
    tmplText, err := agent.LoadBuiltinTemplate(tmplName)
    if err != nil {
        panic(err)
    }

    // 构建变量
    vars := agent.NewTemplateVars()
    vars.Model = "claude-3-5-sonnet"
    vars.Provider = "anthropic"
    vars.WorkingDir = "/home/user/project"

    // 渲染模板
    prompt, err := agent.RenderTemplate(tmplText, vars)
    if err != nil {
        panic(err)
    }

    fmt.Println(prompt)
}
```

### 使用环境注入

```go
import (
    "context"
    "github.com/wly2lcl/basework/pkg/agent"
)

// 创建 ContextCollector（使用默认 providers）
collector := agent.NewContextCollector()

// 收集环境信息
envContext := collector.Collect(context.Background())

// 注入到模板
vars := agent.NewTemplateVars()
vars.Context = envContext
```

### 使用用户自定义模板

```go
// 创建用户模板管理器
mgr := agent.NewUserTemplateManager(".basework/prompts/")

// 扫描自定义模板
if err := mgr.Scan(); err != nil {
    // 处理错误
}

// 优先使用用户自定义模板
tmplName := agent.RouteForProvider("anthropic")
tmplText, ok := mgr.Get(tmplName)
if !ok {
    // 回退到内置模板
    tmplText, _ = agent.LoadBuiltinTemplate(tmplName)
}

// 热重载
changed, err := mgr.Reload()
if changed {
    fmt.Println("模板已更新")
}
```

---

## 故障排查

### 模板渲染错误

如果模板渲染失败，检查：

1. **变量名拼写** — 确保使用正确的变量名（如 `{{.Model}}` 而非 `{{.model}}`）
2. **模板语法** — 确保 `{{ }}` 正确闭合
3. **自定义模板** — 检查 `.txt` 文件编码（使用 UTF-8）

### 环境注入为空

如果 `{{.Context}}` 为空，检查：

1. `git` 命令是否可用（不在 Git 目录时静默跳过）
2. `SHELL` 环境变量是否设置
3. 当前目录是否存在项目文件

### 模板未生效

如果自定义模板未生效，检查：

1. `templates.custom_dir` 配置是否正确指向 `prompts/` 目录
2. 文件名是否正确（带 `.txt` 扩展名）
3. 模板内容语法是否合法

---

## 相关文档

- [配置参考](configuration.md) — 模板配置项说明
- [TUI 使用指南](tui-guide.md) — 主题配置
- [扩展指南](extending.md) — Hook、Plugin、Skill 扩展
- [Provider 配置](provider-guide.md) — Provider 配置指南