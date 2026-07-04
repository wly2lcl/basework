// Package main 是 basework CLI 的入口点
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/wly2lcl/basework/pkg/agent"
	"github.com/wly2lcl/basework/pkg/config"
	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/provider"
	"github.com/wly2lcl/basework/pkg/session"
)

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

	// 环境变量覆盖 Provider 选择
	providerType := cfg.Provider
	if envProvider := os.Getenv("BASEWORK_PROVIDER"); envProvider != "" {
		providerType = envProvider
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "配置: provider=%s model=%s\n", providerType, cfg.Model)
	}

	// 2. 构建 provider 选项
	opts := buildProviderOptions(cfg)

	// 3. 创建 LLM 模型
	model, err := provider.Create(provider.Config{
		Type:    providerType,
		ModelID: cfg.Model,
		APIKey:  lookupAPIKey(providerType),
		Options: opts,
	})
	if err != nil {
		return fmt.Errorf("创建 LLM 模型失败: %w", err)
	}

	// 4. 创建 Session 存储
	sessionDir := getSessionDir()
	sess, err := session.NewJSONLStore(sessionDir)
	if err != nil {
		return fmt.Errorf("创建会话存储失败: %w", err)
	}

	// 5. 创建 Agent
	agt, err := agent.New(
		agent.WithModel(model),
		agent.WithSession(sess),
		agent.WithSystemPrompt(cfg.SystemPrompt),
		agent.WithMaxSteps(cfg.MaxIterations),
	)
	if err != nil {
		return fmt.Errorf("创建 Agent 失败: %w", err)
	}
	defer agt.Close()

	// 6. 判断模式：一次性消息 vs REPL
	if message != "" {
		return handleOneShot(cmd.Context(), agt, message)
	}
	return runREPL(cmd.Context(), agt)
}

// runAgent 是默认子命令处理器（当 basework 无子命令时调用）
func runAgent(cmd *cobra.Command, args []string) {
	if err := runAgentE(cmd, args); err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}

// handleOneShot 处理一次性消息模式
func handleOneShot(ctx context.Context, agt agent.Agent, msg string) error {
	resp, err := agt.HandleMessage(ctx, msg)
	if err != nil {
		return fmt.Errorf("处理消息失败: %w", err)
	}
	if resp != nil {
		for _, part := range resp.Message.Content {
			if part.Type == llm.ContentTypeText {
				fmt.Print(part.Text)
			}
		}
		fmt.Println()
	}
	return nil
}

// runREPL 运行交互式 REPL 循环
func runREPL(ctx context.Context, agt agent.Agent) error {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Fprintln(os.Stderr, "进入交互模式，输入 exit 或 Ctrl+D 退出")

	for {
		// 检测 Ctrl+C 中断
		replCtx, cancel := signal.NotifyContext(ctx, syscall.SIGINT)
		defer cancel()

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

		// 处理消息
		resp, err := agt.HandleMessage(replCtx, input)
		cancel() // 确保取消信号处理

		if err != nil {
			// 检查是否被中断
			if replCtx.Err() != nil {
				fmt.Fprintln(os.Stderr, "\n[响应被中断]")
				continue
			}
			return fmt.Errorf("处理消息失败: %w", err)
		}

		// 输出响应
		if resp != nil {
			for _, part := range resp.Message.Content {
				if part.Type == llm.ContentTypeText {
					fmt.Print(part.Text)
				}
			}
			fmt.Println()
		}
	}
}

// lookupAPIKey 根据 provider 类型查找对应的环境变量
func lookupAPIKey(providerType string) string {
	switch providerType {
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

// buildProviderOptions 根据配置构建 provider 的 Options map
func buildProviderOptions(cfg *config.Config) map[string]any {
	opts := make(map[string]any)

	switch cfg.Provider {
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
		opts["endpoint"] = cfg.Ollama.Endpoint

	case "opencode":
		if cfg.OpenCode.APIKey != "" {
			opts["api_key"] = cfg.OpenCode.APIKey
		}
	}

	return opts
}

// getSessionDir 返回会话存储目录
func getSessionDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return os.TempDir() + "/basework-sessions"
	}
	dir := home + "/.local/share/basework/sessions"
	return dir
}