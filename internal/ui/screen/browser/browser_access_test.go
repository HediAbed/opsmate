package browser

import (
	"strings"
	"testing"

	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/failure"
	"github.com/HediAbed/opsmate/internal/kube"
	clusterui "github.com/HediAbed/opsmate/internal/ui/cluster"
	"github.com/HediAbed/opsmate/internal/ui/screen"
)

func deniedError(operation kube.Operation, subject kube.Subject) error {
	return &kube.Error{Operation: operation, Subject: subject, Code: failure.CodePermissionDenied}
}

func TestBrowserDeniedListRendersNoticeWithoutErrorBanner(t *testing.T) {
	model := newTestBrowserModel("payments")
	model.SetSize(120, 40)
	model.active = true
	model.resourceType = resourceTypePods
	set := newTestResourceLiveSet(clusterui.LiveState[cluster.Pod]{Err: deniedError(kube.OperationObserve, kube.SubjectPods)})
	model.podLive.Set(set)
	closed := screen.LiveMessage{Generation: model.podLive.Generation(), Closed: true, Payload: screen.LiveSnapshot[cluster.Pod]{State: set.state}}

	model, command := model.handleSupervisedLiveMessage(closed)
	if command != nil || model.loading || model.errBanner != "" || !model.listDenied() {
		t.Fatalf("denied closure state = command:%v loading:%v banner:%q denied:%v", command != nil, model.loading, model.errBanner, model.listDenied())
	}
	view := stripAnsiForTest(model.View())
	if !strings.Contains(view, "Not permitted: list pods") || strings.Contains(view, "ERROR") {
		t.Fatalf("browser view = %q, want calm notice without error banner", view)
	}
}

func TestBrowserDeniedFetchRendersNotice(t *testing.T) {
	model := newTestBrowserModel("payments")
	model.SetSize(120, 40)
	model.resourceType = resourceTypeRBAC
	model, _ = model.Update(cluster.RBACMsg{Err: deniedError(kube.OperationList, kube.SubjectRBAC)})
	if model.errBanner != "" || !model.listDenied() {
		t.Fatalf("denied fetch state = banner:%q denied:%v", model.errBanner, model.listDenied())
	}
	if view := stripAnsiForTest(model.View()); !strings.Contains(view, "Not permitted: list role-based access control resources") {
		t.Fatalf("browser view = %q, want RBAC notice", view)
	}
}

func TestBrowserDeniedMutationAndInspectionUseStatusNotice(t *testing.T) {
	model := newTestBrowserModel("payments")
	model.SetSize(120, 40)
	model.state = stateDeleteConfirm
	model, command := model.Update(cluster.MutationResultMsg{Err: deniedError(kube.OperationDelete, kube.SubjectPod)})
	if command != nil || model.state != stateBrowsing {
		t.Fatalf("denied mutation = command:%v state:%v", command != nil, model.state)
	}
	if status := stripAnsiForTest(model.statusMsg); status != "Not permitted: delete pod" {
		t.Fatalf("status = %q, want delete notice", status)
	}

	model, _ = model.Update(cluster.DescribeMsg{Err: deniedError(kube.OperationGet, kube.SubjectResource)})
	if model.errBanner != "" || model.showDetail {
		t.Fatalf("denied describe = banner:%q detail:%v", model.errBanner, model.showDetail)
	}
	if status := stripAnsiForTest(model.statusMsg); status != "Not permitted: get resource" {
		t.Fatalf("status = %q, want describe notice", status)
	}
}

func TestBrowserNonPermissionFailuresKeepErrorBanner(t *testing.T) {
	model := newTestBrowserModel("payments")
	model.SetSize(120, 40)
	model, _ = model.Update(cluster.DescribeMsg{Err: errStub("connection refused")})
	if model.errBanner == "" {
		t.Fatal("plain describe failure must keep the error banner")
	}
	model, _ = model.Update(cluster.RBACMsg{Err: errStub("timeout")})
	if model.errBanner == "" || model.listDenied() {
		t.Fatalf("plain fetch failure = banner:%q denied:%v", model.errBanner, model.listDenied())
	}
}

func TestBrowserDeniedOperationSetupUsesStatusNotice(t *testing.T) {
	model := newTestBrowserModel("payments")
	model.loading = true
	model.showOperationSetupError(deniedError(kube.OperationStart, kube.SubjectPortForward))
	if model.loading {
		t.Fatal("denied setup must clear loading")
	}
	if status := stripAnsiForTest(model.statusMsg); status != "Not permitted: start port forward" {
		t.Fatalf("status = %q, want port forward notice", status)
	}
}
