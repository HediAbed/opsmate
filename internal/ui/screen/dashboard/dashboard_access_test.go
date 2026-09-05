package dashboard

import (
	"strings"
	"testing"

	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/failure"
	"github.com/HediAbed/opsmate/internal/kube"
	clusterui "github.com/HediAbed/opsmate/internal/ui/cluster"
	"github.com/HediAbed/opsmate/internal/ui/screen"
)

func deniedObserveError(subject kube.Subject) error {
	return &kube.Error{Operation: kube.OperationObserve, Subject: subject, Code: failure.CodePermissionDenied}
}

func TestDashboardDeniedPodsRenderCalmNoticeInsteadOfError(t *testing.T) {
	model := newTestDashboardModel("payments")
	model.SetSize(120, 40)
	model.loading = true
	denied := deniedObserveError(kube.SubjectPods)
	closed := dashboardPodSnapshot(&model, clusterui.LiveState[cluster.Pod]{Err: denied})
	closed.Closed = true

	model, command := model.Update(closed)
	if command != nil || model.loading {
		t.Fatalf("denied pod closure = command:%v loading:%v", command != nil, model.loading)
	}
	if model.err != nil {
		t.Fatalf("denied pods surfaced as dashboard error: %v", model.err)
	}
	view := stripAnsiForTest(model.View())
	if !strings.Contains(view, "Not permitted: list pods") || strings.Contains(view, "ERROR") {
		t.Fatalf("dashboard view = %q, want calm pod notice without error banner", view)
	}
}

func TestDashboardDeniedDeploymentsAndEventsRenderSectionNotices(t *testing.T) {
	model := newTestDashboardModel("payments")
	model.SetSize(120, 40)
	deploymentsClosed := dashboardDeploymentSnapshot(&model, clusterui.LiveState[cluster.Deployment]{Err: deniedObserveError(kube.SubjectDeployments)})
	deploymentsClosed.Closed = true
	model, _ = model.Update(deploymentsClosed)
	eventsClosed := dashboardEventSnapshot(&model, clusterui.LiveState[cluster.Event]{Err: deniedObserveError(kube.SubjectEvents)})
	eventsClosed.Closed = true
	model, _ = model.Update(eventsClosed)

	if model.err != nil {
		t.Fatalf("denied sections surfaced as dashboard error: %v", model.err)
	}
	view := stripAnsiForTest(model.View())
	for _, want := range []string{"DEPLOYMENT HEALTH", "Not permitted: list deployments", "RECENT EVENTS", "Not permitted: list events", "Deploys:n/a"} {
		if !strings.Contains(view, want) {
			t.Fatalf("dashboard view missing %q:\n%s", want, view)
		}
	}
}

func TestDashboardNamespaceChangeClearsStaleDenials(t *testing.T) {
	model := newTestDashboardModel("payments")
	model.SetSize(120, 40)
	deploymentsClosed := dashboardDeploymentSnapshot(&model, clusterui.LiveState[cluster.Deployment]{Err: deniedObserveError(kube.SubjectDeployments)})
	deploymentsClosed.Closed = true
	model, _ = model.Update(deploymentsClosed)
	if !model.deploymentsDenied() {
		t.Fatal("setup: deployments must be denied before the namespace change")
	}

	model.SetNamespace("checkout")
	if model.deploymentsDenied() || model.err != nil {
		t.Fatalf("stale denial survived namespace change: denied=%v err=%v", model.deploymentsDenied(), model.err)
	}
	if view := stripAnsiForTest(model.View()); strings.Contains(view, "Not permitted") || strings.Contains(view, "Deploys:n/a") {
		t.Fatalf("dashboard view for new namespace still shows old denial:\n%s", view)
	}
}

func TestDashboardNonPermissionClosureStillReportsError(t *testing.T) {
	model := newTestDashboardModel("payments")
	model.SetSize(120, 40)
	closed := dashboardPodSnapshot(&model, clusterui.LiveState[cluster.Pod]{})
	closed.Closed = true
	model, _ = model.Update(closed)
	if model.err == nil || !strings.Contains(model.err.Error(), screen.ErrLiveUpdatesStopped.Error()) {
		t.Fatalf("plain closure error = %v, want live updates stopped", model.err)
	}
	if !strings.Contains(stripAnsiForTest(model.View()), "ERROR") {
		t.Fatal("plain closure must keep the error banner")
	}
}
