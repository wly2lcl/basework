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
