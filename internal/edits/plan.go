// Package edits 生成「将要写入文件」的只读预览计划。
//
// 存在的理由是：`builtin.EditTool` 与 `WriteTool` 是「检查 → 读 → 改 → 写」一条路走到
// 底，没有任何一步能把结果交给人看。于是"预览"只能靠模型口述，而模型口述的 diff 不
// 是文件事实。本包把中间结果变成一等对象：基线哈希、工作区相对路径、上下文 diff、
// operation ID，全部来自真实字节，且生成过程不碰磁盘。
//
// 本包只做「计划」，不做「提交」。批量提交、跨文件补偿与撤销属于 EDIT-002。
package edits

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// 明确的可判定错误。调用方按 errors.Is 判定，不匹配错误文本。
var (
	// ErrOutsideWorkspace 表示目标路径落在工作区之外（含 ../ 穿越）。
	ErrOutsideWorkspace = errors.New("edits: 目标路径超出工作区")
	// ErrSymlinkEscape 表示符号链接解析后指向工作区之外。
	ErrSymlinkEscape = errors.New("edits: 符号链接指向工作区之外")
	// ErrPermissionDenied 表示产品层路径检查拒绝了该目标。
	ErrPermissionDenied = errors.New("edits: 路径检查拒绝")
	// ErrNoMatch 表示待替换文本在文件中不存在。
	ErrNoMatch = errors.New("edits: 未找到待替换文本")
	// ErrAmbiguousMatch 表示待替换文本出现多次，无法唯一确定位置。
	ErrAmbiguousMatch = errors.New("edits: 待替换文本出现多次")
	// ErrEmptySearch 表示待替换文本为空。
	ErrEmptySearch = errors.New("edits: 待替换文本不能为空")
	// ErrBinaryContent 表示文件不是可处理的 UTF-8 文本。
	ErrBinaryContent = errors.New("edits: 文件不是 UTF-8 文本")
	// ErrBaseConflict 表示文件当前内容与计划记录的基线不一致。
	ErrBaseConflict = errors.New("edits: 基线不一致")
)

// 换行风格。计划会保留文件原有风格，避免一次替换把整个文件的换行改掉。
const (
	NewlineLF   = "lf"
	NewlineCRLF = "crlf"
)

// LineKind 是 diff 行的类型。
type LineKind int

const (
	// LineContext 表示未改动的上下文行。
	LineContext LineKind = iota
	// LineAdded 表示新增行。
	LineAdded
	// LineRemoved 表示删除行。
	LineRemoved
)

// Prefix 返回统一 diff 使用的单字符前缀。
func (k LineKind) Prefix() string {
	switch k {
	case LineAdded:
		return "+"
	case LineRemoved:
		return "-"
	default:
		return " "
	}
}

// DiffLine 是一行 diff。
type DiffLine struct {
	Kind LineKind
	Text string
}

// Operation 是一次待提交的文件修改计划。
//
// 所有字段都从磁盘真实内容推导，没有任何一个是"模型说的"。
type Operation struct {
	// ID 是操作标识。由 相对路径 + 基线哈希 + 替换内容 派生，因此同一份计划
	// 在任何机器、任何次数上重新生成都得到同一个 ID（可用于幂等与去重）。
	ID string
	// RequestedPath 是调用方给出的路径原样，用于回溯"用户当时想改哪个文件"。
	RequestedPath string
	// RelPath 是工作区相对路径（指向解析后的真实文件），不会泄漏工作区之外的绝对路径。
	// 目标若本身是软链，这里给出的是链接目标——因为写入作用在目标上，报告链接本身
	// 会让人以为改的是另一个文件。
	RelPath string
	// AbsPath 是解析后的绝对路径；仅用于提交阶段，不用于展示。
	AbsPath string
	// BaseHash 是变更前文件内容的 sha256（含 BOM 与原始换行）。
	// 提交前用它做冲突校验——文件时间会被 cp/touch/checkout 改动，内容哈希不会。
	BaseHash string
	// ResultHash 是变更后内容的 sha256。
	ResultHash string
	// Old / New 是调用方给出的替换文本（原样保留，便于回溯）。
	Old string
	New string
	// NewContent 是变更后的完整文件内容。
	NewContent string
	// Diff 是围绕替换点的上下文 diff，不是通用最小编辑距离 diff。
	Diff []DiffLine
	// Newline 是文件原有换行风格（"lf" / "crlf"）。
	Newline string
	// HadBOM 记录文件是否带 UTF-8 BOM；带则原样保留。
	HadBOM bool
	// NormalizedSearch 为 true 表示调用方给的 old 用了另一种换行风格，
	// 计划为匹配把它归一化成了文件风格。这是需要让人知道的隐式改写。
	NormalizedSearch bool
}

// Options 配置 Planner。
type Options struct {
	// Root 是工作区根目录。所有目标必须落在它之内。
	Root string
	// CheckPath 是产品层路径权限入口（与 builtin 的敏感路径检查同一个）。
	// 为 nil 表示不做额外权限检查，此时只依赖工作区边界。
	CheckPath func(path string) (allowed bool, reason string)
	// ContextLines 是 diff 两侧保留的上下文行数，<=0 时用 3。
	ContextLines int
}

// Planner 生成只读的编辑预览。
type Planner struct {
	root         string
	rootResolved string
	checkPath    func(string) (bool, string)
	contextLines int
}

// NewPlanner 创建工作区绑定的 Planner。
func NewPlanner(opts Options) (*Planner, error) {
	if opts.Root == "" {
		return nil, fmt.Errorf("edits: 必须指定工作区根目录")
	}
	abs, err := filepath.Abs(opts.Root)
	if err != nil {
		return nil, fmt.Errorf("edits: 解析工作区根目录失败: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("edits: 解析工作区根目录符号链接失败: %w", err)
	}
	ctxLines := opts.ContextLines
	if ctxLines <= 0 {
		ctxLines = 3
	}
	return &Planner{
		root:         abs,
		rootResolved: resolved,
		checkPath:    opts.CheckPath,
		contextLines: ctxLines,
	}, nil
}

// Root 返回工作区根目录。
func (p *Planner) Root() string { return p.root }

// ResolvePath 把目标路径解析为工作区内的规范绝对路径。
//
// 三道检查缺一不可：
//  1. 词法包含——挡住 `../` 这种不需要文件系统配合的穿越；
//  2. 符号链接解析后的包含——词法检查对"工作区内一个指向 /etc 的软链"完全无效；
//  3. 产品层路径检查——对**调用方给的路径**和**解析后的真实路径**都做一次，
//     否则软链可以把受保护路径藏在合法路径后面。
func (p *Planner) ResolvePath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("edits: 目标路径不能为空")
	}

	abs := path
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(p.root, path)
	}
	abs = filepath.Clean(abs)

	if !containsPath(p.root, abs) {
		return "", fmt.Errorf("%w: %s", ErrOutsideWorkspace, abs)
	}

	// 文件必须已存在：本包只做"改现有文件"的预览。
	info, err := os.Lstat(abs)
	if err != nil {
		return "", fmt.Errorf("edits: 读取目标失败: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("edits: 目标是目录: %s", abs)
	}

	// 解析符号链接：文件本身可能是软链，其父目录也可能在软链路径里。
	resolvedFile, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("edits: 解析符号链接失败: %w", err)
	}
	resolvedDir, err := filepath.EvalSymlinks(filepath.Dir(resolvedFile))
	if err != nil {
		resolvedDir = filepath.Dir(resolvedFile)
	}
	resolved := filepath.Join(resolvedDir, filepath.Base(resolvedFile))

	if !containsPath(p.rootResolved, resolved) {
		return "", fmt.Errorf("%w: %s → %s", ErrSymlinkEscape, abs, resolved)
	}

	if p.checkPath != nil {
		if ok, reason := p.checkPath(path); !ok {
			return "", fmt.Errorf("%w: %s (%s)", ErrPermissionDenied, path, reason)
		}
		if ok, reason := p.checkPath(resolved); !ok {
			return "", fmt.Errorf("%w: %s (%s)", ErrPermissionDenied, resolved, reason)
		}
	}

	return resolved, nil
}

// containsPath 判断 target 是否等于 base 或位于 base 之下。两者都须已 Clean。
func containsPath(base, target string) bool {
	if target == base {
		return true
	}
	return strings.HasPrefix(target, base+string(filepath.Separator))
}

// Preview 生成一次替换的预览，全程不写盘。
//
// old 必须在文件中唯一出现，与同步 edit 工具口径一致：出现 0 次或多次都返回明确错误，
// 而不是挑第一处改掉。
func (p *Planner) Preview(path, old, new string) (*Operation, error) {
	if old == "" {
		return nil, ErrEmptySearch
	}

	abs, err := p.ResolvePath(path)
	if err != nil {
		return nil, err
	}

	raw, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("edits: 读取目标内容失败: %w", err)
	}

	text, hadBOM, err := decodeText(raw)
	if err != nil {
		return nil, err
	}

	newline, body := detectNewline(text)

	// 先按原文精确匹配；换行风格不同时再尝试归一化后匹配，并显式标记。
	search := old
	normalized := false
	if strings.Count(body, search) != 1 {
		if alt := normalizeNewlines(old, newline); alt != search && strings.Count(body, alt) == 1 {
			search = alt
			normalized = true
		}
	}

	switch n := strings.Count(body, search); {
	case n == 0:
		return nil, fmt.Errorf("%w: %q", ErrNoMatch, truncateForError(old))
	case n > 1:
		return nil, fmt.Errorf("%w: %q 出现 %d 次", ErrAmbiguousMatch, truncateForError(old), n)
	}

	replacement := normalizeNewlines(new, newline)
	resultBody := strings.Replace(body, search, replacement, 1)
	resultText := resultBody
	if hadBOM {
		resultText = "\ufeff" + resultBody
	}
	resultBytes := []byte(resultText)

	rel, err := filepath.Rel(p.root, abs)
	if err != nil {
		rel = filepath.Base(abs)
	}

	op := &Operation{
		RequestedPath:    path,
		RelPath:          filepath.ToSlash(rel),
		AbsPath:          abs,
		BaseHash:         hashBytes(raw),
		ResultHash:       hashBytes(resultBytes),
		Old:              old,
		New:              new,
		NewContent:       resultText,
		Diff:             buildDiff(text, search, replacement, newline, p.contextLines),
		Newline:          newline,
		HadBOM:           hadBOM,
		NormalizedSearch: normalized,
	}
	op.ID = deriveOperationID(op)
	return op, nil
}

// ValidateBase 用基线哈希校验文件当前内容是否仍是计划所基于的那一份。
//
// 刻意不看文件时间：cp -p、touch、git checkout 都会改动时间而不改内容，
// 反过来编辑器保存也会在内容未变时更新时间。只有内容哈希能回答"还能不能安全应用"。
func (op *Operation) ValidateBase(current []byte) error {
	got := hashBytes(current)
	if got != op.BaseHash {
		return fmt.Errorf("%w: 计划基线 %s，当前内容 %s", ErrBaseConflict, shortHash(op.BaseHash), shortHash(got))
	}
	return nil
}

// decodeText 处理 UTF-8 BOM 并拒绝非文本内容。
//
// 拒绝二进制是刻意的：把二进制当文本做字符串替换会静默损坏文件，
// 而这类损坏往往要到编译或运行时才暴露。
func decodeText(raw []byte) (text string, hadBOM bool, err error) {
	if !utf8.Valid(raw) {
		return "", false, fmt.Errorf("%w: 含无效 UTF-8 字节", ErrBinaryContent)
	}
	if len(raw) >= 3 && raw[0] == 0xEF && raw[1] == 0xBB && raw[2] == 0xBF {
		return string(raw[3:]), true, nil
	}
	for _, b := range raw {
		if b == 0 {
			return "", false, fmt.Errorf("%w: 含 NUL 字节", ErrBinaryContent)
		}
	}
	return string(raw), false, nil
}

// detectNewline 判定主流换行风格并返回去掉 BOM 后的正文。
func detectNewline(text string) (newline, body string) {
	body = text
	crlf := strings.Count(text, "\r\n")
	lf := strings.Count(text, "\n") - crlf
	if crlf > lf {
		return NewlineCRLF, body
	}
	return NewlineLF, body
}

// normalizeNewlines 把 text 的换行统一为 want。
func normalizeNewlines(text, want string) string {
	unified := strings.ReplaceAll(text, "\r\n", "\n")
	if want == NewlineCRLF {
		return strings.ReplaceAll(unified, "\n", "\r\n")
	}
	return unified
}

// buildDiff 生成围绕替换点的上下文 diff。
//
// 只产出一个 hunk：本次计划就是一处连续替换，伪装成通用多 hunk diff 会让人以为
// 工具做了最小编辑距离优化。行号与 hunk 头由调用方按需补充。
func buildDiff(body, search, replacement, newline string, contextLines int) []DiffLine {
	before, after := splitAround(body, search)

	beforeLines := splitLines(before, newline)
	afterLines := splitLines(after, newline)
	removed := splitLines(search, newline)
	added := splitLines(replacement, newline)

	// 切分会在末尾留下一个空元素（换行是分隔符，不是一行），三个切片都要去掉，
	// 否则 diff 里会多出一行假空行。
	beforeLines = trimTrailingEmpty(beforeLines)
	afterLines = trimTrailingEmpty(afterLines)
	removed = trimTrailingEmpty(removed)
	added = trimTrailingEmpty(added)

	var out []DiffLine
	ctxStart := len(beforeLines) - contextLines
	if ctxStart < 0 {
		ctxStart = 0
	}
	for _, l := range beforeLines[ctxStart:] {
		out = append(out, DiffLine{Kind: LineContext, Text: l})
	}
	for _, l := range removed {
		out = append(out, DiffLine{Kind: LineRemoved, Text: l})
	}
	for _, l := range added {
		out = append(out, DiffLine{Kind: LineAdded, Text: l})
	}
	limit := contextLines
	if len(afterLines) < limit {
		limit = len(afterLines)
	}
	for _, l := range afterLines[:limit] {
		out = append(out, DiffLine{Kind: LineContext, Text: l})
	}
	return out
}

// splitAround 把正文按唯一匹配点切成前后两半。
func splitAround(body, search string) (before, after string) {
	idx := strings.Index(body, search)
	if idx < 0 {
		return body, ""
	}
	return body[:idx], body[idx+len(search):]
}

// splitLines 按指定换行风格切分，保留空行的语义。
func splitLines(s, newline string) []string {
	if s == "" {
		return nil
	}
	sep := "\n"
	if newline == NewlineCRLF {
		sep = "\r\n"
	}
	parts := strings.Split(s, sep)
	return parts
}

// trimTrailingEmpty 去掉因结尾换行切分产生的空字符串元素。
func trimTrailingEmpty(lines []string) []string {
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// deriveOperationID 由计划的关键字段派生稳定 ID。
//
// 用哈希而不是随机数或计数器：同一份计划重复生成必须得到同一个 ID，
// 否则重试会产生"看起来是新操作"的重复条目。
func deriveOperationID(op *Operation) string {
	payload := strings.Join([]string{
		op.RelPath, op.BaseHash, hashString(op.Old), hashString(op.New),
	}, "|")
	return "op-" + hashString(payload)[:16]
}

func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func hashString(s string) string { return hashBytes([]byte(s)) }

func shortHash(h string) string {
	if len(h) <= 12 {
		return h
	}
	return h[:12] + "…"
}

// truncateForError 截短待匹配文本，避免错误信息里塞进整段文件内容。
func truncateForError(s string) string {
	const max = 60
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
