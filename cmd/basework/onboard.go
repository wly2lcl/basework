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
	"github.com/wly2lcl/basework/pkg/provider"
)

var (
	initNonInteractive bool
	initForce          bool
	initProvider       string
	initModel          string
	initPreset         string
)

// initCmd 表示 init 子命令
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "初始化 basework 配置",
	Long: `初始化 basework 配置文件。
默认进入交互式配置；使用 --yes 可在脚本/无人值守环境中确定结束。`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if initNonInteractive {
			return runInitNonInteractive()
		}
		return runInit()
	},
}

func init() {
	initCmd.Flags().BoolVarP(&initNonInteractive, "yes", "y", false, "非交互初始化；不读取 stdin")
	initCmd.Flags().BoolVar(&initForce, "force", false, "允许覆盖已存在的配置文件（默认保留原文件）")
	initCmd.Flags().StringVar(&initProvider, "provider", "", "非交互模式的 provider（默认取唯一环境 key，否则 opencode）")
	initCmd.Flags().StringVar(&initModel, "model", "", "非交互模式的模型（默认按 provider 选择）")
	initCmd.Flags().StringVar(&initPreset, "preset", "", "非交互模式的启动预设：readonly 或 coding")
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

// runInitNonInteractive 是 OPT-001 的无人值守入口。
// 它只从显式参数/环境变量取值，不触碰 stdin；已有配置默认保持不变，--force
// 才允许修改。API key 只从环境读取并用于校验，绝不写入配置或日志。
func runInitNonInteractive() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("获取用户目录失败: %w", err)
	}
	cfgPath := cfgFile
	if cfgPath == "" {
		cfgPath = filepath.Join(home, ".config", "basework", "config.json")
	}
	if _, statErr := os.Stat(cfgPath); statErr == nil && !initForce {
		fmt.Printf("配置文件已存在，保持不变: %s（如需覆盖请使用 --force）\n", cfgPath)
		return nil
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return fmt.Errorf("检查配置文件失败: %w", statErr)
	}

	selectedProvider := strings.TrimSpace(initProvider)
	if selectedProvider == "" {
		for _, ev := range apiEnvVars {
			if os.Getenv(ev.envName) != "" {
				if selectedProvider == "" {
					selectedProvider = ev.provider
				} else if selectedProvider != ev.provider {
					// 多个 key 时不猜用户意图，使用稳定默认并在输出中说明。
					selectedProvider = "opencode"
					break
				}
			}
		}
	}
	if selectedProvider == "" {
		selectedProvider = "opencode"
	}
	known := false
	for _, name := range provider.SortedNames() {
		if name == selectedProvider {
			known = true
			break
		}
	}
	if !known {
		return fmt.Errorf("未知 provider %q；可用值：%s", selectedProvider, strings.Join(provider.SortedNames(), ", "))
	}

	model := strings.TrimSpace(initModel)
	if model == "" {
		model = initDefaultModel(selectedProvider)
	}
	if initPreset != "" && initPreset != config.PresetReadonly && initPreset != config.PresetCoding {
		return fmt.Errorf("未知 preset %q；可用值：readonly、coding", initPreset)
	}
	// OpenCode 的默认 big-pickle 走其免费模型路径，和既有交互初始化一致，
	// 无 key 也可以生成可继续配置的文件；其他 provider 缺 key 直接失败。
	if selectedProvider != "opencode" {
		if vars := apiKeyEnvVars(selectedProvider); len(vars) > 0 {
			hasKey := false
			for _, name := range vars {
				if strings.TrimSpace(os.Getenv(name)) != "" {
					hasKey = true
					break
				}
			}
			if !hasKey {
				return fmt.Errorf("provider %q 缺少 API key；请设置 %s（密钥不会写入配置）",
					selectedProvider, strings.Join(vars, " 或 "))
			}
		}
	}

	store := config.NewStore(cfgPath)
	if _, statErr := os.Stat(cfgPath); statErr == nil {
		if err := store.Reload(); err != nil {
			return fmt.Errorf("加载已有配置失败: %w", err)
		}
	}
	if err := store.Mutate(func(c *config.Config) {
		c.Provider = selectedProvider
		c.Model = model
		if initPreset != "" {
			c.Preset = initPreset
		}
		if c.Temperature == 0 {
			c.Temperature = 0.7
		}
		if c.MaxTokens == 0 {
			c.MaxTokens = 4096
		}
	}); err != nil {
		return err
	}
	if err := store.Save(); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}
	fmt.Printf("非交互初始化完成: provider=%s model=%s", selectedProvider, model)
	if initPreset != "" {
		fmt.Printf(" preset=%s", initPreset)
	}
	fmt.Printf(" 配置=%s\n", cfgPath)
	return nil
}

func initDefaultModel(selectedProvider string) string {
	switch selectedProvider {
	case "opencode":
		return "big-pickle"
	case "openai", "openai-compat":
		return "gpt-4o"
	case "anthropic":
		return "claude-sonnet-4-20250514"
	case "gemini":
		return "gemini-2.5-flash"
	case "ollama":
		return "llama3"
	default:
		return "gpt-4o"
	}
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
