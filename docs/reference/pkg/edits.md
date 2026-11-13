# pkg/edits

## 用途

编辑的**预览、批量提交与安全撤销**。存在的理由很直接：直接写盘的工具（`Edit`/`Write`）是「检查 → 读 → 改 → 写」一条路到底，中间结果无法交给人看，于是「预览」只能靠模型口述，而模型口述的 diff 不是文件事实。本包把中间结果变成一等对象，全部由磁盘真实字节推导。

- **预览** — `Planner.Preview(path, old, new)` 生成 `Operation`：基线哈希、工作区相对路径、上下文 diff、`ID`、换行风格、BOM 标记。预览全程不写盘。`ID` 由 `相对路径 | 基线哈希 | old 哈希 | new 哈希` 派生，同一计划重复生成必得同一 ID。
- **路径安全** — `ResolvePath` 依次做词法包含（挡 `../`）、符号链接解析后的包含（挡工作区内的软链指向区外）、`Options.CheckPath` 对**原始路径与解析后路径各检查一次**。预览与提交都走同一条解析路径。
- **批量提交** — `Prepare` 把多个操作按文件分组为 `Batch`；`Batch.Commit` 逐文件重解析、重比对基线、原子替换（同目录临时文件 + rename）。每个文件在写入**之前**重新检查，不信任预览结果。
- **部分成功** — `CommitResult.Outcomes` 逐文件给出 `written` / `conflict` / `failed` / `not_attempted`，`Summary()` 输出人类可读清单；即使部分失败也返回结果而不是只返回错误。
- **安全撤销** — `CommitResult.Rollback` 只恢复仍等于「提交后内容」的文件；被后来编辑过的文件按文件跳过并写明原因，不覆盖别人的修改。

## 配置

| 项 | 说明 |
|---|---|
| `Options.Root` | 工作区根目录，必填；所有目标必须落在它之内 |
| `Options.CheckPath` | 产品层路径权限回调 `func(path string) (bool, string)`；`nil` 表示只依赖工作区边界 |
| `Options.ContextLines` | diff 两侧上下文行数，`<=0` 用 3 |
| `CommitOptions.WriteFile` | 自定义写入实现；`nil` 用原子替换。用于注入「第 N 个文件写失败」 |
| `CommitOptions.StopOnError` | `true` 时首个冲突/失败后续文件记 `not_attempted`；`false` 继续尝试。两种模式都给完整清单 |

错误值均可用 `errors.Is` 判定：`ErrOutsideWorkspace`、`ErrSymlinkEscape`、`ErrPermissionDenied`、`ErrNoMatch`、`ErrAmbiguousMatch`、`ErrEmptySearch`、`ErrBinaryContent`、`ErrBaseConflict`、`ErrNoOperations`、`ErrBatchPartial`。

## 扩展点

- **权限模型**：通过 `Options.CheckPath` 注入；包内不假设任何具体权限模式，也不把审批交互带进来。
- **写入后端**：`CommitOptions.WriteFile` 可替换落盘方式（如内存文件系统、远端同步），语义要求是「失败必须报告，不得静默半写」。
- **diff 渲染**：`Operation.Diff` 是结构化 `[]DiffLine`，可自行渲染成 markdown、TUI 或 HTML。
- **基线校验**：`Operation.ValidateBase(current)` 只用内容哈希，不依赖 mtime。

## Model Experience

- 模型先得到 `plan_id` 与 diff 再决定是否提交，预览与最终写入是同一批事实，避免「模型声称改了但文件没变」。
- `commit`/`rollback` 的逐文件结局会进入会话的 `file.edited` 事实（产品层），模型在后续回合能看到哪些文件真的写入、哪些被跳过。
- 冲突返回 `ErrBaseConflict` 与具体文件，模型可以重新读取文件后再生成新计划，而不是盲目重试覆盖用户编辑。
- 撤销报告「因后来被修改而跳过」的文件，模型不会误以为已完整还原。

## Known Limitations

- **逐文件原子，跨文件非事务**：一个批次中第 2 个文件失败时，第 1 个文件已经落盘，不会自动回滚；调用方需读取 `NotWritten()` 并显式决定是否 `Rollback`。
- **撤销不是版本控制**：只恢复本次提交写入的内容，遇到后来编辑按文件跳过；不处理重命名、删除或目录级变更。
- 只支持 UTF-8 文本（可含 BOM）；`ErrBinaryContent` 明确拒绝其它编码与含 NUL 的内容。
- diff 是围绕替换点的单 hunk 上下文 diff，不是通用最小编辑距离 diff，也不跨文件合并展示。
- 依赖调用方提供正确的 `Root`；本包不是 OS 沙箱，路径规则之外的文件系统权限仍由操作系统决定。
- 本包是**实验 API（0.x）**（见 [ADR 0008](../../adr/0008-promote-internal-capabilities-to-pkg.md)）：1.0 前允许破坏性调整，但会走 ADR 并记录迁移方式。
