package model

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/service"
)

func TestLogsLayoutSupportsScrollingBeforeView(t *testing.T) {
	m := NewLogsModel("payments")
	m.SetSize(80, 12)
	m.selectedPod = "api"
	m.selectedPodNamespace = "payments"
	lines := make([]string, 100)
	for index := range lines {
		lines[index] = fmt.Sprintf("line %d", index)
	}
	m, _ = m.Update(service.LogsMsg{Lines: lines})
	m.logView.GotoTop()

	if m.logView.Width() <= 0 || m.logView.Height() <= 0 {
		t.Fatalf("log viewport was not sized during state synchronization: %dx%d", m.logView.Width(), m.logView.Height())
	}
	m, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if m.logView.YOffset() == 0 {
		t.Fatal("log viewport did not scroll before View was called")
	}
}

func TestBrowserViewDoesNotChangeRetainedWidgetGeometry(t *testing.T) {
	m := NewBrowserModel("payments")
	m.SetSize(120, 30)
	m.pods = []service.Pod{{Name: "api", Namespace: "payments", Status: "Running"}}
	m.rebuildTable()
	m.showDetail = true
	m.state = stateDetail
	m.detailContent = "name: api"
	m.rebuildDetailContent()
	m.syncBrowserLayout()

	tableWidth, tableHeight := m.resourceTable.Width(), m.resourceTable.Height()
	detailWidth, detailHeight := m.detailView.Width(), m.detailView.Height()
	_ = m.View()
	if m.resourceTable.Width() != tableWidth || m.resourceTable.Height() != tableHeight {
		t.Fatalf("View changed table geometry from %dx%d to %dx%d", tableWidth, tableHeight, m.resourceTable.Width(), m.resourceTable.Height())
	}
	if m.detailView.Width() != detailWidth || m.detailView.Height() != detailHeight {
		t.Fatalf("View changed detail geometry from %dx%d to %dx%d", detailWidth, detailHeight, m.detailView.Width(), m.detailView.Height())
	}
}

func TestDashboardLayoutSupportsScrollingBeforeView(t *testing.T) {
	m := NewDashboardModel("payments")
	m.SetSize(100, 12)
	pods := make([]service.Pod, 20)
	for index := range pods {
		pods[index] = service.Pod{Name: fmt.Sprintf("pod-%02d", index), Namespace: "payments", Status: "Running"}
	}
	deployments := make([]service.Deployment, 8)
	for index := range deployments {
		deployments[index] = service.Deployment{Name: fmt.Sprintf("deployment-%02d", index), Namespace: "payments", Ready: "0/1"}
	}
	events := make([]service.Event, 8)
	for index := range events {
		events[index] = service.Event{Namespace: "payments", Type: "Warning", Reason: "BackOff", Object: fmt.Sprintf("Pod/pod-%02d", index), Message: "retrying"}
	}
	m, _ = m.Update(service.DeploymentsMsg{Deployments: deployments})
	m, _ = m.Update(service.EventsMsg{Events: events})
	m, _ = m.Update(service.PodsMsg{Pods: pods})
	m.bodyView.GotoTop()

	if !m.bodyOverflows() {
		t.Fatalf("dashboard body should overflow: lines=%d height=%d", m.bodyView.TotalLineCount(), m.bodyView.Height())
	}
	m, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if m.bodyView.YOffset() == 0 {
		t.Fatal("dashboard body did not scroll before View was called")
	}
}

func TestAIPanelViewDoesNotChangeRetainedWidgetGeometry(t *testing.T) {
	m := NewAIPanelModel()
	m.SetVisible(true)
	m.SetSize(80, 24)
	inputWidth := m.input.Width()
	viewportWidth, viewportHeight := m.responseView.Width(), m.responseView.Height()

	_ = m.View()
	if m.input.Width() != inputWidth {
		t.Fatalf("View changed input width from %d to %d", inputWidth, m.input.Width())
	}
	if m.responseView.Width() != viewportWidth || m.responseView.Height() != viewportHeight {
		t.Fatalf("View changed response geometry from %dx%d to %dx%d", viewportWidth, viewportHeight, m.responseView.Width(), m.responseView.Height())
	}
}

func TestHelmValuesViewUsesPrecomputedViewportGeometry(t *testing.T) {
	m := NewHelmModel("payments")
	m.SetSize(100, 30)
	m.valuesPopupVisible = true
	m.valuesPopupRelease = "api"
	m.syncValuesPopupLayout()
	width, height := m.valuesPopupView.Width(), m.valuesPopupView.Height()

	_ = m.View()
	if m.valuesPopupView.Width() != width || m.valuesPopupView.Height() != height {
		t.Fatalf("View changed values geometry from %dx%d to %dx%d", width, height, m.valuesPopupView.Width(), m.valuesPopupView.Height())
	}
}
