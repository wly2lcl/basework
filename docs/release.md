# 发布指南

本文档描述当前仓库实际支持的发布路径，避免发布入口、构建产物和安装文档漂移。

## 当前发布门槛

2026-09-14 复审发现尚未关闭的 P1/P2 问题；不能以旧看板 29/29 或检查报告已写完为由发布。先按 [TASKS](TASKS.md) 完成修复、QA-001 和 SHIP-001/002/003；[复审报告](development/evidence/REVIEW-2026-09-14.md) 保存本次依据。

## 发布产物

正式发布由 `.github/workflows/release.yml` 和 `.goreleaser.yml` 驱动，当前会生成：

- GitHub Release 页面和变更日志
- Linux amd64/arm64、macOS amd64/arm64、Windows amd64 二进制压缩包
- GHCR Docker 镜像：`ghcr.io/wly2lcl/basework:<version>`，稳定版额外更新 `ghcr.io/wly2lcl/basework:latest`

GoReleaser 构建统一启用 `sqlite memory` build tags，与 CI、Makefile 和文档命令保持一致。

## 正式发布

推荐使用 GitHub Actions 的 `Release` workflow 手动发布：

1. 打开 GitHub Actions 中的 `Release` workflow。
2. 选择 `Run workflow`。
3. 填写版本号，例如 `v0.2.1`。
4. 保持 `dry_run=false`。
5. 如需预发布，打开 `prerelease=true`。

workflow 会校验版本格式，创建并推送对应 tag，然后从该 tag 执行 GoReleaser 正式发布。预发布不会更新 Docker 的 `latest` 标签。

也可以在本地手动创建 tag 触发发布：

```bash
git tag -a v0.2.1 -m "Release v0.2.1"
git push origin v0.2.1
```

tag push 会触发同一个 `Release` workflow。

## 发布演练

如只想验证 GoReleaser 配置，不创建 tag、不发布二进制、不推送 Docker 镜像：

1. 打开 GitHub Actions 中的 `Release` workflow。
2. 选择 `Run workflow`。
3. 填写版本号。
4. 设置 `dry_run=true`。

日常 CI 的 `release-dry-run` job 也会执行 GoReleaser snapshot，用于提前发现发布配置问题。它使用 `--skip=docker,publish`，不构建/运行镜像，也不在目标平台解包执行产物；这些必须单独验收。

## 本地验证

发布前建议至少运行：

```bash
go test -tags "sqlite memory" -count=1 ./...
go vet -tags "sqlite memory" ./...
goreleaser check
goreleaser release --snapshot --clean --skip=docker,publish
```

其中 GoReleaser 命令需要本机已安装 `goreleaser`。

如需在发布前验证双架构镜像，使用仓库脚本生成临时 Docker context，不要手工在仓库根目录
留下 `linux/amd64` 或 `linux/arm64` 交叉编译目录：

```bash
BASEWORK_VERSION=0.1.4-SNAPSHOT-$(git rev-parse --short HEAD) \
  BUILDX_BUILDER=basework-builder \
  BASEWORK_DOCKER_SMOKE=1 \
  ./scripts/build_docker_candidate.sh \
  basework:candidate-$(git rev-parse --short HEAD) \
  /private/tmp/basework-candidate-$(git rev-parse --short HEAD).oci
```

脚本会用 `sqlite memory` tags 构建两个 Linux 二进制、注入版本信息、执行
`docker buildx build --platform linux/amd64,linux/arm64`，最后打印 OCI 文件 SHA-256。
设置 `BASEWORK_DOCKER_SMOKE=1` 时，脚本还会为两个架构分别执行单平台
`buildx --load`，本地标签为 `basework:candidate-<commit>-amd64` 和
`basework:candidate-<commit>-arm64`。多架构 OCI manifest 作为归档保存，不直接交给
`docker load`；目标架构的冒烟命令应使用脚本加载的单平台标签、独立可写会话目录，并把
源码工作区挂载为只读：

```bash
docker run --rm --platform linux/amd64 --read-only -w /workspace \
  -v "$PWD:/workspace:ro" \
  -v /private/tmp/basework-docker-home/config:/root/.config/basework \
  -v /private/tmp/basework-docker-home/data:/root/.local/share/basework \
  basework:candidate-<commit>-amd64 facts show
```

`--skip=docker` 的 GoReleaser dry-run 只验证归档配置，不替代这组镜像构建与运行检查。
`.github/workflows/build.yml` 的 `docker-smoke` job 会在候选 CI 中执行同一脚本，并在
QEMU 下分别加载单平台 amd64/arm64 标签，运行两个架构的 `version` 与只读 `facts show`；
多架构 OCI 文件仍作为构建产物保存。CI 通过前，不能把这段定义当成运行证据。

## 安装渠道状态

当前已接入以下发布配置；具体候选版本的构建/安装验证状态见 SHIP-002，不由配置存在性证明：

- GitHub Releases 预编译二进制
- Go install
- GHCR Docker 镜像

Homebrew tap 尚未接入当前 GoReleaser 配置；在新增 tap 自动发布前，不应把 Homebrew 作为已支持安装方式写入主安装路径。
