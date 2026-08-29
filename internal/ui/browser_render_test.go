package ui

import (
	"strings"
	"testing"

	"github.com/HediAbed/opsmate/internal/cluster"
)

func TestRenderConfirmBox_DeleteAction(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.confirmAction = "delete"
	m.confirmTarget = "pod/alpha"
	out := stripAnsiForTest(m.renderConfirmBox())
	if !strings.Contains(out, "DELETE") {
		t.Errorf("delete confirm should show DELETE label; got %q", out)
	}
	if !strings.Contains(out, "alpha") {
		t.Error("confirm box should show target")
	}
	if !strings.Contains(out, "IRREVERSIBLE") {
		t.Error("delete should warn IRREVERSIBLE")
	}
}

func TestRenderConfirmBox_RestartAction(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.confirmAction = "restart"
	m.confirmTarget = "deploy/web"
	out := stripAnsiForTest(m.renderConfirmBox())
	if !strings.Contains(out, "RESTART") {
		t.Errorf("restart confirm should show RESTART label; got %q", out)
	}
}

func TestRenderConfirmBox_ScaleAction(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.confirmAction = "scale"
	m.confirmTarget = "deploy/web to 3"
	out := stripAnsiForTest(m.renderConfirmBox())
	if !strings.Contains(out, "SCALE") {
		t.Errorf("scale confirm should show SCALE label; got %q", out)
	}
}

func TestConfirmDialogStyle_AllActions(t *testing.T) {
	for _, action := range []string{"delete", "restart", "scale", "unknown"} {
		label, _, border := confirmDialogStyle(action)
		if label == "" {
			t.Errorf("action %q: label should not be empty", action)
		}
		if border == nil {
			t.Errorf("action %q: border color should not be nil", action)
		}
	}
}

func TestRenderScaleBox_RendersTitleAndInput(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.scaleName = "web"
	m.scaleCurrentInfo = "currently 3/3 ready, 3 up-to-date"
	out := stripAnsiForTest(m.renderScaleBox())
	if !strings.Contains(out, "Scale web") {
		t.Errorf("scale box should show 'Scale web'; got %q", out)
	}
	if !strings.Contains(out, "currently 3/3") {
		t.Error("scale box should show current info")
	}
}

func TestRenderScaleBox_NoCurrentInfoSkipsLine(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.scaleName = "web"
	out := stripAnsiForTest(m.renderScaleBox())
	if !strings.Contains(out, "Scale web") {
		t.Error("scale box should still show title without current info")
	}
}

func TestRenderFilterBar_FilterStateShowsInput(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(120, 30)
	m.state = stateFilter
	m.filterInput.Focus()
	out := stripAnsiForTest(m.renderFilterBar())
	if out == "" {
		t.Error("filter state should render filter prompt")
	}
}

func TestRenderFilterBar_ActiveFilterShowsBadge(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(120, 30)
	m.filterActive = true
	m.filterText = "alpha"
	out := stripAnsiForTest(m.renderFilterBar())
	if out == "" {
		t.Error("active filter should render badge")
	}
}

func TestRenderFilterBar_NoFilterReturnsEmpty(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(120, 30)
	if got := m.renderFilterBar(); got != "" {
		t.Error("no filter should return empty filter bar")
	}
}

func TestRenderStatusLine_LoadingShowsSpinner(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(120, 30)
	m.loading = true
	m.statusMsg = "loading"
	out := stripAnsiForTest(m.renderStatusLine())
	if out == "" {
		t.Error("loading status line should render content")
	}
}

func TestRenderStatusLine_SelectionShowsBadge(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(120, 30)
	toggleNamedResource(&m, "a")
	toggleNamedResource(&m, "b")
	out := stripAnsiForTest(m.renderStatusLine())
	if out == "" {
		t.Error("selection status line should render content")
	}
}

func TestLookupReplicaInfo_DeploymentMatches(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetResourceType("deployments")
	m.deployments = []cluster.Deployment{
		{Name: "web", Namespace: "ns", Ready: "3/3", UpToDate: 3},
	}
	got := m.lookupReplicaInfoFor(resourceIdentity{Namespace: "ns", Name: "web"})
	if !strings.Contains(got, "3/3") || !strings.Contains(got, "3 up-to-date") {
		t.Errorf("deployment lookup wrong: %q", got)
	}
}

func TestLookupReplicaInfo_StatefulSetMatches(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetResourceType("statefulsets")
	m.statefulsets = []cluster.StatefulSet{
		{Name: "db", Namespace: "ns", Ready: "2/2", Replicas: 2},
	}
	got := m.lookupReplicaInfoFor(resourceIdentity{Namespace: "ns", Name: "db"})
	if !strings.Contains(got, "2/2") || !strings.Contains(got, "2 replicas") {
		t.Errorf("statefulset lookup wrong: %q", got)
	}
}

func TestLookupReplicaInfo_MissingReturnsEmpty(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetResourceType("deployments")
	if got := m.lookupReplicaInfoFor(resourceIdentity{Namespace: "ns", Name: "does-not-exist"}); got != "" {
		t.Errorf("missing name should return empty; got %q", got)
	}
}

func TestRenderSplitContent_NotEmpty(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.detailContent = "describe output\nline 2"
	m.detailView.SetContent(m.detailContent)
	if got := m.renderSplitContent(40); got == "" {
		t.Error("renderSplitContent should produce non-empty output")
	}
}

func TestRenderSplitContent_TinyHeight(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(120, 6)
	m.detailContent = "x"
	m.detailView.SetContent(m.detailContent)
	if got := m.renderSplitContent(6); got == "" {
		t.Error("tight-height split should still render with floor clamps")
	}
}

func TestLookupReplicaInfo_UnknownResourceTypeReturnsEmpty(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetResourceType("pods")
	if got := m.lookupReplicaInfoFor(resourceIdentity{Namespace: "ns", Name: "anything"}); got != "" {
		t.Errorf("non-deploy/non-sts kind should return empty; got %q", got)
	}
}
