# 代码统计（自动生成）

> 本文件由 `make stats` 自动生成，请勿手工编辑。
> 修改代码后重新运行 `make stats` 刷新即可。

## 统计口径

| 项 | 口径 |
|---|---|
| 代码行数 | 非空、非纯注释行；不含 `/* */` 块内注释 |
| 测试代码 | 文件名以 `_test.go` 结尾的 `.go` 文件 |
| 测试用例 | 匹配 `func Test\|Benchmark\|Fuzz` 的函数声明数 |
| 排除目录 | `.git`、`vendor`、`bin`、`.opencode`、`node_modules` |
| 覆盖率 | `go test -tags "sqlite memory" -cover ./...`，仅统计含语句的包 |

生成时间：2026-09-12 09:19:08

## 总览

| 指标 | 数值 |
|---|---|
| 代码总行数 | 57816 |
| 非测试代码行数 | 22810 |
| 测试代码行数 | 35006 |
| 测试代码占比 | 60.5% |
| `.go` 文件数 | 347 |
| `_test.go` 文件数 | 146 |
| 测试用例数 | 1645 |

## 按目录分布

| 目录 | 总行数 | 非测试行数 | 测试行数 | .go 文件 | 测试文件 | 测试用例 |
|---|---|---|---|---|---|---|
| `pkg` | 33246 | 12225 | 21021 | 185 | 84 | 953 |
| `internal` | 17315 | 7815 | 9500 | 122 | 41 | 547 |
| `tests` | 3874 | 0 | 3874 | 15 | 15 | 116 |
| `cmd` | 3029 | 2418 | 611 | 23 | 6 | 29 |
| `scripts` | 285 | 285 | 0 | 1 | 0 | 0 |
| `examples` | 67 | 67 | 0 | 1 | 0 | 0 |

## 测试覆盖率（升序，优先关注靠前项）

| 包 | 覆盖率 |
|---|---|
| `cmd/basework` | 21.7% |
| `pkg/llm` | 38.3% |
| `internal/tui` | 53.6% |
| `internal/tui/theme` | 59.5% |
| `internal/tools` | 68.3% |
| `internal/oauth` | 68.6% |
| `pkg/provider` | 69.5% |
| `internal/permission` | 70.1% |
| `pkg/tool/builtin` | 73.3% |
| `pkg/agent` | 78.5% |
| `pkg/session` | 81.6% |
| `internal/tui/command` | 82.8% |
| `internal/retry` | 84.4% |
| `pkg/lsp` | 85.2% |
| `pkg/memory` | 85.6% |
| `pkg/config` | 85.7% |
| `pkg/tool` | 85.9% |
| `internal/subagent` | 87.2% |
| `pkg/mcp` | 87.4% |
| `internal/loopdetect` | 87.7% |
| `internal/compaction` | 88.9% |
| `pkg/hook` | 90.1% |
| `internal/observability` | 90.8% |
| `internal/tui/dialog` | 91.0% |
| `pkg/skill` | 92.3% |
| `internal/tui/keymap` | 93.0% |
| `internal/tui/plugin` | 100.0% |

