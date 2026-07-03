# Basework 项目状态

> 最后更新：2025-01-03

## 项目概述

**Basework** 是一个可嵌入的 Go AI Agent 框架，目标是成为功能完整的终端产品。

- **定位**：既是可嵌入的 Go 库，也是独立的终端 AI 助手
- **默认模型**：OpenCode Zen 的 `big-pickle`（免费）
- **代码规模**：~26,000 行 Go 代码
- **测试覆盖**：524 个测试全部通过
- **Go 版本**：1.26+

---

## 架构层次

```
┌─────────────────────────────────────────────────────────┐
│  cmd/basework/ — CLI 入口（cobra + 增强 REPL/TUI）       │
├─────────────────────────────────────────────────────────┤
│  internal/ — 终端产品专用逻辑                            │
│  ├── tui/          终端 UI（Bubble Tea）                 │
│  ├── compaction/   上下文压缩                            │
│  ├── retry/        重试机制                              │
│  ├── permission/   权限系统                              │
│  ├── loopdetect/   循环检测                              │
│  ├── costtrack/    成本追踪                              │
│  └── observability/ 可观测性                             │
├─────────────────────────────────────────────────────────┤
│  pkg/ — 核心框架（可嵌入，稳定 API）                      │
│  ├── agent/        Agent 循环                            │
│  ├── llm/          类型系统 + 错误分类                   │
│  ├── provider/     Provider 工厂（10 个 Provider）       │
│  ├── tool/         工具系统（8 个内置工具）              │
│  ├── session/      会话管理                              │
│  ├── hook/         钩子系统                              │
│  ├── lsp/          LSP 集成                              │
│  ├── mcp/          MCP 集成                              │
│  ├── memory/       记忆系统（FTS5）                      │
│  ├── config/       配置管理（CoW）                       │
│  └── skill/        技能加载                              │
└─────────────────────────────────────────────────────────┘
```

---

## 已完成功能（Phase 1-12）

### ✅ 核心框架

| 模块 | 状态 | 说明 |
|------|------|------|
| `pkg/llm/` | ✅ 完成 | 统一类型系统、错误分类、能力检测 |
| `pkg/tool/` | ✅ 完成 | 工具接口、注册表、8 个内置工具 |
| `pkg/hook/` | ✅ 完成 | Hook 注册、PreToolUse/PostToolUse |
| `pkg/session/` | ✅ 完成 | JSONL 存储、事件溯源、投影 |
| `pkg/agent/` | ✅ 完成 | Agent 循环、流式处理、工具调用 |
| `pkg/provider/` | ✅ 完成 | 工厂模式、10 个 Provider |
| `pkg/lsp/` | ✅ 完成 | LSP 客户端、自动发现、6 个工具 |
| `pkg/mcp/` | ✅ 完成 | MCP 客户端、stdio/HTTP 传输、工具注入 |
| `pkg/memory/` | ✅ 完成 | 4 层文件映射、FTS5 检索（build tag） |
| `pkg/config/` | ✅ 完成 | JSON 配置、CoW 模式、热重载 |
| `pkg/skill/` | ✅ 完成 | 技能加载、同名去重 |

### ✅ 终端产品基础

| 模块 | 状态 | 说明 |
|------|------|------|
| `cmd/basework/` | ✅ 完成 | CLI 入口、cobra 子命令 |
| `tests/` | ✅ 完成 | 11 个集成测试、竞态检测通过 |
| `docs/guides/` | ✅ 完成 | 嵌入指南、配置参考、扩展指南 |

### ✅ Provider 支持

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

### ✅ 内置工具

| 工具 | 状态 | 说明 |
|------|------|------|
| `bash` | ✅ | Shell 命令执行 |
| `read` | ✅ | 文件读取 |
| `write` | ✅ | 文件写入 |
| `edit` | ✅ | 精确字符串替换 |
| `glob` | ✅ | 文件模式匹配 |
| `grep` | ✅ | 正则内容搜索 |
| `lsp_*` | ✅ | LSP 诊断/引用/重启（6 个工具） |

---

## 待完成功能（Phase 13-25）

### 🔲 生产韧性

| 功能 | 优先级 | 说明 |
|------|--------|------|
| 上下文压缩 | P0 | 自动摘要、工具输出修剪、保留最近轮次 |
| 重试机制 | P0 | 指数退避、retry-after 解析、速率限制检测 |
| 循环检测 | P1 | SHA-256 签名追踪、10 步 > 5 次自动中断 |
| Prompt 缓存 | P1 | Anthropic CacheHint 自动注入 |

### 🔲 安全与权限

| 功能 | 优先级 | 说明 |
|------|--------|------|
| 权限系统 | P0 | 规则引擎（allow/deny）、交互提示、YOLO 模式 |
| 命令黑名单 | P1 | bash 工具禁用危险命令 |
| OAuth 2.0 | P2 | PKCE 流程、令牌刷新、凭证存储 |

### 🔲 工具增强

| 工具 | 优先级 | 说明 |
|------|--------|------|
| `web_fetch` | P0 | URL 获取、markdown 转换 |
| `web_search` | P1 | 网页搜索（Tavily/Exa） |
| `todowrite` | P1 | 任务列表管理 |
| `apply_patch` | P2 | 结构化补丁应用 |
| `question` | P2 | 向用户提问 |

### 🔲 子代理系统

| 功能 | 优先级 | 说明 |
|------|--------|------|
| `task` 工具 | P0 | 启动子代理、隔离子会话 |
| 成本传播 | P1 | 子代理 token 累加到父会话 |
| 只读代理 | P1 | 信息搜索任务（无写入权限） |

### 🔲 会话增强

| 功能 | 优先级 | 说明 |
|------|--------|------|
| SQLite 后端 | P0 | 替换 JSONL、支持查询 |
| 自动标题 | P1 | 首次消息生成会话标题 |
| 会话队列 | P2 | 忙时排队提示 |
| 文件追踪 | P2 | 跟踪访问/修改的文件 |

### 🔲 Provider 扩展

| Provider | 优先级 | 说明 |
|----------|--------|------|
| OpenCode Zen | P0 | big-pickle 免费模型、默认选项 |
| Amazon Bedrock | P1 | AWS Converse API |
| Azure OpenAI | P1 | Azure 端点 |
| GitHub Copilot | P2 | OAuth 认证 |
| Ollama | P2 | 本地模型自动发现 |

### 🔲 终端 UI

| 功能 | 优先级 | 说明 |
|------|--------|------|
| TUI 框架 | P0 | Bubble Tea 基础界面 |
| Markdown 渲染 | P0 | Glamour 渲染响应 |
| 语法高亮 | P1 | Chroma 代码高亮 |
| Diff 视图 | P1 | 统一/分屏模式 |
| 会话选择器 | P2 | 浏览/切换会话 |
| 模型选择器 | P2 | 模型切换对话框 |

### 🔲 可观测性

| 功能 | 优先级 | 说明 |
|------|--------|------|
| 结构化日志 | P0 | slog 日志、文件输出 |
| 成本追踪 | P1 | token 计数、费用估算 |
| OpenTelemetry | P2 | 追踪导出 |

### 🔲 MCP 增强

| 功能 | 优先级 | 说明 |
|------|--------|------|
| 资源支持 | P1 | 列出/读取 MCP 资源 |
| 提示支持 | P1 | 获取 MCP 提示消息 |
| 自动重连 | P2 | ping 失败时重建连接 |
| Shell 变量展开 | P2 | MCP 配置中的 $VAR 展开 |

---

## 与参考项目对比

| 特性 | basework | opencode | crush |
|------|----------|----------|-------|
| **核心框架** | ✅ | ✅ | ✅ |
| **TUI** | 🔲 简单 REPL | ✅ OpenTUI | ✅ Bubble Tea |
| **上下文压缩** | 🔲 | ✅ 多策略 | ✅ 自动摘要 |
| **重试机制** | 🔲 | ✅ 指数退避 | ✅ OnRetry |
| **权限系统** | 🔲 | ✅ 规则引擎 | ✅ 交互提示 |
| **子代理** | 🔲 | ✅ task 工具 | ✅ agent 工具 |
| **循环检测** | 🔲 | 🔲 | ✅ SHA-256 |
| **Prompt 缓存** | 🔲 | ✅ CacheHint | ✅ 自动标记 |
| **结构化日志** | 🔲 | ✅ + OpenTelemetry | ✅ slog |
| **成本追踪** | 🔲 | ✅ | ✅ |
| **OAuth** | 🔲 | ✅ | ✅ |
| **本地模型** | 🔲 | 🔲 | ✅ Ollama |
| **Provider 数量** | 10 | 10+ | 20+ |
| **内置工具数量** | 8 | 13 | 22 |

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

---

## 下一步

1. **Phase 13**: 上下文压缩（compaction）
2. **Phase 14**: 重试机制（retry）
3. **Phase 15**: 权限系统（permission）
4. **Phase 16**: 子代理（sub-agents）
5. **Phase 17**: 增强工具（web fetch/search/todowrite）
6. **Phase 18**: TUI（Bubble Tea 终端界面）
7. **Phase 19-25**: 会话、Provider、可观测性、MCP 增强

详见 [TASKS.md](./TASKS.md)。
