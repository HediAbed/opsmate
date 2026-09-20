package browser

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	clustermodel "github.com/HediAbed/opsmate/internal/cluster"
)

const (
	frameProbeWidth  = 183
	frameProbeHeight = 55
)

func TestTablePanelKeepsItsBottomBorderWhenFilled(t *testing.T) {
	model := newTestBrowserModel("kube-system")
	model.pods = []clustermodel.Pod{{Name: "a", Namespace: "ns"}}
	model.resourceType = resourceTypePods
	model.SetSize(frameProbeWidth, frameProbeHeight)
	model.statusMsg = "Loaded 1 pod"

	panel := model.renderTableContent(model.browserContentHeight())
	lastRow := panel[strings.LastIndex(panel, "\n")+1:]
	if !strings.Contains(lastRow, "╰") {
		t.Fatalf("panel bottom row = %q, want the bottom border", lastRow)
	}
	if got := lipgloss.Height(model.View()); got > frameProbeHeight {
		t.Fatalf("view height = %d, exceeds the %d rows it was given", got, frameProbeHeight)
	}
}
