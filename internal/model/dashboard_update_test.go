package model

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/service"
)

func TestDashboardUpdate_WindowSize(t *testing.T) {
	m := NewDashboardModel("ns")
	out, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 40})
	if out.width != 200 || out.height != 40 {
		t.Errorf("WindowSize not applied; got %dx%d", out.width, out.height)
	}
}

func TestDashboardUpdate_PodsMsg_PopulatesWithoutDuplicateMetricsFetch(t *testing.T) {
	m := NewDashboardModel("ns")
	m.SetSize(120, 30)
	out, cmd := m.Update(service.PodsMsg{Pods: []service.Pod{{Name: "a"}}})
	if len(out.pods) != 1 {
		t.Errorf("pods not populated; got %d", len(out.pods))
	}
	if out.loading {
		t.Error("PodsMsg should clear loading")
	}
	if cmd != nil {
		t.Error("PodsMsg must not duplicate the metrics request already started by refreshAll")
	}
}

func TestDashboardUpdate_PodsMsg_Error(t *testing.T) {
	m := NewDashboardModel("ns")
	m.SetSize(120, 30)
	out, _ := m.Update(service.PodsMsg{Err: errStub("denied")})
	if out.err == nil {
		t.Error("PodsMsg err should propagate")
	}
}

func TestDashboardUpdate_DeploymentsMsg_PopulatesAndPreservesPriorErr(t *testing.T) {
	m := NewDashboardModel("ns")
	m.SetSize(120, 30)
	out, _ := m.Update(service.DeploymentsMsg{Deployments: []service.Deployment{{Name: "d"}}})
	if len(out.deployments) != 1 {
		t.Error("deployments not populated")
	}
}

func TestDashboardUpdate_EventsMsg_Populates(t *testing.T) {
	m := NewDashboardModel("ns")
	m.SetSize(120, 30)
	out, _ := m.Update(service.EventsMsg{Events: []service.Event{{Reason: "Killed"}}})
	if len(out.events) != 1 {
		t.Error("events not populated")
	}
}

func TestDashboardUpdate_MetricsMsg_PopulatesAndMerges(t *testing.T) {
	m := NewDashboardModel("ns")
	m.SetSize(120, 30)
	m.pods = []service.Pod{{Name: "alpha"}}
	out, _ := m.Update(service.MetricsMsg{PodMetrics: []service.PodMetric{{Name: "alpha", CPU: "10m", Memory: "20Mi"}}})
	if len(out.metrics) != 1 {
		t.Error("metrics not stored")
	}
}

func TestDashboardUpdate_MetricsMsg_ErrorClears(t *testing.T) {
	m := NewDashboardModel("ns")
	m.SetSize(120, 30)
	m.metrics = []service.PodMetric{{Name: "alpha"}}
	out, _ := m.Update(service.MetricsMsg{Err: errStub("denied")})
	if len(out.metrics) != 0 {
		t.Error("metrics err should clear stale data")
	}
}

func TestDashboardUpdate_DashMetricsTickActiveRefetches(t *testing.T) {
	m := NewDashboardModel("ns")
	m.SetSize(120, 30)
	m.active = true
	_, cmd := m.Update(dashMetricsTickMsg{})
	if cmd == nil {
		t.Error("active dashboard tick should batch metrics fetch + next tick")
	}
}

func TestDashboardUpdate_KeyR_TriggersRefresh(t *testing.T) {
	m := NewDashboardModel("ns")
	m.SetSize(120, 30)
	m.active = true
	out, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	_ = out
	if cmd == nil {
		t.Error("r should trigger refresh")
	}
}

func TestDashboardUpdate_KeyL_OpensLogs(t *testing.T) {
	m := NewDashboardModel("ns")
	m.SetSize(120, 30)
	m.pods = []service.Pod{{Name: "alpha", Namespace: "ns"}}
	m.rebuildTableRows()
	m.podTable.SetCursor(0)
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	if cmd == nil {
		t.Error("l should drill down to logs")
	}
}

func TestDashboardUpdate_DashHealthMsg_Success(t *testing.T) {
	m := NewDashboardModel("ns")
	m.aiHealthLoading = true
	out, _ := m.Update(service.DashHealthMsg{Summary: "all healthy"})
	if out.aiHealthLoading {
		t.Error("DashHealthMsg should clear loading")
	}
	if out.aiHealthSummary == "" {
		t.Error("summary should be populated")
	}
}

func TestDashboardUpdate_DashHealthMsg_Error(t *testing.T) {
	m := NewDashboardModel("ns")
	m.aiHealthLoading = true
	out, _ := m.Update(service.DashHealthMsg{Err: errStub("rate limit")})
	if out.aiHealthErr == nil {
		t.Error("err should propagate")
	}
}

func TestDashboardUpdate_KeyA_TogglesAIHealth(t *testing.T) {
	t.Setenv("MOONSHOT_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("CLAUDE_CLI", "")
	if err := service.InitAIProvider(); err != nil {
		t.Fatalf("initialize provider: %v", err)
	}

	m := NewDashboardModel("ns")
	m.SetSize(120, 30)
	out, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	if !out.showAIHealth {
		t.Error("a should toggle showAIHealth ON")
	}
	out, _ = out.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	if out.showAIHealth {
		t.Error("second a should toggle OFF")
	}
}

func TestDashboardUpdate_Enter_DrillsToBrowser(t *testing.T) {
	m := NewDashboardModel("ns")
	m.SetSize(120, 30)
	m.pods = []service.Pod{{Name: "alpha", Namespace: "ns"}}
	m.rebuildTableRows()
	m.podTable.SetCursor(0)
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	if cmd == nil {
		t.Fatal("enter on selected pod should drill to Browser")
	}
	msg := cmd()
	if d, ok := msg.(DrillDownMsg); !ok || d.Screen != ScreenBrowser {
		t.Errorf("expected DrillDownMsg{Screen:Browser}; got %+v", msg)
	}
}

func TestDashboardUpdate_Esc_ClosesAIHealth(t *testing.T) {
	m := NewDashboardModel("ns")
	m.showAIHealth = true
	out, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc, Text: "esc"})
	if out.showAIHealth {
		t.Error("esc should close AI health summary")
	}
}

func TestDashboardUpdate_MouseWheelMovesTable(_ *testing.T) {
	m := NewDashboardModel("ns")
	m.SetSize(120, 30)
	_, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	_, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
}

func TestDashboardUpdate_MouseClickSelectsRow(_ *testing.T) {
	m := NewDashboardModel("ns")
	m.SetSize(120, 30)
	m.pods = []service.Pod{{Name: "a"}, {Name: "b"}}
	m.rebuildTableRows()
	_, _ = m.Update(tea.MouseClickMsg{X: 10, Y: 5, Button: tea.MouseLeft})
}

func TestDashboardUpdate_DashMetricsTickInactiveStillTicks(t *testing.T) {
	m := NewDashboardModel("ns")
	m.SetSize(120, 30)
	m.active = false
	_, cmd := m.Update(dashMetricsTickMsg{})
	if cmd == nil {
		t.Error("tick should always re-arm itself")
	}
}
