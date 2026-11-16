# ADR 0009: apidiff API 兼容门禁的工具、基线与分层执行策略

- Status: Accepted
- Date: 2026-11-16
- Supersedes: —

## 背景

[ARCHITECTURE 的 API 分层](../ARCHITECTURE.md#api-兼容性分层)对核心/扩展 `pkg` 做出 SemVer 承诺，但只有文档约束。M11 要求把承诺变成 CI 门禁：稳定层出现破坏性变更而未走迁移流程时直接失败。API-001 评估了工具行为、真实基线差异与例外规则，结论如下。

评估事实（详见 [API-001 证据](../development/evidence/API-001.md)）：

- `v0.1.6` → HEAD（`484c703`）13 个共有 `pkg` 包默认导出面**零差异**（兼容与不兼容均无）；新增包只有四个实验包。
- 当前 `pkg` 非测试代码**没有**直接引用 `internal/` 类型的导出符号，ARCHITECTURE 例外条款当前为空集。
- apidiff 对不兼容变更**打印报告但退出码仍为 0**；门禁必须解析输出自行失败。
- apidiff 目录模式不接受相对路径；跨版本对比用导出快照（`-w`）+ import path 模式最稳。
- build tags 通过 `GOFLAGS='-tags=sqlite,memory'` 传入有效（空格分隔会被 go 拒绝）。

## 决定

### 工具与版本

- 使用官方 `golang.org/x/exp/cmd/apidiff`，**固定版本 `v0.0.0-20261113113407-e3970619a1a4`**，以 `go run ...@<version>` 调用（与 govulncheck 同一模式），不加入模块依赖。`x/exp` 无稳定性承诺；升级工具版本视为门禁变更，须重跑失败自测夹具。
- 被否决方案：`joelanford/go-apidiff`（第三方封装，导出快照模式已满足需求，不引入额外依赖）；`gorelease`（面向发布决策，过重，不适合每次推送）；仅人工审查（无法机器强制，正是本轮要解决的缺口）。

### 基线与对比方式

- 基线为**最近发布 tag**（当前 `v0.1.6`），由脚本解析（`git describe --tags --abbrev=0` 语义，取语义化版本最高的 tag）。
- 在基线 tag 的临时 detached worktree 中用 `apidiff -w` 生成每个包的导出快照，再在当前工作树以「快照文件 vs import path」对比。快照与当前代码用同一工具链生成。
- 基线解析失败、worktree 创建失败、快照生成失败、工具下载失败均**显式失败**，不得静默跳过（与漏洞门禁同一纪律）。
- 基线中不存在的新包按「新增」报告，不失败。

### 分层执行

| 层级 | 包 | 门禁行为 |
|---|---|---|
| 核心 | `pkg/llm`、`pkg/tool`、`pkg/tool/builtin`、`pkg/session`、`pkg/agent` | incompatible → 失败 |
| 扩展 | `pkg/provider`、`pkg/hook`、`pkg/mcp`、`pkg/lsp`、`pkg/config`、`pkg/skill`、`pkg/memory` | incompatible → 失败（只增不删） |
| 实验 | `pkg/jobs`、`pkg/edits`、`pkg/permission`、`pkg/observability` | 仅报告；破坏性变更按 ADR 0008 走 ADR + CHANGELOG 迁移说明，由审查把关 |

- `pkg/config`、`pkg/skill`、`pkg/memory` 此前未列入 ARCHITECTURE 分层表，本 ADR 将其归入扩展层，分层表同步更新。
- 包清单以 `go list ./pkg/...` 现场枚举为准，按上述规则分层；未来新增 `pkg` 包默认归扩展层，归入核心或实验须修订本 ADR 或 ARCHITECTURE。
- 无非测试 Go 文件的包（如仅含测试的目录）跳过。

### 例外与豁免

- ARCHITECTURE 例外继续有效：形参/返回类型直接引用 `internal/` 的导出符号不计入兼容承诺。当前为空集，**不实现自动豁免名单**；将来出现时由维护者在证据中逐条登记并按例外处理，门禁输出保留原文供审查。
- 稳定层真实破坏性变更的唯一放行路径：按 SemVer 提升主版本（1.0 前为次版本评估）并在 CHANGELOG 写明迁移；不得通过放宽门禁放行。

### Build tags 覆盖面

- 两遍检查：默认导出面 + `sqlite,memory` 完整导出面（`GOFLAGS` 传入）。带 tag 的公共 API（如 SQLite 存储、压缩）同样是对使用方的承诺，须受保护；基线快照按相同 tags 生成。

### 失败语义

- apidiff 自身退出码不可信（不兼容也返回 0）。门禁脚本解析输出中的 `Incompatible changes:` 段：稳定层命中即非零退出；实验层命中仅打印并提示 ADR 0008 流程。
- 输出中的 `Compatible changes:` 仅记录，不阻断。

## 后果

- CI Quality（或独立 job）与本地 `make check-api` 行为一致；破坏性变更夹具自测（`make test-api-script`）证明失败路径有效。
- 门禁运行成本约为每包一次导出加载；两遍 tags 加倍，必要时独立 job 与 Quality 并行。
- 1.0 评估时实验四包毕业：从实验层名单移入扩展层即自动获得门禁保护，无需改脚本逻辑。