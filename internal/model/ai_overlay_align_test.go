package model

import (
	"testing"

	"charm.land/lipgloss/v2"
)

func TestBrowser_AIOverlayBounds_SumsToTotal(t *testing.T) {
	for _, total := range []int{20, 40, 60, 80} {
		m := NewBrowserModel("default")
		m.SetSize(200, total)
		topOff, panelH, bottomOff := m.AIOverlayBounds(total)
		if topOff+panelH+bottomOff != total {
			t.Errorf("total=%d: top(%d)+panel(%d)+bottom(%d)=%d, want %d",
				total, topOff, panelH, bottomOff, topOff+panelH+bottomOff, total)
		}
		if panelH < 6 {
			t.Errorf("total=%d: panelH=%d below floor 6", total, panelH)
		}
	}
}

func TestLogs_AIOverlayBounds_SumsToTotal(t *testing.T) {
	for _, total := range []int{20, 40, 60, 80} {
		m := NewLogsModel("default")
		m.SetSize(200, total)
		topOff, panelH, bottomOff := m.AIOverlayBounds(total)
		if topOff+panelH+bottomOff != total {
			t.Errorf("total=%d: top(%d)+panel(%d)+bottom(%d)=%d, want %d",
				total, topOff, panelH, bottomOff, topOff+panelH+bottomOff, total)
		}
	}
}

func TestBrowser_AIOverlayBounds_MatchesActualChrome(t *testing.T) {
	m := NewBrowserModel("default")
	m.SetSize(200, 40)
	topOff, _, bottomOff := m.AIOverlayBounds(40)

	wantTop := lipgloss.Height(m.renderTitleBar())
	if filter := m.renderFilterBar(); filter != "" {
		wantTop += lipgloss.Height(filter)
	}
	if errBan := m.renderErrBanner(); errBan != "" {
		wantTop += lipgloss.Height(errBan)
	}
	wantBottom := lipgloss.Height(m.renderStatusLine()) + lipgloss.Height(m.renderHelpBar())

	if topOff != wantTop {
		t.Errorf("topOffset=%d, want %d (matching browser's pre-content chrome)", topOff, wantTop)
	}
	if bottomOff != wantBottom {
		t.Errorf("bottomOffset=%d, want %d (matching browser's post-content chrome)", bottomOff, wantBottom)
	}
}
