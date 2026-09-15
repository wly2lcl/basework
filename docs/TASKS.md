# 开发任务与进度看板

此表是任务状态的唯一手工来源。任务卡只维护实施内容；证据记录运行结果。原有功能不是重新待实现的任务；本轮复审重开的是有证据的缺口。

## 怎么使用

1. 先按下方复审顺序继续 P1/P2 任务；没有待修复项时，再从 `make progress` 中挑选依赖已完成的待办。默认一次只执行一个任务。
2. 读对应任务卡，改为“进行中”并填负责人（人名或 AI 会话标识）；先确认已有实现，避免重复建设。
3. 代码已写但真实验证未做，记“待验证”；缺凭证/环境/决定而无法推进，记“阻塞”，证据文件写清解除条件。
4. 完成全部验收才记“完成”，必须填写有效证据链接，并更新受影响的当前文档。

允许状态：**待办 / 进行中 / 待验证 / 阻塞 / 完成 / 暂缓**。后置任务的依赖未完成不是“阻塞”，保持待办即可。

“完成任务数/总数”只表示任务数量，不表示工作量百分比、代码质量或发布时间。阶段没有额外人工进度表，避免两份状态漂移。

## 2026-09-14 复审后的执行顺序

[复审报告](development/evidence/REVIEW-2026-09-14.md) 记录证据与完成度判断；[最小复现](development/review-reproduction.md) 可直接转为回归测试。

“进行中”包含已重开、尚待继续修复的任务，不表示已有 AI 在后台并行开发。“待验证”保留既有实现，等待依赖修复或补齐验收。本轮已执行代码修复；未完成的任务保留为进行中/待验证，等待真实入口或跨平台证据。

1. **P1 修复回归**：EDIT-002、JOB-004、RUN-002 已完成；保留对应回归测试，后续变更不得破坏安全撤销、日志恢复和关闭竞态。
2. **恢复产品闭环**：EDIT-003、RUN-003、CTX-001、CTX-003 已完成；继续 CTX-002 的 Windows 路径与外部临时文件验收。
3. **终端验收**：UI-001、UI-002 已完成；UI-003 的恢复面板、会话切换和迟到事件已有单测/PTY 证据，继续候选发布门禁收口。
4. **重新验收**：QA-001 保证夹具可复跑，再完成 SHIP-001 → SHIP-002 → SHIP-003。
5. **后续优化**：OPT-001、OPT-002，优先级低于已有 P1/P2 问题。

卡片中的“2026-09-14 复审补充”给出具体步骤与验收，不从头重做已通过的模块。修复时先读依赖，若前置未完成，只处理不依赖该前置的调查/小切片，不提前验收。

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
| CTX-002 | M5 | [受预算约束的项目摘要](tasks/05-context.md#ctx-002) | 进行中 | CTX-001 | Unix/交叉编译与路径防护已补；待 Windows runner 实测外部临时文件、UNC、软链 | [记录](development/evidence/CTX-002.md) |
| CTX-003 | M5 | [重启与压缩后的项目恢复](tasks/05-context.md#ctx-003) | 进行中 | CTX-002, REL-004, RUN-003 | 真实 TUI 双压缩重启已通过；待 CTX-002 收口后关闭依赖状态 | [记录](development/evidence/CTX-003.md) |
| RUN-001 | M6 | [抽取运行服务接口与所有权](tasks/06-runtime.md#run-001) | 完成 | CFG-003 | 舟（WorkBuddy AI 会话） | [记录](development/evidence/RUN-001.md) |
| RUN-002 | M6 | [有序事件与会话取消](tasks/06-runtime.md#run-002) | 完成 | RUN-001, JOB-004 | Codex | [记录](development/evidence/RUN-002.md) |
| RUN-003 | M6 | [CLI/TUI 统一接入服务](tasks/06-runtime.md#run-003) | 完成 | RUN-002 | Codex | [记录](development/evidence/RUN-003.md) |
| UI-001 | M7 | [工具与后台任务状态卡片](tasks/07-terminal.md#ui-001) | 完成 | RUN-003 | Codex | [记录](development/evidence/UI-001.md) |
| UI-002 | M7 | [权限与编辑确认体验](tasks/07-terminal.md#ui-002) | 完成 | UI-001, EDIT-003 | Codex | [记录](development/evidence/UI-002.md) |
| UI-003 | M7 | [会话切换与可恢复进度](tasks/07-terminal.md#ui-003) | 进行中 | UI-002, CTX-003 | `/session`、恢复面板、迟到事件和 PTY 已通过；待 CTX-002 与候选发布门禁 | [记录](development/evidence/UI-003.md) |
| SHIP-001 | M8 | [固定编码场景回归集](tasks/08-release.md#ship-001) | 待验证 | REL-003, CTX-003 | 清洁候选 checkout、离线/当前 PTY/真实 Provider runner 已在；待注入凭据绑定候选 commit 和当前 Provider 记录 | [记录](development/evidence/SHIP-001.md) |
| SHIP-002 | M8 | [跨平台安装与升级验证](tasks/08-release.md#ship-002) | 待验证 | SHIP-001, UI-003, CFG-002 | 清洁候选双架构镜像已构建并运行；待声明平台安装、候选 CI 与发布包关系 | [记录](development/evidence/SHIP-002.md) |
| SHIP-003 | M8 | [候选版本验收与文档收口](tasks/08-release.md#ship-003) | 待验证 | SHIP-002, QA-001 | 清洁候选 PTY 1–8、恢复/审批和本地门禁已通过；待前置任务和发布候选证据收口 | [记录](development/evidence/SHIP-003.md) |
| QA-001 | M8 | [可复跑的产品验收与证据门禁](tasks/09-follow-up.md#qa-001) | 待验证 | BASE-001, UI-003 | 清洁候选一键 PTY、外部 module、真实 Provider runner、未知 session 与 mutation 负测已在；待凭据运行与候选 CI/平台证据 | [记录](development/evidence/QA-001.md) |
| OPT-001 | M9 | [非交互初始化](tasks/09-follow-up.md#opt-001) | 完成 | CFG-002 | Codex | [记录](development/evidence/OPT-001.md) |
| OPT-002 | M9 | [长会话与长任务性能基线](tasks/09-follow-up.md#opt-002) | 完成 | BASE-001 | Codex | [记录](development/evidence/OPT-002.md) |

## 进度更新纪律

- 完成必须满足依赖已完成、任务卡全部验收项、有实际结果证据。禁止仅因编译通过、加了一个文件或写完计划而勾选完成。
- 阻塞和待验证必须链接进展记录，写“已经做了什么、缺什么、如何继续”；不要改写成完成以美化进度。
- 新发现工作：先判断是否本任务验收必需；否则新增稳定 ID 的卡片和表行，写清依赖，不把临时发现塞进历史归档。
- 完成后发现回归：同时检查下游验收依赖；直接缺陷恢复为“进行中”或“待验证”，证据保留之前结果并说明回退原因。
- `make progress` 从此表现场统计；`make check-docs` 校验合法状态、唯一 ID、有效依赖、循环依赖、任务锚点、卡片依赖一致性以及证据结构。内容真实性仍需代码审查和运行结果支持。

旧 Phase 对应关系见 [路线图](ROADMAP.md)，历史清单见 [归档](archive/README.md)。
