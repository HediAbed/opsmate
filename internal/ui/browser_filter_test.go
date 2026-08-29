package ui

import (
	"testing"

	"charm.land/bubbles/v2/table"
	"github.com/HediAbed/opsmate/internal/cluster"
)

func TestFilterRows_CaseInsensitiveSubstring(t *testing.T) {
	rows := []table.Row{
		{"WEB-frontend", "Running"},
		{"api-backend", "Running"},
		{"db-primary", "Pending"},
	}
	identities := []resourceIdentity{{Name: "web"}, {Name: "api"}, {Name: "db"}}
	got, gotIdentities := filterRowsWithIdentities(rows, identities, "web")
	if len(got) != 1 || got[0][0] != "WEB-frontend" || gotIdentities[0].Name != "web" {
		t.Errorf("expected aligned WEB-frontend match, got rows=%+v identities=%+v", got, gotIdentities)
	}
	got, gotIdentities = filterRowsWithIdentities(rows, identities, "running")
	if len(got) != 2 || len(gotIdentities) != 2 {
		t.Errorf("expected two aligned rows matching status Running, got rows=%+v identities=%+v", got, gotIdentities)
	}
	got, gotIdentities = filterRowsWithIdentities(rows, identities, "")
	if len(got) != len(rows) || len(gotIdentities) != len(identities) {
		t.Error("empty filter should return all rows and identities")
	}
}

func TestRefreshRows_DoesNotRecreateTheTable(t *testing.T) {
	m := newTestBrowserModel("default")
	m.SetSize(200, 40)
	m.pods = []cluster.Pod{
		{Name: "alpha", Status: "Running", Ready: "1/1", Age: "1m", Node: "n1"},
		{Name: "beta", Status: "Running", Ready: "1/1", Age: "2m", Node: "n2"},
	}
	m.resourceType = "pods"
	m.rebuildTable()

	cols := m.resourceTable.Columns()
	cols[0].Title = "REFRESH-MARKER"
	m.resourceTable.SetColumns(cols)

	m.filterText = "alpha"
	m.refreshRows(m.resourceTable.Cursor())

	if got := m.resourceTable.Columns()[0].Title; got != "REFRESH-MARKER" {
		t.Errorf("refreshRows must keep the existing table; column title=%q (want %q; recreation lost the marker)",
			got, "REFRESH-MARKER")
	}
}

func TestRefreshRows_AppliesFilterAndRestores(t *testing.T) {
	m := newTestBrowserModel("default")
	m.SetSize(200, 40)
	m.pods = []cluster.Pod{
		{Name: "alpha", Status: "Running", Ready: "1/1", Age: "1m", Node: "n1"},
		{Name: "beta", Status: "Running", Ready: "1/1", Age: "2m", Node: "n2"},
		{Name: "gamma", Status: "Pending", Ready: "0/1", Age: "3m", Node: "n3"},
	}
	m.resourceType = "pods"
	m.rebuildTable()

	m.filterText = "alpha"
	m.refreshRows(m.resourceTable.Cursor())
	if got := len(m.resourceTable.Rows()); got != 1 {
		t.Errorf("filter should leave 1 matching row, got %d", got)
	}

	m.filterText = ""
	m.refreshRows(m.resourceTable.Cursor())
	if got := len(m.resourceTable.Rows()); got != 3 {
		t.Errorf("clearing filter should restore all 3 rows, got %d", got)
	}
}
