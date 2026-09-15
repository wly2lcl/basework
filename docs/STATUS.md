# 当前实现状态

核对日期：2026-09-15；核心业务代码候选为 `9d7458bb5c53274b5d5b1742cb88abe75ad4ac64`，发布辅助脚本候选为 `ef3041bc319690cce8e30aa9edd45af7a4a8c808`，当前主工作区在这些候选之后只增加了文档证据提交，工作区干净。任务状态只在 [TASKS](TASKS.md) 维护；本页只描述当前实现边界。

**结论：核心实现、真实 Unix PTY 入口和自动测试已具备；CTX-002 的 Windows 实机边界与 M8 发布验收仍未完成。** 双压缩重启链路已由 CTX-003 的真实产品场景收口。缺陷、复现与历史证据的边界见 [复审报告](development/evidence/REVIEW-2026-09-14.md)。任务状态只在 [TASKS](TASKS.md) 维护。

## 当前能力与缺口

| 能力 | 已有实现与证据 | 当前实际边界 |
|---|---|---|
| Agent 与模型协议 | Agent 循环、流式工具参数归一化、能力三态、缺 key 说明；REL-001/002 | Provider 静态声明不等于每个真实端点都已验证 |
| 请求审计与精确投影 | 完整 ChatMessage 比较、按请求 Seq 重建、schema v2、完整压缩快照、旧无快照兼容；REL-004 与包级测试 | compatible 允许审计写入失败后继续；strict 在 Provider 前中断。Hook 与传输改写仍超出重建边界 |
| 主运行入口行为配置 | 压缩、循环检测、观测、子代理和动态事实摘要均通过 runtime options 接入 | 真实外部 Provider 的兼容性仍按 REL-003/SHIP-001 单独验证 |
| 后台命令 | owner、状态机、输出配额/溢出、读取、取消、进程树终止、JSONL 历史、尾部修复、CLI/TUI 接线；重启归并 interrupted | 输出文件清理和跨平台 shell 能力仍受目标系统约束 |
| 编辑预览与提交 | 工作区/软链/权限检查、内容基线、逐文件写入、部分成功清单；撤销重新检查路径与权限 | 跨文件不是事务；撤销遇到用户二次编辑会按文件跳过 |
| 编辑产品闭环 | edit_files 预览/提交/撤销、真实 plan 路径/diff 审批、file.edited 事件、edits list/show、事实持久化 | 外部 Provider 与发布候选仍单独验收 |
| 配置与资源管理 | 脱敏 config explain、readonly/coding 预设、自定义 base_url、实例注入、逆序幂等释放；服务关闭会取消并等待在途运行 | 不承诺热重载/插件热卸载；跨平台发布与外部 Provider 仍单独验收 |
| 工作区事实 | WorkspaceFacts 数据模型、版本信封、工作区归属、运行时编辑事件折叠保存、CLI facts show、重启后读取 | read 事实仍按范围控制；Windows 外部路径夹具属于 CTX-002 |
| 事实摘要 | 按事实生成文本、来源信息、预算裁剪、过期/未读取标识、默认关闭；主 Agent 每轮请求前刷新；拒绝 Windows 盘符/UNC/根相对路径及工作区外符号链接 | Windows 实机路径和跨平台发布仍待 CTX-002 |
| 历史与重启继续 | `--session` 绑定旧 ID、TUI `/session <id>` 切换、历史投影、压缩快照与 steering、旧 job interrupted、真实双压缩重启、恢复面板 PTY | UI-003 仍需在候选发布门禁中收口；CTX-002 前置 Windows 路径证据未完成 |
| 运行服务 | internal/runtime.Service，CLI/TUI 经 Start，关闭等待在途运行，排队可取消，瞬时事件有界投递，回调按 run/session 路由 | 发布平台和真实 Provider 体验仍单独验收 |
| TUI | Unicode 输入、消息/工具展示、任务卡片、审批组件、恢复面板、忙碌状态栏、Ctrl+C 取消本轮、`/session` 会话切换与历史隔离 | 真人手感、Windows PTY 和跨平台安装仍待发布任务 |
| 嵌入 | `pkg/agent`、provider、session、tool 公共 API；仓库外 module 可编译运行 examples/embed | 发布包和第三方版本兼容仍按 QA/SHIP 验收 |
| 发布准备 | 三系统源码测试、代码候选 GoReleaser 五平台归档与哈希、Darwin arm64 归档运行、linux/amd64+arm64 OCI 构建运行与恢复场景 | 尚缺 Windows/Linux 与 Darwin x86_64 目标安装、真实 CI run 和外部 Provider 验收（SHIP-002/003） |

Unix 进程终止使用进程组信号；Windows 使用 taskkill /T /F，无温和阶段。此实现已有目标 CI 测试记录，但不能据此推导所有 Windows 安装环境均有 bash/sh 或终端支持。

## 本轮验证

- 默认与 `sqlite memory` 全量测试通过；完整构建、vet、架构检查和生成物新鲜度通过。
- CI 原 race 范围及额外 runtime/jobs/edits 检查通过。
- 真实 PTY 场景 1–8 均已通过仓库内一键编排器在独立临时根、隔离 HOME、当前 checkout 二进制和本地确定性 Provider 下全量复跑，覆盖读改跑/事实重启、后台取消、硬杀恢复、模型能力、外部嵌入、`/session` 切换、审批允许/拒绝和双压缩重启；默认临时根会自动清理。
- 长会话基准以 `-benchmem -benchtime=1x -count=3` 保存原始结果；JSONL 逐条追加在 10000 事件达到百秒级，已转为后续优化候选。
- 当前尚未验证真实外部 Provider、代码候选对应的 Windows/Linux 与 Darwin x86_64 发布产物实际安装、真实 CI run 和真人主观体验；Docker 多平台镜像已对清洁候选完成构建与本地运行，但不能替代候选 CI 和目标平台安装证据。

## 如何阅读历史证据

[REL-003](development/evidence/REL-003.md) 保存两种协议各 3 次核心调用和 3 次 OpenAI 兼容 CLI 调用；[SHIP-001](development/evidence/SHIP-001.md) 保存离线场景和旧候选工作区的真实模型记录。它们是指定时间/模型/入口的历史结果，不应推广成当前版本或所有模型的可靠性。

[SHIP-003](development/evidence/SHIP-003.md) 记录了用户选择由真实 PTY 自动化替代人工按键验收。继续保留这一验收方式；真人“是否好用”的评价单独标明。此次回退的原因是恢复断言、夹具可复跑性和产品缺陷，并非要求用户重新人工走查。

## 当前工程边界

- Bash 在宿主执行，权限规则不构成 OS 沙箱。
- JSONL 单次会话追加重写全文件，长会话成本需量化；CLI 当前固定 JSONL，SQLite 的库支持不等于已接入产品后端切换。
- 完整压缩快照与累积 steering 增加存储/请求体积；优化前先补正确性与基准证据。
- 文档检查只验证布局、链接、任务与证据结构，不能自动证明业务完成。root README 仅导航，正文仍统一在 docs。
