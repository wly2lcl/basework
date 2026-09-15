// 嵌入示例：把 basework 当作库嵌进任意 Go 服务（SHIP-003 场景 5 的验收载体）。
//
// 只用公共 API：provider.Create 构造模型，agent.New 组装会话、工具和回调。
// 一次运行里完成 read → edit → bash → 答复，并断言磁盘产物、回调与会话事件。
//
// 退出码 0 = 全部断言通过；非 0 = 失败（消息写 stderr）。
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wly2lcl/basework/pkg/agent"
	"github.com/wly2lcl/basework/pkg/llm"
	"github.com/wly2lcl/basework/pkg/provider"
	"github.com/wly2lcl/basework/pkg/session"
	"github.com/wly2lcl/basework/pkg/tool"
	"github.com/wly2lcl/basework/pkg/tool/builtin"
)

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "嵌入验收失败: "+format+"\n", args...)
	os.Exit(1)
}

func main() {
	if len(os.Args) < 4 {
		fail("用法: embed <fake-port> <workdir> <home>")
	}
	port, workDir, home := os.Args[1], os.Args[2], os.Args[3]
	if err := os.Chdir(workDir); err != nil {
		fail("切换工作目录: %v", err)
	}

	// 1) 模型：指向本地假 OpenAI 兼容端点。
	model, err := provider.Create(provider.Config{
		Type:    "openai",
		ModelID: "gpt-4o",
		APIKey:  "embed-fake-key",
		BaseURL: "http://127.0.0.1:" + port + "/v1",
	})
	if err != nil {
		fail("构造模型: %v", err)
	}

	// 2) 会话存储：隔离 HOME 下的规范目录。
	sessDir := filepath.Join(home, ".local", "share", "basework", "sessions")
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		fail("建会话目录: %v", err)
	}
	sess, err := session.NewJSONLStore(sessDir)
	if err != nil {
		fail("建会话存储: %v", err)
	}

	// 3) Agent：模型 + 会话 + 内置工具。
	var eventMu sync.Mutex
	eventsSeen := map[string]int{}
	callback := &agent.CallbackFuncs{
		TextDelta: func(string) {
			eventMu.Lock()
			eventsSeen["text.delta"]++
			eventMu.Unlock()
		},
		ToolCallStart: func(llm.ToolCall) {
			eventMu.Lock()
			eventsSeen["tool.started"]++
			eventMu.Unlock()
		},
		ToolCallEnd: func(llm.ToolCall, *tool.Result, error) {
			eventMu.Lock()
			eventsSeen["tool.finished"]++
			eventMu.Unlock()
		},
		TurnEnd: func(*agent.Response) {
			eventMu.Lock()
			eventsSeen["turn.finished"]++
			eventMu.Unlock()
		},
	}
	agt, err := agent.New(
		agent.WithModel(model),
		agent.WithSession(sess),
		agent.WithTools(builtin.All()...),
		agent.WithCallback(callback),
		agent.WithMaxSteps(12),
	)
	if err != nil {
		fail("组装 Agent: %v", err)
	}
	sid := ""
	if p, ok := agt.(interface{ SessionID() string }); ok {
		sid = p.SessionID()
	}
	if sid == "" {
		fail("Agent 未暴露会话 ID")
	}

	// 4) 跑一个完整回合：读文件 → 修复 → 跑测试 → 答复。
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	run, err := agt.HandleMessage(ctx, "[SE] 请修复 calc.go 里 Add 的缺陷，并跑测试确认")
	if err != nil {
		fail("HandleMessage: %v", err)
	}
	var reply strings.Builder
	for _, part := range run.Message.Content {
		if part.Type == llm.ContentTypeText {
			reply.WriteString(part.Text)
		}
	}
	if !strings.Contains(reply.String(), "嵌入服务已修复") {
		fail("最终答复不符合脚本: %q", reply.String())
	}
	if len(run.ToolCalls) < 3 {
		fail("工具调用数不足: %d", len(run.ToolCalls))
	}
	for _, rec := range run.ToolCalls {
		if rec.Result != nil && rec.Result.IsError {
			fail("工具 %s 执行失败: %s", rec.Call.Name, rec.Result.Content)
		}
	}

	// 6) 磁盘产物：缺陷真的被修掉了。
	content, err := os.ReadFile(filepath.Join(workDir, "calc.go"))
	if err != nil {
		fail("读结果文件: %v", err)
	}
	if !strings.Contains(string(content), "return a + b") {
		fail("calc.go 未被修复:\n%s", content)
	}

	// 7) 会话事实已落盘：重开一个存储能读到同一会话。
	sess2, err := session.NewJSONLStore(sessDir)
	if err != nil {
		fail("重开会话存储: %v", err)
	}
	events, err := sess2.Events(session.EventFilter{SessionID: sid})
	if err != nil {
		fail("读会话事件: %v", err)
	}
	if len(events) == 0 {
		fail("会话事件为空：嵌入路径没有持久化")
	}

	// 7) Agent 生命周期：Close 幂等。回调与 HandleMessage 同步交付，
	// 不用固定 sleep 作为事件同步依据。
	if err := agt.Close(); err != nil {
		fail("Close: %v", err)
	}
	if err := agt.Close(); err != nil {
		fail("Close 不幂等: %v", err)
	}
	eventMu.Lock()
	seenCopy := map[string]int{}
	for k, v := range eventsSeen {
		seenCopy[k] = v
	}
	eventMu.Unlock()
	if seenCopy["tool.started"] < 3 || seenCopy["tool.finished"] < 3 || seenCopy["turn.finished"] < 1 {
		fail("回调事件不足: %v", seenCopy)
	}

	fmt.Printf("嵌入验收通过: 会话=%s 工具调用=%d 事件=%v 事件落盘=%d 条\n",
		sid[:8], len(run.ToolCalls), seenCopy, len(events))
}
