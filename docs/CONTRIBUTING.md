# 贡献指南

先读 [文档中心](README.md)、[架构](ARCHITECTURE.md) 和 [AI 开发流程](development/ai-workflow.md)。从 [任务看板](TASKS.md) 选择依赖已满足的一项，小步实现并按任务卡验证。

## 开发与验证

Go **1.26.10 或更新的安全修订版**；最低版本由 `go.mod` 约束。`make build` 构建终端产品，使用 `sqlite memory` 两个可选 tag。不存在 `tui` 或 `otel` 的可选构建承诺。默认无 tag 的核心也必须保持可构建。

验证标准见 [validation](development/validation.md)；状态记录与证据使用 [模板](development/evidence/TEMPLATE.md)。代码改动后运行 `make gen`，文档或任务更新后运行 `make check-docs`。

修改 Go 工具链或依赖时运行 `make check-vuln`，分别检查默认与 `sqlite memory` 源码；发布前还需扫描本轮重新构建的二进制。记录实际 Go 版本、扫描器版本、漏洞库更新时间和扫描范围，旧结果不能证明后来披露的漏洞已修复。具体规则见[安全策略](SECURITY.md)与 [SEC-002](development/evidence/SEC-002.md)。

## 代码和兼容要求

遵循 Go 命名、格式化与显式错误处理；新增行为关注失败、取消和清理路径。`pkg/` 不导入 `internal/`，`internal/` 不导入 `cmd/`。公开接口与持久化格式改变时说明兼容方式并记录 ADR。

提交说明建议 `feat:`、`fix:`、`docs:`、`refactor:`、`test:` 前缀。PR 描述写清原问题、最终行为、验证与未验证项；范围大时按独立可验收步骤拆分。不要混入无关改动或覆盖已有未提交工作。

## 文档要求

正文只写在 `docs/`，根 README 仅导航。新增 pkg 包在 `docs/reference/pkg/` 创建对应五节契约。用户可见行为变化同步指南，实际能力变化同步 STATUS，任务进度只改 TASKS，不再新建第二份总清单。

中文撰写，保留必要英文术语；代码示例先核对真实接口。未来功能放任务卡或 ADR，不伪装为当前可执行命令。

## 社区

遵守 [行为准则](CODE_OF_CONDUCT.md)。安全问题见 [安全策略](SECURITY.md)。贡献按仓库 [MIT 许可证](../LICENSE) 提供。
