# Basework 项目状态

> 最后更新：2026-07-07

## 项目概述

**Basework** 是一个可嵌入的 Go AI Agent 框架，目标是成为功能完整的终端产品。

- **定位**：既是可嵌入的 Go 库，也是独立的终端 AI 助手
- **默认模型**：OpenCode Zen 的 `big-pickle`（免费）
- **代码规模**：~50,000 行 Go 代码
- **测试覆盖**：1,372 个测试全部通过
- **Go 版本**：1.26+

---

## 2026-07-07 审计结论与优化计划

近期审计发现：底层模块实现较完整，但终端产品主路径存在 wiring 缺口。`cmd/basework agent` 未注册基础工具，`cmd/basework tui` 未把用户输入接入 agent，配置加载在用户提供局部 JSON 时会丢失默认值。这些问题会导致 README 中承诺的“终端 AI 编程助手”能力与实际 CLI/TUI 行为不一致。

### 第一轮优化（P0，当前执行）

| 任务 | 状态 | 说明 |
|------|------|------|
| 配置默认值合并 | ✅ 完成 | `Load/Reload` 以 `defaultConfig()` 打底，再用用户 JSON 覆盖，保留默认嵌套配置 |
| 公共 agent 创建逻辑 | ✅ 完成 | `cmd/basework/runtime.go` 统一 provider/session/tools/security wiring |
| CLI 基础工具接入 | ✅ 完成 | `basework agent` 默认注册 `bash/read/write/edit/grep/glob` |
| 基础安全初始化 | ✅ 完成 | 初始化工具超时、Bash 黑名单、自定义黑名单、敏感路径保护 |
| TUI 接入 agent | ✅ 完成 | TUI 用户输入调用 `Agent.HandleMessage`，并将回复写回消息区 |
| 回归测试 | ✅ 完成 | 覆盖配置默认值合并、TUI 输入 handler |

### 后续优化路线

| 阶段 | 优先级 | 目标 |
|------|--------|------|
| 第二轮 | P0 | 接入增强工具：`web_fetch/web_search/todowrite/apply_patch/question`，修复 `web_search` nil config 和真实网络测试 |
| 第三轮 | P0 | 接入 MCP/Skill/LSP 到 CLI/TUI 主路径，补端到端验收 |
| 第四轮 | P1 | 完整权限交互与审计日志：TUI/REPL ask 流程、SQLite 权限规则、审计查询 |
| 第五轮 | P1 | CI/CD 加强：lint、跨平台 build matrix、release dry-run/tag 策略 |
| 第六轮 | P2 | README/安装文档与默认 provider 行为对齐 |

---

## 代码量对比

| 指标 | basework | crush | opencode |
|------|----------|-------|----------|
| 总代码量 | 50,051 行 | 117,230 行 | ~200,000+ 行（TypeScript） |
| 非测试代码 | 20,840 行 | 78,369 行 | ~150,000 行 |
| 测试代码 | 29,211 行 (58.4%) | 38,861 行 (33.1%) | ~50,000 行 |
| Go/TS 文件数 | 223 个 | 484 个 | ~1,000+ 个 |
| 测试文件数 | 85 个 | 163 个 | ~300+ 个 |

**关键发现**：
- basework 测试密度 58.4%，高于 crush 的 33.1%
- crush 的 `internal/ui` 一个模块（35.5K 行）接近 basework 全部非测试代码
- basework 的 `pkg/` 层（32.9K 行）是 crush 完全没有的可嵌入架构

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

## 已完成功能（Phase 1-25）

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
| `internal/oauth/` | ✅ 完成（Phase 25） | OAuth 2.0：PKCE 流程、令牌刷新、凭证安全存储 |

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

## 待完成功能（Phase 25+）

### 🔲 未来规划

| 功能 | 优先级 | 说明 |
|------|--------|------|
| 多模态支持 | P2 | 图片/音频输入处理 |
| 工作流引擎 | P2 | 多步骤任务编排与 DAG 执行 |
| 评估框架 | P3 | LLM 输出质量评估与回归测试 |
| 远程 Agent | P3 | 分布式 Agent 通信与协作 |
| 多语言支持 | P3 | Agent 回复语言自适应切换 |

---

## 优化规划（Phase 26-30）

基于与 opencode/crush 项目的对比分析，以下是最优的实施路径：

### Phase 26: 会话稳定性加固 🔲
**目标**：解决长会话稳定性、并发安全、压缩策略

| 任务 | 优先级 | 说明 |
|------|--------|------|
| Session 文件锁 | P0 | 参考 opencode flock.ts，实现文件级锁防止多进程写入冲突 |
| SQLite WAL 模式 | P0 | 启用 WAL 模式提升并发读写性能 |
| 压缩双策略 | P1 | 增加"修剪"策略（释放工具输出空间），参考 opencode compaction.ts |
| tail_turns 保护 | P1 | 可配置保留最近 N 轮对话不被压缩 |
| 工具输出截断 | P2 | 超过 2000 字符的工具输出自动截断 |
| 长会话压力测试 | P0 | 100+ 轮对话稳定性测试 |
| 并发写入测试 | P1 | 多进程/多线程并发写入安全性测试 |

**预计新增代码**：~2,000 行
**依赖**：无

### Phase 27: CI/CD + 发布流程 🔲
**目标**：建立自动化测试、构建、发布流水线

| 任务 | 优先级 | 说明 |
|------|--------|------|
| GitHub Actions 基础 | P0 | test + build + lint workflow |
| 交叉编译 | P0 | Linux/macOS/Windows 三平台 |
| goreleaser 集成 | P1 | 自动化版本发布 + 二进制打包 |
| Homebrew 发布 | P2 | `brew install basework` |
| Docker 镜像 | P2 | 可选，用于容器化部署 |
| 自动 CHANGELOG | P2 | 基于 git commit 生成 CHANGELOG |

**预计新增代码**：~500 行（workflow 配置）
**依赖**：无

### Phase 28: 安全加固 + 性能基线 🔲
**目标**：提升安全性、建立性能基准

| 任务 | 优先级 | 说明 |
|------|--------|------|
| 权限持久化 | P1 | 权限规则存储到 SQLite，跨会话保留 |
| 敏感路径保护 | P0 | 禁止访问 `.git/`, `~/.ssh/`, `~/.aws/` 等 |
| 工具执行超时 | P1 | 默认 30s 超时，可配置 |
| pprof 集成 | P2 | CPU/内存性能分析 |
| benchmark 套件 | P1 | token 计数、流式延迟、工具执行性能基准测试 |
| 安全审计清单 | P2 | 工具执行沙箱化评估 |

**预计新增代码**：~1,500 行
**依赖**：无

### Phase 29: TUI 增强 + 模板系统 🔲
**目标**：提升终端用户体验

| 任务 | 优先级 | 说明 |
|------|--------|------|
| 主题系统 | P2 | 亮/暗/自定义主题（参考 opencode theme/） |
| 命令面板 | P2 | `/` 斜杠命令 + fuzzy 搜索 |
| 键盘绑定系统 | P2 | 三层绑定（全局/应用/退出） |
| 按模型分发模板 | P1 | 根据 Provider 选择系统提示模板（参考 opencode prompt/*.txt） |
| 用户自定义模板 | P2 | `.basework/prompts/` 目录支持 |
| 环境动态注入 | P2 | 自动注入工作目录、Git 状态、平台信息 |

**预计新增代码**：~3,000 行
**依赖**：Phase 18 TUI 基础

### Phase 30: 多模态 + 插件生态 🔲
**目标**：扩展能力和生态系统

| 任务 | 优先级 | 说明 |
|------|--------|------|
| 图片输入支持 | P2 | resize + base64 编码（参考 opencode image.ts） |
| Hook 系统扩展 | P2 | PreStep, PostStep, OnToolError 等生命周期钩子 |
| 提供商插件化 | P3 | Provider 适配可通过插件扩展，无需修改核心代码 |
| TUI 插件插槽 | P3 | 允许第三方注册 UI 组件（长期） |

**预计新增代码**：~2,500 行
**依赖**：无

---

## 与参考项目对比

| 特性 | basework | opencode | crush |
|------|----------|----------|-------|
| **核心框架** | ✅ | ✅ | ✅ |
| **TUI** | ✅ Bubble Tea | ✅ OpenTUI | ✅ Bubble Tea |
| **上下文压缩** | ✅ | ✅ 多策略 | ✅ 自动摘要 |
| **重试机制** | ✅ | ✅ 指数退避 | ✅ OnRetry |
| **权限系统** | ✅ | ✅ 规则引擎 | ✅ 交互提示 |
| **子代理** | ✅ | ✅ task 工具 | ✅ agent 工具 |
| **循环检测** | ✅ | 🔲 | ✅ SHA-256 |
| **Prompt 缓存** | ✅ CacheHint + OpenAI/Gemini | ✅ CacheHint | ✅ 自动标记 |
| **结构化日志** | ✅ | ✅ + OpenTelemetry | ✅ slog |
| **成本追踪** | ✅ | ✅ | ✅ |
| **OAuth** | ✅ | ✅ | ✅ |
| **本地模型** | ✅ Ollama | 🔲 | ✅ Ollama |
| **Provider 数量** | 15+ | 10+ | 20+ |
| **内置工具数量** | 13（8 内置 + 5 增强） | 13 | 22 |
| **会话存储** | ✅ JSONL + SQLite | ✅ JSONL | ✅ SQLite |
| **文件追踪** | ✅ | ✅ | ✅ |

**对比分析**：
- basework 架构设计更优：可嵌入的 `pkg/` + `internal/` 分层、更完整的 MCP 协议（工具+资源+提示）、更多 Provider（15+ vs 20+）、LSP/Hook/Skill/OAuth 系统
- crush 工程成熟度更高：会话并发安全（文件锁+多 DB 分区）、TUI 体验（35K 行代码 vs 13K 行）、模板灵活性、真实场景验证
- opencode 最完整：事件溯源架构、27 个 CI/CD workflows、插件生态系统、多模态支持、完善的发布流程

**一句话总结**：basework 是设计更好的蓝图，crush 是打磨更久的成品。basework 需要用 Phase 26-30 补齐工程成熟度的差距。

---

## Git 提交历史

| 提交 | 说明 | 文件数 | 行数 |
|------|------|--------|------|
| `e604e70` | Phase 11-12: CLI + 测试 + 文档 | 13 | +2462 |
| `2a7bbf5` | Phase 9-10: Memory + Config + Skill | 16 | +3755 |
| `3f51760` | Phase 8: MCP 集成 | 8 | +2885 |
| `7651dcc` | Phase 7: LSP 集成 | 11 | +4601 |
| `0fe5802` | Phase 6: Provider 工厂 | 14 | +4684 |
| `26fbe0a` | Phase 4-5: Session + Agent | - | - |
| `6a2aa08` | Phase 1-3: 基础框架 | - | - |
| `4be26c8` | 初始提交 | - | - |
| `(未提交)` | Phase 13-16, 21-22, 25: 7 个 internal 模块 | - | - |
| `(未提交)` | Phase 17-20: 增强工具 + TUI + SQLite + Provider 扩展 | 10 | +6726 |

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
| `go.opentelemetry.io/otel` | OpenTelemetry 追踪 | build tag `otel` |

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
basework model set <model-id>         # 设置默认模型
basework model default                # 显示当前默认模型
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
| 20 | ✅ `model set/default`（已实现） |
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

1. **Phase 26**: 会话稳定性加固（文件锁 + WAL + 压缩双策略）
2. **Phase 27**: CI/CD + 发布流程（GitHub Actions + goreleaser）
3. **Phase 28**: 安全加固 + 性能基线（权限持久化 + pprof + benchmark）
4. **Phase 29**: TUI 增强 + 模板系统（主题 + 命令面板 + 按模型模板）
5. **Phase 30**: 多模态 + 插件生态（图片输入 + Hook 扩展 + 提供商插件化）

详见 [TASKS.md](./TASKS.md)。
