package dashboard

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/HediAbed/opsmate/internal/cluster"
	clusterui "github.com/HediAbed/opsmate/internal/ui/cluster"
)

func TestDashboardLayoutSupportsScrollingBeforeView(t *testing.T) {
	m := newTestDashboardModel("payments")
	m.SetSize(100, 12)
	pods := make([]cluster.Pod, 20)
	for index := range pods {
		pods[index] = cluster.Pod{Name: fmt.Sprintf("pod-%02d", index), Namespace: "payments", Status: "Running"}
	}
	deployments := make([]cluster.Deployment, 8)
	for index := range deployments {
		deployments[index] = cluster.Deployment{Name: fmt.Sprintf("deployment-%02d", index), Namespace: "payments", Ready: "0/1"}
	}
	events := make([]cluster.Event, 8)
	for index := range events {
		events[index] = cluster.Event{Namespace: "payments", Type: "Warning", Reason: "BackOff", Object: fmt.Sprintf("Pod/pod-%02d", index), Message: "retrying"}
	}
	m, _ = m.Update(dashboardDeploymentSnapshot(&m, clusterui.LiveState[cluster.Deployment]{Ready: true, Items: deployments}))
	m, _ = m.Update(dashboardEventSnapshot(&m, clusterui.LiveState[cluster.Event]{Ready: true, Items: events}))
	m, _ = m.Update(dashboardPodSnapshot(&m, clusterui.LiveState[cluster.Pod]{Ready: true, Items: pods}))
	m.bodyView.GotoTop()

	if !m.bodyOverflows() {
		t.Fatalf("dashboard body should overflow: lines=%d height=%d", m.bodyView.TotalLineCount(), m.bodyView.Height())
	}
	m, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if m.bodyView.YOffset() == 0 {
		t.Fatal("dashboard body did not scroll before View was called")
	}
}
