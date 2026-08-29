package ui

import (
	"testing"

	"github.com/HediAbed/opsmate/internal/cluster"
)

func TestBrowserModel_Init_ReturnsSpinnerTick(t *testing.T) {
	m := newTestBrowserModel("ns")
	if cmd := m.Init(); cmd == nil {
		t.Error("Init must return a non-nil cmd (spinner tick)")
	}
}

func TestBrowserModel_ResourceType_ReportsCurrent(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetResourceType("ingresses")
	if got := m.ResourceType(); got != "ingresses" {
		t.Errorf("ResourceType = %q, want ingresses", got)
	}
}

func TestBrowserModel_SelectedResource_ExposesKindAndName(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.SetResourceType("pods")
	m.pods = []cluster.Pod{{Name: "alpha", Namespace: "ns"}}
	m.rebuildTable()
	m.resourceTable.SetCursor(0)

	kind, name := m.SelectedResource()
	if kind != "pod" {
		t.Errorf("kind = %q, want singular 'pod'", kind)
	}
	if name != "alpha" {
		t.Errorf("name = %q, want 'alpha'", name)
	}
}

func TestBrowserModel_SelectedResourceNS_PinnedNamespaceShortCircuits(t *testing.T) {
	m := newTestBrowserModel("kube-system")
	if got := m.selectedResourceNS(); got != "kube-system" {
		t.Errorf("pinned ns short-circuit; got %q", got)
	}
}

func TestBrowserModel_SelectedResourceNS_NoSelectionReturnsEmpty(t *testing.T) {
	m := newTestBrowserModel("")
	m.SetSize(200, 40)
	if got := m.selectedResourceNS(); got != "" {
		t.Errorf("no selection should return empty; got %q", got)
	}
}

func TestBrowserModel_SelectedResourceNS_AllNamespaces_LooksUpCatalog(t *testing.T) {
	m := newTestBrowserModel("")
	m.SetSize(200, 40)
	m.SetResourceType("pods")
	m.pods = []cluster.Pod{
		{Name: "alpha", Namespace: "ns1"},
		{Name: "beta", Namespace: "ns2"},
	}
	m.rebuildTable()
	m.resourceTable.SetCursor(1)
	if got := m.selectedResourceNS(); got != "ns2" {
		t.Errorf("cursor-based ns lookup wrong; got %q", got)
	}
}

func TestBrowserModel_SelectedResource_NewKindReturnsSingular(t *testing.T) {
	cases := map[string]string{
		"ingresses":       "ingress",
		"networkpolicies": "networkpolicy",
		"pvcs":            "pvc",
		"cronjobs":        "cronjob",
		"hpas":            "hpa",
		"secrets":         "secret",
		"replicasets":     "replicaset",
	}
	for plural, singular := range cases {
		m := newTestBrowserModel("ns")
		m.SetSize(200, 40)
		m.SetResourceType(plural)
		kind, _ := m.SelectedResource()
		if kind != singular {
			t.Errorf("plural %q → kind %q, want %q", plural, kind, singular)
		}
	}
}
