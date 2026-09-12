## 20. 安全模型

> 工具执行的安全约束和输入/输出清洗。

### 20.1 工具安全层级

```
┌─────────────────────────────────────────────┐
│ L1: 权限检查（PermissionHook）               │
│      → 用户确认 allow/deny/ask               │
├─────────────────────────────────────────────┤
│ L2: 路径约束                                 │
│      → 文件操作限制在工作区内                  │
├─────────────────────────────────────────────┤
│ L3: 输出清洗                                 │
│      → 工具输出截断（防止上下文溢出）          │
├─────────────────────────────────────────────┤
│ L4: 沙箱（进程级隔离）                        │
│      → 进程级隔离                             │
└─────────────────────────────────────────────┘
```

### 20.2 路径约束

```go
// pkg/tool/builtin/safety.go
package builtin

// ValidatePath 验证文件路径是否在工作区内
func ValidatePath(path string, workspace string) error {
    abs, err := filepath.Abs(path)
    if err != nil {
        return fmt.Errorf("invalid path: %w", err)
    }
    
    absWorkspace, err := filepath.Abs(workspace)
    if err != nil {
        return fmt.Errorf("invalid workspace: %w", err)
    }
    
    if !strings.HasPrefix(abs, absWorkspace) {
        return fmt.Errorf("path %q is outside workspace %q", path, workspace)
    }
    
    return nil
}
```

### 20.3 输出截断

```go
// pkg/tool/truncate.go
package tool

const MaxOutputSize = 100_000 // 字符，约 25k tokens

// TruncateOutput 截断过大的工具输出
func TruncateOutput(content string) string {
    if len(content) <= MaxOutputSize {
        return content
    }
    half := MaxOutputSize / 2
    return content[:half] + "\n\n[... truncated " + 
        strconv.Itoa(len(content)-MaxOutputSize) + 
        " characters ...]\n\n" + content[len(content)-half:]
}
```

### 20.4 Prompt Injection 防护

框架层面不做 prompt injection 检测（这是宿主应用的责任），但提供以下安全约束：
- 工具输出作为 `ToolResult` 返回，而非直接注入 user message
- LLM 不会将工具输出视为用户指令
- 宿主应用可通过 `BeforeTool` hook 检查/清洗工具参数

---

## 21. Logging 与可观测性

> 框架内部的日志和可观测性策略。

### 21.1 设计原则

- **框架不内置日志库** — 使用 Go 标准 `log/slog`
- **宿主应用控制日志级别** — 通过 `WithLogger` 选项注入
- **结构化日志** — 所有日志使用 key-value 格式

### 21.2 Logger 接口

```go
// pkg/agent/logger.go
package agent

// Logger 是框架日志接口
type Logger interface {
    Debug(msg string, args ...any)
    Info(msg string, args ...any)
    Warn(msg string, args ...any)
    Error(msg string, args ...any)
}

// WithLogger 注入自定义 logger
func WithLogger(l Logger) Option

// DefaultLogger 默认使用 slog.Default()
func DefaultLogger() Logger
```

### 21.3 关键日志点

| 事件 | 级别 | 内容 |
|------|------|------|
| Turn 开始 | Debug | session_id, step, model |
| LLM 调用 | Debug | model, token_count |
| LLM 响应 | Debug | finish_reason, usage |
| 工具调用 | Info | tool_name, args_summary |
| 工具结果 | Debug | tool_name, result_size |
| 权限拒绝 | Warn | tool_name, action |
| 上下文压缩 | Info | before_tokens, after_tokens |
| 错误 | Error | error_type, message |

### 21.4 OpenTelemetry（未来增强）

框架通过 Hook 系统的 `AfterLLM` 和 `AfterTool` 可以无侵入地集成 OpenTelemetry：
- 宿主应用注册一个 OTel Hook，自动采集 span
- 不需要框架内置 OTel 依赖

---

## 22. MCP/LSP 工具的权限集成

> 动态注册的工具同样经过 PermissionHook。

### 22.1 统一权限模型

所有工具（内置、MCP、LSP）在 `ToolRegistry.Settle()` 执行前，都经过 `hook.Chain.RunBeforeTool()`：

```
ToolRegistry.Settle(call)
    │
    ├── hook.Chain.RunBeforeTool(call)  ← 权限检查
    │   └── PermissionHook 匹配规则
    │       ├── "tool:bash" → allow
    │       ├── "tool:mcp_server_search" → ask
    │       └── "tool:write:/etc/**" → deny
    │
    └── tool.Execute(ctx, args)
```

### 22.2 权限规则命名约定

| 工具来源 | 规则格式 | 示例 |
|---------|---------|------|
| 内置工具 | `tool:{name}` | `tool:bash`, `tool:write` |
| 内置 + 资源 | `tool:{name}:{path}` | `tool:write:/etc/**` |
| MCP 工具 | `tool:mcp_{server}_{name}` | `tool:mcp_github_create_issue` |
| LSP 工具 | `tool:lsp_{operation}` | `tool:lsp_definition`, `tool:lsp_references` |

### 22.3 默认权限

```go
// 默认权限规则
var DefaultRules = []hook.Rule{
    // 只读工具默认允许
    {Action: "tool:read", Effect: hook.Allow},
    {Action: "tool:grep", Effect: hook.Allow},
    {Action: "tool:glob", Effect: hook.Allow},
    {Action: "tool:lsp_*", Effect: hook.Allow},
    
    // 写入工具默认需要确认
    {Action: "tool:write", Effect: hook.Ask},
    {Action: "tool:edit", Effect: hook.Ask},
    {Action: "tool:bash", Effect: hook.Ask},
    
    // MCP 工具默认需要确认
    {Action: "tool:mcp_*", Effect: hook.Ask},
}
```

---

## 23. 错误处理策略

> 统一的错误分类、重试和恢复机制。

### 23.1 错误分类

```go
// pkg/llm/error.go 中定义
type ErrorType string

const (
    // 可重试错误
    ErrRateLimit     ErrorType = "rate_limit"      // 429
    ErrOverloaded    ErrorType = "overloaded"       // 529/503
    ErrNetwork       ErrorType = "network"          // 连接超时/DNS
    
    // 需恢复的错误
    ErrContextOverflow ErrorType = "context_overflow" // 上下文超长
    ErrAuthExpired     ErrorType = "auth_expired"     // 401, 可刷新 token
    
    // 不可恢复错误
    ErrAuth        ErrorType = "auth"            // 401, 无法恢复
    ErrModelNotFound ErrorType = "model_not_found" // 404
    ErrInvalidRequest ErrorType = "invalid_request" // 400
    ErrInternal    ErrorType = "internal"         // 500
)
```

### 23.2 重试策略

```go
// pkg/agent/retry.go
package agent

// RetryConfig 重试配置
type RetryConfig struct {
    MaxRetries    int           // 最大重试次数，默认 3
    InitialDelay  time.Duration // 初始延迟，默认 1s
    MaxDelay      time.Duration // 最大延迟，默认 30s
    BackoffFactor float64       // 退避因子，默认 2.0
}

// shouldRetry 判断错误是否可重试
func shouldRetry(err error) bool {
    var llmErr *llm.Error
    if errors.As(err, &llmErr) {
        switch llmErr.Type {
        case llm.ErrRateLimit, llm.ErrOverloaded, llm.ErrNetwork:
            return true
        }
    }
    return false
}

// retryWithBackoff 指数退避重试
func retryWithBackoff(ctx context.Context, cfg RetryConfig, fn func() error) error
```

### 23.3 各层错误处理职责

| 层 | 职责 |
|---|------|
| **Provider** | HTTP 状态码 → `llm.ErrorType` 映射 |
| **Agent Loop** | 可重试错误 → 指数退避重试；ContextOverflow → 触发压缩后重试 |
| **Tool** | 工具执行 panic 恢复 → 返回 error；单个工具失败不影响其他工具 |
| **Session** | 事件写入失败 → 重试 1 次后返回 error（不丢数据） |
| **宿主应用** | 最终错误处理和用户提示 |

### 23.4 不可恢复错误处理

```
不可恢复错误流程:
1. Agent Loop 记录错误到 session (EventTurnFailed)
2. 调用 Callback.OnError(err)
3. 返回 error 给宿主应用
4. Agent 状态重置为 idle，可接收下一个消息
```

---

## 24. 优雅关闭流程

> Agent 关闭时的资源清理顺序。

### 24.1 关闭顺序

```
AgentLoop.Close()
    │
    ├── 1. 设置关闭标记 (atomic.Bool)
    │
    ├── 2. 停止接收新消息
    │   └── HandleMessage 返回 ErrAgentClosed
    │
    ├── 3. 等待活跃 turn 完成
    │   └── 给当前 turn 最多 30s 完成
    │   └── 超时后强制取消 (context cancel)
    │
    ├── 4. 关闭 steering/pending 通道
    │
    ├── 5. 逆序关闭 Plugin
    │   └── for i := len(plugins)-1; i >= 0; i-- { plugins[i].Shutdown(ctx) }
    │
    ├── 6. 关闭 Provider（释放 HTTP 连接池）
    │
    ├── 7. 关闭 MCP 连接
    │
    ├── 8. 关闭 LSP 客户端
    │
    ├── 9. Flush session（确保所有事件持久化）
    │
    └── 10. 关闭 PubSub Broker
```

### 24.2 Close 接口

```go
// AgentLoop.Close 实现
func (a *AgentLoop) Close() error {
    // 幂等：多次调用安全
    if !a.closed.CompareAndSwap(false, true) {
        return nil
    }
    
    // 1-2: 停止接收
    // 3: 等待活跃 turn
    done := make(chan struct{})
    go func() {
        a.activeWg.Wait()
        close(done)
    }()
    select {
    case <-done:
    case <-time.After(30 * time.Second):
        a.cancel() // 强制取消
        <-done
    }
    
    // 4-10: 逆序关闭资源
    // ...
    
    return nil
}
```

### 24.3 Context 传播

```
AgentLoop 持有 root context
    │
    ├── 所有 turn 使用 child context（可被 cancel 中断）
    ├── Plugin Init 使用独立 context
    ├── Tool Execute 使用 turn context（继承 turn 取消）
    └── Provider Stream 使用 turn context
```

Close() 取消 root context → 级联取消所有活跃操作。

---

## 25. 迁移策略

从 openwork 到 basework 的迁移路径（终版）：

1. **Phase 1**：新建 `pkg/llm/` 统一类型（无依赖，可并行）
2. **Phase 2**：新建 `pkg/hook/` PubSub + Hook（依赖 pkg/llm）
3. **Phase 3**：新建 `pkg/session/` 事件溯源（依赖 pkg/llm）
4. **Phase 4**：新建 `pkg/tool/` 精简版（依赖 pkg/llm）
5. **Phase 5**：新建 `pkg/agent/` 核心循环 + streaming + steering + compaction（从 openwork 搬 + 精简）
6. **Phase 6**：新建 `pkg/provider/` 简化工厂
7. **Phase 7**：新建 `pkg/lsp/` LSP 集成
8. **Phase 8**：新建 `pkg/mcp/` MCP 协议支持
9. **Phase 9**：新建 `pkg/memory/` 记忆系统
10. **Phase 10**：新建 `pkg/config/` + `pkg/skill/`
11. **Phase 11**：新建 `cmd/basework/` CLI 参考实现
12. **Phase 12**：集成测试 + 文档
13. **Phase 13**：`internal/compaction/` 上下文压缩
14. **Phase 14**：`internal/retry/` 重试机制
15. **Phase 15**：`internal/permission/` 权限系统
16. **Phase 16**：`internal/subagent/` 子代理系统
17. **Phase 17**：`internal/tools/` 增强工具包
18. **Phase 18**：`internal/tui/` 终端 UI
19. **Phase 19**：`pkg/session/` SQLite 存储 + 自动标题 + 队列 + 文件追踪
20. **Phase 20**：`pkg/provider/` Provider 扩展（OpenCode Zen、Bedrock、Azure、Copilot、Ollama）

---

