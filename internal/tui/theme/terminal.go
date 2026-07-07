// Package theme 提供 TUI 主题系统
package theme

import (
	"fmt"
	"os"
)

// ColorDepth 表示终端支持的颜色深度
type ColorDepth int

const (
	ColorDepthNone      ColorDepth = 0  // 无颜色支持
	ColorDepth16        ColorDepth = 4  // 16 色
	ColorDepth256       ColorDepth = 8  // 256 色
	ColorDepthTrueColor ColorDepth = 24 // 真彩色
)

// DetectColorDepth 检测终端颜色能力
// 检查 COLORTERM 和 TERM 环境变量
func DetectColorDepth() ColorDepth {
	colorterm := os.Getenv("COLORTERM")
	switch colorterm {
	case "truecolor", "24bit":
		return ColorDepthTrueColor
	case "256color":
		return ColorDepth256
	}

	term := os.Getenv("TERM")
	switch {
	case contains(term, "truecolor"), contains(term, "24bit"):
		return ColorDepthTrueColor
	case contains(term, "256color"):
		return ColorDepth256
	case contains(term, "color"):
		return ColorDepth16
	case term == "dumb", term == "":
		return ColorDepthNone
	default:
		return ColorDepth16
	}
}

// AdaptTheme 根据终端能力降级颜色
// 将不在终端支持范围内的颜色替换为最接近的可用颜色
func AdaptTheme(t *Theme, depth ColorDepth) *Theme {
	if depth >= ColorDepth256 {
		// 256 色及以上可以直接使用
		return t
	}

	adapted := &Theme{
		Name:   t.Name + "_adapted",
		Colors: make(map[ColorRole]string, len(t.Colors)),
	}

	for role, color := range t.Colors {
		adapted.Colors[role] = adaptColor(color, depth)
	}

	return adapted
}

// adaptColor 将颜色值适应到指定深度
func adaptColor(color string, depth ColorDepth) string {
	if depth >= ColorDepth256 {
		return color
	}

	// 如果是 16 色范围内的颜色，直接保留
	switch color {
	case "0", "1", "2", "3", "4", "5", "6", "7",
		"8", "9", "10", "11", "12", "13", "14", "15":
		return color
	}

	// 降级到 16 色范围内的近似色
	if depth == ColorDepth16 {
		return mapTo16Color(color)
	}

	return color
}

// mapTo16Color 将 256 色映射到 16 色范围
func mapTo16Color(color string) string {
	// 常见颜色的简单映射
	// 对于 256 色，216-255 是亮色/灰色系
	switch color {
	case "16", "17", "18", "19", "20", "21", "22", "23", "24", "25", "26", "27":
		return "4" // 蓝色
	case "28", "29", "30", "31", "32", "33", "34", "35", "36", "37", "38", "39":
		return "6" // 青色
	case "40", "41", "42", "43", "44", "45", "46", "47", "48", "49", "50", "51":
		return "2" // 绿色
	case "52", "53", "54", "55", "56", "57", "58", "59", "60", "61", "62", "63":
		return "5" // 紫色
	case "88", "89", "90", "91", "92", "93", "94", "95", "96", "97", "98", "99":
		return "5"
	case "124", "125", "126", "127", "128", "129", "130", "131", "132", "133", "134", "135":
		return "1" // 红色
	case "136", "137", "138", "139", "140", "141":
		return "3" // 黄色
	case "142", "143", "144", "145", "146", "147":
		return "3"
	case "148", "149", "150", "151", "152", "153":
		return "2"
	case "154", "155", "156", "157", "158", "159":
		return "2"
	case "160", "161", "162", "163", "164", "165":
		return "1"
	case "166", "167", "168", "169", "170", "171":
		return "3"
	case "172", "173", "174", "175", "176", "177":
		return "3"
	case "178", "179", "180", "181", "182", "183":
		return "3"
	case "184", "185", "186", "187", "188", "189":
		return "7" // 白色
	case "190", "191", "192", "193", "194", "195":
		return "7"
	case "196", "197", "198", "199", "200", "201":
		return "1"
	case "202", "203", "204", "205", "206", "207":
		return "1"
	case "208", "209", "210", "211", "212", "213":
		return "3"
	case "214", "215", "216", "217", "218", "219":
		return "3"
	case "220", "221", "222", "223", "224", "225":
		return "3"
	case "226", "227", "228", "229", "230", "231":
		return "7"
	}

	// 灰色系 (232-255)
	num := 0
	if _, err := fmt.Sscanf(color, "%d", &num); err == nil && num >= 232 {
		if num < 244 {
			return "0" // 深灰 -> 黑色
		}
		return "7" // 浅灰 -> 白色
	}

	// 默认返回白色
	return "7"
}

// contains 检查字符串是否包含子串
func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

// searchSubstring 简单子串搜索
func searchSubstring(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if s[i+j] != substr[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
