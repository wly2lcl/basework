package edits

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newPlanner 在一个临时工作区里建立 Planner。
func newPlanner(t *testing.T, checkPath func(string) (bool, string)) (*Planner, string) {
	t.Helper()
	root := t.TempDir()
	// macOS 的 TempDir 可能位于 /var（指向 /private/var），根目录必须解析后再比较，
	// 否则工作区边界检查会被软链本身误判。
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("解析临时目录失败: %v", err)
	}
	p, err := NewPlanner(Options{Root: resolved, CheckPath: checkPath})
	if err != nil {
		t.Fatalf("NewPlanner: %v", err)
	}
	return p, resolved
}

// writeFile 在工作区里写入文件并返回绝对路径。
func writeFile(t *testing.T, root, rel, content string) string {
	t.Helper()
	abs := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatalf("写入文件失败: %v", err)
	}
	return abs
}

// snapshotDir 记录目录下所有文件路径与内容，用于断言"预览没写盘"。
func snapshotDir(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if info.IsDir() {
			out[rel+"/"] = "<dir>"
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		out[rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("遍历目录失败: %v", err)
	}
	return out
}

func equalSnapshots(a, b map[string]string) (string, bool) {
	for k, v := range a {
		other, ok := b[k]
		if !ok {
			return "多出文件: " + k, false
		}
		if other != v {
			return "文件内容被改动: " + k, false
		}
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			return "文件被删除: " + k, false
		}
	}
	return "", true
}

// TestPreview_DoesNotWriteAnything 是本任务最核心的验收：预览不会改文件。
func TestPreview_DoesNotWriteAnything(t *testing.T) {
	p, root := newPlanner(t, nil)
	writeFile(t, root, "a.txt", "line1\nline2\nline3\n")
	writeFile(t, root, "sub/b.txt", "other\n")

	before := snapshotDir(t, root)
	infoBefore, err := os.Stat(filepath.Join(root, "a.txt"))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	op, err := p.Preview("a.txt", "line2", "LINE-TWO")
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if op.NewContent == "" {
		t.Fatal("计划应包含变更后内容")
	}

	after := snapshotDir(t, root)
	if reason, ok := equalSnapshots(before, after); !ok {
		t.Fatalf("预览改动了工作区: %s", reason)
	}
	infoAfter, err := os.Stat(filepath.Join(root, "a.txt"))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !infoAfter.ModTime().Equal(infoBefore.ModTime()) {
		t.Errorf("预览改动了文件时间: %v → %v", infoBefore.ModTime(), infoAfter.ModTime())
	}
}

// TestPreview_BaselineHashAndStableOperationID 验证基线哈希与 operation ID。
func TestPreview_BaselineHashAndStableOperationID(t *testing.T) {
	p, root := newPlanner(t, nil)
	original := "alpha\nbeta\n"
	writeFile(t, root, "f.txt", original)

	op, err := p.Preview("f.txt", "beta", "gamma")
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	sum := sha256.Sum256([]byte(original))
	if op.BaseHash != hex.EncodeToString(sum[:]) {
		t.Errorf("BaseHash 应为原文件内容的 sha256，得到 %s", op.BaseHash)
	}
	resultSum := sha256.Sum256([]byte(op.NewContent))
	if op.ResultHash != hex.EncodeToString(resultSum[:]) {
		t.Errorf("ResultHash 应为变更后内容的 sha256，得到 %s", op.ResultHash)
	}

	op2, err := p.Preview("f.txt", "beta", "gamma")
	if err != nil {
		t.Fatalf("第二次 Preview: %v", err)
	}
	if op.ID != op2.ID {
		t.Errorf("同一份计划应得到同一个 ID：%s vs %s", op.ID, op2.ID)
	}
	if !strings.HasPrefix(op.ID, "op-") {
		t.Errorf("ID 前缀应为 op-，得到 %s", op.ID)
	}
	// 不同替换内容必须得到不同 ID
	op3, err := p.Preview("f.txt", "beta", "delta")
	if err != nil {
		t.Fatalf("第三次 Preview: %v", err)
	}
	if op3.ID == op.ID {
		t.Error("不同替换内容不应复用同一个 operation ID")
	}
}

// TestResolvePath_RejectsTraversal 验证 ../ 穿越被拒。
func TestResolvePath_RejectsTraversal(t *testing.T) {
	p, root := newPlanner(t, nil)
	outside := filepath.Join(filepath.Dir(root), "outside.txt")
	if err := os.WriteFile(outside, []byte("x\n"), 0o644); err != nil {
		t.Fatalf("写入外部文件失败: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(outside) })

	for _, bad := range []string{"../outside.txt", "sub/../../outside.txt"} {
		if _, err := p.Preview(bad, "x", "y"); !errors.Is(err, ErrOutsideWorkspace) {
			t.Errorf("%q 应被判定为超出工作区，得到 %v", bad, err)
		}
	}
}

// TestResolvePath_RejectsSymlinkEscape 验证软链逃逸被拒。
//
// 这条是词法检查挡不住的：`inside.txt` 完全落在工作区内，但它是 /etc/hosts 的软链。
func TestResolvePath_RejectsSymlinkEscape(t *testing.T) {
	p, root := newPlanner(t, nil)
	target := filepath.Join(filepath.Dir(root), "secret.txt")
	if err := os.WriteFile(target, []byte("secret\n"), 0o644); err != nil {
		t.Fatalf("写入外部文件失败: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(target) })

	link := filepath.Join(root, "inside.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("当前环境不支持创建符号链接: %v", err)
	}

	if _, err := p.Preview("inside.txt", "secret", "leaked"); !errors.Is(err, ErrSymlinkEscape) {
		t.Fatalf("软链逃逸应被拒绝，得到 %v", err)
	}
}

// TestResolvePath_AllowsSymlinkInsideWorkspace 验证工作区内部的软链可用。
func TestResolvePath_AllowsSymlinkInsideWorkspace(t *testing.T) {
	p, root := newPlanner(t, nil)
	writeFile(t, root, "real/target.txt", "hello\n")
	link := filepath.Join(root, "link.txt")
	if err := os.Symlink(filepath.Join(root, "real", "target.txt"), link); err != nil {
		t.Skipf("当前环境不支持创建符号链接: %v", err)
	}

	op, err := p.Preview("link.txt", "hello", "world")
	if err != nil {
		t.Fatalf("工作区内软链应可用，得到 %v", err)
	}
	if op.RequestedPath != "link.txt" {
		t.Errorf("RequestedPath 应保留调用方原样路径，得到 %q", op.RequestedPath)
	}
	// RelPath 指向真实文件：写入作用在目标上，报告链接会误导读者。
	if op.RelPath != "real/target.txt" {
		t.Errorf("RelPath 应指向解析后的真实文件，得到 %q", op.RelPath)
	}
}

// TestPreview_PermissionCheckAppliesToEveryTarget 验证权限入口对原始路径与解析路径都生效。
func TestPreview_PermissionCheckAppliesToEveryTarget(t *testing.T) {
	p, root := newPlanner(t, func(path string) (bool, string) {
		if strings.Contains(path, "protected") {
			return false, "受保护路径"
		}
		return true, ""
	})
	writeFile(t, root, "protected/f.txt", "secret\n")

	if _, err := p.Preview("protected/f.txt", "secret", "x"); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("权限拒绝应返回 ErrPermissionDenied，得到 %v", err)
	} else if !strings.Contains(err.Error(), "受保护路径") {
		t.Errorf("错误信息应带拒绝原因，得到 %v", err)
	}
}

// TestPreview_RejectsAmbiguousAndMissingMatch 验证匹配唯一性口径与同步 edit 一致。
func TestPreview_RejectsAmbiguousAndMissingMatch(t *testing.T) {
	p, root := newPlanner(t, nil)
	writeFile(t, root, "dup.txt", "same\nsame\n")

	if _, err := p.Preview("dup.txt", "same", "x"); !errors.Is(err, ErrAmbiguousMatch) {
		t.Errorf("多次匹配应返回 ErrAmbiguousMatch，得到 %v", err)
	}
	if _, err := p.Preview("dup.txt", "nope", "x"); !errors.Is(err, ErrNoMatch) {
		t.Errorf("无匹配应返回 ErrNoMatch，得到 %v", err)
	}
	if _, err := p.Preview("dup.txt", "", "x"); !errors.Is(err, ErrEmptySearch) {
		t.Errorf("空搜索串应返回 ErrEmptySearch，得到 %v", err)
	}
}

// TestPreview_PreservesCRLF 验证换行风格被保留，不会把整个文件换成 LF。
func TestPreview_PreservesCRLF(t *testing.T) {
	p, root := newPlanner(t, nil)
	writeFile(t, root, "crlf.txt", "a\r\nb\r\nc\r\n")

	op, err := p.Preview("crlf.txt", "b", "B")
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if op.Newline != NewlineCRLF {
		t.Fatalf("应识别为 crlf，得到 %s", op.Newline)
	}
	if strings.Contains(strings.ReplaceAll(op.NewContent, "\r\n", ""), "\n") {
		t.Errorf("变更后内容仍是统一 CRLF，不应出现孤立 LF: %q", op.NewContent)
	}
	if op.NewContent != "a\r\nB\r\nc\r\n" {
		t.Errorf("内容不正确: %q", op.NewContent)
	}
}

// TestPreview_NormalizesSearchNewlines 验证调用方用 LF 描述 CRLF 文件时能匹配，
// 且这件事被显式标记，不做静默改写。
func TestPreview_NormalizesSearchNewlines(t *testing.T) {
	p, root := newPlanner(t, nil)
	writeFile(t, root, "crlf.txt", "a\r\nb\r\nc\r\n")

	op, err := p.Preview("crlf.txt", "a\nb", "a\nB")
	if err != nil {
		t.Fatalf("应能匹配归一化后的文本，得到 %v", err)
	}
	if !op.NormalizedSearch {
		t.Error("归一化匹配必须被标记，不能让调用方以为用的就是自己给的字节")
	}
	if op.NewContent != "a\r\nB\r\nc\r\n" {
		t.Errorf("内容不正确: %q", op.NewContent)
	}
}

// TestPreview_PreservesBOM 验证 UTF-8 BOM 被保留。
func TestPreview_PreservesBOM(t *testing.T) {
	p, root := newPlanner(t, nil)
	bom := "\ufeff"
	writeFile(t, root, "bom.txt", bom+"hello\nworld\n")

	op, err := p.Preview("bom.txt", "world", "WORLD")
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if !op.HadBOM {
		t.Fatal("应识别出 BOM")
	}
	if !strings.HasPrefix(op.NewContent, bom) {
		t.Errorf("BOM 必须保留，实际内容以 %q 开头", op.NewContent)
	}
	// 基线哈希基于原始字节（含 BOM），否则冲突校验会误判。
	sum := sha256.Sum256([]byte(bom + "hello\nworld\n"))
	if op.BaseHash != hex.EncodeToString(sum[:]) {
		t.Error("BaseHash 应基于含 BOM 的原始字节")
	}
}

// TestPreview_RejectsBinaryContent 验证二进制文件被明确拒绝而不是静默损坏。
func TestPreview_RejectsBinaryContent(t *testing.T) {
	p, root := newPlanner(t, nil)
	writeFile(t, root, "bin.dat", "abc")

	// 无效 UTF-8
	invalid := filepath.Join(root, "invalid.txt")
	if err := os.WriteFile(invalid, []byte{0xff, 0xfe, 'a'}, 0o644); err != nil {
		t.Fatalf("写入文件失败: %v", err)
	}
	if _, err := p.Preview("invalid.txt", "a", "b"); !errors.Is(err, ErrBinaryContent) {
		t.Errorf("无效 UTF-8 应被拒绝，得到 %v", err)
	}

	// 含 NUL
	nul := filepath.Join(root, "nul.txt")
	if err := os.WriteFile(nul, []byte("a\x00b"), 0o644); err != nil {
		t.Fatalf("写入文件失败: %v", err)
	}
	if _, err := p.Preview("nul.txt", "a", "b"); !errors.Is(err, ErrBinaryContent) {
		t.Errorf("含 NUL 的内容应被拒绝，得到 %v", err)
	}
}

// TestValidateBase_UsesContentNotModTime 验证冲突校验看内容哈希而非文件时间。
func TestValidateBase_UsesContentNotModTime(t *testing.T) {
	p, root := newPlanner(t, nil)
	abs := writeFile(t, root, "f.txt", "stable\n")

	op, err := p.Preview("f.txt", "stable", "changed")
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	original, err := os.ReadFile(abs)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if err := op.ValidateBase(original); err != nil {
		t.Fatalf("基线未变时校验应通过: %v", err)
	}

	// 内容不变但时间变化：必须仍然通过（例如 cp -p / touch / checkout）。
	future := time.Now().Add(2 * time.Hour)
	if err := os.Chtimes(abs, future, future); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
	after, err := os.ReadFile(abs)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if err := op.ValidateBase(after); err != nil {
		t.Errorf("仅时间变化不应被判为冲突: %v", err)
	}

	// 内容真的变了：必须报冲突，并带上两个哈希
	if err := os.WriteFile(abs, []byte("user edited\n"), 0o644); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	edited, err := os.ReadFile(abs)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	err = op.ValidateBase(edited)
	if !errors.Is(err, ErrBaseConflict) {
		t.Fatalf("内容变化应返回 ErrBaseConflict，得到 %v", err)
	}
	if !strings.Contains(err.Error(), "计划基线") {
		t.Errorf("冲突错误应说明期望与实际: %v", err)
	}
}

// TestPreview_DiffIsScopedToOneHunk 验证 diff 只围绕替换点，不是全文件。
func TestPreview_DiffIsScopedToOneHunk(t *testing.T) {
	p, root := newPlanner(t, nil)
	var b strings.Builder
	for i := 1; i <= 20; i++ {
		b.WriteString("line")
		if i < 10 {
			b.WriteString("0")
		}
		b.WriteString(itoa(i))
		b.WriteString("\n")
	}
	writeFile(t, root, "big.txt", b.String())

	op, err := p.Preview("big.txt", "line10", "LINE-TEN")
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	var removed, added, context int
	for _, line := range op.Diff {
		switch line.Kind {
		case LineRemoved:
			removed++
			if line.Text != "line10" {
				t.Errorf("删除行应为 line10，得到 %q", line.Text)
			}
		case LineAdded:
			added++
			if line.Text != "LINE-TEN" {
				t.Errorf("新增行应为 LINE-TEN，得到 %q", line.Text)
			}
		default:
			context++
		}
	}
	if removed != 1 || added != 1 {
		t.Errorf("应各有 1 行增删，得到 删 %d 增 %d", removed, added)
	}
	if context > 6 {
		t.Errorf("上下文行数应受 ContextLines=3 限制，得到 %d", context)
	}
	if len(op.Diff) >= 20 {
		t.Errorf("diff 不应包含整个文件，得到 %d 行", len(op.Diff))
	}
}

// TestPreview_PathCannotEscapeThroughSubdirSymlink 验证父目录是软链时的逃逸也被挡住。
func TestPreview_PathCannotEscapeThroughSubdirSymlink(t *testing.T) {
	p, root := newPlanner(t, nil)
	outsideDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outsideDir, "f.txt"), []byte("data\n"), 0o644); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(root, "esc")); err != nil {
		t.Skipf("当前环境不支持创建符号链接: %v", err)
	}

	if _, err := p.Preview("esc/f.txt", "data", "x"); !errors.Is(err, ErrSymlinkEscape) {
		t.Fatalf("经父目录软链逃逸应被拒绝，得到 %v", err)
	}
}

// itoa 避免为测试引入格式化依赖。
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var digits []byte
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return string(digits)
}
