# 当前实现状态

核对日期：2026-09-17；产品可执行代码候选仍为 `a78efba`（包含只读预设空权限模式修复、SEC-001 作用域过滤、在途检查上下文快照、精确参数缓存匹配、审计归属、旧自动规则兼容、SQLite 参数缓存回归、作用域 ID help，以及 `gofmt` 修复）。真实 Provider 验收 runner 随 `71b4bba` 修复 Windows 进程树收尾，并在 `09fd055` 改为显式安全环境白名单、临时 HOME/TMP；远端 `main` 当前为 runner 修复后的提交。之前三次真实 Provider 结果仍绑定 `a78efba`，需要在 runner 修复后重新绑定。历史发布候选 `2a86886` 及更早记录继续保留。任务状态只在 [TASKS](TASKS.md) 维护。

**结论：核心功能、常规测试、最终候选 CI 和 OpenAI-compatible 真实 Provider 连续 3 次均已通过；不同协议 Provider 的当前候选连续 3 次证据仍缺失，因此 SHIP-001/QA-001/SHIP-002/SHIP-003 仍不能整体标记完成。** 本次确认并修复运行关闭竞态、真实模型验收假阳性、秘密进入结果和独立验证超时/CI 覆盖缺口，并完成 JSONL 长会话追加优化。详见 [2026-09-16 复审报告](development/evidence/REVIEW-2026-09-16.md)；历史修复见 [上次复审](development/evidence/REVIEW-2026-09-14.md)。真人主观 TUI 手感仍单独记录，不是自动化回验结论。

## 当前能力与缺口

| 能力 | 已有实现与证据 | 当前实际边界 |
|---|---|---|
| Agent 与模型协议 | Agent 循环、流式工具参数归一化、能力三态、缺 key 说明；REL-001/002 | Provider 静态声明不等于每个真实端点都已验证 |
| 请求审计与精确投影 | 完整 ChatMessage 比较、按请求 Seq 重建、schema v2、完整压缩快照、旧无快照兼容；REL-004 与包级测试 | compatible 允许审计写入失败后继续；strict 在 Provider 前中断。Hook 与传输改写仍超出重建边界 |
| 主运行入口行为配置 | 压缩、循环检测、观测、子代理和动态事实摘要均通过 runtime options 接入 | 真实外部 Provider 的兼容性仍按 REL-003/SHIP-001 单独验证 |
| 后台命令 | owner、状态机、输出配额/溢出、读取、取消、进程树终止、JSONL 历史、尾部修复、CLI/TUI 接线；重启归并 interrupted | 输出文件清理和跨平台 shell 能力仍受目标系统约束 |
| 编辑预览与提交 | 工作区/软链/权限检查、内容基线、逐文件写入、部分成功清单；撤销重新检查路径与权限 | 跨文件不是事务；撤销遇到用户二次编辑会按文件跳过 |
| 编辑产品闭环 | edit_files 预览/提交/撤销、真实 plan 路径/diff 审批、file.edited 事件、edits list/show、事实持久化 | 外部 Provider 与发布候选仍单独验收 |
| 配置与资源管理 | 脱敏 config explain、readonly/coding 预设、自定义 base_url、实例注入、逆序幂等释放；服务关闭与运行登记已按 RUN-002 修复并有回归 | 不承诺热重载/插件热卸载；QA/发布候选仍需重新验收 |
| 权限与安全边界 | Checker 的 interactive/yolo/deny-all、Bash 黑名单、strict/warn/off 敏感路径、SQLite 审计和审批 broker；规则作用域已接入会话/工作区上下文过滤并记录项目 ID，见 SEC-001 证据 | SEC-001 因 QA-001 前置依赖未收口仍为待验证；权限规则不是 OS 沙箱 |
| 工作区事实 | WorkspaceFacts 数据模型、版本信封、工作区归属、运行时编辑事件折叠保存、CLI facts show、重启后读取 | read 事实仍按范围控制；目标平台安装仍按 SHIP-002 单独验收 |
| 事实摘要 | 按事实生成文本、来源信息、预算裁剪、过期/未读取标识、默认关闭；主 Agent 每轮请求前刷新；拒绝 Windows 盘符/UNC/根相对路径及工作区外符号链接 | Windows runner 已覆盖路径边界；真人/外部 Provider 行为仍单独验收 |
| 历史与重启继续 | `--session` 绑定旧 ID、TUI `/session <id>` 切换、历史投影、压缩快照与 steering、旧 job interrupted、真实双压缩重启、恢复面板 PTY | 修复候选上的依赖回验和 PTY scenario1–8 已通过；真人主观体验仍未评价 |
| 运行服务 | internal/runtime.Service，CLI/TUI 经 Start，关闭等待在途运行，排队可取消，瞬时事件有界投递，回调按 run/session 路由；B01 已修复 | RUN-003/UI 依赖回验已通过；真人体验仍单独记录 |
| TUI | Unicode 输入、消息/工具展示、任务卡片、审批组件、恢复面板、忙碌状态栏、Ctrl+C 取消本轮、`/session` 会话切换与历史隔离 | 修复候选的运行服务依赖回验和 PTY scenario1–8 已通过；Windows PTY/真人手感未评价，五平台归档已有上一候选运行证据 |
| 嵌入 | `pkg/agent`、provider、session、tool 公共 API；仓库外 module 可编译运行 examples/embed | 发布包和第三方版本兼容仍按 QA/SHIP 验收 |
| 发布准备 | 五平台源码测试、候选 GoReleaser dry-run、五平台发布归档 Smoke、linux/amd64+arm64 Docker Smoke、最终候选真实 Provider 记录 | B02/B03/B04/B06 已修复；runner 修复后的 CI run `35193874248` 全绿；OpenAI-compatible 旧候选连续 3 次结果仍待在当前 runner 候选重绑，B05 仍缺不同协议/入口证据 |

Unix 进程终止使用进程组信号；Windows 使用 taskkill /T /F，并按 ParentProcessId
补清理 taskkill 竞态漏掉的后代进程，无温和阶段。此实现已有目标 CI 测试记录，但不能据此
推导所有 Windows 安装环境均有 bash/sh 或终端支持。

## 上轮实现验证记录（保留历史）

- 默认与 `sqlite memory` 全量测试通过；完整构建、vet、架构检查和生成物新鲜度通过。
- CI 原 race 范围及额外 runtime/jobs/edits 检查通过；候选 `1e9c819` 的 run
  `34945363069` 中 Quality、Ubuntu、Windows、macOS、Docker Smoke、Release Dry Run 全部通过。
- 候选 `6a2cc3e` 的 run `34946819197` 中三平台 Release Artifact Smoke、Docker Smoke 和 Release Dry Run 全部通过；随后文档提交 `90b5a98` 的 run `34948470600` 重新验证了 Quality、三平台测试、三平台归档 Smoke、Docker Smoke 与 Release Dry Run。
- 真实 PTY 场景 1–8 均已通过仓库内一键编排器在独立临时根、隔离 HOME、当前 checkout 二进制和本地确定性 Provider 下全量复跑，覆盖读改跑/事实重启、后台取消、硬杀恢复、模型能力、外部嵌入、`/session` 切换、审批允许/拒绝和双压缩重启；默认临时根会自动清理。
- 长会话基准以 `-benchmem -benchtime=1x -count=3` 保存原始结果；JSONL 追加已由 [OPT-003](tasks/09-follow-up.md#opt-003) 优化，10000 事件同条件中位数约 1.07 秒，旧基线约 112–130 秒。
- 上轮候选真实外部 Provider 的旧 runner 已由 run `35049625067` 验证：Agent 回合成功、实现文件发生修改、独立 `go test ./...` 退出码为 0；历史结果见 [SHIP-001](development/evidence/SHIP-001.md)；B02/B03 暴露旧 runner 的校验/脱敏边界，需修复后补证。
- 五平台发布归档已在对应 GitHub runner 上解包运行并完成 init、v1→v2 迁移、源文件哈希保持、未来版本拒绝 Smoke，候选 CI 与 Docker Smoke 均有可追溯结果。

## 如何阅读历史证据

[REL-003](development/evidence/REL-003.md) 保存两种协议各 3 次核心调用和 3 次 OpenAI 兼容 CLI 调用；[SHIP-001](development/evidence/SHIP-001.md) 保存离线场景和旧候选工作区的真实模型记录。它们是指定时间/模型/入口的历史结果，不应推广成当前版本或所有模型的可靠性。

[SHIP-003](development/evidence/SHIP-003.md) 记录了用户选择由真实 PTY 自动化替代人工按键验收。继续保留这一验收方式；真人“是否好用”的评价单独标明。此次回退的原因是恢复断言、夹具可复跑性和产品缺陷，并非要求用户重新人工走查。

## 当前工程边界

- Bash 在宿主执行，权限规则不构成 OS 沙箱。
- JSONL 会话追加已改为增量单行写入并保留有界批量同步；CLI 当前固定 JSONL，SQLite 的库支持不等于已接入产品后端切换。
- 完整压缩快照与累积 steering 仍增加存储/请求体积；OPT-003 已补同条件 JSONL/SQLite 对照、快照体积和 profile。
- 文档检查只验证布局、链接、任务与证据结构，不能自动证明业务完成。root README 仅导航，正文仍统一在 docs。

## 2026-09-16 上轮 M8 验收记录（本次复审已回退）

- [全量 CI run 35045978660](https://github.com/wly2lcl/basework/actions/runs/35045978660) 绑定提交
  `2a868864f31c8a66527c1679fd2951a8869c2267`，Quality、五平台测试与发布归档 Smoke、Docker
  Smoke、Release Dry Run 全部成功；发布包迁移与未来版本拒绝断言在每个平台执行。
- [真实 Provider run 35049625067](https://github.com/wly2lcl/basework/actions/runs/35049625067)
  使用 `openai`、`https://newapi.doubb.top/v1`、`agnes-2.5-flash`，Agent 修复固定夹具并由
  独立 `go test ./...` 验证通过；脱敏结果保存在
  [SHIP-001-real-provider-2026-09-16.json](development/evidence/SHIP-001-real-provider-2026-09-16.json)。
- M8 的 SHIP-001、SHIP-002、SHIP-003、QA-001 曾标记完成；本次按 [TASKS](TASKS.md) 回退。真实模型
  结果、脚本化 PTY 结果和发布包结果分开记录；真人主观 TUI 手感不是本次自动化验收结论。

## 2026-09-16 再复审后的行动

- RUN-002 B01 已修复；QA-001 的 B02/B03/B04/B06 已完成局部修复，需在新候选上完成整体验收。
- RUN-003、CTX-003 与 UI 依赖链已在修复候选回验；SHIP-001 仍需补可信协议样本，再重建 SHIP-002/003 的候选证据。
- OPT-002 保留性能基线完成结论；OPT-003 已完成 JSONL 增量追加、锁/残尾回归和同条件 JSONL/SQLite 基准，结果见证据文件。
- 默认/完整构建、相关 race、PTY scenario1–8 与默认 CI 的本轮结果见 [复审报告](development/evidence/REVIEW-2026-09-16.md)。B01–B04/B06 修复回归、正确实现正向验收和超时测试均通过。

## 2026-09-16 修复进度

- RUN-002 的 B01 关闭登记竞态已修复；新增回归与相关 race 通过。
- QA-001 的 B02/B03/B04/B06 已修复：验证使用可信测试副本，Bash 子进程使用显式去凭证环境，独立测试有超时/有界输出/进程树终止，CI 已加入无 tags 全量测试。
- B05 在上一候选 `3ad67a7` 上的 OpenAI-compatible 连续 3 次已完成（runs 35071026911、35071530401、35071644174）；优化后的当前候选 `8394227` 尚未重新生成远端 CI/真实 Provider 证据，不同协议/入口证据仍未完成。RUN-003/UI 依赖回验已通过；OPT-003 已完成。
- 当前本地文档收口已提交并推送到远端 `main`；推送后的 CI 已通过，但真实 Provider 证据仍必须绑定当前远端候选，不能复用上一候选结果。

## 2026-09-16 当前远端候选 CI 回填

提交 `4c40e18e2662166902500bf1ba18124efdfc5a9f` 的 [GitHub Actions run
35077896624](https://github.com/wly2lcl/basework/actions/runs/35077896624) 已全绿。Quality
通过格式、默认全量测试、vet、架构与文档门禁、race、Unix PTY smoke 和 OPT-001 smoke；五个
平台源码测试、五个平台发布归档 Smoke、Docker Smoke 与 Release Dry Run 也全部通过。该提交
只刷新了生成统计，但它是当前远端分支的完整验收候选，后续真实 Provider 结果必须绑定不晚于
该提交且代码内容相同的候选，并在证据中记录实际 workflow run。

这条 CI 证据关闭了当前候选的远端构建、测试、发布归档和镜像门禁；SHIP-001/QA-001 仍等待
当前候选的真实 Provider 运行，且不同协议样本尚未补齐。

## 2026-09-17 当前远端文档候选 CI 回填

提交 `140eddf4cfd0ba457d7b82623c5e83d1b3c46574` 的 [GitHub Actions run
35079524005](https://github.com/wly2lcl/basework/actions/runs/35079524005) 已全绿。Quality、五个平台源码测试、五个平台发布归档 Smoke、Docker Smoke 与 Release Dry Run 全部通过；Quality 同时完成默认全量测试、vet、架构/文档门禁、race、Unix PTY smoke 和 OPT-001 smoke。

该 run 是当前远端文档候选的完整 CI 证据，关闭了构建、测试、发布归档和镜像门禁。它不产生真实 Provider 证据；SHIP-001/QA-001 仍需在当前候选上补 OpenAI-compatible 与不同协议的连续 3 次可信运行。

## 2026-09-17 最新远端提交 CI 回填

远端提交 `8280bda95fe15f21bf8a6bee4c354f2fff4cf277` 的 [GitHub Actions run
35176770398](https://github.com/wly2lcl/basework/actions/runs/35176770398) 已全绿。Quality、五个平台源码测试、五个平台发布归档 Smoke、Docker Smoke 和 Release Dry Run 全部通过；代码行为与当前审查候选 `150e77e` 一致，仅包含文档证据回填。

该 run 关闭了当前远端提交的构建、测试、发布归档和镜像门禁，但不产生真实 Provider 证据。OpenAI-compatible 结果仍需重新绑定当前代码候选，不同协议 Provider 仍需补齐连续 3 次可信运行。

## 2026-09-17 CI 竞态修复

文档候选 `612f670` 的 [CI run 35168351723](https://github.com/wly2lcl/basework/actions/runs/35168351723) 暴露了 macOS Intel `internal/jobs` 测试的真实竞态：shell 重定向先创建空 pid 文件，测试只检查文件存在便读取，偶发得到空内容；本地 goroutine 栈同时确认 LSP 并发启动夹具在共享 `c.conn` 被覆盖时会互相等待。

提交 `66b7cc0` 已修复两处边界：进程树测试等待 pid 内容可解析后再断言；LSP 初始化握手全程使用本次调用的局部连接，并为并发测试提供 5 秒上下文。修复后 jobs/LSP 定向测试、race 压力、默认全量和 sqlite/memory 全量均通过；推送证据提交后的 CI attempt 2 已在 run `35170954145` 全绿回验。

## 2026-09-17 上一候选真实 Provider 与 CI 复核

上一代码候选为 `dd08592adce712af73f1b235dfc829352dc31f1d`，真实 Provider workflow 使用
`provider=openai`、端点 `https://newapi.doubb.top/v1`、模型 `agnes-2.5-flash`。6 次运行的
脱敏 JSON 已随仓库保存；第 3 次 `35169796044` 明确因 `validation_ok=false`、
`file_changed=false` 被门禁拒绝，第 4–6 次 `35170019011`、`35170100696`、`35170184392`
连续通过 `agent_ok`、`validation_ok`、`tests_executed`、`file_changed`，独立测试退出码均为 0，
测试/实现哈希均存在。第 1–2 次也通过，但不跨越第 3 次失败样本计入连续序列。结果文件见
[SHIP-001 证据](development/evidence/SHIP-001.md)。

该候选的 [CI run 35169388623](https://github.com/wly2lcl/basework/actions/runs/35169388623)
中五个平台测试、构建、Docker Smoke 和 Release Dry Run 均通过，Quality 仅在“生成物新鲜度”
步骤失败；日志显示 `docs/STATS.md` 少了修复新增的 4 行测试代码。当前提交已运行 `make gen`
刷新生成物。Provider workflow 绑定的是
`dd08592` 代码候选；本次后续提交只包含证据/文档/生成物，不改变该代码候选的真实模型结论。

当前收口状态：OpenAI-compatible 在上一代码候选上的门禁完成；`150e77e` 的只读预设修复尚未
重新绑定真实 Provider 证据。不同协议 Provider 仍需明确协议、端点、模型和授权后，按相同夹具
在当前候选连续运行 3 次。未获得该输入前不擅自发送其他协议请求，也不把历史 REL-003 多协议
记录冒充当前候选证据。

## 2026-09-17 推送候选 CI 最终结果

提交 `ea8a53c5969cf7b3e07779f6e61405703ad8cd69`（仅包含证据、文档和生成统计）的 [CI run
35170954145](https://github.com/wly2lcl/basework/actions/runs/35170954145) 首次 attempt 1
因 macOS Intel runner 在 GoReleaser 的 `go mod tidy` 阶段解析 `proxy.golang.org` 超时而失败，
日志未出现代码或测试断言失败。随后只重跑失败 job，attempt 2 全绿：Quality、五个平台源码
测试、五个平台发布归档 Smoke、Docker Smoke 和 Release Dry Run 全部通过。

因此候选 CI/发布辅助门禁已完成；首次网络型失败与重跑结果均保留在 workflow 历史中。该 run
的代码内容与 Provider 运行绑定的 `dd08592` 一致，`ea8a53c` 只是证据提交头，不应被写成新的
可执行代码候选。

## 2026-09-17 当前代码候选 CI 回填

提交 `28df499d793c6fa4b17d9a5809df88c64b98547a` 的 [GitHub Actions run
35175075477](https://github.com/wly2lcl/basework/actions/runs/35175075477) 已全绿。Quality
完成格式、默认全量测试、vet、架构/文档门禁、生成物新鲜度、race、Unix PTY 和 OPT-001 PTY；
五个平台源码测试、五个平台发布归档 Smoke、Docker Smoke（amd64/arm64）与 Release Dry Run
也全部通过。

该 run 证明 `150e77e` 代码与本轮文档收口在远端可构建、可测试并可打包；它不产生真实 Provider
证据。OpenAI-compatible 与不同协议的连续 3 次真实运行仍需在当前候选上重新绑定和补齐。

## 2026-09-17 最终候选 CI 与真实 Provider 回填

推送后的最终可执行代码候选为 `a78efba5eabbc9a7391986c86e8b234593bf862c`。远端最新
文档/证据提交为 `01b6277c49e402b3de307f6c87f954cdada538d7`，只增加脱敏结果和记录，不改变
代码候选。`a78efba` 只在 `763a845` 基础上修复 `cmd/basework/permission.go` 的 `gofmt` 缩进；首个 CI run
`35188263404` 因该格式问题失败，其他平台测试已通过。修复后 [CI run
35188993611](https://github.com/wly2lcl/basework/actions/runs/35188993611) 全绿，Quality、五个平台源码测试、五个平台发布归档 Smoke、Docker Smoke 和 Release Dry Run 均通过。

在同一最终候选上使用 `provider=openai`、端点 `https://newapi.doubb.top/v1`、模型
`agnes-2.5-flash` 连续运行 3 次：[35189820351](https://github.com/wly2lcl/basework/actions/runs/35189820351)、
[35189839503](https://github.com/wly2lcl/basework/actions/runs/35189839503)、
[35189879941](https://github.com/wly2lcl/basework/actions/runs/35189879941)。三次均成功，
`agent_ok=true`、`validation_ok=true`、`tests_executed=true`、`file_changed=true`，独立
`go test -count=1 -run ^TestAdd$ ./...` 退出码为 0；脱敏 JSON 已保存到 SHIP-001 证据目录。

文档/证据提交 `01b6277` 触发的 [CI run 35190411247](https://github.com/wly2lcl/basework/actions/runs/35190411247)
也已全绿；该 run 只验证文档提交未破坏构建和发布门禁，不改变真实 Provider 的代码候选绑定。

当前仍未完成的是不同协议 Provider 的当前候选连续 3 次验证，以及依赖其完成的 SHIP-002、
SHIP-003；不能用 OpenAI-compatible 结果替代不同协议证据。

## 2026-09-17 Windows runner 回归修复

文档说明提交 `fb55672` 触发的 [CI run 35191971964](https://github.com/wly2lcl/basework/actions/runs/35191971964)
在 `windows-latest` 的 `tests/real_provider` 超时夹具收尾阶段失败：`go test` 主进程虽已终止，
其子进程仍持有临时目录句柄，导致 `testing.TempDir` 清理报文件被占用。其他四个平台、Quality
和本地 PTY/全量测试均通过。

已在 `tests/real_provider/process_windows.go` 将取消路径改为 PowerShell 快照、逆序终止并等待
整棵验证进程树退出，PowerShell 不可用时保留 `taskkill /T /F` 回退。本地 runner 测试、Windows
amd64 交叉编译和全仓 `-race` 均通过；修复后的 [CI run 35192734568](https://github.com/wly2lcl/basework/actions/runs/35192734568)
已全绿，Windows runner、五平台发布归档 Smoke、Docker Smoke 与 Release Dry Run 均通过。

## 2026-09-17 Windows runner 修复回验

修复提交 `71b4bba768d63591720a9201ebb9eccfe81db508` 的 [CI run 35192734568](https://github.com/wly2lcl/basework/actions/runs/35192734568)
已完成且全绿。`windows-latest` 的 `TestIndependentTestHonorsTimeoutAndBoundsOutput` 通过，说明超时后
验证进程树已在临时目录清理前收尾；Quality、五个平台源码测试、五个平台发布归档 Smoke、Docker Smoke
与 Release Dry Run 也全部通过。该回验关闭了本次 Windows runner 竞态缺口，不改变不同协议 Provider
仍需当前候选连续 3 次真实结果的外部验收要求。

## 2026-09-17 验收 runner 环境白名单加固

提交 `09fd055` 将 `tests/real_provider` 的模型 Bash 与独立测试环境改为显式白名单，移除宿主机
`GOFLAGS`/其他 Provider key 等未声明变量，代理 URL 去除 userinfo，并为两类子进程设置临时
HOME/TMP。随后修复 Windows 白名单缺失 `GOCACHE` 的回归，当前始终使用隔离 HOME 下的
`GOCACHE`/`GOMODCACHE`/`GOPATH`；runner 定向、默认/SQLite 全量、相关 race 和 Windows amd64
交叉编译均通过。该 runner 变化使既有真实 Provider artifact 需重新绑定后才能作为当前候选证据。
