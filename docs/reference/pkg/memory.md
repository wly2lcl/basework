# pkg/memory

## 用途

记忆系统：跨会话保存可检索的长期信息，供 agent 在后续对话中取用。

- `Engine` — 记忆引擎（压缩、写入、检索）。
- `Store` 接口 — `Write(layer, content, tags...)` / `Read(layer)` / `Search(query, limit)` /
  `Clear(layer)` / `List()`。
- `FileStore` — 基于工作区目录的文件实现（`NewFileStore(workspace)`）。
- `MemoryLayer` — 记忆分层（`LayerUser` 等）。
- 全文检索（FTS5）实现见 `fts.go`。

## 配置

| 项 | 说明 |
|---|---|
| 构建标签 | **`memory`**：启用真实引擎与 FTS5 检索 |
| 无标签时 | `engine_stub.go` 提供**空操作桩**：`Engine` 的方法全部不做任何事 |
| 存储位置 | `NewFileStore(workspace)` 绑定工作区目录 |

即：默认构建下记忆功能是静默关闭的，必须带 `-tags memory` 才有实际行为。

## 扩展点

- **换存储介质**：实现 `Store` 接口（如数据库、远端服务）。
- **新增记忆层级**：扩展 `MemoryLayer` 常量，并在检索策略里纳入。
- **换检索算法**：替换 `Search` 的实现，`Store` 契约不变。

## Model Experience

持久记忆提供检索材料，模型需结合当前文件事实使用；检索为空不代表历史不存在。是否启用与存储配置由宿主决定，不能把可选构建能力写成默认总可用。

## Known Limitations

- **默认构建下 `Engine` 是空操作桩。** 这是最容易踩的坑：不带 `memory` 标签时调用记忆
  接口不会报错，也不会记住任何东西。带标签构建见 `Makefile` / CI 的构建参数。
- FTS5 依赖 `modernc.org/sqlite`，因此 `memory` 标签会连带引入该依赖。
- `FileStore` 面向单个 workspace 目录，跨工作区共享记忆不在其范围内。
- 记忆写什么由调用方决定，本包**不做敏感信息过滤**。
