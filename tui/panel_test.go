package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func framedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
}

func marginedStyle() lipgloss.Style {
	return framedStyle().Margin(1, 2)
}

func assertRenderedSize(t *testing.T, out string, want Size) {
	t.Helper()
	if w := lipgloss.Width(out); w != want.Width {
		t.Errorf("rendered width = %d, want %d", w, want.Width)
	}
	if h := lipgloss.Height(out); h != want.Height {
		t.Errorf("rendered height = %d, want %d", h, want.Height)
	}
}

func TestPanelContentSize_SubtractsFrame(t *testing.T) {
	got := NewPanel(framedStyle()).ContentSize(Size{Width: 20, Height: 5})
	want := Size{Width: 16, Height: 3}
	if got != want {
		t.Errorf("ContentSize(20x5) = %+v, want %+v", got, want)
	}
}

func TestPanelContentSize_IncludesMargins(t *testing.T) {
	got := NewPanel(marginedStyle()).ContentSize(Size{Width: 20, Height: 7})
	want := Size{Width: 12, Height: 3}
	if got != want {
		t.Errorf("ContentSize(20x7) = %+v, want %+v", got, want)
	}
}

func TestPanelContentSize_ClampsAtZeroOnTinyOuter(t *testing.T) {
	got := NewPanel(framedStyle()).ContentSize(Size{Width: 3, Height: 1})
	want := Size{Width: 0, Height: 0}
	if got != want {
		t.Errorf("ContentSize(3x1) = %+v, want %+v", got, want)
	}
}

func TestPanelRender_FillsOuterExactly(t *testing.T) {
	out := NewPanel(framedStyle()).Render(Size{Width: 20, Height: 5}, "hello")
	assertRenderedSize(t, out, Size{Width: 20, Height: 5})
	if !strings.Contains(out, "hello") {
		t.Errorf("rendered panel must contain the content; got %q", out)
	}
}

func TestPanelRender_MarginsCountTowardOuter(t *testing.T) {
	out := NewPanel(marginedStyle()).Render(Size{Width: 20, Height: 7}, "hello")
	assertRenderedSize(t, out, Size{Width: 20, Height: 7})
	if !strings.Contains(out, "hello") {
		t.Errorf("rendered panel must contain the content; got %q", out)
	}
}

func TestPanelRender_ClipsTallContentToOuter(t *testing.T) {
	content := strings.TrimSuffix(strings.Repeat("line\n", 9), "\n")
	out := NewPanel(framedStyle()).Render(Size{Width: 20, Height: 4}, content)
	assertRenderedSize(t, out, Size{Width: 20, Height: 4})
}

func TestPanelRender_ClipsLongHorizontalContentToOuter(t *testing.T) {
	out := NewPanel(framedStyle()).Render(Size{Width: 12, Height: 3}, strings.Repeat("x", 200))
	if w := lipgloss.Width(out); w > 12 {
		t.Errorf("rendered width = %d, must not exceed 12", w)
	}
	if h := lipgloss.Height(out); h > 3 {
		t.Errorf("rendered height = %d, must not exceed 3", h)
	}
}

func TestPanelRender_ClipsOverflowingContentInMarginedPanel(t *testing.T) {
	content := strings.TrimSuffix(strings.Repeat(strings.Repeat("y", 50)+"\n", 9), "\n")
	out := NewPanel(marginedStyle()).Render(Size{Width: 20, Height: 7}, content)
	if w := lipgloss.Width(out); w > 20 {
		t.Errorf("rendered width = %d, must not exceed 20", w)
	}
	if h := lipgloss.Height(out); h > 7 {
		t.Errorf("rendered height = %d, must not exceed 7", h)
	}
}

func TestPanelRender_SmallestValidOuterHoldsOneCell(t *testing.T) {
	out := NewPanel(framedStyle()).Render(Size{Width: 5, Height: 3}, "z")
	assertRenderedSize(t, out, Size{Width: 5, Height: 3})
	if !strings.Contains(out, "z") {
		t.Errorf("rendered panel must contain the content; got %q", out)
	}
}

func TestPanelRender_EmptyWhenWidthCannotHoldFrame(t *testing.T) {
	if out := NewPanel(framedStyle()).Render(Size{Width: 4, Height: 5}, "hello"); out != "" {
		t.Errorf("width at frame size must render nothing; got %q", out)
	}
}

func TestPanelRender_EmptyWhenHeightCannotHoldFrame(t *testing.T) {
	if out := NewPanel(framedStyle()).Render(Size{Width: 20, Height: 2}, "hello"); out != "" {
		t.Errorf("height at frame size must render nothing; got %q", out)
	}
}

func TestPanelRender_EmptyOnZeroOuter(t *testing.T) {
	if out := NewPanel(framedStyle()).Render(Size{}, "hello"); out != "" {
		t.Errorf("zero outer must render nothing; got %q", out)
	}
}

func TestPanelRender_EmptyOnNegativeOuter(t *testing.T) {
	if out := NewPanel(framedStyle()).Render(Size{Width: -5, Height: -2}, "hello"); out != "" {
		t.Errorf("negative outer must render nothing; got %q", out)
	}
}

func TestPanelRender_FramelessStyleUsesWholeOuter(t *testing.T) {
	out := NewPanel(lipgloss.NewStyle()).Render(Size{Width: 3, Height: 1}, "x")
	assertRenderedSize(t, out, Size{Width: 3, Height: 1})
}
