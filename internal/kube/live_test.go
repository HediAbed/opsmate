package kube

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apiextensionsfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8swatch "k8s.io/apimachinery/pkg/watch"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	cachetesting "k8s.io/client-go/tools/cache/testing"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	metricsfake "k8s.io/metrics/pkg/client/clientset/versioned/fake"

	"github.com/HediAbed/opsmate/failure"
)

const liveSetTestTimeout = 10 * time.Second

type liveSetProbe struct {
	changes <-chan struct{}
	state   func() (bool, int, error)
	stop    func()
}

type fakeLiveInformer struct {
	store           cache.Store
	handler         cache.ResourceEventHandler
	watchError      cache.WatchErrorHandlerWithContext
	transform       cache.TransformFunc
	transformErr    error
	watchHandlerErr error
	eventHandlerErr error
	synced          atomic.Bool
	stopRelease     <-chan struct{}
}

func (f *fakeLiveInformer) AddEventHandler(handler cache.ResourceEventHandler) (cache.ResourceEventHandlerRegistration, error) {
	f.handler = handler
	return nil, f.eventHandlerErr
}

func (f *fakeLiveInformer) GetStore() cache.Store {
	if f.store == nil {
		f.store = cache.NewStore(cache.MetaNamespaceKeyFunc)
	}
	return f.store
}

func (f *fakeLiveInformer) HasSynced() bool {
	return f.synced.Load()
}

func (f *fakeLiveInformer) RunWithContext(ctx context.Context) {
	<-ctx.Done()
	if f.stopRelease != nil {
		<-f.stopRelease
	}
}

func (f *fakeLiveInformer) SetTransform(transform cache.TransformFunc) error {
	f.transform = transform
	return f.transformErr
}

func (f *fakeLiveInformer) SetWatchErrorHandlerWithContext(handler cache.WatchErrorHandlerWithContext) error {
	f.watchError = handler
	return f.watchHandlerErr
}

func TestLiveSetConvergesFromListAndWatch(t *testing.T) {
	source := cachetesting.NewFakeControllerSource()
	alpha := livePod("alpha", "Pending")
	zeta := livePod("zeta", "Running")
	source.Add(zeta)
	source.Add(alpha)

	observed, err := newLiveSet(
		context.Background(),
		SubjectPods,
		source,
		&corev1.Pod{},
		decodeLivePod,
		stripManagedFields,
	)
	if err != nil {
		t.Fatalf("newLiveSet() error = %v", err)
	}
	defer observed.Stop()

	state := waitForLiveState(t, observed, func(state LiveState[corev1.Pod]) bool {
		return state.Ready && len(state.Items) == 2
	})
	if state.Err != nil || state.Items[0].Name != "alpha" || state.Items[1].Name != "zeta" {
		t.Fatalf("initial state = %+v, want sorted pods", state)
	}
	state.Items[0].Labels["mutable"] = "changed"
	if _, found := observed.State().Items[0].Labels["mutable"]; found {
		t.Fatal("State() exposed the informer cache for mutation")
	}

	updated := livePod("alpha", "Running")
	source.Modify(updated)
	state = waitForLiveState(t, observed, func(state LiveState[corev1.Pod]) bool {
		return len(state.Items) == 2 && state.Items[0].Status.Phase == corev1.PodRunning
	})
	if state.Err != nil {
		t.Fatalf("updated state error = %v", state.Err)
	}

	source.Delete(zeta)
	state = waitForLiveState(t, observed, func(state LiveState[corev1.Pod]) bool {
		return len(state.Items) == 1
	})
	if state.Items[0].Name != "alpha" {
		t.Fatalf("state after delete = %+v, want alpha only", state.Items)
	}
}

func TestLiveSetRelistsAfterAWatchGap(t *testing.T) {
	source := cachetesting.NewFakeControllerSource()
	removed := livePod("removed", "Running")
	remaining := livePod("remaining", "Running")
	source.Add(removed)
	source.Add(remaining)

	observed, err := newLiveSet(
		context.Background(),
		SubjectPods,
		source,
		&corev1.Pod{},
		decodeLivePod,
		stripManagedFields,
	)
	if err != nil {
		t.Fatalf("newLiveSet() error = %v", err)
	}
	defer observed.Stop()
	waitForLiveState(t, observed, func(state LiveState[corev1.Pod]) bool {
		return state.Ready && len(state.Items) == 2
	})

	source.DeleteDropWatch(removed)
	source.ResetWatch()
	state := waitForLiveState(t, observed, func(state LiveState[corev1.Pod]) bool {
		return state.Ready && len(state.Items) == 1 && state.Items[0].Name == "remaining"
	})
	if state.Err != nil {
		t.Fatalf("recovered state error = %v", state.Err)
	}
}

func TestLiveSetCoalescesNotificationsAndTracksErrors(t *testing.T) {
	source := cachetesting.NewFakeControllerSource()
	observed, err := newLiveSet(
		context.Background(),
		SubjectPods,
		source,
		&corev1.Pod{},
		decodeLivePod,
		stripManagedFields,
	)
	if err != nil {
		t.Fatalf("newLiveSet() error = %v", err)
	}
	concrete := observed.(*liveSet[corev1.Pod])
	waitForLiveState(t, observed, func(state LiveState[corev1.Pod]) bool { return state.Ready })

	sentinel := errors.New("connection interrupted")
	concrete.handleWatchError(context.Background(), nil, sentinel)
	receiveLiveChange(t, observed.Changes())
	state := observed.State()
	if !errors.Is(state.Err, sentinel) || failure.CodeOf(state.Err) != failure.CodeUnknown {
		t.Fatalf("live state error = %v, want typed sentinel", state.Err)
	}
	concrete.recordConnectionSuccess()
	receiveLiveChange(t, observed.Changes())
	if state := observed.State(); state.Err != nil {
		t.Fatalf("state error after reconnect = %v, want nil", state.Err)
	}
	concrete.handleWatchError(context.Background(), nil, sentinel)
	receiveLiveChange(t, observed.Changes())
	handler := concrete.eventHandler()
	handler.OnAdd(&corev1.Pod{}, true)
	handler.OnAdd(&corev1.Pod{}, false)
	handler.OnUpdate(&corev1.Pod{}, &corev1.Pod{})
	handler.OnDelete(&corev1.Pod{})
	if pending := len(observed.Changes()); pending != 1 {
		t.Fatalf("pending change notifications = %d, want one coalesced notification", pending)
	}
	receiveLiveChange(t, observed.Changes())
	if state := observed.State(); state.Err != nil {
		t.Fatalf("state error after recovery = %v, want nil", state.Err)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	concrete.handleWatchError(cancelled, nil, sentinel)
	if state := observed.State(); state.Err != nil {
		t.Fatalf("canceled error handler changed state: %v", state.Err)
	}
	observed.Stop()
	concrete.recordConnectionSuccess()
	concrete.recordChange()
	concrete.notify()
	observed.Stop()
	assertChangesClosed(t, observed.Changes())
}

func TestConnectionTrackingListerWatcherReportsSuccessfulConnections(t *testing.T) {
	sentinel := errors.New("connection failed")
	listFails := true
	watchFails := true
	stream := k8swatch.NewFake()
	defer stream.Stop()
	delegate := &cache.ListWatch{
		ListWithContextFunc: func(context.Context, metav1.ListOptions) (runtime.Object, error) {
			if listFails {
				return nil, sentinel
			}
			return &corev1.PodList{}, nil
		},
		WatchFuncWithContext: func(context.Context, metav1.ListOptions) (k8swatch.Interface, error) {
			if watchFails {
				return nil, sentinel
			}
			return stream, nil
		},
	}
	var successfulConnections atomic.Int32
	tracked := newConnectionTrackingListerWatcher(delegate, func() {
		successfulConnections.Add(1)
	})

	if _, err := tracked.List(metav1.ListOptions{}); !errors.Is(err, sentinel) {
		t.Fatalf("failed List() error = %v", err)
	}
	listFails = false
	if _, err := tracked.ListWithContext(context.Background(), metav1.ListOptions{}); err != nil {
		t.Fatalf("successful ListWithContext() error = %v", err)
	}
	if _, err := tracked.WatchWithContext(context.Background(), metav1.ListOptions{}); !errors.Is(err, sentinel) {
		t.Fatalf("failed WatchWithContext() error = %v", err)
	}
	watchFails = false
	if _, err := tracked.Watch(metav1.ListOptions{}); err != nil {
		t.Fatalf("successful Watch() error = %v", err)
	}
	if got := successfulConnections.Load(); got != 2 {
		t.Fatalf("successful connection callbacks = %d, want 2", got)
	}
}

func TestEveryManagerObserverProducesAReadySnapshot(t *testing.T) {
	metadata := metav1.ObjectMeta{Name: "sample", Namespace: testNamespace}
	typedClient := kubernetesfake.NewSimpleClientset(
		&corev1.Pod{ObjectMeta: metadata},
		&appsv1.Deployment{ObjectMeta: metadata},
		&corev1.Event{ObjectMeta: metadata},
		&networkingv1.Ingress{ObjectMeta: metadata},
		&networkingv1.NetworkPolicy{ObjectMeta: metadata},
		&corev1.PersistentVolumeClaim{ObjectMeta: metadata},
		&batchv1.CronJob{ObjectMeta: metadata},
		&autoscalingv2.HorizontalPodAutoscaler{ObjectMeta: metadata},
		&appsv1.ReplicaSet{ObjectMeta: metadata},
	)
	secretClient := newMetadataClient(t, &metav1.PartialObjectMetadata{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{
			Name:        "sample",
			Namespace:   testNamespace,
			Annotations: map[string]string{"private": "not retained"},
		},
	})
	manager := resourceManager(
		typedClient,
		metricsfake.NewSimpleClientset(),
		apiextensionsfake.NewSimpleClientset(),
		secretClient,
	)

	tests := []struct {
		name  string
		start func() (liveSetProbe, error)
	}{
		{name: "pods", start: observerProbeStarter(manager.ObservePods)},
		{name: "deployments", start: observerProbeStarter(manager.ObserveDeployments)},
		{name: "events", start: observerProbeStarter(manager.ObserveEvents)},
		{name: "ingresses", start: observerProbeStarter(manager.ObserveIngresses)},
		{name: "network policies", start: observerProbeStarter(manager.ObserveNetworkPolicies)},
		{name: "persistent volume claims", start: observerProbeStarter(manager.ObservePersistentVolumeClaims)},
		{name: "cron jobs", start: observerProbeStarter(manager.ObserveCronJobs)},
		{name: "horizontal pod autoscalers", start: observerProbeStarter(manager.ObserveHorizontalPodAutoscalers)},
		{name: "secrets", start: observerProbeStarter(manager.ObserveSecrets)},
		{name: "replica sets", start: observerProbeStarter(manager.ObserveReplicaSets)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			probe, err := test.start()
			if err != nil {
				t.Fatalf("observer error = %v", err)
			}
			defer probe.stop()
			receiveLiveChange(t, probe.changes)
			ready, count, stateErr := probe.state()
			if !ready || count != 1 || stateErr != nil {
				t.Fatalf("observer state = (ready %t, count %d, error %v)", ready, count, stateErr)
			}
		})
	}
}

func TestSecretObserverDoesNotRetainAnnotations(t *testing.T) {
	metadataClient := newMetadataClient(t, &metav1.PartialObjectMetadata{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{
			Name:        "credentials",
			Namespace:   testNamespace,
			Annotations: map[string]string{"private": "payload"},
		},
	})
	manager := resourceManager(
		kubernetesfake.NewSimpleClientset(),
		metricsfake.NewSimpleClientset(),
		apiextensionsfake.NewSimpleClientset(),
		metadataClient,
	)
	observed, err := manager.ObserveSecrets(context.Background(), testNamespace)
	if err != nil {
		t.Fatalf("ObserveSecrets() error = %v", err)
	}
	defer observed.Stop()
	waitForLiveState(t, observed, func(state LiveState[ResourceMetadata]) bool { return state.Ready })
	stored := observed.(*liveSet[ResourceMetadata]).informer.GetStore().List()
	if len(stored) != 1 {
		t.Fatalf("stored secret count = %d, want one", len(stored))
	}
	secret := stored[0].(*metav1.PartialObjectMetadata)
	if secret.Annotations != nil {
		t.Fatalf("stored secret annotations = %v, want nil", secret.Annotations)
	}
}

func observerProbeStarter[T any](observe func(context.Context, string) (LiveSet[T], error)) func() (liveSetProbe, error) {
	return func() (liveSetProbe, error) {
		set, err := observe(context.Background(), testNamespace)
		return probeLiveSet(set, err), err
	}
}

func TestObserverValidatesContextAndConnection(t *testing.T) {
	manager := &Manager{}
	var missingContext context.Context
	if observed, err := manager.ObservePods(missingContext, testNamespace); observed != nil || !errors.Is(err, ErrContextRequired) {
		t.Fatalf("ObservePods(nil) = (%v, %v)", observed, err)
	}
	if observed, err := manager.ObservePods(context.Background(), testNamespace); observed != nil || !errors.Is(err, ErrClientUnavailable) {
		t.Fatalf("ObservePods(disconnected) = (%v, %v)", observed, err)
	}
}

func TestManagerContextSwitchStopsExistingLiveSets(t *testing.T) {
	config := clientcmdConfigWithContexts("primary", "primary", "secondary")
	source := &fakeConfigSource{rawConfig: config, rest: testRESTConfig()}
	typedClient := kubernetesfake.NewSimpleClientset()
	clients := healthyTestClients()
	clients.kubernetes = typedClient
	manager, err := NewManager(source, &fakeClientBuilder{clients: clients})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	if err := manager.Connect(context.Background(), "primary"); err != nil {
		t.Fatalf("Connect(primary) error = %v", err)
	}
	observed, err := manager.ObservePods(context.Background(), testNamespace)
	if err != nil {
		t.Fatalf("ObservePods() error = %v", err)
	}
	waitForLiveState(t, observed, func(state LiveState[corev1.Pod]) bool { return state.Ready })
	if err := manager.Connect(context.Background(), "secondary"); err != nil {
		t.Fatalf("Connect(secondary) error = %v", err)
	}
	assertChangesClosed(t, observed.Changes())
	observed.Stop()
}

func TestNewLiveSetRequiresUsableContext(t *testing.T) {
	source := cachetesting.NewFakeControllerSource()
	validPrototype := runtime.Object(&corev1.Pod{})
	validDecoder := liveObjectDecoder[corev1.Pod](decodeLivePod)
	var missingContext context.Context
	if set, err := newLiveSet(missingContext, SubjectPods, source, validPrototype, validDecoder, stripManagedFields); set != nil || !errors.Is(err, ErrContextRequired) {
		t.Fatalf("newLiveSet(nil context) = (%v, %v)", set, err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if set, err := newLiveSet(cancelled, SubjectPods, source, validPrototype, validDecoder, stripManagedFields); set != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("newLiveSet(canceled) = (%v, %v)", set, err)
	}
}

func TestNewLiveSetValidatesInputs(t *testing.T) {
	source := cachetesting.NewFakeControllerSource()
	validPrototype := runtime.Object(&corev1.Pod{})
	validDecoder := liveObjectDecoder[corev1.Pod](decodeLivePod)
	tests := []struct {
		name      string
		watcher   cache.ListerWatcher
		prototype runtime.Object
		decoder   liveObjectDecoder[corev1.Pod]
		transform cache.TransformFunc
		want      error
	}{
		{name: "list watcher", prototype: validPrototype, decoder: validDecoder, transform: stripManagedFields, want: ErrListWatcherRequired},
		{name: "prototype", watcher: source, decoder: validDecoder, transform: stripManagedFields, want: ErrResourcePrototypeRequired},
		{name: "decoder", watcher: source, prototype: validPrototype, transform: stripManagedFields, want: ErrResourceDecoderRequired},
		{name: "transform", watcher: source, prototype: validPrototype, decoder: validDecoder, want: ErrResourceTransformRequired},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			set, err := newLiveSet(context.Background(), SubjectPods, test.watcher, test.prototype, test.decoder, test.transform)
			if set != nil || !errors.Is(err, test.want) {
				t.Fatalf("newLiveSet() = (%v, %v), want %v", set, err, test.want)
			}
		})
	}
}

func TestStartLiveSetReportsConfigurationFailures(t *testing.T) {
	sentinel := errors.New("configuration failed")
	configurationTests := []struct {
		name     string
		informer liveInformer
		want     error
	}{
		{name: "missing informer", want: ErrInformerRequired},
		{name: "transform", informer: &fakeLiveInformer{transformErr: sentinel}, want: sentinel},
		{name: "watch error handler", informer: &fakeLiveInformer{watchHandlerErr: sentinel}, want: sentinel},
		{name: "event handler", informer: &fakeLiveInformer{eventHandlerErr: sentinel}, want: sentinel},
	}
	for _, test := range configurationTests {
		t.Run(test.name, func(t *testing.T) {
			ctx, stop := context.WithCancel(context.Background())
			set, err := startLiveSet(ctx, stop, SubjectPods, test.informer, decodeLivePod, stripManagedFields)
			if set != nil || !errors.Is(err, test.want) {
				t.Fatalf("startLiveSet() = (%v, %v), want %v", set, err, test.want)
			}
			select {
			case <-ctx.Done():
			default:
				t.Fatal("failed live-set setup did not cancel its context")
			}
		})
	}
}

func TestLiveSetStopsBeforeInitialSync(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	informer := &fakeLiveInformer{}
	observed, err := startLiveSet(ctx, cancel, SubjectPods, informer, decodeLivePod, stripManagedFields)
	if err != nil {
		t.Fatalf("startLiveSet() error = %v", err)
	}
	observed.Stop()
	state := observed.State()
	if state.Ready || state.Err != nil || state.Items == nil || len(state.Items) != 0 {
		t.Fatalf("stopped unsynced state = %+v", state)
	}
	assertChangesClosed(t, observed.Changes())
}

func TestLiveSetStopDoesNotWaitForInformerShutdown(t *testing.T) {
	release := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	informer := &fakeLiveInformer{stopRelease: release}
	observed, err := startLiveSet(ctx, cancel, SubjectPods, informer, decodeLivePod, stripManagedFields)
	if err != nil {
		t.Fatalf("startLiveSet() error = %v", err)
	}
	returned := make(chan struct{})
	go func() {
		observed.Stop()
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("Stop() waited for informer shutdown")
	}
	select {
	case _, open := <-observed.Changes():
		if !open {
			t.Fatal("change channel closed before informer shutdown")
		}
	default:
	}
	close(release)
	assertChangesClosed(t, observed.Changes())
}

func TestLiveSetStateReportsInvalidStoreObjects(t *testing.T) {
	keyStore := cache.NewStore(func(any) (string, error) { return "stored", nil })
	if err := keyStore.Add("not a resource"); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	set := &liveSet[corev1.Pod]{informer: &fakeLiveInformer{store: keyStore}, subject: SubjectPods, decode: decodeLivePod}
	if state := set.State(); state.Err == nil {
		t.Fatal("State() accepted an object without Kubernetes metadata")
	}

	objectStore := cache.NewStore(cache.MetaNamespaceKeyFunc)
	if err := objectStore.Add(&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "wrong"}}); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	set.informer = &fakeLiveInformer{store: objectStore}
	if state := set.State(); !errors.Is(state.Err, ErrUnexpectedResourceObject) {
		t.Fatalf("State() error = %v, want unexpected resource object", state.Err)
	}
}

func TestLiveObjectDecodersRejectWrongAndNilObjects(t *testing.T) {
	var nilPod *corev1.Pod
	var nilDeployment *appsv1.Deployment
	var nilEvent *corev1.Event
	var nilIngress *networkingv1.Ingress
	var nilNetworkPolicy *networkingv1.NetworkPolicy
	var nilClaim *corev1.PersistentVolumeClaim
	var nilCronJob *batchv1.CronJob
	var nilAutoscaler *autoscalingv2.HorizontalPodAutoscaler
	var nilReplicaSet *appsv1.ReplicaSet
	var nilMetadata *metav1.PartialObjectMetadata
	tests := []struct {
		name  string
		wrong func() error
		nil   func() error
	}{
		{name: "pod", wrong: func() error { _, err := decodeLivePod(&corev1.Service{}); return err }, nil: func() error { _, err := decodeLivePod(nilPod); return err }},
		{name: "deployment", wrong: func() error { _, err := decodeLiveDeployment(&corev1.Service{}); return err }, nil: func() error { _, err := decodeLiveDeployment(nilDeployment); return err }},
		{name: "event", wrong: func() error { _, err := decodeLiveEvent(&corev1.Service{}); return err }, nil: func() error { _, err := decodeLiveEvent(nilEvent); return err }},
		{name: "ingress", wrong: func() error { _, err := decodeLiveIngress(&corev1.Service{}); return err }, nil: func() error { _, err := decodeLiveIngress(nilIngress); return err }},
		{name: "network policy", wrong: func() error { _, err := decodeLiveNetworkPolicy(&corev1.Service{}); return err }, nil: func() error { _, err := decodeLiveNetworkPolicy(nilNetworkPolicy); return err }},
		{name: "persistent volume claim", wrong: func() error { _, err := decodeLivePersistentVolumeClaim(&corev1.Service{}); return err }, nil: func() error { _, err := decodeLivePersistentVolumeClaim(nilClaim); return err }},
		{name: "cron job", wrong: func() error { _, err := decodeLiveCronJob(&corev1.Service{}); return err }, nil: func() error { _, err := decodeLiveCronJob(nilCronJob); return err }},
		{name: "horizontal pod autoscaler", wrong: func() error { _, err := decodeLiveHorizontalPodAutoscaler(&corev1.Service{}); return err }, nil: func() error { _, err := decodeLiveHorizontalPodAutoscaler(nilAutoscaler); return err }},
		{name: "replica set", wrong: func() error { _, err := decodeLiveReplicaSet(&corev1.Service{}); return err }, nil: func() error { _, err := decodeLiveReplicaSet(nilReplicaSet); return err }},
		{name: "metadata", wrong: func() error { _, err := decodeLiveMetadata(&corev1.Service{}); return err }, nil: func() error { _, err := decodeLiveMetadata(nilMetadata); return err }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.wrong(); !errors.Is(err, ErrUnexpectedResourceObject) {
				t.Fatalf("wrong object error = %v", err)
			}
			if err := test.nil(); !errors.Is(err, ErrUnexpectedResourceObject) {
				t.Fatalf("nil object error = %v", err)
			}
		})
	}
}

func TestLiveSetMetadataTransforms(t *testing.T) {
	managedFields := []metav1.ManagedFieldsEntry{{Manager: "controller"}}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{ManagedFields: managedFields}}
	transformed, err := stripManagedFields(pod)
	if err != nil || transformed != pod || pod.ManagedFields != nil {
		t.Fatalf("stripManagedFields() = (%v, %v), managed fields %v", transformed, err, pod.ManagedFields)
	}
	secret := &metav1.PartialObjectMetadata{ObjectMeta: metav1.ObjectMeta{
		ManagedFields: managedFields,
		Annotations:   map[string]string{"private": "payload"},
	}}
	transformed, err = stripSensitiveMetadata(secret)
	if err != nil || transformed != secret || secret.ManagedFields != nil || secret.Annotations != nil {
		t.Fatalf("stripSensitiveMetadata() = (%v, %v), metadata %+v", transformed, err, secret.ObjectMeta)
	}
	if _, err := stripManagedFields("invalid"); err == nil {
		t.Fatal("stripManagedFields() accepted invalid object")
	}
	if _, err := stripSensitiveMetadata("invalid"); err == nil {
		t.Fatal("stripSensitiveMetadata() accepted invalid object")
	}
}

func livePod(name string, phase corev1.PodPhase) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
			UID:       types.UID(name),
			Labels:    map[string]string{"app": name},
		},
		Status: corev1.PodStatus{Phase: phase},
	}
}

func waitForLiveState[T any](t *testing.T, set LiveSet[T], ready func(LiveState[T]) bool) LiveState[T] {
	t.Helper()
	deadline := time.NewTimer(liveSetTestTimeout)
	defer deadline.Stop()
	for {
		state := set.State()
		if ready(state) {
			return state
		}
		select {
		case _, open := <-set.Changes():
			if !open {
				t.Fatalf("live set closed before reaching expected state: %+v", state)
			}
		case <-deadline.C:
			t.Fatalf("timed out waiting for live state: %+v", state)
		}
	}
}

func receiveLiveChange(t *testing.T, changes <-chan struct{}) {
	t.Helper()
	select {
	case _, open := <-changes:
		if !open {
			t.Fatal("live-set change channel closed unexpectedly")
		}
	case <-time.After(liveSetTestTimeout):
		t.Fatal("timed out waiting for live-set change")
	}
}

func assertChangesClosed(t *testing.T, changes <-chan struct{}) {
	t.Helper()
	deadline := time.NewTimer(liveSetTestTimeout)
	defer deadline.Stop()
	for {
		select {
		case _, open := <-changes:
			if !open {
				return
			}
		case <-deadline.C:
			t.Fatal("timed out waiting for live-set change channel to close")
		}
	}
}

func probeLiveSet[T any](set LiveSet[T], err error) liveSetProbe {
	if err != nil || set == nil {
		return liveSetProbe{}
	}
	return liveSetProbe{
		changes: set.Changes(),
		state: func() (bool, int, error) {
			state := set.State()
			return state.Ready, len(state.Items), state.Err
		},
		stop: set.Stop,
	}
}

func clientcmdConfigWithContexts(current string, names ...string) clientcmdapi.Config {
	contexts := make(map[string]*clientcmdapi.Context, len(names))
	for _, name := range names {
		contexts[name] = &clientcmdapi.Context{Cluster: name}
	}
	return clientcmdapi.Config{CurrentContext: current, Contexts: contexts}
}

func testRESTConfig() *rest.Config {
	return &rest.Config{Host: "https://cluster.invalid"}
}
