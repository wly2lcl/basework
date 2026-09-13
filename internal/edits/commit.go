// Package edits —— 批量提交与安全撤销（EDIT-002）。
//
// 与 plan.go 的分工：plan.go 只回答"打算改成什么"（不碰磁盘），本文件回答"真的改"和
// "改回去"。两者共用一个不可绕过的前提——所有路径解析与权限检查都走
// `Planner.ResolvePath`，提交阶段**再走一遍**，而不是相信预览阶段的结果。
//
// 三条边界是刻意设计的，也是不能含糊的地方：
//
//  1. **逐文件原子**：每个文件写进同目录的临时文件再 rename 覆盖。rename 在同一个
//     文件系统内是原子的，所以单个文件不会出现"写了一半"。
//  2. **跨文件不是事务**：没有两阶段提交，也没有全局锁。N 个文件就是 N 次独立的
//     rename，中间任何时刻崩溃都会留下"前 k 个已改、后面的没改"的磁盘状态。
//     所以本包不声称原子性，而是把**每一个文件的结局**都记录下来（已写/冲突/失败/
//     未尝试），让调用方拿着准确清单决定下一步。
//  3. **撤销不覆盖后来编辑**：撤销前重新读文件并比对"本次写入结果哈希"。不等就跳过
//     并说明原因——用户在我们提交之后又改了文件，这时撤销等于用旧内容覆盖新工作。
package edits

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// 提交与撤销相关的可判定错误。
var (
	// ErrNoOperations 表示没有给出任何操作。
	ErrNoOperations = errors.New("edits: 没有待提交的操作")
	// ErrBatchPartial 表示批量提交没有全部成功（部分文件已写、部分未写）。
	ErrBatchPartial = errors.New("edits: 批量提交部分成功")
)

// CommitState 是单个文件在提交后的结局。
type CommitState string

const (
	// CommitWritten 表示该文件已经写入。
	CommitWritten CommitState = "written"
	// CommitConflict 表示文件当前内容与计划基线不一致，**未写入**。
	CommitConflict CommitState = "conflict"
	// CommitFailed 表示写入或校验过程中出错，**未写入**。
	CommitFailed CommitState = "failed"
	// CommitNotAttempted 表示因为前面的失败而根本没有尝试。
	CommitNotAttempted CommitState = "not_attempted"
)

// FileOutcome 是一个文件在本次提交中的结局。
type FileOutcome struct {
	RelPath string      `json:"rel_path"`
	AbsPath string      `json:"abs_path"`
	State   CommitState `json:"state"`
	// Err 是失败/冲突的原因，成功时为空。
	Err string `json:"error,omitempty"`
	// BaseHash / ResultHash 是该文件计划中的基线与目标哈希（短形式便于阅读）。
	BaseHash   string `json:"base_hash"`
	ResultHash string `json:"result_hash"`
	// OperationIDs 是该文件涉及的操作，便于把清单对回用户看到的预览条目。
	OperationIDs []string `json:"operation_ids"`
}

// FileChange 是同一个文件上（可能多个）操作汇总后的一个写入单元。
type FileChange struct {
	AbsPath string
	RelPath string
	// BaseHash 是变更前文件内容的哈希；同文件的所有操作必须共享同一个基线。
	BaseHash string
	// ResultHash 是应用全部操作后的内容哈希。
	ResultHash string
	// OriginalContent / NewContent 分别是变更前与变更后的完整内容。
	OriginalContent []byte
	NewContent      []byte
	// Ops 按应用顺序排列。Diff 逐条保留在 Operation 上——把多个操作的 diff 拼成
	// 一个"看起来统一"的 diff 会掩盖"它们是在不同步骤应用的"这一事实。
	Ops []*Operation
}

// Batch 是一批待提交的文件变更。
type Batch struct {
	Files []*FileChange
}

// Files 返回涉及的文件数。
func (b *Batch) Len() int { return len(b.Files) }

// OperationIDs 返回全部操作 ID，按提交顺序。
func (b *Batch) OperationIDs() []string {
	var out []string
	for _, f := range b.Files {
		for _, op := range f.Ops {
			out = append(out, op.ID)
		}
	}
	return out
}

// RelPaths 返回全部文件的相对路径，按提交顺序。
func (b *Batch) RelPaths() []string {
	out := make([]string, 0, len(b.Files))
	for _, f := range b.Files {
		out = append(out, f.RelPath)
	}
	return out
}

// Prepare 把一组操作按文件归并成一个批次，并在此阶段发现"操作之间"的冲突。
//
// 为什么要有这个阶段而不是直接写：多个操作落在同一个文件上时，它们的基线都是
// **原始文件**，逐个独立应用并不等价于"按顺序应用"——后面的操作可能因为前面的替换
// 而找不到匹配，或者因为前面的新文本恰好包含它的搜索文本而变得不唯一。
// 这些情况必须在写盘之前暴露，否则就会出现"改了半个文件"。
//
// 本阶段不写盘。它只做三件事：解析路径（含权限检查）、校验基线哈希、按顺序试应用。
func (p *Planner) Prepare(ops ...*Operation) (*Batch, error) {
	if len(ops) == 0 {
		return nil, ErrNoOperations
	}
	for i, op := range ops {
		if op == nil {
			return nil, fmt.Errorf("edits: 第 %d 个操作为空", i+1)
		}
	}

	order := make([]string, 0, len(ops))
	grouped := make(map[string][]*Operation, len(ops))

	for _, op := range ops {
		// 再解析一次：路径解析同时完成"工作区内"、"软链不逃逸"、"权限允许"三项检查。
		// 预览到提交之间目录可能被换成软链，所以这一步不能省。
		abs, err := p.ResolvePath(op.RequestedPath)
		if err != nil {
			return nil, fmt.Errorf("edits: 操作 %s 的目标不可提交: %w", op.ID, err)
		}
		if _, ok := grouped[abs]; !ok {
			order = append(order, abs)
		}
		grouped[abs] = append(grouped[abs], op)
	}

	batch := &Batch{}
	for _, abs := range order {
		fileOps := grouped[abs]
		raw, err := os.ReadFile(abs)
		if err != nil {
			return nil, fmt.Errorf("edits: 读取 %s 失败: %w", abs, err)
		}

		// 同一文件上的所有操作必须基于同一份基线；否则它们本来就是互斥的计划。
		baseHash := fileOps[0].BaseHash
		for _, op := range fileOps[1:] {
			if op.BaseHash != baseHash {
				return nil, fmt.Errorf("%w: %s 上的操作基线不一致（%s vs %s）",
					ErrBaseConflict, fileOps[0].RelPath, shortHash(baseHash), shortHash(op.BaseHash))
			}
		}
		if got := hashBytes(raw); got != baseHash {
			return nil, fmt.Errorf("%w: %s 计划基线 %s，当前内容 %s",
				ErrBaseConflict, fileOps[0].RelPath, shortHash(baseHash), shortHash(got))
		}

		text, hadBOM, err := decodeText(raw)
		if err != nil {
			return nil, fmt.Errorf("edits: %s: %w", fileOps[0].RelPath, err)
		}

		// 按顺序试应用：任意一步找不到或匹配不唯一就整体放弃，不写任何内容。
		working := text
		for _, op := range fileOps {
			next, err := applyOperation(working, op)
			if err != nil {
				return nil, fmt.Errorf("edits: 应用操作 %s 失败: %w", op.ID, err)
			}
			working = next
		}
		// decodeText 会把 BOM 摘掉，写回时必须原样补上——否则一次编辑就静默丢掉 BOM，
		// 而带 BOM 的文件（Windows 工具链常见）会因此改变读取行为。
		if hadBOM {
			working = "\ufeff" + working
		}

		resultBytes := []byte(working)
		// 单操作时结果必须与预览算出的内容逐字节一致——这是"预览不是另一种算法"的证据。
		if len(fileOps) == 1 && string(resultBytes) != fileOps[0].NewContent {
			return nil, fmt.Errorf("edits: 操作 %s 的提交结果与预览不一致（预览与提交算法漂移）", fileOps[0].ID)
		}

		rel := fileOps[0].RelPath
		if r, err := filepath.Rel(p.root, abs); err == nil {
			rel = filepath.ToSlash(r)
		}
		batch.Files = append(batch.Files, &FileChange{
			AbsPath:         abs,
			RelPath:         rel,
			BaseHash:        baseHash,
			ResultHash:      hashBytes(resultBytes),
			OriginalContent: raw,
			NewContent:      resultBytes,
			Ops:             fileOps,
		})
	}
	return batch, nil
}

// applyOperation 在一次替换的层面应用操作，匹配规则与 Preview 完全一致
// （先原文精确匹配，必要时按文件换行风格归一化后重试）。
func applyOperation(text string, op *Operation) (string, error) {
	search := op.Old
	if strings.Count(text, search) != 1 {
		alt := normalizeNewlines(op.Old, op.Newline)
		if alt != search && strings.Count(text, alt) == 1 {
			search = alt
		}
	}
	switch n := strings.Count(text, search); {
	case n == 0:
		return "", fmt.Errorf("%w: %q", ErrNoMatch, truncateForError(op.Old))
	case n > 1:
		return "", fmt.Errorf("%w: %q 出现 %d 次", ErrAmbiguousMatch, truncateForError(op.Old), n)
	}
	return strings.Replace(text, search, normalizeNewlines(op.New, op.Newline), 1), nil
}

// CommitOptions 配置提交与撤销。
type CommitOptions struct {
	// WriteFile 是可注入的写入实现；nil 时用"同目录临时文件 + rename"的原子替换。
	// 注入点是必要的：没有它，"第 N 个文件写失败"这条验收只能靠祈祷。
	WriteFile func(path string, data []byte, perm os.FileMode) error
	// StopOnError 为 true 时，第一个冲突或失败之后的文件记为 not_attempted；
	// 为 false 时继续尝试其余文件。两种模式都会给出完整的逐文件清单。
	StopOnError bool
}

func (o CommitOptions) writeFile() func(string, []byte, os.FileMode) error {
	if o.WriteFile != nil {
		return o.WriteFile
	}
	return writeFileAtomic
}

// CommitResult 是一次提交的结果。
//
// 它保留已写文件的原始内容，用于撤销。**这是内存代价**：本包只处理文本编辑，
// 不处理大文件；因为撤销需要"原始字节"，而跨进程撤销需要持久化（EDIT-003 的会话事件）。
// 本包不假装支持跨进程撤销。
type CommitResult struct {
	Batch    *Batch
	Outcomes []FileOutcome

	originals map[string][]byte
	perm      map[string]os.FileMode
}

// Written 返回已写入的文件结局。
func (r *CommitResult) Written() []FileOutcome { return r.filter(CommitWritten) }

// NotWritten 返回未写入的文件结局（冲突 / 失败 / 未尝试）。
func (r *CommitResult) NotWritten() []FileOutcome {
	return r.filter(CommitConflict, CommitFailed, CommitNotAttempted)
}

// OK 报告是否全部文件都已写入。
func (r *CommitResult) OK() bool { return len(r.Outcomes) > 0 && len(r.Written()) == len(r.Outcomes) }

func (r *CommitResult) filter(states ...CommitState) []FileOutcome {
	want := make(map[CommitState]bool, len(states))
	for _, s := range states {
		want[s] = true
	}
	var out []FileOutcome
	for _, o := range r.Outcomes {
		if want[o.State] {
			out = append(out, o)
		}
	}
	return out
}

// Summary 给出人类可读的清单摘要，用于"中途失败后到底改了什么"。
func (r *CommitResult) Summary() string {
	var b strings.Builder
	fmt.Fprintf(&b, "提交结果：共 %d 个文件，已写 %d，未写 %d",
		len(r.Outcomes), len(r.Written()), len(r.NotWritten()))
	for _, o := range r.Outcomes {
		fmt.Fprintf(&b, "\n  - %s [%s]", o.RelPath, o.State)
		if o.Err != "" {
			fmt.Fprintf(&b, " %s", o.Err)
		}
	}
	return b.String()
}

// Commit 执行批次提交。
//
// 每个文件在写入**之前**重新做三件事：路径与权限重解析、基线哈希比对、
// 原子替换。顺序不能变——先写再校验等于"先覆盖再问用户同不同意"。
//
// 返回值：即使部分文件失败也返回 CommitResult（清单是主要产出），
// 同时返回包装了 ErrBatchPartial 的错误，让调用方既能拿到清单也能拿到"没全成功"的信号。
func (p *Planner) Commit(batch *Batch, opts CommitOptions) (*CommitResult, error) {
	if batch == nil || len(batch.Files) == 0 {
		return nil, ErrNoOperations
	}
	write := opts.writeFile()
	result := &CommitResult{
		Batch:     batch,
		originals: make(map[string][]byte, len(batch.Files)),
		perm:      make(map[string]os.FileMode, len(batch.Files)),
	}

	stopped := false
	for _, fc := range batch.Files {
		outcome := FileOutcome{
			RelPath:      fc.RelPath,
			AbsPath:      fc.AbsPath,
			BaseHash:     shortHash(fc.BaseHash),
			ResultHash:   shortHash(fc.ResultHash),
			OperationIDs: opIDs(fc.Ops),
		}

		if stopped {
			outcome.State = CommitNotAttempted
			outcome.Err = "前一个文件未成功，已停止"
			result.Outcomes = append(result.Outcomes, outcome)
			continue
		}

		// 1. 重新解析路径：工作区边界、软链逃逸、权限三项都要再查一遍。
		abs, err := p.ResolvePath(fc.Ops[0].RequestedPath)
		if err != nil {
			outcome.State = CommitFailed
			outcome.Err = err.Error()
			result.Outcomes = append(result.Outcomes, outcome)
			stopped = opts.StopOnError
			continue
		}
		if abs != fc.AbsPath {
			outcome.State = CommitFailed
			outcome.Err = fmt.Sprintf("目标路径在预览后发生变化：%s → %s", fc.AbsPath, abs)
			result.Outcomes = append(result.Outcomes, outcome)
			stopped = opts.StopOnError
			continue
		}

		// 2. 重新读取并比对基线：冲突时**不写**，绝不覆盖用户的新改动。
		current, err := os.ReadFile(abs)
		if err != nil {
			outcome.State = CommitFailed
			outcome.Err = fmt.Sprintf("读取当前内容失败: %v", err)
			result.Outcomes = append(result.Outcomes, outcome)
			stopped = opts.StopOnError
			continue
		}
		if got := hashBytes(current); got != fc.BaseHash {
			outcome.State = CommitConflict
			outcome.Err = fmt.Sprintf("%v: 计划基线 %s，当前内容 %s",
				ErrBaseConflict, shortHash(fc.BaseHash), shortHash(got))
			result.Outcomes = append(result.Outcomes, outcome)
			stopped = opts.StopOnError
			continue
		}

		// 3. 写入。权限沿用原文件的，不用 umask 猜。
		info, statErr := os.Stat(abs)
		perm := os.FileMode(0o644)
		if statErr == nil {
			perm = info.Mode().Perm()
		}
		if err := write(abs, fc.NewContent, perm); err != nil {
			outcome.State = CommitFailed
			outcome.Err = fmt.Sprintf("写入失败: %v", err)
			result.Outcomes = append(result.Outcomes, outcome)
			stopped = opts.StopOnError
			continue
		}

		outcome.State = CommitWritten
		result.originals[abs] = fc.OriginalContent
		result.perm[abs] = perm
		result.Outcomes = append(result.Outcomes, outcome)
	}

	if !result.OK() {
		return result, fmt.Errorf("%w: %d 个文件已写，%d 个未写",
			ErrBatchPartial, len(result.Written()), len(result.NotWritten()))
	}
	return result, nil
}

// RollbackState 是单个文件在撤销后的结局。
type RollbackState string

const (
	// RollbackReverted 表示已恢复为提交前的内容。
	RollbackReverted RollbackState = "reverted"
	// RollbackSkippedModified 表示文件在提交之后又被改动过，**未恢复**。
	RollbackSkippedModified RollbackState = "skipped_modified"
	// RollbackFailed 表示恢复过程中出错。
	RollbackFailed RollbackState = "failed"
	// RollbackNotWritten 表示该文件本来就没有被本次提交写入。
	RollbackNotWritten RollbackState = "not_written"
)

// RollbackOutcome 是一个文件的撤销结局。
type RollbackOutcome struct {
	RelPath string        `json:"rel_path"`
	State   RollbackState `json:"state"`
	Err     string        `json:"error,omitempty"`
}

// RollbackResult 是一次撤销的结果。
type RollbackResult struct {
	Outcomes []RollbackOutcome
}

// Reverted 返回已恢复的文件数。
func (r *RollbackResult) Reverted() int {
	n := 0
	for _, o := range r.Outcomes {
		if o.State == RollbackReverted {
			n++
		}
	}
	return n
}

// Summary 给出人类可读的撤销摘要。
func (r *RollbackResult) Summary() string {
	var b strings.Builder
	fmt.Fprintf(&b, "撤销结果：共 %d 个文件，已恢复 %d", len(r.Outcomes), r.Reverted())
	for _, o := range r.Outcomes {
		fmt.Fprintf(&b, "\n  - %s [%s]", o.RelPath, o.State)
		if o.Err != "" {
			fmt.Fprintf(&b, " %s", o.Err)
		}
	}
	return b.String()
}

// Rollback 撤销本次提交中**已经写入**的文件。
//
// 撤销前必须确认"文件仍然等于本次写入的结果"：不相等说明用户在提交之后又编辑了它，
// 此时恢复旧内容就是用我们的旧快照覆盖用户的新工作。这类文件被跳过并标注原因，
// 而不是"尽力而为地覆盖"。
//
// 反过来说，本方法也不宣称把文件系统复原成一个全局一致状态：它逐个文件检查、逐个
// 文件恢复，中途失败的文件会被标出来。没有跨文件事务，也就没有"撤回了全部"这种说法。
func (r *CommitResult) Rollback(opts CommitOptions) (*RollbackResult, error) {
	if r == nil || r.Batch == nil {
		return nil, ErrNoOperations
	}
	write := opts.writeFile()
	res := &RollbackResult{}

	written := make(map[string]bool, len(r.Written()))
	for _, o := range r.Written() {
		written[o.AbsPath] = true
	}

	// 按提交的相反顺序恢复：这是依赖顺序上更安全的方向（后写的先撤）。
	files := make([]*FileChange, 0, len(r.Batch.Files))
	files = append(files, r.Batch.Files...)
	for i, j := 0, len(files)-1; i < j; i, j = i+1, j-1 {
		files[i], files[j] = files[j], files[i]
	}

	var failed int
	for _, fc := range files {
		outcome := RollbackOutcome{RelPath: fc.RelPath}
		if !written[fc.AbsPath] {
			outcome.State = RollbackNotWritten
			res.Outcomes = append(res.Outcomes, outcome)
			continue
		}
		current, err := os.ReadFile(fc.AbsPath)
		if err != nil {
			outcome.State = RollbackFailed
			outcome.Err = fmt.Sprintf("读取当前内容失败: %v", err)
			failed++
			res.Outcomes = append(res.Outcomes, outcome)
			continue
		}
		if got := hashBytes(current); got != fc.ResultHash {
			outcome.State = RollbackSkippedModified
			outcome.Err = fmt.Sprintf("提交后文件又被改动（当前 %s，本次写入结果 %s），不覆盖",
				shortHash(got), shortHash(fc.ResultHash))
			res.Outcomes = append(res.Outcomes, outcome)
			continue
		}
		perm := r.perm[fc.AbsPath]
		if perm == 0 {
			perm = 0o644
		}
		if err := write(fc.AbsPath, r.originals[fc.AbsPath], perm); err != nil {
			outcome.State = RollbackFailed
			outcome.Err = fmt.Sprintf("恢复失败: %v", err)
			failed++
			res.Outcomes = append(res.Outcomes, outcome)
			continue
		}
		outcome.State = RollbackReverted
		res.Outcomes = append(res.Outcomes, outcome)
	}

	if failed > 0 {
		return res, fmt.Errorf("edits: 撤销有 %d 个文件失败", failed)
	}
	return res, nil
}

// writeFileAtomic 用"同目录临时文件 + rename"替换文件。
//
// 临时文件必须与目标同目录：跨文件系统的 rename 会退化成"复制 + 删除"，
// 那就不再是原子的了。
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".basework-edit-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

// opIDs 收集操作 ID。
func opIDs(ops []*Operation) []string {
	out := make([]string, 0, len(ops))
	for _, op := range ops {
		out = append(out, op.ID)
	}
	return out
}
