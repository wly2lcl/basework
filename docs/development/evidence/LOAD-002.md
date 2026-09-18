# LOAD-002：项目级并发与持续稳定性门禁

**状态**：完成（2026-09-18）

## 基线

PROJECT-LOAD-001 只覆盖单次本地负载；LOAD-002 将驱动器和持续稳定性门禁纳入仓库。Provider key、模型质量和外部渠道容量不在本任务的基线中。

## 变更

新增 `tests/load/load_test.go`、主 CI 短门禁和 `.github/workflows/project-load.yml`。驱动器固定 seed、端点说明和 fixture hash，使用确定性进程内模型；子进程具备超时和回收路径，证据 JSON 不保存敏感配置或临时绝对路径。

## 验证

项目级负载驱动器已纳入仓库，并通过同进程并发、取消后重启、JSONL 多进程追加/截断尾恢复、有界订阅和 10,000 次本地 soak。测试使用仓库真实的 `pkg/agent`、`internal/runtime.LocalService`、`pkg/session.JSONLStore` 和回调/事件链路；模型是进程内确定性 `llm.Model`，没有使用外部 Provider key，因此结果只用于判断 Basework 自身的负载与生命周期边界。

这组结果证明当前夹具和环境下项目负载门禁通过，不等同于模型质量、真实 Provider 配额、跨机器生产 SLO 或真人 TUI 手感验收。外部 Agnes 渠道容量继续由 [LOAD-001](LOAD-001.md) 单独记录。

## 可复跑入口

```bash
# 短门禁（默认 256 次 soak，约数秒）
go test ./tests/load -run TestProjectLoad -count=1 -timeout 10m -v

# 完整 LOAD-002（10,000 次 soak，生成脱敏 JSON）
BASEWORK_LOAD_SOAK_OPS=10000 \
BASEWORK_LOAD_EVIDENCE=/tmp/basework-load-002.json \
go test ./tests/load -run TestProjectLoad -count=1 -timeout 30m -v
```

固定参数：seed `basework-load-2026-09-18`；端点 `none (in-process deterministic model)`；fixture hash 记录在 JSON 顶层。结果不写入 HOME、key、端口或临时绝对路径；多进程子进程有 30 秒超时并由 `CommandContext` 回收。

## 原始脱敏统计

完整结果：[LOAD-002-local-2026-09-18.json](LOAD-002-local-2026-09-18.json)。本次本地环境为 Go 1.26.0 / macOS arm64；10,000 次 soak 用时 204.521 秒，10,000/10,000 成功，吞吐 48.895 req/s，p50 20.319 ms、p95 37.897 ms、p99 42.236 ms、最大 119.560 ms，失败分类为空，产生 60,001 个会话事件。Soak 资源起止为 goroutine 3→3、heap 376,936→13,462,488 bytes、FD 6→6、JSONL 0→14,099,195 bytes，均在测试边界内。

同进程并发档位 1/4/8/16/32 均分别完成 1/4/8/16/32 个请求；每档 session ID、run ID、callback、事件序号均通过断言，丢弃事件为 0，goroutine 和 FD 起止不增长。取消验证返回 `context.Canceled`，结束后再次 `Cancel` 返回 false，随后新 run 成功。

JSONL 验证由 8 个独立进程各追加 128 个事件组成 1,024 个连续序号；截断最后一条 JSON 后可恢复并继续追加。有界 EventBus buffer=1 的慢订阅者记录了 15 个丢弃事件，并明确要求通过持久化 session 事件重读。

## CI 与夜间任务

- 短门禁已加入主 [CI Build workflow](../../../.github/workflows/build.yml)，在默认全量测试后显式运行 `tests/load`。
- 10,000 次 soak 已加入 [Project Load Soak workflow](../../../.github/workflows/project-load.yml)，支持手动触发和每日 02:17 UTC 夜间运行；每次上传脱敏 JSON artifact。首次远端运行完成后，将在此补充运行链接和 artifact 名称。

## 验收边界

- [x] 新 checkout 可直接运行驱动器并生成脱敏 JSON；临时子进程有超时和回收路径。
- [x] 同进程 1/4/8/16/32 交错运行无 session、callback、结果串线；取消后重启成功，关闭后资源回收。
- [x] 10,000 次 soak 的 p50/p95/p99、吞吐、失败分类、goroutine、heap、FD 和 JSONL 体积起止值均有记录与边界断言。
- [x] JSONL 多进程追加/截断尾恢复和 EventBus 有界投递有机器断言。
- [x] 短门禁 CI 与手动/夜间 soak 工作流已提交；运行证据由本文件和脱敏 JSON 维护。

## 剩余与交接

夜间 workflow 会在后续运行中继续采集同一夹具的漂移数据；若需要生产 SLO，必须另行定义目标环境、Provider、请求分布和告警阈值。真人 TUI 手感和真实 Provider 容量继续按独立任务验收。

## 结论

LOAD-002 的项目级门禁在当前本地环境和确定性模型下完成验收；不将该结果推广为真实 Provider 或跨机器生产承诺。
