package model

import (
	"testing"

	"charm.land/lipgloss/v2"
)

func TestAIPanelView_FitsInGivenHeight(t *testing.T) {
	for _, h := range []int{20, 30, 40, 50} {
		for _, w := range []int{60, 100, 200} {
			m := NewAIPanelModel()
			m.SetVisible(true)
			m.SetSize(w, h)
			view := m.View()
			rendered := lipgloss.Height(view)
			if rendered != h {
				t.Errorf("AI panel w=%d h=%d → rendered=%d (want %d)", w, h, rendered, h)
			}
		}
	}
}

func TestAIPanelView_FitsInGivenWidth(t *testing.T) {
	for _, w := range []int{60, 100, 200} {
		m := NewAIPanelModel()
		m.SetVisible(true)
		m.SetSize(w, 30)
		view := m.View()
		rendered := lipgloss.Width(view)
		want := w - 2
		if rendered != want {
			t.Errorf("AI panel w=%d → rendered_w=%d (want %d)", w, rendered, want)
		}
	}
}
