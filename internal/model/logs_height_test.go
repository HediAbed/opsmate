package model

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestLogsView_FitsInGivenSize(t *testing.T) {
	for _, h := range []int{20, 30, 40, 50} {
		for _, w := range []int{60, 100, 200} {
			m := NewLogsModel("default")
			m.SetSize(w, h)
			m.selectedPod = "mypod"
			lines := make([]string, 50)
			for i := range lines {
				lines[i] = strings.Repeat("X", w-4)
			}
			m.allLines = lines
			m.filteredLines = lines
			m.logView.SetContent(strings.Join(lines, "\n"))

			view := m.View()
			if rH := lipgloss.Height(view); rH > h {
				t.Errorf("Logs view w=%d h=%d → rendered_h=%d > terminal height", w, h, rH)
			}
			if rW := lipgloss.Width(view); rW > w {
				t.Errorf("Logs view w=%d h=%d → rendered_w=%d > terminal width", w, h, rW)
			}
		}
	}
}
