package browser

import (
	"testing"

	"github.com/HediAbed/opsmate/internal/cluster"
)

func TestCurrentResourceRows_RBAC_RendersFiveColumnsAcrossKinds(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.SetResourceType("rbac")
	m.rbac = []cluster.RBAC{
		{Kind: "ServiceAccount", Name: "ci", Namespace: "ns", Count: 0, Scope: "Namespace", Age: "10d"},
		{Kind: "Role", Name: "reader", Namespace: "ns", Count: 2, Scope: "Namespace", Age: "5d"},
		{Kind: "ClusterRoleBinding", Name: "system:cluster-admin", Count: 1, Scope: "Cluster", Age: "1y"},
	}
	rows := m.currentResourceRows()
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	specCols := resourceColSpecs["rbac"]
	for i, row := range rows {
		if len(row) != len(specCols) {
			t.Fatalf("row %d has %d cells, tablespec declares %d columns", i, len(row), len(specCols))
		}
	}

	wantFirst := []string{"ServiceAccount", "ci", "0", "Namespace", "10d"}
	for i, w := range wantFirst {
		if rows[0][i] != w {
			t.Errorf("row 0 col %d (%s) = %q, want %q", i, specCols[i].Title, rows[0][i], w)
		}
	}
	if rows[2][0] != "ClusterRoleBinding" || rows[2][3] != "Cluster" {
		t.Errorf("cluster-scoped row not rendered correctly: %+v", rows[2])
	}
}

func TestSelectedIdentity_RBACUsesRowKindNameAndNamespace(t *testing.T) {
	m := newTestBrowserModel("")
	m.SetSize(120, 30)
	m.SetResourceType("rbac")
	m.rbac = []cluster.RBAC{{Kind: "Role", Name: "reader", Namespace: "payments", Scope: "Namespace"}}
	m.rebuildTable()

	identity, ok := m.selectedIdentity()
	if !ok {
		t.Fatal("RBAC identity is missing")
	}
	want := resourceIdentity{Kind: "role", Namespace: "payments", Name: "reader"}
	if identity != want {
		t.Fatalf("RBAC identity = %+v, want %+v", identity, want)
	}
}
