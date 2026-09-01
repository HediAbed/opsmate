package component

import "testing"

func TestNewViewportAppliesSize(t *testing.T) {
	view := NewViewport(40, 10)
	if view.Width() != 40 {
		t.Errorf("Width() = %d, want 40", view.Width())
	}
	if view.Height() != 10 {
		t.Errorf("Height() = %d, want 10", view.Height())
	}
}
