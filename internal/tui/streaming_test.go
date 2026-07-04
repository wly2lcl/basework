// Package tui 测试
package tui

import (
	"testing"
)

// TestNewStreamingView 测试创建流式渲染组件
func TestNewStreamingView(t *testing.T) {
	sv := NewStreamingView()
	if sv == nil {
		t.Fatal("NewStreamingView() 返回了 nil")
	}
	if sv.IsRunning() {
		t.Fatal("新建的 StreamingView 不应处于运行状态")
	}
}

// TestStreamingViewStartStop 测试开始和停止流式输出
func TestStreamingViewStartStop(t *testing.T) {
	sv := NewStreamingView()

	sv.Start()
	if !sv.IsRunning() {
		t.Fatal("Start 后 IsRunning 应为 true")
	}

	sv.Stop()
	if sv.IsRunning() {
		t.Fatal("Stop 后 IsRunning 应为 false")
	}
}

// TestStreamingViewUpdateText 测试更新文本
func TestStreamingViewUpdateText(t *testing.T) {
	sv := NewStreamingView()
	sv.Start()

	sv.UpdateText("Hello")
	if sv.FullText() != "Hello" {
		t.Fatalf("期望 'Hello'，得到 %q", sv.FullText())
	}

	sv.UpdateText("World")
	if sv.FullText() != "World" {
		t.Fatalf("期望 'World'，得到 %q", sv.FullText())
	}
}

// TestStreamingViewAppendText 测试追加文本
func TestStreamingViewAppendText(t *testing.T) {
	sv := NewStreamingView()
	sv.Start()

	sv.AppendText("Hello ")
	sv.AppendText("World")
	if sv.FullText() != "Hello World" {
		t.Fatalf("期望 'Hello World'，得到 %q", sv.FullText())
	}
}

// TestStreamingViewThinking 测试思考过程
func TestStreamingViewThinking(t *testing.T) {
	sv := NewStreamingView()
	sv.Start()

	sv.AppendThinking("分析中")
	sv.AppendThinking("...")
	// 思考文本不应影响 FullText
	if sv.FullText() != "" {
		t.Fatalf("思考过程中 FullText 应为空，得到 %q", sv.FullText())
	}
}

// TestStreamingViewToolProgress 测试工具调用进度指示
func TestStreamingViewToolProgress(t *testing.T) {
	sv := NewStreamingView()
	sv.Start()

	sv.SetToolInProgress("web_search")
	render := sv.Render(80)
	if render == "" {
		t.Fatal("工具调用进度指示渲染不应为空")
	}
}

// TestStreamingViewRenderStopped 测试停止后的渲染
func TestStreamingViewRenderStopped(t *testing.T) {
	sv := NewStreamingView()
	sv.Start()
	sv.Stop()

	render := sv.Render(80)
	if render != "" {
		t.Fatal("停止后的渲染应为空")
	}
}

// TestStreamingViewTick 测试心跳更新
func TestStreamingViewTick(t *testing.T) {
	sv := NewStreamingView()
	sv.Start()

	initialIdx := sv.spinnerIdx
	sv.Tick()
	if sv.spinnerIdx != initialIdx+1 {
		t.Fatalf("Tick 后 spinnerIdx 应增加 1")
	}
}

// TestStreamingViewFullTextWithAppend 测试追加后的完整文本
func TestStreamingViewFullTextWithAppend(t *testing.T) {
	sv := NewStreamingView()
	sv.Start()

	sv.AppendText("Hello")
	sv.AppendText(" ")
	sv.AppendText("World")
	sv.AppendText("!")
	sv.Stop()

	if sv.FullText() != "Hello World!" {
		t.Fatalf("期望 'Hello World!'，得到 %q", sv.FullText())
	}
}

// TestStreamingViewMultipleRestart 测试多次启停
func TestStreamingViewMultipleRestart(t *testing.T) {
	sv := NewStreamingView()

	sv.Start()
	sv.AppendText("第一轮")
	sv.Stop()

	sv.Start()
	sv.AppendText("第二轮")
	sv.Stop()

	// 第二次启动应清空文本
	if sv.FullText() != "第二轮" {
		t.Fatalf("重启后应只有新文本，得到 %q", sv.FullText())
	}
}