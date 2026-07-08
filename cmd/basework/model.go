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

func init() {
	modelCmd.AddCommand(modelListCmd)
	modelCmd.AddCommand(modelUseCmd)
	modelListCmd.Flags().BoolVar(&modelListFreeOnly, "free", false, "只显示无需 API key 的免费模型")
}

// modelEntry 描述一个已知的模型
type modelEntry struct {
	Provider string
	ModelID  string
	Free     bool
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

// runModelList 列出所有已知模型
func runModelList(out io.Writer, freeOnly bool) error {
	w := tabwriter.NewWriter(out, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "Provider\tModel ID\tFree")
	fmt.Fprintln(w, "--------\t--------\t----")
	for _, m := range knownModels {
		if freeOnly && !m.Free {
			continue
		}
		free := "no"
		if m.Free {
			free = "yes"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", m.Provider, m.ModelID, free)
	}
	return w.Flush()
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
