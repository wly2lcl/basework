# REUSE-002 验证记录：后台任务包上移

## 基线

- 起点 HEAD：`d01d812`（docs: record M10 reuse evaluation and promotion contract）。
- 环境：macOS arm64（darwin/arm64）/ go1.26.10。
- 移动前基线（来自 [REUSE-001](REUSE-001.md)）：`internal/jobs` 60 个用例，排序清单 sha256 `5f6d9ab22f21a2fe71962dd21a543770b40c9a922384162054ead06b0d538b21`。
- 决策依据：[ADR 0008](../../adr/0008-promote-internal-capabilities-to-pkg.md)。

## 变更

- `git mv internal/jobs/*.go pkg/jobs/`：14 个文件（7 个实现 + 7 个测试），包名与导出符号不变。
- 消费方 import 改名（只改路径，不改调用）：`internal/runtime/service.go`、`internal/tools/{background,jobs_tools}.go` 与测试、`cmd/basework/{jobs,facts,resume,runtime,tui}.go` 与测试、`tests/benchmark/long_session_bench_test.go`。
- `internal/tools/background.go` 注释中的 `internal/jobs/process_windows.go` 改为 `pkg/jobs/process_windows.go`。
- 新增 `docs/reference/pkg/jobs.md`（五节契约）并在 [包契约索引](../../reference/README.md) 登记。
- `make gen` 刷新 `docs/DEPGRAPH.md` 与 `docs/STATS.md`。

## 验证

```
go build ./...
→ BUILD_OK

go list -deps ./pkg/jobs | grep basework/
→ pkg/llm、pkg/session、pkg/jobs（无 internal）
```

测试基线（移动后）：

```
go test -list '.*' ./pkg/jobs | grep '^Test' | sort | shasum -a 256
→ 5f6d9ab22f21a2fe71962dd21a543770b40c9a922384162054ead06b0d538b21
go test -list '.*' ./pkg/jobs | grep -c '^Test'
→ 60
```

与移动前逐字一致（用例名与数量均未变）。

```
go test -race ./pkg/jobs ./internal/runtime ./internal/tools ./cmd/basework -count=1
→ ok pkg/jobs 5.680s；ok internal/runtime 1.436s；ok internal/tools 11.364s；ok cmd/basework 1.646s

go test -count=10 ./pkg/jobs
→ ok pkg/jobs 37.724s

GOOS=windows GOARCH=amd64 go build ./...
→ WIN_BUILD_OK（交叉编译，非运行结论）

make check-docs
→ 通过（pkg 包契约 13 个）
make check-arch
→ pkg→上层违规 0；注入点白名单 3/3 未变
go vet ./pkg/jobs ./internal/tools ./internal/runtime ./cmd/basework
→ 通过
```

纯移动审查：

```
git diff --color-moved=plain --stat HEAD -- pkg/jobs internal/jobs
→ 14 files changed, 0 insertions(+), 0 deletions(-)
```

平台分支核对：`process_unix.go`/`process_windows.go`/`process_other.go` 及对应测试随包移动，构建标签未改；本机只运行 unix 分支，Windows 分支由交叉编译与源码清单核对，运行结论留到 REUSE-006 的真实平台记录。

## 剩余与交接

- 未运行 Windows 实机测试；未执行五平台 CI（REUSE-006 汇总）。
- `internal/jobs` 目录已空，仅本机空目录残留，git 不跟踪。
- 无未决问题。

## 结论

`internal/jobs` 已纯移动为 `pkg/jobs`：依赖只剩 `pkg/session`（与传递的 `pkg/llm`），60 个用例名与数量基线完全一致，race、`-count=10`、Windows amd64 交叉编译与文档/架构门禁全部通过。REUSE-003 可开始。
