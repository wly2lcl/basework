# pkg/llm

## 用途

全仓共享的 LLM 词汇表与客户端，是框架的最底层（L0），不依赖任何其他 `pkg/` 包。

- **类型系统** — `ChatMessage` / `ContentPart` / `ToolCall` / `ToolDefinition` / `Request` /
  `Response` / `Usage` / `Capability`，以及 `Model` 接口。全仓所有对话数据都走这一套类型。
- **错误分类** — `Error` + `ErrorType`，配合 `IsRateLimit` / `IsContextOverflow` 做判定，
  供重试策略与上下文压缩决策。
- **多模态图片** — `LoadImage` / `LoadImageFromReader` / `ImageData`，以及按 provider
  转成各家协议格式的 `ImageContentForProvider`。
- **带重试的客户端** — `NewClient(model, opts...)`，把重试与事件发布收在一处。
  流式调用不重试（见下方契约），只有 `Generate` 走重试。

## 流式工具调用参数契约

`ToolCallDelta.ArgsJSON` 的语义由 `Complete` 决定。**归一化由各 Provider 负责**，
消费方不需要按 Provider 猜测方言：

| `Complete` | `ArgsJSON` 的含义 | 消费方应如何用 |
|---|---|---|
| `false` | 自上次同 `Index` 事件以来的**参数增量片段** | 按 `Index` 分组、按到达顺序拼接 |
| `true` | **完整参数**，唯一权威值 | 用该值覆盖拼接结果 |

- 同一个 `Index` 在一次响应内只出现一次 `Complete=true`。
- `Complete=true` 的事件按 `Index` 升序发出，与运行次数无关，可复现。
- 增量事件不得携带累计值；发出累计值会让按序拼接的消费方重复拼接。
- 空文本增量（`content: ""`）不产生 `StreamEventText`。
- 流式请求不做重试，断流只补齐当前工具调用，不会重新发起请求导致有副作用的执行重复。

回归测试见 `pkg/provider/stream_toolcall_regression_test.go` 与
`pkg/agent/pipeline_stream_order_test.go`，实施记录见
[REL-001 证据](../../development/evidence/REL-001.md)。

## 能力结论契约

`Model.Supports(cap) bool` 保留原样（`false` 只表示"没有声明支持"），
需要区分**支持 / 不支持 / 未知**时用 `DescribeCapability(model, cap)`：

| `Support` | 含义 | 调用方应如何用 |
|---|---|---|
| `SupportSupported` | 已确认支持 | 可以依赖 |
| `SupportUnsupported` | 已确认不支持 | 应给出明确错误与可选替代 |
| `SupportUnknown` | 没有依据（未声明也未实测） | **不得当作支持**，按需报错或降级 |

- 实现 `CapabilityReporter` 可报告带来源（`declared` / `configured` / `probed`）的结论；
  未实现的模型由 `DescribeCapability` 退回 `Supports()`，`false` 一律解释为 `Unknown`
  而不是 `Unsupported`——`bool` 的 `false` 不构成"不支持"的证据。
- `CheckRequiredCapabilities` 在能力为 `Unsupported` **或** `Unknown` 时都返回
  `*MissingCapabilityError`，错误里带结论、来源与可选做法。它不会静默禁用能力或换模型。
- 结论来源目前只有 `declared`（静态声明）。`probed`（真实请求实测）尚无写入方。

回归测试见 `pkg/llm/capability_test.go` 与 `pkg/provider/capabilities_test.go`。

## 配置

本包不读配置文件；配置由 `pkg/config` 解析后由调用方注入。可配置项：

| 方式 | 内容 |
|---|---|
| `ClientOption` | `WithRetry(RetryConfig)`、`WithRetryEnabled(bool)`、`WithEventBus(EventBus)` |
| 常量 | `MaxImageWidth` 等图片尺寸上限（超出会等比缩放）、`CacheControlEphemeral` |

## 扩展点

- **接入新模型后端**：实现 `Model` 接口即可，`pkg/provider` 就是这么做的。
- **接收客户端事件**：实现 `EventBus` 接口。
- **扩展客户端行为**：新增 `ClientOption` 函数，不修改既有签名。

## Model Experience

统一消息、工具定义与流事件是模型适配契约。消费者需处理空文本、只有工具调用的回复、部分参数、取消与错误；不能假设每次响应都有普通文本。

## Known Limitations

- `golang.org/x/image` **未做 build tag 隔离**，默认构建即引入，即使完全不使用多模态输入。
  见 [docs/adr/0002](../../adr/0002-decouple-pkg-from-internal.md)。
- `ImageContentForProvider` 只覆盖已收录的 provider 的图片编码格式，遇到未收录的 provider
  返回错误，不会静默降级为纯文本。
- 错误分类依赖各家返回的状态码与文本特征，属启发式判定，不是协议保证。
