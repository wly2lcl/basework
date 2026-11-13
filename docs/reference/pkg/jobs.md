# pkg/jobs

## 用途

后台执行单元的状态、归属与输出。包只回答四个问题：这个 job 现在是什么状态、谁有权看/取消它、输出在哪读到哪、重启后记录还能不能对上。命令怎么拼、模型怎么调用属于产品层。

- **状态机** — `queued → running → succeeded / failed / canceled / timed_out / interrupted`。终态 `OnTerminal` **恰好一次**；`interrupted` 表示进程重启时无法确认结果，不会自动重放。
- **归属边界** — `Start`/`Get`/`List`/`Cancel`/`ReadOutput`/`Await` 都要求 owner（产品层传会话 ID）。owner 必填不是可选项：忘记传的调用方会拿到一个所有会话可见的 job 且不报错，这是刻意避免的默认。
- **输出** — 每个流有内存配额、总字节上限与单次读取上限；超出内存自动溢出到 0600 私有文件，`OutputRefs` 给出路径，`OutputTruncated` 明确标记丢弃。`ReadOutput` 按字节分页且不切断 UTF-8 字符。
- **进程树** — `PrepareCommand` 让子进程成为进程组/作业对象领导者，`TerminateCommand` 先温和后强杀整棵树；`ProcessTreeTerminationSupported()` 显式声明平台能力，不支持的平台退化为只杀直接子进程。
- **持久化** — `JSONLJournal` 按行追加 `started`/`terminal` 记录，与 `pkg/session` 的 JSONL 格式取向一致。`History`/`AllHistory` 只读；`ReconcileOwner` 在重启后把非终态记录改写成 `interrupted`。

## 配置

| 项 | 说明 |
|---|---|
| `Options.MaxConcurrent` | 全局并发上限，`<=0` 不限 |
| `Options.MaxPerOwner` | 单 owner 并发上限，`<=0` 不限 |
| `Options.IDPrefix` | 默认 ID 生成器前缀。默认生成器只保证进程内唯一，跨进程唯一需由产品给每次启动不同前缀 |
| `Options.Journal` | `Journal` 实现；`nil` 表示纯内存，重启后无历史 |
| `Options.WorkspaceID` | 写入记录的 workspace 归属；空表示未归属（按旧数据读取） |
| `Options.OnTerminal` | 终态回调，恰好一次，在 Manager 锁外调用，可做 I/O |
| `Options.Output.MemoryLimit` | 每流驻留内存上限，默认 `DefaultMemoryLimit`（256 KiB） |
| `Options.Output.MaxBytes` | 每流总存储上限，默认 `DefaultMaxBytes` |
| `Options.Output.MaxReadBytes` | 单次读取上限，默认 `DefaultMaxReadBytes` |
| `Options.Output.Retention` | 输出保留期，默认 `DefaultRetention`，过期返回 `ErrOutputExpired` |
| `Options.Output.Dir` | 溢出文件目录，默认用户缓存目录下 `basework/job-output` |
| `Options.Output.OpenSpill` | 自定义溢出文件打开方式，测试可注入失败 |
| `Options.Now` / `Options.NewID` | 可注入时钟与 ID 生成器，便于可复现测试 |

错误值均可用 `errors.Is` 判定：`ErrNotFound`、`ErrMissingOwner`、`ErrEmptyCommand`、`ErrNilRunner`、`ErrConcurrencyLimit`、`ErrAlreadyTerminal`、`ErrJournal`、`ErrOutputExpired`、`ErrOutputClosed`、`ErrOutputUnavailable`、`ErrBadStream`、`ErrBadOffset`、`ErrProcessTreeUnsupported`。

## 扩展点

- **持久化**：实现 `Journal`（`Append(Record)`），或用 `NewJSONLJournal(path)`；写入失败通过 `Job.JournalErr` 与 `ErrJournal` 暴露，不静默吞掉。
- **输出承接**：实现 `SpillFile` / `SpillOpener`，接入自有存储；`SinkRunner` 允许把命令输出直接写入调用方提供的 writer。
- **状态消费**：`OnTerminal` 做通知；需要阻塞等待时用 `Await`（带 context）。
- **执行函数**：`Runner` 只需返回退出码与错误；需要接管 stdout/stderr 时用 `SinkRunner`。

## Model Experience

- 模型侧工具（`internal/tools` 的 background/jobs 工具）通过 `Owner` 限定可见范围，模型只能看到当前会话的 job，不能凭 job ID 越权查询。
- 输出分页返回精确字节区间与 `NextOffset`，模型可继续读取而不重复；`Truncated` 与 `OutputRefs` 让「输出被截断」成为可见事实，而不是模型编造完整日志。
- 取消/超时进入终态后，模型再次读取得到同一终态与结果，不会看到状态回退；`interrupted` 明确告诉模型「上次进程结束时不知道结果」，避免把未完成当成功。

## Known Limitations

- 默认 ID 只保证进程内唯一；跨进程唯一依赖调用方设置不同的 `IDPrefix`，否则历史上会出现同名 `job-001`。
- 纯内存模式（`Journal == nil`）无重启历史；`JournalErr` 非空表示本次运行没有可恢复记录。
- 进程树终止依赖平台：unix 用进程组信号，Windows 用 `taskkill /T` + 后代补清理，其他平台只杀直接子进程（`ProcessTreeTerminationSupported()` 为 false）。
- 输出保留期按 `Options.Now` 计算，过期后 `ReadOutput` 返回 `ErrOutputExpired`；溢出文件由 `Close`/`CloseOwner` 或 `CleanupOutputs` 清理，进程被强杀会留下文件。
- 本包不负责重跑：`ReconcileOwner` 只改写状态，任何自动重放都会造成副作用重复，必须由人显式发起。
- 本包是**实验 API（0.x）**（见 [ADR 0008](../../adr/0008-promote-internal-capabilities-to-pkg.md)）：1.0 前允许破坏性调整，但会走 ADR 并记录迁移方式。
