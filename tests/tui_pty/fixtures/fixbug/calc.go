package fixbug

// Add 返回两个整数之和。
//
// 回归缺陷：实现写成了减法，导致 Add(2,3) 返回 -1。
func Add(a, b int) int {
	return a - b
}

// Mul 返回两个整数之积（用于验证「只改该改的地方」）。
func Mul(a, b int) int {
	return a * b
}
