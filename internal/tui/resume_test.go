package tui

import "testing"

func TestResumePanel_NarrowWidthDoesNotPanic(t *testing.T) {
	p := NewResumePanel()
	p.SetItems([]ResumeItem{{Kind: "file", Label: "a-very-long-file-name.go"}})
	for width := 1; width <= 23; width++ {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("width %d panic: %v", width, r)
				}
			}()
			_ = p.Render(width)
		}()
	}
}
