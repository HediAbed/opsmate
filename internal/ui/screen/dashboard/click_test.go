package dashboard

import (
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/cluster"
)

const (
	dashboardClickTestRowCount = 5
	dashboardClickTargetRow    = 2
)

func TestDashboardPodTableTopBoundary(t *testing.T) {
	model := newTestDashboardModel("default")
	model.SetSize(120, 40)
	seedDashboardClickPods(&model)
	headerY := renderedRowY(t, model, "NAME")
	if got := model.podTableTopBoundary(); got != headerY {
		t.Errorf("pod table top boundary = %d, want rendered header row %d", got, headerY)
	}
}

func TestDashboardMouseClickInTableAreaSelectsRow(t *testing.T) {
	model := newTestDashboardModel("default")
	model.SetSize(120, 40)
	seedDashboardClickPods(&model)
	clickRow := renderedRowY(t, model, "pod-2")

	updated, _ := model.Update(tea.MouseClickMsg{X: 10, Y: clickRow, Button: tea.MouseLeft})
	if got := updated.podTable.Cursor(); got != dashboardClickTargetRow {
		t.Errorf("selected row = %d, want %d", got, dashboardClickTargetRow)
	}
}

func TestDashboardMouseClickAboveTableIsIgnored(t *testing.T) {
	model := newTestDashboardModel("default")
	model.SetSize(120, 40)
	seedDashboardClickPods(&model)
	model.podTable.SetCursor(0)

	updated, _ := model.Update(tea.MouseClickMsg{X: 10, Y: 1, Button: tea.MouseLeft})
	if got := updated.podTable.Cursor(); got != 0 {
		t.Errorf("selected row = %d, want 0", got)
	}
}

func TestDashboardNonLeftMouseClickIsIgnored(t *testing.T) {
	model := newTestDashboardModel("default")
	model.SetSize(120, 40)
	seedDashboardClickPods(&model)
	model.podTable.SetCursor(0)
	clickRow := renderedRowY(t, model, "pod-2")

	for _, button := range []tea.MouseButton{tea.MouseRight, tea.MouseMiddle} {
		updated, _ := model.Update(tea.MouseClickMsg{X: 10, Y: clickRow, Button: button})
		if got := updated.podTable.Cursor(); got != 0 {
			t.Errorf("button %v selected row %d", button, got)
		}
	}
}

func renderedRowY(t *testing.T, model DashboardModel, marker string) int {
	t.Helper()
	for row, line := range strings.Split(stripAnsiForTest(model.View()), "\n") {
		if strings.Contains(line, marker) {
			return row
		}
	}
	t.Fatalf("rendered view has no row containing %q", marker)
	return 0
}

func seedDashboardClickPods(model *DashboardModel) {
	pods := make([]cluster.Pod, dashboardClickTestRowCount)
	for index := range pods {
		pods[index] = cluster.Pod{Name: "pod-" + strconv.Itoa(index), Namespace: "default", Status: "Running"}
	}
	model.pods = pods
	model.loading = false
	model.rebuildTableRows()
}
