package crds

import (
	"strings"
	"testing"

	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/failure"
	"github.com/HediAbed/opsmate/internal/kube"
)

func TestCRDsDeniedListRendersNoticeInsteadOfError(t *testing.T) {
	model := newTestCRDsModel("payments")
	model.SetSize(160, 40)
	model.loading = true
	denied := &kube.Error{Operation: kube.OperationList, Subject: kube.SubjectCRDs, Code: failure.CodePermissionDenied}

	model, _ = model.Update(cluster.CRDsMsg{Err: denied})
	if model.loading || len(model.crds) != 0 {
		t.Fatalf("denied CRD list = loading:%v crds:%d", model.loading, len(model.crds))
	}
	view := stripAnsiForTest(model.View())
	if !strings.Contains(view, "Not permitted: list custom resource definitions") {
		t.Fatalf("crds view = %q, want calm notice", view)
	}
	if strings.Contains(view, denied.Error()) {
		t.Fatalf("crds view = %q, must not render the raw error banner", view)
	}
}

func TestCRDsDeniedInstancesRenderNotice(t *testing.T) {
	model := newTestCRDsModel("payments")
	model.SetSize(160, 40)
	model.view = crdsViewInstances
	model.selectedCRD = cluster.CRD{Resource: "certificates.cert-manager.io", Kind: "Certificate"}
	denied := &kube.Error{Operation: kube.OperationList, Subject: kube.SubjectResource, Identifier: "certificates.cert-manager.io", Code: failure.CodePermissionDenied}

	model, _ = model.Update(cluster.CRDInstancesMsg{Resource: "certificates.cert-manager.io", Namespace: "payments", Err: denied})
	if view := stripAnsiForTest(model.View()); !strings.Contains(view, "Not permitted: list resource certificates.cert-manager.io") {
		t.Fatalf("crds view = %q, want instance notice", view)
	}
}
