// Package command 提供命令面板系统
package command

import (
	"math"
	"sort"
	"strings"
)

// FuzzyMatch 使用编辑距离进行 fuzzy 匹配
// 返回匹配分数和是否匹配。分数越高表示匹配越好。
func FuzzyMatch(query, target string) (score int, matched bool) {
	if query == "" {
		return 0, true
	}
	if target == "" {
		return 0, false
	}

	q := strings.ToLower(query)
	t := strings.ToLower(target)

	// 精确匹配最高分
	if t == q {
		return 1000, true
	}

	// 前缀匹配高分
	if strings.HasPrefix(t, q) {
		return 500 + len(q), true
	}

	// 子串匹配
	if strings.Contains(t, q) {
		return 300 + len(q), true
	}

	// 逐字符匹配（按顺序匹配每个字符）
	charScore := matchChars(q, t)
	if charScore > 0 {
		return charScore, true
	}

	// 编辑距离匹配
	dist := levenshteinDistance(q, t)
	maxLen := len(q)
	if len(t) > maxLen {
		maxLen = len(t)
	}
	if maxLen == 0 {
		return 0, false
	}
	similarity := 1.0 - float64(dist)/float64(maxLen)
	if similarity >= 0.4 {
		score := int(math.Round(similarity * 100))
		return score, true
	}

	return 0, false
}

// FuzzyFilter 返回匹配的项，按分数降序排列
func FuzzyFilter(query string, items []string) []string {
	type match struct {
		item  string
		score int
	}

	var matches []match
	for _, item := range items {
		score, ok := FuzzyMatch(query, item)
		if ok {
			matches = append(matches, match{item: item, score: score})
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return matches[i].item < matches[j].item
	})

	result := make([]string, len(matches))
	for i, m := range matches {
		result[i] = m.item
	}
	return result
}

// matchChars 检查 query 中的字符是否按顺序出现在 target 中
func matchChars(query, target string) int {
	qi := 0
	ti := 0
	score := 0

	for qi < len(query) && ti < len(target) {
		if query[qi] == target[ti] {
			// 位置越靠前分数越高
			score += 100 - ti
			qi++
		}
		ti++
	}

	if qi == len(query) {
		return score
	}
	return 0
}

// levenshteinDistance 计算编辑距离
func levenshteinDistance(a, b string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	// 使用一维数组优化空间
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)

	for j := 0; j <= len(b); j++ {
		prev[j] = j
	}

	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(curr[j-1]+1, min(prev[j]+1, prev[j-1]+cost))
		}
		prev, curr = curr, prev
	}

	return prev[len(b)]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
