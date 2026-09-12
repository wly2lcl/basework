# 迁移指南

本文档介绍从 JSONL 到 SQLite 的会话存储迁移、配置格式演进以及 API 兼容性变更。

---

## JSONL → SQLite 迁移

### 概述

basework 的会话存储从 JSONL 迁移到 SQLite（Phase 19）。SQLite 提供更好的并发性能、更低的启动延迟和更小的磁盘占用。

| 对比项 | JSONL | SQLite |
|--------|-------|--------|
| 文件格式 | 纯文本逐行 JSON | 单一二进制文件 |
| 并发安全 | 文件锁 | WAL 模式，读写并发 |
| 查询能力 | 全量扫描 + 线性搜索 | 按时间/会话 ID 索引查询 |
| 启动耗时 | O(n) 全量加载 | O(1) 延迟打开 |
| 磁盘占用 | 大（未压缩） | 小（B-tree 存储） |

### 迁移前准备

- 确认 basework 版本 >= 0.2.0（Phase 19 发布后）
- 备份现有会话数据

```bash
# 规范目录（当前版本使用）
cp -r ~/.local/share/basework/sessions/ \
     ~/.local/share/basework/sessions.backup.$(date +%Y%m%d)
```

- 若你是从旧版本升级，旧数据可能在 `~/.basework/sessions/`，一并备份

- 确认磁盘空间充足（SQLite 数据库约为 JSONL 的 30%-50% 大小）

### 自动迁移

```bash
# 执行迁移
basework migrate sessions

# 验证迁移结果
basework session list
```

迁移命令会自动执行以下步骤：

1. 扫描会话源目录下的所有 `.jsonl` 文件。源目录优先 `~/.local/share/basework/sessions/`，
   该目录不存在而 `~/.basework/sessions/` 存在时回退到后者
2. 逐行解析 JSON 事件，写入 SQLite 数据库
3. 在源目录下生成 `sessions.db`（可用 `--sqlite-path` 覆盖）
4. 输出迁移统计（总会话数、事件数、耗时）

> **注意**：回退判定依据是「规范目录是否存在」。一旦你运行过新版本的
> `basework agent` / `basework tui`，规范目录会被创建，回退随之失效。
> 因此**升级后请先迁移、再启动 agent/TUI**，否则旧数据不会被自动扫描到。

### 手动迁移

```bash
# 导出为 SQLite（如果自动迁移失败）
basework migrate sessions --force

# 回滚到 JSONL
basework migrate sessions --rollback
```

`--force` 参数会覆盖已存在的 `sessions.db`。

`--rollback` 参数会：

1. 将 SQLite 数据导出回 JSONL 格式
2. 删除 `sessions.db`
3. 恢复使用 JSONL 存储

### 配置切换

在配置文件中指定会话存储后端：

```json
{
  "session": {
    "store": "sqlite"
  }
}
```

可选值：

| 值 | 说明 |
|----|------|
| `"jsonl"` | JSONL 文件存储（Phase 19 前默认） |
| `"sqlite"` | SQLite 数据库存储（Phase 19 起默认） |
| `"memory"` | 内存存储，重启后丢失 |

### 兼容性

- 迁移不删除原始 JSONL 文件，`sessions.db` 和 `.jsonl` 文件可共存
- SQLite 和 JSONL 可以同时存在，但同一时刻只有一个后端活跃
- 支持增量迁移 — 首次迁移后新增的会话会通过 SQLite 存储，旧的 JSONL 数据保留不变
- 降级到 JSONL 时，SQLite 中的数据会完整导出

### 故障排查

| 现象 | 可能原因 | 解决方法 |
|------|----------|----------|
| migration failed: permission denied | 文件权限不足 | 检查会话目录（`~/.local/share/basework/sessions/` 或旧目录 `~/.basework/sessions/`）读写权限 |
| migration failed: disk full | 磁盘空间不足 | 清理磁盘后重试 |
| session list 为空 | SQLite 路径配置错误 | 检查 `session.store` 配置值，确认 `.jsonl` 文件位置 |
| rollback 后数据丢失 | SQLite 数据未完全导出 | 检查 `--rollback` 日志，确认无解析错误 |

---

## 配置格式演进

### v0.1.0 配置（Phase 1-12）

```json
{
  "model": {
    "type": "anthropic",
    "api_key": "..."
  },
  "tools": {
    "enabled": ["bash", "read", "write"]
  }
}
```

该版本配置较为简洁，仅包含模型和工具的基本配置。

### v0.2.0+ 配置（Phase 13-25）

```json
{
  "model": {
    "type": "anthropic",
    "api_key": "...",
    "model_id": "claude-sonnet-4-20250514"
  },
  "tools": {
    "enabled": ["bash", "read", "write"]
  },
  "compaction": {
    "enabled": true,
    "max_tokens": 64000,
    "strategy": "summary"
  },
  "retry": {
    "max_attempts": 3,
    "initial_delay": "1s",
    "max_delay": "30s"
  },
  "permission": {
    "mode": "auto",
    "allowed_commands": ["ls", "cat"]
  },
  "tui": {
    "theme": "dark",
    "keybindings": "vim"
  },
  "session": {
    "store": "sqlite",
    "max_sessions": 100
  },
  "observability": {
    "tracing": false,
    "metrics": false
  },
  "loop_detect": {
    "enabled": true,
    "max_iterations": 20
  },
  "cache": {
    "enabled": true,
    "ttl": "5m",
    "max_entries": 1000
  }
}
```

新增字段说明：

| 字段 | 引入阶段 | 默认值 | 说明 |
|------|----------|--------|------|
| `compaction` | Phase 13 | `{"enabled": true}` | 上下文压缩策略 |
| `retry` | Phase 14 | `{"max_attempts": 3}` | API 调用重试 |
| `permission` | Phase 15 | `{"mode": "auto"}` | 工具权限控制 |
| `tui` | Phase 16 | `{"theme": "dark"}` | 终端 UI 配置 |
| `session` | Phase 19 | `{"store": "sqlite"}` | 会话存储配置 |
| `observability` | Phase 21 | `{"tracing": false}` | 可观测性 |
| `loop_detect` | Phase 22 | `{"enabled": true}` | 循环检测 |
| `cache` | Phase 23 | `{"enabled": true}` | 响应缓存 |

旧字段（`model`, `tools`）保持完全兼容，无需修改现有配置。

---

## API 迁移指南

### Agent 扩展

```go
// 旧代码继续工作
a, _ := agent.New(agent.WithModel(model))

// 新功能通过 Option 添加
a, _ := agent.New(
    agent.WithModel(model),
    agent.WithCompaction(compaction.Config{
        Enabled:   true,
        MaxTokens: 64000,
    }),
    agent.WithRetry(retry.Config{
        MaxAttempts: 3,
    }),
)
```

所有新增 Option 函数均为可选，不传递则使用对应功能的默认值（通常为禁用状态）。

### Hook 扩展

```go
// 旧 Hook 接口不变
type Hook interface {
    PreToolUse(ctx, tool, args) error
    PostToolUse(ctx, tool, result) error
}

// 新扩展接口（可选实现）
type ExtendedHook interface {
    Hook
    PreStep(ctx, messages) error
    PostStep(ctx, response) error
}
```

现有实现 Hook 接口的代码无需修改。需要新功能的代码可以实现 `ExtendedHook` 接口以获得更多生命周期回调。

### 其他 API 变更

| 包 | 变更 | 兼容性 |
|----|------|--------|
| `pkg/config` | 新增 `Session`, `Compaction`, `Retry` 等配置结构体 | 向后兼容，新增字段默认零值 |
| `pkg/session` | 新增 `SQLiteStore`，`Store` 接口扩展 `Count()` 方法 | 接口扩展可选实现 |
| `pkg/provider` | 新增 Provider 类型（见 Provider 配置指南） | 旧 Provider 配置继续生效 |

---

## 版本升级检查清单

- [ ] 阅读 `CHANGELOG.md` 了解当前版本与目标版本之间的所有变更
- [ ] 备份配置文件（`~/.config/basework/config.json`）
- [ ] 备份会话数据（`~/.local/share/basework/sessions/`，旧版本为 `~/.basework/sessions/`）
- [ ] 运行 `basework migrate sessions` 迁移会话存储
- [ ] 运行 `basework session list` 验证迁移结果
- [ ] 测试核心功能（会话管理、工具调用、模型切换）
- [ ] 更新 CI/CD 配置中的 basework 版本号
- [ ] 检查废弃 API 的告警日志，规划后续迁移

---

> **参考**：完整配置说明见 [配置参考](configuration.md)，Provider 配置见 [Provider 配置指南](provider-guide.md)。
