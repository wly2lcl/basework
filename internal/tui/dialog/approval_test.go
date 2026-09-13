package dialog

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func approvalKey(k string) tea.KeyPressMsg {
	// KeyPressMsg 由键位字符串构造（与 ConfirmDialog 测试同思路）。
	return tea.KeyPressMsg{Code: rune(k[0])}
}

// TestApprovalDialog_KeysCompleteFlow 键盘可完成全部操作（验收条件）：
// y=允许、n=拒绝、esc=关闭（等价拒绝），且决定只提交一次。
func TestApprovalDialog_KeysCompleteFlow(t *testing.T) {
	calls := 0
	lastApproved := false
	d := NewApproval(ApprovalRequest{
		ID:         "apr-1",
		ToolName:   "bash",
		Purpose:    "执行 shell 命令",
		Paths:      []string{"/tmp/x"},
		RiskReason: "命令包含: 删除文件",
		Diff:       "$ rm -rf /tmp/x",
	}, func(approved bool) bool {
		calls++
		lastApproved = approved
		return true
	})

	// esc：关闭 = 拒绝。
	_, _ = d.Update(approvalKey("y"))
	if !d.Decided() || !lastApproved || calls != 1 {
		t.Fatalf("y 应允许: decided=%v approved=%v calls=%d", d.Decided(), lastApproved, calls)
	}

	// 决定后重复按键不再提交（幂等）。
	_, _ = d.Update(approvalKey("n"))
	if calls != 1 {
		t.Fatalf("决定后不应重复提交: %d", calls)
	}
}

// TestApprovalDialog_DenyAndClosePaths 拒绝与关闭路径。
func TestApprovalDialog_DenyAndClosePaths(t *testing.T) {
	// n = 拒绝。
	var approved bool
	hasResponded := false
	d := NewApproval(ApprovalRequest{ID: "apr-2", ToolName: "bash"}, func(a bool) bool {
		approved, hasResponded = a, true
		return true
	})
	_, _ = d.Update(approvalKey("n"))
	if !hasResponded || approved {
		t.Fatalf("n 应拒绝: approved=%v responded=%v", approved, hasResponded)
	}

	// esc = 关闭 = 拒绝。
	hasResponded = false
	d2 := NewApproval(ApprovalRequest{ID: "apr-3", ToolName: "bash"}, func(a bool) bool {
		approved, hasResponded = a, true
		return true
	})
	_, _ = d2.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if !hasResponded || approved {
		t.Fatalf("esc 应等价拒绝: approved=%v responded=%v", approved, hasResponded)
	}
}

// TestApprovalDialog_ViewShowsEvidence 展示层：目的/路径/风险/预览可见。
func TestApprovalDialog_ViewShowsEvidence(t *testing.T) {
	d := NewApproval(ApprovalRequest{
		ID:         "apr-4",
		ToolName:   "edit_files",
		Purpose:    "修改文件",
		Paths:      []string{"/repo/a.go"},
		RiskReason: "命中敏感路径",
		Diff:       "- old\n+ new",
	}, nil)
	view := d.View(80)
	for _, want := range []string{"edit_files", "修改文件", "/repo/a.go", "命中敏感路径", "- old", "+ new"} {
		if !strings.Contains(view, want) {
			t.Fatalf("视图缺少 %q:\n%s", want, view)
		}
	}
}
