# 更新日志 (Changelog)

格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [Unreleased]

### Phase 34: 深度修复 — 协议/数据/可靠性 (2026-07-07)

**Critical 安全与协议修复**
- OAuth CSRF 防护：生成随机 state 参数，回调时验证匹配性（10 分钟过期、一次性使用）
- MCP StdioTransport 重连：`Reset()` 杀死旧进程 + 重启，重连后重新发现工具/资源/提示
- Stream 错误传播：pipeline 检查 `StreamEvent.Error`，流式错误不再静默丢弃
- 上下文取消检测：stream 循环后检查 `ctx.Err()`，返回取消错误

**Critical 数据完整性修复**
- FTS5 真全文搜索：替换 LIKE 为 FTS5 虚拟表 + BM25 相关性排序，CJK 回退 LIKE
- Memory ID 统一：FTS 索引与存储层使用相同 ID 格式
- LSP 生命周期：nil 指针防护（8 个方法）、进程泄漏清理（Kill）、readLoop panic 恢复
- 配置验证框架：`Validate()` 检查温度/token/端口/TopP 等范围，`Load`/`Reload` 后自动调用

**High Provider 错误处理**
- Gemini/Bedrock UTF-8：`strings.TrimPrefix` 替代字节切片，CJK 输出不再乱码
- OpenAI tool call：基于 `finish_reason` 判断完成，替代脆弱的空字符串检测
- HTTP 重试：429/5xx 指数退避重试（3 次），尊重 `Retry-After` header
- Copilot token 自动刷新：过期前自动通过 TokenSource 刷新

**High MCP 能力协商**
- 解析 initialize 响应中的 capabilities 字段
- 仅对声明支持的能力调用 tools/list、resources/list、prompts/list

**Medium 代码质量改进**
- todowrite 会话隔离：全局 map 改为实例注入 sessionID
- TUI 退出清理：退出时清理插件、取消 context
- JSONLStore Events 缓存优先：从缓存读取而非每次读盘
- OAuth expires_in=0 处理：使用默认 1 小时过期，防止无限刷新循环
- MCP HTTPTransport 超时：5 分钟超时防止永久挂起
- LSP fileToURI 编码：使用 `url.URL.String()` 正确 percent-encode 路径

**测试覆盖**
- 新增 OAuth CSRF 测试（5 个用例）
- 新增 MCP 重连/能力协商测试（12+ 个用例）
- 新增 FTS5 搜索测试（12 个用例）
- 新增 LSP 生命周期测试（14 个用例）
- 新增配置验证测试（33 个用例）
- 新增 HTTP 重试测试（8 个用例）

### Phase 33: 关键安全修复 + 架构解耦 (2026-07-07)

**P0 安全修复**
- `apply_patch` 路径遍历防护：净化路径防止 `../../` 穿越，集成敏感路径检查（`.git/`、`.ssh/`、`.aws/` 等）
- `web_fetch` SSRF 防护：协议白名单（仅 http/https）、DNS 解析后 IP 检查（拦截内网/回环/链路本地地址）、重定向拦截
- `JSONLStore` 并发安全：分离 cacheMu 保护 cache map，修复读锁下写 map 的数据竞态
- `EventBus` 非阻塞发布：信号量满时使用 select+default 丢弃事件，避免发布者永久阻塞

**P1 功能修复**
- `HandleMessage` 错误处理：关闭后返回 `ErrAgentClosed` 替代 `nil, nil`
- 上下文压缩激活：压缩结果写入 `EventCompacted` 事件，`ProjectMessages` 根据 `KeepFrom` 截断旧消息
- 转向系统集成：`SteeringManager` 连接到 agent loop，转向消息注入到 system prompt 之后

**P1 架构重构**
- `pkg/agent` 解耦 `internal/`：定义 `Compactor`、`LoopDetector`、`PermissionChecker`、`EventPublisher`、`SubAgentRunner` 接口
- `pkg/llm` 解耦 `internal/`：定义 `EventBus`、`TokenSource` 接口
- 新增 4 个适配器（`internal/loopdetect/adapter.go`、`permission/adapter.go`、`observability/adapter.go`、`subagent/adapter.go`）

**测试覆盖**
- 新增 apply_patch 安全测试（7 个用例）
- 新增 web_fetch SSRF 测试（协议/IP/重定向拦截）
- 新增 JSONLStore 并发压力测试（10 goroutine × 100 轮）
- 新增 EventBus 非阻塞测试
- 新增转向系统集成测试

### 即将推出

- **工作流引擎** — 多步骤任务编排与 DAG 执行
- **评估框架** — LLM 输出质量评估与回归测试
- **远程 Agent** — 分布式 Agent 通信与协作
- **多语言支持** — Agent 回复语言自适应切换

## [0.5.0] - 2026-07-06

### Phase 29-30: TUI 增强 + 模板系统 + 多模态 + 插件生态

**TUI 增强**
- 主题系统：亮/暗主题切换、自定义主题、终端自适应
- 斜杠命令面板：/触发、fuzzy 搜索、命令补全
- 键盘绑定：三层绑定模型、可配置快捷键
- 对话框系统：管理器、堆栈、模态/非模态

**模板系统**
- Provider 感知系统提示模板
- 内置 anthropic/openai/gemini/default 模板
- 用户自定义模板 + 热重载
- 环境动态注入（工作目录、Git、平台信息）

**多模态 + 插件**
- 图片输入支持（JPEG/PNG/WebP）
- Hook 系统扩展（PreStep/PostStep/OnToolError/OnCompaction）
- Provider 插件化
- TUI 插件插槽

### 新配置项

- `theme.name` — TUI 主题选择（dark/light/dracula/monokai）
- `theme.custom_path` — 自定义主题目录
- `keybindings.path` — 键盘绑定配置文件路径
- `templates.custom_dir` — 用户自定义模板目录
- `templates.default_provider` — 默认模板 Provider

### 文档

- `docs/guides/theme.md` — 主题配置指南
- `docs/guides/templates.md` — 模板系统指南

[0.5.0]: https://github.com/wly2lcl/basework/releases/tag/v0.5.0

## [0.4.0] - 2026-07-06

### 安全加固 + 性能基线（Phase 28）

#### 权限持久化

- **权限规则 SQLite 持久化** (`internal/permission/persist.go`) — 跨会话保留权限规则：
  - 规则存储到 SQLite 数据库，重启后不丢失
  - 支持 `always` 授权持久化
  - 配置项: `security.permission_store`（`sqlite` / `memory`）
- **权限审计日志** (`internal/permission/audit.go`) — 记录所有权限决策：
  - 记录时间、工具名、参数、决策结果
  - 配置保留天数（默认 30 天）
  - `basework permission audit` 查询命令
- **权限迁移工具** — `basework permission export/import` 命令：
  - 导出当前权限规则为 JSON
  - 从 JSON 文件导入权限规则

#### 敏感路径保护

- **路径检查** (`internal/permission/paths.go`) — 工具执行前路径安全检查：
  - 默认保护 `.git/`、`~/.ssh/`、`~/.aws/`、`~/.gnupg/` 等敏感路径
  - 支持白名单/黑名单配置
  - 三种保护级别：`strict`（禁止）/ `warn`（记录）/ `off`（关闭）
- **Agent 集成** (`internal/permission/hook.go`) — 通过 Hook 在工具执行前拦截：
  - 与现有权限系统无缝集成
  - 黑名单匹配时阻止执行
  - 白名单覆盖黑名单

#### 工具执行超时

- **超时控制** (`internal/permission/timeout.go`) — 工具执行超时管理：
  - 默认 30s 超时，bash 工具 60s，LSP 工具 10s
  - 可配置覆盖：`tools.timeout.default`、`tools.timeout.overrides`
  - 超时后优雅终止，发送超时事件通知
- **配置示例**:
  ```yaml
  tools:
    timeout:
      default: 30
      overrides:
        bash: 60
        read: 10
  ```

#### 性能分析

- **pprof 集成** (`internal/observability/pprof.go`) — 性能分析支持：
  - HTTP pprof 端点（默认 `127.0.0.1:6060`）
  - CLI 命令：`basework profile cpu`、`basework profile memory`、`basework profile goroutine`
  - goroutine 泄漏检测
  - 配置项：`profiling.enabled`、`profiling.host`、`profiling.port`

#### Benchmark 套件

- **性能基准** (`tests/benchmark/`) — 77 项基准测试覆盖：
  - token 计数性能基准
  - 流式响应延迟测试
  - 工具执行性能测试
  - 会话读写性能测试

### 文档

- `docs/guides/security.md` — 安全配置指南（权限持久化、敏感路径保护、审计日志）
- `docs/guides/profiling.md` — 性能分析指南（pprof、benchmark 套件）

[0.4.0]: https://github.com/wly2lcl/basework/releases/tag/v0.4.0

## [0.3.0] - 2026-07-06

### 新增

#### Phase 26: 会话稳定性加固

- **SQLite WAL 模式** (`pkg/session/sqlite.go`) — Write-Ahead Logging 提升并发读写性能 2-3x：
  - 配置项: `database.mode`（`wal` 或 `delete`，默认 `wal`）
  - 支持并发读取和写入（读不阻塞写）
- **文件锁机制** (`pkg/session/lock_unix.go`, `pkg/session/lock_windows.go`) — 操作系统级跨进程锁：
  - Unix: `syscall.Flock`，Windows: `LockFileEx`
  - 非阻塞模式 + 超时机制（默认 5 秒）
  - `basework session unlock <id>` 强制解锁命令
- **会话恢复** (`pkg/session/recovery.go`) — 损坏检测与自动恢复：
  - `PRAGMA integrity_check` 完整性检查
  - 从 WAL 文件自动恢复
  - `basework session status --check-integrity` 批量检查
- **长会话压缩** (`pkg/session/compress.go`) — Snappy/Gzip 压缩存储：
  - 超过 1000 条消息自动触发
  - 配置项: `session.compression.enabled`
  - 透明解压，对上层 API 无感
- **会话状态监控** — `basework session status` 命令：
  - 显示会话 ID、标题、消息数、创建/更新时间
  - 支持 `--check-integrity` 完整性检查

#### Phase 27: CI/CD + 发布流程

- **GitHub Actions CI/CD** (`.github/workflows/ci.yml`) — 完整自动化流程：
  - PR 触发：lint + 测试 + 覆盖率
  - Tag 触发：跨平台构建 + GitHub Release + Homebrew + Docker
- **goreleaser 跨平台构建** (`.goreleaser.yml`) — 自动化分发：
  - Linux (amd64/arm64)、macOS (amd64/arm64)、Windows (amd64)
  - 自动生成 checksum、changelog
  - Homebrew Formula 自动更新
- **Docker 镜像** (`Dockerfile`) — 容器化部署：
  - 基于 `gcr.io/distroless/static-debian11`（< 30MB）
  - 发布到 `ghcr.io/wly2lcl/basework`
  - 标签: `latest`、版本号、主版本号
- **版本信息** (`cmd/basework/version.go`) — `basework version` 命令：
  - 显示版本号、Git commit、构建时间、Go 版本、平台

### 文档

- `docs/installation.md` — 完整安装指南（Homebrew/Docker/go install/二进制）
- `docs/docker.md` — Docker 使用指南

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