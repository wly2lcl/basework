# REUSE-001 验证记录：公共能力上移评估与 API 契约

## 基线

- 起点 HEAD：`ccb3767`（docs: plan M10 reusable capability promotion），工作树干净。
- 环境：macOS arm64（darwin/arm64）/ go1.26.10；本机无 Windows 运行时，Windows 结论只记录交叉编译与源码清单。
- 决策记录：[ADR 0008](../../adr/0008-promote-internal-capabilities-to-pkg.md)，状态 Accepted。
- 本任务只做评估，不移动代码、不改公共符号、不调整 CI。

## 逐包盘点

### internal/jobs → pkg/jobs

- 文件 14 个（含测试 7 个）：`jobs.go`、`journal.go`、`output.go`、`process.go`、`process_unix.go`、`process_windows.go`、`process_other.go` 及对应测试。
- 构建标签：`process_unix.go` / `process_windows.go` / `process_other.go` 分别为 `unix`、`windows`、`!unix && !windows`，对应测试同标签。
- 模块内依赖：仅 `pkg/session`（`journal.go`，JSONL 记录格式与其一致）。`go list -deps` 中 `pkg/llm` 经由 `pkg/session` 传递，不是直接依赖。
- 导出面（21 项）：`Manager`/`New`/`Options`/`Job`/`State`/`Stream`/`Record`/`RecordStarted` 等常量、`Journal`/`JSONLJournal`/`NewJSONLJournal`、`OutputOptions`/`OutputPage`/`SpillFile`/`SpillOpener`、`Runner`/`SinkRunner`、`PrepareCommand`/`TerminateCommand`/`ProcessTreeTerminationSupported`、`ErrNotFound`/`ErrOutputExpired`/`ErrJournal`/`ErrProcessTreeUnsupported`。
- 消费方：`internal/runtime/service.go`；`internal/tools/{background,jobs_tools}.go` 与测试；`cmd/basework/{jobs,facts,resume,runtime,tui}.go` 与测试；`tests/benchmark/long_session_bench_test.go`。
- 行为不变量：job 终态恰好一次发布；`interrupted` 不自动重放；owner 必填且隔离；输出配额与溢出文件边界不退化；`CloseOwner` 只影响该 owner。

### internal/edits → pkg/edits

- 文件 4 个（含测试 2 个）：`plan.go`、`commit.go`、`plan_test.go`、`commit_test.go`。
- 构建标签：无。
- 模块内依赖：**零**，只用标准库；路径权限通过 `Options.CheckPath` 回调注入。
- 导出面：`Planner`/`NewPlanner`/`Options`/`Operation`/`Batch`/`CommitOptions`/`CommitResult`/`RollbackResult`/`FileChange`/`FileOutcome`/`DiffLine`/`LineKind`/`CommitState`/`RollbackState`/`ErrNoOperations`/`ErrOutsideWorkspace` 及错误值。
- 消费方：仅 `internal/tools/edit_files.go`（预览/提交/撤销与 `file.edited` 事实）。
- 行为不变量：预览不写盘；逐文件原子、跨文件非事务；提交阶段重新检查权限与基线；撤销遇到后来编辑按文件跳过并给原因。

### internal/permission → pkg/permission

- 文件 18 个（含测试 5 个，其中 2 个带 `sqlite` 标签）。
- 构建标签：`sqlite` 作用于 `sqlite_store.go`、`audit_sqlite.go`、`migration.go` 与对应测试。
- 模块内依赖：核心文件**零**模块内 import；`pkg/agent` 只出现在 `adapter.go`（`PermissionAdapter`、`PathPermissionAdapter`）。
- 导出面：`Checker`/`NewChecker`/`NewCheckerWithStore`/`NewCheckerFromConfig`、`Mode`/`ParseMode`、`Rule`/`MatchRules`/`RuleApplies`/`RuleScope`/`ScopeRank`/`StoredRule`/`ScopeContext`、`PathChecker`/`NewPathChecker`/`NormalizePath`/`DefaultSensitivePaths`/`ProtectionLevel`/`ParseProtectionLevel`、`Cache`/`CacheKey`/`Store`/`SQLiteStore`/`NewSQLiteStore`、`AuditLogger`/`AuditRecord`/`ExportRules`/`ImportRules`/`VerifyImport`、`ApprovalBroker`/`ApprovalRequest`/`BuildApprovalRequest`/`ApprovalPromptFunc`/`PromptFunc`/`RiskFunc`/`Decision`/`DefaultApprovalTimeout`/`ErrBrokerClosed`，以及适配器 `PermissionAdapter`/`PathPermissionAdapter`（留 `internal`）。
- 消费方：`internal/tools/{applypatch,question}.go` 与测试；`cmd/basework/{permission,runtime,runtime_permission_sqlite,tui,facts}.go` 与测试；`tests/permission_migration_test.go`。
- 行为不变量：默认拒绝；`global/session/project` 优先级；缺上下文 fail-closed；缓存按精确参数匹配；SQLite 旧库可读；审批超时与审计字段不变；路径规则不是 OS 沙箱。

### internal/observability → pkg/observability

- 文件 12 个（含测试 3 个）：`logger.go`、`cost.go`、`tokens.go`、`event.go`、`bus.go`、`subscriber.go`、`config.go`、`pprof.go` 及测试与 `adapter.go`。
- 构建标签：无。
- 模块内依赖：核心文件**零**模块内 import；`pkg/agent` 只出现在 `adapter.go`（`EventBusAdapter`）。
- 导出面：`Logger`/`LogLevel`/`ParseLogLevel`、`CostTracker`/`TokenTracker`、`Event`/`NewEvent`/事件常量、`EventBus`/`EventHandler`/`Subscriber`/`LogSubscriber`/`MetricsSubscriber`/`SetupSubscribers`、`Config`、`Profiler`/`ProfilerConfig`/`DefaultProfilerConfig`，以及 `EventBusAdapter`（留 `internal`）。
- 消费方：`cmd/basework/{runtime,profile}.go`；`tests/benchmark/token_bench_test.go`。
- 行为不变量：日志格式与字段不变；成本/token 口径不变；订阅有界、panic 隔离、关闭后不再投递；pprof 默认关闭。

## 移动前测试基线（darwin/arm64，go1.26.10）

命令：`go test -list '.*' ./<pkg>`，取 `^Test` 行排序后计算 sha256，用于移动后逐字比对。

| 包 | 用例数 | 排序清单 sha256 |
|---|---|---|
| internal/jobs | 60 | `5f6d9ab22f21a2fe71962dd21a543770b40c9a922384162054ead06b0d538b21` |
| internal/edits | 36 | `b7b3b1018032acd649ef1c1cdce7409dcb218a51913efb6edbaab1a9fe428c5d` |
| internal/permission | 57 | `502ba28d855d69c29116ad4db05277ac2be3c0200d31133eb0a523d37ff556d7` |
| internal/permission（`-tags sqlite`） | 81 | `14ac9a0d6cbe9d8b57cc3d5e901e6866d2eb2c4218171ece7218a1fd78be801a` |
| internal/observability | 68 | `c9bde345b5c7b68ea6b2b209eaf35c38e1b253517eebc55fffe17f9b1cd910af` |
| internal/observability（`-tags sqlite memory`） | 68 | 同上（本包无标签文件） |

平台专属用例按源码清单核对：`jobs` 的 `process_unix_test.go`（unix）、`process_windows_test.go`（windows）、`process_other_test.go`（其他平台）；权限的 `store_test.go`/`audit_test.go` 受 `sqlite` 标签控制。本机只能执行 unix 分支，Windows 分支记录为交叉编译 + 源码清单，不冒充运行结论。

## 首版公共 API 与生命周期

- 构造函数全部走 `Options` 或显式参数，不新增 `Set*` 包级全局；`tests/arch_test.go` 的 `injectionWhitelist` 不变。
- `jobs.Manager`：`New(Options)` 创建，`Close()` 释放；`CloseOwner` 只清理单个 owner；并发安全的边界见包契约。
- `permission`：`NewChecker`/`NewCheckerWithStore` 创建，`Checker` 并发安全；`ApprovalBroker` 需在退出前 `Close`；`Cache` 可按 session 隔离。
- `observability`：`NewLogger`/`NewCostTracker`/`NewEventBus`/`NewProfiler` 均实例化；`Profiler` 默认关闭，`Close`/`Stop` 由调用方负责。
- 实例作用域矩阵见 ADR 0008；REUSE-006 用同进程双实例测试验证互不污染。

## 兼容层级

四个新包首版标记「实验 API（0.x）」：不纳入 `ARCHITECTURE.md` 现有核心/扩展 API 的 SemVer 承诺；1.0 前破坏性变更须走 ADR 并在 `CHANGELOG.md` 写迁移方式；稳定后归入扩展 API。REUSE-006 负责把该层级写入 `ARCHITECTURE.md`。

## 迁移映射与回退

| 源 | 目标 | 消费方改动 | 回退 |
|---|---|---|---|
| `internal/jobs/*` | `pkg/jobs/*` | `internal/runtime`、`internal/tools`、`cmd/basework`、`tests/benchmark` 改 import | `git mv` 回退该包 commit，其他包不受影响 |
| `internal/edits/*` | `pkg/edits/*` | `internal/tools/edit_files.go` 改 import | 同上 |
| `internal/permission/*`（除 `adapter.go`） | `pkg/permission/*` | `internal/tools`、`cmd/basework`、`tests` 改 import；`adapter.go` 留 `internal/permission` 并改引用 `pkg/permission` | 同上 |
| `internal/observability/*`（除 `adapter.go`） | `pkg/observability/*` | `cmd/basework`、`tests/benchmark` 改 import；`adapter.go` 留 `internal/observability` | 同上 |

每个包一个 commit，移动 commit 只允许 import 路径与包注释变化；可见性收窄若需要，单独提交。

## 变更

本任务不移动代码，只产出决策与基线：

- 新增 [ADR 0008](../../adr/0008-promote-internal-capabilities-to-pkg.md)：四包上移、适配层归属、实例作用域矩阵、实验 API 兼容层级与行为不变量。
- 新增本证据文件：逐包导出面/依赖/消费方盘点、移动前测试清单基线、迁移映射与按包回退方案。
- `docs/TASKS.md` 中 REUSE-001 置为完成，其余五个任务保持待办。

## 验证

```
go list -deps ./internal/jobs ./internal/edits ./internal/permission ./internal/observability
```

结果与上文「模块内依赖」一致：jobs 仅 `pkg/session`（传递 `pkg/llm`），edits 为零，permission 与 observability 仅 `adapter.go` 引入 `pkg/agent`。

```
make check-docs
→ 通过（ADR 0008 三节齐备；本文件链接有效）
```

## 剩余与交接

- 未移动任何代码；REUSE-002 起按 ADR 0008 执行，每包移动后重新计算测试清单 sha256 与本记录比对。
- Windows 真实运行结论与五平台 CI 留到 REUSE-006；本任务只记录交叉编译计划。
- 无未决问题。

## 结论

四个包的边界、适配层归属、实例作用域与兼容层级已定稿并可执行：核心实现上移 `pkg`，`pkg/agent` 适配器留 `internal`，首版为实验 API。移动前测试基线（60/36/57/68，sqlite 81）已记录，可作为后续纯移动提交的对照。REUSE-002 可开始。

