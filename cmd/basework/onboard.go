// Package main 是 basework CLI 的入口点
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/wly2lcl/basework/pkg/config"
)

// initCmd 表示 init 子命令
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "初始化 basework 配置",
	Long: `初始化 basework 配置文件。
检测环境变量中的 API Key 并提供交互式配置。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInit()
	},
}

// envVarInfo 描述一个环境变量
type envVarInfo struct {
	envName  string
	provider string
	label    string
}

// apiEnvVars 是已知的 API Key 环境变量
var apiEnvVars = []envVarInfo{
	{envName: "OPENCODE_API_KEY", provider: "opencode", label: "OpenCode Zen (big-pickle)"},
	{envName: "OG_API_KEY", provider: "opencode", label: "OpenCode Zen (legacy OG_API_KEY)"},
	{envName: "ANTHROPIC_API_KEY", provider: "anthropic", label: "Anthropic (Claude)"},
	{envName: "OPENAI_API_KEY", provider: "openai", label: "OpenAI (GPT)"},
	{envName: "GOOGLE_API_KEY", provider: "gemini", label: "Google (Gemini)"},
}

// runInit 执行初始化配置
func runInit() error {
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Println("=== basework 初始化 ===")
	fmt.Println()

	// 1. 检测环境变量
	fmt.Println("检测到以下 API Key 环境变量：")
	detected := make([]envVarInfo, 0)
	for _, ev := range apiEnvVars {
		val := os.Getenv(ev.envName)
		if val != "" {
			prefix := val
			if len(prefix) > 8 {
				prefix = prefix[:8] + "..."
			}
			fmt.Printf("  ✓ %s (%s) = %s\n", ev.label, ev.envName, prefix)
			detected = append(detected, ev)
		} else {
			fmt.Printf("  ✗ %s (%s) = 未设置\n", ev.label, ev.envName)
		}
	}
	fmt.Println()

	// 2. 选择 provider
	var selectedProvider string
	var enteredAPIKey string
	if len(detected) == 1 {
		selectedProvider = detected[0].provider
		fmt.Printf("使用检测到的 provider: %s\n", detected[0].label)
	} else if len(detected) > 1 {
		fmt.Println("检测到多个 API Key，请选择默认 provider：")
		for i, ev := range detected {
			fmt.Printf("  %d) %s (%s)\n", i+1, ev.label, ev.envName)
		}
		fmt.Print("请输入编号 (1-", len(detected), "): ")
		scanner.Scan()
		choice := strings.TrimSpace(scanner.Text())
		idx := 0
		fmt.Sscanf(choice, "%d", &idx)
		if idx < 1 || idx > len(detected) {
			selectedProvider = detected[0].provider
			fmt.Printf("使用默认: %s\n", detected[0].label)
		} else {
			selectedProvider = detected[idx-1].provider
		}
	} else {
		// 未检测到任何环境变量，交互式输入
		fmt.Println("未检测到 API Key 环境变量。")
		fmt.Println("请选择要配置的 provider：")
		for i, ev := range apiEnvVars {
			fmt.Printf("  %d) %s (%s)\n", i+1, ev.label, ev.envName)
		}
		fmt.Printf("请输入编号 (1-%d): ", len(apiEnvVars))
		scanner.Scan()
		choice := strings.TrimSpace(scanner.Text())
		idx := 0
		fmt.Sscanf(choice, "%d", &idx)
		if idx < 1 || idx > len(apiEnvVars) {
			selectedProvider = "opencode"
			fmt.Println("使用默认: OpenCode Zen")
		} else {
			selectedProvider = apiEnvVars[idx-1].provider
		}

		// 交互式输入 API Key
		fmt.Printf("请输入 %s 的 API Key: ", selectedProvider)
		scanner.Scan()
		apiKey := strings.TrimSpace(scanner.Text())
		if apiKey != "" {
			enteredAPIKey = apiKey
			os.Setenv(strings.ToUpper(selectedProvider)+"_API_KEY", apiKey)
		}
	}

	// 3. 选择模型
	var defaultModel string
	switch selectedProvider {
	case "opencode":
		defaultModel = "big-pickle"
	case "openai":
		defaultModel = "gpt-4o"
	case "anthropic":
		defaultModel = "claude-sonnet-4-20250514"
	case "gemini":
		defaultModel = "gemini-2.5-flash"
	default:
		defaultModel = "gpt-4o"
	}
	if selectedProvider == "opencode" && enteredAPIKey == "" && providerAPIKey(config.NewStore("").Get(), "opencode") == "" {
		defaultModel = promptOpenCodeFreeModel(scanner, defaultModel)
	}
	fmt.Printf("默认模型: %s\n", defaultModel)
	fmt.Println()

	// 4. 确定配置文件路径
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("获取用户目录失败: %w", err)
	}
	cfgDir := filepath.Join(home, ".config", "basework")
	cfgPath := filepath.Join(cfgDir, "config.json")

	// 5. 创建或更新配置
	store := config.NewStore(cfgPath)
	store.Mutate(func(c *config.Config) {
		c.Provider = selectedProvider
		c.Model = defaultModel
		c.Temperature = 0.7
		c.MaxTokens = 4096
	})

	if err := store.Save(); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}

	fmt.Printf("配置文件已写入: %s\n", cfgPath)
	fmt.Println("初始化完成！现在可以使用 `basework agent` 开始对话。")
	return nil
}

func promptOpenCodeFreeModel(scanner *bufio.Scanner, fallback string) string {
	models := openCodeFreeModelEntries()
	if len(models) == 0 {
		return fallback
	}

	fmt.Println("OpenCode 未配置 API Key，将使用免费模型。可选模型：")
	for i, m := range models {
		fmt.Printf("  %d) %s\n", i+1, m.ModelID)
	}
	fmt.Printf("请输入编号 (1-%d，默认 %s): ", len(models), fallback)
	scanner.Scan()
	choice := strings.TrimSpace(scanner.Text())
	if choice == "" {
		return fallback
	}
	idx := 0
	fmt.Sscanf(choice, "%d", &idx)
	if idx < 1 || idx > len(models) {
		fmt.Printf("使用默认免费模型: %s\n", fallback)
		return fallback
	}
	return models[idx-1].ModelID
}
