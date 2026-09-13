# EDIT-002 验证记录：批量提交与安全撤销

## 基线

- 起点 HEAD：`935fde1`；工作树含 BASE-001 / REL-001 / REL-002 / REL-004 / JOB-001 / JOB-002 / JOB-003 / EDIT-001 的未提交改动。
- 环境：macOS 26.6.2 arm64 / go1.26.0 / APFS。
- 相关既有实现核对结果（按任务卡的"先读"范围逐项核对）：
  - `internal/edits/plan.go`（EDIT-001 新增）只有 `Preview` / `ValidateBase` / `ResolvePath`，
    包注释明确写「本包只做计划，不做提交」。没有提交、没有撤销。
  - `pkg/tool/builtin/edit.go`、`write.go` 是「检查 → 读 → 改 → 写」一条路写到底：
    没有批准后复查、没有多文件批次、没有冲突处理、没有撤销，写失败时也不会告诉你
    "前几个文件已经改了"。
  - `internal/tools/` 里没有任何批量编辑的提交/回滚路径。
  - `pkg/session/` 没有编辑事件（把 edit 事实写进会话事件属 EDIT-003）。
- 因此本任务的起点是"能预览、不能安全提交、不能安全撤销"。

## 变更

### 新增 `internal/edits/commit.go`

**准备阶段 `Planner.Prepare(ops...)`**——把一组操作按文件归并，并在写盘之前暴露冲突：

- 每个操作的目标**重新解析一次**（走 `ResolvePath`），因此"工作区边界 / 软链不逃逸 /
  权限允许"三项在准备阶段就再查一遍；预览到提交之间目录可能被换成软链。
- 同一文件的多个操作**必须共享同一个基线哈希**，否则它们本来就是互斥的计划。
- 基线按**内容哈希**比对（沿用 EDIT-001 的口径，不看 mtime）。
- **按顺序试应用**：多操作落在同一文件上时，逐个独立应用并不等价于按顺序应用——
  后面的操作可能因为前面的替换而找不到匹配（`ErrNoMatch`），或者因为前面的新文本
  恰好包含它的搜索文本而变得不唯一（`ErrAmbiguousMatch`）。这两种情况在准备阶段
  就被拒绝，因此不会出现"改了半个文件"。
- **单操作时有自校验**：提交路径算出的内容必须与预览给出的 `NewContent` 逐字节相同。
  这不是形式主义——它在实现过程中真的抓到了一个 bug（见下）。
- 整个准备阶段不写盘。

**提交阶段 `Planner.Commit(batch, opts)`**：

- 每个文件在写入**之前**重新做三件事，顺序不可换：
  1. 重新解析路径与权限（`ResolvePath`）；
  2. 重新读取并比对基线哈希——**冲突时不写**，绝不覆盖用户的新改动；
  3. 权限沿用原文件（不靠 umask 猜），用"同目录临时文件 + `Chmod` + `Sync` + `rename`"
     做**逐文件原子**替换。临时文件必须与目标同目录：跨文件系统的 `rename` 会退化成
     复制 + 删除，那就不再是原子的。
- 记录**每一个文件**的结局：`written` / `conflict` / `failed` / `not_attempted`，
  并带上失败原因与它涉及的操作 ID。`StopOnError` 决定"首个失败后是否继续尝试"，
  两种策略下清单都是完整的。
- 返回 `CommitResult` + 包装了 `ErrBatchPartial` 的错误：调用方既能拿到准确清单，
  也能拿到"没全成功"的信号。
- **不声称跨文件事务**：没有两阶段提交、没有全局锁，N 个文件就是 N 次独立 rename。
  中间任何时刻中断都会留下"前 k 个已改、后面的没改"的磁盘状态，所以本包的做法是
  把清单给全，而不是宣称原子性。

**撤销 `CommitResult.Rollback(opts)`**：

- 只处理**本次确实写入**的文件；冲突/失败/未尝试的文件标记为 `not_written`。
- 恢复前重新读文件并比对**本次写入结果哈希**。不等就跳过并说明（`skipped_modified`
  带"不覆盖"原因）——用户在提交后又编辑了它，此时恢复旧内容就是用旧快照覆盖新工作。
- 按提交的**相反顺序**恢复（依赖顺序上更安全的方向）。
- 恢复失败逐个标记 `failed` 并返回错误，不宣称"已撤回全部"。
- 撤销依赖内存中保留的原始字节；**跨进程撤销需要持久化，本包不假装支持**。

### 文档

- `docs/STATUS.md`：编辑能力一行补上批量提交与撤销，写明"跨文件不构成事务、
  撤销仅限进程内"。
- `docs/development/evidence/EDIT-002.md`（本文件）。

### 范围说明（任务卡"范围外"条款）

- 未改动 `pkg/tool/builtin/` 与 `pkg/session/`：本任务不需要改它们的公共 API，
  也没有顺手改。`git status --short pkg/tool/` 为空。
- 新增公共 API 仅限 `internal/edits`（内部包，不被 `pkg/` 依赖）：
  `Prepare` / `Commit` / `Rollback` / `Batch` / `CommitResult` / `RollbackResult` 及
  相关错误值。它们都是验收项（冲突不覆盖、准确清单、安全撤销）的必要条件，
  没有额外改动存储格式或默认权限。

## 验证

针对性验证（任务卡指定，仓库根执行）：

```
go test ./pkg/tool/builtin ./internal/tools ./pkg/session -count=1
→ ok github.com/wly2lcl/basework/pkg/tool/builtin  1.515s
→ ok github.com/wly2lcl/basework/internal/tools    5.667s
→ ok github.com/wly2lcl/basework/pkg/session       0.759s

go test -race ./internal/edits -count=1     # 本任务新增代码所在包
→ ok github.com/wly2lcl/basework/internal/edits  1.598s
```

任务卡把范围写成 `internal/tools/`、`pkg/tool/builtin/`、`pkg/session/`，但实际实现的
归属文件是 EDIT-001 新建的 `internal/edits/`（卡中允许"拟新增路径不存在属正常"），
所以两处都跑了：既跑卡里点名的三个包确认没有被破坏，也跑新代码所在包。

`internal/edits/commit_test.go`（20 条）：

| 测试 | 覆盖的验收点 |
|---|---|
| `TestPrepare_GroupsOpsByFileWithoutWriting` | 同文件多操作归并成一个写入单元（结果 = 顺序应用后的内容），准备阶段全程不写盘 |
| `TestPrepare_DetectsCrossOpConflict` | 操作间冲突（第二个操作在前一个之后找不到匹配）在写盘前被拒 |
| `TestPrepare_DetectsAmbiguousAfterFirstOp` | 操作间冲突的另一种形态：前一个操作让后一个的搜索文本变多义 → 拒 |
| `TestPrepare_RejectsStaleBaseline` | 基线在准备阶段就比对（内容哈希），且不覆盖用户的新内容 |
| `TestPrepare_SingleOpResultMatchesPreview` | 提交算法与预览算法不漂移 |
| `TestPrepare_RejectsEmptyInput` | 空操作 / nil 操作有明确返回 |
| `TestCommit_WritesAllFilesAndCleansUpTemp` | 正常提交：内容正确、**权限沿用原文件**（0600/0755）、无临时文件残留 |
| `TestCommit_ConflictDoesNotOverwriteUserEdit` | **冲突时不覆盖用户新改动**；冲突文件保留用户内容，后续文件记 `not_attempted`；不能宣称成功 |
| `TestCommit_PartialFailureListsExactlyWhatHappened` | **注入第 2 个文件写失败**：1 写 / 1 失败 / 1 未尝试，清单与实际磁盘状态逐一对齐，摘要含三种状态 |
| `TestCommit_StopOnErrorFalseKeepsGoing` | 另一种策略：继续尝试后续文件，清单同样准确 |
| `TestCommit_RechecksPermissionAfterApproval` | **批准后再次校验权限**：权限被收回 → 记 `failed` 且不写盘 |
| `TestCommit_DetectsSymlinkSwapAfterApproval` | 批准后目标被换成指向工作区外的软链 → 拒绝，且工作区外文件未被改 |
| `TestRollback_RevertsWrittenFiles` | 撤销把已写文件恢复原样（2/2） |
| `TestRollback_SkipsFileModifiedAfterCommit` | **撤销不覆盖后来编辑**：提交后被改过的文件标记 `skipped_modified` 并保留用户内容，另一个文件正常恢复 |
| `TestRollback_MarksNeverWrittenFiles` | 冲突/未尝试的文件在撤销时标记 `not_written`，不被改动 |
| `TestRollback_ReportFailureWhenWriteFails` | 撤销写失败被如实标记 `failed` 并返回错误，不宣称已恢复 |
| `TestCommitAndRollback_PreserveBOMAndCRLF` | BOM + CRLF 在提交与撤销两端都保真；撤销后逐字节还原 |
| `TestCommit_AppliesMultipleOpsInOrder` | 同文件多操作**只写一次**，结局回指全部操作 ID |
| `TestCommit_LeavesNoExtraFilesBehind` | 提交后目录里只有目标文件（原子替换不留临时文件） |
| `TestCommit_RejectsEmptyBatch` | 空批次、nil 批次、nil 结果的撤销都有明确错误 |

测试非恒真的证据（本轮实现中真实出现并被修正的失败）：

1. **单操作自校验抓到了一个真 bug**：`Prepare` 用 `decodeText` 解码时会摘掉 UTF-8 BOM，
   而写回时没有补上，于是"预览说内容不变、提交却把 BOM 删了"。这个 bug 由
   `TestCommitAndRollback_PreserveBOMAndCRLF` 直接暴露（报"提交结果与预览不一致"）。
   若没有这条自校验断言，一次普通编辑就会静默改变带 BOM 文件的读取行为。
   已在 `Prepare` 中按 `hadBOM` 原样补回。
2. 测试辅助函数与 EDIT-001 的 `plan_test.go` 重名（`writeFile` / `newPlanner`）导致
   编译失败，改为复用既有辅助函数 + 新增 `writeFileMode`（权限可控）后通过。

全量门禁与生成物新鲜度见 `## 结论`。

## 剩余与交接

- **CLI 审阅闭环与编辑事实（EDIT-003）**：`Prepare`/`Commit`/`Rollback` 目前只有内部
  API 与测试使用；没有把 plan/commit/result/undo 写进会话事件，也没有 CLI 展示差异与
  结果。批准/拒绝策略仍由调用方（未来的 CLI）决定，本包只保证"批准之后再校验一遍"。
- **跨进程撤销不支持**：`CommitResult` 把原始内容留在内存里，进程结束即丢失。
  要让撤销跨重启可用，需要把原始内容或撤销引用持久化（属 EDIT-003 的编辑事件）。
- **跨文件不是事务**：如前所述，这是设计选择而非遗漏。若将来需要"全成功或全不改"，
  需要引入预写日志 + 恢复流程，属于独立任务。
- **未接线到工具层**：`internal/tools/` 里没有调用 `Prepare`/`Commit` 的工具，
  模型目前还看不到"批量编辑"这个能力。这属于 EDIT-003。
- **大文件的撤销内存代价**：批次里每个已写文件都保留一份原始内容。文本编辑场景下可接受，
  但没有做大小上限；若将来支持大文件编辑，需要改成备份文件 + 引用。
- **未验证范围**：文件系统语义只在本机 APFS 上验证；网络文件系统上 `rename` 的原子性、
  Windows 上 `Chmod` 的语义差异都未实测。没有做断电/崩溃注入测试。

## 结论

验收项逐条对照：

| 验收条件 | 结果 | 依据 |
|---|---|---|
| 冲突时不覆盖用户新改动 | 通过 | 写入前重新比对内容哈希，冲突即记 `conflict` 且不写；`TestCommit_ConflictDoesNotOverwriteUserEdit`、`TestCommit_RechecksPermissionAfterApproval`、`TestCommit_DetectsSymlinkSwapAfterApproval` |
| 中途失败有准确已写/未写列表 | 通过 | `CommitResult` 逐个文件记录 `written`/`conflict`/`failed`/`not_attempted` + 原因 + 操作 ID；`TestCommit_PartialFailureListsExactlyWhatHappened`（注入第 2 个文件写失败，逐项对齐磁盘状态）、`TestCommit_StopOnErrorFalseKeepsGoing`、`TestCommit_ConflictDoesNotOverwriteUserEdit` |
| 撤销不覆盖后来编辑 | 通过 | 恢复前比对本次写入结果哈希，不等则 `skipped_modified` 并保留用户内容；`TestRollback_SkipsFileModifiedAfterCommit`、`TestRollback_MarksNeverWrittenFiles` |
| 不宣称跨文件全局原子事务 | 通过 | 实现是"逐文件原子 + 完整清单"，无两阶段提交；`TestCommit_PartialFailureListsExactlyWhatHappened` 直接断言"部分成功"就是可观测的磁盘状态；文档与 `STATUS.md` 同步写明该边界 |
| 批准后再次校验基线与权限 | 通过 | 提交阶段重新 `ResolvePath`（含权限）+ 重新比对哈希；`TestCommit_RechecksPermissionAfterApproval`、`TestCommit_DetectsSymlinkSwapAfterApproval`、`TestPrepare_RejectsStaleBaseline` |

针对性验证（含 `-race`）通过，新增 20 条用例覆盖全部验收条件。
未实现的部分（编辑事件与 CLI 闭环、跨进程撤销、跨文件事务）已在"剩余与交接"逐条写明，
未以"编译通过"冒充完成。
