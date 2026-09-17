# 安全配置指南

本文集中说明 basework 当前已接入的应用层安全边界。权限规则、路径检查和 Bash 黑名单不会把宿主机变成操作系统沙箱；需要隔离执行时仍要使用外部容器或沙箱。

## 权限检查

启用 `permission.enabled` 后，运行时在工具调用前使用 `permission.mode`：

| 值 | 行为 |
|---|---|
| `interactive` | 先匹配缓存/规则，未命中后请求审批；无 prompt 时拒绝 |
| `yolo` | 工具调用直接允许，不提示、不读规则 |
| `deny-all` | 所有工具调用拒绝 |

Checker 的内存规则按列表顺序采用第一条匹配项；SQLite 存储查询按 `created_at ASC` 采用最早创建的匹配项。`permission list` 为显示方便按新到旧列出，因此显示顺序不等于实际命中顺序。持久化规则的 `scope`、`session_id`、`project_id` 当前只作为存储字段，匹配接口尚未接收会话/项目上下文，不能当成已实现的隔离边界。

## 存储和配置

```json
{
  "permission": {
    "enabled": true,
    "mode": "interactive",
    "command_blacklist": {
      "blocked_commands": ["git\\s+push\\s+--force"]
    }
  },
  "security": {
    "permission_store": "sqlite",
    "protection_level": "strict",
    "audit_retention_days": 30
  }
}
```

`permission_store` 支持 `sqlite` 和 `memory`。SQLite 运行时和权限 CLI 使用 `~/.basework/permissions.db`；会话 JSONL 和配置文件仍使用各自目录。没有 `sqlite` build tag 时，`sqlite`/`memory` 配置仍可加载，但 SQLite 权限持久化和管理命令不编译进二进制。

当前配置 schema 没有 `permission.rules`、`permission.blocked_commands`、`security.rules` 等字段；规则请使用 `permission add`，黑名单请使用 `permission.command_blacklist.blocked_commands`。

## 敏感路径

`security.protection_level` 的值为 `strict`、`warn` 或 `off`，默认 `strict`。默认保护模式来自代码中的以下模式：

- `.git/`、`.svn/`、`.hg/`
- `/.ssh/`、`/.aws/`、`/.gnupg/`、`/.config/gcloud/`
- `/etc/shadow`、`/etc/sudoers`

配置中的 `sensitive_paths.block` 会追加黑名单，`sensitive_paths.allow` 优先覆盖黑名单。路径会展开 `~`、清理 `..` 和相对路径，并检查规范化结果；可使用目录模式和 glob。`~/.config/basework/` 不是内置固定保护项，需要时显式加入 block。

路径检查通过独立的 `PathPermissionAdapter` 接入工具。即使权限模式为 `yolo`，`strict` 路径检查仍会拒绝敏感路径；不要把 yolo 当作绕过路径保护的开关。

## Bash 黑名单

同步和后台 Bash 共用内置危险命令规则，并追加 `permission.command_blacklist.blocked_commands` 中的 Go 正则。规则覆盖设备读写、格式化、删根、fork bomb、下载后交给 shell、关机等。命中后的 Bash 行为由实现模式决定；黑名单和敏感路径是两条独立检查链。

## 审计

SQLite 审计记录包含时间、会话 ID、工具名、规则 ID、决策、上下文和记录 ID。运行时记录异步批量写入，进程关闭时刷新。`permission audit` 的实际参数只有：

```bash
basework permission audit
basework permission audit --session <session-id> --tool bash --days 7
```

查询按时间倒序，CLI 固定最多返回 100 条；没有 `--effect` 或 `--limit`。审计组件提供 `Cleanup(retentionDays)`，保留期由 `security.audit_retention_days` 表达；CLI 没有独立的清理命令。

## 导入、导出和迁移

权限管理命令需要 `sqlite` build tag：

```bash
basework permission list
basework permission add --type deny --pattern 'bash:*rm*' --scope global
basework permission remove --id <rule-id>
basework permission export --output rules.json
basework permission import rules.json
```

`export` 省略 `--output` 时写 stdout，不能按工具筛选。`import` 接收一个 JSON 文件路径位置参数，不支持 `--file`、`--dry-run`；导入会重新生成 ID 并将来源写为 `migration`。当前没有 `permission delete`、`clear`、`reset` 或 mode/blocked 管理子命令。

SQLite 数据库路径由 CLI 固定为 `~/.basework/permissions.db`，用户目录不可用时回退到系统临时目录中的 `basework_permissions.db`。修改或备份前应先停止使用该数据库的运行实例，避免并发写入造成不一致。

## 审批安全属性

交互审批请求有唯一 ID；关闭、取消、超时和无效/过期应答不会放行下一请求。默认超时为 120 秒，超时等价于拒绝。审批卡片展示工具目的、尽力解析的路径、diff 和风险依据；批准后编辑层仍会再次检查内容基线和路径。

## 常见问题

### 如何启用或关闭权限？

在配置文件中设置 `permission.enabled` 和 `permission.mode`，然后用 `basework config explain` 检查脱敏后的生效值。Agent 没有 `--yolo` flag；不要照抄旧文档中的 `basework agent --yolo`。

### 为什么 yolo 仍拒绝某个路径？

yolo 只改变 Checker 的工具决策。`security.protection_level=strict` 的 PathChecker 仍独立拒绝默认或自定义敏感路径；请修改安全配置或显式加入白名单，并评估风险。

### 为什么规则看起来顺序相反？

`permission list` 以最新创建的规则优先显示，但匹配使用 SQLite 最早创建的命中项。需要改变优先级时，先导出并按明确顺序重建规则，完成后再用实际工具调用和审计记录验证。

## 相关文档

- [权限系统使用指南](permission-guide.md)
- [配置参考](configuration.md)
- [CLI 使用指南](cli-guide.md)
- [TUI 使用指南](tui-guide.md)
- [迁移指南](migration.md)
