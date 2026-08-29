package ui

import (
	"testing"

	"github.com/HediAbed/opsmate/internal/cluster"
)

func TestCurrentResourceRows_ReplicaSets_RendersFiveColumns(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(160, 40)
	m.SetResourceType("replicasets")
	m.replicasets = []cluster.ReplicaSet{
		{Name: "web-rs-abc", Namespace: "ns", Desired: 3, Current: 2, Ready: 1, Age: "5m"},
	}
	rows := m.currentResourceRows()
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	row := rows[0]
	specCols := resourceColSpecs["replicasets"]
	if len(row) != len(specCols) {
		t.Fatalf("row has %d cells, tablespec declares %d columns", len(row), len(specCols))
	}
	want := []string{"web-rs-abc", "3", "2", "1", "5m"}
	for i, w := range want {
		if row[i] != w {
			t.Errorf("col %d (%s) = %q, want %q", i, specCols[i].Title, row[i], w)
		}
	}
}
