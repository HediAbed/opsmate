package cluster

import (
	"context"
	"errors"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	model "github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/kube"
)

func projectedPodLiveSetForTest(t *testing.T) LiveSet[model.Pod] {
	t.Helper()
	now := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	source := newTestKubeLiveSet([]corev1.Pod{{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "api",
			Namespace:         "payments",
			CreationTimestamp: metav1.NewTime(now.Add(-time.Hour)),
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}}, nil)
	commands := ResourceCommands{now: func() time.Time { return now }}
	projected, err := observeProjectedResource(commands, func() (kube.LiveSet[corev1.Pod], error) {
		return source, nil
	}, projectPods)
	if err != nil {
		t.Fatalf("observeProjectedResource() error = %v", err)
	}
	return projected
}

func TestProjectedLiveSetForwardsChangesAndState(t *testing.T) {
	projected := projectedPodLiveSetForTest(t)
	defer projected.Stop()
	select {
	case <-projected.Changes():
	default:
		t.Fatal("projected live set did not forward the change signal")
	}
	state := projected.State()
	if !state.Ready || state.Err != nil || len(state.Items) != 1 ||
		state.Items[0].Name != "api" || state.Items[0].Status != "Running" || state.Items[0].Age != "1h" {
		t.Fatalf("projected state = %+v", state)
	}
}

func TestProjectedLiveSetForwardsStop(t *testing.T) {
	projected := projectedPodLiveSetForTest(t)
	select {
	case <-projected.Changes():
	default:
		t.Fatal("projected live set did not forward the initial change signal")
	}
	projected.Stop()
	select {
	case _, open := <-projected.Changes():
		if open {
			t.Fatal("projected live set remained open after Stop()")
		}
	default:
		t.Fatal("projected live set did not forward Stop()")
	}
}

func TestProjectedLiveSetReturnsObservationError(t *testing.T) {
	want := errors.New("observation failed")
	commands := ResourceCommands{now: time.Now}
	projected, err := observeProjectedResource(commands, func() (kube.LiveSet[corev1.Pod], error) {
		return nil, want
	}, projectPods)
	if projected != nil || !errors.Is(err, want) {
		t.Fatalf("observeProjectedResource() = set:%v error:%v", projected, err)
	}
}

func TestNativeClusterObservationPropagatesObserverErrors(t *testing.T) {
	want := errors.New("observer failed")
	observer := &testResourceObserver{err: want}
	commands := NewCommands(context.Background(), &testResourceReader{}, observer)
	set, err := commands.ObservePods("payments")
	if set != nil || !errors.Is(err, want) {
		t.Fatalf("ObservePods() = set:%v error:%v", set, err)
	}
}

type observedLiveState struct {
	itemCount int
	ready     bool
	err       error
}

func summarizeLiveSet[T interface{}](set LiveSet[T], err error) observedLiveState {
	if err != nil {
		return observedLiveState{err: err}
	}
	state := set.State()
	set.Stop()
	return observedLiveState{itemCount: len(state.Items), ready: state.Ready, err: state.Err}
}

type observationAdapterCase struct {
	name    string
	observe func(ResourceCommands) observedLiveState
}

func observationAdapterCases() []observationAdapterCase {
	const namespace = "payments"
	return []observationAdapterCase{
		{name: "pods", observe: func(commands ResourceCommands) observedLiveState {
			return summarizeLiveSet(commands.ObservePods(namespace))
		}},
		{name: "deployments", observe: func(commands ResourceCommands) observedLiveState {
			return summarizeLiveSet(commands.ObserveDeployments(namespace))
		}},
		{name: "events", observe: func(commands ResourceCommands) observedLiveState {
			return summarizeLiveSet(commands.ObserveEvents(namespace))
		}},
		{name: "ingresses", observe: func(commands ResourceCommands) observedLiveState {
			return summarizeLiveSet(commands.ObserveIngresses(namespace))
		}},
		{name: "network policies", observe: func(commands ResourceCommands) observedLiveState {
			return summarizeLiveSet(commands.ObserveNetworkPolicies(namespace))
		}},
		{name: "persistent volume claims", observe: func(commands ResourceCommands) observedLiveState {
			return summarizeLiveSet(commands.ObservePersistentVolumeClaims(namespace))
		}},
		{name: "cron jobs", observe: func(commands ResourceCommands) observedLiveState {
			return summarizeLiveSet(commands.ObserveCronJobs(namespace))
		}},
		{name: "horizontal pod autoscalers", observe: func(commands ResourceCommands) observedLiveState {
			return summarizeLiveSet(commands.ObserveHorizontalPodAutoscalers(namespace))
		}},
		{name: "secrets", observe: func(commands ResourceCommands) observedLiveState {
			return summarizeLiveSet(commands.ObserveSecrets(namespace))
		}},
		{name: "replica sets", observe: func(commands ResourceCommands) observedLiveState {
			return summarizeLiveSet(commands.ObserveReplicaSets(namespace))
		}},
	}
}

func TestNativeClusterObservationAdaptersProjectResources(t *testing.T) {
	for _, test := range observationAdapterCases() {
		t.Run(test.name, func(t *testing.T) {
			observer := &testResourceObserver{}
			commands := NewCommands(context.Background(), &testResourceReader{}, observer)
			state := test.observe(commands)
			if state.err != nil || !state.ready || state.itemCount != 1 {
				t.Fatalf("observed state = %+v", state)
			}
			if len(observer.calls) != 1 || observer.calls[0] != test.name {
				t.Fatalf("observer calls = %v, want [%s]", observer.calls, test.name)
			}
		})
	}
}
