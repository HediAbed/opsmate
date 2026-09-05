package helm

import (
	"strings"
	"testing"

	"github.com/HediAbed/opsmate/internal/failure"
	"github.com/HediAbed/opsmate/internal/kube"
)

func TestHelmDeniedReleasesRenderNoticeInsteadOfError(t *testing.T) {
	model := newTestHelmModel("payments")
	model.SetSize(120, 40)
	model.loading = true
	denied := &kube.Error{Operation: kube.OperationList, Subject: kube.SubjectHelmReleases, Code: failure.CodePermissionDenied}

	model, _ = model.Update(helmReleasesMsg{Err: denied})
	if model.loading || len(model.releases) != 0 {
		t.Fatalf("denied releases = loading:%v releases:%d", model.loading, len(model.releases))
	}
	view := stripAnsiForTest(model.View())
	if !strings.Contains(view, "Not permitted: list helm releases") {
		t.Fatalf("helm view = %q, want calm notice", view)
	}
	if strings.Contains(view, denied.Error()) {
		t.Fatalf("helm view = %q, must not render the raw error banner", view)
	}
}

func TestHelmDeniedValuesRenderNoticeInsidePopup(t *testing.T) {
	model, _ := helmModelWithSelectedRelease().openValuesPopup()
	denied := &kube.Error{Operation: kube.OperationGet, Subject: kube.SubjectHelmValues, Identifier: "edge/gateway", Code: failure.CodePermissionDenied}
	updated := model.applyHelmValues(helmValuesMsg{Release: "gateway", Namespace: "edge", Err: denied})
	if updated.valuesPopupErr == nil || updated.valuesPopupLoading {
		t.Fatalf("denied values = err:%v loading:%v", updated.valuesPopupErr, updated.valuesPopupLoading)
	}
	rendered := stripAnsiForTest(updated.valuesPopupView.View())
	if !strings.Contains(rendered, "Not permitted: get helm release values edge/gateway") {
		t.Fatalf("popup = %q, want calm notice", rendered)
	}
}
