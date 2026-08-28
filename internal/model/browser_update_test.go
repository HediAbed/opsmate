package model

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/service"
)

func TestBrowserUpdate_WindowSize(t *testing.T) {
	m := NewBrowserModel("ns")
	m.SetSize(50, 20)
	out, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 40})
	if out.width != 200 || out.height != 40 {
		t.Errorf("WindowSize not applied; got width=%d height=%d", out.width, out.height)
	}
}

func TestBrowserUpdate_PodsMsgPopulates(t *testing.T) {
	m := NewBrowserModel("ns")
	m.SetSize(120, 40)
	out, _ := m.Update(service.PodsMsg{Pods: []service.Pod{{Name: "p"}}})
	if len(out.pods) != 1 {
		t.Errorf("PodsMsg should populate pods; got %d", len(out.pods))
	}
}

type browserFetchCase struct {
	resourceType string
	message      tea.Msg
	wantName     string
}

func TestBrowserUpdate_AllFetchMsgsPopulate(t *testing.T) {
	for _, test := range browserFetchCases() {
		t.Run(test.resourceType, func(t *testing.T) {
			model := NewBrowserModel("ns")
			model.SetSize(120, 40)
			model.SetResourceType(test.resourceType)
			updated, _ := model.Update(test.message)
			assertFetchedBrowserResource(t, updated, test)
		})
	}
}

func browserFetchCases() []browserFetchCase {
	return []browserFetchCase{
		{"deployments", service.DeploymentsMsg{Deployments: []service.Deployment{{Name: "d"}}}, "d"},
		{"services", service.ServicesMsg{Services: []service.Service{{Name: "s"}}}, "s"},
		{"statefulsets", service.StatefulSetsMsg{StatefulSets: []service.StatefulSet{{Name: "ss"}}}, "ss"},
		{"daemonsets", service.DaemonSetsMsg{DaemonSets: []service.DaemonSet{{Name: "ds"}}}, "ds"},
		{"configmaps", service.ConfigMapsMsg{ConfigMaps: []service.ConfigMap{{Name: "cm"}}}, "cm"},
		{"nodes", service.NodesMsg{Nodes: []service.Node{{Name: "n"}}}, "n"},
		{"jobs", service.JobsMsg{Jobs: []service.Job{{Name: "j"}}}, "j"},
		{"ingresses", service.IngressesMsg{Ingresses: []service.Ingress{{Name: "i"}}}, "i"},
		{"networkpolicies", service.NetworkPoliciesMsg{NetworkPolicies: []service.NetworkPolicy{{Name: "np"}}}, "np"},
		{"pvcs", service.PVCsMsg{PVCs: []service.PersistentVolumeClaim{{Name: "v"}}}, "v"},
		{"cronjobs", service.CronJobsMsg{CronJobs: []service.CronJob{{Name: "c"}}}, "c"},
		{"hpas", service.HPAsMsg{HPAs: []service.HPA{{Name: "h"}}}, "h"},
		{"secrets", service.SecretsMsg{Secrets: []service.Secret{{Name: "s"}}}, "s"},
		{"replicasets", service.ReplicaSetsMsg{ReplicaSets: []service.ReplicaSet{{Name: "rs"}}}, "rs"},
		{"rbac", service.RBACMsg{RBAC: []service.RBAC{{Name: "r"}}}, "r"},
	}
}

func assertFetchedBrowserResource(t *testing.T, model BrowserModel, test browserFetchCase) {
	t.Helper()
	binding := resourceCatalog[test.resourceType]
	identities := binding.IdentitiesOf(&model)
	if len(identities) != 1 {
		t.Fatalf("%s count = %d, want 1", test.resourceType, len(identities))
	}
	if identities[0].Name != test.wantName {
		t.Errorf("%s name = %q, want %q", test.resourceType, identities[0].Name, test.wantName)
	}
}

func TestBrowserUpdate_DescribeMsgPopulatesDetail(t *testing.T) {
	m := NewBrowserModel("ns")
	m.SetSize(120, 40)
	out, _ := m.Update(service.DescribeMsg{Output: "describe text"})
	if !out.showDetail {
		t.Error("DescribeMsg should set showDetail")
	}
	if out.detailContent == "" {
		t.Error("DescribeMsg should populate detailContent")
	}
}

func TestBrowserUpdate_DescribeMsg_ErrorSetsBanner(t *testing.T) {
	m := NewBrowserModel("ns")
	m.SetSize(120, 40)
	out, _ := m.Update(service.DescribeMsg{Err: errStub("denied")})
	if out.errBanner == "" {
		t.Error("describe err should set banner")
	}
}

func TestBrowserUpdate_DescribeSummaryMsg_Success(t *testing.T) {
	m := NewBrowserModel("ns")
	m.SetSize(120, 40)
	m.aiSummaryLoad = true
	out, _ := m.Update(service.DescribeSummaryMsg{Summary: "running healthy"})
	if out.aiSummaryLoad {
		t.Error("DescribeSummaryMsg should clear loading")
	}
	if out.aiSummary == "" {
		t.Error("aiSummary should be populated")
	}
}

func TestBrowserUpdate_DescribeSummaryMsg_Error(t *testing.T) {
	m := NewBrowserModel("ns")
	m.SetSize(120, 40)
	m.aiSummaryLoad = true
	out, _ := m.Update(service.DescribeSummaryMsg{Err: errStub("rate limit")})
	if out.aiSummaryErr == nil {
		t.Error("DescribeSummary err should propagate")
	}
}

func TestBrowserUpdate_EventsMsg_PopulatesDetail(t *testing.T) {
	m := NewBrowserModel("ns")
	m.SetSize(120, 40)
	out, _ := m.Update(service.EventsMsg{Events: []service.Event{{Reason: "Killed", Message: "oom"}}})
	if !out.showDetail {
		t.Error("EventsMsg should show detail")
	}
	if out.detailKind != "events" {
		t.Errorf("detailKind = %q", out.detailKind)
	}
}

func TestBrowserUpdate_EventsMsg_Error(t *testing.T) {
	m := NewBrowserModel("ns")
	m.SetSize(120, 40)
	out, _ := m.Update(service.EventsMsg{Err: errStub("denied")})
	if out.errBanner == "" {
		t.Error("EventsMsg err should set banner")
	}
}

func TestBrowserUpdate_YAMLMsg_Success(t *testing.T) {
	m := NewBrowserModel("ns")
	m.SetSize(120, 40)
	out, _ := m.Update(service.YAMLMsg{Output: "apiVersion: v1\nkind: Pod"})
	if !out.showDetail {
		t.Error("YAMLMsg should show detail")
	}
	if out.detailKind != "yaml" {
		t.Errorf("detailKind = %q", out.detailKind)
	}
}

func TestBrowserUpdate_YAMLMsg_Error(t *testing.T) {
	m := NewBrowserModel("ns")
	m.SetSize(120, 40)
	out, _ := m.Update(service.YAMLMsg{Err: errStub("denied")})
	if out.errBanner == "" {
		t.Error("YAMLMsg err should set banner")
	}
}

func TestBrowserUpdate_CommandResultMsg_SuccessRefetches(t *testing.T) {
	m := NewBrowserModel("ns")
	m.SetSize(120, 40)
	m.showConfirm = true
	m.state = stateDeleteConfirm
	_, cmd := m.Update(service.CommandResultMsg{Output: "deleted"})
	if cmd == nil {
		t.Error("CommandResultMsg success should refetch resources")
	}
}

func TestBrowserUpdate_CommandResultMsg_ErrorShowsStatus(t *testing.T) {
	m := NewBrowserModel("ns")
	m.SetSize(120, 40)
	out, _ := m.Update(service.CommandResultMsg{Err: errStub("forbidden")})
	if out.statusMsg == "" {
		t.Error("CommandResultMsg err should set status")
	}
}

func TestBrowserUpdate_SharedWatchClosedRoutesToHandler(t *testing.T) {
	m := NewBrowserModel("ns")
	m.SetSize(120, 40)
	m.active = false
	out, cmd := m.Update(browserWatchClosedMsg{Kind: "pods"})
	if cmd != nil {
		t.Error("inactive screen should produce no reconnect cmd")
	}
	_ = out
}
