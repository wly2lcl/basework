package main

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/wly2lcl/basework/internal/jobs"
	"github.com/wly2lcl/basework/pkg/session"
)

// 本文件是工作区事实的 CLI 聚合视图（CTX-001）。
//
// 与 edits/jobs 一样只读：把两类已持久化的记录（file.edited 会话事件、
// jobs.jsonl 任务日志）折叠成统一事实模型展示。CLI 不写事实文件——
// 事实的持久化写入属于运行时链路（CTX-003 接线），审阅入口若能落盘，
// "审阅"就不可信了。

var (
	factsSessionID string
	factsLimit     int
)

var factsCmd = &cobra.Command{
	Use:   "facts",
	Short: "查看工作区事实（只读聚合视图）",
}

var factsShowCmd = &cobra.Command{
	Use:   "show",
	Short: "折叠编辑事件与后台任务日志，展示当前工作区的事实集合",
	RunE:  runFactsShow,
}

func init() {
	factsCmd.AddCommand(factsShowCmd)
	factsShowCmd.Flags().StringVar(&factsSessionID, "session", "", "编辑事件的目标会话 ID（默认最近一个会话）")
	factsShowCmd.Flags().IntVar(&factsLimit, "limit", 500, "最多读取的编辑事件条数")
}

// foldJobFacts 把后台任务日志折叠成命令事实。
// 同一 job id 跨重启、跨多次查询只累计一次（去重键见 CommandFactKey）。
// 返回折叠前后的事实数，供展示层报告「本次折叠命中多少条」。
func foldJobFacts(w *session.WorkspaceFacts, mgr *jobs.Manager) (int, error) {
	history, err := mgr.AllHistory()
	if err != nil {
		return 0, fmt.Errorf("读取后台任务日志失败: %w", err)
	}
	before := w.Count()
	for _, job := range history {
		source := "job"
		switch job.WorkspaceID {
		case w.WorkspaceID:
			// 本工作区的任务，正常归属。
		case "":
			// 旧日志没有归属戳：显示为「未归属」，绝不默认归给当前
			// 工作区——把别的目录的任务算到头上比少显示一条危害大。
			source = "job:未归属(旧数据)"
		default:
			continue // 其他工作区的任务，隔离
		}
		w.Add(session.Fact{
			Kind:      session.FactCommand,
			Key:       session.CommandFactKey(job.ID),
			Ref:       job.ID,
			Source:    source,
			FirstSeen: job.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	return w.Count() - before, nil
}

// foldEditEventFacts 读取会话的 file.edited 事件并折叠进事实集合。
// FirstSeen 用事件自身的 CreatedAt——事实的发生时间不该取决于查看时刻。
// 单条损坏事件跳过并告警，与 loadEditEvents 的容错口径一致。
func foldEditEventFacts(w *session.WorkspaceFacts, s *session.JSONLStore, sessionID string) (int, error) {
	events, err := s.Events(session.EventFilter{
		SessionID: sessionID,
		Types:     []session.EventType{session.EventFileEdited},
		Limit:     factsLimit,
	})
	if err != nil {
		return 0, fmt.Errorf("读取编辑事件失败: %w", err)
	}
	before := w.Count()
	for _, ev := range events {
		decoded, err := session.DecodeData(ev)
		if err != nil {
			fmt.Fprintf(os.Stderr, "警告：第 %d 条编辑事件解码失败，已跳过: %v\n", ev.Seq, err)
			continue
		}
		data, ok := decoded.(*session.FileEditedData)
		if !ok {
			continue
		}
		// 归属过滤：其他工作区的编辑不折叠。
		if data.WorkspaceID != "" && data.WorkspaceID != w.WorkspaceID {
			continue
		}
		// 无戳旧事件仍折叠，但来源标记为「未归属」，不冒充当前工作区的事实。
		if data.WorkspaceID == "" {
			foldFileEditedWithSource(w, *data, ev.CreatedAt, "edit_files:未归属(旧数据)")
			continue
		}
		session.FoldFileEdited(w, *data, ev.CreatedAt)
	}
	return w.Count() - before, nil
}

// foldFileEditedWithSource 与 session.FoldFileEdited 相同的折叠规则，
// 但允许调用方覆盖来源标记（用于无归属戳的旧数据）。
func foldFileEditedWithSource(w *session.WorkspaceFacts, data session.FileEditedData, at time.Time, source string) {
	seen := at.UTC().Format(time.RFC3339)
	for _, rec := range data.Files {
		if rec.Path == "" {
			continue
		}
		var kind string
		switch {
		case rec.State == "written":
			kind = session.FactFileModified
		case data.Phase == "rolled_back" && rec.State == "reverted":
			kind = session.FactFileReverted
		default:
			continue
		}
		w.Add(session.Fact{
			Kind:      kind,
			Key:       session.FileFactKey(kind, rec.Path),
			Path:      rec.Path,
			Ref:       data.PlanID,
			Source:    source,
			FirstSeen: seen,
		})
	}
}

// runFactsShow 展示当前工作区的事实集合（内存折叠，不写事实文件）。
func runFactsShow(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("获取工作目录失败: %w", err)
	}
	workspaceID := session.WorkspaceID(cwd)
	w := session.NewWorkspaceFacts(workspaceID)

	// --- 命令事实：后台任务日志（跨会话共享，全部折叠） ---
	mgr, err := openJobsHistory()
	if err != nil {
		return fmt.Errorf("打开后台任务记录失败: %w", err)
	}
	defer mgr.Close()
	jobFacts, err := foldJobFacts(w, mgr)
	if err != nil {
		return err
	}

	// --- 文件事实：最近会话（或指定会话）的 file.edited 事件 ---
	var sessionID string
	var editFacts int
	s, err := openSessionStore()
	if err != nil {
		return fmt.Errorf("打开会话存储失败: %w", err)
	}
	sid, err := resolveEditsSessionID(s, factsSessionID)
	if err != nil {
		fmt.Fprintln(os.Stderr, "提示：没有可用会话，编辑事实为空（后台任务事实照常折叠）")
	} else {
		sessionID = sid
		edited, err := foldEditEventFacts(w, s, sid)
		if err != nil {
			return err
		}
		editFacts = edited
	}

	// --- 渲染 ---
	fmt.Printf("工作区 %s（%s）\n\n", workspaceID, cwd)
	fmt.Printf("来源：后台任务新折叠 %d 条", jobFacts)
	if sessionID != "" {
		fmt.Printf("；编辑事件来自会话 %s（折叠出 %d 条）", shortSessionID(sessionID), editFacts)
	}
	fmt.Println()

	facts := w.List()
	if len(facts) == 0 {
		fmt.Println("当前工作区没有事实记录")
		return nil
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(tw, "类别\t路径/引用\t来源\t首次记录")
	fmt.Fprintln(tw, "----\t--------\t----\t--------")
	for _, f := range facts {
		subject := f.Path
		if subject == "" {
			subject = f.Ref
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", f.Kind, subject, f.Source, f.FirstSeen)
	}
	tw.Flush()
	fmt.Printf("\n共 %d 条事实（同键事实只累计一次）\n", len(facts))
	return nil
}
