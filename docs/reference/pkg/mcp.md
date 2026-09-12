# pkg/mcp

## 用途

MCP（Model Context Protocol）集成：把外部 MCP server 提供的工具、资源与提示接进 agent。

- `Manager` — 管理多个 MCP server 连接，汇总其能力。
- 传输层：`StdioTransport`（子进程）与 `HTTPTransport`（远程端点），统一为 `Transport` 接口。
- 能力对象：`ServerCapabilities`、`ReadResourceResult`、`ResourceContent`、`GetPromptResult`、
  `PromptMessage`。
- `ServerStatus` — 连接状态（如 `available`）。

## 配置

```go
mcp.NewStdioTransport(command, args, env)   // 本地 MCP server
mcp.NewHTTPTransport(url, headers)          // 远程 MCP server
```

每个 server 用 `ServerConfig` 描述，交给 `Manager` 统一管理。

## 扩展点

- **新增传输方式**：实现 `Transport` 接口（如 WebSocket、SSE）。
- **按需启用**：通过 `Manager` 只连接当前需要的 server，避免一次拉起过多外部进程。
- 远端 server 的能力由其自身 `ServerCapabilities` 声明，本包不做能力补齐。

## Model Experience

外部工具、资源和提示来自 MCP server。模型只能使用已发现并注册的能力；server 连接失败或重连期间不可把缺失能力当作成功。工具结果中的外部文本按数据处理。

## Known Limitations

- **依赖外部进程或网络端点**：stdio 传输要求命令行可执行文件存在，HTTP 传输要求端点可达。
  两者不可用时对应能力直接缺失，不会降级。
- 首次调用可能较慢：stdio server 需要启动子进程并完成 MCP 握手。
- 子进程的生命周期（僵尸进程、异常退出）需要调用方通过 `Manager` 显式关闭来保证。
- 本包只做协议适配，**不校验远端返回内容的可信度**；接入第三方 MCP server 时需自行评估。
