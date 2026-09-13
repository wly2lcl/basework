# BASE-001 验证记录

## 基线

- 日期：2026-09-12；操作者：舟（WorkBuddy AI 会话）。
- 系统：macOS 26.6.2（Build 25G83）arm64。
- 工具链：go1.26.0 darwin/arm64；`go.mod` 声明 `go 1.26`。
- commit：`935fde1c23d48dc7c322c6917eb3a75b1e7df1aa`（*fix: make context replay deterministic*，2026-09-12 14:58:56 +0800）。
- 工作区差异范围：`git status --porcelain` 为空，无未提交改动。本地领先 `origin/main` 5 个提交，**未 push**。
- 任务与用户场景：为后续 27 个任务建立可复跑的当前基线，区分「已通过」「失败」与「未验证」，避免把历史失败或未验证内容当作新功能完成。
- 复跑方式：`export PATH=/usr/local/go/bin:$PATH` 后于仓库根执行；Go 构建缓存使用用户默认 `~/Library/Caches/go-build`（本轮无需改写 `GOCACHE`）。

## 变更

- 无产品代码变更。本轮只做基线测量与证据回填，未触碰 `pkg/`、`internal/`、`cmd/`、`tests/`。
- 修改文件：
  - 新增 `docs/development/evidence/BASE-001.md`（本文件）。
  - `docs/TASKS.md` 中 BASE-001 行的状态、负责人与证据链接。
- 兼容影响：无。公共 API、存储格式、默认权限均未变化。
- 生成物：本轮运行过 `make gen`，`docs/DEPGRAPH.md` 与 `docs/STATS.md` 的 SHA-256 在生成前后一致，因此仓库内无生成物改动。
- 仓库外临时产物（未入库，仓库命令不依赖）：`/private/tmp/basework-base001/` 存放 `basework` 二进制与三份测试日志。

## 验证

通过 PATH 显式指向 `/usr/local/go/bin` 后执行，全部在仓库根运行。

| 检查 / 场景 | 命令或操作 | 实际结果 | 证据与边界 |
|---|---|---|---|
| 默认模式全包编译 | `go build ./...` | exit 0 | 无 tag，即不含 `sqlite` / `memory` |
| 默认模式静态检查 | `go vet ./...` | exit 0 | 无输出 |
| 格式检查 | `gofmt -l`（枚举仓库 `.go`，排除 `.git`） | 无输出，exit 0 | 与 CI 的 `test -z "$(gofmt -l ...)"` 等价 |
| 完整模式全包编译 | `go build -tags "sqlite memory" ./...` | exit 0 | — |
| 完整模式 CLI 构建 | `go build -tags "sqlite memory" -o /private/tmp/basework-base001/basework ./cmd/basework` | exit 0；产物 25,083,026 字节（约 24 MB） | 仅本机 darwin/arm64 |
| 完整模式静态检查 | `go vet -tags "sqlite memory" ./...` | exit 0 | CI 的 vet 步骤即此命令 |
| 默认模式全量测试 | `go test ./... -count=1` | exit 0；33 个包中 31 个 `ok`，2 个 `[no test files]`（`scripts/docstats`、`scripts/gendeps`）；无 `FAIL`、无 `SKIP` | 测试数不等于覆盖率 |
| 完整模式全量测试 | `go test -tags "sqlite memory" ./... -count=1` | exit 0；31 个 `ok`，覆盖包集合与默认模式一致 | `tests/benchmark` 输出 `ok ... [no tests to run]` |
| 架构边界 | `make check-arch` | exit 0；pkg 下 193 个 `.go` 发现 0 处 pkg→上层违规；internal 下 122 个 `.go` 发现 0 处 internal→cmd 违规；3 个注入点、白名单 3 条、未登记 0 | 只证明分层与注入登记，不证明业务正确 |
| 文档校验 | `make check-docs` | exit 0；ADR 5 个、markdown 81 个、pkg 包契约 12 个、链接全部有效 | 不校验外部网页 |
| 任务进度 | `make progress` | 1/28 完成；`可领取：BASE-001` | 与看板状态一致，确认 BASE-001 为依赖已满足的首个待办 |
| 生成物新鲜度 | `make gen` 后 `git diff --exit-code docs/DEPGRAPH.md docs/STATS.md` | exit 0；两文件 SHA-256 前后不变 | 复现 CI 的新鲜度门禁 |
| 生成物确定性 | `make gen` 后比对 SHA-256 | `DEPGRAPH.md` = `a28d6162…1421ae`，`STATS.md` = `bd22aec6…d0ab02`，与生成前一致 | 幂等 |
| 并发竞争 | `go test -tags "sqlite memory" -race -count=1 ./pkg/... ./internal/permission ./internal/tui ./cmd/basework` | exit 0；16 个包 `ok`；无 `DATA RACE`、无 `FAIL`；约 44s | 范围与 CI 的 race 步骤一致；未含 `tests/` 与 `internal/compaction` |

失败归类：**代码类 0 项、环境类 0 项、外部服务类 0 项**。因此本轮没有把任何已有失败计入新功能完成，也无需保留失败原始摘要。

## 剩余与交接

未验证内容（本机条件不足或需人工，均不构成本轮阻塞）：

- 真实 Provider：`OPENAI_API_KEY`、`ANTHROPIC_API_KEY`、`GOOGLE_API_KEY`、`DEEPSEEK_API_KEY`、`OPENCODE_API_KEY`、`BASEWORK_PROVIDER` 当前**均未设置**，本轮没有任何真实模型调用。此项由 REL-002 / REL-003 跟踪。
- 跨平台：本机仅 macOS 26.6.2 arm64。Ubuntu / Windows 只有 CI 矩阵定义（`.github/workflows/build.yml`），本轮**未触发**，不得据此宣称已验证。
- 人工 TUI：未做交互式体验验证，需人工执行。
- 覆盖率：本轮未采集；`docs/STATS.md` 刻意不含覆盖率（含并发包的 `-cover` 数字不可复现，见 `Makefile` 注释与 `make coverage`）。

顺带发现两处「验证空洞」，本轮**未修改**，属后续任务范围：

1. `tests/integration_test.go` 的 `TestIntegration_OpenAI_E2E`（约 573 行起）：无 `OPENAI_API_KEY` 时 `t.Skip`，有 key 时只执行 `t.Log` 后成功返回——测试体被整段注释，**任何时候都不会发起真实请求**，却计入 `tests` 包通过。真实模型编码闭环属 REL-003。
2. `tests/benchmark` 包含 5 个基准测试文件但只有 `Benchmark*` 函数，`go test` 报 `[no tests to run]`；CI 未传 `-bench`，基准测试从未实际执行。

下一步：REL-001（「流式工具调用协议回归」），依赖 BASE-001；BASE-001 完成后其依赖已满足。交接不需要解除条件。

不要重复执行：本轮未产生代码改动，重跑上述命令应得到相同结论；但需注意 `make gen` 会重写 `docs/STATS.md`，若后续代码变更则哈希预期会改变。

## 结论

- 是否满足任务卡全部验收：**满足**。两个构建模式（默认、`sqlite memory`）结果明确且均通过；没有把现有失败算作新功能完成；证据已写明真实 Provider、跨平台、人工 TUI 与覆盖率尚未验证的范围。
- 额外满足：架构、文档、生成物新鲜度、生成物确定性与 race 并发检查均通过，可作为后续任务的对照基准。
- 建议状态及原因：**完成**。本项交付物是基线与证据本身，两项验收条件与针对性验证命令（`go test ./... -count=1`、`go test -tags "sqlite memory" ./... -count=1`、`make check-arch`）均已实际执行并记录退出码。
