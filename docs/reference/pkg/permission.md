# pkg/permission

## 用途

应用层权限决策：工具能不能调用、路径能不能读写、需要人工确认时怎么问、每次决定是否留审计。实现与产品 CLI/TUI 解耦，可单独嵌入。

- **`Checker`** — 三种模式：`interactive`（规则/缓存未命中时调用 `PromptFunc`）、`yolo`（直接允许）、`deny-all`（直接拒绝）。`Check` 并发安全；作用域上下文（`SessionID`/`ProjectID`）在检查开始时取快照，prompt 期间被改写不会影响本次判定。
- **规则** — 内存 `Rule`（`ToolPattern` + 可选 `ArgPattern` + `Allow`）按列表顺序取第一条匹配；持久化 `StoredRule` 支持 `global`/`session`/`project` 作用域，优先级 `session > project > global`，缺少对应上下文时 scoped 规则**安全地不命中**。
- **`PathChecker`** — 独立于权限模式的敏感路径保护：`strict` 拒绝、`warn` 记录并允许、`off` 关闭；`allow` 优先于默认与自定义黑名单；支持 glob、`~` 展开、软链目标检查。
- **审批** — `ApprovalBroker` 为每个请求生成唯一 ID，`SetNotifier` 推给 UI，`Respond(id, decision)` 结算；超时（`DefaultApprovalTimeout`）等价拒绝，过期 ID 不影响下一次请求。
- **持久化与审计** — `SQLiteStore`（需 `sqlite` 标签）+ 迁移 + `AuditLogger`；`ExportRules`/`ImportRules`/`VerifyImport` 支持规则迁移。
- **缓存** — `Cache` 按工具名 + 参数精确匹配，可绑定作用域上下文；不做模糊参数的越权复用。

## 配置

| 项 | 说明 |
|---|---|
| `Config.Enabled` | 是否启用权限检查 |
| `Config.Mode` | `interactive` / `yolo` / `deny-all`，用 `ParseMode` 校验 |
| `Config.Rules` | 内存规则列表 |
| `PromptFunc` | 审批回调 `func(ctx, toolName, args) (allow, cacheDecision bool, err error)`；`interactive` 下为 nil 会拒绝 |
| `ApprovalPromptFunc(broker, timeout)` | 把 broker 包装成 `PromptFunc`；`timeout<=0` 用默认 120s |
| `ScopeContext` | `SessionID` / `ProjectID`，由 `SetScopeContext` 绑定 |
| `ProtectionLevel` | `strict` / `warn` / `off`，用 `ParseProtectionLevel` 校验 |
| `NewPathChecker(blocked, allowed, level)` | 自定义阻断/放行列表与保护级别 |
| `NewSQLiteStore(path)` | 规则存储；仅 `sqlite` 标签可用 |
| `NewAuditLogger()` / `NewAuditLoggerWithDB(db)` | 审计写入 |
| `DefaultSensitivePaths` | 默认保护项：`.git/`、`.svn/`、`.hg/`、`~/.ssh/`、`~/.aws/`、`~/.gnupg/`、`~/.config/gcloud/`、`/etc/shadow`、`/etc/sudoers` |

错误值：`ErrBrokerClosed`；store/迁移错误按 `sqlite` 标签返回具体原因。

## 扩展点

- **审批 UI**：实现 `PromptFunc` 或使用 `ApprovalBroker` + 自有 notifier；请求结构 `ApprovalRequest` 已包含工具名、目的、路径、diff 与风险依据。
- **存储后端**：实现 `Store`；`ContextualStore` 额外支持按作用域过滤。SQLite 之外的后端只需满足接口。
- **审计后端**：`AuditLogger` 可接自有 writer/DB；记录字段与作用域上下文见 `AuditRecord`。
- **路径策略**：注入自定义 `blocked`/`allowed` 与 `ProtectionLevel`；也可在上层实现 `builtin.PathChecker` 同形接口做组合。
- **接口桥接**：需要 `agent.PermissionChecker` 时，按 [权限指南](../../guides/permission-guide.md#作为库使用) 写约 10 行适配器，必须同时实现 `Check` 与 `CheckPath`。

## Model Experience

- 交互模式下，模型发起的敏感工具调用会得到「等待用户确认」而不是静默成功或失败；超时明确按拒绝处理。
- 拒绝结果带工具名与参数上下文，模型能据此换一条路径（例如改用工作区内文件），而不是反复重试同一调用。
- 规则命中与审计记录保留作用域上下文，跨会话的 session/project 规则不会误命中当前会话。
- 路径拒绝返回具体路径与原因，模型可读到「被保护」而非「文件不存在」，避免把权限问题误判为路径错误。

## Known Limitations

- **不是 OS 沙箱**：Bash 仍在宿主机执行；本包只做工具调用与路径的应用层检查，不提供进程、网络或文件系统隔离。
- `yolo` 关闭的是工具审批，不关闭独立的敏感路径保护；`deny-all` 拒绝所有工具调用。
- 内存 `Rule` 不支持作用域；需要 `session`/`project` 优先级必须使用 `StoredRule` + 支持上下文的 `Store`。
- SQLite 存储、迁移与审计需要 `sqlite` 构建标签；无标签时只有内存与接口能力。
- 审批默认 120 秒超时；超时等价拒绝，且不会把决定写成永久规则。
- `warn` 级别只记录不阻断，不能当作防护保证。
- 本包是**实验 API（0.x）**（见 [ADR 0008](../../adr/0008-promote-internal-capabilities-to-pkg.md)）：1.0 前允许破坏性调整，但会走 ADR 并记录迁移方式。
