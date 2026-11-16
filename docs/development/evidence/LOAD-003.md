# LOAD-003 验证记录：JSONL 跨进程锁公平性修复

## 基线

- 日期：2026-11-16；操作者：Qoder AI 会话；系统：macOS arm64；Go 1.26.11（与 SEC-003 同工作区）。
- 起点 HEAD：`ed269c2`；失败现场为 CI run `43558316429` 的 `Test (windows-latest)`。
- 失败形态（CI 日志原文）：8 个并发追加子进程中 3 个报 `session: 获取会话追加锁失败: session: 获取锁超时（5s）`（各耗时约 5.27s），父测试断言 `多进程 JSONL 序列不连续：len=764`（期望 1024）。失败是锁获取超时，不是序号错乱或写入损坏。

## 变更

- 根因：`pkg/session/lock_unix.go` 与 `lock_windows.go` 的非阻塞锁轮询均为固定 `time.Sleep(100ms)`。持锁进程在 `AppendEvent` 释放锁后立即发起下一次追加并再次抢锁，等待者仍在睡眠，形成护航效应（convoy）；Windows runner 文件操作慢（Defender 实时扫描），且每次追加在锁内回读外部变化（`refreshIfChanged`），持锁窗口长，等待者约 50 次轮询全部落空直至 5s 超时。Unix 同样存在该缺陷，只是文件操作快、概率低。
- 修复：新增 `pkg/session/lock_backoff.go`，两平台共用 `lockBackoff`：1ms 起步、每次翻倍、封顶 50ms，睡眠时长取 `[delay/2, delay]` 区间随机抖动（`math/rand/v2`）。两处 `time.Sleep(100 * time.Millisecond)` 替换为 `backoff.wait()`。
- 不变量：锁互斥语义（flock / LockFileEx 排他字节锁）、5s 超时上限、锁文件布局（Windows 侧 `.lock` 旁文件）、JSONL 格式与序号分配均未改动；`jsonl.go`、`tests/load/load_test.go` 只读未改。

## 验证

| 检查 / 场景 | 命令或操作 | 实际结果 | 证据与边界 |
|---|---|---|---|
| 双平台编译 | `go build ./...`、`GOOS=windows GOARCH=amd64 go build ./...` | 均通过 | 交叉编译非运行结论 |
| session 包测试 | `go test ./pkg/session/ -count=1` | ok（0.576s） | |
| 多进程负载重复 | `go test ./tests/load -run 'TestProjectLoad/jsonl-multiprocess-and-tail-recovery' -count=10 -v` | 10/10 PASS，单次 0.17–0.30s | 修复前同机该子测试为秒级；Windows 运行结论待 CI |
| race 定向 | `HOME=$(mktemp -d) go test -race -tags 'sqlite memory' ./pkg/session ./tests/load ./internal/runtime ./cmd/basework -count=1 -p 1` | 全部 ok（session 2.701s、load 4.734s） | |
| 默认/完整 tags 全量 | 与 SEC-003 同一轮回归 | 均全部通过（exit 0） | 见 SEC-003 证据 |
| 远端 Windows runner | 推送后触发 CI | 未运行 | 需用户授权推送 |

## 剩余与交接

- 本机为 darwin/arm64，无法本地复现 Windows runner 的慢文件操作环境；修复正确性由锁语义不变 + 本地重复负载 + 退避公平性推理支撑，最终运行结论以推送后 CI `Test (windows-latest)` 为准。
- 未改动 `refreshIfChanged` 的全量回读与 5s 超时上限；若 CI 仍超时，下一步候选是锁内增量尾部回读（需另立任务，不在本卡内扩大范围）。

## 结论

- 根因有 CI 日志与代码路径支撑，修复最小且不改锁语义；本地验收项通过。
- 建议状态：待验证（解除条件：推送后远端 CI Windows runner `tests/load` 通过、五平台测试全绿）。