# 复审后的验证与优化任务

状态只在 [TASKS](../TASKS.md) 维护。先处理已重开的 P1/P2 问题，再执行体验和性能优化。实现前阅读 [AI 开发流程](../development/ai-workflow.md)。

<a id="qa-001"></a>

## QA-001：可复跑的产品验收与证据门禁

**前置任务**：BASE-001, UI-003。优先级 P2；服务于当前发布验收，不新增业务功能。依赖未完成时可准备夹具和失败断言，但最终正向验收等待产品修复。

**先读 / 主要修改范围**：`tests/tui_pty/`、`examples/embed/`、`.github/workflows/build.yml`、`docs/development/validation.md`、[复审报告](../development/evidence/REVIEW-2026-09-14.md) A13/A14。

**小步骤**：

1. 把夹具资源从脚本所在仓库目录读取；只把 HOME、构建产物、测试副本和日志放到随机临时目录。发现当前 Go 工具链路径，允许显式覆盖，不硬编码某个人的 SDK。
2. 修 scenario5 的示例路径；用仓库外临时 Go module 加 replace 引用本仓库，验证仅用 pkg 公共接口可编译。内部 Service 范例明确标产品内部，不能伪称外部 API。
3. 去掉 sleep 作为并发同步依据；读完订阅通道后再核对示例的事件统计。测试失败也回收 PTY、服务与自己创建的子进程。
4. 恢复场景按相同 session ID、历史唯一标记、具体 facts 和模型请求内容断言；移除“切换到会话”等宽松替代条件。假模型不含旧消息时必须拒绝确认已恢复。
5. 将真实模型场景夹具与脱敏输入/结果结构保存到仓库；密钥仅环境注入，不保存原始秘密，失败样本也记录。临时路径只能是输出目录，不能是唯一复现材料。
6. CI 保留默认及完整 tag 测试；race 增加 internal/runtime、jobs、edits。增加 Unix PTY 小型必跑集，完整场景可按成本分组。基准命令必须用 -bench，不把 no tests to run 记为性能验证。
7. 扫描活跃文档中声明的 CLI 命令/flag 和配置生效说明，对实际 Cobra 命令树、Config schema 与 runtime 消费链做验证；先修影响使用的例子，历史归档不当作当前操作入口。

**验收条件**：

- [x] 全新 checkout + 全新测试目录可一条命令准备并运行，失败给出清楚原因，清理后无该测试的进程残留。
- [x] 更换为未知会话 ID 时验收明确失败且不创建新会话；正常会话切换场景通过。
- [x] 在临时副本移除 session 绑定接线后，scenario6 负向验收失败；说明接线断开不会被宽松断言掩盖。
- [x] 外部 module 的嵌入示例编译、运行均通过；无需导入 internal。
- [x] CI job 的具体范围、commit 和结果可查，真实模型与脚本化模型证据分开。

**验证**：Python 入口在空目录运行；`go test -race -tags "sqlite memory" ./internal/runtime ./internal/jobs ./internal/edits ./internal/tui ./cmd/basework -count=1`；外部 module 与 CLI help 对照；保存 `docs/development/evidence/QA-001.md`。

**范围外**：不把内部 runtime 整包搬入 pkg，不修改平台支持承诺，不发布。

<a id="opt-001"></a>

## OPT-001：非交互初始化

**前置任务**：CFG-002。优先级 P3；改善 AI/脚本安装体验。

**先读 / 主要修改范围**：`cmd/basework/init.go`、`pkg/config/`、`docs/installation.md`。

**小步骤**：

1. 复现 stdin 保持打开而没有输入时 init 持续等候；明确既有交互模式与 EOF 的兼容行为。
2. 设计显式非交互入口（例如 --yes，为拟新增参数），提供 provider/model/preset 的确定默认值和错误信息。
3. 已存在配置默认不覆盖；若需覆写，提供独立明确选项和备份策略。密钥不回显，不把环境秘密写入日志。
4. 更新实际 help、安装文档与隔离安装夹具。

**验收条件**：

- [x] 无人值守模式在空目录有确定终态；缺必填值退出非零，不无限等待。
- [x] 旧交互模式可继续使用；已有配置保持不变，敏感值不泄漏。
- [x] stdin EOF、保持打开的空管道和异常输入均有测试，非交互路径不读取 stdin。
- [x] 真实 PTY 交互输入已由 `tests/opt_pty_init.py` 验证；Windows 目标终端仍属发布平台边界。

**验证**：`go test ./cmd/basework ./pkg/config -count=1`，隔离 HOME 的真实二进制初始化；保存 `docs/development/evidence/OPT-001.md`。

<a id="opt-002"></a>

## OPT-002：长会话与长任务性能基线

**前置任务**：BASE-001。优先级 P3；先测量再决定存储/渲染优化。

**先读 / 主要修改范围**：`tests/benchmark/`、`pkg/session/`、`internal/jobs/`、`internal/tui/`。

**小步骤**：

1. 实际执行基准：固定 100/1000/10000 事件、大小输出与同条件快照，记录操作系统、CPU、Go、commit、样本数。
2. 分别量化 JSONL 追加/投影、SQLite、两次压缩快照体积、摘要读取、jobs 历史轮询与 TUI 输入延迟。
3. 用 `-benchmem` 和 pprof 定位增长原因；区分冷缓存、热缓存和模型网络等待，不把一次快跑当性能结论。
4. 依据数据拆出小优化任务（例如增量投影、分页历史、缓存或后端选择）；先写兼容与预算目标，不直接重写存储。
5. 对长时间无输出场景记录调用开始、首 token、最后活动、取消耗时；确认默认/调用方超时边界，再决定是否独立增加空闲超时。

**验收条件**：

- [x] 有可重复的基准命令、原始结果与至少 3 次样本；不能用普通 go test 的 no tests to run 替代。
- [x] 比较使用相同数据集和环境；内存、延迟、日志体积分别记录。
- [x] 优化建议有证据与验收阈值，尚未测量的项目明确标未知。

**验证**：`go test -tags "sqlite memory" ./tests/benchmark -run '^$' -bench . -benchmem -count=3`；独立 TUI/长任务夹具；保存 `docs/development/evidence/OPT-002.md`。
