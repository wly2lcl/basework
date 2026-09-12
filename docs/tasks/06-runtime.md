# M6：进程内运行服务

状态只在 [任务看板](../TASKS.md) 更新。执行前读 [AI 开发流程](../development/ai-workflow.md)。下面是实施范围，新增名称均为拟定；发现已有实现先验证缺口。每张卡可分多个小提交，但验收全部满足后才算完成。

<a id="run-001"></a>

## RUN-001：抽取运行服务接口与所有权

**前置任务**：CFG-003。依赖尚未完成时，只能调查或细化方案，不能宣称本项已验收。

**先读 / 主要修改范围**：`cmd/basework/runtime.go`、`cmd/basework/agent.go`、`cmd/basework/tui.go`。首次阅读应确认实际文件/符号，拟新增路径不存在属正常。

**实施步骤**：

1. 盘点 CLI/TUI 使用的运行能力。
2. 在拟新增 internal/runtime 定义 start/cancel/session/subscribe 最小契约。
3. 写清 Agent、store、job 与订阅的 owner。
4. 将产品组装移到服务实现但不新增网络 server。

**验收条件**：

- pkg 不依赖 internal。
- 服务不依赖 Bubble Tea。
- 旧 CLI 输入/输出和嵌入方式保持兼容。

**针对性验证**：make check-arch；新增包后 go test ./internal/runtime ./cmd/basework -count=1。命令均从仓库根执行；必须记录结果和覆盖边界。

**交付与回填**：代码/文档 diff、必要测试，以及 `docs/development/evidence/RUN-001.md`。更新看板的状态和证据；有用户可见行为变化时更新对应指南及 STATUS。

**范围外**：不顺手改其他任务；公共 API、存储格式、默认权限的额外变化必须先写明影响并拆分。

<a id="run-002"></a>

## RUN-002：有序事件与会话取消

**前置任务**：RUN-001, JOB-004。依赖尚未完成时，只能调查或细化方案，不能宣称本项已验收。

**先读 / 主要修改范围**：`internal/runtime/（RUN-001 新增）`、`pkg/session/`、`pkg/agent/`。首次阅读应确认实际文件/符号，拟新增路径不存在属正常。

**实施步骤**：

1. 定义持久化事件与瞬时 UI 事件的区别。
2. 实现订阅缓冲、慢消费者策略和关闭通知。
3. 明确同会话并发请求是排队还是拒绝。
4. 验证取消/关闭/新建交错时序。

**验收条件**：

- 无无界队列和 goroutine 泄漏。
- 同会话事件顺序稳定。
- 取消目标不影响其他会话且中断结果持久化。

**针对性验证**：go test -race -tags "sqlite memory" ./internal/runtime ./pkg/session ./pkg/agent -count=1。命令均从仓库根执行；必须记录结果和覆盖边界。

**交付与回填**：代码/文档 diff、必要测试，以及 `docs/development/evidence/RUN-002.md`。更新看板的状态和证据；有用户可见行为变化时更新对应指南及 STATUS。

**范围外**：不顺手改其他任务；公共 API、存储格式、默认权限的额外变化必须先写明影响并拆分。

<a id="run-003"></a>

## RUN-003：CLI/TUI 统一接入服务

**前置任务**：RUN-002。依赖尚未完成时，只能调查或细化方案，不能宣称本项已验收。

**先读 / 主要修改范围**：`cmd/basework/agent.go`、`cmd/basework/tui.go`、`internal/tui/callback.go`。首次阅读应确认实际文件/符号，拟新增路径不存在属正常。

**实施步骤**：

1. 改两入口调用同一服务。
2. UI 回调只投递消息不直接改 Model。
3. 订阅按 session/run ID 路由。
4. 验证退出、切换会话和错误路径释放。

**验收条件**：

- CLI/TUI 取消和错误语义一致。
- 旧任务回调不串到新会话。
- 所有关闭路径回收服务订阅。

**针对性验证**：go test -race ./cmd/basework ./internal/tui -count=1；两个入口实测。命令均从仓库根执行；必须记录结果和覆盖边界。

**交付与回填**：代码/文档 diff、必要测试，以及 `docs/development/evidence/RUN-003.md`。更新看板的状态和证据；有用户可见行为变化时更新对应指南及 STATUS。

**范围外**：不顺手改其他任务；公共 API、存储格式、默认权限的额外变化必须先写明影响并拆分。
