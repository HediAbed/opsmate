package cluster

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"

	model "github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/kube"
)

type testResourceReader struct {
	err               error
	calls             []string
	metadataResource  schema.GroupVersionResource
	metadataNamespace string
}

type testResourceObserver struct {
	err   error
	calls []string
}

type testKubeLiveSet[T interface{}] struct {
	changes chan struct{}
	state   kube.LiveState[T]
	stop    sync.Once
}

func newTestKubeLiveSet[T interface{}](items []T, err error) kube.LiveSet[T] {
	changes := make(chan struct{}, 1)
	changes <- struct{}{}
	return &testKubeLiveSet[T]{
		changes: changes,
		state:   kube.LiveState[T]{Items: items, Ready: err == nil, Err: err},
	}
}

func (s *testKubeLiveSet[T]) Changes() <-chan struct{} {
	return s.changes
}

func (s *testKubeLiveSet[T]) State() kube.LiveState[T] {
	return s.state
}

func (s *testKubeLiveSet[T]) Stop() {
	s.stop.Do(func() { close(s.changes) })
}

func (o *testResourceObserver) record(name string) error {
	o.calls = append(o.calls, name)
	return o.err
}

func (o *testResourceObserver) ObservePods(context.Context, string) (kube.LiveSet[corev1.Pod], error) {
	err := o.record("pods")
	return newTestKubeLiveSet([]corev1.Pod{{}}, err), err
}

func (o *testResourceObserver) ObserveDeployments(context.Context, string) (kube.LiveSet[appsv1.Deployment], error) {
	err := o.record("deployments")
	return newTestKubeLiveSet([]appsv1.Deployment{{}}, err), err
}

func (o *testResourceObserver) ObserveEvents(context.Context, string) (kube.LiveSet[corev1.Event], error) {
	err := o.record("events")
	return newTestKubeLiveSet([]corev1.Event{{}}, err), err
}

func (o *testResourceObserver) ObserveIngresses(context.Context, string) (kube.LiveSet[networkingv1.Ingress], error) {
	err := o.record("ingresses")
	return newTestKubeLiveSet([]networkingv1.Ingress{{}}, err), err
}

func (o *testResourceObserver) ObserveNetworkPolicies(context.Context, string) (kube.LiveSet[networkingv1.NetworkPolicy], error) {
	err := o.record("network policies")
	return newTestKubeLiveSet([]networkingv1.NetworkPolicy{{}}, err), err
}

func (o *testResourceObserver) ObservePersistentVolumeClaims(context.Context, string) (kube.LiveSet[corev1.PersistentVolumeClaim], error) {
	err := o.record("persistent volume claims")
	return newTestKubeLiveSet([]corev1.PersistentVolumeClaim{{}}, err), err
}

func (o *testResourceObserver) ObserveCronJobs(context.Context, string) (kube.LiveSet[batchv1.CronJob], error) {
	err := o.record("cron jobs")
	return newTestKubeLiveSet([]batchv1.CronJob{{}}, err), err
}

func (o *testResourceObserver) ObserveHorizontalPodAutoscalers(context.Context, string) (kube.LiveSet[autoscalingv2.HorizontalPodAutoscaler], error) {
	err := o.record("horizontal pod autoscalers")
	return newTestKubeLiveSet([]autoscalingv2.HorizontalPodAutoscaler{{}}, err), err
}

func (o *testResourceObserver) ObserveSecrets(context.Context, string) (kube.LiveSet[kube.ResourceMetadata], error) {
	err := o.record("secrets")
	return newTestKubeLiveSet([]kube.ResourceMetadata{{}}, err), err
}

func (o *testResourceObserver) ObserveReplicaSets(context.Context, string) (kube.LiveSet[appsv1.ReplicaSet], error) {
	err := o.record("replica sets")
	return newTestKubeLiveSet([]appsv1.ReplicaSet{{}}, err), err
}

func (r *testResourceReader) record(name string) error {
	r.calls = append(r.calls, name)
	return r.err
}

func (r *testResourceReader) ListPods(context.Context, string) ([]corev1.Pod, error) {
	return []corev1.Pod{{}}, r.record("pods")
}

func (r *testResourceReader) ListDeployments(context.Context, string) ([]appsv1.Deployment, error) {
	return []appsv1.Deployment{{}}, r.record("deployments")
}

func (r *testResourceReader) ListEvents(context.Context, string) ([]corev1.Event, error) {
	return []corev1.Event{{}}, r.record("events")
}

func (r *testResourceReader) ListServices(context.Context, string) ([]corev1.Service, error) {
	return []corev1.Service{{}}, r.record("services")
}

func (r *testResourceReader) ListStatefulSets(context.Context, string) ([]appsv1.StatefulSet, error) {
	return []appsv1.StatefulSet{{}}, r.record("stateful sets")
}

func (r *testResourceReader) ListDaemonSets(context.Context, string) ([]appsv1.DaemonSet, error) {
	return []appsv1.DaemonSet{{}}, r.record("daemon sets")
}

func (r *testResourceReader) ListConfigMaps(context.Context, string) ([]corev1.ConfigMap, error) {
	return []corev1.ConfigMap{{}}, r.record("config maps")
}

func (r *testResourceReader) ListNodes(context.Context) ([]corev1.Node, error) {
	return []corev1.Node{{}}, r.record("nodes")
}

func (r *testResourceReader) ListJobs(context.Context, string) ([]batchv1.Job, error) {
	return []batchv1.Job{{}}, r.record("jobs")
}

func (r *testResourceReader) ListIngresses(context.Context, string) ([]networkingv1.Ingress, error) {
	return []networkingv1.Ingress{{}}, r.record("ingresses")
}

func (r *testResourceReader) ListNetworkPolicies(context.Context, string) ([]networkingv1.NetworkPolicy, error) {
	return []networkingv1.NetworkPolicy{{}}, r.record("network policies")
}

func (r *testResourceReader) ListPVCs(context.Context, string) ([]corev1.PersistentVolumeClaim, error) {
	return []corev1.PersistentVolumeClaim{{}}, r.record("persistent volume claims")
}

func (r *testResourceReader) ListCronJobs(context.Context, string) ([]batchv1.CronJob, error) {
	return []batchv1.CronJob{{}}, r.record("cron jobs")
}

func (r *testResourceReader) ListHPAs(context.Context, string) ([]autoscalingv2.HorizontalPodAutoscaler, error) {
	return []autoscalingv2.HorizontalPodAutoscaler{{}}, r.record("horizontal pod autoscalers")
}

func (r *testResourceReader) ListSecrets(context.Context, string) ([]kube.ResourceMetadata, error) {
	return []kube.ResourceMetadata{{}}, r.record("secrets")
}

func (r *testResourceReader) ListReplicaSets(context.Context, string) ([]appsv1.ReplicaSet, error) {
	return []appsv1.ReplicaSet{{}}, r.record("replica sets")
}

func (r *testResourceReader) ListRBAC(context.Context, string) (kube.RBACResources, error) {
	resources := kube.RBACResources{Roles: []rbacv1.Role{{}}}
	return resources, r.record("role-based access control")
}

func (r *testResourceReader) ListCRDs(context.Context) ([]apiextensionsv1.CustomResourceDefinition, error) {
	return []apiextensionsv1.CustomResourceDefinition{{}}, r.record("custom resource definitions")
}

func (r *testResourceReader) ListPodMetrics(context.Context, string) ([]metricsv1beta1.PodMetrics, error) {
	return []metricsv1beta1.PodMetrics{{}}, r.record("pod metrics")
}

func (r *testResourceReader) ListResourceMetadata(_ context.Context, resource schema.GroupVersionResource, namespace string) ([]kube.ResourceMetadata, error) {
	r.metadataResource = resource
	r.metadataNamespace = namespace
	return []kube.ResourceMetadata{{}}, r.record("resource metadata")
}

func TestNativeClusterCommandsFetchEveryResource(t *testing.T) {
	reader := &testResourceReader{}
	commands := NewCommands(context.Background(), reader, &testResourceObserver{})
	commands.now = func() time.Time { return time.Unix(100, 0) }
	tests := []struct {
		name    string
		command func() tea.Msg
		want    tea.Msg
	}{
		{name: "pods", command: func() tea.Msg { return commands.FetchPods("ns")() }, want: model.PodsMsg{}},
		{name: "deployments", command: func() tea.Msg { return commands.FetchDeployments("ns")() }, want: model.DeploymentsMsg{}},
		{name: "events", command: func() tea.Msg { return commands.FetchEvents("ns")() }, want: model.EventsMsg{}},
		{name: "pod metrics", command: func() tea.Msg { return commands.FetchPodMetrics("ns")() }, want: model.MetricsMsg{}},
		{name: "services", command: func() tea.Msg { return commands.FetchServices("ns")() }, want: model.ServicesMsg{}},
		{name: "stateful sets", command: func() tea.Msg { return commands.FetchStatefulSets("ns")() }, want: model.StatefulSetsMsg{}},
		{name: "daemon sets", command: func() tea.Msg { return commands.FetchDaemonSets("ns")() }, want: model.DaemonSetsMsg{}},
		{name: "config maps", command: func() tea.Msg { return commands.FetchConfigMaps("ns")() }, want: model.ConfigMapsMsg{}},
		{name: "nodes", command: func() tea.Msg { return commands.FetchNodes()() }, want: model.NodesMsg{}},
		{name: "jobs", command: func() tea.Msg { return commands.FetchJobs("ns")() }, want: model.JobsMsg{}},
		{name: "ingresses", command: func() tea.Msg { return commands.FetchIngresses("ns")() }, want: model.IngressesMsg{}},
		{name: "network policies", command: func() tea.Msg { return commands.FetchNetworkPolicies("ns")() }, want: model.NetworkPoliciesMsg{}},
		{name: "persistent volume claims", command: func() tea.Msg { return commands.FetchPVCs("ns")() }, want: model.PVCsMsg{}},
		{name: "cron jobs", command: func() tea.Msg { return commands.FetchCronJobs("ns")() }, want: model.CronJobsMsg{}},
		{name: "horizontal pod autoscalers", command: func() tea.Msg { return commands.FetchHPAs("ns")() }, want: model.HPAsMsg{}},
		{name: "secrets", command: func() tea.Msg { return commands.FetchSecrets("ns")() }, want: model.SecretsMsg{}},
		{name: "replica sets", command: func() tea.Msg { return commands.FetchReplicaSets("ns")() }, want: model.ReplicaSetsMsg{}},
		{name: "role-based access control", command: func() tea.Msg { return commands.FetchRBAC("ns")() }, want: model.RBACMsg{}},
		{name: "custom resource definitions", command: func() tea.Msg { return commands.FetchCRDs()() }, want: model.CRDsMsg{}},
		{name: "custom resource instances", command: func() tea.Msg {
			crd := model.CRD{Group: "example.invalid", Plural: "widgets", PreferredVersion: "v1", Resource: "widgets.example.invalid"}
			return commands.FetchCRDInstances(crd, "ns")()
		}, want: model.CRDInstancesMsg{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			message := test.command()
			if reflect.TypeOf(message) != reflect.TypeOf(test.want) {
				t.Fatalf("message type = %T, want %T", message, test.want)
			}
		})
	}
	if len(reader.calls) != len(tests) {
		t.Fatalf("reader calls = %v, want one per command", reader.calls)
	}
}

func TestNativeClusterCommandsPropagateListErrors(t *testing.T) {
	sentinel := errors.New("list failed")
	reader := &testResourceReader{err: sentinel}
	commands := NewCommands(context.Background(), reader, &testResourceObserver{})
	pods := commands.FetchPods("ns")().(model.PodsMsg)
	if pods.Pods != nil || !errors.Is(pods.Err, sentinel) {
		t.Fatalf("FetchPods() = %+v, want sentinel and no payload", pods)
	}
	rbac := commands.FetchRBAC("ns")().(model.RBACMsg)
	if len(rbac.RBAC) != 1 || !errors.Is(rbac.Err, sentinel) {
		t.Fatalf("FetchRBAC() = %+v, want partial payload and sentinel", rbac)
	}
}

func TestNativeClusterCommandsUseClusterScopeForClusterCRDs(t *testing.T) {
	reader := &testResourceReader{}
	commands := NewCommands(context.Background(), reader, &testResourceObserver{})
	crd := model.CRD{
		Group: "example.invalid", Plural: "widgets", PreferredVersion: "v1",
		Resource: "widgets.example.invalid", Scope: "Cluster",
	}
	message := commands.FetchCRDInstances(crd, "selected-namespace")().(model.CRDInstancesMsg)
	wantResource := schema.GroupVersionResource{Group: "example.invalid", Version: "v1", Resource: "widgets"}
	if reader.metadataResource != wantResource || reader.metadataNamespace != "" {
		t.Fatalf("metadata request = (%v, %q), want (%v, cluster scope)", reader.metadataResource, reader.metadataNamespace, wantResource)
	}
	if message.Namespace != "selected-namespace" {
		t.Fatalf("message namespace = %q, want request scope", message.Namespace)
	}
}

var _ kube.ResourceReader = (*testResourceReader)(nil)
var _ kube.ResourceObserver = (*testResourceObserver)(nil)
