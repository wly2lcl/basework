# 2026-09-14 复审最小复现

这些测试对应 [复审报告](evidence/REVIEW-2026-09-14.md)。在 bb455ff 上共 16 项，预期全部失败；修复后应逐项变绿。使用隔离临时目录和虚拟文件映射，不改仓库代码、不调用真实模型。部分测试复用同包已有夹具，需从仓库根运行。macOS 已验证；软链测试在 Windows 需要平台权限，目标平台结果另记。

## 一次执行

下方 Python 仅将本文的 Go 代码块写入临时目录，生成 overlay，然后运行定向测试。自动断言失败会返回非零，这是复审的预期；不要把它当常规门禁已经通过。

验证范围：16 个测试已在 macOS 分批执行，本文源码与执行版本一致；下方汇总脚本只完成语法与源码提取检查。最后一次汇总复跑因自动审批复核的用量额度限制未执行，详见复审报告。

```bash
python3 - <<'PY'
import json, pathlib, re, subprocess, tempfile
root = pathlib.Path.cwd()
text = (root / "docs/development/review-reproduction.md").read_text()
pattern = r"### ([a-zA-Z0-9_./]+_test\.go)\n\n```go\n(.*?)\n```"
parts = re.findall(pattern, text, re.S)
assert len(parts) == 8, f"expected 8 source blocks, got {len(parts)}"
with tempfile.TemporaryDirectory(prefix="basework-review-") as tmp:
    mapping = {}
    for i, (rel, body) in enumerate(parts):
        source = pathlib.Path(tmp) / f"probe{i}_test.go"
        source.write_text(body + "\n")
        mapping[str(root / rel)] = str(source)
    overlay = pathlib.Path(tmp) / "overlay.json"
    overlay.write_text(json.dumps({"Replace": mapping}))
    packages = ["./" + str(pathlib.PurePosixPath(rel).parent) for rel, _ in parts]
    result = subprocess.run(["go", "test", "-overlay", str(overlay), "-tags", "sqlite memory",
                             "-run", "^TestReview", "-count=1", "-timeout=2m", "-v", *packages])
    raise SystemExit(result.returncode)
PY
```

## 测试源码

### internal/runtime/review_20260914_test.go

```go
package runtime
import (
 "context"
 "errors"
 "testing"
 "time"
 "github.com/wly2lcl/basework/pkg/agent"
)
func TestReviewStartQueuedAtClose(t *testing.T) {
 calls := make(chan string, 1)
 s := mustNew(t, &fakeAgent{handle:func(_ context.Context, input string)(*agent.Response,error){calls<-input; return &agent.Response{},nil}},LocalDeps{})
 s.runsMu.Lock()
 done := make(chan error,1)
 go func(){_,err:=s.Start(context.Background(),"queued"); done<-err}()
 deadline:=time.Now().Add(time.Second)
 for s.runSeq.Load()==0 && time.Now().Before(deadline) { time.Sleep(time.Millisecond) }
 if s.runSeq.Load()==0 {s.runsMu.Unlock(); t.Fatal("Start not entered")}
 if err:=s.Close();err!=nil {t.Fatal(err)}
 s.runsMu.Unlock()
 if err:=<-done; !errors.Is(err,ErrClosed) {t.Errorf("queued Start after Close: got %v, want ErrClosed",err)}
 select {case input:=<-calls: t.Errorf("Agent invoked after Close: %s",input);default:}
}
func TestReviewSubscribeAfterClose(t *testing.T) {
 s:=mustNew(t,&fakeAgent{},LocalDeps{})
 _=s.Close()
 sub:=s.Subscribe()
 defer sub.Unsubscribe()
 select {case _,ok:=<-sub.Events(): if ok {t.Error("unexpected event")}; default:t.Error("subscription after Close stays open")}
}
```

### internal/tui/review_20260914_test.go

```go
package tui
import (
 "fmt"
 "strings"
 "testing"
 "github.com/wly2lcl/basework/internal/tui/theme"
)
func TestReviewResumeNarrow(t *testing.T) {
 defer func(){if r:=recover(); r!=nil {t.Errorf("narrow resume panic: %v",r)}}()
 p:=NewResumePanel();p.SetItems([]ResumeItem{{Kind:"file",Label:"a.go"}})
 _=p.Render(20)
}
func TestReviewSwitchSessionIsolation(t *testing.T) {
 a:=NewApp("model","provider","old")
 a.AddAssistantMessage("old-history")
 a.StartStreaming()
 a.SwitchSession("new")
 for _,m:=range a.Messages { if m.Content=="old-history" {t.Error("old messages survive SwitchSession")} }
 if a.IsStreaming {t.Error("old IsStreaming survives SwitchSession")}
 a.Update(AgentResponseMsg{Text:"old-late-reply"})
 for _,m:=range a.Messages {if m.Content=="old-late-reply" {t.Error("late response appended to new session")}}
}
func TestReviewJobsPaginationReachable(t *testing.T) {
 var b strings.Builder
 for i:=1;i<=60;i++ {fmt.Fprintf(&b,"LINE-%03d\n",i)}
 text:=b.String()
 v:=NewJobsView(func(id string,off int64)JobOutputMsg{return JobOutputMsg{JobID:id,Offset:off,Text:text[off:],More:false}},nil)
 v.Update(JobStatusMsg{Jobs:[]JobStatus{{ID:"job1",State:"succeeded"}}})
 v.Toggle();v.HandleKey("enter")
 first:=v.Render(100,theme.DefaultTheme)
 v.HandleKey("]")
 next:=v.Render(100,theme.DefaultTheme)
 if !strings.Contains(first,"LINE-031") && !strings.Contains(next,"LINE-031") {t.Error("lines 31-60 fit one byte page but are unreachable through next-page key")}
}
```

### internal/edits/review_20260914_test.go

```go
package edits
import (
 "os"
 "path/filepath"
 "testing"
)
func TestReviewRollbackRechecksPermission(t *testing.T) {
 allowed:=true
 p,root:=newPlanner(t,func(string)(bool,string){return allowed,"revoked"})
 file:=writeFile(t,root,"a.txt","old")
 op,err:=p.Preview("a.txt","old","new");if err!=nil {t.Fatal(err)}
 batch,err:=p.Prepare(op);if err!=nil {t.Fatal(err)}
 r,err:=p.Commit(batch,CommitOptions{});if err!=nil {t.Fatal(err)}
 allowed=false
 _,_=r.Rollback(CommitOptions{})
 data,err:=os.ReadFile(file);if err!=nil {t.Fatal(err)}
 if string(data)!="new" {t.Errorf("rollback writes after permission revoked: %q",data)}
}
func TestReviewRollbackParentSymlink(t *testing.T) {
 p,root:=newPlanner(t,nil)
 writeFile(t,root,"sub/a.txt","old")
 op,err:=p.Preview("sub/a.txt","old","new");if err!=nil {t.Fatal(err)}
 batch,err:=p.Prepare(op);if err!=nil {t.Fatal(err)}
 r,err:=p.Commit(batch,CommitOptions{});if err!=nil {t.Fatal(err)}
 outside:=t.TempDir()
 target:=filepath.Join(outside,"a.txt")
 if err:=os.WriteFile(target,[]byte("new"),0600);err!=nil {t.Fatal(err)}
 if err:=os.Rename(filepath.Join(root,"sub"),filepath.Join(root,"saved"));err!=nil {t.Fatal(err)}
 if err:=os.Symlink(outside,filepath.Join(root,"sub"));err!=nil {t.Fatal(err)}
 _,_=r.Rollback(CommitOptions{})
 data,err:=os.ReadFile(target);if err!=nil {t.Fatal(err)}
 if string(data)!="new" {t.Errorf("rollback escaped workspace through parent symlink: %q",data)}
}
```

### internal/jobs/review_20260914_test.go

```go
package jobs
import (
 "os"
 "path/filepath"
 "testing"
)
func TestReviewJournalAppendAfterTruncatedTail(t *testing.T) {
 path:=filepath.Join(t.TempDir(),"jobs.jsonl")
 j:=NewJSONLJournal(path)
 rec:=Record{Kind:RecordStarted,JobID:"one",Owner:"owner",State:StateRunning}
 if err:=j.Append(rec);err!=nil {t.Fatal(err)}
 f,err:=os.OpenFile(path,os.O_APPEND|os.O_WRONLY,0600);if err!=nil {t.Fatal(err)}
 if _,err=f.WriteString("{\"kind\":");err!=nil {t.Fatal(err)}
 if err=f.Close();err!=nil {t.Fatal(err)}
 if _,err=j.Records();err!=nil {t.Fatal(err)}
 rec.Kind=RecordTerminal;rec.State=StateInterrupted
 if err=j.Append(rec);err!=nil {t.Fatal(err)}
 if _,err=j.Records();err!=nil {t.Errorf("append after tolerated tail corrupts journal: %v",err)}
}
```

### internal/permission/review_20260914_test.go

```go
package permission
import "testing"
func TestReviewCommitApprovalHasPlanDetails(t *testing.T) {
 req:=BuildApprovalRequest("edit_files",map[string]interface{}{"action":"commit","plan_id":"plan-known"},nil)
 if len(req.Paths)==0 || req.Diff=="" {t.Errorf("commit approval missing trusted plan detail: paths=%v diff=%q",req.Paths,req.Diff)}
}
```

### pkg/session/review_20260914_test.go

```go
package session
import (
 "testing"
 "time"
)
func TestReviewFactsHardBudget(t *testing.T) {
 w:=factsFixture(t)
 got:=SummarizeFacts(w,FactsSummaryOptions{Budget:80})
 if len(got.Text)>80 {t.Errorf("budget=80, actual=%d",len(got.Text))}
}
func TestReviewPartialCommitFacts(t *testing.T) {
 w:=NewWorkspaceFacts("workspace")
 FoldFileEdited(w,FileEditedData{Phase:"failed",PlanID:"p",Files:[]FileEditRecord{{Path:"a.go",State:"written"},{Path:"b.go",State:"conflict"}}},time.Now())
 if w.Count()!=1 {t.Errorf("partial commit wrote a.go but facts=%d",w.Count())}
}
```

### pkg/agent/review_20260914_test.go

```go
package agent
import (
 "strings"
 "testing"
 "github.com/wly2lcl/basework/pkg/session"
)
func TestReviewFactsProviderStalenessAcrossCollect(t *testing.T) {
 dir:=t.TempDir()
 w:=session.NewWorkspaceFacts("ws")
 w.Add(session.Fact{Kind:session.FactFileModified,Key:"a",Path:"a.go",Source:"edit_files"})
 if err:=session.SaveWorkspaceFacts(dir,w);err!=nil {t.Fatal(err)}
 hash:="old"
 p:=NewFactsSummaryProvider(dir,"ws",8192,func(string)(string,error){return hash,nil})
 if _,err:=p.Collect();err!=nil {t.Fatal(err)}
 hash="new"
 text,err:=p.Collect();if err!=nil {t.Fatal(err)}
 if !strings.Contains(text,"已过期") {t.Error("same provider second Collect cannot detect changed hash")}
}
func TestReviewFactsWindowsRelativeTraversal(t *testing.T) {
 called:=false
 p:=NewFactsSummaryProvider(t.TempDir(),"ws",8192,func(string)(string,error){called=true;return "hash",nil})
 _,err:=p.wrapHash("..\\outside.txt")
 if err==nil || called {t.Errorf("Windows relative traversal accepted: err=%v hashCalled=%v",err,called)}
}
```

### cmd/basework/review_20260914_test.go

```go
package main
import (
 "context"
 "encoding/json"
 "os"
 "path/filepath"
 "reflect"
 "regexp"
 "testing"
 "github.com/wly2lcl/basework/internal/jobs"
 "github.com/wly2lcl/basework/pkg/config"
 "github.com/wly2lcl/basework/pkg/session"
)
func reviewRuntime(t *testing.T, cfg *config.Config) *runtimeAgent {
 t.Helper()
 cfg.Provider="openai";cfg.Model="gpt-4o-mini"
 cfg.Providers=map[string]config.ProviderEndpoint{"openai":{BaseURL:"http://127.0.0.1:1/v1",APIKey:"review-fake"}}
 rt,err:=newRuntimeAgent(cfg,runtimeAgentOptions{});if err!=nil {t.Fatal(err)}
 t.Cleanup(func(){_=rt.Service().Close()})
 return rt
}
func TestReviewRuntimeBehaviorWiring(t *testing.T) {
 cfg:=isolatedConfig(t);cfg.Compaction.Enabled=true
 rt:=reviewRuntime(t,cfg)
 fields:=reflect.ValueOf(rt.Agent).Elem()
 for _,name:=range []string{"compactor","loopDetector"} {
  if fields.FieldByName(name).IsNil() {t.Errorf("enabled runtime %s is nil",name)}
 }
}
func TestReviewRuntimeEditPersistsFacts(t *testing.T) {
 cfg:=isolatedConfig(t)
 work:=t.TempDir();t.Chdir(work)
 if err:=os.WriteFile(filepath.Join(work,"a.txt"),[]byte("old"),0600);err!=nil {t.Fatal(err)}
 rt:=reviewRuntime(t,cfg)
 found:=false
 for _,tool:=range rt.Agent.Tools() {
  if tool.Name()!="edit_files" {continue};found=true
  preview,err:=tool.Execute(context.Background(),json.RawMessage(`{"action":"preview","edits":[{"path":"a.txt","old":"old","new":"new"}]}`))
  if err!=nil || preview.IsError {t.Fatalf("preview=%+v err=%v",preview,err)}
  id:=regexp.MustCompile("plan-[a-f0-9]+").FindString(preview.Content)
  raw,_:=json.Marshal(map[string]string{"action":"commit","plan_id":id})
  result,err:=tool.Execute(context.Background(),raw)
  if err!=nil || result.IsError {t.Fatalf("commit=%+v err=%v",result,err)}
 }
 if !found {t.Fatal("edit_files missing")}
 if err:=rt.Service().Close();err!=nil {t.Fatal(err)}
 w,err:=session.LoadWorkspaceFacts(getSessionDir(),session.WorkspaceID(work));if err!=nil {t.Fatal(err)}
 if w.Count()==0 {t.Error("real edit committed and runtime closed but persisted facts remain empty")}
}
func TestReviewRestartLeavesOldJobRunning(t *testing.T) {
 cfg:=isolatedConfig(t)
 first:=reviewRuntime(t,cfg)
 old:=first.sessionID()
 journal:=jobs.NewJSONLJournal(jobJournalPath())
 if err:=journal.Append(jobs.Record{Kind:jobs.RecordStarted,JobID:"old-job",Owner:old,State:jobs.StateRunning});err!=nil {t.Fatal(err)}
 _=first.Service().Close()
 second:=reviewRuntime(t,cfg)
 t.Logf("old session=%s new session=%s",old,second.sessionID())
 history,err:=second.jobManager.History(old);if err!=nil {t.Fatal(err)}
 if len(history)!=1 {t.Fatalf("history=%+v",history)}
 if history[0].State!=jobs.StateInterrupted {t.Errorf("old job remains %s after restart",history[0].State)}
}
```
