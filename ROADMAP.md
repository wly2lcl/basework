# Basework 路线图

> 从可嵌入框架到独立终端产品的完整演进路线。
>
> 本文件是**唯一的路线图**。此前根目录与 `docs/` 各有一份且结论互斥，已合并至此。
> 可执行的细化待办见 [docs/TASKS.md](docs/TASKS.md)。

---

## 当前状态

| 项 | 值 |
|----|-----|
| 版本 | `v0.1.2`（以 git tag 为准） |
| 定位 | 可嵌入的 Go Agent 框架 + 终端 AI 编程助手（alpha） |
| 代码规模 / 测试 | 见 [docs/STATS.md](docs/STATS.md)，由 `make stats` 自动生成 |
| 默认模型 | OpenCode Zen 的 `big-pickle`（免费） |
| Go 版本 | 1.26+ |

> 文档中的规模与覆盖率数字一律引用 `make stats` 的输出，不再手工维护 —— 历史上
> 手工维护的数字曾与真实值相差数十个百分点（旧 STATUS.md 称「50,051 行 / 1,372 测试」，
> 与自动统计口径下的真实值不符）。

### 阶段状态判定口径

本文件对每个 Phase 的状态以「**代码实际存在 + git tag + CHANGELOG 条目**」三者交叉判定，
不采信单一文档的叙述：

- 仓库中存在对应目录/符号，且 CHANGELOG 有相应发布条目 → ✅ 已完成
- 否则 → 🔲 未开始 / 🚧 进行中

---

## 已完成

### Phase 1-12：核心框架 ✅

| Phase | 模块 |
|-------|------|
| 1 | `pkg/llm/` 统一类型系统、错误分类、能力检测 |
| 2 | `pkg/hook/` Hook 注册、PubSub 事件总线 |
| 3 | `pkg/session/` JSONL 存储、事件溯源、投影 |
| 4 | `pkg/tool/` 工具接口、注册表、内置工具 |
| 5 | `pkg/agent/` Agent 循环、流式处理、工具调用 |
| 6 | `pkg/provider/` 工厂模式 |
| 7 | `pkg/lsp/` LSP 客户端、自动发现 |
| 8 | `pkg/mcp/` MCP 客户端、stdio/HTTP 传输 |
| 9 | `pkg/memory/` 文件映射、FTS5 检索（build tag: `memory`） |
| 10 | `pkg/config/` + `pkg/skill/` 配置（CoW 模式）、技能加载 |
| 11 | `cmd/basework/` CLI 入口、cobra 子命令 |
| 12 | 集成测试 + 文档 |

### Phase 13-25：终端产品化 ✅

| Phase | 模块 |
|-------|------|
| 13 | `internal/compaction/` 上下文压缩 |
| 14 | `internal/retry/` 指数退避重试 |
| 15 | `internal/permission/` 权限规则引擎、YOLO 模式 |
| 16 | `internal/subagent/` 子代理、成本传播 |
| 17 | `internal/tools/` 增强工具（web_fetch/web_search/todowrite/apply_patch/question） |
| 18 | `internal/tui/` Bubble Tea 终端 UI |
| 19 | `pkg/session/` SQLite 后端（`sqlite` tag）、文件追踪、队列 |
| 20 | `pkg/provider/` 扩展至 15+ Provider |
| 21 | `internal/observability/` 结构化日志、成本追踪 |
| 22 | `internal/loopdetect/` SHA-256 签名循环检测 |
| 23 | `pkg/provider/cache.go` Prompt 缓存；`pkg/tool/builtin/blacklist.go` 命令黑名单 |
| 24 | `pkg/mcp/` 资源 + 提示 + 自动重连 |
| 25 | `internal/oauth/` PKCE 流程、令牌刷新 |

### Phase 26-30：生产韧性 + 工程化收口 ✅

| Phase | 内容 |
|-------|------|
| 26 | 会话稳定性：文件锁 + force unlock、SQLite WAL、长会话压缩、会话队列与恢复 |
| 27 | CI/CD：GitHub Actions（quality / 跨平台 test / release dry-run）、GoReleaser、GitHub Release、GHCR 镜像 |
| 28 | 安全加固：权限持久化（SQLite）、敏感路径保护、工具执行超时、pprof、benchmark 套件、审计日志 |
| 29 | TUI 增强：主题系统、命令面板、键盘绑定、Provider 感知模板、环境动态注入、对话框 |
| 30 | 多模态 + 生态：图片输入、Hook 扩展点、Provider 扩展点、TUI 插件基础设施 |

### Phase 31-35：关键缺陷修复与主路径收口 ✅

Phase 31-34 为分轮次的关键问题修复（pipeline/并发、SQLite、配置默认值合并、CLI/TUI 主路径
wiring、安全修复如 `apply_patch` 路径遍历与 `web_fetch` SSRF 防护、协议/数据/可靠性加固）。
Phase 35 为主路径收口。

详见 [CHANGELOG.md](CHANGELOG.md)。

---

## 规划中

### Phase 36：核心能力追赶 crush 🚧

2026-07-09 与 crush 对比后确认：basework 底层模块覆盖已较完整，但真实编程任务的执行体验、
工具生命周期、TUI 产品化和后台服务架构仍有明显差距。后续不再优先堆 Provider 或零散 UI，
而是集中补齐下列能力。

| 序号 | 优先级 | 能力 | 目标 |
|------|--------|------|------|
| 36.1 | P0 | Provider + tool call 稳定性 | 免费模型、OpenAI-compatible 与商业模型都能稳定保留工具调用能力 |
| 36.2 | P0 | 后台 shell/job 管理 | 长命令可后台运行、流式输出、取消、查询历史输出 |
| 36.3 | P0 | 可靠编辑与 diff 工作流 | multi-edit、结构化 diff、写入前确认、touched files 追踪 |
| 36.4 | P1 | Workspace 上下文 | 记录读过/改过/运行过什么，恢复会话时重建项目摘要 |
| 36.5 | P1 | Backend/server/client 分层 | TUI 与 agent runtime 解耦，支持取消运行、多会话、事件订阅 |
| 36.6 | P1 | TUI 产品化第一阶段 | 工具状态、diff view、模型/会话/权限弹窗、补全与通知 |

**依赖顺序**：关键路径为 `36.1 → 36.3 → 36.4 → 36.6`；`36.2` 与 `36.3` 可并行，
`36.4` 与 `36.5` 可并行。`36.5` 会再次触及 `pkg↔internal` 边界，必须在架构边界
清理完成之后进行。

验收标准见 [docs/TASKS.md](docs/TASKS.md) 的 Phase 36 章节。

### 持续维护项

- **真实使用验证**：用长会话、跨仓库编码任务和多人机器环境持续验证稳定性
- **Homebrew 发布**：接入 tap 后再恢复 Homebrew 安装文档（当前未接入）
- **Release 体验**：持续校验 GitHub Release、GHCR 镜像与安装文档的一致性
- **文档一致性**：配置 schema、CLI 行为、默认模型与发布方式保持同步

### 未排期方向

以下已识别但未排入具体 Phase，视需求优先级择机实施：

- 工作流引擎（多步骤任务编排与 DAG 执行）
- 评估框架（LLM 输出质量评估与回归测试）
- 远程 Agent（分布式通信与协作）

---

## 与参考项目对比

| 维度 | basework | crush | opencode |
|------|----------|-------|----------|
| 定位 | 可嵌入框架 + 终端产品 | 终端产品 | 终端产品 |
| 架构 | `pkg/` + `internal/` 分层 | 全 `internal/` | monorepo |
| Provider | 15+ | 较少 | 10+ |
| MCP | 工具 + 资源 + 提示 | 仅工具 | 工具 + 资源 |
| TUI | 基础 TUI，工具过程展示不足 | 生产级 | 自研框架 |
| 会话管理 | JSONL + SQLite + 文件锁 | SQLite + 文件锁 | 事件溯源 + 文件锁 |
| 后台服务 | 单进程为主，server 分层待建 | backend/server/client 分层 | 多进程/服务化 |
| 工具体验 | 基础工具完整，后台 job/diff/multiedit 待增强 | 工具 UI 与后台任务更成熟 | 工具生态更完整 |
| LSP | ✅ | 较少 | 部分 |
| 多模态 | ✅ 图片输入 | ❌ | ✅ 图片 |
| 插件生态 | 基础扩展点 | ❌ | ✅ 多种扩展 |
| CI/CD | ✅ quality / test / release dry-run | ✅ | ✅ 多条工作流 |

**核心判断**：basework 是「可嵌入的 `pkg/` 层」这一架构优势的持有者 —— 这是 crush 与
opencode 都不具备的。劣势在于真实编程任务的执行体验与产品成熟度。

**一句话总结**：架构更好，打磨更少。下一阶段的重点不是继续堆功能广度，而是围绕真实
编程任务补齐 provider/tool 稳定性、后台命令、可靠编辑、workspace 上下文、backend 分层
与 TUI 产品化。

---

## 设计原则

### 可嵌入优先

框架核心可独立于终端产品使用，`agent.New(WithModel(...), WithTools(...))` 即可嵌入任何
Go 应用。**`pkg/` 不得依赖 `internal/`** —— 该约束由测试
`TestPkgDoesNotImportInternal`（`tests/pkg_no_internal_test.go`）强制守护，CI 中执行。

需要 `pkg/` 接收产品层实现时，统一做法是**在 `pkg` 内定义最小接口**，由 `internal/`
的具体类型结构化满足，而不是反向 import。已有的接口注入点见
[ARCHITECTURE.md 分层硬约束](ARCHITECTURE.md)。

> 背景：Phase 33 已为 `pkg/agent` 定义过 `Compactor` / `LoopDetector` /
> `PermissionChecker` / `EventPublisher` / `SubAgentRunner` 五个接口，但漏掉了
> `pkg/tool/builtin`（它反向 import 了 `internal/permission` 与
> `internal/observability`），2026-09 的架构边界清理补齐了这一处。

### 标准库优先

`pkg/` 层除明确批准的例外（`golang.org/x/image`，用于 WebP/图片处理）外不依赖第三方库。
第三方依赖仅出现在终端产品（`internal/`、`cmd/`）或可选模块中。

### 可选复杂度

通过 build tag 控制可选模块。**实际存在的 tag 如下**（历史文档中出现的 `tui`、`otel`
并不存在）：

| Tag | 默认 | 说明 |
|-----|------|------|
| `sqlite` | 关 | SQLite 会话存储、权限持久化、WAL 模式 |
| `memory` | 关 | SQLite 持久化记忆 + FTS5 全文搜索 |
| `windows` | — | 平台相关实现 |
| `example_provider` | 关 | 示例 Provider |

```bash
# 与 CI / Release 保持一致
go build -tags "sqlite memory" ./cmd/basework
```

### 向后兼容

`pkg/` API 遵循 SemVer：

| 层级 | 包 | 承诺 |
|------|----|------|
| 核心 API | `pkg/llm/`、`pkg/tool/`、`pkg/session/`、`pkg/agent/` | 主版本兼容：不删除/修改导出类型签名 |
| 扩展 API | `pkg/provider/`、`pkg/hook/`、`pkg/mcp/`、`pkg/lsp/` | 次版本兼容：可新增，不可删除 |
| 内部实现 | `internal/*` | 无兼容性承诺 |

废弃流程：`// Deprecated:` 标注 → 至少保留 2 个 Phase → 移除前记入 CHANGELOG。

---

## 预估工作量

| Phase | 预估规模 | 建议时间 |
|-------|---------|---------|
| 36.1 Provider/tool-call 稳定性 | — | 1-2 周 |
| 36.2 后台 shell/job | — | 1-2 周 |
| 36.3 编辑与 diff 工作流 | — | 1-2 周 |
| 36.4 Workspace 上下文 | — | 1 周 |
| 36.5 Backend 分层 | — | 1-2 周 |
| 36.6 TUI 产品化第一阶段 | — | 2-3 周 |
| **合计** | **约 17,500-21,500 行** | **7-12 周**（部分可并行） |

---

## 长期愿景

成为 Go 生态中最优秀的可嵌入 AI Agent 框架，同时提供生产级的终端产品体验。

- **短期**（Phase 36 P0）：稳定真实模型工具调用、后台命令与可靠编辑
- **中期**（Phase 36 P1）：形成 workspace 上下文、backend 分层与产品化 TUI
- **长期**：成为 Go AI Agent 的事实标准

---

*最后更新：2026-09-11*
