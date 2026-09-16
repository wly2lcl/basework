# 2026-09-16 审查最小复现

对应 [复审报告](evidence/REVIEW-2026-09-16.md) B01–B03。基线 `dc7c518` 的三个探针已运行并以断言失败确认缺陷；当前 checkout 的修复回归必须全部通过，不能把“预期失败”当成修复完成。

只使用本地 HTTP 假服务、合成密钥和临时 Go 夹具；不调用真实 Provider、不读取真实凭证。Go overlay 只注入测试文件，不改业务源码。测试结束清理假服务、夹具和 goroutine。失败分支打印的是合成测试结果。

## 一键复现

在仓库根执行以下命令（Python 3 和项目 Go 工具链）。需要允许本地 loopback 监听；沙箱端口拒绝不是产品缺陷。

```bash
python3 - <<'PYCODE'
import json, os, pathlib, re, subprocess, tempfile
root = pathlib.Path.cwd()
text = (root / 'docs/development/review-reproduction-2026-09-16.md').read_text()
blocks = re.findall(r'```go\n(.*?)\n```', text, re.S)
assert len(blocks) == 2, 'expected two Go probes'
with tempfile.TemporaryDirectory(prefix='basework-review-') as tmp:
    tmp = pathlib.Path(tmp)
    mapping = {}
    for package, code in zip(['internal/runtime', 'tests/real_provider'], blocks):
        source = tmp / (package.replace('/', '-') + '_test.go')
        source.write_text(code + '\n')
        mapping[str(root / package / 'review_audit_test.go')] = str(source)
    overlay = tmp / 'overlay.json'
    overlay.write_text(json.dumps({'Replace': mapping}))
    env = dict(os.environ, GOCACHE='/tmp/basework-go-cache')
    result = subprocess.run(['go', 'test', '-overlay', str(overlay), '-tags',
        'sqlite memory', './internal/runtime', './tests/real_provider',
        '-run', '^TestReview', '-count=1', '-timeout=30s', '-v'], env=env)
    raise SystemExit(result.returncode)
PYCODE
```

## B01：关闭与运行登记交错

通过真实的 ForRun 回调扩展点建立确定时序：Start 已检查 closed → Close 清空 cancels 并关闭订阅 → Start 登记新 cancel → Agent 收到未取消的 context。通道用于同步，计时器只防止测试清理卡死。

```go
package runtime

import (
 "context"
 "testing"
 "time"
 "github.com/wly2lcl/basework/pkg/agent"
)

type reviewBlockingCallback struct {
 agent.NopCallback
 entered, release chan struct{}
}
func (c *reviewBlockingCallback) ForRun(string, string) agent.Callback {
 close(c.entered)
 <-c.release
 return c
}

func TestReviewCloseDuringRegistration(t *testing.T) {
 cb := &reviewBlockingCallback{entered: make(chan struct{}), release: make(chan struct{})}
 enteredAgent := make(chan context.Context, 1)
 ag := &fakeAgent{handle: func(ctx context.Context, _ string) (*agent.Response, error) {
  enteredAgent <- ctx
  <-ctx.Done()
  return nil, ctx.Err()
 }}
 svc, err := NewLocal(LocalDeps{Agent: ag})
 if err != nil { t.Fatal(err) }
 caller, cancel := context.WithCancel(context.Background())
 defer cancel()
 startDone := make(chan struct{})
 go func() { defer close(startDone); svc.StartWithCallback(caller, "review", cb) }()
 <-cb.entered
 sub := svc.Subscribe()
 closeDone := make(chan struct{})
 go func() { defer close(closeDone); svc.Close() }()
 <-svc.closedCh
 // Close closes subscriptions after draining cancels under the same mutex.
 <-sub.Events()
 close(cb.release)
 select {
 case ctx := <-enteredAgent:
  if ctx.Err() == nil { t.Error("Close 已清空 cancels 后，Start 仍进入未取消的 Agent；Close 将等待调用方取消") }
 case <-startDone:
 case <-time.After(2*time.Second): t.Error("Start 未结束")
 }
 cancel()
 select { case <-startDone: case <-time.After(2*time.Second): t.Fatal("cleanup Start timeout") }
 select { case <-closeDone: case <-time.After(2*time.Second): t.Fatal("cleanup Close timeout") }
}
```

## B02/B03：验收假阳性与秘密进入结果

B02 通过本地 OpenAI 流式假服务调用真实 bash 工具：仅给错误的 calc.go 加注释，删除 calc_test.go 的测试断言；基线 run 仍返回 nil，报告 agent_ok=true、test_exit_code=0、file_changed=true。B03 仅注入合成环境变量，基线证明工具继承秘密且 recorder 原样序列化失败输出；修复回归使用 runner 注入的最小环境。

```go
package main

import (
 "context"
 "encoding/json"
 "fmt"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "sync/atomic"
 "testing"
 "github.com/wly2lcl/basework/pkg/llm"
 "github.com/wly2lcl/basework/pkg/tool/builtin"
)

func TestReviewRealProviderRejectsTamperedTests(t *testing.T) {
 cwd, err := os.Getwd(); if err != nil { t.Fatal(err) }
 defer os.Chdir(cwd)
 var calls atomic.Int32
 server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
  w.Header().Set("Content-Type", "text/event-stream")
  var delta map[string]any
  finish := "stop"
  if calls.Add(1) == 1 {
   // Keep Add broken, change only a comment, and remove the actual assertions.
   args, _ := json.Marshal(map[string]string{"command": "printf '\n// touched\n' >> calc.go; printf 'package main\n' > calc_test.go"})
   delta = map[string]any{"tool_calls": []any{map[string]any{"index":0,"id":"review-1","type":"function","function":map[string]any{"name":"bash","arguments":string(args)}}}}
   finish = "tool_calls"
  } else { delta = map[string]any{"content":"done"} }
  data, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index":0,"delta":delta,"finish_reason":finish}}})
  fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", data)
 }))
 defer server.Close()
 t.Setenv("BASEWORK_REAL_PROVIDER", "openai")
 t.Setenv("BASEWORK_REAL_API_KEY", "review-fake-key")
 t.Setenv("BASEWORK_REAL_BASE_URL", server.URL+"/v1")
 t.Setenv("BASEWORK_REAL_MODEL", "review-fixture")
 output := filepath.Join(t.TempDir(), "result.json")
 err = run(filepath.Join(cwd,"fixtures","fixbug"), output)
 if err == nil {
  data, _ := os.ReadFile(output)
  t.Errorf("broken Add + deleted assertions 被判验收通过: %s", data)
 }
}

func TestReviewRecorderRedactsToolSecrets(t *testing.T) {
 const sentinel = "review-SYNTHETIC-secret-only"
 t.Setenv("BASEWORK_REAL_API_KEY", sentinel)
 b := &builtin.BashTool{Runtime: &builtin.Runtime{Environment: sanitizedEnvironment()}}
 res, err := b.Execute(context.Background(), json.RawMessage(`{"command":"printenv BASEWORK_REAL_API_KEY; exit 1"}`))
 if err != nil || res == nil { t.Fatalf("probe execution failed: %v", err) }
 r := &recorder{}
 r.OnToolCallEnd(llm.ToolCall{Name:"bash"}, res, err)
 data, _ := json.Marshal(r.snapshot())
 if strings.Contains(string(data), sentinel) {
  t.Error("合成密钥进入工具输出或声称脱敏的 JSON")
 }
 if strings.Contains(res.Content, sentinel) { t.Error("Bash 子进程继承了 Provider key") }
}
```

## 基线已观察到的结果

```text
在 `dc7c518` 基线运行时，B01、B02、B03 分别表现为：关闭等待外部取消；删除测试后仍报告 `test_exit_code=0` 与 `file_changed=true`；合成密钥出现在工具错误和结果 JSON。以上原始输出已摘录在复审报告。

当前修复回归的预期结果：

```text
ok  github.com/wly2lcl/basework/internal/runtime
ok  github.com/wly2lcl/basework/tests/real_provider
```
```

基线结果证明旧门禁允许错误结果通过，并不证明历史真实 Provider 那次修改过测试或泄漏了密钥。旧 JSON 缺少测试哈希和补丁，无法仅凭该文件补证测试完整性；修复后的结果必须包含可信测试哈希、实际执行标志和脱敏字段。
