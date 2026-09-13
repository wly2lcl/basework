package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/wly2lcl/basework/pkg/session"
	"github.com/wly2lcl/basework/pkg/tool"
)

// fakeEditSink 记录全部编辑事件，供断言。
type fakeEditSink struct {
	mu     sync.Mutex
	events []*session.FileEditedData
}

func (s *fakeEditSink) AppendEditEvent(data *session.FileEditedData) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, data)
	return nil
}

func (s *fakeEditSink) all() []*session.FileEditedData {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*session.FileEditedData(nil), s.events...)
}

// newEditTool 创建绑定临时工作区的工具。
func newEditTool(t *testing.T, sink EditEventSink) (*EditFilesTool, string) {
	t.Helper()
	root := t.TempDir()
	tool := NewEditFilesTool(root)
	tool.Sink = sink
	return tool, root
}

// mustWriteFile 写初始文件并返回绝对路径。
func mustWriteFile(t *testing.T, root, rel, content string) {
	t.Helper()
	abs := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("创建目录: %v", err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatalf("写文件: %v", err)
	}
}

func readEditFile(t *testing.T, root, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("读文件 %s: %v", rel, err)
	}
	return string(data)
}

func callEdit(t *testing.T, tool *EditFilesTool, params map[string]interface{}) *tool.Result {
	t.Helper()
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("序列化参数: %v", err)
	}
	res, err := tool.Execute(context.Background(), raw)
	if err != nil {
		t.Fatalf("Execute 返回错误: %v", err)
	}
	return res
}

func editEntry(path, old, new string) map[string]interface{} {
	return map[string]interface{}{"path": path, "old": old, "new": new}
}

// planIDFrom 从预览结果里抠出 plan_id。
func planIDFrom(t *testing.T, content string) string {
	t.Helper()
	marker := "计划 plan-"
	idx := strings.Index(content, marker)
	if idx < 0 {
		t.Fatalf("预览结果里没有 plan_id: %s", content)
	}
	rest := content[idx+len(marker):]
	if len(rest) < 12 {
		t.Fatalf("plan_id 截断: %s", content)
	}
	return "plan-" + rest[:12]
}

func TestEditFiles_PreviewDoesNotWrite(t *testing.T) {
	tool, root := newEditTool(t, &fakeEditSink{})
	mustWriteFile(t, root, "a.txt", "hello world\n")

	res := callEdit(t, tool, map[string]interface{}{
		"action": "preview",
		"edits":  []interface{}{editEntry("a.txt", "world", "Go")},
	})
	if res.IsError {
		t.Fatalf("预览不应报错: %s", res.Content)
	}
	if got := readEditFile(t, root, "a.txt"); got != "hello world\n" {
		t.Fatalf("预览改了文件: %q", got)
	}
	// diff 围绕替换点展示：被移除的匹配文本行与新增文本行都要出现。
	if !strings.Contains(res.Content, "-world") || !strings.Contains(res.Content, "+Go") {
		t.Fatalf("预览缺 diff 行:\n%s", res.Content)
	}
	if !strings.Contains(res.Content, "未写入磁盘") {
		t.Fatalf("预览未说明未写盘:\n%s", res.Content)
	}
}

func TestEditFiles_CommitWritesAndEmitsEvent(t *testing.T) {
	sink := &fakeEditSink{}
	tool, root := newEditTool(t, sink)
	tracker := session.NewFileTracker()
	tool.Tracker = tracker
	mustWriteFile(t, root, "a.txt", "hello world\n")

	preview := callEdit(t, tool, map[string]interface{}{
		"action": "preview",
		"edits":  []interface{}{editEntry("a.txt", "world", "Go")},
	})
	planID := planIDFrom(t, preview.Content)

	res := callEdit(t, tool, map[string]interface{}{
		"action":  "commit",
		"plan_id": planID,
		"verify":  "cat a.txt",
	})
	if res.IsError {
		t.Fatalf("提交不应报错: %s", res.Content)
	}
	if got := readEditFile(t, root, "a.txt"); got != "hello Go\n" {
		t.Fatalf("提交后内容不符: %q", got)
	}
	if !trackerHasFile(tracker, "a.txt") {
		t.Fatalf("提交后 FileTracker 应记录 a.txt")
	}

	events := sink.all()
	if len(events) != 2 {
		t.Fatalf("期望 2 条事件（预览+提交），得到 %d", len(events))
	}
	if events[0].Phase != "preview" || events[1].Phase != "committed" {
		t.Fatalf("阶段不符: %s, %s", events[0].Phase, events[1].Phase)
	}
	if events[1].Verify != "cat a.txt" {
		t.Fatalf("验证命令未记录: %+v", events[1])
	}
	if len(events[1].Files) != 1 || events[1].Files[0].State != "written" {
		t.Fatalf("提交事件文件结局不符: %+v", events[1].Files)
	}
}

func trackerHasFile(tracker *session.FileTracker, path string) bool {
	for _, f := range tracker.GetModifiedFiles() {
		if f == path {
			return true
		}
	}
	return false
}

func TestEditFiles_RejectsDuplicateCommit(t *testing.T) {
	tool, root := newEditTool(t, &fakeEditSink{})
	mustWriteFile(t, root, "a.txt", "one\n")

	preview := callEdit(t, tool, map[string]interface{}{
		"action": "preview",
		"edits":  []interface{}{editEntry("a.txt", "one", "two")},
	})
	planID := planIDFrom(t, preview.Content)

	if res := callEdit(t, tool, map[string]interface{}{"action": "commit", "plan_id": planID}); res.IsError {
		t.Fatalf("首次提交不应报错: %s", res.Content)
	}
	res := callEdit(t, tool, map[string]interface{}{"action": "commit", "plan_id": planID})
	if !res.IsError {
		t.Fatalf("重复提交应报错")
	}
	if !strings.Contains(res.Content, "拒绝重复提交") {
		t.Fatalf("报错应说明拒绝重复提交: %s", res.Content)
	}
	if got := readEditFile(t, root, "a.txt"); got != "two\n" {
		t.Fatalf("文件内容不应再次变化: %q", got)
	}
}

func TestEditFiles_CommitConflictAfterExternalChange(t *testing.T) {
	sink := &fakeEditSink{}
	tool, root := newEditTool(t, sink)
	mustWriteFile(t, root, "a.txt", "base\n")

	preview := callEdit(t, tool, map[string]interface{}{
		"action": "preview",
		"edits":  []interface{}{editEntry("a.txt", "base", "edited")},
	})
	planID := planIDFrom(t, preview.Content)

	// 预览后外部改动文件：提交必须以 conflict 结局拒绝写入。
	mustWriteFile(t, root, "a.txt", "user edited\n")

	res := callEdit(t, tool, map[string]interface{}{"action": "commit", "plan_id": planID})
	if !res.IsError {
		t.Fatalf("基线冲突应报错: %s", res.Content)
	}
	if got := readEditFile(t, root, "a.txt"); got != "user edited\n" {
		t.Fatalf("冲突时不得覆盖用户新改动: %q", got)
	}
	var conflictEvent *session.FileEditedData
	for _, ev := range sink.all() {
		if ev.Phase == "failed" {
			conflictEvent = ev
		}
	}
	if conflictEvent == nil {
		t.Fatalf("应有 failed 阶段事件")
	}
	if len(conflictEvent.Files) != 1 || conflictEvent.Files[0].State != "conflict" {
		t.Fatalf("事件应记录 conflict 结局: %+v", conflictEvent.Files)
	}
}

func TestEditFiles_RollbackRestoresAndGuardsUserEdits(t *testing.T) {
	sink := &fakeEditSink{}
	tool, root := newEditTool(t, sink)
	mustWriteFile(t, root, "a.txt", "v1\n")
	mustWriteFile(t, root, "b.txt", "b1\n")

	preview := callEdit(t, tool, map[string]interface{}{
		"action": "preview",
		"edits": []interface{}{
			editEntry("a.txt", "v1", "v2"),
			editEntry("b.txt", "b1", "b2"),
		},
	})
	planID := planIDFrom(t, preview.Content)
	if res := callEdit(t, tool, map[string]interface{}{"action": "commit", "plan_id": planID}); res.IsError {
		t.Fatalf("提交失败: %s", res.Content)
	}

	// 提交后用户又改了 b.txt：撤销必须跳过它。
	mustWriteFile(t, root, "b.txt", "user wrote\n")

	res := callEdit(t, tool, map[string]interface{}{"action": "rollback", "plan_id": planID})
	if res.IsError {
		t.Fatalf("撤销不应报错: %s", res.Content)
	}
	if got := readEditFile(t, root, "a.txt"); got != "v1\n" {
		t.Fatalf("a.txt 应恢复为 v1: %q", got)
	}
	if got := readEditFile(t, root, "b.txt"); got != "user wrote\n" {
		t.Fatalf("b.txt 被用户改过，不得覆盖: %q", got)
	}
	if !strings.Contains(res.Content, "skipped_modified") {
		t.Fatalf("撤销结果应标注 skipped_modified: %s", res.Content)
	}

	// 重复撤销被拒绝。
	res = callEdit(t, tool, map[string]interface{}{"action": "rollback", "plan_id": planID})
	if !res.IsError || !strings.Contains(res.Content, "不能重复撤销") {
		t.Fatalf("重复撤销应被拒绝: %s / %v", res.Content, res.IsError)
	}

	var rbEvent *session.FileEditedData
	for _, ev := range sink.all() {
		if ev.Phase == "rolled_back" {
			rbEvent = ev
		}
	}
	if rbEvent == nil {
		t.Fatalf("应有 rolled_back 事件")
	}
	states := map[string]string{}
	for _, f := range rbEvent.Files {
		states[f.Path] = f.State
	}
	if states["a.txt"] != "reverted" || states["b.txt"] != "skipped_modified" {
		t.Fatalf("撤销事件结局不符: %+v", states)
	}
}

func TestEditFiles_RollbackWithoutCommitRejected(t *testing.T) {
	tool, root := newEditTool(t, &fakeEditSink{})
	mustWriteFile(t, root, "a.txt", "x\n")

	preview := callEdit(t, tool, map[string]interface{}{
		"action": "preview",
		"edits":  []interface{}{editEntry("a.txt", "x", "y")},
	})
	planID := planIDFrom(t, preview.Content)

	res := callEdit(t, tool, map[string]interface{}{"action": "rollback", "plan_id": planID})
	if !res.IsError {
		t.Fatalf("未提交就撤销应报错")
	}
	if got := readEditFile(t, root, "a.txt"); got != "x\n" {
		t.Fatalf("文件不应被改: %q", got)
	}
}

func TestEditFiles_RejectsTraversalAndUnknownPlan(t *testing.T) {
	tool, root := newEditTool(t, &fakeEditSink{})
	mustWriteFile(t, root, "a.txt", "x\n")

	res := callEdit(t, tool, map[string]interface{}{
		"action": "preview",
		"edits":  []interface{}{editEntry("../outside.txt", "a", "b")},
	})
	if !res.IsError {
		t.Fatalf("越界路径应报错")
	}
	if !strings.Contains(res.Content, "超出工作区") {
		t.Fatalf("报错应说明越界: %s", res.Content)
	}

	res = callEdit(t, tool, map[string]interface{}{"action": "commit", "plan_id": "plan-nonexistent00"})
	if !res.IsError || !strings.Contains(res.Content, "不存在") {
		t.Fatalf("未知计划应报错: %s", res.Content)
	}

	res = callEdit(t, tool, map[string]interface{}{"action": "fly"})
	if !res.IsError || !strings.Contains(res.Content, "未知 action") {
		t.Fatalf("未知 action 应报错: %s", res.Content)
	}
}

func TestEditFiles_SamePlanIDForSameContent(t *testing.T) {
	tool, root := newEditTool(t, &fakeEditSink{})
	mustWriteFile(t, root, "a.txt", "stable\n")

	first := callEdit(t, tool, map[string]interface{}{
		"action": "preview",
		"edits":  []interface{}{editEntry("a.txt", "stable", "changed")},
	})
	id1 := planIDFrom(t, first.Content)

	// 回滚文件到基线后再次预览同一替换：plan_id 必须相同（稳定派生）。
	mustWriteFile(t, root, "a.txt", "stable\n")
	second := callEdit(t, tool, map[string]interface{}{
		"action": "preview",
		"edits":  []interface{}{editEntry("a.txt", "stable", "changed")},
	})
	id2 := planIDFrom(t, second.Content)
	if id1 != id2 {
		t.Fatalf("同一计划的 plan_id 应稳定: %s vs %s", id1, id2)
	}

	// 不同替换内容 → 不同 plan_id。
	third := callEdit(t, tool, map[string]interface{}{
		"action": "preview",
		"edits":  []interface{}{editEntry("a.txt", "stable", "other")},
	})
	if id3 := planIDFrom(t, third.Content); id3 == id1 {
		t.Fatalf("不同计划不应共享 plan_id")
	}
}

func TestEditFiles_PlansAreFIFOEvicted(t *testing.T) {
	tool, root := newEditTool(t, &fakeEditSink{})
	mustWriteFile(t, root, "a.txt", "v0\n")

	makePreview := func(from, to string) string {
		mustWriteFile(t, root, "a.txt", from+"\n")
		res := callEdit(t, tool, map[string]interface{}{
			"action": "preview",
			"edits":  []interface{}{editEntry("a.txt", from, to)},
		})
		return planIDFrom(t, res.Content)
	}

	// 填满上限 + 1 个计划，最旧的应被淘汰。
	var first string
	for i := 0; i <= maxTrackedPlans; i++ {
		id := makePreview("v"+itoa(i), "w"+itoa(i))
		if i == 0 {
			first = id
		}
	}
	// 注意：最后一步把文件写成了 v32（from），提交 first（基线 v0）必然 conflict，
	// 但这里断言的是"计划被淘汰"而非冲突，所以直接看报错文本。
	res := callEdit(t, tool, map[string]interface{}{"action": "commit", "plan_id": first})
	if !res.IsError {
		t.Fatalf("超出上限后最旧计划应已失效")
	}
	if !strings.Contains(res.Content, "不存在或已被清理") {
		t.Fatalf("报错应说明计划被清理: %s", res.Content)
	}
}

func TestEditFiles_SinkFailureDoesNotBlockEdit(t *testing.T) {
	tool, root := newEditTool(t, failingSink{})
	mustWriteFile(t, root, "a.txt", "hello world\n")

	preview := callEdit(t, tool, map[string]interface{}{
		"action": "preview",
		"edits":  []interface{}{editEntry("a.txt", "world", "Go")},
	})
	if preview.IsError {
		t.Fatalf("事件失败不应阻断预览: %s", preview.Content)
	}
	res := callEdit(t, tool, map[string]interface{}{"action": "commit", "plan_id": planIDFrom(t, preview.Content)})
	if res.IsError {
		t.Fatalf("事件失败不应阻断提交: %s", res.Content)
	}
	if got := readEditFile(t, root, "a.txt"); got != "hello Go\n" {
		t.Fatalf("提交结果不符: %q", got)
	}
}

type failingSink struct{}

func (failingSink) AppendEditEvent(*session.FileEditedData) error {
	return context.DeadlineExceeded
}
