# 真实 Provider 验收夹具

这是 SHIP-001/QA-001 的可选真实模型入口。默认 Go 测试不会联网，也不会读取
Provider 密钥；只有显式设置以下环境变量并运行命令时才会发请求：

```bash
AGNES_API_KEY="$AGNES_API_KEY" \
BASEWORK_REAL_PROVIDER=agnes-responses \
BASEWORK_REAL_BASE_URL=https://apihub.agnes-ai.com/v1 \
BASEWORK_REAL_MODEL=agnes-3.0-flash \
BASEWORK_REAL_OUTPUT=/tmp/basework-real-provider.json \
go run ./tests/real_provider
```

运行器会把仓库内 `tests/real_provider/fixtures/fixbug` 复制到可信临时目录，再复制一份
给模型工作；模型工作区之外保留原始测试和 `go.mod`。回合结束后只把允许修改的实现
文件带入第二个验证目录，拒绝测试/模块配置被改写、删除或新增越界文件，并记录原始测试
哈希。独立验证命令固定为
`go test -count=1 -run '^TestAdd$' ./...`：禁用缓存，确认目标测试实际执行，且使用
单独 60 秒 context、进程树回收和 64 KiB 有界输出。结果文件保存验证状态、测试哈希、
实现文件哈希、测试退出码和输出尾部；不保存 API key、完整模型输出或临时工作目录内容。
Agent 失败、验证门禁失败、测试未执行、独立测试失败或文件未修改时均返回非零，并且仍
写出脱敏结果，便于记录失败样本。

真实 Agent 使用 `builtin.Runtime.Environment` 的显式安全白名单和临时 HOME/TMP 运行 Bash；
独立测试同样使用隔离环境。白名单保留运行 shell/Go 所需的路径、缓存、系统变量和去掉
userinfo 的代理地址，拒绝宿主机 `GOFLAGS`、任意 Provider key 及其他未声明变量；并将
`GOCACHE`、`GOMODCACHE`、`GOPATH` 固定在隔离 HOME 下，避免 Windows 因隔离 HOME 缺少
`LocalAppData` 而无法定位 Go build cache。代理地址同时去掉 userinfo、query 和 fragment；
runner 会统一清洗已知 Provider key 的工具错误、Agent 错误、验证输出、结果 JSON 和最终
stderr。这个过滤保证只
属于验收运行器，不把 Bash 权限规则误写成 OS 沙箱。

真实 Provider 结果不能写入默认 CI，也不能把脚本化 TUI 场景当成真实模型结果。每次
运行应将脱敏 JSON 复制到任务证据目录，并注明日期、候选 commit、Provider/model、
协议入口和失败/重试次数。

仓库提供 `.github/workflows/real-provider.yml` 作为手动触发入口。它只在明确的
`workflow_dispatch` 下运行，API key 从 `AGNES_API_KEY` secret 注入，结果以
artifact 上传脱敏 JSON；默认 `build.yml` 不调用外部模型。

## GitHub Actions 运行方式

密钥只添加到仓库的 Actions secret，不要写入仓库、命令历史或聊天记录。Agnes 验收统一
使用 `AGNES_API_KEY`；运行器也兼容旧的 `BASEWORK_REAL_API_KEY` 本地变量，但不会把任一
变量传入模型工具的 Bash 环境。已安装并登录 GitHub CLI 时，可在本地交互设置：

```bash
gh secret set AGNES_API_KEY --repo wly2lcl/basework
```

然后按实际端点手动触发工作流；`provider`、`base_url` 和 `model` 必须与这次验收使用的
协议入口和模型一致。`repetitions=3` 会在同一 workflow、同一候选 commit 和同一组参数下
顺序运行三次，并为每次运行保存独立脱敏 JSON；`repetitions=1` 只适合单次探针。每种协议
在同一候选 commit 上至少连续运行 3 次；当前 Responses 三次结果见 [RESP-001](evidence/RESP-001.md)；表格还要标明
核心 API、CLI 或 TUI 入口，不能把核心 API 结果写成 CLI/TUI 结果：

```bash
gh workflow run real-provider.yml --repo wly2lcl/basework \
  -f provider=openai \
  -f base_url=https://apihub.agnes-ai.com/v1 \
  -f model=agnes-3.0-flash \
  -f repetitions=3

gh workflow run real-provider.yml --repo wly2lcl/basework \
  -f provider=agnes-responses \
  -f base_url=https://apihub.agnes-ai.com/v1 \
  -f model=agnes-3.0-flash \
  -f repetitions=3

gh workflow run real-provider.yml --repo wly2lcl/basework \
  -f provider=anthropic \
  -f base_url=https://apihub.agnes-ai.com \
  -f model=agnes-3.0-flash \
  -f repetitions=3
```

触发后可用 `gh run list --repo wly2lcl/basework --workflow real-provider.yml` 找到运行，
下载其中的 `real-provider-result-<run_id>` artifact，其中包含每次重复的独立 JSON；并将脱敏
JSON 与候选 commit、日期、协议入口和重复次数一起回填到 SHIP-001/QA-001。结果中的
`validation_ok=true`、
`tests_executed=true`、`file_changed=true`、`independent_test_exit_code=0` 和测试哈希
共同构成可信通过条件；只看 Agent 成功或退出码 0 不足以通过。若 secret、端点或模型
缺失，工作流必须保持失败，不能用脚本化 Provider 结果替代真实请求。

Agnes 3.0 Flash 官方文档同时列出 Chat Completions、Responses 和 Anthropic Messages
三种接口。本项目的 `openai`、`agnes-responses`、`anthropic` 分别覆盖三种协议；Responses
适配器使用 `input`/`function_call`/Responses SSE 事件，并已通过本地 fake 协议回归与一次真实
Agnes Agent 闭环。发布前的三次连续真实证据仍按本文件的 workflow 规则执行，不能用一次本地
或真实成功推广成稳定成功率。
