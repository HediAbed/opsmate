package component

import "charm.land/lipgloss/v2"

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
