# Basework 项目状态

> 最后更新：2026-09-11

> **本文件职责**：只描述**当前实际状态**（已建成什么、各模块在什么位置）。
> 未来规划见 [ROADMAP.md](../ROADMAP.md)，可执行待办见 [TASKS.md](./TASKS.md)，
> 规模与覆盖率数字见 [STATS.md](./STATS.md)（`make stats` 生成）。

## 项目概述

**Basework** 是一个可嵌入的 Go AI Agent 框架，目标是成为功能完整的终端产品。

- **定位**：既是可嵌入的 Go 库，也是独立的终端 AI 助手
- **版本**：`v0.1.2`（以 git tag 为准）
- **默认模型**：OpenCode Zen 的 `big-pickle`（免费）
- **代码规模 / 测试**：见 [STATS.md](./STATS.md)，不再在本文件手工维护
- **Go 版本**：1.26+

> 历史遗留：本文件此前手工维护的规模数字（50,051 行 / 1,372 测试）与真实值相差约 45%，
> 现已统一改为引用 `make stats` 的自动输出。

---

## 2026-07-09 核心能力差距审计

以 crush 作为终端 AI 编程工具参照，basework 当前已经具备 agent loop、provider、tool、session、MCP、LSP、memory、permission、subagent、loop detect 等底层能力，但在真实编程任务的执行体验、工具生命周期、TUI 产品化和后台服务架构上仍有明显差距。

### 核心判断

| 维度 | 当前状态 | 与 crush 的主要差距 |
|------|----------|---------------------|
| Agent 执行闭环 | ✅ 基础可用 | 复杂任务运行状态、取消/恢复、工具生命周期和错误恢复还不够成熟 |
| 工具能力 | ✅ 基础工具完整 | 后台 shell/job、流式输出、multi-edit、diff 体验、诊断/引用工具仍需增强 |
| Provider 适配 | ✅ 覆盖面较广 | 真实模型兼容性、OpenAI-compatible 流式 tool call、免费模型能力探测仍需打磨 |
| 上下文/项目理解 | ✅ 有 memory/session/LSP | 文件追踪、已读文件、workspace 级状态和跨轮恢复能力还不够厚 |
| 权限与安全 | ✅ 底层不弱 | 权限确认、工具状态展示、取消和审计的产品体验仍需整合进 TUI |
| 后台服务能力 | ⚠️ 单进程为主 | 缺少 server/backend/client/proto 分层，暂不支持多客户端和后台常驻会话 |
| TUI 产品化 | ⚠️ 基础 TUI | 工具调用展示、diff view、模型/会话/权限弹窗、补全、附件、通知等差距较大 |

### 新优先级

1. **Provider + tool call 稳定性**：保证 OpenCode/free/OpenAI-compatible 等真实模型下不损失工具能力。
2. **后台 shell/job 能力**：支持长命令、流式输出、取消、历史输出查询和工具状态生命周期。
3. **可靠编辑能力**：补齐 multi-edit、diff 预览/确认、文件追踪和编辑结果展示。
4. **Workspace 上下文**：记录读过、改过、运行过什么，增强会话恢复和项目理解。
5. **Backend/server 分层**：为多会话、多客户端、TUI 状态同步和后台任务打基础。
6. **TUI 产品化**：在核心能力稳定后补齐工具 UI、diff view、模型/会话/权限弹窗、补全和通知。

---

## 2026-07-07 审计结论与优化计划

近期审计发现：底层模块实现较完整，但终端产品主路径存在 wiring 缺口。`cmd/basework agent` 未注册基础工具，`cmd/basework tui` 未把用户输入接入 agent，配置加载在用户提供局部 JSON 时会丢失默认值。这些问题会导致 README 中承诺的“终端 AI 编程助手”能力与实际 CLI/TUI 行为不一致。

### 第一轮优化（P0，已完成）

| 任务 | 状态 | 说明 |
|------|------|------|
| 配置默认值合并 | ✅ 完成 | `Load/Reload` 以 `defaultConfig()` 打底，再用用户 JSON 覆盖，保留默认嵌套配置 |
| 公共 agent 创建逻辑 | ✅ 完成 | `cmd/basework/runtime.go` 统一 provider/session/tools/security wiring |
| CLI 基础工具接入 | ✅ 完成 | `basework agent` 默认注册 `bash/read/write/edit/grep/glob` |
| 基础安全初始化 | ✅ 完成 | 初始化工具超时、Bash 黑名单、自定义黑名单、敏感路径保护 |
| TUI 接入 agent | ✅ 完成 | TUI 用户输入调用 `Agent.HandleMessage`，并将回复写回消息区 |
| 回归测试 | ✅ 完成 | 覆盖配置默认值合并、TUI 输入 handler |

### 第二轮优化（P0，已完成）

| 任务 | 状态 | 说明 |
|------|------|------|
| 增强工具接入 | ✅ 完成 | runtime 默认注册 `web_fetch/web_search/todowrite/apply_patch/question` |
| `web_search` 稳定性 | ✅ 完成 | nil config 安全返回错误；搜索后端可注入，测试不再访问真实外网 |
| 增强工具回归测试 | ✅ 完成 | 覆盖默认后端、参数边界、nil config、API key 缺失 |

### 第三轮优化（P0，已完成）

| 任务 | 状态 | 说明 |
|------|------|------|
| MCP 主路径接入 | ✅ 完成 | 从 `mcp_configs` 初始化 manager，注册 MCP tool/resource/prompt 工具；单个 server 失败不阻断启动 |
| Skill 主路径接入 | ✅ 完成 | 扫描 workspace/user skill 目录并注入 system prompt |
| LSP 主路径接入 | ✅ 完成 | 初始化 LSP manager 并注册 6 个 LSP 工具 |
| 生命周期清理 | ✅ 完成 | 通过 agent plugin 在关闭时释放 MCP/LSP 资源 |
| Runtime 回归测试 | ✅ 完成 | 覆盖增强工具/LSP 工具注册和 Skill prompt 注入 |

### 第四轮优化（P1，已完成）

| 任务 | 状态 | 说明 |
|------|------|------|
| 权限规则闭环 | ✅ 完成 | runtime checker 接入与 `permission` 子命令同源的 SQLite 规则库 |
| 交互式权限确认 | ✅ 完成 | interactive 模式支持本次允许、始终允许、本次拒绝、始终拒绝 |
| 审计日志写入 | ✅ 完成 | runtime 权限检查会写入 SQLite audit，并随 agent 关闭刷新 |
| `ask` 规则修复 | ✅ 完成 | 持久化 `ask` 规则进入 PromptFunc，不再被误判为 deny |
| build tag 降级 | ✅ 完成 | 无 sqlite tag 时保持 memory/空操作权限存储，默认构建不受影响 |

### 第五轮优化（P1，已完成）

| 任务 | 状态 | 说明 |
|------|------|------|
| CI 分层 | ✅ 完成 | build workflow 拆分为 quality、跨平台 test、release dry-run |
| 静态检查 | ✅ 完成 | 增加全仓 gofmt、带 `sqlite memory` tag 的 vet、race test 子集 |
| 跨平台矩阵 | ✅ 完成 | Ubuntu/macOS/Windows 均运行测试和构建 |
| Release dry-run | ✅ 完成 | GoReleaser snapshot 验证发布配置但不发布 artifact/镜像 |
| 发布策略 | ✅ 完成 | tag push 正式发布；手动 dispatch 默认正式发布并创建 tag，dry-run 需显式开启 |

### 第六轮优化（P2，已完成）

| 任务 | 状态 | 说明 |
|------|------|------|
| 默认配置对齐 | ✅ 完成 | 默认 provider/model 从 `openai/gpt-4` 对齐为 `opencode/big-pickle` |
| OpenCode API Key | ✅ 完成 | CLI 优先读取 `opencode.api_key`，环境变量支持 `OPENCODE_API_KEY` 并兼容 `OG_API_KEY` |
| 初始化体验 | ✅ 完成 | `basework init` 支持 OpenCode Zen，并在无 key 输入时默认选择 OpenCode |
| 模型列表 | ✅ 完成 | `basework model list` 增加 OpenCode Zen 免费模型 |
| 文档对齐 | ✅ 完成 | README、安装、配置、CLI、Provider、FAQ 对齐默认模型、API Key 和配置 schema |

### 格式化基线（P2，已完成）

| 任务 | 状态 | 说明 |
|------|------|------|
| 全仓 gofmt 基线 | ✅ 完成 | 单独执行全仓 gofmt 纯格式化基线，并将 CI gofmt 检查升级为全仓范围 |

### 第七轮优化（P1，已完成）

| 任务 | 状态 | 说明 |
|------|------|------|
| Runtime 行为配置闭环 | ✅ 完成 | `compaction/loop_detect/observability/sub_agent` 配置接入 CLI/TUI 共享 runtime |
| 子代理主路径接入 | ✅ 完成 | 默认注册 `sub_agent` 工具；子代理使用隔离内存会话，避免递归注册自身 |
| 只读子代理工具边界 | ✅ 完成 | readonly 子代理仅暴露 `read/grep/glob` 与只读 LSP/MCP 工具 |
| 可观测性事件总线 | ✅ 完成 | `observability.enabled` 时 runtime 注入 agent event bus，并同步给内置工具超时事件 |
| Runtime 回归测试 | ✅ 完成 | 覆盖行为 option、compactor/loop detector 开关、子代理执行和 readonly 工具集 |

### 第八轮优化（P1，已完成）

| 任务 | 状态 | 说明 |
|------|------|------|
| OAuth 配置闭环 | ✅ 完成 | `pkg/config` 支持 `oauth.providers`，`auth login/status` 能读取 Provider OAuth 端点配置 |
| OAuth 存储后端语义 | ✅ 完成 | `storage_backend=keychain` 在系统密钥链未实现前明确报错，避免静默退回文件存储 |
| SubAgent Option 闭环 | ✅ 完成 | `agent.WithSubAgentRunner` 会自动注册 `sub_agent` 适配工具，且不覆盖 runtime 显式工具 |
| 本地构建入口对齐 | ✅ 完成 | `Makefile build-full/test-all/vet` 统一使用 `sqlite memory` tag，与 CI/Release 保持一致 |
| Build Tag 文档收口 | ✅ 完成 | README/FAQ/Provider/配置文档移除不存在的 `tui/otel` tag 与旧 Provider schema |

### 发布体验/文档一致性收口（P2，已完成）

| 任务 | 状态 | 说明 |
|------|------|------|
| 发布路径文档 | ✅ 完成 | 新增 `docs/release.md`，明确手动发布、tag 发布、dry-run 和本地验证步骤 |
| 安装渠道校准 | ✅ 完成 | README/安装指南移除 Homebrew 已支持表述，保留 GitHub Releases、Go install、Docker |
| Docker 标签校准 | ✅ 完成 | Docker 文档和 GoReleaser 对齐：稳定版发布完整版本 tag 和 `latest`，预发布不更新 `latest` |
| 预发布语义修复 | ✅ 完成 | GoReleaser 自动识别预发布 tag，手动 prerelease 输入会标记 GitHub Release 为预发布 |
| 路线图状态校准 | ✅ 完成 | ROADMAP 不再把已完成的 CI/CD、发布、安全和 TUI 增强列为未完成阶段 |

---

## 代码量对比

basework 自身的规模与覆盖率见 [STATS.md](./STATS.md)（`make stats` 自动生成）。
与参考项目的横向对比见 [ROADMAP.md](../ROADMAP.md#与参考项目对比)。

**结论**：basework 的测试密度显著高于参照项目，且 `pkg/` 层（可嵌入架构）是同类终端
产品不具备的结构性优势；差距在真实编程任务的执行体验与产品成熟度，而非底层覆盖度。

---

## 架构层次

```
┌─────────────────────────────────────────────────────────┐
│  cmd/basework/ — CLI 入口（cobra + 增强 REPL + TUI）     │
├─────────────────────────────────────────────────────────┤
│  internal/ — 终端产品专用逻辑                            │
│  ├── compaction/     上下文压缩                           │
│  ├── retry/          重试机制                             │
│  ├── permission/     权限系统                             │
│  ├── subagent/       子代理系统                           │
│  ├── loopdetect/     循环检测                             │
│  ├── observability/  可观测性（日志 + 成本）              │
│  ├── oauth/          OAuth 2.0 认证                       │
│  ├── tools/          增强工具（web_fetch/search/todo…）  │
│  └── tui/            终端 UI（Bubble Tea 已实现）         │
├─────────────────────────────────────────────────────────┤
│  pkg/ — 核心框架（可嵌入，稳定 API）                      │
│  ├── agent/        Agent 循环                            │
│  ├── llm/          类型系统 + 错误分类                   │
│  ├── provider/     Provider 工厂（15+ 个 Provider）      │
│  ├── tool/         工具系统（6 个内置工具）              │
│  ├── session/      会话管理（JSONL + SQLite + 队列）     │
│  ├── hook/         钩子系统                              │
│  ├── lsp/          LSP 集成                              │
│  ├── mcp/          MCP 集成                              │
│  ├── memory/       记忆系统（FTS5）                      │
│  ├── config/       配置管理（CoW）                       │
│  └── skill/        技能加载                              │
└─────────────────────────────────────────────────────────┘
```

---

## 已完成功能（Phase 1-35）

> Phase 与模块的对应关系、以及每个 Phase 的判定依据，见 [ROADMAP.md](../ROADMAP.md)。

### ✅ 核心框架

| 模块 | 状态 | 说明 |
|------|------|------|
| `pkg/llm/` | ✅ 完成 | 统一类型系统、错误分类、能力检测 |
| `pkg/tool/` | ✅ 完成 | 工具接口、注册表、6 个内置工具 |
| `pkg/hook/` | ✅ 完成 | Hook 注册、PreToolUse/PostToolUse |
| `pkg/session/` | ✅ 完成 | JSONL 存储、事件溯源、投影 |
| `pkg/agent/` | ✅ 完成 | Agent 循环、流式处理、工具调用 |
| `pkg/provider/` | ✅ 完成 | 工厂模式、10 个 Provider |
| `pkg/lsp/` | ✅ 完成 | LSP 客户端、自动发现、6 个工具 |
| `pkg/mcp/` | ✅ 完成 | MCP 客户端、stdio/HTTP 传输、工具注入 |
| `pkg/mcp/resource.go` | ✅ 完成（Phase 24） | MCP 资源协议：resources/list、resources/read |
| `pkg/mcp/prompt.go` | ✅ 完成（Phase 24） | MCP 提示协议：prompts/list、prompts/get |
| `pkg/mcp/config_expand.go` | ✅ 完成（Phase 24） | Shell 变量展开：$VAR 和 ${VAR} 语法支持 |
| `pkg/mcp/reconnect.go` | ✅ 完成（Phase 24） | 自动重连：指数退避、状态机、最大重试配置 |
| `pkg/memory/` | ✅ 完成 | 4 层文件映射、FTS5 检索（build tag） |
| `pkg/config/` | ✅ 完成 | JSON 配置、CoW 模式、热重载 |
| `pkg/skill/` | ✅ 完成 | 技能加载、同名去重 |

### ✅ 终端产品基础

| 模块 | 状态 | 说明 |
|------|------|------|
| `cmd/basework/` | ✅ 完成 | CLI 入口、cobra 子命令 |
| `tests/` | ✅ 完成 | 11 个集成测试、竞态检测通过 |
| `docs/guides/` | ✅ 完成 | 嵌入指南、配置参考、扩展指南 |

### ✅ Provider 支持（15+）

| Provider | 状态 | 协议 |
|----------|------|------|
| OpenAI | ✅ | openai-chat |
| Anthropic | ✅ | anthropic-messages |
| Gemini | ✅ | gemini |
| DeepSeek | ✅ | openai-compat |
| Groq | ✅ | openai-compat |
| Together | ✅ | openai-compat |
| OpenRouter | ✅ | openai-compat |
| xAI | ✅ | openai-compat |
| Mistral | ✅ | openai-compat |
| openai-compat | ✅ | 通用兼容 |
| OpenCode Zen | ✅（Phase 20） | openai-compat，免费模型 big-pickle |
| Amazon Bedrock | ✅（Phase 20） | AWS Converse API，SigV4 签名 |
| Azure OpenAI | ✅（Phase 20） | OpenAI 兼容，api-key 认证 |
| GitHub Copilot | ✅（Phase 20） | OAuth 设备授权，openai-compat |
| Ollama | ✅（Phase 20） | 本地模型，自动发现 /api/tags |

### ✅ 内置工具 + 增强工具

| 工具 | 状态 | 说明 |
|------|------|------|
| `bash` | ✅ | Shell 命令执行 |
| `read` | ✅ | 文件读取 |
| `write` | ✅ | 文件写入 |
| `edit` | ✅ | 精确字符串替换 |
| `glob` | ✅ | 文件模式匹配 |
| `grep` | ✅ | 正则内容搜索 |
| `lsp_*` | ✅ | LSP 诊断/引用/重启（6 个工具） |
| `web_fetch` | ✅（Phase 17） | URL 获取、HTML 转 text/markdown |
| `web_search` | ✅（Phase 17） | 网络搜索（Tavily/Exa 后端） |
| `todowrite` | ✅（Phase 17） | 任务列表管理 |
| `apply_patch` | ✅（Phase 17） | 结构化补丁应用（Add/Delete/Update） |
| `question` | ✅（Phase 17） | 向用户提问并获取选择 |

### ✅ 生产韧性模块

| 模块 | 状态 | 说明 |
|------|------|------|
| `internal/compaction/` | ✅ 完成（Phase 13） | 上下文压缩：自动摘要、滑动窗口、选择性保留 |
| `internal/retry/` | ✅ 完成（Phase 14） | 重试机制：指数退避、错误分类、Retry-After 支持 |
| `internal/loopdetect/` | ✅ 完成（Phase 22） | 循环检测：SHA-256 签名追踪、模式匹配、工具融合 |
| `pkg/provider/cache.go` | ✅ 完成（Phase 23） | Prompt 缓存：Anthropic/OpenAI/Gemini cache_control 自动注入 |

### ✅ 安全与权限

| 模块 | 状态 | 说明 |
|------|------|------|
| `internal/permission/` | ✅ 完成（Phase 15） | 权限系统：规则引擎、交互提示、YOLO 模式 |
| `pkg/tool/builtin/blacklist.go` | ✅ 完成（Phase 23） | 命令黑名单：12+ 内置危险模式、自定义扩展、权限集成 |
| `internal/oauth/` | ✅ 完成（Phase 25） | OAuth 2.0：PKCE 流程、令牌刷新、加密文件凭证存储 |

### ✅ 子代理系统

| 模块 | 状态 | 说明 |
|------|------|------|
| `internal/subagent/` | ✅ 完成（Phase 16） | 任务委托、隔离子会话、成本传播、只读代理 |

### ✅ 增强工具系统

| 模块 | 状态 | 说明 |
|------|------|------|
| `internal/tools/` | ✅ 完成（Phase 17） | 5 个增强工具：web_fetch、web_search、todowrite、apply_patch、question |

### ✅ 终端 UI

| 模块 | 状态 | 说明 |
|------|------|------|
| `internal/tui/` | ✅ 完成（Phase 18） | Bubble Tea TUI 主应用 |
| `internal/tui/input.go` | ✅ 完成 | 输入区：多行输入、历史导航、Tab 补全 |
| `internal/tui/streaming.go` | ✅ 完成 | 流式输出：逐字显示、spinner 动画、工具进度 |
| `internal/tui/statusbar.go` | ✅ 完成 | 状态栏：模型名、Provider、token 用量、MCP 状态 |
| `internal/tui/message.go` | ✅ 完成 | 消息渲染：Glamour Markdown、语法高亮 |
| `cmd/basework tui` | ✅ 完成 | `basework tui` 命令启动 TUI |

### ✅ 会话增强

| 模块 | 状态 | 说明 |
|------|------|------|
| `pkg/session/sqlite.go` | ✅ 完成（Phase 19） | SQLite 会话存储（build tag: sqlite） |
| `pkg/session/filetrack.go` | ✅ 完成 | FileTracker 追踪读写编辑的文件 |
| `pkg/session/queue.go` | ✅ 完成 | 线程安全 FIFO 消息队列，支持取消 |
| `pkg/config` AutoTitle | ✅ 完成 | 自动标题生成配置 |
| `pkg/config` QueueConfig | ✅ 完成 | 会话队列配置 |

### ✅ 可观测性

| 模块 | 状态 | 说明 |
|------|------|------|
| `internal/observability/` | ✅ 完成（Phase 21） | 结构化日志、Token 计数、成本追踪 |

---

## 待完成 / 规划

本文件不再维护规划内容。原先此处的「待完成功能（Phase 25+）」与「优化规划（Phase 26-30）」
与代码实际状态互相矛盾（把已完成的 Phase 26-30 标为 🔲），已移除，统一收敛到：

- **阶段与未来规划** → [ROADMAP.md](../ROADMAP.md)（含 Phase 36 拆解、依赖顺序、未排期方向）
- **可执行待办与验收标准** → [TASKS.md](./TASKS.md)

---

## 与参考项目对比

横向对比表（basework / crush / opencode）与结论已统一到
[ROADMAP.md](../ROADMAP.md#与参考项目对比)，避免两处各有一份而口径漂移。

---

## Git 提交历史

以 `git log` / [CHANGELOG.md](../CHANGELOG.md) 为准。本文件不再手工维护提交清单 ——
此前版本存在错误标注（把已发布的 Phase 13-20 记为「未提交」）。

---

## 迁移策略

### 会话存储：JSONL → SQLite

| 阶段 | 说明 |
|------|------|
| Phase 19 前 | 保持 JSONL 为默认后端 |
| **Phase 19** | ✅ **已完成**: 新增 SQLiteStore（build tag `sqlite`） |
| 迁移工具 | `basework migrate sessions` 命令：扫描 JSONL 文件，逐行解析事件，写入 SQLite |
| 兼容期 | 两种 Store 共存，通过 `session.store` 配置切换 |
| 最终 | JSONL 降级为可选后端，SQLite 为默认 |

**迁移原则**：
- 不删除旧数据，保留 JSONL 文件作为备份
- 迁移失败时回滚，不产生部分写入
- 支持增量迁移（仅导入上次迁移后的新事件）

### Agent API 兼容性

| 策略 | 说明 |
|------|------|
| 接口不变 | `pkg/agent.Agent` 接口签名不变 |
| 新增可选参数 | 通过 `agent.Option` 模式传入新功能（compaction, retry 等） |
| Hook 扩展 | 新 Hook 点（PreStep, PostStep）通过新接口 `ExtendedHook` 暴露 |
| 废弃标记 | 旧 API 用 `// Deprecated:` 注释标记，至少保留 2 个 Phase 周期 |

---

## 新增依赖清单

### Phase 13-17 + 21-22 + 25（核心功能 — 已完成）

| 依赖 | 用途 | Phase | 大小 |
|------|------|-------|------|
| 无新增 | 全部使用标准库 | 13-17, 21-22, 25 | - |

### Phase 18（TUI）

| 依赖 | 用途 | 大小 |
|------|------|------|
| `charm.land/bubbletea/v2` | TUI 框架 | ~2MB |
| `charm.land/lipgloss/v2` | 样式系统 | ~500KB |
| `charm.land/glamour` | Markdown 渲染 | ~1MB |

### Phase 20（Provider 扩展）

| 依赖 | 用途 | 大小 |
|------|------|------|
| `github.com/aws/aws-sdk-go-v2` | Bedrock 认证 | ~15MB |
| `github.com/Azure/azure-sdk-for-go` | Azure 认证 | ~20MB |

### 可选依赖

| 依赖 | 用途 | 条件 |
|------|------|------|
| `github.com/alecthomas/chroma/v2` | 语法高亮 | TUI 模式启用时 |

### 依赖增长估算

| 阶段 | 新增依赖 | 二进制增量 |
|------|----------|-----------|
| Phase 1-12（已完成） | cobra, modernc.org/sqlite | ~30MB |
| Phase 13-17 + 21-22 + 25（已完成） | 无 | ~0MB |
| Phase 18 | Bubble Tea 生态 | ~3.5MB |
| Phase 20 | AWS + Azure SDK | ~35MB |
| **总计** | - | **~68MB** |

---

## 配置演进

### 新增配置项

```jsonc
{
  // Phase 13: 上下文压缩
  "compaction": {
    "enabled": true,
    "threshold": 0.8,          // 窗口使用率超过 80% 时触发
    "preserve_recent": 2,      // 保留最近 2 轮对话
    "strategy": "auto",        // "auto" | "truncate" | "summarize"
    "model": ""                // 摘要用小模型（空则用主模型）
  },

  // Phase 14: 重试机制
  "retry": {
    "max_attempts": 5,
    "initial_delay": "2s",
    "max_delay": "30s",
    "backoff_factor": 2
  },

  // Phase 15: 权限系统
  "permission": {
    "mode": "interactive",     // "interactive" | "yolo" | "deny-all"
    "rules": [
      {"action": "bash", "resource": "*", "effect": "ask"},
      {"action": "write", "resource": "*.go", "effect": "allow"}
    ],
    "blocked_commands": ["rm -rf /", "mkfs", "dd if=/dev/"]
  },

  // Phase 18: 终端 UI
  "tui": {
    "enabled": true,
    "theme": "dark",           // "dark" | "light" | "dracula" | "monokai"
    "markdown": true,
    "syntax_highlight": true,
    "compact_mode": false,
    "diff_mode": "unified"     // "unified" | "split"
  },

  // Phase 19: 会话增强
  "session": {
    "store": "sqlite",         // "jsonl" | "sqlite" | "memory"
    "auto_title": true,
    "title_model": ""          // 标题生成用小模型
  },

  // Phase 20: 默认模型
  "model": {
    "default": "opencode/big-pickle",
    "small": "opencode/deepseek-v4-flash-free"  // 用于摘要/标题
  },

  // Phase 21: 可观测性
  "observability": {
    "log_level": "info",       // "debug" | "info" | "warn" | "error"
    "log_file": "~/.basework/logs/basework.log",
    "cost_tracking": true
  },

  // Phase 22: 循环检测
  "loop_detect": {
    "enabled": true,
    "window_size": 10,
    "threshold": 5
  },

  // Phase 23: Prompt 缓存
  "cache": {
    "enabled": true,
    "policy": "auto"           // "auto" | "none" | "explicit"
  }
}
```

### 配置发现优先级（不变）

```
命令行标志 > 环境变量 > .basework/config.json > ~/.config/basework/config.json
```

### 配置迁移

- 旧配置自动识别，新增字段使用默认值
- 无破坏性变更，旧配置文件继续可用
- `basework config validate` 命令检查配置合法性

---

## CLI 命令路线图

### 现有命令

```
basework agent        # 启动 Agent（简单 REPL）
basework init         # 初始化配置
basework model list   # 列出模型
basework model list --free            # 只列出免费模型
basework model use <model-id>         # 切换当前模型
basework session list # 列出会话
basework session clear # 清除会话
basework permission list              # 列出权限规则
basework permission add <rule>        # 添加规则
basework permission remove <rule>     # 删除规则
basework auth login <provider>        # OAuth 登录
basework auth logout <provider>       # 登出
basework auth status                  # 查看认证状态
basework config validate              # 验证配置文件
basework config show                  # 显示当前配置
basework logs [--tail N] [--follow]   # 查看日志
```

### 新增命令（已实现）

```
basework tui                          # 启动 TUI 模式
basework session resume <id>          # 恢复指定会话
basework session export <id>          # 导出会话（markdown/json）
basework session search <query>       # 搜索会话内容
basework migrate sessions             # JSONL → SQLite 迁移
```

### 命令演进路线

| Phase | 新增命令 |
|-------|---------|
| 15 | ✅ `permission list/add/remove`（已实现） |
| 18 | ✅ `tui`（已实现） |
| 19 | ✅ `session resume/export/search`, `migrate`（已实现） |
| 20 | ✅ `model list --free`, `model use`（已实现） |
| 25 | ✅ `auth login/logout/status`（已实现） |

---

## API 兼容性策略

### 分层保证

| 层 | 包 | 兼容性承诺 |
|----|-----|-----------|
| 核心 API | `pkg/llm/`, `pkg/tool/`, `pkg/session/`, `pkg/agent/` | **SemVer 严格兼容**：不删除/修改导出类型签名 |
| 扩展 API | `pkg/provider/`, `pkg/hook/`, `pkg/mcp/`, `pkg/lsp/` | **SemVer 次版本兼容**：可新增，不可删除 |
| 内部实现 | `internal/*` | **无兼容性承诺**：随时可重构 |

### 扩展原则

```go
// ✅ 正确：通过 Option 模式扩展
func WithCompaction(cfg CompactionConfig) Option { ... }

// ❌ 错误：修改接口签名
type Agent interface {
    Run(ctx, req) Response        // 不可改
    RunWithCompaction(ctx, req, cfg) Response  // 不可加
}
```

### 废弃策略

1. `// Deprecated: 使用 Xxx 替代。将在 Phase N+2 移除。`
2. 至少保留 2 个 Phase 的开发周期
3. 移除前在 CHANGELOG.md 中记录

---

## 测试策略

### 测试分层

| 层级 | 位置 | 覆盖范围 | 工具 |
|------|------|----------|------|
| 单元测试 | `*_test.go`（每个包内） | 函数/方法级别 | `testing` |
| 集成测试 | `tests/` | 跨包交互 | `testing` + mock |
| E2E 测试 | `tests/e2e/` | 完整流程 | `testing` + 真实 API |
| 压力测试 | `tests/stress/` | 并发/性能 | `testing` + `-race` |

### 新增功能的测试要求

| Phase | 测试类型 | 覆盖率目标 |
|-------|---------|-----------|
| 13 压缩 | ✅ 已完成 | 单元 + 集成 | > 80% |
| 14 重试 | ✅ 已完成 | 单元 + 集成 | > 80% |
| 15 权限 | ✅ 已完成 | 单元 + 集成 | > 90%（安全关键） |
| 16 子代理 | ✅ 已完成 | 单元 + 集成 | > 70% |
| 17 工具 | ✅ 已完成 | 单元 + 集成 | > 80% |
| 18 TUI | ✅ 已完成 | 单元（渲染逻辑） | > 60% |
| 19 会话增强 | ✅ 已完成 | 单元 + 集成 | > 70% |
| 20 Provider 扩展 | ✅ 已完成 | 单元 + 集成 | > 70% |
| 23-24 Prompt 缓存 + MCP 增强 | ✅ 已完成 | 集成测试 | > 70% |

### Mock 策略

```go
// mockModel — 模拟 LLM 响应
type mockModel struct {
    responses []llm.Response
    calls     int
}

// mockTool — 模拟工具调用
type mockTool struct {
    name    string
    handler func(args json.RawMessage) (tool.Result, error)
}

// mockStore — 模拟会话存储
type mockStore struct {
    events []session.Event
}
```

### 测试命令

```bash
# 全量测试
go test ./... -count=1 -timeout=180s

# 竞态检测
go test ./... -race -count=1

# 覆盖率
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out

# 仅运行某 Phase 的测试
go test ./internal/compaction/... -v
go test ./internal/retry/... -v
```

---

## 下一步

1. **Phase 36.1**: Provider + tool call 稳定性，优先保证真实模型不损失工具能力
2. **Phase 36.2**: 后台 shell/job 管理，支持长命令、流式输出、取消和历史输出
3. **Phase 36.3**: 可靠编辑与 diff 工作流，补齐 multi-edit、写入确认和 touched files
4. **Phase 36.4**: Workspace 上下文，记录读过、改过、搜索过和运行过什么
5. **Phase 36.5-36.6**: backend/server/client 最小分层与 TUI 产品化第一阶段

详见 [TASKS.md](./TASKS.md)。
