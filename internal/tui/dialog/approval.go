package dialog

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ApprovalDialog 是权限审批对话框（UI-002）。
//
// 展示工具目的/涉及路径/diff 预览/风险依据，键位完成全部操作：
//   - y / enter：允许
//   - n / esc：拒绝；关闭窗口（组件被移除）同样等价拒绝
//
// 应答通过 Respond 回调发出（闭包由 App 侧绑定 broker.Respond 与请求 ID）。
// 决策后对话框自我关闭；重复应答由 broker 按 ID 幂等忽略。
type ApprovalDialog struct {
	baseDialog
	req        ApprovalRequest
	Respond    func(approved bool) bool
	decided    bool
	scroll     int
	heightHint int
}

// ApprovalRequest 是对话框需要的展示数据（与 permission.ApprovalRequest
// 字段对齐但独立定义：TUI 不反向依赖 permission 包）。
type ApprovalRequest struct {
	ID         string
	ToolName   string
	Purpose    string
	Paths      []string
	Diff       string
	RiskReason string
}

// NewApproval 创建审批对话框。respond 在用户做出决定时被调用一次；
// 返回值 false 表示应答已过期（请求已被超时/关闭），调用方无需处理。
func NewApproval(req ApprovalRequest, respond func(approved bool) bool) *ApprovalDialog {
	return &ApprovalDialog{
		baseDialog: baseDialog{
			title: "权限确认",
		},
		req:     req,
		Respond: respond,
	}
}

// Request 返回本对话框对应的请求（测试观察点）。
func (d *ApprovalDialog) Request() ApprovalRequest { return d.req }

// Decided 报告是否已做出决定（测试观察点）。
func (d *ApprovalDialog) Decided() bool { return d.decided }

func (d *ApprovalDialog) respond(approved bool) tea.Cmd {
	if d.decided {
		return nil
	}
	d.decided = true
	if d.Respond != nil {
		d.Respond(approved)
	}
	return nil
}

// Update 处理按键：y=允许，n/esc=拒绝，上下键滚动 diff 预览。
func (d *ApprovalDialog) Update(msg tea.Msg) (Dialog, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "y", "Y", "enter":
			return d, tea.Batch(d.respond(true), closeDialogCmd())
		case "n", "N", "esc", "q":
			return d, tea.Batch(d.respond(false), closeDialogCmd())
		case "up", "k":
			if d.scroll > 0 {
				d.scroll--
			}
		case "down", "j":
			d.scroll++
		}
	}
	return d, nil
}

// closeDialogCmd 通知 Manager 移除本对话框。
func closeDialogCmd() tea.Cmd { return func() tea.Msg { return CloseDialogMsg{} } }

// CloseDialogMsg 由 Manager 消费：移除栈顶对话框。审批（以及其他自关闭）
// 对话框用它把「我决定完了」传回事件循环。
type CloseDialogMsg struct{}

// View 渲染审批内容：目的/路径/diff/风险依据/键位提示。
func (d *ApprovalDialog) View(width int) string {
	var buf strings.Builder
	buf.WriteString(styleDialogTitle.Render(d.title))
	buf.WriteString("\n\n")

	buf.WriteString(styleDialogMsg.Render("工具: " + d.req.ToolName))
	buf.WriteString("\n")
	buf.WriteString(styleDialogMsg.Render("目的: " + d.req.Purpose))
	buf.WriteString("\n")

	if len(d.req.Paths) > 0 {
		buf.WriteString(styleDialogMsg.Render("路径:"))
		buf.WriteString("\n")
		for _, p := range d.req.Paths {
			buf.WriteString(styleDialogMsg.Render("  " + p))
			buf.WriteString("\n")
		}
	}

	if d.req.RiskReason != "" {
		buf.WriteString(styleDialogWarn.Render("风险: " + d.req.RiskReason))
		buf.WriteString("\n")
	}

	if d.req.Diff != "" {
		buf.WriteString("\n")
		buf.WriteString(styleDialogMsg.Render("预览:"))
		buf.WriteString("\n")
		lines := strings.Split(d.req.Diff, "\n")
		// 简单滚动窗口：以 scroll 为起点，最多 12 行。
		if d.scroll > len(lines) {
			d.scroll = len(lines)
		}
		end := d.scroll + 12
		if end > len(lines) {
			end = len(lines)
		}
		for _, line := range lines[d.scroll:end] {
			r := []rune(line)
			if len(r) > width-6 && width > 8 {
				line = string(r[:width-6]) + "…"
			}
			buf.WriteString(styleDialogMsg.Render("  " + line))
			buf.WriteString("\n")
		}
		if end < len(lines) {
			buf.WriteString(styleDialogUnselected.Render(fmt.Sprintf("  …（%d/%d 行，↓ 继续）", end, len(lines))))
			buf.WriteString("\n")
		}
	}

	buf.WriteString("\n")
	buf.WriteString(styleDialogSelected.Render(" y 允许 "))
	buf.WriteString("  ")
	buf.WriteString(styleDialogUnselected.Render(" n 拒绝 "))
	buf.WriteString("  ")
	buf.WriteString(styleDialogUnselected.Render(" esc 关闭(=拒绝) "))
	buf.WriteString("\n")
	buf.WriteString(styleDialogUnselected.Render("超时未响应将自动拒绝"))
	return buf.String()
}

// styleDialogWarn 风险行样式（黄色系，非唯一信息载体——文字前缀「风险:」
// 承载语义，颜色只是辅助）。
var styleDialogWarn = lipgloss.NewStyle().
	Foreground(lipgloss.Color("220"))
