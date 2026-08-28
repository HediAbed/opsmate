package model

const aiOverlayMinPanelHeight = 6

// aiOverlayBounds applies the shared minimum height to screen-specific chrome.
func aiOverlayBounds(totalHeight, topOffset, bottomOffset int) (int, int, int) {
	panelHeight := totalHeight - topOffset - bottomOffset
	if panelHeight < aiOverlayMinPanelHeight {
		panelHeight = aiOverlayMinPanelHeight
	}
	return topOffset, panelHeight, bottomOffset
}
