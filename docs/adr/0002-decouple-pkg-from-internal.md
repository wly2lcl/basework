# ADR 0002: pkg 层用本地接口解耦 internal，而非共享接口包

- Status: Accepted
- Date: 2026-09-12
- Supersedes: —

## 背景

`pkg/` 是 basework 的「可嵌入核心」卖点：外部项目 `go get` 后 import `pkg/agent` 即可使用。但 `pkg/tool/builtin` 反向 import 了 `internal/permission` 与 `internal/observability`，导致任何嵌入方都被拖入终端产品层的依赖树——「可嵌入」承诺实际不成立。

值得注意的是，这不是新问题：Phase 33 已经做过同样的解耦（在 `pkg/agent/interfaces.go` 定义了 Compactor / LoopDetector / PermissionChecker / EventPublisher / SubAgentRunner 五个接口），但**漏掉了 `pkg/tool/builtin`**，这才是反向依赖残存至今的根因。

修复时面临一个真实选择：`builtin` 需要的路径检查与事件发布两个能力，在 `pkg/agent` 中已有同名同形接口，是复用、还是另建？

## 决定

**在 `pkg/tool/builtin` 内各自定义最小接口；不复用 `pkg/agent` 的定义，也不新建共享接口包。**

```go
// pkg/tool/builtin —— 接口定义在使用方本地
type PathChecker interface { CheckPath(path string) (bool, string) }
type EventPublisher interface { PublishEvent(eventType string, data map[string]interface{}) }
```

三个方案的取舍：

| 方案 | 结论 | 理由 |
|---|---|---|
| 复用 `pkg/agent.EventPublisher` | ❌ 否决 | `pkg/agent` 依赖 `pkg/tool`，反向复用造成分层倒置（`builtin` → `agent` → `tool`） |
| 新建 `pkg/interfaces` 共享包 | ❌ 否决 | 会变成一个「谁都能往里塞东西」的垃圾桶，且让依赖图变深；接口本应按使用方需求定义 |
| 在 `builtin` 内本地定义 | ✅ 采纳 | 零适配成本，且 `builtin` 不再知道 `internal/` 的存在 |

采纳第三种的关键前提：Go 的接口是**结构化满足**的。两个接口方法签名完全一致，因此同一个 `*observability.EventBusAdapter` 同时满足 `pkg/agent.EventPublisher` 与 `builtin.EventPublisher`，**不需要写任何适配器**。如果两个接口签名不完全一致，这个方案的成本会显著上升。

配套约束：`pkg/` 层不得 import `internal/`、`cmd/`；`internal/` 不得 import `cmd/`。由 `tests/arch_test.go` 守护，同时 `scripts/gendeps` 在依赖图中报告违规。

## 后果

正面：
- 嵌入方不再被拖入 `internal/` 依赖树，`pkg/` 具备拆成独立 module（`go.work`）的前提
- 零适配成本：接口同形，现有的 `permission.PathChecker` 直接可用，调用点全部不变
- `builtin` 不再知道 `internal/` 的存在，也不会随 `internal/` 的重构而破坏

代价与后续：
- 同形接口在不同包重复定义（目前 2 处）。这是 Go 生态的常见取舍，接受；若重复超过 5 处则应重新评估
- `timeout.go` 的注入方式从「直接调用 `internal/observability`」改为 `builtin.SetEventBus(...)`，属**破坏性 API 变更**，目标版本 v0.2.0
- `pkg/` 层仍有两个「默认构建即引入」的第三方依赖（`golang.org/x/image`、`golang.org/x/oauth2`）未做 build tag 隔离，与 `modernc.org/sqlite`、`github.com/golang/snappy` 的 tag 隔离做法不一致。明细见 `docs/DEPGRAPH.md` 的「pkg/ 层第三方依赖明细」，留待后续评估
- 新增 `pkg/` 注入点需要显式登记到 `tests/arch_test.go` 的 `injectionWhitelist`，避免核心的隐式全局状态无声膨胀
