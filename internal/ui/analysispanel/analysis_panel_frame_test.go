package analysispanel

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

const framePanelWidth = 46

func TestPanelKeepsItsBottomBorderAtEveryHeight(t *testing.T) {
	for _, height := range []int{20, 34, 52} {
		model := NewAnalysisPanelModel()
		model.SetVisible(true)
		model.SetSize(framePanelWidth, height)

		view := model.View()
		if got := lipgloss.Height(view); got != height {
			t.Fatalf("height %d rendered %d rows", height, got)
		}
		lastRow := view[strings.LastIndex(view, "\n")+1:]
		if !strings.Contains(lastRow, "╰") {
			t.Fatalf("height %d bottom row = %q, want the bottom border", height, lastRow)
		}
	}
}
