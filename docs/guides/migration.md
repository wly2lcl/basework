# 迁移指南

本文说明当前仓库提供的会话 JSONL→SQLite 迁移工具，以及 SQLite WAL 模式维护。迁移命令是辅助能力；当前 Agent/TUI 主运行时仍固定使用 JSONL，配置中的 `session.store` / `session.sqlite_path` 不会自动切换主运行时后端。

## 使用前确认

迁移相关 CLI 需要 `sqlite` build tag：

```bash
make build
# 或
go run -tags 'sqlite memory' ./cmd/basework migrate --help
```

先停止正在写入会话的 Agent/TUI，备份源目录，并确认目标磁盘可写。默认 JSONL 会话目录是 `~/.local/share/basework/sessions/`；若该目录不存在而旧目录 `~/.basework/sessions/` 存在，迁移命令会回退读取旧目录。

```bash
cp -R ~/.local/share/basework/sessions \
  ~/.local/share/basework/sessions.backup.$(date +%Y%m%d-%H%M%S)
```

## JSONL 导入 SQLite

```bash
basework migrate sessions
basework migrate sessions --sqlite-path /path/to/sessions.db
```

命令会：

1. 扫描源目录中的 `.jsonl` 文件；
2. 通过真实 JSONL store 读取事件，执行版本检查和已支持的迁移链；
3. 保留原会话 ID，将事件写入目标 SQLite；
4. 跳过目标库中已经存在的会话，并输出成功、跳过、失败统计；
5. 任何会话失败都以非零退出码结束，未来 schema 版本不会被部分导入。

目标路径默认是源目录下的 `sessions.db`，可用 `--sqlite-path` 覆盖。源 JSONL 只读不改写；迁移成功不代表主 Agent/TUI 已改用 SQLite，当前产品入口仍读写 JSONL。

迁移完成后可以直接检查 SQLite 结果：

```bash
basework session status
basework session status --check-integrity
```

`session status` 读取迁移生成的 SQLite 数据库；`session list` 仍是 JSONL 查询入口。两者出现差异时，先确认目录和 `--sqlite-path`，不要把 `session.store` 当作已接入的运行时切换开关。

## WAL 模式维护

迁移命令还提供 SQLite journal 模式维护：

```bash
basework migrate to-wal
basework migrate rollback
```

`migrate to-wal` 以会话目录下的 `sessions.db` 为默认目标，也会在切换前保存带进程号的 `.backup-*` 文件；已经是 WAL 时不重复转换。`migrate rollback` 查找最新备份，恢复数据库文件并移除 `-wal` / `-shm` 文件。它回滚的是 WAL 数据库维护操作，不是把 SQLite 数据导回 JSONL，也不会改变 Agent/TUI 的主运行时后端。

如果数据库路径不是默认值，迁移会话时用过 `--sqlite-path` 后，WAL/rollback 命令仍需按当前实现使用默认会话目录路径；需要维护自定义路径时应在停机后通过 SQLite 工具或内部组件完成，避免误操作另一份数据库。

## 数据与版本安全

- 源 JSONL 会保留；迁移是读取后另写 SQLite。
- 已支持的旧 schema 会沿迁移链升级；高于当前支持版本的文件会被拒绝。
- 非法会话 ID、重复 ID 和事件全部失败的会话会返回错误，不会静默报告成功。
- 迁移失败后先保留源文件和备份，查清错误再重试；不要直接删除唯一副本。

## 配置字段说明

`session.store` 和 `session.sqlite_path` 是配置 schema 中的存储字段，当前主 runtime 固定使用 JSONL，因此它们不会自动把 Agent/TUI 切换到 SQLite。`database.mode` 的 `wal`/`delete` 说明对应数据库组件，不等于 CLI 已完成后端切换。

```json
{
  "session": {
    "store": "jsonl",
    "sqlite_path": ""
  },
  "database": {
    "mode": "wal"
  }
}
```

## 常见问题

| 现象 | 排查 |
|---|---|
| `migrate` 不在 help 中 | 使用 `sqlite` build tag 构建；无 tag 二进制不包含该命令 |
| 找不到会话目录 | 先确认 XDG 目录和旧版 `~/.basework/sessions/`，迁移前不要先启动新 Agent/TUI 创建空目录 |
| 迁移报告有失败 | 退出码为非零；检查未来版本、损坏文件、权限和磁盘空间，源文件不会被删除 |
| `session list` 与 `session status` 不一致 | 前者读 JSONL，后者读 SQLite；核对源目录、`sessions.db` 路径和迁移结果 |
| 想把 SQLite 回到 JSONL | 当前 `migrate rollback` 只回滚 WAL；SQLite→JSONL 导出不提供 CLI 自动入口，请保留原 JSONL 或使用内部组件编写受控导出 |

## 升级清单

- [ ] 阅读当前版本 [CHANGELOG](../CHANGELOG.md)，确认 schema 和目录变化
- [ ] 停止 Agent/TUI 并备份配置与 JSONL 会话目录
- [ ] 用完整 build tag 查看 `migrate`/`session status` help
- [ ] 运行 `migrate sessions`，保存终端输出和目标数据库路径
- [ ] 用 `session status --check-integrity` 检查 SQLite 文件
- [ ] 保留源 JSONL，确认主运行时仍按 [当前状态](../STATUS.md) 的边界运行

## 相关文档

- [配置参考](configuration.md)
- [CLI 使用指南](cli-guide.md)
- [安装指南](../installation.md)
- [发布验收证据](../development/evidence/SHIP-002.md)
