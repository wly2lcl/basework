# 开发任务与进度看板

此表是任务状态的唯一手工来源。任务卡只维护实施内容；证据记录运行结果。原有功能不是重新待实现的任务；本轮复审重开的是有证据的缺口。

## 怎么使用

1. 先按下方复审顺序继续 P1/P2 任务；没有待修复项时，再从 `make progress` 中挑选依赖已完成的待办。默认一次只执行一个任务。
2. 读对应任务卡，改为“进行中”并填负责人（人名或 AI 会话标识）；先确认已有实现，避免重复建设。
3. 代码已写但真实验证未做，记“待验证”；缺凭证/环境/决定而无法推进，记“阻塞”，证据文件写清解除条件。
4. 完成全部验收才记“完成”，必须填写有效证据链接，并更新受影响的当前文档。

允许状态：**待办 / 进行中 / 待验证 / 阻塞 / 完成 / 暂缓**。后置任务的依赖未完成不是“阻塞”，保持待办即可。

“完成任务数/总数”只表示任务数量，不表示工作量百分比、代码质量或发布时间。阶段没有额外人工进度表，避免两份状态漂移。

## 2026-09-16 复审后的执行顺序

[本次复审报告](development/evidence/REVIEW-2026-09-16.md) 记录 B01–B07；[一键最小复现](development/review-reproduction-2026-09-16.md) 保存三个已确认的失败探针。此前 32/32 是旧快照，不能覆盖新发现。历史修复见 [上次复审](development/evidence/REVIEW-2026-09-14.md)。

1. **RUN-002 / P1**：修复 Close 与 cancel 登记交错（B01），验证关闭不会漏取消。
2. **QA-001 / P1、P2**：先做不依赖 UI 的局部修复：凭证隔离/脱敏（B03）→ 可信独立测试（B02）→ 验证超时/清理（B04）→ CI 默认构建（B06）。最终验收仍需前置 UI-003 完成。
3. **依赖回验**：RUN-003 → CTX-003 → UI-001/002/003。只复测受影响的关闭、取消、恢复和入口，不重写已完成模块。
4. **重新验收**：QA-001 满足后，SHIP-001 补原任务要求的协议/次数/入口样本。上一代码候选 `dd08592` 的 OpenAI-compatible 连续 3 次已完成（runs `35170019011`、`35170100696`、`35170184392`）；本次 `150e77e` 新增只读预设修复，需在当前候选重新绑定真实 Provider，再补不同协议连续 3 次证据，随后完成 SHIP-002 → SHIP-003。已有五平台归档证据保留，新代码候选需更新对应记录。
5. **OPT-003 / P3**：依据 OPT-002 基线实施长会话存储优化；不是发布前 P1 修复的替代。

“进行中”表示已重开待修复，不表示 AI 在后台运行。“待验证”含依赖回退，保留既有实现和通过记录。每次只领取一个明确切片，卡片中的新补充与原验收必须同时满足。

## 任务表

| ID | 阶段 | 任务卡 | 状态 | 依赖 | 负责人 | 证据 |
|---|---|---|---|---|---|---|
| DOC-001 | M0 | [文档集中与可执行任务体系](tasks/00-baseline.md#doc-001) | 完成 | — | Codex / 本轮文档整理 | [记录](development/evidence/DOC-001.md) |
| BASE-001 | M0 | [建立可复跑的当前基线](tasks/00-baseline.md#base-001) | 完成 | DOC-001 | 舟（WorkBuddy AI 会话） | [记录](development/evidence/BASE-001.md) |
| REL-001 | M1 | [流式工具调用协议回归](tasks/01-reliability.md#rel-001) | 完成 | BASE-001 | 舟（WorkBuddy AI 会话） | [记录](development/evidence/REL-001.md) |
| REL-002 | M1 | [模型能力与认证信息说明](tasks/01-reliability.md#rel-002) | 完成 | REL-001 | 舟（WorkBuddy AI 会话） | [记录](development/evidence/REL-002.md) |
| REL-003 | M1 | [真实模型编码闭环验证](tasks/01-reliability.md#rel-003) | 完成 | REL-002 | 舟（WorkBuddy AI 会话） | [记录](development/evidence/REL-003.md) |
| REL-004 | M1 | [请求审计运行时策略](tasks/01-reliability.md#rel-004) | 完成 | REL-001 | 舟（WorkBuddy AI 会话） | [记录](development/evidence/REL-004.md) |
| JOB-001 | M2 | [定义并实现 job 状态与归属](tasks/02-jobs.md#job-001) | 完成 | BASE-001 | 舟（WorkBuddy AI 会话） | [记录](development/evidence/JOB-001.md) |
| JOB-002 | M2 | [增量输出与文件溢出](tasks/02-jobs.md#job-002) | 完成 | JOB-001 | 舟（WorkBuddy AI 会话） | [记录](development/evidence/JOB-002.md) |
| JOB-003 | M2 | [超时取消与进程树清理](tasks/02-jobs.md#job-003) | 完成 | JOB-002 | 舟（WorkBuddy AI 会话） | [记录](development/evidence/JOB-003.md) |
| JOB-004 | M2 | [后台任务恢复与 CLI 接线](tasks/02-jobs.md#job-004) | 完成 | JOB-003 | Codex | [记录](development/evidence/JOB-004.md) |
| EDIT-001 | M3 | [统一编辑预览和冲突契约](tasks/03-editing.md#edit-001) | 完成 | BASE-001 | 舟（WorkBuddy AI 会话） | [记录](development/evidence/EDIT-001.md) |
| EDIT-002 | M3 | [批量提交与安全撤销](tasks/03-editing.md#edit-002) | 完成 | EDIT-001 | Codex | [记录](development/evidence/EDIT-002.md) |
| EDIT-003 | M3 | [编辑事实与 CLI 审阅闭环](tasks/03-editing.md#edit-003) | 完成 | EDIT-002 | Codex | [记录](development/evidence/EDIT-003.md) |
| CFG-001 | M4 | [脱敏的有效配置解释](tasks/04-composition.md#cfg-001) | 完成 | BASE-001 | 舟（WorkBuddy AI 会话） | [记录](development/evidence/CFG-001.md) |
| CFG-002 | M4 | [可组合启动预设](tasks/04-composition.md#cfg-002) | 完成 | CFG-001 | 舟（WorkBuddy AI 会话） | [记录](development/evidence/CFG-002.md) |
| CFG-003 | M4 | [注册资源归属与逆序释放](tasks/04-composition.md#cfg-003) | 完成 | BASE-001 | 舟（WorkBuddy AI 会话） | [记录](development/evidence/CFG-003.md) |
| CFG-004 | M4 | [自定义模型端点配置](tasks/04-composition.md#cfg-004) | 完成 | BASE-001 | 舟（WorkBuddy AI 会话） | [记录](development/evidence/CFG-004.md) |
| CTX-001 | M5 | [统一工作区事实模型](tasks/05-context.md#ctx-001) | 完成 | JOB-004, EDIT-003 | Codex | [记录](development/evidence/CTX-001.md) |
| CTX-002 | M5 | [受预算约束的项目摘要](tasks/05-context.md#ctx-002) | 完成 | CTX-001 | Codex | [记录](development/evidence/CTX-002.md) |
| CTX-003 | M5 | [重启与压缩后的项目恢复](tasks/05-context.md#ctx-003) | 完成 | CTX-002, REL-004, RUN-003 | Codex | [记录](development/evidence/CTX-003.md) |
| RUN-001 | M6 | [抽取运行服务接口与所有权](tasks/06-runtime.md#run-001) | 完成 | CFG-003 | 舟（WorkBuddy AI 会话） | [记录](development/evidence/RUN-001.md) |
| RUN-002 | M6 | [有序事件与会话取消](tasks/06-runtime.md#run-002) | 完成 | RUN-001, JOB-004 | Codex | [记录](development/evidence/RUN-002.md) |
| RUN-003 | M6 | [CLI/TUI 统一接入服务](tasks/06-runtime.md#run-003) | 完成 | RUN-002 | Codex | [记录](development/evidence/RUN-003.md) |
| UI-001 | M7 | [工具与后台任务状态卡片](tasks/07-terminal.md#ui-001) | 完成 | RUN-003 | Codex | [记录](development/evidence/UI-001.md) |
| UI-002 | M7 | [权限与编辑确认体验](tasks/07-terminal.md#ui-002) | 完成 | UI-001, EDIT-003 | Codex | [记录](development/evidence/UI-002.md) |
| UI-003 | M7 | [会话切换与可恢复进度](tasks/07-terminal.md#ui-003) | 完成 | UI-002, CTX-003 | Codex | [记录](development/evidence/UI-003.md) |
| SHIP-001 | M8 | [固定编码场景回归集](tasks/08-release.md#ship-001) | 进行中 | REL-003, CTX-003, QA-001 | Codex / 复审重开 | [记录](development/evidence/SHIP-001.md) |
| SHIP-002 | M8 | [跨平台安装与升级验证](tasks/08-release.md#ship-002) | 待验证 | SHIP-001, UI-003, CFG-002 | Codex / 依赖回验 | [记录](development/evidence/SHIP-002.md) |
| SHIP-003 | M8 | [候选版本验收与文档收口](tasks/08-release.md#ship-003) | 待验证 | SHIP-002, QA-001 | Codex / 依赖回验 | [记录](development/evidence/SHIP-003.md) |
| QA-001 | M8 | [可复跑的产品验收与证据门禁](tasks/09-follow-up.md#qa-001) | 进行中 | BASE-001, UI-003 | Codex / 复审重开 | [记录](development/evidence/QA-001.md) |
| SEC-001 | M9 | [权限作用域上下文过滤](tasks/09-follow-up.md#sec-001) | 待验证 | QA-001 | Codex / 当前会话 | [记录](development/evidence/SEC-001.md) |
| OPT-001 | M9 | [非交互初始化](tasks/09-follow-up.md#opt-001) | 完成 | CFG-002 | Codex | [记录](development/evidence/OPT-001.md) |
| OPT-002 | M9 | [长会话与长任务性能基线](tasks/09-follow-up.md#opt-002) | 完成 | BASE-001 | Codex | [记录](development/evidence/OPT-002.md) |
| OPT-003 | M9 | [降低长会话追加开销](tasks/09-follow-up.md#opt-003) | 完成 | OPT-002 | Codex | [记录](development/evidence/OPT-003.md) |

## 进度更新纪律

- 完成必须满足依赖已完成、任务卡全部验收项、有实际结果证据。禁止仅因编译通过、加了一个文件或写完计划而勾选完成。
- 阻塞和待验证必须链接进展记录，写“已经做了什么、缺什么、如何继续”；不要改写成完成以美化进度。
- 新发现工作：先判断是否本任务验收必需；否则新增稳定 ID 的卡片和表行，写清依赖，不把临时发现塞进历史归档。
- 完成后发现回归：同时检查下游验收依赖；直接缺陷恢复为“进行中”或“待验证”，证据保留之前结果并说明回退原因。
- `make progress` 从此表现场统计；`make check-docs` 校验合法状态、唯一 ID、有效依赖、循环依赖、任务锚点、卡片依赖一致性以及证据结构。内容真实性仍需代码审查和运行结果支持。

## 2026-09-17 当前候选进度

- `be737c0` 是本轮 SEC-001 业务修复候选；在 `150e77e` 补齐 readonly 预设空权限模式安全回退的基础上，新增权限作用域上下文过滤、SQLite 审计项目归属、CLI 作用域关联校验，以及在途检查的上下文快照、精确参数缓存匹配、审计归属、旧自动规则兼容和作用域 ID help，并完成 SQLite 参数缓存回归与运行时文档收口。最终可执行代码候选为 `a78efba`，远端最新文档/证据提交为 `01b6277`。
- 本地默认与 `sqlite memory` 全量测试、jobs/LSP 定向回归、race 压力、`make check-docs` 和 `git diff --check` 已通过。
- 最终远端候选为 `a78efba`：`763a845` 推送后只追加一处 `gofmt` 格式修复，解决 Quality 门禁发现的帮助文本缩进问题。默认与 `sqlite memory` 全量测试、jobs/LSP 定向回归、race 压力、`make check-docs`、`git diff --check` 和 `gofmt` 均通过。
- 最终候选的 GitHub Actions CI run `35188993611` 已全绿：Quality、五个平台源码测试、五个平台发布归档 Smoke、Docker Smoke 和 Release Dry Run 均通过。
- 文档/证据提交 `01b6277` 只增加 3 份脱敏 Provider JSON 和验收记录；其 GitHub Actions CI run `35190411247` 也已全绿，不改变 `a78efba` 的代码候选绑定。
- 文档说明提交 `fb55672` 的 CI run `35191971964` 暴露 Windows `tests/real_provider` 超时夹具的子进程句柄收尾竞态；已在 `tests/real_provider/process_windows.go` 增加整棵进程树快照、终止和等待，并通过本地 runner 回归与 Windows amd64 交叉编译。修复提交 `71b4bba` 的 CI run `35192734568` 已全绿回验，Windows runner、五平台发布归档 Smoke、Docker Smoke 与 Release Dry Run 均通过。
- `09fd055` 进一步将 `tests/real_provider` 的模型 Bash 与独立测试环境改为显式白名单，移除宿主机 `GOFLAGS`/其他 Provider key 等未声明变量，代理 URL 去除 userinfo，并为两类子进程设置临时 HOME/TMP。随后修复 Windows 白名单缺失 `GOCACHE` 的回归，当前始终使用隔离 HOME 下的 `GOCACHE`/`GOMODCACHE`/`GOPATH`；runner 定向、默认/SQLite 全量、相关 race 和 Windows amd64 交叉编译均通过。修复后的 CI run `35196510402` 已全绿，包含 Windows 源码测试、五平台发布归档 Smoke、Docker Smoke 和 Release Dry Run。该 runner 变化使既有真实 Provider artifact 需重新绑定后才能作为当前候选证据。
- OpenAI-compatible 真实 Provider 使用 `agnes-2.5-flash` 在产品候选 `a78efba` 连续 3 次可信通过（runs `35189820351`、`35189839503`、`35189879941`）；三份脱敏 artifact 已纳入 `docs/development/evidence/`，每次均为 `agent_ok=true`、`validation_ok=true`、`tests_executed=true`、`file_changed=true`，独立测试退出码为 0。因 `09fd055` 更新验收 runner，需在当前 runner 候选重新执行后再作为最终证据。
- 推送提交 `ea8a53c` 后的 GitHub Actions run `35170954145` attempt 2 全绿；attempt 1 的 macOS Intel 归档步骤因 `proxy.golang.org` DNS 超时失败，重跑后五平台测试/构建、归档 Smoke、Docker Smoke、Release Dry Run 和 Quality 均通过。
- 当前任务表仍保持 `SHIP-001=进行中`、`QA-001=进行中`、`SHIP-002/SHIP-003=待验证`。唯一明确的外部验收缺口是不同协议 Provider 的当前候选连续 3 次结果；需要用户提供协议、端点、模型并授权后执行。
- SEC-001 已接入 Checker、SQLite、缓存、迁移、运行时会话/工作区上下文和审计项目字段；实现验证见 [SEC-001](development/evidence/SEC-001.md)，因 QA-001 前置依赖未收口而保持待验证，不改变当前发布候选的 Provider/CI 门禁结论。

旧 Phase 对应关系见 [路线图](ROADMAP.md)，历史清单见 [归档](archive/README.md)。
