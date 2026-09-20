package dashboard

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/HediAbed/opsmate/internal/ui/screen"
)

const (
	overlayTestWidth  = 200
	overlayTestHeight = 40
)

func TestDashboardOverlayBoundsStartBelowTheHeaderAndEndAtTheBody(t *testing.T) {
	model := newTestDashboardModel("default")
	model.SetSize(overlayTestWidth, overlayTestHeight)

	topOffset, panelHeight, bottomOffset := model.AnalysisOverlayBounds(overlayTestHeight)
	sections := model.renderDashboardSections()
	wantTop := lipgloss.Height(sections.title) + lipgloss.Height(sections.overview)

	if topOffset != wantTop {
		t.Errorf("overlay starts at row %d, want %d so it lines up with the first panel", topOffset, wantTop)
	}
	if topOffset+panelHeight+bottomOffset != overlayTestHeight {
		t.Errorf("bounds %d/%d/%d do not fill %d rows", topOffset, panelHeight, bottomOffset, overlayTestHeight)
	}
	if bottomOffset < lipgloss.Height(sections.help) {
		t.Errorf("overlay bottom %d overlaps the help line", bottomOffset)
	}
}

func TestDashboardOverlayBoundsMakeRoomForTheErrorBanner(t *testing.T) {
	model := newTestDashboardModel("default")
	model.SetSize(overlayTestWidth, overlayTestHeight)
	withoutBanner, _, _ := model.AnalysisOverlayBounds(overlayTestHeight)

	model.err = screen.ErrLiveUpdatesStopped
	withBanner, _, _ := model.AnalysisOverlayBounds(overlayTestHeight)

	banner := model.renderDashboardSections().errorBanner
	if banner == "" || !strings.Contains(banner, "ERROR") {
		t.Fatalf("setup: expected an error banner, got %q", banner)
	}
	if withBanner != withoutBanner+lipgloss.Height(banner) {
		t.Errorf("overlay top = %d with a banner, want %d so the panel clears it", withBanner, withoutBanner+lipgloss.Height(banner))
	}
}
