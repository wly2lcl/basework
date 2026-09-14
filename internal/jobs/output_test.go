package jobs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

// memClock 是可推进的时钟，用于验证保留期而不真的等待。
type memClock struct {
	mu  sync.Mutex
	now time.Time
}

func newMemClock() *memClock {
	return &memClock{now: time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)}
}

func (c *memClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *memClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// newOutMgr 复用共享的测试 Manager 构造器，只取 Manager（输出测试不需要终态记录器）。
func newOutMgr(t *testing.T, opts Options) *Manager {
	t.Helper()
	mgr, _ := newTestManager(t, opts)
	return mgr
}

// runEchoJob 启动一个把 payload 原样写到两个流的 job，并等到终态。
func runEchoJob(t *testing.T, mgr *Manager, owner string, stdout, stderr []byte) Job {
	t.Helper()
	job, err := mgr.StartWithOutput(owner, "echo", func(ctx context.Context, out, errOut io.Writer) (int, error) {
		if len(stdout) > 0 {
			if _, werr := out.Write(stdout); werr != nil {
				return -1, werr
			}
		}
		if len(stderr) > 0 {
			if _, werr := errOut.Write(stderr); werr != nil {
				return -1, werr
			}
		}
		return 0, nil
	})
	if err != nil {
		t.Fatalf("启动 job 失败: %v", err)
	}
	final, err := mgr.Await(context.Background(), owner, job.ID)
	if err != nil {
		t.Fatalf("等待终态失败: %v", err)
	}
	return final
}

// readAllPages 按 limit 反复分页读取，返回拼接结果与各页快照。
func readAllPages(t *testing.T, mgr *Manager, owner, id string, stream Stream, limit int64) ([]byte, []OutputPage) {
	t.Helper()
	var (
		got   []byte
		pages []OutputPage
		off   int64
	)
	for i := 0; ; i++ {
		if i > 100000 {
			t.Fatalf("分页读取未在合理次数内结束，已读 %d 字节", len(got))
		}
		page, err := mgr.ReadOutput(owner, id, stream, off, limit)
		if err != nil {
			t.Fatalf("第 %d 页读取失败: %v", i, err)
		}
		pages = append(pages, page)
		if len(page.Data) == 0 {
			break
		}
		got = append(got, page.Data...)
		if page.NextOffset <= off {
			t.Fatalf("第 %d 页 NextOffset 未前进: off=%d next=%d", i, off, page.NextOffset)
		}
		off = page.NextOffset
		if page.EOF {
			break
		}
	}
	return got, pages
}

// readAllPagesConcat 只返回拼接后的字节，内部断言偏移连续。
func readAllPagesConcat(t *testing.T, mgr *Manager, owner, id string, stream Stream, limit int64) []byte {
	t.Helper()
	var got []byte
	var off int64
	for i := 0; ; i++ {
		if i > 100000 {
			t.Fatalf("分页读取未在合理次数内结束")
		}
		page, err := mgr.ReadOutput(owner, id, stream, off, limit)
		if err != nil {
			t.Fatalf("第 %d 页读取失败: %v", i, err)
		}
		if page.Offset != off {
			t.Fatalf("第 %d 页 offset 回显不一致: want %d got %d", i, off, page.Offset)
		}
		if len(page.Data) == 0 {
			break
		}
		got = append(got, page.Data...)
		if page.NextOffset != off+int64(len(page.Data)) {
			t.Fatalf("第 %d 页 NextOffset 与返回长度不一致: next=%d len=%d", i, page.NextOffset, len(page.Data))
		}
		off = page.NextOffset
		if page.EOF {
			break
		}
	}
	return got
}

// TestOutput_LargeVolumeStaysInMemoryBudget 验证大输出不会无限占内存：
// 内存里的字节数不超过 MemoryLimit，其余落在溢出文件里，且总保留量等于写入量。
func TestOutput_LargeVolumeStaysInMemoryBudget(t *testing.T) {
	const memLimit = 32 << 10
	clock := newMemClock()
	mgr := newOutMgr(t, Options{
		Now: clock.Now,
		Output: OutputOptions{
			MemoryLimit: memLimit,
			MaxBytes:    8 << 20,
		},
	})

	payload := bytes.Repeat([]byte("0123456789abcdef"), 64*1024) // 1 MiB
	job := runEchoJob(t, mgr, "sess-A", payload, nil)

	mgr.mu.Lock()
	sink := mgr.jobs[job.ID].out.stdout
	mgr.mu.Unlock()
	sink.mu.Lock()
	memLen := len(sink.mem)
	fileSize := sink.fileSize
	sink.mu.Unlock()

	if int64(memLen) > memLimit {
		t.Fatalf("驻留内存超限: %d > %d", memLen, memLimit)
	}
	if int64(memLen) != memLimit {
		t.Fatalf("内存应恰好填满上限: %d != %d", memLen, memLimit)
	}
	wantFile := int64(len(payload)) - memLimit
	if fileSize != wantFile {
		t.Fatalf("溢出文件大小不符: want %d got %d", wantFile, fileSize)
	}
	if job.StdoutBytes != int64(len(payload)) {
		t.Fatalf("快照总字节不符: want %d got %d", len(payload), job.StdoutBytes)
	}
	if job.OutputTruncated {
		t.Fatalf("未达总配额，不应标记截断: %+v", job)
	}
	if len(job.OutputRefs) != 1 {
		t.Fatalf("应恰好有一个溢出文件引用: %v", job.OutputRefs)
	}
	if _, err := os.Stat(job.OutputRefs[0]); err != nil {
		t.Fatalf("溢出文件引用不可访问: %v", err)
	}
}

// TestOutput_PaginationExactBytes 验证分页不漏不重：任意 page size 拼出的字节等于原文。
func TestOutput_PaginationExactBytes(t *testing.T) {
	clock := newMemClock()
	mgr := newOutMgr(t, Options{Now: clock.Now})

	var sb strings.Builder
	for i := 0; i < 5000; i++ {
		fmt.Fprintf(&sb, "%04d;", i)
	}
	payload := []byte(sb.String())
	job := runEchoJob(t, mgr, "sess-A", payload, nil)

	for _, limit := range []int64{1, 2, 3, 7, 13, 1024, 1 << 20} {
		got := readAllPagesConcat(t, mgr, "sess-A", job.ID, StreamStdout, limit)
		if !bytes.Equal(got, payload) {
			t.Fatalf("limit=%d 分页结果与原文不一致: len got=%d want=%d", limit, len(got), len(payload))
		}
	}
}

// TestOutput_UTF8BoundaryNeverSplitsRune 验证切点永远落在 rune 边界上。
func TestOutput_UTF8BoundaryNeverSplitsRune(t *testing.T) {
	clock := newMemClock()
	mgr := newOutMgr(t, Options{Now: clock.Now})

	payload := []byte(strings.Repeat("中文字符🚀é", 500))
	job := runEchoJob(t, mgr, "sess-A", payload, nil)

	_, pages := readAllPages(t, mgr, "sess-A", job.ID, StreamStdout, 1)
	var joined []byte
	for i, page := range pages {
		if len(page.Data) == 0 {
			continue
		}
		if page.EOF && !utf8.Valid(page.Data) {
			// 末页允许出现不完整序列的唯一情形是被总配额截断，这里没有配额。
			t.Fatalf("末页不是合法 UTF-8: %q", page.Data)
		}
		if !page.EOF && !utf8.Valid(page.Data) {
			t.Fatalf("第 %d 页切在 rune 内部: %q", i, page.Data)
		}
		joined = append(joined, page.Data...)
	}
	if !bytes.Equal(joined, payload) {
		t.Fatalf("UTF-8 分页拼接与原文不一致")
	}
}

// TestOutput_PageGrowsToFitOneRune 验证 limit 小于单个 rune 时不会返回空页（否则调用方死循环）。
func TestOutput_PageGrowsToFitOneRune(t *testing.T) {
	clock := newMemClock()
	mgr := newOutMgr(t, Options{Now: clock.Now, Output: OutputOptions{MaxReadBytes: 1}})
	// MaxReadBytes=1 时 limit 会被抬到 1；内容全是 3 字节 rune。
	job := runEchoJob(t, mgr, "sess-A", []byte("中中中"), nil)

	page, err := mgr.ReadOutput("sess-A", job.ID, StreamStdout, 0, 1)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if len(page.Data) == 0 {
		t.Fatalf("limit 小于 rune 时返回了空页，调用方会原地打转")
	}
	if !utf8.Valid(page.Data) {
		t.Fatalf("返回的字节不是合法 UTF-8: %q", page.Data)
	}
}

// TestOutput_QuotaDropsTailAndReports 验证超过总存储配额时丢弃尾部并如实报告。
func TestOutput_QuotaDropsTailAndReports(t *testing.T) {
	const maxBytes = 1000
	clock := newMemClock()
	mgr := newOutMgr(t, Options{
		Now:    clock.Now,
		Output: OutputOptions{MemoryLimit: 128, MaxBytes: maxBytes},
	})

	payload := bytes.Repeat([]byte("x"), 4000)
	job := runEchoJob(t, mgr, "sess-A", payload, nil)

	page, err := mgr.ReadOutput("sess-A", job.ID, StreamStdout, 0, 4096)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if page.TotalSize != 4000 {
		t.Fatalf("总字节数应为写入量: %d", page.TotalSize)
	}
	if page.StoredSize != maxBytes {
		t.Fatalf("保留字节数应等于配额: %d", page.StoredSize)
	}
	if !page.Truncated || page.DroppedBytes != 4000-maxBytes {
		t.Fatalf("丢弃统计不符: truncated=%v dropped=%d", page.Truncated, page.DroppedBytes)
	}
	if page.DropReason == "" {
		t.Fatalf("丢弃必须带原因说明")
	}
	if len(page.Data) != maxBytes {
		t.Fatalf("可读字节数应等于保留量: %d", len(page.Data))
	}
	if !job.OutputTruncated {
		t.Fatalf("快照应标记截断")
	}
}

// failingSpillFile 在写入 bytesBeforeFailure 之后开始报错，用来模拟磁盘满/写失败。
type failingSpillFile struct {
	path     string
	failAt   int64
	written  int64
	closed   bool
	contents bytes.Buffer
}

func (f *failingSpillFile) Write(p []byte) (int, error) {
	room := f.failAt - f.written
	if room <= 0 {
		return 0, fmt.Errorf("no space left on device")
	}
	take := p
	if int64(len(take)) > room {
		take = take[:room]
	}
	f.written += int64(len(take))
	f.contents.Write(take)
	if int64(len(p)) > room {
		return len(take), fmt.Errorf("no space left on device")
	}
	return len(take), nil
}

func (f *failingSpillFile) ReadAt(p []byte, off int64) (int, error) {
	b := f.contents.Bytes()
	if off >= int64(len(b)) {
		return 0, io.EOF
	}
	n := copy(p, b[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (f *failingSpillFile) Close() error { f.closed = true; return nil }
func (f *failingSpillFile) Path() string { return f.path }

// TestOutput_SpillOpenFailureDegrades 验证溢出文件打不开时不打死 job，而是降级为丢弃并说明原因。
func TestOutput_SpillOpenFailureDegrades(t *testing.T) {
	clock := newMemClock()
	mgr := newOutMgr(t, Options{
		Now: clock.Now,
		Output: OutputOptions{
			MemoryLimit: 16,
			MaxBytes:    1 << 20,
			OpenSpill: func(string) (SpillFile, error) {
				return nil, errors.New("permission denied")
			},
		},
	})

	payload := bytes.Repeat([]byte("y"), 200)
	job := runEchoJob(t, mgr, "sess-A", payload, nil)

	if job.State != StateSucceeded {
		t.Fatalf("写失败不应改变 job 终态: %s (%s)", job.State, job.Err)
	}
	page, err := mgr.ReadOutput("sess-A", job.ID, StreamStdout, 0, 4096)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if !page.Truncated || page.DroppedBytes == 0 {
		t.Fatalf("应记录丢弃: %+v", page)
	}
	if !strings.Contains(page.DropReason, "溢出文件写入失败") {
		t.Fatalf("丢弃原因应说明写失败: %q", page.DropReason)
	}
	if page.StoredSize != 16 {
		t.Fatalf("只有内存部分应保留: %d", page.StoredSize)
	}
	if len(job.OutputRefs) != 0 {
		t.Fatalf("文件没建成，不应给出引用: %v", job.OutputRefs)
	}
}

// TestOutput_SpillWriteFailureMidway 验证写到一半失败：已写部分可读，剩下部分计入丢弃。
func TestOutput_SpillWriteFailureMidway(t *testing.T) {
	clock := newMemClock()
	const failAfter = 64
	mgr := newOutMgr(t, Options{
		Now: clock.Now,
		Output: OutputOptions{
			MemoryLimit: 32,
			MaxBytes:    1 << 20,
			OpenSpill: func(path string) (SpillFile, error) {
				return &failingSpillFile{path: path, failAt: failAfter}, nil
			},
		},
	})

	payload := bytes.Repeat([]byte("z"), 500)
	job := runEchoJob(t, mgr, "sess-A", payload, nil)

	if job.State != StateSucceeded {
		t.Fatalf("job 应正常结束: %s", job.State)
	}
	page, err := mgr.ReadOutput("sess-A", job.ID, StreamStdout, 0, 4096)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if page.StoredSize != 32+failAfter {
		t.Fatalf("已写部分应保留: got %d want %d", page.StoredSize, 32+failAfter)
	}
	if page.DroppedBytes != 500-(32+failAfter) {
		t.Fatalf("丢弃量不符: %d", page.DroppedBytes)
	}
	if !strings.Contains(page.DropReason, "溢出文件写入失败") {
		t.Fatalf("原因应说明写失败: %q", page.DropReason)
	}
}

// TestOutput_UnauthorizedReadRejected 验证越权读取被拒，且错误与「不存在」不可区分。
func TestOutput_UnauthorizedReadRejected(t *testing.T) {
	clock := newMemClock()
	mgr := newOutMgr(t, Options{Now: clock.Now})
	job := runEchoJob(t, mgr, "sess-A", []byte("secret"), nil)

	if _, err := mgr.ReadOutput("sess-B", job.ID, StreamStdout, 0, 1024); !errors.Is(err, ErrNotFound) {
		t.Fatalf("越权读取应返回 ErrNotFound，实际 %v", err)
	}
	if _, err := mgr.ReadOutput("sess-A", "job-999", StreamStdout, 0, 1024); !errors.Is(err, ErrNotFound) {
		t.Fatalf("不存在的 job 应返回 ErrNotFound，实际 %v", err)
	}
	if _, err := mgr.ReadOutput("", job.ID, StreamStdout, 0, 1024); !errors.Is(err, ErrMissingOwner) {
		t.Fatalf("缺 owner 应返回 ErrMissingOwner，实际 %v", err)
	}
	if _, err := mgr.ReadOutput("sess-A", job.ID, Stream("sideways"), 0, 1024); !errors.Is(err, ErrBadStream) {
		t.Fatalf("未知流应返回 ErrBadStream，实际 %v", err)
	}
	if _, err := mgr.ReadOutput("sess-A", job.ID, StreamStdout, -1, 1024); !errors.Is(err, ErrBadOffset) {
		t.Fatalf("负偏移应返回 ErrBadOffset，实际 %v", err)
	}
	// 越权与不存在必须无法区分：错误值相同。
	_, e1 := mgr.ReadOutput("sess-B", job.ID, StreamStdout, 0, 1024)
	_, e2 := mgr.ReadOutput("sess-C", job.ID, StreamStdout, 0, 1024)
	if e1 != e2 {
		t.Fatalf("不同越权者的错误不可区分: %v vs %v", e1, e2)
	}
}

// TestOutput_ExpiryAndCleanup 验证保留期到期后的明确说明与文件清理。
func TestOutput_ExpiryAndCleanup(t *testing.T) {
	const retention = time.Minute
	clock := newMemClock()
	mgr := newOutMgr(t, Options{
		Now: clock.Now,
		Output: OutputOptions{
			MemoryLimit: 8,
			MaxBytes:    1 << 20,
			Retention:   retention,
		},
	})

	job := runEchoJob(t, mgr, "sess-A", bytes.Repeat([]byte("q"), 200), nil)
	if len(job.OutputRefs) != 1 {
		t.Fatalf("应先产生溢出文件: %v", job.OutputRefs)
	}
	path := job.OutputRefs[0]
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("溢出文件应存在: %v", err)
	}

	// 未过期时可正常读取。
	if _, err := mgr.ReadOutput("sess-A", job.ID, StreamStdout, 0, 16); err != nil {
		t.Fatalf("未过期读取应成功: %v", err)
	}

	clock.Advance(retention + time.Second)
	page, err := mgr.ReadOutput("sess-A", job.ID, StreamStdout, 0, 16)
	if !errors.Is(err, ErrOutputExpired) {
		t.Fatalf("过期读取应返回 ErrOutputExpired，实际 %v", err)
	}
	if !page.Expired || !page.EOF {
		t.Fatalf("过期页应标记 Expired 且 EOF: %+v", page)
	}
	if page.ExpiresAt.IsZero() {
		t.Fatalf("过期页应告知截止时刻")
	}

	removed, freed := mgr.CleanupOutputs()
	// 只有 stdout 发生溢出，所以只有一个文件可删。
	if removed != 1 {
		t.Fatalf("应清理一个溢出文件，实际 %d", removed)
	}
	if freed == 0 {
		t.Fatalf("应报告释放的字节数")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("过期文件应被删除: %v", err)
	}
	// 再次清理应无事可做（幂等）。
	if again, _ := mgr.CleanupOutputs(); again != 0 {
		t.Fatalf("重复清理应为空操作，实际 %d", again)
	}
}

// TestOutput_CloseRemovesFiles 验证关闭时清理文件，且此后读取给出明确状态。
func TestOutput_CloseRemovesFiles(t *testing.T) {
	clock := newMemClock()
	mgr := New(Options{Now: clock.Now, Output: OutputOptions{Dir: t.TempDir(), MemoryLimit: 4}})
	job := runEchoJob(t, mgr, "sess-A", bytes.Repeat([]byte("c"), 128), nil)
	paths := job.OutputRefs
	if len(paths) == 0 {
		t.Fatalf("应先产生溢出文件")
	}
	mgr.Close()
	for _, p := range paths {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("关闭后文件应被删除: %s (%v)", p, err)
		}
	}
	if _, err := mgr.ReadOutput("sess-A", job.ID, StreamStdout, 0, 8); !errors.Is(err, ErrOutputClosed) {
		t.Fatalf("关闭后读取应返回 ErrOutputClosed，实际 %v", err)
	}
}

// TestOutput_OffsetBeyondEndIsEOF 验证越过末尾的偏移返回 EOF 而不是报错或重复数据。
func TestOutput_OffsetBeyondEndIsEOF(t *testing.T) {
	clock := newMemClock()
	mgr := newOutMgr(t, Options{Now: clock.Now})
	job := runEchoJob(t, mgr, "sess-A", []byte("hello"), nil)

	for _, off := range []int64{5, 6, 1 << 30} {
		page, err := mgr.ReadOutput("sess-A", job.ID, StreamStdout, off, 16)
		if err != nil {
			t.Fatalf("offset=%d 读取失败: %v", off, err)
		}
		if len(page.Data) != 0 || !page.EOF {
			t.Fatalf("offset=%d 应为空且 EOF: %+v", off, page)
		}
	}
}

// TestOutput_StreamsAreSeparate 验证两个流的偏移与内容互不干扰。
func TestOutput_StreamsAreSeparate(t *testing.T) {
	clock := newMemClock()
	mgr := newOutMgr(t, Options{Now: clock.Now})
	job := runEchoJob(t, mgr, "sess-A", []byte("OUT"), []byte("ERR-out"))

	out := readAllPagesConcat(t, mgr, "sess-A", job.ID, StreamStdout, 2)
	errOut := readAllPagesConcat(t, mgr, "sess-A", job.ID, StreamStderr, 3)
	if string(out) != "OUT" {
		t.Fatalf("stdout 不符: %q", out)
	}
	if string(errOut) != "ERR-out" {
		t.Fatalf("stderr 不符: %q", errOut)
	}
	if job.StdoutBytes != 3 || job.StderrBytes != 7 {
		t.Fatalf("两个流应分别计数: %d / %d", job.StdoutBytes, job.StderrBytes)
	}
}

// TestOutput_ConcurrentWriteAndRead 在 -race 下验证读写并发安全。
func TestOutput_ConcurrentWriteAndRead(t *testing.T) {
	clock := newMemClock()
	mgr := newOutMgr(t, Options{
		Now:    clock.Now,
		Output: OutputOptions{MemoryLimit: 64, MaxBytes: 1 << 20},
	})

	const chunks, chunkSize = 200, 64
	job, err := mgr.StartWithOutput("sess-A", "chatter", func(ctx context.Context, out, _ io.Writer) (int, error) {
		for i := 0; i < chunks; i++ {
			out.Write(bytes.Repeat([]byte{byte('a' + i%26)}, chunkSize))
		}
		return 0, nil
	})
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			if _, err := mgr.ReadOutput("sess-A", job.ID, StreamStdout, 0, 128); err != nil {
				t.Errorf("并发读取失败: %v", err)
				return
			}
		}
	}()

	if _, err := mgr.Await(context.Background(), "sess-A", job.ID); err != nil {
		t.Fatalf("等待终态失败: %v", err)
	}
	close(stop)
	wg.Wait()

	got := readAllPagesConcat(t, mgr, "sess-A", job.ID, StreamStdout, 333)
	if len(got) != chunks*chunkSize {
		t.Fatalf("并发场景下分页丢失字节: got %d want %d", len(got), chunks*chunkSize)
	}
}

// TestOutput_OwnerDirIsNotTraversable 验证 owner 不能靠 ../ 把文件写到输出目录之外。
func TestOutput_OwnerDirIsNotTraversable(t *testing.T) {
	root := t.TempDir()
	clock := newMemClock()
	mgr := New(Options{Now: clock.Now, Output: OutputOptions{Dir: root, MemoryLimit: 4}})
	// 溢出文件句柄要等 Close/CloseOwner 才释放；不关的话 Windows 上
	// t.TempDir 的清理会因「文件被占用」失败（Unix 允许删除打开中的文件）。
	t.Cleanup(mgr.Close)
	job := runEchoJob(t, mgr, "../../etc/passwd", bytes.Repeat([]byte("t"), 64), nil)

	if len(job.OutputRefs) != 1 {
		t.Fatalf("应产生溢出引用: %v", job.OutputRefs)
	}
	rel, err := filepath.Rel(root, job.OutputRefs[0])
	if err != nil {
		t.Fatalf("引用应在输出根目录内: %v", err)
	}
	if strings.HasPrefix(rel, "..") {
		t.Fatalf("owner 目录名可穿越: %s", rel)
	}
}

// TestOutput_StartWithoutSinksStillReadable 验证用 Start 启动的 job 也能读取（内容为空而非报错）。
func TestOutput_StartWithoutSinksStillReadable(t *testing.T) {
	clock := newMemClock()
	mgr := newOutMgr(t, Options{Now: clock.Now})
	job, err := mgr.Start("sess-A", "noop", func(ctx context.Context) (int, error) { return 0, nil })
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if _, err := mgr.Await(context.Background(), "sess-A", job.ID); err != nil {
		t.Fatalf("等待终态失败: %v", err)
	}
	page, err := mgr.ReadOutput("sess-A", job.ID, StreamStdout, 0, 16)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if !page.EOF || page.TotalSize != 0 {
		t.Fatalf("没有输出时应为空且 EOF: %+v", page)
	}
}

// TestOutput_DefaultOptionsAreBounded 验证零值配置也会得到有界的内存与存储配额。
func TestOutput_DefaultOptionsAreBounded(t *testing.T) {
	clock := newMemClock()
	mgr := New(Options{Now: clock.Now, Output: OutputOptions{Dir: t.TempDir()}})
	if mgr.opts.Output.MemoryLimit != DefaultMemoryLimit {
		t.Fatalf("默认内存上限未生效: %d", mgr.opts.Output.MemoryLimit)
	}
	if mgr.opts.Output.MaxBytes != DefaultMaxBytes {
		t.Fatalf("默认存储上限未生效: %d", mgr.opts.Output.MaxBytes)
	}
	if mgr.opts.Output.MaxReadBytes != DefaultMaxReadBytes {
		t.Fatalf("默认单次读取上限未生效: %d", mgr.opts.Output.MaxReadBytes)
	}
	if mgr.opts.Output.Retention != DefaultRetention {
		t.Fatalf("默认保留期未生效: %v", mgr.opts.Output.Retention)
	}
	if mgr.opts.Output.Now == nil || mgr.opts.Output.OpenSpill == nil {
		t.Fatalf("默认时钟与文件打开函数应被补齐")
	}
	mgr.Close()
}

// TestAlignCut 直接覆盖 UTF-8 边界对齐的边界情形。
func TestAlignCut(t *testing.T) {
	// "中" = E4 B8 AD，"a" = 61
	buf := []byte{'a', 0xE4, 0xB8, 0xAD, 'b'}
	if got := alignCut(buf, 1); got != 1 {
		t.Fatalf("alignCut(limit=1) = %d, want 1", got)
	}
	// buf[2]=0xB8 是续字节 → 回退到 rune 起点 1。
	if got := alignCut(buf, 2); got != 1 {
		t.Fatalf("alignCut(limit=2) = %d, want 1", got)
	}
	if got := alignCut(buf, 3); got != 1 {
		t.Fatalf("alignCut(limit=3) = %d, want 1", got)
	}
	if got := alignCut(buf, 4); got != 4 {
		t.Fatalf("alignCut(limit=4) = %d, want 4", got)
	}
	if got := alignCut(buf, 5); got != 5 {
		t.Fatalf("alignCut(limit=5) = %d, want 5", got)
	}
	// limit=0 落在首个 rune 内部：至少取满一个 rune，否则调用方原地打转。
	if got := alignCut(buf, 0); got != 1 {
		t.Fatalf("alignCut(limit=0) = %d, want 1", got)
	}
	// 非法 UTF-8 也必须终止且不越界。
	bad := []byte{0xFF, 0xFE, 0xFD}
	if got := alignCut(bad, 1); got < 1 || got > len(bad) {
		t.Fatalf("非法 UTF-8 对齐越界: %d", got)
	}
}
