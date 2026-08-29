package ui

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/HediAbed/opsmate/internal/cluster"
)

func dashboardPodSnapshot(model *DashboardModel, state resourceLiveState[cluster.Pod]) supervisedLiveMsg {
	model.podLive.Set(newTestResourceLiveSet(resourceLiveState[cluster.Pod]{}))
	return supervisedLiveMsg{generation: model.podLive.Generation(), payload: liveSnapshotMsg[cluster.Pod]{State: state}}
}

func dashboardDeploymentSnapshot(model *DashboardModel, state resourceLiveState[cluster.Deployment]) supervisedLiveMsg {
	model.deploymentLive.Set(newTestResourceLiveSet(resourceLiveState[cluster.Deployment]{}))
	return supervisedLiveMsg{generation: model.deploymentLive.Generation(), payload: liveSnapshotMsg[cluster.Deployment]{State: state}}
}

func dashboardEventSnapshot(model *DashboardModel, state resourceLiveState[cluster.Event]) supervisedLiveMsg {
	model.eventLive.Set(newTestResourceLiveSet(resourceLiveState[cluster.Event]{}))
	return supervisedLiveMsg{generation: model.eventLive.Generation(), payload: liveSnapshotMsg[cluster.Event]{State: state}}
}

func TestDashboardLiveStartupObservesAllResources(t *testing.T) {
	observer := &testResourceObserver{}
	commands := newNativeClusterCommands(context.Background(), &testResourceReader{}, observer)
	model := NewDashboardModel("payments", commands)
	started := model.startDashboardLiveSets()
	if len(started) != 3 {
		t.Fatalf("started commands = %d, want 3", len(started))
	}
	if got := observer.calls; len(got) != 3 || got[0] != "pods" || got[1] != "deployments" || got[2] != "events" {
		t.Fatalf("observer calls = %v", got)
	}
	model.stopDashboardLiveSets()
}

func TestDashboardLiveStartupReportsObserverFailure(t *testing.T) {
	startupError := errors.New("observer unavailable")
	observer := &testResourceObserver{err: startupError}
	commands := newNativeClusterCommands(context.Background(), &testResourceReader{}, observer)
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
	message := dashboardPodSnapshot(&model, resourceLiveState[cluster.Pod]{
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
	message := dashboardDeploymentSnapshot(&model, resourceLiveState[cluster.Deployment]{
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
	message := dashboardEventSnapshot(&model, resourceLiveState[cluster.Event]{
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
	if !model.ownsSupervisedLiveMessage(message) || model.ownsSupervisedLiveMessage(supervisedLiveMsg{generation: ^uint64(0)}) {
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
	model, command := model.handlePodLivePayload(liveSnapshotMsg[cluster.Pod]{State: resourceLiveState[cluster.Pod]{
		Items: []cluster.Pod{{Name: "discarded"}}, Ready: true, Err: streamError,
	}})
	if command != nil || model.loading || !errors.Is(model.err, streamError) {
		t.Fatalf("pod error state = command:%v loading:%v error:%v", command != nil, model.loading, model.err)
	}
	model, _ = model.handleDeploymentLivePayload(liveSnapshotMsg[cluster.Deployment]{State: resourceLiveState[cluster.Deployment]{
		Items: []cluster.Deployment{{Name: "discarded"}}, Ready: true, Err: streamError,
	}})
	model, _ = model.handleEventLivePayload(liveSnapshotMsg[cluster.Event]{State: resourceLiveState[cluster.Event]{
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
	podSet := newTestResourceLiveSet(resourceLiveState[cluster.Pod]{})
	model.podLive.Set(podSet)
	model, command := model.handleSupervisedLiveMessage(supervisedLiveMsg{generation: model.podLive.Generation(), closed: true})
	if command != nil || !errors.Is(model.podLiveError, ErrLiveUpdatesStopped) || podSet.stops.Load() != 1 {
		t.Fatalf("pod closure = command:%v error:%v stops:%d", command != nil, model.podLiveError, podSet.stops.Load())
	}

	deploymentSet := newTestResourceLiveSet(resourceLiveState[cluster.Deployment]{})
	model.deploymentLive.Set(deploymentSet)
	model, command = model.handleSupervisedLiveMessage(supervisedLiveMsg{generation: model.deploymentLive.Generation(), closed: true})
	if command != nil || !errors.Is(model.deploymentLiveError, ErrLiveUpdatesStopped) || deploymentSet.stops.Load() != 1 {
		t.Fatalf("deployment closure = command:%v error:%v stops:%d", command != nil, model.deploymentLiveError, deploymentSet.stops.Load())
	}

	eventSet := newTestResourceLiveSet(resourceLiveState[cluster.Event]{})
	model.eventLive.Set(eventSet)
	model, command = model.handleSupervisedLiveMessage(supervisedLiveMsg{generation: model.eventLive.Generation(), closed: true})
	if command != nil || !errors.Is(model.eventLiveError, ErrLiveUpdatesStopped) || eventSet.stops.Load() != 1 {
		t.Fatalf("event closure = command:%v error:%v stops:%d", command != nil, model.eventLiveError, eventSet.stops.Load())
	}
}

func TestDashboardIgnoresUnownedAndNonLiveMessages(t *testing.T) {
	model := newTestDashboardModel("payments")
	model.err = ErrLiveUpdatesStopped
	model, command := model.handleSupervisedLiveMessage(supervisedLiveMsg{generation: ^uint64(0)})
	if command != nil || !errors.Is(model.err, ErrLiveUpdatesStopped) {
		t.Fatalf("unowned message changed dashboard state: command=%v error=%v", command != nil, model.err)
	}
	for _, kind := range []dashboardDataKind{dashboardMetrics, dashboardDataKindCount} {
		updated, next := model.handleDashboardLiveSetClosed(kind)
		if next != nil || !errors.Is(updated.err, ErrLiveUpdatesStopped) {
			t.Fatalf("non-live kind %d changed dashboard state", kind)
		}
	}
	if events := trimEventsToRecent([]cluster.Event{{Message: "ignored"}}, 0); events != nil {
		t.Fatalf("zero event limit returned %+v", events)
	}
}

func TestDashboardActiveNamespaceChangeRestartsLiveSets(t *testing.T) {
	observer := &testResourceObserver{}
	commands := newNativeClusterCommands(context.Background(), &testResourceReader{}, observer)
	model := NewDashboardModel("old", commands)
	model.active = true
	command := model.SetNamespace("payments")
	if command == nil || !model.active || model.namespace != "payments" {
		t.Fatalf("namespace restart = command:%v active:%v namespace:%q", command != nil, model.active, model.namespace)
	}
	if got := observer.calls; len(got) != 3 || got[0] != "pods" || got[1] != "deployments" || got[2] != "events" {
		t.Fatalf("observer calls = %v", got)
	}
	model.Deactivate()
}

func TestDashboardAndRootAcceptCurrentMetricsResult(t *testing.T) {
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

	root := newTestRootModel(t, "payments")
	root.dashboard.requestIDs[dashboardMetrics] = 4
	updated, _ := root.Update(result)
	got := updated.(RootModel).dashboard.metrics
	if len(got) != 1 || got[0].CPU != "25m" {
		t.Fatalf("root-routed dashboard metrics = %+v", got)
	}
}
