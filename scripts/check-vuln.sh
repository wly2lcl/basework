#!/usr/bin/env bash
set -euo pipefail

# 固定官方扫描器版本，避免同一次发布因 latest 漂移使用不同工具。
# 使用 Go 的模块与构建缓存复用扫描器，不依赖 PATH 中另装的 govulncheck。
scanner='golang.org/x/vuln/cmd/govulncheck@v1.8.0'
mode="${1-source}"
if (( $# > 0 )); then
  shift
fi

scan() (
  # 交叉构建的 post hook 也必须在宿主机上运行扫描器。
  unset GOOS GOARCH GOVERSION
  # 不继承可跳过执行的 -exec、改变源码范围的 -tags 或外部 workspace；
  # 固定官方漏洞库，禁止 GOVULNDB 把检查替换为本地空库。
  GOFLAGS='' GOENV=off GOWORK=off go run "$scanner" -db=https://vuln.go.dev "$@"
)

case "$mode" in
  source)
    if (( $# != 0 )); then
      echo 'source 模式不接受额外参数，固定检查整个模块。' >&2
      exit 2
    fi
    echo "漏洞检查：默认构建与测试源码，扫描器 ${scanner}"
    scan -test ./...
    echo '漏洞检查：sqlite,memory 构建与测试源码'
    scan -test -tags=sqlite,memory ./...
    ;;
  binary)
    if (( $# == 0 )); then
      echo 'binary 模式必须提供至少一个明确的二进制文件路径。' >&2
      exit 2
    fi
    # 先校验全部输入，禁止空产物列表或错误路径被当成检查成功。
    for artifact in "$@"; do
      if [[ ! -f "$artifact" || ! -s "$artifact" ]]; then
        echo "二进制产物不存在、不是普通文件或为空：${artifact}" >&2
        exit 2
      fi
    done
    for artifact in "$@"; do
      echo "漏洞检查：二进制 ${artifact}"
      # 保留文本模式的失败退出码。JSON/SARIF 模式发现漏洞仍可能退出 0，
      # 不能直接用于安全门禁；工具执行、数据库访问失败同样必须阻断。
      scan -mode=binary -- "$artifact"
    done
    ;;
  *)
    echo '用法：scripts/check-vuln.sh [source | binary <文件> ...]' >&2
    exit 2
    ;;
esac
