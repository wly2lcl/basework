# 子代理系统使用指南

> **状态**：✅ 已实现（Phase 16）
> **版本**：Basework v0.1+
> **相关组件**：`internal/subagent/` — 子代理管理器、task 工具、成本传播、只读代理；`pkg/agent/subturn.go` — 精简版子代理

---

## 概述

子代理（Subagent）系统允许主 Agent 将复杂任务委托给独立的 Agent 实例，在隔离子会话中执行。子代理可专注执行单一任务，支持只读安全模式和异步并行。

从 openwork 的 22 字段 `SubTurnConfig` 精简到 8 字段：隔离子会话、工具隔离、模型可选、异步支持、成本传播。

```text
父 Agent → task 工具 → Manager.Spawn()
  ├→ 创建隔离子会话 + 配置工具集
  ├→ 前台模式：阻塞等待结果
  └→ 后台模式：返回 task_id，异步执行
```

---

## 核心概念

- **子代理（Subagent）**：独立的 Agent 实例，拥有自己的会话和工具集
- **task 工具**：实现 `tool.Tool` 接口，Agent 调用此工具启动子代理
- **隔离子会话**：每个子代理在独立会话中运行，不共享父 Agent 上下文
- **成本传播**：子代理 token 消耗通过 `CostAggregator` 累加到父会话

---

## 使用方式

### task 工具参数

| 参数 | 类型 | 必填 | 默认 | 说明 |
|------|------|------|------|------|
| `description` | string | 是 | — | 任务描述 |
| `prompt` | string | 是 | — | 执行指令 |
| `subagent_type` | string | 否 | `"general"` | `general` 或 `readonly` |
| `background` | bool | 否 | `false` | 后台异步执行 |

```json
{
  "name": "task",
  "arguments": {
    "description": "搜索所有 TODO 注释",
    "prompt": "搜索项目中所有 TODO 或 FIXME，列出文件路径和行号",
    "subagent_type": "readonly"
  }
}
```

### 子代理类型

| 类型 | 可用工具 | 禁用工具 | 适用场景 |
|------|---------|---------|---------|
| `general` | 全部工具 | 无 | 代码生成、修改、重构、测试 |
| `readonly` | read, grep, glob, lsp_diagnostics | write, edit, bash | 代码分析、搜索、审查 |

### 前台模式

父 Agent 阻塞等待子代理完成。适立即需要结果的任务、串行依赖链。

### 后台模式

立即返回 `task_id`，子代理异步执行。后通过 `task_id` 查询状态或结果：

```json
{
  "name": "task",
  "arguments": {
    "task_id": "subtask-abc123",
    "action": "status"
  }
}
```

---

## 配置

### 全局配置

```json
{
  "subagent": {
    "enabled": true,
    "max_concurrent": 3,
    "timeout": "5m",
    "default_type": "general"
  }
}
```

| 配置项 | 默认 | 说明 |
|--------|------|------|
| `enabled` | `true` | 启用 |
| `max_concurrent` | `3` | 最大并发 |
| `timeout` | `"5m"` | 超时 |
| `default_type` | `"general"` | 默认类型 |

### 工具访问控制

```json
{
  "subagent": {
    "readonly": { "tools": ["read", "grep", "glob", "lsp_diagnostics"] },
    "general": { "tools": ["*"] }
  }
}
```

### 模型与 Session

子代理可指定独立模型和 Session Store，不指定时沿用父 Agent。

---

## 使用示例

### 示例 1：代码审查

```
用户：审查 src/agent.go 代码质量

父 Agent：
  task(description="审查 agent.go", subagent_type="readonly",
       prompt="分析 src/agent.go 的代码质量：潜在 bug、风格问题、性能瓶颈、改进建议")
  → 子代理返回分析报告 → 父 Agent 格式化呈现
```

### 示例 2：并行搜索

```
父 Agent 启动 3 个子代理：
  task(prompt="搜索 src/ 目录中的 TODO")
  task(prompt="搜索 tests/ 目录中的 TODO")
  task(prompt="搜索 docs/ 目录中的 TODO")
→ 合并去重呈结果
```

### 示例 3：复杂重构

```
task(description="jsoniter 迁移", prompt="搜索 → 替换 import → 替换调用 → go build → go test")
```

### 示例 4：后台批处理

```
用户：运行项目静态分析

父 Agent：
  task(description="运行 lint", background=true,
       prompt="运行 golangci-lint run ./... 保存到 lint-report.md")
  → 返回 task_id: "subtask-abc123"
  → 完成后通知用户，可通过 task_id 查询
```

---

## API 使用

### Manager API

```go
manager := subagent.NewManager(subagent.Config{MaxConcurrent: 3, Timeout: 5 * time.Minute})
result, err := manager.Spawn(ctx, subagent.Task{
    Description: "搜索 TODO",
    Prompt:      "搜索所有 TODO 注释",
    Type:        subagent.TypeGeneral,
})
```

方法：`NewManager`, `Spawn`, `Wait(ctx, taskID)`, `Status(taskID)`, `Cancel(taskID)`, `Close()`

### SubTurn API（精简版）

```go
cfg := agent.SubTurnConfig{
    Prompt:   "分析 main.go 代码质量",
    Tools:    readonlyTools,
    MaxSteps: 5,
    Async:    false,
    Budget:   &budget,
}
result, err := agent.RunSubTurn(ctx, parentAgent, cfg)
```

### 取消任务

```go
err := manager.Cancel("subtask-abc123")
```

---

## 成本追踪

### 成本传播

子代理 token 消耗通过 `CostAggregator` 累加到父会话：

```text
父会话成本 = 父 Agent 成本 + Σ(所有子代理成本)
```

### 查看成本

```bash
basework session show <session-id>
# Session: abc123 | Total: 5,000
#   Main: 3,000 | Subagent 1: 1,500 | Subagent 2: 500
```

### 成本限制与预算共享

```json
{ "subagent": { "max_tokens_per_task": 10000, "max_total_tokens": 50000 } }
```
```go
budget := int64(50000)
cfg := SubTurnConfig{Budget: &budget}  // 子代理从同一预算池扣减
```

---

## 最佳实践

### 1. 搜索分析用只读代理

```json
{ "subagent_type": "readonly", "prompt": "搜索分析..." }
```

天然安全，适合代码搜索、审查、依赖分析。

### 2. 并行处理独立任务

拆分互不依赖的任务，减少总等待时间：

```go
ch1 := manager.Spawn(ctx, task1)
ch2 := manager.Spawn(ctx, task2)
results := Merge(<-ch1, <-ch2)
```

### 3. 设置合理超时

简单搜索 30s，代码生成 2m，批量重构 5m。

### 4. 限制并发数

`max_concurrent: 3` — 避免资源耗尽或 API 速率超限。

### 5. 分模型使用

小模型做搜索分析，大模型做代码生成：

```go
cfg := SubTurnConfig{Prompt: "搜索错误处理", Model: llm.NewCheapModel()}
cfg2 := SubTurnConfig{Prompt: "重构错误处理", Model: llm.NewPowerfulModel()}
```

### 6. 显式传递上下文

子代理不共享父 Agent 上下文，必要信息通过 prompt 传递：

```
子代理 prompt:
  "父 Agent 发现的问题：...
   请在 src/server.go 中修复，使用 sync.Mutex 而非 channel。"
```

---

## 故障排查

| 问题 | 排查 |
|------|------|
| 超时 | 增大 timeout；拆分任务；检查 API 响应时间 |
| 失败 | `basework logs \| grep subagent`；检查 `subagent_type`；只读代理切换为 general |
| 成本高 | 用只读代理；设 `max_tokens_per_task`；精简 prompt；用小模型；减少 max_steps

---

## 注意事项

- **子代理在独立会话中运行**，不共享父会话上下文。必要信息通过 prompt 显式传递。
- **子代理成本累加到父会话**，包括所有 LLM 调用和工具执行的 token 消耗。
- **后台子代理在主会话结束后仍继续执行**，完成后自动回收。
- **子代理不能嵌套**：子代理不能使用 `task` 工具启动新子代理，Manager 和 task 工具中强制检查。
- **只读代理不提供写工具**，但不提供文件系统级写保护。更强隔离需 OS 层面处理。
- **并发限制**：`max_concurrent` 限制同时运行数，超出的排队等待。
- **Session 持久化**：子代理默认使用内存 session，需持久化则显式传入 `session.Store`。
- **与 SubTurn 的关系**：`internal/subagent/` 是 Phase 16 完整实现（manager + task 工具 + 成本传播 + 只读代理），`pkg/agent/subturn.go` 是轻量版子代理供嵌入场景使用。