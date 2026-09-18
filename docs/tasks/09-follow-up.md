# 复审后的验证与优化任务

状态只在 [TASKS](../TASKS.md) 维护。先处理已重开的 P1/P2 问题，再执行体验和性能优化。实现前阅读 [AI 开发流程](../development/ai-workflow.md)。

<a id="resp-001"></a>

## RESP-001：Agnes Responses API 接入

**前置任务**：QA-001。优先级 P1；补齐 Agnes 3.0 Flash 的第三种官方协议入口，不能把
Chat Completions 或 Anthropic Messages 的结果冒充 Responses 支持。

**主要修改范围**：`pkg/provider/responses.go`、`pkg/provider/responses_test.go`、CLI provider
映射、Responses/Agnes 文档与脱敏真实验收证据。

**小步骤**：

1. 实现 Responses `input`、工具定义、function call/output 和非流式响应解析。
2. 实现 Responses SSE 文本、工具参数增量、完整参数、用量、完成和失败事件归一化。
3. 注册 `responses`、`openai-responses` 与 `agnes-responses`，明确默认端点和 `AGNES_API_KEY`。
4. 用本地 fake 服务覆盖请求形状、认证、流式/非流式、错误和工具循环，再用 Agnes 真实端点
   运行固定编码夹具并保存脱敏结果。

**验收条件**：

- [x] `llm.Model` 的 Generate/Stream 均走 `/responses`，并保留工具调用与用量语义。
- [x] Agnes 历史 function call 所需字段经过真实端点验证，工具结果可继续下一轮请求。
- [x] 本地协议回归与 CLI/工厂/密钥映射测试通过，默认测试仍不联网。
- [x] Agnes `agnes-3.0-flash` 真实 Agent 连续 3 次闭环通过，独立测试退出码均为 0，结果脱敏入库。
- [x] 文档、配置示例、真实验收说明和后续三次连续矩阵要求已同步。

**验证与证据**：见 [`RESP-001`](../development/evidence/RESP-001.md)；真实结果见同目录
`RESP-001-real-provider-2026-09-18.json`。

**范围外**：不实现服务端 `previous_response_id` 会话、不把固定夹具三次成功解释为所有请求的稳定成功率，
不在本任务中改造既有 Chat/Anthropic 适配器。

<a id="load-001"></a>

## LOAD-001：真实负载与稳定性验证

**前置任务**：RESP-001。优先级 P1；验证真实 Provider 在连续和并发 Agent 负载下的可用边界，不能把一次成功或脚本化模型结果写成稳定性结论。

**主要修改范围**：`pkg/provider/responses.go`、`pkg/provider/responses_test.go`、真实负载脱敏证据与发布状态文档。

**验收条件**：

- [x] 使用合成夹具执行 5 次连续真实 Agent 闭环，每次独立测试退出码为 0、实现文件确实发生修改。
- [x] 使用合成夹具执行 3 路并发真实 Agent 闭环，全部通过且无超时或进程残留。
- [x] 对 4 路并发的 429 结果保留失败证据，明确记录为 Agnes 账号/渠道容量边界，不把失败伪装成通过。
- [x] 修复空 `function_call_output` 和 Responses 流式 429/5xx 重试，并通过本地协议回归、全量测试和 race 门禁。

**验证与证据**：见 [`LOAD-001`](../development/evidence/LOAD-001.md) 和同目录脱敏 JSON。

**范围外**：不把当前渠道的 3 路并发上限推广为所有账号的服务承诺；更高并发需要 Provider 配额、队列或上层限流方案。真人 TUI 评价仍单独进行。

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

### 2026-09-16 再复审补充：B02/B03/B04/B06

依据：[本次复审](../development/evidence/REVIEW-2026-09-16.md) 与 [三个可运行探针](../development/review-reproduction-2026-09-16.md)。优先级 P1（可信测试与秘密处理），其次 P2（超时与 CI）。下列局部修复不依赖 UI；全任务验收仍等待 UI-003。

**按顺序独立交付**：

1. **秘密隔离**：盘点 runner 的工具输出、AgentError、toolSummary.Error、TestOutputTail、stdout/stderr 出口。Provider 凭证仅供 Provider 客户端；工具和验证子进程显式最小环境，保留必要 PATH/临时 HOME/Go cache 配置，不继承秘密。对上传产物和错误日志统一清洗，优先使用错误码/类别。不要仅修改 URL 清洗函数或依赖 Actions 日志遮罩。
2. **可信验证**：在模型工作区外保留原始测试与 go.mod，只复制允许的实现文件到独立验证目录；拒绝测试/构建配置篡改、符号链接和越界实现。记录原始测试哈希、实现差异/哈希、预期用例实际执行结果。用 `-count=1` 禁用缓存，明确拒绝没有执行预期测试的结果。修改文案不能代替机器断言。
3. **确定终态**：Agent 和独立测试分别使用有界预算；验证使用可取消进程与进程树回收、有界输出。初始化失败、Provider 失败、验证超时、取消都保存结构化结果与阶段。禁止静默跳过失败 artifact。
4. **持续门禁**：CI 添加默认无 tags 的全量测试；保留完整 tags、race、五平台和 PTY 必跑子集。配置和证据注明各 job 的真实范围。
5. **重新验收**：用本地假 Provider 同时验证错误场景失败与正确修复通过；前置恢复后重跑受影响产品验收，最后运行真实 Provider 并保存可信结果。

**新增验收**：

- [x] 合成秘密不会出现在工具返回模型的消息、结果 JSON 或 stdout/stderr 中；工具/测试子进程不通过继承环境获得 Provider key。
- [x] 错误 Add + 删除/跳过断言、改模块、只加注释均验收失败；正确实现通过，原始测试哈希完整且预期测试实际执行。
- [x] 超时/大输出/派生子进程夹具在短预算内终止，进程无残留、输出有界、失败结果文件存在且已脱敏。
- [x] 默认和完整 tags 的 CI 都实际运行；不能拿架构测试替代默认全量测试（run 35170954145 attempt 2 的 Quality/平台 job 全部通过；attempt 1 的 macOS Intel 网络失败已重跑）。
- [x] 两个 Provider 探针转正式回归；相关默认/完整测试通过，真实结果按 SHIP-001 原要求补证（2026-09-18 Agnes 双协议各连续 3 次）。

**范围控制**：修 runner 的凭证与验证边界，不顺带实现全平台 OS 沙箱；历史 JSON 原样保留并标局限。没有凭证的新协议记录缺项，不要求用户重发已提供的 key。

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

<a id="sec-001"></a>

## SEC-001：权限作用域上下文过滤

**前置任务**：QA-001。优先级 P2；以下基线描述的是修复前状态：规则表接受 `global`、`session`、`project` 字段，但匹配接口尚未接收会话/项目上下文，作用域当时只是存储元数据。

**小步骤**：

1. 扩展规则匹配调用的上下文契约，明确 global/session/project 的优先级、缺少上下文时的安全默认值和旧规则兼容方式。
2. 在 SQLite 查询、内存缓存和迁移路径统一应用作用域过滤；不能让跨会话缓存复用 session/project 规则。
3. 为同一工具的 global、session、project 规则补正向、负向、重启和并发测试，并在 `permission list/audit` 中展示足够的关联信息；会话切换与在途检查交错时也必须沿用检查开始时的上下文快照。
4. 更新权限指南、配置解释和证据，证明规则不会越过所属会话或项目。

**验收条件**：

- [x] 不同 session/project 的规则不会互相命中；global 规则按明确优先级生效。
- [x] 旧数据库规则可读，缺少关联字段时遵循文档化的安全默认值。
- [x] 内存、SQLite、迁移和缓存路径的测试均通过，含 race 和重启场景。
- [x] CLI、审计和文档能让用户区分规则作用域与实际命中结果。
- [x] 带参数的缓存决定保持精确参数匹配；glob 元字符不会把一次具体决定放大为工具级权限。

**验证**：`go test -race -tags 'sqlite memory' ./internal/permission ./cmd/basework -count=1`，配合隔离 HOME 的 CLI 规则场景；保存 `docs/development/evidence/SEC-001.md`。

实现与验证已完成；QA-001 已收口，SEC-001 可转为“完成”。

**范围外**：不把应用层规则包装成 OS 沙箱，不在本任务内实现远程策略服务或全平台权限代理。

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

<a id="opt-003"></a>

## OPT-003：降低长会话追加开销

**前置任务**：OPT-002。优先级 P3；依据 [本次复审](../development/evidence/REVIEW-2026-09-16.md) B07。当前瓶颈已测得，JSONL 增量追加优化已实施并完成验收，结果见 [OPT-003 证据](../development/evidence/OPT-003.md)。

**先读 / 主要修改范围**：`pkg/session/jsonl.go`、锁/恢复/压缩测试、`tests/benchmark/long_session_bench_test.go`、[OPT-002 原始基线](../development/evidence/OPT-002.md)。

**按顺序实施**：

1. 固定 100/1000/10000 事件和同样 payload，补 JSONL/SQLite 同条件对照、两次完整压缩快照体积与 pprof。记录 OS/CPU/Go/commit、冷/热条件、三次样本；已有 TUI 输入状态转换基准不能当端到端渲染延迟。
2. 根据 profile 选择增量追加或批处理最小方案，先写清 seq 分配、跨进程互斥、fsync 和崩溃恢复不变量。格式变更必须先写 ADR/兼容方案，不直接重写全部存储。
3. 用原日志回放、完整无换行尾、截断尾、中间损坏、多进程追加、取消后重启与双压缩恢复证明兼容；确保不会重复或丢失已确认事件。
4. 同环境重复基准，分别记录耗时、B/op、allocs/op、实际文件大小。最后回填 STATUS 和证据。

**验收目标（拟定目标，不是现有性能承诺）**：

- [x] 与 OPT-002 同条件三次样本，10000 事件总追加耗时中位数 ≤10 秒且较重测基线至少快 10 倍；累计分配 ≤1 GB。
- [x] 1000→10000 事件耗时增长 ≤20 倍，避免仍然是二次重写；profile 与方案记录见 OPT-003 证据。
- [x] 旧日志/锁/崩溃恢复/双压缩回归均通过，默认与完整 tags 兼容；用户数据不被静默迁移或丢弃。
- [x] 受影响平台存储测试及三次原始基准结果可追溯，代码和性能证据均已保存。

**验证**：既有 OPT-002 命令按相同规模执行，加入 SQLite 对照；`go test -race -tags "sqlite memory" ./pkg/session ./pkg/agent -count=1`。交付记录保存到 [OPT-003](../development/evidence/OPT-003.md)。

**范围外**：CLI 后端切换、远程会话服务、分页 UI、并发编辑不是此任务；先完成 P1 门禁修复。
