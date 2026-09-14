# 当前实现状态

核对日期：2026-09-14；代码基线：`bb455ff93bbb`，审查开始时工作区干净。本次修改为文档复审，未修复业务代码。最新本地标签为 `v0.1.4`；候选代码、Git tag 与实际发布产物分别核对。

**结论：核心实现和现有自动测试已具备，M0–M8 的产品验收尚未全部完成。** 缺陷、复现与历史证据的边界见 [复审报告](development/evidence/REVIEW-2026-09-14.md)。任务状态只在 [TASKS](TASKS.md) 维护。

## 当前能力与缺口

| 能力 | 已有实现与证据 | 当前实际边界 |
|---|---|---|
| Agent 与模型协议 | Agent 循环、流式工具参数归一化、能力三态、缺 key 说明；REL-001/002 | Provider 静态声明不等于每个真实端点都已验证 |
| 请求审计与精确投影 | 完整 ChatMessage 比较、按请求 Seq 重建、schema v2、完整压缩快照、旧无快照兼容；REL-004 与包级测试 | compatible 允许审计写入失败后继续；strict 在 Provider 前中断。Hook 与传输改写仍超出重建边界 |
| 主运行入口行为配置 | 压缩、循环检测、观测与子代理适配器存在 | 主 newRuntimeAgent 遗漏 runtimeBehaviorOptions；启用压缩后 compactor 仍为 nil，默认 loopDetector 也为 nil。不能据 helper 单测宣称入口生效（RUN-003） |
| 后台命令 | owner、状态机、输出配额/溢出、读取、取消、进程树终止、JSONL 历史、CLI 接线 | 日志残缺尾后继续追加会损坏历史；重启新会话不能归并旧 owner；输出持久化引用不等于跨重启可读全部输出（JOB-004） |
| 编辑预览与提交 | 工作区/软链检查、内容基线、逐文件临时文件+rename、部分成功清单 | 跨文件不是事务；撤销仅进程内，且当前撤销未重查权限/路径，可因父目录软链改动写到工作区外（EDIT-002） |
| 编辑产品闭环 | edit_files 预览/提交/撤销、file.edited 事件、edits list/show | commit 审批无法从 plan_id 展示真实计划；部分成功写入会被事实投影遗漏；并发计划操作需补验收（EDIT-003/UI-002） |
| 配置与资源管理 | 脱敏 config explain、readonly/coding 预设、自定义 base_url、实例注入、逆序幂等释放 | 不承诺热重载/插件热卸载；服务关闭与在途运行的协调仍有缺口 |
| 工作区事实 | WorkspaceFacts 数据模型、版本信封、工作区归属、CLI 临时聚合 | SaveWorkspaceFacts 没有生产调用方；真实提交后重启摘要仍为空。普通工具路径、部分成功与来源覆盖需补齐（CTX-001） |
| 事实摘要 | 按事实生成文本、来源信息、配置开关，默认关闭 | 目前只在启动时读取事实文件；Collect 丢失哈希基线，实际过期检测不生效；最终文本可超预算，单文件读取未限额；Windows 反斜杠路径保护需补（CTX-002） |
| 历史与重启继续 | Store 可重开、事件可投影、两次手工构造快照可恢复 | 每次 agent.New 创建新 session；没有按旧 ID 恢复 CLI/TUI 的入口。旧 running job 保持 running。存储测试不等于真实重启继续（CTX-003） |
| 运行服务 | internal/runtime.Service，CLI/TUI 经 Start，瞬时生命周期事件有界投递 | 排队 Start 在关闭后仍可进入 Agent；关闭后 Subscribe 不结束；Close 未先等待在途调用。流式消息路由不能只靠解绑回调（RUN-002/003） |
| TUI | Unicode 输入、消息/工具展示、任务卡片、审批组件与恢复组件 | 分页可漏掉已读未展示的行；窄恢复面板可 panic；SwitchSession 不清旧消息/忙状态且不是可操作的切换入口。Ctrl+C 当前退出整个界面（UI-001/002/003） |
| 嵌入 | pkg/agent、provider、session、tool 可供宿主调用 | examples/embed 引用了 internal/runtime，不能当作外部 module 的公共 API 范例；需要外部模块编译验收（QA-001） |
| 发布准备 | 三系统源码测试、GoReleaser dry-run、旧 macOS arm64 包的安装/迁移记录 | 尚缺当前候选产物在声明平台的实际运行、Docker 构建运行和可靠的恢复场景验收（SHIP-002/003） |

Unix 进程终止使用进程组信号；Windows 使用 taskkill /T /F，无温和阶段。此实现已有目标 CI 测试记录，但不能据此推导所有 Windows 安装环境均有 bash/sh 或终端支持。

## 本轮验证

- 默认与 `sqlite memory` 全量测试通过；完整构建、vet、架构检查和生成物新鲜度通过。
- CI 原 race 范围及额外 runtime/jobs/edits 检查通过。
- 16 个针对缺口的临时 overlay 测试全部复现失败，代码保存在 [复现文档](development/review-reproduction.md)，没有加入或修改现有业务测试。
- 全新目录执行现有 PTY 文档入口，因缺少 fixtures 退出 1；未把旧临时目录的成功当作干净复跑成功。
- [CI run 34797549128](https://github.com/wly2lcl/basework/actions/runs/34797549128) 已实时核验 success，对应 `84a39e4`。其三平台源码测试与打包 dry-run 有效；它不等于当前文档修改已在 CI 验证，也不包含 Docker/安装产物运行。
- 本轮未重跑真实模型、目标平台安装、Docker 或真人主观体验。构建标签、测试次数和环境见复审报告。

## 如何阅读历史证据

[REL-003](development/evidence/REL-003.md) 保存两种协议各 3 次核心调用和 3 次 OpenAI 兼容 CLI 调用；[SHIP-001](development/evidence/SHIP-001.md) 保存离线场景和旧候选工作区的真实模型记录。它们是指定时间/模型/入口的历史结果，不应推广成当前版本或所有模型的可靠性。

[SHIP-003](development/evidence/SHIP-003.md) 记录了用户选择由真实 PTY 自动化替代人工按键验收。继续保留这一验收方式；真人“是否好用”的评价单独标明。此次回退的原因是恢复断言、夹具可复跑性和产品缺陷，并非要求用户重新人工走查。

## 当前工程边界

- Bash 在宿主执行，权限规则不构成 OS 沙箱。
- JSONL 单次会话追加重写全文件，长会话成本需量化；CLI 当前固定 JSONL，SQLite 的库支持不等于已接入产品后端切换。
- 完整压缩快照与累积 steering 增加存储/请求体积；优化前先补正确性与基准证据。
- 文档检查只验证布局、链接、任务与证据结构，不能自动证明业务完成。root README 仅导航，正文仍统一在 docs。
