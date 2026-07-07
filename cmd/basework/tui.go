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

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"
	"github.com/wly2lcl/basework/internal/tui"
	"github.com/wly2lcl/basework/pkg/agent"
	"github.com/wly2lcl/basework/pkg/config"
	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/tool"
)

// tuiCmd 表示 tui 子命令
var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "启动终端 UI 模式",
	Long: `启动一个基于 Bubble Tea 的终端 UI 交互模式。
提供更丰富的界面，包括消息渲染、输入历史、Tab 补全等。`,
	RunE: runTUIE,
}

var (
	noTUI bool
)

func init() {
	tuiCmd.Flags().BoolVar(&noTUI, "no-tui", false, "回退到简单 REPL 模式（无 TUI）")
}

// runTUIE 执行 tui 子命令
func runTUIE(cmd *cobra.Command, args []string) error {
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

	rt, err := newRuntimeAgent(cfg, runtimeAgentOptions{})
	if err != nil {
		return err
	}
	agt := rt.Agent
	defer agt.Close()

	if verbose {
		fmt.Fprintf(os.Stderr, "配置: provider=%s model=%s tools=%d\n", rt.ProviderName, rt.ModelName, len(agt.Tools()))
	}

	if noTUI {
		return runSimpleREPL(cmd.Context(), agt)
	}
	return runTUI(cmd.Context(), agt, rt.ModelName, rt.ProviderName)
}

// runTUI 启动 TUI 模式
func runTUI(ctx context.Context, agt agent.Agent, modelName, providerName string) error {
	// 获取或创建会话
	app := tui.NewApp(modelName, providerName, "")
	app.SetInputHandler(func(ctx context.Context, input string) (string, error) {
		resp, err := agt.HandleMessage(ctx, input)
		if err != nil {
			return "", fmt.Errorf("处理消息失败: %w", err)
		}
		return responseText(resp), nil
	})

	// 启动 Bubble Tea 程序
	program := tea.NewProgram(app)

	// 在 goroutine 中运行程序
	errCh := make(chan error, 1)
	go func() {
		_, err := program.Run()
		errCh <- err
	}()

	// 监听 agent 消息
	for {
		select {
		case <-ctx.Done():
			program.Send(tea.QuitMsg{})
			return ctx.Err()
		case err := <-errCh:
			return err
		}
	}
}

// runSimpleREPL 运行简单 REPL 模式（无 TUI 的回退方案）
func runSimpleREPL(ctx context.Context, agt agent.Agent) error {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Fprintln(os.Stderr, "进入简单 REPL 模式，输入 exit 或 Ctrl+D 退出")

	for {
		replCtx, cancel := signal.NotifyContext(ctx, syscall.SIGINT)

		fmt.Fprint(os.Stderr, "> ")
		scanned := scanner.Scan()
		if !scanned {
			if err := scanner.Err(); err != nil {
				cancel()
				return fmt.Errorf("读取输入失败: %w", err)
			}
			fmt.Fprintln(os.Stderr)
			cancel()
			return nil
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			cancel()
			continue
		}
		if input == "exit" {
			fmt.Fprintln(os.Stderr, "再见！")
			cancel()
			return nil
		}

		// 流式响应用 Callback
		resp, err := agt.HandleMessage(replCtx, input)
		cancel()

		if err != nil {
			if replCtx.Err() != nil {
				fmt.Fprintln(os.Stderr, "\n[响应被中断]")
				continue
			}
			return fmt.Errorf("处理消息失败: %w", err)
		}

		if resp != nil {
			fmt.Println(responseText(resp))
		}
	}
}

// tuiCallback 为 TUI 模式提供 Callback 实现
type tuiCallback struct {
	app *tui.App
}

func (cb *tuiCallback) OnTextDelta(delta string) {
	cb.app.UpdateStreamingText(cb.app.Streaming.FullText() + delta)
}

func (cb *tuiCallback) OnToolCallStart(call llm.ToolCall) {
	cb.app.Streaming.SetToolInProgress(call.Name)
}

func (cb *tuiCallback) OnToolCallEnd(call llm.ToolCall, result *tool.Result, err error) {
	if err != nil {
		cb.app.AddToolCall(call.Name, call.ArgsJSON, err.Error(), true)
	} else {
		cb.app.AddToolCall(call.Name, call.ArgsJSON, result.Content, false)
	}
}

func (cb *tuiCallback) OnThinkingDelta(delta string) {
	cb.app.Streaming.AppendThinking(delta)
}

func (cb *tuiCallback) OnTurnEnd(resp *agent.Response) {
	if resp != nil {
		for _, part := range resp.Message.Content {
			if part.Type == llm.ContentTypeText {
				cb.app.AddAssistantMessage(part.Text)
			}
		}
	}
}

func (cb *tuiCallback) OnError(err error) {
	cb.app.AddErrorMessage(err.Error())
}
