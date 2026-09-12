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
