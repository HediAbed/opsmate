package model

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/service"
)

func TestCurrentResourceRows_Secrets_RendersFourColumns(t *testing.T) {
	m := NewBrowserModel("ns")
	m.SetSize(160, 40)
	m.SetResourceType("secrets")
	m.secrets = []service.Secret{
		{Name: "creds", Namespace: "ns", Type: "Opaque", Data: 3, Age: "1h"},
	}
	rows := m.currentResourceRows()
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	row := rows[0]
	specCols := resourceColSpecs["secrets"]
	if len(row) != len(specCols) {
		t.Fatalf("row has %d cells, tablespec declares %d columns", len(row), len(specCols))
	}
	want := []string{"creds", "Opaque", "3", "1h"}
	for i, w := range want {
		if row[i] != w {
			t.Errorf("col %d (%s) = %q, want %q", i, specCols[i].Title, row[i], w)
		}
	}
}

func TestSecretYAMLActionIsBlocked(t *testing.T) {
	m := NewBrowserModel("ns")
	m.SetSize(160, 40)
	m.SetResourceType("secrets")
	m.secrets = []service.Secret{{Name: "credentials", Namespace: "ns"}}
	m.rebuildTable()
	m.loading = false

	out, cmd := m.handleKey(tea.KeyPressMsg{Code: 'y', Text: "y"})

	if cmd != nil {
		t.Fatal("secret YAML action must not dispatch a command")
	}
	if out.loading {
		t.Error("blocked secret YAML action must not enter loading state")
	}
	if !strings.Contains(out.statusMsg, restrictedResourceYAMLMessage) {
		t.Errorf("status = %q, want a safe-data warning", out.statusMsg)
	}
}

func TestSecretDetailCannotBeSentForAnalysis(t *testing.T) {
	const encodedValue = "c3VwZXItc2VjcmV0"
	m := NewBrowserModel("ns")
	m.SetSize(160, 40)
	m.SetResourceType("secrets")
	m.secrets = []service.Secret{{Name: "credentials", Namespace: "ns"}}
	m.rebuildTable()
	m.state = stateDetail
	m.showDetail = true
	m.detailKind = "describe"
	m.detailContent = "data:\n  token: " + encodedValue

	out, cmd := m.handleKey(tea.KeyPressMsg{Code: 'a', Text: "a"})

	if cmd != nil {
		t.Fatal("secret detail analysis must not dispatch a command")
	}
	if out.aiSummaryLoad {
		t.Error("blocked secret analysis must not enter loading state")
	}
	if strings.Contains(out.providerDetailContext(), encodedValue) {
		t.Fatal("provider detail context contains secret data")
	}
}

func TestRootProviderContextExcludesSecretDetail(t *testing.T) {
	const encodedValue = "c3VwZXItc2VjcmV0"
	m := NewRootModel("ns")
	m.screen = ScreenBrowser
	m.browser.SetResourceType("secrets")
	m.browser.detailKind = "yaml"
	m.browser.detailContent = "data:\n  token: " + encodedValue

	m.updateAIScreenContext()

	if strings.Contains(m.aiPanel.screenContext, encodedValue) {
		t.Fatal("provider screen context contains secret data")
	}
}
