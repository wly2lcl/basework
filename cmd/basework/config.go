package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/wly2lcl/basework/pkg/config"
	"github.com/wly2lcl/basework/pkg/provider"
)

// 本文件是配置解释入口（CFG-001）。
//
// 三条铁律：
//  1. 只读——不写配置文件，不调用任何会改状态的东西；
//  2. 不发网络请求——解释配置不该依赖任何远端，也不该因此把 key 发出去；
//  3. 秘密绝不进 stdout/stderr——所有敏感字段经 config.RedactedView 脱敏，
//     API key 只显示「是否设置 + 来源」，永远不显示值本身。
//
// 生效值的口径与运行入口保持一致：provider 的环境覆盖读 BASEWORK_PROVIDER
//（同 newRuntimeAgent），端点覆盖读 BASEWORK_BASE_URL（同 newRuntimeAgent），
// 自定义端点解析复用 config.ResolveEndpoint（同 Provider 创建），API key 来源
// 判定复用 providerAPIKey（同 Provider 创建）。
// 两边必须同步改：这里展示的和 runtime 用的不是同一套逻辑，就会出现
// "explain 说 A、runtime 用 B"的信任裂缝。

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "查看与解释配置（只读，不改动）",
}

var configExplainCmd = &cobra.Command{
	Use:   "explain",
	Short: "输出配置来源、环境覆盖与脱敏后的有效值",
	RunE:  runConfigExplain,
}

func init() {
	configCmd.AddCommand(configExplainCmd)
}

func runConfigExplain(cmd *cobra.Command, args []string) error {
	cfgPath := cfgFile
	if cfgPath == "" {
		var err error
		cfgPath, err = config.Discover()
		if err != nil {
			return fmt.Errorf("查找配置文件失败: %w", err)
		}
	}
	store, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}
	return renderConfigExplain(os.Stdout, cfgPath, store.Get())
}

// renderConfigExplain 渲染解释输出。写到注入的 io.Writer 而不是直接 print，
// 让「秘密不出现在输出里」这件事可以直接被测试断言。
func renderConfigExplain(w io.Writer, cfgPath string, cfg *config.Config) error {
	// --- 来源 ---
	exists := true
	if _, err := os.Stat(cfgPath); err != nil {
		exists = false
	}
	fmt.Fprintf(w, "配置来源: %s", cfgPath)
	if exists {
		fmt.Fprintln(w, "（已加载）")
	} else {
		fmt.Fprintln(w, "（文件不存在，当前全部为默认值）")
	}
	fmt.Fprintln(w, "解析顺序: 当前目录 → 父目录逐级向上 → ~/.config/basework/config.json")

	// --- 环境覆盖（与 runtime 同口径） ---
	fmt.Fprintln(w, "\n环境覆盖:")
	envProvider := os.Getenv("BASEWORK_PROVIDER")
	if envProvider != "" {
		fmt.Fprintf(w, "  BASEWORK_PROVIDER=%s → 生效 provider（覆盖配置里的 %q）\n", envProvider, cfg.Provider)
	} else {
		fmt.Fprintf(w, "  BASEWORK_PROVIDER 未设置 → 生效 provider = 配置值 %q\n", cfg.Provider)
	}
	auditMode := strings.TrimSpace(os.Getenv("BASEWORK_AUDIT_MODE"))
	if auditMode != "" {
		fmt.Fprintf(w, "  BASEWORK_AUDIT_MODE=%s\n", auditMode)
	} else {
		fmt.Fprintln(w, "  BASEWORK_AUDIT_MODE 未设置 → 默认 compatible")
	}
	// CFG-004：端点也有环境覆盖，口径与 runtime 一致。
	// 打印前去掉 URL 里可能内嵌的 userinfo（https://user:pass@host）。
	if envBaseURL := strings.TrimSpace(os.Getenv("BASEWORK_BASE_URL")); envBaseURL != "" {
		fmt.Fprintf(w, "  BASEWORK_BASE_URL=%s → 生效自定义端点（覆盖配置文件 providers.<provider>.base_url）\n",
			config.DisplayURL(envBaseURL))
	} else {
		fmt.Fprintln(w, "  BASEWORK_BASE_URL 未设置 → 生效端点取配置文件 providers.<provider>.base_url")
	}

	// --- 生效 provider ---
	effectiveProvider := cfg.EffectiveProviderName(envProvider)
	fmt.Fprintf(w, "\n生效 provider: %s   生效 model: %s\n", effectiveProvider, cfg.Model)

	// --- 生效自定义端点（CFG-004） ---
	// 与 runtime 用同一个解析函数、同一个环境变量：这里显示的就是 provider.Create
	// 实际会收到的 BaseURL。URL 不是秘密（排查时需要看到），但不显示任何 key。
	endpoint := cfg.ResolveEndpoint(effectiveProvider, os.Getenv("BASEWORK_BASE_URL"))
	fmt.Fprintln(w, "\n自定义模型端点:")
	if endpoint.IsCustom() {
		fmt.Fprintf(w, "  生效 base_url: %s\n", config.DisplayURL(endpoint.BaseURL))
		if endpoint.Source == config.EndpointSourceEnv {
			fmt.Fprintln(w, "  来源: 环境变量 BASEWORK_BASE_URL（覆盖配置文件）")
		} else {
			fmt.Fprintf(w, "  来源: 配置文件 providers.%s.base_url\n", effectiveProvider)
		}
	} else {
		fmt.Fprintf(w, "  未配置 → provider %q 使用内置默认端点\n", effectiveProvider)
		if effectiveProvider == "openai-compat" {
			fmt.Fprintln(w, "  注意: provider \"openai-compat\" 没有内置默认端点，必须提供 base_url，否则启动会失败")
		}
	}

	// --- 启动预设（CFG-002，ADR 0006） ---
	// Load 已把文件内 preset 展开进 Tools.Allowed / Permission；命令行 --preset
	// 只在 agent/tui 启动时叠加，explain 是纯查看口，所以这里显示的是
	// 「配置文件层」的最终结果，并说明命令行参数会再叠加。
	if cfg.Preset != "" {
		fmt.Fprintf(w, "\n启动预设: %s（文件内声明，加载时已展开；命令行 --preset 可叠加，冲突报错）\n", cfg.Preset)
		switch cfg.Preset {
		case config.PresetReadonly:
			fmt.Fprintf(w, "  工具白名单: %s\n", strings.Join(cfg.Tools.Allowed, ", "))
			fmt.Fprintf(w, "  权限检查: 强制开启（mode=%s）\n", cfg.Permission.Mode)
		case config.PresetCoding:
			fmt.Fprintln(w, "  完整能力：不裁剪工具、不改权限")
		}
	} else {
		fmt.Fprintln(w, "\n启动预设: 未指定（可用 readonly / coding，见 docs/adr/0006）")
	}

	// --- API key 来源（只报有无，不报值） ---
	fmt.Fprintln(w, "\nAPI key 状态（只显示是否设置与来源，不显示值）:")
	for _, name := range provider.SortedNames() {
		fmt.Fprintf(w, "  %-16s %s\n", name, apiKeySource(cfg, name))
	}

	// --- 脱敏后的有效配置 ---
	view, err := config.RedactedView(cfg)
	if err != nil {
		return fmt.Errorf("生成脱敏视图失败: %w", err)
	}
	encoded, err := json.MarshalIndent(view, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}
	fmt.Fprintf(w, "\n有效配置（敏感字段已脱敏，标记 %s）:\n%s\n", config.RedactionMarker, string(encoded))

	fmt.Fprintln(w, "\n说明: 本命令只读——不改动配置文件、不发起网络请求。")
	return nil
}

// apiKeySource 判定某 provider 的 key 来源。只返回来源描述，不返回 key 本身。
//
// 判定顺序与 providerAPIKey（Provider 创建时的实际取值路径）一致：
// 配置字段优先，其次环境变量链。不能用 RequiresAPIKey 之类的静态声明——
// 它与 runtime 实际读不读 key 是两回事（bedrock/opencode 都会读），解释输出
// 必须以 runtime 的真实行为为准。
func apiKeySource(cfg *config.Config, providerType string) string {
	if configHasKey(cfg, providerType) {
		return "已设置（来源：配置文件）"
	}
	vars := apiKeyEnvVars(providerType)
	for _, envVar := range vars {
		if os.Getenv(envVar) != "" {
			return fmt.Sprintf("已设置（来源：环境变量 %s）", envVar)
		}
	}
	if len(vars) == 0 {
		return "无需 key"
	}
	return fmt.Sprintf("未设置（可配置在配置文件，或环境变量 %s）", strings.Join(vars, " → "))
}

// apiKeyEnvVars 返回该 provider 可能读取的环境变量链，与 lookupAPIKey 的
// 读取顺序同构（含 opencode 的多级回落）。新增 provider 时两处一起改。
func apiKeyEnvVars(providerType string) []string {
	switch providerType {
	case "agnes-responses":
		return []string{"AGNES_API_KEY"}
	case "openai", "openai-compat":
		return []string{"OPENAI_API_KEY"}
	case "anthropic":
		return []string{"ANTHROPIC_API_KEY"}
	case "gemini":
		return []string{"GOOGLE_API_KEY"}
	case "opencode":
		return []string{"OPENCODE_API_KEY", "OG_API_KEY", "OPENAI_API_KEY"}
	case "bedrock":
		return []string{"AWS_ACCESS_KEY_ID"}
	case "azure":
		return []string{"AZURE_API_KEY"}
	case "copilot", "ollama":
		// 与 lookupAPIKey 一致：这两类 provider 运行时不读任何 key。
		return nil
	default:
		return []string{"OPENAI_API_KEY"}
	}
}

// configHasKey 判断配置文件里是否直接写了该 provider 的 key。
// 与 providerAPIKey 的 switch 保持同构：新增 provider 时两处一起改。
func configHasKey(cfg *config.Config, providerType string) bool {
	// CFG-004：provider 级 api_key 是通用载体，优先判定。
	if cfg.ProviderEndpointFor(providerType).APIKey != "" {
		return true
	}
	switch providerType {
	case "opencode":
		return cfg.OpenCode.APIKey != ""
	case "azure":
		return cfg.Azure.APIKey != ""
	case "bedrock":
		return cfg.Bedrock.AccessKey != ""
	default:
		return false
	}
}
