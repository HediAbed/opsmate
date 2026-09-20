package component

const MinimumAnalysisPanelHeight = 6

func AnalysisOverlayBounds(totalHeight, topOffset, bottomOffset int) (int, int, int) {
	topOffset = max(0, min(topOffset, max(0, totalHeight-MinimumAnalysisPanelHeight)))
	bottomOffset = max(0, min(bottomOffset, max(0, totalHeight-topOffset-MinimumAnalysisPanelHeight)))
	return topOffset, max(MinimumAnalysisPanelHeight, totalHeight-topOffset-bottomOffset), bottomOffset
}
