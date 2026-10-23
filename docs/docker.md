# Docker 使用指南

Go 标准库随二进制编入镜像。2026-10-23 安全更新需用 Go 1.26.10 或更新安全修订版重新构建二进制并重建镜像，升级宿主 Go 或仅更换基础镜像均不能修复旧二进制。当前修复与镜像验证边界见 [SEC-002](development/evidence/SEC-002.md)；历史 `latest` 或版本 tag 不因源码修改自动更新。

## 快速开始

### 拉取镜像

```bash
docker pull ghcr.io/wly2lcl/basework:latest
```

### 运行 TUI

```bash
docker run -it ghcr.io/wly2lcl/basework
```

### 挂载配置

```bash
docker run -it -v ~/.config/basework:/root/.config/basework ghcr.io/wly2lcl/basework
```

### 运行特定命令

```bash
docker run ghcr.io/wly2lcl/basework session list
docker run ghcr.io/wly2lcl/basework version
```

## 镜像标签

- `latest`: 最新稳定版，预发布不会更新该标签
- `v1.3.0`: 特定版本

当前 GoReleaser 配置只发布完整版本标签和 `latest`，暂不发布 `v1.3` 或 `v1` 这类滚动标签。

## 数据持久化

### 挂载配置目录

```bash
docker run -it \
  -v ~/.config/basework:/root/.config/basework \
  ghcr.io/wly2lcl/basework
```

### 挂载会话数据

```bash
docker run -it \
  -v ~/.config/basework:/root/.config/basework \
  -v ~/.basework:/root/.basework \
  ghcr.io/wly2lcl/basework
```

## 环境变量

可以通过环境变量配置：

```bash
docker run -it \
  -e BASEWORK_PROVIDER=opencode \
  -e BASEWORK_MODEL=big-pickle \
  ghcr.io/wly2lcl/basework
```

## 构建本地镜像

```bash
docker build -t basework:local .
docker run -it basework:local
```

## 故障排除

### 无法连接到 TUI

确保使用 `-it` 参数：
```bash
docker run -it ghcr.io/wly2lcl/basework
```

### 配置未保存

确保正确挂载配置目录：
```bash
docker run -it -v ~/.config/basework:/root/.config/basework ghcr.io/wly2lcl/basework
```

### 权限问题

检查挂载目录的权限：
```bash
chmod -R 755 ~/.config/basework
```
