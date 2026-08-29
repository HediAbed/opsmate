package ui

import (
	"testing"

	"github.com/HediAbed/opsmate/internal/cluster"
)

func TestCollectSearchCorpus_DedupsAcrossSources(t *testing.T) {
	m := newTestRootModel(t, "default")

	shared := cluster.Pod{Name: "web-7d9f", Namespace: "default"}
	m.dashboard.pods = []cluster.Pod{shared}
	m.browser.pods = []cluster.Pod{shared}

	got := m.collectSearchCorpus()
	hits := 0
	for _, r := range got {
		if r.Kind == "pod" && r.Name == "web-7d9f" {
			hits++
		}
	}
	if hits != 1 {
		t.Errorf("expected 1 hit for deduplicated pod, got %d", hits)
	}
}

func TestCollectSearchCorpus_SkipsNamelessEntries(t *testing.T) {
	m := newTestRootModel(t, "default")
	m.browser.pods = []cluster.Pod{{Name: "", Namespace: "default"}}

	got := m.collectSearchCorpus()
	if len(got) != 0 {
		t.Errorf("nameless rows should be skipped, got %+v", got)
	}
}

func TestCollectSearchCorpus_CoversAllResourceKinds(t *testing.T) {
	m := newTestRootModel(t, "default")
	m.browser.pods = []cluster.Pod{{Name: "p", Namespace: "ns"}}
	m.browser.deployments = []cluster.Deployment{{Name: "d", Namespace: "ns"}}
	m.browser.services = []cluster.Service{{Name: "s", Namespace: "ns"}}
	m.browser.statefulsets = []cluster.StatefulSet{{Name: "ss", Namespace: "ns"}}
	m.browser.daemonsets = []cluster.DaemonSet{{Name: "ds", Namespace: "ns"}}
	m.browser.configmaps = []cluster.ConfigMap{{Name: "cm", Namespace: "ns"}}
	m.browser.jobs = []cluster.Job{{Name: "j", Namespace: "ns"}}

	kinds := map[string]bool{}
	for _, r := range m.collectSearchCorpus() {
		kinds[r.Kind] = true
	}
	wantKinds := []string{"pod", "deployment", "service", "statefulset", "daemonset", "configmap", "job"}
	for _, k := range wantKinds {
		if !kinds[k] {
			t.Errorf("corpus is missing kind %q", k)
		}
	}
}

func TestFilterSearchResults_SubstringCaseInsensitive(t *testing.T) {
	m := newTestRootModel(t, "default")
	m.browser.pods = []cluster.Pod{
		{Name: "WEB-frontend", Namespace: "default"},
		{Name: "api-backend", Namespace: "default"},
		{Name: "db-primary", Namespace: "default"},
	}
	m.searchCorpus = m.collectSearchCorpus()

	got := m.filterSearchResults("web")
	if len(got) != 1 || got[0].Name != "WEB-frontend" {
		t.Errorf("case-insensitive substring search failed: %+v", got)
	}

	got = m.filterSearchResults("backend")
	if len(got) != 1 || got[0].Name != "api-backend" {
		t.Errorf("backend search failed: %+v", got)
	}

	got = m.filterSearchResults("xyzzy")
	if len(got) != 0 {
		t.Errorf("no-match query should return empty, got %+v", got)
	}
}

func TestFilterSearchResults_EmptyQueryReturnsAll(t *testing.T) {
	m := newTestRootModel(t, "default")
	m.browser.pods = []cluster.Pod{
		{Name: "a", Namespace: "ns"},
		{Name: "b", Namespace: "ns"},
	}
	m.searchCorpus = m.collectSearchCorpus()
	got := m.filterSearchResults("")
	if len(got) != 2 {
		t.Errorf("empty query should return every row, got %d", len(got))
	}
}

func TestOpenSearch_PopulatesCorpusOnce(t *testing.T) {
	m := newTestRootModel(t, "default")
	m.browser.pods = []cluster.Pod{
		{Name: "first", Namespace: "ns"},
	}
	m.openSearch()
	if len(m.searchCorpus) != 1 {
		t.Fatalf("openSearch should snapshot the live data into searchCorpus, got %d", len(m.searchCorpus))
	}

	m.browser.pods = append(m.browser.pods, cluster.Pod{Name: "second", Namespace: "ns"})
	got := m.filterSearchResults("")
	if len(got) != 1 {
		t.Errorf("filter must read from the cached corpus snapshot, got %d", len(got))
	}
}
