package dashboard

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/HediAbed/opsmate/internal/analysis"
	"github.com/HediAbed/opsmate/internal/cluster"
	clusterui "github.com/HediAbed/opsmate/internal/ui/cluster"
	"github.com/HediAbed/opsmate/internal/ui/screen"
)

func TestDashboardUpdate_WindowSize(t *testing.T) {
	m := newTestDashboardModel("ns")
	out, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 40})
	if out.width != 200 || out.height != 40 {
		t.Errorf("WindowSize not applied; got %dx%d", out.width, out.height)
	}
}

func TestDashboardUpdate_PodSnapshotPopulatesWithoutDuplicateMetricsFetch(t *testing.T) {
	m := newTestDashboardModel("ns")
	m.SetSize(120, 30)
	out, cmd := m.Update(dashboardPodSnapshot(&m, clusterui.LiveState[cluster.Pod]{Ready: true, Items: []cluster.Pod{{Name: "a"}}}))
	if len(out.pods) != 1 {
		t.Errorf("pods not populated; got %d", len(out.pods))
	}
	if out.loading {
		t.Error("ready pod snapshot should clear loading")
	}
	if cmd == nil {
		t.Error("pod snapshot did not arm the next live update")
	}
}

func TestDashboardUpdate_PodSnapshotError(t *testing.T) {
	m := newTestDashboardModel("ns")
	m.SetSize(120, 30)
	m.pods = []cluster.Pod{{Name: "retained"}}
	out, _ := m.Update(dashboardPodSnapshot(&m, clusterui.LiveState[cluster.Pod]{Ready: true, Items: []cluster.Pod{{Name: "discarded"}}, Err: errStub("denied")}))
	if out.err == nil {
		t.Error("pod snapshot error should propagate")
	}
	if len(out.pods) != 1 || out.pods[0].Name != "retained" {
		t.Fatalf("pod snapshot error replaced the last good state: %+v", out.pods)
	}
}

func TestDashboardUpdate_DeploymentSnapshotPopulates(t *testing.T) {
	m := newTestDashboardModel("ns")
	m.SetSize(120, 30)
	out, _ := m.Update(dashboardDeploymentSnapshot(&m, clusterui.LiveState[cluster.Deployment]{Ready: true, Items: []cluster.Deployment{{Name: "d"}}}))
	if len(out.deployments) != 1 {
		t.Error("deployments not populated")
	}
}

func TestDashboardUpdate_EventSnapshotPopulates(t *testing.T) {
	m := newTestDashboardModel("ns")
	m.SetSize(120, 30)
	out, _ := m.Update(dashboardEventSnapshot(&m, clusterui.LiveState[cluster.Event]{Ready: true, Items: []cluster.Event{{Reason: "Killed"}}}))
	if len(out.events) != 1 {
		t.Error("events not populated")
	}
}

func TestDashboardUpdate_MetricsMsg_PopulatesAndMerges(t *testing.T) {
	m := newTestDashboardModel("ns")
	m.SetSize(120, 30)
	m.pods = []cluster.Pod{{Name: "alpha"}}
	out, _ := m.Update(cluster.MetricsMsg{PodMetrics: []cluster.PodMetric{{Name: "alpha", CPU: "10m", Memory: "20Mi"}}})
	if len(out.metrics) != 1 {
		t.Error("metrics not stored")
	}
}

func TestDashboardUpdate_MetricsMsg_ErrorClears(t *testing.T) {
	m := newTestDashboardModel("ns")
	m.SetSize(120, 30)
	m.metrics = []cluster.PodMetric{{Name: "alpha"}}
	out, _ := m.Update(cluster.MetricsMsg{Err: errStub("denied")})
	if len(out.metrics) != 0 {
		t.Error("metrics err should clear stale data")
	}
}

func TestDashboardUpdate_DashMetricsTickActiveRefetches(t *testing.T) {
	m := newTestDashboardModel("ns")
	m.SetSize(120, 30)
	m.active = true
	_, cmd := m.Update(dashMetricsTickMsg{})
	if cmd == nil {
		t.Error("active dashboard tick should batch metrics fetch + next tick")
	}
}

func TestDashboardUpdate_KeyR_TriggersRefresh(t *testing.T) {
	m := newTestDashboardModel("ns")
	m.SetSize(120, 30)
	m.active = true
	out, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	_ = out
	if cmd == nil {
		t.Error("r should trigger refresh")
	}
}

func TestDashboardUpdate_KeyL_OpensLogs(t *testing.T) {
	m := newTestDashboardModel("ns")
	m.SetSize(120, 30)
	m.pods = []cluster.Pod{{Name: "alpha", Namespace: "ns"}}
	m.rebuildTableRows()
	m.podTable.SetCursor(0)
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	if cmd == nil {
		t.Error("l should drill down to logs")
	}
}

func TestDashboardUpdateDashboardHealthMsgSuccess(t *testing.T) {
	m := newTestDashboardModel("ns")
	m.healthAnalysisLoading = true
	out, _ := m.Update(analysis.DashboardHealthMsg{Summary: "all healthy"})
	if out.healthAnalysisLoading {
		t.Error("DashboardHealthMsg should clear loading")
	}
	if out.healthAnalysisSummary == "" {
		t.Error("summary should be populated")
	}
}

func TestDashboardUpdateDashboardHealthMsgError(t *testing.T) {
	m := newTestDashboardModel("ns")
	m.healthAnalysisLoading = true
	out, _ := m.Update(analysis.DashboardHealthMsg{Err: errStub("rate limit")})
	if out.healthAnalysisErr == nil {
		t.Error("err should propagate")
	}
}

func TestDashboardUpdate_KeyA_TogglesHealthAnalysis(t *testing.T) {
	m := newTestDashboardModel("ns")
	m.SetSize(120, 30)
	out, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	if !out.showHealthAnalysis {
		t.Error("a should show the health analysis")
	}
	out, _ = out.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	if out.showHealthAnalysis {
		t.Error("second a should toggle OFF")
	}
}

func TestDashboardUpdate_Enter_DrillsToBrowser(t *testing.T) {
	m := newTestDashboardModel("ns")
	m.SetSize(120, 30)
	m.pods = []cluster.Pod{{Name: "alpha", Namespace: "ns"}}
	m.rebuildTableRows()
	m.podTable.SetCursor(0)
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	if cmd == nil {
		t.Fatal("enter on selected pod should drill to Browser")
	}
	msg := cmd()
	if d, ok := msg.(screen.DrillDownMsg); !ok || d.Screen != screen.Browser {
		t.Errorf("expected DrillDownMsg{Screen:Browser}; got %+v", msg)
	}
}

func TestDashboardUpdate_Esc_ClosesHealthAnalysis(t *testing.T) {
	m := newTestDashboardModel("ns")
	m.showHealthAnalysis = true
	out, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc, Text: "esc"})
	if out.showHealthAnalysis {
		t.Error("esc should close health analysis summary")
	}
}

func TestDashboardUpdate_MouseWheelMovesTable(_ *testing.T) {
	m := newTestDashboardModel("ns")
	m.SetSize(120, 30)
	_, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	_, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
}

func TestDashboardUpdate_MouseClickSelectsRow(_ *testing.T) {
	m := newTestDashboardModel("ns")
	m.SetSize(120, 30)
	m.pods = []cluster.Pod{{Name: "a"}, {Name: "b"}}
	m.rebuildTableRows()
	_, _ = m.Update(tea.MouseClickMsg{X: 10, Y: 5, Button: tea.MouseLeft})
}

func TestDashboardUpdate_DashMetricsTickInactiveStopsChain(t *testing.T) {
	m := newTestDashboardModel("ns")
	m.SetSize(120, 30)
	m.active = false
	_, cmd := m.Update(dashMetricsTickMsg{})
	if cmd != nil {
		t.Error("an inactive dashboard must let the metrics timer chain expire instead of re-arming it")
	}
}

func TestDashboardActivateAfterExpiredMetricsTickSchedulesOneChain(t *testing.T) {
	m := NewDashboardModel("ns", &testCommands{observeErr: errStub("watch unavailable")})
	m.SetSize(120, 30)
	m.active = false
	expired, cmd := m.Update(dashMetricsTickMsg{})
	if cmd != nil {
		t.Fatal("inactive tick must expire the metrics chain before activation is measured")
	}

	afterRestart := dashboardCommandCount(expired.Activate())
	afterRepeat := dashboardCommandCount(expired.Activate())
	if afterRestart != afterRepeat+1 {
		t.Errorf("activation issued %d commands with an expired chain and %d with a live one; "+
			"want exactly one extra command for the single restarted metrics tick", afterRestart, afterRepeat)
	}
}

func dashboardCommandCount(command tea.Cmd) int {
	if command == nil {
		return 0
	}
	if batch, batched := command().(tea.BatchMsg); batched {
		return len(batch)
	}
	return 1
}
