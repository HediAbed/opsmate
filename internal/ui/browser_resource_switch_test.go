package ui

import (
	"testing"

	"github.com/HediAbed/opsmate/internal/cluster"
)

func TestSetResourceType_LiveSnapshotUsesCurrentTableShape(t *testing.T) {
	m := newTestBrowserModel("default")
	m.SetSize(200, 40)

	m.pods = []cluster.Pod{
		{Name: "alpha", Namespace: "default", Status: "Running", Ready: "1/1", Age: "1m", Node: "n1"},
	}
	m.rebuildTable()

	m.SetResourceType("deployments")
	m.deployments = []cluster.Deployment{{Name: "dep", Namespace: "default", Ready: "1/1", Age: "1d"}}
	m.rebuildTable()

	m.SetResourceType("pods")

	_ = applyBrowserLiveState(&m, &m.podLive, &m.pods, "pods", resourceLiveState[cluster.Pod]{Ready: true, Items: m.pods})
	if len(m.resourceTable.Columns()) != len(resourceColSpecs["pods"]) {
		t.Fatalf("table has %d columns, want %d", len(m.resourceTable.Columns()), len(resourceColSpecs["pods"]))
	}
	if rows := m.resourceTable.Rows(); len(rows) != 1 || len(rows[0]) != len(resourceColSpecs["pods"]) {
		t.Fatalf("pod snapshot produced incompatible rows: %v", rows)
	}
	if view := m.resourceTable.View(); view == "" {
		t.Fatal("updated pod table rendered empty")
	}
}

func TestCycleResourceType_RightAdvancesAndWraps(t *testing.T) {
	m := newTestBrowserModel("default")
	m.SetSize(200, 40)

	for i := 0; i < len(allResourceTypes)+1; i++ {
		want := allResourceTypes[(i+1)%len(allResourceTypes)]
		m, _ = m.cycleResourceType(1)
		if m.resourceType != want {
			t.Errorf("step %d: cycleResourceType(+1) = %q; want %q", i, m.resourceType, want)
		}
	}
}

func TestCycleResourceType_LeftWalksBackwardAndWraps(t *testing.T) {
	m := newTestBrowserModel("default")
	m.SetSize(200, 40)

	want := []string{
		allResourceTypes[len(allResourceTypes)-1],
		allResourceTypes[len(allResourceTypes)-2],
	}
	for i, w := range want {
		m, _ = m.cycleResourceType(-1)
		if m.resourceType != w {
			t.Errorf("step %d: cycleResourceType(-1) = %q; want %q", i, m.resourceType, w)
		}
	}
}

func TestSetResourceType_TableShapeFollowsType(t *testing.T) {
	m := newTestBrowserModel("default")
	m.SetSize(200, 40)

	if got := len(m.resourceTable.Columns()); got != len(resourceColSpecs["pods"]) {
		t.Fatalf("initial columns=%d, want %d (pods)", got, len(resourceColSpecs["pods"]))
	}

	m.SetResourceType("deployments")
	if got := len(m.resourceTable.Columns()); got != len(resourceColSpecs["deployments"]) {
		t.Errorf("after switch to deployments columns=%d, want %d", got, len(resourceColSpecs["deployments"]))
	}

	m.SetResourceType("services")
	if got := len(m.resourceTable.Columns()); got != len(resourceColSpecs["services"]) {
		t.Errorf("after switch to services columns=%d, want %d", got, len(resourceColSpecs["services"]))
	}
}
