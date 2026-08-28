package model

import (
	"fmt"

	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"

	"github.com/HediAbed/opsmate/internal/theme"
)

const (
	modalSideGutter               = 2
	modalMinUsableWidth           = 12
	popupHardGutter               = 1
	rootMinimumContentHeight      = 1
	rootHorizontalPadding         = 1
	rootStatusBarHeight           = 1
	rootViewSectionCapacity       = 3
	rootActivationCommandCapacity = 2
	assistantPanelWidthRatio      = 3
	assistantPanelMaximumRatio    = 2
	assistantPanelMinimumWidth    = 30
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

func clampModalWidth(desired, terminal int) int {
	limit := terminal - pairedSides*modalSideGutter
	if limit < modalMinUsableWidth {
		return desired
	}
	if desired > limit {
		return limit
	}
	return desired
}

func viewportScrollPct(v viewport.Model) int {
	return int(v.ScrollPercent() * percentageScale)
}

// viewportScrollDirection returns "▲", "▼", "▲▼", or "" matching the
// scroll position so badges across screens stay visually consistent.
func viewportScrollDirection(v viewport.Model) string {
	if v.AtTop() && v.AtBottom() {
		return ""
	}
	switch {
	case v.AtTop():
		return "▼"
	case v.AtBottom():
		return "▲"
	default:
		return "▲▼"
	}
}

// popupScrollIndicator stays empty when content fits so the chrome
// stays quiet in the common case.
func popupScrollIndicator(v viewport.Model) string {
	dir := viewportScrollDirection(v)
	if dir == "" {
		return ""
	}
	hint := ""
	switch dir {
	case "▼":
		hint = " more below"
	case "▲":
		hint = " more above"
	}
	return theme.Dim.Render(fmt.Sprintf(" · %d%% %s%s", viewportScrollPct(v), dir, hint))
}

// appendHelpHint drops the hint silently when it would push the bar
// past terminal width — line-wrapping help text is worse than missing
// the secondary cue.
func appendHelpHint(helpBar, hint string, width int) string {
	combined := helpBar + " " + theme.Dim.Render(hint)
	if lipgloss.Width(combined) > width {
		return helpBar
	}
	return combined
}

// tableCursorLabel formats the cursor position for tables — bubbles/v2
// table.Model exposes no ScrollPercent equivalent, so "cursor/total"
// is the closest user-facing analogue.
func tableCursorLabel(cursor, total int) string {
	if total == 0 {
		return ""
	}
	return fmt.Sprintf("%d/%d", cursor+1, total)
}
