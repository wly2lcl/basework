# 模块依赖图（自动生成）

> 本文件由 `make deps` 自动生成，请勿手工编辑。
> 修改代码后重新运行 `make deps` 刷新即可；CI 会校验生成物是否与代码一致。

## 统计口径

| 项 | 口径 |
|---|---|
| 扫描范围 | `pkg/`、`internal/`、`cmd/` 下的目录 |
| 包判定 | 目录内含至少一个非测试 `.go` 文件 |
| 依赖边 | 同一包内非测试 `.go` 文件的 import，去重 |
| 标准库 | 导入路径首段不含 `.`（如 `net/http`），不计入依赖 |
| 构建约束 | 从 `//go:build` 读取，用于判断第三方依赖是否参与默认构建 |
| 排序 | 全部按字典序，保证输出确定性（CI 需要 `git diff --exit-code`） |
| 时间戳 | 刻意不输出，否则新鲜度门禁将永久失败 |

## 层级概览

| 层级 | 包数 | 非测试 .go 文件 | 定位 |
|---|---|---|---|
| `pkg/` | 12 | 103 | 可嵌入核心（稳定 API，不得依赖上层） |
| `internal/` | 14 | 81 | 终端产品专用逻辑（无兼容性承诺） |
| `cmd/` | 1 | 17 | 可执行入口 |

## 层级依赖图

```mermaid
graph BT
  pkg["pkg/"]
  internal["internal/"]
  cmd["cmd/"]
  internal --> pkg
  cmd --> internal
  cmd --> pkg
```

依赖方向为「上层 → 下层」。`pkg/` 不得指回 `internal/` 或 `cmd/`，
该约束由 `tests/arch_test.go` 守护（见「层级违规检测」）。

## 包级依赖图

```mermaid
graph LR
  subgraph pkg["pkg/"]
    direction TB
    n15["pkg/agent"]
    n16["pkg/config"]
    n17["pkg/hook"]
    n18["pkg/llm"]
    n19["pkg/lsp"]
    n20["pkg/mcp"]
    n21["pkg/memory"]
    n22["pkg/provider"]
    n23["pkg/session"]
    n24["pkg/skill"]
    n25["pkg/tool"]
    n26["pkg/tool/builtin"]
  end
  subgraph internal["internal/"]
    direction TB
    n1["internal/compaction"]
    n2["internal/loopdetect"]
    n3["internal/oauth"]
    n4["internal/observability"]
    n5["internal/permission"]
    n6["internal/retry"]
    n7["internal/subagent"]
    n8["internal/tools"]
    n9["internal/tui"]
    n10["internal/tui/command"]
    n11["internal/tui/dialog"]
    n12["internal/tui/keymap"]
    n13["internal/tui/plugin"]
    n14["internal/tui/theme"]
  end
  subgraph cmd["cmd/"]
    direction TB
    n0["cmd/basework"]
  end
  n0 --> n1
  n0 --> n15
  n0 --> n16
  n0 --> n18
  n0 --> n19
  n0 --> n2
  n0 --> n20
  n0 --> n22
  n0 --> n23
  n0 --> n24
  n0 --> n25
  n0 --> n26
  n0 --> n3
  n0 --> n4
  n0 --> n5
  n0 --> n7
  n0 --> n8
  n0 --> n9
  n1 --> n15
  n1 --> n18
  n1 --> n23
  n15 --> n17
  n15 --> n18
  n15 --> n23
  n15 --> n25
  n17 --> n18
  n17 --> n25
  n19 --> n25
  n2 --> n15
  n2 --> n18
  n20 --> n25
  n21 --> n18
  n22 --> n16
  n22 --> n18
  n23 --> n18
  n25 --> n18
  n26 --> n25
  n4 --> n15
  n5 --> n15
  n6 --> n18
  n7 --> n15
  n7 --> n25
  n8 --> n16
  n8 --> n20
  n8 --> n25
  n8 --> n5
  n9 --> n10
  n9 --> n11
  n9 --> n12
  n9 --> n13
  n9 --> n14
  n9 --> n15
  n9 --> n18
  n9 --> n25
```

## 依赖边清单

| 包 | 依赖 |
|---|---|
| `cmd/basework` | `internal/compaction`、`internal/loopdetect`、`internal/oauth`、`internal/observability`、`internal/permission`、`internal/subagent`、`internal/tools`、`internal/tui`、`pkg/agent`、`pkg/config`、`pkg/llm`、`pkg/lsp`、`pkg/mcp`、`pkg/provider`、`pkg/session`、`pkg/skill`、`pkg/tool`、`pkg/tool/builtin` |
| `internal/compaction` | `pkg/agent`、`pkg/llm`、`pkg/session` |
| `internal/loopdetect` | `pkg/agent`、`pkg/llm` |
| `internal/observability` | `pkg/agent` |
| `internal/permission` | `pkg/agent` |
| `internal/retry` | `pkg/llm` |
| `internal/subagent` | `pkg/agent`、`pkg/tool` |
| `internal/tools` | `internal/permission`、`pkg/config`、`pkg/mcp`、`pkg/tool` |
| `internal/tui` | `internal/tui/command`、`internal/tui/dialog`、`internal/tui/keymap`、`internal/tui/plugin`、`internal/tui/theme`、`pkg/agent`、`pkg/llm`、`pkg/tool` |
| `pkg/agent` | `pkg/hook`、`pkg/llm`、`pkg/session`、`pkg/tool` |
| `pkg/hook` | `pkg/llm`、`pkg/tool` |
| `pkg/lsp` | `pkg/tool` |
| `pkg/mcp` | `pkg/tool` |
| `pkg/memory` | `pkg/llm` |
| `pkg/provider` | `pkg/config`、`pkg/llm` |
| `pkg/session` | `pkg/llm` |
| `pkg/tool` | `pkg/llm` |
| `pkg/tool/builtin` | `pkg/tool` |
| `internal/oauth` | （无模块内依赖） |
| `internal/tui/command` | （无模块内依赖） |
| `internal/tui/dialog` | （无模块内依赖） |
| `internal/tui/keymap` | （无模块内依赖） |
| `internal/tui/plugin` | （无模块内依赖） |
| `internal/tui/theme` | （无模块内依赖） |
| `pkg/config` | （无模块内依赖） |
| `pkg/llm` | （无模块内依赖） |
| `pkg/skill` | （无模块内依赖） |

## 扇入 / 扇出

扇入（有多少个包依赖它）高的包是「改动影响面最大」的包，重构时应优先关注
其兼容性承诺；扇出则反映一个包的依赖复杂度。

### 扇入 Top（被依赖最多）

| 包 | 计数 |
|---|---|
| `pkg/llm` | 11 |
| `pkg/tool` | 9 |
| `pkg/agent` | 7 |
| `pkg/config` | 3 |
| `pkg/session` | 3 |
| `internal/permission` | 2 |
| `pkg/mcp` | 2 |
| `internal/compaction` | 1 |
| `internal/loopdetect` | 1 |
| `internal/oauth` | 1 |
| `internal/observability` | 1 |
| `internal/subagent` | 1 |

（共 24 个包有非零计数，此处仅列前 12）

### 扇出 Top（依赖最多）

| 包 | 计数 |
|---|---|
| `cmd/basework` | 18 |
| `internal/tui` | 8 |
| `internal/tools` | 4 |
| `pkg/agent` | 4 |
| `internal/compaction` | 3 |
| `internal/loopdetect` | 2 |
| `internal/subagent` | 2 |
| `pkg/hook` | 2 |
| `pkg/provider` | 2 |
| `internal/observability` | 1 |
| `internal/permission` | 1 |
| `internal/retry` | 1 |

（共 18 个包有非零计数，此处仅列前 12）

## 层级违规检测

✅ 未发现层级或依赖边界违规。

## pkg/ 层第三方依赖明细

用于核对 docs/ARCHITECTURE.md 的依赖策略声明。构建约束为空表示该依赖在**默认构建**
（不带 build tag）下即被引入；否则仅在使用对应 tag 时引入。

| 包 | 第三方依赖 | 构建约束 | 白名单 |
|---|---|---|---|
| `pkg/llm` | `golang.org/x/image/draw` | 默认构建 | ✅ |
| `pkg/llm` | `golang.org/x/image/webp` | 默认构建 | ✅ |
| `pkg/memory` | `modernc.org/sqlite` | `memory` | ✅ |
| `pkg/provider` | `golang.org/x/oauth2` | 默认构建 | ✅ |
| `pkg/session` | `github.com/golang/snappy` | `sqlite` | ✅ |
| `pkg/session` | `modernc.org/sqlite` | `sqlite` | ✅ |
