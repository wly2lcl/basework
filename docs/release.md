# 发布指南

本文档描述当前仓库实际支持的发布路径，避免发布入口、构建产物和安装文档漂移。

## 当前发布门槛

安全基线当前为 [SEC-003](development/evidence/SEC-003.md)（Go 1.26.11、`x/net v0.62.0`，CI run `43820106813` 全绿）；首轮 [SEC-002](development/evidence/SEC-002.md) 记录保留。正式 Release 产物与镜像仍须按本流程用当前工具链重建并做产物级扫描，源码扫描通过不代表旧产物已修复。此前 QA-001、SHIP-001/002/003 的完成记录保留，但不能代替后来披露漏洞的修复与重新扫描。

构建至少使用 Go **1.26.11 或更新的安全修订版**；默认/完整 tags 源码和本轮实际二进制均须通过漏洞扫描。发布候选还须通过 `make check-api`（[ADR 0009](adr/0009-apidiff-api-compatibility-gate.md)）：稳定层 `pkg` 相对上一发布 tag 不得有未走迁移流程的不兼容变更；tag ↔ CHANGELOG 条目 ↔ 候选记录三链路一致后才可打 tag。修改 `go.mod`、升级开发机 Go 或通过旧 CI，都不会更新已经发布的二进制与镜像。新发布应记录候选 commit、实际 Go 版本、扫描器、漏洞库时间、tags 与产物哈希；历史 tag 不重写。

## 发布产物

正式发布由 `.github/workflows/release.yml` 和 `.goreleaser.yml` 驱动，当前会生成：

- GitHub Release 页面和变更日志
- Linux amd64/arm64、macOS amd64/arm64、Windows amd64 二进制压缩包
- GHCR Docker 镜像：`ghcr.io/wly2lcl/basework:<version>`，稳定版额外更新 `ghcr.io/wly2lcl/basework:latest`

GoReleaser 构建统一启用 `sqlite memory` build tags，与 CI、Makefile 和文档命令保持一致。

## 发布前检查项（三链路）

打 tag 前逐项核对（API-003 固化）：

- [ ] `make check-api` 通过：稳定层相对上一 tag 无未走迁移流程的不兼容变更。
- [ ] CHANGELOG 已为本版本建立正式 `[x.y.z]` 条目（Unreleased 归档），不再使用开发期内部编号。
- [ ] 本文件候选记录写明候选 commit、实际 Go 版本、扫描器与漏洞库时间、tags、产物哈希。
- [ ] `git tag` ↔ CHANGELOG 条目 ↔ 候选记录三者版本号与内容一致。
- [ ] 产物级漏洞扫描与五平台/Docker Smoke 证据绑定当前候选。

## 候选版本记录

### v0.2.0（2026-11-16 发布）

- 候选 commit：`98eeab8`（可执行代码候选）；tag `v0.2.0` 指向发布提交 `d907dde`（仅证据/文档对齐）；实际构建 Go 1.26.11；tags `sqlite memory`。
- 扫描器：govulncheck v1.8.0，官方漏洞库 https://vuln.go.dev（2026-11-16 在线查询）；源码双 tags 与五平台二进制扫描均无告警；发布后下载的正式 Darwin_arm64 二进制复扫亦无告警。
- 门禁：CI run [43829471517](https://github.com/wly2lcl/basework/actions/runs/43829471517) 13/13 全绿（含 API 兼容门禁）；`make check-api` 相对 `v0.1.6` 稳定层零不兼容；发布提交 CI run [43843500594](https://github.com/wly2lcl/basework/actions/runs/43843500594) 13/13 全绿。
- 真实 Provider：SHIP-001 口径 OpenAI Chat + Anthropic（`agnes-3.0-flash`）各连续 3 次通过并绑定候选；Responses 渠道 8×400 为外部边界，本版本不宣称已验证 Responses。
- 正式发布：Release run [43843505146](https://github.com/wly2lcl/basework/actions/runs/43843505146) 成功；GitHub Release 含五平台归档与 `basework_0.2.0_checksums.txt`；GHCR `ghcr.io/wly2lcl/basework:v0.2.0` 与 `latest` 同 digest（linux/amd64 `sha256:7f011179…`、linux/arm64 `sha256:ab57d47d…`）。
- 发布后验证：Releases 归档隔离 HOME 冒烟链（version/config explain/init/v1 迁移源哈希不变/v99 拒绝）、`go install -tags 'sqlite memory' …@v0.2.0`、GHCR 双架构只读运行冒烟均通过。
- 官方归档哈希、镜像 digest 与完整证据见 [API-004](development/evidence/API-004.md)。
- 发布授权：用户 2026-11-16 明确指令“按两协议口径发布 + 授权完整发布”（tag、GitHub Release、GHCR 双架构镜像）。

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
make check-vuln
goreleaser check
goreleaser release --snapshot --clean --skip=docker,publish
```

其中 GoReleaser 命令需要本机已安装 `goreleaser`。

源码扫描覆盖默认与完整 tags 的源码（含测试），不能替代生成产物的检查。GoReleaser 的源码/产物 hooks 在归档、镜像和发布前扫描，扫描失败不继续发布。手动复核明确的产物路径可运行：

```bash
bash scripts/check-vuln.sh binary ./bin/basework
```

同时核对 `go version -m` 的工具链、模块版本和 SHA-256，完整记录见 [SEC-002](development/evidence/SEC-002.md)。需要验证多个产物时，向 `binary` 模式传入每个明确文件路径；目录或未匹配的通配符不能当作扫描成功。

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
