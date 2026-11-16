#!/usr/bin/env bash
set -euo pipefail

# API 兼容门禁（ADR 0009）：对比最近发布 tag 与当前工作树的 pkg 导出面。
# apidiff 对不兼容变更仍退出 0，必须解析输出自行失败；工具/基线错误同样阻断。
apidiff_tool='golang.org/x/exp/cmd/apidiff@v0.0.0-20261113113407-e3970619a1a4'

baseline="${1:-}"
if [[ -z "$baseline" ]]; then
  baseline="$(git tag -l 'v*' | sort -V | tail -1)"
fi
if [[ -z "$baseline" ]] || ! git rev-parse -q --verify "refs/tags/$baseline" >/dev/null; then
  echo 'API 检查：无法解析基线 tag；请显式传入 tag 或先创建发布 tag。' >&2
  exit 2
fi

repo_root="$(pwd)"
worktree_parent="$(mktemp -d "${TMPDIR:-/tmp}/basework-api-base.XXXXXX")"
worktree="$worktree_parent/base"
git worktree add --detach "$worktree" "$baseline" >/dev/null 2>&1
cleanup() {
  git worktree remove --force "$worktree" >/dev/null 2>&1 || true
  rm -rf "$worktree_parent"
}
trap cleanup EXIT

export GOWORK=off
unset GOOS GOARCH GOVERSION || true

# 预装固定版本工具二进制：go run pkg@version 的模块解析依赖使用者 GOPROXY，
# 不能用 GOENV=off 丢掉它；对比阶段只控制 GOFLAGS/GOWORK。
GOBIN="$worktree_parent/bin" go install "$apidiff_tool"
apidiff="$worktree_parent/bin/apidiff"

module="$(go list -m)"
packages="$(go list ./pkg/... | sed "s|^$module/||")"
baseline_packages="$(cd "$worktree" && go list ./pkg/... | sed "s|^$module/||")"

tier_of() {
  case "$1" in
    pkg/jobs|pkg/edits|pkg/permission|pkg/observability) echo experimental ;;
    pkg/llm|pkg/tool|pkg/tool/builtin|pkg/session|pkg/agent) echo core ;;
    *) echo extension ;;
  esac
}

stable_failures=''
experimental_warnings=''
new_packages=''
tool_errors=''

for tags in '' 'sqlite,memory'; do
  goflags=''
  label='默认'
  if [[ -n "$tags" ]]; then
    goflags="-tags=$tags"
    label="$tags"
  fi
  echo "API 检查：${label}导出面，基线 ${baseline}，工具 ${apidiff_tool}"
  while read -r pkg; do
    [[ -z "$pkg" ]] && continue
    if ! printf '%s\n' "$baseline_packages" | grep -qx "$pkg"; then
      if [[ -z "$tags" ]]; then
        new_packages="${new_packages}${pkg}"$'\n'
      fi
      continue
    fi
    tier="$(tier_of "$pkg")"
    export_file="$worktree_parent/$(printf '%s' "$pkg" | tr '/' '_').${tags:-default}.export"
    if ! (cd "$worktree" && GOFLAGS="$goflags" "$apidiff" -w "$export_file" "$module/$pkg" >/dev/null 2>&1); then
      tool_errors="${tool_errors}${pkg} (${label}): 基线导出快照生成失败"$'\n'
      continue
    fi
    if ! output="$(cd "$repo_root" && GOFLAGS="$goflags" "$apidiff" "$export_file" "$module/$pkg" 2>&1)"; then
      tool_errors="${tool_errors}${pkg} (${label}): apidiff 执行失败"$'\n'
      continue
    fi
    if printf '%s\n' "$output" | grep -q '^Compatible changes:'; then
      echo "兼容变更（不阻断）：${pkg} (${label})"
      printf '%s\n' "$output" | sed 's/^/  /'
    fi
    if printf '%s\n' "$output" | grep -q '^Incompatible changes:'; then
      if [[ "$tier" == experimental ]]; then
        experimental_warnings="${experimental_warnings}${pkg} (${label})：实验 API 破坏性变更，须按 ADR 0008 走 ADR + CHANGELOG 迁移说明"$'\n'
      else
        stable_failures="${stable_failures}${pkg} (${label}, ${tier})："$'\n'"${output}"$'\n'
      fi
    fi
  done <<< "$packages"
done

if [[ -n "$new_packages" ]]; then
  echo '新增包（基线不存在，按新增报告）：'
  printf '%s' "$new_packages" | sed 's/^/  - /'
fi
if [[ -n "$experimental_warnings" ]]; then
  echo '实验 API 警告（不阻断）：'
  printf '%s' "$experimental_warnings" | sed 's/^/  - /'
fi
if [[ -n "$tool_errors" ]]; then
  echo 'API 检查工具/基线错误：' >&2
  printf '%s' "$tool_errors" >&2
  exit 2
fi
if [[ -n "$stable_failures" ]]; then
  echo 'API 检查失败：稳定层存在不兼容变更（ADR 0009）。' >&2
  printf '%s\n' "$stable_failures" >&2
  exit 1
fi
echo 'API 检查通过：稳定层无不兼容变更。'