# RESP-001：Agnes Responses API 接入

## 基线

在本次改动前，`pkg/provider` 只有 Chat Completions、Anthropic Messages 等适配器；项目
文档明确写着 Agnes Responses 尚未接入 `llm.Model`，因此不能用 Responses 端点做真实闭环。

## 变更

让项目可以把 Agnes 3.0 Flash 的 OpenAI-compatible Responses API 作为一等
`llm.Model` 使用，覆盖非流式输出、文本流、function call、function call output、用量和
错误映射；密钥使用 `AGNES_API_KEY`，不写入配置或工具子进程。

## 实现

- `pkg/provider/responses.go` 新增 Responses 适配器，支持 `input` 历史项、图片输入、函数工具、
  `max_output_tokens`、非流式响应和 Responses SSE。
- 工具调用历史会带上 Agnes 反序列化所需的 `id`、`call_id`、`name`、`arguments` 和
  `status=completed`；工具结果使用 `function_call_output`。
- 新增 provider 类型：`responses`、`openai-responses`（OpenAI 默认端点）和
  `agnes-responses`（默认 `https://apihub.agnes-ai.com/v1`，读取 `AGNES_API_KEY`）。
- CLI 的 provider 列表、初始化默认模型、配置解释和文档均已同步。

## 验证

| 层级 | 结果 |
|---|---|
| 本地 fake Responses 服务 | `TestResponsesRequestTranslation`、`TestResponsesGenerate`、`TestResponsesStream`、`TestResponsesHTTPError` 通过 |
| 工厂/能力/密钥映射 | `TestResponsesFactoryAliases`、`TestRequiresAPIKey`、`TestAPIKeyEnvVar` 通过 |
| 真实 Agnes | `agnes-responses` / `https://apihub.agnes-ai.com/v1` / `agnes-3.0-flash`，固定编码夹具连续 3 次通过 |

三份真实脱敏结果分别为 [`run-1`](RESP-001-real-provider-2026-09-18-run-1.json)、
[`run-2`](RESP-001-real-provider-2026-09-18-run-2.json)、[`run-3`](RESP-001-real-provider-2026-09-18-run-3.json)。
每份均为 `agent_ok=true`、`validation_ok=true`、`tests_executed=true`、`file_changed=true`，
独立测试退出码为 `0`。run-1 包含一次无效工作目录命令失败，但 Agent 随后恢复并完成修复；
这不影响最终验证条件，也证明失败工具结果仍能继续走 Responses 工具循环。

## 剩余与交接

当前适配器按无状态方式每轮重放完整 `input`，不使用 `previous_response_id` 或服务端会话。
三次连续运行均使用同一协议、端点、模型和固定夹具；这组证据关闭了本任务要求的真实重复性
门禁，但不等价于所有模型、所有请求类型的成功率承诺。

## 结论

RESP-001 的协议实现、默认测试、本地 fake 服务回归和 Agnes Agent 连续三次真实闭环均已完成，
任务可标记为完成。后续只需在代码候选变化时按真实验收 workflow 重新绑定 artifact，不需要
重做适配器实现。
