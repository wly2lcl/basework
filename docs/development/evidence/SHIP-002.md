# SHIP-002 验证记录：跨平台安装与升级验证（历史完成记录；当前待验证）

> 当前状态以 [任务看板](../../TASKS.md) 为准：**待验证**。早期“完成”只描述历史快照；当前候选、镜像和平台缺口见文末日期段。

> **2026-09-14 复审说明**：以下为历史实施记录，不能继续单独支撑当前验收。新发现或依赖回退涉及 A14；详见 [本轮复审报告](REVIEW-2026-09-14.md) 与 [任务卡](../../tasks/08-release.md#ship-002) 的复审补充。实际状态只维护在 [TASKS](../../TASKS.md)。旧结论保留用于追溯，本轮未修业务代码。

## 基线

- 日期、操作者、系统、Go 版本：2026-09-13，舟（WorkBuddy AI 会话），macOS 25.6.0（Darwin arm64），go1.26.0 darwin/arm64。
- commit / 工作区差异范围：起点 HEAD `935fde1`；本轮改动均在工作区，未提交（遵守"未经授权不 commit"）。涉及的仓库文件：`pkg/session/{store,memory,jsonl,sqlite}.go`、`cmd/basework/migrate.go`、新增 3 个测试文件、`.gitignore`。
- 任务与用户场景：任务卡口径 `docs/tasks/08-release.md#ship-002`。前置任务 SHIP-001、UI-003、CFG-002 均已完成。
- **本机能覆盖到什么**（先说边界，避免把"交叉编译成功"读成"目标平台已验证"）：
  - 本机是 darwin/arm64。**只有 darwin/arm64 产物可以真正执行**。
  - linux/amd64、linux/arm64、darwin/amd64、windows/amd64 只能产出交叉编译包并核对哈希，**不能在本机冒充目标系统运行**。
  - Docker 镜像渠道本机**无法验证**：Docker CLI 29.3.1 存在，但缺 `buildx` 插件（`docker buildx version` → `unknown command: docker buildx`），而 `.goreleaser.yml` 的 `dockers_v2` 需要 buildx 构建多平台镜像。
- 隔离原则：全部验证在 `/private/tmp/ship002` 下的独立目录与独立 `HOME` 中完成，**不触碰真实用户数据**，不使用真实 `~/.config/basework`。

## 变更

**本轮发现并修复了 3 个真实迁移缺陷**（不是文档问题，是会让用户丢数据或误判数据不存在的问题），按任务卡"发现已有实现先验证缺口"的要求一并修复并补回归测试。

1. **`migrate sessions` 对短会话 ID 直接 panic，且丢数据。**
   `cmd/basework/migrate.go` 迁移时用 `sessionID[:8]` / `sessionID[:12]` 拼新 ID 并新建会话，于是：
   (a) ID 短于 12 字符时 `slice bounds out of range [:12] with length 10` 直接崩溃；
   (b) 新建的会话用的是**新随机 ID**，而事件仍带**原 session_id**，外键全部失配，事件被静默跳过——迁移"成功"但数据没进来。
   修复：`pkg/session/store.go` 的 `CreateOpts` 新增可选字段 `ID`，`migrate` 传原会话 ID（`session.CreateOpts{ID: sessionID, ...}`），新增 `resolveCreateID` 统一校验 ID 合法性（只允许 `[a-zA-Z0-9_-]`，拒绝路径穿越）并在重复时返回错误。

2. **`session list` 对"未来版本"文件谎报"没有会话"。**
   `pkg/session/jsonl.go` 的 `List` 把 `loadSession` 的所有错误一并吞掉，其中包含 `ErrSchemaTooNew`。于是一个 v99 会话文件在 `session list` 里表现为**不存在**——用户会以为数据没了。
   修复：`List` 向上传播 `ErrSchemaTooNew`；真正的损坏文件仍按原语义跳过（`TestJSONLList_StillSkipsCorruptFile` 守住这条）。

3. **`migrate sessions` 绕过版本检查，把未来版本数据"成功"导入。**
   `migrate` 原先自行逐行解析 JSONL，**绕过了 store 的版本检查与迁移链**，于是 v99 数据也能"迁移成功"。这正好抵消了第 2 条的修复意义（读路径拒绝了，写路径还在放行）。
   修复：改为经 `srcStore.Events(session.EventFilter{SessionID: sessionID})` 走**真实读路径**（含版本检查与迁移链）；跳过头部 schema 事件；`failed > 0` 时返回错误（非零退出码），使脚本能感知失败。

- **兼容性影响**：`CreateOpts.ID` 是**加法式可选字段**，不传时行为与之前完全一致（仍生成随机 ID）。存储格式未变（`SchemaVersion` 仍为 2），无破坏性变更。新增的错误路径只影响"未来版本"与"非法/重复 ID"这两类此前本就错误的输入。
- 新增回归测试：`pkg/session/create_id_test.go`（memory + jsonl）、`pkg/session/create_id_sqlite_test.go`（`//go:build sqlite`）、`cmd/basework/migrate_sessions_test.go`（`//go:build sqlite`，覆盖短 ID 与会拒绝未来版本两条）。
- `.gitignore` 增加 `dist/`：GoReleaser 本地 dry-run 会生成该目录，此前未被忽略，容易误提交构建产物。

## 验证

| 检查 / 场景 | 命令或操作 | 实际结果 | 证据与边界 |
|---|---|---|---|
| 配置校验 | `goreleaser check` | `1 configuration file(s) validated`，退出码 0 | 本机 goreleaser v2（`/private/tmp/ship002/bin/goreleaser`） |
| 打包 dry-run（与 CI 同款） | `goreleaser release --snapshot --clean --skip=docker,publish` | 退出码 0，产出 5 个包 + checksums | 与 `.github/workflows/build.yml` 的 `release-dry-run` job 命令逐字一致 |
| 产物哈希与缺失平台 | `shasum -a 256 dist/*` | 见下表 | `windows/arm64` 按 `.goreleaser.yml` 的 `ignore` 规则**有意缺失**，非构建失败 |
| 产物真实运行（本机平台） | 解包 `basework_Darwin_arm64.tar.gz` 后 `./basework version` | `Mach-O 64-bit executable arm64`；`basework version 0.1.4-SNAPSHOT-935fde1`，`platform: darwin/arm64`，退出码 0 | **只有 darwin/arm64 真正执行过**；其余平台仅交叉编译 |
| 初次配置（隔离 HOME） | `HOME=<tmp> ./basework init < /dev/null` | 退出码 0，生成 `~/.config/basework/config.json`（2230 字节） | 见下方"已知限制"一条：`init` 是交互式菜单 |
| 配置可读 | `HOME=<tmp> ./basework config explain` | 退出码 0，`配置来源: ...（已加载）`，生效 provider=opencode / model=big-pickle | 只显示 key 是否设置，不回显 key 值 |
| 旧版本(v1)升级 | 放入 `upgrade-v1.jsonl`（schema v1）→ `session list` → `migrate sessions` | `list` 退出码 0 且列出 `upgrade-v1`；`migrate` 退出码 0，`✅ 导入 upgrade-v1（1 个事件）` | 见下表 SQLite 核对 |
| 升级不破坏原数据 | 升级前后对原 JSONL 做 sha256 | 均为 `ee788e03…3af1`，**逐字节未变** | 迁移是"读取 + 另写 SQLite"，不改源文件 |
| 迁移结果真的落库 | 用 `sqlite3` 模块直接查 `sessions.db` | `sessions` 表 `('upgrade-v1','迁移会话 upgrade-v1')`；`events` 表 `('evt-v1-1','upgrade-v1','prompted',1)`；原正文 `升级前写入的内容：请保留我` **存在** | 直接读库，不采信 CLI 自述 |
| 未知高版本(v99)拒绝（读路径） | `session list`（v99 文件） | 退出码 **1**，报错 `会话格式版本高于当前支持版本（文件 future-v99.jsonl 为 v99，当前支持 v2）` | 修复前这里退出码 0 且谎报"没有找到会话" |
| 未知高版本(v99)拒绝（写路径） | `migrate sessions`（v99 文件） | 退出码 **1**，`会话迁移未全部成功：1 个失败` | 修复前这里"成功"导入 |
| v99 无部分导入 | 查迁移后 `sessions.db` | `sessions` 行数 = 0，`events` 行数 = 0 | 拒绝是彻底的，不留半截数据 |
| 新增/回归测试（存储层） | `go test -tags "sqlite memory" ./pkg/session -run 'TestCreateOptsID\|TestJSONLList' -count=1 -v` | 5 个测试全 PASS（含 memory/jsonl 双子用例） | `TestCreateOptsID_RejectsInvalid` 覆盖路径穿越 ID |
| 新增/回归测试（迁移） | `go test -tags "sqlite memory" ./cmd/basework -run TestRunMigrateSessions -count=1 -v` | `TestRunMigrateSessions_PreservesIDAndData`、`_RejectsFutureVersion` 全 PASS | 短 ID（10 字符 / 2 字符）用例正是修复前会 panic 的输入 |

产物哈希（`dist/`，快照版本 `0.1.4-SNAPSHOT-935fde1`）：

| 产物 | sha256 | 本机是否运行过 |
|---|---|---|
| `basework_Darwin_arm64.tar.gz` | `57e48f7b4ae773d547c161ed39518f906df14eacc3708521e2af6a83d1109d47` | **是**（解包后二进制 sha256 `f563103a2b1105743cfed8e8c551b4eba3e8b405a938d895ed2b293179f26219`） |
| `basework_Darwin_x86_64.tar.gz` | `2df708718a4e7b91d54c83837f11e7bf2ac702f88be4bb3389762a803c02c6ed` | 否（仅交叉编译） |
| `basework_Linux_arm64.tar.gz` | `fe9a1a8182030bd11e04aa103ff776d2f426fa3b8650c3bfb7d8ef083139c19d` | 否（仅交叉编译） |
| `basework_Linux_x86_64.tar.gz` | `1c10c05430d5f37dceaff22d981ba2a5b6f0edd6f27d60bd974625f64e82aae5` | 否（仅交叉编译） |
| `basework_Windows_x86_64.zip` | `47697d513eca6babee4cc63e706b5c71e41c6856bacc365377e500fcb2177fbb` | 否（仅交叉编译） |
| `windows/arm64` | — | **有意不构建**，`.goreleaser.yml` 的 `ignore` 显式排除 |

安装说明与渠道一致性（验收条件"安装说明只包含已接入渠道"）：

| 文档声明 | 配置实际 | 是否一致 |
|---|---|---|
| `docs/installation.md`：GitHub Releases 二进制（Linux amd64/arm64、macOS amd64/arm64、Windows amd64） | `.goreleaser.yml` builds = linux/darwin/windows × amd64/arm64，`ignore` 掉 windows/arm64 | 一致 |
| `docs/installation.md` / `docs/release.md`：Go install | `go install -tags "sqlite memory"`，与 goreleaser 的 `tags` 一致 | 一致 |
| `docs/release.md`：GHCR Docker 镜像 | `dockers_v2` → `ghcr.io/<owner>/basework`，tag `{{.Tag}}` + `latest` | 一致 |
| 两文档均写明"Homebrew tap 尚未接入" | `.goreleaser.yml` 无 `brews` / `nfpms` / `scoops` / `winget` 段 | 一致，未把未接入渠道写成已支持 |

## 剩余与交接

- **未在目标系统运行**：linux/amd64、linux/arm64、darwin/amd64、windows/amd64 只做了交叉编译与哈希核对，**本机没有也不能在目标 OS 上运行**。这些平台的真实运行证据只能来自 CI 矩阵（`.github/workflows/build.yml` 的 `test` job 在 ubuntu-latest / macos-latest / windows-latest 上跑 `go test -tags "sqlite memory"` 与 `go build`）。**本轮没有实际触发 CI**，所以"CI 通过"目前是定义而非观测结果。
- **Docker 镜像未构建**：本机缺 `buildx` 插件，`dockers_v2` 无法执行；并且 `.goreleaser.yml` 的镜像地址与 tag 依赖 `GITHUB_REPOSITORY_OWNER`、`GORELEASER_PRERELEASE` 两个 CI 才有的环境变量（前者由 GitHub 注入，后者在 `release.yml` 第 124 行注入）。CI 自己的 `release-dry-run` job 也**故意**带 `--skip=docker,publish`，因此"docker 渠道已可用"目前只有配置层证据（`goreleaser check` 通过 + 文档一致），**没有镜像构建/运行证据**。
- **`basework init` 的交互限制**：`init` 无参数、无 `--yes`/非交互开关，只提供交互式 provider 菜单。stdin 为 `/dev/null`（立即 EOF）时会退回默认值并正常写出配置；但 stdin 是**保持打开且没有数据**的管道时，进程会一直阻塞等待输入（本次验证中遇到过一次，需外部 kill）。记为已知限制，**本轮未修**——修改交互入口的行为超出本卡范围（属 UI-002/UI-003 领域），改动前应先在对应卡片里定义非交互契约。
- **升级的恢复路径**：`docs/installation.md` 的"故障排除"给出 `basework migrate rollback`（回滚 WAL 迁移）与 `session unlock`；`docs/release.md` 给出 dry-run 步骤。本轮验证的迁移是 JSONL → SQLite 的**新增写入**，源 JSONL 逐字节保留，因此"恢复"的最直接形式就是继续使用原 JSONL（本次已验证源文件未变）。
- 复现方式（全部在 `/private/tmp/ship002`，属临时目录、不随仓库保留）：
  - `verify-artifact.sh`：本文件的第 1–4 项验证脚本（用 GoReleaser 产物而非常规 `go build`）。
  - `fixtures/upgrade-v1.jsonl`（v1）、`fixtures/future-v99.jsonl`（v99）。
  - dry-run 命令见 `docs/release.md` 的"本地验证"一节。
- 下一步（不属本项）：SHIP-003 候选版本验收，需在本卡与 SHIP-001 完成后进行。

## 2026-09-15 当前工作区镜像复核

旧记录中的“缺 buildx”只描述 2026-09-13 的环境快照；当前环境已安装并可用
Docker Buildx v0.30.0，`basework-builder` 支持 linux/amd64、linux/arm64。
这段证据仍明确标记为 dirty worktree 产物，不能替代候选 commit 的发布包。

- 当前工作区用 `CGO_ENABLED=0`、`-tags "sqlite memory"` 分别构建
  `linux/amd64` 与 `linux/arm64` 二进制；SHA-256 分别为
  `c231eb71c83fffbb8027eba9e2fe1bcd8e66ab61835d4e61801cc4eb2f66328b` 和
  `f61c35d0ef00230b12f5d824d3b06b00a2f4cf3532eb8e9f75bffa5a10833991`。
- 使用 `docker buildx build --platform linux/amd64,linux/arm64` 从
  `docker/Dockerfile.goreleaser` 构建离线 OCI 镜像；manifest list digest 为
  `sha256:e898c8e2dea1deb4e452ab5c0b20880e9de0baa7ae93f78be071da3f4ad30f5c`，
  OCI 文件 SHA-256 为
  `96de5ae011ab790b8f9ce82c8b13dac8d71857ada0452889168aa7d69d1829e3`。
- 两个架构均实际运行 `basework version`：输出分别为 `linux/arm64`、
  `linux/amd64`；两个架构均在只读工作区挂载下运行 `facts show`，返回空事实且
  无写入工作区。镜像默认用户为 UID 0，`/usr/local/bin/basework` 权限为 0755，
  这是当前 Dockerfile 的实际发布语义。
- 只读根文件系统直接运行 `facts show` 会因产品需要创建会话数据目录而失败；补充
  隔离可写 `/root/.local/share/basework` 后通过。这一区分已记录，不能把“只读工作区”
  误写成“整个容器根目录只读可运行”。

## 2026-09-15 当前边界

本节补齐了当前候选工作区的 Docker 构建与运行证据，但 GoReleaser 当前候选包、
Windows/macOS/Linux 目标系统安装、真实 CI run 和外部 Provider 仍未获得，因此
SHIP-002 继续保持“待验证”，不把 dirty worktree 镜像当作发布候选。

当前本机 `gh auth status` 返回 GitHub token 无效，无法从这个工作区为未提交修改
触发或读取新的候选 CI run；这解释了为什么 CI 结果仍是待补证据，而不是把旧 run
或 workflow 定义误写成当前候选已通过。

## 2026-09-15 清洁候选镜像复核

为避免把 dirty worktree 镜像当成发布候选，本轮在清洁临时 checkout 的候选提交
`ac6852dc33d77a01ccb927e5a6cc46dafad804d5` 中重新交叉构建二进制，并用本机
`basework-builder`（BuildKit v0.32.2，支持 linux/amd64、linux/arm64）构建 OCI
manifest：

```text
docker buildx build --builder basework-builder \
  --platform linux/amd64,linux/arm64 \
  --file docker/Dockerfile.goreleaser \
  --tag basework:candidate-ac6852d \
  --output type=oci,dest=/private/tmp/basework-candidate-CMwi7Z/basework-ac6852d.oci .
```

结果可追溯为：manifest `sha256:97beb43a4bdf317d7be93b97623ffef2cce2dc0d9cba549b155e2b04bd0ef5b0`，
OCI 文件 SHA-256 `b74f8f0ebc7a4cc295541da5b50cefecd927966ff07b66c7a74976ce3605c455`；
候选二进制 SHA-256 为 linux/amd64 `dbbeb59d8638723e4ebd6065a48bee7626b6c57c23bea6568cb09e58fc8742eb`
和 linux/arm64 `00e1067b724cb04500cbfd540e1fbc4631e7f5c69fd380d0faf9f7288e250a5c`。

载入本机 Docker 后，两种架构均实际运行 `basework version`，分别返回
`platform: linux/amd64` 与 `platform: linux/arm64`；在 `/workspace:ro` 挂载候选源码、
并将会话数据隔离到临时可写目录时，两种架构均实际运行 `facts show`，返回空事实且
候选 checkout 的 Git 状态除构建上下文目录外没有源文件改动。候选全量
`go test -tags 'sqlite memory' -count=1 ./...` 已通过，覆盖迁移/恢复回归。

这段证据关闭“镜像构建与运行有记录”补充验收；它仍不能关闭“每个声明平台都有实际
安装结果且 CI 与候选 commit 可追溯”，因为 macOS/Windows 安装与候选 CI 尚未在对应
目标环境执行。

## 2026-09-15 当前 GoReleaser 快照

使用当前工作区（`HEAD=7a874c9`，仍有未提交修改）安装 GoReleaser 2.18.1 后，
重新执行与 CI 相同的检查和快照命令：

```text
goreleaser check
goreleaser release --snapshot --clean --skip=docker,publish
```

两条命令均退出 0。快照版本为 `0.1.4-SNAPSHOT-7a874c9`，产出 5 个平台包：
Darwin arm64/x86_64、Linux arm64/x86_64、Windows x86_64。当前机器实际解包运行
的只有 Darwin arm64：`basework version` 返回 `platform: darwin/arm64`，随后在
隔离 HOME 下运行 `config explain`，默认配置可解析且没有网络请求。其余包已用
`file` 核对为对应 ELF/PE/Mach-O 架构并记录哈希，但未在目标系统执行。

当前快照包 SHA-256：

| 产物 | SHA-256 |
|---|---|
| `basework_Darwin_arm64.tar.gz` | `9d102e9455d5a831544416a6d6fec3fdd7fc42e4de3f951d109c93e80babea6b` |
| `basework_Darwin_x86_64.tar.gz` | `a3d040dd14fff791cf716ee6f6e61cbcdfdba986595b78a915f034193b009398` |
| `basework_Linux_arm64.tar.gz` | `0087bd9dd4d9c56f05e22f088fe7ebb167d6a03c43a4833a1a45d5b582778192` |
| `basework_Linux_x86_64.tar.gz` | `db5b028ff71000d013f0d0acb7c13bf77371d27f7527cb196d94c22a0c9a7f85` |
| `basework_Windows_x86_64.zip` | `93ca415b3e28b9be5133e9d8fbe2eb515d3cb6d13f79148b26ee4489e3ee7143` |

这段证据把“旧 macOS 包”替换为当前 dirty worktree 快照，但仍不把它写成候选
commit 或 CI 结果；Docker 多平台证据见上一节。

补充状态校正：历史“已知限制”中关于 `basework init` 在保持打开的空管道上阻塞的
描述已由 OPT-001 修复覆盖。当前 `cmd/basework` 单测与真实 PTY 初始化入口均通过，
该段只保留为历史问题记录，不再作为当前能力缺口；当前发布缺口仍是目标平台安装、
候选 CI 和真实 Provider。

## 结论

- **是否满足任务卡全部验收**：
  - "不能用本机交叉编译冒充目标系统运行" —— 满足。本文件明确区分了"真正运行过的平台"（仅 darwin/arm64）与"仅交叉编译的平台"，并列出缺失平台。
  - "安装说明只包含已接入渠道" —— 满足。逐条核对 `docs/installation.md` / `docs/release.md` 与 `.goreleaser.yml`，且 Homebrew 被显式标注为未接入。
  - "升级不破坏原用户数据且有可执行恢复说明" —— 满足。v1 升级后源 JSONL 逐字节未变、内容完整落库；恢复说明在安装指南的故障排除一节。
- **建议状态：完成**，且附带"发现并修复 3 个迁移缺陷"。
- 原因：卡片的四条实施步骤（验证承诺平台产物、验证初次配置与升级/拒绝、执行打包 dry-run 并核对文档、记录系统与产物哈希及缺失平台）均有可复现证据。同时必须连带说明**两项未获得的证据**（目标平台真实运行、Docker 镜像构建），它们不构成本卡的阻塞项——因为卡片要求的正是"记录缺失平台"与"不得冒充"，而不是"必须本机跑通所有平台"；但 SHIP-003 在写发布报告时**不得**把这两项写成已验证。
