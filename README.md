# Basework

[![CI](https://github.com/wly2lcl/basework/actions/workflows/build.yml/badge.svg?branch=main)](https://github.com/wly2lcl/basework/actions/workflows/build.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Basework 是一个用 Go 编写的**可嵌入 AI Agent 核心 + CLI/TUI 终端助手**。它把模型调用、工具执行、会话持久化、权限检查、后台任务和终端交互放在同一套运行时里，既可以直接作为命令行工具使用，也可以嵌入其他 Go 服务。

> 根目录 README 负责快速定位；完整正文、任务状态和运行证据统一维护在 [`docs/`](docs/README.md)，避免根目录与 `docs/` 产生两套不一致的说明。

## 适合什么场景

- 在本地项目中进行代码阅读、修改、测试和后台命令执行。
- 通过 CLI 或 TUI 进行可恢复的多轮 Agent 工作。
- 在 Go 应用中注入模型、会话存储、工具和回调，组合自己的 Agent 产品。
- 接入 OpenAI-compatible、Responses、Anthropic、Gemini 以及其他已实现的 Provider，并配置自定义网关端点。

## 当前能力

| 层次 | 已提供的能力 | 入口 |
|---|---|---|
| Agent 运行时 | 流式输出、工具调用、循环/步数限制、回调、子代理 | [`pkg/agent`](docs/reference/pkg/agent.md) |
| 内置工具 | 文件读取与搜索、编辑预览/提交/撤销、前台/后台 Bash、LSP、MCP | [工具与扩展](docs/guides/extending.md) |
| 会话与恢复 | JSONL 会话事件、历史查询、`--session` 恢复、迁移和压缩快照 | [迁移指南](docs/guides/migration.md) · [会话契约](docs/reference/pkg/session.md) |
| 终端入口 | 一次性 `agent -m`、交互式 REPL、Bubble Tea TUI、任务和审批状态 | [CLI 指南](docs/guides/cli-guide.md) · [TUI 指南](docs/guides/tui-guide.md) |
| Provider | 环境变量、模型能力说明、自定义 `base_url`、请求审计 | [Provider 指南](docs/guides/provider-guide.md) · [配置参考](docs/guides/configuration.md) |
| 安全边界 | readonly/coding 预设、权限规则、敏感路径保护、脱敏配置解释 | [权限指南](docs/guides/permission-guide.md) · [安全边界](docs/guides/security.md) |

项目级负载门禁已经覆盖同进程 1/4/8/16/32 并发、取消后重启、JSONL 多进程恢复和 10,000 次 soak；完整数据见 [LOAD-002 证据](docs/development/evidence/LOAD-002.md)。该证据使用确定性本地模型，验证的是 Basework 自身的运行时稳定性，不代表任何真实 Provider 的配额、模型质量或生产 SLO。

## 五分钟从源码开始

要求 Go 1.26 或更高版本：

```bash
git clone https://github.com/wly2lcl/basework.git
cd basework
go mod download
make build
./bin/basework version
./bin/basework --help
```

配置一个你有权限使用的 Provider。下面以 OpenAI-compatible 配置为例，密钥只通过环境变量提供：

```bash
export OPENAI_API_KEY="<your-api-key>"
./bin/basework init --yes \
  --provider openai \
  --model gpt-4o-mini \
  --preset readonly

./bin/basework config explain
./bin/basework model list
./bin/basework agent -m "概览当前项目的目录结构，并指出测试入口"
```

需要编辑文件或运行命令时，可以显式使用 coding 预设；需要连续交互时启动 TUI：

```bash
./bin/basework tui --preset coding
./bin/basework session list
./bin/basework tui --session <session-id>
```

`config explain` 只显示脱敏后的生效配置。Provider 的可用性、价格、速率限制和工具兼容性取决于实际端点，不能仅凭静态模型列表推断；完整配置方式见 [Provider 指南](docs/guides/provider-guide.md)。

## 作为 Go 库嵌入

公共 API 位于 `pkg/`，产品专用运行时位于 `internal/`。从 [嵌入指南](docs/guides/embedder-guide.md) 开始，或直接查看可运行的 [`examples/embed`](examples/embed/main.go)：

```go
model, err := provider.Create(provider.Config{
	Type:    "openai",
	ModelID: "gpt-4o-mini",
	APIKey:  os.Getenv("OPENAI_API_KEY"),
})
if err != nil {
	return err
}

store, err := session.NewJSONLStore("./sessions")
if err != nil {
	return err
}
agt, err := agent.New(
	agent.WithModel(model),
	agent.WithSession(store),
)
```

嵌入时只依赖 `pkg/` 的公开契约；如果需要替换工具、权限、事件或存储实现，请先阅读对应的 [包契约索引](docs/reference/README.md)。

## 开发与验证

```bash
# 默认项目测试（含 sqlite/memory 可选模块）
make test

# 文档、任务、相对链接和证据结构
make check-docs

# 刷新依赖图和代码统计；CI 会检查生成物是否新鲜
make gen

# 项目级短负载门禁
go test ./tests/load -run TestProjectLoad -count=1 -timeout 10m -v
```

提交代码前请阅读 [贡献指南](docs/CONTRIBUTING.md) 和 [验证规则](docs/development/validation.md)。AI 接续开发应按 [AI 开发流程](docs/development/ai-workflow.md) 一次领取一个任务，并把结果写回 [任务看板](docs/TASKS.md)。

## 文档导航

| 目标 | 文档 |
|---|---|
| 安装、首次运行、Docker | [安装](docs/installation.md) · [快速开始](docs/getting-started.md) · [Docker](docs/docker.md) |
| CLI、TUI、配置、Provider | [CLI](docs/guides/cli-guide.md) · [TUI](docs/guides/tui-guide.md) · [配置](docs/guides/configuration.md) · [Provider](docs/guides/provider-guide.md) |
| 嵌入、扩展、MCP/LSP、子代理 | [嵌入](docs/guides/embedder-guide.md) · [扩展](docs/guides/extending.md) · [子代理](docs/guides/subagent-guide.md) |
| 当前能力和限制 | [当前状态](docs/STATUS.md) · [架构](docs/ARCHITECTURE.md) · [FAQ](docs/FAQ.md) |
| 后续规划和开发进度 | [路线图](docs/ROADMAP.md) · [任务看板](docs/TASKS.md) · [AI 开发流程](docs/development/ai-workflow.md) |
| 发布、贡献和安全 | [发布流程](docs/release.md) · [贡献指南](docs/CONTRIBUTING.md) · [安全策略](docs/SECURITY.md) |

## 边界说明

- 权限规则是应用层检查，不是操作系统沙箱；Bash 仍在宿主机执行。需要隔离时，应在外部容器或沙箱中运行。
- 本地确定性 Provider 的负载结果不代表外部模型渠道的限流、计费、可用性或回答质量。
- 发布候选、真实 Provider 验收和真人 TUI 手感分别记录在 `docs/development/evidence/`，不能互相替代。
- 版本以 Git tag 和 [`docs/CHANGELOG.md`](docs/CHANGELOG.md) 为准；当前状态以 [`docs/STATUS.md`](docs/STATUS.md) 和 [`docs/TASKS.md`](docs/TASKS.md) 为准。

许可证：[MIT](LICENSE)。
