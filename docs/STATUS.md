# 当前实现状态

核对日期：2026-09-12。基线为当前工作区，含本轮开始前已有的未提交实现；不能据此声称已发布。最新本地标签为 `v0.1.2`，发布记录以 Git tag 和 Release 为准。

## 能力与实际边界

| 能力 | 当前实现依据 | 已知边界 |
|---|---|---|
| Agent 循环、流式响应、工具调用 | [agent 契约](reference/pkg/agent.md)、`pkg/agent/` | 真实 Provider 兼容性仍需矩阵验证 |
| CLI/TUI 共享组装 | `cmd/basework/runtime.go`、`agent.go`、`tui.go` | 基础入口已接线，不代表所有交互体验成熟 |
| 基础编辑、命令、搜索及增强工具 | [tool 契约](reference/pkg/tool.md)、`internal/tools/` | 后台 job、统一编辑预览/提交/撤销尚无完整产品闭环 |
| 会话事件、JSONL/SQLite、压缩 | [session 契约](reference/pkg/session.md)、`internal/compaction/` | JSONL 重写成本和异常恢复需要持续检验 |
| 格式版本与相邻迁移 | `pkg/session/migrate.go` | 当前 schema v2；已有迁移不应重新实现；高版本日志拒绝处理 |
| 压缩结果快照与历史恢复 | `pkg/session/projection.go`、`pkg/agent/loop.go` | 新 `compacted` 事件保存策略实际产出的完整消息快照，投影精确恢复选择/重排结果；旧无快照事件仍走 `Summary` / `TruncatedSeq` / `KeepFrom` 兼容分支。包级测试和单次 AgentLoop 集成测试已覆盖；快照增加存储体积，CTX-003 两次压缩、重启恢复及真实 job/file 证据仍待验收 |
| 请求来源与指纹 | `pkg/agent/request.go`、`pipeline.go` | `CheckRequestInvariant` 深比较完整 `ChatMessage`，但尚未接入运行时；Hook 改写与 Provider 传输改写超出现有重建边界。system prompt / steering 落盘失败阻断本轮 Provider 调用并保留重试状态；`request.built` 写入失败仍只记日志继续 |
| Hook / Plugin / Skill | [hook](reference/pkg/hook.md)、[skill](reference/pkg/skill.md)、`pkg/agent/plugin.go` | Plugin 为 Initialize/Shutdown 生命周期，不具备通用注册回滚和热卸载 |
| MCP / LSP / Memory | 对应[包契约](reference/README.md) | 外部服务可用性需另验；memory 为可选构建能力 |
| 权限、超时、审计、子代理 | `internal/permission/`、`internal/subagent/` | Bash 在宿主执行；权限规则不构成操作系统沙箱 |
| 配置、主题、模板、性能分析 | `pkg/config/`、`internal/tui/` | 当前 `profile` 子命令是 pprof 性能分析，不是 dsh 式配置组合 |
| 文档与架构门禁 | `scripts/doccheck/`、`scripts/gendeps/`、`tests/arch_test.go` | 检查结构和证据存在性，不自动证明文档语义或真实业务完成 |

## 验证证据如何理解

本次文档整理的检查结果见 [DOC-001 记录](development/evidence/DOC-001.md)。此前工作区已运行过带 `sqlite memory` 的全量测试和架构检查；这属于历史本地证据，本轮没有因此把未来任务勾为完成。

当前缺少按统一模板保存的真实模型编码闭环、长任务中断恢复、跨平台干净安装和人工 TUI 验证报告。它们属于下一阶段的验收内容。CI 配置存在不等于本次跨平台 CI 已通过，mock 测试通过不等于真实模型验证通过。

规模见自动生成的 [STATS](STATS.md)，依赖见 [DEPGRAPH](DEPGRAPH.md)。后续方向见 [路线图](ROADMAP.md)，执行与进度只在 [TASKS](TASKS.md) 更新。
