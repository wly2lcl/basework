# pkg/hook

## 用途

生命周期钩子与进程内事件总线，是 agent 与外部逻辑解耦的主要手段。

- **`Hook` / `ExtendedHook`** — 在 `BeforeTool` / `AfterTool` / `BeforeLLM` / `AfterLLM`
  以及扩展的 step 级时机（`EventPreStep` 等）插入逻辑。`NopHook` / `NopExtendedHook`
  提供空实现，嵌入即可只覆写关心的那一个。
- **`Chain`** — 串行执行多个 hook，顺序即注册顺序。
- **`Broker[T]`** — 泛型发布订阅，用于跨模块广播事件。
- **`PermissionHook`** — 基于 `Rule` 的权限判定，无规则命中时回调 `onAsk` 询问调用方。

## 配置

无包级配置。权限规则通过 `NewPermissionHook(rules, onAsk)` 构造，`Rule` 与 `Effect`
（`allow` / `deny` / …）在调用方组装。

## 扩展点

- 嵌入 `NopHook` 或 `NopExtendedHook`，只覆写需要的方法。
- 用 `FuncHook` / `FuncExtendedHook` 把单个函数适配成 hook，适合一次性逻辑。
- 新增 hook 点：扩展 `ExtendedHook` 与 `EventType`（会改动接口，属于核心 API 变更）。

## Model Experience

Hook 可拦截模型请求与工具执行；拒绝必须让调用方能辨认原因。PubSub 用于观察，不能用瞬时通知替代需要重启后恢复的会话事实。

## Known Limitations

- **`BeforeLLM` 可以任意重写消息列表，且不留痕。** 这是「模型看到的 ≠ 日志记的」的
  来源之一：一旦装上有改写行为的 hook，`agent.CheckRequestInvariant` 必然失败，
  这是设计上的必然，不是 bug。
- `Chain` 是串行同步的：任一 hook 阻塞会拖住整条 agent 流水线。
- `PermissionHook` 的 `onAsk` 是同步回调，交互式确认会阻塞调用线程直到用户应答。
