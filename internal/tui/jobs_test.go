package tui

import (
	"strings"
	"testing"

	"github.com/wly2lcl/basework/internal/tui/theme"
)

// ---- UI-001：工具与后台任务状态卡片 ----

// TestJobStatus_StateLabelHasTextMarker 状态必须有文字标记，不依赖颜色。
func TestJobStatus_StateLabelHasTextMarker(t *testing.T) {
	cases := map[string]string{
		"queued":      "[排队]",
		"running":     "[运行中]",
		"succeeded":   "[成功]",
		"failed":      "[失败(退出码 2)]",
		"canceled":    "[已取消]",
		"timed_out":   "[超时]",
		"interrupted": "[已中断]",
	}
	for state, wantPrefix := range cases {
		got := JobStatus{State: state, ExitCode: 2}.StateLabel()
		if !strings.HasPrefix(got, wantPrefix) {
			t.Errorf("状态 %s 的标签应为 %s 前缀，得到 %q", state, wantPrefix, got)
		}
	}
}

// TestJobsView_LargeOutputTruncated 大输出必须截断分页，不整块渲染。
func TestJobsView_LargeOutputTruncated(t *testing.T) {
	big := strings.Repeat("line\n", 100000) // 约 600KB
	v := NewJobsView(func(jobID string, offset int64) JobOutputMsg {
		return JobOutputMsg{JobID: jobID, Text: big, More: true}
	}, nil)
	v.Toggle()
	v.jobs = []JobStatus{{ID: "job-1", Command: "yes", State: "running"}}
	v.HandleKey("enter") // 加载输出

	rendered := v.Render(80, theme.DefaultTheme)
	if strings.Count(rendered, "line") > jobOutputPageSize+5 {
		t.Fatalf("渲染行数应被限制在页大小内，实际 %d 行", strings.Count(rendered, "line"))
	}
	if !strings.Contains(rendered, "按 ] 续读") && !strings.Contains(rendered, "读取更多") {
		t.Fatal("应有续读提示")
	}
	// 输入不被卡死的关键：渲染是纯字符串拼接、有行数上限——上面已验证。
}

// TestJobsView_CancelTerminalRefusedAndRunningRequested 取消入口：
// 终态任务拒绝取消并提示；运行中任务发出取消请求。
func TestJobsView_CancelTerminalRefusedAndRunningRequested(t *testing.T) {
	cancelled := map[string]bool{}
	v := NewJobsView(nil, func(jobID string) error {
		cancelled[jobID] = true
		return nil
	})
	v.Toggle()
	v.jobs = []JobStatus{
		{ID: "job-done", Command: "echo done", State: "succeeded"},
		{ID: "job-run", Command: "sleep 100", State: "running"},
	}

	// 选中终态任务（第 0 项），x 应拒绝。
	v.HandleKey("x")
	if cancelled["job-done"] {
		t.Fatal("终态任务不应发出取消请求")
	}
	if !strings.Contains(v.Notice(), "已结束") {
		t.Fatalf("应有拒绝提示: %q", v.Notice())
	}

	// 移到运行中任务，x 应请求取消。
	v.HandleKey("down")
	v.HandleKey("x")
	if !cancelled["job-run"] {
		t.Fatal("运行中任务应发出取消请求")
	}
}

// TestJobsView_Pagination 翻页：] 前进（无更多时提示），[ 回退不低于 0。
func TestJobsView_Pagination(t *testing.T) {
	var pageSize int64 = 4096
	var lastOffset int64
	v := NewJobsView(func(jobID string, offset int64) JobOutputMsg {
		lastOffset = offset
		return JobOutputMsg{JobID: jobID, Offset: offset, Text: strings.Repeat("x", int(pageSize)), More: offset < pageSize*2}
	}, nil)
	v.Toggle()
	v.jobs = []JobStatus{{ID: "job-p", Command: "gen", State: "succeeded"}}

	v.HandleKey("enter")
	if lastOffset != 0 {
		t.Fatalf("首页 offset 应为 0，得到 %d", lastOffset)
	}
	// 输出 4096 字节 → 前进一页应到 4096+len(text)？按实现：offset+len(text)。
	v.HandleKey("]")
	if lastOffset <= 0 {
		t.Fatal("] 应前进")
	}
	// 继续前进直到无更多。
	for i := 0; i < 5; i++ {
		v.HandleKey("]")
	}
	if !strings.Contains(v.Notice(), "最后一页") {
		t.Fatalf("越过末页应提示: %q", v.Notice())
	}
	// 回退不应低于 0。
	v.HandleKey("[")
	v.HandleKey("[")
	if lastOffset < 0 {
		t.Fatal("回退不应低于 0")
	}
}

// TestJobsView_ListCapped 列表条目封顶，防止历史任务无限撑爆渲染。
func TestJobsView_ListCapped(t *testing.T) {
	v := NewJobsView(nil, nil)
	msg := JobStatusMsg{}
	for i := 0; i < jobsListMax+100; i++ {
		msg.Jobs = append(msg.Jobs, JobStatus{ID: "job-x", State: "succeeded"})
	}
	v.Update(msg)
	if len(v.jobs) != jobsListMax {
		t.Fatalf("列表应封顶在 %d，得到 %d", jobsListMax, len(v.jobs))
	}
}

// TestApp_Update_JobStatusMsgAndRunCancelNotice App 层：快照更新卡片、
// 运行取消/失败通知进入消息流（文字标记可见）。
func TestApp_Update_JobStatusMsgAndRunCancelNotice(t *testing.T) {
	app := NewApp("m", "p", "s1")

	app.Update(JobStatusMsg{Jobs: []JobStatus{
		{ID: "job-1", Command: "make", State: "running"},
		{ID: "job-0", Command: "old", State: "interrupted"},
	}})
	if !app.Jobs.Visible() == false {
		// 卡片默认隐藏，数据照常更新（打开即见）。
		_ = app.Jobs.Visible()
	}
	app.Jobs.Toggle()
	rendered := app.Jobs.Render(80, theme.DefaultTheme)
	if !strings.Contains(rendered, "[运行中]") || !strings.Contains(rendered, "[已中断]") {
		t.Fatalf("卡片应展示两种状态的文字标记:\n%s", rendered)
	}
	if !strings.Contains(rendered, "job-1") || !strings.Contains(rendered, "job-0") {
		t.Fatal("卡片应包含任务 ID")
	}

	// 取消的运行 → 通知可见。
	app.Update(RunEventMsg{RunID: "run-1", SessionID: "s1", Kind: "run.finished", Err: "context canceled"})
	found := false
	for _, msg := range app.Messages {
		if strings.Contains(msg.Content, "[运行已取消 run-1]") {
			found = true
		}
	}
	if !found {
		t.Fatal("取消的运行应有文字标记通知")
	}

	// 失败的运行 → 错误消息可见。
	app.Update(RunEventMsg{RunID: "run-2", SessionID: "s1", Kind: "run.finished", Err: "boom"})
	found = false
	for _, msg := range app.Messages {
		if strings.Contains(msg.Content, "[运行失败 run-2] boom") {
			found = true
		}
	}
	if !found {
		t.Fatal("失败的运行应有错误通知")
	}
}

// TestJobsView_NarrowWindow 窄窗口（宽度 20）渲染不 panic、不产生超长行。
func TestJobsView_NarrowWindow(t *testing.T) {
	big := strings.Repeat(strings.Repeat("宽", 200)+"\n", 5000)
	v := NewJobsView(func(jobID string, offset int64) JobOutputMsg {
		return JobOutputMsg{JobID: jobID, Text: big, More: true}
	}, nil)
	v.Toggle()
	v.jobs = []JobStatus{{ID: "job-narrow-1234567890", Command: strings.Repeat("cmd ", 60), State: "running"}}
	v.HandleKey("enter")

	rendered := v.Render(20, theme.DefaultTheme) // 不 panic 即第一关
	for _, line := range strings.Split(rendered, "\n") {
		if len([]rune(line)) > 400 {
			t.Fatalf("窄窗口下单行 rune 数应受控，得到 %d", len([]rune(line)))
		}
	}
}
