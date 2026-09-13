// Package main 是 basework CLI 的入口点
package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/wly2lcl/basework/pkg/config"
	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/provider"
)

var modelListFreeOnly bool

// modelCmd 表示 model 子命令
var modelCmd = &cobra.Command{
	Use:   "model",
	Short: "管理 LLM 模型",
	Long:  `列出或切换可用的 LLM 模型。`,
}

// modelListCmd 表示 model list 子命令
var modelListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出所有已知的模型",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runModelList(os.Stdout, modelListFreeOnly)
	},
}

// modelUseCmd 表示 model use 子命令
var modelUseCmd = &cobra.Command{
	Use:   "use <model-id>",
	Short: "切换当前使用的模型",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runModelUse(args[0])
	},
}

// modelInfoCmd 表示 model info 子命令
var modelInfoCmd = &cobra.Command{
	Use:   "info <model-id>",
	Short: "显示模型的能力结论、来源与 key 要求",
	Long: `显示模型的能力结论与依据。

能力结论有三种：
  supported    已确认支持
  unsupported  已确认不支持
  unknown      没有依据（未声明也未实测），不代表支持

“免费”与“需要 API key”是两个独立属性，本命令分别展示。`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runModelInfo(os.Stdout, args[0])
	},
}

func init() {
	modelCmd.AddCommand(modelListCmd)
	modelCmd.AddCommand(modelUseCmd)
	modelCmd.AddCommand(modelInfoCmd)
	modelListCmd.Flags().BoolVar(&modelListFreeOnly, "free", false, "只显示无需 API key 的免费模型")
}

// modelEntry 描述一个已知的模型
type modelEntry struct {
	Provider string
	ModelID  string
	Free     bool
}

// requiresKey 报告该模型在当前实现下是否需要 API key。
//
// “免费”与“需要 API key”是两件独立的事：opencode 在协议层允许空 key，但只有它
// 的免费模型能在无 key 的情况下真正跑起来；其余 provider 一律需要 key。
// 这里不复用 Free 字段来推断整个 provider 的规则，避免把两者混为一谈。
func (m modelEntry) requiresKey() bool {
	if !provider.RequiresAPIKey(m.Provider) {
		return !m.Free
	}
	return true
}

// keyStatus 返回 CLI 展示用的 key 需求文本。
func (m modelEntry) keyStatus() string {
	if m.requiresKey() {
		return "required"
	}
	return "not required"
}

// knownModels 是已知的模型列表
var knownModels = []modelEntry{
	// OpenCode Zen
	{Provider: "opencode", ModelID: "big-pickle", Free: true},
	{Provider: "opencode", ModelID: "deepseek-v4-flash-free", Free: true},
	{Provider: "opencode", ModelID: "mimo-v2.5-free", Free: true},
	{Provider: "opencode", ModelID: "nemotron-3-ultra-free", Free: true},
	{Provider: "opencode", ModelID: "nemotron-3-super-free", Free: true},
	{Provider: "opencode", ModelID: "north-mini-code-free", Free: true},

	// OpenAI
	{Provider: "openai", ModelID: "gpt-4o"},
	{Provider: "openai", ModelID: "gpt-4o-mini"},
	{Provider: "openai", ModelID: "gpt-4-turbo"},
	{Provider: "openai", ModelID: "gpt-4"},
	{Provider: "openai", ModelID: "gpt-3.5-turbo"},
	{Provider: "openai", ModelID: "o1"},
	{Provider: "openai", ModelID: "o1-mini"},
	{Provider: "openai", ModelID: "o3-mini"},

	// Anthropic
	{Provider: "anthropic", ModelID: "claude-sonnet-4-20250514"},
	{Provider: "anthropic", ModelID: "claude-3-5-sonnet-20241022"},
	{Provider: "anthropic", ModelID: "claude-3-5-haiku-20241022"},
	{Provider: "anthropic", ModelID: "claude-opus-4-20250514"},

	// Gemini
	{Provider: "gemini", ModelID: "gemini-2.5-flash"},
	{Provider: "gemini", ModelID: "gemini-2.5-pro"},
	{Provider: "gemini", ModelID: "gemini-2.0-flash"},

	// DeepSeek (OpenAI compatible)
	{Provider: "deepseek", ModelID: "deepseek-chat"},
	{Provider: "deepseek", ModelID: "deepseek-reasoner"},

	// Groq (OpenAI compatible)
	{Provider: "groq", ModelID: "llama-3.3-70b-versatile"},

	// Together (OpenAI compatible)
	{Provider: "together", ModelID: "mistralai/Mixtral-8x7B-Instruct-v0.1"},

	// OpenRouter
	{Provider: "openrouter", ModelID: "anthropic/claude-3.5-sonnet"},

	// xAI
	{Provider: "xai", ModelID: "grok-2"},
	{Provider: "xai", ModelID: "grok-3"},

	// Mistral
	{Provider: "mistral", ModelID: "mistral-large-latest"},
	{Provider: "mistral", ModelID: "mistral-small-latest"},
}

// runModelList 列出所有已知模型。
//
// 能力列只反映有依据的结论：`yes` = 已声明支持，`no` = 已声明不支持，
// `unknown` = 没有依据。历史实现直接声明所有 openai-compat 模型支持视觉输入，
// 那是把未验证的猜测当成结论，这里不再这样做。
func runModelList(out io.Writer, freeOnly bool) error {
	w := tabwriter.NewWriter(out, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "Provider\tModel ID\tTools\tStreaming\tVision\tAPI key\tFree")
	fmt.Fprintln(w, "--------\t--------\t-----\t---------\t------\t-------\t----")
	for _, m := range knownModels {
		if freeOnly && !m.Free {
			continue
		}
		caps := provider.CapabilitiesFor(m.Provider, m.ModelID)
		free := "no"
		if m.Free {
			free = "yes"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			m.Provider, m.ModelID,
			caps[llm.CapTools].Support.ShortLabel(),
			caps[llm.CapStreaming].Support.ShortLabel(),
			caps[llm.CapVision].Support.ShortLabel(),
			m.keyStatus(),
			free,
		)
	}
	return w.Flush()
}

// runModelInfo 展示单个模型的能力依据与 key 要求。
func runModelInfo(out io.Writer, modelID string) error {
	entry, ok := findKnownModel(modelID)
	if !ok {
		return fmt.Errorf("未知模型 %q，请运行 `basework model list` 查看可用模型", modelID)
	}

	fmt.Fprintf(out, "模型: %s/%s\n", entry.Provider, entry.ModelID)
	fmt.Fprintf(out, "需要 API key: %s", entry.keyStatus())
	if entry.requiresKey() {
		fmt.Fprintf(out, "（环境变量 %s；若该 provider 支持配置文件中的 key，配置文件优先）",
			provider.APIKeyEnvVar(entry.Provider))
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "免费: %v\n", entry.Free)
	fmt.Fprintln(out, "能力:\n  capability  support     source")
	caps := provider.CapabilitiesFor(entry.Provider, entry.ModelID)
	for _, cap := range provider.CapabilityOrder() {
		d := caps[cap]
		fmt.Fprintf(out, "  %-11s %-11s %s\n", cap, d.Support.String(), d.Source)
		if d.Note != "" {
			fmt.Fprintf(out, "  %-11s %s\n", "", d.Note)
		}
	}
	if unknown := unknownCapabilities(caps); len(unknown) > 0 {
		fmt.Fprintf(out, "\n注意: %s 的结论为 unknown，表示未声明也未实测，不代表支持。\n",
			strings.Join(unknown, ", "))
	}
	return nil
}

// unknownCapabilities 返回结论为 unknown 的能力名，保持稳定顺序。
func unknownCapabilities(caps map[llm.Capability]llm.CapabilityDetail) []string {
	var out []string
	for _, cap := range provider.CapabilityOrder() {
		if caps[cap].Support == llm.SupportUnknown {
			out = append(out, string(cap))
		}
	}
	return out
}

func runModelUse(modelID string) error {
	entry, ok := findKnownModel(modelID)
	if !ok {
		return fmt.Errorf("未知模型 %q，请运行 `basework model list` 查看可用模型", modelID)
	}

	cfgPath := cfgFile
	if cfgPath == "" {
		var err error
		cfgPath, err = config.Discover()
		if err != nil {
			return fmt.Errorf("发现配置文件失败: %w", err)
		}
	}
	store, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}

	// 切换前先确认这个模型真的能跑。缺 key 时给出明确错误和可选替代，
	// 而不是先把配置改掉、等到第一次请求才失败，也不静默换成别的模型。
	if entry.requiresKey() && providerAPIKey(store.Get(), entry.Provider) == "" {
		return fmt.Errorf(
			"模型 %s/%s 需要 API key，但当前未找到（环境变量 %s）\n"+
				"可选替代：\n"+
				"  1. 设置环境变量 %s 后重试\n"+
				"  2. 在配置文件中填写该 provider 的凭据\n"+
				"  3. 改用无需 key 的免费模型：%s",
			entry.Provider, entry.ModelID, provider.APIKeyEnvVar(entry.Provider),
			provider.APIKeyEnvVar(entry.Provider),
			strings.Join(freeModelIDs(3), ", "),
		)
	}

	if err := store.Mutate(func(c *config.Config) {
		c.Provider = entry.Provider
		c.Model = entry.ModelID
	}); err != nil {
		return fmt.Errorf("更新配置失败: %w", err)
	}
	if err := store.Save(); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}

	if entry.Free {
		fmt.Printf("已切换到免费模型: %s/%s\n", entry.Provider, entry.ModelID)
	} else {
		fmt.Printf("已切换到模型: %s/%s\n", entry.Provider, entry.ModelID)
	}
	return nil
}

// freeModelIDs 返回前 n 个免费模型 ID，用于缺 key 时的替代建议。
func freeModelIDs(n int) []string {
	out := make([]string, 0, n)
	for _, m := range knownModels {
		if !m.Free {
			continue
		}
		out = append(out, m.ModelID)
		if len(out) == n {
			break
		}
	}
	if len(out) == 0 {
		out = append(out, "(当前无免费模型)")
	}
	return out
}

func findKnownModel(modelID string) (modelEntry, bool) {
	modelID = strings.TrimSpace(modelID)
	for _, m := range knownModels {
		if m.ModelID == modelID {
			return m, true
		}
	}
	return modelEntry{}, false
}

func openCodeFreeModelEntries() []modelEntry {
	models := make([]modelEntry, 0)
	for _, m := range knownModels {
		if m.Provider == "opencode" && m.Free {
			models = append(models, m)
		}
	}
	return models
}
