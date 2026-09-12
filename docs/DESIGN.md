# Basework 设计文档

> AI Agent 编程框架 — 从 openwork 做减法，取 opencode 与 crush 之长
>
> 本文件为索引。原 3137 行的单文件设计文档已按主题拆分到 `docs/design/`。
> 拆分本身不改内容，**除下表「拆分后的修订记录」逐条登记者外，内容逐字保留**。
> 新增设计请写进对应主题文件，并在此登记。

> **核对方式**：以 `git show <拆分前提交>:docs/DESIGN.md` 为基线，对
> `docs/design/*.md` 合并结果做标题与正文的双重集合比对，差异必须全部能在
> 下方修订记录里找到对应条目。

## 主题索引

| 文件 | 内容 | 对应原章节 |
|------|------|-----------|
| [design/01-overview.md](design/01-overview.md) | 定位与目标、架构总览、与 openwork 的对比、Build Tag 策略、目录结构、核心设计决策记录 | §1 §2 §4 §5 §6 §7 §15 §16 |
| [design/02-layers.md](design/02-layers.md) | 各层详细设计（L0 基础层 ~ L4 应用层） | §3 |
| [design/03-integrations.md](design/03-integrations.md) | 补充设计：LSP 集成、MCP 协议支持 | §8 §9 |
| [design/04-runtime.md](design/04-runtime.md) | 补充设计：上下文压缩、Steering、流式传播、Memory 系统、次要缺口 | §10 §11 §12 §13 §14 |
| [design/05-prompt-runtime.md](design/05-prompt-runtime.md) | System Prompt 组合流程、并发模型、Token 计数策略 | §17 §18 §19 |
| [design/06-security-obs.md](design/06-security-obs.md) | 安全模型、Logging 与可观测性、MCP/LSP 权限集成、错误处理、优雅关闭、迁移策略 | §20 §21 §22 §23 §24 §25 |
| [design/07-terminal-product.md](design/07-terminal-product.md) | 终端产品模块设计（`internal/`） | §26 |

## 拆分后的修订记录

拆分本身不改内容；下列修订是拆分后为与实现对齐而单独做的，逐条登记：

- **2026-09-12** `design/01-overview.md` §2.2（架构总览）— **新增内容**
  - 新增「§2.2 下一阶段核心能力演进」及其 6 个子节（2.2.1 Provider 与 Tool Call
    稳定性、2.2.2 Tool Job 生命周期、2.2.3 可靠编辑与 Diff、2.2.4 Workspace
    Context、2.2.5 Backend / Server / Client 分层、2.2.6 TUI 产品化边界）。
    **原文没有这一节** —— 拆分前 `docs/DESIGN.md` 的 §2 只有「分层架构」「层间依赖
    规则」「数据流」三节。
  - 连带仅改编号：原 §2.2 层间依赖规则 → §2.3，原 §2.3 数据流 → §2.4（正文未动）。
  - 与 `docs/TASKS.md` 的 Phase 36（36.1~36.6）配对：本处写设计取舍与硬边界，
    TASKS.md 写可执行任务项，两者分工、不重复任务清单。
  - 新增动机：Phase 36 的两条硬边界（「不污染 `pkg/` 可嵌入核心」「不为模型兼容
    牺牲工具能力」）属于设计契约，放在路线图里容易被读成排期，故移入设计文档。

- **2026-09-11** `design/07-terminal-product.md` §26.10（终端 UI）
  - 修正「组件通信模式」：原描述为「回调直接调用 `app.AddToolCall()` /
    `UpdateStreamingText()` 等方法」，该写法在 `tea.Cmd` goroutine 中改 Bubble Tea
    Model，属数据竞态；改为「回调只投递 `tea.Msg`，状态变更集中在事件循环内」，
    并新增 §26.10.1 说明 post-binding 接线。
  - 补充 `callback.go` 与 4 个流式 `tea.Msg`，补全 `theme/ keymap/ command/
    dialog/ plugin/` 子包。
  - 依据：`ARCHITECTURE.md` 分层约束 + 代码实际实现（`internal/tui/callback.go`、
    `cmd/basework/tui.go`）。权威架构描述以 `ARCHITECTURE.md` 为准。

## 相关文档

- [架构概览](../ARCHITECTURE.md) — 系统架构、模块依赖、API 兼容性承诺（**权威**）
- [项目状态](STATUS.md) — 当前实现状态与已知问题
- [代码统计](STATS.md) — 自动生成的规模与覆盖率数据
- [路线图](../ROADMAP.md) — 未来规划
- [任务清单](TASKS.md) — 可执行的 checklist

## 待处理

拆分过程中发现以下内容增生，**未擅自删除**，留待确认。拆分后它们已同处
`design/01-overview.md` 内（行号见括号），合并成本低于拆分前：

- §6 目录结构 与 §15 更新后的目录结构 内容重叠（同文件 L232 / L291）
- §7 核心设计决策记录 与 §16 更新后的设计决策记录 内容重叠，后者是前者的增量版本
  （D1~D4 / D5~D13，同文件 L238 / L346）

确认后建议合并为单一小节，并同步核对 `ARCHITECTURE.md` 的目录树——
**目录结构以 `ARCHITECTURE.md` 为准**，`design/01-overview.md` 只保留设计意图。
