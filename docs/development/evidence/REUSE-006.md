# REUSE-006 验证记录：外部嵌入验收与文档收口

## 基线

- 起点 HEAD：`2507249`（feat: promote observability to pkg/observability），即四个移动 commit 全部完成之后。
- 环境：macOS arm64 / go1.26.10；未在本机运行 Windows 实机或五平台 CI。
- 本任务依据 [ADR 0008](../../adr/0008-promote-internal-capabilities-to-pkg.md) 收口。

## 变更

- 新增 `examples/embed_components/main.go`：仅用 `pkg` 公共 API 组合四包，覆盖：
  - 两个 `jobs.Manager`（ws-a/ws-b，不同 ID 前缀与输出目录）并行启动，一个取消、一个成功并读回输出；
  - 同 owner 跨 Manager 查询返回 `jobs.ErrNotFound`，`List` 各为 1，证明实例隔离；
  - 编辑预览不写盘 → 提交落盘 → 撤销恢复原内容；
  - `yolo` 允许、`deny-all` 拒绝，`strict` 路径检查拒绝 `.git/config`、允许普通工作区文件；
  - 事件总线：16 个慢订阅者事件发布耗时 < 200ms，全部收到，证明不阻塞发布者；成本与 token 统计断言；
  - 工具通过 `builtin.AllWithRuntime(&builtin.Runtime{PathChecker, EventBus})` 注入，全程未调用 `builtin.Set*`。
- `docs/ARCHITECTURE.md`：新增「实验 API（0.x）」层级与归入条件，依赖清单加入四包。
- `docs/guides/embedder-guide.md`：新增「可靠性组件（实验 API）」小节（用途、边界、实例注入、适配器示例指引）。
- `docs/README.md`：使用指南增加可靠性组件入口；`docs/STATUS.md` 嵌入行更新；`docs/CHANGELOG.md` 记录本次上移。
- `internal/jobs`、`internal/edits` 已无 Go 文件；`internal/permission`、`internal/observability` 只剩 `adapter.go`。

## 验证

### 测试与门禁（本机）

```
go build ./...
→ BUILD_OK
GOOS=windows GOARCH=amd64 go build ./...
→ WIN_BUILD_OK（交叉编译，非运行结论）

H=$(mktemp -d) && HOME=$H go test ./... -count=1 -p 1
→ 全部通过（isolated HOME，避免写入真实用户配置）

H=$(mktemp -d) && HOME=$H go test -tags 'sqlite memory' ./... -count=1 -p 1
→ 全部通过

H=$(mktemp -d) && HOME=$H go test -race -tags 'sqlite memory' \
  ./pkg/jobs ./pkg/edits ./pkg/permission ./pkg/observability \
  ./internal/tools ./internal/runtime ./cmd/basework -count=1 -p 1
→ 全部 ok

make vet
→ go vet -tags "sqlite memory" ./... 通过
make check-docs
→ 通过（pkg 包契约 16 个）
make check-arch
→ pkg→上层违规 0；注入点白名单 3/3 未变
make gen && git diff --exit-code
→ 无差异（DEPGRAPH/STATS 已是最新）
```

### PTY 场景

```
python3 tests/tui_pty/run_all.py
→ scenario1..scenario8 全部 PASS (exit 0)
```

### 外部 module（仅公共 API）

```
EXT=$(mktemp -d /tmp/basework-embed-ext.XXXXXX)
cd "$EXT"
go mod init embedcheck
go mod edit -require=github.com/wly2lcl/basework@v0.0.0 \
  -replace=github.com/wly2lcl/basework=<repo>
cp <repo>/examples/embed_components/main.go .
go mod tidy && go run .
→ 嵌入组件验收通过: jobs=a-job-001/b-job-001 编辑=notes.txt 事件=16 成本=0.0123 token=1500 工具=6
→ go list -deps . | grep basework/ 只含 pkg/{edits,jobs,observability,permission,llm,session,tool,tool/builtin}
→ basework/internal 依赖：none
```

### 用例基线抽查

```
pkg/jobs            5f6d9ab22f21a2fe71962dd21a543770b40c9a922384162054ead06b0d538b21（60）
pkg/edits           b7b3b1018032acd649ef1c1cdce7409dcb218a51913efb6edbaab1a9fe428c5d（36）
pkg/permission      502ba28d855d69c29116ad4db05277ac2be3c0200d31133eb0a523d37ff556d7（57）
pkg/permission+s   14ac9a0d6cbe9d8b57cc3d5e901e6866d2eb2c4218171ece7218a1fd78be801a（81）
pkg/observability   c9bde345b5c7b68ea6b2b209eaf35c38e1b253517eebc55fffe17f9b1cd910af（68）
```

与 [REUSE-001](REUSE-001.md) 记录的移动前基线全部逐字一致；对应移动 commit 为 `bc89bca`、`660bb67`、`26b7286`、`2507249`。

## 环境限制与异常

- `make lint` 失败：本机未安装 `golangci-lint`（`No such file or directory`），不是代码问题；`make vet` 已通过。
- 五平台 CI 与 Windows 实机运行未在本会话执行，只记录本机 darwin/arm64 运行与 Windows amd64 交叉编译；不能把交叉编译当作 Windows 运行结论。
- 首次 `go test ./...` 出现两个与本变更无关的现象，均已单独复验：
  - `pkg/jobs` 的 `TestTerminateCommand_ForceKillsAfterGrace` 在整仓并行高负载下超时，单独 `-count=5` 与 `-count=10` 均通过（计时敏感）；
  - `pkg/provider` 的 `TestCopilot_TokenRefreshExpired` 在沙箱内写 `~/.config` 被拒绝，在沙箱外通过；`pkg/provider` 本次未改动。使用隔离 `HOME` 后整仓测试为绿。

## 剩余与交接

- 发布与 API 兼容门禁（1.0 tag、apidiff）按计划不在本任务内，见 [10-reuse.md](../../tasks/10-reuse.md#后续衔接不在本计划内)。
- 四个新包仍是实验 API；首次破坏性调整需新增 ADR 并在 `CHANGELOG.md` 写迁移方式。
- 无未决问题。

## 结论

四包上移完成并收口：仓库外 module 只用公共 API 即可组合 jobs/edits/permission/observability，双实例互不污染；全量默认与 `sqlite memory` 测试、race、PTY 1–8、Windows 交叉编译、vet、文档与架构门禁、`make gen` 无差异全部通过。M10 六项任务全部完成。`golangci-lint` 与五平台 CI 未在本机执行，已在「环境限制」中如实记录。
