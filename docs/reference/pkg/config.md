# pkg/config

## 用途

配置的读写与原子持久化。所有运行期开关（模型、provider、权限、重试、压缩、会话目录、
可观测性、键位……）都定义在这里的 `Config` 结构下，由 `Store` 统一管理。

- `Load(path)` / `NewStore(path)` — 从文件加载或新建。
- `Get()` — 取当前配置快照。
- `Mutate(fn)` — 串行化地修改配置（避免并发写互相覆盖）。
- `Save()` / `Reload()` — 落盘与重读。
- `Discover()` — 从当前目录逐级向上查找 `config.json`，找不到时回退到用户主目录。

## 配置

- 配置文件名约定为 **`config.json`**。
- 路径来源二选一：显式传入，或用 `Discover()` 自动查找。
- 主要分组（均为 `Config` 的字段）：provider 与模型、`CompactionConfig`、`PermissionConfig`、
  `RetryConfig`、`SessionConfig`、`QueueConfig`、`ObservabilityConfig`、`SecurityConfig`、
  `SensitivePathsConfig`、`LoopDetectConfig`、`KeybindingsConfig`、`AutoTitleConfig`、
  `PromptCacheConfig`、`ProfilingConfig`、`DatabaseConfig`，以及 `OAuthConfig` /
  `AzureConfig` / `BedrockConfig` / `CopilotConfig` / `OllamaConfig` / `OpenCodeConfig`。

## 扩展点

- 新增配置项：在 `Config` 下加字段（新增可选字段属于兼容变更），并在产品层接线。
- 自定义加载来源：`Discover()` 之外自行决定路径后交给 `Load`。

## Model Experience

本包不直接提供模型工具。调用方将有效配置传给 runtime；密钥和内部配置不能直接拼入模型提示。配置加载成功也不代表 provider 能连接或某模型支持工具。

## Known Limitations

- **配置里会存放 API Key，本包不做加密或脱敏**。文件权限与存放位置由使用者负责。
- `Store` 绑定单个文件路径；同时打开同一路径的两个 `Store` 不会互相感知，
  并发写会以后写者为准。
- `Discover()` 只认 `config.json` 这一种文件名，且从**当前工作目录**开始向上查找——
  在子目录里启动进程可能选中意料之外的配置。
