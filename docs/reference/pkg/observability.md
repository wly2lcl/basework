# pkg/observability

## 用途

运行时可观测性的四个小件：结构化日志、成本统计、token 统计、事件总线，以及可选的 pprof。每个都是独立实例，不依赖 `pkg/agent`，可单独嵌入。

- **`Logger`** — JSON Lines 输出（`timestamp`/`level`/`message`/`fields`），级别 `debug < info < warn < error`，`SetLevel`/`SetOutput` 可运行时调整。并发写有互斥。
- **`CostTracker`** — 按模型单价估算每次调用成本，累计总成本与按模型成本；未知模型按 `$1.0/百万 token` 兜底。`Reset` 清空。
- **`TokenTracker`** — 记录输入/输出 token，提供总量与按模型查询。
- **`EventBus`** — 发布-订阅；`Publish` **异步、不阻塞发布者**，每个 handler 一个 goroutine，并发上限 100，超出时丢弃事件并写一条 `EventBus handler dropped` 日志。handler panic 被 recover 并打印堆栈，不影响其它订阅者与 Agent 主流程。
- **`SetupSubscribers`** — 按 `Config` 一次性接好日志级别/输出与 `LogSubscriber`/`MetricsSubscriber`；`Enabled=false` 时什么都不做。
- **`Profiler`** — 在 `127.0.0.1:6060` 暴露 `net/http/pprof`；默认不启动，必须显式 `Start`。

## 配置

| 项 | 说明 |
|---|---|
| `Config.Enabled` | 是否装配订阅者；`false` 时 `SetupSubscribers` 直接返回 |
| `Config.LogLevel` | `debug`/`info`/`warn`/`error`，交给 `ParseLogLevel` |
| `Config.LogOutput` | `stdout`/`stderr`/文件路径；文件打不开时告警并保持原输出 |
| `NewLogger(level, output)` | 直接构造；`output` 为 nil 时用默认输出 |
| `NewCostTracker()` / `NewTokenTracker()` | 独立实例，内部有锁 |
| `NewEventBus()` | 每个产品/测试实例独立；并发上限固定 100 |
| `ProfilerConfig{Host, Port}` / `DefaultProfilerConfig()` | 默认 `127.0.0.1:6060` |
| `SetupSubscribers(config, bus, logger, costTracker, tokenTracker)` | 组装入口 |

事件类型常量：`agent.start`、`agent.end`、`agent.error`、`tool.start`、`tool.end`、`tool.error`、`tool.timeout`、`llm.call.start`、`llm.call.end`。

## 扩展点

- **订阅者**：实现 `Subscriber`（`Handle(Event) error` + `EventTypes() []string`），或用 `bus.Subscribe(type, handler)` 直接注册函数。
- **日志去向**：`Logger.SetOutput(io.Writer)` 可接文件、buffer 或测试 writer。
- **计价**：`CostTracker.RecordCall(model, in, out, cost)` 允许调用方传已算好的成本；`GetCallCost` 仅做估算。
- **pprof**：`ProfilerConfig` 可换绑定地址/端口；生产环境默认关闭。
- **接口桥接**：需要 `agent.EventPublisher` 时，按 `internal/observability/adapter.go` 的 8 行适配即可；核心包不依赖 `pkg/agent`。

## Model Experience

- 事件与日志记录模型调用与工具调用的开始/结束/错误，模型行为异常时能按时间线复盘，而不是只有最终答复。
- token 与成本统计按模型分桶，长会话可看到增长来源；这是运维指标，不进入模型上下文，不会挤占上下文预算。
- 事件投递异步且有界：观测慢或订阅者卡住不会拖慢模型回合；事件被丢弃时日志里能查到，而不是静默消失。

## Known Limitations

- **事件会丢**：并发 handler 达到 100 时新事件直接丢弃并记日志；这是为避免阻塞主流程的刻意取舍，不适合当作审计日志。
- 订阅者状态不隔离：`EventBus` 只负责投递，订阅者若共享可变状态需自行加锁；`SetupSubscribers` 不保证重复调用幂等。
- `CostTracker` 的价格表是内置常量，未知模型用统一兜底价，不能替代 Provider 账单。
- `Profiler` 绑定明文 HTTP，无鉴权；只能用于本机/受信网络，默认不启动。
- `SetupSubscribers` 打开日志文件后不负责关闭，生命周期由调用方管理。
- 本包是**实验 API（0.x）**（见 [ADR 0008](../../adr/0008-promote-internal-capabilities-to-pkg.md)）：1.0 前允许破坏性调整，但会走 ADR 并记录迁移方式。
