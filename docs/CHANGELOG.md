# 更新日志 (Changelog)

格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

> **版本口径说明（重要）**
>
> 版本标签以 **Git tag** 为准，实际发布还需核对 Release 和产物。2026-09-14 本地核对已有
> `v0.1.0` 至 `v0.1.4`；下方 `0.3.0` / `0.4.0` / `0.5.0` 等条目
> 是开发期的内部编号，**尚未对应到任何 git tag**，其 release 链接暂不可达。
> 后续发布请以 tag 为准，并补齐两条链路：`git tag` ↔ 本文件条目 ↔
> [发布流程](release.md) 中的候选版本记录。

## [Unreleased]

### 计划完成度复审与文档校正（2026-09-14）

- 对 bb455ff 的 29 项计划进行代码/证据/入口审查，保留 15 项验收，重开 14 项；另列 QA-001 和两项后续优化。当前状态以 TASKS 为准。
- 保存 16 个可重复失败样例，覆盖撤销边界、服务关闭、日志残尾、事实摘要、产品接线和 TUI；详见 [复审报告](development/evidence/REVIEW-2026-09-14.md)。
- 统一 TUI 使用文档，纠正不存在的命令参数、会话恢复和发布范围声明。
- 本轮仅修改文档，下面发现的问题尚未修复；不能将本条理解为功能修复发布。

### 跨平台 CI 首轮失败修复（2026-09-14）

M2–M8 首次推送后 CI 矩阵（ubuntu / windows / macos）实际运行，Ubuntu 失败 1 个、Windows 失败 13 个测试。
按根因分两类修完：**2 个真实产品缺陷**（表现为 4 个 Windows 失败），另 10 个（9 个 Windows + 1 个 Ubuntu）
是测试自身的假设错误。

**修复（产品缺陷）**
- **Windows 上超时/取消对后台命令不生效**：`internal/jobs` 在 Windows 退化为「只终止直接子进程」，而
  `sh -c "sleep 30"` 的孙进程仍持有继承来的管道写端，`cmd.Wait()` 会一直阻塞到它自然退出——1 秒超时
  实测拖满 30 秒才进终态，即超时在 Windows 上等于没有。新增 `internal/jobs/process_windows.go`：
  `prepareCommand` 设 `CREATE_NEW_PROCESS_GROUP`（脱离调用方控制台，不被 Ctrl+C 连带），
  `terminateCommand` 先用 `taskkill /T /F` 按父进程链终止整棵树，再按 `ParentProcessId` 调用系统
  PowerShell 进程表补清理 taskkill 的竞态漏网进程；`taskkill` 退出码 128（进程已不存在）按 unix 的
  `ESRCH` 语义处理。Windows 侧**没有温和阶段**——控制台进程没有可捕获的终止信号，
  等一个发不出去的宽限期没有意义（签名用空标识符接收 `grace` 以显式表达这一点）。
  平台能力随之从 `ProcessGroupSupported()` 改名为 `ProcessTreeTerminationSupported()`：旧名字在
  Windows 上会变成谎话；`ErrProcessGroupUnsupported` 改名 `ErrProcessTreeUnsupported`，只保留给
  既无进程组也无等价手段的平台（`process_other.go` 的构建约束收窄为 `!unix && !windows`）。
- **`FactsSummaryProvider.wrapHash` 漏判 Windows 根路径**：判绝对路径只用 `filepath.IsAbs`，而它在
  Windows 上对 `/abs/path` 返回 false（只认盘符与 UNC），越界路径会被放行到哈希函数。改为
  `path.IsAbs || filepath.IsAbs` 取并集——前者认所有平台的前导 `/`，后者认 `C:\`。

**修复（测试自身的平台假设，不影响产品行为，但此前会让 CI 红灯）**
- 只设 `HOME` 做隔离，在 Windows 上不生效（`os.UserHomeDir` 读 `USERPROFILE`）：`cmd/basework` 的
  会话目录 / 会话迁移 / jobs 重启归并 / provider 端点 / sqlite 权限存储 五处统一走新增的
  `setTestHome`；`tests/cli_integration_test.go` 补设 `USERPROFILE`。
- 断言 0600 文件权限：Windows 不实现 Unix 权限位（`os.Stat` 一律返回 0666，私有性由目录 ACL 表达），
  `internal/edits`、`internal/jobs`、`internal/tools` 三处改为仅在类 Unix 平台断言，并在注释里写明
  该平台覆盖不到——不假装通过。
- `internal/jobs` 溢出文件句柄直到 `Close`/`CloseOwner` 才释放，未关会让 Windows 的 `t.TempDir`
  清理因「文件被占用」失败（Unix 允许删除打开中的文件）：补 `t.Cleanup(mgr.Close)`。
- `TestBackgroundBash_RespectsWorkingDirectory` 用字符串拼接构造工具参数 JSON，Windows 临时目录里的
  `C:\Users\...` 使 `\U` 成为非法 JSON 转义，参数解析直接失败；改用 `json.Marshal`，并把路径
  `ToSlash`（shell 里反斜杠是转义符，原生分隔符会被吃掉）。
- `TestCloseOwner_CancelsAndCleansOnlyThatOwner`（Ubuntu）在输出写入落地前就断言，Linux 调度下偶发
  `TotalSize=0`；改为先等写入完成信号。

**新增**
- `internal/jobs/process_windows_test.go`：Windows 专属平台测试。其中
  `TestTerminateCommand_KillsGrandchildren` 让 stdout 走 `io.Writer` 而非 `*os.File`，使 `os/exec`
  自建管道并被孙进程继承，从而**精确复现**上述「孙进程持管道 → `Wait` 阻塞」的缺陷形态。

**验证结果**
- 修复后重跑 CI（run `34797549128`）：macos 1m9s、ubuntu 1m14s、windows 3m47s、Quality 2m58s、
  Release Dry Run 3m47s **全部通过**——本仓库首次跨平台 CI 全绿。本地在 macOS 上双 tag 全量测试
  （各 34 个 `ok` 行、0 FAIL）、race 子集、`make gen` 幂等、`check-arch`/`check-docs`、看板 29/29 亦全部通过。
- 仍未获得：Docker/GHCR 镜像的实际构建（本机 Docker daemon 未运行，沙箱内无法启动 colima）。

### M8 发布准备：固定场景回归集、安装升级验证与迁移缺陷修复（2026-09-13）

**修复（TUI 与并发，候选版本验收中发现）**
- TUI 输入框按**字节**过滤按键，导致所有非 ASCII 字符（中文）与空格被静默丢弃、退格可能产出非法 UTF-8。插入、删除、光标移动全部改为 rune 语义，可打印字符改用按键携带的原始文本；`internal/tui/input.go` 与 `internal/tui/dialog/input.go` 两处副本同修。回归测试：`internal/tui/input_unicode_test.go`、`internal/tui/dialog/input_unicode_test.go`。
- 后台任务归属从未绑定：`agent.New` 返回的 `*AgentLoop` 未实现 `SessionIDProvider`（只有包装类型实现），产品层类型断言静默失败，`bash_background` 在所有模式下必然报「当前会话为空」。补 `(*AgentLoop).SessionID()`（加法式）。回归测试：`pkg/agent/sessionid_provider_test.go`、`cmd/basework/bind_owner_realagent_test.go`。
- `pkg/lsp` 的 `Conn.Start()` 为 check-then-act，并发调用会启动两个 `readLoop` 共享同一 `bufio.Reader`（数据竞争 + 协议解析撕裂）；改为原子 CAS。
- `internal/observability` 的 pprof `Start()` 在后台 goroutine 中 `ListenAndServe`，**端口被占用时静默打印日志却返回 nil**（调用方以为已启动）；且测试绑定固定端口 16060–16066，多个 `go test` 进程并行时会互踩 FAIL。现改为同步 `net.Listen`（绑定失败立即报错）、支持端口 0（内核随机分配、`Addr()` 回填实际端口），测试全部改用随机端口；回归测试覆盖端口冲突报错与多实例共存。

**修复（会话迁移相关，均会让用户丢数据或误判数据不存在）**
- `migrate sessions`：会话 ID 短于 12 字符时 `slice bounds out of range` 直接 panic；迁移后新会话用的是随机 ID 而事件仍带原 `session_id`，导致外键失配、事件被静默跳过（迁移"成功"但数据没进来）。现在按原会话 ID 建会话。
- `migrate sessions`：原先自行逐行解析 JSONL，**绕过**了存储层的版本检查与迁移链，使格式版本高于当前支持版本（如 v99）的数据也能"成功"导入。现改为经 `session.Store.Events` 走真实读路径，并在存在失败项时返回非零退出码。
- `session list`：把 `ErrSchemaTooNew` 与其他读取错误一并吞掉，对未来版本的会话文件谎报"没有找到会话"。现明确报错并非零退出；真正的损坏文件仍按原语义跳过。

**新增**
- `pkg/session`：`CreateOpts` 增加可选字段 `ID`（加法式，不传时行为与之前一致），统一校验 `[a-zA-Z0-9_-]` 并拒绝重复。
- `tests/coding_scenarios_test.go`：五个固定编码场景的**离线确定性**回归集（小函数修复、多文件修改、大输出、权限拒绝、断流恢复），外加半截参数变体；断言落在磁盘产物与工具轨迹上，不依赖模型文案与网络。
- `examples/embed`：把 basework 作为库嵌入 Go 服务的最小示例（`provider.Create` + `agent.New` + `runtime.NewLocal`，含工具执行、事件订阅与会话落盘断言）。
- `tests/tui_pty/`：终端级 TUI 验收夹具（PTY 驱动 + 脚本化 OpenAI SSE 服务 + VT 抓屏），覆盖路线图 5 条代表性场景；验收工具而非 `go test`，运行方法见 [tui-pty-acceptance](development/tui-pty-acceptance.md)。
- 回归测试：`pkg/session/create_id_test.go`、`pkg/session/create_id_sqlite_test.go`、`cmd/basework/migrate_sessions_test.go`。

**文档与仓库**
- `.gitignore` 忽略 GoReleaser 打包产物 `dist/`，避免本地 dry-run 后被误提交。
- 证据记录：[SHIP-001](development/evidence/SHIP-001.md)、[SHIP-002](development/evidence/SHIP-002.md)、[SHIP-003](development/evidence/SHIP-003.md)。

### 文档集中与任务进度（2026-09-12）

- 文档正文统一到 `docs/`，根 README 仅导航；架构、贡献、安全、行为准则和变更日志已迁移。
- 包说明集中到 `docs/reference/pkg/`，补充 Model Experience；旧 Phase、设计分册和过时统计整体归档。
- 新路线图按用户场景拆为 M0–M8；任务看板配套实施卡、依赖、验收、证据模板与 AI 交接流程。
- `make progress` 从任务看板统计；文档检查新增布局、依赖循环、任务锚点与完成证据校验。
- 校正强制请求审计和免费模型免密钥的过度表述；以下旧条目中的文档路径反映当时布局，当前以文档中心为准。

### 架构边界清理 + 主路径缺陷修复（目标 v0.2.0，破坏性）

**Breaking**
- `pkg/tool/builtin` 不再依赖 `basework/internal/*`：移除对
  `internal/observability` 的类型依赖，改为在 `pkg` 内定义最小接口
  （`PathChecker`、`EventPublisher`）。使用方从直接传
  `*observability.EventBus` 改为经由 `builtin.SetEventBus(EventPublisher)`
  注入 —— 结构体实现该接口，故现有调用点无需改动类型，语义等价。
- 该改动是 `pkg/` 层实现「可独立发布为 Go module」的前置条件。

**Fixed**
- TUI 流式回调从未接线：`cmd/basework tui` 构造的 agent option 中
  `Callback` 恒为 nil，导致流式文本、thinking、工具开始/结束事件全部丢失。
  现改为「构造回调 → 拿到 `tea.Program` 后再绑定 `Send`」的 post-binding
  模式，避免回调在 `tea.Cmd` goroutine 中直接修改 `App` 造成数据竞争。
- 会话目录三处不一致：`session status`、`session unlock`、`migrate` 曾分别
  使用 `~/.local/share/basework/sessions` 与 `~/.basework/sessions`，读不到同一份
  数据。现统一为——**写入固定规范目录** `~/.local/share/basework/sessions`，
  **所有读取路径**（`session list/status/unlock`、`migrate`）走
  `resolveSessionDir()`：优先规范目录，仅当规范目录不存在且旧目录存在时回退到
  `~/.basework/sessions`。
- `session` 子命令在会话 ID 短于 12 字符时切片越界 panic，新增
  `shortSessionID` 统一截断。
- `pkg/session` 的 JSONL 读取使用 `bufio.Scanner` 默认 64KB 上限，单条携带长工具
  输出的事件会直接报 `token too long`。上限提升到 16MB。

**Added**
- 架构防护测试 `TestArch*`（`tests/arch_test.go`）：用 `go/parser` 静态扫描
  `pkg/` 与 `internal/`，覆盖三条规则——`pkg/` 不得依赖 `internal/`/`cmd/`、
  `internal/` 不得依赖 `cmd/`、`pkg/` 内的 `Set*` 注入点必须登记在白名单。
  已接入 `make check-arch` 与 CI `quality` job。
- 代码统计生成器 `scripts/docstats`（纯标准库）+ `make stats`，输出
  [docs/STATS.md](STATS.md)，杜绝文档手写数字漂移。

**Docs**
- `ARCHITECTURE.md`：补充 §分层硬约束（`pkg/` 不得依赖 `internal/` + 守护测试 +
  接口注入白名单表）；修正 `internal/` 结构图中并不存在的 `cache/`；兼容性表补充
  「形参引用 `internal/` 类型的导出符号不计入承诺」。
- `docs/design/07-terminal-product.md` §26.10：修正 TUI 通信模型 —— 原文档描述的
  是「回调直接调 `app.AddToolCall` / `UpdateStreamingText`」这一已被移除的竞态
  写法，改为「回调只投递 `tea.Msg`、状态变更在事件循环内」，并补充
  `callback.go`、4 个流式 Msg 与 post-binding 接线说明。
- 会话目录文档对齐：`cli-guide.md`、`tui-guide.md`、`installation.md`、
  `migration.md` 中 `~/.config/basework/sessions/` 与 `~/.basework/sessions/`
  改为规范目录 + 旧目录回退说明。
- 合并互斥的两份 ROADMAP 为根目录单一份；删除 `docs/ROADMAP.md` 与 `openspec/`。
- 拆分 3137 行 `docs/DESIGN.md` 为 `docs/design/01-07`（除索引登记的修订外逐字保留）。
- `docs/STATUS.md` 收敛为「仅描述当前状态」，移除与代码矛盾的 Phase 26-30 规划段。

### 借鉴 deepseek-harness 的治理改造（未发布）

对照 [docs/analysis/deepseek-harness-vs-basework.md](analysis/deepseek-harness-vs-basework.md)
的结论，补四类此前确实缺失的能力：会话数据安全、架构与文档制度、请求可审计性、
包级契约文档。方案、取舍与范围见
[docs/plans/2026-09-12-dsh-borrow.md](archive/2026-09-12-dsh-borrow.md)。

**Added — 会话格式版本化与相邻迁移链（数据安全）**
- `Event` 新增 `SchemaVersion` 字段（JSON 键 `v`，`omitempty`）。零值即 v0，指引入该
  字段之前的历史数据；因此旧文件的字节不变、可无损读出。
- JSONL 文件新增首行文件头 `{"_schema":"basework.session","v":N}`（`N` 为当前
  `SchemaVersion`，随版本递增，当前为 2）。读取端只看首行：
  含 `_schema` 即为文件头，否则视为 v0 老文件（首行即事件）。判定是 O(1) 的，
  读路径（每个会话每次读都经过）不做额外全文件扫描。
- SQLite 使用原生 `PRAGMA user_version` 承载版本，不新建 meta 表——少一张表就少一处
  可能与真实状态不一致的地方。
- 新增 `pkg/session/migrate.go`：**只允许相邻步进**的迁移链（v0→v1→v2…），禁止跨版本
  直达。理由是 N 个相邻步骤只需 N 个测试，覆盖任意版本组合需要 N² 个。
- 读到更高版本时返回 `ErrSchemaTooNew`，**读取与写入路径都必须失败**，不得尽力解析或
  降级写回旧格式：同一份日志在不同投影规则下会产出不同历史，静默继续等于制造难以
  察觉的错误上下文。
- v0→v1 迁移本身只是纯版本戳（事件结构未变）。建立机制的成本在「还没有任何不兼容
  变更」时最低，因此先立起来。

**Added — 压缩锚点稳定化 + 摘要回填（修复两个此前未被记录的缺陷）**
- **锚点**：`EventCompacted` 原先只写 `KeepFrom`（消息**下标**），投影按
  `msgs[KeepFrom:]` 截断。一旦投影规则变化（例如新增事件类型改变了消息条数），同一份
  日志在不同版本下会投影出**不同历史**且无法察觉；`CompactedData.TruncatedSeq` 字段虽
  已存在却从未被赋值。现改为写入「被丢弃的最后一条消息所来源的事件 Seq」，投影优先按
  Seq 定位，`KeepFrom` 保留为 v0 数据与退化数据的回退路径。
- **摘要**：压缩产生的摘要此前被丢弃两次——`Compactor.Compact()` 的返回值在 `loop.go`
  中只用于比较长度，而 `CompactedData.Summary` 又留空。由于请求是由事件日志投影重建
  的，摘要在内存里等于没生成，「压缩」实际退化为「静默丢消息」。现经新增的
  `CompactReporter` 回传、随事件落盘，并由投影作为消息插回列表头部。
- 摘要角色选择：不插为 `system`——`pipeline.setupTurn` 会用当前配置的 prompt
  **替换**首条 system 消息，摘要会被静默吃掉；也不插为 `assistant`——那会让模型误以为
  该摘要出自它自己。最终用 `user`，与 `internal/compaction` 既有约定一致。
- 摘要消息的来源 Seq 取**截断边界**（`TruncatedSeq`）而非压缩事件自身的 Seq：否则
  `seqs` 会变成非单调序列，下一次压缩定位锚点时会错误命中这条摘要，于是把本该丢弃的
  整段历史又保留下来、并出现两份摘要叠加。

**Added — 依赖图生成器 + 文档校验器（把制度交给机器）**
- `scripts/gendeps` + `make deps` → [docs/DEPGRAPH.md](DEPGRAPH.md)：层级依赖图、
  包级依赖图（mermaid）、依赖边清单、扇入扇出排行、层级违规检测，以及 `pkg/` 层第三方
  依赖明细。**构建约束感知**：区分「默认构建即引入」与「仅在 build tag 下引入」，
  避免用统一的 `go build` 判断造成误报。
- `scripts/doccheck` + `make check-docs`：校验 ADR 的文件名格式 / 三节齐备 / `Status`
  合法，以及全仓 markdown 相对链接的有效性。
- 两个生成器都**不输出时间戳**且全部字典序排序。任何随时间变化的字段或顺序抖动都会让
  CI 的 `git diff --exit-code` 永久失败，门禁随即形同虚设。
- CI `quality` job 新增三个步骤：架构边界（`TestArch*`）、文档校验、**新鲜度门禁**
  （`make gen` 后执行 `git diff --exit-code docs/DEPGRAPH.md docs/STATS.md`）。
- `make gen` 一键刷新全部生成物。
- `make deps` 在发现边界违规时以**非零退出**：若只把 ❌ 写进文档，那么「把带违规的
  文档提交进仓库」就能让新鲜度门禁通过。

**Added — 轻量 ADR（决策留痕）**
- 新增 [docs/adr/](adr/TEMPLATE.md)：命名 `NNNN-kebab-title.md`，每篇**只允许
  「背景 / 决定 / 后果」三节**，外加一个 `Status` 字段。不做审批链、不做状态机——
  `openspec/` 正是因流程过重而被删除。
- 回填 3 条真实决策：0001（用 ADR 替代 openspec）、0002（`pkg` 用本地接口解耦
  `internal`，含被否决的两个替代方案及其否决理由）、0003（文档数字与结构必须机器生成
  且单一权威）。

**Added — 请求可审计 + 请求可由日志重建（"模型可见即已记录"）**
- **问题**：事件日志是历史的来源，但**请求**并不完全来自日志。`pkg/agent/pipeline.go`
  里有两条绕过日志的路径——system prompt 直接取自配置、steering 消息在 `Drain()` 之后
  即被丢弃。后果是「模型看到的」与「日志记的」可以不一致，且事后无法解释模型为什么
  那样回答，全程没有任何报错。
- 新增 `session.EventSystemPromptSet`（记录 system prompt 内容与哈希）与
  `session.EventSteered`（记录一次 steering 注入）。两者**不投影为消息**：它们是
  「请求级上下文」，由 `agent.BuildRequestMessages` 在组装请求时放到消息列表最前。
  不放进投影结果是因为会话级上下文的 Seq 必然大于其后所有历史，插到列表头部会让
  消息来源 Seq 变成**非单调**（压缩锚点定位依赖单调性）；而 system 消息必须在最前、
  压缩又只截断尾部，两者叠加会让压缩把 system prompt 一并丢掉。
- `agent.BuildRequestMessages(events)` 成为请求的唯一组装入口：`setupTurn` 不再读
  `TurnD.SystemPrompt()`、不再使用内存态 steering 队列。请求因此是**事件日志的纯函数**。
- 新增 `agent.RequestFingerprint`（messages + tools 的稳定哈希）与
  `session.EventRequestBuilt`：每次请求尝试在日志里留一条指纹
  （条数 + 哈希 + 来源描述），可事后核对。写入失败只记日志、不中断请求——审计能力
  缺失不该让对话本身失败。
- 新增 `agent.CheckRequestInvariant(events, actual)`：校验「实际请求 == 日志重建的请求」，
  差异会指出首个不一致的消息位置与原因。装了会改写请求的 hook 时该校验必然失败，
  这是设计边界（hook 是调用方代码，不在日志管辖内）。
- 新增测试 `pkg/agent/request_invariant_test.go`：逐轮校验多轮对话中该等式恒成立，
  并按 `request.built` 的 Seq 把日志截回「请求那一刻」再比对。
- **`SchemaVersion` 升到 2**。v1→v2 是**纯版本戳**（不改变投影结果，也不改写任何数据）；
  升版本的理由是**能力门槛**：旧版本程序会忽略这两类事件、改用配置里的 system prompt，
  从而构造出与写入方意图不同的请求且不报错。

**Added — `pkg/` 包 README 契约**
- `pkg/` 下 12 个 Go 包此前 **0 个 README**。使用者只能读源码猜「这个包该配什么、
  从哪扩展、有什么坑」，而这三件事恰是源码里最难读出来的部分。
- 每个包补齐 README，统一四节：**用途 / 配置 / 扩展点 / Known Limitations**。
- 契约并入 `scripts/doccheck`（`make check-docs` 与 CI 一并生效）：缺 README 或四节不全
  即非零退出，并带假阴性防护（扫到 0 个包时判定"检查未生效"而非通过）。
- 「是 Go 包」由「目录下存在非 `_test.go` 的 `.go` 文件」判定，因此 `pkg` 根目录
  （只有集成测试）与 `pkg/agent/templates` 会自动排除，不需要维护手写清单。

**Changed — 兼容性口径修正**
- 方案原预计需要破坏 `agent.Compactor` 与 `compaction.Strategy` 两处签名，实际结果：
  - `agent.Compactor` **未破坏**。改为新增可选扩展接口 `agent.CompactReporter`
    （方法 `CompactWithReport`）。Go 接口结构化满足，已实现 `Compactor` 的嵌入方无需
    改动，只是压缩事件里不带摘要（与历史行为一致）。
  - `compaction.Strategy` 的 `Compact` 改为返回 `Result`（含 `Messages` / `Summary`），
    属破坏性改动；但 `internal/compaction` 位于 `internal/`，按本项目的兼容性策略
    **无对外承诺**，因此不构成对使用者的破坏。
- 结论：本轮**没有**引入需要 v0.2.0 承接的对外破坏性变更。

**Fixed（本轮发现并修正的文档失实）**
- `ARCHITECTURE.md` 先后声称 `pkg/` 层「不依赖任何第三方库」与「唯一例外是
  `golang.org/x/image`」，**两次都与代码不符**：实际有 4 个（`golang.org/x/image`、
  `golang.org/x/oauth2`、`modernc.org/sqlite`、`github.com/golang/snappy`）。现已改为
  与 `docs/DEPGRAPH.md` 一致的清单，并把「两个依赖未做 build tag 隔离」记为已知偏差。
  这是依赖图生成器上线后立刻暴露出的第一条结构性失实。

### Phase 34: 深度修复 — 协议/数据/可靠性 (2026-07-07)

**Critical 安全与协议修复**
- OAuth CSRF 防护：生成随机 state 参数，回调时验证匹配性（10 分钟过期、一次性使用）
- MCP StdioTransport 重连：`Reset()` 杀死旧进程 + 重启，重连后重新发现工具/资源/提示
- Stream 错误传播：pipeline 检查 `StreamEvent.Error`，流式错误不再静默丢弃
- 上下文取消检测：stream 循环后检查 `ctx.Err()`，返回取消错误

**Critical 数据完整性修复**
- FTS5 真全文搜索：替换 LIKE 为 FTS5 虚拟表 + BM25 相关性排序，CJK 回退 LIKE
- Memory ID 统一：FTS 索引与存储层使用相同 ID 格式
- LSP 生命周期：nil 指针防护（8 个方法）、进程泄漏清理（Kill）、readLoop panic 恢复
- 配置验证框架：`Validate()` 检查温度/token/端口/TopP 等范围，`Load`/`Reload` 后自动调用

**High Provider 错误处理**
- Gemini/Bedrock UTF-8：`strings.TrimPrefix` 替代字节切片，CJK 输出不再乱码
- OpenAI tool call：基于 `finish_reason` 判断完成，替代脆弱的空字符串检测
- HTTP 重试：429/5xx 指数退避重试（3 次），尊重 `Retry-After` header
- Copilot token 自动刷新：过期前自动通过 TokenSource 刷新

**High MCP 能力协商**
- 解析 initialize 响应中的 capabilities 字段
- 仅对声明支持的能力调用 tools/list、resources/list、prompts/list

**Medium 代码质量改进**
- todowrite 会话隔离：全局 map 改为实例注入 sessionID
- TUI 退出清理：退出时清理插件、取消 context
- JSONLStore Events 缓存优先：从缓存读取而非每次读盘
- OAuth expires_in=0 处理：使用默认 1 小时过期，防止无限刷新循环
- MCP HTTPTransport 超时：5 分钟超时防止永久挂起
- LSP fileToURI 编码：使用 `url.URL.String()` 正确 percent-encode 路径

**测试覆盖**
- 新增 OAuth CSRF 测试（5 个用例）
- 新增 MCP 重连/能力协商测试（12+ 个用例）
- 新增 FTS5 搜索测试（12 个用例）
- 新增 LSP 生命周期测试（14 个用例）
- 新增配置验证测试（33 个用例）
- 新增 HTTP 重试测试（8 个用例）

### Phase 33: 关键安全修复 + 架构解耦 (2026-07-07)

**P0 安全修复**
- `apply_patch` 路径遍历防护：净化路径防止 `../../` 穿越，集成敏感路径检查（`.git/`、`.ssh/`、`.aws/` 等）
- `web_fetch` SSRF 防护：协议白名单（仅 http/https）、DNS 解析后 IP 检查（拦截内网/回环/链路本地地址）、重定向拦截
- `JSONLStore` 并发安全：分离 cacheMu 保护 cache map，修复读锁下写 map 的数据竞态
- `EventBus` 非阻塞发布：信号量满时使用 select+default 丢弃事件，避免发布者永久阻塞

**P1 功能修复**
- `HandleMessage` 错误处理：关闭后返回 `ErrAgentClosed` 替代 `nil, nil`
- 上下文压缩激活：压缩结果写入 `EventCompacted` 事件，`ProjectMessages` 根据 `KeepFrom` 截断旧消息
- 转向系统集成：`SteeringManager` 连接到 agent loop，转向消息注入到 system prompt 之后

**P1 架构重构**
- `pkg/agent` 解耦 `internal/`：定义 `Compactor`、`LoopDetector`、`PermissionChecker`、`EventPublisher`、`SubAgentRunner` 接口
- `pkg/llm` 解耦 `internal/`：定义 `EventBus`、`TokenSource` 接口
- 新增 4 个适配器（`internal/loopdetect/adapter.go`、`permission/adapter.go`、`observability/adapter.go`、`subagent/adapter.go`）

**测试覆盖**
- 新增 apply_patch 安全测试（7 个用例）
- 新增 web_fetch SSRF 测试（协议/IP/重定向拦截）
- 新增 JSONLStore 并发压力测试（10 goroutine × 100 轮）
- 新增 EventBus 非阻塞测试
- 新增转向系统集成测试

### 即将推出

- **工作流引擎** — 多步骤任务编排与 DAG 执行
- **评估框架** — LLM 输出质量评估与回归测试
- **远程 Agent** — 分布式 Agent 通信与协作
- **多语言支持** — Agent 回复语言自适应切换

## [0.5.0] - 2026-07-06

### Phase 29-30: TUI 增强 + 模板系统 + 多模态 + 插件生态

**TUI 增强**
- 主题系统：亮/暗主题切换、自定义主题、终端自适应
- 斜杠命令面板：/触发、fuzzy 搜索、命令补全
- 键盘绑定：三层绑定模型、可配置快捷键
- 对话框系统：管理器、堆栈、模态/非模态

**模板系统**
- Provider 感知系统提示模板
- 内置 anthropic/openai/gemini/default 模板
- 用户自定义模板 + 热重载
- 环境动态注入（工作目录、Git、平台信息）

**多模态 + 插件**
- 图片输入支持（JPEG/PNG/WebP）
- Hook 系统扩展（PreStep/PostStep/OnToolError/OnCompaction）
- Provider 插件化
- TUI 插件插槽

### 新配置项

- `theme.name` — TUI 主题选择（dark/light/dracula/monokai）
- `theme.custom_path` — 自定义主题目录
- `keybindings.path` — 键盘绑定配置文件路径
- `templates.custom_dir` — 用户自定义模板目录
- `templates.default_provider` — 默认模板 Provider

### 文档

- `docs/guides/theme.md` — 主题配置指南
- `docs/guides/templates.md` — 模板系统指南

[0.5.0]: https://github.com/wly2lcl/basework/releases/tag/v0.5.0

## [0.4.0] - 2026-07-06

### 安全加固 + 性能基线（Phase 28）

#### 权限持久化

- **权限规则 SQLite 持久化** (`internal/permission/persist.go`) — 跨会话保留权限规则：
  - 规则存储到 SQLite 数据库，重启后不丢失
  - 支持 `always` 授权持久化
  - 配置项: `security.permission_store`（`sqlite` / `memory`）
- **权限审计日志** (`internal/permission/audit.go`) — 记录所有权限决策：
  - 记录时间、工具名、参数、决策结果
  - 配置保留天数（默认 30 天）
  - `basework permission audit` 查询命令
- **权限迁移工具** — `basework permission export/import` 命令：
  - 导出当前权限规则为 JSON
  - 从 JSON 文件导入权限规则

#### 敏感路径保护

- **路径检查** (`internal/permission/paths.go`) — 工具执行前路径安全检查：
  - 默认保护 `.git/`、`~/.ssh/`、`~/.aws/`、`~/.gnupg/` 等敏感路径
  - 支持白名单/黑名单配置
  - 三种保护级别：`strict`（禁止）/ `warn`（记录）/ `off`（关闭）
- **Agent 集成** (`internal/permission/hook.go`) — 通过 Hook 在工具执行前拦截：
  - 与现有权限系统无缝集成
  - 黑名单匹配时阻止执行
  - 白名单覆盖黑名单

#### 工具执行超时

- **超时控制** (`internal/permission/timeout.go`) — 工具执行超时管理：
  - 默认 30s 超时，bash 工具 60s，LSP 工具 10s
  - 可配置覆盖：`tools.timeout.default`、`tools.timeout.overrides`
  - 超时后优雅终止，发送超时事件通知
- **配置示例**:
  ```yaml
  tools:
    timeout:
      default: 30
      overrides:
        bash: 60
        read: 10
  ```

#### 性能分析

- **pprof 集成** (`internal/observability/pprof.go`) — 性能分析支持：
  - HTTP pprof 端点（默认 `127.0.0.1:6060`）
  - CLI 命令：`basework profile cpu`、`basework profile memory`、`basework profile goroutine`
  - goroutine 泄漏检测
  - 配置项：`profiling.enabled`、`profiling.host`、`profiling.port`

#### Benchmark 套件

- **性能基准** (`tests/benchmark/`) — 77 项基准测试覆盖：
  - token 计数性能基准
  - 流式响应延迟测试
  - 工具执行性能测试
  - 会话读写性能测试

### 文档

- `docs/guides/security.md` — 安全配置指南（权限持久化、敏感路径保护、审计日志）
- `docs/guides/profiling.md` — 性能分析指南（pprof、benchmark 套件）

[0.4.0]: https://github.com/wly2lcl/basework/releases/tag/v0.4.0

## [0.3.0] - 2026-07-06

### 新增

#### Phase 26: 会话稳定性加固

- **SQLite WAL 模式** (`pkg/session/sqlite.go`) — Write-Ahead Logging 提升并发读写性能 2-3x：
  - 配置项: `database.mode`（`wal` 或 `delete`，默认 `wal`）
  - 支持并发读取和写入（读不阻塞写）
- **文件锁机制** (`pkg/session/lock_unix.go`, `pkg/session/lock_windows.go`) — 操作系统级跨进程锁：
  - Unix: `syscall.Flock`，Windows: `LockFileEx`
  - 非阻塞模式 + 超时机制（默认 5 秒）
  - `basework session unlock <id>` 强制解锁命令
- **会话恢复** (`pkg/session/recovery.go`) — 损坏检测与自动恢复：
  - `PRAGMA integrity_check` 完整性检查
  - 从 WAL 文件自动恢复
  - `basework session status --check-integrity` 批量检查
- **长会话压缩** (`pkg/session/compress.go`) — Snappy/Gzip 压缩存储：
  - 超过 1000 条消息自动触发
  - 配置项: `session.compression.enabled`
  - 透明解压，对上层 API 无感
- **会话状态监控** — `basework session status` 命令：
  - 显示会话 ID、标题、消息数、创建/更新时间
  - 支持 `--check-integrity` 完整性检查

#### Phase 27: CI/CD + 发布流程

- **GitHub Actions CI/CD** (`.github/workflows/ci.yml`) — 完整自动化流程：
  - PR 触发：lint + 测试 + 覆盖率
  - Tag 触发：跨平台构建 + GitHub Release + Homebrew + Docker
- **goreleaser 跨平台构建** (`.goreleaser.yml`) — 自动化分发：
  - Linux (amd64/arm64)、macOS (amd64/arm64)、Windows (amd64)
  - 自动生成 checksum、changelog
  - Homebrew Formula 自动更新
- **Docker 镜像** (`Dockerfile`) — 容器化部署：
  - 基于 `gcr.io/distroless/static-debian11`（< 30MB）
  - 发布到 `ghcr.io/wly2lcl/basework`
  - 标签: `latest`、版本号、主版本号
- **版本信息** (`cmd/basework/version.go`) — `basework version` 命令：
  - 显示版本号、Git commit、构建时间、Go 版本、平台

### 文档

- `docs/installation.md` — 完整安装指南（Homebrew/Docker/go install/二进制）
- `docs/docker.md` — Docker 使用指南

## [0.2.0] - 2026-07-04

### 新增

#### Phase 23: Prompt 缓存 + 命令黑名单

- **Prompt 缓存** (`pkg/provider/cache.go`) — Anthropic/OpenAI/Gemini 自动注入 `cache_control` 标记：
  - Anthropic: system 消息转为带 `cache_control` 的对象数组，第一条 user 消息前 2 个 text block 标记
  - OpenAI: 第一条 user 消息前 2 个 text parts 添加 `cache_control` 标记
  - Gemini: 前 2 个 user contents 添加缓存标记
  - 配置项: `prompt_cache.enabled`（默认 true）
- **命令黑名单** (`pkg/tool/builtin/blacklist.go`) — 12+ 内置危险命令模式：
  - 支持 `rm -rf /`、`mkfs`、`dd if=/dev/`、fork 炸弹、管道下载执行等
  - 用户自定义扩展（`config.yaml` 的 `blocked_commands`）
  - 权限集成：default（拒绝）/ interactive（确认）/ yolo（跳过）

#### Phase 24: MCP 增强

- **MCP 资源支持** (`pkg/mcp/resource.go`) — 实现 `resources/list`、`resources/read` 协议
  - 暴露为 `mcp_read` 工具，支持资源 URI 自动路由
  - 大小限制（默认 10MB），超大资源自动截断标记
- **MCP 提示支持** (`pkg/mcp/prompt.go`) — 实现 `prompts/list`、`prompts/get` 协议
  - 暴露为 `mcp_prompt` 工具，支持 prompt 名称自动查找
  - 大小限制与截断保护
- **MCP 自动重连** (`pkg/mcp/reconnect.go`) — 指数退避重连（1s, 2s, 4s, 8s...）
  - 状态机：available → reconnecting → available / unavailable
  - 最大重试次数可配置（默认 3 次）
  - 隔离性：一个服务器 unavailable 不影响其他
- **MCP 变量展开** (`pkg/mcp/config_expand.go`) — Shell 变量展开支持
  - 支持 `$VAR` 和 `${VAR}` 两种语法
  - 展开字段：`command`、`args`、`env`
  - 启动时一次性展开，运行时零开销
- **集成测试** (`tests/`) — 5 个新集成测试文件（28 个测试用例）：
  - `prompt_cache_integration_test.go` — 缓存标记 + 命中统计（6 个测试）
  - `command_blacklist_integration_test.go` — 黑名单拦截 + 权限绕过（8 个测试）
  - `mcp_resources_integration_test.go` — 资源读取 + 大小限制（6 个测试）
  - `mcp_prompts_integration_test.go` — 提示获取 + Context 取消（7 个测试）
  - `mcp_resilience_integration_test.go` — 自动重连 + 变量展开（10 个测试）

[0.2.0]: https://github.com/wly2lcl/basework/releases/tag/v0.2.0

## [0.1.0] - 2025-07-03

### 新增

#### Phase 1-3: 基础框架 (6a2aa08)

- **统一类型系统** (`pkg/llm/`) — LLM 请求/响应的类型定义与错误类型
- **工具接口** (`pkg/tool/`) — 工具接口定义 + 8 个内置工具实现
- **事件总线** (`pkg/hook/`) — PubSub 模式的事件发布订阅系统

#### Phase 4-5: Session + Agent (26fbe0a)

- **会话管理** (`pkg/session/`) — 事件溯源架构，JSONL 持久化存储
- **Agent 循环** (`pkg/agent/`) — 流式处理、工具调用循环、消息路由
- **Hook 系统增强** (`pkg/hook/`) — 完整的 PubSub 事件总线

#### Phase 6: Provider 工厂 (0fe5802)

- **Provider 工厂模式** (`pkg/provider/`) — 统一接口 + 工厂注册机制
- **10 个 LLM Provider** — OpenAI、Anthropic、Gemini 原生支持 + 7 个 OpenAI 兼容 Provider
- **错误类型** (`pkg/llm/error.go`) — Provider 错误分类与处理

#### Phase 7: LSP 集成 (7651dcc)

- **LSP 客户端** (`pkg/lsp/`) — 语言服务器协议客户端实现
- **自动发现** — 按项目类型自动发现并启动 LSP 服务
- **6 个 LSP 工具** — 代码补全、诊断、跳转定义、查找引用、悬停信息、文档符号
- **JSON-RPC over stdio** — 自实现的 JSON-RPC 通信层

#### Phase 8: MCP 集成 (3f51760)

- **MCP 客户端** (`pkg/mcp/`) — Model Context Protocol 客户端实现
- **双传输模式** — stdio 和 HTTP 传输支持
- **工具注入** — 将 MCP 服务端工具动态注入 Agent 工具链
- **自实现 JSON-RPC 层** — 不依赖外部 JSON-RPC 库

#### Phase 9-10: Memory + Config + Skill (2a7bbf5)

- **四层文件映射** (`pkg/memory/`) — ephemeral/short-term/long-term/file 四层记忆存储
- **FTS5 全文检索** — 基于 SQLite FTS5 的语义检索（build tag: memory）
- **JSON 配置** (`pkg/config/`) — 支持热重载、Copy-on-Write 模式的配置管理
- **技能加载** (`pkg/skill/`) — 技能文件自动发现与同名去重

#### Phase 11-12: CLI + 集成测试 + 文档 (e604e70)

- **CLI 参考实现** (`cmd/basework/`) — 基于 Cobra v1.10.2，支持以下子命令：
  - `start` — 启动 Agent REPL
  - `agent` — Agent 管理
  - `model` — 模型配置
  - `session` — 会话管理
  - `init` — 初始化项目
- **11 个集成测试** (`tests/integration_test.go`) — 覆盖核心工作流
- **3 份开发者指南** (`docs/guides/`) — Embedder 指南、配置指南、扩展指南
- **新增依赖**: cobra v1.10.2, modernc.org/sqlite

[0.1.0]: https://github.com/wly2lcl/basework/releases/tag/v0.1.0
