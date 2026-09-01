package browser

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/HediAbed/opsmate/internal/analysis"
	"github.com/HediAbed/opsmate/internal/cluster"
)

func TestBrowserUpdate_WindowSize(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(50, 20)
	out, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 40})
	if out.width != 200 || out.height != 40 {
		t.Errorf("WindowSize not applied; got width=%d height=%d", out.width, out.height)
	}
}

func TestBrowserUpdate_PodsMsgPopulates(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(120, 40)
	out, _ := m.Update(cluster.PodsMsg{Pods: []cluster.Pod{{Name: "p"}}})
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
			model := newTestBrowserModel("ns")
			model.SetSize(120, 40)
			model.SetResourceType(test.resourceType)
			updated, _ := model.Update(test.message)
			assertFetchedBrowserResource(t, updated, test)
		})
	}
}

func browserFetchCases() []browserFetchCase {
	return []browserFetchCase{
		{"deployments", cluster.DeploymentsMsg{Deployments: []cluster.Deployment{{Name: "d"}}}, "d"},
		{"services", cluster.ServicesMsg{Services: []cluster.Service{{Name: "s"}}}, "s"},
		{"statefulsets", cluster.StatefulSetsMsg{StatefulSets: []cluster.StatefulSet{{Name: "ss"}}}, "ss"},
		{"daemonsets", cluster.DaemonSetsMsg{DaemonSets: []cluster.DaemonSet{{Name: "ds"}}}, "ds"},
		{"configmaps", cluster.ConfigMapsMsg{ConfigMaps: []cluster.ConfigMap{{Name: "cm"}}}, "cm"},
		{"nodes", cluster.NodesMsg{Nodes: []cluster.Node{{Name: "n"}}}, "n"},
		{"jobs", cluster.JobsMsg{Jobs: []cluster.Job{{Name: "j"}}}, "j"},
		{"ingresses", cluster.IngressesMsg{Ingresses: []cluster.Ingress{{Name: "i"}}}, "i"},
		{"networkpolicies", cluster.NetworkPoliciesMsg{NetworkPolicies: []cluster.NetworkPolicy{{Name: "np"}}}, "np"},
		{"pvcs", cluster.PVCsMsg{PVCs: []cluster.PersistentVolumeClaim{{Name: "v"}}}, "v"},
		{"cronjobs", cluster.CronJobsMsg{CronJobs: []cluster.CronJob{{Name: "c"}}}, "c"},
		{"hpas", cluster.HPAsMsg{HPAs: []cluster.HPA{{Name: "h"}}}, "h"},
		{"secrets", cluster.SecretsMsg{Secrets: []cluster.Secret{{Name: "s"}}}, "s"},
		{"replicasets", cluster.ReplicaSetsMsg{ReplicaSets: []cluster.ReplicaSet{{Name: "rs"}}}, "rs"},
		{"rbac", cluster.RBACMsg{RBAC: []cluster.RBAC{{Name: "r"}}}, "r"},
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
	m := newTestBrowserModel("ns")
	m.SetSize(120, 40)
	out, _ := m.Update(cluster.DescribeMsg{Output: "describe text"})
	if !out.showDetail {
		t.Error("DescribeMsg should set showDetail")
	}
	if out.detailContent == "" {
		t.Error("DescribeMsg should populate detailContent")
	}
}

func TestBrowserUpdate_DescribeMsg_ErrorSetsBanner(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(120, 40)
	out, _ := m.Update(cluster.DescribeMsg{Err: errStub("denied")})
	if out.errBanner == "" {
		t.Error("describe err should set banner")
	}
}

func TestBrowserUpdate_DescribeSummaryMsg_Success(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(120, 40)
	m.analysisSummaryLoading = true
	out, _ := m.Update(analysis.DescribeSummaryMsg{Summary: "running healthy"})
	if out.analysisSummaryLoading {
		t.Error("DescribeSummaryMsg should clear loading")
	}
	if out.analysisSummary == "" {
		t.Error("aiSummary should be populated")
	}
}

func TestBrowserUpdate_DescribeSummaryMsg_Error(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(120, 40)
	m.analysisSummaryLoading = true
	out, _ := m.Update(analysis.DescribeSummaryMsg{Err: errStub("rate limit")})
	if out.analysisSummaryErr == nil {
		t.Error("DescribeSummary err should propagate")
	}
}

func TestBrowserUpdate_EventsMsg_PopulatesDetail(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(120, 40)
	out, _ := m.Update(cluster.EventsMsg{Events: []cluster.Event{{Reason: "Killed", Message: "oom"}}})
	if !out.showDetail {
		t.Error("EventsMsg should show detail")
	}
	if out.detailKind != "events" {
		t.Errorf("detailKind = %q", out.detailKind)
	}
}

func TestBrowserUpdate_EventsMsg_Error(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(120, 40)
	out, _ := m.Update(cluster.EventsMsg{Err: errStub("denied")})
	if out.errBanner == "" {
		t.Error("EventsMsg err should set banner")
	}
}

func TestBrowserUpdate_YAMLMsg_Success(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(120, 40)
	out, _ := m.Update(cluster.YAMLMsg{Output: "apiVersion: v1\nkind: Pod"})
	if !out.showDetail {
		t.Error("YAMLMsg should show detail")
	}
	if out.detailKind != "yaml" {
		t.Errorf("detailKind = %q", out.detailKind)
	}
}

func TestBrowserUpdate_YAMLMsg_Error(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(120, 40)
	out, _ := m.Update(cluster.YAMLMsg{Err: errStub("denied")})
	if out.errBanner == "" {
		t.Error("YAMLMsg err should set banner")
	}
}

func TestBrowserUpdateMutationResultSuccessRefetches(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(120, 40)
	m.showConfirm = true
	m.state = stateDeleteConfirm
	_, cmd := m.Update(cluster.MutationResultMsg{Output: "deleted"})
	if cmd == nil {
		t.Error("successful mutation should refetch resources")
	}
}

func TestBrowserUpdateMutationResultErrorShowsStatus(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(120, 40)
	out, _ := m.Update(cluster.MutationResultMsg{Err: errStub("forbidden")})
	if out.statusMsg == "" {
		t.Error("mutation error should set status")
	}
}
