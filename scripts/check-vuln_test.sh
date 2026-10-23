#!/usr/bin/env bash
set -euo pipefail

# 用假的 go 命令验证门禁协议，不访问网络，也不把假扫描器入口带入正式脚本。
script_root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
fixture_root="$(mktemp -d "${TMPDIR:-/tmp}/basework-vuln-test.XXXXXX")"
trap 'rm -rf "$fixture_root"' EXIT
mkdir -p "$fixture_root/bin" "$fixture_root/path with spaces"
export BASEWORK_VULN_TEST_LOG="$fixture_root/calls"
export BASEWORK_VULN_TEST_STATUS=0
export PATH="$fixture_root/bin:$PATH"
cat > "$fixture_root/bin/go" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ -n "${GOOS:-}" || -n "${GOARCH:-}" || -n "${GOVERSION:-}" ]]; then
  echo '扫描器错误继承了交叉编译环境或虚假的工具链版本。' >&2
  exit 98
fi
if [[ -n "${GOFLAGS:-}" || "${GOENV:-}" != off || "${GOWORK:-}" != off ]]; then
  echo '扫描器错误继承了外部 Go 配置。' >&2
  exit 97
fi
printf '%s\n' "$#" "$@" >> "$BASEWORK_VULN_TEST_LOG"
if [[ -n "${BASEWORK_VULN_TEST_FAIL_ON:-}" ]]; then
  for argument in "$@"; do
    if [[ "$argument" == "$BASEWORK_VULN_TEST_FAIL_ON" ]]; then
      exit "$BASEWORK_VULN_TEST_STATUS"
    fi
  done
  exit 0
fi
exit "$BASEWORK_VULN_TEST_STATUS"
EOF
chmod +x "$fixture_root/bin/go"
artifact="$fixture_root/path with spaces/basework artifact"
printf 'binary fixture\n' > "$artifact"

expect_failure() {
  local label="$1"
  shift
  if bash "$script_root/check-vuln.sh" "$@" > "$fixture_root/output" 2>&1; then
    echo "未正确拒绝：${label}" >&2
    cat "$fixture_root/output" >&2
    exit 1
  fi
}

# 默认入口必须覆盖整个模块、测试文件和两种标签配置。
bash "$script_root/check-vuln.sh" > "$fixture_root/output" 2>&1
cat > "$fixture_root/expected" <<'EOF'
5
run
golang.org/x/vuln/cmd/govulncheck@v1.8.0
-db=https://vuln.go.dev
-test
./...
6
run
golang.org/x/vuln/cmd/govulncheck@v1.8.0
-db=https://vuln.go.dev
-test
-tags=sqlite,memory
./...
EOF
cmp "$fixture_root/expected" "$BASEWORK_VULN_TEST_LOG"

# 带空格路径保持单个参数；交叉构建环境不能影响宿主扫描器。
: > "$BASEWORK_VULN_TEST_LOG"
GOOS=linux GOARCH=arm64 GOVERSION=go99.0 GOFLAGS='-exec=/usr/bin/true -tags=unexpected' GOVULNDB=file:///empty \
  bash "$script_root/check-vuln.sh" binary "$artifact" > "$fixture_root/output" 2>&1
printf '6\nrun\ngolang.org/x/vuln/cmd/govulncheck@v1.8.0\n-db=https://vuln.go.dev\n-mode=binary\n--\n%s\n' "$artifact" > "$fixture_root/expected"
cmp "$fixture_root/expected" "$BASEWORK_VULN_TEST_LOG"

# 多产物均需扫描；以减号开头的文件名只能作为文件，不能注入扫描参数。
: > "$BASEWORK_VULN_TEST_LOG"
printf 'binary fixture\n' > "$fixture_root/-json"
(cd "$fixture_root" && bash "$script_root/check-vuln.sh" binary "$artifact" -json) > "$fixture_root/output" 2>&1
printf '6\nrun\ngolang.org/x/vuln/cmd/govulncheck@v1.8.0\n-db=https://vuln.go.dev\n-mode=binary\n--\n%s\n6\nrun\ngolang.org/x/vuln/cmd/govulncheck@v1.8.0\n-db=https://vuln.go.dev\n-mode=binary\n--\n-json\n' "$artifact" > "$fixture_root/expected"
cmp "$fixture_root/expected" "$BASEWORK_VULN_TEST_LOG"

# 漏洞、工具/数据库网络错误都必须失败，而且首次失败后不再执行第二次扫描。
for status in 3 1 17; do
  export BASEWORK_VULN_TEST_STATUS="$status"
  : > "$BASEWORK_VULN_TEST_LOG"
  expect_failure "源码扫描失败 ${status}" source
  test "$(wc -l < "$BASEWORK_VULN_TEST_LOG" | tr -d ' ')" = 6
  expect_failure "产物扫描失败 ${status}" binary "$artifact"
done
export BASEWORK_VULN_TEST_STATUS=0

# 首次扫描成功也不能吞掉后续配置或后续产物的失败。
export BASEWORK_VULN_TEST_STATUS=3
export BASEWORK_VULN_TEST_FAIL_ON='-tags=sqlite,memory'
: > "$BASEWORK_VULN_TEST_LOG"
expect_failure '可选构建才发现漏洞' source
test "$(wc -l < "$BASEWORK_VULN_TEST_LOG" | tr -d ' ')" = 13
export BASEWORK_VULN_TEST_FAIL_ON="$fixture_root/-json"
: > "$BASEWORK_VULN_TEST_LOG"
expect_failure '第二个产物才发现漏洞' binary "$artifact" "$fixture_root/-json" "$artifact"
test "$(wc -l < "$BASEWORK_VULN_TEST_LOG" | tr -d ' ')" = 14
unset BASEWORK_VULN_TEST_FAIL_ON
export BASEWORK_VULN_TEST_STATUS=0

: > "$BASEWORK_VULN_TEST_LOG"
expect_failure '未提供产物' binary
expect_failure '空路径' binary ''
expect_failure '缺失产物' binary "$fixture_root/missing"
expect_failure '目录不是产物' binary "$fixture_root"
touch "$fixture_root/empty"
expect_failure '零字节产物' binary "$fixture_root/empty"
expect_failure '部分产物缺失' binary "$artifact" "$fixture_root/missing"
expect_failure '禁止自定义源码范围' source ./cmd/basework
expect_failure '未知模式' unknown
expect_failure '空模式' ''
test ! -s "$BASEWORK_VULN_TEST_LOG"
echo '漏洞扫描脚本回归检查通过。'
