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

## 多 key 复测补充（2026-09-18）

用户新增 `AGNES_API_KEY1` 至 `AGNES_API_KEY5` 后，使用 6 个独立进程分别注入 6 个 key，重复执行真实 Agent 闭环。每次仍使用合成夹具，验证条件仍是 Agent 成功、实现文件发生修改、独立 `go test` 执行且退出码为 0；密钥值没有写入证据。

- 6 个 key 均至少完成过一次独立成功闭环；`AGNES_API_KEY1` 和 `AGNES_API_KEY4` 在并发超时后单独复测成功。
- 两轮 6 路并发分别为 **5/6** 和 **5/6**，失败均为请求上下文超时，**没有 429**。
- 12 路突发（每个 key 同时承载两个请求）为 **8/12**，4 个失败均为上下文超时，**没有 429**。

这说明增加 key 已解除本次 6 路测试中观察到的账号/渠道 `429`，但不能把 6 路宣称为 100% 稳定 SLO；Provider 在更高突发下仍可能长时间不返回。当前应用仍只读取 `AGNES_API_KEY`，本轮通过“一进程一 key”验证多 key 能力，不代表应用已经自动轮换或均衡这些 key。原始统计见同目录的 [`LOAD-001-multikey-2026-09-18.json`](LOAD-001-multikey-2026-09-18.json)。

## 项目本身的本地负载补充（PROJECT-LOAD-001，2026-09-18）

上面的连续/并发结果是 Agnes 外部 Provider 的容量观察，不能作为 Basework 项目负载证据。为隔离模型渠道，另用仓库已有的 `tests/tui_pty/fake_openai.py` 启动仅绑定 `127.0.0.1` 的确定性 SSE 服务，并通过当前 `basework` 真实二进制启动独立进程；没有使用 Agnes key，也没有访问外网。

| 场景 | 并发 | 通过 | 结果 |
|---|---:|---:|---|
| S4：运行时启动、Provider 请求、单轮回复、退出与会话落盘 | 32 | **32/32** | 本地请求 32/32，P95 165ms，32 个会话均落盘 |
| S1：读取 → `edit_files` 预览 → 提交 → `bash go test` → 最终回复 | 8 | **8/8** | 本地请求 40/40；8 个工作区均改成 `a + b`，测试文件均保持不变，8 个会话均落盘 |

本轮本地假 Provider 共收到 72 个请求，脚本轮次和响应均符合预期。`go test -race` 的 Agent、session、runtime、jobs、edits、observability 与集成测试范围通过；选定的 Session/Streaming/Token/Tool 基准也通过。竞态测试中的 pprof 用例需要本机回环监听权限，放行该权限后通过。原始脱敏统计见 [`PROJECT-LOAD-001-local-2026-09-18.json`](PROJECT-LOAD-001-local-2026-09-18.json)。

这组结果证明的是 Basework 本身的运行时装配、请求传输、Agent 工具循环、会话持久化和工作区不变量在给定本地负载下成立；它不证明 Agnes 或其他 Provider 的容量、模型质量、长期 SLO、真实用户 TUI 手感或生产环境可用性。两类证据必须分开阅读。
