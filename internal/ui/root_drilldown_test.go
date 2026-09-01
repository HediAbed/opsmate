package ui

import (
	"context"
	"errors"
	"slices"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/kube"
	browserscreen "github.com/HediAbed/opsmate/internal/ui/screen/browser"
)

type recordingInspectOperations struct {
	testClusterOperations
	yamlReferences []kube.ResourceReference
}

func (o *recordingInspectOperations) ResourceYAML(_ context.Context, reference kube.ResourceReference) (string, error) {
	o.yamlReferences = append(o.yamlReferences, reference)
	return "recorded", nil
}

func newTestRootModelWithInspector(t *testing.T, namespace string, operations *recordingInspectOperations) RootModel {
	t.Helper()
	root, err := NewRootModel(namespace, RuntimeDependencies{
		Context:           context.Background(),
		ClusterContext:    &testContextManager{},
		ClusterResources:  &testResourceReader{},
		ClusterSnapshots:  &snapshotCollectorStub{},
		ClusterObserver:   &testResourceObserver{},
		ClusterOperations: operations,
		HelmReleases:      operations,
	})
	if err != nil {
		t.Fatalf("NewRootModel() error = %v", err)
	}
	return root
}

func TestRootHandleDrillDown_RecordsExactDeploymentReference(t *testing.T) {
	operations := &recordingInspectOperations{}
	model := newTestRootModelWithInspector(t, "default", operations)
	_, command := model.handleDrillDown(DrillDownMsg{
		Screen: ScreenBrowser, ResourceType: "deployment", ResourceName: "web", ResourceNS: "team-a",
	})
	if command == nil {
		t.Fatal("deployment drill-down returned no command")
	}
	_ = commandMessages(t, command)
	want := kube.ResourceReference{
		Resource:  schema.GroupResource{Group: "apps", Resource: "deployments"},
		Namespace: "team-a",
		Name:      "web",
	}
	if !slices.Equal(operations.yamlReferences, []kube.ResourceReference{want}) {
		t.Fatalf("inspected references = %+v, want %+v", operations.yamlReferences, want)
	}
}

func TestRootHandleDrillDown_UnsupportedKindReturnsTypedError(t *testing.T) {
	operations := &recordingInspectOperations{}
	model := newTestRootModelWithInspector(t, "default", operations)
	_, command := model.handleDrillDown(DrillDownMsg{
		Screen: ScreenBrowser, ResourceType: "widget", ResourceName: "thing", ResourceNS: "team-a",
	})
	if command == nil {
		t.Fatal("unsupported drill-down returned no command")
	}
	var describeErr error
	for _, message := range commandMessages(t, command) {
		if describe, ok := message.(cluster.DescribeMsg); ok {
			describeErr = describe.Err
		}
	}
	if !errors.Is(describeErr, browserscreen.ErrUnsupportedResourceKind) {
		t.Fatalf("drill-down error = %v, want ErrUnsupportedResourceKind", describeErr)
	}
	var kindErr *browserscreen.ResourceKindError
	if !errors.As(describeErr, &kindErr) || kindErr.Kind != "widget" {
		t.Fatalf("drill-down error = %#v, want resource kind error for widget", describeErr)
	}
	if len(operations.yamlReferences) != 0 {
		t.Fatalf("unsupported kind inspected %+v", operations.yamlReferences)
	}
}
