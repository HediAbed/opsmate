package dashboard

import (
	"testing"

	"charm.land/bubbles/v2/spinner"
)

func TestDashboardSpinnerTickIdleProducesNoFollowUp(t *testing.T) {
	m := newTestDashboardModel("default")
	m.loading = false
	m.healthAnalysisLoading = false

	_, command := m.Update(spinner.TickMsg{})
	if command != nil {
		t.Error("idle dashboard scheduled another spinner tick")
	}
}
