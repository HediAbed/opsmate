package browser

import (
	"strings"
	"testing"
)

func TestBrowser_AnalysisOverlayBounds_SumsToTotal(t *testing.T) {
	for _, total := range []int{20, 40, 60, 80} {
		m := newTestBrowserModel("default")
		m.SetSize(200, total)
		topOff, panelH, bottomOff := m.AnalysisOverlayBounds(total)
		if topOff+panelH+bottomOff != total {
			t.Errorf("total=%d: top(%d)+panel(%d)+bottom(%d)=%d, want %d",
				total, topOff, panelH, bottomOff, topOff+panelH+bottomOff, total)
		}
		if panelH < 6 {
			t.Errorf("total=%d: panelH=%d below floor 6", total, panelH)
		}
	}
}

func TestBrowserOverlayBoundsCoverExactlyTheRenderedTableRows(t *testing.T) {
	m := newBrowserWithMarkerPod(t, "default")

	topOffset, panelHeight, bottomOffset := m.AnalysisOverlayBounds(40)

	var boxTop, boxBottom = -1, -1
	for index, row := range strings.Split(m.View(), "\n") {
		if strings.Contains(row, "\u256d") && boxTop < 0 {
			boxTop = index
		}
		if strings.Contains(row, "\u2570") {
			boxBottom = index
		}
	}

	if topOffset != boxTop {
		t.Errorf("overlay starts at row %d but the table panel starts at row %d", topOffset, boxTop)
	}
	if got := topOffset + panelHeight - 1; got != boxBottom {
		t.Errorf("overlay ends at row %d but the table panel ends at row %d", got, boxBottom)
	}
	if topOffset+panelHeight+bottomOffset != 40 {
		t.Errorf("bounds %d/%d/%d do not fill 40 rows", topOffset, panelHeight, bottomOffset)
	}
}
