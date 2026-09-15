# TUI 使用指南

核对基线：2026-09-15 / 当前工作区。本文是终端按键、任务卡片、审批和会话交互的唯一使用说明；发布边界见 [当前状态](../STATUS.md)，开发清单见 [终端任务](../tasks/07-terminal.md)。

## 启动

```bash
basework tui
basework tui --preset coding
basework tui --preset readonly
basework tui --no-tui
basework tui --session <session-id>
basework tui --help
```

模型与端点从配置解析，使用全局 `--config` 指定配置文件。`--session <id>` 会在启动时绑定已有会话；运行中可输入 `/session <id>` 切换，先用 `basework session list` 查找 ID。切换成功后会重建该会话的运行服务、任务归属和事件订阅，并加载恢复面板。

## 主界面按键

| 按键 | 当前动作 |
|---|---|
| Enter | 发送输入；流式运行时不接受另一条提交 |
| 左/右、Home/End、Backspace/Delete | 输入框光标与删除 |
| Ctrl+P | 打开命令面板 |
| Ctrl+J | 打开/关闭后台任务卡片 |
| Ctrl+C / Ctrl+D | 流式运行中取消当前轮；空闲时退出程序并触发清理 |
| Esc | 关闭正在处理该按键的任务卡片或审批弹窗 |

模态弹窗和任务面板优先消费按键，必要时先按 Esc 返回主界面。

注册的内置命令为 `/help`、`/clear`、`/theme`、`/quit`、`/config`、`/session <id>`。`/session` 需要已有会话 ID；无 ID 或不存在时保留当前会话并显示错误。`/quit` 触发清理，可靠退出仍可使用主界面的 Ctrl+C/Ctrl+D。

## 后台任务卡片

- 状态用文字区分 queued、running、succeeded、failed、canceled、timed_out、interrupted。
- ↑/↓ 选择任务，Enter/o 查看输出，[/] 翻页，x 请求取消所选运行中任务，Esc 关闭。
- 当前产品读取 stdout；没有 stderr 切换入口。列表截取前 50 项，显示每页最多 30 行。
- 历史列表能保留状态与引用；重启后遗留的 running 任务会归并为 interrupted，并在恢复面板标记为需要重试。

## 权限审批

启用权限并使用 `permission.mode: "interactive"` 时，未被规则放行的调用会进入审批。当前组件支持：

| 按键 | 动作 |
|---|---|
| y / Enter | 允许本次请求 |
| n / Esc | 拒绝本次请求 |
| ↑/↓ | 滚动预览 |

超时默认 120 秒，未响应则拒绝；决定按请求 ID 区分，交互决定不缓存为永久规则。

审批请求展示工具目的、真实路径、计划 diff 和风险依据；批准后仍会重新检查权限与内容基线。拒绝、关闭、超时和过期回答都不会提交编辑。

## 会话与恢复

启动省略 `--session` 时创建新会话；指定 ID 或使用 `/session <id>` 时继续已有会话。切换会清空旧消息、取消当前轮、重建任务卡片并按会话过滤迟到事件。恢复面板来自落盘任务、文件事实和验证命令；`interrupted`/`failed` 会标为需要重试或复核。

## 验收与问题定位

[PTY 验收说明](../development/tui-pty-acceptance.md) 给出可复跑的产品入口场景。脚本化模型验证产品行为，真实模型兼容性、跨平台安装和真人主观体验仍按发布任务单独记录。
