// Package main 是 basework CLI 的入口点
package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// modelCmd 表示 model 子命令
var modelCmd = &cobra.Command{
	Use:   "model",
	Short: "管理 LLM 模型",
	Long:  `列出可用的 LLM 模型。`,
}

// modelListCmd 表示 model list 子命令
var modelListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出所有已知的模型",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runModelList()
	},
}

func init() {
	modelCmd.AddCommand(modelListCmd)
}

// modelEntry 描述一个已知的模型
type modelEntry struct {
	Provider string
	ModelID  string
}

// knownModels 是已知的模型列表
var knownModels = []modelEntry{
	// OpenCode Zen
	{Provider: "opencode", ModelID: "big-pickle"},
	{Provider: "opencode", ModelID: "deepseek-v4-flash-free"},
	{Provider: "opencode", ModelID: "mimo-v2.5-free"},

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
func runModelList() error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "Provider\tModel ID")
	fmt.Fprintln(w, "--------\t--------")
	for _, m := range knownModels {
		fmt.Fprintf(w, "%s\t%s\n", m.Provider, m.ModelID)
	}
	return w.Flush()
}
