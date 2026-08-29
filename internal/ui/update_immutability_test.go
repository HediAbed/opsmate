package ui

import (
	"testing"
	"time"

	"github.com/HediAbed/opsmate/internal/cluster"
)

func TestBrowserLiveSnapshotDoesNotMutatePreviousModel(t *testing.T) {
	tests := []struct {
		name      string
		items     []cluster.Pod
		wantCount int
	}{
		{
			name:      "replace",
			items:     []cluster.Pod{{Name: "api", Namespace: "ns", Status: "Failed"}},
			wantCount: 1,
		},
		{
			name:      "remove",
			items:     []cluster.Pod{},
			wantCount: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pods := make([]cluster.Pod, 1, 4)
			pods[0] = cluster.Pod{Name: "api", Namespace: "ns", Status: "Running"}
			m := newTestBrowserModel("ns")
			m.SetResourceType("pods")
			m.pods = pods
			m.rebuildTable()
			before := m

			updated, _ := m.Update(liveSnapshotMsg[cluster.Pod]{State: resourceLiveState[cluster.Pod]{
				Items: test.items,
				Ready: true,
			}})

			if before.pods[0].Status != "Running" || len(before.pods) != 1 {
				t.Fatalf("previous model changed: %+v", before.pods)
			}
			if len(updated.pods) != test.wantCount {
				t.Fatalf("updated pod count = %d, want %d", len(updated.pods), test.wantCount)
			}
			if len(test.items) == 1 && updated.pods[0].Status != "Failed" {
				t.Errorf("updated pod status = %q, want Failed", updated.pods[0].Status)
			}
		})
	}
}

func TestDashboardEventUpdateDoesNotMutatePreviousModel(t *testing.T) {
	older := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	newer := older.Add(time.Minute)
	events := make([]cluster.Event, 2, 5)
	events[0] = cluster.Event{Namespace: "ns", Reason: "Older", Object: "pod/api", Message: "original", LastTimestamp: older}
	events[1] = cluster.Event{Namespace: "ns", Reason: "Newer", Object: "pod/api", Message: "newer", LastTimestamp: newer}
	m := newTestDashboardModel("ns")
	m.events = events
	before := m

	updated, _ := m.Update(dashboardEventSnapshot(&m, resourceLiveState[cluster.Event]{
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
