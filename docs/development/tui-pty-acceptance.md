# tests/tui_pty — 终端级 TUI 验收夹具（SHIP-003）

> **2026-09-15 复核**：夹具已从当前 checkout 构建二进制，fixtures 固定从仓库读取，
> 默认使用随机临时根；可用 `SHIP003_ROOT` 保留报告。脚本化模型证据与真实 Provider
> 证据分开，当前任务状态仍以 [TASKS](../TASKS.md) 为准。


这是**验收工具，不是 `go test` 测试**：用 PTY 在真实终端里启动编译出的
`basework tui`，投递按键、抓屏、断言路线图的代表性业务场景。`go test`
不运行它们（它们依赖 python3 + pty + 真实终端语义，且耗时以分钟计）。

## 场景与路线图对应

| 脚本 | 路线图代表性场景 |
|------|------------------|
| scenario1.py | 修复一个缺陷（流式输出 / 工具卡片 / 差异 / 测试结果可见） |
| scenario2.py | 执行耗时检查（后台任务可见 / 输出可读 / 取消后子进程退出 / 结果可回看） |
| scenario3.py | 中断后继续（SIGKILL 硬杀 → 重启恢复 / 遗留任务标 interrupted / 不自动重跑） |
| scenario4.py | 使用不同模型（能力来源明确 / 缺密钥明确告知 / 工具不被静默关闭） |
| scenario5.py | 嵌入 Go 服务（examples/embed 公共 API 组装，见上两级目录） |
| scenario6.py | 运行中 `/session <id>` 切换历史会话（历史恢复 / 消息隔离 / 未知 ID 拒绝 / 无残留） |
| scenario7.py | 交互权限审批（真实路径与预览 / 拒绝不改盘 / 批准后提交） |
| scenario8.py | 真实压缩配置（两次压缩 / 关闭重启 / 同 ID 继续 / 历史请求断言） |

## 运行方法

```bash
# 1) 编译被测二进制（sqlite memory 与产品发布口径一致）
go build -tags "sqlite memory" -o /private/tmp/ship003/bin/basework ./cmd/basework

# 2) 跑单个场景（先起本地假 OpenAI SSE 服务，再驱动 TUI）
python3 tests/tui_pty/scenario1.py
# 全部场景
for i in 1 2 3 4 5 6 7 8; do python3 tests/tui_pty/scenario$i.py || break; done

# 推荐：每个场景独立临时根，一条命令运行并在结束时清理
python3 tests/tui_pty/run_all.py
# 失败时保留报告，便于复盘
python3 tests/tui_pty/run_all.py --keep
# 只复跑指定场景
python3 tests/tui_pty/run_all.py --scenarios 3,8

# QA-001 负向门禁：临时副本移除 session 绑定后，scenario6 必须失败
python3 tests/tui_pty/mutation_check.py
```

每次运行会重建 `fixtures/` 的隔离副本、写独立 HOME，并把抓屏快照、原始
终端字节流、断言结果落在 `<ROOT>/runs/`（默认 `/private/tmp/ship003/runs`，
用 `SHIP003_ROOT` 改写），不污染仓库。

## 组成

- `fake_openai.py` — 脚本化 OpenAI 兼容 SSE 服务：按会话消息里的 `[Sx]`
  标记选脚本，按已完成工具轮次推进；带人为延迟，让「流式增量 → 工具卡片 →
  最终答复」的中间态真正可观察。
- `tui_drive.py` — PTY 驱动 + 精简 VT 解析器（CSI/OSC/UTF-8 跨块），把
  终端字节流还原成字符网格供断言。
- `harness.py` — 隔离 HOME、假服务生命周期、断言报告、Go 工具链环境
  （见下）。
- `run_all.py` — 以独立子进程和独立临时根编排 scenario1–8；默认自动清理，
  `--keep` 或 `--root` 用于保留证据。
- `mutation_check.py` — 在临时副本移除 session 绑定接线，确认 scenario6 的恢复断言
  会以非零退出，防止验收被宽松提示掩盖。
- [真实 Provider 验收夹具](real-provider-acceptance.md) — 可选真实模型编码夹具；凭据只从环境变量注入，结果结构脱敏。
- `fixtures/` — 每个场景的独立最小 Go 项目（缺陷态出发，保证可复现）。

## 已知环境边界

- `GO_ROOT` 默认通过 `go env GOROOT` 发现，也可用 `SHIP003_GO_ROOT` 显式指定；
  夹具把 `GOTOOLCHAIN` 固定为 `local`，避免隔离 HOME 触发自动下载。不同机器
  仍需提供可用的 Go SDK 和 Python PTY 环境。
- Windows 无 pty，本夹具不适用；Windows 上的 TUI 行为未验证。
