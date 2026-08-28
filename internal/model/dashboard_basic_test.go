package model

import (
	"testing"

	"github.com/HediAbed/opsmate/internal/service"
)

func TestDashboardModel_Init_ReturnsNonNilCmd(t *testing.T) {
	if cmd := NewDashboardModel("ns").Init(); cmd == nil {
		t.Error("Init must return a non-nil cmd (spinner + metrics tick)")
	}
}

func TestScheduleMetricsTick_ReturnsTickCmd(t *testing.T) {
	if cmd := scheduleMetricsTick(); cmd == nil {
		t.Error("scheduleMetricsTick must return a non-nil cmd")
	}
}

func TestDashboardModel_SelectedPodNS_FallsThroughForAllNamespacesMode(t *testing.T) {
	m := NewDashboardModel("")
	m.SetSize(120, 40)
	m.pods = []service.Pod{
		{Name: "alpha", Namespace: "ns1"},
		{Name: "beta", Namespace: "ns2"},
	}
	m.rebuildTableRows()

	m.podTable.SetCursor(0)
	if got := m.SelectedPodNS(); got != "ns1" {
		t.Errorf("SelectedPodNS = %q, want ns1", got)
	}

	m.podTable.SetCursor(1)
	if got := m.SelectedPodNS(); got != "ns2" {
		t.Errorf("SelectedPodNS = %q, want ns2", got)
	}
}

func TestDashboardModel_SelectedPodNS_PinnedNamespaceShortCircuits(t *testing.T) {
	m := NewDashboardModel("kube-system")
	if got := m.SelectedPodNS(); got != "kube-system" {
		t.Errorf("SelectedPodNS with pinned ns = %q, want kube-system", got)
	}
}

func TestDashboardModel_DuplicatePodNamesRetainNamespaceIdentity(t *testing.T) {
	m := NewDashboardModel("")
	m.SetSize(120, 40)
	m.pods = []service.Pod{
		{Name: "web", Namespace: "payments"},
		{Name: "web", Namespace: "platform"},
	}
	m.rebuildTableRows()

	rows := m.podTable.Rows()
	if rows[0][0] != "payments/web" || rows[1][0] != "platform/web" {
		t.Fatalf("duplicate dashboard rows are ambiguous: %v", rows)
	}
	m.podTable.SetCursor(1)
	if m.SelectedPod() != "web" || m.SelectedPodNS() != "platform" {
		t.Fatalf("selection = %s/%s", m.SelectedPodNS(), m.SelectedPod())
	}
}

func TestDashboardModel_MetricsJoinUsesNamespaceAndName(t *testing.T) {
	m := NewDashboardModel("")
	m.pods = []service.Pod{
		{Name: "web", Namespace: "payments"},
		{Name: "web", Namespace: "platform"},
	}
	m.metrics = []service.PodMetric{
		{Name: "web", Namespace: "payments", CPU: "10m", Memory: "20Mi"},
		{Name: "web", Namespace: "platform", CPU: "30m", Memory: "40Mi"},
	}

	m.mergeMetrics()
	if m.pods[0].CPU != "10m" || m.pods[1].CPU != "30m" {
		t.Fatalf("metrics joined to wrong namespace: %+v", m.pods)
	}
}
