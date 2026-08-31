package component

import (
	"fmt"

	"charm.land/bubbles/v2/viewport"
)

type ScrollDirection int

const (
	ScrollNone ScrollDirection = iota
	ScrollAbove
	ScrollBelow
	ScrollBoth
)

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

func ViewportScrollPercent(view viewport.Model) int {
	return int(view.ScrollPercent() * scrollPercentScale)
}

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

func ViewportScrollIndicator(view viewport.Model) string {
	direction := ViewportScrollDirection(view)
	if direction == ScrollNone {
		return ""
	}
	return fmt.Sprintf(" · %d%% %s%s",
		ViewportScrollPercent(view), direction.Arrows(), direction.moreHint())
}
