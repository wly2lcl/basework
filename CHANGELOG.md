# 更新日志 (Changelog)

格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [Unreleased]

### 即将推出（Phase 25+ 规划）

- **多模态支持** — 图片/音频输入处理
- **工作流引擎** — 多步骤任务编排与 DAG 执行
- **评估框架** — LLM 输出质量评估与回归测试
- **远程 Agent** — 分布式 Agent 通信与协作
- **多语言支持** — Agent 回复语言自适应切换

## [0.2.0] - 2026-07-04

### 新增

#### Phase 23: Prompt 缓存 + 命令黑名单

- **Prompt 缓存** (`pkg/provider/cache.go`) — Anthropic/OpenAI/Gemini 自动注入 `cache_control` 标记：
  - Anthropic: system 消息转为带 `cache_control` 的对象数组，第一条 user 消息前 2 个 text block 标记
  - OpenAI: 第一条 user 消息前 2 个 text parts 添加 `cache_control` 标记
  - Gemini: 前 2 个 user contents 添加缓存标记
  - 配置项: `prompt_cache.enabled`（默认 true）
- **命令黑名单** (`pkg/tool/builtin/blacklist.go`) — 12+ 内置危险命令模式：
  - 支持 `rm -rf /`、`mkfs`、`dd if=/dev/`、fork 炸弹、管道下载执行等
  - 用户自定义扩展（`config.yaml` 的 `blocked_commands`）
  - 权限集成：default（拒绝）/ interactive（确认）/ yolo（跳过）

#### Phase 24: MCP 增强

- **MCP 资源支持** (`pkg/mcp/resource.go`) — 实现 `resources/list`、`resources/read` 协议
  - 暴露为 `mcp_read` 工具，支持资源 URI 自动路由
  - 大小限制（默认 10MB），超大资源自动截断标记
- **MCP 提示支持** (`pkg/mcp/prompt.go`) — 实现 `prompts/list`、`prompts/get` 协议
  - 暴露为 `mcp_prompt` 工具，支持 prompt 名称自动查找
  - 大小限制与截断保护
- **MCP 自动重连** (`pkg/mcp/reconnect.go`) — 指数退避重连（1s, 2s, 4s, 8s...）
  - 状态机：available → reconnecting → available / unavailable
  - 最大重试次数可配置（默认 3 次）
  - 隔离性：一个服务器 unavailable 不影响其他
- **MCP 变量展开** (`pkg/mcp/config_expand.go`) — Shell 变量展开支持
  - 支持 `$VAR` 和 `${VAR}` 两种语法
  - 展开字段：`command`、`args`、`env`
  - 启动时一次性展开，运行时零开销
- **集成测试** (`tests/`) — 5 个新集成测试文件（28 个测试用例）：
  - `prompt_cache_integration_test.go` — 缓存标记 + 命中统计（6 个测试）
  - `command_blacklist_integration_test.go` — 黑名单拦截 + 权限绕过（8 个测试）
  - `mcp_resources_integration_test.go` — 资源读取 + 大小限制（6 个测试）
  - `mcp_prompts_integration_test.go` — 提示获取 + Context 取消（7 个测试）
  - `mcp_resilience_integration_test.go` — 自动重连 + 变量展开（10 个测试）

[0.2.0]: https://github.com/wly2lcl/basework/releases/tag/v0.2.0

## [0.1.0] - 2025-07-03

### 新增

#### Phase 1-3: 基础框架 (6a2aa08)

- **统一类型系统** (`pkg/llm/`) — LLM 请求/响应的类型定义与错误类型
- **工具接口** (`pkg/tool/`) — 工具接口定义 + 8 个内置工具实现
- **事件总线** (`pkg/hook/`) — PubSub 模式的事件发布订阅系统

#### Phase 4-5: Session + Agent (26fbe0a)

- **会话管理** (`pkg/session/`) — 事件溯源架构，JSONL 持久化存储
- **Agent 循环** (`pkg/agent/`) — 流式处理、工具调用循环、消息路由
- **Hook 系统增强** (`pkg/hook/`) — 完整的 PubSub 事件总线

#### Phase 6: Provider 工厂 (0fe5802)

- **Provider 工厂模式** (`pkg/provider/`) — 统一接口 + 工厂注册机制
- **10 个 LLM Provider** — OpenAI、Anthropic、Gemini 原生支持 + 7 个 OpenAI 兼容 Provider
- **错误类型** (`pkg/llm/error.go`) — Provider 错误分类与处理

#### Phase 7: LSP 集成 (7651dcc)

- **LSP 客户端** (`pkg/lsp/`) — 语言服务器协议客户端实现
- **自动发现** — 按项目类型自动发现并启动 LSP 服务
- **6 个 LSP 工具** — 代码补全、诊断、跳转定义、查找引用、悬停信息、文档符号
- **JSON-RPC over stdio** — 自实现的 JSON-RPC 通信层

#### Phase 8: MCP 集成 (3f51760)

- **MCP 客户端** (`pkg/mcp/`) — Model Context Protocol 客户端实现
- **双传输模式** — stdio 和 HTTP 传输支持
- **工具注入** — 将 MCP 服务端工具动态注入 Agent 工具链
- **自实现 JSON-RPC 层** — 不依赖外部 JSON-RPC 库

#### Phase 9-10: Memory + Config + Skill (2a7bbf5)

- **四层文件映射** (`pkg/memory/`) — ephemeral/short-term/long-term/file 四层记忆存储
- **FTS5 全文检索** — 基于 SQLite FTS5 的语义检索（build tag: memory）
- **JSON 配置** (`pkg/config/`) — 支持热重载、Copy-on-Write 模式的配置管理
- **技能加载** (`pkg/skill/`) — 技能文件自动发现与同名去重

#### Phase 11-12: CLI + 集成测试 + 文档 (e604e70)

- **CLI 参考实现** (`cmd/basework/`) — 基于 Cobra v1.10.2，支持以下子命令：
  - `start` — 启动 Agent REPL
  - `agent` — Agent 管理
  - `model` — 模型配置
  - `session` — 会话管理
  - `init` — 初始化项目
- **11 个集成测试** (`tests/integration_test.go`) — 覆盖核心工作流
- **3 份开发者指南** (`docs/guides/`) — Embedder 指南、配置指南、扩展指南
- **新增依赖**: cobra v1.10.2, modernc.org/sqlite

[0.1.0]: https://github.com/wly2lcl/basework/releases/tag/v0.1.0