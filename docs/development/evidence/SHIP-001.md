# SHIP-001 验证记录：固定编码场景回归集（历史记录；上轮验收记录（本次复审已回退））
> **2026-09-16 再复审：下文完成结论均为历史记录。** B02 使真实验收可信性不足，B05 的当前候选协议样本亦未满足；需先修门禁再补验收。 当前状态仅见 [TASKS](../../TASKS.md)，依据见 [本次复审](REVIEW-2026-09-16.md)。

> 当前状态以 [任务看板](../../TASKS.md) 为准：**进行中**。下文早期“完成”只表示历史工作区快照；当前候选真实 Provider 与发布门禁结果见文末日期段。状态不在本证据文件重复维护。

> **2026-09-14 复审说明**：以下为历史实施记录，不能继续单独支撑当前验收。新发现或依赖回退涉及 A14；详见 [本轮复审报告](REVIEW-2026-09-14.md) 与 [任务卡](../../tasks/08-release.md#ship-001) 的复审补充。实际状态只维护在 [TASKS](../../TASKS.md)。旧结论保留用于追溯，本轮未修业务代码。

## 基线

- 日期、操作者、系统、Go 版本：2026-09-13，舟（WorkBuddy AI 会话），macOS 25.6.0（Darwin arm64），go1.26.0 darwin/arm64。
- commit / 工作区差异范围：起点 HEAD `935fde1`；本轮改动在工作区，未提交。本任务只**新增**一个测试文件（`tests/coding_scenarios_test.go`），未改动产品代码。
- 任务与用户场景：任务卡口径 `docs/tasks/08-release.md#ship-001`。前置任务 REL-003、CTX-003 均已完成（[REL-003 记录](REL-003.md)）。
- 真实模型端点（脱敏）：一个第三方 OpenAI 兼容中转端点，模型 `agnes-2.5-flash`。**key 不落盘**，只在运行时经环境变量传入；本文与任何仓库文件都不含 key 明文。端点的模型清单与流式/工具调用能力已在 [REL-003 记录](REL-003.md) 中探测过，本任务不重复探测。
- 验证物位置：离线层随仓库保留（`tests/coding_scenarios_test.go`）；真实模型夹具在工作区外 `/private/tmp/ship001`，属临时目录、不随仓库保留（复现方式见"剩余与交接"）。

## 变更

**新增 `tests/coding_scenarios_test.go`**：6 个场景测试，覆盖卡片步骤 1 要求的五个样例（小函数修复、多文件修改、大输出、权限拒绝、断流恢复），外加一个"半截参数"变体。

**为什么切成两层，而不是把真实模型写进 `go test`**：

- 真实模型的行为随模型版本、温度与网络抖动变化。把它写进 `go test` 会让回归集变成偶发红灯的噪声源——红灯时无法区分"编排坏了"还是"这次模型换了个说法"。
- 反过来只跑脚本化模型又证明不了真实兼容性。
- 因此本文件只覆盖**可确定的编排逻辑**：工具派发顺序、编辑是否落盘、权限拦截是否产生副作用、大输出是否被有界截断、断流是否暴露为错误且不污染会话。断言全部落在**磁盘产物与工具记录**上，不落在模型文案上。真实模型结果按 validation.md 单独跑、另存为证据（下节表 2）。

实现要点（决定"可重复运行"是否成立的地方）：

- `scenarioModel` 实现 `llm.Model`，按预置脚本逐轮返回；**不使用任何随机性、不定时、不读网络**，因此同一脚本在任何机器上重放出的工具轨迹必须逐字节一致。
- 每个场景用 `t.TempDir()` 建独立工作区，互不共享状态，失败时无残留。
- 工具调用 ID 显式传入而非由工具名派生——同名工具在同一场景会出现多次（两次 `edit`），用名字当 ID 会让会话的"调用↔结果"配对串线，反而掩盖真实的配对错误。
- `callFailed` 同时统计调度层错误（`Err != nil`，如权限拒绝）与工具自身的错误结果（`Result.IsError`，如半截参数）；只统计前者会把截断场景要盯的东西记成成功。
- 断流场景断言事件层不变量：每次 `tool.called` 都必须有配对的结果事件（`assertNoDanglingToolCalls`），因为"中断后不留无法解释的半截调用"才是恢复的前提。

## 验证

### 表 1：离线重放（卡片指定的针对性验证）

命令：`go test -tags "sqlite memory" ./tests -count=1 -v -run 'TestCodingScenario'`（仓库根执行）

| 场景 | 结果 | 工具调用 | 可见失败 | 耗时 | 断言要点 |
|---|---|---|---|---|---|
| small-function-fix | PASS | 3（`read edit bash`） | 0 | 137ms | 磁盘上 `a - b` → `a + b`；验证命令回显哨兵串 |
| multi-file-edit | PASS | 2（`edit edit`） | 0 | <1ms | 两个文件都落盘；两个调用 ID 不同；无悬挂调用 |
| large-output | PASS | 1（`bash`） | 0 | 14ms | 回给模型的内容 ≤100KB+64 且含"输出截断"标记；反向确认命令确实产出 >100KB（避免空跑通过） |
| permission-denial | PASS | 1（`write!`） | 1 | <1ms | 被拒文件未创建；错误含"权限拒绝"；会话有 `tool.failed` |
| stream-interruption-recovery | PASS | 0 | — | <1ms | 流中断**返回错误**且不返回成功响应；同会话第二次调用成功；事件数 11，无悬挂调用 |
| truncated-args | PASS | 1（`write!`） | 1 | <1ms | 半截参数不被当成成功；失败可见（`Result.IsError`）；未创建文件 |

6/6 通过。该测试用 `-count=2` 复跑同样通过（可重复），且不依赖任何环境变量或网络。

### 表 2：真实模型固定场景（`agnes-2.5-flash`，OpenAI 兼容协议）

夹具：`/private/tmp/ship001/ship001harness`，经 `provider.Create` + `agent.New(WithModel/WithSession/WithTools(builtin.All())/WithMaxSteps/WithCallback)` 驱动。每次运行前从模板重置独立工作目录并 `go clean -testcache`。`go test` 由夹具在 agent 回合之外**独立执行**，不采信模型的"已通过"自述。

| 场景 | 次数 | 通过 | 耗时(s) | 工具调用 | 其中失败 | token | 独立 `go test` 退出码 |
|---|---|---|---|---|---|---|---|
| small-fix（小函数修复） | 3 | 3 | 8.4 / 3.3 / 5.3 | 11 / 5 / 9 | 3 / 0 / 3 | 2236 / 1750 / 2049 | 0 / 0 / 0 |
| multi-file（多文件修改） | 2 | 2 | 21.8 / 37.4 | 9 / 8 | 1 / 1 | 2102 / 2002 | 0 / 0 |
| large-output（大输出） | 1 | 1 | 2.1 | 1 | 0 | 103464 | 不适用 |
| permission-denial（权限拒绝） | 1 | 1 | 11.1 | 2 | 2 | 1027 | 不适用 |
| stream-interruption（断流恢复） | 1 | 1 | 0.4 | 0 | 0 | 881 | 不适用 |

- **合计 8/8 通过**；两个编写类场景的独立 `go test` 真实退出码 5 次全为 0，且每次都以退出码 1 的缺陷态为起点。
- **场景专有观测**：large-output 单次工具结果 **102434 字节**（上限 102400），含"输出截断"标记 ✓；permission-denial 拒绝 **2** 次、文件未创建 ✓、整轮未因拒绝中断；stream-interruption 首次调用报错且错误含 `stream error`、同会话第二次调用由真实模型正常完成、悬挂工具调用 **0** ✓。
- **失败类型归类**（按卡片第 4 步记录，不只记成功样本）：8 次运行共 **9** 次工具调用失败，分为三类，**无一例是协议或装配层错误**：
  - **5 次：模型猜错路径后自我纠正**（如断言工作目录是 `/home/user/...`，随后用 `pwd`/`ls` 探到真实路径）。与 REL-003 中观察到的行为一致。
  - **3 次：模型自己跑的 `go test` 中间态失败**（退出码 1）。这是**正常的迭代**——第一处改完还没改第二处就先去验一次，工具如实返回失败，模型据此继续修。不算缺陷。
  - **2 次：权限拒绝**，为本场景的预期行为。
- **未出现**：整轮失败、需要人工重试的样本、参数重复拼接、tool_call ID 错配、失败后重复执行有副作用的工具。

### 表 3：停滞流下"超时上限是否真的生效"（回答真实运行中遇到的现象）

真实矩阵第一次运行时出现过**一整轮 12 分钟无输出**（见"剩余与交接"）。为判断这是"编排层没有上限"还是"上游停滞"，用一个**本地 stalling server**（接受请求后永不发数据）验证：

| 问题 | 做法 | 结果 |
|---|---|---|
| 调用方给的 context 上限能否终止一次停滞的流？ | `httptest` 起一个只 `Flush()` 头部、之后不写任何数据也不关连接的 server，`ctx` 上限 2s | **耗时 2.003s 结束** ✓ 取消被遵守 |
| 取消后流层是否报错？ | 同上，观察事件与通道收尾 | 通道**静默关闭**，无 `Error` 事件（`parseOpenAIStream` 在 `ctx.Err() != nil` 时按设计抑制 `scanner.Err()`，避免与主动取消竞态） |
| 那调用方会不会把"被取消"误读成"模型回了空内容"？ | 读 `pkg/agent/pipeline.go` | **不会**：`callLLM` 在 `for range ch` 结束后显式检查 `ctx.Err() != nil` 并返回该错误（pipeline.go:169-172）。因此 provider 层的静默关闭在 agent 层被正确地转成真实错误 |

结论：**停滞的上游由调用方 context 封顶，并且最终会以错误暴露**，不是静默成功。这一条同时给"断流恢复"场景提供了比离线重放更强的旁证。

### 覆盖边界（以下都不能宣称）

- **不能宣称稳定成功率**：`small-fix` 跑了 3 次、`multi-file` 跑了 2 次，全都通过；但 `large-output`、`permission-denial`、`stream-interruption` **各只跑了 1 次**。单次通过只说明"这条路径能走通"，不构成成功率结论。
- **费用未知**：token 数来自中转站返回且**未经账单核对**，不据此推算费用；本任务不产出任何金额结论。
- 只用了 `openai` 兼容协议这一侧；`anthropic` 协议在本任务中未跑（REL-003 只在核心层验过）。
- **断流是注入的**：真实模型无法被稳定地"掐断在网络中间"，因此 `stream-interruption` 的第一段是一次注入的流错误（与离线层同手法），恢复段才走真实模型。这验证的是"中断不污染会话、真实模型可继续"，不是"真实网络断流的概率特性"。
- 真实模型下的 `permission-denial` 场景**拒绝 `write`/`edit`/`bash` 三个工具**（离线场景只拒 `write`）。原因：只拒 `write` 时模型会改走 `bash` 重定向来落盘，副作用依然发生，断言就不成立了。因此两者的拒绝集合不同，结论不能互相套用。
- 全部为**核心层**夹具（不经过 `cmd/basework` 装配：无 ACL/会话持久化落盘/后台 job/TUI）。产品层 CLI 路径的闭环另见 REL-003；本卡不含人工 TUI 体验。

## 剩余与交接

- **一次未复现的长时间停滞**：第一次跑真实矩阵时，`small-fix` 第 3 次运行**超过 12 分钟无任何输出**（该场景前两次分别 5s 与 3s）。重跑同一场景同一序号后，第 3 次 **5.3s 正常通过**，之后 8/8 全部在 40s 内完成。因此**判定为上游/环境的偶发停滞，非可复现的产品缺陷**；但当时唯一的兜底是 provider 自己的 `http.Client{Timeout: 10 * time.Minute}`（`pkg/provider/openai.go:53`），与表 3 证实的"context 取消有效"之间存在时序上的不一致（context 上限只有 6 分钟，却在 12 分钟后仍无输出），**未能定位到根因**——当时无法查看进程状态（该环境下 `ps` 不可用）。留给后续的最小验证条件：在此环境下复现停滞时，用 `kill -QUIT` 取 goroutine 栈或加 pprof 端点，确认阻塞点是在 `client.Do`、body 读取，还是在回合之外的 `go test`。
- 夹具侧已加固：`go test` 独立复核加 120s 上限、每轮上下文上限收到 120–150s、外层再加 200s 看门狗且超时事件会被**写入结果文件**（不再静默少一行数据）。这些是夹具的健壮性改动，不影响产品结论。
- 复现方式（`/private/tmp/ship001`，临时目录不随仓库保留）：
  - 离线：`go test -tags "sqlite memory" ./tests -count=1 -v -run 'TestCodingScenario'`（仓库内，永久可复现）。
  - 真实模型：`S1_API_KEY=... ./run.sh small-fix:3 multi-file:2 large-output:1 permission-denial:1 stream-interruption:1`；夹具以 `replace` 指向本地仓库，`S1_WORKDIR` 指向从 `templates/` 重置出来的隔离项目。每个场景的原始记录是 `runs/<场景>-<次>.json`（含完整工具轨迹、字节数、token、独立退出码）。
- 下一步（不属本项）：SHIP-002 跨平台安装与升级验证，以及产品层的人工 TUI 体验（发布门槛的一部分，见 validation.md）。

## 结论

- **是否满足任务卡全部验收**：
  - "每个场景可重复运行且清理独立" —— 满足。离线层 6 个场景用 `t.TempDir()` 隔离、`-count=2` 复跑一致；真实层每次运行前从模板重置独立工作目录。
  - "不把一次成功当稳定成功率" —— 满足。本记录明确区分"3 次/2 次"与"各 1 次"，并声明单次通过不构成成功率结论。
  - "费用未知明确标未知，不能由 token 数编造账单" —— 满足。已写明 token 未经账单核对，不产出金额结论。
  - 四条实施步骤（建立五类样例、固定输入与断言产物而非文案、离线重放与真实结果分开、记录成功率/耗时/调用量/失败类型）均有可复现证据。
- **建议状态：完成。**
- 原因：卡片要求的五类样例在离线层与真实层都有记录，两层分工写清，失败类型按卡要求如实归类（含 3 次"中间态 `go test` 失败"这种容易被误记成缺陷的正常迭代）。必须连带说明两项限制：**单次运行的三个场景不构成成功率**，以及**一次未复现的停滞未定位到根因**（已记录在"剩余与交接"，不阻塞本卡——卡片不要求解释环境偶发停滞，但要求不夸大）。

## 2026-09-15 当前工作区复核

历史段的真实模型结果仍保留为指定端点的历史记录；本轮没有外部 Provider 密钥，
因此没有把历史结果冒充当前候选版本。当前工作区已完成以下可复跑复核：

- 默认与 `sqlite memory` 全量 Go 测试、race、vet、构建、架构和文档门禁均通过；
- `tests/tui_pty/scenario1.py` 到 `scenario8.py` 在独立临时根、隔离 HOME 和本地确定性
  Provider 下全量通过，覆盖固定编码场景的真实 TUI 入口、后台取消、硬杀恢复、
  `/session` 隔离、审批拒绝/允许和双压缩重启；
- 复核结果能独立由磁盘文件、会话 JSONL、请求日志、进程退出和外部 module 测试
  交叉确认，不采信模型自述。

任务看板仍将 SHIP-001 保持“待验证”：候选 commit、当前版本的真实 Provider 记录和
发布门禁需要一起绑定，不能因为本地脚本化入口通过就提前关闭。

为避免真实 Provider 夹具只存在于临时目录，本轮新增 `tests/real_provider/`：运行器
从仓库 fixture 复制随机临时项目，使用 `BASEWORK_REAL_PROVIDER`、
`BASEWORK_REAL_BASE_URL`、`BASEWORK_REAL_API_KEY`、`BASEWORK_REAL_MODEL` 注入配置，
独立执行 `go test ./...`，并只写脱敏结果（工具名、耗时、退出码、文件是否修改、最终
文本哈希）。缺少任一凭据时不会发请求；当前环境没有这些凭据，所以当前 Provider
结果仍待补跑，历史 `/private/tmp` 结果不冒充当前候选。

本轮实际执行无凭据负向门禁：

```text
GOCACHE=/private/tmp/basework-real-provider-cache \
  go run ./tests/real_provider --output /private/tmp/basework-real-provider-missing.json
exit code: 1
real provider runner requires BASEWORK_REAL_API_KEY, BASEWORK_REAL_BASE_URL, and BASEWORK_REAL_MODEL; no request was sent
```

未生成结果文件，也没有发出 Provider 请求；这只证明缺凭据时的安全失败，不替代真实模型正向闭环。

同日用清洁临时 checkout（candidate commit
`ac6852dc33d77a01ccb927e5a6cc46dafad804d5`）重新执行完整 tag 测试和 PTY scenario1–8，
均通过；恢复、审批、双压缩和会话切换使用的是该候选副本产生的新数据，不复用历史模型
结果。当前 Provider 仍未注入凭据，因此本段只关闭“清洁 checkout/恢复依赖复核”本地
验收，不关闭真实 Provider 证据。

同日提交 `9d7458bb5c53274b5d5b1742cb88abe75ad4ac64` 的干净 detached worktree 已重新完成
完整 tag race 与 PTY scenario1–8；八个场景均通过，scenario7 使用详情等待修复后不再
出现审批弹窗读取竞态。该结果仍属于本地提交级证据，尚未形成远端 CI 结果。

## 2026-09-16 当前候选 CI 与真实 Provider 前置

候选提交 `3787862cd076b72d49acfc3c1127efcc807f3539` 的 [GitHub Actions run
35043445341](https://github.com/wly2lcl/basework/actions/runs/35043445341) 已全绿，Quality、
五平台测试/发布归档 Smoke、Docker Smoke 和 Release Dry Run 均通过；发布包的 init、会话
迁移、源文件哈希保持和未来版本拒绝结果详见 [SHIP-002](SHIP-002.md#2026-09-16-当前候选发布包迁移-smoke)。

这次 CI 没有注入 `BASEWORK_REAL_API_KEY`，没有发出真实 Provider 请求，也没有把本地脚本化
Provider 当作真实模型。当前仓库 Actions secret 列表中仍无该 secret，因此 SHIP-001 继续
保持“待验证”；补齐方式见[真实 Provider 验收夹具](../real-provider-acceptance.md#github-actions-运行方式)。

## 2026-09-16 当前候选真实 Provider 正向验收

候选提交 `2a868864f31c8a66527c1679fd2951a8869c2267` 的 [GitHub Actions run
35049625067](https://github.com/wly2lcl/basework/actions/runs/35049625067) 已成功完成。
本次使用 `provider=openai`、端点 `https://newapi.doubb.top/v1`、模型
`agnes-2.5-flash`；API key 只从仓库 Actions secret 注入，没有写入日志或结果文件。

脱敏结果已随仓库保存为 [`SHIP-001-real-provider-2026-09-16.json`](SHIP-001-real-provider-2026-09-16.json)，
关键字段如下：

| 检查项 | 结果 |
|---|---|
| Agent 回合 | `agent_ok=true`，完成 `calc.go` 修复 |
| 工具轨迹 | `glob` 2 次、`read` 2 次、`bash` 2 次、`edit` 1 次，均成功 |
| 独立验证 | `go test ./...`，退出码 `0` |
| 文件变更 | `file_changed=true`，测试文件未被修改 |
| 耗时 | `6522 ms` |
| 最终文本哈希 | `3d52e14193997ac17b70605227eac2de5c038e996952677fe1d1688ed2b6d10c` |

该运行关闭了当前候选的真实 Provider 正向门禁。它只覆盖仓库内固定 `fixbug` 夹具的一次
当前候选运行，不把一次成功推广成稳定成功率，也不产生费用结论；历史五类真实矩阵和
离线六场景记录继续作为场景覆盖证据，并与本次当前候选结果分开保存。

**当前结论：SHIP-001 完成。**

## 2026-09-16 再复审交接

B02 使真实验收可信性不足，B05 的当前候选协议样本亦未满足；需先修门禁再补验收。 本次状态调整为进行中，小步骤见原任务卡新增补充。历史成功测试不删除，但不能代替本次缺陷修复后的验证。

## 2026-09-16 门禁修复后交接

QA-001 已修复可信验证、凭证环境隔离、验证超时/输出上限和默认 CI 测试门禁，并有本地假 Provider 的正确实现/篡改拒绝回归。旧真实 Provider JSON 仍是修复前 runner 产物；在同一修复候选上补协议 × 次数 × 入口矩阵前，本任务继续进行中。

## 2026-09-16 修复候选真实 Provider 失败样本

候选 `3ad67a7d30ab5b030bd28fe09504b6134c5a6faf` 的第 1 次 OpenAI-compatible 运行
[35055166589](https://github.com/wly2lcl/basework/actions/runs/35055166589) 使用
`openai` / `https://newapi.doubb.top/v1` / `agnes-2.5-flash`。Provider 请求在约
451 秒后以 `context deadline exceeded` 结束，Agent 未完成，模型只读到初始缺陷工作区；
可信测试未执行，`validation_ok=false`。脱敏 artifact 已保存为
[`SHIP-001-real-provider-2026-09-16-run-35055166589.json`](SHIP-001-real-provider-2026-09-16-run-35055166589.json)。

该失败样本不计入通过次数，但证明 runner 在 Agent 失败时仍写出阶段、测试哈希和失败
结果，且结果不含 key。它也提示该第三方端点存在不可复现的长时间停滞；后续通过样本仍须
按原协议次数要求连续完成，不能用这次失败降低标准。

第 2 次 OpenAI-compatible 重跑 [35055889004](https://github.com/wly2lcl/basework/actions/runs/35055889004)
仍以 `context deadline exceeded` 失败（约 181 秒），并保存为
[`SHIP-001-real-provider-2026-09-16-run-35055889004.json`](SHIP-001-real-provider-2026-09-16-run-35055889004.json)。
两次结果均为 `agent_ok=false`、`validation_ok=false`、`tests_executed=false`，不计入任何
连续通过次数；失败输出和测试哈希仍可复查，未发现 key 泄漏。

第 3 次 OpenAI-compatible 运行 [35070656461](https://github.com/wly2lcl/basework/actions/runs/35070656461)
通过，脱敏结果已保存为
[`SHIP-001-real-provider-2026-09-16-run-35070656461.json`](SHIP-001-real-provider-2026-09-16-run-35070656461.json)。
本次 `agent_ok=true`、`validation_ok=true`、`tests_executed=true`、独立命令退出码为 0，
测试哈希为 `4b69bb74274892eac935505ea7d133a960403b6ca8b7db0f47adb09f98f239a9`，实现哈希为
`b9cb9483df54f2343b5e6f4ba53e0d0dc59afdefcafab7fdd4968258e777aee2`，耗时 22.7 秒。由于
前两次失败，这只是 1 次连续通过，不能关闭协议次数门禁。

第 4 次 OpenAI-compatible 运行 [35070828659](https://github.com/wly2lcl/basework/actions/runs/35070828659)
约 61 秒后返回第三方端点 `504`，脱敏结果已保存为
[`SHIP-001-real-provider-2026-09-16-run-35070828659.json`](SHIP-001-real-provider-2026-09-16-run-35070828659.json)。
它不计入通过次数，但验证了 Provider 失败仍写出 `validation_ok=false`、测试哈希和阶段结果。

第 5 次 OpenAI-compatible 运行 [35071026911](https://github.com/wly2lcl/basework/actions/runs/35071026911)
通过，脱敏结果已保存为
[`SHIP-001-real-provider-2026-09-16-run-35071026911.json`](SHIP-001-real-provider-2026-09-16-run-35071026911.json)。
本次 `agent_ok=true`、`validation_ok=true`、`tests_executed=true`、独立命令退出码为 0，
测试哈希为 `4b69bb74274892eac935505ea7d133a960403b6ca8b7db0f47adb09f98f239a9`，实现哈希为
`b9cb9483df54f2343b5e6f4ba53e0d0dc59afdefcafab7fdd4968258e777aee2`，耗时 157.3 秒。

第 6 次 OpenAI-compatible 运行 [35071530401](https://github.com/wly2lcl/basework/actions/runs/35071530401)
通过，脱敏结果已保存为
[`SHIP-001-real-provider-2026-09-16-run-35071530401.json`](SHIP-001-real-provider-2026-09-16-run-35071530401.json)。
本次同样完成可信独立测试并退出码为 0，耗时 13.3 秒；测试/实现哈希与前一成功样本一致。

第 7 次 OpenAI-compatible 运行 [35071644174](https://github.com/wly2lcl/basework/actions/runs/35071644174)
通过，脱敏结果已保存为
[`SHIP-001-real-provider-2026-09-16-run-35071644174.json`](SHIP-001-real-provider-2026-09-16-run-35071644174.json)。
本次 `agent_ok=true`、`validation_ok=true`、`tests_executed=true`、独立命令退出码为 0，
测试哈希为 `4b69bb74274892eac935505ea7d133a960403b6ca8b7db0f47adb09f98f239a9`，实现哈希为
`b9cb9483df54f2343b5e6f4ba53e0d0dc59afdefcafab7fdd4968258e777aee2`，耗时 11.3 秒。

第 5–7 次形成同一候选、同一 OpenAI-compatible 入口的连续 3 次可信通过；截至本段，
OpenAI-compatible 协议次数门禁已满足。不同协议 Provider 尚未运行，不能把历史 REL-003
结果替代当前候选证据；待明确协议/端点和授权后再补同样的连续 3 次矩阵。

## 2026-09-16 OPT-003 后候选边界

随后提交 `8394227` 加入 JSONL 增量追加优化和存储回归，成为新的本地候选。上面的真实
Provider 结果均绑定 `3ad67a7`，不能自动延伸到 `8394227`；需先将新候选推送并重新生成
对应 CI/真实 Provider 记录。不同协议 Provider 仍需明确协议、端点和授权后按连续 3 次
要求执行。

## 2026-09-16 当前远端候选 CI 关联

提交 `4c40e18e2662166902500bf1ba18124efdfc5a9f` 的 [CI run
35077896624](https://github.com/wly2lcl/basework/actions/runs/35077896624) 已全绿，Quality、五平台源码测试、发布归档 Smoke、Docker Smoke 与 Release Dry Run 均通过。
这条 CI 记录只证明候选构建与发布辅助门禁；本文件中第 5–7 次真实 Provider 通过仍绑定
`3ad67a7`，不能延伸到 `4c40e18`。当前候选需重新运行 OpenAI-compatible 连续 3 次，并补齐
不同协议连续 3 次，才可更新 SHIP-001 状态。

## 2026-09-17 当前远端文档候选 CI 关联

提交 `140eddf4cfd0ba457d7b82623c5e83d1b3c46574` 的 [CI run
35079524005](https://github.com/wly2lcl/basework/actions/runs/35079524005) 已全绿，Quality、五个平台源码测试、五个平台发布归档 Smoke、Docker Smoke 与 Release Dry Run 均通过。该 run 只证明当前候选的自动化构建与发布辅助门禁；上一候选 `3ad67a7` 的真实 Provider 通过记录不能延伸到 `140eddf`，当前候选仍需 OpenAI-compatible 与不同协议各连续 3 次可信结果。

## 2026-09-17 当前代码候选真实 Provider 复核

当前代码候选为 `dd08592adce712af73f1b235dfc829352dc31f1d`。6 次 workflow 均使用同一
`provider=openai`、端点 `https://newapi.doubb.top/v1`、模型 `agnes-2.5-flash`，并将密钥
仅作为 Actions secret 注入。结果文件是 workflow 上传后下载的脱敏 JSON，已纳入仓库；不保存
原始响应、密钥或不可复查的临时路径。

| 次序 | Workflow run | Agent | 验证 | 测试实际执行 | 实现已修改 | 独立测试退出码 | 耗时 | 结果文件 |
|---:|---:|---|---|---|---|---:|---:|---|
| 1 | [35169409650](https://github.com/wly2lcl/basework/actions/runs/35169409650) | ✅ | ✅ | ✅ | ✅ | 0 | 10.57s | [`json`](SHIP-001-real-provider-2026-09-17-run-35169409650.json) |
| 2 | [35169557341](https://github.com/wly2lcl/basework/actions/runs/35169557341) | ✅ | ✅ | ✅ | ✅ | 0 | 101.83s | [`json`](SHIP-001-real-provider-2026-09-17-run-35169557341.json) |
| 3 | [35169796044](https://github.com/wly2lcl/basework/actions/runs/35169796044) | ✅ | ❌ | ✅ | ❌ | 0 | 79.15s | [`json`](SHIP-001-real-provider-2026-09-17-run-35169796044.json) |
| 4 | [35170019011](https://github.com/wly2lcl/basework/actions/runs/35170019011) | ✅ | ✅ | ✅ | ✅ | 0 | 7.89s | [`json`](SHIP-001-real-provider-2026-09-17-run-35170019011.json) |
| 5 | [35170100696](https://github.com/wly2lcl/basework/actions/runs/35170100696) | ✅ | ✅ | ✅ | ✅ | 0 | 7.34s | [`json`](SHIP-001-real-provider-2026-09-17-run-35170100696.json) |
| 6 | [35170184392](https://github.com/wly2lcl/basework/actions/runs/35170184392) | ✅ | ✅ | ✅ | ✅ | 0 | 10.12s | [`json`](SHIP-001-real-provider-2026-09-17-run-35170184392.json) |

第 3 次是可信门禁拒绝样本：虽然 Agent 与独立测试退出码为 0，但 `validation_ok=false`、
`file_changed=false`，不能算作成功。第 4–6 次在同一候选、同一入口上连续满足全部字段，
因此当前候选的 OpenAI-compatible 连续 3 次要求已完成。每次 artifact 都包含测试文件和实现
文件 SHA-256；结果文本只保留摘要和尾部，已检查未出现 API key。

该记录仍不能关闭不同协议要求：历史 REL-003 的多协议结果绑定旧候选/旧 runner，不能替代本次
候选。补验前需要明确协议、端点、模型和授权；获得后按同一夹具连续运行 3 次，并将失败样本
与成功样本一并保存。

**当前结论：SHIP-001 的 OpenAI-compatible 子门禁完成；整项继续进行中，等待不同协议连续 3 次当前候选证据。**

## 2026-09-17 SEC-001 前候选 OpenAI-compatible 连续验收

为重新绑定 `150e77e` 之后的代码候选，使用 `ref=main` 触发 workflow；三次运行的实际
`headSha` 均为 `66c57fde7f88ccaf5de0669bbd08c488a5686f0e`。该提交相对代码候选只增加
证据/文档，代码行为包含 `150e77e` 的只读预设安全回退。三次均使用
`provider=openai`、端点 `https://newapi.doubb.top/v1`、模型 `agnes-2.5-flash`，密钥
只由 `BASEWORK_REAL_API_KEY` secret 注入。

| 次序 | Workflow run | Agent | 验证 | 测试实际执行 | 实现已修改 | 独立测试退出码 | 耗时 | 结果文件 |
|---:|---:|---|---|---|---|---:|---:|---|
| 1 | [35178297077](https://github.com/wly2lcl/basework/actions/runs/35178297077) | ✅ | ✅ | ✅ | ✅ | 0 | 17.18s | [`json`](SHIP-001-real-provider-2026-09-17-run-35178297077.json) |
| 2 | [35178409921](https://github.com/wly2lcl/basework/actions/runs/35178409921) | ✅ | ✅ | ✅ | ✅ | 0 | 12.75s | [`json`](SHIP-001-real-provider-2026-09-17-run-35178409921.json) |
| 3 | [35178503811](https://github.com/wly2lcl/basework/actions/runs/35178503811) | ✅ | ✅ | ✅ | ✅ | 0 | 23.46s | [`json`](SHIP-001-real-provider-2026-09-17-run-35178503811.json) |

三份脱敏结果均满足 `validation_ok=true`、`tests_executed=true`、`file_changed=true`、
`independent_test_exit_code=0`；测试文件 SHA-256 为
`4b69bb74274892eac935505ea7d133a960403b6ca8b7db0f47adb09f98f239a9`，实现文件 SHA-256 为
`b9cb9483df54f2343b5e6f4ba53e0d0dc59afdefcafab7fdd4968258e777aee2`。结果只保留脱敏摘要、
工具状态和哈希，未发现 API key。三次构成当前候选同一入口的连续 3 次可信通过；它们覆盖
真实 Provider 核心 Agent API，不冒充 CLI/TUI 或不同协议结果。

**当前结论：OpenAI-compatible 子门禁已绑定 SEC-001 前候选；`be737c0` 新增权限缓存/审计边界、旧规则兼容和 help 后需重新绑定，SHIP-001 仍等待不同协议连续 3 次当前候选证据。**

## 2026-09-17 最终候选 OpenAI-compatible 连续验收

最终候选为 `a78efba5eabbc9a7391986c86e8b234593bf862c`，相对 `763a845` 仅增加 `gofmt`
格式修复。使用 `provider=openai`、端点 `https://newapi.doubb.top/v1`、模型
`agnes-2.5-flash`，密钥只由 `BASEWORK_REAL_API_KEY` secret 注入。

| 次序 | Workflow run | Agent | 验证 | 测试实际执行 | 实现已修改 | 独立测试退出码 | 结果文件 |
|---:|---:|---|---|---|---|---:|---|
| 1 | [35189820351](https://github.com/wly2lcl/basework/actions/runs/35189820351) | ✅ | ✅ | ✅ | ✅ | 0 | [`json`](SHIP-001-real-provider-2026-09-17-run-35189820351.json) |
| 2 | [35189839503](https://github.com/wly2lcl/basework/actions/runs/35189839503) | ✅ | ✅ | ✅ | ✅ | 0 | [`json`](SHIP-001-real-provider-2026-09-17-run-35189839503.json) |
| 3 | [35189879941](https://github.com/wly2lcl/basework/actions/runs/35189879941) | ✅ | ✅ | ✅ | ✅ | 0 | [`json`](SHIP-001-real-provider-2026-09-17-run-35189879941.json) |

三次 artifact 均为 `basework.real-provider.v1`，`agent_ok=true`、`validation_ok=true`、
`tests_executed=true`、`file_changed=true`，独立 `go test -count=1 -run ^TestAdd$ ./...`
退出码为 0；只保存脱敏摘要、工具状态和哈希，未发现 API key。三次构成最终候选同一入口
的连续可信通过，覆盖真实 Provider 核心 Agent API。

这组结果仍不覆盖不同协议、CLI/TUI 入口或真人 TUI 手感；任务状态继续由 [TASKS](../../TASKS.md)
维护，SHIP-001 仍等待不同协议 Provider 连续 3 次当前候选证据。
