# pkg/tool

## 用途

工具接口与注册表。工具是 agent 与外界交互的唯一手段——文件读写、执行命令、检索代码，
全部以 `Tool` 形式接入，再由 `Registry` 汇总成模型可见的 `ToolDefinition`。

- `Tool` 接口：`Name` / `Description` / `Parameters`（JSON Schema）/ `Execute`。
- `Registry`：注册、按名查找、禁用、克隆，以及 `Settle`（执行一次工具调用并包装结果）。
- `Materialize` 产出给模型的工具描述；`MaterializeAsTools` 返回工具本身。

## 配置

无包级配置，也不读配置文件。`Registry` 由调用方通过 `NewRegistry()` 创建并注入 agent。

## 扩展点

- **新增工具**：实现 `Tool` 接口后 `Registry.Register` 即可。
- **按场景裁剪工具集**：用 `Registry.Clone()` 复制一份再 `Disable(name)`，
  避免影响其他 agent 的工具视图。
- 内置工具集见 [pkg/tool/builtin](tool/builtin.md)。

## Model Experience

模型根据工具名称、参数 schema 与描述生成调用；实现要返回明确成功/失败与必要输出。注册一个工具不等于已接入 CLI/TUI 或具备安全策略，宿主必须完成接线。

## Known Limitations

- `Registry` 本身不提供超时与权限控制：这两件事分别由 `pkg/tool/builtin` 的超时包装和
  `internal/permission` 承担。脱离这两者单独使用 `Registry` 时，工具会无限期执行。
- `Parameters` 是裸 `json.RawMessage`，本包不做 JSON Schema 校验——参数合法性由各工具
  自己负责。
