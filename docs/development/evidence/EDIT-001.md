# EDIT-001 验证记录：统一编辑预览和冲突契约

## 基线

- 起点 HEAD：`935fde1`；工作树含 BASE-001 / REL-001 / REL-002 / REL-004 / JOB-001 的未提交改动。
- 环境：macOS 26.6.2 arm64 / go1.26.0。
- 写入口核对结果（全部为"检查 → 读 → 改 → 写"一条路走到尾，没有任何一步交付给人看）：
  - `pkg/tool/builtin/edit.go`：路径检查 → `os.ReadFile` → 唯一性检查（0 处/多处都报错）
    → `strings.Replace` → `os.WriteFile`。**没有预览**，也没有记录变更前的内容或哈希。
  - `pkg/tool/builtin/write.go`：路径检查 → `MkdirAll` → 写 `.tmp` → `Rename`。
    同为直接落盘，且不检查目标原有内容。
  - 两处都只调用 `getPathChecker().CheckPath(path)`，只检查**调用方给的路径**。
    对"路径本身在工作区内、但它是 /etc/hosts 的软链"这种情况，词法检查完全无效。
  - 不存在任何基线哈希、operation ID、diff 或换行/BOM 处理的实现。
- 结论：本任务是新建产品层能力，不是修补既有预览逻辑。

## 变更

新增 `internal/edits`（只做计划，不做提交）：

- **`Operation`**：`ID` / `RequestedPath` / `RelPath` / `AbsPath` / `BaseHash` /
  `ResultHash` / `Old` / `New` / `NewContent` / `Diff` / `Newline` / `HadBOM` /
  `NormalizedSearch`。所有字段都由磁盘真实字节推导，没有一个是模型声称的。
- **operation ID 是派生的，不是随机的**：由 `相对路径 | 基线哈希 | old 的哈希 | new 的哈希`
  取 sha256 前 16 位。同一份计划重复生成必须得到同一个 ID，否则重试会产生"看起来是新操作"
  的重复条目。
- **`Planner.ResolvePath` 三道检查**：
  1. 词法包含（挡 `../`）；
  2. 符号链接解析后的包含（挡"工作区内的软链指向区外"——词法检查对此完全无效）；
  3. 产品层 `CheckPath` 对**原始路径与解析后路径各做一次**，否则软链能把受保护路径
     藏在合法路径后面。
- **`Preview` 全程不写盘**：只做 `Lstat` / `EvalSymlinks` / `ReadFile` 与纯计算。
- **唯一性口径与同步 edit 一致**：0 处 `ErrNoMatch`、多处 `ErrAmbiguousMatch`，
  不挑第一处改掉。
- **编码与换行**：
  - 换行风格 `lf` / `crlf` 保留，替换文本按文件风格归一化，避免一次替换把整个文件换行改掉；
  - 调用方用 LF 描述 CRLF 文件时能匹配，但会置 `NormalizedSearch=true`——
    这是隐式改写，必须让人知道；
  - UTF-8 BOM 保留，且 `BaseHash` 基于**含 BOM 的原始字节**，否则冲突校验会误判；
  - 无效 UTF-8 或含 NUL 的内容返回 `ErrBinaryContent`，不做"当文本改"这种静默损坏。
- **`Operation.ValidateBase(current)`**：只用内容 sha256 比对，刻意不看文件时间——
  `cp -p` / `touch` / `git checkout` 会改时间不改内容，编辑器保存也会在内容未变时改时间。
- **diff 是围绕替换点的单 hunk 上下文 diff**（`ContextLines` 默认 3），不是通用最小编辑
  距离 diff。之所以不假装是通用 diff：本次计划就是一处连续替换，包装成多 hunk 会让人
  误以为工具做了更复杂的优化。
- 明确错误：`ErrOutsideWorkspace` / `ErrSymlinkEscape` / `ErrPermissionDenied` /
  `ErrNoMatch` / `ErrAmbiguousMatch` / `ErrEmptySearch` / `ErrBinaryContent` /
  `ErrBaseConflict`。
- **`pkg/tool` 与 `pkg/tool/builtin` 均未改动**（`git status --short pkg/tool/` 为空）。

## 验证

任务卡指定验证（仓库根执行）：

```
go test ./pkg/tool/builtin ./internal/tools -count=1
→ ok github.com/wly2lcl/basework/pkg/tool/builtin 1.521s
→ ok github.com/wly2lcl/basework/internal/tools   5.195s

go test -race ./internal/edits -count=1
→ ok github.com/wly2lcl/basework/internal/edits    1.470s
```

`internal/edits/plan_test.go`（14 条）：

| 测试 | 覆盖的验收点 |
|---|---|
| `TestPreview_DoesNotWriteAnything` | **预览不会改文件**：遍历整个工作区比对文件集合与内容，并断言 mtime 未变 |
| `TestPreview_BaselineHashAndStableOperationID` | 基线/结果哈希为真实内容的 sha256；同一计划 ID 稳定，不同替换 ID 不同 |
| `TestResolvePath_RejectsTraversal` | `../outside.txt`、`sub/../../outside.txt` 均 `ErrOutsideWorkspace` |
| `TestResolvePath_RejectsSymlinkEscape` | 工作区内的软链指向区外 → `ErrSymlinkEscape` |
| `TestResolvePath_AllowsSymlinkInsideWorkspace` | 区内软链可用；`RequestedPath` 保留原样，`RelPath` 指向真实文件 |
| `TestPreview_PathCannotEscapeThroughSubdirSymlink` | 父目录是软链时的逃逸同样被挡 |
| `TestPreview_PermissionCheckAppliesToEveryTarget` | **所有目标都经过权限检查**：拒绝时 `ErrPermissionDenied` 且带原因 |
| `TestPreview_RejectsAmbiguousAndMissingMatch` | 唯一性口径：多处/无匹配/空搜索各自明确报错 |
| `TestPreview_PreservesCRLF` | CRLF 保留，变更后无孤立 LF，内容逐字节正确 |
| `TestPreview_NormalizesSearchNewlines` | 归一化匹配被显式标记 |
| `TestPreview_PreservesBOM` | BOM 保留；`BaseHash` 基于含 BOM 字节 |
| `TestPreview_RejectsBinaryContent` | 无效 UTF-8 与含 NUL 均 `ErrBinaryContent` |
| `TestValidateBase_UsesContentNotModTime` | **基线用于冲突验证且不只依赖时间**：仅改 mtime → 通过；内容变 → `ErrBaseConflict` 且错误带两个哈希 |
| `TestPreview_DiffIsScopedToOneHunk` | 20 行文件中 diff 只有 1 增 1 删 + ≤6 行上下文，不含整个文件 |

全量门禁：见 `## 结论`。

## 剩余与交接

- **只预览不提交**：`Planner` 没有 `Apply`。批量提交、同文件多处与多文件计划、
  逐文件原子性、跨文件失败补偿、撤销，全部属于 EDIT-002。
- **未接入 CLI/工具表**：`internal/edits` 目前无人调用。把预览接到批准流程与
  `cmd/basework` 的展示属 EDIT-003。
- **diff 不是通用 diff**：只有单 hunk 上下文形式，没有行号与 hunk 头
  （`@@ -a,b +c,d @@`）。EDIT-003 要在终端展示时补上行号。
- **软链场景未在 Windows 上验证**：`os.Symlink` 在 Windows 需要特权，
  相关两条测试会 `t.Skip`。当前只在 macOS 上确认通过。
- 二进制判定基于"无效 UTF-8 或含 NUL"，不是完整的二进制嗅探；
  UTF-8 编码的二进制（如某些图片格式）不会被拦下。这是保守方向的选择：
  宁可漏判也不误拒合法文本。

## 结论

验收项逐条对照：

| 验收条件 | 结果 | 依据 |
|---|---|---|
| 预览不会改文件 | 通过 | `Preview` 只做 Lstat/EvalSymlinks/ReadFile + 纯计算；`TestPreview_DoesNotWriteAnything` 比对整个工作区的文件集合、内容与 mtime |
| 所有目标都经过权限与路径检查 | 通过 | `ResolvePath` 三道检查（词法包含 / 软链解析后包含 / CheckPath 对原始与解析路径各一次）；`TestPreview_PermissionCheckAppliesToEveryTarget`、`TestResolvePath_RejectsSymlinkEscape`、`TestPreview_PathCannotEscapeThroughSubdirSymlink` |
| 基线能用于提交前冲突验证，不能只依赖文件时间 | 通过 | `BaseHash` 为变更前原始字节 sha256；`ValidateBase` 只比对内容；`TestValidateBase_UsesContentNotModTime` 显式证明"仅改时间不判冲突、内容变才判冲突" |
| 覆盖符号链接、路径穿越和编码/换行 | 通过 | 软链逃逸（2 条）、路径穿越、CRLF 保留、LF→CRLF 归一化标记、BOM 保留、二进制拒绝 |

针对性验证（含 `-race`）通过；14 条用例覆盖全部验收条件。`pkg/tool` 未改动。
