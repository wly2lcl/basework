# ADR 0008: 将四个可靠性与治理能力从 internal 上移为 pkg 实验 API

- Status: Accepted
- Date: 2026-11-13
- Supersedes: —

## 背景

ADR 0002 用「使用方本地定义最小接口」让 `pkg/tool/builtin` 不再反向依赖 `internal/`，可嵌入承诺在**接口层面**成立。但外部嵌入者真正需要的四块实现仍留在 `internal/`：

- `internal/jobs`：后台任务状态机、owner 隔离、输出配额与溢出、取消与进程树清理、JSONL 历史；
- `internal/edits`：编辑预览、基线哈希、逐文件提交、部分成功清单与安全撤销；
- `internal/permission`：interactive/yolo/deny-all、路径保护、规则作用域、审批 broker、审计与缓存；
- `internal/observability`：结构化日志、成本与 token 统计、事件总线、pprof。

Go 的 `internal` 规则使仓库外无法导入这些包。结果是接口可用、电池要自己重写：`agent.PermissionChecker` 只是 10 行适配，而 `Checker` 的模式优先级、缓存精确匹配、SQLite 迁移、审批超时都是重写者一定会写错的部分。可嵌入价值因此只兑现了一半。

另一条歧路是把四个包的核心直接依赖 `pkg/agent` 以便复用接口。`pkg/agent` 是高层组装包（依赖 `pkg/llm`、`pkg/tool`、`pkg/session`），让底层能力反向依赖它，后续极易成环，也会把「用不用 Agent」绑死在一起。同理，「把接口下沉到新共享包、让 `pkg/agent` 反向依赖」只是把倒置换了个方向，未采纳。

## 决定

**把四个包的核心实现按原行为上移为 `pkg/jobs`、`pkg/edits`、`pkg/permission`、`pkg/observability`，`pkg/agent` 适配器留在 `internal/<name>/adapter.go`，首版标记为实验 API（0.x）。**

具体约定：

1. **纯移动优先**：`git mv` 文件与测试，包名与导出符号不变；只改 import 路径与包注释。函数体、默认值、错误文本、日志字段必须不变。可见性收窄（导出转未导出）不混入移动提交。
2. **依赖方向**：核心包只允许标准库、`pkg/session`（仅 jobs）与同层 `pkg`；`pkg/*` 不得导入 `internal/*`。`pkg/permission` 与 `pkg/observability` 的核心零 `pkg/agent` 依赖；`PermissionAdapter`、`PathPermissionAdapter`、`EventBusAdapter` 留在 `internal/permission`、`internal/observability`，只做接口桥接。
3. **外部适配**：嵌入者若需要 `agent.PermissionChecker`，按契约文档的约 10 行示例自行桥接，且必须同时实现 `Check` 与 `CheckPath`、基于 `Checker` + `PathChecker`；借鉴 `PathPermissionAdapter`，不得复制 `CheckPath` 直接返回 nil 的 `PermissionAdapter`。本期不新增公共适配子包。
4. **实例作用域矩阵**（任何新包不得用包级变量隐式共享实例状态）：

| 能力 | 归属 | 生命周期 | 并发边界 |
|---|---|---|---|
| `jobs.Manager` | 每个 workspace 一个实例 | 显式 `Close()`；`CloseOwner` 只清理该 owner | 多 goroutine 可并发 `Start`/`Get`/`Cancel`；owner 隔离由必填 owner 保证 |
| `permission.Checker` / `Cache` / `ApprovalBroker` | 每个 Agent 或 session 一个实例 | 随 Agent/session 释放；broker 有 `Close` | `Checker.Check` 并发安全；作用域上下文按检查开始时的快照判定 |
| `observability.Logger` / `CostTracker` / `EventBus` | 由产品装配决定 | 进程/产品生命周期；`Profiler` 默认关闭 | `Publish` 有界投递，订阅者 panic 隔离，不得阻塞 Agent 回合 |

5. **接线方式**：产品与示例新接线一律走已有的 `builtin.AllWithRuntime`（CFG-003）；`builtin.SetEventBus` / `SetPathChecker` / `SetTimeoutConfig` 仅作兼容回落，不新增 `Set*` 全局。
6. **兼容层级**：四个新包首版为**实验 API（0.x）**，不纳入现有「核心 API / 扩展 API」的 SemVer 承诺；1.0 前允许破坏性调整，但必须走 ADR 并在 `CHANGELOG.md` 写明迁移方式。稳定后归入扩展 API（只增不删）。REUSE-006 在 `ARCHITECTURE.md` 增加该层级。
7. **行为不变量**：job 终态恰好一次发布，`interrupted` 不自动重放；编辑逐文件原子、跨文件非事务，撤销遇到后来编辑按文件跳过并说明原因；权限默认拒绝、作用域按 SEC-001、路径保护不是 OS 沙箱；观测订阅者不阻塞主流程，关闭后不再投递。
8. **验收方式**：每包独立 commit，移动前后 `go test -list '.*'` 用例名与数量一致，`git diff --color-moved` 无逻辑改动；jobs/permission 增加 `-count=10` 与 race；外部 module 用 `replace` 引用本仓库仅用公共 API 组合验证。

被否决方案与否决理由：

| 方案 | 结论 | 理由 |
|---|---|---|
| 核心包直接依赖 `pkg/agent` 以复用接口 | ❌ 否决 | 高层包（agent 依赖 llm/tool/session）反向压到低层，后续易成环；嵌入者被迫接受 Agent 组装 |
| 接口下沉到新共享包，`pkg/agent` 反向依赖 | ❌ 否决 | 只是把倒置换方向，且共享包会变成「谁都能塞」的垃圾场（同 ADR 0002） |
| 新增公共适配子包 | ❌ 否决 | 适配只有约 10 行，新增包等于把 `pkg/agent` 依赖换名带回来；契约给示例即可 |
| 继续留在 `internal/`，只补文档 | ❌ 否决 | 不解决仓库外不可导入这一根因 |

## 后果

正面：

- 嵌入者从「拿到接口」升级为「拿到电池」：四个包可直接导入，`go list -deps` 不含 `internal/`。
- 依赖图变浅：`pkg/permission`、`pkg/observability` 核心只依赖标准库，与 `pkg/agent` 解耦，可单独拆 module。
- 行为有据：移动提交与测试基线绑定，避免「搬着搬着顺手重构」。

代价与后续：

- `internal/` 与 `pkg/` 在过渡期各有一份适配器代码（无核心重复），消费方需要同时 import 两个包。
- 四个新包首版是实验 API，1.0 前仍可能破坏性变更；这是为了先兑现复用、再固化契约。
- 仓库外嵌入者要自己写约 10 行 `agent.PermissionChecker` 适配；契约文档必须给出双方法示例，否则容易复制只实现 `Check` 的错误版本。
- 需要在 CI 保持测试用例基线与 `-count=10`，否则并发/平台回归只能靠人工发现。
