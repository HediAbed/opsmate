package component

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/viewport"
)

func TestPopupScrollIndicatorIsEmptyWhenContentFits(t *testing.T) {
	view := viewport.New()
	view.SetWidth(40)
	view.SetHeight(20)
	view.SetContent("line\n")
	if got := PopupScrollIndicator(view); got != "" {
		t.Errorf("scroll indicator = %q, want empty", got)
	}
}

func TestPopupScrollIndicatorShowsContentBelow(t *testing.T) {
	view := overflowingPopupView()
	view.GotoTop()
	got := PopupScrollIndicator(view)
	if !strings.Contains(got, "▼") || !strings.Contains(got, "more below") {
		t.Errorf("top scroll indicator = %q", got)
	}
}

func TestPopupScrollIndicatorShowsContentAbove(t *testing.T) {
	view := overflowingPopupView()
	view.GotoBottom()
	got := PopupScrollIndicator(view)
	if !strings.Contains(got, "▲") || !strings.Contains(got, "more above") {
		t.Errorf("bottom scroll indicator = %q", got)
	}
}

func TestPopupScrollIndicatorShowsBothDirections(t *testing.T) {
	view := overflowingPopupView()
	view.SetYOffset(10)
	if got := PopupScrollIndicator(view); !strings.Contains(got, "▲▼") {
		t.Errorf("middle scroll indicator = %q", got)
	}
}

func overflowingPopupView() viewport.Model {
	view := viewport.New()
	view.SetWidth(40)
	view.SetHeight(3)
	view.SetContent(strings.Repeat("line\n", 30))
	return view
}
