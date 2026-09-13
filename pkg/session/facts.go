package session

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// 本文件实现统一工作区事实模型（CTX-001）。
//
// 与既有组件的分工（避免重复模型）：
//   - FileTracker：单个会话内的内存态文件追踪，喂给上下文预算等即时决策；
//     不落盘、无工作区概念。
//   - WorkspaceFacts：跨会话的持久化事实投影，来源是已经持久化的两类记录——
//     会话事件（file.edited）与后台任务日志（jobs.jsonl）。它不重复追踪
//     「即时」状态，只回答「这个工作区发生过什么」。
//
// 诊断信息刻意不入模型：lsp_diagnostics 是一次性查询结果而非发生事实，
// 可随时重查；把它固化下来只会制造与代码现实漂移的过期快照。

// FactsSchemaVersion 是工作区事实文件的格式版本。
//
// v1：首个带版本信封的格式（{"version":1,...}）。
// v0：无信封的裸 Fact 数组（历史上从未正式发布过，但加载器仍可读——
// 版本机制从第一天就要证明「能读旧数据」而不只是「能拒绝新数据」）。
const FactsSchemaVersion = 1

// ErrFactsTooNew 表示事实文件版本高于当前程序支持版本。
// 与会话日志同理：必须拒绝而不是尽力解析，静默继续等于用过期规则解释新数据。
var ErrFactsTooNew = errors.New("session: 工作区事实版本高于当前支持版本")

// 事实类别。
const (
	FactFileRead     = "file_read"     // 文件被读取
	FactFileModified = "file_modified" // 文件被提交写入（edit_files committed）
	FactFileReverted = "file_reverted" // 文件被撤销回基线
	FactCommand      = "command"       // 后台命令执行过（按 job id 去重）
)

// Fact 是一条工作区事实。
type Fact struct {
	// Kind 是事实类别（见上方常量）。
	Kind string `json:"kind"`
	// Key 是去重键。同一 Key 的事实只累计一次。
	Key string `json:"key"`
	// Path 是工作区相对路径（文件类事实）。
	Path string `json:"path,omitempty"`
	// Ref 是来源关联标识：job id / 编辑计划 id / 会话 id。
	Ref string `json:"ref,omitempty"`
	// Source 是产出事实的链路（如 "edit_files"、"job"）。
	Source string `json:"source,omitempty"`
	// FirstSeen 是首次记录时间（RFC3339）。重复累计不刷新该时间。
	FirstSeen string `json:"first_seen"`
}

// WorkspaceFacts 是一个工作区的去重事实集合。线程安全。
type WorkspaceFacts struct {
	WorkspaceID string

	mu    sync.Mutex
	facts map[string]Fact
}

// NewWorkspaceFacts 创建空的事实集合。
func NewWorkspaceFacts(workspaceID string) *WorkspaceFacts {
	return &WorkspaceFacts{
		WorkspaceID: workspaceID,
		facts:       make(map[string]Fact),
	}
}

// WorkspaceID 从工作区根目录派生稳定标识：解析软链后取绝对路径的
// SHA-256 前 16 个 hex 字符。不同工作区（不同目录）天然隔离；
// 同一目录经软链或相对路径访问得到同一 ID，不因调用方式漂移。
func WorkspaceID(root string) string {
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	sum := sha256.Sum256([]byte(filepath.Clean(root)))
	return hex.EncodeToString(sum[:8])
}

// FileFactKey 生成文件事实的去重键。
func FileFactKey(kind, path string) string { return kind + ":" + path }

// CommandFactKey 生成命令事实的去重键——按 job id 去重，
// 同一任务跨重启、跨多次查询只累计一次。
func CommandFactKey(jobID string) string { return "job:" + jobID }

// Add 记录一条事实。返回是否为新事实：已存在的 Key 直接忽略，
// 且不刷新 FirstSeen——「相同事实不重复累计」的语义由这里保证。
func (w *WorkspaceFacts) Add(f Fact) bool {
	if f.Key == "" || f.Kind == "" {
		return false // 无 Key 的事实无法去重，拒绝静默吞掉
	}
	if f.FirstSeen == "" {
		f.FirstSeen = time.Now().UTC().Format(time.RFC3339)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, exists := w.facts[f.Key]; exists {
		return false
	}
	w.facts[f.Key] = f
	return true
}

// List 返回全部事实，按 (Kind, Key) 排序，输出稳定可 diff。
func (w *WorkspaceFacts) List() []Fact {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]Fact, 0, len(w.facts))
	for _, f := range w.facts {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// Count 返回事实总数。
func (w *WorkspaceFacts) Count() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.facts)
}

// factsEnvelope 是持久化信封。version 参与会话日志同一套升级纪律。
type factsEnvelope struct {
	Version     int    `json:"version"`
	WorkspaceID string `json:"workspace_id"`
	Facts       []Fact `json:"facts"`
}

// FactsDir 返回事实文件目录：<base>/facts。
func FactsDir(base string) string { return filepath.Join(base, "facts") }

// factsPath 返回某工作区的事实文件路径。文件名即 workspace ID，
// 不同工作区落在不同文件，物理隔离先于逻辑校验。
func factsPath(base, workspaceID string) string {
	return filepath.Join(FactsDir(base), workspaceID+".json")
}

// SaveWorkspaceFacts 原子写出事实文件（同目录临时文件 + rename）。
func SaveWorkspaceFacts(base string, w *WorkspaceFacts) error {
	if err := os.MkdirAll(FactsDir(base), 0o755); err != nil {
		return fmt.Errorf("session: 创建事实目录失败: %w", err)
	}
	env := factsEnvelope{
		Version:     FactsSchemaVersion,
		WorkspaceID: w.WorkspaceID,
		Facts:       w.List(),
	}
	data, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return fmt.Errorf("session: 编码事实失败: %w", err)
	}
	path := factsPath(base, w.WorkspaceID)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("session: 写事实临时文件失败: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("session: 原子替换事实文件失败: %w", err)
	}
	return nil
}

// LoadWorkspaceFacts 读取事实文件。
//
//   - 文件不存在：返回空集合（首个会话的正常状态，不是错误）。
//   - v0 裸数组：按旧格式读取并升级到当前版本结构。
//   - version > FactsSchemaVersion：返回 ErrFactsTooNew，拒绝解析。
//   - 信封 workspace_id 与请求的不一致：报错——调用方拿错工作区了，
//     返回别人的事实比报错危险得多。
func LoadWorkspaceFacts(base, workspaceID string) (*WorkspaceFacts, error) {
	data, err := os.ReadFile(factsPath(base, workspaceID))
	if err != nil {
		if os.IsNotExist(err) {
			return NewWorkspaceFacts(workspaceID), nil
		}
		return nil, fmt.Errorf("session: 读事实文件失败: %w", err)
	}

	// v0：裸数组，无信封。按第一个字符区分——'[' 是旧格式。
	trimmed := strings.TrimSpace(string(data))
	if strings.HasPrefix(trimmed, "[") {
		var facts []Fact
		if err := json.Unmarshal(data, &facts); err != nil {
			return nil, fmt.Errorf("session: 解析 v0 事实失败: %w", err)
		}
		w := NewWorkspaceFacts(workspaceID)
		for _, f := range facts {
			// v0 记录没有 Key 字段，按当前规则重建，保证去重语义一致。
			if f.Key == "" {
				if f.Kind == FactCommand {
					f.Key = CommandFactKey(f.Ref)
				} else {
					f.Key = FileFactKey(f.Kind, f.Path)
				}
			}
			w.Add(f)
		}
		return w, nil
	}

	var env factsEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("session: 解析事实文件失败: %w", err)
	}
	if env.Version > FactsSchemaVersion {
		return nil, fmt.Errorf("%w: 文件版本 %d > 支持版本 %d", ErrFactsTooNew, env.Version, FactsSchemaVersion)
	}
	if env.WorkspaceID != workspaceID {
		return nil, fmt.Errorf("session: 事实文件属于工作区 %q，与请求的 %q 不一致", env.WorkspaceID, workspaceID)
	}
	w := NewWorkspaceFacts(workspaceID)
	for _, f := range env.Facts {
		w.Add(f)
	}
	return w, nil
}

// FoldFileEdited 把一条 file.edited 事件折叠进事实集合。
//
// 折叠是幂等的：同一事件重复投影（重放、多次审阅）产出的事实集合不变，
// 因为 Key 由 (类别, 路径) 决定，与事件本身的出现次数无关。
// preview 阶段不产生事实——预览不写盘，「看过」不是「改过」。
func FoldFileEdited(w *WorkspaceFacts, data FileEditedData, at time.Time) {
	seen := at.UTC().Format(time.RFC3339)
	for _, rec := range data.Files {
		if rec.Path == "" {
			continue
		}
		var kind, source string
		switch {
		case data.Phase == "committed" && rec.State == "written":
			kind, source = FactFileModified, "edit_files"
		case data.Phase == "rolled_back" && rec.State == "reverted":
			kind, source = FactFileReverted, "edit_files"
		default:
			continue // 预览、冲突、未尝试等都不构成「已发生」事实
		}
		w.Add(Fact{
			Kind:      kind,
			Key:       FileFactKey(kind, rec.Path),
			Path:      rec.Path,
			Ref:       data.PlanID,
			Source:    source,
			FirstSeen: seen,
		})
	}
}
