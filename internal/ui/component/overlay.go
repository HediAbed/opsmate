package component

const MinimumAnalysisPanelHeight = 6

func AnalysisOverlayBounds(totalHeight, topOffset, bottomOffset int) (int, int, int) {
	panelHeight := totalHeight - topOffset - bottomOffset
	if panelHeight < MinimumAnalysisPanelHeight {
		panelHeight = MinimumAnalysisPanelHeight
	}
	return topOffset, panelHeight, bottomOffset
}
