# M11：API 兼容门禁与 v0.2.0 发布

## 背景与目标

M10 完成后，`pkg` 已承载核心、扩展与实验三层公共 API（分层定义见 [ARCHITECTURE](../ARCHITECTURE.md#api-兼容性分层)），但 SemVer 承诺目前只有文档约束，没有机器门禁；CHANGELOG 的 `0.3.0`–`0.5.0` 内部编号尚未对应任何 git tag（现有 tag 为 `v0.1.0`–`v0.1.6`）。本轮目标：

1. 把 API 兼容检查（apidiff）接入 CI，使稳定层的破坏性变更在未走迁移流程时直接失败。
2. 补齐 tag ↔ CHANGELOG ↔ 发布记录的历史欠账。
3. 在门禁生效后发布 **v0.2.0** 作为首个受门禁保护的候选，为后续 1.0 评估积累运行证据。

**版本策略（2026-11-16 用户已确认）**：先 v0.2.0，再评估 1.0。本轮不打 `v1.0.0`，不承诺 1.0 日期；四个实验包毕业标准在 1.0 评估时另行定义。

## 范围与边界

- 门禁只覆盖 `pkg` 导出面；`internal/*`、`cmd/*` 无兼容承诺，不检查。
- 分层执行策略：
  - **核心 API**（`pkg/llm`、`pkg/tool`、`pkg/session`、`pkg/agent`）：incompatible 变更 → CI 失败。
  - **扩展 API**（`pkg/provider`、`pkg/hook`、`pkg/mcp`、`pkg/lsp` 及其余稳定 `pkg`）：只增不删，incompatible → CI 失败。
  - **实验 API**（`pkg/jobs`、`pkg/edits`、`pkg/permission`、`pkg/observability`）：报告不阻断；破坏性变更仍须按 [ADR 0008](../adr/0008-promote-internal-capabilities-to-pkg.md) 走 ADR + CHANGELOG 迁移说明，由审查把关。
- ARCHITECTURE 已定义的例外继续有效：形参/返回类型直接引用 `internal/` 的导出符号不计入兼容承诺，门禁须能识别并豁免，不得误报。
- 默认按无 build tag 的导出面检查；`sqlite memory` 可选面是否纳入由 API-001 评估决定并记录理由。
- 正式发布（tag、GitHub Release、镜像推送）仍按 [发布流程](../release.md) 执行，且每一步须用户明确指令；本计划不自动发布。

## 工具结论（API-001 可调整，调整须写入 ADR）

- 默认候选 `golang.org/x/exp/cmd/apidiff`（Go 官方实现，输出 compatible/incompatible 明细）。`x/exp` 无稳定性承诺，必须像 govulncheck 一样**固定版本**以 `go run ...@<version>` 调用，不引入模块依赖。
- 备选 `joelanford/go-apidiff`（对 GitHub Actions 更友好）；若官方工具误报率或接线成本不可接受再切换，理由写入 ADR。
- 基线为最近发布 tag（当前 `v0.1.6`）；PR/推送对比基线与当前工作树。基线解析失败（tag 缺失、网络不可用）必须显式失败，不得静默跳过——与漏洞门禁同一纪律。

## 任务分解

<a id="api-001"></a>

## API-001：API 兼容门禁评估与基线契约

**前置任务**：BASE-001。优先级 P1；设计前置，不改产品代码。

**先读 / 主要修改范围**：[ARCHITECTURE 分层](../ARCHITECTURE.md#api-兼容性分层)、[包契约](../reference/README.md)、`docs/CHANGELOG.md` 版本口径说明、[release.md](../release.md)、`.github/workflows/build.yml`、`scripts/check-vuln.sh`（门禁纪律参照）。产出 ADR 0009 与本任务证据。

**小步骤**：

1. 固定 apidiff 版本，对 `v0.1.6` → 当前 HEAD 实际跑一遍全部稳定 `pkg`，记录真实差异清单；区分真实破坏、internal 引用例外与工具误报。
2. 确定分层执行策略的机器表达（包清单来源、实验包豁免方式、例外识别规则），确定 build tags 覆盖面。
3. 用构造的破坏性变更夹具验证工具能报 incompatible、兼容新增不误报；记录误报与处理规则。
4. 写 ADR 0009：工具与版本、基线策略、分层规则、例外与豁免、失败语义；被否决方案写明理由。

**验收条件**：

- [x] 有 `v0.1.6` → HEAD 的真实差异清单与分类结论，不是工具介绍。
- [x] 破坏性/兼容性夹具各至少 1 个，工具输出与预期一致。
- [x] ADR 0009 覆盖工具版本、基线、分层、例外、失败语义；包清单与 ARCHITECTURE 一致。

**范围外**：不改 CI、不动产品代码、不打 tag。

<a id="api-002"></a>

## API-002：apidiff CI 门禁接线

**前置任务**：API-001。优先级 P1；按 ADR 0009 把门禁接入 CI 与本地 make 目标。

**先读 / 主要修改范围**：ADR 0009、`.github/workflows/build.yml`、Makefile、`scripts/`（新增检查脚本与失败自测，参照 `check-vuln.sh` / `test-vuln-script` 模式）。

**小步骤**：

1. 新增 `make check-api`（或等价目标）：解析基线 tag、按分层跑 apidiff、稳定层 incompatible 即非零退出；实验层输出报告。
2. 新增失败路径自测（构造破坏性变更证明门禁会挂），接入 Quality；基线缺失/工具下载失败显式失败。
3. 本地全量回归 + 远端 CI 验证门禁在真实 PR/推送上运行；记录运行耗时，必要时独立 job 避免拖慢 Quality。
4. 更新开发/发布文档：破坏性变更的处理流程（稳定层走版本与迁移、实验层走 ADR）。

**验收条件**：

- [x] 门禁在 CI 实际运行并通过；失败自测证明破坏性变更会被拦截，基线缺失不会静默跳过。（run 43829471517 全绿）
- [x] 稳定层当前 HEAD 相对 `v0.1.6` 无未豁免的 incompatible（若有，先按流程处理再收口）。
- [x] 本地 make 目标与 CI 行为一致，文档已更新。

**范围外**：不打 tag、不发布；不改分层承诺本身。

<a id="api-003"></a>

## API-003：tag 与 CHANGELOG 版本欠账清理

**前置任务**：API-001。优先级 P2；可与 API-002 并行，一次只领取一个。

**先读 / 主要修改范围**：`docs/CHANGELOG.md`、`docs/release.md`、`git tag -l`、各历史证据中的候选 commit。

**小步骤**：

1. 逐条核对 `0.3.0`–`0.5.0` 内部编号对应的实际发布 tag（或确认从未发布），建立映射表；不改写历史事实。
2. 修订 CHANGELOG 版本口径：内部编号明确标注映射或「未发布」，release 链接指向真实 tag。
3. 在 release.md 固化「tag ↔ CHANGELOG 条目 ↔ 候选记录」三链路规则，作为后续发布的检查项。

**验收条件**：

- [x] 每个内部编号有明确结论（对应 tag / 未发布），映射有 git 证据支撑。
- [x] CHANGELOG 不再有不可达 release 链接；三链路规则进入发布检查项。

**范围外**：不补打历史 tag、不重写历史 Release。

<a id="api-004"></a>

## API-004：v0.2.0 候选验收与发布

**前置任务**：API-002, API-003。优先级 P1；门禁生效后的首个真实发布，按 [release.md](../release.md) 全流程执行。

**先读 / 主要修改范围**：release.md、SHIP-001/002/003 与 SEC-002/003 证据（产物扫描与协议矩阵流程）、GoReleaser 配置。

**小步骤**：

1. 确定候选 commit，用当前安全工具链（Go 1.26.11+）重建五平台产物，执行产物级漏洞扫描（`scripts/check-vuln.sh binary`）；重建双架构镜像并扫描镜像内二进制。
2. 按 SHIP-001 流程在当前候选重绑真实 Provider 协议矩阵（runner 变更后的证据须重新生成，沿用既有纪律）。
3. Release Dry Run、五平台归档 Smoke、Docker Smoke 全绿；CHANGELOG 新增 v0.2.0 条目并与 tag 对齐。
4. **用户明确指令后**才创建 `v0.2.0` tag 与 GitHub Release、推送镜像；发布后核对安装入口与产物哈希，回填证据。

**验收条件**：

- [x] 五平台产物与双架构镜像均有构建信息、哈希与无告警扫描结果。
- [x] 真实 Provider 协议矩阵在当前候选通过，证据绑定候选 commit。（SHIP-001 口径 OpenAI Chat + Anthropic 各 3/3；Responses 渠道 8×400 作为外部边界记录，见证据）
- [x] tag、CHANGELOG、release.md 候选记录三链路一致；发布动作均有用户指令记录。（tag v0.2.0=d907dde ↔ CHANGELOG [0.2.0] - 2026-11-16 ↔ release.md 候选记录；用户 2026-11-16 授权完整发布）
- [x] 干净安装验证通过；历史产物边界如实标注。（Releases 归档隔离 HOME 冒烟链、Go install @v0.2.0、GHCR 双架构镜像运行均通过；Responses 渠道边界与历史产物口径见 API-004 证据）

**范围外**：不打 `v1.0.0`；1.0 评估（含实验包毕业标准）另立任务。

## 风险与对策

| 风险 | 对策 |
|---|---|
| apidiff 误报（internal 引用例外、类型别名、符号移动） | API-001 先用真实差异清单校准；豁免规则写入 ADR，逐条可追溯 |
| `x/exp` 工具自身不稳定 | 固定版本；升级工具版本视为门禁变更，须重跑夹具自测 |
| 基线 tag 与 HEAD 差异过大导致首轮门禁失败 | API-001 先出差异清单；真实破坏先按迁移流程处理，不放宽门禁 |
| 门禁拖慢 CI | 记录耗时；必要时独立 job，与 Quality 并行 |
| 发布流程长、外部 Provider 容量边界 | 沿用 LOAD-001 的本地确定性 Provider 与真实 Provider 分离口径 |

## 后续衔接（不在本计划内）

v0.2.0 发布且门禁稳定运行后，另立任务评估 1.0：定义实验四包毕业标准（API 冻结观察期、真实嵌入者反馈）、核心/扩展层承诺复核、`v1.0.0` 发布窗口。本计划不承诺 1.0 日期。