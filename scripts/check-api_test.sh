#!/usr/bin/env bash
set -euo pipefail

# 用临时 git 仓库验证 check-api.sh 的门禁协议：
# 兼容新增放行、稳定层破坏性变更拦截、实验层仅警告、基线缺失显式失败。
script_root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
fixture_root="$(mktemp -d "${TMPDIR:-/tmp}/basework-api-test.XXXXXX")"
trap 'rm -rf "$fixture_root"' EXIT

toy="$fixture_root/toy"
mkdir -p "$toy/pkg/a" "$toy/pkg/jobs"
cd "$toy"
git init -q
git config user.email apifix@example.com
git config user.name apifix
cat > go.mod <<'EOF'
module example.com/apifix

go 1.26.11
EOF
cat > pkg/a/a.go <<'EOF'
package a

type Config struct{ Name string }

func Do(c Config) error { return nil }
EOF
cat > pkg/jobs/j.go <<'EOF'
package jobs

func Run() {}
EOF
git add -A
git commit -qm base
git tag v0.0.1

run_gate() {
  bash "$script_root/check-api.sh" v0.0.1 > "$fixture_root/out" 2>&1
}

# 1. 无变更：通过
if ! run_gate; then
  echo '自测失败：无变更应当通过' >&2
  cat "$fixture_root/out" >&2
  exit 1
fi

# 2. 兼容新增：通过且报告 Added
cat >> pkg/a/a.go <<'EOF'

func Added() {}
EOF
if ! run_gate || ! grep -q 'Added' "$fixture_root/out"; then
  echo '自测失败：兼容新增应当通过并报告' >&2
  cat "$fixture_root/out" >&2
  exit 1
fi

# 3. 稳定层破坏性变更：拦截
cat > pkg/a/a.go <<'EOF'
package a

type Config struct{ Name string }

func Do(c Config, extra int) error { return nil }
EOF
if run_gate; then
  echo '自测失败：稳定层破坏性变更必须被拦截' >&2
  cat "$fixture_root/out" >&2
  exit 1
fi
grep -q 'Incompatible changes:' "$fixture_root/out"

# 4. 恢复稳定层，仅实验层破坏：放行但警告
cat > pkg/a/a.go <<'EOF'
package a

type Config struct{ Name string }

func Do(c Config) error { return nil }
EOF
cat > pkg/jobs/j.go <<'EOF'
package jobs

func Run(extra int) {}
EOF
if ! run_gate || ! grep -q '实验 API 警告' "$fixture_root/out"; then
  echo '自测失败：实验层破坏应放行并警告' >&2
  cat "$fixture_root/out" >&2
  exit 1
fi

# 5. 基线缺失：显式失败
if bash "$script_root/check-api.sh" v9.9.9 > "$fixture_root/out" 2>&1; then
  echo '自测失败：缺失基线必须显式失败' >&2
  exit 1
fi

echo 'check-api 门禁自测通过：兼容放行 / 稳定拦截 / 实验警告 / 基线缺失失败。'