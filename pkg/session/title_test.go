package session

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestGenerateTitle_Success(t *testing.T) {
	client := NewSimpleTitleClient(func(ctx context.Context, prompt string) (string, error) {
		return "用户询问" + "Go 语言并发编程", nil
	})

	title, err := GenerateTitle(context.Background(), client, "Go 语言的 goroutine 和 channel 怎么用？")
	if err != nil {
		t.Fatalf("GenerateTitle 失败: %v", err)
	}

	if title == "" {
		t.Fatal("标题不应为空")
	}
	t.Logf("生成的标题: %s", title)
}

func TestGenerateTitle_FallbackOnError(t *testing.T) {
	client := NewSimpleTitleClient(func(ctx context.Context, prompt string) (string, error) {
		return "", errors.New("LLM 调用失败")
	})

	title, err := GenerateTitle(context.Background(), client, "今天天气怎么样？")
	if err != nil {
		t.Fatalf("GenerateTitle 不应返回错误（回退）：%v", err)
	}

	if title != "今天天气怎么样？" {
		t.Errorf("回退标题应为原始消息，得到 %q", title)
	}
}

func TestGenerateTitle_FallbackOnEmptyResponse(t *testing.T) {
	client := NewSimpleTitleClient(func(ctx context.Context, prompt string) (string, error) {
		return "", nil
	})

	title, err := GenerateTitle(context.Background(), client, "测试消息")
	if err != nil {
		t.Fatalf("GenerateTitle 不应返回错误（回退）：%v", err)
	}

	if title != "测试消息" {
		t.Errorf("回退标题应为原始消息，得到 %q", title)
	}
}

func TestGenerateTitle_FallbackOnNilClient(t *testing.T) {
	title, err := GenerateTitle(context.Background(), nil, "nil client test")
	if err != nil {
		t.Fatalf("GenerateTitle 不应返回错误（回退）：%v", err)
	}

	if title != "nil client test" {
		t.Errorf("回退标题应为原始消息，得到 %q", title)
	}
}

func TestFallbackTitle_Short(t *testing.T) {
	title := fallbackTitle("你好")
	if title != "你好" {
		t.Errorf("短消息应保持原样，得到 %q", title)
	}
}

func TestFallbackTitle_Long(t *testing.T) {
	// 创建一个超过 50 字符的消息
	longMsg := strings.Repeat("测试消息", 20) // 80 个字符
	title := fallbackTitle(longMsg)

	if len([]rune(title)) > 54 { // 50 + "..."
		t.Errorf("回退标题应被截断到约 50 字符，得到 %d 字符", len([]rune(title)))
	}

	if !strings.HasSuffix(title, "...") {
		t.Errorf("截断标题应以 ... 结尾，得到 %q", title)
	}
}

func TestFallbackTitle_Exact50(t *testing.T) {
	// 刚好 50 个字符的消息
	shortMsg := strings.Repeat("a", 50)
	title := fallbackTitle(shortMsg)
	if title != shortMsg {
		t.Errorf("50 字符消息应保持原样，得到 %q", title)
	}
}

func TestFallbackTitle_TrimSpace(t *testing.T) {
	title := fallbackTitle("  hello  ")
	if title != "hello" {
		t.Errorf("应去除首尾空格，得到 %q", title)
	}
}

func TestGenerateTitle_TrimsQuotes(t *testing.T) {
	client := NewSimpleTitleClient(func(ctx context.Context, prompt string) (string, error) {
		return `"Go 并发编程"`, nil
	})

	title, _ := GenerateTitle(context.Background(), client, "goroutine 问题")
	if strings.Contains(title, `"`) {
		t.Errorf("标题应去除引号，得到 %q", title)
	}
}

func TestGenerateTitle_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	client := NewSimpleTitleClient(func(ctx context.Context, prompt string) (string, error) {
		return "", ctx.Err()
	})

	title, err := GenerateTitle(ctx, client, "测试")
	if err != nil {
		t.Fatalf("上下文取消后也应该回退：%v", err)
	}
	if title != "测试" {
		t.Errorf("回退标题应为原始消息，得到 %q", title)
	}
}