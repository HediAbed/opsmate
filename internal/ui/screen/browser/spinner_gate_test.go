package browser

import (
	"testing"

	"charm.land/bubbles/v2/spinner"
)

func TestBrowser_SpinnerTickIdleProducesNoFollowUp(t *testing.T) {
	m := newTestBrowserModel("default")
	m.loading = false
	m.analysisSummaryLoading = false

	_, cmd := m.Update(spinner.TickMsg{})
	if cmd != nil {
		t.Error("idle browser must not schedule another spinner tick")
	}
}

func TestBrowser_SpinnerTickWhileLoadingKeepsTicking(t *testing.T) {
	m := newTestBrowserModel("default")
	m.loading = true

	_, cmd := m.Update(spinner.TickMsg{})
	if cmd == nil {
		t.Error("loading browser must keep the spinner ticking")
	}
}
