// Package jobs —— 输出存储：内存配额、溢出到私有文件、分页读取、保留期。
//
// 设计取舍说明（为什么不是「全都写文件」或「全都留在内存」）：
//
//   - 只留内存：一条 `yes` 就能把进程撑爆。所以必须有溢出。
//   - 全都写文件：每条 10 字节的命令也产生一次 open/write/close 和一份 inode，
//     且读取要走磁盘。所以先驻留内存，越过阈值才落盘。
//
// 于是每个流有两条界线：MemoryLimit（驻留内存上限）与 MaxBytes（总存储上限）。
// 越过 MaxBytes 之后仍然继续"吞"数据并返回成功，只是把多出来的字节记为丢弃——
// 因为对子进程返回 EPIPE 会把它打死，而"输出太多"不该表现为"命令失败了"。
// 丢弃的字节数与原因会出现在每一次分页读取的返回值里，不会被悄悄抹掉。
//
// UTF-8 边界：分页的切点只会落在 rune 起点上。切点落在 rune 内部时向前回退到最近的
// rune 起点；若 limit 小到连一个 rune 都放不下，则向前扩到一个完整 rune。回退而不是
// 前扩是默认策略，因为回退时那几个字节尚未返回，下一页会重新读到，既不漏也不重。
package jobs

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unicode/utf8"
)

// Stream 标识 job 的一个输出流。
type Stream string

const (
	// StreamStdout 是标准输出。
	StreamStdout Stream = "stdout"
	// StreamStderr 是标准错误。
	StreamStderr Stream = "stderr"
)

// Valid 报告流名是否可识别。未知流名不会被当成 stdout 处理。
func (s Stream) Valid() bool {
	return s == StreamStdout || s == StreamStderr
}

// 输出配额与保留期的默认值。全部可被 OutputOptions 覆盖。
const (
	// DefaultMemoryLimit 是每个流驻留内存的上限。
	DefaultMemoryLimit int64 = 256 << 10
	// DefaultMaxBytes 是每个流内存+文件的总存储上限。
	DefaultMaxBytes int64 = 64 << 20
	// DefaultMaxReadBytes 是单次分页读取返回的字节上限。
	DefaultMaxReadBytes int64 = 64 << 10
	// DefaultRetention 是输出文件的保留期。
	DefaultRetention = 24 * time.Hour
)

// 输出相关的可判定错误。
var (
	// ErrOutputExpired 表示输出已超过保留期。它同时意味着文件可能已被清理。
	ErrOutputExpired = errors.New("jobs: 输出已过期")
	// ErrOutputClosed 表示输出已随 Manager 关闭，不再接受写入。
	ErrOutputClosed = errors.New("jobs: 输出已关闭")
	// ErrOutputUnavailable 表示内部状态不一致：记录到了需要读文件的长度，
	// 却没有可读的文件句柄。正常路径不可达，出现即表示缺陷。
	ErrOutputUnavailable = errors.New("jobs: 输出记录不可用")
	// ErrBadStream 表示流名未知。
	ErrBadStream = errors.New("jobs: 未知的输出流")
	// ErrBadOffset 表示 offset 为负数。
	ErrBadOffset = errors.New("jobs: 输出偏移不能为负")
)

// SpillFile 是单个溢出文件的读写句柄。
//
// 把它做成接口而不是直接用 *os.File，是为了能在测试里注入"磁盘满"和"写到一半失败"，
// 否则这两条验收只能靠祈祷。
type SpillFile interface {
	io.Writer
	io.ReaderAt
	io.Closer
	// Path 返回该文件的私有路径，用于给出可检索引用。
	Path() string
}

// SpillOpener 打开一个新的溢出文件。默认实现创建 0600 私有文件。
type SpillOpener func(path string) (SpillFile, error)

// OutputOptions 配置输出存储。
type OutputOptions struct {
	// MemoryLimit 是每个流驻留内存的字节上限；<=0 用 DefaultMemoryLimit。
	MemoryLimit int64
	// MaxBytes 是每个流（内存+文件）的总存储上限；<=0 用 DefaultMaxBytes。
	MaxBytes int64
	// MaxReadBytes 是单次读取返回的字节上限；<=0 用 DefaultMaxReadBytes。
	MaxReadBytes int64
	// Retention 是输出保留期；<=0 用 DefaultRetention。
	Retention time.Duration
	// Dir 是溢出文件目录；为空用用户缓存目录下的 basework/job-output。
	Dir string
	// Now 可注入时钟；为空时继承 Manager 的时钟。
	Now func() time.Time
	// OpenSpill 可注入文件打开函数；为空用 0600 私有文件的默认实现。
	OpenSpill SpillOpener
}

func (o OutputOptions) normalized(now func() time.Time) OutputOptions {
	if o.MemoryLimit <= 0 {
		o.MemoryLimit = DefaultMemoryLimit
	}
	if o.MaxBytes <= 0 {
		o.MaxBytes = DefaultMaxBytes
	}
	if o.MaxReadBytes <= 0 {
		o.MaxReadBytes = DefaultMaxReadBytes
	}
	if o.Retention <= 0 {
		o.Retention = DefaultRetention
	}
	if o.Dir == "" {
		o.Dir = defaultOutputDir()
	}
	if o.Now == nil {
		o.Now = now
	}
	if o.OpenSpill == nil {
		o.OpenSpill = openPrivateFile
	}
	return o
}

// defaultOutputDir 返回溢出文件的默认目录。
func defaultOutputDir() string {
	if dir, err := os.UserCacheDir(); err == nil && dir != "" {
		return filepath.Join(dir, "basework", "job-output")
	}
	return filepath.Join(os.TempDir(), "basework-job-output")
}

// openPrivateFile 以 0600 创建文件，目录 0700。
//
// 用 O_RDWR 而不是 O_WRONLY：分页读取要在同一个句柄上做 ReadAt（pread），
// 只写句柄读回时会得到 EBADF。
func openPrivateFile(path string) (SpillFile, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	return &osSpillFile{File: f}, nil
}

// osSpillFile 让 *os.File 满足 SpillFile。
type osSpillFile struct{ *os.File }

func (f *osSpillFile) Path() string { return f.Name() }

// ownerDirName 把 owner 映射成文件系统安全的目录名。
//
// 直接拼接 owner 会被 `../` 和路径分隔符穿出去，所以做一次摘要。
func ownerDirName(owner string) string {
	sum := sha256.Sum256([]byte(owner))
	return hex.EncodeToString(sum[:8])
}

// OutputPage 是一次分页读取的结果。
type OutputPage struct {
	Stream Stream `json:"stream"`
	// Offset 是本次请求的起始偏移。
	Offset int64 `json:"offset"`
	// Data 是本次返回的字节，长度不会超过 MaxReadBytes，且切点在 UTF-8 边界上。
	Data []byte `json:"-"`
	// Text 是 Data 的字符串形式，便于直接放进工具结果。
	Text string `json:"text"`
	// NextOffset 是下一次读取应传入的偏移。Data 为空且 EOF 为真时表示读到末尾。
	NextOffset int64 `json:"next_offset"`
	// TotalSize 是该流已接收的总字节数（含被丢弃的）。
	TotalSize int64 `json:"total_size"`
	// StoredSize 是实际保留、可读取的字节数。TotalSize-StoredSize 即丢弃量。
	StoredSize int64 `json:"stored_size"`
	// Truncated 表示有字节因配额或写失败被丢弃。
	Truncated bool `json:"truncated"`
	// DroppedBytes 是被丢弃的字节数。
	DroppedBytes int64 `json:"dropped_bytes"`
	// DropReason 是首个丢弃原因，未丢弃时为空。
	DropReason string `json:"drop_reason,omitempty"`
	// EOF 表示 NextOffset 之后没有更多可读数据。
	EOF bool `json:"eof"`
	// Expired 表示该输出已超过保留期。
	Expired bool `json:"expired"`
	// ExpiresAt 是保留期截止时刻。
	ExpiresAt time.Time `json:"expires_at"`
}

// outputSink 承接单个流的写入。所有导出方法都可并发调用。
type outputSink struct {
	mu   sync.Mutex
	opts OutputOptions

	stream    Stream
	spillPath string

	mem      []byte
	file     SpillFile
	fileSize int64

	total   int64 // 已接收
	stored  int64 // 已保留（mem + file）
	dropped int64 // 已丢弃
	reason  string
	writeEr string

	closed    bool
	createdAt time.Time
	expiresAt time.Time

	removed bool
}

func newOutputSink(stream Stream, opts OutputOptions, spillPath string) *outputSink {
	now := opts.Now()
	return &outputSink{
		opts:      opts,
		stream:    stream,
		spillPath: spillPath,
		createdAt: now,
		expiresAt: now.Add(opts.Retention),
	}
}

// Write 实现 io.Writer。
//
// 它永远返回 (len(p), nil)：配额丢弃和溢出写失败都不表现为写入错误，理由见包注释。
// 真实失败通过 DropReason 暴露给读取方。
func (s *outputSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0, ErrOutputClosed
	}
	n := len(p)
	s.total += int64(n)
	if n == 0 {
		return 0, nil
	}

	keep := p
	if remaining := s.opts.MaxBytes - s.stored; remaining <= 0 {
		s.drop(int64(n), "已达总存储配额")
		return n, nil
	} else if int64(len(keep)) > remaining {
		s.drop(int64(len(keep))-remaining, "已达总存储配额")
		keep = keep[:remaining]
	}

	if room := s.opts.MemoryLimit - int64(len(s.mem)); room > 0 {
		take := keep
		if int64(len(take)) > room {
			take = take[:room]
		}
		s.mem = append(s.mem, take...)
		s.stored += int64(len(take))
		keep = keep[len(take):]
	}

	if len(keep) > 0 {
		written, err := s.spillLocked(keep)
		s.stored += int64(written)
		if err != nil {
			s.writeEr = err.Error()
			s.drop(int64(len(keep))-int64(written), "溢出文件写入失败: "+err.Error())
			return n, nil
		}
	}
	return n, nil
}

// spillLocked 把数据追加到溢出文件，必要时先创建它。
func (s *outputSink) spillLocked(p []byte) (int, error) {
	if s.file == nil {
		f, err := s.opts.OpenSpill(s.spillPath)
		if err != nil {
			return 0, err
		}
		s.file = f
	}
	written, err := s.file.Write(p)
	s.fileSize += int64(written)
	if err != nil {
		return written, err
	}
	if written != len(p) {
		return written, io.ErrShortWrite
	}
	return written, nil
}

// drop 记录被丢弃的字节。首个原因被保留，因为后续原因通常是同一根因。
func (s *outputSink) drop(n int64, reason string) {
	if n <= 0 {
		return
	}
	s.dropped += n
	if s.reason == "" {
		s.reason = reason
	}
}

// read 执行一次分页读取。now 由 Manager 提供，便于测试推进时间。
func (s *outputSink) read(offset, limit int64, now time.Time) (OutputPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	page := OutputPage{
		Stream:       s.stream,
		Offset:       offset,
		TotalSize:    s.total,
		StoredSize:   s.stored,
		Truncated:    s.dropped > 0,
		DroppedBytes: s.dropped,
		DropReason:   s.reason,
		ExpiresAt:    s.expiresAt,
	}
	if offset < 0 {
		return page, ErrBadOffset
	}
	// 过期先于「已关闭」判断：过期清理会把 sink 置为已关闭，
	// 但调用方需要知道的是"过期了"，而不是内部状态变了。
	if !s.expiresAt.IsZero() && now.After(s.expiresAt) {
		page.Expired = true
		page.EOF = true
		return page, ErrOutputExpired
	}
	if s.closed {
		return page, ErrOutputClosed
	}
	if limit <= 0 || limit > s.opts.MaxReadBytes {
		limit = s.opts.MaxReadBytes
	}
	if offset >= s.stored {
		page.EOF = true
		return page, nil
	}
	end := offset + limit
	if end > s.stored {
		end = s.stored
	}
	// 多读几个字节用于判断切点是否落在 rune 内部。
	windowEnd := end
	if windowEnd < s.stored {
		if s.stored-windowEnd > int64(utf8.UTFMax) {
			windowEnd += int64(utf8.UTFMax)
		} else {
			windowEnd = s.stored
		}
	}
	buf, err := s.sliceLocked(offset, windowEnd)
	if err != nil {
		return page, err
	}
	cut := alignCut(buf, int(end-offset))
	page.Data = append([]byte(nil), buf[:cut]...)
	page.Text = string(page.Data)
	page.NextOffset = offset + int64(cut)
	page.EOF = page.NextOffset >= s.stored
	return page, nil
}

// sliceLocked 取出 [offset,end) 的连续字节，跨内存/文件边界时拼接。
func (s *outputSink) sliceLocked(offset, end int64) ([]byte, error) {
	memLen := int64(len(s.mem))
	if end <= memLen {
		return s.mem[offset:end], nil
	}
	if offset >= memLen {
		return s.readFileLocked(offset-memLen, end-offset)
	}
	out := make([]byte, 0, end-offset)
	out = append(out, s.mem[offset:memLen]...)
	rest, err := s.readFileLocked(0, end-memLen)
	if err != nil {
		return nil, err
	}
	return append(out, rest...), nil
}

// readFileLocked 从溢出文件读取。EOF 不算错误，返回值短读即结尾。
func (s *outputSink) readFileLocked(offset, n int64) ([]byte, error) {
	if n <= 0 {
		return nil, nil
	}
	if s.file == nil {
		// 记录数与文件不一致：只可能发生在 Close 之后，按不可用处理。
		return nil, ErrOutputUnavailable
	}
	buf := make([]byte, n)
	read, err := s.file.ReadAt(buf, offset)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("jobs: 读取溢出文件失败: %w", err)
	}
	return buf[:read], nil
}

// stats 返回 (总接收, 已保留, 已丢弃, 溢出文件路径) 的快照。
func (s *outputSink) stats() (total, stored, dropped int64, path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file != nil {
		path = s.spillPath
	}
	return s.total, s.stored, s.dropped, path
}

// closeAndMaybeRemove 关闭溢出文件；remove 为真时同时删除它。
//
// 返回是否真的删除了文件，便于统计清理结果。
func (s *outputSink) closeAndMaybeRemove(remove bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	if s.file != nil {
		_ = s.file.Close()
	}
	if remove && !s.removed {
		s.removed = true
		if s.spillPath != "" {
			if err := os.Remove(s.spillPath); err == nil {
				return true
			}
		}
	}
	return false
}

// expired 报告该输出在 now 是否已过保留期。
func (s *outputSink) expired(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.expiresAt.IsZero() && now.After(s.expiresAt)
}

// alignCut 把截断点调整到 UTF-8 rune 边界。
//
// limit 是期望的切点下标；buf 至少包含 limit 处及其后最多 UTFMax 个字节（若尚未结尾）。
func alignCut(buf []byte, limit int) int {
	if limit >= len(buf) {
		return len(buf)
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(buf[cut]) {
		cut--
	}
	if cut > 0 {
		return cut
	}
	// limit 落在首个 rune 内部：向前扩满一个 rune，避免返回空页卡死调用方。
	cut = 1
	for cut < len(buf) && !utf8.RuneStart(buf[cut]) {
		cut++
	}
	return cut
}
