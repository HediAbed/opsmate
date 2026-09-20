package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/HediAbed/opsmate/internal/ui/screen"
)

const (
	alignmentWidth      = 183
	alignmentHeight     = 56
	alignmentPanelStart = 120
)

func borderRows(rows []string, from, to int) (top, bottom int) {
	top, bottom = -1, -1
	for index, row := range rows {
		runes := []rune(row)
		if from >= len(runes) {
			continue
		}
		segment := string(runes[from:min(to, len(runes))])
		if strings.Contains(segment, "╭") && top < 0 {
			top = index
		}
		if strings.Contains(segment, "╰") {
			bottom = index
		}
	}
	return top, bottom
}

func TestAnalysisPanelAlignsWithTheScreenBesideIt(t *testing.T) {
	for _, active := range []screen.ID{ScreenDashboard, ScreenLogs} {
		model := newTestRootModel(t, "kube-system")
		model.width, model.height, model.ready = alignmentWidth, alignmentHeight, true
		model.screen = active
		model.analysisPanel.SetVisible(true)
		model.resizeChildren()

		rows := strings.Split(model.renderActiveScreen(model.height-lipgloss.Height(model.renderRootFooter())), "\n")
		mainTop, mainBottom := borderRows(rows, 0, alignmentPanelStart)
		panelTop, panelBottom := borderRows(rows, alignmentPanelStart, alignmentWidth)

		if panelTop < 0 || panelBottom < 0 {
			t.Fatalf("screen %v: analysis panel drew no box (top=%d bottom=%d)", active, panelTop, panelBottom)
		}
		if panelTop != mainTop {
			t.Errorf("screen %v: panel starts on row %d but the screen starts on row %d", active, panelTop, mainTop)
		}
		if panelBottom != mainBottom {
			t.Errorf("screen %v: panel ends on row %d but the screen ends on row %d", active, panelBottom, mainBottom)
		}
	}
}
