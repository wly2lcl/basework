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

### Phase 13-25: 生产韧性 + 增强功能（2025-2026）

| Phase | 模块 | 说明 |
|-------|------|------|
| Phase 13 | `internal/compaction/` | 上下文压缩：自动摘要、工具输出修剪、保留最近轮次 |
| Phase 14 | `internal/retry/` | 重试机制：指数退避、retry-after 解析、速率限制检测 |
| Phase 15 | `internal/permission/` | 权限系统：规则引擎（allow/deny）、交互提示、YOLO 模式 |
| Phase 16 | `internal/subagent/` | 子代理：`task` 工具、隔离子会话、成本传播 |
| Phase 17 | `internal/tools/` + `pkg/tool/builtin/` | 增强工具：`web_fetch`、`web_search`、`todowrite`、`apply_patch`、`question` + 命令黑名单 |
| Phase 18 | `internal/tui/` | 终端 UI：Bubble Tea 框架、Markdown 渲染（Glamour）、语法高亮（Chroma）、Diff 视图、会话/模型选择器 |
| Phase 19 | `pkg/session/` | 会话增强：SQLite 后端、自动标题、会话队列、文件追踪 |
| Phase 20 | `pkg/provider/` | Provider 扩展：+5 个 Provider（OpenCode Zen、Amazon Bedrock、Azure OpenAI、GitHub Copilot、Ollama），共 15+ |
| Phase 21 | `internal/observability/` | 可观测性：结构化日志、成本追踪、OpenTelemetry |
| Phase 22 | `internal/loopdetect/` | 循环检测：SHA-256 签名追踪、10 步超 5 次自动中断 |
| Phase 23 | `pkg/provider/cache.go` | Prompt 缓存：Anthropic CacheHint 自动注入 |
| Phase 24 | `pkg/mcp/` | MCP 增强：资源支持、提示支持、自动重连 |
| Phase 25 | `internal/oauth/` | OAuth 认证：PKCE 流程、令牌刷新、凭证存储 |

**统计数据**：代码规模 ~50,000 行 Go 代码，测试覆盖 1,372 个测试全部通过。

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
| OpenCode Zen | openai-compat |
| Amazon Bedrock | aws-bedrock |
| Azure OpenAI | azure-openai |
| GitHub Copilot | copilot-chat |
| Ollama | ollama |

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
| `web_fetch` | URL 抓取 + 内容提取 |
| `web_search` | Web 搜索（需配置搜索引擎 API） |
| `todowrite` | 任务清单管理 |
| `apply_patch` | 精确代码补丁应用 |
| `question` | 交互式用户提问 |

---

## 优化规划

### Phase 26: 会话稳定性加固（🔴 高优先级）

| 任务 | 说明 |
|------|------|
| 会话持久化错误恢复 | 崩溃后自动恢复未保存会话，Write-Ahead Log 保护 |
| Provider 故障转移 | 主 Provider 失败时自动切换到备用 Provider，无缝降级 |
| 速率限制智能调度 | 跨 Provider 请求队列、令牌桶算法、动态节流 |
| 大上下文分割 | 超长上下文自动分片处理，避免 Token 超限 |
| 会话完整性校验 | 周期性 Checksum 校验，提前发现存储损坏 |

**预计代码量**：~3000 行，依赖标准库 + 现有 `pkg/session/` 和 `internal/retry/`。

### Phase 27: CI/CD + 发布流程（🔴 高优先级）

| 任务 | 说明 |
|------|------|
| GitHub Actions 流水线 | Lint → 测试 → 构建 → 发布全自动化 |
| 多平台构建 | macOS (amd64/arm64)、Linux (amd64/arm64)、Windows (amd64) |
| Homebrew Tap | 发布到 Homebrew 第三方仓库，支持 `brew install basework` |
| 自动版本管理 | 基于 Git Tag 的 SemVer 自动生成 + Changelog 生成 |
| 集成测试套件 | 模拟真实 LLM 调用（Mock Provider）、端到端回归测试 |

**预计代码量**：~500 行（CI 配置）+ 测试夹具，依赖 GitHub 生态。

### Phase 28: 安全加固 + 性能基线（🔴 高优先级）

| 任务 | 说明 |
|------|------|
| 凭证管理增强 | 系统密钥链集成（macOS Keychain、Linux Secret Service） |
| 工具沙箱 | 命令执行超时 + 资源限制 + 路径白名单 |
| 外部输入校验 | 所有用户输入/工具输出的严格边界校验，防止注入 |
| 性能基准测试套件 | 关键路径 Benchmarks + 火焰图分析 + 内存/CPU Profiling |
| 启动时间优化 | 懒加载 Provider、延迟初始化非关键模块 |

**预计代码量**：~2500 行，依赖标准库 + `internal/permission/`。

### Phase 29: TUI 增强 + 模板系统（🟡 中优先级）

| 任务 | 说明 |
|------|------|
| 多窗口布局 | 分屏编辑、侧边栏导航、可拖拽面板 |
| 会话搜索 | 历史会话全文搜索 + 过滤 + 导出 |
| Prompt 模板系统 | 可复用模板、变量注入、模板版本管理 |
| 自定义配色主题 | 主题文件格式、社区主题市场 |
| 键盘快捷键自定义 | 用户可配置快捷键映射 |

**预计代码量**：~4000 行，依赖 Bubble Tea 生态 + `pkg/session/`。

### Phase 30: 多模态 + 插件生态（🟢 低优先级）

| 任务 | 说明 |
|------|------|
| 图片输入处理 | 图片上传、Base64 编码、自动尺寸调整 |
| 音频输入处理 | 音频文件转文字、Whisper API 集成 |
| 插件 SDK | 插件接口定义、生命周期管理、独立沙箱运行 |
| 插件市场 | 社区插件注册、版本管理、一键安装 |
| Web UI 服务模式 | HTTP 服务模式、REST API、WebSocket 实时推送 |

**预计代码量**：~5000 行，依赖第三方库（图片处理、音频处理）+ 插件运行时。

---

## 与参考项目对比

| 维度 | basework | crush/opencode |
|------|----------|----------------|
| **架构设计** | 分层清晰，`pkg/` 可嵌入框架 + `internal/` 终端产品 | 单体架构，框架与产品耦合紧密 |
| **Provider 支持** | 15+ Provider，工厂模式统一管理 | 有限 Provider，硬编码集成 |
| **工具系统** | 13 个工具，接口化注册，可扩展 | 工具数量较少，扩展性有限 |
| **MCP 支持** | 完整 MCP 客户端（资源 + 提示 + 工具） | 无 MCP 支持 |
| **LSP 支持** | 集成 LSP 客户端，6 个语言工具 | 无 LSP 支持 |
| **可观测性** | 结构化日志 + 成本追踪 + OpenTelemetry | 基础日志 |
| **测试覆盖** | 1,372 个测试 | 较少测试覆盖 |
| **代码规模** | ~50,000 行 Go | 更小代码库 |
| **成熟度** | 架构更完善，功能更丰富 | 打磨更久，稳定性更高 |
| **社区** | 新项目，社区成长中 | 已有用户基础 |

**核心判断**：basework 是设计更好的蓝图，crush 是打磨更久的成品。basework 在架构设计、Provider 支持、工具系统、MCP/LSP 集成方面领先，但需要更多时间打磨稳定性。crush 在用户基础和实战验证方面更成熟，但架构扩展性受限。

---

## 时间估算

| Phase | 内容 | 预计时间 | 优先级 |
|-------|------|----------|--------|
| Phase 26 | 会话稳定性加固 | 1.5-2 周 | 🔴 高 |
| Phase 27 | CI/CD + 发布流程 | 1-1.5 周 | 🔴 高 |
| Phase 28 | 安全加固 + 性能基线 | 1.5-2 周 | 🔴 高 |
| Phase 29 | TUI 增强 + 模板系统 | 2-3 周 | 🟡 中 |
| Phase 30 | 多模态 + 插件生态 | 2-3 周 | 🟢 低 |
| **总计** | Phase 26-30 | **~7-9 周** | — |

Phase 26-28 可并行推进，Phase 29-30 依赖前序 Phase 完成。

---

## 未来方向（未规划）

以下功能已识别但未排入具体路线图，根据社区反馈和需求优先级择机实施：

- **代码索引** — codebase 搜索与语义理解（Phase 26-30 已纳入规划）
- **企业级功能** — SSO、审计日志、RBAC（Phase 26-30 已纳入规划）
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
| `sqlite` | 关 | SQLite 会话存储（替换 JSONL） |
| `tui` | 关 | Bubble Tea 终端 UI（Phase 29） |
| `otel` | 关 | OpenTelemetry 追踪导出 |

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
| Phase 13-17（已完成） | 无 | ~0MB |
| Phase 18（已完成） | Bubble Tea 生态 | ~3.5MB |
| Phase 20（已完成） | AWS + Azure SDK | ~35MB |
| Phase 26-30（规划中） | 图片/音频处理库、插件运行时 | ~10MB |
| **总计** | - | **~78MB** |