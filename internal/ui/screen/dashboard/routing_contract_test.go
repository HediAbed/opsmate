package dashboard

import (
	"testing"

	"github.com/HediAbed/opsmate/internal/analysis"
	"github.com/HediAbed/opsmate/internal/cluster"
	clusterui "github.com/HediAbed/opsmate/internal/ui/cluster"
	"github.com/HediAbed/opsmate/internal/ui/screen"
)

func TestDashboardRefreshStartsAnIdentifiedMetricsLoad(t *testing.T) {
	model := newTestDashboardModel("team-a")
	model.loading = false
	previousRequestID := model.requestIDs[dashboardMetrics]

	command := model.Refresh()
	if command == nil || !model.Loading() {
		t.Fatalf("Refresh() = command:%v loading:%v", command, model.Loading())
	}
	message, ok := command().(dashboardResultMsg)
	if !ok || message.kind != dashboardMetrics || message.requestID != previousRequestID+1 || !model.Accepts(message) {
		t.Fatalf("Refresh() metrics message = %#v", message)
	}
}

func TestDashboardRejectsStaleDataResult(t *testing.T) {
	model := newTestDashboardModel("team-a")
	message := model.fetchDashboardData(dashboardMetrics, model.cluster.FetchPodMetrics(model.namespace))().(dashboardResultMsg)
	_ = model.fetchDashboardData(dashboardMetrics, model.cluster.FetchPodMetrics(model.namespace))
	message.payload = cluster.MetricsMsg{PodMetrics: []cluster.PodMetric{{Name: "stale"}}}
	if model.Accepts(message) {
		t.Fatal("dashboard accepted a stale data result")
	}
	updated, command := model.Update(message)
	if command != nil || len(updated.metrics) != 0 {
		t.Fatalf("stale data result changed dashboard: metrics=%v command=%v", updated.metrics, command)
	}
}

func TestDashboardHealthResultRequiresCurrentIdentity(t *testing.T) {
	model := newTestDashboardModel("team-a")
	stale := model.fetchHealthSummary()().(dashboardHealthResultMsg)
	current := model.fetchHealthSummary()().(dashboardHealthResultMsg)

	stale.payload = analysis.DashboardHealthMsg{Summary: "stale sentinel"}
	if model.Accepts(stale) {
		t.Fatal("dashboard accepted a superseded health result")
	}
	unchanged, command := model.Update(stale)
	if command != nil || unchanged.healthAnalysisSummary != "" {
		t.Fatalf("stale health result changed dashboard: summary=%q command=%v", unchanged.healthAnalysisSummary, command)
	}

	foreign := current
	foreign.namespace = "team-b"
	foreign.payload = analysis.DashboardHealthMsg{Summary: "foreign sentinel"}
	if model.Accepts(foreign) {
		t.Fatal("dashboard accepted a foreign-namespace health result")
	}
	unchanged, command = model.Update(foreign)
	if command != nil || unchanged.healthAnalysisSummary != "" {
		t.Fatalf("foreign health result changed dashboard: summary=%q command=%v", unchanged.healthAnalysisSummary, command)
	}

	current.payload = analysis.DashboardHealthMsg{Summary: "healthy"}
	if !model.Accepts(current) {
		t.Fatal("dashboard rejected its current health result")
	}
	updated, command := model.Update(current)
	if command != nil || updated.healthAnalysisSummary != "healthy" {
		t.Fatalf("current health result = summary:%q command:%v", updated.healthAnalysisSummary, command)
	}
}

func TestDashboardMessageContractsClassifyTicksAndContextChanges(t *testing.T) {
	model := newTestDashboardModel("team-a")
	if !model.Accepts(dashMetricsTickMsg{}) {
		t.Fatal("dashboard did not accept its metrics tick")
	}
	if !model.ContextChangedBy(model.fetchDashboardData(dashboardMetrics, model.cluster.FetchPodMetrics(model.namespace))()) {
		t.Fatal("dashboard ignored its current data result")
	}
	snapshot := dashboardPodSnapshot(&model, clusterui.LiveState[cluster.Pod]{
		Ready: true,
		Items: []cluster.Pod{{Name: "owned", Namespace: "team-a"}},
	})
	if !model.ContextChangedBy(snapshot) {
		t.Fatal("dashboard ignored its owned live snapshot")
	}
	model.stopDashboardLiveSets()
	if model.ContextChangedBy(snapshot) {
		t.Fatal("dashboard still claims a live snapshot after stopping its sets")
	}
	if model.ContextChangedBy(screen.LiveMessage{}) {
		t.Fatal("dashboard claimed a live message without an owned generation")
	}
	if model.ContextChangedBy(struct{}{}) {
		t.Fatal("dashboard claimed an unrelated message changed its context")
	}
}
