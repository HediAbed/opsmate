package ui

import (
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/HediAbed/opsmate/internal/cluster"
	screenmodel "github.com/HediAbed/opsmate/internal/ui/screen"
)

func TestRootHandleSearch_DownArrowAdvancesCursor(t *testing.T) {
	m := freshRoot(t)
	m.showSearch = true
	m.searchResults = []screenmodel.SearchItem{{Name: "a"}, {Name: "b"}}
	m.searchCursor = 0
	model, _ := m.handleSearch("down", tea.KeyPressMsg{Code: tea.KeyDown, Text: "down"})
	r := model.(RootModel)
	if r.searchCursor != 1 {
		t.Errorf("down should advance cursor; got %d", r.searchCursor)
	}
}

func TestRootHandleSearch_UpArrowRetractsCursor(t *testing.T) {
	m := freshRoot(t)
	m.showSearch = true
	m.searchResults = []screenmodel.SearchItem{{Name: "a"}, {Name: "b"}}
	m.searchCursor = 1
	model, _ := m.handleSearch("up", tea.KeyPressMsg{Code: tea.KeyUp, Text: "up"})
	r := model.(RootModel)
	if r.searchCursor != 0 {
		t.Errorf("up should retract cursor; got %d", r.searchCursor)
	}
}

func TestRootHandleSearch_EnterDrillsDown(t *testing.T) {
	m := freshRoot(t)
	m.showSearch = true
	m.searchResults = []screenmodel.SearchItem{{Kind: screenmodel.ResourceKindPod, Name: "alpha", Namespace: "ns"}}
	m.searchCursor = 0
	model, cmd := m.handleSearch("enter", tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	r := model.(RootModel)
	if r.showSearch {
		t.Error("enter should close search")
	}
	if cmd == nil {
		t.Fatal("enter should return drill-down cmd")
	}
	message := cmd()
	drillDown, ok := message.(DrillDownMsg)
	if !ok {
		t.Fatalf("search command returned %T", message)
	}
	want := DrillDownMsg{Screen: ScreenBrowser, ResourceType: "pod", ResourceName: "alpha", ResourceNS: "ns"}
	if drillDown != want {
		t.Fatalf("search drill-down = %+v, want %+v", drillDown, want)
	}
}

func TestRootHandleSearch_EnterEmptyResultsIsNoOp(t *testing.T) {
	m := freshRoot(t)
	m.showSearch = true
	m.searchResults = nil
	model, cmd := m.handleSearch("enter", tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	r := model.(RootModel)
	if !r.showSearch {
		t.Error("enter with no results should not close search")
	}
	if cmd != nil {
		t.Error("enter with no results should not return a cmd")
	}
}

func TestRootHandleSearch_TypingFiltersCorpus(t *testing.T) {
	m := freshRoot(t)
	m.showSearch = true
	m.searchInput.Focus()
	m.searchCorpus = []screenmodel.SearchItem{
		{Kind: screenmodel.ResourceKindPod, Name: "alpha"},
		{Kind: screenmodel.ResourceKindPod, Name: "beta"},
	}
	m.searchResults = m.searchCorpus
	model, _ := m.handleSearch("l", tea.KeyPressMsg{Code: 'l', Text: "l"})
	r := model.(RootModel)
	if r.searchInput.Value() != "l" {
		t.Fatalf("search input = %q, want l", r.searchInput.Value())
	}
	if len(r.searchResults) != 1 || r.searchResults[0].Name != "alpha" {
		t.Fatalf("filtered results = %+v", r.searchResults)
	}
}

func TestRootCollectSearchCorpus_AggregatesAcrossSubmodels(t *testing.T) {
	model := newTestRootModelWithObserver(t, "ns1", &testResourceObserver{
		resourceName:      "dashboard-resource",
		resourceNamespace: "ns1",
	})
	for _, message := range commandMessages(t, model.dashboard.Activate()) {
		model.dashboard, _ = model.dashboard.Update(message)
	}
	t.Cleanup(func() { model.dashboard.Deactivate() })
	seedRootBrowser(&model,
		cluster.PodsMsg{Pods: []cluster.Pod{{Name: "br-pod", Namespace: "ns2"}}},
		cluster.DeploymentsMsg{Deployments: []cluster.Deployment{{Name: "br-dep", Namespace: "ns2"}}},
		cluster.ServicesMsg{Services: []cluster.Service{{Name: "br-svc", Namespace: "ns2"}}},
		cluster.StatefulSetsMsg{StatefulSets: []cluster.StatefulSet{{Name: "br-sts", Namespace: "ns2"}}},
		cluster.DaemonSetsMsg{DaemonSets: []cluster.DaemonSet{{Name: "br-ds", Namespace: "ns2"}}},
		cluster.ConfigMapsMsg{ConfigMaps: []cluster.ConfigMap{{Name: "br-cm", Namespace: "ns2"}}},
		cluster.JobsMsg{Jobs: []cluster.Job{{Name: "br-job", Namespace: "ns2"}}},
	)
	model.logs, _ = model.logs.Update(cluster.PodsMsg{Pods: []cluster.Pod{{Name: "logs-pod", Namespace: "ns3"}}})

	const expectedCorpusSize = 10
	results := model.collectSearchCorpus()
	if len(results) != expectedCorpusSize {
		t.Fatalf("search corpus has %d entries, want %d: %+v", len(results), expectedCorpusSize, results)
	}
	if !slices.Contains(results, screenmodel.SearchItem{Kind: screenmodel.ResourceKindPod, Name: "dashboard-resource", Namespace: "ns1"}) {
		t.Fatalf("search corpus omitted dashboard pod: %+v", results)
	}
	if !slices.Contains(results, screenmodel.SearchItem{Kind: screenmodel.ResourceKindDeployment, Name: "dashboard-resource", Namespace: "ns1"}) {
		t.Fatalf("search corpus omitted dashboard deployment: %+v", results)
	}
}

func TestRootCollectSearchCorpus_DedupesIdenticalEntries(t *testing.T) {
	m := newTestRootModelWithObserver(t, "ns", &testResourceObserver{
		resourceName:      "shared",
		resourceNamespace: "ns",
	})
	for _, message := range commandMessages(t, m.dashboard.Activate()) {
		m.dashboard, _ = m.dashboard.Update(message)
	}
	t.Cleanup(func() { m.dashboard.Deactivate() })
	shared := cluster.Pod{Name: "shared", Namespace: "ns"}
	seedRootBrowser(&m, cluster.PodsMsg{Pods: []cluster.Pod{shared}})
	m.logs, _ = m.logs.Update(cluster.PodsMsg{Pods: []cluster.Pod{shared}})
	got := m.collectSearchCorpus()
	count := 0
	for _, result := range got {
		if result.Kind == screenmodel.ResourceKindPod && result.Name == shared.Name && result.Namespace == shared.Namespace {
			count++
		}
	}
	if count != 1 {
		t.Errorf("dashboard, browser, and logs pod entries were not deduplicated: count=%d corpus=%+v", count, got)
	}
}

func seedRootBrowser(model *RootModel, messages ...tea.Msg) {
	for _, message := range messages {
		model.browser, _ = model.browser.Update(message)
	}
}

func TestRootFilterSearchResults_EmptyQueryReturnsCorpus(t *testing.T) {
	m := freshRoot(t)
	m.searchCorpus = []screenmodel.SearchItem{{Name: "a"}, {Name: "b"}}
	got := m.filterSearchResults("")
	if len(got) != 2 {
		t.Errorf("empty query should return full corpus; got %d", len(got))
	}
}

func TestRootFilterSearchResults_QueryFilters(t *testing.T) {
	m := freshRoot(t)
	m.searchCorpus = []screenmodel.SearchItem{
		{Name: "alpha"},
		{Name: "beta"},
		{Name: "gamma-alpha"},
	}
	got := m.filterSearchResults("alpha")
	if len(got) != 2 {
		t.Errorf("query 'alpha' should match 2 entries; got %d", len(got))
	}
}

func TestUniqueSearchResultsRejectsInvalidEntries(t *testing.T) {
	valid := screenmodel.SearchItem{Kind: screenmodel.ResourceKindPod, Name: "api", Namespace: "payments"}
	results := uniqueSearchResults([]screenmodel.SearchItem{
		valid,
		valid,
		{Kind: screenmodel.ResourceKind("pods"), Name: "plural-kind", Namespace: "payments"},
		{Kind: screenmodel.ResourceKindPod, Namespace: "payments"},
	})
	if !slices.Equal(results, []screenmodel.SearchItem{valid}) {
		t.Fatalf("validated search results = %+v, want only %+v", results, valid)
	}
}
