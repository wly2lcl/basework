# REUSE-004 验证记录：权限引擎包上移

## 基线

- 起点 HEAD：`660bb67`（feat: promote edit planning and commit to pkg/edits）。
- 环境：macOS arm64 / go1.26.10。
- 移动前基线（来自 [REUSE-001](REUSE-001.md)）：`internal/permission` 57 个用例（sha256 `502ba28d…56d7`）、`-tags sqlite` 81 个（sha256 `14ac9a0d…801a`）。
- 决策依据：[ADR 0008](../../adr/0008-promote-internal-capabilities-to-pkg.md)。

## 变更

- `git mv` 核心 17 个文件到 `pkg/permission/`（`checker/mode/rule/scope/paths/cache/store/sqlite_store/audit/audit_sqlite/migration/approval` 与对应测试），包名与导出符号不变。
- `internal/permission/adapter.go` 保留，只把 `Checker`/`PathChecker` 类型引用改为 `pkg/permission` 的限定名；`PermissionAdapter`、`PathPermissionAdapter` 的签名与行为不变。
- 消费方改 import：`internal/tools/{applypatch,question}.go` 与测试、`cmd/basework/{permission,tui,runtime_permission_sqlite,runtime_test,facts_test}.go`、`tests/permission_migration_test.go` 改为 `pkg/permission`；`cmd/basework/runtime.go` 叠加 `permissionadapter` 别名引用 `internal/permission`。
- 新增 `docs/reference/pkg/permission.md` 并登记包契约索引；[权限指南](../../guides/permission-guide.md) 更新实现位置并新增「作为库使用」适配示例（同时实现 `Check` 与 `CheckPath`）。
- `make gen` 刷新 `docs/DEPGRAPH.md`、`docs/STATS.md`。

SEC-001 语义未变：模式优先级、缺上下文 fail-closed、检查开始时的上下文快照、缓存精确参数匹配、`session > project > global` 规则优先级、SQLite 迁移与审计字段均保持原实现。

## 验证

```
go build ./...
→ BUILD_OK
go build -tags 'sqlite memory' ./...
→ BUILD_TAGS_OK

go list -deps ./pkg/permission | grep basework/
→ pkg/permission（仅标准库；无 internal、无 pkg/agent）
```

测试基线（移动后）：

```
go test -list '.*' ./pkg/permission | grep '^Test' | sort | shasum -a 256
→ 502ba28d855d69c29116ad4db05277ac2be3c0200d31133eb0a523d37ff556d7（57 个）
go test -tags 'sqlite' -list '.*' ./pkg/permission | grep '^Test' | sort | shasum -a 256
→ 14ac9a0d6cbe9d8b57cc3d5e901e6866d2eb2c4218171ece7218a1fd78be801a（81 个）
```

与移动前逐字一致。

```
go test -race -tags 'sqlite memory' ./pkg/permission ./internal/tools ./cmd/basework -count=1
→ ok pkg/permission 2.151s；ok internal/tools 11.007s；ok cmd/basework 1.808s

go test -count=10 -tags 'sqlite memory' ./pkg/permission
→ ok pkg/permission 2.530s

make check-docs
→ 通过（pkg 包契约 15 个）
make check-arch
→ pkg→上层违规 0；注入点白名单仍为 3 条
go vet ./pkg/permission ./internal/permission ./internal/tools ./cmd/basework
→ 通过
go vet -tags 'sqlite memory' ./...
→ 通过
```

隔离 HOME 的 CLI 规则场景：

```
TMPHOME=$(mktemp -d)
HOME=$TMPHOME /tmp/basework-m10 permission add --type allow --pattern 'read_file' --scope global
→ ✅ 已创建权限规则: da0e4f45-…（类型 allow，作用域 global）
HOME=$TMPHOME /tmp/basework-m10 permission list
→ 列出该规则（来源 user）
HOME=$TMPHOME /tmp/basework-m10 permission audit --days 7
→ 没有找到匹配的审计记录
```

纯移动审查：

```
git diff --color-moved=plain --stat HEAD -- pkg/permission internal/permission
→ 18 files changed, 9 insertions(+), 7 deletions(-)
```

其中核心 17 个文件均为 0 改动；`9/7` 全部来自保留的 `adapter.go` 类型限定名调整（无逻辑变化）。

## 剩余与交接

- Windows 实机与五平台 CI 留到 REUSE-006。
- `internal/permission` 现在只有 `adapter.go`，符合 ADR 0008 的适配层归属。
- 无未决问题。

## 结论

权限核心已纯移动为 `pkg/permission`，依赖仅标准库、不依赖 `pkg/agent`；适配器留在 `internal/permission`。57/81 个用例名与数量与基线完全一致，race、`-count=10`、完整 tags 构建、文档/架构门禁、隔离 HOME 的 CLI 规则场景全部通过。REUSE-005 可开始。
