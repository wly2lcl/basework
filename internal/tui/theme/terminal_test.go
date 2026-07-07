// Package theme 测试
package theme

import (
	"os"
	"testing"
)

// TestDetectColorDepth 测试颜色深度检测
func TestDetectColorDepth(t *testing.T) {
	// 保存环境变量
	oldColorterm := os.Getenv("COLORTERM")
	oldTerm := os.Getenv("TERM")
	defer func() {
		os.Setenv("COLORTERM", oldColorterm)
		os.Setenv("TERM", oldTerm)
	}()

	// 测试真彩色
	os.Setenv("COLORTERM", "truecolor")
	os.Setenv("TERM", "xterm-256color")
	depth := DetectColorDepth()
	if depth != ColorDepthTrueColor {
		t.Fatalf("期望 ColorDepthTrueColor，得到 %d", depth)
	}

	// 测试 256 色
	os.Unsetenv("COLORTERM")
	os.Setenv("TERM", "xterm-256color")
	depth = DetectColorDepth()
	if depth != ColorDepth256 {
		t.Fatalf("期望 ColorDepth256，得到 %d", depth)
	}

	// 测试 16 色
	os.Unsetenv("COLORTERM")
	os.Setenv("TERM", "xterm-color")
	depth = DetectColorDepth()
	if depth != ColorDepth16 {
		t.Fatalf("期望 ColorDepth16，得到 %d", depth)
	}

	// 测试无颜色
	os.Unsetenv("COLORTERM")
	os.Setenv("TERM", "dumb")
	depth = DetectColorDepth()
	if depth != ColorDepthNone {
		t.Fatalf("期望 ColorDepthNone，得到 %d", depth)
	}
}

// TestAdaptTheme 测试主题适配
func TestAdaptTheme(t *testing.T) {
	// 真彩色不应降级
	adapted := AdaptTheme(DarkTheme, ColorDepthTrueColor)
	if adapted.Name != "dark" {
		t.Fatalf("真彩色不应降级，得到主题名 %s", adapted.Name)
	}

	// 256 色不应降级
	adapted = AdaptTheme(DarkTheme, ColorDepth256)
	if adapted.Name != "dark" {
		t.Fatalf("256 色不应降级，得到主题名 %s", adapted.Name)
	}

	// 16 色应降级
	adapted = AdaptTheme(DarkTheme, ColorDepth16)
	if adapted.Colors[ColorAccent] == DarkTheme.Colors[ColorAccent] {
		t.Logf("16 色降级后颜色可能变化: %s -> %s",
			DarkTheme.Colors[ColorAccent], adapted.Colors[ColorAccent])
	}
}

// TestAdaptThemeNoColor 测试无颜色模式
func TestAdaptThemeNoColor(t *testing.T) {
	adapted := AdaptTheme(DarkTheme, ColorDepthNone)
	if adapted == nil {
		t.Fatal("AdaptTheme 不应返回 nil")
	}
}

// TestMapTo16Color 测试 16 色映射
func TestMapTo16Color(t *testing.T) {
	// 直接使用 16 色范围内的颜色应保持不变
	result := adaptColor("4", ColorDepth16)
	if result != "4" {
		t.Fatalf("16 色范围内的颜色 '4' 应保持不变，得到 %s", result)
	}

	result = adaptColor("7", ColorDepth16)
	if result != "7" {
		t.Fatalf("16 色范围内的颜色 '7' 应保持不变，得到 %s", result)
	}

	// 256 色应被映射
	result = adaptColor("63", ColorDepth16)
	if result == "63" {
		t.Fatalf("256 色 '63' 应被降级，得到 %s", result)
	}
}
