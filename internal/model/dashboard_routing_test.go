//go:build !windows

package model

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/service"
)

func TestDashboardSupervisedWatchRoutesToOwningResource(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*DashboardModel) supervisedWatchMsg
		assert func(*testing.T, DashboardModel)
	}{
		{
			name: "pod",
			setup: func(model *DashboardModel) supervisedWatchMsg {
				model.podWatcher.Set(newFakeWatcher[service.Pod]())
				return supervisedWatchMsg{generation: model.podWatcher.Generation(), payload: service.WatchEventMsg[service.Pod]{Event: service.WatchEvent[service.Pod]{Kind: service.WatchAdded, Item: service.Pod{Name: "worker", Namespace: "team-a"}}}}
			},
			assert: func(t *testing.T, model DashboardModel) {
				t.Helper()
				if len(model.pods) != 1 || model.pods[0].Name != "worker" {
					t.Errorf("pod payload was not routed: %+v", model.pods)
				}
			},
		},
		{
			name: "deployment",
			setup: func(model *DashboardModel) supervisedWatchMsg {
				model.deploymentWatcher.Set(newFakeWatcher[service.Deployment]())
				return supervisedWatchMsg{generation: model.deploymentWatcher.Generation(), payload: service.WatchEventMsg[service.Deployment]{Event: service.WatchEvent[service.Deployment]{Kind: service.WatchAdded, Item: service.Deployment{Name: "api", Namespace: "team-a"}}}}
			},
			assert: func(t *testing.T, model DashboardModel) {
				t.Helper()
				if len(model.deployments) != 1 || model.deployments[0].Name != "api" {
					t.Errorf("deployment payload was not routed: %+v", model.deployments)
				}
			},
		},
		{
			name: "event",
			setup: func(model *DashboardModel) supervisedWatchMsg {
				model.eventWatcher.Set(newFakeWatcher[service.Event]())
				return supervisedWatchMsg{generation: model.eventWatcher.Generation(), payload: service.WatchEventMsg[service.Event]{Event: service.WatchEvent[service.Event]{Kind: service.WatchAdded, Item: service.Event{Name: "warning", Namespace: "team-a"}}}}
			},
			assert: func(t *testing.T, model DashboardModel) {
				t.Helper()
				if len(model.events) != 1 || model.events[0].Name != "warning" {
					t.Errorf("event payload was not routed: %+v", model.events)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := NewDashboardModel("team-a")
			message := test.setup(&model)
			if !model.ownsSupervisedWatchMessage(message) {
				t.Fatal("dashboard did not recognize its supervised message")
			}
			updated, command := model.handleSupervisedWatchMessage(message)
			if command == nil {
				t.Error("handled watcher event must request the next frame")
			}
			test.assert(t, updated)
			model.Deactivate()
		})
	}
}

func TestDashboardSupervisedWatchRejectsUnknownGeneration(t *testing.T) {
	model := NewDashboardModel("team-a")
	message := supervisedWatchMsg{generation: 999, payload: struct{}{}}
	if model.ownsSupervisedWatchMessage(message) {
		t.Error("dashboard claimed a generation it does not own")
	}
	updated, command := model.handleSupervisedWatchMessage(message)
	if command != nil || updated.namespace != model.namespace {
		t.Fatalf("unknown generation changed model: command=%v model=%+v", command, updated)
	}
}

func TestDashboardWatchPayloadHandlersCoverEventAndCloseFrames(t *testing.T) {
	t.Run("pod", func(t *testing.T) {
		assertDashboardWatchPayloadLifecycle(
			t,
			func(model *DashboardModel) { model.podWatcher.Set(newFakeWatcher[service.Pod]()) },
			service.WatchEventMsg[service.Pod]{Event: service.WatchEvent[service.Pod]{Kind: service.WatchAdded, Item: service.Pod{Name: "worker", Namespace: "team-a"}}},
			dashboardPodWatchClosedMsg{},
			func(model DashboardModel, message tea.Msg) (DashboardModel, tea.Cmd) {
				return model.handlePodWatchPayload(message)
			},
			func(model DashboardModel) int { return len(model.pods) },
		)
	})
	t.Run("deployment", func(t *testing.T) {
		assertDashboardWatchPayloadLifecycle(
			t,
			func(model *DashboardModel) { model.deploymentWatcher.Set(newFakeWatcher[service.Deployment]()) },
			service.WatchEventMsg[service.Deployment]{Event: service.WatchEvent[service.Deployment]{Kind: service.WatchAdded, Item: service.Deployment{Name: "api", Namespace: "team-a"}}},
			dashboardDeploymentWatchClosedMsg{},
			func(model DashboardModel, message tea.Msg) (DashboardModel, tea.Cmd) {
				return model.handleDeploymentWatchPayload(message)
			},
			func(model DashboardModel) int { return len(model.deployments) },
		)
	})
	t.Run("event", func(t *testing.T) {
		assertDashboardWatchPayloadLifecycle(
			t,
			func(model *DashboardModel) { model.eventWatcher.Set(newFakeWatcher[service.Event]()) },
			service.WatchEventMsg[service.Event]{Event: service.WatchEvent[service.Event]{Kind: service.WatchAdded, Item: service.Event{Name: "warning", Namespace: "team-a"}}},
			dashboardEventWatchClosedMsg{},
			func(model DashboardModel, message tea.Msg) (DashboardModel, tea.Cmd) {
				return model.handleEventWatchPayload(message)
			},
			func(model DashboardModel) int { return len(model.events) },
		)
	})
}

func assertDashboardWatchPayloadLifecycle(
	t *testing.T,
	configure func(*DashboardModel),
	event tea.Msg,
	closedMessage tea.Msg,
	handle func(DashboardModel, tea.Msg) (DashboardModel, tea.Cmd),
	itemCount func(DashboardModel) int,
) {
	t.Helper()
	model := NewDashboardModel("team-a")
	model.active = true
	configure(&model)
	updated, command := handle(model, event)
	if command == nil || itemCount(updated) != 1 {
		t.Fatalf("event not handled: command=%v itemCount=%d", command, itemCount(updated))
	}
	closed, reconnect := handle(updated, closedMessage)
	if reconnect == nil {
		t.Error("close frame did not schedule reconnect")
	}
	unchanged, command := handle(closed, struct{}{})
	if command != nil || itemCount(unchanged) != 1 {
		t.Error("unknown payload should preserve cached data")
	}
	model.Deactivate()
}

type dashboardReconnectResult struct {
	command         tea.Cmd
	handled         bool
	retainedWatcher bool
}

func TestDashboardReconnectHandlersReplaceMatchingWatchers(t *testing.T) {
	installFakeKubectl(t, "#!/bin/sh\nsleep 5\n")
	tests := []struct {
		name string
		run  func(*DashboardModel) dashboardReconnectResult
	}{
		{name: "pod", run: reconnectDashboardPodForTest},
		{name: "deployment", run: reconnectDashboardDeploymentForTest},
		{name: "event", run: reconnectDashboardEventForTest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := NewDashboardModel("team-a")
			model.active = true
			result := test.run(&model)
			if !result.handled {
				t.Fatal("reconnect was not marked handled")
			}
			if result.command == nil {
				t.Fatal("valid reconnect did not return a pull command")
			}
			if result.retainedWatcher {
				t.Error("reconnect retained previous watcher")
			}
			model.Deactivate()
		})
	}
}

func reconnectDashboardPodForTest(model *DashboardModel) dashboardReconnectResult {
	model.podWatcher.Set(newFakeWatcher[service.Pod]())
	previous := model.podWatcher.Current()
	updated, command, handled := model.handlePodReconnect(
		dashboardPodReconnectMsg{namespace: model.namespace, generation: model.podWatcher.Generation()},
	)
	*model = updated
	return dashboardReconnectResult{command: command, handled: handled, retainedWatcher: model.podWatcher.Current() == previous}
}

func reconnectDashboardDeploymentForTest(model *DashboardModel) dashboardReconnectResult {
	model.deploymentWatcher.Set(newFakeWatcher[service.Deployment]())
	previous := model.deploymentWatcher.Current()
	updated, command, handled := model.handleDeploymentReconnect(
		dashboardDeploymentReconnectMsg{namespace: model.namespace, generation: model.deploymentWatcher.Generation()},
	)
	*model = updated
	return dashboardReconnectResult{command: command, handled: handled, retainedWatcher: model.deploymentWatcher.Current() == previous}
}

func reconnectDashboardEventForTest(model *DashboardModel) dashboardReconnectResult {
	model.eventWatcher.Set(newFakeWatcher[service.Event]())
	previous := model.eventWatcher.Current()
	updated, command, handled := model.handleEventReconnect(
		dashboardEventReconnectMsg{namespace: model.namespace, generation: model.eventWatcher.Generation()},
	)
	*model = updated
	return dashboardReconnectResult{command: command, handled: handled, retainedWatcher: model.eventWatcher.Current() == previous}
}

func TestDashboardReconnectHandlersIgnoreStaleMessages(t *testing.T) {
	model := NewDashboardModel("team-a")
	model.active = true
	if _, command, handled := model.handlePodReconnect(dashboardPodReconnectMsg{namespace: "other"}); command != nil || !handled {
		t.Errorf("stale pod reconnect: command=%v handled=%v", command, handled)
	}
	if _, command, handled := model.handleDeploymentReconnect(dashboardDeploymentReconnectMsg{namespace: "other"}); command != nil || !handled {
		t.Errorf("stale deployment reconnect: command=%v handled=%v", command, handled)
	}
	if _, command, handled := model.handleEventReconnect(dashboardEventReconnectMsg{namespace: "other"}); command != nil || !handled {
		t.Errorf("stale event reconnect: command=%v handled=%v", command, handled)
	}
	if _, command, handled := model.updateDashboardReconnectMessage(struct{}{}); command != nil || handled {
		t.Errorf("unknown reconnect: command=%v handled=%v", command, handled)
	}
}

func TestDashboardWatchClosedEventsPreserveCaches(t *testing.T) {
	model := NewDashboardModel("team-a")
	model.deployments = []service.Deployment{{Name: "api", Namespace: "team-a"}}
	model.events = []service.Event{{Name: "warning", Namespace: "team-a"}}

	withDeployments, _ := model.handleDeploymentWatchEvent(service.WatchEventMsg[service.Deployment]{
		Event: service.WatchEvent[service.Deployment]{Kind: service.WatchClosed},
	})
	if len(withDeployments.deployments) != 1 {
		t.Errorf("deployment close dropped cache: %+v", withDeployments.deployments)
	}
	withEvents, _ := model.handleEventWatchEvent(service.WatchEventMsg[service.Event]{
		Event: service.WatchEvent[service.Event]{Kind: service.WatchClosed},
	})
	if len(withEvents.events) != 1 {
		t.Errorf("event close dropped cache: %+v", withEvents.events)
	}
}

func TestTrimEventsToRecentRejectsNonPositiveLimit(t *testing.T) {
	events := []service.Event{{Name: "warning"}}
	if trimmed := trimEventsToRecent(events, 0); trimmed != nil {
		t.Errorf("zero limit returned %v, want nil", trimmed)
	}
	if trimmed := trimEventsToRecent(events, -1); trimmed != nil {
		t.Errorf("negative limit returned %v, want nil", trimmed)
	}
}
