# pkg/lsp

## 用途

LSP 集成：把语言服务器的能力（诊断、跳转、符号）包装成 agent 可调用的工具。

- `Manager` — 按工作区管理多个语言服务器进程。
- `Client` — 单个语言服务器的会话（JSON-RPC over stdio），带 `ClientState` 状态机。
- `Detect(workspacePath)` — 扫描工作区，推断该启哪些语言服务器。
- `Tools(manager)` — 把 manager 转成 `pkg/tool` 的工具集交给 agent。
- `DefaultServers` — 内置的语言 → 服务器命令映射。

内置映射（`DefaultServers`）：`go` → `gopls`；`typescript` / `javascript` →
`typescript-language-server --stdio`；`python` → `pyright-langserver --stdio`。

## 配置

```go
lsptool.NewManager(lsp.Config{...})
```

- 用 `Detect()` 自动推断，或自建 `Config` 指定服务器。
- `ServerConfig{Command, Args, Env}` 描述单个服务器的启动方式。
- 继承自 LSP 协议的可调项：初始化选项、根目录、超时等。

## 扩展点

- **接入新语言**：扩展 `DefaultServers`（改代码）或自建 `Config` 传入（不改代码）。
- **直接发协议请求**：用 `Client` / `Conn` 发送任意 LSP `Request` / `Notification`，
  不必局限于已封装的动作。
- **新增工具**：仿照 `Tools()` 的实现，把 `Manager` 的其它能力暴露成 `tool.Tool`。

## Model Experience

语言服务能力通过工具暴露。模型应先确认文件和语言服务可用；服务未启动、文件不支持或诊断为空不能一概解释为代码正确。具体返回结构以工具定义为准。

## Known Limitations

- **依赖外部可执行文件**：`gopls` / `typescript-language-server` / `pyright-langserver`
  必须已在 `PATH` 中，否则工具调用会失败。本包不做安装或版本管理。
- 语言服务器是**独立进程**，其可用性、内存占用与崩溃恢复都不受本包控制；服务器异常退出后
  需要重建 `Manager`。
- `DefaultServers` 是硬编码表：新增语言要么改本包，要么由调用方构造 `Config`。
- 初始化握手有状态机（`ClientState`），首次就绪前发出的请求需要调用方处理等待或失败。
