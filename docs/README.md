# Basework 文档中心

项目文档正文只在 `docs/` 维护，根目录 README 只负责导航。代码包说明统一在 `reference/pkg/`，不再散落在源码目录。GitHub Issue/PR 表单模板保留在 `.github/` 的固定位置；它们是平台入口模板，不是另一套项目文档。

## 按目的阅读

| 我想做什么 | 从这里开始 | 内容边界 |
|---|---|---|
| 使用项目 | [快速开始](getting-started.md) | 构建、配置、首次使用 |
| 看现在能做什么 | [当前状态](STATUS.md) | 实现与验证边界，不维护待办 |
| 看后续做什么 | [业务路线图](ROADMAP.md) | 用户场景、阶段目标、发布门槛 |
| 看开发进度、选下一项 | [任务看板](TASKS.md) | 任务状态的唯一手工来源 |
| 交给能力较弱的 AI 开发 | [AI 开发流程](development/ai-workflow.md) | 一次一个任务、输入、验收、交接模板 |
| 理解代码怎么分层 | [架构](ARCHITECTURE.md) · [依赖图](DEPGRAPH.md) | 当前边界与自动生成的结构 |
| 找接口与模块约束 | [包契约索引](reference/README.md) | 用途、配置、扩展点、模型使用体验、限制 |
| 找设计和借鉴依据 | [设计入口](DESIGN.md) · [dsh 借鉴分析](analysis/deepseek-harness-vs-basework.md) | 当前约束、未来方案与历史依据分开标记 |
| 参与开发或发布 | [贡献指南](CONTRIBUTING.md) · [验证规则](development/validation.md) · [发布流程](release.md) | 开发与发布检查 |

## 使用指南

- [安装](installation.md) · [Docker](docker.md) · [FAQ](FAQ.md)
- [CLI](guides/cli-guide.md) · [TUI](guides/tui-guide.md) · [配置](guides/configuration.md) · [Provider](guides/provider-guide.md)
- [嵌入 Go 应用](guides/embedder-guide.md) · [扩展](guides/extending.md) · [迁移](guides/migration.md)
- [权限](guides/permission-guide.md) · [安全边界](guides/security.md) · [子代理](guides/subagent-guide.md)
- [主题](guides/theme.md) · [提示模板](guides/templates.md) · [性能分析](guides/profiling.md)

## 文档维护规则

| 信息 | 唯一权威位置 | 更新时机 |
|---|---|---|
| 任务状态、依赖、证据链接 | `TASKS.md` | 开始、阻塞、交接、验收时 |
| 每项任务的步骤和验收条件 | `tasks/` 中对应任务卡 | 拆任务或范围变更时，不复制状态 |
| 实际实现 | `STATUS.md` 与相关包契约 | 功能验收后 |
| 用户目标与阶段边界 | `ROADMAP.md` | 调整方向时，不维护完成比例 |
| 代码数量、模块依赖 | `STATS.md`、`DEPGRAPH.md` | `make gen` 自动生成 |
| 架构决定 | `adr/` | 改变公共契约或持久化语义时 |
| 运行证据 | `development/evidence/` | 测试/人工验证后，记录日期与环境 |
| 过去方案、旧 Phase 和旧统计 | [历史归档](archive/README.md) | 保留追溯，不作为执行入口 |

现有使用指南描述当前命令；任务卡中的“拟新增”接口、目录和命令仅是规划。发现冲突时先核对代码，再改对应权威文件。

## 治理与记录

[变更日志](CHANGELOG.md) · [安全策略](SECURITY.md) · [行为准则](CODE_OF_CONDUCT.md) · [ADR 模板](adr/TEMPLATE.md) · [代码统计](STATS.md)

运行 `make check-docs` 检查布局、相对文件链接、包契约与任务状态；运行 `make progress` 查看按阶段统计的任务数量。计数不等于工作量或产品成熟度。
