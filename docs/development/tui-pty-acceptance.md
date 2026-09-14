# tests/tui_pty — 终端级 TUI 验收夹具（SHIP-003）

> **2026-09-14 复审**：下面保留历史运行方式用于定位；当前不能从干净目录直接复跑，QA-001 待修。全新 SHIP003_ROOT 运行 scenario1 会因缺少 fixtures 退出；fake_openai.py 也从该临时根读取。scenario5 的 examples 路径少一级，GO_ROOT 硬编码。详见 [复审报告](evidence/REVIEW-2026-09-14.md) A13/A14。旧临时目录成功不构成新 checkout 的验收。


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

## 运行方法

```bash
# 1) 编译被测二进制（sqlite memory 与产品发布口径一致）
go build -tags "sqlite memory" -o /private/tmp/ship003/bin/basework ./cmd/basework

# 2) 跑单个场景（先起本地假 OpenAI SSE 服务，再驱动 TUI）
python3 tests/tui_pty/scenario1.py
# 全部场景
for i in 1 2 3 4 5; do python3 tests/tui_pty/scenario$i.py || break; done
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
- `fixtures/` — 每个场景的独立最小 Go 项目（缺陷态出发，保证可复现）。

## 已知环境边界

- `harness.GO_ROOT` 硬编码本机真实 go1.26.0 SDK 路径（`/usr/local/go` 实为
  1.24.10，且 `GOTOOLCHAIN=auto` 在隔离 HOME 下会命中 golang.org/dl 桩）。
  其他机器需按注释调整。
- Windows 无 pty，本夹具不适用；Windows 上的 TUI 行为未验证。
