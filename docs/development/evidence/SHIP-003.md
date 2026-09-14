# SHIP-003 验证记录：候选版本验收与文档收口（完成 —— 人工走查由终端级自动化 TUI 验收替代，用户指定）

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
| 声明平台的安装/升级结果 | **部分** | 只有 darwin/arm64 产物真实运行过；linux/amd64、linux/arm64、darwin/amd64、windows/amd64 仅交叉编译；Docker/GHCR 只有配置层证据（本机缺 `buildx`，未构建镜像）。详见 [SHIP-002](SHIP-002.md) |
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
- TUI 运行期间状态栏的会话状态点保持「空闲」不变，运行反馈只体现在消息区的旋转器与工具卡片上（真实模型走查与自动化场景的抓屏均如此）。功能不受影响，但"一眼看清是否在跑"的状态反馈不一致，属 UX 打磨，未在本轮修改。
- `basework init` 无 `--yes`/非交互开关：stdin 为"保持打开且无数据"的管道时会阻塞等待菜单输入（`/dev/null` 时正常）。修它需要先定义非交互契约，属 UI-002/UI-003 范畴。
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
