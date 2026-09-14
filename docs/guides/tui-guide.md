# TUI 使用指南

核对基线：2026-09-14 / bb455ff。本文是终端按键、任务卡片与审批交互的唯一使用说明；实现缺口见 [当前状态](../STATUS.md)，开发清单见 [终端任务](../tasks/07-terminal.md)。

## 启动

```bash
basework tui
basework tui --preset coding
basework tui --preset readonly
basework tui --no-tui
basework tui --help
```

模型与端点从配置解析，使用全局 `--config` 指定配置文件。当前 TUI 没有 `--theme` 或 `--resume` 参数；简单 REPL 的回退参数属于 `tui` 子命令。恢复已有会话和交互切换尚待 CTX-003/UI-003 接线。

## 主界面按键

| 按键 | 当前动作 |
|---|---|
| Enter | 发送输入；流式运行时不接受另一条提交 |
| 左/右、Home/End、Backspace/Delete | 输入框光标与删除 |
| Ctrl+P | 打开命令面板 |
| Ctrl+J | 打开/关闭后台任务卡片 |
| Ctrl+C / Ctrl+D | 在主界面退出程序并触发清理 |
| Esc | 关闭正在处理该按键的任务卡片或审批弹窗 |

Ctrl+C 当前没有“只取消这一轮并留在界面”的语义。模态弹窗和任务面板优先消费按键，必要时先按 Esc 返回主界面。取消与退出分离由 RUN-003/UI-003 跟踪。

注册的内置命令为 `/help`、`/clear`、`/theme`、`/quit`、`/config`；其中 help/config 没有完整内容展示，/quit 当前只调用清理而未向程序发送退出消息，可靠退出使用主界面的 Ctrl+C/Ctrl+D。未注册 /session、/model switch、/exit；不要按旧指南输入这些命令。

## 后台任务卡片

- 状态用文字区分 queued、running、succeeded、failed、canceled、timed_out、interrupted。
- ↑/↓ 选择任务，Enter/o 查看输出，[/] 翻页，x 请求取消所选运行中任务，Esc 关闭。
- 当前产品读取 stdout；没有 stderr 切换入口。列表截取前 50 项，显示每页最多 30 行。
- **已知问题**：超过 30 行但不足一页字节上限的输出会出现不可访问的后半段，反向翻页游标也不准确；UI-001 修复前，需用 CLI/job_output 的偏移读取核对完整输出。
- 历史列表能保留状态与引用，不保证进程结束后输出文件或内存输出仍可读取。当前重启不能自动把旧 owner 的任务归并为 interrupted，不能把旧 running 当作仍可取消的活任务。

## 权限审批

启用权限并使用 `permission.mode: "interactive"` 时，未被规则放行的调用会进入审批。当前组件支持：

| 按键 | 动作 |
|---|---|
| y / Enter | 允许本次请求 |
| n / Esc | 拒绝本次请求 |
| ↑/↓ | 滚动预览 |

超时默认 120 秒，未响应则拒绝；决定按请求 ID 区分，交互决定不缓存为永久规则。

**当前信息边界**：预览主要来自工具参数。edit_files commit 只有 plan_id，尚不能展示该计划的真实路径和 diff；不能据空预览认为没有文件修改。UI-002 将接通真实计划详情，提交仍须检查权限和内容基线。

## 会话与恢复

每次启动当前都会创建新会话。事件可持久化和查询，但尚无用户可用的“选择旧会话继续”命令。App 的 SwitchSession 只是内部方法，且尚有旧消息/忙状态/迟到响应未隔离的问题。

恢复面板组件已经存在，但事实持久化和旧 owner 绑定未完整接线；出现“切换到会话”提示不能证明旧历史已恢复。面板有内容时，终端宽度 11–23 列可触发崩溃，UI-001/UI-003 待修。

## 验收与问题定位

[PTY 验收说明](../development/tui-pty-acceptance.md) 给出夹具现状与待补入口。恢复、审批、分页、取消与窄窗必须从真实产品路径验证；[本轮复审](../development/evidence/REVIEW-2026-09-14.md) 保存可重复失败样例。脚本化模型验证产品行为，真实模型兼容性与真人主观体验另行记录。
