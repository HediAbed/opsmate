package crds

import (
	"testing"

	"github.com/HediAbed/opsmate/internal/cluster"
)

func TestCRDListResultRequiresCurrentNamespace(t *testing.T) {
	model := newTestCRDsModel("team-a")
	current := model.fetchCRDList()().(crdResultMsg)
	current.payload = cluster.CRDsMsg{CRDs: []cluster.CRD{{Name: "local"}}}
	foreign := current
	foreign.namespace = "team-b"
	foreign.payload = cluster.CRDsMsg{CRDs: []cluster.CRD{{Name: "foreign"}}}

	if !model.Accepts(foreign) {
		t.Fatal("CRD screen did not claim its routed result type")
	}
	unchanged, command := model.Update(foreign)
	if command != nil || len(unchanged.crds) != 0 || unchanged.namespace != "team-a" {
		t.Fatalf("foreign result changed model: namespace=%q crds=%v command=%v", unchanged.namespace, unchanged.crds, command)
	}
	if !model.Accepts(current) {
		t.Fatal("CRD screen rejected its current result")
	}
	updated, command := model.Update(current)
	if command != nil || len(updated.crds) != 1 || updated.crds[0].Name != "local" {
		t.Fatalf("current result = crds:%v command:%v", updated.crds, command)
	}
}

func TestCRDListResultRequiresCurrentRequest(t *testing.T) {
	model := newTestCRDsModel("team-a")
	stale := model.fetchCRDList()().(crdResultMsg)
	_ = model.fetchCRDList()
	stale.payload = cluster.CRDsMsg{CRDs: []cluster.CRD{{Name: "stale"}}}

	unchanged, command := model.Update(stale)
	if command != nil || len(unchanged.crds) != 0 {
		t.Fatalf("superseded list result changed model: crds=%v command=%v", unchanged.crds, command)
	}
}

func TestCRDListResultRejectedInInstanceView(t *testing.T) {
	model := newTestCRDsModel("team-a")
	current := model.fetchCRDList()().(crdResultMsg)
	current.payload = cluster.CRDsMsg{CRDs: []cluster.CRD{{Name: "local"}}}
	model.view = crdsViewInstances

	unchanged, command := model.Update(current)
	if command != nil || len(unchanged.crds) != 0 {
		t.Fatalf("list result in instance view changed model: crds=%v command=%v", unchanged.crds, command)
	}
}
