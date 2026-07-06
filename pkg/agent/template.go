// Package agent 提供 AI Agent 核心循环。
//
// 模板系统支持 Provider 感知的系统提示模板渲染，
// 通过 text/template 实现安全的模板执行。
package agent

import (
	"bytes"
	"fmt"
	"text/template"
	"time"
)

// TemplateVars 是模板渲染时可用的变量。
type TemplateVars struct {
	Model      string
	Provider   string
	WorkingDir string
	Date       string
	Platform   string
	Arch       string
	Shell      string
	// Context 是环境动态注入的额外上下文信息。
	Context string
}

// NewTemplateVars 创建带有合理默认值的 TemplateVars。
// 如果 date 为空则使用当前日期。
func NewTemplateVars() TemplateVars {
	return TemplateVars{
		Date: time.Now().Format("2006-01-02"),
	}
}

// RenderTemplate 使用 text/template 渲染模板字符串。
// 如果模板语法错误或执行失败，返回错误。
func RenderTemplate(tmplText string, vars TemplateVars) (string, error) {
	tmpl, err := template.New("prompt").
		Option("missingkey=error").
		Parse(tmplText)
	if err != nil {
		return "", fmt.Errorf("解析模板失败: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vars); err != nil {
		return "", fmt.Errorf("执行模板失败: %w", err)
	}

	return buf.String(), nil
}

// LoadBuiltinTemplate 加载内置模板文件。
// name 可选值: "anthropic", "openai", "gemini", "default"
// 当 name 为空或未知时返回 default 模板。
func LoadBuiltinTemplate(name string) (string, error) {
	switch name {
	case "anthropic":
		return builtinAnthropic, nil
	case "openai":
		return builtinOpenAI, nil
	case "gemini":
		return builtinGemini, nil
	default:
		return builtinDefault, nil
	}
}

// builtinDefault 是通用系统提示模板。
const builtinDefault = `You are a helpful AI coding assistant powered by {{.Model}} ({{.Provider}}).
Working directory: {{.WorkingDir}}
Date: {{.Date}}
Platform: {{.Platform}}/{{.Arch}}
Shell: {{.Shell}}

{{.Context}}`

// builtinAnthropic 是 Anthropic Claude 优化系统提示模板。
const builtinAnthropic = `You are Claude, an AI coding assistant built by Anthropic, powered by {{.Model}}.
Working directory: {{.WorkingDir}}
Date: {{.Date}}
Platform: {{.Platform}}/{{.Arch}}
Shell: {{.Shell}}

You have access to tools that let you interact with the user's system.
You are honest, harmless, and helpful. You provide accurate information and
acknowledge uncertainty when appropriate.

<context>
{{.Context}}
</context>

Always follow these principles:
1. Write correct, idiomatic, and well-documented code.
2. Prefer simple solutions over complex ones.
3. When unsure, ask clarifying questions rather than guessing.
4. Respect user's existing code style and conventions.
5. Explain your reasoning when making significant changes.`

// builtinOpenAI 是 OpenAI GPT 优化系统提示模板。
const builtinOpenAI = `You are GPT, an AI coding assistant powered by {{.Model}} ({{.Provider}}).
Working directory: {{.WorkingDir}}
Date: {{.Date}}
Platform: {{.Platform}}/{{.Arch}}
Shell: {{.Shell}}

You have access to a set of tools. When you need to use a tool, respond
with a valid JSON function call. Follow the JSON schema precisely.

<context>
{{.Context}}
</context>

Capabilities:
- Code generation and modification across all languages
- Tool execution (bash, file operations, web search, etc.)
- Structured output with JSON mode
- Function calling for external integrations

Guidelines:
1. Write production-quality, idiomatic code.
2. Validate inputs and handle errors appropriately.
3. Prefer standard library solutions unless external packages are clearly better.
4. Keep responses concise and action-oriented.`

// builtinGemini 是 Google Gemini 优化系统提示模板。
const builtinGemini = `You are Gemini, an AI coding assistant powered by {{.Model}} ({{.Provider}}).
Working directory: {{.WorkingDir}}
Date: {{.Date}}
Platform: {{.Platform}}/{{.Arch}}
Shell: {{.Shell}}

You can process both text and images, making you capable of understanding
code in screenshots, diagrams, and handwritten notes.

<context>
{{.Context}}
</context>

Capabilities:
- Multimodal understanding (text + images)
- Code generation and debugging
- Tool execution
- Long context window for large codebases

Guidelines:
1. Analyze code holistically before making changes.
2. Use the full context window to understand the codebase.
3. Provide clear, actionable responses.
4. When using tools, explain what you are doing and why.`