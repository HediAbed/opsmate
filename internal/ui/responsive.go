package ui

import (
	"charm.land/bubbles/v2/viewport"

	"github.com/HediAbed/opsmate/internal/theme"
	"github.com/HediAbed/opsmate/tui"
)

const (
	popupHardGutter               = 1
	rootMinimumContentHeight      = 1
	rootHorizontalPadding         = 1
	rootStatusBarHeight           = 1
	rootViewSectionCapacity       = 3
	rootActivationCommandCapacity = 2
	analysisPanelWidthRatio       = 3
	analysisPanelMaximumRatio     = 2
	analysisPanelMinimumWidth     = 30
	allNamespacesPickerItemCount  = 1
	allNamespacesPickerIndex      = 0
	pickerChromeHeight            = 6
	pickerMinimumVisibleItems     = 5
	pickerItemTopOffset           = 3
	rootInputMinimumWidth         = 10
	rootInputHorizontalMargin     = 10
	rootSearchInputMaximumWidth   = 50
	pairedSides                   = 2
	percentageScale               = 100
	initialScreenWidth            = 80
	initialViewportHeight         = 20

	narrowHsplitMinWidth = 90

	browserTabStripMinWidth  = 10
	browserTabStripMinViable = 6
	titleBarSidePadding      = 2

	helpModalDesiredWidth        = 60
	searchModalDesiredWidth      = 64
	portForwardModalDesiredWidth = 72
	nsPickerModalDesiredWidth    = 44
	ctxPickerModalDesiredWidth   = 50
	confirmModalDesiredWidth     = 56
	scaleModalDesiredWidth       = 45
)

// popupScrollIndicator dims the raw indicator so popup chrome stays
// quiet; empty stays empty so callers can append it unconditionally.
func popupScrollIndicator(view viewport.Model) string {
	indicator := tui.ViewportScrollIndicator(view)
	if indicator == "" {
		return ""
	}
	return theme.Dim.Render(indicator)
}
