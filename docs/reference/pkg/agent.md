# pkg/agent

## 用途

Agent 循环与请求组装，是 `pkg/` 的门面层：把 LLM、工具、会话、钩子串成「四阶段 turn 管线」。

```
setupTurn ──► callLLM ──► executeTools ──► finalize
 (组装请求)   (调模型)    (跑工具)         (汇总结果)
```

- **请求组装** — `BuildRequestMessages(events)`：从事件日志重建将要发给 provider 的消息
  列表，顺序为最新 system prompt、按写入顺序累积的全部 steering、再到历史投影。
  已落盘的 steering 会进入后续每次请求；它不是只消费一次的临时提示。
- **请求审计** — `RequestFingerprint`（messages + tools 的稳定哈希）与
  `CheckRequestInvariant`（深比较完整 `ChatMessage`，校验实际请求与日志重建结果）。每次请求尝试写入
  `request.built`；该事件写入失败只记日志并继续发送。校验器当前未接入运行时。
- **上下文治理** — `Compactor` / `CompactReporter`（可选扩展）、`TruncateMessages`、
  `ShouldCompact`、`CompactSummary`、`ContextCollector`。
- **运行期交互** — `SteeringManager`（运行中插话）、`Callback`（流式回调）、
  `Observer`（观测）、`Plugin`（生命周期挂钩）、模板渲染。

## 配置

一律通过 Option 模式注入，不修改接口签名：

| 分组 | Option |
|---|---|
| 模型与提示 | `WithModel`、`WithSystemPrompt` |
| 工具 | `WithTools`、`WithToolRegistry`、`WithToolFactory` |
| 会话 | `WithSession` |
| 行为 | `WithMaxSteps`、`WithCompactor`、`WithLoopDetector`、`WithSteeringManager` |
| 观测与权限 | `WithObserver`、`WithCallback`、`WithPermissionChecker`、`WithEventBus` |
| 扩展 | `WithHook`、`WithPlugin`、`WithSubAgentRunner` |

## 扩展点

- **自定义压缩**：实现 `Compactor`；想同时拿到摘要文本就再实现 `CompactReporter`
  （可选扩展接口，实现与否都不破坏签名）。
- **生命周期逻辑**：实现 `Hook`（见 [pkg/hook](hook.md)）或 `Plugin`。
- **流式 UI**：实现 `Callback` 接收文本增量与工具起止。
- **额外上下文**：实现 `ContextProvider` 并交给 `NewContextCollector`。
- **注入动态工具**：实现 `ToolFactory`。

## Model Experience

模型收到 system prompt、steering 与历史投影；工具列表来自注册表。调用方从流式回调和最终响应读取结果。重建校验的限制见下节，不能把指纹存在当作请求完整记录的保证。

## Known Limitations

- **请求重建不覆盖调用方与 Provider 的所有变换。** `BeforeLLM` hook 改写消息后，
  `CheckRequestInvariant` 会报告差异；Provider 传输层在调用后再做的改写也不在本包可重建范围内。
  校验器用于测试和诊断，尚未接入运行时阻断策略。
- **上下文事件的持久化失败会阻断本轮 Provider 调用。** `system.prompt_set` 写入失败时，
  不提交 prompt 已记录状态，以便后续重试；`steered` 写入失败时，本轮不调用 Provider，
  并把已 Drain 的消息放回队列以便后续重试。`request.built` 仍是尽力记录：写入失败仅记日志，
  不阻断请求，因此不能保证每次请求都有对应审计事件。
- **Steering 会持续累积并重放。** 每次请求都会重放日志里的全部 steering，当前没有过期、清除或
  只重放最近一轮的策略；反复插话会增加后续请求上下文长度。
- **每步都会重新读取并投影全量事件日志**（`setupTurn` 一次、压缩判断一次、循环检测一次），
  长会话下存在 O(事件数) 的重复开销。上下文压缩是主要缓解手段。
- **一个 `AgentLoop` 绑定一个 sessionID**：多会话需要多个实例，实例之间不共享状态。
- `Agent` 接口属于核心 API（SemVer 严格兼容），只能通过 Option 扩展，不能改签名。
- 请求指纹覆盖 messages + tools 的**内容**；若 provider 在传输层再做改写（例如自行裁剪
  历史），指纹无法反映，需要在该 provider 内单独记录。
