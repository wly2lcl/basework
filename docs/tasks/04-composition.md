# M4：配置与扩展生命周期

状态只在 [任务看板](../TASKS.md) 更新。执行前读 [AI 开发流程](../development/ai-workflow.md)。下面是实施范围，新增名称均为拟定；发现已有实现先验证缺口。每张卡可分多个小提交，但验收全部满足后才算完成。

<a id="cfg-001"></a>

## CFG-001：脱敏的有效配置解释

**前置任务**：BASE-001。依赖尚未完成时，只能调查或细化方案，不能宣称本项已验收。

**先读 / 主要修改范围**：`pkg/config/config.go`、`cmd/basework/`。首次阅读应确认实际文件/符号，拟新增路径不存在属正常。

**实施步骤**：

1. 核对 Discover/Load 与环境覆盖的实际顺序。
2. 拟新增 config explain 只读入口，输出来源路径和有效值。
3. 统一递归脱敏 key/token/password/授权头等敏感字段。
4. 补局部配置默认合并与嵌套配置验证。

**验收条件**：

- 输出与实际 runtime 使用值一致。
- 测试样例中的秘密不出现在 stdout/stderr。
- 解释配置不发网络请求且不改配置。

**针对性验证**：go test ./pkg/config ./cmd/basework -count=1。命令均从仓库根执行；必须记录结果和覆盖边界。

**交付与回填**：代码/文档 diff、必要测试，以及 `docs/development/evidence/CFG-001.md`。更新看板的状态和证据；有用户可见行为变化时更新对应指南及 STATUS。

**范围外**：不顺手改其他任务；公共 API、存储格式、默认权限的额外变化必须先写明影响并拆分。

<a id="cfg-002"></a>

## CFG-002：可组合启动预设

**前置任务**：CFG-001。依赖尚未完成时，只能调查或细化方案，不能宣称本项已验收。

**先读 / 主要修改范围**：`pkg/config/`、`cmd/basework/runtime.go`、`cmd/basework/profile.go`。首次阅读应确认实际文件/符号，拟新增路径不存在属正常。

**实施步骤**：

1. 在 ADR 固定预设覆盖次序与数组替换规则。
2. 拟增加最小 read-only/coding 预设并保留未指定时旧行为。
3. 启动时校验工具/权限与依赖。
4. 用 config explain 展示最终结果，不复用现有 pprof profile 子命令。

**验收条件**：

- 只读预设没有写/执行能力泄漏。
- 不存在的预设/冲突配置明确报错。
- CLI/TUI 使用相同配置解析，无热重载承诺。

**针对性验证**：go test ./pkg/config ./cmd/basework -count=1。命令均从仓库根执行；必须记录结果和覆盖边界。

**交付与回填**：代码/文档 diff、必要测试，以及 `docs/development/evidence/CFG-002.md`。更新看板的状态和证据；有用户可见行为变化时更新对应指南及 STATUS。

**范围外**：不顺手改其他任务；公共 API、存储格式、默认权限的额外变化必须先写明影响并拆分。

<a id="cfg-003"></a>

## CFG-003：注册资源归属与逆序释放

**前置任务**：BASE-001。依赖尚未完成时，只能调查或细化方案，不能宣称本项已验收。

**先读 / 主要修改范围**：`pkg/agent/plugin.go`、`pkg/tool/registry.go`、`pkg/tool/builtin/`、`pkg/hook/`、`cmd/basework/runtime.go`。首次阅读应确认实际文件/符号，拟新增路径不存在属正常。

**实施步骤**：

1. 列出现有注册资源和创建/关闭方，特别检查 builtin 包级 PathChecker/EventBus/TimeoutConfig 的跨实例覆盖；为产品运行时提供实例级注入，同时保留旧嵌入入口的兼容方式。
2. 设计可选 scoped registration 或产品适配层，不强改公开 Plugin。
3. 初始化部分失败时逆序释放本次资源。
4. 测试重复 Close、多实例、依赖缺失与失败回滚。

**验收条件**：

- 一个实例关闭不影响另一个。
- 失败注册不遗留工具/订阅/子进程。
- 保持 Initialize/Shutdown 现有兼容，不承诺 Go 动态插件热卸载。

**针对性验证**：go test -race ./pkg/agent ./pkg/tool ./pkg/hook ./cmd/basework -count=1。命令均从仓库根执行；必须记录结果和覆盖边界。

**交付与回填**：代码/文档 diff、必要测试，以及 `docs/development/evidence/CFG-003.md`。更新看板的状态和证据；有用户可见行为变化时更新对应指南及 STATUS。

**范围外**：不顺手改其他任务；公共 API、存储格式、默认权限的额外变化必须先写明影响并拆分。

<a id="cfg-004"></a>

## CFG-004：自定义模型端点配置

**前置任务**：BASE-001。依赖尚未完成时，只能调查或细化方案，不能宣称本项已验收。

本卡来自 REL-003 的真实模型验证：拿到可用凭证后才发现，**产品没有任何路径把模型请求指向自定义端点**。`cmd/basework` 的 `newRuntimeAgent` 调用 `provider.Create` 时从不设置 `Config.BaseURL`（全仓库非测试代码中 `BaseURL:` 赋值 0 处），配置文件也没有 `base_url` 字段；而 `docs/guides/cli-guide.md` 却教用户写 `providers.<name>.base_url`——该 schema 并不存在，写了会被**静默忽略**（实测把 base_url 指向不可达地址，请求仍打到默认端点并返回 401）。后果是：中转站、企业网关、自建兼容服务这三类用户按文档配置后不会报错，只会得到莫名其妙的鉴权失败。

**先读 / 主要修改范围**：`cmd/basework/agent.go`、`cmd/basework/runtime.go`、`pkg/config/config.go`、`docs/guides/cli-guide.md`。首次阅读应确认实际文件/符号，拟新增路径不存在属正常。

**实施步骤**：

1. 先写清现状与缺口：`provider.Config.BaseURL` 的注入点只有可嵌入 API，CLI 无入口。
2. 定配置载体并写轻量 ADR：配置文件 provider 级 `base_url` + key（与 `ollama.endpoint` 的既有做法保持一致），或采用标准环境变量；明确它与 `BASEWORK_PROVIDER` 的优先级关系。
3. 接线：把生效端点的 base_url 与 key 传到 `provider.Create`，覆盖需要自定义端点的类型（`openai-compat` 必填，`openai`/`anthropic` 为覆盖默认）。
4. 修正文档：让 `cli-guide.md` 的示例与真实 schema 一致，或让代码真正实现它；二者必须一致。
5. 补边界测试：端点生效、缺 key 明确报错、不可达端点不静默回落、`config explain` 不泄漏 key。

**验收条件**：

- 配置/环境变量给出的 base_url 确实生效——请求打到该端点而不是默认端点（用本地 httptest 或不可达地址即可证明）。
- 缺 key 或端点不可达时给出明确错误，**不静默回落到默认端点**。
- 未配置自定义端点时行为与改动前完全一致（旧嵌入路径与旧配置保持兼容）。
- `pkg` 不依赖 `internal`；默认与 `sqlite memory` 两种构建兼容；凭据不出现在 `config explain`、日志或错误信息里。

**针对性验证**：go test ./cmd/basework ./pkg/config ./pkg/provider -count=1；随后用 REL-003 的隔离场景重跑 CLI 路径并记录结果。命令均从仓库根执行；必须记录结果和覆盖边界。

**交付与回填**：代码/文档 diff、必要测试、ADR，以及 `docs/development/evidence/CFG-004.md`。更新看板的状态和证据；有用户可见行为变化时更新对应指南及 STATUS。

**范围外**：不顺手改其他任务；不改各 Provider 的默认端点常量，不改权限默认值，不引入新的第三方依赖。
