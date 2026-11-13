# 可复用能力上移计划（M10）

状态只在 [TASKS](../TASKS.md) 维护。本文件只描述任务步骤、边界与验收口径；实现前先读 [AI 开发流程](../development/ai-workflow.md)。

## 方向与目标

当前 `pkg/` 已有 12 个可嵌入包，但外部使用者最需要的四块可靠性能力仍在 `internal/`：

- `internal/jobs`：后台任务状态机、owner 隔离、输出配额/溢出、取消与进程树清理、JSONL 历史；
- `internal/edits`：编辑预览、基线哈希、逐文件提交、部分成功清单与安全撤销；
- `internal/permission`：interactive/yolo/deny-all、路径保护、规则作用域、审批 broker、审计与缓存；
- `internal/observability`：结构化日志、成本与 token 统计、事件总线、pprof。

按 Go 的 `internal` 规则，仓库外无法导入这四块。结果是：嵌入者只能拿到接口，自己重写电池，`pkg/` 的复用价值被显著压低。

本计划把这四块**按原行为**提升为 `pkg/jobs`、`pkg/edits`、`pkg/permission`、`pkg/observability`，并给出契约、示例与外部 module 验收。产品侧 CLI/TUI 行为不得变化。

## 非目标

- 不重写业务逻辑，不顺带优化性能或改默认行为；先做到“纯移动”。
- 不把 TUI、`internal/runtime`、`internal/tools`、`internal/subagent` 上移。
- 不实现 OS 沙箱、远程策略服务、Responses API 或工作流编排。
- 不在本计划内创建 1.0 tag 或发布；发布与 API 兼容门禁见“后续衔接”。

## 包边界与依赖结论

搬运时的**默认结论**如下；REUSE-001 可以调整，但必须在 ADR 中写明被否决方案与理由。

- 核心实现进 `pkg`，产品适配留 `internal`。
- `pkg/permission`、`pkg/observability` 的**核心包不导入 `pkg/agent`**；两者的 `adapter.go` 只做 `pkg/agent` 接口桥接，保留在 `internal/<name>/adapter.go`。
- 外部嵌入者若需要 `agent.PermissionChecker`，按契约文档中的约 10 行适配示例自行桥接；本期不为适配器新增公共子包。
- 被否决方案：核心包直接依赖 `pkg/agent`（把高层包压到低层，后续易成环）；把接口下沉并让 `pkg/agent` 反向依赖新包。
- 实例级注入优先使用已有的 `builtin.AllWithRuntime` / `builtin.Runtime`（CFG-003），不依赖 `SetPathChecker` / `SetTimeoutConfig` / `SetEventBus` 包级全局；`pkg/permission.PathChecker` 的方法签名已满足 `builtin.PathChecker`，无需适配器。

| 源包 | 目标包 | 当前模块内依赖 | 上移后的依赖约束 | 适配层归属 |
|---|---|---|---|---|
| `internal/jobs` | `pkg/jobs` | `pkg/session` | 只允许 `pkg/session` 与标准库 | 无，消费方改 import |
| `internal/edits` | `pkg/edits` | 无 | 只允许标准库 | 无 |
| `internal/permission` | `pkg/permission` | `pkg/agent`（仅 `adapter.go`） | 核心零 `pkg/agent` 依赖；不得新增包级全局 | `adapter.go` 留在 `internal/permission` |
| `internal/observability` | `pkg/observability` | `pkg/agent`（仅 `adapter.go`） | 核心零 `pkg/agent` 依赖；订阅者不得阻塞主流程 | `adapter.go` 留在 `internal/observability` |

**实例作用域**（REUSE-001 必须给出归属矩阵）：`jobs.Manager` 按 workspace 一实例；permission 的 `Checker`、缓存与审批 broker 按 Agent 或 session 实例；observability 的 Logger/CostTracker/EventBus 按产品装配决定，但订阅者不得共享可变状态。任何新包不得通过包级变量隐式共享实例状态。REUSE-006 用同进程双实例测试验证互不污染。

**API 兼容层级**：四个新包首版标记为**实验 API（0.x）**，暂不纳入 `ARCHITECTURE.md` 现有“核心 API / 扩展 API”的 SemVer 承诺；1.0 前允许破坏性调整，但必须走 ADR 并在 `CHANGELOG.md` 写明迁移方式。稳定后再归入“扩展 API”层级（可新增、不可删除）。REUSE-006 负责在 `ARCHITECTURE.md` 增加“实验 API”层级及归入条件；`pkg/agent`、`pkg/llm` 等核心 API 的兼容承诺不变。

硬约束：`pkg/*` 不得导入 `basework/internal/*`，由 `tests/arch_test.go` 与 CI 守护；上移后新增的四条边必须出现在 `make deps` 生成的依赖图中。

## 通用执行规则

1. **纯移动优先**：先 `git mv` 文件与测试，保持函数体、错误文本、日志字段不变；只改 import 路径与包注释。行为修改必须单列并给出回归理由。
2. **一次一个包**：REUSE-002 至 REUSE-005 各自独立可回退；默认串行，REUSE-002/003 依赖最少可先做。
3. **契约随包**：每个新 `pkg` 包必须在 `docs/reference/pkg/<name>.md` 提供“用途 / 配置 / 扩展点 / Model Experience / Known Limitations”五节，并登记到 [包契约索引](../reference/README.md)，否则 `make check-docs` 失败。
4. **显式依赖**：公共构造函数使用 `Options` 或显式参数，不新增 `SetXxx` 包级全局；已有 `builtin.Set*` 只保留为兼容回落，新代码、示例与产品新接线一律走 `builtin.AllWithRuntime`。
5. **平台文件原样保留**：`process_unix.go`、`process_windows.go`、`audit_sqlite.go`、`sqlite_store.go`、`migration.go` 及对应测试随包移动，构建标签不变。
6. **行为等价证明**：现有包内测试、`internal/tools`/`internal/runtime`/`cmd/basework` 消费方测试、PTY 场景与五平台 CI 全部保持通过；证据文件记录命令、提交与平台。
7. **移动可验证**：每包一个独立 commit；移动前后在同一平台分别记录 `go test -list '.*' ./<包>` 的用例名与数量，二者必须完全一致；平台专属用例（如 Windows）按源码清单核对，或在对应平台 CI 记录基线；移动 commit 内不得出现函数体、默认值、错误文本、日志字段变化；导出→未导出的可见性收窄只改可见性，单独提交，不混入移动 commit；审查使用 `git diff --color-moved`。
8. **重复运行**：并发与平台敏感包（jobs、permission）移动后至少执行一次 `-count=10` 包内测试，并至少执行一次 race。
9. **文档同步**：每个任务结束前跑 `make check-docs`；涉及依赖图与统计时跑 `make gen` 并提交生成结果；新包必须进入 `docs/README.md` 与包契约索引的可发现路径。

## 风险与对策

| 风险 | 表现 | 对策 |
|---|---|---|
| `pkg` 内出现向上依赖 | `pkg/permission`、`pkg/observability` 为适配 `pkg/agent` 接口而反向耦合 | 按默认结论执行：核心包零 `pkg/agent` 依赖，适配器留 `internal`；REUSE-001 如需更改必须走 ADR |
| 包级全局与多实例污染 | 嵌入方同进程跑两个 Agent，后设置的 `Set*` 覆盖先设置者 | 新代码与示例一律走 `builtin.AllWithRuntime`；REUSE-006 增加双实例并行测试；`Set*` 仅作兼容回落 |
| 实例作用域不清 | jobs/permission 被误当进程单例，跨 workspace 串数据 | REUSE-001 产出实例归属矩阵；契约写明生命周期与并发边界 |
| 适配层各自复制 | 外部各写一版适配器，行为漂移 | 契约给出最小适配示例与编译期接口断言；不新增公共适配子包 |
| 行为漂移 | 移动时顺手改错误文本、默认值或并发语义 | 纯移动提交与行为修改提交分开；用例名/数量基线 + `git diff --color-moved` 审查 |
| 移动无法验证 | 移动 commit 混入逻辑改动，测试仍通过 | 每包独立 commit；移动前后测试清单必须一致；REUSE-006 抽查 |
| API 过早固化 | 新包符号未经评审即承诺兼容 | 首版只暴露最小可用面；REUSE-001 定义 API 与兼容层级，REUSE-006 写入 `ARCHITECTURE.md` |
| Windows/并发回归 | 进程树、文件锁、SQLite 平台差异 | 保留平台文件与测试；交叉编译 + 真实平台证据；jobs/permission 加 `-count=10` |
| 文档门禁失败 | 新 `pkg` 包缺少五节契约、依赖图/统计过期 | 每个任务内同步契约、`docs/README.md`、`make gen` 与 `make check-docs` |

## 中止与回退

触发任一条件时，停止当前包，不继续硬推：

- 为保持行为必须让 `pkg` 导入 `internal`，或形成 `pkg` 内 import 环；
- 必须改变公共语义、默认值、错误文本或日志字段才能完成移动；
- Windows 或非 Unix 行为无法在 `pkg` 内等价保留；
- 实例作用域无法用显式注入表达，只能退回包级全局；
- 契约评审发现首版 API 需要暴露产品专用概念（TUI 文案、配置路径、审批交互）。

处理方式：按包独立 commit 回退，其他已完成包不受影响；回到 REUSE-001 的 ADR 重新切片（拆“核心 + 适配”、收窄 API 或改为文档化限制），确认后再继续。禁止用“先合进去再说”绕过。

## 完成后的整体验收

- `go build ./...`、`make check-docs`、`make check-arch`、`make gen` 后 `git diff --exit-code` 无差异。
- `make vet`、`make lint` 与 `go vet -tags 'sqlite memory' ./...` 通过。
- `go test ./...` 与 `go test -tags 'sqlite memory' ./...` 全量通过。
- `go test -race ./pkg/jobs ./pkg/edits ./pkg/permission ./pkg/observability ./internal/tools ./internal/runtime ./cmd/basework` 通过。
- `go test -count=10 ./pkg/jobs` 与 `go test -count=10 -tags 'sqlite memory' ./pkg/permission` 通过，用于捕捉并发与迁移抖动。
- 同进程双实例测试通过：两个 `jobs.Manager`、两个权限 `Checker` 使用不同 workspace/配置并行运行，互不覆盖；全程不依赖 `builtin.Set*`。
- 四个移动 commit 的 `go test -list '.*'` 用例名与数量与移动前同平台基线一致；平台专属用例按源码清单或平台 CI 核对；`git diff --color-moved` 审查无逻辑改动。
- 仓库外临时 module 通过 `replace` 引用本仓库，仅用 `pkg` 公共 API 完成 jobs/edits/permission/observability 的组合，不导入 `internal`。
- PTY scenario1–8 与五平台 CI（含 Windows）通过；证据分开记录真实平台结果，不用交叉编译代替运行结论。
- `docs/STATUS.md`、`docs/CHANGELOG.md`、`docs/README.md` 与嵌入指南反映新公共包；`TASKS.md` 六项任务各自带有有效证据。

## 后续衔接（不在本计划内）

四包上移完成后，另立任务评估：创建 `v1.0.0` 或 `v0.2.0` tag、在 CI 增加 API 兼容检查（apidiff/go-apidiff）、补齐真实 Provider 协议矩阵。本计划不承诺发布日期。

---

<a id="reuse-001"></a>

## REUSE-001：公共能力上移评估与 API 契约

**前置任务**：BASE-001。优先级 P1；这是四个上移任务的设计前置，不改产品代码。

**先读 / 主要修改范围**：`internal/jobs/`、`internal/edits/`、`internal/permission/`、`internal/observability/`、`pkg/agent/interfaces.go`、`pkg/tool/`、`tests/arch_test.go`、[依赖图](../DEPGRAPH.md)、[架构](../ARCHITECTURE.md)、[包契约](../reference/README.md)。产出 `docs/adr/0008-promote-internal-capabilities-to-pkg.md` 与本任务证据。

**小步骤**：

1. 逐包盘点：导出符号、文件与测试数量、平台构建标签、模块内 import、现有消费方（`internal/tools`、`internal/runtime`、`cmd/basework`、TUI）以及对外行为不变量。
2. 按默认结论确认 `adapter.go` 归属：`pkg/permission`、`pkg/observability` 的核心包零 `pkg/agent` 依赖，适配器留 `internal/<name>/adapter.go`；若证据要求改为“接口下沉”或“允许 `pkg/* → pkg/agent`”，在 ADR 中写明被否决方案与触发条件。
3. 定义首版公共 API：构造函数、`Options`、稳定错误值、可注入接口、生命周期（谁负责关闭、是否可重入、并发安全边界）与**实例归属矩阵**（jobs 按 workspace、permission 按 Agent/session、observability 按产品装配）；新代码不得依赖 `builtin.Set*` 全局。区分“搬运后原样导出”与“收窄 API”：后者只改可见性、单独提交，并在契约中列出暂定导出符号。
4. 明确兼容层级：四个新包首版标记为“实验 API（0.x）”，不纳入现有核心/扩展 API 的 SemVer 承诺；1.0 前允许破坏性调整，但必须走 ADR 并在 `CHANGELOG.md` 写明迁移方式，稳定后再归入扩展 API 层级。
5. 为每个包写明行为不变量：job 终态恰好一次、`interrupted` 不自动重放；编辑逐文件原子但跨文件非事务、撤销不覆盖后来编辑；权限默认拒绝、作用域按 SEC-001 语义；观测订阅者不得阻塞或拖垮主流程。
6. 输出迁移映射表（源文件 → 目标文件 → 消费方改动）、移动前测试用例基线（`go test -list '.*'` 的名称与数量）、按包独立回退方案；同步更新本文件中的包边界结论。

**验收条件**：

- [ ] 四个包都有符号与消费方清单，未解释的 `pkg → internal` 边为零。
- [ ] ADR 记录上移决定、被否决方案、适配层归属与兼容层级；状态为 Accepted。
- [ ] 实例归属矩阵覆盖 jobs/permission/observability，标明并发与生命周期。
- [ ] 兼容层级写入 ADR，明确 1.0 前后的破坏性变更规则。
- [ ] 首版 API 草案可直接落到 `go doc` 形态，且不含新增包级可变全局；新包接线方式统一走 `builtin.AllWithRuntime`。
- [ ] 每个包的行为不变量都有对应的现有测试或明确的补测清单。
- [ ] 迁移映射表覆盖全部源文件与测试，含测试用例基线，回退方案可执行。

**验证**：`go list -deps ./internal/jobs ./internal/edits ./internal/permission ./internal/observability` 对照映射表；`make check-docs`；保存 `docs/development/evidence/REUSE-001.md`。

**范围外**：不移动代码、不改公共符号、不调整 CI。

---

<a id="reuse-002"></a>

## REUSE-002：后台任务包上移

**前置任务**：REUSE-001。优先级 P1；把 `internal/jobs` 提升为 `pkg/jobs`，行为保持不变。

**先读 / 主要修改范围**：`internal/jobs/` 全部文件与测试、`internal/runtime/`、`internal/tools/`、`cmd/basework/` 中 jobs 的引用；新增 `pkg/jobs/` 与 `docs/reference/pkg/jobs.md`。

**小步骤**：

1. 纯移动 `jobs.go`、`journal.go`、`output.go`、`process.go`、`process_unix.go`、`process_windows.go`、`process_other.go` 及对应测试到 `pkg/jobs`，包名与导出符号不变。
2. 更新消费方 import；不改变 owner、状态机、输出配额、溢出文件、取消、进程树终止与重启 `Reconcile` 的任何语义。
3. 写明公共 API 的关闭与并发契约：`Manager` 是否需要在进程退出前显式关闭、`Journal` 写失败如何返回、输出页在过期后的错误值。
4. 新增五节契约与 `docs/reference/README.md` 索引行；说明 JSONL 历史与 `pkg/session` 的关系。
5. 补一个仓库外视角的最小用法断言（可在示例中延后到 REUSE-006），确认只依赖 `pkg/jobs` 即可启动、取消并读取一个后台命令。

**验收条件**：

- [ ] `go list -deps ./pkg/jobs` 不含任何 `basework/internal/` 包。
- [ ] 现有 jobs 测试、runtime/tools/CLI 消费方测试全量通过，错误文本与日志字段未变。
- [ ] 移动 commit 为纯移动：`pkg/jobs` 的测试用例名与数量与移动前基线一致，`git diff --color-moved` 审查无逻辑改动。
- [ ] Unix 进程组终止、Windows `taskkill /T` 与后代补清理路径均有测试覆盖；Windows amd64 交叉编译通过。
- [ ] `docs/reference/pkg/jobs.md` 五节齐备，Known Limitations 明确平台与输出保留边界。

**验证**：`go test -race ./pkg/jobs ./internal/runtime ./internal/tools ./cmd/basework -count=1`；`go test -count=10 ./pkg/jobs`；`GOOS=windows GOARCH=amd64 go build ./...`；保存 `docs/development/evidence/REUSE-002.md`。

**范围外**：不新增远程任务、不改变输出配额默认值、不把 job 事件接入 TUI 新组件。

---

<a id="reuse-003"></a>

## REUSE-003：编辑计划与提交包上移

**前置任务**：REUSE-001。优先级 P1；把 `internal/edits` 提升为 `pkg/edits`，行为保持不变。

**先读 / 主要修改范围**：`internal/edits/plan.go`、`commit.go` 与测试、`internal/tools/` 中编辑工具接线；新增 `pkg/edits/` 与 `docs/reference/pkg/edits.md`。

**小步骤**：

1. 纯移动 `plan.go`、`commit.go` 与测试到 `pkg/edits`，保持 `Planner`、`Options`、`Batch`、`CommitResult`、`RollbackResult` 等符号与语义。
2. 保持路径解析与权限检查的不可绕过性：预览与提交都走同一路径解析；提交阶段重新检查，不信任预览结果。
3. 保持三条边界：逐文件原子、跨文件非事务、撤销遇到后来编辑按文件跳过并说明原因；在契约中明确这不是事务型文件系统。
4. 更新 `internal/tools` 的编辑工具接线与测试，确保 plan/diff 审批、`file.edited` 事件与事实持久化行为不变。
5. 新增五节契约与索引行，给出最小预览/提交/撤销示例。

**验收条件**：

- [ ] `go list -deps ./pkg/edits` 只含标准库与允许的 `pkg` 包。
- [ ] 路径越界、软链、权限、基线冲突、部分成功与撤销跳过场景的现有测试全部通过。
- [ ] `internal/tools` 的编辑预览/提交/撤销回归通过，事件与事实记录未变。
- [ ] 移动 commit 为纯移动：`pkg/edits` 的测试用例名与数量与移动前基线一致，审查无逻辑改动。
- [ ] 契约写明跨文件非事务与撤销跳过规则，示例可运行。

**验证**：`go test -race ./pkg/edits ./internal/tools ./cmd/basework -count=1`；保存 `docs/development/evidence/REUSE-003.md`。

**范围外**：不实现跨文件事务、不引入新的 diff 格式、不改审批 UI。

---

<a id="reuse-004"></a>

## REUSE-004：权限引擎包上移

**前置任务**：REUSE-001。优先级 P1；把 `internal/permission` 的核心提升为 `pkg/permission`。

**先读 / 主要修改范围**：`internal/permission/` 全部文件与测试、`pkg/agent/interfaces.go`、`internal/tools/`、`cmd/basework/` 权限接线、[SEC-001 证据](../development/evidence/SEC-001.md)；新增 `pkg/permission/` 与 `docs/reference/pkg/permission.md`。

**小步骤**：

1. 按 REUSE-001 结论移动核心文件：`checker.go`、`mode.go`、`rule.go`、`scope.go`、`paths.go`、`cache.go`、`store.go`、`sqlite_store.go`、`audit.go`、`audit_sqlite.go`、`migration.go`、`approval.go` 及测试；`adapter.go` 保留在 `internal/permission`，核心包零 `pkg/agent` 依赖。
2. 保持 SEC-001 语义：`global/session/project` 优先级、缺少上下文时的安全默认值、在途检查的上下文快照、缓存精确参数匹配、SQLite 迁移与旧规则兼容。
3. 明确交互与库的边界：`ApprovalBroker`、`PromptFunc`、`PathChecker` 可独立使用；不把 TUI 文案或配置路径带入 `pkg`。
4. 更新 `internal/tools` 与 `cmd/basework` 的 import 和适配代码：产品接线走 `builtin.AllWithRuntime` 注入实例级 `PathChecker`，不再扩大 `builtin.SetPathChecker` 的使用；确保 interactive/yolo/deny-all 行为与审计记录不变。
5. 新增五节契约与索引行；更新权限指南，补充“作为库使用”的最小示例、`agent.PermissionChecker` 适配示例与安全边界说明。适配示例必须同时实现 `Check` 与 `CheckPath`（基于 `Checker` + `PathChecker`，等价 `PathPermissionAdapter`），不得复制只实现 `Check`、`CheckPath` 直接返回 nil 的 `PermissionAdapter`。

**验收条件**：

- [ ] `go list -deps ./pkg/permission` 不含 `basework/internal/` 与 `pkg/agent`，`adapter.go` 仍在 `internal/permission`。
- [ ] 规则作用域、路径保护、审批、审计、SQLite 迁移与缓存的现有测试全部通过，含 restart 与 race 场景。
- [ ] 移动 commit 为纯移动：`pkg/permission` 的测试用例名与数量与移动前基线一致，审查无逻辑改动。
- [ ] CLI 的权限模式、审计输出与拒绝文案未变；旧数据库可直接读取；产品新接线不使用 `SetPathChecker`。
- [ ] 契约明确“权限规则不是 OS 沙箱”；适配示例同时实现 `Check` 与 `CheckPath`，并断言一条被拒路径。

**验证**：`go test -race -tags 'sqlite memory' ./pkg/permission ./internal/tools ./cmd/basework -count=1`；`go test -count=10 -tags 'sqlite memory' ./pkg/permission`；隔离 HOME 的 CLI 规则场景；保存 `docs/development/evidence/REUSE-004.md`。

**范围外**：不实现 OS 沙箱、远程策略服务或跨进程审批；不改变默认权限模式。

---

<a id="reuse-005"></a>

## REUSE-005：可观测性包上移

**前置任务**：REUSE-001。优先级 P1；把 `internal/observability` 提升为 `pkg/observability`。

**先读 / 主要修改范围**：`internal/observability/` 全部文件与测试、`pkg/agent/observer.go`、`cmd/basework/` 日志与 pprof 接线；新增 `pkg/observability/` 与 `docs/reference/pkg/observability.md`。

**小步骤**：

1. 纯移动 `logger.go`、`cost.go`、`tokens.go`、`event.go`、`bus.go`、`subscriber.go`、`config.go`、`pprof.go` 及测试；`adapter.go` 保留在 `internal/observability`，核心包零 `pkg/agent` 依赖。
2. 保持事件订阅不阻塞主流程：有界投递、订阅者 panic 隔离、关闭语义与现有实现一致。
3. 保持日志格式、成本/token 统计口径与 pprof 默认关闭行为不变；不在库内启动后台 goroutine 泄漏。
4. 更新 `cmd/basework` 的装配路径与测试：实例级事件总线走 `builtin.AllWithRuntime`，`SetEventBus` 仅作兼容回落；确认 `agent.start` 等事件类型和字段未变。
5. 新增五节契约与索引行，说明哪些能力是库内可复用、哪些仍由产品配置决定。

**验收条件**：

- [ ] `go list -deps ./pkg/observability` 不含 `basework/internal/` 与 `pkg/agent`，`adapter.go` 仍在 `internal/observability`。
- [ ] 日志、成本、token、事件总线、pprof 的现有测试通过；race 下无数据竞争。
- [ ] 移动 commit 为纯移动：`pkg/observability` 的测试用例名与数量与移动前基线一致，审查无逻辑改动。
- [ ] 订阅者阻塞或 panic 不会阻断 Agent 回合；关闭后不再投递；产品新接线不使用 `SetEventBus`。
- [ ] CLI 日志格式与事件字段未变，契约写明有界投递与丢弃语义。

**验证**：`go test -race ./pkg/observability ./cmd/basework -count=1`；保存 `docs/development/evidence/REUSE-005.md`。

**范围外**：不接入 OpenTelemetry、不做远程指标上报、不改日志采样策略。

---

<a id="reuse-006"></a>

## REUSE-006：内部薄适配与外部嵌入验收

**前置任务**：REUSE-002, REUSE-003, REUSE-004, REUSE-005。优先级 P1；收口四个包上移后的产品侧改动与外部可用性证明。

**先读 / 主要修改范围**：`internal/tools/`、`internal/runtime/`、`cmd/basework/`、`examples/embed/`、`docs/README.md`、`docs/ARCHITECTURE.md`、`docs/reference/README.md`、`docs/guides/embedder-guide.md`、`docs/STATUS.md`、`docs/CHANGELOG.md`、`tests/arch_test.go`。

**小步骤**：

1. 清理 `internal` 中已上移的重复实现，只保留确实产品专用的适配层；确认没有两份可漂移的核心逻辑。
2. 新增 `examples/embed_components/main.go`：仅用公共 API 组合四个包，断言 job 超时/取消、编辑预览与撤销、权限拒绝、观测事件与成本统计；不导入任何 `internal`，不调用 `builtin.Set*`。
3. 在同一示例中启动两套不同 workspace/配置的 jobs 与权限实例并行运行，验证互不覆盖；事件总线与路径检查全部通过 `builtin.Runtime` 注入。
4. 用仓库外临时 module 加 `replace` 引用本仓库，编译并运行该示例；记录 Go 版本、平台与命令。
5. 更新架构与文档：在 `ARCHITECTURE.md` 的 API 兼容层级中加入四个新包；`make gen` 刷新 `DEPGRAPH.md`/`STATS.md`；`docs/README.md` 文档索引、包契约索引、嵌入指南、`STATUS.md` 与 `CHANGELOG.md` 同步。
6. 全量回归：默认与 `sqlite memory` 测试、关键 race、PTY scenario1–8、五平台 CI；抽查四个移动 commit 的测试用例基线与 `git diff --color-moved`；证据分开记录本地、交叉编译与真实平台结果。

**验收条件**：

- [ ] `internal/` 中不再保留四个包的可执行核心副本；消费方只通过 `pkg` 导入。
- [ ] 外部 module 示例编译、运行通过，`go list -deps` 结果不含 `basework/internal/`。
- [ ] 同进程双实例并行通过，jobs/permission 配置互不覆盖，示例全程不依赖 `builtin.Set*`。
- [ ] 四个移动 commit 的测试用例名/数量与移动前基线一致，抽查无逻辑改动。
- [ ] `make check-docs`、`make check-arch`、`make gen` 新鲜度检查与链接检查全部通过。
- [ ] 默认与完整 tags 全量测试、race、PTY 与五平台 CI 通过；证据含提交与平台。
- [ ] `docs/README.md`、包契约索引与嵌入指南均能发现四个新包；文档说明兼容层级、适用场景与限制，不把上移等同于行为重写。

**验证**：`go test ./... -count=1`；`go test -tags 'sqlite memory' ./... -count=1`；`go test -race ./pkg/jobs ./pkg/edits ./pkg/permission ./pkg/observability ./internal/tools ./internal/runtime ./cmd/basework -count=1`；仓库外 module 运行示例；保存 `docs/development/evidence/REUSE-006.md`。

**范围外**：不创建 release tag、不新增远程服务、不改产品默认配置。
