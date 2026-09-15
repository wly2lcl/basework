package main

import "testing"

func TestAdd(t *testing.T) {
	for _, tc := range []struct {
		a, b int
		want int
	}{
		{2, 3, 5},
		{0, 7, 7},
		{-2, 3, 1},
	} {
		if got := Add(tc.a, tc.b); got != tc.want {
			t.Fatalf("Add(%d, %d) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
