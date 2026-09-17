# 安装指南

## 验证范围

打包配置覆盖 Linux amd64/arm64、macOS amd64/arm64 和 Windows amd64（Windows arm64 按配置明确排除）。当前候选的五平台发布归档 Smoke、Docker Smoke 和迁移/未来版本拒绝检查已有 CI 记录；本机实际安装能力仍取决于所运行的平台。下载时核对 tag、架构与校验和，完整边界见 [当前状态](STATUS.md) 与 [SHIP-002 证据](development/evidence/SHIP-002.md)。

## 安装方式

### 方式 1: 下载二进制文件

从 [GitHub Releases](https://github.com/wly2lcl/basework/releases) 下载对应平台的二进制文件。

支持平台：
- Linux (amd64, arm64)
- macOS (amd64, arm64)
- Windows (amd64)

### 方式 2: Go Install

```bash
go install -tags "sqlite memory" github.com/wly2lcl/basework/cmd/basework@latest
```

### 方式 3: Docker

```bash
docker pull ghcr.io/wly2lcl/basework:latest
docker run -it -v ~/.config/basework:/root/.config/basework ghcr.io/wly2lcl/basework
```

> Homebrew tap 尚未接入当前 GoReleaser 配置；新增 tap 自动发布前，请使用 GitHub Releases、Go install 或 Docker。

## 验证安装

```bash
basework version
```

应显示版本信息。

## 配置

首次运行时，basework 会在 `~/.config/basework/` 创建配置文件。
默认 Provider 是 OpenCode Zen，默认模型是 `big-pickle`。如需显式配置 API Key，可设置 `OPENCODE_API_KEY`，旧环境变量名 `OG_API_KEY` 也兼容。

编辑 `config.json` 配置 Provider、模型等：

```json
{
  "provider": "opencode",
  "model": "big-pickle"
}
```

## 升级

### Go Install
```bash
go install -tags "sqlite memory" github.com/wly2lcl/basework/cmd/basework@latest
```

### Docker
```bash
docker pull ghcr.io/wly2lcl/basework:latest
```

## 卸载

### Docker
```bash
docker rmi ghcr.io/wly2lcl/basework:latest
```

## 发布流程

正式发布由 GitHub Actions `Release` workflow 和 GoReleaser 驱动，会生成 GitHub Release 二进制包和 GHCR Docker 镜像。维护者操作步骤见 [发布指南](release.md)。

## 故障排除

### SQLite 相关错误

确保使用正确的 build tags 编译（完整迁移、权限和版本命令需要该构建）：
```bash
go build -tags "sqlite memory" ./cmd/basework
```

### 文件锁问题

如果会话被锁定，使用（需要 `sqlite` build tag）：
```bash
basework session unlock <session-id>
```

### 数据库损坏

检查 SQLite 会话完整性（需要 `sqlite` build tag）：
```bash
basework session status --check-integrity
```

回滚最近一次 WAL 模式迁移：
```bash
basework migrate rollback

该命令恢复 WAL 迁移备份，不会把 SQLite 数据导回 JSONL；当前 Agent/TUI 主运行时仍固定使用 JSONL。
```
