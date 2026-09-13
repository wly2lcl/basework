# pkg/tool/builtin

## 用途

内置工具集，`All()` 返回全部：`Read` / `Write` / `Edit` / `Bash` / `Glob` / `Grep`。

这是 agent 默认能干活的基础。除工具本身外，本包还提供三项横切能力：

- **路径检查** — `PathChecker` 接口 + `SetPathChecker`，把「这个路径能不能碰」交给产品层决定。
- **命令黑名单** — `CheckBlacklist(cmd, extraPatterns)`。
- **工具超时** — `WithTimeout(ctx, toolName, d)` 与 `TimeoutConfig`，超时发布
  `EventToolTimeout` 事件。

## 配置

两种注入方式（CFG-003）：

**实例级注入**（推荐，多实例安全）：`Runtime` 结构体携带 PathChecker / Timeout / EventBus，
经 `AllWithRuntime(rt)` 注入到每个工具实例；解析顺序为「实例字段 → 包级全局 → 内置默认」。
产品运行时与需要多 Agent 隔离的嵌入方应使用本入口。

**包级注入**（兼容路径，必须在创建 agent 之前调用）：

| 注入点 | 作用 | 由谁注入 |
|---|---|---|
| `SetPathChecker(PathChecker)` | 路径准入 | `internal/permission` |
| `SetEventBus(EventPublisher)` | 发布 `tool.timeout` 等事件 | `internal/observability` |
| `SetTimeoutConfig(TimeoutConfig)` | 各工具超时（`DefaultTimeoutConfig()` 取默认） | 产品层 |

## 扩展点

- 在 `internal/tools/` 中按同样的 `Tool` 接口增加增强工具，不必改本包。
- 需要新的准入维度（如网络白名单）时，扩展 `PathChecker` 同级的最小接口，并按架构约束
  在 `pkg` 内定义接口、由 `internal` 结构化满足。

## Model Experience

read/write/edit/grep/glob/bash 是基础文件与命令能力。模型应按实际结果继续，避免假设命令退出就代表目标完成。Bash 使用宿主执行环境；后台 job 和统一编辑事务属于后续产品任务。

## Known Limitations

- `SetPathChecker` / `SetEventBus` / `SetTimeoutConfig` 是**包级全局状态**：进程内所有
  未做实例注入的 agent 共享同一份，多 workspace 场景会互相覆盖，且由调用方自行保证
  「先设置、后使用」的时序。这三个注入点由 `tests/arch_test.go` 白名单登记，新增同类
  注入点必须先登记。多实例隔离请改用实例级 `Runtime` 注入（`AllWithRuntime`），
  全局仅作为未注入字段的回落。
- 内置工具直接操作宿主文件系统与 shell，**没有沙箱**。安全边界完全依赖注入的
  `PathChecker` 与命令黑名单；不注入任何检查器时它们来者不拒。
- `WithTimeout` 只覆盖本包提供的工具，不影响自定义工具。
