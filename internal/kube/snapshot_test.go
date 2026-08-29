package kube

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/HediAbed/opsmate/failure"
)

type snapshotContextManager struct {
	name string
	err  error
}

func (*snapshotContextManager) Connect(context.Context, string) error {
	return nil
}

func (*snapshotContextManager) Contexts(context.Context) ([]ContextInfo, error) {
	return nil, nil
}

func (m *snapshotContextManager) CurrentContext(context.Context) (string, error) {
	return m.name, m.err
}

func (*snapshotContextManager) Namespaces(context.Context) ([]string, error) {
	return nil, nil
}

type snapshotResourceReader struct {
	ResourceReader
	pods        func(context.Context, string) ([]corev1.Pod, error)
	deployments func(context.Context, string) ([]appsv1.Deployment, error)
	services    func(context.Context, string) ([]corev1.Service, error)
	events      func(context.Context, string) ([]corev1.Event, error)
	nodes       func(context.Context) ([]corev1.Node, error)
}

func (r *snapshotResourceReader) ListPods(ctx context.Context, namespace string) ([]corev1.Pod, error) {
	return r.pods(ctx, namespace)
}

func (r *snapshotResourceReader) ListDeployments(ctx context.Context, namespace string) ([]appsv1.Deployment, error) {
	return r.deployments(ctx, namespace)
}

func (r *snapshotResourceReader) ListServices(ctx context.Context, namespace string) ([]corev1.Service, error) {
	return r.services(ctx, namespace)
}

func (r *snapshotResourceReader) ListEvents(ctx context.Context, namespace string) ([]corev1.Event, error) {
	return r.events(ctx, namespace)
}

func (r *snapshotResourceReader) ListNodes(ctx context.Context) ([]corev1.Node, error) {
	return r.nodes(ctx)
}

func TestNewSnapshotCollectorValidatesDependencies(t *testing.T) {
	contexts := &snapshotContextManager{}
	resources := successfulSnapshotReader()
	tests := []struct {
		name      string
		contexts  ContextManager
		resources ResourceReader
		cause     error
	}{
		{name: "missing context manager", resources: resources, cause: ErrContextManagerRequired},
		{name: "missing resource reader", contexts: contexts, cause: ErrResourceReaderRequired},
		{name: "valid", contexts: contexts, resources: resources},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			collector, err := NewSnapshotCollector(test.contexts, test.resources)
			if test.cause != nil {
				if collector != nil || !errors.Is(err, test.cause) {
					t.Fatalf("NewSnapshotCollector() = (%v, %v), want cause %v", collector, err, test.cause)
				}
				var typedError *Error
				if !errors.As(err, &typedError) || typedError.FailureCode() != failure.CodeInvalidArgument {
					t.Fatalf("NewSnapshotCollector() error = %v, want invalid argument", err)
				}
				return
			}
			if err != nil || collector == nil {
				t.Fatalf("NewSnapshotCollector() = (%v, %v), want collector", collector, err)
			}
		})
	}
}

func TestSnapshotCollectorCollectsBoundedTypedData(t *testing.T) {
	requestedNamespaces := make(chan string, 4)
	resources := successfulSnapshotReader()
	resources.pods = func(_ context.Context, namespace string) ([]corev1.Pod, error) {
		requestedNamespaces <- namespace
		pods := make([]corev1.Pod, maximumSnapshotItems+1)
		for index := range pods {
			pods[index].Name = fmt.Sprintf("pod-%03d", len(pods)-index)
			pods[index].Namespace = namespace
		}
		return pods, nil
	}
	resources.deployments = recordSnapshotNamespace(requestedNamespaces, []appsv1.Deployment{{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "team-a"},
	}})
	resources.services = recordSnapshotNamespace(requestedNamespaces, []corev1.Service{{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "team-a"},
	}})
	resources.events = recordSnapshotNamespace(requestedNamespaces, []corev1.Event{{
		ObjectMeta: metav1.ObjectMeta{Name: "started", Namespace: "team-a"},
	}})
	collector, err := NewSnapshotCollector(&snapshotContextManager{name: "work"}, resources)
	if err != nil {
		t.Fatalf("NewSnapshotCollector() error = %v", err)
	}

	snapshot, err := collector.Collect(context.Background(), "team-a")
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	requireSnapshotScopeAndTotals(t, snapshot)
	requireBoundedSnapshotPods(t, snapshot)
	close(requestedNamespaces)
	for namespace := range requestedNamespaces {
		if namespace != "team-a" {
			t.Fatalf("resource namespace = %q, want team-a", namespace)
		}
	}
}

func requireSnapshotScopeAndTotals(t *testing.T, snapshot ClusterSnapshot) {
	t.Helper()
	if snapshot.ContextName != "work" || snapshot.Namespace != "team-a" {
		t.Fatalf("Collect() scope = (%q, %q)", snapshot.ContextName, snapshot.Namespace)
	}
	if snapshot.Totals.Deployments != 1 || snapshot.Totals.Services != 1 || snapshot.Totals.Events != 1 || snapshot.Totals.Nodes != 0 {
		t.Fatalf("Collect() totals = %+v", snapshot.Totals)
	}
	if len(snapshot.Warnings) != 0 {
		t.Fatalf("Collect() warnings = %+v, want none", snapshot.Warnings)
	}
}

func requireBoundedSnapshotPods(t *testing.T, snapshot ClusterSnapshot) {
	t.Helper()
	if len(snapshot.Pods) != maximumSnapshotItems || snapshot.Totals.Pods != maximumSnapshotItems+1 {
		t.Fatalf("Collect() pods = %d/%d, want %d/%d", len(snapshot.Pods), snapshot.Totals.Pods, maximumSnapshotItems, maximumSnapshotItems+1)
	}
	if snapshot.Pods[0].Name != "pod-001" || snapshot.Pods[len(snapshot.Pods)-1].Name != "pod-100" {
		t.Fatalf("Collect() pod order = %q..%q", snapshot.Pods[0].Name, snapshot.Pods[len(snapshot.Pods)-1].Name)
	}
}

func TestSnapshotCollectorReturnsPartialDataWithOrderedWarnings(t *testing.T) {
	podsErr := errors.New("pods unavailable")
	eventsErr := errors.New("events unavailable")
	resources := successfulSnapshotReader()
	resources.pods = func(context.Context, string) ([]corev1.Pod, error) { return nil, podsErr }
	resources.events = func(context.Context, string) ([]corev1.Event, error) { return nil, eventsErr }
	collector, err := NewSnapshotCollector(
		&snapshotContextManager{err: errors.New("context unavailable")},
		resources,
	)
	if err != nil {
		t.Fatalf("NewSnapshotCollector() error = %v", err)
	}

	snapshot, err := collector.Collect(context.Background(), "team-a")
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	wantSections := []SnapshotSection{SnapshotContext, SnapshotPods, SnapshotEvents}
	sections := make([]SnapshotSection, 0, len(snapshot.Warnings))
	for _, warning := range snapshot.Warnings {
		sections = append(sections, warning.Section)
	}
	if !slices.Equal(sections, wantSections) {
		t.Fatalf("warning sections = %v, want %v", sections, wantSections)
	}
	if !errors.Is(snapshot.Warnings[1].Err, podsErr) || !errors.Is(snapshot.Warnings[2].Err, eventsErr) {
		t.Fatalf("warnings = %+v, want original causes", snapshot.Warnings)
	}
}

func TestSnapshotCollectorRejectsUnavailableOrCanceledReads(t *testing.T) {
	sentinel := errors.New("read failed")
	failed := successfulSnapshotReader()
	failed.pods = failingSnapshotRead[corev1.Pod](sentinel)
	failed.deployments = failingSnapshotRead[appsv1.Deployment](sentinel)
	failed.services = failingSnapshotRead[corev1.Service](sentinel)
	failed.events = failingSnapshotRead[corev1.Event](sentinel)
	failed.nodes = func(context.Context) ([]corev1.Node, error) { return nil, sentinel }
	collector, err := NewSnapshotCollector(&snapshotContextManager{name: "work"}, failed)
	if err != nil {
		t.Fatalf("NewSnapshotCollector() error = %v", err)
	}
	if _, collectErr := collector.Collect(context.Background(), "team-a"); !errors.Is(collectErr, sentinel) {
		t.Fatalf("Collect() error = %v, want read cause", collectErr)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, collectErr := collector.Collect(canceled, "team-a"); !errors.Is(collectErr, context.Canceled) {
		t.Fatalf("Collect(canceled) error = %v", collectErr)
	}
	var missingContext context.Context
	if _, collectErr := collector.Collect(missingContext, "team-a"); !errors.Is(collectErr, ErrContextRequired) {
		t.Fatalf("Collect(nil) error = %v", collectErr)
	}
	var missing *SnapshotCollector
	if _, collectErr := missing.Collect(context.Background(), "team-a"); !errors.Is(collectErr, ErrSnapshotCollectorRequired) {
		t.Fatalf("nil Collect() error = %v", collectErr)
	}
}

func TestSnapshotCollectorObservesCancellationDuringReads(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	resources := successfulSnapshotReader()
	resources.services = func(context.Context, string) ([]corev1.Service, error) {
		cancel()
		return nil, context.Canceled
	}
	collector, err := NewSnapshotCollector(&snapshotContextManager{name: "work"}, resources)
	if err != nil {
		t.Fatalf("NewSnapshotCollector() error = %v", err)
	}
	if _, collectErr := collector.Collect(ctx, "team-a"); !errors.Is(collectErr, context.Canceled) {
		t.Fatalf("Collect() error = %v, want cancellation", collectErr)
	}
}

func TestSnapshotPodProjectionPreservesDiagnosticState(t *testing.T) {
	deletionTime := metav1.Now()
	pods := projectSnapshotPods([]corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "terminating", DeletionTimestamp: &deletionTime}},
		{ObjectMeta: metav1.ObjectMeta{Name: "init"}, Status: corev1.PodStatus{InitContainerStatuses: []corev1.ContainerStatus{{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"}}}}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "waiting"}, Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{Ready: true, RestartCount: 3, State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}}}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "unknown"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "running"}, Status: corev1.PodStatus{Phase: corev1.PodRunning}},
	})
	statusByName := make(map[string]string, len(pods))
	for _, pod := range pods {
		statusByName[pod.Name] = pod.Status
	}
	wantStatuses := map[string]string{
		"terminating": "Terminating",
		"init":        "Init:ImagePullBackOff",
		"waiting":     "CrashLoopBackOff",
		"unknown":     unknownSnapshotState,
		"running":     string(corev1.PodRunning),
	}
	if !mapsEqual(statusByName, wantStatuses) {
		t.Fatalf("pod statuses = %v, want %v", statusByName, wantStatuses)
	}
	if pods[3].Name != "unknown" || pods[3].Ready != 0 {
		t.Fatalf("sorted pod projection = %+v", pods)
	}
	waiting := pods[4]
	if waiting.Ready != 1 || waiting.Desired != 1 || waiting.Restarts != 3 {
		t.Fatalf("waiting pod state = %+v", waiting)
	}
}

func TestSnapshotDeploymentProjectionDefaultsReplicas(t *testing.T) {
	replicas := int32(4)
	deployments := projectSnapshotDeployments([]appsv1.Deployment{
		{ObjectMeta: metav1.ObjectMeta{Name: "explicit"}, Spec: appsv1.DeploymentSpec{Replicas: &replicas}},
		{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
	})
	if deployments[0].Desired != defaultReplicaCount || deployments[1].Desired != replicas {
		t.Fatalf("deployment replicas = %+v", deployments)
	}
}

func TestSnapshotServiceProjectionKeepsSortedPorts(t *testing.T) {
	services := projectSnapshotServices([]corev1.Service{
		{ObjectMeta: metav1.ObjectMeta{Name: "worker", Namespace: "team-b"}},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "team-a"},
			Spec: corev1.ServiceSpec{
				Type:      corev1.ServiceTypeClusterIP,
				ClusterIP: "10.0.0.8",
				Ports:     []corev1.ServicePort{{Name: "http", Protocol: corev1.ProtocolTCP, Port: 8080}},
			},
		},
	})
	if len(services) != 2 || len(services[0].Ports) != 1 || services[0].Ports[0].Port != 8080 {
		t.Fatalf("service projection = %+v", services)
	}
}

func TestSnapshotNodeProjectionReportsReadiness(t *testing.T) {
	nodes := projectSnapshotNodes([]corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "ready"}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "unknown"}},
	})
	if !nodes[0].Ready || nodes[1].Ready {
		t.Fatalf("node readiness = %+v", nodes)
	}
}

func TestSnapshotEventsUseBestTimestampAndNewestLimit(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	events := make([]corev1.Event, maximumSnapshotEvents+4)
	for index := range events {
		events[index] = corev1.Event{
			ObjectMeta: metav1.ObjectMeta{
				Name:              fmt.Sprintf("event-%02d", index),
				CreationTimestamp: metav1.NewTime(base.Add(time.Duration(index) * time.Minute)),
			},
		}
	}
	events[0].EventTime = metav1.NewMicroTime(base.Add(4 * time.Hour))
	events[1].Series = &corev1.EventSeries{LastObservedTime: metav1.NewMicroTime(base.Add(3 * time.Hour))}
	events[2].LastTimestamp = metav1.NewTime(base.Add(2 * time.Hour))

	projected := projectSnapshotEvents(events)
	if len(projected) != maximumSnapshotEvents {
		t.Fatalf("event count = %d, want %d", len(projected), maximumSnapshotEvents)
	}
	wantFirst := []string{"event-00", "event-01", "event-02"}
	for index, name := range wantFirst {
		if projected[index].Object != "/" || projected[index].LastSeen.IsZero() {
			t.Fatalf("event projection %d = %+v", index, projected[index])
		}
		if eventsByTimestampName(events, projected[index].LastSeen) != name {
			t.Fatalf("event %d timestamp belongs to %q, want %q", index, eventsByTimestampName(events, projected[index].LastSeen), name)
		}
	}
}

func successfulSnapshotReader() *snapshotResourceReader {
	return &snapshotResourceReader{
		pods:        func(context.Context, string) ([]corev1.Pod, error) { return nil, nil },
		deployments: func(context.Context, string) ([]appsv1.Deployment, error) { return nil, nil },
		services:    func(context.Context, string) ([]corev1.Service, error) { return nil, nil },
		events:      func(context.Context, string) ([]corev1.Event, error) { return nil, nil },
		nodes:       func(context.Context) ([]corev1.Node, error) { return nil, nil },
	}
}

func recordSnapshotNamespace[T any](namespaces chan<- string, items []T) func(context.Context, string) ([]T, error) {
	return func(_ context.Context, namespace string) ([]T, error) {
		namespaces <- namespace
		return items, nil
	}
}

func failingSnapshotRead[T any](err error) func(context.Context, string) ([]T, error) {
	return func(context.Context, string) ([]T, error) {
		return nil, err
	}
}

func mapsEqual[K comparable, V comparable](left, right map[K]V) bool {
	if len(left) != len(right) {
		return false
	}
	for key, leftValue := range left {
		if right[key] != leftValue {
			return false
		}
	}
	return true
}

func eventsByTimestampName(events []corev1.Event, timestamp time.Time) string {
	for _, event := range events {
		if snapshotEventTime(event).Equal(timestamp) {
			return event.Name
		}
	}
	return ""
}
