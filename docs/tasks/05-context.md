# M5：项目事实与恢复

状态只在 [任务看板](../TASKS.md) 更新。执行前读 [AI 开发流程](../development/ai-workflow.md)。下面是实施范围，新增名称均为拟定；发现已有实现先验证缺口。每张卡可分多个小提交，但验收全部满足后才算完成。

<a id="ctx-001"></a>

## CTX-001：统一工作区事实模型

**前置任务**：JOB-004, EDIT-003。依赖尚未完成时，只能调查或细化方案，不能宣称本项已验收。

**先读 / 主要修改范围**：`pkg/session/filetrack.go`、`pkg/session/event.go`、`internal/tools/`。首次阅读应确认实际文件/符号，拟新增路径不存在属正常。

**实施步骤**：

1. 核对已有文件追踪避免重复模型。
2. 定义读取/修改/命令/诊断的事实来源和 workspace ID。
3. 为 job/edit 结果建立关联。
4. 补事件版本迁移与重复投影处理。

**验收条件**：

- 相同事实不重复累计。
- 不同工作区隔离。
- 持久化升级可读旧数据并拒绝未知未来版本。

**针对性验证**：go test -tags "sqlite memory" ./pkg/session ./internal/tools -count=1。命令均从仓库根执行；必须记录结果和覆盖边界。

**交付与回填**：代码/文档 diff、必要测试，以及 `docs/development/evidence/CTX-001.md`。更新看板的状态和证据；有用户可见行为变化时更新对应指南及 STATUS。

**范围外**：不顺手改其他任务；公共 API、存储格式、默认权限的额外变化必须先写明影响并拆分。

### 2026-09-14 复审补充

**优先级**：P1。问题依据：[复审报告](../development/evidence/REVIEW-2026-09-14.md)（A04、A08）；先复跑相关 [最小复现](../development/review-reproduction.md)。原实现与历史测试保留，以下是继续工作清单。

**小步骤**：

1. 先复跑 TestReviewRuntimeEditPersistsFacts 与 TestReviewPartialCommitFacts；确认当前生产链路没有 SaveWorkspaceFacts 调用方。
2. 实现单一的事实投影/保存入口，由实际读/编辑/job 终态接线；启动从持久化事件与日志恢复增量，不依赖手工种子 facts 文件。
3. 按单文件 outcome 折叠部分成功，不依赖整批 phase=committed；明确普通 read/write/edit/apply_patch/bash 与 edit_files 的覆盖范围，避免只覆盖一条小众路径却宣称全部。
4. 为每条事实保留可回到原事件/job 的引用、观测时间和必要基线；采用版本迁移兼容旧文件，避免重复投影、并发覆盖或跨工作区污染。
5. 修 facts show 与恢复面板的选取范围：较早事件不能因固定 Limit 截断导致最近事实消失；未知高版本/损坏不要伪装没有事实。

**补充验收**：

- [ ] 从空数据目录真实读→改→跑→关闭→重启，事实存在且不重复；不直接调用 SaveWorkspaceFacts 准备成功前提。
- [ ] 部分成功、失败原因、不同工作区、同文件多次变化均有可追溯结果。

**针对性验证**：go test -race -tags "sqlite memory" ./pkg/session ./internal/tools ./cmd/basework -count=1。记录在本任务原证据文件的新日期段，保留旧记录；满足全部原有与补充验收后才更新看板。

<a id="ctx-002"></a>

## CTX-002：受预算约束的项目摘要

**前置任务**：CTX-001。依赖尚未完成时，只能调查或细化方案，不能宣称本项已验收。

**先读 / 主要修改范围**：`pkg/agent/environment.go`、`internal/compaction/`、`pkg/session/`。首次阅读应确认实际文件/符号，拟新增路径不存在属正常。

**实施步骤**：

1. 确定上下文提供者的现有接口。
2. 按相关性与预算选文件/命令事实并附来源。
3. 检测文件哈希变化，把旧事实标为过期。
4. 摘要按请求审计策略注入且允许关闭。

**验收条件**：

- 不会无上限扫描整个仓库。
- 来源可追踪且过期事实有标识。
- 关闭后恢复旧行为，不读取保护路径。

**针对性验证**：go test ./pkg/agent ./internal/compaction ./pkg/session -count=1。命令均从仓库根执行；必须记录结果和覆盖边界。

**交付与回填**：代码/文档 diff、必要测试，以及 `docs/development/evidence/CTX-002.md`。更新看板的状态和证据；有用户可见行为变化时更新对应指南及 STATUS。

**范围外**：不顺手改其他任务；公共 API、存储格式、默认权限的额外变化必须先写明影响并拆分。

### 2026-09-14 复审补充

**优先级**：P1。问题依据：[复审报告](../development/evidence/REVIEW-2026-09-14.md)（A04、A07）；先复跑相关 [最小复现](../development/review-reproduction.md)。原实现与历史测试保留，以下是继续工作清单。

**小步骤**：

1. 在 CTX-001 的真实事实基础上刷新请求摘要；关闭功能时不加载事实和文件，开启时明确启动/每轮刷新时机。
2. 修 Provider Collect 每次丢弃 tracker：持久化或从事实来源重建观测哈希，文件内容未重新核验前，不能把过期基线重置为新鲜。
3. 最终文本的标题、正文、截断提示全部计入 Budget；增加单文件与总读取字节限制，过大文件标未知而非整文件读内存。
4. 规范 Windows 路径：反斜杠 ..、盘符相对 C:、根相对、UNC 与软链都经过实际根目录检查。保护回调收到的应是验证过的目标。
5. 保留未知/不可读/过期三态；连续两次 Collect 与进程重启后都验证标记，摘要跟随 request.built 核对。

**补充验收**：

- [ ] 80 字节等小预算下最终输出不超限；超大文件不会触发无界读取。
- [ ] 事实文件变化能在产品下一次请求中出现；来源明确，过期事实不被当作当前文件状态。
- [ ] Windows 目标系统用外部临时文件证明不能越界读取；默认关闭行为保持兼容。

**针对性验证**：go test -race ./pkg/agent ./pkg/session ./cmd/basework -count=1。记录在本任务原证据文件的新日期段，保留旧记录；满足全部原有与补充验收后才更新看板。

<a id="ctx-003"></a>

## CTX-003：重启与压缩后的项目恢复

**前置任务**：CTX-002, REL-004, RUN-003。依赖尚未完成时，只能调查或细化方案，不能宣称本项已验收。

**先读 / 主要修改范围**：`cmd/basework/session.go`、`pkg/session/`、`internal/compaction/`。首次阅读应确认实际文件/符号，拟新增路径不存在属正常。

**实施步骤**：

1. 构建读改跑测试的会话样例。
2. 让 `compacted` 事件保存压缩器实际产出的完整 `compacted.Messages` 快照；投影遇到快照时精确替换此前历史，验证选择、删除和重排消息的策略都可恢复。
3. 保留旧的无快照事件兼容分支，按 `Summary`、`TruncatedSeq`、`KeepFrom` 解释既有日志。
4. 触发两次压缩再重启恢复，核对摘要来源、job 终态、文件冲突与工具调用配对。
5. 保存真实流程证据，并记录完整快照带来的日志体积增量。

**验收条件**：

- 恢复不丢失最近变更与失败原因。
- 压缩后的完整消息快照与策略输出逐项一致；多次压缩和重启后不重复、不丢消息，也不吞 system prompt 或 steering。
- 旧的无快照压缩事件仍可按旧字段恢复；新快照支持选择和重排，不能再用单一头部截断锚点代替。
- 已持久化的 steering 会在后续请求中全部重放；本任务需记录其累积行为与上下文增长，不将其误写成一次性消费。
- 旧运行中任务被明确标为中断。

**针对性验证**：go test -tags "sqlite memory" ./pkg/session ./pkg/agent ./internal/compaction ./cmd/basework -count=1；人工重启验证。命令均从仓库根执行；必须记录结果和覆盖边界。

**交付与回填**：代码/文档 diff、必要测试，以及 `docs/development/evidence/CTX-003.md`。更新看板的状态和证据；有用户可见行为变化时更新对应指南及 STATUS。

**范围外**：不顺手改其他任务；公共 API、存储格式、默认权限的额外变化必须先写明影响并拆分。

### 2026-09-14 复审补充

**优先级**：P1。问题依据：[复审报告](../development/evidence/REVIEW-2026-09-14.md)（A02、A03）；先复跑相关 [最小复现](../development/review-reproduction.md)。原实现与历史测试保留，以下是继续工作清单。

**小步骤**：

1. 先修 RUN-003 的行为选项接线；明确区分已有快照投影能力与缺少的旧会话重新绑定能力。
2. 新增可选旧 session ID 的核心/产品入口，并以 ADR 固定创建、恢复、未知 ID、工作区归属、并发占有、system prompt 与 steering 的规则；保持现有 WithSession 兼容。
3. 让 CLI/TUI 显式恢复指定旧会话；将同一个旧 owner 传入 job 恢复及 facts/编辑读取，不能静默新建会话后显示“已恢复”。
4. 用真实二进制+本地脚本化 Provider 跑读改命令，实际触发两次压缩，退出进程后按相同 ID 继续。
5. 核对恢复请求的消息、工具配对、steering、文件状态和 job 终态；旧快照单测保留，不能手工 AppendEvent 代替完整流程。

**补充验收**：

- [ ] 重启前后选中的 session ID 一致，模型请求确含旧事实；丢失旧历史或关闭压缩后测试必须失败。
- [ ] 旧 running job 明确 interrupted 且无自动重跑；两次快照结果和审计可重建。

**针对性验证**：go test -race -tags "sqlite memory" ./pkg/session ./pkg/agent ./internal/compaction ./cmd/basework -count=1。记录在本任务原证据文件的新日期段，保留旧记录；满足全部原有与补充验收后才更新看板。
