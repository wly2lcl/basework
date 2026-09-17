# 子代理系统使用指南

本文说明当前主 Agent runtime 接入的子代理能力。实现位于 `internal/subagent/`，属于终端产品内部组件；外部 Go module 不能导入该目录。公共嵌入场景请使用 `pkg/agent` 中公开的接口。

## 当前能力

主 Agent 暴露一个名为 `sub_agent` 的工具。它创建隔离的子会话，执行一次任务并把结构化结果返回给父 Agent。当前产品工具是同步调用：没有 `background` 参数，也没有通过 CLI 查询子任务的 `status/cancel` 命令。

子代理不共享父 Agent 的消息历史；父 Agent 必须通过任务描述和可选 `context` 传递必要信息。每个任务有独立 ID、成功状态、文本结果、token 使用量和错误字段。

## 工具参数

| 参数 | 类型 | 必填 | 默认 | 说明 |
|---|---|---:|---|---|
| `description` | string | 是 | — | 子代理要完成的任务 |
| `agent_type` | string | 否 | 配置中的 `default_type` | `general` 或 `readonly` |
| `context` | object | 否 | 空对象 | 注入子代理系统提示的任务上下文 |

示例：

```json
{
  "name": "sub_agent",
  "arguments": {
    "description": "审查 pkg/session 的并发写入风险",
    "agent_type": "readonly",
    "context": {"focus": "锁、残尾和恢复"}
  }
}
```

工具会返回类似：

```json
{
  "id": "任务 ID",
  "success": true,
  "output": "分析结果",
  "token_usage": {"input_tokens": 0, "output_tokens": 0, "total_tokens": 0},
  "cost": 0
}
```

失败时 `success=false`，错误放在 `error`；父 Agent 应检查该字段，不能只根据工具调用成功判断子任务完成。

## 子代理类型和工具边界

| 类型 | 当前 runtime 工具 | 用途 |
|---|---|---|
| `general` | 与主 runtime 相同的普通工具集，但不注册后台任务工具和子代理工具 | 修改、测试和综合任务 |
| `readonly` | `read`、`grep`、`glob`，以及名称以 `lsp_` 开头的工具和只读 MCP 工具 | 搜索、审查和分析 |

只读类型的工具表过滤和系统提示都会生效，但这不是文件系统级写保护；宿主机仍需要 OS 沙箱或只读挂载才能提供更强隔离。子代理不能嵌套启动 `sub_agent`。

## 配置

配置字段位于顶层 `sub_agent`：

```json
{
  "sub_agent": {
    "enabled": true,
    "default_type": "general",
    "cost_limit": 1.0,
    "max_concurrent": 5
  }
}
```

| 字段 | 默认值 | 说明 |
|---|---:|---|
| `enabled` | `true` | 是否向主 Agent 注册子代理能力 |
| `default_type` | `general` | 未传 `agent_type` 时使用；可选 `general`/`readonly` |
| `cost_limit` | `1.0` | 每个子代理任务的成本限制（美元，0 表示不限制） |
| `max_concurrent` | `5` | Coordinator 并发执行上限；小于等于 0 表示不限制 |

单个子代理执行的内部超时目前固定为 30 秒，不由配置文件中的 `timeout` 字段控制。主 runtime 会把权限检查器传给子代理；`readonly` 的工具过滤仍先于模型调用发生。

## 调用和结果处理建议

- 描述写成一个可独立验收的任务，包含文件范围、输出格式和测试要求。
- 只需要搜索或审查时指定 `agent_type: readonly`，避免给子代理不必要的写入能力。
- 将会话 ID、事实标记、失败背景等必要信息放进 `context`；不要假设子代理能看到父会话历史。
- 对返回结果检查 `success`、`error` 和 `token_usage`，再决定是否继续修改或重试。
- 多个任务的并发编排由内部 `Coordinator.ExecuteTasks` 提供，当前 `sub_agent` 工具入口仍逐个同步执行；不要在 prompt 中假设有后台 task ID 查询接口。

## 故障排查

1. `sub_agent` 不在工具列表：检查 `sub_agent.enabled`，以及是否使用了 `readonly` 启动预设（只读预设会移除子代理工具）。
2. 类型错误：`agent_type` 只能是 `general` 或 `readonly`；空值使用 `default_type`。
3. 超时：单任务内部上限为 30 秒；拆小任务或降低需要模型处理的范围后重试。
4. 成本超限：检查 `cost_limit` 和返回的 `token_usage`，把大任务拆成可独立验收的步骤。
5. 工具被拒绝：父 runtime 的权限检查和敏感路径保护仍适用于子代理；查看 [权限指南](permission-guide.md) 和 [安全指南](security.md)。

## 相关文档

- [CLI 使用指南](cli-guide.md)
- [TUI 使用指南](tui-guide.md)
- [权限系统](permission-guide.md)
- [嵌入指南](embedder-guide.md)
- [当前状态](../STATUS.md)
