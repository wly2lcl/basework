# Basework 路线图

> 从可嵌入框架到独立终端产品的完整演进路线。
>
> 详细任务清单见 [docs/TASKS.md](docs/TASKS.md)。

---

## 已完成

### Phase 1-12: 核心框架（2025）

| Phase | 模块 | 说明 |
|-------|------|------|
| Phase 1 | `pkg/llm/` | 统一类型系统、错误分类、能力检测 |
| Phase 2 | `pkg/hook/` | Hook 注册、PreToolUse/PostToolUse、PubSub 事件总线 |
| Phase 3 | `pkg/session/` | JSONL 存储、事件溯源、投影 |
| Phase 4 | `pkg/tool/` | 工具接口、注册表、8 个内置工具 |
| Phase 5 | `pkg/agent/` | Agent 循环、流式处理、工具调用 |
| Phase 6 | `pkg/provider/` | 工厂模式、10 个 Provider |
| Phase 7 | `pkg/lsp/` | LSP 客户端、自动发现、6 个工具 |
| Phase 8 | `pkg/mcp/` | MCP 客户端、stdio/HTTP 传输、工具注入 |
| Phase 9 | `pkg/memory/` | 4 层文件映射、FTS5 检索（build tag） |
| Phase 10 | `pkg/config/` + `pkg/skill/` | JSON 配置（CoW 模式）、技能加载 |
| Phase 11 | `cmd/basework/` | CLI 入口、cobra 子命令 |
| Phase 12 | 集成测试 + 文档 | 嵌入指南、配置参考、扩展指南 |

**统计数据**：52 个任务，~4800 行核心代码，524 个测试通过。

### Provider 支持

| Provider | 协议 |
|----------|------|
| OpenAI | openai-chat |
| Anthropic | anthropic-messages |
| Gemini | gemini |
| DeepSeek | openai-compat |
| Groq | openai-compat |
| Together | openai-compat |
| OpenRouter | openai-compat |
| xAI | openai-compat |
| Mistral | openai-compat |
| openai-compat | 通用兼容 |

### 内置工具

| 工具 | 说明 |
|------|------|
| `bash` | Shell 命令执行 |
| `read` | 文件读取 |
| `write` | 文件写入 |
| `edit` | 精确字符串替换 |
| `glob` | 文件模式匹配 |
| `grep` | 正则内容搜索 |
| `lsp_*` | LSP 诊断/引用/重启（6 个工具） |

---

## 当前规划

### Phase 13-17: 生产韧性 + 工具增强（P0，2-3 周）

| Phase | 模块 | 优先级 | 说明 |
|-------|------|--------|------|
| 13 | `internal/compaction/` | P0 | 上下文压缩：自动摘要、工具输出修剪、保留最近轮次 |
| 14 | `internal/retry/` | P0 | 重试机制：指数退避、retry-after 解析、速率限制检测 |
| 15 | `internal/permission/` | P0 | 权限系统：规则引擎（allow/deny）、交互提示、YOLO 模式 |
| 16 | `internal/subagent/` | P0 | 子代理：`task` 工具、隔离子会话、成本传播 |
| 17 | `internal/tools/` | P0 | 增强工具：`web_fetch`、`web_search`、`todowrite` |

Phase 13-17 可并行开发，无新增第三方依赖（全部使用标准库）。

### Phase 18: 终端 UI（P0，2-3 周）

| 功能 | 说明 |
|------|------|
| TUI 框架 | Bubble Tea 基础界面 |
| Markdown 渲染 | Glamour 渲染响应 |
| 语法高亮 | Chroma 代码高亮 |
| Diff 视图 | 统一/分屏模式 |
| 会话选择器 | 浏览/切换会话 |
| 模型选择器 | 模型切换对话框 |

依赖 Bubble Tea 生态（~3.5MB），依赖 Phase 13-17 完成。

### Phase 19-25: 会话/Provider/可观测性增强（2-3 周）

| Phase | 模块 | 优先级 | 说明 |
|-------|------|--------|------|
| 19 | `pkg/session/` SQLite 增强 | P0 | SQLite 后端、自动标题、会话队列、文件追踪 |
| 20 | `pkg/provider/` 扩展 | P0 | OpenCode Zen、Amazon Bedrock、Azure OpenAI |
| 21 | `internal/observability/` | P0 | 结构化日志、成本追踪、OpenTelemetry |
| 22 | `internal/loopdetect/` | P1 | SHA-256 签名追踪、10 步超 5 次自动中断 |
| 23 | `internal/cache/` | P1 | Prompt 缓存（Anthropic CacheHint 自动注入） |
| 24 | `pkg/mcp/` 增强 | P1 | 资源支持、提示支持、自动重连 |
| 25 | `internal/oauth/` | P2 | PKCE 流程、令牌刷新、凭证存储 |

---

## 未来方向（未规划）

以下功能已识别但未排入具体路线图，根据社区反馈和需求优先级择机实施：

- **多模态支持增强** — 图片/音频输入处理
- **代码索引** — codebase 搜索与语义理解
- **Web UI / API 服务** — HTTP 服务模式
- **企业级功能** — SSO、审计日志、RBAC
- **工作流引擎** — 多步骤任务编排与 DAG 执行
- **评估框架** — LLM 输出质量评估与回归测试
- **远程 Agent** — 分布式 Agent 通信与协作

---

## 设计原则

### 可嵌入优先

框架核心可独立于终端产品使用，`agent.New(WithModel(...), WithTools(...))` 即可嵌入任何 Go 应用。

### 标准库优先

`pkg/` 层核心不依赖第三方库，确保长期可维护性。第三方依赖仅在终端产品（`internal/`、`cmd/`）或可选模块（`pkg/memory/`）中使用。

### 可选复杂度

通过 build tag 控制可选模块：

| Tag | 默认 | 说明 |
|-----|------|------|
| `memory` | 关 | SQLite 持久化记忆 + FTS5 全文搜索 |
| `tui` | 关 | Bubble Tea 终端 UI（Phase 18） |

```bash
# 启用可选模块构建
go build -tags memory ./...
go build -tags "memory tui" ./...
```

### 向后兼容

`pkg/` API 严格遵循 SemVer：

- 核心 API（`pkg/llm/`, `pkg/tool/`, `pkg/session/`, `pkg/agent/`）— 主版本兼容
- 扩展 API（`pkg/provider/`, `pkg/hook/`, `pkg/mcp/`, `pkg/lsp/`）— 次版本兼容
- 内部实现（`internal/*`）— 无兼容性承诺

---

## 依赖增长估算

| 阶段 | 新增依赖 | 二进制增量 |
|------|----------|-----------|
| Phase 1-12（已完成） | cobra, modernc.org/sqlite | ~30MB |
| Phase 13-17 | 无 | ~0MB |
| Phase 18 | Bubble Tea 生态 | ~3.5MB |
| Phase 20 | AWS + Azure SDK | ~35MB |
| **总计** | - | **~68MB** |