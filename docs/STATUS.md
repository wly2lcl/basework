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

## 迁移策略

### 会话存储：JSONL → SQLite

| 阶段 | 说明 |
|------|------|
| Phase 19 前 | 保持 JSONL 为默认后端 |
| Phase 19 | 新增 SQLiteStore（build tag `sqlite`） |
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

### Phase 13-17（核心功能）

| 依赖 | 用途 | Phase | 大小 |
|------|------|-------|------|
| 无新增 | 全部使用标准库 | 13-17 | - |

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

### Phase 21（可观测性）

| 依赖 | 用途 | 大小 |
|------|------|------|
| 无新增 | 使用 `log/slog`（标准库） | - |

### 可选依赖

| 依赖 | 用途 | 条件 |
|------|------|------|
| `github.com/alecthomas/chroma/v2` | 语法高亮 | TUI 模式启用时 |
| `go.opentelemetry.io/otel` | OpenTelemetry 追踪 | build tag `otel` |

### 依赖增长估算

| 阶段 | 新增依赖 | 二进制增量 |
|------|----------|-----------|
| Phase 1-12（已完成） | cobra, modernc.org/sqlite | ~30MB |
| Phase 13-17 | 无 | ~0MB |
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
```

### 新增命令

```
# Phase 15: 权限
basework permission list              # 列出当前权限规则
basework permission add <rule>        # 添加规则
basework permission remove <rule>     # 删除规则

# Phase 18: TUI
basework tui                          # 启动 TUI 模式
basework tui --theme dark             # 指定主题
basework tui --resume <session-id>    # 恢复会话

# Phase 20: 模型管理
basework model set <model-id>         # 设置默认模型
basework model default                # 显示当前默认模型

# Phase 25: 认证
basework auth login <provider>        # OAuth 登录
basework auth logout <provider>       # 登出
basework auth status                  # 查看认证状态

# Phase 19: 会话管理增强
basework session resume <id>          # 恢复指定会话
basework session export <id>          # 导出会话（markdown/json）
basework session search <query>       # 搜索会话内容

# 通用
basework config validate              # 验证配置文件
basework config show                  # 显示当前配置
basework migrate sessions             # JSONL → SQLite 迁移
basework logs [--tail N] [--follow]   # 查看日志
```

### 命令演进路线

| Phase | 新增命令 |
|-------|---------|
| 15 | `permission list/add/remove` |
| 18 | `tui` |
| 19 | `session resume/export/search`, `migrate`, `logs` |
| 20 | `model set/default` |
| 25 | `auth login/logout/status` |

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
| 13 压缩 | 单元 + 集成 | > 80% |
| 14 重试 | 单元 + 集成 | > 80% |
| 15 权限 | 单元 + 集成 | > 90%（安全关键） |
| 16 子代理 | 单元 + 集成 | > 70% |
| 17 工具 | 单元 + 集成 | > 80% |
| 18 TUI | 单元（渲染逻辑） | > 60% |
| 19-25 | 单元 + 集成 | > 70% |

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

1. **Phase 13**: 上下文压缩（compaction）
2. **Phase 14**: 重试机制（retry）
3. **Phase 15**: 权限系统（permission）
4. **Phase 16**: 子代理（sub-agents）
5. **Phase 17**: 增强工具（web fetch/search/todowrite）
6. **Phase 18**: TUI（Bubble Tea 终端界面）
7. **Phase 19-25**: 会话、Provider、可观测性、MCP 增强

详见 [TASKS.md](./TASKS.md)。
