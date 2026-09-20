package component

import "testing"

func TestAnalysisOverlayBoundsKeepsOffsetsWhenPanelFits(t *testing.T) {
	top, height, bottom := AnalysisOverlayBounds(20, 3, 2)
	if top != 3 || height != 15 || bottom != 2 {
		t.Fatalf("AnalysisOverlayBounds(20, 3, 2) = (%d, %d, %d), want (3, 15, 2)", top, height, bottom)
	}
}

func TestAnalysisOverlayBoundsNeverExceedTheAvailableHeight(t *testing.T) {
	for _, total := range []int{6, 8, 12, 40} {
		top, height, bottom := AnalysisOverlayBounds(total, 4, 4)
		if height < MinimumAnalysisPanelHeight {
			t.Errorf("total %d: panel height %d is below the %d row minimum", total, height, MinimumAnalysisPanelHeight)
		}
		if total >= MinimumAnalysisPanelHeight && top+height+bottom != total {
			t.Errorf("total %d: bounds %d/%d/%d do not fill the column exactly", total, top, height, bottom)
		}
		if top < 0 || bottom < 0 {
			t.Errorf("total %d: negative offsets %d/%d", total, top, bottom)
		}
	}
}
