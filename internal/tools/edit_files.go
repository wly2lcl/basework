package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wly2lcl/basework/internal/edits"
	"github.com/wly2lcl/basework/pkg/session"
	"github.com/wly2lcl/basework/pkg/tool"
)

// 本文件把 internal/edits 的「只读预览 → 批准 → 提交 → 撤销」流程暴露为模型
// 可调用工具（EDIT-003），并把每一步落成 file.edited 会话事件。
//
// 与 apply_patch 的分工：apply_patch 是"检查后一次写到底"的快捷入口，没有
// 预览与撤销；本工具走完整流程——diff 来自真实字节、提交前基线再校验、撤销
// 前确认文件未被二次修改。需要审阅与回退的编辑应使用本工具。
//
// 批准语义：preview 只生成预览不改盘；commit 只接受 preview 给出的 plan_id，
// 且提交阶段重新做路径解析、权限检查与基线比对——即使调用方跳过预览直接编造
// plan_id 也无法绕过这些检查（不存在即拒绝）。拒绝则什么都不发生。

// maxTrackedPlans 是进程内保留的计划数上限。计划里的批次持有原始内容快照，
// 无上限会随会话长度膨胀；只保留最近 N 个，过期的需要重新 preview。
const maxTrackedPlans = 32

// EditEventSink 接收编辑流程的可追溯事件。实现方决定存储位置；失败由实现方
// 记录，不阻断编辑流程本身（事件是审计线索，不是提交的组成部分）。
type EditEventSink interface {
	AppendEditEvent(data *session.FileEditedData) error
}

// EditFilesTool 实现 edit_files 工具。
type EditFilesTool struct {
	// WorkDir 是编辑工作区的根目录，所有目标必须落在它之内。
	WorkDir string
	// CheckPath 是产品层路径权限入口，与 builtin 敏感路径检查同一策略。
	// 对调用方给的路径和符号链接解析后的真实路径都会各查一次。
	CheckPath func(path string) (allowed bool, reason string)
	// Sink 可选：把编辑事实写成会话事件。nil 时只返回结果不落事件。
	Sink EditEventSink
	// Tracker 可选：进程内文件追踪（会话生命周期内）。nil 时跳过。
	Tracker *session.FileTracker

	planner     *edits.Planner
	plannerErr  error
	plannerOnce sync.Once

	mu    sync.Mutex
	plans map[string]*editPlanRecord
	order []string // planID 按创建顺序，用于淘汰最旧
}

type editPlanRecord struct {
	id         string
	batch      *edits.Batch
	committed  *edits.CommitResult
	rolledBack bool
}

// NewEditFilesTool 创建 edit_files 工具。
func NewEditFilesTool(workDir string) *EditFilesTool {
	if workDir == "" {
		workDir = "."
	}
	return &EditFilesTool{WorkDir: workDir}
}

// Name 返回工具名称。
func (t *EditFilesTool) Name() string { return "edit_files" }

// Description 返回工具描述。
func (t *EditFilesTool) Description() string {
	return "对工作区内已有文本文件做可审阅的编辑：先 preview 生成基于真实文件内容的 diff（不写盘），确认后 commit 提交（提交前重新校验基线与权限），必要时 rollback 撤销本次提交。"
}

// Parameters 返回工具参数 JSON Schema。
func (t *EditFilesTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {
				"type": "string",
				"enum": ["preview", "commit", "rollback"],
				"description": "preview：生成预览（需要 edits）；commit：提交计划（需要 plan_id）；rollback：撤销已提交的计划（需要 plan_id）"
			},
			"edits": {
				"type": "array",
				"description": "preview 时的替换列表，同一文件可出现多次（按顺序应用）",
				"items": {
					"type": "object",
					"properties": {
						"path": {"type": "string", "description": "工作区相对路径"},
						"old": {"type": "string", "description": "待替换的文本（必须在文件中唯一）"},
						"new": {"type": "string", "description": "替换后的文本"}
					},
					"required": ["path", "old", "new"]
				}
			},
			"plan_id": {"type": "string", "description": "commit/rollback 时由 preview 返回的计划 ID"},
			"verify": {"type": "string", "description": "commit 可选：声明提交后将运行的验证命令（如 go test ./...），会原样记录进会话事件"}
		},
		"required": ["action"]
	}`)
}

// editFilesParams 工具参数结构。
type editFilesParams struct {
	Action string            `json:"action"`
	Edits  []editFileRequest `json:"edits"`
	PlanID string            `json:"plan_id"`
	Verify string            `json:"verify"`
}

type editFileRequest struct {
	Path string `json:"path"`
	Old  string `json:"old"`
	New  string `json:"new"`
}

// Execute 执行 edit_files 工具。
func (t *EditFilesTool) Execute(ctx context.Context, args json.RawMessage) (*tool.Result, error) {
	var params editFilesParams
	if err := json.Unmarshal(args, &params); err != nil {
		return errorResult(fmt.Sprintf("参数解析失败: %v", err)), nil
	}
	switch params.Action {
	case "preview":
		return t.preview(params), nil
	case "commit":
		return t.commit(params), nil
	case "rollback":
		return t.rollback(params), nil
	default:
		return errorResult(fmt.Sprintf("未知 action %q，可用：preview / commit / rollback", params.Action)), nil
	}
}

func errorResult(content string) *tool.Result {
	return &tool.Result{Content: content, IsError: true}
}

// plannerFor 惰性创建 Planner。构造函数不返回错误（与同包其他工具一致），
// 工作区不可用这类问题推迟到首次调用时如实报出。
//
// Root 传符号链接解析后的真实路径：edits 内部用「真实根 → 真实目标」算相对
// 路径，若 Root 仍带软链前缀（如 macOS 的 /tmp → /private/tmp），相对路径
// 计算会失败并回退成冗长路径，事件与 CLI 展示都会变得难读。
func (t *EditFilesTool) plannerFor() (*edits.Planner, error) {
	t.plannerOnce.Do(func() {
		root := t.WorkDir
		if abs, err := filepath.Abs(root); err == nil {
			if resolved, err := filepath.EvalSymlinks(abs); err == nil {
				root = resolved
			}
		}
		t.planner, t.plannerErr = edits.NewPlanner(edits.Options{
			Root:      root,
			CheckPath: t.CheckPath,
		})
	})
	return t.planner, t.plannerErr
}

// preview 生成只读预览。Prepare 阶段顺带发现"多个操作落在同一文件"时的
// 顺序冲突，但绝不写盘。
func (t *EditFilesTool) preview(params editFilesParams) *tool.Result {
	if len(params.Edits) == 0 {
		return errorResult("preview 需要至少一个 edits 条目")
	}
	planner, err := t.plannerFor()
	if err != nil {
		return errorResult(fmt.Sprintf("编辑工作区不可用: %v", err))
	}

	ops := make([]*edits.Operation, 0, len(params.Edits))
	for i, e := range params.Edits {
		if e.Old == "" {
			return errorResult(fmt.Sprintf("第 %d 个替换的 old 不能为空", i+1))
		}
		op, err := planner.Preview(e.Path, e.Old, e.New)
		if err != nil {
			return errorResult(fmt.Sprintf("第 %d 个替换（%s）预览失败: %v", i+1, e.Path, err))
		}
		ops = append(ops, op)
	}

	// Prepare 不写盘：解析路径、校验基线、按顺序试应用，把操作间冲突
	// 在预览阶段暴露出来，而不是留到提交时才发现改了半个文件。
	batch, err := planner.Prepare(ops...)
	if err != nil {
		return errorResult(fmt.Sprintf("计划校验失败: %v", err))
	}

	planID := derivePlanID(ops)
	t.storePlan(planID, batch)

	files := make([]session.FileEditRecord, 0, batch.Len())
	for _, fc := range batch.Files {
		files = append(files, session.FileEditRecord{
			Path:  fc.RelPath,
			Op:    "replace",
			State: "preview",
		})
	}
	t.emit(&session.FileEditedData{
		Phase:   "preview",
		PlanID:  planID,
		Files:   files,
		Verify:  params.Verify,
		Summary: fmt.Sprintf("预览 %d 个文件、%d 处替换（未写入磁盘）", batch.Len(), len(ops)),
	})

	var b strings.Builder
	fmt.Fprintf(&b, "计划 %s：共 %d 个文件、%d 处替换（只读预览，未写入磁盘）\n", planID, batch.Len(), len(ops))
	for _, fc := range batch.Files {
		for _, op := range fc.Ops {
			fmt.Fprintf(&b, "\n--- %s (操作 %s)\n", fc.RelPath, shortID(op.ID))
			for _, line := range op.Diff {
				b.WriteString(line.Kind.Prefix())
				b.WriteString(line.Text)
				b.WriteByte('\n')
			}
		}
	}
	fmt.Fprintf(&b, "\n确认无误后调用 edit_files action=commit plan_id=%s 提交；", planID)
	b.WriteString("提交前文件若被外部改动，对应文件会以 conflict 结局拒绝写入。")
	return &tool.Result{Content: b.String()}
}

// commit 提交计划。拒绝重复提交：同一 plan_id 第二次 commit 直接报错。
// 即使不靠这个检查，基线哈希比对也会让"对已提交文件再提交同一计划"以
// conflict 收场（文件内容已不等于基线）——两层防线，一层都不能少。
func (t *EditFilesTool) commit(params editFilesParams) *tool.Result {
	if params.PlanID == "" {
		return errorResult("commit 需要 plan_id（来自 preview 的返回）")
	}
	planner, err := t.plannerFor()
	if err != nil {
		return errorResult(fmt.Sprintf("编辑工作区不可用: %v", err))
	}

	rec, ok := t.lookupPlan(params.PlanID)
	if !ok {
		return errorResult(fmt.Sprintf("计划 %s 不存在或已被清理（进程重启后需重新 preview；过期上限 %d 个计划）", params.PlanID, maxTrackedPlans))
	}
	if rec.rolledBack {
		return errorResult(fmt.Sprintf("计划 %s 已撤销，需要重新 preview 生成新计划后再提交", params.PlanID))
	}
	if rec.committed != nil {
		return errorResult(fmt.Sprintf("计划 %s 已提交过，拒绝重复提交；如需回退用 action=rollback", params.PlanID))
	}

	result, err := planner.Commit(rec.batch, edits.CommitOptions{})
	rec.committed = result // 部分成功也保留，rollback 只恢复已写文件

	phase := "committed"
	if err != nil {
		phase = "failed"
	}
	files := make([]session.FileEditRecord, 0, len(result.Outcomes))
	for _, o := range result.Outcomes {
		files = append(files, session.FileEditRecord{
			Path:  o.RelPath,
			Op:    "replace",
			State: string(o.State),
			Err:   o.Err,
		})
	}
	if t.Tracker != nil {
		for _, o := range result.Written() {
			t.Tracker.TrackEdit(o.RelPath)
		}
	}
	summary := "（无结果）"
	if result != nil {
		summary = result.Summary()
	}
	t.emit(&session.FileEditedData{
		Phase:   phase,
		PlanID:  params.PlanID,
		Files:   files,
		Verify:  params.Verify,
		Summary: summary,
	})

	if err != nil {
		content := summary
		if params.Verify != "" {
			content += fmt.Sprintf("\n验证命令（未执行，仅记录）：%s", params.Verify)
		}
		return errorResult(content)
	}

	content := summary
	if params.Verify != "" {
		content += fmt.Sprintf("\n验证命令（未执行，仅记录）：%s", params.Verify)
	}
	content += fmt.Sprintf("\n如需回退本次提交：edit_files action=rollback plan_id=%s", params.PlanID)
	return &tool.Result{Content: content}
}

// rollback 撤销一次提交。撤销前内部会比对"本次写入结果哈希"，文件被二次
// 修改的会以 skipped_modified 跳过，不会用旧快照覆盖用户的新工作。
func (t *EditFilesTool) rollback(params editFilesParams) *tool.Result {
	if params.PlanID == "" {
		return errorResult("rollback 需要 plan_id")
	}
	rec, ok := t.lookupPlan(params.PlanID)
	if !ok {
		return errorResult(fmt.Sprintf("计划 %s 不存在或已被清理", params.PlanID))
	}
	if rec.committed == nil {
		return errorResult(fmt.Sprintf("计划 %s 没有可撤销的提交（先 commit 才有 rollback）", params.PlanID))
	}
	if rec.rolledBack {
		return errorResult(fmt.Sprintf("计划 %s 已撤销过，不能重复撤销", params.PlanID))
	}

	rb, err := rec.committed.Rollback(edits.CommitOptions{})
	rec.rolledBack = true

	files := make([]session.FileEditRecord, 0, len(rb.Outcomes))
	for _, o := range rb.Outcomes {
		files = append(files, session.FileEditRecord{
			Path:  o.RelPath,
			Op:    "replace",
			State: string(o.State),
			Err:   o.Err,
		})
	}
	if t.Tracker != nil {
		for _, o := range rb.Outcomes {
			if o.State == edits.RollbackReverted {
				t.Tracker.TrackEdit(o.RelPath)
			}
		}
	}
	t.emit(&session.FileEditedData{
		Phase:   "rolled_back",
		PlanID:  params.PlanID,
		Files:   files,
		Summary: rb.Summary(),
	})

	if err != nil {
		return errorResult(rb.Summary())
	}
	return &tool.Result{Content: rb.Summary()}
}

// storePlan 登记计划并按 FIFO 淘汰最旧记录，防止长会话内存无界增长。
func (t *EditFilesTool) storePlan(planID string, batch *edits.Batch) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.plans == nil {
		t.plans = make(map[string]*editPlanRecord)
	}
	if _, exists := t.plans[planID]; !exists {
		t.order = append(t.order, planID)
	}
	t.plans[planID] = &editPlanRecord{id: planID, batch: batch}
	for len(t.order) > maxTrackedPlans {
		oldest := t.order[0]
		t.order = t.order[1:]
		delete(t.plans, oldest)
	}
}

func (t *EditFilesTool) lookupPlan(planID string) (*editPlanRecord, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	rec, ok := t.plans[planID]
	return rec, ok
}

// ApprovalDetails 返回已保存计划的真实路径与差异，供权限审批卡片展示。
// 未知 plan_id 或非编辑调用返回空；这不会放宽权限检查。
func (t *EditFilesTool) ApprovalDetails(toolName string, args map[string]interface{}) ([]string, string) {
	if toolName != "edit_files" {
		return nil, ""
	}
	planID, _ := args["plan_id"].(string)
	if planID == "" {
		return nil, ""
	}
	rec, ok := t.lookupPlan(planID)
	if !ok || rec.batch == nil {
		return nil, ""
	}
	paths := rec.batch.RelPaths()
	var b strings.Builder
	for _, fc := range rec.batch.Files {
		for _, op := range fc.Ops {
			fmt.Fprintf(&b, "--- %s (操作 %s)\n", fc.RelPath, shortID(op.ID))
			for _, line := range op.Diff {
				b.WriteString(line.Kind.Prefix())
				b.WriteString(line.Text)
				b.WriteByte('\n')
			}
		}
	}
	return paths, b.String()
}

// emit 落一条编辑事实事件。失败只记日志：事件是审计线索，不阻断编辑流程。
func (t *EditFilesTool) emit(data *session.FileEditedData) {
	if t.Sink == nil {
		return
	}
	// 工作区归属（CTX-001）：同一份会话存储可能被多个工作区共用，
	// 不打戳的话事实无法隔离，会把 A 工作区的改动算到 B 头上。
	data.WorkspaceID = session.WorkspaceID(t.WorkDir)
	if err := t.Sink.AppendEditEvent(data); err != nil {
		log.Printf("[edit_files] 编辑事件记录失败 (plan=%s phase=%s): %v", data.PlanID, data.Phase, err)
	}
}

// derivePlanID 从操作集合派生稳定的计划 ID。
// 操作 ID 本身由 路径+基线哈希+替换内容 派生，因此同一份计划重新预览
// 得到同一个 plan_id，可用于跨事件串联；不同计划必然不同 ID。
func derivePlanID(ops []*edits.Operation) string {
	h := sha256.New()
	for _, op := range ops {
		h.Write([]byte(op.ID))
		h.Write([]byte{0})
	}
	return "plan-" + hex.EncodeToString(h.Sum(nil))[:12]
}

// shortID 取哈希前 8 位用于展示。
func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}
