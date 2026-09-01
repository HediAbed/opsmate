package dashboard

import (
	"errors"
	"testing"
	"time"

	"github.com/HediAbed/opsmate/internal/cluster"
	clusterui "github.com/HediAbed/opsmate/internal/ui/cluster"
	"github.com/HediAbed/opsmate/internal/ui/screen"
)

func dashboardPodSnapshot(model *DashboardModel, state clusterui.LiveState[cluster.Pod]) screen.LiveMessage {
	model.podLive.Set(newTestResourceLiveSet(clusterui.LiveState[cluster.Pod]{}))
	return screen.LiveMessage{Generation: model.podLive.Generation(), Payload: screen.LiveSnapshot[cluster.Pod]{State: state}}
}

func dashboardDeploymentSnapshot(model *DashboardModel, state clusterui.LiveState[cluster.Deployment]) screen.LiveMessage {
	model.deploymentLive.Set(newTestResourceLiveSet(clusterui.LiveState[cluster.Deployment]{}))
	return screen.LiveMessage{Generation: model.deploymentLive.Generation(), Payload: screen.LiveSnapshot[cluster.Deployment]{State: state}}
}

func dashboardEventSnapshot(model *DashboardModel, state clusterui.LiveState[cluster.Event]) screen.LiveMessage {
	model.eventLive.Set(newTestResourceLiveSet(clusterui.LiveState[cluster.Event]{}))
	return screen.LiveMessage{Generation: model.eventLive.Generation(), Payload: screen.LiveSnapshot[cluster.Event]{State: state}}
}

func TestDashboardLiveStartupObservesAllResources(t *testing.T) {
	commands := &testCommands{}
	model := NewDashboardModel("payments", commands)
	started := model.startDashboardLiveSets()
	if len(started) != 3 {
		t.Fatalf("started commands = %d, want 3", len(started))
	}
	if got := commands.calls; len(got) != 3 || got[0] != "pods" || got[1] != "deployments" || got[2] != "events" {
		t.Fatalf("observer calls = %v", got)
	}
	model.stopDashboardLiveSets()
}

func TestDashboardLiveStartupReportsObserverFailure(t *testing.T) {
	startupError := errors.New("observer unavailable")
	commands := &testCommands{observeErr: startupError}
	model := NewDashboardModel("payments", commands)
	model.loading = true
	if failed := model.startDashboardLiveSets(); len(failed) != 0 {
		t.Fatalf("failed startup returned %d commands", len(failed))
	}
	if model.loading || !errors.Is(model.err, startupError) ||
		!errors.Is(model.podLiveError, startupError) ||
		!errors.Is(model.deploymentLiveError, startupError) ||
		!errors.Is(model.eventLiveError, startupError) {
		t.Fatalf("startup failure state = loading:%v error:%v", model.loading, model.err)
	}
}

func TestDashboardRoutesOwnedPodSnapshots(t *testing.T) {
	model := newTestDashboardModel("payments")
	message := dashboardPodSnapshot(&model, clusterui.LiveState[cluster.Pod]{
		Ready: true,
		Items: []cluster.Pod{{Name: "api", Namespace: "payments"}},
	})
	model, command := model.handleSupervisedLiveMessage(message)
	if command == nil || len(model.pods) != 1 || model.pods[0].Name != "api" {
		t.Fatalf("pod snapshot = command:%v pods:%+v", command != nil, model.pods)
	}
	model.stopDashboardLiveSets()
}

func TestDashboardRoutesOwnedDeploymentSnapshots(t *testing.T) {
	model := newTestDashboardModel("payments")
	message := dashboardDeploymentSnapshot(&model, clusterui.LiveState[cluster.Deployment]{
		Ready: true,
		Items: []cluster.Deployment{{Name: "api", Namespace: "payments"}},
	})
	model, command := model.handleSupervisedLiveMessage(message)
	if command == nil || len(model.deployments) != 1 || model.deployments[0].Name != "api" {
		t.Fatalf("deployment snapshot = command:%v deployments:%+v", command != nil, model.deployments)
	}
	model.stopDashboardLiveSets()
}

func TestDashboardRoutesOwnedEventSnapshots(t *testing.T) {
	model := newTestDashboardModel("payments")
	newer := time.Date(2026, time.August, 28, 10, 0, 0, 0, time.UTC)
	message := dashboardEventSnapshot(&model, clusterui.LiveState[cluster.Event]{
		Ready: true,
		Items: []cluster.Event{
			{Message: "older", LastTimestamp: newer.Add(-time.Minute)},
			{Message: "newer", LastTimestamp: newer},
		},
	})
	model, command := model.handleSupervisedLiveMessage(message)
	if command == nil || len(model.events) != 2 || model.events[0].Message != "newer" {
		t.Fatalf("event snapshot = command:%v events:%+v", command != nil, model.events)
	}
	if !model.OwnsLiveMessage(message) || model.OwnsLiveMessage(screen.LiveMessage{Generation: ^uint64(0)}) {
		t.Fatal("dashboard live ownership was not scoped to active generations")
	}
	model.stopDashboardLiveSets()
}

func TestDashboardLiveStreamErrorsRetainLastGoodState(t *testing.T) {
	model := newTestDashboardModel("payments")
	streamError := errors.New("stream failed")
	model.loading = true
	model.pods = []cluster.Pod{{Name: "retained-pod"}}
	model.deployments = []cluster.Deployment{{Name: "retained-deployment"}}
	model.events = []cluster.Event{{Message: "retained-event"}}
	model, command := model.handlePodLivePayload(screen.LiveSnapshot[cluster.Pod]{State: clusterui.LiveState[cluster.Pod]{
		Items: []cluster.Pod{{Name: "discarded"}}, Ready: true, Err: streamError,
	}})
	if command != nil || model.loading || !errors.Is(model.err, streamError) {
		t.Fatalf("pod error state = command:%v loading:%v error:%v", command != nil, model.loading, model.err)
	}
	model, _ = model.handleDeploymentLivePayload(screen.LiveSnapshot[cluster.Deployment]{State: clusterui.LiveState[cluster.Deployment]{
		Items: []cluster.Deployment{{Name: "discarded"}}, Ready: true, Err: streamError,
	}})
	model, _ = model.handleEventLivePayload(screen.LiveSnapshot[cluster.Event]{State: clusterui.LiveState[cluster.Event]{
		Items: []cluster.Event{{Message: "discarded"}}, Ready: true, Err: streamError,
	}})
	if !errors.Is(model.err, streamError) {
		t.Fatalf("combined stream error = %v", model.err)
	}
	if model.pods[0].Name != "retained-pod" || model.deployments[0].Name != "retained-deployment" || model.events[0].Message != "retained-event" {
		t.Fatalf("stream error replaced last good state: pods=%+v deployments=%+v events=%+v", model.pods, model.deployments, model.events)
	}
}

func TestDashboardIgnoresUnexpectedLivePayloads(t *testing.T) {
	model := newTestDashboardModel("payments")
	model.pods = []cluster.Pod{{Name: "retained-pod"}}
	model.deployments = []cluster.Deployment{{Name: "retained-deployment"}}
	model.events = []cluster.Event{{Message: "retained-event"}}
	if updated, next := model.handlePodLivePayload(struct{}{}); next != nil || len(updated.pods) != len(model.pods) {
		t.Fatal("unexpected pod payload changed dashboard state")
	}
	if updated, next := model.handleDeploymentLivePayload(struct{}{}); next != nil || len(updated.deployments) != len(model.deployments) {
		t.Fatal("unexpected deployment payload changed dashboard state")
	}
	if updated, next := model.handleEventLivePayload(struct{}{}); next != nil || len(updated.events) != len(model.events) {
		t.Fatal("unexpected event payload changed dashboard state")
	}
}

func TestDashboardLiveClosuresStopEachLiveSet(t *testing.T) {
	model := newTestDashboardModel("payments")
	podSet := newTestResourceLiveSet(clusterui.LiveState[cluster.Pod]{})
	model.podLive.Set(podSet)
	model, command := model.handleSupervisedLiveMessage(screen.LiveMessage{Generation: model.podLive.Generation(), Closed: true})
	if command != nil || !errors.Is(model.podLiveError, screen.ErrLiveUpdatesStopped) || podSet.stops.Load() != 1 {
		t.Fatalf("pod closure = command:%v error:%v stops:%d", command != nil, model.podLiveError, podSet.stops.Load())
	}

	deploymentSet := newTestResourceLiveSet(clusterui.LiveState[cluster.Deployment]{})
	model.deploymentLive.Set(deploymentSet)
	model, command = model.handleSupervisedLiveMessage(screen.LiveMessage{Generation: model.deploymentLive.Generation(), Closed: true})
	if command != nil || !errors.Is(model.deploymentLiveError, screen.ErrLiveUpdatesStopped) || deploymentSet.stops.Load() != 1 {
		t.Fatalf("deployment closure = command:%v error:%v stops:%d", command != nil, model.deploymentLiveError, deploymentSet.stops.Load())
	}

	eventSet := newTestResourceLiveSet(clusterui.LiveState[cluster.Event]{})
	model.eventLive.Set(eventSet)
	model, command = model.handleSupervisedLiveMessage(screen.LiveMessage{Generation: model.eventLive.Generation(), Closed: true})
	if command != nil || !errors.Is(model.eventLiveError, screen.ErrLiveUpdatesStopped) || eventSet.stops.Load() != 1 {
		t.Fatalf("event closure = command:%v error:%v stops:%d", command != nil, model.eventLiveError, eventSet.stops.Load())
	}
}

func TestDashboardIgnoresUnownedAndNonLiveMessages(t *testing.T) {
	model := newTestDashboardModel("payments")
	model.err = screen.ErrLiveUpdatesStopped
	model, command := model.handleSupervisedLiveMessage(screen.LiveMessage{Generation: ^uint64(0)})
	if command != nil || !errors.Is(model.err, screen.ErrLiveUpdatesStopped) {
		t.Fatalf("unowned message changed dashboard state: command=%v error=%v", command != nil, model.err)
	}
	for _, kind := range []dashboardDataKind{dashboardMetrics, dashboardDataKindCount} {
		updated, next := model.handleDashboardLiveSetClosed(kind)
		if next != nil || !errors.Is(updated.err, screen.ErrLiveUpdatesStopped) {
			t.Fatalf("non-live kind %d changed dashboard state", kind)
		}
	}
	if events := trimEventsToRecent([]cluster.Event{{Message: "ignored"}}, 0); events != nil {
		t.Fatalf("zero event limit returned %+v", events)
	}
}

func TestDashboardActiveNamespaceChangeRestartsLiveSets(t *testing.T) {
	commands := &testCommands{}
	model := NewDashboardModel("old", commands)
	model.active = true
	command := model.SetNamespace("payments")
	if command == nil || !model.active || model.namespace != "payments" {
		t.Fatalf("namespace restart = command:%v active:%v namespace:%q", command != nil, model.active, model.namespace)
	}
	if got := commands.calls; len(got) != 3 || got[0] != "pods" || got[1] != "deployments" || got[2] != "events" {
		t.Fatalf("observer calls = %v", got)
	}
	model.Deactivate()
}

func TestDashboardAcceptsCurrentMetricsResult(t *testing.T) {
	model := newTestDashboardModel("payments")
	model.requestIDs[dashboardMetrics] = 4
	result := dashboardResultMsg{
		kind:      dashboardMetrics,
		requestID: 4,
		namespace: "payments",
		payload: cluster.MetricsMsg{PodMetrics: []cluster.PodMetric{
			{Name: "api", Namespace: "payments", CPU: "25m"},
		}},
	}
	model, _ = model.Update(result)
	if len(model.metrics) != 1 || model.metrics[0].CPU != "25m" {
		t.Fatalf("dashboard metrics = %+v", model.metrics)
	}
}
