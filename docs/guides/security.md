# 安全配置指南

本文档介绍 basework 的安全功能，包括权限持久化、敏感路径保护、审计日志和权限迁移。

---

## 权限系统概述

basework 的权限系统采用三值评估模型，在工具调用前检查权限：

| 模式 | 说明 |
|------|------|
| **allow** | 允许执行，不提示 |
| **deny** | 拒绝执行，返回错误 |
| **ask** | 交互式提示，等待用户确认 |

权限规则按顺序匹配，最后匹配的规则生效（`findLast` 语义）。支持通配符匹配：
- `tool:*` — 匹配所有工具
- `tool:write:/etc/**` — 匹配写入 /etc/ 目录的操作
- `tool:bash:*rm*` — 匹配包含 rm 的 bash 命令

### 规则匹配流程

```
工具调用发起
    ↓
路径安全检查（敏感路径保护）
    ↓
权限规则匹配（有序规则列表）
    ├── Allow → 直接执行
    ├── Deny  → 返回拒绝错误
    └── Ask  → 交互式提示
                ├── 一次允许 → 仅本次
                ├── 永远允许 → 持久化到 SQLite
                └── 拒绝     → 返回错误
```

---

## 权限持久化

### 存储后端

权限规则支持两种存储后端：

| 后端 | 配置值 | 说明 |
|------|--------|------|
| SQLite | `sqlite` | 跨会话持久化，重启后规则不丢失（默认） |
| 内存 | `memory` | 仅当前会话有效，重启后清空 |

配置方式：

```json
{
  "security": {
    "permission_store": "sqlite"
  }
}
```

### SQLite 持久化

当使用 SQLite 后端时，以下数据持久化到 `~/.config/basework/permissions.db`：

- 用户授权的 "always allow" 规则
- 自定义的黑名单/白名单规则
- 审计日志记录

### 内存模式

内存模式适用于临时会话或测试场景。规则不会保存到磁盘，Agent 重启后所有授权记录丢失。

---

## 审计日志

审计日志记录所有权限决策，便于事后审查和安全分析。

### 记录内容

每次权限检查记录以下信息：

| 字段 | 说明 |
|------|------|
| 时间戳 | 权限决策发生时间 |
| 工具名 | 被检查的工具名称 |
| 参数 | 工具调用的参数（可配置脱敏） |
| 路径 | 操作涉及的文件路径 |
| 决策结果 | allow / deny / ask |
| 规则来源 | 匹配的规则 ID |
| 会话 ID | 发起调用的会话 |

### 配置

```json
{
  "security": {
    "audit_retention_days": 30
  }
}
```

`audit_retention_days` 设置审计日志保留天数（默认 30 天），超期日志自动清理。

### 查询审计日志

```bash
# 查看最近 50 条审计记录
basework permission audit

# 查看特定工具的审计记录
basework permission audit --tool bash

# 查看拒绝操作的审计记录
basework permission audit --effect deny

# 指定日志条数
basework permission audit --limit 100
```

---

## 敏感路径保护

敏感路径保护在工具执行前检查文件路径，防止 Agent 意外访问或修改敏感文件。

### 默认保护路径

以下路径默认受保护：

| 路径 | 说明 | 风险 |
|------|------|------|
| `.git/` | Git 仓库元数据 | 泄露提交历史、凭证 |
| `~/.ssh/` | SSH 密钥和配置 | 泄露服务器访问权限 |
| `~/.aws/` | AWS 凭证 | 泄露云服务访问权限 |
| `~/.gnupg/` | GPG 密钥 | 泄露签名密钥 |
| `~/.config/basework/` | Basework 配置 | 泄露 API key 等敏感配置 |

### 保护级别

| 级别 | 配置值 | 行为 |
|------|--------|------|
| 严格 | `strict` | 禁止访问敏感路径（默认） |
| 警告 | `warn` | 记录警告日志但允许访问 |
| 关闭 | `off` | 不进行路径检查 |

### 自定义路径

```json
{
  "security": {
    "protection_level": "strict",
    "sensitive_paths": {
      "block": [
        "/etc/shadow",
        "/var/run/secrets/",
        "/custom/secret/"
      ],
      "allow": [
        "~/.ssh/config",
        "/etc/hosts"
      ]
    }
  }
}
```

- `block` — 额外黑名单，追加到默认保护列表
- `allow` — 白名单，覆盖黑名单中的路径

白名单优先级高于黑名单，可用于需要访问特定敏感文件的场景。

### 路径匹配规则

- 基于绝对路径或相对路径匹配
- 支持通配符模式（`/etc/**` 匹配 /etc/ 下所有路径）
- 符号链接会解析为目标路径再检查

---

## 权限迁移

basework 提供权限规则的导入/导出工具，方便批量管理权限规则。

### 导出权限规则

```bash
# 导出所有规则到 JSON 文件
basework permission export --output rules.json

# 导出特定工具的规则
basework permission export --tool bash --output bash-rules.json
```

导出格式：

```json
[
  {
    "action": "tool:bash",
    "resource": "*rm*",
    "effect": "deny",
    "source": "manual",
    "created_at": "2026-07-06T10:00:00Z"
  },
  {
    "action": "tool:read",
    "resource": "~/.ssh/config",
    "effect": "allow",
    "source": "session_abc123",
    "created_at": "2026-07-06T11:30:00Z",
    "expires_at": "2026-08-06T11:30:00Z"
  }
]
```

### 导入权限规则

```bash
# 从 JSON 文件导入规则
basework permission import --file rules.json

# 导入前验证（不实际写入）
basework permission import --file rules.json --dry-run
```

导入规则：
- 自动去重（相同 action + resource 覆盖旧规则）
- 支持 `--dry-run` 预览变更
- 导入后刷新缓存，立即生效

---

## 完整安全配置示例

```json
{
  "security": {
    "protection_level": "strict",
    "permission_store": "sqlite",
    "audit_retention_days": 30,
    "sensitive_paths": {
      "block": ["/custom/secret/"],
      "allow": ["~/.ssh/config"]
    }
  }
}
```

---

## 常见问题

### 如何临时关闭所有权限检查？

```bash
# 启动时使用 YOLO 模式
basework agent --yolo
```

YOLO 模式下跳过所有权限提示和敏感路径检查。

### 持久化规则如何删除？

```bash
# 删除特定规则
basework permission delete --id <rule_id>

# 清空所有持久化规则
basework permission clear

# 重置为默认配置
basework permission reset
```

### 审计日志占用空间大吗？

审计日志存储在 SQLite 数据库中，每条记录约 200-500 字节。
以每天 1000 条权限决策计算，30 天约占用 6-15MB 空间。

### 敏感路径保护影响性能吗？

路径检查是轻量级字符串匹配操作，单次检查耗时 < 0.1ms。
即使在大量文件操作场景下，性能影响可忽略不计。

---

## 相关文档

- [配置参考](configuration.md) — 配置文件完整参考
- [CLI 使用指南](cli-guide.md) — CLI 命令完整参考
- [权限系统](permission-guide.md) — 权限规则和交互提示