package model

import (
	"testing"
	"time"

	"github.com/HediAbed/opsmate/internal/service"
)

func TestBrowserWatchUpdateDoesNotMutatePreviousModel(t *testing.T) {
	tests := []struct {
		name      string
		kind      service.WatchEventKind
		item      service.Pod
		wantCount int
	}{
		{
			name:      "replace",
			kind:      service.WatchModified,
			item:      service.Pod{Name: "api", Namespace: "ns", Status: "Failed"},
			wantCount: 1,
		},
		{
			name:      "remove",
			kind:      service.WatchDeleted,
			item:      service.Pod{Name: "api", Namespace: "ns"},
			wantCount: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pods := make([]service.Pod, 1, 4)
			pods[0] = service.Pod{Name: "api", Namespace: "ns", Status: "Running"}
			m := NewBrowserModel("ns")
			m.SetResourceType("pods")
			m.pods = pods
			m.rebuildTable()
			before := m

			updated, _ := m.Update(service.WatchEventMsg[service.Pod]{
				Event: service.WatchEvent[service.Pod]{Kind: test.kind, Item: test.item},
			})

			if before.pods[0].Status != "Running" || len(before.pods) != 1 {
				t.Fatalf("previous model changed: %+v", before.pods)
			}
			if len(updated.pods) != test.wantCount {
				t.Fatalf("updated pod count = %d, want %d", len(updated.pods), test.wantCount)
			}
			if test.kind == service.WatchModified && updated.pods[0].Status != "Failed" {
				t.Errorf("updated pod status = %q, want Failed", updated.pods[0].Status)
			}
		})
	}
}

func TestDashboardEventUpdateDoesNotMutatePreviousModel(t *testing.T) {
	older := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	newer := older.Add(time.Minute)
	events := make([]service.Event, 2, 5)
	events[0] = service.Event{Namespace: "ns", Reason: "Older", Object: "pod/api", Message: "original", LastTimestamp: older}
	events[1] = service.Event{Namespace: "ns", Reason: "Newer", Object: "pod/api", Message: "newer", LastTimestamp: newer}
	m := NewDashboardModel("ns")
	m.events = events
	before := m

	updated, _ := m.Update(service.WatchEventMsg[service.Event]{
		Event: service.WatchEvent[service.Event]{
			Kind: service.WatchModified,
			Item: service.Event{
				Namespace: "ns", Reason: "Older", Object: "pod/api", Message: "modified", LastTimestamp: older,
			},
		},
	})

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
