# 权限系统使用指南

> **状态**：✅ 已实现（Phase 15）
> **组件**：`internal/permission/`

---

## 概述

权限系统位于 Agent 与工具执行之间，在每个工具调用前检查权限。核心能力：

- **防止危险操作**：黑名单拦截 `rm -rf /`、`mkfs` 等破坏性命令
- **细粒度访问控制**：按工具类型 + 资源路径精确控制
- **交互式决策**：关键操作前通知用户决策
- **自动化兼容**：`yolo` 模式跳过错有提示，适合 CI/CD

### 架构流程

```
Agent 调用工具 → PermissionHook.PreToolUse()
  ├── 黑名单检查 ──→ 命中 → 拒绝
  ├── 规则检查 (Ruleset.Evaluate)
  │     ├── allow → 放行
  │     ├── deny  → 拒绝
  │     └── ask   → 交互提示
  ├── 交互提示 (一次允许/永远允许/拒绝)
  └── 返回结果给 Agent
```

---

## 核心概念

### 规则（Rule）

基本单元，包含三个字段：

| 字段 | 类型 | 说明 | 示例 |
|------|------|------|------|
| `action` | string | 工具动作名称 | `"bash"`, `"write"` |
| `resource` | string | 资源匹配模式（支持 `*` 通配） | `"*.go"`, `"/etc/**"` |
| `effect` | Effect | 允许/拒绝/询问 | `"allow"`, `"deny"`, `"ask"` |

```go
type Rule struct {
    Action   string `json:"action"`
    Resource string `json:"resource"`
    Effect   Effect `json:"effect"`
}
```

### 模式（Mode）

| 模式 | 说明 | 适用场景 |
|------|------|----------|
| `interactive` | 匹配 `ask` 时交互提示 | 日常开发 |
| `yolo` | 跳过错有提示，`ask` 视为允许 | CI/CD |
| `deny-all` | 拒绝所有非明确允许的操作 | 沙盒 |

### 资源模式

| 模式 | 匹配示例 |
|------|----------|
| `*` | 所有资源 |
| `*.go` | `main.go` |
| `src/**` | `src/pkg/util.go` |
| `/etc/**` | `/etc/hosts` |

### Ruleset（规则集）

有序规则列表，按顺序匹配，**第一个匹配的规则生效**：

```go
func (rs *Ruleset) Evaluate(action, resource string) Decision
```

未匹配任何规则时默认拒绝。

---

## 配置

### 权限模式

```json
{ "permission": { "mode": "interactive" } }
```

可选值：`"interactive"`（默认）、`"yolo"`、`"deny-all"`。

### 权限规则

```json
{
  "permission": {
    "rules": [
      {"action": "bash", "resource": "*", "effect": "ask"},
      {"action": "write", "resource": "*.go", "effect": "allow"},
      {"action": "write", "resource": "/etc/*", "effect": "deny"},
      {"action": "read", "resource": "*", "effect": "allow"}
    ]
  }
}
```

上例含义：bash 需询问；`.go` 写入允许；`/etc/` 写入拒绝；读取全部允许；其余默认拒绝。

### 命令黑名单

黑名单检查在规则检查**之前**执行，命中直接拒绝：

```json
{
  "permission": {
    "blocked_commands": ["rm -rf /", "mkfs", "dd if=/dev/", ":(){ :|:& };:"]
  }
}
```

内置禁止命令：`rm -rf /`（删根）、`mkfs`（格式化）、`dd if=/dev/`（设备操作）、`chmod -R 000 /`、fork 炸弹、远程执行管道等。

---

## CLI 命令

> 基于规划中的 CLI 设计，实际接口可能调整。

```bash
basework permission list                    # 查看规则
basework permission add --action write --resource "*.py" --effect allow   # 添加规则
basework permission remove --id 5           # 删除规则
basework permission mode                    # 查看模式
basework permission mode set yolo           # 切换模式
basework permission blocked                 # 查看黑名单
```

---

## 交互提示

模式为 `interactive` 且规则结果为 `ask` 时显示：

```
? 允许执行 bash "ls -la" ?
  ▸ 一次允许（本次会话）
    永远允许（记住选择）
    拒绝
```

| 选项 | 效果 | 有效期 |
|------|------|--------|
| 一次允许 | 仅放行本次调用 | 单次执行 |
| 永远允许 | 放行并缓存到会话 | 本次会话结束前 |
| 拒绝 | 拒绝调用，返回错误 | 单次执行 |

"永远允许"存入会话级缓存（键 `"action:resource"`），相同组合不再提示。会话结束或 `basework session clear` 清除缓存。

---

## API 使用

### 在代码中使用

```go
import "github.com/wly2lcl/basework/internal/permission"

svc := permission.New(permission.Config{
    Mode: permission.ModeInteractive,
    Rules: []permission.Rule{
        {Action: "bash", Resource: "*", Effect: permission.EffectAsk},
        {Action: "write", Resource: "*.go", Effect: permission.EffectAllow},
        {Action: "read", Resource: "*", Effect: permission.EffectAllow},
    },
    BlockedCommands: []string{"rm -rf /", "mkfs"},
})

allowed, _, err := svc.Check(context.Background(), "bash", map[string]any{"command": "ls"})
```

### PromptHandler 自定义

```go
type PromptHandler interface {
    Prompt(ctx context.Context, req PromptRequest) (PromptResponse, error)
}
```

可实现自定义提示行为（GUI、API 回调等）。

### Agent Hook 集成

```go
permHook := permission.NewPermissionHook(svc)
agent := agent.New(agent.WithHooks(permHook))
// Hook 接口：PreToolUse(ctx, req) → 检查黑名单 → 评估规则 → 提示 → 返回决策
```

---

## 最佳实践

### 开发环境

```json
{
  "permission": {
    "mode": "interactive",
    "rules": [
      {"action": "read",  "resource": "*",      "effect": "allow"},
      {"action": "write", "resource": "src/**",  "effect": "allow"},
      {"action": "write", "resource": "/**",     "effect": "ask"},
      {"action": "bash",  "resource": "*",       "effect": "ask"}
    ]
  }
}
```

### 生产环境（自动化）

```json
{
  "permission": {
    "mode": "yolo",
    "rules": [
      {"action": "read",  "resource": "*",      "effect": "allow"},
      {"action": "write", "resource": "/app/**", "effect": "allow"},
      {"action": "write", "resource": "/etc/**", "effect": "deny"},
      {"action": "bash",  "resource": "*",       "effect": "allow"}
    ],
    "blocked_commands": ["rm -rf /", "mkfs", "dd if="]
  }
}
```

### 沙盒环境

```json
{
  "permission": {
    "mode": "deny-all",
    "rules": [
      {"action": "read", "resource": "*.md", "effect": "allow"},
      {"action": "bash", "resource": "*",    "effect": "ask"}
    ]
  }
}
```

### 设计原则

1. **具体优先**：具体规则在前，通用兜底在后
2. **最小权限**：只赋予完成任务所需的最小权限
3. **明确拒绝**：优先声明拒绝规则，再放行其余
4. **黑名单兜底**：即使规则放行，黑名单仍拦截极端危险命令

---

## 故障排查

### 权限被意外拒绝

1. **检查规则顺序** — `basework permission list`
2. **检查 mode 设置** — `basework permission mode`
3. **检查黑名单** — `basework permission blocked`
4. **查看日志** — `basework logs | grep permission`
   日志示例：`[permission] Check: action=bash, resource="ls -la" → decision=ask`

### 交互提示未显示

- 确认 `mode` 为 `interactive`
- 确认终端支持交互输入
- 确认规则评估结果为 `ask`

### 重置缓存

```bash
basework session clear
```

重启 basework 也会自动清除旧会话缓存。

---

## 参考

### 文件结构

```text
internal/permission/
├── rules.go        # Rule/Ruleset、Evaluate 方法
├── service.go      # Service、Check 方法、会话缓存
├── prompt.go       # PromptHandler 接口、TerminalPromptHandler
├── hook.go         # PermissionHook — Agent Hook 集成
├── bash_guard.go   # 命令黑名单检查
└── permission_test.go
```

### 实现路线

| 任务 | 文件 | 依赖 |
|------|------|------|
| 15.1 权限规则 | `rules.go` | 无 |
| 15.2 权限服务 | `service.go` | 15.1 |
| 15.3 交互提示 | `prompt.go` | 15.2 |
| 15.4 Agent 集成 | `hook.go` | 15.2, 15.3, `pkg/agent/hook` |
| 15.5 命令黑名单 | `bash_guard.go` | 无 |

### 相关文档

- [配置参考](configuration.md)
- [完整任务列表](../TASKS.md)