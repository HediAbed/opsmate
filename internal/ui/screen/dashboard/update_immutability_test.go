package dashboard

import (
	"testing"
	"time"

	"github.com/HediAbed/opsmate/internal/cluster"
	clusterui "github.com/HediAbed/opsmate/internal/ui/cluster"
)

func TestDashboardEventUpdateDoesNotMutatePreviousModel(t *testing.T) {
	older := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	newer := older.Add(time.Minute)
	events := make([]cluster.Event, 2, 5)
	events[0] = cluster.Event{Namespace: "ns", Reason: "Older", Object: "pod/api", Message: "original", LastTimestamp: older}
	events[1] = cluster.Event{Namespace: "ns", Reason: "Newer", Object: "pod/api", Message: "newer", LastTimestamp: newer}
	m := newTestDashboardModel("ns")
	m.events = events
	before := m

	updated, _ := m.Update(dashboardEventSnapshot(&m, clusterui.LiveState[cluster.Event]{
		Ready: true,
		Items: []cluster.Event{
			{Namespace: "ns", Reason: "Older", Object: "pod/api", Message: "modified", LastTimestamp: older},
			{Namespace: "ns", Reason: "Newer", Object: "pod/api", Message: "newer", LastTimestamp: newer},
		},
	}))

	if before.events[0].Message != "original" || before.events[0].Reason != "Older" {
		t.Fatalf("previous event model changed: %+v", before.events)
	}
	if updated.events[0].Reason != "Newer" {
		t.Errorf("updated events were not sorted newest-first: %+v", updated.events)
	}
	if updated.events[1].Message != "modified" {
		t.Errorf("matching event was not replaced: %+v", updated.events)
	}
}
