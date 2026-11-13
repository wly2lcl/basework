# REUSE-003 验证记录：编辑计划与提交包上移

## 基线

- 起点 HEAD：`bc89bca`（feat: promote background jobs to pkg/jobs）。
- 环境：macOS arm64 / go1.26.10。
- 移动前基线（来自 [REUSE-001](REUSE-001.md)）：`internal/edits` 36 个用例，排序清单 sha256 `b7b3b1018032acd649ef1c1cdce7409dcb218a51913efb6edbaab1a9fe428c5d`。
- 决策依据：[ADR 0008](../../adr/0008-promote-internal-capabilities-to-pkg.md)。

## 变更

- `git mv internal/edits/{plan.go,commit.go,plan_test.go,commit_test.go} pkg/edits/`，包名与导出符号不变。
- `internal/tools/edit_files.go` 仅改 import 路径。
- 新增 `docs/reference/pkg/edits.md`（五节契约）并在 [包契约索引](../../reference/README.md) 登记。
- `make gen` 刷新 `docs/DEPGRAPH.md`、`docs/STATS.md`。

行为边界未变：预览不写盘；逐文件原子、跨文件非事务；提交前重新解析路径/权限/基线；撤销遇到后来编辑按文件跳过并说明原因。`file.edited` 事实与审批接线未改。

## 验证

```
go build ./...
→ BUILD_OK

go list -deps ./pkg/edits | grep basework/
→ pkg/edits（仅标准库；无 internal、无其它模块内包）
```

```
go test -list '.*' ./pkg/edits | grep '^Test' | sort | shasum -a 256
→ b7b3b1018032acd649ef1c1cdce7409dcb218a51913efb6edbaab1a9fe428c5d
go test -list '.*' ./pkg/edits | grep -c '^Test'
→ 36
```

与移动前逐字一致。

```
go test -race ./pkg/edits ./internal/tools ./cmd/basework -count=1
→ ok pkg/edits 1.534s；ok internal/tools 11.919s；ok cmd/basework 1.711s

make check-docs
→ 通过（pkg 包契约 14 个）
make check-arch
→ pkg→上层违规 0；注入点白名单 3/3 未变
go vet ./pkg/edits ./internal/tools ./cmd/basework
→ 通过

git diff --color-moved=plain --stat HEAD -- pkg/edits internal/edits
→ 4 files changed, 0 insertions(+), 0 deletions(-)
```

测试覆盖的既有场景（全部通过）：路径越界、软链逃逸、权限拒绝、基线冲突、部分成功、`StopOnError` 两种模式、撤销跳过后来编辑、BOM/CRLF 保留、二进制拒绝、预览不写盘。

## 剩余与交接

- Windows 实机与五平台 CI 留到 REUSE-006。
- 无未决问题。

## 结论

`internal/edits` 已纯移动为 `pkg/edits`，零模块内依赖，36 个用例名与数量与基线完全一致；race、文档/架构门禁与 vet 全部通过，`internal/tools` 的预览/提交/撤销回归未变。REUSE-004 可开始。
