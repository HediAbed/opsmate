package component

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/viewport"
)

func fittingViewport() viewport.Model {
	v := viewport.New()
	v.SetWidth(40)
	v.SetHeight(3)
	v.SetContent("only one\nshort\nfile")
	return v
}

func overflowingViewport() viewport.Model {
	v := viewport.New()
	v.SetWidth(40)
	v.SetHeight(3)
	v.SetContent(strings.Repeat("line\n", 30))
	return v
}

func TestViewportScrollDirection(t *testing.T) {
	if got := ViewportScrollDirection(fittingViewport()); got != ScrollNone {
		t.Errorf("fitting content must yield ScrollNone; got %v", got)
	}

	v := overflowingViewport()
	v.GotoTop()
	if got := ViewportScrollDirection(v); got != ScrollBelow {
		t.Errorf("at top with more below should be ScrollBelow; got %v", got)
	}
	v.GotoBottom()
	if got := ViewportScrollDirection(v); got != ScrollAbove {
		t.Errorf("at bottom with more above should be ScrollAbove; got %v", got)
	}
	v.SetYOffset(10)
	if got := ViewportScrollDirection(v); got != ScrollBoth {
		t.Errorf("middle should be ScrollBoth; got %v", got)
	}
}

func TestScrollDirectionArrows(t *testing.T) {
	cases := []struct {
		direction ScrollDirection
		want      string
	}{
		{ScrollNone, ""},
		{ScrollAbove, "▲"},
		{ScrollBelow, "▼"},
		{ScrollBoth, "▲▼"},
		{ScrollDirection(99), ""},
	}
	for _, c := range cases {
		if got := c.direction.Arrows(); got != c.want {
			t.Errorf("Arrows(%v) = %q, want %q", c.direction, got, c.want)
		}
	}
	if got := ScrollDirection(99).moreHint(); got != "" {
		t.Fatalf("invalid scroll direction hint = %q, want empty", got)
	}
}

func TestViewportScrollPercent_EdgesAndBounds(t *testing.T) {
	v := overflowingViewport()
	v.GotoTop()
	if got := ViewportScrollPercent(v); got != 0 {
		t.Errorf("at top percent should be 0; got %d", got)
	}
	v.GotoBottom()
	if got := ViewportScrollPercent(v); got != 100 {
		t.Errorf("at bottom percent should be 100; got %d", got)
	}
	v.SetYOffset(10)
	if got := ViewportScrollPercent(v); got <= 0 || got >= 100 {
		t.Errorf("mid-scroll percent should sit strictly between 0 and 100; got %d", got)
	}
}

func TestViewportScrollIndicator_EmptyWhenFits(t *testing.T) {
	if got := ViewportScrollIndicator(fittingViewport()); got != "" {
		t.Errorf("indicator on fitting content should be empty; got %q", got)
	}
}

func TestViewportScrollIndicator_AtTop(t *testing.T) {
	v := overflowingViewport()
	v.GotoTop()
	if got := ViewportScrollIndicator(v); got != " · 0% ▼ more below" {
		t.Errorf("at top indicator = %q, want %q", got, " · 0% ▼ more below")
	}
}

func TestViewportScrollIndicator_AtBottom(t *testing.T) {
	v := overflowingViewport()
	v.GotoBottom()
	if got := ViewportScrollIndicator(v); got != " · 100% ▲ more above" {
		t.Errorf("at bottom indicator = %q, want %q", got, " · 100% ▲ more above")
	}
}

func TestViewportScrollIndicator_Middle(t *testing.T) {
	v := overflowingViewport()
	v.SetYOffset(10)
	got := ViewportScrollIndicator(v)
	if !strings.HasPrefix(got, " · ") || !strings.Contains(got, "▲▼") {
		t.Errorf("mid-scroll indicator should carry both glyphs; got %q", got)
	}
	if strings.Contains(got, "more") {
		t.Errorf("mid-scroll indicator must not name a single side; got %q", got)
	}
}
