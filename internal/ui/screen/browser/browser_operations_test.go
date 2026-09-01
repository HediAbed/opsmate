package browser

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/kube"
	clusterui "github.com/HediAbed/opsmate/internal/ui/cluster"
)

type recordingClusterOperations struct {
	clusterui.Operations
	inspected    kube.ResourceReference
	yaml         kube.ResourceReference
	logs         kube.PodLogRequest
	containers   kube.PodReference
	scale        kube.ScaleRequest
	deleted      kube.ResourceReference
	deleteBatch  kube.ResourceBatch
	restarted    kube.WorkloadReference
	restartBatch kube.WorkloadBatch
}

func (o *recordingClusterOperations) InspectResource(reference kube.ResourceReference) tea.Cmd {
	o.inspected = reference
	return func() tea.Msg { return cluster.DescribeMsg{} }
}

func (o *recordingClusterOperations) ResourceYAML(reference kube.ResourceReference) tea.Cmd {
	o.yaml = reference
	return func() tea.Msg { return cluster.YAMLMsg{} }
}

func (o *recordingClusterOperations) FetchPodLogs(request kube.PodLogRequest) tea.Cmd {
	o.logs = request
	return func() tea.Msg { return cluster.LogsMsg{} }
}

func (o *recordingClusterOperations) FetchPodContainers(reference kube.PodReference) tea.Cmd {
	o.containers = reference
	return func() tea.Msg { return cluster.ContainersMsg{} }
}

func (o *recordingClusterOperations) ScaleWorkload(request kube.ScaleRequest) tea.Cmd {
	o.scale = request
	return func() tea.Msg { return cluster.MutationResultMsg{} }
}

func (o *recordingClusterOperations) DeleteResource(reference kube.ResourceReference) tea.Cmd {
	o.deleted = reference
	return func() tea.Msg { return cluster.MutationResultMsg{} }
}

func (o *recordingClusterOperations) DeleteResources(batch kube.ResourceBatch) tea.Cmd {
	o.deleteBatch = batch
	return func() tea.Msg { return cluster.MutationResultMsg{} }
}

func (o *recordingClusterOperations) RestartWorkload(reference kube.WorkloadReference) tea.Cmd {
	o.restarted = reference
	return func() tea.Msg { return cluster.MutationResultMsg{} }
}

func (o *recordingClusterOperations) RestartWorkloads(batch kube.WorkloadBatch) tea.Cmd {
	o.restartBatch = batch
	return func() tea.Msg { return cluster.MutationResultMsg{} }
}

func browserWithRecordingOperations(namespace string) (BrowserModel, *recordingClusterOperations) {
	operations := &recordingClusterOperations{}
	return NewBrowserModel(namespace, newTestClusterCommands(), operations), operations
}

func TestBrowserRoutesInspectionAndYAMLThroughTypedResources(t *testing.T) {
	model, operations := browserWithRecordingOperations("team-a")
	identity := resourceIdentity{Kind: resourceKindDeployment, Namespace: "team-a", Name: "web"}
	model.visibleResources = []resourceIdentity{identity}

	if command := model.describeSelectedResource(); command == nil {
		t.Fatal("describeSelectedResource() command = nil")
	}
	want := kube.ResourceReference{
		Resource:  schema.GroupResource{Group: apiGroupApps, Resource: resourceTypeDeployments},
		Namespace: identity.Namespace,
		Name:      identity.Name,
	}
	if operations.inspected != want || !strings.Contains(stripAnsiForTest(model.statusMsg), "Inspecting deployment/web") {
		t.Fatalf("inspect reference = %+v, status = %q", operations.inspected, stripAnsiForTest(model.statusMsg))
	}
	if command := model.fetchSelectedResourceYAML(); command == nil {
		t.Fatal("fetchSelectedResourceYAML() command = nil")
	}
	if operations.yaml != want {
		t.Fatalf("YAML reference = %+v, want %+v", operations.yaml, want)
	}
}

func TestBrowserRejectsUnsupportedInspectionAndYAMLKind(t *testing.T) {
	tests := []struct {
		name string
		read func(*BrowserModel) tea.Cmd
	}{
		{name: "inspect", read: (*BrowserModel).describeSelectedResource},
		{name: "yaml", read: (*BrowserModel).fetchSelectedResourceYAML},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model, _ := browserWithRecordingOperations("team-a")
			model.visibleResources = []resourceIdentity{{Kind: "widget", Namespace: "team-a", Name: "sample"}}
			model.loading = true
			if command := test.read(&model); command != nil {
				t.Fatal("unsupported resource returned a command")
			}
			if model.loading || !strings.Contains(stripAnsiForTest(model.statusMsg), ErrUnsupportedResourceKind.Error()) {
				t.Fatalf("operation error state = loading:%t status:%q", model.loading, stripAnsiForTest(model.statusMsg))
			}
		})
	}
}

func TestBrowserRoutesScaleRequest(t *testing.T) {
	model, operations := browserWithRecordingOperations("team-a")
	model.scaleIdentity = resourceIdentity{Kind: resourceKindStatefulSet, Namespace: "team-a", Name: "database"}
	model.scaleReplicas = 4
	updated, command := model.handleScaleConfirmationKey("y")
	if command == nil {
		t.Fatal("scale confirmation command = nil")
	}
	want := kube.ScaleRequest{
		Workload: kube.WorkloadReference{Kind: kube.WorkloadStatefulSet, Namespace: "team-a", Name: "database"},
		Replicas: 4,
	}
	if operations.scale != want || !updated.loading {
		t.Fatalf("scale request = %+v, want %+v", operations.scale, want)
	}

	model, _ = browserWithRecordingOperations("team-a")
	model.scaleIdentity = resourceIdentity{Kind: resourceKindPod, Namespace: "team-a", Name: "web"}
	model.loading = true
	updated, command = model.handleScaleConfirmationKey("y")
	if command != nil || updated.loading || !strings.Contains(stripAnsiForTest(updated.statusMsg), ErrUnsupportedResourceKind.Error()) {
		t.Fatalf("unsupported scale = command:%v loading:%t status:%q", command, updated.loading, stripAnsiForTest(updated.statusMsg))
	}
}

func TestBrowserRoutesSingleMutations(t *testing.T) {
	tests := []struct {
		name   string
		action string
		kind   string
		assert func(*testing.T, *recordingClusterOperations)
	}{
		{
			name:   "delete",
			action: "delete",
			kind:   resourceKindPod,
			assert: func(t *testing.T, operations *recordingClusterOperations) {
				t.Helper()
				want := kube.ResourceReference{Resource: schema.GroupResource{Resource: resourceTypePods}, Namespace: "team-a", Name: "web"}
				if operations.deleted != want {
					t.Fatalf("delete reference = %+v, want %+v", operations.deleted, want)
				}
			},
		},
		{
			name:   "restart",
			action: "restart",
			kind:   resourceKindDeployment,
			assert: func(t *testing.T, operations *recordingClusterOperations) {
				t.Helper()
				want := kube.WorkloadReference{Kind: kube.WorkloadDeployment, Namespace: "team-a", Name: "web"}
				if operations.restarted != want {
					t.Fatalf("restart reference = %+v, want %+v", operations.restarted, want)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model, operations := browserWithRecordingOperations("team-a")
			model.confirmAction = test.action
			model.confirmTarget = test.kind + "/web"
			model.confirmIdentity = resourceIdentity{Kind: test.kind, Namespace: "team-a", Name: "web"}
			if command := model.executeConfirmedResourceAction(); command == nil {
				t.Fatal("mutation command = nil")
			}
			test.assert(t, operations)
		})
	}
}

func TestBrowserRoutesSortedBatchMutations(t *testing.T) {
	tests := []string{"delete", "restart"}
	for _, action := range tests {
		t.Run(action, func(t *testing.T) {
			model, operations := browserWithRecordingOperations("team-a")
			model.resourceType = resourceTypeDeployments
			model.confirmAction = action
			model.confirmTarget = "2 deployments"
			model.toggleResourceSelection(resourceIdentity{Kind: resourceKindDeployment, Namespace: "team-a", Name: "worker"})
			model.toggleResourceSelection(resourceIdentity{Kind: resourceKindDeployment, Namespace: "team-a", Name: "api"})
			if command := model.executeConfirmedResourceAction(); command == nil {
				t.Fatal("batch mutation command = nil")
			}
			wantNames := []string{"api", "worker"}
			if action == "delete" {
				if operations.deleteBatch.Resource != (schema.GroupResource{Group: apiGroupApps, Resource: resourceTypeDeployments}) ||
					operations.deleteBatch.Namespace != "team-a" || !slices.Equal(operations.deleteBatch.Names, wantNames) {
					t.Fatalf("delete batch = %+v", operations.deleteBatch)
				}
				return
			}
			if operations.restartBatch.Kind != kube.WorkloadDeployment || operations.restartBatch.Namespace != "team-a" || !slices.Equal(operations.restartBatch.Names, wantNames) {
				t.Fatalf("restart batch = %+v", operations.restartBatch)
			}
		})
	}
}

func TestBrowserRejectsMixedSelection(t *testing.T) {
	model, _ := browserWithRecordingOperations("team-a")
	model.resourceType = "rbac"
	model.confirmAction = "delete"
	model.toggleResourceSelection(resourceIdentity{Kind: resourceKindRole, Namespace: "team-a", Name: "reader"})
	model.toggleResourceSelection(resourceIdentity{Kind: resourceKindClusterRole, Name: "reader"})
	if command := model.executeConfirmedResourceAction(); command != nil || model.loading || !strings.Contains(stripAnsiForTest(model.statusMsg), ErrMixedResourceSelection.Error()) {
		t.Fatalf("mixed batch = command:%v loading:%t status:%q", command, model.loading, stripAnsiForTest(model.statusMsg))
	}
}

func TestBrowserRejectsUnsupportedMutationTargets(t *testing.T) {
	for _, action := range []string{"delete", "restart"} {
		t.Run("batch "+action, func(t *testing.T) {
			model, _ := browserWithRecordingOperations("team-a")
			model.confirmAction = action
			identity := resourceIdentity{Kind: "widget", Namespace: "team-a"}
			if command := model.executeConfirmedBatchAction(identity, []string{"sample"}); command != nil || model.loading || !strings.Contains(stripAnsiForTest(model.statusMsg), ErrUnsupportedResourceKind.Error()) {
				t.Fatalf("unsupported batch = command:%v loading:%t status:%q", command, model.loading, stripAnsiForTest(model.statusMsg))
			}
		})
		t.Run("single "+action, func(t *testing.T) {
			model, _ := browserWithRecordingOperations("team-a")
			model.confirmAction = action
			model.confirmIdentity = resourceIdentity{Kind: "widget", Namespace: "team-a", Name: "sample"}
			model.loading = true
			if command := model.executeConfirmedSingleAction(); command != nil || model.loading || !strings.Contains(stripAnsiForTest(model.statusMsg), ErrUnsupportedResourceKind.Error()) {
				t.Fatalf("unsupported single = command:%v loading:%t status:%q", command, model.loading, stripAnsiForTest(model.statusMsg))
			}
		})
	}
}

func TestSelectedBatchIdentityUsesFallbackAndHomogeneousIdentity(t *testing.T) {
	model := newTestBrowserModel("team-a")
	model.selected = map[string]bool{"fallback": true}
	identity, err := model.selectedBatchIdentity(resourceKindPod)
	if err != nil || identity != (resourceIdentity{Kind: resourceKindPod, Namespace: "team-a"}) {
		t.Fatalf("fallback identity = (%+v, %v)", identity, err)
	}
	model.selectedIdentities = map[string]resourceIdentity{
		"first":  {Kind: resourceKindRole, Namespace: "team-a", Name: "first"},
		"second": {Kind: resourceKindRole, Namespace: "team-a", Name: "second"},
	}
	model.selected = map[string]bool{"first": true, "second": true}
	identity, err = model.selectedBatchIdentity("rbac")
	if err != nil || identity != (resourceIdentity{Kind: resourceKindRole, Namespace: "team-a"}) {
		t.Fatalf("homogeneous identity = (%+v, %v)", identity, err)
	}
}
