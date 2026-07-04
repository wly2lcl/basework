# Basework 路线图

> 从 alpha 原型到生产级 AI Agent 工具的演进路径

## 当前状态

- **版本**：v0.2.0
- **代码量**：~50,000 行 Go 代码
- **测试覆盖**：1,372 个测试全部通过
- **已完成阶段**：Phase 1-25（核心框架 + 终端产品化）
- **定位**：功能完整的 alpha 原型

## 已完成

### Phase 1-12：核心框架 ✅
- 统一类型系统（pkg/llm）
- 工具系统（pkg/tool + 8 个内置工具）
- 会话管理（pkg/session）
- Agent 核心循环（pkg/agent）
- Provider 工厂（pkg/provider + 10 个 Provider）
- LSP 集成（pkg/lsp）
- MCP 集成（pkg/mcp）
- 记忆系统（pkg/memory）
- 配置管理（pkg/config）
- 技能加载（pkg/skill）
- CLI 入口（cmd/basework）
- 集成测试 + 文档

### Phase 13-25：终端产品化 ✅
- 上下文压缩（internal/compaction）
- 重试机制（internal/retry）
- 权限系统（internal/permission）
- 子代理系统（internal/subagent）
- 增强工具（internal/tools + 5 个工具）
- 终端 UI（internal/tui + Bubble Tea）
- 会话增强（SQLite + 文件追踪 + 队列）
- Provider 扩展（+5 个 Provider，共 15+）
- 可观测性（internal/observability）
- 循环检测（internal/loopdetect）
- Prompt 缓存（pkg/provider/cache.go）
- 命令黑名单（pkg/tool/builtin/blacklist.go）
- MCP 增强（资源 + 提示 + 韧性）
- OAuth 认证（internal/oauth）

## 优化规划

### Phase 26：会话稳定性加固 🔲
**优先级**：🔴 高
**目标**：解决长会话稳定性、并发安全

- Session 文件锁（参考 opencode flock.ts）
- SQLite WAL 模式
- 压缩双策略（摘要 + 修剪）
- tail_turns 保护
- 工具输出截断（>2000 字符）
- 长会话压力测试（100+ 轮）
- 并发写入测试

**预计新增**：~2,000 行代码

### Phase 27：CI/CD + 发布流程 🔲
**优先级**：🔴 高
**目标**：建立自动化测试、构建、发布流水线

- GitHub Actions 基础 workflow（test + build + lint）
- 交叉编译（Linux/macOS/Windows）
- goreleaser 集成
- Homebrew 发布
- Docker 镜像（可选）
- 自动 CHANGELOG

**预计新增**：~500 行配置

### Phase 28：安全加固 + 性能基线 🔲
**优先级**：🔴 高
**目标**：提升安全性、建立性能基准

- 权限持久化（SQLite）
- 敏感路径保护（.git/, ~/.ssh/, ~/.aws/）
- 工具执行超时（默认 30s）
- pprof 集成
- benchmark 套件
- 安全审计清单

**预计新增**：~1,500 行代码

### Phase 29：TUI 增强 + 模板系统 🔲
**优先级**：🟡 中
**目标**：提升终端用户体验

- 主题系统（亮/暗/自定义）
- 命令面板（/ 斜杠命令）
- 键盘绑定系统
- 按模型分发系统提示模板
- 用户自定义模板（.basework/prompts/）
- 环境动态注入
- 对话框系统优化

**预计新增**：~3,000 行代码

### Phase 30：多模态 + 插件生态 🔲
**优先级**：🟢 低
**目标**：扩展能力和生态系统

- 图片输入支持（resize + base64）
- Hook 系统扩展（PreStep, PostStep, OnToolError）
- 提供商插件化
- TUI 插件插槽（长期）

**预计新增**：~2,500 行代码

## 与参考项目对比

| 维度 | basework | crush | opencode |
|------|----------|-------|----------|
| **定位** | 可嵌入框架 + 终端产品 | 终端产品 | 终端产品 |
| **架构** | pkg/ + internal/ 分层 | 全 internal/ | monorepo |
| **代码量** | 50K 行 | 117K 行 | 200K+ 行（TS） |
| **Provider** | 15+ | ~8 | 10+ |
| **MCP** | 工具 + 资源 + 提示 | 仅工具 | 工具 + 资源 |
| **TUI** | 基础组件 | 生产级 | 自研框架 |
| **会话管理** | JSONL + SQLite | SQLite + 文件锁 | 事件溯源 + 文件锁 |
| **CI/CD** | ❌ | ❌ | ✅ 27 workflows |
| **发布流程** | ❌ | ❌ | ✅ 多渠道 |
| **多模态** | ❌ | ❌ | ✅ 图片 |
| **插件生态** | ❌ | ❌ | ✅ 4 种扩展 |

### 核心判断

**basework 的优势**：
- 可嵌入的 `pkg/` 层（独有）
- 更完整的 MCP 协议（工具+资源+提示）
- 更多 Provider（15+ vs crush 8）
- LSP/Hook/Skill/OAuth 系统

**basework 的劣势**：
- 无真实使用验证
- 长会话稳定性未验证
- 会话并发安全缺失
- TUI 成熟度不足
- CI/CD 和发布流程缺失
- 安全审计未完成

**一句话总结**：basework 是设计更好的蓝图，crush 是打磨更久的成品。basework 需要用 Phase 26-30 补齐工程成熟度的差距。

## 实施原则

1. **稳定性优先**：先解决 Phase 26-28（稳定性+安全+CI/CD），再考虑 Phase 29-30（体验+生态）
2. **真实场景驱动**：每个 Phase 完成后，用实际开发任务验证
3. **渐进式打磨**：不追求一步到位，持续迭代优化
4. **保持架构优势**：不因打磨而牺牲可嵌入性和模块化设计

## 时间估算

| Phase | 预计工作量 | 建议时间 |
|-------|-----------|---------|
| Phase 26 | 2,000 行 + 测试 | 1-2 周 |
| Phase 27 | 500 行配置 | 3-5 天 |
| Phase 28 | 1,500 行 + 测试 | 1 周 |
| Phase 29 | 3,000 行 + 测试 | 2 周 |
| Phase 30 | 2,500 行 + 测试 | 2 周 |
| **总计** | ~9,500 行 | 7-9 周 |

## 长期愿景

成为 Go 生态中最优秀的可嵌入 AI Agent 框架，同时提供生产级的终端产品体验。

- **短期目标**（Phase 26-28）：达到生产级稳定性
- **中期目标**（Phase 29-30）：达到生产级体验
- **长期目标**：成为 Go AI Agent 的事实标准

---

*最后更新：2026-07-04*