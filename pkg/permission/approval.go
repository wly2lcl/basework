package permission

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// 本文件实现 UI-002 的批准协议：把「是否允许执行这个工具调用」变成
// 一条带唯一 ID 的请求事件，交给 UI（TUI/未来其他前端）呈现，用户
// 允许/拒绝/关闭后按 ID 应答。
//
// 设计要点（对应任务卡验收条件）：
//   - 每个请求唯一 ID，应答按 ID 匹配；找不到 pending 的应答一律忽略——
//     「批准不会错误应用到下一请求」由 ID 隔离保证，而不是靠 UI 自觉。
//   - 关闭窗口与超时都等价于拒绝（宁可让人再确认一次，不可悄悄放行）。
//   - 决策默认不缓存：缓存会让「本次批准」泄漏到「下次相同调用」，
//     正是验收条件要防的错误应用。想要永久规则应走规则/存储（ask→allow）。

// Decision 是一次批准请求的结局。
type Decision int

const (
	DecisionApproved Decision = iota // 用户明确允许
	DecisionDenied                   // 用户明确拒绝
	DecisionClosed                   // 用户关闭了审批窗口（等价拒绝）
	DecisionTimeout                  // 等待超时（等价拒绝）
)

// String 返回决策的可读文本（用于审计与测试输出）。
func (d Decision) String() string {
	switch d {
	case DecisionApproved:
		return "approved"
	case DecisionDenied:
		return "denied"
	case DecisionClosed:
		return "closed"
	case DecisionTimeout:
		return "timeout"
	default:
		return fmt.Sprintf("decision(%d)", int(d))
	}
}

// Approved 报告该决策是否等于放行。只有用户明确允许才算。
func (d Decision) Approved() bool { return d == DecisionApproved }

// ApprovalRequest 是一次待决的批准请求。
type ApprovalRequest struct {
	// ID 是请求唯一标识。UI 的应答必须携带它。
	ID string `json:"id"`
	// ToolName 是被请求的工具名。
	ToolName string `json:"tool_name"`
	// Purpose 是工具目的的一句话说明（人类可读）。
	Purpose string `json:"purpose"`
	// Paths 是本次调用涉及的文件路径（尽力提取）。
	Paths []string `json:"paths,omitempty"`
	// Diff 是编辑类调用的预览差异（尽力提取，可能为空）。
	Diff string `json:"diff,omitempty"`
	// RiskReason 是风险依据：为什么需要确认（命中了什么特征/规则）。
	RiskReason string `json:"risk_reason"`
	// CreatedAt 是请求创建时间。
	CreatedAt time.Time `json:"created_at"`
}

// ErrBrokerClosed 在 broker 已关闭时由 Request 返回。
var ErrBrokerClosed = errors.New("permission: approval broker closed")

// ApprovalBroker 连接权限检查（执行 goroutine）与 UI（渲染 goroutine）。
// 并发安全：Request 可同时有多个（串行化的运行下通常只有 one）。
type ApprovalBroker struct {
	mu      sync.Mutex
	pending map[string]chan Decision
	seq     int64
	closed  bool
	// notify 是 UI 通知回调；nil 表示没有 UI 在听（Request 会一直等到超时）。
	notify func(ApprovalRequest)
	// idPrefix 用于测试与多实例区分。
	idPrefix string
}

// NewApprovalBroker 创建 broker。
func NewApprovalBroker() *ApprovalBroker {
	return &ApprovalBroker{
		pending:  make(map[string]chan Decision),
		idPrefix: "apr",
	}
}

// SetNotifier 注册 UI 通知回调（TUI 用 program.Send 包装）。传 nil 注销。
// 回调在 Request 的调用方 goroutine 里执行——UI 实现必须把请求转成消息
// 投递到自己的事件循环，绝不能在回调里做阻塞操作。
func (b *ApprovalBroker) SetNotifier(fn func(ApprovalRequest)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.notify = fn
}

// Request 发起一次批准请求并等待结局。
// timeout <=0 时用 DefaultApprovalTimeout。ctx 取消等价于关闭（拒绝）。
func (b *ApprovalBroker) Request(ctx context.Context, req ApprovalRequest, timeout time.Duration) (Decision, error) {
	if timeout <= 0 {
		timeout = DefaultApprovalTimeout
	}

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return DecisionClosed, ErrBrokerClosed
	}
	b.seq++
	id := fmt.Sprintf("%s-%06d", b.idPrefix, b.seq)
	req.ID = id
	req.CreatedAt = time.Now().UTC()
	ch := make(chan Decision, 1)
	b.pending[id] = ch
	notify := b.notify
	b.mu.Unlock()

	// 通知 UI（在锁外：回调可能做投递，不该持有 pending 表）。
	if notify != nil {
		notify(req)
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case dec := <-ch:
		return dec, nil
	case <-timer.C:
		b.resolve(id, DecisionTimeout)
		return DecisionTimeout, nil
	case <-ctx.Done():
		b.resolve(id, DecisionClosed)
		return DecisionClosed, nil
	}
}

// Respond 按 ID 应答。请求不存在（已超时/已取消/从未发出）时返回 false，
// 调用方应忽略——这就是「陈旧批准不泄漏」的机制保证。
func (b *ApprovalBroker) Respond(id string, dec Decision) bool {
	return b.resolve(id, dec)
}

// resolve 内部应答：摘除 pending 并投递，返回是否命中。
func (b *ApprovalBroker) resolve(id string, dec Decision) bool {
	b.mu.Lock()
	ch, ok := b.pending[id]
	if ok {
		delete(b.pending, id)
	}
	b.mu.Unlock()
	if !ok {
		return false
	}
	ch <- dec
	return true
}

// Close 关闭 broker：所有 pending 请求立即以 DecisionClosed 结局。
// 之后的新请求返回 ErrBrokerClosed。
func (b *ApprovalBroker) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for id, ch := range b.pending {
		ch <- DecisionClosed
		delete(b.pending, id)
	}
}

// DefaultApprovalTimeout 是审批等待的默认上限。设上限是因为：静默等待
// 永远不结束比拒绝更糟——模型侧会一直挂起，用户可能根本没在看屏幕。
const DefaultApprovalTimeout = 120 * time.Second

// ApprovalPromptFunc 把 broker 适配成 Checker 的 PromptFunc。
// 超时/关闭一律拒绝；决策不缓存（见文件头说明）。
func ApprovalPromptFunc(b *ApprovalBroker, timeout time.Duration) PromptFunc {
	return func(ctx context.Context, toolName string, args map[string]interface{}) (bool, bool, error) {
		req := BuildApprovalRequest(toolName, args, nil)
		dec, err := b.Request(ctx, req, timeout)
		if err != nil {
			return false, false, err
		}
		// 第二个返回值是 cacheDecision：刻意 false。
		return dec.Approved(), false, nil
	}
}

// RiskFunc 根据工具名与参数给出风险依据；返回空串表示无额外依据。
type RiskFunc func(toolName string, args map[string]interface{}) string

// BuildApprovalRequest 从工具调用参数提取展示信息（目的/路径/diff/风险）。
// riskFn 可为 nil。提取是尽力而为：展示层缺信息不能变成执行层的豁免。
func BuildApprovalRequest(toolName string, args map[string]interface{}, riskFn RiskFunc) ApprovalRequest {
	req := ApprovalRequest{
		ToolName: toolName,
		Purpose:  toolPurpose(toolName),
	}

	paths := map[string]bool{}
	var diffParts []string

	for key, val := range args {
		switch key {
		case "command":
			if s, ok := val.(string); ok {
				diffParts = append(diffParts, "$ "+s)
				for _, tok := range pathTokens(s) {
					paths[tok] = true
				}
			}
		case "path", "file_path", "target":
			if s, ok := val.(string); ok && s != "" {
				paths[s] = true
			}
		case "content", "new_string", "old_string":
			if s, ok := val.(string); ok && s != "" {
				diffParts = append(diffParts, contentPreview(s))
			}
		case "edits":
			// edit_files 的 edits 是结构化列表；序列化预览。
			if data, err := json.Marshal(val); err == nil {
				diffParts = append(diffParts, contentPreview(string(data)))
			}
		case "_approval_paths":
			if list, ok := val.([]string); ok {
				for _, p := range list {
					if p != "" {
						paths[p] = true
					}
				}
			}
		case "_approval_diff":
			if s, ok := val.(string); ok && s != "" {
				diffParts = append(diffParts, s)
			}
		}
		if s, ok := val.(string); ok && looksLikePath(s) {
			paths[s] = true
		}
	}

	for p := range paths {
		req.Paths = append(req.Paths, p)
	}
	sort.Strings(req.Paths)
	req.Diff = strings.Join(diffParts, "\n")
	if len(req.Diff) > maxApprovalDiffBytes {
		req.Diff = req.Diff[:maxApprovalDiffBytes] + "\n…（已截断）"
	}

	req.RiskReason = riskReason(toolName, args)
	if riskFn != nil {
		if extra := riskFn(toolName, args); extra != "" {
			if req.RiskReason != "" {
				req.RiskReason += "；"
			}
			req.RiskReason += extra
		}
	}
	return req
}

// maxApprovalDiffBytes 限制 diff 预览体积：审批卡片是给人看的，
// 不是日志回放；超长内容截断并明示。
const maxApprovalDiffBytes = 2048

// toolPurpose 给出工具目的说明。
func toolPurpose(toolName string) string {
	switch toolName {
	case "bash", "background_bash":
		return "执行 shell 命令"
	case "read", "read_file":
		return "读取文件"
	case "write", "write_file":
		return "写入文件"
	case "edit", "edit_files", "apply_patch":
		return "修改文件"
	case "web_fetch":
		return "抓取网页内容"
	case "web_search":
		return "联网搜索"
	default:
		return "调用工具 " + toolName
	}
}

// riskReason 推导风险依据：命中危险特征时明确说出了什么。
func riskReason(toolName string, args map[string]interface{}) string {
	if toolName != "bash" && toolName != "background_bash" {
		return ""
	}
	cmd, _ := args["command"].(string)
	var hits []string
	dangerous := []struct {
		token string
		why   string
	}{
		{"rm ", "删除文件"},
		{"rm -", "删除文件"},
		{"git push", "推送远端"},
		{"chmod ", "修改权限"},
		{"chown ", "修改属主"},
		{"> /", "写系统路径"},
		{"sudo", "提权执行"},
		{"curl ", "网络下载"},
		{"wget ", "网络下载"},
		{"kill ", "终止进程"},
	}
	lower := strings.ToLower(cmd)
	for _, d := range dangerous {
		if strings.Contains(lower, d.token) {
			hits = append(hits, d.why)
		}
	}
	if len(hits) == 0 {
		return ""
	}
	return "命令包含: " + strings.Join(uniqueStrings(hits), "、")
}

// ---- 小工具 ----

func pathTokens(cmd string) []string {
	// 绝对路径 token 提取：以 / 开头的连续非空白段。
	var out []string
	for _, tok := range strings.Fields(cmd) {
		if strings.HasPrefix(tok, "/") {
			out = append(out, strings.Trim(tok, "\"'"))
		}
	}
	return out
}

func looksLikePath(s string) bool {
	return strings.HasPrefix(s, "/") && !strings.ContainsAny(s, "\n ;|&") && len(s) < 512
}

func contentPreview(s string) string {
	if len(s) <= maxApprovalDiffBytes/2 {
		return s
	}
	return s[:maxApprovalDiffBytes/2] + "…（已截断）"
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
