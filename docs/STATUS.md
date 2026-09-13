# 当前实现状态

核对日期：2026-09-13。基线为本工作区 HEAD `935fde1`，与 `origin/main` 同一提交；工作区存在未提交改动（M2–M7 的全部成果与本次 `REL-003` 验证记录都尚未提交），不能据此声称已发布。最新本地标签为 `v0.1.4`，发布记录以 Git tag 和 Release 为准。

## 能力与实际边界

| 能力 | 当前实现依据 | 已知边界 |
|---|---|---|
| Agent 循环、流式响应、工具调用 | [agent 契约](reference/pkg/agent.md)、`pkg/agent/` | 真实 Provider 兼容性仍需矩阵验证 |
| 流式工具调用参数归一化 | `pkg/provider/openai.go`、`pkg/agent/pipeline.go`、[llm 契约](reference/pkg/llm.md) | 归一化在 Provider 层完成：`Complete=false` 是参数增量片段、`Complete=true` 是完整参数，完成事件按 `Index` 升序；已覆盖多调用交错、断流补齐、空文本与流式不重试；真实 Provider 端到端仍未验证 |
| CLI/TUI 共享组装 | `cmd/basework/runtime.go`、`agent.go`、`tui.go` | 基础入口已接线，不代表所有交互体验成熟。运行服务契约（RUN-001）：`internal/runtime.Service`（start/cancel/session/subscribe/close）+ 进程内 `LocalService` 实现，所有权成文（Agent/任务归 Service，订阅归调用方）；`runtimeAgent.Service()` 幂等适配，CLI/TUI 现有路径零改动；事件排序与流式语义属 RUN-002。RUN-002 已落地：瞬时 RunEvent 与持久化会话事件边界成文；订阅有界缓冲+丢弃计数+关闭唤醒；同会话运行串行排队；Close 取消进行中运行；中断持久化复用 turn.failed 与任务日志。CLI/TUI 已统一接入服务（RUN-003）：两入口运行经 `internal/runtime.Service`，取消/错误语义一致（取消=Run.Err 的 context.Canceled）；`CallbackSwitch` 按运行绑入/解绑 UI 回调，收尾增量不串下一次运行；订阅事件带 RunID/SessionID，TUI 按会话路由，退出路径统一 `defer` 退订。TUI 后台任务卡片（UI-001）：`ctrl+j` 打开任务列表（内存态+历史合并），状态带文字标记（排队/运行中/成功/失败/已取消/超时/已中断），输出分页读取（30 行/8KB 上限，大输出不卡渲染），`x` 取消运行中任务（终态拒绝）；运行终态以文字通知进入消息流（完成/已取消/失败）。权限审批（UI-002）：TUI 弹窗确认（唯一 ID 请求协议，展示工具/目的/路径/diff/风险依据，y 允许、n/esc 关闭均等价拒绝，超时自动拒绝），决策按 ID 隔离且不缓存（批准不泄漏到下一请求）；确认后提交仍受工具层基线与权限校验；CLI 保持终端问答。会话切换与恢复（UI-003）：`SwitchSession` 重绑会话并清空旧状态（运行事件/任务快照按会话隔离，不串界面）；启动时装入恢复面板（interrupted/failed 任务标 `[需重试]`、最近修改、上次验证命令附建议重跑）；操作指南见 docs/TUI.md。事实摘要（CTX-002）：`facts_summary` 配置段（默认关闭）开启后把预算内的工作区事实摘要注入 system prompt，相关性排序、来源可追踪、哈希变化标记过期、受保护路径不读取（与工具层同策略）；注入随 request.built 审计指纹落账。重启恢复（CTX-003）：`compacted` 事件保存压缩器完整消息快照，投影遇快照精确替换历史（支持选择/删除/重排，不能用单锚点近似）；旧无快照事件按 Summary/TruncatedSeq/KeepFrom 兼容恢复；端到端验证两次压缩+重启后不重复不丢消息、system prompt 与 steering 全部保留（steering 累积重放）、工具配对与文件冲突状态保留；快照体积增量已量化（完整历史 vs 单锚点） |
| 基础编辑、命令、搜索及增强工具 | [tool 契约](reference/pkg/tool.md)、`internal/tools/`、`internal/jobs/`、`internal/edits/` | 后台 job 已有状态/归属（`internal/jobs`）、输出承接（内存配额、超配额溢出到 0600 私有文件、按偏移分页且切点在 UTF-8 边界、越权读取返回 `ErrNotFound`、保留期与关闭清理）与进程树终止（独立进程组、温和终止 + 宽限期后强制结束、`timed_out` 与 `canceled` 分开记录、`CloseOwner` 取消并清理单个 owner）；编辑已有只读预览/基线校验与批量提交（`internal/edits`：提交前重新解析路径与权限、重新比对内容哈希、逐文件临时文件 + rename 原子替换、冲突即不写并给出逐文件已写/未写清单、撤销前确认文件仍等于本次写入结果否则跳过）。job 已接线为模型可调用工具（`job_list`/`job_output`/`job_cancel`）与 CLI 只读查看（`basework jobs list`/`show`），JSONL 日志随会话持久化，重启归并 `interrupted`、不自动重跑（JOB-004）。编辑已接入产品闭环：`edit_files` 工具走预览（不写盘）→ 提交（基线与权限重查、拒绝重复提交）→ 撤销（保护用户二次编辑），每步落 `file.edited` 会话事件；`basework edits list`/`show` 只读审阅，会话恢复后记录仍可用（EDIT-003）。仍缺：编辑确认的 TUI 交互体验（UI-002）；跨文件不构成事务，撤销仅限进程内。进程组仅在 unix 生效，Windows 退化为终止直接子进程（`jobs.ProcessGroupSupported()`），运行时行为未实测 
| 会话事件、JSONL/SQLite、压缩 | [session 契约](reference/pkg/session.md)、`internal/compaction/` | JSONL 重写成本和异常恢复需要持续检验。工作区事实模型（CTX-001）：`session.WorkspaceFacts` 以去重键累计读/改/撤销/命令事实，按工作区 ID 隔离；事实文件带版本信封，可读 v0 裸数组、拒绝未知未来版本；`file.edited` 事件与 job 日志新增可选 `workspace_id` 归属戳（兼容变更），旧数据标记「未归属」不冒充当前工作区 |
| 格式版本与相邻迁移 | `pkg/session/migrate.go` | 当前 schema v2；已有迁移不应重新实现；高版本日志拒绝处理 |
| 压缩结果快照与历史恢复 | `pkg/session/projection.go`、`pkg/agent/loop.go` | 新 `compacted` 事件保存策略实际产出的完整消息快照，投影精确恢复选择/重排结果；旧无快照事件仍走 `Summary` / `TruncatedSeq` / `KeepFrom` 兼容分支。包级测试和单次 AgentLoop 集成测试已覆盖；快照增加存储体积，CTX-003 两次压缩、重启恢复及真实 job/file 证据仍待验收 |
| 请求来源与指纹 | `pkg/agent/request.go`、`audit.go`、`pipeline.go` | `CheckRequestInvariant` 深比较完整 `ChatMessage`；`RebuildRequestAt` / `VerifyRequestBuilt` 支持按请求自己的 Seq 重建核对（不能用最终日志代替当时快照）。审计策略两档：默认 `compatible` 写入失败只记日志继续，`BASEWORK_AUDIT_MODE=strict` 时在 Provider 调用前中断。Hook 改写与 Provider 传输改写超出现有重建边界（见 ADR 0005） |
| 模型能力与 key 说明 | `pkg/llm/capability.go`、`pkg/provider/capabilities.go`、`cmd/basework/model.go` | 能力结论三态（supported / unsupported / unknown）并标注来源（declared / configured / probed）；`model list` 展示 tools/streaming/vision 与 key 需求，`model info` 给出依据；运行入口做能力预检。全部为静态声明，尚无 probed 实测来源 |
| Hook / Plugin / Skill | [hook](reference/pkg/hook.md)、[skill](reference/pkg/skill.md)、`pkg/agent/plugin.go` | Plugin 为 Initialize/Shutdown 生命周期，不具备通用注册回滚和热卸载 |
| MCP / LSP / Memory | 对应[包契约](reference/README.md) | 外部服务可用性需另验；memory 为可选构建能力 |
| 权限、超时、审计、子代理 | `internal/permission/`、`internal/subagent/` | Bash 在宿主执行；权限规则不构成操作系统沙箱。`basework config explain` 提供只读的有效配置解释（来源/环境覆盖/各 provider key 来源状态/递归脱敏后的有效值，秘密不进输出，CFG-001）。资源归属与释放：builtin 全局注入点有实例级替代（`Runtime`/`AllWithRuntime`，旧入口兼容），运行时资源统一 `runtimeResourceScope` 逆序/幂等释放，`agent.New` 失败不再遗留 LSP/MCP/子进程（CFG-003）。启动预设（CFG-002，ADR 0006）：`readonly`/`coding` 两档，覆盖次序 内置默认→配置文件（Load 时展开）→命令行 `--preset`（agent/tui 同参），数组整体替换不并集；`readonly` 最终工具表过只读白名单硬校验，写/执行泄漏即拒绝启动；未知名/跨层冲突/文件内双重指定均明确报错，同值幂等合法 |
| 配置、主题、模板、性能分析 | `pkg/config/`、`internal/tui/` | 当前 `profile` 子命令是 pprof 性能分析，不是 dsh 式配置组合。自定义模型端点（CFG-004，ADR 0007）：`providers.<name>.base_url` / `api_key` 与 `BASEWORK_BASE_URL` 都可生效，解析走 `Config.ResolveEndpoint` 单一实现（runtime 与 `config explain` 同口径）；`base_url` 形态写错在加载或启动时明确报错，不静默回落到默认端点；未配置时行为与改动前一致。未做：per-provider 环境变量别名（如 `OPENAI_BASE_URL`）、`bedrock`/`azure` 自定义端点的实测 |
| 文档与架构门禁 | `scripts/doccheck/`、`scripts/gendeps/`、`tests/arch_test.go` | 检查结构和证据存在性，不自动证明文档语义或真实业务完成 |

## 验证证据如何理解

当前工作区基线的复跑记录见 [BASE-001 记录](development/evidence/BASE-001.md)：HEAD `935fde1`，默认与 `sqlite memory` 两个模式均通过构建、静态检查和全量测试（当时各 31 个 `ok` 包），架构边界、文档校验、生成物新鲜度与 race 检查均通过，失败归类为代码 / 环境 / 外部服务各 0 项。文档整理本身的检查结果见 [DOC-001 记录](development/evidence/DOC-001.md)。

2026-09-13 复核：包数随 M2–M7 新增的 `internal/edits`、`internal/runtime` 增至各 33 个 `ok`，构建、vet、gofmt、全量测试与 race 仍全部通过；但**生成物新鲜度门禁当时不通过**——工作区里的 `docs/STATS.md`、`docs/DEPGRAPH.md` 落后于代码（漏计 `internal/tui/dialog/approval.go` 等），已重跑 `make gen` 同步（当前为 434 个 `.go` 文件、`internal` 17 个包）并验证生成器幂等。

真实模型调用已有两条路径的证据，均记录在 [REL-003 记录](development/evidence/REL-003.md)：核心层（可嵌入 API）对两个协议各跑 3 次真实闭环 6/6 通过；产品层（CLI）在自定义端点配置（[CFG-004](development/evidence/CFG-004.md)）落地后，用同一场景重跑 3 次全部通过（退出码 0、文件确实被改、`go test` 由 CLI 进程之外独立复核）。端点生效性有反向对照：把 `base_url` 指向不可达地址后错误直接指向该端点，而不再是此前那种误导性的 401。

仍缺少按统一模板保存的长任务中断恢复、在**目标系统**上的跨平台干净安装（只有 darwin/arm64 产物真实运行过，其余平台仅交叉编译）、Docker 镜像的实际构建与运行（本机缺 `buildx`，CI 的 dry-run 也刻意跳过 docker）和人工 TUI 验证报告；CLI 路径只跑了 `openai` 协议一侧，`anthropic` 协议仅核心层验证。`tests/integration_test.go` 的 `TestIntegration_OpenAI_E2E` 测试体仍被整段注释，属占位测试，不能当作真实闭环证据。仍未在 Ubuntu / Windows 上本地验证；`tests/benchmark` 的基准测试在 `go test` 下报 `no tests to run`，从未实际执行。CI 配置存在不等于跨平台 CI 已通过。

2026-09-13 M8 发布准备（[SHIP-001](development/evidence/SHIP-001.md)、[SHIP-002](development/evidence/SHIP-002.md)、[SHIP-003](development/evidence/SHIP-003.md)）：

- **固定编码场景回归集（SHIP-001）**：新增 `tests/coding_scenarios_test.go`，五个场景（小函数修复、多文件修改、大输出、权限拒绝、断流恢复）+ 半截参数变体共 6 个测试，用 `t.TempDir()` 隔离、脚本化模型驱动，断言落在磁盘产物与工具轨迹上，`-count=2` 复跑一致。**真实模型**（`agnes-2.5-flash`，OpenAI 兼容协议）另跑固定 8 次——小函数修复 3、多文件修改 2、大输出/权限拒绝/断流恢复各 1——**8/8 通过**；两个编写类场景由夹具在 agent 回合之外独立复核 `go test`，退出码 5 次全为 0。实测：大输出单次工具结果 102434 字节被有界截断并带标记；权限拒绝 2 次且无副作用、整轮未中断；断流首次调用按错误暴露、同会话第二次调用由真实模型正常完成、悬挂工具调用 0。另有本地 stalling server 验证「上游停滞可被调用方 context 终止」（2s 上限实测 2.003s 结束），且 agent 层会把取消转成真实错误而非空回复。
  - **不得据此宣称**：单次运行的三个场景不代表成功率；token 数未经账单核对、不推算费用；断流那一次是**注入**的而非真实网络中断；夹具属核心层，不含 ACL/会话落盘/后台 job/TUI。
- **跨平台安装与升级验证（SHIP-002）**：在 macOS arm64 上验证 GoReleaser 产物本身。打包 dry-run 与 CI 命令逐字一致并成功产出 5 个包（`windows/arm64` 按 `.goreleaser.yml` 的 `ignore` 有意不构建）；darwin/arm64 产物**解包后实际运行**（`version` 报 `0.1.4-SNAPSHOT-935fde1`）；隔离 `HOME` 下 `init` 生成配置、`config explain` 可读；旧版本（v1）会话升级后源 JSONL **逐字节未变**且事件与正文完整落进 SQLite；未知高版本（v99）在读、写两条路径都退出码 1 且**无部分导入**。
  - **发现并修复 3 个迁移缺陷**：短会话 ID（<12 字符）触发 panic 且迁移后事件外键失配丢数据；`session list` 吞掉"版本过高"错误而谎报无会话；`migrate` 自行解析 JSONL 绕过版本检查，把未来版本数据"成功"导入。
  - **仍未获得**：linux/amd64、linux/arm64、darwin/amd64、windows/amd64 只在 CI 矩阵定义中，**本轮未实际触发 CI，也从未在目标系统上运行**——不得把交叉编译当作平台验证；Docker/GHCR 渠道只有配置层证据（`goreleaser check` 通过 + 与 `installation.md`/`release.md` 逐条一致），**未构建镜像**。
  - 已知限制：`basework init` 无 `--yes`/非交互开关，stdin 为保持打开且无数据的管道时会阻塞等待菜单输入（stdin 为 `/dev/null` 时正常）。
- **候选版本验收与 TUI 收口（SHIP-003）**：候选版本快照（HEAD `935fde1` + 未提交改动）上重跑全部门禁，过程中发现并修复 **3 个 P1 缺陷**——`pkg/lsp` 的 `Conn.Start()` check-then-act 竞态（改为原子 CAS）；TUI 输入框按字节过滤按键导致**中文与空格全部丢失**（改为 rune 语义，`input.go` 与 `dialog/input.go` 两处）；`agent.New` 返回类型未实现 `SessionIDProvider` 使 `bindRuntimeJobOwner` 静默失败、**后台任务在所有模式下必然报"会话为空"**（补 `(*AgentLoop).SessionID()`，加法式）。三个缺陷均有回归测试。
  - **路线图 5 条代表性场景由终端级自动化 TUI 验收走完**（用户指定以自动化替代人工走查）：夹具固化在 `tests/tui_pty/`（PTY 驱动真实二进制 + 脚本化 SSE 服务 + VT 抓屏），修复缺陷 / 耗时检查 / 中断后继续（SIGKILL 后恢复、遗留任务标 interrupted、不自动重跑）/ 使用不同模型 / 嵌入 Go 服务（新增 `examples/embed`）全部通过；真实模型（`agnes-2.5-flash`）在真实终端完整走一遍修复缺陷场景，14.8s 完成，磁盘修复与独立 `go test` 复核通过。
  - **边界（不得据此宣称）**：终端级自动化证明"按键驱动下产品行为正确"，**不证明**"真人觉得好用"——主观体验无真人证据；跨平台 CI 与 Docker 镜像两项发布外部证据仍不存在；TUI 运行中状态栏保持「空闲」不变（反馈只靠旋转器与工具卡片），属已知 UX 打磨项。


规模见自动生成的 [STATS](STATS.md)，依赖见 [DEPGRAPH](DEPGRAPH.md)。后续方向见 [路线图](ROADMAP.md)，执行与进度只在 [TASKS](TASKS.md) 更新。
