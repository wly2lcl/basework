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
| `pkg/` | 15 | 134 | 可嵌入核心（稳定 API，不得依赖上层） |
| `internal/` | 15 | 79 | 终端产品专用逻辑（无兼容性承诺） |
| `cmd/` | 1 | 23 | 可执行入口 |

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
    n16["pkg/agent"]
    n17["pkg/config"]
    n18["pkg/edits"]
    n19["pkg/hook"]
    n20["pkg/jobs"]
    n21["pkg/llm"]
    n22["pkg/lsp"]
    n23["pkg/mcp"]
    n24["pkg/memory"]
    n25["pkg/permission"]
    n26["pkg/provider"]
    n27["pkg/session"]
    n28["pkg/skill"]
    n29["pkg/tool"]
    n30["pkg/tool/builtin"]
  end
  subgraph internal["internal/"]
    direction TB
    n1["internal/compaction"]
    n2["internal/loopdetect"]
    n3["internal/oauth"]
    n4["internal/observability"]
    n5["internal/permission"]
    n6["internal/retry"]
    n7["internal/runtime"]
    n8["internal/subagent"]
    n9["internal/tools"]
    n10["internal/tui"]
    n11["internal/tui/command"]
    n12["internal/tui/dialog"]
    n13["internal/tui/keymap"]
    n14["internal/tui/plugin"]
    n15["internal/tui/theme"]
  end
  subgraph cmd["cmd/"]
    direction TB
    n0["cmd/basework"]
  end
  n0 --> n1
  n0 --> n10
  n0 --> n16
  n0 --> n17
  n0 --> n2
  n0 --> n20
  n0 --> n21
  n0 --> n22
  n0 --> n23
  n0 --> n25
  n0 --> n26
  n0 --> n27
  n0 --> n28
  n0 --> n29
  n0 --> n3
  n0 --> n30
  n0 --> n4
  n0 --> n5
  n0 --> n7
  n0 --> n8
  n0 --> n9
  n1 --> n16
  n1 --> n21
  n1 --> n27
  n10 --> n11
  n10 --> n12
  n10 --> n13
  n10 --> n14
  n10 --> n15
  n10 --> n16
  n10 --> n21
  n10 --> n29
  n16 --> n19
  n16 --> n21
  n16 --> n27
  n16 --> n29
  n19 --> n21
  n19 --> n29
  n2 --> n16
  n2 --> n21
  n20 --> n27
  n22 --> n29
  n23 --> n29
  n24 --> n21
  n26 --> n17
  n26 --> n21
  n27 --> n21
  n29 --> n21
  n30 --> n29
  n4 --> n16
  n5 --> n16
  n5 --> n25
  n6 --> n21
  n7 --> n16
  n7 --> n20
  n7 --> n21
  n7 --> n27
  n7 --> n29
  n8 --> n16
  n8 --> n29
  n9 --> n17
  n9 --> n18
  n9 --> n20
  n9 --> n23
  n9 --> n25
  n9 --> n27
  n9 --> n29
  n9 --> n30
```

## 依赖边清单

| 包 | 依赖 |
|---|---|
| `cmd/basework` | `internal/compaction`、`internal/loopdetect`、`internal/oauth`、`internal/observability`、`internal/permission`、`internal/runtime`、`internal/subagent`、`internal/tools`、`internal/tui`、`pkg/agent`、`pkg/config`、`pkg/jobs`、`pkg/llm`、`pkg/lsp`、`pkg/mcp`、`pkg/permission`、`pkg/provider`、`pkg/session`、`pkg/skill`、`pkg/tool`、`pkg/tool/builtin` |
| `internal/compaction` | `pkg/agent`、`pkg/llm`、`pkg/session` |
| `internal/loopdetect` | `pkg/agent`、`pkg/llm` |
| `internal/observability` | `pkg/agent` |
| `internal/permission` | `pkg/agent`、`pkg/permission` |
| `internal/retry` | `pkg/llm` |
| `internal/runtime` | `pkg/agent`、`pkg/jobs`、`pkg/llm`、`pkg/session`、`pkg/tool` |
| `internal/subagent` | `pkg/agent`、`pkg/tool` |
| `internal/tools` | `pkg/config`、`pkg/edits`、`pkg/jobs`、`pkg/mcp`、`pkg/permission`、`pkg/session`、`pkg/tool`、`pkg/tool/builtin` |
| `internal/tui` | `internal/tui/command`、`internal/tui/dialog`、`internal/tui/keymap`、`internal/tui/plugin`、`internal/tui/theme`、`pkg/agent`、`pkg/llm`、`pkg/tool` |
| `pkg/agent` | `pkg/hook`、`pkg/llm`、`pkg/session`、`pkg/tool` |
| `pkg/hook` | `pkg/llm`、`pkg/tool` |
| `pkg/jobs` | `pkg/session` |
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
| `pkg/edits` | （无模块内依赖） |
| `pkg/llm` | （无模块内依赖） |
| `pkg/permission` | （无模块内依赖） |
| `pkg/skill` | （无模块内依赖） |

## 扇入 / 扇出

扇入（有多少个包依赖它）高的包是「改动影响面最大」的包，重构时应优先关注
其兼容性承诺；扇出则反映一个包的依赖复杂度。

### 扇入 Top（被依赖最多）

| 包 | 计数 |
|---|---|
| `pkg/llm` | 12 |
| `pkg/tool` | 10 |
| `pkg/agent` | 8 |
| `pkg/session` | 6 |
| `pkg/config` | 3 |
| `pkg/jobs` | 3 |
| `pkg/permission` | 3 |
| `pkg/mcp` | 2 |
| `pkg/tool/builtin` | 2 |
| `internal/compaction` | 1 |
| `internal/loopdetect` | 1 |
| `internal/oauth` | 1 |

（共 28 个包有非零计数，此处仅列前 12）

### 扇出 Top（依赖最多）

| 包 | 计数 |
|---|---|
| `cmd/basework` | 21 |
| `internal/tools` | 8 |
| `internal/tui` | 8 |
| `internal/runtime` | 5 |
| `pkg/agent` | 4 |
| `internal/compaction` | 3 |
| `internal/loopdetect` | 2 |
| `internal/permission` | 2 |
| `internal/subagent` | 2 |
| `pkg/hook` | 2 |
| `pkg/provider` | 2 |
| `internal/observability` | 1 |

（共 20 个包有非零计数，此处仅列前 12）

## 层级违规检测

❌ 发现违规，必须修复：

| 来源 | 目标 | 违反规则 |
|---|---|---|
| `pkg/permission` | `github.com/google/uuid` | pkg/ 引入未登记在白名单的第三方依赖（见 docs/ARCHITECTURE.md） |

## pkg/ 层第三方依赖明细

用于核对 docs/ARCHITECTURE.md 的依赖策略声明。构建约束为空表示该依赖在**默认构建**
（不带 build tag）下即被引入；否则仅在使用对应 tag 时引入。

| 包 | 第三方依赖 | 构建约束 | 白名单 |
|---|---|---|---|
| `pkg/llm` | `golang.org/x/image/draw` | 默认构建 | ✅ |
| `pkg/llm` | `golang.org/x/image/webp` | 默认构建 | ✅ |
| `pkg/memory` | `modernc.org/sqlite` | `memory` | ✅ |
| `pkg/permission` | `github.com/google/uuid` | `sqlite` | ❌ 未登记 |
| `pkg/permission` | `modernc.org/sqlite` | `sqlite` | ✅ |
| `pkg/provider` | `golang.org/x/oauth2` | 默认构建 | ✅ |
| `pkg/session` | `github.com/golang/snappy` | `sqlite` | ✅ |
| `pkg/session` | `modernc.org/sqlite` | `sqlite` | ✅ |
