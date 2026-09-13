package fixbug

import "testing"

func TestAdd(t *testing.T) {
	if got := Add(2, 3); got != 5 {
		t.Fatalf("Add(2,3) = %d, 期望 5", got)
	}
	if got := Add(-1, 1); got != 0 {
		t.Fatalf("Add(-1,1) = %d, 期望 0", got)
	}
}

func TestMul(t *testing.T) {
	if got := Mul(3, 4); got != 12 {
		t.Fatalf("Mul(3,4) = %d, 期望 12", got)
	}
}
