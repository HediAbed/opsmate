package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/HediAbed/opsmate/internal/cluster"
)

func TestRootHandleSearch_DownArrowAdvancesCursor(t *testing.T) {
	m := freshRoot(t)
	m.showSearch = true
	m.searchResults = []searchResult{{Name: "a"}, {Name: "b"}}
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
	m.searchResults = []searchResult{{Name: "a"}, {Name: "b"}}
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
	m.searchResults = []searchResult{{Kind: "pod", Name: "alpha", Namespace: "ns"}}
	m.searchCursor = 0
	model, cmd := m.handleSearch("enter", tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	r := model.(RootModel)
	if r.showSearch {
		t.Error("enter should close search")
	}
	if cmd == nil {
		t.Fatal("enter should return drill-down cmd")
	}
	msg := cmd()
	if d, ok := msg.(DrillDownMsg); !ok || d.ResourceName != "alpha" {
		t.Errorf("expected DrillDownMsg{Name: alpha}; got %+v", msg)
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
	m.searchCorpus = []searchResult{
		{Kind: "pod", Name: "alpha"},
		{Kind: "pod", Name: "beta"},
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
	m := freshRoot(t)
	m.dashboard.pods = []cluster.Pod{{Name: "dash-pod", Namespace: "ns1"}}
	m.dashboard.deployments = []cluster.Deployment{{Name: "dash-dep", Namespace: "ns1"}}
	m.browser.pods = []cluster.Pod{{Name: "br-pod", Namespace: "ns2"}}
	m.browser.deployments = []cluster.Deployment{{Name: "br-dep", Namespace: "ns2"}}
	m.browser.services = []cluster.Service{{Name: "br-svc", Namespace: "ns2"}}
	m.browser.statefulsets = []cluster.StatefulSet{{Name: "br-sts", Namespace: "ns2"}}
	m.browser.daemonsets = []cluster.DaemonSet{{Name: "br-ds", Namespace: "ns2"}}
	m.browser.configmaps = []cluster.ConfigMap{{Name: "br-cm", Namespace: "ns2"}}
	m.browser.jobs = []cluster.Job{{Name: "br-job", Namespace: "ns2"}}
	m.logs.pods = []cluster.Pod{{Name: "logs-pod", Namespace: "ns3"}}

	const minimumCorpusSize = 9
	got := m.collectSearchCorpus()
	if len(got) < minimumCorpusSize {
		t.Errorf("expected at least %d corpus entries; got %d (%+v)", minimumCorpusSize, len(got), got)
	}
}

func TestRootCollectSearchCorpus_DedupesIdenticalEntries(t *testing.T) {
	m := freshRoot(t)
	m.dashboard.pods = []cluster.Pod{{Name: "shared", Namespace: "ns"}}
	m.browser.pods = []cluster.Pod{{Name: "shared", Namespace: "ns"}}
	got := m.collectSearchCorpus()
	count := 0
	for _, r := range got {
		if r.Name == "shared" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("duplicate pod entries should be deduped; got %d", count)
	}
}

func TestRootFilterSearchResults_EmptyQueryReturnsCorpus(t *testing.T) {
	m := freshRoot(t)
	m.searchCorpus = []searchResult{{Name: "a"}, {Name: "b"}}
	got := m.filterSearchResults("")
	if len(got) != 2 {
		t.Errorf("empty query should return full corpus; got %d", len(got))
	}
}

func TestRootFilterSearchResults_QueryFilters(t *testing.T) {
	m := freshRoot(t)
	m.searchCorpus = []searchResult{
		{Name: "alpha"},
		{Name: "beta"},
		{Name: "gamma-alpha"},
	}
	got := m.filterSearchResults("alpha")
	if len(got) != 2 {
		t.Errorf("query 'alpha' should match 2 entries; got %d", len(got))
	}
}
