package main

import (
	"errors"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/wly2lcl/basework/pkg/session"
)

// 本文件是编辑事实的 CLI 审阅入口（EDIT-003）与运行时事件适配器。
//
// 设计约束：这里只读。编辑事件的产生只发生在 edit_files 工具的执行路径里，
// CLI 不重放、不重跑、不补写——审阅入口自身成为写入方，"审阅"就不可信了。

// runtimeEditEventSink 把 edit_files 工具产生的编辑事实落进会话存储。
//
// best-effort 与否由调用方定：工具侧把错误记日志不阻断编辑；这里如实在
// 会话未建立时返回错误，让日志能区分"没记"和"记了"。
type runtimeEditEventSink struct {
	store     *session.JSONLStore
	sessionID func() string
}

// AppendEditEvent 实现 runtimetools.EditEventSink。
func (s *runtimeEditEventSink) AppendEditEvent(data *session.FileEditedData) error {
	if s == nil || s.store == nil || data == nil {
		return nil
	}
	if s.sessionID == nil {
		return errors.New("编辑事件缺少会话归属来源")
	}
	id := s.sessionID()
	if id == "" {
		return errors.New("会话尚未建立，编辑事件无处落盘")
	}
	encoded, err := session.EncodeData(data)
	if err != nil {
		return fmt.Errorf("编码编辑事件失败: %w", err)
	}
	return s.store.AppendEvent(session.Event{
		SessionID: id,
		Type:      session.EventFileEdited,
		Data:      encoded,
	})
}

var (
	editsSessionID string
	editsLimit     int
)

var editsCmd = &cobra.Command{
	Use:   "edits",
	Short: "查看会话中的文件编辑事实（只读，不重放）",
}

var editsListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出会话中全部编辑事实（预览/提交/撤销/失败）",
	RunE:  runEditsList,
}

var editsShowCmd = &cobra.Command{
	Use:   "show <plan_id>",
	Short: "显示单个计划的全部编辑事实",
	Args:  cobra.ExactArgs(1),
	RunE:  runEditsShow,
}

func init() {
	editsCmd.AddCommand(editsListCmd)
	editsCmd.AddCommand(editsShowCmd)
	editsListCmd.Flags().StringVar(&editsSessionID, "session", "", "目标会话 ID（默认最近一个会话）")
	editsListCmd.Flags().IntVar(&editsLimit, "limit", 200, "最多显示的事件条数")
	editsShowCmd.Flags().StringVar(&editsSessionID, "session", "", "目标会话 ID（默认最近一个会话）")
}

// resolveEditsSessionID 决定审阅目标会话：显式指定优先，否则取最近一个。
// 读路径走 resolveSessionDir 回退逻辑（见 openSessionStore），与 session list 一致。
func resolveEditsSessionID(s *session.JSONLStore, explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	infos, err := s.List(session.ListFilter{Limit: 1})
	if err != nil {
		return "", fmt.Errorf("列举会话失败: %w", err)
	}
	if len(infos) == 0 {
		return "", errors.New("没有可用会话；编辑事实随会话产生，先在会话里完成一次 edit_files 流程")
	}
	return infos[0].ID, nil
}

// loadEditEvents 读取目标会话的 file.edited 事件并解码。
func loadEditEvents(s *session.JSONLStore, sessionID string, limit int) ([]*session.FileEditedData, error) {
	events, err := s.Events(session.EventFilter{
		SessionID: sessionID,
		Types:     []session.EventType{session.EventFileEdited},
		Limit:     limit,
	})
	if err != nil {
		return nil, fmt.Errorf("读取编辑事件失败: %w", err)
	}
	out := make([]*session.FileEditedData, 0, len(events))
	for _, ev := range events {
		decoded, err := session.DecodeData(ev)
		if err != nil {
			// 单条损坏跳过并提示，不让一条坏数据毁掉整个审阅视图。
			fmt.Fprintf(os.Stderr, "警告：第 %d 条编辑事件解码失败，已跳过: %v\n", ev.Seq, err)
			continue
		}
		data, ok := decoded.(*session.FileEditedData)
		if !ok {
			continue
		}
		out = append(out, data)
	}
	return out, nil
}

// runEditsList 列出编辑事实，并用 FileTracker 汇总"这个会话动过哪些文件"。
// 会话恢复（重启进程、换终端）后依然可用：数据来源是持久化事件，不是内存。
func runEditsList(cmd *cobra.Command, args []string) error {
	s, err := openSessionStore()
	if err != nil {
		return fmt.Errorf("打开会话存储失败: %w", err)
	}
	sessionID, err := resolveEditsSessionID(s, editsSessionID)
	if err != nil {
		return err
	}
	facts, err := loadEditEvents(s, sessionID, editsLimit)
	if err != nil {
		return err
	}
	if len(facts) == 0 {
		fmt.Printf("会话 %s 没有编辑事实\n", shortSessionID(sessionID))
		return nil
	}

	tracker := session.NewFileTracker()
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "计划\t阶段\t文件数\t已写/已恢复\t失败详情")
	fmt.Fprintln(w, "----\t----\t------\t-----------\t--------")
	for _, f := range facts {
		var touched, bad int
		var firstErr string
		for _, file := range f.Files {
			tracker.TrackOp(file.Path, session.FileEdit)
			switch file.State {
			case "written", "reverted", "preview":
				touched++
			default:
				if file.State != "" {
					bad++
				}
			}
			if file.Err != "" && firstErr == "" {
				firstErr = fmt.Sprintf("%s: %s", file.Path, file.Err)
			}
		}
		detail := firstErr
		if len(f.Files) > 1 && bad > 1 {
			detail += fmt.Sprintf("（共 %d 个未成功）", bad)
		}
		if detail == "" {
			detail = "—"
		}
		fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%s\n",
			shortIDText(f.PlanID), phaseText(f.Phase), len(f.Files), touched, detail)
	}
	w.Flush()

	files := tracker.GetModifiedFiles()
	fmt.Printf("\n会话 %s 共 %d 条编辑事实，涉及 %d 个文件\n", shortSessionID(sessionID), len(facts), len(files))
	if len(files) > 0 {
		fmt.Println("涉及文件：")
		for _, f := range files {
			fmt.Printf("  - %s\n", f)
		}
	}
	if err := s.Delete; err != nil {
		// 不可达；占位防止误用——见下方说明。
		_ = err
	}
	return nil
}

// runEditsShow 显示单个计划的全部事实：每个阶段一条，逐文件结局与原因。
func runEditsShow(cmd *cobra.Command, args []string) error {
	planID := args[0]
	s, err := openSessionStore()
	if err != nil {
		return fmt.Errorf("打开会话存储失败: %w", err)
	}
	sessionID, err := resolveEditsSessionID(s, editsSessionID)
	if err != nil {
		return err
	}
	facts, err := loadEditEvents(s, sessionID, editsLimit)
	if err != nil {
		return err
	}

	var matched int
	for _, f := range facts {
		if f.PlanID != planID {
			continue
		}
		matched++
		fmt.Printf("阶段: %s\n", phaseText(f.Phase))
		if f.Summary != "" {
			fmt.Printf("摘要: %s\n", f.Summary)
		}
		if f.Verify != "" {
			fmt.Printf("验证命令（原样记录，未执行）: %s\n", f.Verify)
		}
		for _, file := range f.Files {
			line := fmt.Sprintf("  - %s [%s]", file.Path, file.State)
			if file.Err != "" {
				line += " " + file.Err
			}
			fmt.Println(line)
		}
		fmt.Println()
	}
	if matched == 0 {
		return fmt.Errorf("会话 %s 中没有计划 %s 的编辑事实", shortSessionID(sessionID), planID)
	}
	return nil
}

// phaseText 把阶段值转成可读文本。
func phaseText(phase string) string {
	switch phase {
	case "preview":
		return "预览"
	case "committed":
		return "已提交"
	case "failed":
		return "失败"
	case "rolled_back":
		return "已撤销"
	default:
		return phase
	}
}

func shortIDText(id string) string {
	if len(id) <= 16 {
		return id
	}
	return id[:16]
}
