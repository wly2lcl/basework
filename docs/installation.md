# 安装指南

## 安装方式

### 方式 1: Homebrew (macOS/Linux)

```bash
brew tap wly2lcl/tap
brew install basework
```

### 方式 2: Go Install

```bash
go install -tags "sqlite memory" github.com/wly2lcl/basework/cmd/basework@latest
```

### 方式 3: Docker

```bash
docker pull ghcr.io/wly2lcl/basework:latest
docker run -it -v ~/.config/basework:/root/.config/basework ghcr.io/wly2lcl/basework
```

### 方式 4: 下载二进制文件

从 [GitHub Releases](https://github.com/wly2lcl/basework/releases) 下载对应平台的二进制文件。

支持平台：
- Linux (amd64, arm64)
- macOS (amd64, arm64)
- Windows (amd64)

## 验证安装

```bash
basework version
```

应显示版本信息。

## 配置

首次运行时，basework 会在 `~/.config/basework/` 创建配置文件。

编辑 `config.json` 配置 Provider、模型等：

```json
{
  "provider": "opencode",
  "model": "big-pickle",
  "session": {
    "store": "sqlite",
    "sqlite_path": "~/.basework/sessions/sessions.db"
  },
  "database": {
    "mode": "wal"
  }
}
```

## 升级

### Homebrew
```bash
brew upgrade basework
```

### Go Install
```bash
go install -tags "sqlite memory" github.com/wly2lcl/basework/cmd/basework@latest
```

### Docker
```bash
docker pull ghcr.io/wly2lcl/basework:latest
```

## 卸载

### Homebrew
```bash
brew uninstall basework
```

### Docker
```bash
docker rmi ghcr.io/wly2lcl/basework:latest
```

## 故障排除

### SQLite 相关错误

确保使用正确的 build tags 编译：
```bash
go build -tags "sqlite memory" ./cmd/basework
```

### 文件锁问题

如果会话被锁定，使用：
```bash
basework session unlock <session-id>
```

### 数据库损坏

检查完整性：
```bash
basework session status --check-integrity
```

回滚到之前的状态：
```bash
basework migrate rollback
```
