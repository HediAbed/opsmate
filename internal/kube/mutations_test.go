package kube

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

const mutationTestTimeout = time.Second

var (
	testPodKind     = schema.GroupVersionKind{Version: "v1", Kind: "Pod"}
	testPodResource = schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	testPodSingular = schema.GroupVersionResource{Version: "v1", Resource: "pod"}
)

type fakeWorkloadScaleClient struct {
	getScale    func(context.Context, string, metav1.GetOptions) (*autoscalingv1.Scale, error)
	updateScale func(context.Context, string, *autoscalingv1.Scale, metav1.UpdateOptions) (*autoscalingv1.Scale, error)
}

func (c fakeWorkloadScaleClient) GetScale(ctx context.Context, name string, options metav1.GetOptions) (*autoscalingv1.Scale, error) {
	return c.getScale(ctx, name, options)
}

func (c fakeWorkloadScaleClient) UpdateScale(ctx context.Context, name string, scale *autoscalingv1.Scale, options metav1.UpdateOptions) (*autoscalingv1.Scale, error) {
	return c.updateScale(ctx, name, scale, options)
}

type restartPatchDocument struct {
	Spec struct {
		Template struct {
			Metadata struct {
				Annotations map[string]string `json:"annotations"`
			} `json:"metadata"`
		} `json:"template"`
	} `json:"spec"`
}

func testPodMapper() meta.RESTMapper {
	return mapperForResource(testPodKind, testPodResource, testPodSingular, meta.RESTScopeNamespace)
}

func testDynamicPodClient(t *testing.T, names ...string) *dynamicfake.FakeDynamicClient {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}
	objects := make([]runtime.Object, 0, len(names))
	for _, name := range names {
		objects = append(objects, &corev1.Pod{
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "team-a"},
		})
	}
	return dynamicfake.NewSimpleDynamicClient(scheme, objects...)
}

func TestDeleteValidatesRequestAndDependencies(t *testing.T) {
	reference := ResourceReference{
		Resource:  testPodResource.GroupResource(),
		Namespace: "team-a",
		Name:      "web",
	}
	unavailable, err := NewManager(&fakeConfigSource{}, &fakeClientBuilder{})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	tests := []struct {
		name      string
		manager   *Manager
		ctx       context.Context
		reference ResourceReference
		wantErr   error
		operation Operation
	}{
		{name: "missing context", manager: unavailable, reference: reference, wantErr: ErrContextRequired, operation: OperationDelete},
		{name: "invalid resource", manager: unavailable, ctx: context.Background(), reference: ResourceReference{Name: "web"}, wantErr: ErrResourceIdentifierRequired, operation: OperationDelete},
		{name: "missing clients", manager: unavailable, ctx: context.Background(), reference: reference, wantErr: ErrClientUnavailable, operation: OperationDelete},
		{name: "missing mapper", manager: managerWithClientsForTest(t, &Clients{}), ctx: context.Background(), reference: reference, wantErr: ErrRESTMapperUnavailable, operation: OperationResolve},
		{name: "missing dynamic client", manager: managerWithClientsForTest(t, &Clients{mapper: testPodMapper()}), ctx: context.Background(), reference: reference, wantErr: ErrDynamicClientUnavailable, operation: OperationDelete},
		{name: "missing namespace", manager: managerWithClientsForTest(t, &Clients{mapper: testPodMapper(), dynamic: testDynamicPodClient(t)}), ctx: context.Background(), reference: ResourceReference{Resource: reference.Resource, Name: reference.Name}, wantErr: ErrNamespaceRequired, operation: OperationDelete},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.manager.Delete(test.ctx, test.reference)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("Delete() error = %v, want %v", err, test.wantErr)
			}
			var resourceErr *Error
			if !errors.As(err, &resourceErr) || resourceErr.Operation != test.operation || resourceErr.Identifier != test.reference.Identifier() {
				t.Fatalf("Delete() error = %#v, want operation %q and identifier %q", resourceErr, test.operation, test.reference.Identifier())
			}
		})
	}
}

func TestDeleteReportsAPIFailure(t *testing.T) {
	sentinel := errors.New("delete failed")
	dynamicClient := testDynamicPodClient(t, "web")
	dynamicClient.PrependReactor("delete", "pods", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, sentinel
	})
	manager := managerWithClientsForTest(t, &Clients{mapper: testPodMapper(), dynamic: dynamicClient})
	reference := ResourceReference{Resource: testPodResource.GroupResource(), Namespace: "team-a", Name: "web"}
	err := manager.Delete(context.Background(), reference)
	if !errors.Is(err, sentinel) {
		t.Fatalf("Delete() error = %v, want %v", err, sentinel)
	}
	var resourceErr *Error
	if !errors.As(err, &resourceErr) || resourceErr.Operation != OperationDelete || resourceErr.Identifier != reference.Identifier() {
		t.Fatalf("Delete() error = %#v", resourceErr)
	}
}

func TestDeleteUsesBackgroundPropagation(t *testing.T) {
	dynamicClient := testDynamicPodClient(t, "web")
	manager := managerWithClientsForTest(t, &Clients{mapper: testPodMapper(), dynamic: dynamicClient})
	reference := ResourceReference{Resource: testPodResource.GroupResource(), Namespace: "team-a", Name: "web"}
	if err := manager.Delete(context.Background(), reference); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	actions := dynamicClient.Actions()
	if len(actions) != 1 {
		t.Fatalf("delete actions = %d, want 1", len(actions))
	}
	deleteAction, ok := actions[0].(clienttesting.DeleteAction)
	if !ok {
		t.Fatalf("action = %T, want DeleteAction", actions[0])
	}
	options := deleteAction.GetDeleteOptions()
	if options.PropagationPolicy == nil || *options.PropagationPolicy != metav1.DeletePropagationBackground {
		t.Fatalf("propagation policy = %v, want background", options.PropagationPolicy)
	}
}

func TestDeleteBatchValidatesRequestAndDependencies(t *testing.T) {
	batch := ResourceBatch{Resource: testPodResource.GroupResource(), Namespace: "team-a", Names: []string{"web"}}
	unavailable, err := NewManager(&fakeConfigSource{}, &fakeClientBuilder{})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	tests := []struct {
		name      string
		manager   *Manager
		ctx       context.Context
		batch     ResourceBatch
		wantErr   error
		operation Operation
	}{
		{name: "missing context", manager: unavailable, batch: batch, wantErr: ErrContextRequired, operation: OperationDelete},
		{name: "invalid batch", manager: unavailable, ctx: context.Background(), batch: ResourceBatch{Resource: batch.Resource}, wantErr: ErrResourceNamesRequired, operation: OperationDelete},
		{name: "missing clients", manager: unavailable, ctx: context.Background(), batch: batch, wantErr: ErrClientUnavailable, operation: OperationDelete},
		{name: "missing mapper", manager: managerWithClientsForTest(t, &Clients{}), ctx: context.Background(), batch: batch, wantErr: ErrRESTMapperUnavailable, operation: OperationResolve},
		{name: "missing dynamic client", manager: managerWithClientsForTest(t, &Clients{mapper: testPodMapper()}), ctx: context.Background(), batch: batch, wantErr: ErrDynamicClientUnavailable, operation: OperationDelete},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			outcome, batchErr := test.manager.DeleteBatch(test.ctx, test.batch)
			if len(outcome.Succeeded) != 0 || len(outcome.Failed) != 0 || !errors.Is(batchErr, test.wantErr) {
				t.Fatalf("DeleteBatch() = (%+v, %v), want %v", outcome, batchErr, test.wantErr)
			}
			var resourceErr *Error
			if !errors.As(batchErr, &resourceErr) || resourceErr.Operation != test.operation {
				t.Fatalf("DeleteBatch() error = %#v, want operation %q", resourceErr, test.operation)
			}
		})
	}
}

func TestDeleteBatchReturnsDeterministicPartialOutcome(t *testing.T) {
	names := []string{"web", "worker", "api"}
	sentinel := errors.New("delete denied")
	dynamicClient := testDynamicPodClient(t, names...)
	dynamicClient.PrependReactor("delete", "pods", func(action clienttesting.Action) (bool, runtime.Object, error) {
		if action.(clienttesting.DeleteAction).GetName() == "worker" {
			return true, nil, sentinel
		}
		return false, nil, nil
	})
	manager := managerWithClientsForTest(t, &Clients{mapper: testPodMapper(), dynamic: dynamicClient})
	outcome, err := manager.DeleteBatch(context.Background(), ResourceBatch{
		Resource:  testPodResource.GroupResource(),
		Namespace: "team-a",
		Names:     names,
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("DeleteBatch() error = %v, want %v", err, sentinel)
	}
	if !slices.Equal(outcome.Succeeded, []string{"web", "api"}) || len(outcome.Failed) != 1 {
		t.Fatalf("DeleteBatch() outcome = %+v", outcome)
	}
	failure := outcome.Failed[0]
	if failure.Name != "worker" || !errors.Is(failure.Err, sentinel) {
		t.Fatalf("DeleteBatch() failure = %+v", failure)
	}
	var resourceErr *Error
	if !errors.As(failure.Err, &resourceErr) || resourceErr.Identifier != "team-a/pods/worker" {
		t.Fatalf("DeleteBatch() failure error = %#v", resourceErr)
	}
}

func TestDeleteBatchReturnsNilErrorWhenEveryDeleteSucceeds(t *testing.T) {
	dynamicClient := testDynamicPodClient(t, "web")
	manager := managerWithClientsForTest(t, &Clients{mapper: testPodMapper(), dynamic: dynamicClient})
	outcome, err := manager.DeleteBatch(context.Background(), ResourceBatch{
		Resource:  testPodResource.GroupResource(),
		Namespace: "team-a",
		Names:     []string{"web"},
	})
	if err != nil || !slices.Equal(outcome.Succeeded, []string{"web"}) || len(outcome.Failed) != 0 {
		t.Fatalf("DeleteBatch() = (%+v, %v)", outcome, err)
	}
}

func TestScaleValidatesRequestAndDependencies(t *testing.T) {
	request := ScaleRequest{
		Workload: WorkloadReference{Kind: WorkloadDeployment, Namespace: "team-a", Name: "web"},
		Replicas: 3,
	}
	unavailable, err := NewManager(&fakeConfigSource{}, &fakeClientBuilder{})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	tests := []struct {
		name    string
		manager *Manager
		ctx     context.Context
		request ScaleRequest
		wantErr error
	}{
		{name: "missing context", manager: unavailable, request: request, wantErr: ErrContextRequired},
		{name: "invalid workload", manager: unavailable, ctx: context.Background(), request: ScaleRequest{Workload: WorkloadReference{Namespace: "team-a", Name: "web"}}, wantErr: ErrUnsupportedWorkloadKind},
		{name: "negative replicas", manager: unavailable, ctx: context.Background(), request: ScaleRequest{Workload: request.Workload, Replicas: -1}, wantErr: ErrReplicaCountInvalid},
		{name: "missing clients", manager: unavailable, ctx: context.Background(), request: request, wantErr: ErrClientUnavailable},
		{name: "missing typed client", manager: managerWithClientsForTest(t, &Clients{}), ctx: context.Background(), request: request, wantErr: ErrTypedClientUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.manager.Scale(test.ctx, test.request)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("Scale() error = %v, want %v", err, test.wantErr)
			}
			var resourceErr *Error
			if !errors.As(err, &resourceErr) || resourceErr.Operation != OperationUpdate || resourceErr.Identifier != test.request.Workload.Identifier() {
				t.Fatalf("Scale() error = %#v", resourceErr)
			}
		})
	}
}

func TestScaleReportsAPIFailure(t *testing.T) {
	sentinel := errors.New("scale failed")
	client := kubernetesfake.NewSimpleClientset()
	client.PrependReactor("get", "deployments", func(action clienttesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() != "scale" {
			return false, nil, nil
		}
		return true, nil, sentinel
	})
	manager := managerWithClientsForTest(t, &Clients{kubernetes: client})
	request := ScaleRequest{
		Workload: WorkloadReference{Kind: WorkloadDeployment, Namespace: "team-a", Name: "web"},
		Replicas: 3,
	}
	err := manager.Scale(context.Background(), request)
	if !errors.Is(err, sentinel) {
		t.Fatalf("Scale() error = %v, want %v", err, sentinel)
	}
}

func TestScaleUpdatesSupportedWorkloads(t *testing.T) {
	tests := []struct {
		name     string
		kind     WorkloadKind
		resource string
		replicas int32
	}{
		{name: "deployment to zero", kind: WorkloadDeployment, resource: "deployments", replicas: 0},
		{name: "stateful set", kind: WorkloadStatefulSet, resource: "statefulsets", replicas: 3},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := kubernetesfake.NewSimpleClientset()
			client.PrependReactor("get", test.resource, func(action clienttesting.Action) (bool, runtime.Object, error) {
				if action.GetSubresource() != "scale" {
					return false, nil, nil
				}
				if action.(clienttesting.GetAction).GetName() != "web" {
					t.Fatalf("scale name = %q, want web", action.(clienttesting.GetAction).GetName())
				}
				return true, &autoscalingv1.Scale{ObjectMeta: metav1.ObjectMeta{Name: "web"}, Spec: autoscalingv1.ScaleSpec{Replicas: 1}}, nil
			})
			client.PrependReactor("update", test.resource, func(action clienttesting.Action) (bool, runtime.Object, error) {
				if action.GetSubresource() != "scale" {
					return false, nil, nil
				}
				scale := action.(clienttesting.UpdateAction).GetObject().(*autoscalingv1.Scale)
				if scale.Spec.Replicas != test.replicas {
					t.Fatalf("updated replicas = %d, want %d", scale.Spec.Replicas, test.replicas)
				}
				return true, scale, nil
			})
			manager := managerWithClientsForTest(t, &Clients{kubernetes: client})
			request := ScaleRequest{
				Workload: WorkloadReference{Kind: test.kind, Namespace: "team-a", Name: "web"},
				Replicas: test.replicas,
			}
			if err := manager.Scale(context.Background(), request); err != nil {
				t.Fatalf("Scale() error = %v", err)
			}
		})
	}
	invalidRequest := ScaleRequest{Workload: WorkloadReference{Kind: invalidWorkloadKind}}
	if err := scaleWorkload(context.Background(), kubernetesfake.NewSimpleClientset(), invalidRequest); !errors.Is(err, ErrUnsupportedWorkloadKind) {
		t.Fatalf("scaleWorkload(invalid) error = %v", err)
	}
}

func TestUpdateScaleWithRetryStopsOnGetFailure(t *testing.T) {
	sentinel := errors.New("get scale failed")
	updateCalled := false
	client := fakeWorkloadScaleClient{
		getScale: func(context.Context, string, metav1.GetOptions) (*autoscalingv1.Scale, error) {
			return nil, sentinel
		},
		updateScale: func(context.Context, string, *autoscalingv1.Scale, metav1.UpdateOptions) (*autoscalingv1.Scale, error) {
			updateCalled = true
			return nil, nil
		},
	}
	if err := updateScaleWithRetry(context.Background(), client, "web", 3); !errors.Is(err, sentinel) {
		t.Fatalf("updateScaleWithRetry() error = %v, want %v", err, sentinel)
	}
	if updateCalled {
		t.Fatal("UpdateScale() was called after GetScale() failed")
	}
}

func TestUpdateScaleWithRetryStopsOnNonConflict(t *testing.T) {
	sentinel := errors.New("update scale failed")
	client := fakeWorkloadScaleClient{
		getScale: func(context.Context, string, metav1.GetOptions) (*autoscalingv1.Scale, error) {
			return &autoscalingv1.Scale{}, nil
		},
		updateScale: func(context.Context, string, *autoscalingv1.Scale, metav1.UpdateOptions) (*autoscalingv1.Scale, error) {
			return nil, sentinel
		},
	}
	if err := updateScaleWithRetry(context.Background(), client, "web", 3); !errors.Is(err, sentinel) {
		t.Fatalf("updateScaleWithRetry() error = %v, want %v", err, sentinel)
	}
}

func TestUpdateScaleWithRetryRefetchesAfterConflict(t *testing.T) {
	conflictCause := errors.New("resource changed")
	conflict := apierrors.NewConflict(schema.GroupResource{Group: "apps", Resource: "deployments"}, "web", conflictCause)
	getCalls := 0
	updateCalls := 0
	client := fakeWorkloadScaleClient{
		getScale: func(context.Context, string, metav1.GetOptions) (*autoscalingv1.Scale, error) {
			getCalls++
			return &autoscalingv1.Scale{}, nil
		},
		updateScale: func(_ context.Context, _ string, scale *autoscalingv1.Scale, _ metav1.UpdateOptions) (*autoscalingv1.Scale, error) {
			updateCalls++
			if scale.Spec.Replicas != 3 {
				t.Fatalf("replicas = %d, want 3", scale.Spec.Replicas)
			}
			if updateCalls == 1 {
				return nil, conflict
			}
			return scale, nil
		},
	}
	if err := updateScaleWithRetry(context.Background(), client, "web", 3); err != nil {
		t.Fatalf("updateScaleWithRetry() error = %v", err)
	}
	if getCalls != 2 || updateCalls != 2 {
		t.Fatalf("retry calls = get:%d update:%d, want 2 each", getCalls, updateCalls)
	}
}

func TestRestartValidatesRequestAndDependencies(t *testing.T) {
	reference := WorkloadReference{Kind: WorkloadDeployment, Namespace: "team-a", Name: "web"}
	unavailable, err := NewManager(&fakeConfigSource{}, &fakeClientBuilder{})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	tests := []struct {
		name      string
		manager   *Manager
		ctx       context.Context
		reference WorkloadReference
		wantErr   error
	}{
		{name: "missing context", manager: unavailable, reference: reference, wantErr: ErrContextRequired},
		{name: "invalid workload", manager: unavailable, ctx: context.Background(), reference: WorkloadReference{Namespace: "team-a", Name: "web"}, wantErr: ErrUnsupportedWorkloadKind},
		{name: "missing clients", manager: unavailable, ctx: context.Background(), reference: reference, wantErr: ErrClientUnavailable},
		{name: "missing typed client", manager: managerWithClientsForTest(t, &Clients{}), ctx: context.Background(), reference: reference, wantErr: ErrTypedClientUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.manager.Restart(test.ctx, test.reference)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("Restart() error = %v, want %v", err, test.wantErr)
			}
			var resourceErr *Error
			if !errors.As(err, &resourceErr) || resourceErr.Operation != OperationUpdate || resourceErr.Identifier != test.reference.Identifier() {
				t.Fatalf("Restart() error = %#v", resourceErr)
			}
		})
	}
}

func TestRestartReportsAPIFailure(t *testing.T) {
	sentinel := errors.New("restart failed")
	client := kubernetesfake.NewSimpleClientset()
	client.PrependReactor("patch", "deployments", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, sentinel
	})
	manager := managerWithClientsForTest(t, &Clients{kubernetes: client})
	manager.clock = func() time.Time { return time.Unix(1, 0) }
	reference := WorkloadReference{Kind: WorkloadDeployment, Namespace: "team-a", Name: "web"}
	err := manager.Restart(context.Background(), reference)
	if !errors.Is(err, sentinel) {
		t.Fatalf("Restart() error = %v, want %v", err, sentinel)
	}
}

func TestRestartPatchesSupportedWorkloads(t *testing.T) {
	restartedAt := time.Date(2026, time.August, 29, 8, 30, 45, 123, time.FixedZone("test", 2*60*60))
	wantTimestamp := restartedAt.UTC().Format(time.RFC3339Nano)
	tests := []struct {
		name     string
		kind     WorkloadKind
		resource string
		result   runtime.Object
	}{
		{name: "deployment", kind: WorkloadDeployment, resource: "deployments", result: &appsv1.Deployment{}},
		{name: "stateful set", kind: WorkloadStatefulSet, resource: "statefulsets", result: &appsv1.StatefulSet{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := kubernetesfake.NewSimpleClientset()
			client.PrependReactor("patch", test.resource, func(action clienttesting.Action) (bool, runtime.Object, error) {
				patchAction := action.(clienttesting.PatchAction)
				if action.GetNamespace() != "team-a" || patchAction.GetName() != "web" {
					t.Fatalf("patch target = %s/%s", action.GetNamespace(), patchAction.GetName())
				}
				if patchAction.GetPatchType() != types.StrategicMergePatchType {
					t.Fatalf("patch type = %q, want %q", patchAction.GetPatchType(), types.StrategicMergePatchType)
				}
				if got := restartTimestampFromPatch(t, patchAction.GetPatch()); got != wantTimestamp {
					t.Fatalf("restart timestamp = %q, want %q", got, wantTimestamp)
				}
				return true, test.result, nil
			})
			manager := managerWithClientsForTest(t, &Clients{kubernetes: client})
			manager.clock = func() time.Time { return restartedAt }
			reference := WorkloadReference{Kind: test.kind, Namespace: "team-a", Name: "web"}
			if err := manager.Restart(context.Background(), reference); err != nil {
				t.Fatalf("Restart() error = %v", err)
			}
		})
	}
	invalid := WorkloadReference{Kind: invalidWorkloadKind}
	if err := restartWorkload(context.Background(), kubernetesfake.NewSimpleClientset(), invalid, restartedAt); !errors.Is(err, ErrUnsupportedWorkloadKind) {
		t.Fatalf("restartWorkload(invalid) error = %v", err)
	}
}

func TestRestartBatchValidatesRequestAndDependencies(t *testing.T) {
	batch := WorkloadBatch{Kind: WorkloadDeployment, Namespace: "team-a", Names: []string{"web"}}
	unavailable, err := NewManager(&fakeConfigSource{}, &fakeClientBuilder{})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	tests := []struct {
		name    string
		manager *Manager
		ctx     context.Context
		batch   WorkloadBatch
		wantErr error
	}{
		{name: "missing context", manager: unavailable, batch: batch, wantErr: ErrContextRequired},
		{name: "invalid batch", manager: unavailable, ctx: context.Background(), batch: WorkloadBatch{Kind: WorkloadDeployment, Namespace: "team-a"}, wantErr: ErrResourceNamesRequired},
		{name: "missing clients", manager: unavailable, ctx: context.Background(), batch: batch, wantErr: ErrClientUnavailable},
		{name: "missing typed client", manager: managerWithClientsForTest(t, &Clients{}), ctx: context.Background(), batch: batch, wantErr: ErrTypedClientUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			outcome, batchErr := test.manager.RestartBatch(test.ctx, test.batch)
			if len(outcome.Succeeded) != 0 || len(outcome.Failed) != 0 || !errors.Is(batchErr, test.wantErr) {
				t.Fatalf("RestartBatch() = (%+v, %v), want %v", outcome, batchErr, test.wantErr)
			}
		})
	}
}

func TestRestartBatchUsesOneTimestampAndReturnsPartialOutcome(t *testing.T) {
	names := []string{"web", "worker", "api"}
	restartedAt := time.Date(2026, time.August, 29, 8, 30, 45, 0, time.UTC)
	wantTimestamp := restartedAt.Format(time.RFC3339Nano)
	sentinel := errors.New("restart denied")
	client := kubernetesfake.NewSimpleClientset()
	var clockCalls atomic.Int32
	var patchesMu sync.Mutex
	patches := make(map[string]string, len(names))
	client.PrependReactor("patch", "deployments", func(action clienttesting.Action) (bool, runtime.Object, error) {
		patchAction := action.(clienttesting.PatchAction)
		timestamp := restartTimestampFromPatch(t, patchAction.GetPatch())
		patchesMu.Lock()
		patches[patchAction.GetName()] = timestamp
		patchesMu.Unlock()
		if patchAction.GetName() == "worker" {
			return true, nil, sentinel
		}
		return true, &appsv1.Deployment{}, nil
	})
	manager := managerWithClientsForTest(t, &Clients{kubernetes: client})
	manager.clock = func() time.Time {
		clockCalls.Add(1)
		return restartedAt
	}
	outcome, err := manager.RestartBatch(context.Background(), WorkloadBatch{
		Kind:      WorkloadDeployment,
		Namespace: "team-a",
		Names:     names,
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("RestartBatch() error = %v, want %v", err, sentinel)
	}
	if !slices.Equal(outcome.Succeeded, []string{"web", "api"}) || len(outcome.Failed) != 1 || outcome.Failed[0].Name != "worker" {
		t.Fatalf("RestartBatch() outcome = %+v", outcome)
	}
	if clockCalls.Load() != 1 {
		t.Fatalf("clock calls = %d, want 1", clockCalls.Load())
	}
	patchesMu.Lock()
	defer patchesMu.Unlock()
	if len(patches) != len(names) {
		t.Fatalf("patch count = %d, want %d", len(patches), len(names))
	}
	for _, name := range names {
		if patches[name] != wantTimestamp {
			t.Fatalf("patch timestamp for %s = %q, want %q", name, patches[name], wantTimestamp)
		}
	}
}

func TestNewRestartPatchProducesValidJSON(t *testing.T) {
	restartedAt := time.Date(2026, time.August, 29, 8, 30, 45, 123, time.UTC)
	patch := newRestartPatch(restartedAt)
	if !json.Valid(patch) {
		t.Fatalf("restart patch is invalid JSON: %s", patch)
	}
	if got := restartTimestampFromPatch(t, patch); got != restartedAt.Format(time.RFC3339Nano) {
		t.Fatalf("restart timestamp = %q", got)
	}
}

func TestRunBoundedLimitsConcurrencyAndPreservesInputOrder(t *testing.T) {
	names := []string{"one", "two", "three", "four", "five", "six", "seven"}
	sentinel := errors.New("mutation failed")
	started := make(chan struct{}, len(names))
	release := make(chan struct{})
	finished := make(chan BatchOutcome, 1)
	var active atomic.Int32
	var peak atomic.Int32
	go func() {
		finished <- runBounded(names, func(name string) error {
			current := active.Add(1)
			updatePeak(&peak, current)
			started <- struct{}{}
			<-release
			active.Add(-1)
			if name == "two" || name == "six" {
				return sentinel
			}
			return nil
		})
	}()
	awaitBoundedMutationStarts(t, started)
	close(release)
	outcome := awaitBatchOutcome(t, finished)
	if peak.Load() != maximumConcurrentMutations {
		t.Fatalf("peak concurrency = %d, want %d", peak.Load(), maximumConcurrentMutations)
	}
	if !slices.Equal(outcome.Succeeded, []string{"one", "three", "four", "five", "seven"}) {
		t.Fatalf("succeeded = %v", outcome.Succeeded)
	}
	if len(outcome.Failed) != 2 || outcome.Failed[0].Name != "two" || outcome.Failed[1].Name != "six" {
		t.Fatalf("failed = %+v", outcome.Failed)
	}
}

func awaitBoundedMutationStarts(t *testing.T, started <-chan struct{}) {
	t.Helper()
	for range maximumConcurrentMutations {
		select {
		case <-started:
		case <-time.After(mutationTestTimeout):
			t.Fatal("timed out waiting for bounded mutations to start")
		}
	}
	select {
	case <-started:
		t.Fatal("more than the configured mutation limit started")
	default:
	}
}

func awaitBatchOutcome(t *testing.T, finished <-chan BatchOutcome) BatchOutcome {
	t.Helper()
	select {
	case outcome := <-finished:
		return outcome
	case <-time.After(mutationTestTimeout):
		t.Fatal("timed out waiting for bounded mutations to finish")
		return BatchOutcome{}
	}
}

func TestRunBoundedAcceptsEmptyInput(t *testing.T) {
	outcome := runBounded(nil, func(string) error {
		t.Fatal("mutate called for empty input")
		return nil
	})
	if outcome.Succeeded == nil || outcome.Failed == nil || len(outcome.Succeeded) != 0 || len(outcome.Failed) != 0 {
		t.Fatalf("runBounded(nil) = %+v, want non-nil empty results", outcome)
	}
}

func restartTimestampFromPatch(t *testing.T, patch []byte) string {
	t.Helper()
	var document restartPatchDocument
	if err := json.Unmarshal(patch, &document); err != nil {
		t.Fatalf("Unmarshal(restart patch) error = %v", err)
	}
	if len(document.Spec.Template.Metadata.Annotations) != 1 {
		t.Fatalf("restart annotations = %v, want one entry", document.Spec.Template.Metadata.Annotations)
	}
	return document.Spec.Template.Metadata.Annotations[restartAnnotation]
}

func updatePeak(peak *atomic.Int32, current int32) {
	for {
		previous := peak.Load()
		if current <= previous || peak.CompareAndSwap(previous, current) {
			return
		}
	}
}
