package tui

import (
	"fmt"

	"charm.land/bubbles/v2/viewport"
)

// ScrollDirection reports where off-screen viewport content lies.
type ScrollDirection int

const (
	ScrollNone ScrollDirection = iota
	ScrollAbove
	ScrollBelow
	ScrollBoth
)

// Arrows returns "▲", "▼", "▲▼", or "" when nothing is hidden.
func (d ScrollDirection) Arrows() string {
	if d == ScrollAbove {
		return "▲"
	}
	if d == ScrollBelow {
		return "▼"
	}
	if d == ScrollBoth {
		return "▲▼"
	}
	return ""
}

func (d ScrollDirection) moreHint() string {
	if d == ScrollAbove {
		return " more above"
	}
	if d == ScrollBelow {
		return " more below"
	}
	return ""
}

const scrollPercentScale = 100

// ViewportScrollPercent returns the scroll position scaled to 0-100.
func ViewportScrollPercent(view viewport.Model) int {
	return int(view.ScrollPercent() * scrollPercentScale)
}

// ViewportScrollDirection reports which directions hold hidden content.
func ViewportScrollDirection(view viewport.Model) ScrollDirection {
	switch {
	case view.AtTop() && view.AtBottom():
		return ScrollNone
	case view.AtTop():
		return ScrollBelow
	case view.AtBottom():
		return ScrollAbove
	default:
		return ScrollBoth
	}
}

// ViewportScrollIndicator formats the raw, unstyled position badge,
// e.g. " · 42% ▲▼", naming the hidden side at either edge. It stays
// empty while the content fits so quiet chrome stays quiet.
func ViewportScrollIndicator(view viewport.Model) string {
	direction := ViewportScrollDirection(view)
	if direction == ScrollNone {
		return ""
	}
	return fmt.Sprintf(" · %d%% %s%s",
		ViewportScrollPercent(view), direction.Arrows(), direction.moreHint())
}
