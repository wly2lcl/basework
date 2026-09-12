# dsh 借鉴与 Basework 落点

核对日期：2026-09-12。dsh 依据为官方主分支文档快照，主分支会变化；本文是设计借鉴，未运行 dsh 做性能或质量对比。

| dsh 的做法 | Basework 当前基础 | 采用方式与任务 |
|---|---|---|
| 请求从日志派生并冻结 | 已有事件请求组装和指纹，写入失败可继续，运行期未强制校验 | REL-004：先定审计模式、Hook 与传输边界，再接线和故障测试 |
| Profiles 组合并可查看有效配置 | 当前是单配置 Store；`profile` 命令用于 pprof | CFG-001/002：脱敏配置解释与启动预设，保留旧配置语义 |
| 服务定义、实现、消费者分离；注册具有可撤销效果 | 小接口注入已存在，Plugin 仅提供生命周期 | CFG-003、RUN-001：明确资源归属与逆序清理，不引入 Cordis |
| Jobs 与 terminal 区分生命周期 | Bash 主要同步执行 | JOB-001～004：先做后台进程与输出、取消、恢复事实；PTY 交互另行设计 |
| 包文档包含 Model Experience 与已知限制 | 原包说明已有用途、配置、扩展点、限制 | 集中到 `docs/reference/pkg/`，补模型/调用方如何使用与如何获知失败 |
| 版本化会话与相邻迁移 | 已有 schema v2、迁移链、压缩 Seq 锚点和摘要 | 保留并回归验证；不重复新建，不将现有 JSONL 称为不可变代际存储 |

依据：[dsh 架构](https://github.com/deepseek-ai/deepseek-harness/blob/master/docs/architecture.md)、[包说明规范](https://github.com/deepseek-ai/deepseek-harness/blob/master/packages/README.md)、[Cordis 入门](https://github.com/deepseek-ai/deepseek-harness/blob/master/docs/cordis-primer.md)、[Bash 包](https://github.com/deepseek-ai/deepseek-harness/blob/master/packages/shell/tool-bash/README.md)、[Terminal 包](https://github.com/deepseek-ai/deepseek-harness/blob/master/packages/terminal/README.md)。

Basework 保留 Go 进程内嵌入与终端优先方向。后台命令、可靠编辑、项目事实恢复与可见进度是本项目的业务优先级判断，不能仅凭参考项目拥有某个模块就推断需要照搬。

dsh 官方仍标为 developer preview；其 sandbox/permission 不应被当作已审计的安全保证。参见 [项目说明](https://github.com/deepseek-ai/deepseek-harness) 与 [安全说明](https://github.com/deepseek-ai/deepseek-harness/blob/master/SAFETY.md)。

旧统计与执行过程保存在 [历史归档](../archive/README.md)，不参与当前任务状态。落实计划见 [路线图](../ROADMAP.md) 与 [任务看板](../TASKS.md)。
