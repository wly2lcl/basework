# 快速开始

Basework 提供可嵌入的 Go Agent 核心和 CLI/TUI 终端入口。当前已有基础编码工具与会话能力；生产使用前请阅读 [当前状态](STATUS.md) 的验证边界。

## 从源码运行

要求 Go 1.26+。在仓库根目录执行：

```bash
go mod download
make build
./bin/basework --help
./bin/basework model list
```

按 [Provider 指南](guides/provider-guide.md) 配置模型、API Key 与端点，然后运行：

```bash
./bin/basework agent -m "说明当前项目的目录结构"
./bin/basework tui
```

默认配置指向 OpenCode Zen / `big-pickle`；服务端可用性、价格和免密钥能力不由本仓库保证。需要密钥的模型必须配置密钥，“免费”不能作为“无需密钥”或“支持工具调用”的依据。

建议第一次在测试项目里依次验证：读一个文件、生成一个小函数、运行对应测试、退出后恢复会话。调用成功与完成一次真实编程任务是不同的验证层次。

## 嵌入使用

从 [嵌入指南](guides/embedder-guide.md) 开始，接口与限制见 [agent 包契约](reference/pkg/agent.md)。代码只依赖 `pkg/` 的公开接口，具体权限策略与工具由宿主注入。

## 下一步

[安装渠道](installation.md) · [配置参考](guides/configuration.md) · [CLI 命令](guides/cli-guide.md) · [任务进度](TASKS.md)
