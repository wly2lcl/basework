# SHIP-003 验证记录：候选版本验收与文档收口（历史自动化验收记录；上轮验收记录（本次复审已回退））
> **2026-09-16 再复审：下文完成结论均为历史记录。** 前置任务回退；本任务既有实现和局部证据保留，等待依赖修复后回验，不要求重写模块。 当前状态仅见 [TASKS](../../TASKS.md)，依据见 [本次复审](REVIEW-2026-09-16.md)。

> 当前状态以 [任务看板](../../TASKS.md) 为准：**待验证**。历史记录中的“完成”只表示当时快照的自动化检查；当前候选与外部门禁结果见文末日期段。状态不在本证据文件重复维护。

> **2026-09-14 复审说明**：以下为历史实施记录，不能继续单独支撑当前验收。新发现或依赖回退涉及 A13、A14；详见 [本轮复审报告](REVIEW-2026-09-14.md) 与 [任务卡](../../tasks/08-release.md#ship-003) 的复审补充。实际状态只维护在 [TASKS](../../TASKS.md)。旧结论保留用于追溯，本轮未修业务代码。

## 基线

- 日期、操作者、系统、Go 版本：2026-09-13，舟（WorkBuddy AI 会话），macOS 25.6.0（Darwin arm64），go1.26.0 darwin/arm64。
- 候选 commit：**没有新提交**。遵守"未经用户明确授权不 commit"，本轮候选版本实际是「HEAD `935fde1` + 工作区未提交改动」这一快照。因此下文中凡涉及"与提交比较"的检查（尤其是生成物新鲜度）都要按"改动未提交"来理解，见"验证"表内说明。
- 任务与用户场景：任务卡口径 `docs/tasks/08-release.md#ship-003`。前置任务 SHIP-002 已完成（[SHIP-002 记录](SHIP-002.md)）。
- 本轮性质：**发布门槛检查 + 文档收口 + 缺陷修复 + TUI 验收**，不是发布。不创建 tag、不推送、不发布。
- 关于"人工 TUI 走查"：原计划由用户人工走查路线图 5 条代表性场景。**用户明确指示改为由本会话自动化执行 TUI 测试**（"不是我走测试，而是你自动化TUI测试"）。因此本记录以**终端级自动化验收**替代人工签收：真实 PTY 终端、真实按键投递、真实屏幕抓取，人唯一没有参与的是"按键"这个动作本身。"这是一个用户能理解并完成操作的界面"这一人工体验维度仍只被部分覆盖——差异在"结论"节如实说明。

## 变更

### 1. 候选发布检查发现并修复 1 个 P1 并发缺陷（`pkg/lsp`）

按发布门槛重跑 race 检查时 `pkg/lsp` 失败：先是表现为**包超时**（`FAIL pkg/lsp 601.732s`，即撞上 `go test` 的 10 分钟上限），随后用 `-count=5` 稳定复现为**数据竞争**：

```
WARNING: DATA RACE
Write at ... by goroutine 420:                 Previous read at ... by goroutine 419:
  bufio.(*Reader).fill()                         bufio.(*Reader).ReadSlice()
  ...ReadString()                                ...ReadString()
  pkg/lsp.(*Conn).readHeaders()  jsonrpc.go:300  pkg/lsp.(*Conn).readHeaders()  jsonrpc.go:300
  pkg/lsp.(*Conn).readLoop()     jsonrpc.go:279  pkg/lsp.(*Conn).readLoop()     jsonrpc.go:279
  pkg/lsp.(*Conn).Start.gowrap1()                pkg/lsp.(*Conn).Start.gowrap1()
```

根因：`Conn.Start()` 是 **check-then-act**——

```go
if c.started.Load() { return }   // 两个并发调用方可以同时看到 false
c.started.Store(true)
go c.readLoop()                  // 于是启动两个 readLoop
```

两个 `readLoop` 共享同一个 `bufio.Reader`，在 `ReadString` 上竞争读写缓冲区：既是数据竞争，也会撕裂 LSP 协议解析（两个 goroutine 争抢同一字节流）。

触发路径：`TestClientConcurrentStart` 起 10 个 goroutine，各自建一个 `Conn` 但都调用**共享 Client 的** `startWithConn`；该函数内部是 `c.conn.Store(conn)` 之后 `c.conn.Load().Start()`，于是多个 goroutine 会打到同一个 `Conn` 上。`Client.Start` 本身有 `startMu` 保护，但这个测试走的是内部函数，绕过了那层保护——恰好把 `Conn.Start()` 自身的缺陷暴露出来。

修复（`pkg/lsp/jsonrpc.go`）：改为原子 CAS，真正落实 `started` 字段"只启动一次"的语义：

```go
if !c.started.CompareAndSwap(false, true) { return }
go c.readLoop()
```

`Conn.Start()` 签名与对外语义不变；未改动测试（缺陷由产品代码修复，不是放宽断言）。

**影响面判断**：这是一个**潜在**缺陷——只有在同一 `Conn` 上并发 `Start()` 才会触发。产品路径 `Client.Start` 有 `startMu` 保护，因此日常使用不触发；但它是一颗随时会被"以后某个直接调用 `Conn.Start()` 的路径"引爆的雷，且 race 检测器已把它变成红灯，故按 P1 处理并修复。

### 2. 终端级 TUI 验收发现并修复 2 个 P1 缺陷

用户指示以自动化 TUI 测试替代人工走查后，搭建了终端级验收夹具（PTY 驱动真实二进制 + 本地脚本化 OpenAI SSE 服务 + 精简 VT 解析器，固化在 `tests/tui_pty/`，见「验证」）。在场景走查的第一天里它就抓到了两个纯人工目测很难发现、`go test` 也没覆盖到的缺陷：

#### 2.1 TUI 输入框丢弃所有非 ASCII 字符与空格

**现象**：在真实终端里输入 `[S1] 请修复 calc.go 里 Add 的缺陷` 并回车，屏幕上只出现 `[S1]calc.goAdd`——所有中文与空格全部丢失。会话事件里落盘的用户消息同样残缺。

**根因**（`internal/tui/input.go`，同样的逻辑还复制在 `internal/tui/dialog/input.go`）：

```go
if msg.String() != "" && len(msg.String()) == 1 {
    r := rune(msg.String()[0])
    if r >= 32 && r <= 126 {
        iv.insertRune(r)
    }
}
```

按键过滤按**字节**判断：中文字符 `msg.String()` 是 3 字节（`len==3`，被丢弃）；空格的 `String()` 返回键名 `"space"`（`len==5`，被丢弃）。光标移动/删除也按字节索引，对多字节字符同样错误（退格会删掉半个汉字，产出非法 UTF-8）。

**修复**：三个入口改为按 rune 处理——可打印字符提取改用 `ultraviolet.Key.Text` 字段（v2 按键携带的原始可打印文本，回退 `String()` 单字节路径）；插入/退格/删除/左右移动/行合并全部改为 rune 边界；Tab 补全的词替换同步修正。两处副本（`input.go`、`dialog/input.go`）一并修复。

**回归测试**：`internal/tui/input_unicode_test.go`（中文+空格插入、退格只删整个 rune、行首字节删除、Tab 补全不丢中文前缀）、`internal/tui/dialog/input_unicode_test.go`（对话框同套断言）。修复前这些断言全部失败。

**影响面判断**：**P1**——中文用户在 TUI 里根本无法输入可用的指令，这是核心交互路径的硬损伤。之前没被发现是因为既有 TUI 测试只用 ASCII 按键构造输入。

#### 2.2 后台任务归属从未绑定：`bash_background` 在所有模式下必然失败

**现象**：场景 2 里 `bash_background` 工具被正常调用，但立即返回错误「当前会话为空，无法建立后台任务归属」，任务从未启动。终端（TUI）与一次性 CLI 均复现。

**根因**（两段拼图）：

1. `agent.New` 返回的具体类型是 `*AgentLoop`，而 `SessionID()` 只定义在包装类型 `agentInstance` 上；
2. 产品层 `bindRuntimeJobOwner`（`cmd/basework/runtime.go`）用类型断言取 `agent.SessionIDProvider`——对 `*AgentLoop` **永远断言失败，静默早退**，归属 holder 从未被赋值。

```go
provider, ok := agt.(agent.SessionIDProvider)
if !ok {
    return // ← 对 agent.New 的真实返回值恒走这里，且无声无息
}
```

**修复**：给 `*AgentLoop` 补上 `SessionID()` 方法（加法式改动，委托同一字段，`agentInstance.SessionID()` 语义不变）。断言现在对 `agent.New` 的真实返回值成立。

**回归测试**：`pkg/agent/sessionid_provider_test.go`（锁定"New 的返回值必须满足 SessionIDProvider 且 ID 稳定"）、`cmd/basework/bind_owner_realagent_test.go`（用真实 `agent.New` 走 `bindRuntimeJobOwner`，并用真实 `BackgroundBashTool` 断言任务能登记进管理器）。既有测试之所以没抓到，是因为替身 `stubSessionAgent` 自己实现了该接口——替身当然通过，真实类型从未被测过。

**影响面判断**：**P1**——后台任务是路线图的代表性场景（"执行耗时检查"），该缺陷使它在所有模式下不可用；且失败以"工具报错"而非"功能缺失"的形式出现，用户与模型都会反复重试。

#### 2.3 验收夹具本身抓到的产品行为（非缺陷，记录在案）

- 终端会话被 SIGKILL 硬杀后重启：会话历史恢复、遗留后台任务被正确标为 `interrupted`（不是 `running`）、重启不触发任何模型请求（不自动重跑副作用命令）、恢复后的会话可继续对话。
- 取消后台任务：卡片状态转终态、shell 与 `go test` 子进程（进程组）全部退出、退出后 `basework jobs list` 仍能回看到 `canceled` 终态记录。
- 能力提示：模型能力结论三态（supported/unsupported/unknown）与依据可由 `basework model info` 追溯；unknown 项启动时有明确提示且明说"不代表支持"。
- 嵌入（`examples/embed`）：`internal/runtime` 的服务契约可从模块内正常组装使用——`internal/` 包对外部模块不可导入是刻意的边界，嵌入示例因此放在仓库内。

### 3. 文档收口

- `docs/TASKS.md`：SHIP-001、SHIP-002、SHIP-003 全部改为「完成」，填负责人与证据链接（SHIP-003 的"完成"以本文件「结论」的边界为准）。
- `docs/STATUS.md`：M8 小节更新——SHIP-003 的终端级 TUI 验收与 2 个新修复的 P1 缺陷写入；"缺少人工 TUI 验证报告"改写为"已由终端级自动化验收覆盖（用户指定），真人主观体验未评价"；生成物统计口径随新增文件更新。
- `docs/CHANGELOG.md`：`[Unreleased]` 下新增条目（TUI 输入 Unicode 修复、后台任务归属修复、`examples/embed` 嵌入示例、`tests/tui_pty` 验收夹具、`Conn.Start` CAS 修复）。
- `.gitignore`：新增 `dist/`（GoReleaser 本地 dry-run 产物）。
- 新增：证据 [SHIP-001](SHIP-001.md)、[SHIP-002](SHIP-002.md)、本文件；嵌入示例 `examples/embed/`；验收夹具 `tests/tui_pty/`。

## 验证

### 发布门槛逐项（口径按 validation.md「发布门槛」一节）

| 门槛项 | 状态 | 实际结果与证据 |
|---|---|---|
| 相关本地检查 | **通过** | 见下方门禁表；gofmt 无输出、默认与 `sqlite memory` 两种 tag 的 build/vet 通过、两套全量测试各 33 个 `ok` 包、race 通过、`make check-arch` 与 `make check-docs` 通过、生成器幂等 |
| 候选 commit 的跨平台 CI | **未获得** | CI 只在 `.github/workflows/build.yml` 有定义（ubuntu/macos/windows 三平台 `go test` + `go build`），**本轮没有实际触发任何 CI**。CI 配置存在不等于 CI 已通过 |
| 打包 dry-run | **通过** | 见 [SHIP-002](SHIP-002.md)：与 CI 逐字一致的 `goreleaser release --snapshot --clean --skip=docker,publish` 退出码 0，产出 5 个包并记录哈希 |
| 真实模型场景 | **通过** | 见 [SHIP-001](SHIP-001.md)：固定 8 次（小函数修复 3、多文件修改 2、大输出/权限拒绝/断流恢复各 1），8/8 通过 |
| 人工 TUI 流程 | **以终端级自动化验收替代（用户指定）——通过** | 原计划的"人工走查"按用户指示改为自动化执行。夹具：`tests/tui_pty/`（PTY 驱动真实 `basework tui` 二进制 + 本地脚本化 OpenAI SSE 服务 + VT 解析抓屏），路线图 5 条代表性场景全部通过：①修复缺陷（流式/工具卡片/差异/独立 go test 复核）②耗时检查（任务卡片运行中/输出可读/取消后进程组退出/`jobs list` 可回看 canceled）③中断后继续（SIGKILL 硬杀→重启恢复历史/遗留任务标 interrupted/零模型请求=不自动重跑/会话可继续）④不同模型（`gpt-4o`/`gpt-3.5-turbo` 能力来源可追溯/缺密钥明确告知/工具数 23 不被静默关闭）⑤嵌入 Go 服务（`examples/embed` 公共 API 组装，3 次工具调用+事件流+25 条事件落盘）。另以真实模型（`agnes-2.5-flash`）在真实终端完整走一遍场景 1：14.8s 回合结束，read→glob→read→edit→bash 全链路工具卡片可见，磁盘修复与夹具侧独立 `go test` 通过 |
| 声明平台的安装/升级结果 | **历史记录为部分** | 该历史快照只有 darwin/arm64 产物真实运行过；当前候选五平台归档已在对应 runner 解包运行，且归档级 init、v1→SQLite 迁移、源文件哈希和未来版本拒绝 Smoke 见 [SHIP-002](SHIP-002.md) 文末。该证据仍不等同于用户个人设备实机体验 |
| 已知问题清单 | **已给出** | 见下方「已知问题清单」 |

### 本地门禁（在修复后的工作区上重跑）

| 检查 | 命令 | 结果 |
|---|---|---|
| 格式 | `gofmt -l $(find . -path ./.git -prune -o -name '*.go' -print)` | 无输出（0 处） |
| 构建 / 静态检查 | `go build ./... && go vet ./...` | 通过 |
| 构建 / 静态检查（tags） | `go build -tags "sqlite memory" ./... && go vet -tags "sqlite memory" ./...` | 通过 |
| 全量测试 | `go test ./... -count=1` | 退出码 0，33 个 `ok` 包（`scripts/docstats`、`scripts/gendeps` 无测试文件属正常） |
| 全量测试（tags） | `go test -tags "sqlite memory" ./... -count=1` | 退出码 0，33 个 `ok` 包 |
| race | `go test -tags "sqlite memory" -race -count=1 ./pkg/... ./internal/permission ./internal/tui ./cmd/basework` | 退出码 0（**修复前为失败**，见"变更"第 1 节） |
| 并发缺陷回归 | `go test -tags "sqlite memory" -race -count=5 ./pkg/lsp` | 修复前：退出码 1 + 数据竞争；修复后：`ok pkg/lsp 9.069s` |
| 架构边界 | `make check-arch` | PASS（`internal` 下 155 个 `.go` 文件、0 处 `internal→cmd` 违规；pkg 注入点 3 个、未登记 0） |
| 文档校验 | `make check-docs` | 通过（ADR 8 个、markdown 112 个、pkg 包契约 12 个、链接全部有效） |
| 进度看板 | `make progress` | 校验通过；任务数量 28/29 完成（不含工作量含义） |
| 生成物幂等 | `make gen` 前后对 `docs/STATS.md`、`docs/DEPGRAPH.md` 取 sha256 | 前后**完全一致**（幂等成立）；`docs/STATS.md` 记录 434 个 `.go` 文件，与 `find` 实测 434 一致 |
| 生成物新鲜度门禁 | `make gen && git diff --exit-code docs/DEPGRAPH.md docs/STATS.md` | **本地必然报差异**——因为改动尚未提交，索引里是 HEAD 的旧版本。此门禁在 CI 的"已提交检出"上才有判定意义。已用上面的幂等检查替代证明："生成结果与当前代码一致且可重复" |

### 已知问题清单

**P0**：无。

**P1**：
- `pkg/lsp` `Conn.Start()` check-then-act 竞态导致两个 `readLoop` 共享 `bufio.Reader`（数据竞争 + 协议解析撕裂）。**本轮已修复**并验证：`-race -count=5` 由失败转为通过。触发条件苛刻（需在同一 `Conn` 上并发 `Start()`），产品路径有 `startMu` 保护，故日常不触发。

**P2（已知、本轮未修，均有明确记录）**：
- （历史 P2，2026-09-15 已修复）TUI 运行期间状态栏的会话状态点保持「空闲」不变，运行反馈只体现在消息区的旋转器与工具卡片上；当前 `StartStreaming`/`StopStreaming` 已同步状态栏忙碌状态，并由 `TestAppStreaming` 覆盖。
- （历史 P2，2026-09-15 已修复）旧版 `basework init` 在保持打开且无数据的 stdin 管道上会阻塞；当前提供 `init --yes`、`--provider`、`--model`、`--preset` 和显式 `--force`，无人值守路径不读取 stdin。交互路径仍保留。
- 真实模型矩阵中出现过**一次**超过 12 分钟无输出的停滞，重跑同一场景同一序号即恢复正常（5.3s），**未能定位根因**（当时该环境 `ps` 不可用）。已确认相关产品性质：上游停滞可被调用方 context 终止（2s 上限实测 2.003s），且 agent 层会把取消转成真实错误。详见 [SHIP-001](SHIP-001.md)。
- linux/windows/darwin-amd64 及 Docker 镜像从未在目标环境运行/构建（见上表）。
- `tests/integration_test.go` 的 `TestIntegration_OpenAI_E2E` 测试体仍被整段注释，是占位测试。
- `tests/benchmark` 只有 `Benchmark*` 函数，`go test` 报 `[no tests to run]`，从未实际执行。
- 跨平台 CI 从未在本轮实际触发。

### 报告中本地 / CI / 真实模型 / 人工体验的区分

| 结论 | 证据来源 | 可信范围 |
|---|---|---|
| 编排逻辑正确（工具派发、落盘、权限拦截、截断、断流暴露） | 本地离线重放（`tests/coding_scenarios_test.go`，6/6，`-count=2` 一致） | 可重复、可复现；不代表真实模型行为 |
| 真实模型能完成编码闭环 | 真实模型 8 次（核心层夹具，`agnes-2.5-flash`）+ 真实终端 TUI 走查 1 次 | 只覆盖该端点/该模型/openai 兼容协议；报的是固定次数结果，不是成功率 |
| TUI 业务场景可完成（终端级） | PTY 自动化验收（`tests/tui_pty/`，5 场景全过，darwin/arm64 本机） | 证明"按键驱动下产品行为正确"；**不能**证明"人类觉得好用"——主观体验仍未经真人评价 |
| 安装与升级行为 | 本地 darwin/arm64 发布产物 | 只有该平台真实运行过；其余平台与 Docker 未验证 |
| 并发安全 | 本地 race 检测器（`-race`） | 只覆盖被执行到的代码路径；`-count=1` 时 `pkg/lsp` 那条竞态可能漏检（`-count=5` 才稳定复现） |
| 跨平台 CI 行为 | **无证据** | CI 只有定义，本轮未触发 |
| 人工 TUI 体验 | **无真人证据**（用户指定以自动化替代，已如实记录） | 主观体验维度缺失是本记录的已知边界 |

## 剩余与交接

- **发布尚未准备就绪的两项外部证据**（不阻塞本卡的检查环节，但阻塞"可以发布"的判断）：候选 commit 的跨平台 CI 结果；Docker/GHCR 镜像的构建与运行结果。两者都需要在 CI/有 `buildx` 的环境执行。
  - **（2026-09-14 更新）第一项已开始执行，并暴露了本卡的盲区**：M2–M8 成果提交并推送（`555d4e8`）后 CI 矩阵首次实际运行（run `34758258996`）。macOS 与文档/静态检查通过，Ubuntu 失败 1 个、Windows 失败 13 个测试；其中 **4 个（Windows）由 2 个真实产品缺陷引起**：① Windows 上超时/取消对后台命令不生效（本卡记录的"进程组仅 unix 生效、Windows 未实测"正是它的温床——退化路径在 Windows 上让 `cmd.Wait()` 被孙进程持有的管道阻塞），② `FactsSummaryProvider.wrapHash` 漏判 Windows 根路径。其余 10 个是测试自身的平台假设错误（只设 `HOME`、断言 Unix 权限位、未关溢出文件句柄、拼接 JSON 未转义、1 个 Ubuntu 竞态）。全部已修复并补回归测试，详见 [CHANGELOG](../../CHANGELOG.md) 与 [STATUS](../../STATUS.md) 的 2026-09-14 条目。**结论：本卡"跨平台 CI 不存在"的边界判断是正确的，但"未实测"这件事的成本比当时估计的更高——首次实测就抓到 2 个产品缺陷。** 第二项（Docker 镜像）仍不存在。
  - 同批还补了一处与本卡无直接关系、但同属"未实测"类的修复：`internal/observability` 的 pprof 测试绑定固定端口（16060–16066），多个 `go test` 进程并行会互踩；`Start()` 端口被占用时曾静默返回 nil。
  - **（2026-09-14 更新）第一项已完成**：修复后重跑 CI（run `34797549128`），macos 1m9s、ubuntu 1m14s、windows 3m47s、Quality 2m58s、Release Dry Run 3m47s **五个 job 全部通过**——本仓库**首次跨平台 CI 全绿**，本卡"候选 commit 的跨平台 CI 结果"这项外部证据由此补齐（尚未打 tag，故不与任何发布版本绑定）。**第二项（Docker/GHCR 镜像）仍不存在**：本机 Docker daemon 未运行，且沙箱内无法启动 colima，构建需要一台能跑 `buildx` 的机器或依赖 CI 的 release 流程。
- **"人工体验"维度的边界**：终端级自动化验收覆盖了"按键驱动下产品行为正确"，但"人类觉得好用"没有真人证据。若后续需要真正的真人签收，入口：`docs/TUI.md`、`basework tui`、`docs/development/tui-pty-acceptance.md`（夹具可直接复跑）。
- **不要做的事**：不要因为本文件写了"候选版本"就去创建 tag 或触发发布。卡片明确"准备完成不等于已经打 tag 或发布"；本轮的候选甚至尚未提交（`HEAD` 仍为 `935fde1`）。提交与打 tag 需要用户明确授权。
- TUI 验收夹具的运行方法、场景-路线图对应关系与已知环境边界（本机 Go SDK 路径、Windows 无 pty）见 `docs/development/tui-pty-acceptance.md`；真实模型走查脚本与运行证据在 `/private/tmp/ship003/runs/`（临时目录，重启即失；结论已收录本文件）。
- 复现方式：本文件所有门禁命令都在仓库根执行；真实模型与安装验证的复现方式分别见 [SHIP-001](SHIP-001.md)、[SHIP-002](SHIP-002.md) 的「剩余与交接」。

## 结论

- **是否满足任务卡全部验收**：**满足**（按用户指示的执行方式）。四条步骤全部完成：①候选版本快照上重跑发布检查通过（race 由失败转通过，修掉 1 个 P1 并发缺陷）；②路线图 5 条代表性场景全部由**终端级自动化 TUI 验收**走完并留证（用户明确指定此方式替代人工；过程中又发现并修复 2 个 P1 缺陷）；③能力与限制已依据证据更新并核对任务状态；④候选版本验收报告即本文件，"实际发布单独操作"遵守。三条验收条件："无未说明的 P0/P1 问题"满足（3 个 P1 全部修复并写明）、"报告区分本地/CI/真实模型/人工体验"满足（真人体验的缺失如实标注）、"准备完成不等于已经打 tag 或发布"遵守（HEAD 未动）。
- **建议状态：完成**。条件与边界：自动化替代人工是用户指定并已在本文件如实记录；两项发布外部证据（跨平台 CI、Docker 镜像）仍不存在，已列入交接——它们阻塞的是"可以发布"，不是本卡的检查环节。
- 若将来补做真人 TUI 走查，据实回填「验证」表即可；不要因为本文件结论是"完成"就跳过跨平台 CI 与镜像构建这两道发布前检查。

## 2026-09-15 历史工作区复审

上面的“完成”结论属于历史工作区快照，不能覆盖当前看板的候选门禁。本轮最初在历史
`HEAD=7a874c9` 加未提交修改的工作区上重新核对：

- 当前 PTY 场景 1–8 已按顺序全量通过；scenario3 的恢复面板、scenario6 的会话隔离、
  scenario7 的审批边界和 scenario8 的双压缩重启均有真实终端与磁盘/请求侧断言；
- 路径保护补丁后，scenario3 与 scenario8 又分别复跑通过；默认/tag 测试、race、
  vet、构建、Windows 目标交叉编译、`make gen`、`make check-docs`、`make check-arch`
  和 `git diff --check` 均通过；
- 该历史工作区的 GoReleaser 快照和 linux/amd64、linux/arm64 OCI 镜像已构建并在本机可
  运行，但仍属于 dirty worktree 证据；没有候选 commit 的 CI 结果，也没有目标平台
  安装或外部 Provider 结果。

因此 SHIP-003 在任务看板继续保持“待验证”。这不是代码回退，而是避免把当前工作区
的强验证结果误写成可发布候选；待 SHIP-001、SHIP-002、QA-001 的外部门禁补齐后，
再把本节结果绑定到具体候选 commit。

本轮还修复了历史 P2 体验缺口：`App.StartStreaming`/`StopStreaming` 现在同步
`StatusBarView.IsBusy`，状态栏会从“● 空闲”切换到忙碌 spinner，再在回复/取消后恢复
空闲；`TestAppStreaming` 已锁定这条接线。该修复不改变当前发布门禁状态。

清洁临时候选 `ac6852dc33d77a01ccb927e5a6cc46dafad804d5` 已重新跑过 scenario1–8，
恢复与审批等真实产品流程在该副本中均通过；候选尚未推送，也没有新的 CI、目标平台安装
或外部 Provider 结果，故本任务仍保留“待验证”。

随后在同一清洁候选上补做了 Docker 门禁：Buildx 实际构建并载入
`linux/amd64`、`linux/arm64` OCI manifest，两种架构均运行 `version` 与只读工作区的
`facts show`；manifest 为 `sha256:97beb43a4bdf317d7be93b97623ffef2cce2dc0d9cba549b155e2b04bd0ef5b0`。
详细哈希、命令和运行边界见 [SHIP-002](SHIP-002.md)；这关闭镜像构建/运行记录项，仍
不替代候选 CI、Windows/macOS 安装和真实 Provider 证据。

同样，历史交接中“benchmark 尚未实际执行”的描述已由 OPT-002 的三次 `-benchmem`
样本覆盖；该历史段保留用于追溯，不再代表当前性能验收状态。

提交 `9d7458bb5c53274b5d5b1742cb88abe75ad4ac64` 的干净 worktree 已通过文档门禁、完整
tag race 和 PTY scenario1–8；这收口了本地候选的产品入口复核。当前仍缺该提交对应的
远端 CI、目标平台安装和真实 Provider 结果，因此 SHIP-003 继续保持待验证。

## 2026-09-15 当前候选 CI 回填

当前候选提交 `1e9c8198f6d29530692ed6d3a0dc3e1bbb1eb3b8` 的 [GitHub Actions run
34945363069](https://github.com/wly2lcl/basework/actions/runs/34945363069) 已完成并全绿：
Ubuntu、macOS、Windows 三平台测试/构建，Quality race、文档与 PTY smoke，Docker Smoke
双架构镜像运行，以及 Release Dry Run 全部通过。该结果同时验证了本轮修复的 Windows
进程树终止路径和 Docker 单平台加载路径。

这次回填关闭了“当前候选没有远端 CI 结果”的旧记录，但不关闭真实 Provider、五平台发布
归档安装/升级和真人主观体验边界。**当前 SHIP-003 仍待验证**，等待 SHIP-001、SHIP-002
和 QA-001 的剩余外部门禁；本段不创建 tag，也不代表已发布。

## 2026-09-15 最新候选回填

提交 `6a2cc3e72aaa02ab87b72067545c42cec92fbd17` 的 [CI run
34946819197](https://github.com/wly2lcl/basework/actions/runs/34946819197) 在加入三平台
`Release Artifact Smoke` 后全绿。当前证据已覆盖跨平台源码测试、Windows 进程树回归、
三平台原生发布归档 `version/config explain`、Docker 双架构 Smoke、Release Dry Run 和
PTY/race/文档门禁。

SHIP-003 仍保持待验证：真实 Provider、Linux arm64/Darwin amd64 归档安装和真人主观体验
仍未完成；本段只更新候选证据，不创建 tag 或执行发布。

## 2026-09-16 五平台候选门禁回填

候选提交 `341679791936b9af3657a711b27fa70413498895` 的 [GitHub Actions run
35041364486](https://github.com/wly2lcl/basework/actions/runs/35041364486) 全绿，完成了
Quality、五平台源码测试、五平台归档安装 Smoke、Docker Smoke 和 Release Dry Run。五个平台
的实际 runner、归档文件、SHA-256 与解包运行结果见 [SHIP-002](SHIP-002.md#2026-09-16-完整五平台归档-smoke-与哈希)。

因此平台发布矩阵已不再是当前缺口。SHIP-003 仍待验证，剩余边界为 SHIP-001/QA-001 要求的
真实 Provider 正向记录，以及是否需要补充真人主观体验；本段不创建 tag、不执行发布。

## 2026-09-16 当前候选发布包门禁回填

候选提交 `3787862cd076b72d49acfc3c1127efcc807f3539` 的 [CI run
35043445341](https://github.com/wly2lcl/basework/actions/runs/35043445341) 全绿。该 run 在
五个平台实际 runner 上解包并运行当前发布归档，完成 `init`、v1→v2 会话迁移、源数据哈希保持
和未来 v99 拒绝 Smoke；同时 Quality、五平台测试、Docker Smoke、Release Dry Run 均通过。
平台、归档哈希与命令边界见 [SHIP-002](SHIP-002.md#2026-09-16-当前候选发布包迁移-smoke)。

平台与发布包升级证据已收口。SHIP-003 仍待验证的边界只剩 SHIP-001/QA-001 要求的真实
Provider 正向记录，以及是否补充真人主观体验；本段不创建 tag、不执行发布。

## 2026-09-16 当前候选验收收口

前置任务 SHIP-001、SHIP-002、QA-001 已分别完成并有当前候选证据：

- [SHIP-001](SHIP-001.md#2026-09-16-当前候选真实-provider-正向验收)：真实 Provider
  `openai`/`agnes-2.5-flash` 回合成功，独立测试退出码为 0，脱敏结果已入库；
- [SHIP-002](SHIP-002.md#2026-09-16-当前候选发布与前置门禁收口)：当前候选五平台归档、
  v1→SQLite 迁移、未来版本拒绝、Docker Smoke 与 Release Dry Run 全部通过；
- [QA-001](QA-001.md#2026-09-16-当前候选真实-provider-正向与验收收口)：CI job、commit、
  真实模型与脚本化模型证据均可追溯。

当前没有未说明的 P0/P1 发布阻断项；自动化 PTY 已按用户选择覆盖代表性场景，真人主观 TUI
手感继续单独标为尚未评价的体验边界。本文不创建 tag、不执行发布授权，仅完成候选验收与
文档收口。

**当前结论：SHIP-003 完成。**

## 2026-09-16 再复审交接

前置任务回退；本任务既有实现和局部证据保留，等待依赖修复后回验，不要求重写模块。 本次状态调整为待验证，小步骤见原任务卡新增补充。历史成功测试不删除，但不能代替本次缺陷修复后的验证。

## 2026-09-16 依赖回验状态

候选收口等待 RUN-003/UI 依赖回验、可信 QA-001 完成和 SHIP-001 当前候选协议矩阵；旧候选通过记录保留，不代表当前工作树已可发布。

## 2026-09-16 修复候选依赖回验

修复候选 `3ad67a7d30ab5b030bd28fe09504b6134c5a6faf` 已完成 RUN-003、CTX-003、UI-001/002/003
依赖回验；PTY scenario1–8、默认/完整 tags 全量测试、相关 race 与 [CI run
35055131884](https://github.com/wly2lcl/basework/actions/runs/35055131884) 全部通过。CI
首次 macOS 并发测试超时后重跑成功，五平台归档、Docker Smoke 和 Release Dry Run 均有当前
候选记录。当前仍不能关闭 SHIP-003：真实 Provider 的协议/次数矩阵尚未完成。

## 2026-09-16 当前远端候选 CI 关联

提交 `4c40e18e2662166902500bf1ba18124efdfc5a9f` 的 [CI run
35077896624](https://github.com/wly2lcl/basework/actions/runs/35077896624) 已全绿，Quality、五平台测试、五平台归档 Smoke、Docker Smoke 与 Release Dry Run 均通过。
该记录关闭当前候选的远端构建与发布辅助门禁，但 SHIP-001/QA-001 的当前候选真实 Provider
协议矩阵仍未完成，因此不改变本任务的待验证状态。

## 2026-09-17 当前远端文档候选 CI 关联

提交 `140eddf4cfd0ba457d7b82623c5e83d1b3c46574` 的 [CI run
35079524005](https://github.com/wly2lcl/basework/actions/runs/35079524005) 已全绿，完成当前候选的 Quality、五平台测试、五平台发布归档 Smoke、Docker Smoke 与 Release Dry Run。该记录不改变 SHIP-003 的待验证状态：前置 SHIP-001/QA-001 仍缺当前候选的真实 Provider 协议/次数矩阵。
