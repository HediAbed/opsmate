package ui

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

func TestDashboard_SpinnerTickIdleProducesNoFollowUp(t *testing.T) {
	m := newTestDashboardModel("default")
	m.loading = false
	m.healthAnalysisLoading = false

	_, cmd := m.Update(spinner.TickMsg{})
	if cmd != nil {
		t.Error("idle dashboard must not schedule another spinner tick")
	}
}

func TestLogs_SpinnerTickIdleProducesNoFollowUp(t *testing.T) {
	m := newTestLogsModel("default")
	m.loading = false
	m.lineExplanationLoading = false

	_, cmd := m.Update(spinner.TickMsg{})
	if cmd != nil {
		t.Error("idle logs must not schedule another spinner tick")
	}
}
