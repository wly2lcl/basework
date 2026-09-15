# CLI 使用指南

核对基线：2026-09-15 / 当前工作区。命令以当前二进制 `--help` 为准；尚未提供的发布能力见 [STATUS](../STATUS.md) 和任务看板。

## 启动与配置

```bash
basework init
basework init --yes --provider opencode --preset readonly
basework model list
basework config explain
basework agent
basework agent -m "说明当前目录结构"
basework tui
```

`init` 默认是交互式菜单。脚本或 AI 开发流程使用 `init --yes`；可用 `--provider`、`--model`、`--preset` 指定确定值，已有配置默认不覆盖，覆盖必须显式加 `--force`。需要密钥的 provider 在缺少对应环境变量时返回非零错误，不会等待 stdin 或写入密钥。

模型 ID 使用配置顶层 `provider` / `model`，自定义端点在 `providers.<provider>.base_url`；密钥可以按 [Provider 指南](provider-guide.md) 设置。示例：

```json
{
  "provider": "openai",
  "model": "gpt-4o-mini",
  "providers": {
    "openai": {
      "base_url": "https://gateway.example.com/v1"
    }
  }
}
```

上面的端点是占位例子，应替换为实际服务。当前 agent 没有 `--model` 参数；使用配置文件或 `model use` 选择已列出的模型。

```bash
basework --config /path/to/config.json config explain
basework --config /path/to/config.json agent -m "解释当前项目"
```

`config explain` 是只读脱敏解释入口，当前没有 `config show` / `config validate` 子命令。字段能够加载不等于所有产品路径已消费它；运行行为缺口见 STATUS。

## Agent 与 TUI

| 入口或参数 | 当前用途 |
|---|---|
| `agent` | 简单 REPL，同一进程内多轮对话 |
| `agent -m / --message` | 发送一次消息并退出 |
| `agent --no-stream` | 一次性输出完整响应 |
| `agent --preset readonly / coding` | 使用启动预设 |
| `tui` | 全屏终端 UI |
| `tui --session <id>` | 绑定并恢复指定历史会话 |
| `tui --no-tui` | 回退简单 REPL |
| `tui --preset readonly / coding` | 与 Agent 相同的预设解析 |
| 全局 `--config` / `-v, --verbose` | 配置路径 / 详细输出 |

交互 REPL 输入 `exit` 或 EOF 退出。TUI 的按键、审批、任务卡片统一见 [TUI 指南](tui-guide.md)，不在这里重复维护。

取消当前轮与退出应用尚未在三条入口完全对齐；`tui --no-tui` 有把普通错误显示为中断的问题，RUN-003 跟踪。自动脚本优先使用单次 `agent -m`，并核对退出码和实际产物。

## 模型与能力

```bash
basework model list
basework model list --free
basework model info <model-id>
basework model use <model-id>
```

先选择 list 中存在的 ID。能力区分 supported/unsupported/unknown 和来源；免费与 requires_api_key 分离。list 的静态声明不保证特定服务此刻可用，也不保证任意网关兼容。

## 会话和历史查看

```bash
basework session list
basework session status --help
basework jobs list
basework jobs show --help
basework edits list --help
basework edits show --help
basework facts show --help
```

`jobs` 查看后台任务历史，`edits` 查看编辑事件，`facts` 做事实聚合。这些查询不重新执行命令或提交编辑。

当前 session 子命令为 list、clear、status、unlock；clear/unlock 会修改本地数据，应先阅读各自 help。TUI 启动时可用 `--session <id>` 绑定历史会话，也可在运行中输入 `/session <id>` 切换；ID 可由 `basework session list` 获取。切换会重建运行服务、任务归属和事件订阅，失败时保留当前会话。尚无独立的 resume/export/search 子命令。

会话数据目录默认是 `~/.local/share/basework/sessions/`（遵循 XDG 数据目录）；当前 CLI/TUI 固定使用 JSONL。SQLite 库与迁移功能存在，但配置文件里的 `session.store` 不会自动改变主运行时后端。

## 编辑与长命令

模型可调用 `edit_files` 的 preview/commit/rollback，以及 `bash_background`、`job_list`、`job_output`、`job_cancel`。预览不写盘，提交重新核对内容基线；后台任务按会话 owner 过滤。

已知限制：跨文件提交不是全局事务；撤销遇到用户二次编辑会逐文件跳过；历史输出能否继续读取取决于输出文件是否仍存在。审批、事实和会话恢复的当前证据见 [任务看板](../TASKS.md) 及其证据记录。

## 数据迁移与其他命令

```bash
basework migrate sessions --help
basework permission --help
basework auth --help
basework profile --help
basework --help
```

执行迁移前先看 [迁移指南](migration.md)，核对源目录、目标与备份。权限不等于操作系统沙箱；profile 是性能分析入口，不是启动预设。其他子命令及参数直接以当前 help 查询，避免照抄历史 Phase 文档。
