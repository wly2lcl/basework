# 权限系统使用指南

本文说明当前运行时权限、敏感路径保护、命令黑名单和 SQLite 规则管理的实际行为。实现位于 `internal/permission/`，它是产品内部组件；外部 Go module 只能使用 `pkg/*` 公共 API，不能导入 `internal/permission`。

## 当前边界

权限检查由两层组成：

1. `Checker` 按工具名和参数决定 allow、deny 或 ask。
2. `PathChecker` 独立检查工具报告的文件路径。

它们是应用层检查，不是操作系统沙箱。Bash 仍在宿主机执行；需要进程、文件系统或网络隔离时，应使用外部沙箱。敏感路径检查也不会因为 `permission.mode` 是 `yolo` 而自动关闭。

带 SQLite 的权限管理 CLI 只在 `sqlite` build tag 下编译。仓库内构建使用：

```bash
make build
# 或
 go build -tags 'sqlite memory' ./cmd/basework
```

不带 tag 的二进制仍可运行 Agent、TUI 和内存权限检查，但没有 `permission`、`migrate`、`version` 等 SQLite 入口。实际命令以当前二进制的 `basework --help` 为准。

## 权限模式

配置字段是 `permission.enabled` 和 `permission.mode`：

| 模式 | 实际行为 |
|---|---|
| `interactive` | 先看内存缓存和持久化规则；未命中时调用审批回调。没有审批回调会拒绝并报错。 |
| `yolo` | `Checker` 对工具调用直接允许，不读取规则，也不提示。路径检查仍由独立的 `security.protection_level` 决定。 |
| `deny-all` | `Checker` 拒绝所有工具调用，不提示。 |

默认配置是 `enabled=false`、`mode=yolo`。启用权限后，若 mode 为空，运行时按 `deny-all` 处理；建议显式写入合法值。

最小配置示例：

```json
{
  "permission": {
    "enabled": true,
    "mode": "interactive",
    "command_blacklist": {
      "blocked_commands": ["git push --force", "rm -rf /"]
    }
  },
  "security": {
    "protection_level": "strict",
    "permission_store": "sqlite"
  }
}
```

`permission.rules` 和 `permission.blocked_commands` 不是当前 `pkg/config.Config` 的字段，写在配置文件中不会配置运行时规则。命令黑名单必须放在 `permission.command_blacklist.blocked_commands`。

## 规则与匹配

内存规则的结构是：

```go
type Rule struct {
    ToolPattern string // path.Match 模式，例如 "read_*"
    ArgPattern  string // 可选；匹配按键排序后的 "key=value" 参数串
    Allow       bool   // true=allow，false=deny
}
```

运行时从配置创建的 Checker 目前不从 JSON 读取 `Rule` 列表；交互模式会读取权限存储中的 `StoredRule`。持久化规则字段为：

| 字段 | 含义 |
|---|---|
| `rule_type` | `allow`、`deny` 或 `ask` |
| `pattern` | `tool_pattern`，或 `tool_pattern:arg_pattern` |
| `scope` | `global`、`session`、`project` 元数据 |
| `session_id` / `project_id` | 预留的关联字段 |
| `source`、时间字段 | 创建来源和审计信息 |

内存规则按列表顺序取**第一条**匹配项。SQLite 查询按创建时间升序返回**最早创建**的匹配项；`permission list` 为便于查看按创建时间降序显示。当前 `FindByPattern` 接口只接收工具和参数，`scope`/会话/项目字段还没有在匹配时做上下文过滤，应把它们理解为存储元数据，不能据此宣称已实现作用域隔离。

参数模式示例（参数串由实现生成，空格分隔）：

```bash
# 允许 read_file 工具
basework permission add --type allow --pattern 'read_file' --scope global

# 拒绝所有 bash 调用
basework permission add --type deny --pattern 'bash:*' --scope global

# 仅匹配包含 command=rm 的 bash 参数串（按需调整 glob）
basework permission add --type deny --pattern 'bash:*rm*' --scope global
```

规则存储只在 `interactive` 模式参与 Checker 决策。`ask` 规则会记录一次 asked，然后仍进入审批；审批结果默认不写成永久规则。

## 命令黑名单

同步 `bash` 和 `bash_background` 都执行内置黑名单，并合并配置中的正则模式。命中后的行为取决于工具实现和权限模式：默认模式拒绝，`interactive` 可转为待确认，`yolo` 的 Bash 工具会跳过黑名单；这不影响独立的敏感路径检查。

内置规则覆盖删根、`mkfs`、设备读写、fork bomb、下载后直接交给 shell、关机等危险命令。自定义模式放在：

```json
{
  "permission": {
    "command_blacklist": {
      "blocked_commands": ["git\\s+push\\s+--force", "terraform\\s+destroy"]
    }
  }
}
```

自定义项按 Go 正则编译；正则非法时工具启动返回错误。

## 交互审批

TUI/运行时审批请求带唯一请求 ID，并尽力展示工具目的、路径、diff 和风险依据。允许、拒绝、关闭、取消和超时都按该 ID 结算；陈旧 ID 不会应用到下一次请求。默认等待上限为 120 秒，超时等价于拒绝。一次审批决定不会自动缓存为永久规则；需要持久化规则请使用 `permission add`。

## 权限 CLI

以下命令需要 `sqlite` build tag，并固定操作 `~/.basework/permissions.db`（不能通过 `security.permission_store` 改变 CLI 数据库路径）：

```bash
basework permission list
basework permission add --type allow|deny|ask --pattern '<glob>' [--scope global|session|project]
basework permission remove --id <rule-id>
basework permission audit [--session <id>] [--tool <name>] [--days 7]
basework permission export [--output <path>]
basework permission import <json-file>
```

`permission audit` 当前最多查询 100 条记录，按时间倒序；没有 `--effect`、`--limit` 参数。`export` 不支持按工具筛选，省略 `--output` 时输出 stdout。`import` 使用唯一的位置参数，不支持 `--file` 或 `--dry-run`；导入会清空输入规则的 ID、标记来源为 `migration` 并创建新记录。当前没有 `mode`、`blocked`、`delete`、`clear`、`reset` 子命令。

SQLite 规则导出格式就是 `StoredRule` 数组，例如：

```json
[
  {
    "id": "",
    "rule_type": "deny",
    "pattern": "bash:*rm*",
    "scope": "global",
    "source": "user"
  }
]
```

数据库还保存权限审计表。运行时审计写入采用异步批量刷新；退出时由运行资源清理触发关闭。`security.audit_retention_days` 是组件提供的保留期参数，查询和清理由对应内部 API 执行，CLI 没有单独的 retention flag。

## 敏感路径保护

配置字段：

```json
{
  "security": {
    "protection_level": "strict",
    "sensitive_paths": {
      "block": ["/custom/secret/"],
      "allow": ["~/.ssh/config"]
    }
  }
}
```

`strict` 拒绝命中路径，`warn` 记录并允许，`off` 关闭检查。`allow` 优先于默认和自定义黑名单；路径会展开 `~`、规范化并解析相对路径，符号链接目标也会参与检查。

默认模式匹配的保护项来自 `internal/permission.DefaultSensitivePaths`：`.git/`、`.svn/`、`.hg/`、`/.ssh/`、`/.aws/`、`/.gnupg/`、`/.config/gcloud/`、`/etc/shadow`、`/etc/sudoers`。列表不包含一个名为 `~/.config/basework/` 的固定条目；如果要保护自定义 Basework 数据路径，请在 `sensitive_paths.block` 中显式配置。

## 排查顺序

1. 用 `basework config explain` 查看生效 provider、端点和脱敏后的配置。
2. 用 `basework permission list` 核对 SQLite 规则（完整 build tag）。
3. 用 `basework permission audit --tool <name> --days 7` 查看最近决策。
4. 检查 `permission.enabled`、`permission.mode`、`security.protection_level` 和黑名单位置。
5. 对规则匹配问题，先确认工具名、参数串和创建顺序；不要只看 `scope` 字段推断已隔离。

## 相关文档

- [安全配置](security.md)
- [配置参考](configuration.md)
- [CLI 使用指南](cli-guide.md)
- [TUI 使用指南](tui-guide.md)
- [任务看板](../TASKS.md)
