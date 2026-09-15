# pkg/session

## 用途

事件溯源会话层：对话不是以「消息列表」保存，而是以**不可变事件序列**保存，消息历史是
每次读取时投影出来的结果（`History()` 即 `ProjectMessages(Events())`）。这样回放、审计、
多视图查询都成立。

- **事件与投影** — `Event`（16 种类型）+ `ProjectMessages`（投影为消息）+
  `ProjectMessagesWithSeq`（额外返回每条消息**来源事件的 Seq**，压缩靠它做稳定锚点）。
- **存储后端** — `MemoryStore`（内存）、`JSONLStore`（按会话一文件）、`SQLiteStore`
  （需 `sqlite` 标签）。
- **格式版本化** — `SchemaVersion` + **相邻迁移链**（只允许 vN→vN+1）+ `ErrSchemaTooNew`。
  读到比当前更高的版本会**拒绝**而不是尽力解析。
- **周边** — `FileTracker`（已读/已改文件追踪）、`GenerateTitle`（自动标题）、
  `FileLock`、事件压缩（`compress.go`，需 `sqlite`）。

### 事件类型速览

| 类别 | 事件 |
|---|---|
| 对话 | `prompted`、`text.started`、`text.delta`、`text.ended` |
| 工具 | `tool.called`、`tool.success`、`tool.failed` |
| 轮次 | `turn.started`、`turn.ended`、`turn.failed`、`agent.switched` |
| 上下文治理 | `compacted`、`system.prompt_set`、`steered` |
| 审计 | `request.built` |
| 编辑事实 | `file.edited` |

`system.prompt_set` / `steered` / `request.built` **不投影为消息**：前两者是「请求级上下文」，
由 [pkg/agent](agent.md) 的 `BuildRequestMessages` 在组装请求时放到历史投影之前；所有已记录的
steering 都按事件顺序放入后续请求。`request.built` 只留请求指纹与来源信息。`file.edited`
记录编辑流程步骤（preview/committed/failed/rolled_back）的逐文件结局与验证命令引用，
供 CLI 审阅与会话恢复后定位修改，同样不投影为消息。

### 压缩事件与恢复

新的 `compacted` 事件应保存压缩器实际产出的完整 `compacted.Messages` 快照。投影遇到该事件时，
用快照精确替换此前投影出的历史，再继续处理后续事件；因此可以重建允许选择、删除或重排消息的
任意压缩策略结果，而不必从单个头部截断锚点猜测。没有快照的旧事件继续通过 `Summary`、
`TruncatedSeq`、`KeepFrom` 走兼容投影分支。

完整快照会增加事件日志和存储体积，因为压缩后的消息内容会再次持久化。这是精确回放任意策略
结果所需的成本。包级两次快照与重开 Store 已有测试；当前产品已支持按旧 ID 继续，双压缩的真实产品触发仍由 CTX-003 验收。

## 配置

| 项 | 说明 |
|---|---|
| 后端选择 | `NewMemoryStore()` / `NewJSONLStore(baseDir)` / `NewSQLiteStore(path)` |
| 构建标签 | `sqlite`：SQLite 后端、WAL 恢复、事件压缩；无标签时只有内存与 JSONL 后端 |
| 格式版本 | `SchemaVersion`（当前 **2**）。升级只需补一个相邻迁移步骤 |
| 投影常量 | `SummaryMessagePrefix`：压缩摘要插回历史时的固定前缀 |

## 扩展点

- **接入新后端**：实现 `Store` 接口。
- **新增事件类型**：加 `EventType` 常量 + `Event` 的 Data 结构 + `DecodeData` 分支。
  若新事件**会改变 `ProjectMessages` 的输出**，必须同时升 `SchemaVersion` 并补上相邻迁移
  步骤——否则同一份日志在不同版本下会投影出不同历史，且没有任何报错。
- **只改请求组装而不改投影**（如本次的请求级上下文事件）同样要升版本：旧版本代码会忽略
  这些事件、改用配置，从而构造出与写入方意图不同的请求。版本号在这里是**能力门槛**。

## Model Experience

本包保存事实并生成历史投影，不直接调用模型。消费者通过事件恢复上下文；压缩摘要与历史原文不同，必须保留来源边界。更高版本日志拒绝处理而非猜测解释。

## Known Limitations

- WorkspaceFacts 由运行时编辑事件接线保存，`facts show` 可读取同一工作区的持久化事实；部分提交按单文件 outcome 折叠，重复投影保持幂等。Windows 路径兼容仍由 CTX-002/发布夹具继续覆盖。

- **JSONL 追加是整文件重写**：`AppendEvent` 每次都把文件头 + 全部事件写进临时文件再
  `rename`。事件数增长后，单次追加的代价是 O(文件大小)。长会话建议改用 SQLite 后端。
- **迁移链不可回退**：`adjacentSteps` 明确拒绝 `from > to`，降级需要重写数据，本机制不支持。
- **版本过高即拒绝**：读路径与写路径都会返回 `ErrSchemaTooNew`，没有「尽力解析」的降级模式。
  这是有意为之——静默继续等于制造难以察觉的错误上下文。
- `JSONLStore.Messages()` 不支持无 `SessionID` 调用，必须走 `Events()` + `ProjectMessages()`。
- **旧的无快照压缩事件只支持旧锚点兼容语义**，不能准确表达选择或重排消息的策略结果。新事件
  保存完整消息快照以恢复任意策略结果；快照增加存储体积，长会话仍需验证性能和恢复行为。
- 文件锁分平台实现（`lock_unix.go` / `lock_windows.go`），跨平台行为需要分别验证。
