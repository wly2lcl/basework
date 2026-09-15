# 真实 Provider 验收夹具

这是 SHIP-001/QA-001 的可选真实模型入口。默认 Go 测试不会联网，也不会读取
Provider 密钥；只有显式设置以下环境变量并运行命令时才会发请求：

```bash
BASEWORK_REAL_PROVIDER=openai \
BASEWORK_REAL_BASE_URL=https://example.invalid/v1 \
BASEWORK_REAL_API_KEY="$OPENAI_API_KEY" \
BASEWORK_REAL_MODEL=gpt-4o \
BASEWORK_REAL_OUTPUT=/tmp/basework-real-provider.json \
go run ./tests/real_provider
```

运行器会把仓库内 `tests/real_provider/fixtures/fixbug` 复制到随机临时目录，在临时
项目中执行一次真实 Agent 回合，再由夹具独立运行 `go test ./...`。结果文件只保存：
Provider、模型、脱敏端点 origin、固定输入、耗时、工具名/成功状态、测试退出码和最终
文本哈希；不保存 API key、完整模型输出或临时工作目录内容。Agent 失败、独立测试
失败或文件未修改时均返回非零，并且仍写出脱敏结果，便于记录失败样本。

真实 Provider 结果不能写入默认 CI，也不能把脚本化 TUI 场景当成真实模型结果。每次
运行应将脱敏 JSON 复制到任务证据目录，并注明日期、候选 commit、Provider/model、
协议入口和失败/重试次数。

仓库提供 `.github/workflows/real-provider.yml` 作为手动触发入口。它只在明确的
`workflow_dispatch` 下运行，API key 从 `BASEWORK_REAL_API_KEY` secret 注入，结果以
artifact 上传脱敏 JSON；默认 `build.yml` 不调用外部模型。
