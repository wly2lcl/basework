// Package main 是 basework CLI 的入口点
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	intruntime "github.com/wly2lcl/basework/internal/runtime"
	"github.com/wly2lcl/basework/pkg/agent"
	"github.com/wly2lcl/basework/pkg/config"
	"github.com/wly2lcl/basework/pkg/llm"
)

// agentPreset 是 agent 子命令的 --preset 启动预设（CFG-002）。
var agentPreset string
var agentSessionID string

// agentCmd 表示 agent 子命令
var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "启动交互式 Agent 会话",
	Long: `启动一个交互式 Agent 会话，默认进入 REPL 模式。
使用 -m "message" 可执行一次性对话。`,
	RunE: runAgentE,
}

var (
	noStream bool
	message  string
)

func init() {
	agentCmd.Flags().BoolVar(&noStream, "no-stream", false, "关闭流式输出（一次性输出完整响应）")
	agentCmd.Flags().StringVarP(&message, "message", "m", "", "一次性消息模式，指定后直接发送消息并退出")
	agentCmd.Flags().StringVar(&agentPreset, "preset", "", "启动预设（readonly / coding），与配置文件 preset 冲突时报错")
	agentCmd.Flags().StringVar(&agentSessionID, "session", "", "恢复指定会话 ID（省略则创建新会话）")
}

// runAgentE 执行 agent 子命令
func runAgentE(cmd *cobra.Command, args []string) error {
	// 1. 加载配置
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
	cfg := store.Get()

	rt, err := newRuntimeAgent(cfg, runtimeAgentOptions{Preset: agentPreset, SessionID: agentSessionID})
	if err != nil {
		return err
	}
	// RUN-003：运行统一走运行服务。Service.Close 会先取消进行中的运行、
	// 再释放 Agent 与任务管理器——退出路径只有一条，资源不会双关或漏关。
	svc := rt.Service()
	if svc == nil {
		// Service 构造失败只在缺 Agent 时发生；这里如实报错而非静默降级。
		return fmt.Errorf("构造运行服务失败")
	}
	defer svc.Close()

	if verbose {
		fmt.Fprintf(os.Stderr, "配置: provider=%s model=%s tools=%d\n", rt.ProviderName, rt.ModelName, len(rt.Agent.Tools()))
	}
	reportRuntimeCapabilities(rt, verbose)

	if message != "" {
		return handleOneShot(cmd.Context(), svc, message)
	}
	return runREPL(cmd.Context(), svc)
}

// runAgent 是默认子命令处理器（当 basework 无子命令时调用）
func runAgent(cmd *cobra.Command, args []string) {
	if err := runAgentE(cmd, args); err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}

// handleOneShot 处理一次性消息模式（RUN-003：经运行服务，取消与错误语义
// 与 REPL/TUI 一致——取消体现为 Run.Err 的 context.Canceled，而非裸 error）。
func handleOneShot(ctx context.Context, svc intruntime.Service, msg string) error {
	run, err := svc.Start(ctx, msg)
	if err != nil {
		return fmt.Errorf("启动运行失败: %w", err)
	}
	if run.Err != nil {
		return fmt.Errorf("处理消息失败: %w", run.Err)
	}
	if run.Response != nil {
		fmt.Println(responseText(run.Response))
	}
	return nil
}

// runREPL 运行交互式 REPL 循环（RUN-003：运行经 Service，取消/错误语义
// 与 TUI 同源——都是 Run.Err 的 context.Canceled 与统一错误包装）。
func runREPL(ctx context.Context, svc intruntime.Service) error {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Fprintln(os.Stderr, "进入交互模式，输入 exit 或 Ctrl+D 退出")

	for {
		// 检测 Ctrl+C 中断
		replCtx, cancel := signal.NotifyContext(ctx, syscall.SIGINT)

		fmt.Fprint(os.Stderr, "> ")
		scanned := scanner.Scan()
		if !scanned {
			// Ctrl+D 或 EOF
			if err := scanner.Err(); err != nil {
				return fmt.Errorf("读取输入失败: %w", err)
			}
			fmt.Fprintln(os.Stderr)
			return nil
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		if input == "exit" {
			fmt.Fprintln(os.Stderr, "再见！")
			return nil
		}

		// 处理消息（经运行服务；SIGINT 取消 replCtx 传导为运行取消）
		run, err := svc.Start(replCtx, input)
		wasCanceled := replCtx.Err() != nil
		cancel() // 确保取消信号处理

		if err != nil {
			return fmt.Errorf("启动运行失败: %w", err)
		}
		if run.Err != nil {
			// 检查是否被中断：取消在 Run.Err 里体现为 context.Canceled
			if errors.Is(run.Err, context.Canceled) || wasCanceled {
				fmt.Fprintln(os.Stderr, "\n[响应被中断]")
				continue
			}
			return fmt.Errorf("处理消息失败: %w", run.Err)
		}

		// 输出响应
		if run.Response != nil {
			fmt.Println(responseText(run.Response))
		}
	}
}

func responseText(resp *agent.Response) string {
	if resp == nil {
		return ""
	}
	var b strings.Builder
	for _, part := range resp.Message.Content {
		if part.Type == llm.ContentTypeText {
			b.WriteString(part.Text)
		}
	}
	return b.String()
}

// lookupAPIKey 根据 provider 类型查找对应的环境变量
func lookupAPIKey(providerType string) string {
	switch providerType {
	case "agnes-responses":
		if key := os.Getenv("AGNES_API_KEY"); key != "" {
			return key
		}
	case "openai", "openai-compat":
		if key := os.Getenv("OPENAI_API_KEY"); key != "" {
			return key
		}
	case "anthropic":
		if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
			return key
		}
	case "gemini":
		if key := os.Getenv("GOOGLE_API_KEY"); key != "" {
			return key
		}
	case "opencode":
		if key := os.Getenv("OPENCODE_API_KEY"); key != "" {
			return key
		}
		if key := os.Getenv("OG_API_KEY"); key != "" {
			return key
		}
		if key := os.Getenv("OPENAI_API_KEY"); key != "" {
			return key
		}
	case "bedrock":
		return os.Getenv("AWS_ACCESS_KEY_ID")
	case "azure":
		if key := os.Getenv("AZURE_API_KEY"); key != "" {
			return key
		}
	case "copilot":
		return ""
	case "ollama":
		return ""
	}
	// 通用 fallback
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		return key
	}
	return ""
}

// endpointLabel 给出自定义端点来源的可读标签，用于形态校验失败时的错误信息。
// 标签必须指明「去哪里改」，否则用户只能看到一句「URL 不合法」。
func endpointLabel(providerType string, source config.EndpointSource) string {
	if source == config.EndpointSourceEnv {
		return "BASEWORK_BASE_URL（环境变量）"
	}
	return fmt.Sprintf("providers.%s.base_url（配置文件 provider %q）", providerType, providerType)
}

// providerAPIKey 按 provider 取得实际使用的 API key。
//
// 优先级：配置文件 providers.<生效 provider>.api_key（CFG-004，显式配置）
// → provider 专属配置块（opencode / azure / bedrock）→ 环境变量链。
// `config explain` 的 configHasKey 必须与此保持同构，否则会出现
// 「explain 说没 key、启动却说有」这类信任裂缝。
func providerAPIKey(cfg *config.Config, providerType string) string {
	if key := cfg.ProviderEndpointFor(providerType).APIKey; key != "" {
		return key
	}
	switch providerType {
	case "opencode":
		if cfg.OpenCode.APIKey != "" {
			return cfg.OpenCode.APIKey
		}
	case "azure":
		if cfg.Azure.APIKey != "" {
			return cfg.Azure.APIKey
		}
	case "bedrock":
		if cfg.Bedrock.AccessKey != "" {
			return cfg.Bedrock.AccessKey
		}
	}
	return lookupAPIKey(providerType)
}

// buildProviderOptions 根据配置构建 provider 的 Options map。
//
// endpoint 是 CFG-004 解析出的生效自定义端点，只有 ollama 需要它：
// ollama 的构造器同时认 BaseURL 与 opts["endpoint"]，两者都传时后者赢。
// 若不加处理，同时写了 ollama.endpoint 与 providers.ollama.base_url 的用户
// 会遇到「explain 显示 A、实际用 B」。ADR 0007 定的是 provider 级 base_url 为准，
// 所以有自定义端点时不再下发旧的 endpoint。
func buildProviderOptions(cfg *config.Config, providerType string, endpoint config.Endpoint) map[string]any {
	opts := make(map[string]any)

	switch providerType {
	case "bedrock":
		opts["access_key"] = cfg.Bedrock.AccessKey
		if opts["access_key"] == "" {
			opts["access_key"] = os.Getenv("AWS_ACCESS_KEY_ID")
		}
		opts["secret_key"] = cfg.Bedrock.SecretKey
		if opts["secret_key"] == "" {
			opts["secret_key"] = os.Getenv("AWS_SECRET_ACCESS_KEY")
		}
		opts["region"] = cfg.Bedrock.Region
		if opts["region"] == "" {
			opts["region"] = os.Getenv("AWS_REGION")
		}
		if opts["region"] == "" {
			opts["region"] = "us-east-1"
		}

	case "azure":
		opts["resource"] = cfg.Azure.Resource
		opts["deployment"] = cfg.Azure.Deployment
		opts["api_version"] = cfg.Azure.APIVersion
		if opts["api_version"] == "" {
			opts["api_version"] = "2024-02-01"
		}
		opts["api_key"] = cfg.Azure.APIKey

	case "copilot":
		opts["token_path"] = cfg.Copilot.TokenPath

	case "ollama":
		if !endpoint.IsCustom() {
			opts["endpoint"] = cfg.Ollama.Endpoint
		}

	case "opencode":
		if cfg.OpenCode.APIKey != "" {
			opts["api_key"] = cfg.OpenCode.APIKey
		}
	}

	return opts
}

// getSessionDir 返回会话存储的规范目录。
//
// 这是 agent 实际写入会话的位置，也是所有 session 子命令应当读取的位置。
// getSessionDir 返回会话数据根目录。var 形式便于测试注入隔离目录
// （生产代码不改动它）。
var getSessionDir = func() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "basework-sessions")
	}
	return filepath.Join(home, ".local", "share", "basework", "sessions")
}

// legacySessionDir 返回历史版本的会话目录。
//
// v0.2.0 之前，部分子命令（session status/unlock、migrate 的 SQLite 目标）
// 硬编码了这个路径，与 agent 实际写入的规范目录不一致，导致这些命令读不到
// 真实会话。此处保留它仅用于向后兼容的只读回退，不再作为写入位置。
func legacySessionDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".basework", "sessions")
}

// resolveSessionDir 返回实际应当使用的会话目录。
//
// 优先规范目录；仅当规范目录不存在而旧目录存在时回退到旧目录，
// 以免升级后已有用户的会话突然"消失"。不在此处搬迁数据：
// 显式合并请使用 `basework migrate sessions`。
func resolveSessionDir() string {
	canonical := getSessionDir()
	if _, err := os.Stat(canonical); err == nil {
		return canonical
	}
	if legacy := legacySessionDir(); legacy != "" {
		if _, err := os.Stat(legacy); err == nil {
			return legacy
		}
	}
	return canonical
}
