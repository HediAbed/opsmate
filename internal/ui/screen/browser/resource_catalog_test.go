package browser

import "testing"

func TestResourceCatalog_CoversEveryRegisteredKind(t *testing.T) {
	for _, kind := range allResourceTypes {
		b, ok := resourceCatalog[kind]
		if !ok {
			t.Errorf("kind %q is in allResourceTypes but missing from resourceCatalog", kind)
			continue
		}
		if b.Singular == "" {
			t.Errorf("kind %q has empty Singular", kind)
		}
		if b.RowsOf == nil {
			t.Errorf("kind %q has nil RowsOf", kind)
		}
		if b.IdentitiesOf == nil {
			t.Errorf("kind %q has nil IdentitiesOf", kind)
		}
		if b.Fetch == nil {
			t.Errorf("kind %q has nil Fetch", kind)
		}
		if b.Clear == nil {
			t.Errorf("kind %q has nil Clear", kind)
		}
		if b.Count == nil {
			t.Errorf("kind %q has nil Count", kind)
		}
	}
}

func TestResourceCatalog_NoOrphansVsAllResourceTypes(t *testing.T) {
	known := make(map[string]bool, len(allResourceTypes))
	for _, k := range allResourceTypes {
		known[k] = true
	}
	for kind := range resourceCatalog {
		if !known[kind] {
			t.Errorf("resourceCatalog has orphan key %q not declared in allResourceTypes", kind)
		}
	}
}

func TestResourceKindSingular_RoutesThroughCatalog(t *testing.T) {
	cases := map[string]string{
		"pods":            "pod",
		"ingresses":       "ingress",
		"networkpolicies": "networkpolicy",
		"pvcs":            "pvc",
		"replicasets":     "replicaset",
	}
	for plural, singular := range cases {
		m := newTestBrowserModel("ns")
		m.SetResourceType(plural)
		if got := m.resourceKindSingular(); got != singular {
			t.Errorf("plural %q → singular %q, want %q", plural, got, singular)
		}
	}
}
