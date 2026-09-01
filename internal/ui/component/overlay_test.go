package component

import "testing"

func TestAnalysisOverlayBoundsKeepsOffsetsWhenPanelFits(t *testing.T) {
	top, height, bottom := AnalysisOverlayBounds(20, 3, 2)
	if top != 3 || height != 15 || bottom != 2 {
		t.Fatalf("AnalysisOverlayBounds(20, 3, 2) = (%d, %d, %d), want (3, 15, 2)", top, height, bottom)
	}
}

func TestAnalysisOverlayBoundsClampsToMinimumPanelHeight(t *testing.T) {
	top, height, bottom := AnalysisOverlayBounds(8, 4, 4)
	if top != 4 || height != MinimumAnalysisPanelHeight || bottom != 4 {
		t.Fatalf("AnalysisOverlayBounds(8, 4, 4) = (%d, %d, %d), want (4, %d, 4)", top, height, bottom, MinimumAnalysisPanelHeight)
	}
}
