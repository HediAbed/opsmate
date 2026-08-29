package tui

import "charm.land/lipgloss/v2"

// AppendHint returns bar with hint appended after a single space when
// their combined display width fits within width, and bar unchanged
// otherwise because a line-wrapped help bar is worse than a missing hint.
// An empty hint leaves bar untouched, and an empty bar takes a
// fitting hint without a leading space. Widths are ANSI-aware, so
// styled bars and hints measure correctly.
func AppendHint(bar, hint string, width int) string {
	if hint == "" {
		return bar
	}
	combined := hint
	if bar != "" {
		combined = bar + " " + hint
	}
	if lipgloss.Width(combined) > width {
		return bar
	}
	return combined
}
