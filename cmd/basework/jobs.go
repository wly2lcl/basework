package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/wly2lcl/basework/internal/jobs"
	"github.com/wly2lcl/basework/pkg/session"
)

// jobOwnerHolder 保存"当前会话 ID"。
//
// 为什么需要它：工具必须在 Agent 之前构造（Agent 构造时要拿到工具表），而会话 ID 要等
// Agent 构造完成才存在。用一个可以在构造后回填的持有者，让两者的顺序不必颠倒，
// 也不会出现"启动时猜一个归属"的窗口。
//
// 加锁的理由：Get 会在工具执行（Agent 循环的 goroutine）里被调用，Set 发生在构造线程。
// 不加锁时 -race 会把它判成数据竞争，而且"回填"与"读取"之间确实没有顺序保证。
type jobOwnerHolder struct {
	mu    sync.RWMutex
	value string
}

func (h *jobOwnerHolder) Set(id string) {
	h.mu.Lock()
	h.value = id
	h.mu.Unlock()
}

func (h *jobOwnerHolder) Get() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.value
}

// jobJournalPath 返回 job 持久化日志的路径。
//
// 整个会话目录共用一个日志文件，靠记录里的 owner 字段区分会话。理由：
// 工具在会话 ID 确定之前就要构造，无法按会话决定文件路径；而"一个文件 + owner 过滤"
// 既满足按会话隔离，又不必在启动时就猜出会话 ID。
func jobJournalPath() string {
	return filepath.Join(getSessionDir(), "jobs.jsonl")
}

// jobOutputDir 返回后台任务输出溢出的目录。
func jobOutputDir() string {
	return filepath.Join(getSessionDir(), "job-output")
}

// processToken 返回本次进程的短随机标记。
//
// 为什么需要它：默认编号在进程内从 001 起，重启后又从头开始。job 日志是追加的，
// 同一个会话两轮运行生成同样的 ID 会让 foldRecords 把两条不同的运行折叠成一条，
// 历史就此失真（比如上轮的 running 与本轮的 succeeded 被拼成一条）。带上每次启动
// 都不同的标记，ID 才跨进程唯一。
func processToken() string {
	var buf [2]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// crypto/rand 失败在实践中不可达；退化为进程号，仍能保证"重启后不同"。
		return fmt.Sprintf("%04x", os.Getpid()&0xffff)
	}
	return hex.EncodeToString(buf[:])
}

// newRuntimeJobManager 创建运行时 job 管理器。
//
// 归属（owner）用 holder 晚绑定；ReconcileOwner 在会话 ID 确定后由调用方显式触发，
// 把上次进程留下的非终态记录改写成 interrupted——既不伪称"还在跑"，也不会重跑命令。
// runtimeWorkspaceID 返回当前工作区标识（CTX-001）。任务日志与编辑事件
// 都以它归属；读取方据此隔离不同工作区的事实。
func runtimeWorkspaceID() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return session.WorkspaceID(cwd)
}

func newRuntimeJobManager(owner *jobOwnerHolder) *jobs.Manager {
	return newRuntimeJobManagerAt(owner, jobJournalPath(), jobOutputDir())
}

// newRuntimeJobManagerAt 是 newRuntimeJobManager 的可注入版本，便于在测试里指向临时目录。
func newRuntimeJobManagerAt(owner *jobOwnerHolder, journalPath, outputDir string) *jobs.Manager {
	var seq int64
	token := processToken()
	return jobs.New(jobs.Options{
		Journal:     jobs.NewJSONLJournal(journalPath),
		WorkspaceID: runtimeWorkspaceID(),
		Output:      jobs.OutputOptions{Dir: outputDir},
		NewID: func() string {
			prefix := shortSessionID(owner.Get())
			if prefix == "" {
				prefix = "pending"
			}
			return fmt.Sprintf("%s-%s-job-%03d", prefix, token, atomic.AddInt64(&seq, 1))
		},
	})
}

// runtimeJobCleanup 在运行时退出时结束本会话的后台任务并清理输出文件。
//
// 它就是 JOB-003 的 `CloseOwner` 的产品接线点：会话结束后后台命令不该继续跑，
// 输出文件也不该继续占磁盘。
func runtimeJobCleanup(mgr *jobs.Manager, owner *jobOwnerHolder) func() error {
	return func() error {
		if mgr == nil {
			return nil
		}
		if id := owner.Get(); id != "" {
			mgr.CloseOwner(id)
		}
		mgr.Close()
		return nil
	}
}

// ---------------------------------------------------------------- CLI

// jobsCmd 表示 jobs 子命令。
var jobsCmd = &cobra.Command{
	Use:   "jobs",
	Short: "查看后台任务记录",
	Long: `查看后台任务的持久化记录。

本命令只读取记录，不会重新执行任何命令；上次进程退出时仍在运行的任务会被标记为
interrupted（表示"无法接管"，而不是"失败"）。`,
}

var jobsListOwn string

var jobsListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出后台任务记录",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runJobsList(os.Stdout)
	},
}

var jobsShowCmd = &cobra.Command{
	Use:   "show <job-id>",
	Short: "查看单个后台任务的记录",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runJobsShow(os.Stdout, args[0])
	},
}

func init() {
	jobsCmd.AddCommand(jobsListCmd)
	jobsCmd.AddCommand(jobsShowCmd)
	jobsListCmd.Flags().StringVar(&jobsListOwn, "owner", "", "只看某个会话的记录（默认列出全部）")
}

// openJobsHistory 打开只读的历史视图。
//
// 只读体现在：这里只构造 Manager 并读取记录，从不调用 Start——
// 也就是说，"看一眼历史"在任何情况下都不会让命令再跑一遍。
func openJobsHistory() (*jobs.Manager, error) {
	path := jobJournalPath()
	if _, err := os.Stat(path); err != nil {
		// 也检查历史会话目录，避免升级后用户看到"没有记录"。
		if legacy := legacySessionDir(); legacy != "" {
			legacyPath := filepath.Join(legacy, "jobs.jsonl")
			if _, legacyErr := os.Stat(legacyPath); legacyErr == nil {
				path = legacyPath
			}
		}
	}
	return openJobsHistoryAt(path)
}

// openJobsHistoryAt 是 openJobsHistory 的可注入版本。
func openJobsHistoryAt(path string) (*jobs.Manager, error) {
	return jobs.New(jobs.Options{Journal: jobs.NewJSONLJournal(path)}), nil
}

// runJobsList 列出后台任务记录。
func runJobsList(w io.Writer) error {
	mgr, err := openJobsHistory()
	if err != nil {
		return fmt.Errorf("打开后台任务记录失败: %w", err)
	}
	all, err := mgr.AllHistory()
	if err != nil {
		return fmt.Errorf("读取后台任务记录失败: %w", err)
	}
	return renderJobsList(w, all, jobsListOwn)
}

// renderJobsList 渲染任务列表。
//
// 非终态记录会额外提示：它们来自更早的进程，本命令没有它们的句柄，无法确认是否仍在
// 运行。只显示 "running" 而不加说明，等于替一个可能已经消失的进程背书。
func renderJobsList(w io.Writer, all []jobs.Job, ownerFilter string) error {
	shown := make([]jobs.Job, 0, len(all))
	stale := 0
	for _, job := range all {
		if ownerFilter != "" && job.Owner != ownerFilter {
			continue
		}
		if !job.State.Terminal() {
			stale++
		}
		shown = append(shown, job)
	}
	if len(shown) == 0 {
		if len(all) > 0 && ownerFilter != "" {
			fmt.Fprintf(w, "会话 %s 没有后台任务记录（共 %d 条记录）。\n", ownerFilter, len(all))
			return nil
		}
		fmt.Fprintln(w, "没有后台任务记录")
		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	fmt.Fprintln(tw, "ID\t会话\t状态\t命令\t退出码\t结束时间")
	fmt.Fprintln(tw, "--\t----\t----\t----\t------\t--------")
	for _, job := range shown {
		exit := "-"
		if job.ExitCode >= 0 {
			exit = fmt.Sprintf("%d", job.ExitCode)
		}
		ended := "-"
		if !job.EndedAt.IsZero() {
			ended = job.EndedAt.Format(time.RFC3339)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			job.ID, shortSessionID(job.Owner), job.State, commandSummary(job.Command), exit, ended)
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	fmt.Fprintf(w, "共 %d 条记录。用 `basework jobs show <job-id>` 查看单条详情。\n", len(shown))
	if stale > 0 {
		fmt.Fprintf(w, "注意：%d 条记录处于非终态，它们来自更早的进程，本命令无法确认是否仍在运行；\n", stale)
		fmt.Fprintln(w, "      重新启动 Agent 会把这类记录归并为 interrupted。")
	}
	fmt.Fprintln(w, "注意：本命令只读取记录，不会重新执行任何命令。")
	return nil
}

// runJobsShow 展示单个 job 的记录。
func runJobsShow(w io.Writer, id string) error {
	mgr, err := openJobsHistory()
	if err != nil {
		return fmt.Errorf("打开后台任务记录失败: %w", err)
	}
	all, err := mgr.AllHistory()
	if err != nil {
		return fmt.Errorf("读取后台任务记录失败: %w", err)
	}
	for _, job := range all {
		if job.ID == id {
			return renderJobDetail(w, job)
		}
	}
	return fmt.Errorf("未找到后台任务记录: %s", id)
}

// renderJobDetail 渲染单条记录。
func renderJobDetail(w io.Writer, job jobs.Job) error {
	fmt.Fprintf(w, "ID: %s\n", job.ID)
	fmt.Fprintf(w, "会话: %s\n", job.Owner)
	fmt.Fprintf(w, "命令: %s\n", job.Command)
	fmt.Fprintf(w, "状态: %s\n", job.State)
	if job.Err != "" {
		fmt.Fprintf(w, "原因: %s\n", job.Err)
	}
	if !job.CreatedAt.IsZero() {
		fmt.Fprintf(w, "登记时间: %s\n", job.CreatedAt.Format(time.RFC3339))
	}
	if !job.EndedAt.IsZero() {
		fmt.Fprintf(w, "结束时间: %s\n", job.EndedAt.Format(time.RFC3339))
	}
	if job.ExitCode >= 0 {
		fmt.Fprintf(w, "退出码: %d\n", job.ExitCode)
	}
	fmt.Fprintf(w, "输出: stdout=%d 字节 stderr=%d 字节\n", job.StdoutBytes, job.StderrBytes)
	if job.OutputTruncated {
		fmt.Fprintln(w, "注意: 输出超出配额，尾部已被丢弃")
	}
	if job.JournalErr != "" {
		fmt.Fprintf(w, "记录写入失败: %s\n", job.JournalErr)
	}
	if len(job.OutputRefs) > 0 {
		fmt.Fprintln(w, "输出文件:")
		for _, ref := range job.OutputRefs {
			if info, statErr := os.Stat(ref); statErr == nil {
				fmt.Fprintf(w, "  %s (%d 字节)\n", ref, info.Size())
			} else {
				fmt.Fprintf(w, "  %s (已不可读: %v)\n", ref, statErr)
			}
		}
	}
	switch {
	case job.State == jobs.StateInterrupted:
		fmt.Fprintln(w, "说明: 上一进程退出时该任务仍在运行，无法接管；本命令不会重跑它。")
	case !job.State.Terminal():
		fmt.Fprintf(w, "说明: 记录停在非终态（%s）。它来自更早的进程，本命令无法确认它是否仍在运行，\n", job.State)
		fmt.Fprintln(w, "      也不会替它重跑命令；重新启动 Agent 会把它归并为 interrupted。")
	}
	fmt.Fprintln(w, "注意：本命令只读取记录，不会重新执行任何命令。")
	return nil
}

// commandSummary 截短命令用于表格展示。
func commandSummary(cmd string) string {
	const max = 48
	flat := strings.ReplaceAll(strings.ReplaceAll(cmd, "\n", " "), "\t", " ")
	runes := []rune(flat)
	if len(runes) <= max {
		return flat
	}
	return string(runes[:max]) + "…"
}
