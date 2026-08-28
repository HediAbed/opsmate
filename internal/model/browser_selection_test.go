package model

import (
	"strings"
	"testing"

	"github.com/HediAbed/opsmate/internal/service"
)

func TestPodSupportsExec_OnlyRunning(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{"Running", true},
		{"Pending", false},
		{"Succeeded", false},
		{"Failed", false},
		{"Completed", false},
		{"CrashLoopBackOff", false},
		{"ImagePullBackOff", false},
		{"Terminating", false},
		{"Unknown", false},
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			if got := podSupportsExec(tc.status); got != tc.want {
				t.Errorf("podSupportsExec(%q) = %v; want %v", tc.status, got, tc.want)
			}
		})
	}
}

func toggleNamedResource(m *BrowserModel, name string) {
	m.toggleResourceSelection(resourceIdentity{
		Kind:      m.resourceKindSingular(),
		Namespace: m.namespace,
		Name:      name,
	})
}

func TestPodStatus_LookupByName(t *testing.T) {
	m := NewBrowserModel("default")
	m.pods = []service.Pod{
		{Name: "web-1", Namespace: "default", Status: "Running"},
		{Name: "batch-xyz", Namespace: "default", Status: "Succeeded"},
	}

	if status, ok := m.podStatusFor(resourceIdentity{Namespace: "default", Name: "web-1"}); !ok || status != "Running" {
		t.Errorf("web-1: status=%q ok=%v", status, ok)
	}
	if status, ok := m.podStatusFor(resourceIdentity{Namespace: "default", Name: "batch-xyz"}); !ok || status != "Succeeded" {
		t.Errorf("batch-xyz: status=%q ok=%v", status, ok)
	}
	if _, ok := m.podStatusFor(resourceIdentity{Namespace: "default", Name: "missing"}); ok {
		t.Error("missing pod should return ok=false")
	}
}

func TestPodStatusFor_DoesNotTreatMissingNamespaceAsWildcard(t *testing.T) {
	m := NewBrowserModel("")
	m.pods = []service.Pod{{Name: "web", Namespace: "payments", Status: "Running"}}

	if _, ok := m.podStatusFor(resourceIdentity{Name: "web"}); ok {
		t.Fatal("an identity without a namespace matched a namespaced pod")
	}
}

func TestAllNamespaceSelection_QualifiesDuplicateNames(t *testing.T) {
	m := NewBrowserModel("")
	m.SetSize(120, 30)
	m.pods = []service.Pod{
		{Name: "web", Namespace: "payments", Status: "Running"},
		{Name: "web", Namespace: "platform", Status: "Pending"},
	}
	m.rebuildTable()

	rows := m.resourceTable.Rows()
	if rows[0][0] != "payments/web" || rows[1][0] != "platform/web" {
		t.Fatalf("duplicate pod rows are ambiguous: %v", rows)
	}
	m.resourceTable.SetCursor(0)
	first, ok := m.selectedIdentity()
	if !ok {
		t.Fatal("first pod identity is missing")
	}
	m.toggleResourceSelection(first)
	m.resourceTable.SetCursor(1)
	second, ok := m.selectedIdentity()
	if !ok {
		t.Fatal("second pod identity is missing")
	}
	m.toggleResourceSelection(second)
	if first.Namespace != "payments" || second.Namespace != "platform" || len(m.selected) != 2 {
		t.Fatalf("qualified selections = first %+v, second %+v, selected %v", first, second, m.selected)
	}
}

func TestToggleSelection_AddsAndRemoves(t *testing.T) {
	m := NewBrowserModel("default")

	toggleNamedResource(&m, "pod-a")
	if !m.selected["pod-a"] {
		t.Error("pod-a should be selected after first toggle")
	}

	toggleNamedResource(&m, "pod-a")
	if m.selected["pod-a"] {
		t.Error("pod-a should be unselected after second toggle")
	}
}

func TestToggleSelection_MultipleRows(t *testing.T) {
	m := NewBrowserModel("default")
	toggleNamedResource(&m, "a")
	toggleNamedResource(&m, "b")
	toggleNamedResource(&m, "c")
	if len(m.selected) != 3 {
		t.Errorf("len(selected) = %d; want 3", len(m.selected))
	}
}

func TestClearSelection_Resets(t *testing.T) {
	m := NewBrowserModel("default")
	toggleNamedResource(&m, "a")
	toggleNamedResource(&m, "b")
	m.clearSelection()
	if len(m.selected) != 0 {
		t.Errorf("len(selected) after clear = %d; want 0", len(m.selected))
	}
}

func TestDisplayName_PrefixesSelectionMark(t *testing.T) {
	m := NewBrowserModel("default")

	identity := resourceIdentity{Kind: "pod", Namespace: "default", Name: "pod-a"}
	if got := m.displayIdentity(identity); got != "pod-a" {
		t.Errorf("unselected: got %q; want pod-a", got)
	}

	m.toggleResourceSelection(identity)
	got := m.displayIdentity(identity)
	if !strings.HasPrefix(got, selectionMark) {
		t.Errorf("selected name should start with %q, got %q", selectionMark, got)
	}
	if !strings.Contains(got, "pod-a") {
		t.Errorf("original name must survive, got %q", got)
	}
}

func TestSetResourceType_ClearsSelection(t *testing.T) {
	m := NewBrowserModel("default")
	toggleNamedResource(&m, "pod-a")
	if len(m.selected) != 1 {
		t.Fatal("sanity check: expected one selected row")
	}

	m.SetResourceType("deployments")
	if len(m.selected) != 0 {
		t.Errorf("selection should clear on tab switch, len = %d", len(m.selected))
	}
}

func TestSetResourceType_NoOpPreservesSelection(t *testing.T) {
	m := NewBrowserModel("default")
	toggleNamedResource(&m, "pod-a")

	m.SetResourceType("pods")
	if len(m.selected) != 1 {
		t.Errorf("same-tab SetResourceType must not clear selection, len = %d", len(m.selected))
	}
}

func TestSetNamespace_ClearsSelection(t *testing.T) {
	m := NewBrowserModel("default")
	toggleNamedResource(&m, "pod-a")

	m.SetNamespace("kube-system")
	if len(m.selected) != 0 {
		t.Errorf("selection should clear on namespace switch, len = %d", len(m.selected))
	}
}

func TestSelectedNames_ReturnsAllMarked(t *testing.T) {
	m := NewBrowserModel("default")
	toggleNamedResource(&m, "a")
	toggleNamedResource(&m, "b")
	toggleNamedResource(&m, "c")

	got := m.selectedNames()
	want := map[string]bool{"a": true, "b": true, "c": true}
	if len(got) != 3 {
		t.Fatalf("len = %d; want 3", len(got))
	}
	for _, n := range got {
		if !want[n] {
			t.Errorf("unexpected name %q in selection", n)
		}
	}
}

func TestResourceKindSingular_AllKinds(t *testing.T) {
	cases := []struct {
		resourceType, want string
	}{
		{"pods", "pod"},
		{"deployments", "deployment"},
		{"services", "service"},
		{"statefulsets", "statefulset"},
		{"daemonsets", "daemonset"},
		{"configmaps", "configmap"},
		{"nodes", "node"},
		{"jobs", "job"},
		{"foobar", "foobar"},
	}
	for _, tc := range cases {
		m := NewBrowserModel("default")
		m.resourceType = tc.resourceType
		if got := m.resourceKindSingular(); got != tc.want {
			t.Errorf("%q → %q; want %q", tc.resourceType, got, tc.want)
		}
	}
}
