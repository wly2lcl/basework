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
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"
	"github.com/wly2lcl/basework/internal/jobs"
	"github.com/wly2lcl/basework/internal/permission"
	intruntime "github.com/wly2lcl/basework/internal/runtime"
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
	noTUI     bool
	tuiPreset string
)

func init() {
	tuiCmd.Flags().BoolVar(&noTUI, "no-tui", false, "回退到简单 REPL 模式（无 TUI）")
	tuiCmd.Flags().StringVar(&tuiPreset, "preset", "", "启动预设（readonly / coding），与配置文件 preset 冲突时报错")
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
		rt, err := newRuntimeAgent(cfg, runtimeAgentOptions{Preset: tuiPreset})
		if err != nil {
			return err
		}
		svc := rt.Service()
		if svc == nil {
			return fmt.Errorf("构造运行服务失败")
		}
		defer svc.Close()
		return runSimpleREPL(cmd.Context(), svc)
	}

	// 2. 先建 App，再建带流式回调的 runtime。
	//
	//    这里存在构造顺序依赖：runtimeAgent 需要回调，回调需要 program.Send，
	//    而 program 又需要 app。因此先用转发闭包占位，待 program 建好后再绑定。
	app := tui.NewApp(cfg.Model, providerName, "")
	opts, bindSend := newTUIStreamingOptions()

	// 权限审批（UI-002）：TUI 用 broker 审批（弹窗确认），CLI 用终端问答。
	// notifier 在 runTUI 里绑定 program.Send——此刻 program 还不存在。
	approvalBroker := permission.NewApprovalBroker()
	defer approvalBroker.Close()

	opts.Preset = tuiPreset
	opts.ApprovalPrompt = permission.ApprovalPromptFunc(approvalBroker, 0)
	rt, err := newRuntimeAgent(cfg, opts)
	if err != nil {
		return err
	}
	// RUN-003：TUI 与 CLI 同走运行服务。流式回调经 CallbackSwitch 按运行
	// 绑入/解绑，取消与错误语义与 CLI 一致。
	svc := rt.Service()
	if svc == nil {
		return fmt.Errorf("构造运行服务失败")
	}
	defer svc.Close()

	if verbose {
		fmt.Fprintf(os.Stderr, "配置: provider=%s model=%s tools=%d\n", rt.ProviderName, rt.ModelName, len(rt.Agent.Tools()))
	}
	reportRuntimeCapabilities(rt, verbose)

	app.SetInputHandler(func(ctx context.Context, input string) (string, error) {
		run, err := svc.Start(ctx, input)
		if err != nil {
			return "", fmt.Errorf("启动运行失败: %w", err)
		}
		if run.Err != nil {
			// 取消与错误统一为 Run.Err（RUN-003 语义对齐）。
			return "", fmt.Errorf("处理消息失败: %w", run.Err)
		}
		return responseText(run.Response), nil
	})

	// 会话绑定与恢复进度（UI-003）：App 构造时还不知道会话 ID，这里
	// 显式切换绑定；随后装入「重启后可恢复进度」事实。
	app.SwitchSession(rt.sessionID())
	if sess := rt.sess; sess != nil {
		app.SetResumeItems(collectResumeItems(rt.jobManager, rt.sessionID(), sess, rt.sessionID(), runtimeWorkspaceID()))
	}

	// 后台任务卡片（UI-001）：注入读取/取消能力 + 快照轮询。
	jobOwner := rt.sessionID()
	jobMgr := rt.jobManager
	app.SetJobsRuntime(
		func(jobID string, offset int64) tui.JobOutputMsg {
			return readJobOutputPage(jobMgr, jobOwner, jobID, offset)
		},
		func(jobID string) error {
			return jobMgr.Cancel(jobOwner, jobID)
		},
	)
	snapshotMsg := func() tui.JobStatusMsg {
		return tui.JobStatusMsg{Jobs: mergedJobStatuses(jobMgr, jobOwner), SessionID: jobOwner}
	}

	return runTUI(cmd.Context(), app, bindSend, svc, snapshotMsg, approvalBroker)
}

// forwardApprovalRequests 把 broker 的批准请求转成 TUI 消息（UI-002）。
// Respond 闭包按请求 ID 回投决定；用户关闭窗口等价拒绝（n/esc 同路径）。
func forwardApprovalRequests(broker *permission.ApprovalBroker, send func(tea.Msg)) {
	broker.SetNotifier(func(req permission.ApprovalRequest) {
		send(tui.ApprovalRequestMsg{
			ID:         req.ID,
			ToolName:   req.ToolName,
			Purpose:    req.Purpose,
			Paths:      req.Paths,
			Diff:       req.Diff,
			RiskReason: req.RiskReason,
			Respond: func(approved bool) bool {
				dec := permission.DecisionDenied
				if approved {
					dec = permission.DecisionApproved
				}
				return broker.Respond(req.ID, dec)
			},
			Timeout: permission.DefaultApprovalTimeout,
		})
	})
}

// readJobOutputPage 读取一页任务输出（stdout），封装归属与页大小。
func readJobOutputPage(mgr *jobs.Manager, owner, jobID string, offset int64) tui.JobOutputMsg {
	if mgr == nil {
		return tui.JobOutputMsg{JobID: jobID, Err: "后台任务不可用"}
	}
	page, err := mgr.ReadOutput(owner, jobID, jobs.StreamStdout, offset, 4096)
	if err != nil {
		return tui.JobOutputMsg{JobID: jobID, Err: err.Error()}
	}
	return tui.JobOutputMsg{
		JobID:  jobID,
		Offset: page.Offset,
		Text:   page.Text,
		More:   !page.EOF,
	}
}

// mergedJobStatuses 合并内存态与持久化历史（内存优先），转成展示快照。
func mergedJobStatuses(mgr *jobs.Manager, owner string) []tui.JobStatus {
	if mgr == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []tui.JobStatus
	for _, j := range mgr.List(owner) {
		seen[j.ID] = true
		out = append(out, tui.JobStatus{ID: j.ID, Command: j.Command, State: string(j.State), ExitCode: j.ExitCode})
	}
	if hist, err := mgr.History(owner); err == nil {
		for _, j := range hist {
			if !seen[j.ID] {
				out = append(out, tui.JobStatus{ID: j.ID, Command: j.Command, State: string(j.State), ExitCode: j.ExitCode})
			}
		}
	}
	return out
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
func runTUI(ctx context.Context, app *tui.App, bindSend func(func(tea.Msg)), svc intruntime.Service, snapshotMsg func() tui.JobStatusMsg, approvalBroker *permission.ApprovalBroker) error {
	program := tea.NewProgram(app)
	if bindSend != nil {
		bindSend(program.Send)
	}
	// 审批请求转发（UI-002）：program 已就绪，绑定 notifier。
	if approvalBroker != nil {
		forwardApprovalRequests(approvalBroker, program.Send)
	}

	// 后台任务快照轮询（UI-001）：500ms 一拍，全量替换式投递。
	// 退出路径统一走 defer（与订阅退订同一处）。
	stopPoll := make(chan struct{})
	defer close(stopPoll)
	if snapshotMsg != nil {
		go func() {
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-stopPoll:
					return
				case <-ticker.C:
					program.Send(snapshotMsg())
				}
			}
		}()
	}

	// 订阅运行事件并转发给 UI（RUN-003：订阅按 session/run ID 路由——
	// 每条事件带 ID，App.Update 据此忽略不属于当前会话的事件）。
	// defer Unsubscribe 覆盖全部退出路径：退订会关闭事件通道，
	// 转发 goroutine 随之结束，不遗留。
	sub := svc.Subscribe()
	defer sub.Unsubscribe()
	go func() {
		for ev := range sub.Events() {
			program.Send(tui.RunEventMsg{
				RunID:     ev.RunID,
				SessionID: ev.SessionID,
				Kind:      ev.Kind,
				Err:       ev.Err,
			})
		}
	}()

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

// runSimpleREPL 运行简单 REPL 模式（无 TUI 的回退方案；RUN-003 经运行服务）
func runSimpleREPL(ctx context.Context, svc intruntime.Service) error {
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

		// 经运行服务处理（取消/错误语义与 CLI、TUI 一致）
		run, startErr := svc.Start(replCtx, input)
		cancel()

		err := startErr
		resp := (*agent.Response)(nil)
		if err == nil {
			if run.Err != nil {
				err = run.Err
			} else {
				resp = run.Response
			}
		}

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
