# LOAD-001：真实负载与稳定性验证

**日期**：2026-09-18  
**代码提交**：`3c0322e`
**Provider**：`agnes-responses`  
**模型**：`agnes-3.0-flash`  
**端点**：`https://apihub.agnes-ai.com/v1`  
**结果**：[脱敏 JSON](LOAD-001-real-load-2026-09-18.json)

## 基线

验证使用发布版 `tests/real_provider` runner，但夹具是临时生成的合成 Go 项目，不读取或上传仓库源码。每次运行都由真实 Agnes Agent 完成“修改实现文件 → 工具调用 → 独立 `go test` 验证”的闭环；runner 为每次运行隔离工作区、HOME、缓存和验证目录，结果只保留结构化脱敏状态。

合成夹具包含一个故意错误的 `Add` 函数、一个可信测试文件和 `go.mod`，SHA-256 记录在 JSON 证据中。

## 验证

| 场景 | 次数 | 通过 | 通过条件 |
|---|---:|---:|---|
| 连续真实 Agent | 5 | **5/5** | `agent_ok`、`validation_ok`、测试确实执行、退出码 0、实现文件发生修改 |
| 3 路并发真实 Agent | 3 | **3/3** | 同上；无超时、无残留、独立测试均通过 |
| 4 路并发探针 | 4 | 2/4 | 2 路收到 Agnes `429 rate_limit`，重试后仍受账号/渠道配额限制 |

4 路结果不计为应用成功，也没有被重试逻辑掩盖。它证明当前 Agnes 渠道在本次模型、账号和请求负载下的外部容量边界；当前可复现的稳定负载上限记录为 3 路并发。若需要更高并发，应先提高 Provider 配额或增加上层队列/限流，而不是把 429 当作成功。

## 变更

真实负载先发现了两个可复现问题，均已在 `3c0322e` 修复并有回归测试：

1. 空 stdout 的工具结果会生成空 `function_call_output`，Agnes 返回 400；现在发送明确的 `(no output)` 占位文本。
2. Responses 流式请求原先没有重试 429/5xx；现在复用指数退避和 `Retry-After`，流式路径最多 6 次尝试，并受调用 context 限制。

## 本地质量门禁

- 默认与 `sqlite memory` 全量测试：通过。
- `go vet -tags "sqlite memory" ./...`：通过。
- Provider、CLI、runtime、jobs、edits、permission、TUI 相关 race：通过。
- `go build -tags "sqlite memory" ./cmd/basework`：通过。
- `make check-docs`、`make gen`、`git diff --check`：通过。

本证据证明指定 Agnes 端点和合成 Agent 负载的当前稳定性，不推广为所有 Provider、模型、账号配额或长期运行 SLO；真人 TUI 体验仍需单独评价。

## 剩余与交接

当前渠道的可复现稳定负载为 3 路并发；4 路需要更高 Agnes 配额或上层队列/限流。真人主观 TUI 评价仍由用户后续完成。

## 结论

`LOAD-001` 的连续与 3 路并发真实负载门禁通过；修复后的代码可以进入新的远端 CI 和发布候选流程。
