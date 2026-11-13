# REUSE-005 验证记录：可观测性包上移

## 基线

- 起点 HEAD：`a1f4d7f`（feat: promote permission engine to pkg/permission）。
- 环境：macOS arm64 / go1.26.10。
- 移动前基线（来自 [REUSE-001](REUSE-001.md)）：`internal/observability` 68 个用例，排序清单 sha256 `c9bde345b5c7b68ea6b2b209eaf35c38e1b253517eebc55fffe17f9b1cd910af`。
- 决策依据：[ADR 0008](../../adr/0008-promote-internal-capabilities-to-pkg.md)。

## 变更

- `git mv` 核心 11 个文件到 `pkg/observability/`（`logger/cost/tokens/event/bus/subscriber/config/pprof` 与 3 个测试），包名与导出符号不变。
- `internal/observability/adapter.go` 保留，`EventBusAdapter` 改引用 `pkg/observability.Event`/`EventBus`，8 行桥接逻辑不变。
- 消费方改 import：`cmd/basework/profile.go`、`tests/benchmark/token_bench_test.go` 改为 `pkg/observability`；`cmd/basework/runtime.go` 叠加 `observabilityadapter` 别名引用 `internal/observability`。
- 新增 `docs/reference/pkg/observability.md` 并登记包契约索引。
- `make gen` 刷新 `docs/DEPGRAPH.md`、`docs/STATS.md`。

行为不变：日志 JSON 字段与级别、成本/token 口径、事件异步有界投递（并发 100）、panic 隔离、关闭后不再投递、pprof 默认关闭。

## 验证

```
go build ./...
→ BUILD_OK
go build -tags 'sqlite memory' ./...
→ BUILD_TAGS_OK

go list -deps ./pkg/observability | grep basework/
→ pkg/observability（仅标准库；无 internal、无 pkg/agent）
```

```
go test -list '.*' ./pkg/observability | grep '^Test' | sort | shasum -a 256
→ c9bde345b5c7b68ea6b2b209eaf35c38e1b253517eebc55fffe17f9b1cd910af
go test -list '.*' ./pkg/observability | grep -c '^Test'
→ 68
```

与移动前逐字一致。

```
go test -race ./pkg/observability ./cmd/basework -count=1
→ ok pkg/observability 4.842s；ok cmd/basework 1.672s

go test -run XXX ./tests/benchmark
→ ok（编译通过，无测试运行）

make check-docs
→ 通过（pkg 包契约 16 个）
make check-arch
→ pkg→上层违规 0；注入点白名单 3/3 未变
go vet ./pkg/observability ./internal/observability ./cmd/basework
→ 通过

git diff --color-moved=plain --stat HEAD -- pkg/observability internal/observability
→ 12 files changed, 6 insertions(+), 4 deletions(-)
```

其中核心 11 个文件均为 0 改动；`6/4` 全部来自保留的 `adapter.go` 类型限定名调整（无逻辑变化）。

## 剩余与交接

- Windows 实机与五平台 CI 留到 REUSE-006。
- `internal/observability` 现在只有 `adapter.go`，符合 ADR 0008 的适配层归属。
- 无未决问题。

## 结论

可观测性核心已纯移动为 `pkg/observability`，依赖仅标准库、不依赖 `pkg/agent`；适配器留在 `internal/observability`。68 个用例名与数量与基线完全一致，race、完整 tags 构建、文档/架构门禁与 vet 全部通过。REUSE-006 可开始。
