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

	providerName := cfg.Provider
	if env := os.Getenv("BASEWORK_PROVIDER"); env != "" {
		providerName = env
	}

	// --no-tui：回退到简单 REPL，无需流式回调
	if noTUI {
		rt, err := newRuntimeAgent(cfg, runtimeAgentOptions{})
		if err != nil {
			return err
		}
		agt := rt.Agent
		defer agt.Close()
		return runSimpleREPL(cmd.Context(), agt)
	}

	// 2. 先建 App，再建带流式回调的 runtime。
	//
	//    这里存在构造顺序依赖：runtimeAgent 需要回调，回调需要 program.Send，
	//    而 program 又需要 app。因此先用转发闭包占位，待 program 建好后再绑定。
	app := tui.NewApp(cfg.Model, providerName, "")
	opts, bindSend := newTUIStreamingOptions()

	rt, err := newRuntimeAgent(cfg, opts)
	if err != nil {
		return err
	}
	agt := rt.Agent
	defer agt.Close()

	if verbose {
		fmt.Fprintf(os.Stderr, "配置: provider=%s model=%s tools=%d\n", rt.ProviderName, rt.ModelName, len(agt.Tools()))
	}

	app.SetInputHandler(func(ctx context.Context, input string) (string, error) {
		resp, err := agt.HandleMessage(ctx, input)
		if err != nil {
			return "", fmt.Errorf("处理消息失败: %w", err)
		}
		return responseText(resp), nil
	})

	return runTUI(cmd.Context(), app, bindSend)
}

// newTUIStreamingOptions 构造 TUI 模式所需的 runtime 选项（含流式回调），
// 并返回用于绑定 tea.Program.Send 的函数。
//
// 单独抽出是为了可测：TUI 只能拿到整段最终文本（流式增量、工具进度、
// thinking 全部失效），根因就是"忘记把 Callback 传进 runtimeAgentOptions"。
// 这里用一个测试锁住该行为，防止回归。
func newTUIStreamingOptions() (runtimeAgentOptions, func(func(tea.Msg))) {
	var send func(tea.Msg)
	cb := tui.NewAgentCallback(func(msg tea.Msg) {
		if send != nil {
			send(msg)
		}
	})
	return runtimeAgentOptions{Callback: cb}, func(bind func(tea.Msg)) {
		send = bind
	}
}

// runTUI 启动 TUI 模式。
//
// bindSend 在 tea.Program 创建之后被调用，用于把 program.Send 交给调用方，
// 从而接上 agent 流式回调（见 internal/tui/callback.go）。
func runTUI(ctx context.Context, app *tui.App, bindSend func(func(tea.Msg))) error {
	program := tea.NewProgram(app)
	if bindSend != nil {
		bindSend(program.Send)
	}

	// 在 goroutine 中运行程序
	errCh := make(chan error, 1)
	go func() {
		_, err := program.Run()
		errCh <- err
	}()

	// 监听退出信号
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
