package kube

import (
	"context"
	"errors"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
	metadatafake "k8s.io/client-go/metadata/fake"
	clienttesting "k8s.io/client-go/testing"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsfake "k8s.io/metrics/pkg/client/clientset/versioned/fake"
)

const testNamespace = "team-a"

type resourceCall struct {
	name string
	call func() (int, string, error)
}

func namespacedListProbe[T interface{}](list func(context.Context, string) ([]T, error), name func(T) string) func() (int, string, error) {
	return func() (int, string, error) {
		items, err := list(context.Background(), testNamespace)
		return len(items), firstName(items, name), err
	}
}

func clusterListProbe[T interface{}](list func(context.Context) ([]T, error), name func(T) string) func() (int, string, error) {
	return func() (int, string, error) {
		items, err := list(context.Background())
		return len(items), firstName(items, name), err
	}
}

func namespacedNilContextProbe[T interface{}](list func(context.Context, string) ([]T, error)) func() error {
	return func() error {
		var missingContext context.Context
		_, err := list(missingContext, testNamespace)
		return err
	}
}

func clusterNilContextProbe[T interface{}](list func(context.Context) ([]T, error)) func() error {
	return func() error {
		var missingContext context.Context
		_, err := list(missingContext)
		return err
	}
}

func TestManagerListsTypedResources(t *testing.T) {
	manager := populatedResourceManager(t)
	calls := []resourceCall{
		{name: "pods", call: namespacedListProbe(manager.ListPods, func(item corev1.Pod) string { return item.Name })},
		{name: "deployments", call: namespacedListProbe(manager.ListDeployments, func(item appsv1.Deployment) string { return item.Name })},
		{name: "events", call: namespacedListProbe(manager.ListEvents, func(item corev1.Event) string { return item.Name })},
		{name: "services", call: namespacedListProbe(manager.ListServices, func(item corev1.Service) string { return item.Name })},
		{name: "stateful sets", call: namespacedListProbe(manager.ListStatefulSets, func(item appsv1.StatefulSet) string { return item.Name })},
		{name: "daemon sets", call: namespacedListProbe(manager.ListDaemonSets, func(item appsv1.DaemonSet) string { return item.Name })},
		{name: "config maps", call: namespacedListProbe(manager.ListConfigMaps, func(item corev1.ConfigMap) string { return item.Name })},
		{name: "nodes", call: clusterListProbe(manager.ListNodes, func(item corev1.Node) string { return item.Name })},
		{name: "jobs", call: namespacedListProbe(manager.ListJobs, func(item batchv1.Job) string { return item.Name })},
		{name: "ingresses", call: namespacedListProbe(manager.ListIngresses, func(item networkingv1.Ingress) string { return item.Name })},
		{name: "network policies", call: namespacedListProbe(manager.ListNetworkPolicies, func(item networkingv1.NetworkPolicy) string { return item.Name })},
		{name: "persistent volume claims", call: namespacedListProbe(manager.ListPVCs, func(item corev1.PersistentVolumeClaim) string { return item.Name })},
		{name: "cron jobs", call: namespacedListProbe(manager.ListCronJobs, func(item batchv1.CronJob) string { return item.Name })},
		{name: "horizontal pod autoscalers", call: namespacedListProbe(manager.ListHPAs, func(item autoscalingv2.HorizontalPodAutoscaler) string { return item.Name })},
		{name: "secrets", call: namespacedListProbe(manager.ListSecrets, func(item ResourceMetadata) string { return item.Name })},
		{name: "replica sets", call: namespacedListProbe(manager.ListReplicaSets, func(item appsv1.ReplicaSet) string { return item.Name })},
		{name: "custom resource definitions", call: clusterListProbe(manager.ListCRDs, func(item apiextensionsv1.CustomResourceDefinition) string { return item.Name })},
		{name: "pod metrics", call: namespacedListProbe(manager.ListPodMetrics, func(item metricsv1beta1.PodMetrics) string { return item.Name })},
	}
	for _, call := range calls {
		t.Run(call.name, func(t *testing.T) {
			count, name, err := call.call()
			if err != nil || count != 1 || name != "sample" {
				t.Fatalf("list result = (%d, %q, %v), want one sample", count, name, err)
			}
		})
	}
}

func TestManagerListsRBACResources(t *testing.T) {
	manager := populatedResourceManager(t)
	rbac, err := manager.ListRBAC(context.Background(), testNamespace)
	if err != nil {
		t.Fatalf("ListRBAC() error = %v", err)
	}
	if len(rbac.ServiceAccounts) != 1 || len(rbac.Roles) != 1 || len(rbac.RoleBindings) != 1 || len(rbac.ClusterRoles) != 1 || len(rbac.ClusterRoleBindings) != 1 {
		t.Fatalf("ListRBAC() = %+v, want one of each resource", rbac)
	}
}

func TestManagerReturnsOnlySafeSecretMetadata(t *testing.T) {
	createdAt := metav1.Now()
	metadataClient := newMetadataClient(t, &metav1.PartialObjectMetadata{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{
			Name:              "credentials",
			Namespace:         testNamespace,
			CreationTimestamp: createdAt,
			Annotations: map[string]string{
				"kubectl.kubernetes.io/last-applied-configuration": "sensitive payload",
			},
		},
	})
	manager := resourceManager(
		kubernetesfake.NewSimpleClientset(),
		metricsfake.NewSimpleClientset(),
		apiextensionsfake.NewSimpleClientset(),
		metadataClient,
	)

	secrets, err := manager.ListSecrets(context.Background(), testNamespace)
	if err != nil {
		t.Fatalf("ListSecrets() error = %v", err)
	}
	want := ResourceMetadata{Name: "credentials", Namespace: testNamespace, CreatedAt: createdAt.Time}
	if len(secrets) != 1 || secrets[0] != want {
		t.Fatalf("ListSecrets() = %+v, want only safe metadata %+v", secrets, want)
	}
}

func TestManagerListsSafeCustomResourceMetadata(t *testing.T) {
	createdAt := metav1.Now()
	metadataClient := newMetadataClient(t)
	metadataClient.PrependReactor("list", "widgets", func(clienttesting.Action) (bool, runtime.Object, error) {
		item := &metav1.PartialObjectMetadata{ObjectMeta: metav1.ObjectMeta{
			Name: "widget-a", Namespace: testNamespace, CreationTimestamp: createdAt,
			Annotations: map[string]string{"private": "not returned"},
		}}
		return true, &metav1.List{Items: []runtime.RawExtension{{Object: item}}}, nil
	})
	manager := resourceManager(
		kubernetesfake.NewSimpleClientset(),
		metricsfake.NewSimpleClientset(),
		apiextensionsfake.NewSimpleClientset(),
		metadataClient,
	)
	resource := schema.GroupVersionResource{Group: "example.invalid", Version: "v1", Resource: "widgets"}

	items, err := manager.ListResourceMetadata(context.Background(), resource, testNamespace)
	if err != nil {
		t.Fatalf("ListResourceMetadata() error = %v", err)
	}
	want := ResourceMetadata{Name: "widget-a", Namespace: testNamespace, CreatedAt: createdAt.Time}
	if len(items) != 1 || items[0] != want {
		t.Fatalf("ListResourceMetadata() = %+v, want %+v", items, want)
	}
}

func TestManagerNormalizesEmptyResourceLists(t *testing.T) {
	manager := resourceManager(
		kubernetesfake.NewSimpleClientset(),
		metricsfake.NewSimpleClientset(),
		apiextensionsfake.NewSimpleClientset(),
		newMetadataClient(t),
	)
	pods, err := manager.ListPods(context.Background(), testNamespace)
	if err != nil || pods == nil || len(pods) != 0 {
		t.Fatalf("ListPods() = (%#v, %v), want non-nil empty slice", pods, err)
	}
}

func TestManagerResourceListFailures(t *testing.T) {
	sentinel := errors.New("request failed")
	kubernetesClient := kubernetesfake.NewSimpleClientset()
	kubernetesClient.PrependReactor("list", "*", failingListReactor(sentinel))
	metricsClient := metricsfake.NewSimpleClientset()
	metricsClient.PrependReactor("list", "*", failingListReactor(sentinel))
	extensionsClient := apiextensionsfake.NewSimpleClientset()
	extensionsClient.PrependReactor("list", "*", failingListReactor(sentinel))
	metadataClient := newMetadataClient(t)
	metadataClient.PrependReactor("list", "*", failingListReactor(sentinel))
	manager := resourceManager(kubernetesClient, metricsClient, extensionsClient, metadataClient)

	calls := []struct {
		name string
		call func() error
	}{
		{name: "pods", call: func() error { _, err := manager.ListPods(context.Background(), testNamespace); return err }},
		{name: "deployments", call: func() error { _, err := manager.ListDeployments(context.Background(), testNamespace); return err }},
		{name: "events", call: func() error { _, err := manager.ListEvents(context.Background(), testNamespace); return err }},
		{name: "services", call: func() error { _, err := manager.ListServices(context.Background(), testNamespace); return err }},
		{name: "stateful sets", call: func() error { _, err := manager.ListStatefulSets(context.Background(), testNamespace); return err }},
		{name: "daemon sets", call: func() error { _, err := manager.ListDaemonSets(context.Background(), testNamespace); return err }},
		{name: "config maps", call: func() error { _, err := manager.ListConfigMaps(context.Background(), testNamespace); return err }},
		{name: "nodes", call: func() error { _, err := manager.ListNodes(context.Background()); return err }},
		{name: "jobs", call: func() error { _, err := manager.ListJobs(context.Background(), testNamespace); return err }},
		{name: "ingresses", call: func() error { _, err := manager.ListIngresses(context.Background(), testNamespace); return err }},
		{name: "network policies", call: func() error { _, err := manager.ListNetworkPolicies(context.Background(), testNamespace); return err }},
		{name: "persistent volume claims", call: func() error { _, err := manager.ListPVCs(context.Background(), testNamespace); return err }},
		{name: "cron jobs", call: func() error { _, err := manager.ListCronJobs(context.Background(), testNamespace); return err }},
		{name: "horizontal pod autoscalers", call: func() error { _, err := manager.ListHPAs(context.Background(), testNamespace); return err }},
		{name: "secrets", call: func() error { _, err := manager.ListSecrets(context.Background(), testNamespace); return err }},
		{name: "replica sets", call: func() error { _, err := manager.ListReplicaSets(context.Background(), testNamespace); return err }},
		{name: "custom resource definitions", call: func() error { _, err := manager.ListCRDs(context.Background()); return err }},
		{name: "pod metrics", call: func() error { _, err := manager.ListPodMetrics(context.Background(), testNamespace); return err }},
		{name: "resource metadata", call: func() error {
			resource := schema.GroupVersionResource{Group: "example.invalid", Version: "v1", Resource: "widgets"}
			_, err := manager.ListResourceMetadata(context.Background(), resource, testNamespace)
			return err
		}},
		{name: "role-based access control", call: func() error { _, err := manager.ListRBAC(context.Background(), testNamespace); return err }},
	}
	for _, call := range calls {
		t.Run(call.name, func(t *testing.T) {
			if err := call.call(); !errors.Is(err, sentinel) {
				t.Fatalf("list error = %v, want sentinel", err)
			}
		})
	}
}

func TestManagerReturnsPartialRBACResults(t *testing.T) {
	sentinel := errors.New("cluster roles forbidden")
	kubernetesClient := kubernetesfake.NewSimpleClientset(
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "builder", Namespace: testNamespace}},
		&rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: "reader", Namespace: testNamespace}},
		&rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "readers", Namespace: testNamespace}},
		&rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "viewers"}},
	)
	kubernetesClient.PrependReactor("list", "clusterroles", failingListReactor(sentinel))
	manager := resourceManager(
		kubernetesClient,
		metricsfake.NewSimpleClientset(),
		apiextensionsfake.NewSimpleClientset(),
		newMetadataClient(t),
	)

	resources, err := manager.ListRBAC(context.Background(), testNamespace)
	if !errors.Is(err, sentinel) {
		t.Fatalf("ListRBAC() error = %v, want sentinel", err)
	}
	if len(resources.ServiceAccounts) != 1 || len(resources.Roles) != 1 || len(resources.RoleBindings) != 1 || len(resources.ClusterRoles) != 0 || len(resources.ClusterRoleBindings) != 1 {
		t.Fatalf("ListRBAC() resources = %+v, want successful partial results", resources)
	}
}

func TestListResourcesHonorCanceledContext(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	manager := &Manager{}
	if pods, err := manager.ListPods(cancelled, testNamespace); pods != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("ListPods(cancelled) = (%v, %v)", pods, err)
	}
	if rbac, err := manager.ListRBAC(cancelled, testNamespace); !errors.Is(err, context.Canceled) || !rbacResourcesEmpty(rbac) {
		t.Fatalf("ListRBAC(cancelled) = (%+v, %v)", rbac, err)
	}
	resource := schema.GroupVersionResource{Group: "example.invalid", Version: "v1", Resource: "widgets"}
	if items, err := manager.ListResourceMetadata(cancelled, resource, testNamespace); items != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("ListResourceMetadata(cancelled) = (%v, %v)", items, err)
	}
}

func TestResourceListsRejectNilContext(t *testing.T) {
	manager := populatedResourceManager(t)
	var missingContext context.Context
	calls := []struct {
		name string
		call func() error
	}{
		{name: "pods", call: namespacedNilContextProbe(manager.ListPods)},
		{name: "deployments", call: namespacedNilContextProbe(manager.ListDeployments)},
		{name: "events", call: namespacedNilContextProbe(manager.ListEvents)},
		{name: "services", call: namespacedNilContextProbe(manager.ListServices)},
		{name: "stateful sets", call: namespacedNilContextProbe(manager.ListStatefulSets)},
		{name: "daemon sets", call: namespacedNilContextProbe(manager.ListDaemonSets)},
		{name: "config maps", call: namespacedNilContextProbe(manager.ListConfigMaps)},
		{name: "nodes", call: clusterNilContextProbe(manager.ListNodes)},
		{name: "jobs", call: namespacedNilContextProbe(manager.ListJobs)},
		{name: "ingresses", call: namespacedNilContextProbe(manager.ListIngresses)},
		{name: "network policies", call: namespacedNilContextProbe(manager.ListNetworkPolicies)},
		{name: "persistent volume claims", call: namespacedNilContextProbe(manager.ListPVCs)},
		{name: "cron jobs", call: namespacedNilContextProbe(manager.ListCronJobs)},
		{name: "horizontal pod autoscalers", call: namespacedNilContextProbe(manager.ListHPAs)},
		{name: "secrets", call: namespacedNilContextProbe(manager.ListSecrets)},
		{name: "replica sets", call: namespacedNilContextProbe(manager.ListReplicaSets)},
		{name: "custom resource definitions", call: clusterNilContextProbe(manager.ListCRDs)},
		{name: "pod metrics", call: namespacedNilContextProbe(manager.ListPodMetrics)},
		{name: "rbac", call: func() error {
			_, err := manager.ListRBAC(missingContext, testNamespace)
			return err
		}},
		{name: "resource metadata", call: func() error {
			resource := schema.GroupVersionResource{Group: "example.invalid", Version: "v1", Resource: "widgets"}
			_, err := manager.ListResourceMetadata(missingContext, resource, testNamespace)
			return err
		}},
	}
	for _, test := range calls {
		t.Run(test.name, func(t *testing.T) {
			err := errorWithoutPanic(t, "listing "+test.name+" without a context", test.call)
			if !errors.Is(err, ErrContextRequired) {
				t.Fatalf("listing %s without a context: error = %v, want context-required error", test.name, err)
			}
		})
	}
}

func TestListResourcesRequireIdentifierAndConnection(t *testing.T) {
	manager := &Manager{}
	if items, err := manager.ListResourceMetadata(context.Background(), schema.GroupVersionResource{}, testNamespace); items != nil || !errors.Is(err, ErrResourceIdentifierRequired) {
		t.Fatalf("ListResourceMetadata(invalid) = (%v, %v)", items, err)
	}
	if pods, err := manager.ListPods(context.Background(), testNamespace); pods != nil || !errors.Is(err, ErrClientUnavailable) {
		t.Fatalf("ListPods(disconnected) = (%v, %v)", pods, err)
	}
	if rbac, err := manager.ListRBAC(context.Background(), testNamespace); !errors.Is(err, ErrClientUnavailable) || !rbacResourcesEmpty(rbac) {
		t.Fatalf("ListRBAC(disconnected) = (%+v, %v)", rbac, err)
	}
}

func rbacResourcesEmpty(resources RBACResources) bool {
	return len(resources.ServiceAccounts) == 0 &&
		len(resources.Roles) == 0 &&
		len(resources.RoleBindings) == 0 &&
		len(resources.ClusterRoles) == 0 &&
		len(resources.ClusterRoleBindings) == 0
}

func populatedResourceManager(t *testing.T) *Manager {
	t.Helper()
	objectMeta := func() metav1.ObjectMeta {
		return metav1.ObjectMeta{Name: "sample", Namespace: testNamespace}
	}
	kubernetesClient := kubernetesfake.NewSimpleClientset(
		&corev1.Pod{ObjectMeta: objectMeta()},
		&appsv1.Deployment{ObjectMeta: objectMeta()},
		&corev1.Event{ObjectMeta: objectMeta()},
		&corev1.Service{ObjectMeta: objectMeta()},
		&appsv1.StatefulSet{ObjectMeta: objectMeta()},
		&appsv1.DaemonSet{ObjectMeta: objectMeta()},
		&corev1.ConfigMap{ObjectMeta: objectMeta()},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "sample"}},
		&batchv1.Job{ObjectMeta: objectMeta()},
		&networkingv1.Ingress{ObjectMeta: objectMeta()},
		&networkingv1.NetworkPolicy{ObjectMeta: objectMeta()},
		&corev1.PersistentVolumeClaim{ObjectMeta: objectMeta()},
		&batchv1.CronJob{ObjectMeta: objectMeta()},
		&autoscalingv2.HorizontalPodAutoscaler{ObjectMeta: objectMeta()},
		&appsv1.ReplicaSet{ObjectMeta: objectMeta()},
		&corev1.ServiceAccount{ObjectMeta: objectMeta()},
		&rbacv1.Role{ObjectMeta: objectMeta()},
		&rbacv1.RoleBinding{ObjectMeta: objectMeta()},
		&rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "sample"}},
		&rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "sample"}},
	)
	metricsClient := metricsfake.NewSimpleClientset()
	metricsClient.PrependReactor("list", "pods", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, &metricsv1beta1.PodMetricsList{
			Items: []metricsv1beta1.PodMetrics{{ObjectMeta: objectMeta()}},
		}, nil
	})
	extensionsClient := apiextensionsfake.NewSimpleClientset(&apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "sample"}})
	metadataClient := newMetadataClient(t, &metav1.PartialObjectMetadata{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: objectMeta(),
	})
	return resourceManager(kubernetesClient, metricsClient, extensionsClient, metadataClient)
}

func resourceManager(
	kubernetesClient *kubernetesfake.Clientset,
	metricsClient *metricsfake.Clientset,
	extensionsClient *apiextensionsfake.Clientset,
	metadataClient *metadatafake.FakeMetadataClient,
) *Manager {
	return &Manager{clients: &Clients{
		kubernetes: kubernetesClient,
		metrics:    metricsClient,
		extensions: extensionsClient,
		metadata:   metadataClient,
	}}
}

func newMetadataClient(t *testing.T, objects ...runtime.Object) *metadatafake.FakeMetadataClient {
	t.Helper()
	scheme := metadatafake.NewTestScheme()
	if err := metav1.AddMetaToScheme(scheme); err != nil {
		t.Fatalf("AddMetaToScheme() error = %v", err)
	}
	return metadatafake.NewSimpleMetadataClient(scheme, objects...)
}

func failingListReactor(err error) clienttesting.ReactionFunc {
	return func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, err
	}
}

func firstName[T interface{}](items []T, name func(T) string) string {
	if len(items) == 0 {
		return ""
	}
	return name(items[0])
}
