package edits

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

// writeFileMode 落一个权限可控的文件（plan_test.go 的 writeFile 固定 0644）。
func writeFileMode(t *testing.T, abs, content string, perm os.FileMode) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	if err := os.WriteFile(abs, []byte(content), perm); err != nil {
		t.Fatalf("写入 %s 失败: %v", abs, err)
	}
	// os.WriteFile 受 umask 影响，显式 chmod 一次保证权限断言可靠。
	if err := os.Chmod(abs, perm); err != nil {
		t.Fatalf("设置权限失败: %v", err)
	}
	return abs
}

// readFileText 读取文件内容。
func readFileText(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 %s 失败: %v", path, err)
	}
	return string(b)
}

// previewOp 生成一次操作的预览，失败即终止。
func previewOp(t *testing.T, p *Planner, path, old, new string) *Operation {
	t.Helper()
	op, err := p.Preview(path, old, new)
	if err != nil {
		t.Fatalf("生成预览失败: %v", err)
	}
	return op
}

// dirEntries 返回目录下的条目名，用于断言没有残留临时文件。
func dirEntries(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("列目录失败: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// TestPrepare_GroupsOpsByFileWithoutWriting 验证准备阶段归并同文件操作且不写盘。
func TestPrepare_GroupsOpsByFileWithoutWriting(t *testing.T) {
	p, root := newPlanner(t, nil)
	writeFile(t, root, "a.txt", "alpha one\nalpha two\n")
	writeFile(t, root, "b.txt", "beta\n")

	opA1 := previewOp(t, p, "a.txt", "alpha one", "ALPHA ONE")
	opA2 := previewOp(t, p, "a.txt", "alpha two", "ALPHA TWO")
	opB := previewOp(t, p, "b.txt", "beta", "BETA")

	batch, err := p.Prepare(opA1, opA2, opB)
	if err != nil {
		t.Fatalf("Prepare 失败: %v", err)
	}
	if batch.Len() != 2 {
		t.Fatalf("应归并成 2 个文件，实际 %d: %v", batch.Len(), batch.RelPaths())
	}
	if got := len(batch.Files[0].Ops); got != 2 {
		t.Fatalf("第一个文件应有 2 个操作，实际 %d", got)
	}
	want := "ALPHA ONE\nALPHA TWO\n"
	if got := string(batch.Files[0].NewContent); got != want {
		t.Fatalf("同文件多操作结果不符:\n want %q\n got  %q", want, got)
	}
	if got := len(batch.OperationIDs()); got != 3 {
		t.Fatalf("应记录 3 个操作 ID，实际 %d", got)
	}

	// 准备阶段全程不写盘。
	if got := readFileText(t, filepath.Join(root, "a.txt")); got != "alpha one\nalpha two\n" {
		t.Fatalf("Prepare 不应改文件，实际 %q", got)
	}
	if got := readFileText(t, filepath.Join(root, "b.txt")); got != "beta\n" {
		t.Fatalf("Prepare 不应改文件，实际 %q", got)
	}
}

// TestPrepare_DetectsCrossOpConflict 验证"操作之间"的冲突在写盘之前暴露。
func TestPrepare_DetectsCrossOpConflict(t *testing.T) {
	p, root := newPlanner(t, nil)
	target := writeFile(t, root, "f.txt", "foo\n")

	op1 := previewOp(t, p, "f.txt", "foo", "bar")
	op2 := previewOp(t, p, "f.txt", "foo", "baz")

	// 两个操作都基于同一份原文，但顺序应用时第二个已经找不到匹配。
	if _, err := p.Prepare(op1, op2); !errors.Is(err, ErrNoMatch) {
		t.Fatalf("应报 ErrNoMatch，得到 %v", err)
	}
	if got := readFileText(t, target); got != "foo\n" {
		t.Fatalf("Prepare 失败时不应改文件，实际 %q", got)
	}
}

// TestPrepare_DetectsAmbiguousAfterFirstOp 验证前一个操作把后一个的搜索文本变多义时被拒绝。
func TestPrepare_DetectsAmbiguousAfterFirstOp(t *testing.T) {
	p, root := newPlanner(t, nil)
	writeFile(t, root, "f.txt", "x\n")

	op1 := previewOp(t, p, "f.txt", "x", "yy")
	op2 := previewOp(t, p, "f.txt", "x", "z") // 先应用 op1 后，文本变成 "yy\n"，"x" 消失
	if _, err := p.Prepare(op1, op2); !errors.Is(err, ErrNoMatch) {
		t.Fatalf("应报 ErrNoMatch，得到 %v", err)
	}

	// 另一组：前一个操作让后一个的搜索文本变得不唯一。
	h := writeFile(t, root, "h.txt", "a\n")
	h1 := previewOp(t, p, "h.txt", "a", "a a") // 应用后 "a a\n" 里有 2 个 "a"
	h2 := previewOp(t, p, "h.txt", "a", "b")
	if _, err := p.Prepare(h1, h2); !errors.Is(err, ErrAmbiguousMatch) {
		t.Fatalf("应报 ErrAmbiguousMatch，得到 %v", err)
	}
	if got := readFileText(t, h); got != "a\n" {
		t.Fatalf("Prepare 失败时不应改文件: %q", got)
	}
}

// TestPrepare_RejectsStaleBaseline 验证基线在准备阶段就被比对（不看文件时间）。
func TestPrepare_RejectsStaleBaseline(t *testing.T) {
	p, root := newPlanner(t, nil)
	target := writeFile(t, root, "f.txt", "original\n")
	op := previewOp(t, p, "f.txt", "original", "changed")

	// 用户在预览之后改了文件：时间变了，内容也变了。
	writeFile(t, root, "f.txt", "user edit\n")

	if _, err := p.Prepare(op); !errors.Is(err, ErrBaseConflict) {
		t.Fatalf("应报 ErrBaseConflict，得到 %v", err)
	}
	if got := readFileText(t, target); got != "user edit\n" {
		t.Fatalf("准备失败不应改动用户的新内容: %q", got)
	}
}

// TestPrepare_SingleOpResultMatchesPreview 验证提交路径与预览路径不会漂移：
// 单操作批次的结果必须与预览给出的 NewContent 逐字节相同。
func TestPrepare_SingleOpResultMatchesPreview(t *testing.T) {
	p, root := newPlanner(t, nil)
	writeFile(t, root, "f.txt", "keep\nswap me\nkeep2\n")
	op := previewOp(t, p, "f.txt", "swap me", "replaced")

	batch, err := p.Prepare(op)
	if err != nil {
		t.Fatalf("Prepare 失败: %v", err)
	}
	if got, want := string(batch.Files[0].NewContent), op.NewContent; got != want {
		t.Fatalf("提交结果与预览不一致:\n want %q\n got  %q", want, got)
	}
}

// TestPrepare_RejectsEmptyInput 验证空输入有明确返回。
func TestPrepare_RejectsEmptyInput(t *testing.T) {
	p, _ := newPlanner(t, nil)
	if _, err := p.Prepare(); !errors.Is(err, ErrNoOperations) {
		t.Fatalf("无操作应报 ErrNoOperations，得到 %v", err)
	}
	if _, err := p.Prepare(nil); err == nil {
		t.Fatal("nil 操作应报错")
	}
}

// TestCommit_WritesAllFilesAndCleansUpTemp 验证正常提交：内容、权限与临时文件清理。
func TestCommit_WritesAllFilesAndCleansUpTemp(t *testing.T) {
	p, root := newPlanner(t, nil)
	writeFileMode(t, filepath.Join(root, "a.txt"), "alpha\n", 0o600)
	writeFileMode(t, filepath.Join(root, "b.txt"), "beta\n", 0o755)

	batch, err := p.Prepare(
		previewOp(t, p, "a.txt", "alpha", "ALPHA"),
		previewOp(t, p, "b.txt", "beta", "BETA"),
	)
	if err != nil {
		t.Fatalf("Prepare 失败: %v", err)
	}
	res, err := p.Commit(batch, CommitOptions{StopOnError: true})
	if err != nil {
		t.Fatalf("Commit 失败: %v\n%s", err, res.Summary())
	}
	if !res.OK() {
		t.Fatalf("应全部成功: %s", res.Summary())
	}
	if got := readFileText(t, filepath.Join(root, "a.txt")); got != "ALPHA\n" {
		t.Fatalf("内容不符: %q", got)
	}
	if got := readFileText(t, filepath.Join(root, "b.txt")); got != "BETA\n" {
		t.Fatalf("内容不符: %q", got)
	}
	// 权限沿用原文件，不被 umask 或临时文件默认值改写。
	//
	// Windows 不实现 Unix 权限位：os.Stat 一律返回 0666，私有性由目录 ACL 表达，
	// 断言只读位之外的东西在那边没有意义。这里不对 Windows 假装通过——
	// 而是明确说明这条断言覆盖不到它。
	if runtime.GOOS != "windows" {
		for name, want := range map[string]os.FileMode{"a.txt": 0o600, "b.txt": 0o755} {
			info, err := os.Stat(filepath.Join(root, name))
			if err != nil {
				t.Fatalf("stat 失败: %v", err)
			}
			if got := info.Mode().Perm(); got != want {
				t.Fatalf("%s 权限应为 %o，实际 %o", name, want, got)
			}
		}
	}
	// 目录里不应残留临时文件。
	for _, name := range dirEntries(t, root) {
		if strings.HasPrefix(name, ".basework-") {
			t.Fatalf("残留临时文件: %s", name)
		}
	}
}

// TestCommit_ConflictDoesNotOverwriteUserEdit 是核心验收：
// 冲突时不覆盖用户新改动，且后续文件不被继续尝试。
func TestCommit_ConflictDoesNotOverwriteUserEdit(t *testing.T) {
	p, root := newPlanner(t, nil)
	writeFile(t, root, "a.txt", "one\n")
	writeFile(t, root, "b.txt", "two\n")

	batch, err := p.Prepare(
		previewOp(t, p, "a.txt", "one", "ONE"),
		previewOp(t, p, "b.txt", "two", "TWO"),
	)
	if err != nil {
		t.Fatalf("Prepare 失败: %v", err)
	}

	// 批准之后、提交之前，用户改了第一个文件。
	writeFile(t, root, "a.txt", "user wrote this\n")

	res, err := p.Commit(batch, CommitOptions{StopOnError: true})
	if !errors.Is(err, ErrBatchPartial) {
		t.Fatalf("应返回 ErrBatchPartial，得到 %v", err)
	}
	if res.OK() {
		t.Fatal("有冲突时不能宣称成功")
	}
	// 用户的新内容必须原样保留。
	if got := readFileText(t, filepath.Join(root, "a.txt")); got != "user wrote this\n" {
		t.Fatalf("冲突文件被覆盖了: %q", got)
	}
	// 冲突之后的文件不应被尝试。
	if got := readFileText(t, filepath.Join(root, "b.txt")); got != "two\n" {
		t.Fatalf("冲突后不应继续写后续文件: %q", got)
	}
	if len(res.Outcomes) != 2 {
		t.Fatalf("应给出 2 条结局，实际 %d", len(res.Outcomes))
	}
	if res.Outcomes[0].State != CommitConflict {
		t.Fatalf("第一个文件应为 conflict，实际 %s", res.Outcomes[0].State)
	}
	if !strings.Contains(res.Outcomes[0].Err, "基线不一致") {
		t.Fatalf("冲突原因应可读: %q", res.Outcomes[0].Err)
	}
	if res.Outcomes[1].State != CommitNotAttempted {
		t.Fatalf("第二个文件应为 not_attempted，实际 %s", res.Outcomes[1].State)
	}
	if len(res.Written()) != 0 || len(res.NotWritten()) != 2 {
		t.Fatalf("已写/未写清单不符: %d/%d", len(res.Written()), len(res.NotWritten()))
	}
}

// TestCommit_PartialFailureListsExactlyWhatHappened 是核心验收：
// 注入第 2 个文件写失败，必须给出准确的已写/未写清单。
func TestCommit_PartialFailureListsExactlyWhatHappened(t *testing.T) {
	p, root := newPlanner(t, nil)
	names := []string{"a.txt", "b.txt", "c.txt"}
	for _, n := range names {
		writeFile(t, root, n, "old "+n+"\n")
	}

	var ops []*Operation
	for _, n := range names {
		ops = append(ops, previewOp(t, p, n, "old "+n, "new "+n))
	}
	batch, err := p.Prepare(ops...)
	if err != nil {
		t.Fatalf("Prepare 失败: %v", err)
	}

	var calls int32
	res, err := p.Commit(batch, CommitOptions{
		StopOnError: true,
		WriteFile: func(path string, data []byte, perm os.FileMode) error {
			if atomic.AddInt32(&calls, 1) == 2 {
				return fmt.Errorf("模拟磁盘写满")
			}
			return writeFileAtomic(path, data, perm)
		},
	})
	if !errors.Is(err, ErrBatchPartial) {
		t.Fatalf("应返回 ErrBatchPartial，得到 %v", err)
	}
	if len(res.Written()) != 1 || len(res.NotWritten()) != 2 {
		t.Fatalf("应 1 写 2 未写，实际 %d/%d", len(res.Written()), len(res.NotWritten()))
	}
	if res.Outcomes[0].State != CommitWritten {
		t.Fatalf("第 1 个应为 written，实际 %s", res.Outcomes[0].State)
	}
	if res.Outcomes[1].State != CommitFailed || !strings.Contains(res.Outcomes[1].Err, "磁盘写满") {
		t.Fatalf("第 2 个应为 failed 且带原因，实际 %s %q", res.Outcomes[1].State, res.Outcomes[1].Err)
	}
	if res.Outcomes[2].State != CommitNotAttempted {
		t.Fatalf("第 3 个应为 not_attempted，实际 %s", res.Outcomes[2].State)
	}
	// 磁盘状态必须与清单一致。
	if got := readFileText(t, filepath.Join(root, "a.txt")); got != "new a.txt\n" {
		t.Fatalf("a.txt 应已写入: %q", got)
	}
	if got := readFileText(t, filepath.Join(root, "b.txt")); got != "old b.txt\n" {
		t.Fatalf("b.txt 不应被写入: %q", got)
	}
	if got := readFileText(t, filepath.Join(root, "c.txt")); got != "old c.txt\n" {
		t.Fatalf("c.txt 不应被写入: %q", got)
	}
	summary := res.Summary()
	for _, want := range []string{"a.txt", "b.txt", "c.txt", "written", "failed", "not_attempted"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("摘要缺少 %q:\n%s", want, summary)
		}
	}
}

// TestCommit_StopOnErrorFalseKeepsGoing 验证另一种失败策略同样给出准确清单。
func TestCommit_StopOnErrorFalseKeepsGoing(t *testing.T) {
	p, root := newPlanner(t, nil)
	names := []string{"a.txt", "b.txt", "c.txt"}
	for _, n := range names {
		writeFile(t, root, n, "old\n")
	}
	var ops []*Operation
	for _, n := range names {
		ops = append(ops, previewOp(t, p, n, "old", "new"))
	}
	batch, err := p.Prepare(ops...)
	if err != nil {
		t.Fatalf("Prepare 失败: %v", err)
	}

	var calls int32
	res, err := p.Commit(batch, CommitOptions{
		StopOnError: false,
		WriteFile: func(path string, data []byte, perm os.FileMode) error {
			if atomic.AddInt32(&calls, 1) == 2 {
				return fmt.Errorf("模拟写入失败")
			}
			return writeFileAtomic(path, data, perm)
		},
	})
	if !errors.Is(err, ErrBatchPartial) {
		t.Fatalf("应返回 ErrBatchPartial，得到 %v", err)
	}
	if len(res.Written()) != 2 || len(res.NotWritten()) != 1 {
		t.Fatalf("应 2 写 1 未写，实际 %d/%d", len(res.Written()), len(res.NotWritten()))
	}
	if res.Outcomes[1].State != CommitFailed {
		t.Fatalf("第 2 个应为 failed，实际 %s", res.Outcomes[1].State)
	}
	if res.Outcomes[2].State != CommitWritten {
		t.Fatalf("不停止时第 3 个应已写入，实际 %s", res.Outcomes[2].State)
	}
	if got := readFileText(t, filepath.Join(root, "b.txt")); got != "old\n" {
		t.Fatalf("失败文件不应被改动: %q", got)
	}
}

// TestCommit_RechecksPermissionAfterApproval 验证"批准后再次校验权限"。
func TestCommit_RechecksPermissionAfterApproval(t *testing.T) {
	var deny atomic.Bool
	p, root := newPlanner(t, func(string) (bool, string) {
		if deny.Load() {
			return false, "已被拒绝"
		}
		return true, ""
	})
	target := writeFile(t, root, "secret.txt", "s\n")

	batch, err := p.Prepare(previewOp(t, p, "secret.txt", "s", "S"))
	if err != nil {
		t.Fatalf("Prepare 失败: %v", err)
	}

	deny.Store(true) // 批准之后权限被收回
	res, err := p.Commit(batch, CommitOptions{StopOnError: true})
	if !errors.Is(err, ErrBatchPartial) {
		t.Fatalf("应返回 ErrBatchPartial，得到 %v", err)
	}
	if res.Outcomes[0].State != CommitFailed {
		t.Fatalf("应为 failed，实际 %s", res.Outcomes[0].State)
	}
	if !strings.Contains(res.Outcomes[0].Err, "已被拒绝") {
		t.Fatalf("原因应说明权限拒绝: %q", res.Outcomes[0].Err)
	}
	if got := readFileText(t, target); got != "s\n" {
		t.Fatalf("权限被拒时不应写盘: %q", got)
	}
}

// TestCommit_DetectsSymlinkSwapAfterApproval 验证预览后被换成软链时不会写到工作区之外。
func TestCommit_DetectsSymlinkSwapAfterApproval(t *testing.T) {
	p, root := newPlanner(t, nil)
	outside := t.TempDir()
	victim := writeFile(t, outside, "victim.txt", "outside original\n")

	target := writeFile(t, root, "f.txt", "inside\n")
	batch, err := p.Prepare(previewOp(t, p, "f.txt", "inside", "INSIDE"))
	if err != nil {
		t.Fatalf("Prepare 失败: %v", err)
	}

	// 批准之后把目标换成指向工作区外的软链。
	if err := os.Remove(target); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if err := os.Symlink(victim, target); err != nil {
		t.Fatalf("创建软链失败: %v", err)
	}

	res, err := p.Commit(batch, CommitOptions{StopOnError: true})
	if !errors.Is(err, ErrBatchPartial) {
		t.Fatalf("应返回 ErrBatchPartial，得到 %v", err)
	}
	if res.Outcomes[0].State != CommitFailed {
		t.Fatalf("应为 failed，实际 %s", res.Outcomes[0].State)
	}
	if got := readFileText(t, victim); got != "outside original\n" {
		t.Fatalf("工作区外的文件被改了: %q", got)
	}
}

// TestRollback_RevertsWrittenFiles 验证撤销把已写文件恢复原样。
func TestRollback_RevertsWrittenFiles(t *testing.T) {
	p, root := newPlanner(t, nil)
	writeFile(t, root, "a.txt", "orig a\n")
	writeFile(t, root, "b.txt", "orig b\n")
	batch, err := p.Prepare(
		previewOp(t, p, "a.txt", "orig a", "new a"),
		previewOp(t, p, "b.txt", "orig b", "new b"),
	)
	if err != nil {
		t.Fatalf("Prepare 失败: %v", err)
	}
	res, err := p.Commit(batch, CommitOptions{StopOnError: true})
	if err != nil {
		t.Fatalf("Commit 失败: %v", err)
	}

	rb, err := res.Rollback(CommitOptions{})
	if err != nil {
		t.Fatalf("Rollback 失败: %v\n%s", err, rb.Summary())
	}
	if rb.Reverted() != 2 {
		t.Fatalf("应恢复 2 个文件: %s", rb.Summary())
	}
	if got := readFileText(t, filepath.Join(root, "a.txt")); got != "orig a\n" {
		t.Fatalf("a.txt 未恢复: %q", got)
	}
	if got := readFileText(t, filepath.Join(root, "b.txt")); got != "orig b\n" {
		t.Fatalf("b.txt 未恢复: %q", got)
	}
}

// TestRollback_SkipsFileModifiedAfterCommit 是核心验收：
// 撤销不覆盖后来编辑——文件在提交后又被改过时跳过并说明。
func TestRollback_SkipsFileModifiedAfterCommit(t *testing.T) {
	p, root := newPlanner(t, nil)
	writeFile(t, root, "a.txt", "orig a\n")
	writeFile(t, root, "b.txt", "orig b\n")
	batch, err := p.Prepare(
		previewOp(t, p, "a.txt", "orig a", "new a"),
		previewOp(t, p, "b.txt", "orig b", "new b"),
	)
	if err != nil {
		t.Fatalf("Prepare 失败: %v", err)
	}
	res, err := p.Commit(batch, CommitOptions{StopOnError: true})
	if err != nil {
		t.Fatalf("Commit 失败: %v", err)
	}

	// 提交之后用户又改了 b.txt。
	writeFile(t, root, "b.txt", "user changed later\n")

	rb, err := res.Rollback(CommitOptions{})
	if err != nil {
		t.Fatalf("Rollback 失败: %v", err)
	}
	if rb.Reverted() != 1 {
		t.Fatalf("只应恢复 1 个文件: %s", rb.Summary())
	}
	if got := readFileText(t, filepath.Join(root, "a.txt")); got != "orig a\n" {
		t.Fatalf("a.txt 应被恢复: %q", got)
	}
	if got := readFileText(t, filepath.Join(root, "b.txt")); got != "user changed later\n" {
		t.Fatalf("后来的编辑被覆盖了: %q", got)
	}
	var skipped *RollbackOutcome
	for i := range rb.Outcomes {
		if rb.Outcomes[i].RelPath == "b.txt" {
			skipped = &rb.Outcomes[i]
		}
	}
	if skipped == nil || skipped.State != RollbackSkippedModified {
		t.Fatalf("b.txt 应标记为 skipped_modified: %+v", rb.Outcomes)
	}
	if !strings.Contains(skipped.Err, "不覆盖") {
		t.Fatalf("跳过原因应可读: %q", skipped.Err)
	}
}

// TestRollback_MarksNeverWrittenFiles 验证冲突/失败的文件在撤销时被标注为未写入。
func TestRollback_MarksNeverWrittenFiles(t *testing.T) {
	p, root := newPlanner(t, nil)
	writeFile(t, root, "a.txt", "one\n")
	writeFile(t, root, "b.txt", "two\n")
	batch, err := p.Prepare(
		previewOp(t, p, "a.txt", "one", "ONE"),
		previewOp(t, p, "b.txt", "two", "TWO"),
	)
	if err != nil {
		t.Fatalf("Prepare 失败: %v", err)
	}
	writeFile(t, root, "a.txt", "conflict\n")

	res, _ := p.Commit(batch, CommitOptions{StopOnError: true})
	rb, err := res.Rollback(CommitOptions{})
	if err != nil {
		t.Fatalf("Rollback 失败: %v", err)
	}
	if rb.Reverted() != 0 {
		t.Fatalf("没有文件被写入，不应恢复任何文件: %s", rb.Summary())
	}
	for _, o := range rb.Outcomes {
		if o.State != RollbackNotWritten {
			t.Fatalf("%s 应为 not_written，实际 %s", o.RelPath, o.State)
		}
	}
	if got := readFileText(t, filepath.Join(root, "b.txt")); got != "two\n" {
		t.Fatalf("未写入的文件不应被改动: %q", got)
	}
}

// TestRollback_ReportFailureWhenWriteFails 验证撤销写失败时被如实报告。
func TestRollback_ReportFailureWhenWriteFails(t *testing.T) {
	p, root := newPlanner(t, nil)
	writeFile(t, root, "a.txt", "orig\n")
	batch, err := p.Prepare(previewOp(t, p, "a.txt", "orig", "new"))
	if err != nil {
		t.Fatalf("Prepare 失败: %v", err)
	}
	res, err := p.Commit(batch, CommitOptions{})
	if err != nil {
		t.Fatalf("Commit 失败: %v", err)
	}

	rb, err := res.Rollback(CommitOptions{
		WriteFile: func(string, []byte, os.FileMode) error {
			return fmt.Errorf("模拟撤销写失败")
		},
	})
	if err == nil {
		t.Fatal("撤销失败应返回错误")
	}
	if len(rb.Outcomes) != 1 || rb.Outcomes[0].State != RollbackFailed {
		t.Fatalf("应标记 failed: %+v", rb.Outcomes)
	}
	if rb.Reverted() != 0 {
		t.Fatal("失败时不应宣称已恢复")
	}
}

// TestCommitAndRollback_PreserveBOMAndCRLF 验证编码与换行在提交和撤销两端都保真。
func TestCommitAndRollback_PreserveBOMAndCRLF(t *testing.T) {
	p, root := newPlanner(t, nil)
	target := filepath.Join(root, "crlf.txt")
	original := "\ufeffline one\r\nline two\r\n"
	writeFile(t, root, "crlf.txt", original)

	op := previewOp(t, p, "crlf.txt", "line two", "LINE TWO")
	if !op.HadBOM {
		t.Fatal("应识别出 BOM")
	}
	if op.Newline != NewlineCRLF {
		t.Fatalf("应识别出 CRLF，得到 %s", op.Newline)
	}
	batch, err := p.Prepare(op)
	if err != nil {
		t.Fatalf("Prepare 失败: %v", err)
	}
	res, err := p.Commit(batch, CommitOptions{})
	if err != nil {
		t.Fatalf("Commit 失败: %v", err)
	}
	want := "\ufeffline one\r\nLINE TWO\r\n"
	if got := readFileText(t, target); got != want {
		t.Fatalf("提交后内容不符:\n want %q\n got  %q", want, got)
	}
	// BOM 与 CRLF 都应保留，且没有多余的单独 LF。
	if bytes.Contains([]byte(strings.ReplaceAll(readFileText(t, target), "\r\n", "")), []byte("\n")) {
		t.Fatal("出现裸 LF，换行风格被改坏")
	}

	if _, err := res.Rollback(CommitOptions{}); err != nil {
		t.Fatalf("Rollback 失败: %v", err)
	}
	if got := readFileText(t, target); got != original {
		t.Fatalf("撤销后未逐字节还原:\n want %q\n got  %q", original, got)
	}
}

// TestCommit_AppliesMultipleOpsInOrder 验证同文件多操作按顺序应用并只写一次。
func TestCommit_AppliesMultipleOpsInOrder(t *testing.T) {
	p, root := newPlanner(t, nil)
	target := writeFile(t, root, "f.txt", "one\ntwo\nthree\n")

	var writes int32
	batch, err := p.Prepare(
		previewOp(t, p, "f.txt", "one", "ONE"),
		previewOp(t, p, "f.txt", "three", "THREE"),
	)
	if err != nil {
		t.Fatalf("Prepare 失败: %v", err)
	}
	res, err := p.Commit(batch, CommitOptions{
		StopOnError: true,
		WriteFile: func(path string, data []byte, perm os.FileMode) error {
			atomic.AddInt32(&writes, 1)
			return writeFileAtomic(path, data, perm)
		},
	})
	if err != nil {
		t.Fatalf("Commit 失败: %v", err)
	}
	if n := atomic.LoadInt32(&writes); n != 1 {
		t.Fatalf("同文件的多个操作应只写一次，实际 %d 次", n)
	}
	want := "ONE\ntwo\nTHREE\n"
	if got := readFileText(t, target); got != want {
		t.Fatalf("内容不符:\n want %q\n got  %q", want, got)
	}
	if len(res.Outcomes) != 1 || len(res.Outcomes[0].OperationIDs) != 2 {
		t.Fatalf("结局应回指两个操作: %+v", res.Outcomes)
	}
}

// TestCommit_LeavesNoExtraFilesBehind 验证原子替换不会在目录里留下临时文件。
func TestCommit_LeavesNoExtraFilesBehind(t *testing.T) {
	p, root := newPlanner(t, nil)
	writeFile(t, root, "only.txt", "v1\n")
	batch, err := p.Prepare(previewOp(t, p, "only.txt", "v1", "v2"))
	if err != nil {
		t.Fatalf("Prepare 失败: %v", err)
	}
	if _, err := p.Commit(batch, CommitOptions{}); err != nil {
		t.Fatalf("Commit 失败: %v", err)
	}
	names := dirEntries(t, root)
	if len(names) != 1 || names[0] != "only.txt" {
		t.Fatalf("目录内容不符: %v", names)
	}
}

// TestCommit_RejectsEmptyBatch 验证空批次有明确返回。
func TestCommit_RejectsEmptyBatch(t *testing.T) {
	p, _ := newPlanner(t, nil)
	if _, err := p.Commit(nil, CommitOptions{}); !errors.Is(err, ErrNoOperations) {
		t.Fatalf("nil 批次应报 ErrNoOperations，得到 %v", err)
	}
	if _, err := p.Commit(&Batch{}, CommitOptions{}); !errors.Is(err, ErrNoOperations) {
		t.Fatalf("空批次应报 ErrNoOperations，得到 %v", err)
	}
	var nilRes *CommitResult
	if _, err := nilRes.Rollback(CommitOptions{}); !errors.Is(err, ErrNoOperations) {
		t.Fatalf("nil 结果撤销应报 ErrNoOperations，得到 %v", err)
	}
}

func TestRollback_RechecksPermissionAfterCommit(t *testing.T) {
	allowed := true
	p, root := newPlanner(t, func(string) (bool, string) { return allowed, "revoked" })
	file := writeFile(t, root, "a.txt", "old")
	op, err := p.Preview("a.txt", "old", "new")
	if err != nil {
		t.Fatal(err)
	}
	batch, err := p.Prepare(op)
	if err != nil {
		t.Fatal(err)
	}
	res, err := p.Commit(batch, CommitOptions{})
	if err != nil {
		t.Fatal(err)
	}
	allowed = false
	rb, err := res.Rollback(CommitOptions{})
	if err == nil || rb.Outcomes[0].State != RollbackFailed {
		t.Fatalf("撤销应因权限撤回失败: %+v, %v", rb, err)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("权限撤回后不应改写文件: %q", data)
	}
}

func TestRollbackRejectsParentSymlinkEscape(t *testing.T) {
	p, root := newPlanner(t, nil)
	writeFile(t, root, "sub/a.txt", "old")
	op, err := p.Preview("sub/a.txt", "old", "new")
	if err != nil {
		t.Fatal(err)
	}
	batch, err := p.Prepare(op)
	if err != nil {
		t.Fatal(err)
	}
	res, err := p.Commit(batch, CommitOptions{})
	if err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	target := filepath.Join(outside, "a.txt")
	if err := os.WriteFile(target, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "sub"), filepath.Join(root, "saved")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "sub")); err != nil {
		t.Fatal(err)
	}
	rb, err := res.Rollback(CommitOptions{})
	if err == nil || rb.Outcomes[0].State != RollbackFailed {
		t.Fatalf("软链逃逸应被拒绝: %+v, %v", rb, err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("工作区外文件被修改: %q", data)
	}
}
