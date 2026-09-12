# basework vs deepseek-harness 对比分析

> 性质：**竞品/架构研究笔记**，不属于维护文档体系（未登记进 README 索引）。
> 数据采集日期：2026-09-12。dsh 数据来自 GitHub API + 仓库内 `docs/architecture.md`、`packages/README.md`；
> basework 数据来自 `make stats` 输出与本仓库代码。

---

## 一、结论先行

两者**不是同一量级的对手**，但**定位差异是真实的**：

| | basework | deepseek-harness (dsh) |
|---|---|---|
| 本质 | 一个人的 Go agent 框架 + 终端产品 | DeepSeek 官方「Agent 运行环境」 |
| 一句话定位 | **可嵌入的单机库** | **可重组的多端平台** |

关键判断：**这两个定位在 Go 生态里几乎不重叠 —— basework 的不对称优势在这里，而不是在功能清单长度上。**

不要按功能清单和 dsh 打。要打它结构上够不到的位置。

---

## 二、硬数字

| 维度 | basework | deepseek-harness |
|---|---|---|
| 语言 | Go 1.26 | TypeScript |
| 源码规模 | 57,816 行 / 347 个 `.go` | 36.6 MB TS / 3,717 个 `.ts(x)` |
| 非测试代码 | 22,810 行 | 未单独统计（含 1,278 个测试文件） |
| 模块拆分 | 3 层（`pkg/` `internal/` `cmd/`） | 55 个包组 / 298 个 npm 包 |
| 直接依赖 | 12 个 | `pnpm-lock.yaml` 865 KB |
| 测试 | 1,645 用例 / 30 包全绿 | 1,278 测试文件 / 12 套 vitest 配置 |
| 文档 | 35 篇 md（中文） | 127 篇英文 + 中文镜像 + i18n 清单 |
| 决策记录 | 无（`openspec/` 已删） | 2,945 篇 `.agents/notes`（932 implemented / 1,892 archived） |
| CI 校验脚本 | 1 个架构守护测试 + `docstats` | 204 个校验脚本 + 模块图新鲜度门禁 |
| 交互形态 | TUI + CLI | Web UI + Electron 桌面（TUI 仅为示例 profile） |
| 嵌入方式 | `go get` 进程内 | SDK，JSON-RPC 跨进程 |
| LLM Provider | 8 个实现 + OpenAI 兼容工厂 | DeepSeek 官方 + pi-ai catalog |
| 内部能力族 | 9 个 `internal/` 模块 | 50+ 能力族 |
| 活跃度 | 46 commits，最后 2026-07-08 | 每日推送，最后 2026-09-11 |
| Star / Fork | 未发布分发 | 220,579 / 26,129 |
| License | MIT | MIT |
| 版本状态 | v0.1.2 | developer preview（官方承诺破坏性变更） |

---

## 三、basework 的真实优势

### 1. 进程内可嵌入 —— dsh 结构性覆盖不到的位置

dsh 的 SDK 是 **out-of-process**：`dsh --profile sdk` 起一个 JSON-RPC over stdio 的服务端，客户端跨进程通信。
因为它整个产品是 Node 应用，**你没法把它的 agent 循环 link 进自己的程序**。

basework 的 `pkg/` 是真正的库：

```go
a, _ := agent.New(agent.WithModel(m), agent.WithTools(tools))
```

**这是唯一的、也是最硬的不对称优势**，且不随 dsh 迭代而消失 —— 它是语言与运行时决定的。

### 2. 单二进制、零运行时依赖

Go 静态编译 → 一个可执行文件。dsh 需要 Node 22+ / pnpm / 298 个 npm 包 / 865 KB lockfile / 18.6 KB 第三方声明。

在企业内网、离线 CI 容器、受限合规环境里，**依赖审计面差一个数量级**。

### 3. 终端优先

dsh 默认 Web UI（浏览器 + Electron 桌面）。TUI 在它文档里只是

```
dsh --profile tui --resume <id>     # example, assuming the tui profile is installed
```

—— 一个需要自行安装的示例 profile。

SSH / 低带宽 / 无 GUI 场景下，basework 原生就是终端形态。**这恰恰是 dsh 的缺口。**

### 4. 可通读的规模

22,810 行非测试代码，一个人能全部掌握。

对照：dsh 官方 `docs/architecture.md` 原文写着

> We recommend using an agent to explore the codebase and understand its architecture.

**需要另一个 agent 才能读懂你的 agent 框架** —— 这是复杂度的自证。
对需要审计、二次开发、深度定制的团队，**小是特性，不是缺陷**。

### 5. 并发安全有硬工具背书

Go 的 race detector 在 agent 循环这种高并发场景比 TS 更硬。dsh 有 39 KB 的 `tsconfig` 严格配置，但类型系统不检测数据竞争。

basework 上一轮修的 TUI 回调竞态（`tea.Cmd` goroutine 里改 Model）+ `-race` 全量跑通，就是这个优势的体现。

---

## 四、basework 的真实劣势

### A. 架构代差（最根本）

**1. 扩展模型落后一代**

| | dsh | basework |
|---|---|---|
| 机制 | Cordis 微内核：插件贡献 service / typed event / **reversible effect**，卸载即回滚，支持运行时替换且状态不崩 | 编译期接口 + build tag + 静态 hook |
| 加一个工具 | `dsh plugin --profile web add <pkg>`，无需改框架 | 改 `pkg/tool/builtin` → 重新编译 |
| 换 provider | 装/换适配器插件，核心不动 | 重新编译 |

**这不是代码量差距，是范式差距。**

**2. 会话/事件模型不够硬**

dsh 把 append-only 事件日志当**唯一真相**，且有运行时不变量强制：

> **Model-visible means logged.** Anything that reaches a model request must be reconstructable from the log, and a runtime invariant asserts it.

配套：格式版本化（`v0 → vN`）+ 相邻迁移链 + zstd 压缩 + fork/resume + 专门的 cookbook 教「如何增加会话格式版本」。

basework：JSONL + SQLite 并存，路径刚修完三处不一致。

**3. 能力族覆盖窄一个数量级**

dsh 有而 basework 无（或只有雏形）的：

- 沙箱：bwrap / Landlock / Seatbelt 三后端
- E2B 远程运行时装
- 持久 PTY terminal
- 后台 job 运行时 + 模型侧 job 控制工具
- workflow 引擎（含 ralph 循环）
- Agent Teams（持久 roster + 任务板 + 邮箱）
- webhook 触发会话
- spill（工具输出落盘防爆上下文）
- canonical tool output 契约
- guard（重复调用提醒 + `tools/execute` deadline 强制）
- 语义检索 / SQLite FTS 会话检索
- 上下文注入族（workspace 指令、时间上下文、引用）
- goal / schedule / plan / feedback / attachment / credentials / settings 各族

### B. 工程基建

**4. 没有把文档与边界变成契约**

dsh 的 204 个校验脚本进 CI，其中最有代表性的三条：

- 模块依赖图 `docs/module-graph.md` 由 `pnpm run gen-module-graph` 生成，**CI 做新鲜度门禁**
- 每个包 README 必须含 purpose / configuration / extension points / Model Experience 四节，有 omission allowlist + 校验脚本
- 每个包 README 必须含 `## Known Limitations and Deferred Work`，同样有 allowlist + 校验脚本

basework 前一阶段的实际状况：手写数字漂移约 45%、两份互斥 ROADMAP、DESIGN 文档 §6/§15 内容增生。

**同一个病（文档漂移），dsh 用机器治，basework 用人治。**

**5. 决策记录制度缺失**

dsh 有 2,945 篇带日期的 `.agents/notes`，按 `architecture / feature / bug-fix / process / simplification` 分类，每篇是一个 ADR：记录「为什么这么定」以及被废弃时的归档原因。

basework 删掉了 `openspec/` —— 但**这个需求是真实的**，只是不该背那套重流程。

### C. 生态与可持续性

**6. 零外部验证**

未发布分发、无第三方插件、无真实场景压力测试。所有能力都是「自己声称可用」。
对照 dsh：22 万 star / 2.6 万 fork / `dsh-plugin` topic 生态起步。

**7. 单点维护 + 已停摆 65 天**

46 次提交，最后提交 2026-07-08。
dsh 有 issue 模板 / `review-ownership` 审批策略 / dependabot / lefthook / GitLab CI + GitHub Actions 双流水线。

**8. 分发与信任为零**

无 Homebrew tap（ROADMAP 自述「后续接入 tap 后再开放文档入口」）、无签名、无发布产物验证。
dsh 是 `npx @deepseek-ai/dsh web` 一条命令，npm 官方 scope。

---

## 五、dsh 自身的问题与风险

（不盲目对标 —— 它也有明显代价）

1. **Developer preview，官方明写 `THERE WILL BE COMPATIBILITY-BREAKING CHANGES`**
   → 现在不适合做生产依赖。想要稳定性，今年内它不是答案。

2. **微内核的复杂度税很重**
   要理解 Cordis context / service / typed event / reversible effect / capability seam 三角色 / profile / bundle / patch 四层叠加。
   想改一处行为，先得搞清它挂在哪一层、被哪个 patch 覆盖 —— 得靠 `dsh --profile web --dump-config` 才能看清自己机器启动的树。

3. **Provider 广度靠 pi-ai 单个 catalog 适配器**
   不是 15 个独立适配器，而是 catalog 驱动。非主流 provider 的深度特性（特殊参数、缓存语义）可能被抹平。

4. **无默认 TUI**
   终端用户要自己装 `tui` profile，且文档中标注为示例性质。

5. **规模本身是负担**
   298 个包 / 39 KB `tsconfig.base.json` / 865 KB lockfile / 183 MB 仓库 → 供应链审计面巨大。

6. **22 万 star 的仓库 open issues 为 0**
   反馈走 Discussions 而非可追踪 backlog，社区噪音与真实反馈混在一起，外部贡献者难以判断优先级。

7. **实验性的运行时自改插件**
   `packages/extensions/` 允许模型在运行中 mount / unmount 插件（reversible effect 保证状态回滚）。
   能力惊人，但**安全边界完全依赖 sandbox 兜底** —— 企业环境需非常谨慎。

---

## 六、可以立刻借鉴的做法（按性价比排序）

### 1. 把 capability seam 三角色显式化 —— 成本最低、收益最大

dsh 的形式化定义：

> A **seam** is a swappable capability with three roles: a **Service Definition** declaring the interface, a **Service Provider** implementing it, and a **Consumer** using it. A package may combine roles, but one role alone is not a seam; adding a capability means designing all three.

basework 上一轮在 `pkg/tool/builtin` 自定义 `PathChecker` / `EventPublisher` **就是它的雏形**。
上升为命名规范 + 文档约定 + 守护测试即可，不需要重构。

### 2. 引入 "model-visible means logged" 不变量

凡能进模型请求的东西，必须能从会话日志重建，并加运行时断言。
直接收益：bug 可复现、上下文可审计。**这是 dsh 最值钱的一条设计。**

### 3. 把校验脚本当产品做

已有 `docstats` + 架构守护测试。继续补两个高价值的：

- 模块依赖图生成 + 新鲜度校验（对应 dsh `gen-module-graph`）
- 每个 `pkg/` 包必须含「用途 / 配置 / 扩展点」三节的 README 契约检查（对应 dsh 的 README contract）

用机器替代人工修文档。

### 4. 轻量 ADR 替代 openspec

单文件目录 `docs/adr/NNNN-title.md`，每条只写「背景 / 决定 / 后果」，不做流程、不做阶段门禁。
补上被删掉的「变更留痕」，但不背流程包袱。

### 5. 守住 TUI + 进程内嵌入这条线，不要追 Web / Desktop / SDK 多端

dsh 的 Web / Electron / Python SDK 是它的战场，也是它必须背的复杂度。
basework 的战场是「**Go 程序里想内嵌一个 agent，并且想用终端**」—— 这个位置 dsh 现在是空的。

---

## 七、最终判断

**不要按功能清单和 dsh 打。** 你会在每一个维度上输，而且 dsh 每周都在变得更宽。

**要打的是它结构上够不到的地方**：

| basework 的护城河 | 为什么 dsh 够不到 |
|---|---|
| Go 生态进程内嵌入 | dsh 是 Node 应用 + 跨进程 SDK |
| 单二进制零依赖分发 | dsh 是 298 包 + Node 运行时 |
| 终端原生 | dsh 的 TUI 是可选示例插件 |
| 可通读可审计的体量 | dsh 需要 agent 才能读懂自己 |

**同时把 dsh 的工程纪律抄过来**（契约强制、决策留痕、日志唯一真相）。

理由：basework 最大的风险**不是功能落后**，而是它暴露出的可持续性问题 ——
**停摆 65 天 + 文档数字漂移 45% + 零外部验证**。

> dsh 给 basework 的最大价值，不是「要追的功能表」，
> 而是「一个会自我约束的项目是怎么运作的」。

---

## 附：数据来源

| 项 | 来源 |
|---|---|
| dsh 仓库元数据 | GitHub API `repos/deepseek-ai/deepseek-harness`（2026-09-12 采集） |
| dsh 架构描述 | `docs/architecture.md`、`packages/README.md`、`apps/cli/README.md`、`packages/llm/README.md` |
| dsh 规模统计 | `git/trees/master?recursive=1` 路径树统计 + `/languages` 接口 |
| basework 规模 | `make stats` → `docs/STATS.md`（2026-09-12 09:19 生成） |
| basework 模块/依赖 | `go.mod`、`ls pkg/provider`、`ls internal/`、`ls pkg/tool/builtin` |
