# pkg/provider

## 用途

Provider 工厂：把「用哪家模型」这件事收敛成一处创建入口，屏蔽各家协议差异。

- `Create(Config)` — 按 `Type` 创建模型客户端。
- `DefaultRegistry()` / `Registry` — 已登记 provider 的集合。
- 内置实现：OpenAI Chat、OpenAI Responses、Anthropic、Gemini、OpenAI-compatible，以及 Azure、Bedrock、Copilot、
  Ollama、OpenCode 的专用 provider。
- `ListPlugins` / `RegisterPlugin` / `GetPlugin` / `CreateFromPlugin` — 第三方 provider 插件。
- Prompt 缓存支持见同包的 `cache.go`。

## 配置

```go
provider.Config{
    Type:    "openai" | "responses" | "openai-responses" | "agnes-responses" | "anthropic" | "gemini" | "openai-compat" | <插件名>,
    APIKey:  "...",
    BaseURL: "...",        // 可选，自定义端点
    ModelID: "...",
    Options: map[string]any{...}, // provider 特定开关
}
```

专用 provider 另有各自的 `*ConfigOpts`（`AzureConfigOpts`、`BedrockConfigOpts`、
`CopilotConfigOpts`、`OllamaConfigOpts` 等）。

## 扩展点

- **新增内置 provider**：实现 `Provider` / `ModelProvider` 接口并在 registry 登记。
- **外部插件**：实现 `ProviderPlugin` 后 `RegisterPlugin`，再用 `CreateFromPlugin` 创建。
- **自定义端点**：多数场景不需要写代码，`Config.BaseURL` + `Type: "openai-compat"` 即可。
- **Responses API**：`responses` / `openai-responses` 使用 OpenAI Responses API；`agnes-responses`
  默认指向 `https://apihub.agnes-ai.com/v1`，读取 `AGNES_API_KEY`，适合 `agnes-3.0-flash`。
  该适配器把完整历史作为 `input` 重放，并把 function call/output 与 Responses SSE 事件归一化为
  `llm.Model` 的工具调用和流式事件。

## Model Experience

适配器把统一消息与工具定义转为协议请求。工具、图片、流式等支持情况因模型/端点而异；价格、免密钥要求与目录声明不能替代真实验证。

## Known Limitations

- `golang.org/x/oauth2` **未做 build tag 隔离**，默认构建即引入，而它只在 Copilot 设备授权
  时用得到。见 [docs/adr/0002](../../adr/0002-decouple-pkg-from-internal.md)。
- `Config.Options` 是 `map[string]any`，**弱类型且无 schema 校验**：键名写错不会报错，
  只会静默失效。
- 各家 tool call 的流式语义已在本包**归一化**：`Complete=false` 一律是参数增量片段、
  `Complete=true` 是完整参数，完成事件按 `Index` 升序发出。契约见
  [pkg/llm 契约](llm.md#流式工具调用参数契约)，回归测试见
  `pkg/provider/stream_toolcall_regression_test.go`。Gemini 不提供增量阶段，只发一次性完整参数。
- 上述归一化基于本地 mock 与各家协议文档，**未用真实 Provider 端到端验证**；
  真实模型兼容性矩阵属 REL-003 的范围。
- 模型清单与能力探测不在本包，分布在 `cmd/basework/model.go`、`cmd/basework/runtime.go`。
