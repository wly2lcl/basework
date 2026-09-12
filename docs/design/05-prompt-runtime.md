## 17. System Prompt 组合流程

> 最终的 system prompt 由多个来源拼接而成。

### 17.1 组合顺序

```
┌─────────────────────────────────────────────────────┐
│ 1. 基础 system prompt（用户配置或默认）               │
│    WithSystemPrompt("You are a helpful assistant...") │
├─────────────────────────────────────────────────────┤
│ 2. Agent 定义覆盖（如果选了非默认 agent）              │
│    AgentDef.SystemPrompt 追加/替换基础 prompt          │
├─────────────────────────────────────────────────────┤
│ 3. Skill 指令注入（XML 格式）                         │
│    <skills>                                          │
│      <skill name="xxx">instructions</skill>           │
│    </skills>                                         │
├─────────────────────────────────────────────────────┤
│ 4. Memory 上下文注入（检索到的相关记忆）                │
│    <memory>                                          │
│      <project>AGENTS.md 内容</project>                │
│      <auto>自动积累的记忆</auto>                       │
│    </memory>                                         │
├─────────────────────────────────────────────────────┤
│ 5. 工具约束提示（自动附加）                            │
│    "You have access to the following tools: ..."     │
│    "Always ask before destructive operations."       │
└─────────────────────────────────────────────────────┘
```

### 17.2 实现

```go
// pkg/agent/system_prompt.go
package agent

// ComposeSystemPrompt 组装最终 system prompt
func ComposeSystemPrompt(
    base string,                    // 用户配置的基础 prompt
    agentDef *AgentDef,             // agent 定义（可选）
    skills []*skill.Skill,          // 活跃 skill 列表
    memories []memory.Entry,        // 检索到的记忆
    toolDefs []llm.ToolDefinition,  // 可用工具列表
) string {
    var b strings.Builder
    
    // 1. 基础 prompt
    b.WriteString(base)
    
    // 2. Agent 覆盖
    if agentDef != nil && agentDef.SystemPrompt != "" {
        b.WriteString("\n\n")
        b.WriteString(agentDef.SystemPrompt)
    }
    
    // 3. Skill 注入
    if len(skills) > 0 {
        b.WriteString("\n\n")
        b.WriteString(skill.ToPromptXML(skills))
    }
    
    // 4. Memory 注入
    if len(memories) > 0 {
        b.WriteString("\n\n")
        b.WriteString(memory.FormatForPrompt(memories))
    }
    
    // 5. 工具约束
    b.WriteString("\n\n")
    b.WriteString(formatToolConstraints(toolDefs))
    
    return b.String()
}
```

### 17.3 缓存策略

System prompt 在每个 turn 的 `setupTurn` 阶段组装。由于 skill 和 memory 可能动态变化，**不缓存**，每次重新组装。组装开销很小（字符串拼接）。

---

## 18. 并发模型

> 明确框架各组件的并发安全保证。

### 18.1 并发安全矩阵

| 组件 | 并发安全？ | 说明 |
|------|-----------|------|
| `Agent` | **否** | 一个 Agent 实例同一时间只能有一个活跃的 HandleMessage 调用。多并发需创建多个 Agent 实例 |
| `AgentLoop.InjectMessage` | **是** | 可从任意 goroutine 调用，通过 channel 传递 |
| `tool.Registry` | **是** | 内部 sync.RWMutex 保护，支持并发读取和串行写入 |
| `session.Store` | **是** | MemoryStore 用 RWMutex，JSONL/SQLite 用文件锁 |
| `hook.Broker[T]` | **是** | 内部 RWMutex 保护订阅者列表 |
| `hook.Chain` | **是** | 只读结构体，创建后不可变 |
| `config.Store` | **是** | Copy-on-Write 模式，Get 无锁，Mutate 互斥 |
| `llm.Model` | **是** | Provider 内部维护 HTTP 连接池，支持并发请求 |
| `lsp.Manager` | **是** | 内部 RWMutex 保护客户端映射 |
| `mcp.Manager` | **是** | 内部 RWMutex 保护连接映射 |

### 18.2 HandleMessage 的非并发约束

```go
// AgentLoop 内部使用 mutex 防止并发 HandleMessage
type AgentLoop struct {
    runMu sync.Mutex  // 确保同一时间只有一个 turn 在运行
    // ...
}

func (a *AgentLoop) HandleMessage(ctx context.Context, input string) (*Response, error) {
    if !a.runMu.TryLock() {
        return nil, ErrAgentBusy  // 已有 turn 在运行
    }
    defer a.runMu.Unlock()
    // ...
}
```

### 18.3 多实例模式

如果需要并发处理多个对话，创建多个 Agent 实例：

```go
// 每个对话一个 Agent 实例
agent1, _ := agent.New(agent.WithModel(model), agent.WithSession(session1))
agent2, _ := agent.New(agent.WithModel(model), agent.WithSession(session2))

// 可以并发调用
go agent1.HandleMessage(ctx, "task 1")
go agent2.HandleMessage(ctx, "task 2")
```

多个 Agent 可以共享同一个 `llm.Model`（Provider 是并发安全的）和同一个 `tool.Registry`。

---

## 19. Token 计数策略

> 用于上下文压缩触发和 usage 统计。

### 19.1 计数方式

```go
// pkg/llm/tokens.go
package llm

// EstimateTokens 估算 token 数量
// 使用字符数近似：英文 ~4 字符/token，中文 ~2 字符/token
// 误差约 ±20%，仅用于触发压缩判断，不需要精确
func EstimateTokens(messages []ChatMessage) int {
    total := 0
    for _, msg := range messages {
        for _, part := range msg.Content {
            total += estimateString(part.Text)
        }
        // 工具定义开销（每个工具约 200-500 tokens）
        for _, tc := range msg.ToolCalls {
            total += 300 + len(tc.ArgsJSON)/4
        }
    }
    return total
}

func estimateString(s string) int {
    // 简单启发式：ASCII 按 4 字符/token，非 ASCII 按 2 字符/token
    ascii, nonASCII := 0, 0
    for _, r := range s {
        if r < 128 {
            ascii++
        } else {
            nonASCII++
        }
    }
    return ascii/4 + nonASCII/2
}
```

### 19.2 精确值来源

- **Provider 返回的 Usage**：每次 LLM 调用后，`Response.Usage` 包含精确 token 数
- **累积统计**：AgentLoop 在整个 session 中累积精确 Usage
- **压缩触发**：使用 `EstimateTokens`（近似值），避免每次精确计数

### 19.3 压缩触发检查

```go
func (p *Pipeline) shouldCompact(estimatedTokens int, maxContextTokens int) bool {
    threshold := p.config.CompactConfig.TriggerThreshold // 默认 0.8
    return float64(estimatedTokens) > float64(maxContextTokens)*threshold
}
```

---

