package tui

import (
	"fmt"
	"strings"
)

// ResumePanel 是重启后的可恢复进度面板（UI-003）：任务列表、最近修改、
// 验证摘要三类事实，明确标注「哪些操作需要重试」。
//
// 状态全部用文字标记（needs-retry / 失败 / 已完成…），不依赖颜色。
type ResumePanel struct {
	items    []ResumeItem
	visible  bool
	MaxItems int
}

// ResumeItem 是一条恢复事实。
type ResumeItem struct {
	// Kind 是条目类别："job"（后台任务）/ "file"（最近修改）/ "verify"（验证摘要）。
	Kind string
	// Label 是人类可读的主文本。
	Label string
	// State 是状态文本（任务终态/验证状态）。
	State string
	// NeedsRetry 标记重启后需要用户重试或复核的操作。
	NeedsRetry bool
	// Ref 是关联 ID（job ID / 文件路径 / 计划 ID）。
	Ref string
}

// NewResumePanel 创建面板（默认隐藏，有内容时由 App 显示）。
func NewResumePanel() *ResumePanel {
	return &ResumePanel{MaxItems: 20}
}

// SetItems 设置条目并显示面板；空列表不显示。
func (p *ResumePanel) SetItems(items []ResumeItem) {
	p.items = items
	p.visible = len(items) > 0
}

// Visible 报告面板是否显示。
func (p *ResumePanel) Visible() bool { return p.visible }

// Hide 隐藏面板（用户开始新一轮工作后不再打扰）。
func (p *ResumePanel) Hide() { p.visible = false }

// RetryCount 返回需要重试的条目数（测试观察点）。
func (p *ResumePanel) RetryCount() int {
	n := 0
	for _, it := range p.items {
		if it.NeedsRetry {
			n++
		}
	}
	return n
}

// Render 渲染面板。返回空串表示无可渲染内容。
func (p *ResumePanel) Render(width int) string {
	if !p.visible || len(p.items) == 0 {
		return ""
	}
	max := p.MaxItems
	if max <= 0 {
		max = 20
	}

	var b strings.Builder
	b.WriteString("── 恢复进度（上次会话遗留） ──────────────\n")

	retry := p.RetryCount()
	if retry > 0 {
		fmt.Fprintf(&b, "需要重试/复核：%d 项\n", retry)
	} else {
		b.WriteString("无需要重试的操作\n")
	}

	shown := 0
	for _, it := range p.items {
		if shown >= max {
			fmt.Fprintf(&b, "…（其余 %d 项省略）\n", len(p.items)-max)
			break
		}
		marker := "  "
		if it.NeedsRetry {
			marker = "[需重试]"
		}
		label := it.Label
		maxLabel := width - 24
		if maxLabel > 0 && len([]rune(label)) > maxLabel {
			r := []rune(label)
			label = string(r[:maxLabel]) + "…"
		}
		switch it.Kind {
		case "job":
			fmt.Fprintf(&b, "%s 任务 %s  %s\n", marker, label, it.State)
		case "file":
			fmt.Fprintf(&b, "%s 修改 %s\n", marker, label)
		case "verify":
			fmt.Fprintf(&b, "%s 验证 %s\n", marker, label)
		default:
			fmt.Fprintf(&b, "%s %s\n", marker, label)
		}
		shown++
	}
	b.WriteString("（按任意键继续工作，本面板自动收起）")
	return b.String()
}
