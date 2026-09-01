package kube

import (
	"cmp"
	"context"
	"slices"
	"sync"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8swatch "k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/watchlist"
)

const informerResyncPeriod = time.Duration(0)

type LiveState[T interface{}] struct {
	Items []T
	Ready bool
	Err   error
}

type LiveSet[T interface{}] interface {
	Changes() <-chan struct{}
	State() LiveState[T]
	Stop()
}

type ResourceObserver interface {
	ObservePods(context.Context, string) (LiveSet[corev1.Pod], error)
	ObserveDeployments(context.Context, string) (LiveSet[appsv1.Deployment], error)
	ObserveEvents(context.Context, string) (LiveSet[corev1.Event], error)
	ObserveIngresses(context.Context, string) (LiveSet[networkingv1.Ingress], error)
	ObserveNetworkPolicies(context.Context, string) (LiveSet[networkingv1.NetworkPolicy], error)
	ObservePersistentVolumeClaims(context.Context, string) (LiveSet[corev1.PersistentVolumeClaim], error)
	ObserveCronJobs(context.Context, string) (LiveSet[batchv1.CronJob], error)
	ObserveHorizontalPodAutoscalers(context.Context, string) (LiveSet[autoscalingv2.HorizontalPodAutoscaler], error)
	ObserveSecrets(context.Context, string) (LiveSet[ResourceMetadata], error)
	ObserveReplicaSets(context.Context, string) (LiveSet[appsv1.ReplicaSet], error)
}

type liveInformer interface {
	AddEventHandler(cache.ResourceEventHandler) (cache.ResourceEventHandlerRegistration, error)
	GetStore() cache.Store
	HasSynced() bool
	RunWithContext(context.Context)
	SetTransform(cache.TransformFunc) error
	SetWatchErrorHandlerWithContext(cache.WatchErrorHandlerWithContext) error
}

type liveObjectDecoder[T interface{}] func(interface{}) (T, error)

type liveSet[T interface{}] struct {
	ctx       context.Context
	cancel    context.CancelFunc
	informer  liveInformer
	subject   Subject
	decode    liveObjectDecoder[T]
	changes   chan struct{}
	done      chan struct{}
	stopOnce  sync.Once
	statusMu  sync.RWMutex
	ready     bool
	lastError error
}

type keyedLiveObject[T interface{}] struct {
	key   string
	value T
}

type typedListWatchClient[L runtime.Object] interface {
	List(context.Context, metav1.ListOptions) (L, error)
	Watch(context.Context, metav1.ListOptions) (k8swatch.Interface, error)
}

type liveResourceClient[L runtime.Object] struct {
	resource       typedListWatchClient[L]
	capabilityHint interface{}
}

type connectionTrackingListerWatcher struct {
	delegate  cache.ListerWatcherWithContext
	onSuccess func()
}

func observedClient[L runtime.Object](resource typedListWatchClient[L], capabilityHint interface{}) liveResourceClient[L] {
	return liveResourceClient[L]{resource: resource, capabilityHint: capabilityHint}
}

func (m *Manager) ObservePods(ctx context.Context, namespace string) (LiveSet[corev1.Pod], error) {
	return observeResource(ctx, m, SubjectPods, func(clients *Clients) liveResourceClient[*corev1.PodList] {
		return observedClient(clients.Kubernetes().CoreV1().Pods(namespace), clients.Kubernetes())
	}, &corev1.Pod{}, decodeLivePod, stripManagedFields)
}

func (m *Manager) ObserveDeployments(ctx context.Context, namespace string) (LiveSet[appsv1.Deployment], error) {
	return observeResource(ctx, m, SubjectDeployments, func(clients *Clients) liveResourceClient[*appsv1.DeploymentList] {
		return observedClient(clients.Kubernetes().AppsV1().Deployments(namespace), clients.Kubernetes())
	}, &appsv1.Deployment{}, decodeLiveDeployment, stripManagedFields)
}

func (m *Manager) ObserveEvents(ctx context.Context, namespace string) (LiveSet[corev1.Event], error) {
	return observeResource(ctx, m, SubjectEvents, func(clients *Clients) liveResourceClient[*corev1.EventList] {
		return observedClient(clients.Kubernetes().CoreV1().Events(namespace), clients.Kubernetes())
	}, &corev1.Event{}, decodeLiveEvent, stripManagedFields)
}

func (m *Manager) ObserveIngresses(ctx context.Context, namespace string) (LiveSet[networkingv1.Ingress], error) {
	return observeResource(ctx, m, SubjectIngresses, func(clients *Clients) liveResourceClient[*networkingv1.IngressList] {
		return observedClient(clients.Kubernetes().NetworkingV1().Ingresses(namespace), clients.Kubernetes())
	}, &networkingv1.Ingress{}, decodeLiveIngress, stripManagedFields)
}

func (m *Manager) ObserveNetworkPolicies(ctx context.Context, namespace string) (LiveSet[networkingv1.NetworkPolicy], error) {
	return observeResource(ctx, m, SubjectNetworkPolicies, func(clients *Clients) liveResourceClient[*networkingv1.NetworkPolicyList] {
		return observedClient(clients.Kubernetes().NetworkingV1().NetworkPolicies(namespace), clients.Kubernetes())
	}, &networkingv1.NetworkPolicy{}, decodeLiveNetworkPolicy, stripManagedFields)
}

func (m *Manager) ObservePersistentVolumeClaims(ctx context.Context, namespace string) (LiveSet[corev1.PersistentVolumeClaim], error) {
	return observeResource(ctx, m, SubjectPVCs, func(clients *Clients) liveResourceClient[*corev1.PersistentVolumeClaimList] {
		return observedClient(clients.Kubernetes().CoreV1().PersistentVolumeClaims(namespace), clients.Kubernetes())
	}, &corev1.PersistentVolumeClaim{}, decodeLivePersistentVolumeClaim, stripManagedFields)
}

func (m *Manager) ObserveCronJobs(ctx context.Context, namespace string) (LiveSet[batchv1.CronJob], error) {
	return observeResource(ctx, m, SubjectCronJobs, func(clients *Clients) liveResourceClient[*batchv1.CronJobList] {
		return observedClient(clients.Kubernetes().BatchV1().CronJobs(namespace), clients.Kubernetes())
	}, &batchv1.CronJob{}, decodeLiveCronJob, stripManagedFields)
}

func (m *Manager) ObserveHorizontalPodAutoscalers(ctx context.Context, namespace string) (LiveSet[autoscalingv2.HorizontalPodAutoscaler], error) {
	return observeResource(ctx, m, SubjectHPAs, func(clients *Clients) liveResourceClient[*autoscalingv2.HorizontalPodAutoscalerList] {
		return observedClient(clients.Kubernetes().AutoscalingV2().HorizontalPodAutoscalers(namespace), clients.Kubernetes())
	}, &autoscalingv2.HorizontalPodAutoscaler{}, decodeLiveHorizontalPodAutoscaler, stripManagedFields)
}

func (m *Manager) ObserveSecrets(ctx context.Context, namespace string) (LiveSet[ResourceMetadata], error) {
	return observeResource(ctx, m, SubjectSecrets, func(clients *Clients) liveResourceClient[*metav1.PartialObjectMetadataList] {
		return observedClient(clients.Metadata().Resource(corev1.SchemeGroupVersion.WithResource("secrets")).Namespace(namespace), clients.Metadata())
	}, &metav1.PartialObjectMetadata{}, decodeLiveMetadata, stripSensitiveMetadata)
}

func (m *Manager) ObserveReplicaSets(ctx context.Context, namespace string) (LiveSet[appsv1.ReplicaSet], error) {
	return observeResource(ctx, m, SubjectReplicaSets, func(clients *Clients) liveResourceClient[*appsv1.ReplicaSetList] {
		return observedClient(clients.Kubernetes().AppsV1().ReplicaSets(namespace), clients.Kubernetes())
	}, &appsv1.ReplicaSet{}, decodeLiveReplicaSet, stripManagedFields)
}

func observeResource[T interface{}, L runtime.Object](
	parent context.Context,
	manager *Manager,
	subject Subject,
	clientFor func(*Clients) liveResourceClient[L],
	prototype runtime.Object,
	decode liveObjectDecoder[T],
	transform cache.TransformFunc,
) (LiveSet[T], error) {
	clients, ctx, cancel, err := manager.clientSession(parent)
	if err != nil {
		return nil, newError(OperationObserve, subject, "", err)
	}
	client := clientFor(clients)
	return newLiveSetWithContext(ctx, cancel, subject, newTypedListWatch(client.resource, client.capabilityHint), prototype, decode, transform)
}

func newTypedListWatch[L runtime.Object](client typedListWatchClient[L], capabilityHint interface{}) cache.ListerWatcher {
	listWatch := &cache.ListWatch{
		ListWithContextFunc: func(ctx context.Context, options metav1.ListOptions) (runtime.Object, error) {
			return client.List(ctx, options)
		},
		WatchFuncWithContext: func(ctx context.Context, options metav1.ListOptions) (k8swatch.Interface, error) {
			return client.Watch(ctx, options)
		},
	}
	return cache.ToListWatcherWithWatchListSemantics(listWatch, capabilityHint)
}

func newLiveSet[T interface{}](
	parent context.Context,
	subject Subject,
	listWatcher cache.ListerWatcher,
	prototype runtime.Object,
	decode liveObjectDecoder[T],
	transform cache.TransformFunc,
) (LiveSet[T], error) {
	if parent == nil {
		return nil, newError(OperationObserve, subject, "", ErrContextRequired)
	}
	ctx, cancel := context.WithCancel(parent)
	return newLiveSetWithContext(ctx, cancel, subject, listWatcher, prototype, decode, transform)
}

func newLiveSetWithContext[T interface{}](
	ctx context.Context,
	cancel context.CancelFunc,
	subject Subject,
	listWatcher cache.ListerWatcher,
	prototype runtime.Object,
	decode liveObjectDecoder[T],
	transform cache.TransformFunc,
) (LiveSet[T], error) {
	if err := validateLiveSetInputs(ctx, listWatcher, prototype, decode, transform); err != nil {
		cancel()
		return nil, newError(OperationObserve, subject, "", err)
	}
	set := newLiveSetState(ctx, cancel, subject, decode)
	trackedListWatcher := newConnectionTrackingListerWatcher(listWatcher, set.recordConnectionSuccess)
	set.informer = cache.NewSharedIndexInformer(trackedListWatcher, prototype, informerResyncPeriod, cache.Indexers{})
	return configureAndStartLiveSet(set, transform)
}

func startLiveSet[T interface{}](
	ctx context.Context,
	cancel context.CancelFunc,
	subject Subject,
	informer liveInformer,
	decode liveObjectDecoder[T],
	transform cache.TransformFunc,
) (LiveSet[T], error) {
	if informer == nil {
		cancel()
		return nil, newError(OperationObserve, subject, "", ErrInformerRequired)
	}
	set := newLiveSetState(ctx, cancel, subject, decode)
	set.informer = informer
	return configureAndStartLiveSet(set, transform)
}

func newLiveSetState[T interface{}](
	ctx context.Context,
	cancel context.CancelFunc,
	subject Subject,
	decode liveObjectDecoder[T],
) *liveSet[T] {
	return &liveSet[T]{
		ctx:     ctx,
		cancel:  cancel,
		subject: subject,
		decode:  decode,
		changes: make(chan struct{}, 1),
		done:    make(chan struct{}),
	}
}

func configureAndStartLiveSet[T interface{}](set *liveSet[T], transform cache.TransformFunc) (LiveSet[T], error) {
	if err := configureLiveSet(set, transform); err != nil {
		set.cancel()
		return nil, newError(OperationObserve, set.subject, "", err)
	}
	go set.run()
	return set, nil
}

func newConnectionTrackingListerWatcher(listWatcher cache.ListerWatcher, onSuccess func()) *connectionTrackingListerWatcher {
	return &connectionTrackingListerWatcher{
		delegate:  cache.ToListerWatcherWithContext(listWatcher),
		onSuccess: onSuccess,
	}
}

func (w *connectionTrackingListerWatcher) List(options metav1.ListOptions) (runtime.Object, error) {
	return w.ListWithContext(context.Background(), options)
}

func (w *connectionTrackingListerWatcher) ListWithContext(ctx context.Context, options metav1.ListOptions) (runtime.Object, error) {
	object, err := w.delegate.ListWithContext(ctx, options)
	if err == nil {
		w.onSuccess()
	}
	return object, err
}

func (w *connectionTrackingListerWatcher) Watch(options metav1.ListOptions) (k8swatch.Interface, error) {
	return w.WatchWithContext(context.Background(), options)
}

func (w *connectionTrackingListerWatcher) WatchWithContext(ctx context.Context, options metav1.ListOptions) (k8swatch.Interface, error) {
	stream, err := w.delegate.WatchWithContext(ctx, options)
	if err == nil {
		w.onSuccess()
	}
	return stream, err
}

func (w *connectionTrackingListerWatcher) IsWatchListSemanticsUnSupported() bool {
	return watchlist.DoesClientNotSupportWatchListSemantics(w.delegate)
}

func validateLiveSetInputs[T interface{}](
	ctx context.Context,
	listWatcher cache.ListerWatcher,
	prototype runtime.Object,
	decode liveObjectDecoder[T],
	transform cache.TransformFunc,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if listWatcher == nil {
		return ErrListWatcherRequired
	}
	if prototype == nil {
		return ErrResourcePrototypeRequired
	}
	if decode == nil {
		return ErrResourceDecoderRequired
	}
	if transform == nil {
		return ErrResourceTransformRequired
	}
	return nil
}

func configureLiveSet[T interface{}](set *liveSet[T], transform cache.TransformFunc) error {
	if err := set.informer.SetTransform(transform); err != nil {
		return err
	}
	if err := set.informer.SetWatchErrorHandlerWithContext(set.handleWatchError); err != nil {
		return err
	}
	_, err := set.informer.AddEventHandler(set.eventHandler())
	return err
}

func (s *liveSet[T]) eventHandler() cache.ResourceEventHandler {
	return cache.ResourceEventHandlerDetailedFuncs{
		AddFunc: s.recordAddedResource,
		UpdateFunc: func(_, _ interface{}) {
			s.recordChange()
		},
		DeleteFunc: func(interface{}) {
			s.recordChange()
		},
	}
}

func (s *liveSet[T]) recordAddedResource(_ interface{}, initial bool) {
	if !initial {
		s.recordChange()
	}
}

func (s *liveSet[T]) handleWatchError(ctx context.Context, _ *cache.Reflector, err error) {
	if ctx.Err() != nil {
		return
	}
	s.statusMu.Lock()
	s.lastError = newError(OperationObserve, s.subject, "", err)
	s.statusMu.Unlock()
	s.notify()
}

func (s *liveSet[T]) recordChange() {
	if s.ctx.Err() != nil {
		return
	}
	s.statusMu.Lock()
	s.lastError = nil
	s.statusMu.Unlock()
	s.notify()
}

func (s *liveSet[T]) recordConnectionSuccess() {
	if s.ctx.Err() != nil {
		return
	}
	s.statusMu.Lock()
	recovered := s.lastError != nil
	s.lastError = nil
	s.statusMu.Unlock()
	if recovered {
		s.notify()
	}
}

func (s *liveSet[T]) markReady() {
	s.statusMu.Lock()
	s.ready = true
	s.lastError = nil
	s.statusMu.Unlock()
	s.notify()
}

func (s *liveSet[T]) notify() {
	if s.ctx.Err() != nil {
		return
	}
	select {
	case s.changes <- struct{}{}:
	default:
	}
}

func (s *liveSet[T]) run() {
	defer close(s.done)
	defer close(s.changes)
	informerDone := make(chan struct{})
	syncContext, stopSync := context.WithCancel(s.ctx)
	go func() {
		defer close(informerDone)
		defer stopSync()
		s.informer.RunWithContext(s.ctx)
	}()
	if cache.WaitForCacheSync(syncContext.Done(), s.informer.HasSynced) {
		s.markReady()
	}
	<-informerDone
}

func (s *liveSet[T]) Changes() <-chan struct{} {
	return s.changes
}

func (s *liveSet[T]) State() LiveState[T] {
	s.statusMu.RLock()
	state := LiveState[T]{Ready: s.ready, Err: s.lastError}
	s.statusMu.RUnlock()
	objects := s.informer.GetStore().List()
	items := make([]keyedLiveObject[T], 0, len(objects))
	for _, object := range objects {
		key, err := cache.MetaNamespaceKeyFunc(object)
		if err != nil {
			state.Err = newError(OperationObserve, s.subject, "", err)
			return state
		}
		value, err := s.decode(object)
		if err != nil {
			state.Err = newError(OperationObserve, s.subject, "", err)
			return state
		}
		items = append(items, keyedLiveObject[T]{key: key, value: value})
	}
	slices.SortFunc(items, func(left, right keyedLiveObject[T]) int {
		return cmp.Compare(left.key, right.key)
	})
	state.Items = make([]T, 0, len(items))
	for _, item := range items {
		state.Items = append(state.Items, item.value)
	}
	return state
}

func (s *liveSet[T]) Stop() {
	s.stopOnce.Do(s.cancel)
}

func linkedContext(parent, lifetime context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	stopLifetimeLink := context.AfterFunc(lifetime, cancel)
	return ctx, func() {
		stopLifetimeLink()
		cancel()
	}
}
