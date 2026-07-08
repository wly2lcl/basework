# Basework 路线图

> 从 alpha 原型到生产级 AI Agent 工具的演进路径

## 当前状态

- **版本**：v0.2.0
- **代码量**：~50,000 行 Go 代码
- **测试覆盖**：1,372 个测试全部通过
- **已完成阶段**：Phase 1-30（核心框架 + 终端产品化）以及 Phase 35.1-35.12 主路径收口
- **定位**：功能完整的 alpha 产品，主路径 wiring、CI/CD、发布流程和默认配置已完成第一轮收口

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

## 已完成的工程化收口

### Phase 26：会话稳定性加固 ✅
**目标**：解决长会话稳定性、并发安全

- Session 文件锁与 force unlock
- SQLite WAL 模式
- 长会话压缩配置接入
- 会话队列和自动恢复
- 跨平台文件锁测试修复

### Phase 27：CI/CD + 发布流程 ✅
**目标**：建立自动化测试、构建、发布流水线

- GitHub Actions quality/test/release dry-run
- Linux/macOS/Windows 测试和构建
- GoReleaser release workflow
- GitHub Release 二进制包
- GHCR Docker 镜像
- tag push 和手动 dispatch 发布路径

### Phase 28：安全加固 + 性能基线 ✅
**目标**：提升安全性、建立性能基准

- 权限持久化（SQLite）
- 敏感路径保护
- 工具执行超时
- pprof 集成
- benchmark 套件
- 权限审计日志

### Phase 29：TUI 增强 + 模板系统 ✅
**目标**：提升终端用户体验

- 主题系统（亮/暗/自定义）
- 命令面板（/ 斜杠命令）
- 键盘绑定系统
- Provider 感知系统提示模板
- 用户自定义模板
- 环境动态注入
- 对话框系统

### Phase 30：多模态 + 插件生态 ✅
**目标**：扩展能力和生态系统

- 图片输入支持
- Hook 系统扩展
- Provider 扩展点
- TUI 插件基础设施

## 后续优化规划

当前不再按 Phase 26-30 继续滚动新增大阶段。后续建议以小批次维护项推进：

- 真实使用验证：用长会话、跨仓库编码任务和多人机器环境持续验证稳定性
- Homebrew 发布：接入 tap 后再恢复 Homebrew 安装文档
- Release 体验：持续校验 GitHub Release、GHCR 镜像和安装文档一致性
- TUI 打磨：根据实际使用反馈优化布局、快捷键和错误展示

## 与参考项目对比

| 维度 | basework | crush | opencode |
|------|----------|-------|----------|
| **定位** | 可嵌入框架 + 终端产品 | 终端产品 | 终端产品 |
| **架构** | pkg/ + internal/ 分层 | 全 internal/ | monorepo |
| **代码量** | 50K 行 | 117K 行 | 200K+ 行（TS） |
| **Provider** | 15+ | ~8 | 10+ |
| **MCP** | 工具 + 资源 + 提示 | 仅工具 | 工具 + 资源 |
| **TUI** | Bubble Tea TUI，仍需真实使用打磨 | 生产级 | 自研框架 |
| **会话管理** | JSONL + SQLite + 文件锁 | SQLite + 文件锁 | 事件溯源 + 文件锁 |
| **CI/CD** | ✅ quality/test/release dry-run | ❌ | ✅ 27 workflows |
| **发布流程** | ✅ GitHub Release + GHCR | ❌ | ✅ 多渠道 |
| **多模态** | ✅ 图片输入 | ❌ | ✅ 图片 |
| **插件生态** | 基础扩展点 | ❌ | ✅ 4 种扩展 |

### 核心判断

**basework 的优势**：
- 可嵌入的 `pkg/` 层（独有）
- 更完整的 MCP 协议（工具+资源+提示）
- 更多 Provider（15+ vs crush 8）
- LSP/Hook/Skill/OAuth 系统

**basework 的劣势**：
- 无真实使用验证
- 长会话和多人真实场景仍需持续验证
- TUI 成熟度仍需使用反馈打磨
- Homebrew tap 尚未接入发布配置
- 插件生态还停留在基础扩展点阶段

**一句话总结**：basework 已从蓝图推进到可用 alpha 产品；接下来重点不再是补大模块，而是用真实使用反馈打磨稳定性、发布体验和 TUI 细节。

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

*最后更新：2026-07-08*
