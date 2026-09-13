# EDIT-003 编辑事实与 CLI 审阅闭环

## 基线

- 看板：EDIT-002 已完成（前置 EDIT-002 满足）；EDIT-001 的只读预览契约（`internal/edits`）与 EDIT-002 的批量提交/安全撤销就绪。
- 关键基线事实：`internal/edits` 在本任务之前**没有任何产品代码引用**（只有测试与文档）——EDIT-001/002 建立的预览/提交/撤销能力尚未接入产品，本任务是接线闭环。
- 事件系统基线：`pkg/session` 有 15 种事件类型；投影（`ProjectMessages`）对未知类型默认忽略，`EventFilter.Types` 是普通过滤——新增事件类型是**可加变更**，按契约文档规则「不改变投影输出则无需升 SchemaVersion」。
- 批准策略基线：Agent 级 `agent.WithPermissionChecker(PathPermissionAdapter)` 对每次工具调用做 Check（模式化提示/放行）；路径级 `permission.PathChecker` 做敏感路径拦截。edit 工具两者都继承。

## 变更

新增文件：

- `internal/tools/edit_files.go`：`edit_files` 工具，把 `internal/edits` 的「预览 → 批准 → 提交 → 撤销」暴露为模型可调用流程。
  - `action=preview`：逐条 `Planner.Preview`（diff 来自真实字节）+ `Planner.Prepare`（跨操作冲突在预览期暴露），**不写盘**；返回 plan_id 与上下文 diff。
  - `action=commit`：只接受 preview 给出的 plan_id；`Planner.Commit` 提交前重新解析路径（工作区边界/软链/权限三项重查）并比对基线哈希。重复提交直接拒绝（「拒绝重复提交」）；即便绕过该检查，基线哈希比对也会以 conflict 收场——两层防线。
  - `action=rollback`：`CommitResult.Rollback`，文件被二次修改的以 `skipped_modified` 跳过；重复撤销拒绝。
  - plan_id 稳定派生自操作集合（路径+基线哈希+新内容的 sha256），同一计划重新预览得到同一 ID，可用于跨事件串联。
  - 计划注册表 FIFO 淘汰（上限 32），防止长会话内存无界；过期的需重新 preview。
  - `verify` 参数：提交方声明的验证命令**原样记录进事件、不执行**（记录≠放行≠验证）。
  - Planner 的 Root 用符号链接解析后的真实路径：否则 macOS `/tmp → /private/tmp` 类环境下 `filepath.Rel` 失败，事件与 CLI 里的路径会退化为冗长的 cwd 相对路径（测试抓到的真问题）。
- `internal/tools/edit_files_test.go`：10 条测试（预览不写盘、提交落盘+事件、重复提交拒绝、外部改动冲突不覆盖、撤销恢复+保护用户二次编辑、未提交撤销拒绝、越界/未知计划/未知 action、plan_id 稳定性、FIFO 淘汰、事件失败不阻断编辑）。
- `cmd/basework/edits.go`：CLI 审阅入口与运行时事件适配器。
  - `runtimeEditEventSink`：把编辑事实落成会话事件（会话未建立时如实报错）。
  - `basework edits list [--session] [--limit]`：列出全部编辑事实（阶段/文件数/结局），并用 `FileTracker` 汇总会话涉及的文件。
  - `basework edits show <plan_id>`：单计划的逐阶段明细（逐文件结局、失败原因、验证命令）。
  - **全程只读**：只构造 Store + 读事件，不重放、不重跑、不补写。
- `cmd/basework/edits_test.go`：端到端测试——编辑 → 事件落盘 → 重开 Store（模拟重启）→ 审阅记录仍可用；且重启后重放同一编辑被机制性拒绝（old 文本已不存在，preview 即失败）。

修改文件：

- `pkg/session/event.go`（可加变更）：新增 `EventFileEdited`（`file.edited`）与 `FileEditedData`/`FileEditRecord`（phase、plan_id、逐文件 state/err、verify、summary），补 `DecodeData` 分支。不改变 `ProjectMessages` 输出，未升 SchemaVersion。
- `pkg/session/filetrack.go`：新增 `TrackOp(path, op)` 便捷入口（未知类型按 edit 记：宁可多记不可漏记）。
- `cmd/basework/runtime.go`：`runtimeJobWiring` 增 `editTool` 字段（零值=子代理不注册）；`newRuntimeAgent` 构造 `EditFilesTool`，路径检查用与 builtin 同一份 `pathChecker`，事件 sink 指向会话存储，Tracker 随会话生命周期。
- `cmd/basework/main.go`：注册 `editsCmd`。
- `cmd/basework/runtime_test.go`：+2 条（wiring 决定注册与否；拦截路径确认走运行时 pathChecker）。
- `docs/reference/pkg/session.md`：事件速览表补 `file.edited`（编辑事实类，不投影为消息），类型数 15→16。

关键决策：

- 事件落在会话存储而不是新文件：复用既有持久化、过滤与迁移链，会话恢复后天然可用。
- `verify` 只记录不执行：验证命令的执行属 RUN 阶段职责，这里先把「改了什么」与「怎么验证」关联起来。
- apply_patch 保留不动：它是快捷入口，本工具是可审阅路径；两者分工在工具注释中写明。

## 验证

针对性验证（任务卡口径，均通过）：

- `go test -tags "sqlite memory" ./cmd/basework ./internal/tools ./pkg/session -count=1`：全绿（tools 9.4s、session 1.9s、cmd 1.7s）。
- 人工场景（真实二进制，`HOME` 隔离在 `/private/tmp/edit003/home`，种子会话含 5 条 file.edited 事件）：
  - `edits list`：5 条事实按计划/阶段/结局正确列出，汇总 3 个涉及文件，退出码 0。
  - `edits show plan-9f8e7d6c5b4a`：预览+失败两个阶段齐全，conflict 原因（基线不一致）可见。
  - `edits show plan-nope0000000`：非 0 退出并报「没有该计划的编辑事实」。
  - `--session seededit2026a --limit 3`：显式会话与条数限制生效。
  - 只读性：所有查询前后会话文件 SHA-256 一致。

全量门禁（均通过）：

- `go build ./... && go vet ./...`；`go build -tags "sqlite memory" ./... && go vet -tags "sqlite memory" ./...`：通过。
- `go test ./... -count=1` 与 `go test -tags "sqlite memory" ./... -count=1`：全绿。
- `go test -tags "sqlite memory" -race -count=1 ./internal/tools ./pkg/session ./cmd/basework`：全绿（本轮改动包；其余包 race 于 JOB-004 收尾时已覆盖且未被本轮改动触及）。
- `GOOS=windows go build ./...`、`GOOS=linux go build ./...`：通过。
- `gofmt -l`：无输出。
- `make check-arch`：3 项架构检查通过；`make check-docs`：markdown 93 个、链接全部有效；`make gen` 幂等（二次运行无新增 diff）。

## 剩余与交接

- **批准交互的 TUI 呈现**未做：批准策略本身已生效（Agent 级 Check + 提交前基线/权限重查 + 预览/提交两步式），但「用户在终端里逐条确认 diff」的交互体验归 UI-002（依赖本任务）。
- 真实模型凭证不可用：`edit_files` 在真实 Agent 对话中的端到端表现依赖 REL-003；本任务的工具级与 CLI 级链路已用隔离环境验证。
- `file.edited` 事件目前只由 `edit_files` 产生；apply_patch 与 builtin 写工具未落此事件（它们的可审阅化不在本任务范围，如需覆盖应另立任务）。
- 会话内 FileTracker 为进程内状态，未随会话持久化；跨重启的「涉及文件」视图由事件重建（`edits list` 已实现），Tracker 的持久化接入是 CTX-001（统一工作区事实模型）的范围。
- 撤销仍是进程内能力（重启用事件审阅、不自动回滚），边界与 EDIT-002 一致。

## 结论

EDIT-003 完成。`internal/edits` 的预览/提交/撤销能力首次接入产品：`edit_files` 工具走「预览（不写盘）→ 批准（提交前基线与权限重查）→ 提交/撤销」闭环，每一步落成 `file.edited` 会话事件；`basework edits list/show` 提供只读审阅，会话恢复后记录仍可用、同一编辑无法重复提交（机制性拒绝）。用户可看改动前后与失败原因，批准与拒绝不可绕过。全部 28 项门禁命令通过，人工场景 5 项通过。M3 阶段（EDIT-001~003）全部完成。
