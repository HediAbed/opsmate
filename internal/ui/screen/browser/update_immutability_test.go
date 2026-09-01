package browser

import (
	"testing"

	"github.com/HediAbed/opsmate/internal/cluster"
	clusterui "github.com/HediAbed/opsmate/internal/ui/cluster"
	"github.com/HediAbed/opsmate/internal/ui/screen"
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

			m.podLive.Set(newTestResourceLiveSet(clusterui.LiveState[cluster.Pod]{}))
			updated, _ := m.Update(screen.LiveMessage{
				Generation: m.podLive.Generation(),
				Payload: screen.LiveSnapshot[cluster.Pod]{State: clusterui.LiveState[cluster.Pod]{
					Items: test.items,
					Ready: true,
				}},
			})

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
