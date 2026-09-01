package ui

import (
	"context"
	"io"
	"sync"
	"sync/atomic"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"

	"github.com/HediAbed/opsmate/internal/kube"
)

type testResourceReader struct {
	err               error
	calls             []string
	resourceName      string
	resourceNamespace string
	metadataResource  schema.GroupVersionResource
	metadataNamespace string
}

type testResourceObserver struct {
	err               error
	calls             []string
	stops             atomic.Int32
	resourceName      string
	resourceNamespace string
}

type testKubeLiveSet[T interface{}] struct {
	changes chan struct{}
	state   kube.LiveState[T]
	stop    sync.Once
	onStop  func()
}

type testResourceInspector struct {
	reference kube.ResourceReference
	content   string
	err       error
}

type testPodReader struct {
	logRequest         kube.PodLogRequest
	logStream          io.ReadCloser
	logErr             error
	containerReference kube.PodReference
	containers         []string
	containerErr       error
}

type testResourceWriter struct {
	scaleRequest   kube.ScaleRequest
	scaleErr       error
	deleted        kube.ResourceReference
	deleteErr      error
	deleteBatch    kube.ResourceBatch
	deleteOutcome  kube.BatchOutcome
	deleteManyErr  error
	restarted      kube.WorkloadReference
	restartErr     error
	restartBatch   kube.WorkloadBatch
	restartOutcome kube.BatchOutcome
	restartManyErr error
}

type snapshotCollectorStub struct {
	snapshot  kube.ClusterSnapshot
	err       error
	context   context.Context
	namespace string
}

func newTestKubeLiveSet[T interface{}](items []T, err error, onStop func()) kube.LiveSet[T] {
	changes := make(chan struct{}, 1)
	changes <- struct{}{}
	return &testKubeLiveSet[T]{
		changes: changes,
		state:   kube.LiveState[T]{Items: items, Ready: err == nil, Err: err},
		onStop:  onStop,
	}
}

func newObservedTestLiveSet[T interface{}](
	observer *testResourceObserver,
	items []T,
	err error,
) kube.LiveSet[T] {
	return newTestKubeLiveSet(items, err, func() { observer.stops.Add(1) })
}

func (s *testKubeLiveSet[T]) Changes() <-chan struct{} {
	return s.changes
}

func (s *testKubeLiveSet[T]) State() kube.LiveState[T] {
	return s.state
}

func (s *testKubeLiveSet[T]) Stop() {
	s.stop.Do(func() {
		close(s.changes)
		s.onStop()
	})
}

func (o *testResourceObserver) record(name string) error {
	o.calls = append(o.calls, name)
	return o.err
}

func (o *testResourceObserver) metadata() metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: o.resourceName, Namespace: o.resourceNamespace}
}

func (o *testResourceObserver) ObservePods(context.Context, string) (kube.LiveSet[corev1.Pod], error) {
	err := o.record("pods")
	return newObservedTestLiveSet(o, []corev1.Pod{{ObjectMeta: o.metadata()}}, err), err
}

func (o *testResourceObserver) ObserveDeployments(context.Context, string) (kube.LiveSet[appsv1.Deployment], error) {
	err := o.record("deployments")
	return newObservedTestLiveSet(o, []appsv1.Deployment{{ObjectMeta: o.metadata()}}, err), err
}

func (o *testResourceObserver) ObserveEvents(context.Context, string) (kube.LiveSet[corev1.Event], error) {
	err := o.record("events")
	return newObservedTestLiveSet(o, []corev1.Event{{ObjectMeta: o.metadata()}}, err), err
}

func (o *testResourceObserver) ObserveIngresses(context.Context, string) (kube.LiveSet[networkingv1.Ingress], error) {
	err := o.record("ingresses")
	return newObservedTestLiveSet(o, []networkingv1.Ingress{{ObjectMeta: o.metadata()}}, err), err
}

func (o *testResourceObserver) ObserveNetworkPolicies(context.Context, string) (kube.LiveSet[networkingv1.NetworkPolicy], error) {
	err := o.record("network policies")
	return newObservedTestLiveSet(o, []networkingv1.NetworkPolicy{{ObjectMeta: o.metadata()}}, err), err
}

func (o *testResourceObserver) ObservePersistentVolumeClaims(context.Context, string) (kube.LiveSet[corev1.PersistentVolumeClaim], error) {
	err := o.record("persistent volume claims")
	return newObservedTestLiveSet(o, []corev1.PersistentVolumeClaim{{ObjectMeta: o.metadata()}}, err), err
}

func (o *testResourceObserver) ObserveCronJobs(context.Context, string) (kube.LiveSet[batchv1.CronJob], error) {
	err := o.record("cron jobs")
	return newObservedTestLiveSet(o, []batchv1.CronJob{{ObjectMeta: o.metadata()}}, err), err
}

func (o *testResourceObserver) ObserveHorizontalPodAutoscalers(context.Context, string) (kube.LiveSet[autoscalingv2.HorizontalPodAutoscaler], error) {
	err := o.record("horizontal pod autoscalers")
	return newObservedTestLiveSet(o, []autoscalingv2.HorizontalPodAutoscaler{{ObjectMeta: o.metadata()}}, err), err
}

func (o *testResourceObserver) ObserveSecrets(context.Context, string) (kube.LiveSet[kube.ResourceMetadata], error) {
	err := o.record("secrets")
	item := kube.ResourceMetadata{Name: o.resourceName, Namespace: o.resourceNamespace}
	return newObservedTestLiveSet(o, []kube.ResourceMetadata{item}, err), err
}

func (o *testResourceObserver) ObserveReplicaSets(context.Context, string) (kube.LiveSet[appsv1.ReplicaSet], error) {
	err := o.record("replica sets")
	return newObservedTestLiveSet(o, []appsv1.ReplicaSet{{ObjectMeta: o.metadata()}}, err), err
}

func (r *testResourceReader) record(name string) error {
	r.calls = append(r.calls, name)
	return r.err
}

func (r *testResourceReader) objectMetadata() metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: r.resourceName, Namespace: r.resourceNamespace}
}

func (r *testResourceReader) ListPods(context.Context, string) ([]corev1.Pod, error) {
	return []corev1.Pod{{ObjectMeta: r.objectMetadata()}}, r.record("pods")
}

func (r *testResourceReader) ListDeployments(context.Context, string) ([]appsv1.Deployment, error) {
	return []appsv1.Deployment{{ObjectMeta: r.objectMetadata()}}, r.record("deployments")
}

func (r *testResourceReader) ListEvents(context.Context, string) ([]corev1.Event, error) {
	return []corev1.Event{{}}, r.record("events")
}

func (r *testResourceReader) ListServices(context.Context, string) ([]corev1.Service, error) {
	return []corev1.Service{{ObjectMeta: r.objectMetadata()}}, r.record("services")
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
	return []metricsv1beta1.PodMetrics{{ObjectMeta: r.objectMetadata()}}, r.record("pod metrics")
}

func (r *testResourceReader) ListResourceMetadata(_ context.Context, resource schema.GroupVersionResource, namespace string) ([]kube.ResourceMetadata, error) {
	r.metadataResource = resource
	r.metadataNamespace = namespace
	return []kube.ResourceMetadata{{}}, r.record("resource metadata")
}

func (i *testResourceInspector) ResourceYAML(_ context.Context, reference kube.ResourceReference) (string, error) {
	i.reference = reference
	return i.content, i.err
}

func (r *testPodReader) OpenPodLogs(_ context.Context, request kube.PodLogRequest) (io.ReadCloser, error) {
	r.logRequest = request
	return r.logStream, r.logErr
}

func (r *testPodReader) PodContainers(_ context.Context, reference kube.PodReference) ([]string, error) {
	r.containerReference = reference
	return r.containers, r.containerErr
}

func (w *testResourceWriter) Scale(_ context.Context, request kube.ScaleRequest) error {
	w.scaleRequest = request
	return w.scaleErr
}

func (w *testResourceWriter) Delete(_ context.Context, reference kube.ResourceReference) error {
	w.deleted = reference
	return w.deleteErr
}

func (w *testResourceWriter) DeleteBatch(_ context.Context, batch kube.ResourceBatch) (kube.BatchOutcome, error) {
	w.deleteBatch = batch
	return w.deleteOutcome, w.deleteManyErr
}

func (w *testResourceWriter) Restart(_ context.Context, reference kube.WorkloadReference) error {
	w.restarted = reference
	return w.restartErr
}

func (w *testResourceWriter) RestartBatch(_ context.Context, batch kube.WorkloadBatch) (kube.BatchOutcome, error) {
	w.restartBatch = batch
	return w.restartOutcome, w.restartManyErr
}

func (c *snapshotCollectorStub) Collect(ctx context.Context, namespace string) (kube.ClusterSnapshot, error) {
	c.context = ctx
	c.namespace = namespace
	return c.snapshot, c.err
}

var _ kube.ResourceReader = (*testResourceReader)(nil)
var _ kube.ResourceObserver = (*testResourceObserver)(nil)
var _ kube.ResourceInspector = (*testResourceInspector)(nil)
var _ kube.PodReader = (*testPodReader)(nil)
var _ kube.ResourceWriter = (*testResourceWriter)(nil)
var _ ClusterSnapshotCollector = (*snapshotCollectorStub)(nil)
