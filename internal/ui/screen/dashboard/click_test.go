package dashboard

import (
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/cluster"
)

const (
	dashboardClickTestRowCount      = 5
	dashboardClickTargetRow         = 2
	dashboardScrolledClickTargetRow = 2
	dashboardScrollDeploymentCount  = 6
	dashboardScrollEventCount       = 4
	dashboardScrollTerminalWidth    = 120
	dashboardScrollTerminalHeight   = 20
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

func TestDashboardMouseClickTracksScrolledBodyOffset(t *testing.T) {
	model := newTestDashboardModel("default")
	seedDashboardScrollableBody(&model)
	model.SetSize(dashboardScrollTerminalWidth, dashboardScrollTerminalHeight)
	if !model.bodyOverflows() {
		t.Fatalf("body must overflow to scroll: %d content lines in a %d line viewport",
			model.bodyView.TotalLineCount(), model.bodyView.Height())
	}

	scrolled, _ := model.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	offset := scrolled.bodyView.YOffset()
	if offset == 0 {
		t.Fatal("mouse wheel did not scroll the dashboard body")
	}

	clickRow := renderedRowY(t, scrolled, "NAME") + dashTableHeaderRows + dashboardScrolledClickTargetRow
	updated, _ := scrolled.Update(tea.MouseClickMsg{X: 10, Y: clickRow, Button: tea.MouseLeft})
	if got := updated.podTable.Cursor(); got != dashboardScrolledClickTargetRow {
		t.Errorf("selected row = %d, want %d; the click landed on the rendered table row while the body was scrolled %d lines",
			got, dashboardScrolledClickTargetRow, offset)
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

func seedDashboardScrollableBody(model *DashboardModel) {
	seedDashboardClickPods(model)
	deployments := make([]cluster.Deployment, dashboardScrollDeploymentCount)
	for index := range deployments {
		deployments[index] = cluster.Deployment{
			Name:      "deploy-" + strconv.Itoa(index),
			Namespace: "default",
			Ready:     "1/1",
			Age:       "1h",
		}
	}
	events := make([]cluster.Event, dashboardScrollEventCount)
	for index := range events {
		events[index] = cluster.Event{
			Name:      "event-" + strconv.Itoa(index),
			Namespace: "default",
			Type:      "Normal",
			Reason:    "Scheduled",
			Message:   "assigned to a node",
			Age:       "1m",
		}
	}
	model.deployments = deployments
	model.events = events
}
