// Package command 测试
package command

import (
	"testing"
)

// TestFuzzyMatchExact 测试精确匹配
func TestFuzzyMatchExact(t *testing.T) {
	score, ok := FuzzyMatch("help", "help")
	if !ok {
		t.Fatal("精确匹配应返回 true")
	}
	if score != 1000 {
		t.Fatalf("精确匹配期望 1000 分，得到 %d", score)
	}
}

// TestFuzzyMatchPrefix 测试前缀匹配
func TestFuzzyMatchPrefix(t *testing.T) {
	score, ok := FuzzyMatch("hel", "help")
	if !ok {
		t.Fatal("前缀匹配应返回 true")
	}
	if score < 500 {
		t.Fatalf("前缀匹配应高于 500 分，得到 %d", score)
	}
}

// TestFuzzyMatchSubstring 测试子串匹配
func TestFuzzyMatchSubstring(t *testing.T) {
	score, ok := FuzzyMatch("elp", "help")
	if !ok {
		t.Fatal("子串匹配应返回 true")
	}
	if score < 300 {
		t.Fatalf("子串匹配应高于 300 分，得到 %d", score)
	}
}

// TestFuzzyMatchCharSequence 测试字符序列匹配
func TestFuzzyMatchCharSequence(t *testing.T) {
	score, ok := FuzzyMatch("hp", "help")
	if !ok {
		t.Fatal("字符序列匹配 'hp' in 'help' 应返回 true")
	}
	if score <= 0 {
		t.Fatalf("字符序列匹配应返回正分数，得到 %d", score)
	}
}

// TestFuzzyMatchNoMatch 测试不匹配
func TestFuzzyMatchNoMatch(t *testing.T) {
	_, ok := FuzzyMatch("xyz", "help")
	if ok {
		t.Fatal("不匹配应返回 false")
	}
}

// TestFuzzyMatchEmptyQuery 测试空查询
func TestFuzzyMatchEmptyQuery(t *testing.T) {
	score, ok := FuzzyMatch("", "anything")
	if !ok {
		t.Fatal("空查询应匹配任何内容")
	}
	if score != 0 {
		t.Fatalf("空查询期望 0 分，得到 %d", score)
	}
}

// TestFuzzyMatchCaseInsensitive 测试大小写不敏感
func TestFuzzyMatchCaseInsensitive(t *testing.T) {
	_, ok := FuzzyMatch("HELP", "help")
	if !ok {
		t.Fatal("大小写不敏感匹配应返回 true")
	}

	_, ok = FuzzyMatch("help", "HELP")
	if !ok {
		t.Fatal("大小写不敏感匹配应返回 true")
	}
}

// TestFuzzyMatchEditDistance 测试编辑距离匹配
func TestFuzzyMatchEditDistance(t *testing.T) {
	// "hel" 和 "help" 距离为 1，相似度 0.75
	score, ok := FuzzyMatch("hel", "help")
	if !ok {
		t.Fatal("编辑距离接近的应匹配")
	}
	if score <= 0 {
		t.Fatalf("编辑距离匹配应有正分数，得到 %d", score)
	}
}

// TestFuzzyFilter 测试过滤
func TestFuzzyFilter(t *testing.T) {
	items := []string{"help", "clear", "theme", "quit", "config"}
	result := FuzzyFilter("he", items)
	if len(result) == 0 {
		t.Fatal("应有匹配结果")
	}
	if result[0] != "help" {
		t.Fatalf("'he' 的首先匹配应为 'help'，得到 %s", result[0])
	}
}

// TestFuzzyFilterEmptyQuery 测试空查询过滤
func TestFuzzyFilterEmptyQuery(t *testing.T) {
	items := []string{"a", "b", "c"}
	result := FuzzyFilter("", items)
	if len(result) != len(items) {
		t.Fatalf("空查询应返回所有项，期望 %d，得到 %d", len(items), len(result))
	}
}

// TestFuzzyFilterNoMatch 测试无匹配过滤
func TestFuzzyFilterNoMatch(t *testing.T) {
	items := []string{"help", "clear"}
	result := FuzzyFilter("xyz", items)
	if len(result) != 0 {
		t.Fatalf("无匹配应返回空列表，得到 %d", len(result))
	}
}

// TestFuzzyFilterOrder 测试排序
func TestFuzzyFilterOrder(t *testing.T) {
	items := []string{"theme-dark", "theme-light", "the", "other"}
	result := FuzzyFilter("the", items)
	if len(result) == 0 {
		t.Fatal("应有匹配结果")
	}
	// "the" 应排在 "theme-*" 前面（前缀匹配）
	if result[0] != "the" {
		t.Fatalf("精确前缀匹配应排第一，得到 %s", result[0])
	}
}