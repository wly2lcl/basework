// Package jobs —— 持久化：把 job 的命令摘要、状态、输出引用与退出结果写进日志。
//
// 为什么需要它：`Manager` 是纯内存的。进程一退出，用户就再也说不出"我刚才那条命令到底
// 跑完了没有、输出在哪"。重启后如果继续谎称"还在运行"，或者更糟——自动把命令再跑一遍，
// 副作用会被重复施加。这两个坑都由本文件负责堵：
//
//   - **不伪称仍在运行**：重启时调用 `Manager.Reconcile`，把日志里所有非终态的 job
//     改写成 `interrupted`，明确表示"我们不知道它后来怎么样了"。
//   - **不自动重放**：日志只用于读取（`History` / `AllHistory`），没有任何代码路径
//     会因为"日志里有一条未完成记录"而重新执行命令。重跑必须由人显式发起。
//
// 存储格式是 JSONL（每行一个 JSON 对象），与 `pkg/session` 的 JSONL 存储保持一致：
// 追加写不需要重写整个文件，进程在写入途中被杀最多损失最后一行，而前面每一行仍然可读。
package jobs

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wly2lcl/basework/pkg/session"
)

// ErrJournal 表示 job 记录无法落盘。它被单独定义，是为了让"这次提交没有可恢复记录"
// 成为一个可判定的事实，而不是一条被忽略的日志。
var ErrJournal = errors.New("jobs: 无法写入 job 记录")

// 记录类型。
const (
	// RecordStarted 表示 job 已登记。
	RecordStarted = "started"
	// RecordTerminal 表示 job 已进入终态。
	RecordTerminal = "terminal"
)

// Record 是一条可持久化的 job 记录。
//
// 字段刻意与 `Job` 对齐而不是复用 `Job` 本身：日志是**跨版本持久化格式**，
// 直接序列化内存结构会让任何一次字段重构变成一次静默的数据格式变更。
type Record struct {
	Kind      string    `json:"kind"`
	JobID     string    `json:"job_id"`
	Owner     string    `json:"owner"`
	Command   string    `json:"command"`
	State     State     `json:"state"`
	CreatedAt time.Time `json:"created_at"`
	StartedAt time.Time `json:"started_at,omitempty"`
	EndedAt   time.Time `json:"ended_at,omitempty"`
	ExitCode  int       `json:"exit_code"`
	Err       string    `json:"error,omitempty"`

	// WorkspaceID 是产生该任务的工作区标识（启动时由产品层经 Options 注入）。
	// 旧日志没有该字段：读取方按「未归属」处理，不得默认归给当前工作区。
	WorkspaceID string `json:"workspace_id,omitempty"`

	StdoutBytes     int64    `json:"stdout_bytes,omitempty"`
	StderrBytes     int64    `json:"stderr_bytes,omitempty"`
	OutputTruncated bool     `json:"output_truncated,omitempty"`
	OutputRefs      []string `json:"output_refs,omitempty"`

	// RecordedAt 是记录写入时刻，用于排查"记录顺序与状态顺序不一致"。
	RecordedAt time.Time `json:"recorded_at"`
}

// Journal 是 job 记录的去向。nil 表示不做持久化（纯内存运行）。
type Journal interface {
	// Append 追加一条记录。
	Append(rec Record) error
	// Records 返回全部记录，按写入顺序。
	Records() ([]Record, error)
}

// JSONLJournal 把记录追加到单个 JSONL 文件。
type JSONLJournal struct {
	path string

	mu sync.Mutex
	// skipped 记录读取时被跳过的损坏行数，便于诊断而不必打断使用。
	skipped int
}

// NewJSONLJournal 创建（或绑定）一个 JSONL 日志文件。
//
// 这里不创建目录也不打开文件：真正需要写的时候再开。理由是一个"只是读取历史"的
// 进程（比如 `jobs list`）不该顺手在磁盘上留下空文件。
func NewJSONLJournal(path string) *JSONLJournal { return &JSONLJournal{path: path} }

// Path 返回日志文件路径。
func (j *JSONLJournal) Path() string { return j.path }

// Append 追加一条记录。
//
// 用 O_APPEND 单次写入：内核保证追加写的原子性，因此多个进程同时写不会互相覆盖，
// 也保证读者不会看到两行交织的内容。
func (j *JSONLJournal) Append(rec Record) error {
	if rec.RecordedAt.IsZero() {
		rec.RecordedAt = time.Now()
	}
	line, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("%w: 序列化失败: %v", ErrJournal, err)
	}
	line = append(line, '\n')

	j.mu.Lock()
	defer j.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(j.path), 0o700); err != nil {
		return fmt.Errorf("%w: %v", ErrJournal, err)
	}
	// 锁住日志本身，避免为只读查询额外创建 .lock 文件。
	fileLock := session.NewFileLock(j.path)
	if err := fileLock.Lock(5 * time.Second); err != nil {
		return fmt.Errorf("%w: 获取追加锁失败: %v", ErrJournal, err)
	}
	defer func() { _ = fileLock.Unlock() }()
	// Unix FileLock opens the data path itself, so it already exists here;
	// Windows keeps the coordination handle in a sibling .lock file and the
	// data file is created by the append open below. Apply the mode only when
	// the data file exists; OpenFile still creates a new file with 0600.
	if _, statErr := os.Stat(j.path); statErr == nil {
		if err := os.Chmod(j.path, 0o600); err != nil {
			return fmt.Errorf("%w: 设置日志权限失败: %v", ErrJournal, err)
		}
	} else if !os.IsNotExist(statErr) {
		return fmt.Errorf("%w: 检查日志权限失败: %v", ErrJournal, statErr)
	}
	// 如果文件末尾是一次崩溃留下的未结束且无效 JSON，先截掉这条残尾，
	// 再追加新记录。只允许修剪“最后一条未换行的坏记录”；带换行的中间
	// 损坏仍然保留并由 Records 报错，避免把历史问题静默掩盖。
	needsSeparator := false
	if data, readErr := os.ReadFile(j.path); readErr == nil && len(data) > 0 && data[len(data)-1] != '\n' {
		lastNL := -1
		for i := len(data) - 1; i >= 0; i-- {
			if data[i] == '\n' {
				lastNL = i
				break
			}
		}
		tail := bytes.TrimSpace(data[lastNL+1:])
		var ignored Record
		switch {
		case len(tail) == 0:
			// 只有空白残尾：保留此前完整行，去掉无意义的尾部字节。
			f, openErr := os.OpenFile(j.path, os.O_WRONLY, 0o600)
			if openErr != nil {
				return fmt.Errorf("%w: 修剪残尾失败: %v", ErrJournal, openErr)
			}
			if truncErr := f.Truncate(int64(lastNL + 1)); truncErr != nil {
				_ = f.Close()
				return fmt.Errorf("%w: 修剪残尾失败: %v", ErrJournal, truncErr)
			}
			if closeErr := f.Close(); closeErr != nil {
				return fmt.Errorf("%w: 关闭日志失败: %v", ErrJournal, closeErr)
			}
		case json.Unmarshal(tail, &ignored) == nil:
			// 最后一行是完整 JSON 但崩溃前没有写换行：补分隔符，避免
			// 新记录与它粘成同一行。
			needsSeparator = true
		default:
			f, openErr := os.OpenFile(j.path, os.O_WRONLY, 0o600)
			if openErr != nil {
				return fmt.Errorf("%w: 修剪残尾失败: %v", ErrJournal, openErr)
			}
			if truncErr := f.Truncate(int64(lastNL + 1)); truncErr != nil {
				_ = f.Close()
				return fmt.Errorf("%w: 修剪残尾失败: %v", ErrJournal, truncErr)
			}
			if closeErr := f.Close(); closeErr != nil {
				return fmt.Errorf("%w: 关闭日志失败: %v", ErrJournal, closeErr)
			}
		}
	}
	f, err := os.OpenFile(j.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrJournal, err)
	}
	defer f.Close()
	if needsSeparator {
		if _, err := f.Write([]byte{'\n'}); err != nil {
			return fmt.Errorf("%w: %v", ErrJournal, err)
		}
	}
	if _, err := f.Write(line); err != nil {
		return fmt.Errorf("%w: %v", ErrJournal, err)
	}
	return nil
}

// Records 读取全部记录。
//
// 容错口径：**只有最后一行**允许是残缺的（进程在写入途中被杀），它被跳过；
// 中间任何一行解析失败都返回错误。理由是一个"中间行坏了"的日志说明文件被截断或被
// 外部改写，此时继续静默使用剩下的一半，比报错更危险。
func (j *JSONLJournal) Records() ([]Record, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if _, err := os.Stat(j.path); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("jobs: 读取日志失败: %w", err)
	}
	fileLock := session.NewFileLock(j.path)
	if err := fileLock.Lock(5 * time.Second); err != nil {
		return nil, fmt.Errorf("jobs: 获取读取锁失败: %v", err)
	}
	defer func() { _ = fileLock.Unlock() }()
	return j.recordsLocked()
}

// Skipped 返回最近一次读取时被跳过的残缺行数。
func (j *JSONLJournal) Skipped() int {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.skipped
}

func (j *JSONLJournal) recordsLocked() ([]Record, error) {
	j.skipped = 0
	f, err := os.Open(j.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("jobs: 读取日志失败: %w", err)
	}
	defer f.Close()

	var (
		out    []Record
		lines  []string
		reader = bufio.NewReader(f)
	)
	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			lines = append(lines, line)
		}
		if err != nil {
			break
		}
	}

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		var rec Record
		if err := json.Unmarshal([]byte(trimmed), &rec); err != nil {
			last := i == len(lines)-1 && !strings.HasSuffix(line, "\n")
			if last {
				// 崩溃时写了一半的最后一行：可容忍。
				j.skipped++
				continue
			}
			return nil, fmt.Errorf("jobs: 日志第 %d 行损坏: %w", i+1, err)
		}
		out = append(out, rec)
	}
	return out, nil
}

// foldRecords 把 started / terminal 两类记录折叠成每个 job 的最终视图。
//
// 折叠规则：后出现的记录覆盖先出现的同名字段。这样"同一个 job 被写了多条记录"时，
// 看到的始终是最后一条，而不是第一条或某种合并结果。
func foldRecords(records []Record) []Job {
	type slot struct {
		job Job
		seq int
	}
	slots := make(map[string]*slot, len(records))
	order := make([]string, 0, len(records))

	for i, rec := range records {
		s, ok := slots[rec.JobID]
		if !ok {
			s = &slot{job: Job{ID: rec.JobID, Owner: rec.Owner, ExitCode: -1}}
			slots[rec.JobID] = s
			order = append(order, rec.JobID)
		}
		if rec.Owner != "" {
			s.job.Owner = rec.Owner
		}
		if rec.WorkspaceID != "" {
			s.job.WorkspaceID = rec.WorkspaceID
		}
		if rec.Command != "" {
			s.job.Command = rec.Command
		}
		if !rec.CreatedAt.IsZero() {
			s.job.CreatedAt = rec.CreatedAt
		}
		if !rec.StartedAt.IsZero() {
			s.job.StartedAt = rec.StartedAt
		}
		if !rec.EndedAt.IsZero() {
			s.job.EndedAt = rec.EndedAt
		}
		// 状态只在终态记录里出现；started 记录里的 queued/running 不该覆盖已有终态。
		if rec.Kind == RecordTerminal || rec.State.Terminal() {
			s.job.State = rec.State
			s.job.ExitCode = rec.ExitCode
			s.job.Err = rec.Err
			s.job.StdoutBytes = rec.StdoutBytes
			s.job.StderrBytes = rec.StderrBytes
			s.job.OutputTruncated = rec.OutputTruncated
			s.job.OutputRefs = rec.OutputRefs
		} else if s.job.State == "" {
			s.job.State = rec.State
		}
		s.seq = i
	}

	out := make([]Job, 0, len(order))
	for _, id := range order {
		out = append(out, slots[id].job)
	}
	sort.SliceStable(out, func(i, j int) bool {
		// 先按登记时间排，时间相同（同一时钟刻度）时保持记录顺序。
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return false
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

// History 返回该 owner 已持久化的全部 job（含历史进程留下的）。
//
// 它是**只读**的：不会因为日志里存在未完成的记录而重新执行任何命令。
func (m *Manager) History(owner string) ([]Job, error) {
	if owner == "" {
		return nil, ErrMissingOwner
	}
	all, err := m.AllHistory()
	if err != nil {
		return nil, err
	}
	out := make([]Job, 0, len(all))
	for _, job := range all {
		if job.Owner == owner {
			out = append(out, job)
		}
	}
	return out, nil
}

// AllHistory 返回日志里的全部 job，不做归属过滤。
//
// 它给产品层的历史查看入口用（例如 `basework jobs list`）。越权保护发生在
// 面向模型的工具层（按 owner 过滤），而不是这里——CLI 是用户本人，不是某个会话。
func (m *Manager) AllHistory() ([]Job, error) {
	if m.opts.Journal == nil {
		return nil, nil
	}
	records, err := m.opts.Journal.Records()
	if err != nil {
		return nil, err
	}
	return foldRecords(records), nil
}

// ReconcileOwner 处理"上次进程留下的、属于该 owner 的非终态记录"：把它们改写成 interrupted。
//
// 返回被改写的条数。三个设计要点：
//
//   - **必须显式调用**，不在 `New` 里自动做：让"读一眼历史"这种只读操作产生写入副作用
//     是不可接受的。
//   - **按 owner 过滤**：同一台机器上可能同时有另一个进程在跑，它名下的 running 记录
//     是活的。只有自己的记录才可能是"上次遗留"。
//   - **写成 interrupted 而不是 failed**：进程消失时我们并没有观测到那条命令的结局，
//     把它写成 failed 是在编造一个不存在的结论。
func (m *Manager) ReconcileOwner(owner string) (int, error) {
	if m.opts.Journal == nil {
		return 0, nil
	}
	if owner == "" {
		return 0, ErrMissingOwner
	}
	records, err := m.opts.Journal.Records()
	if err != nil {
		return 0, err
	}

	n := 0
	for _, job := range foldRecords(records) {
		if job.Owner != owner {
			continue
		}
		if job.State == "" || job.State.Terminal() {
			continue
		}
		rec := Record{
			Kind:       RecordTerminal,
			JobID:      job.ID,
			Owner:      job.Owner,
			Command:    job.Command,
			State:      StateInterrupted,
			CreatedAt:  job.CreatedAt,
			StartedAt:  job.StartedAt,
			EndedAt:    m.opts.Now(),
			ExitCode:   -1,
			Err:        "进程重启：无法接管上次运行中的任务",
			OutputRefs: job.OutputRefs,
		}
		if err := m.opts.Journal.Append(rec); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// journalStart 在登记 job 之前写入 started 记录。
func (m *Manager) journalStart(job Job) error {
	if m.opts.Journal == nil {
		return nil
	}
	return m.opts.Journal.Append(Record{
		Kind:        RecordStarted,
		JobID:       job.ID,
		Owner:       job.Owner,
		Command:     job.Command,
		State:       job.State,
		CreatedAt:   job.CreatedAt,
		ExitCode:    -1,
		WorkspaceID: m.opts.WorkspaceID,
	})
}

// journalTerminal 在进入终态时写入 terminal 记录。
//
// 返回错误而不是吞掉：调用方会把它记在 job 快照上（`JournalErr`），
// 让"这次运行没有可恢复记录"变成用户能看见的事实。
func (m *Manager) journalTerminal(job Job) error {
	if m.opts.Journal == nil {
		return nil
	}
	return m.opts.Journal.Append(Record{
		Kind:            RecordTerminal,
		JobID:           job.ID,
		Owner:           job.Owner,
		Command:         job.Command,
		State:           job.State,
		CreatedAt:       job.CreatedAt,
		StartedAt:       job.StartedAt,
		EndedAt:         job.EndedAt,
		ExitCode:        job.ExitCode,
		Err:             job.Err,
		StdoutBytes:     job.StdoutBytes,
		StderrBytes:     job.StderrBytes,
		OutputTruncated: job.OutputTruncated,
		OutputRefs:      job.OutputRefs,
		WorkspaceID:     m.opts.WorkspaceID,
	})
}
