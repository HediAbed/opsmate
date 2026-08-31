package ui

const analysisOverlayMinimumPanelHeight = 6

func analysisOverlayBounds(totalHeight, topOffset, bottomOffset int) (int, int, int) {
	panelHeight := totalHeight - topOffset - bottomOffset
	if panelHeight < analysisOverlayMinimumPanelHeight {
		panelHeight = analysisOverlayMinimumPanelHeight
	}
	return topOffset, panelHeight, bottomOffset
}
