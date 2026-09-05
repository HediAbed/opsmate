package logs

import (
	"strings"
	"testing"

	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/failure"
	"github.com/HediAbed/opsmate/internal/kube"
)

func deniedLogsError(subject kube.Subject) error {
	return &kube.Error{Operation: kube.OperationList, Subject: subject, Code: failure.CodePermissionDenied}
}

func TestLogsDeniedStreamStopsPollingAndRendersNotice(t *testing.T) {
	model := newTestLogsModel("payments")
	model.SetSize(120, 30)
	model.selectedPod = "api"
	model.loading = true

	model, command := model.Update(cluster.LogsMsg{Err: deniedLogsError(kube.SubjectPodLogs)})
	if command != nil || model.loading {
		t.Fatalf("denied log stream = command:%v loading:%v, want no retry tick", command != nil, model.loading)
	}
	view := stripAnsiForTest(model.View())
	if !strings.Contains(view, "Not permitted: list pod logs") || strings.Contains(view, "press r to retry") {
		t.Fatalf("logs view = %q, want calm notice without retry prompt", view)
	}
}

func TestLogsDeniedContainerListUsesStatusNotice(t *testing.T) {
	model := newTestLogsModel("payments")
	model.SetSize(120, 30)
	model, command := model.Update(cluster.ContainersMsg{Err: deniedLogsError(kube.SubjectPod)})
	if command == nil {
		t.Fatal("denied container list must still schedule the status clear")
	}
	if status := stripAnsiForTest(model.statusMsg); status != "Not permitted: list pod" {
		t.Fatalf("status = %q, want container notice", status)
	}
}

func TestLogsDeniedPodListRendersNotice(t *testing.T) {
	model := newTestLogsModel("payments")
	model.SetSize(120, 30)
	model.loading = true
	model, command := model.Update(cluster.PodsMsg{Err: deniedLogsError(kube.SubjectPods)})
	if command != nil || model.loading {
		t.Fatalf("denied pod list = command:%v loading:%v", command != nil, model.loading)
	}
	if view := stripAnsiForTest(model.View()); !strings.Contains(view, "Not permitted: list pods") {
		t.Fatalf("logs view = %q, want pod list notice", view)
	}
}
