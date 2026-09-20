package logs

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

const (
	frameProbeWidth  = 183
	frameProbeHeight = 55
	frameProbeLines  = 300
)

func TestLogPanelKeepsItsBottomBorderWhenFilled(t *testing.T) {
	model := newLogsForTest(t)
	model.SetSize(frameProbeWidth, frameProbeHeight)
	model.selectedPod = "web"
	lines := make([]string, frameProbeLines)
	for position := range lines {
		lines[position] = "log line"
	}
	model.allLines = lines
	model.applyFilter()
	model.syncLogContent()

	panel := model.renderLogPanel(model.logContentHeight())
	lastRow := panel[strings.LastIndex(panel, "\n")+1:]
	if !strings.Contains(lastRow, "╰") {
		t.Fatalf("panel bottom row = %q, want the bottom border", lastRow)
	}
	if got := lipgloss.Height(model.View()); got > frameProbeHeight {
		t.Fatalf("view height = %d, exceeds the %d rows it was given", got, frameProbeHeight)
	}
}
