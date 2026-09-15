package tui

import (
	"fmt"
	"strings"

	"github.com/wly2lcl/basework/internal/tui/theme"
)

// 后台任务状态卡片（UI-001）。
//
// 数据来源：cmd 侧的轮询 goroutine 把 job 快照打包成 JobStatusMsg 投进
// Bubble Tea 事件循环；本组件只做展示与按键交互，不直接触碰 job 管理器
// ——与流式回调「只投递消息」同一纪律。
//
// 渲染约束：输出分页（每页固定行数）、列表封顶，大输出绝不整块塞进界面，
// 避免卡死渲染与输入。状态一律带文字标记，不依赖颜色区分。

// job 页大小：一次渲染的输出行数上限。
const jobOutputPageSize = 30

// jobOutputMaxBytes 是单页输出的字节上限（防超长单行撑爆宽度）。
const jobOutputMaxBytes = 8 * 1024

// jobsListMax 是状态卡片列表的条目上限。
const jobsListMax = 50

// JobStatus 是一个后台任务的展示快照。
type JobStatus struct {
	ID      string
	Command string
	// State 是任务状态原文（jobs 包的状态值）。
	State string
	// ExitCode 供终态展示；非终态忽略。
	ExitCode int
}

// StateLabel 把内部状态翻译成带文字标记的中文标签。
// 文字标记是硬要求：不能只靠颜色区分状态（无障碍/无色终端）。
func (j JobStatus) StateLabel() string {
	switch j.State {
	case "queued":
		return "[排队]"
	case "running":
		return "[运行中]"
	case "succeeded":
		return "[成功]"
	case "failed":
		return fmt.Sprintf("[失败(退出码 %d)]", j.ExitCode)
	case "canceled":
		return "[已取消]"
	case "timed_out":
		return "[超时]"
	case "interrupted":
		return "[已中断]"
	default:
		return "[" + j.State + "]"
	}
}

// IsTerminal 报告任务是否已到终态。
func (j JobStatus) IsTerminal() bool {
	switch j.State {
	case "succeeded", "failed", "canceled", "timed_out", "interrupted":
		return true
	}
	return false
}

// JobStatusMsg 携带一轮轮询的 job 快照（全量替换，不是增量）。
type JobStatusMsg struct {
	Jobs []JobStatus
	// SessionID 是快照所属会话（UI-003 隔离）。空表示未归属（全部接受）。
	SessionID string
}

// JobOutputMsg 携带选中任务的一页输出。
type JobOutputMsg struct {
	JobID string
	// Offset 是本页起始字节（用于「还有更多」的续读）。
	Offset int64
	Text   string
	// More 表示后面还有输出。
	More bool
	// Err 是读取失败的提示文本。
	Err string
}

// jobOutputReader 按 job ID 读取一页输出；由 cmd 侧注入（绑定归属与任务管理器）。
type jobOutputReader func(jobID string, offset int64) JobOutputMsg

// jobCancelFunc 取消指定任务；由 cmd 侧注入。
type jobCancelFunc func(jobID string) error

// JobsView 是后台任务状态卡片：列表 + 选中任务的分页输出。
type JobsView struct {
	visible bool
	jobs    []JobStatus
	// selected 是列表光标位置。
	selected int
	// output 是选中任务的当前页输出；空表示未加载。
	output    JobOutputMsg
	showOut   bool
	readOut   jobOutputReader
	cancelJob jobCancelFunc
	// notice 是一行操作反馈（如「已请求取消」）。
	notice string
}

// NewJobsView 构造状态卡片。reader/cancel 由 cmd 侧注入，可为 nil
// （nil 时对应按键提示为不可用）。
func NewJobsView(reader jobOutputReader, cancel jobCancelFunc) *JobsView {
	return &JobsView{readOut: reader, cancelJob: cancel}
}

// Visible 报告卡片是否显示。
func (v *JobsView) Visible() bool { return v.visible }

// Toggle 显示/隐藏卡片。隐藏时清掉残留输出，避免下次打开闪现旧内容。
func (v *JobsView) Toggle() {
	v.visible = !v.visible
	if !v.visible {
		v.output = JobOutputMsg{}
		v.showOut = false
		v.notice = ""
	}
}

// Update 用一轮快照全量替换列表（封顶 jobsListMax，防列表无限增长）。
func (v *JobsView) Update(msg JobStatusMsg) {
	if len(msg.Jobs) > jobsListMax {
		msg.Jobs = msg.Jobs[:jobsListMax]
	}
	v.jobs = msg.Jobs
	if v.selected >= len(v.jobs) {
		v.selected = len(v.jobs) - 1
	}
	if v.selected < 0 {
		v.selected = 0
	}
}

// Notice 返回最近一条操作反馈。
func (v *JobsView) Notice() string { return v.notice }

// HandleKey 处理卡片可见时的按键。返回 true 表示按键已被消费。
func (v *JobsView) HandleKey(key string) bool {
	if !v.visible {
		return false
	}
	switch key {
	case "esc":
		v.Toggle()
		return true
	case "up", "k":
		if v.selected > 0 {
			v.selected--
			v.showOut = false
		}
		return true
	case "down", "j":
		if v.selected < len(v.jobs)-1 {
			v.selected++
			v.showOut = false
		}
		return true
	case "enter", "o":
		v.loadOutput()
		return true
	case "[":
		v.pageOutput(-1)
		return true
	case "]":
		v.pageOutput(1)
		return true
	case "x":
		v.cancelSelected()
		return true
	}
	return false
}

// loadOutput 加载选中任务的输出首页。
func (v *JobsView) loadOutput() {
	if v.selected >= len(v.jobs) || v.readOut == nil {
		return
	}
	job := v.jobs[v.selected]
	v.output = normalizeOutput(v.readOut(job.ID, 0))
	v.showOut = true
	if v.output.Err != "" {
		v.notice = "读取输出失败：" + v.output.Err
	} else {
		v.notice = ""
	}
}

// pageOutput 前后翻页；无更多内容时提示。
func (v *JobsView) pageOutput(dir int) {
	if !v.showOut || v.readOut == nil || v.selected >= len(v.jobs) {
		return
	}
	job := v.jobs[v.selected]
	var next int64
	switch {
	case dir > 0:
		if !v.output.More {
			v.notice = "[已经是最后一页]"
			return
		}
		next = v.output.Offset + int64(len(v.output.Text))
	case dir < 0:
		next = v.output.Offset - int64(jobOutputPageSize*2)
		if next < 0 {
			next = 0
		}
	}
	v.output = normalizeOutput(v.readOut(job.ID, next))
	v.notice = ""
}

// normalizeOutput 将读回内容限制为一页可消费的字节与行数。
// reader 可能因为实现差异返回超过一页的文本；如果只在 Render 时
// 截断而不调整 More/Offset，后续翻页会直接跳到 EOF，导致中间行不可达。
func normalizeOutput(msg JobOutputMsg) JobOutputMsg {
	text := msg.Text
	if len(text) > jobOutputMaxBytes {
		text = text[:jobOutputMaxBytes]
		msg.More = true
	}
	lines := strings.Split(text, "\n")
	if len(lines) > jobOutputPageSize+1 {
		// 保留前 30 行并消费它们末尾的换行；下一页从该字节偏移开始。
		text = strings.Join(lines[:jobOutputPageSize], "\n") + "\n"
		msg.More = true
	}
	msg.Text = text
	return msg
}

// cancelSelected 请求取消选中任务。
func (v *JobsView) cancelSelected() {
	if v.selected >= len(v.jobs) || v.cancelJob == nil {
		return
	}
	job := v.jobs[v.selected]
	if job.IsTerminal() {
		v.notice = fmt.Sprintf("[任务 %s 已结束，无需取消]", shortJobID(job.ID))
		return
	}
	if err := v.cancelJob(job.ID); err != nil {
		v.notice = "取消失败：" + err.Error()
		return
	}
	v.notice = "已请求取消 " + shortJobID(job.ID)
}

func shortJobID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12] + "…"
}

// SetRuntime 注入读取与取消能力（由 cmd 侧绑定归属后调用；可为 nil）。
func (v *JobsView) SetRuntime(reader jobOutputReader, cancel jobCancelFunc) {
	v.readOut = reader
	v.cancelJob = cancel
}

// Render 渲染卡片。width 控制换行；输出区最多 jobOutputPageSize 行。
func (v *JobsView) Render(width int, t *theme.Theme) string {
	if !v.visible {
		return ""
	}
	var b strings.Builder
	b.WriteString("── 后台任务（ctrl+j 关闭 · enter/o 查看输出 · [ ] 翻页 · x 取消） ──\n")
	if len(v.jobs) == 0 {
		b.WriteString("  （暂无后台任务）\n")
	} else {
		for i, job := range v.jobs {
			cursor := "  "
			if i == v.selected {
				cursor = "> "
			}
			cmd := job.Command
			if len(cmd) > 40 {
				cmd = cmd[:37] + "..."
			}
			fmt.Fprintf(&b, "%s%s %s  %s\n", cursor, job.StateLabel(), shortJobID(job.ID), cmd)
		}
	}
	if v.notice != "" {
		b.WriteString("  " + v.notice + "\n")
	}
	if v.showOut {
		b.WriteString(fmt.Sprintf("── 任务 %s 输出（offset %d）──\n", shortJobID(v.output.JobID), v.output.Offset))
		text := v.output.Text
		if len(text) > jobOutputMaxBytes {
			text = text[:jobOutputMaxBytes] + "\n…（超长截断）"
		}
		lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
		if len(lines) == 1 && lines[0] == "" {
			lines = nil
		}
		for i, line := range lines {
			if i >= jobOutputPageSize {
				remaining := len(lines) - jobOutputPageSize
				fmt.Fprintf(&b, "…（还有 %d 行，按 ] 续读）\n", remaining)
				break
			}
			b.WriteString("  " + line + "\n")
		}
		if len(lines) == 0 {
			b.WriteString("  （无输出）\n")
		}
		if v.output.More {
			b.WriteString("  …（按 ] 读取更多）\n")
		}
		if v.output.Err != "" {
			b.WriteString("  读取失败：" + v.output.Err + "\n")
		}
	}
	return b.String()
}
