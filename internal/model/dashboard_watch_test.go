//go:build !windows

package model

import (
	"strconv"
	"testing"
	"time"

	"github.com/HediAbed/opsmate/internal/service"
)

func TestDashboard_HandlePodWatchEvent_AddedAppendsAndRebuildsTable(t *testing.T) {
	m := NewDashboardModel("ns")
	m.SetSize(120, 40)

	out, _ := m.handlePodWatchEvent(service.WatchEventMsg[service.Pod]{
		Event: service.WatchEvent[service.Pod]{
			Kind: service.WatchAdded,
			Item: service.Pod{Name: "added", Namespace: "ns", Status: "Running", Ready: "1/1", Age: "1s"},
		},
	})
	if len(out.pods) != 1 || out.pods[0].Name != "added" {
		t.Errorf("ADDED did not append; pods = %+v", out.pods)
	}
	if out.loading {
		t.Error("watcher event delivery should clear loading")
	}
	if rows := out.podTable.Rows(); len(rows) != 1 {
		t.Errorf("table rows after ADDED = %d; want 1", len(rows))
	}
}

func TestDashboardWatchClosedFramePreservesReconnectBackoff(t *testing.T) {
	m := NewDashboardModel("ns")
	m.podWatcher.Set(newFakeWatcher[service.Pod]())
	m.podWatcher.attempts = 2

	out, _ := m.handlePodWatchEvent(service.WatchEventMsg[service.Pod]{
		Event: service.WatchEvent[service.Pod]{Kind: service.WatchClosed},
	})

	if out.podWatcher.attempts != 2 {
		t.Errorf("closed frame reset reconnect attempts to %d", out.podWatcher.attempts)
	}
}

func TestDashboard_HandlePodWatchEvent_ModifiedReplaces(t *testing.T) {
	m := NewDashboardModel("ns")
	m.SetSize(120, 40)
	m.pods = []service.Pod{{Name: "x", Namespace: "ns", Status: "Pending", Ready: "0/1"}}

	out, _ := m.handlePodWatchEvent(service.WatchEventMsg[service.Pod]{
		Event: service.WatchEvent[service.Pod]{
			Kind: service.WatchModified,
			Item: service.Pod{Name: "x", Namespace: "ns", Status: "Running", Ready: "1/1"},
		},
	})
	if out.pods[0].Status != "Running" {
		t.Errorf("MODIFIED did not patch; pod = %+v", out.pods[0])
	}
}

func TestDashboard_HandlePodWatchEvent_DeletedRemoves(t *testing.T) {
	m := NewDashboardModel("ns")
	m.SetSize(120, 40)
	m.pods = []service.Pod{
		{Name: "a", Namespace: "ns"},
		{Name: "b", Namespace: "ns"},
	}

	out, _ := m.handlePodWatchEvent(service.WatchEventMsg[service.Pod]{
		Event: service.WatchEvent[service.Pod]{
			Kind: service.WatchDeleted,
			Item: service.Pod{Name: "a", Namespace: "ns"},
		},
	})
	if len(out.pods) != 1 || out.pods[0].Name != "b" {
		t.Errorf("DELETED did not drop; pods = %+v", out.pods)
	}
}

func TestDashboard_HandleDeploymentWatchEvent_UpsertsAndDeletes(t *testing.T) {
	m := NewDashboardModel("ns")
	out, _ := m.handleDeploymentWatchEvent(service.WatchEventMsg[service.Deployment]{
		Event: service.WatchEvent[service.Deployment]{
			Kind: service.WatchAdded,
			Item: service.Deployment{Name: "web", Namespace: "ns", Ready: "2/3"},
		},
	})
	if len(out.deployments) != 1 {
		t.Fatalf("ADDED expected 1 deployment, got %d", len(out.deployments))
	}

	out, _ = out.handleDeploymentWatchEvent(service.WatchEventMsg[service.Deployment]{
		Event: service.WatchEvent[service.Deployment]{
			Kind: service.WatchModified,
			Item: service.Deployment{Name: "web", Namespace: "ns", Ready: "3/3"},
		},
	})
	if out.deployments[0].Ready != "3/3" {
		t.Errorf("MODIFIED did not patch; deployment = %+v", out.deployments[0])
	}

	out, _ = out.handleDeploymentWatchEvent(service.WatchEventMsg[service.Deployment]{
		Event: service.WatchEvent[service.Deployment]{
			Kind: service.WatchDeleted,
			Item: service.Deployment{Name: "web", Namespace: "ns"},
		},
	})
	if len(out.deployments) != 0 {
		t.Errorf("DELETED did not drop; deployments = %+v", out.deployments)
	}
}

func TestDashboard_HandleEventWatchEvent_Upserts(t *testing.T) {
	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	m := NewDashboardModel("ns")
	out, _ := m.handleEventWatchEvent(service.WatchEventMsg[service.Event]{
		Event: service.WatchEvent[service.Event]{
			Kind: service.WatchAdded,
			Item: service.Event{
				UID:  "event-uid",
				Type: "Warning", Reason: "BackOff", Object: "Pod/x",
				Message: "first", Count: 1, LastTimestamp: now,
			},
		},
	})
	if len(out.events) != 1 {
		t.Fatalf("ADDED expected 1 event; got %d", len(out.events))
	}

	out, _ = out.handleEventWatchEvent(service.WatchEventMsg[service.Event]{
		Event: service.WatchEvent[service.Event]{
			Kind: service.WatchModified,
			Item: service.Event{
				UID:  "event-uid",
				Type: "Warning", Reason: "BackOff", Object: "Pod/x",
				Message: "still", Count: 5, LastTimestamp: now.Add(time.Minute),
			},
		},
	})
	if len(out.events) != 1 {
		t.Errorf("MODIFIED with same key should replace, got %d events", len(out.events))
	}
	if out.events[0].Count != 5 {
		t.Errorf("MODIFIED did not bump Count: %+v", out.events[0])
	}
	out, _ = out.handleEventWatchEvent(service.WatchEventMsg[service.Event]{
		Event: service.WatchEvent[service.Event]{
			Kind: service.WatchDeleted,
			Item: service.Event{UID: "event-uid", LastTimestamp: now.Add(2 * time.Minute)},
		},
	})
	if len(out.events) != 0 {
		t.Fatalf("DELETED did not remove the stable event identity: %+v", out.events)
	}
}

func TestDashboard_HandleEventWatchEvent_TrimsToCap(t *testing.T) {
	m := NewDashboardModel("ns")
	now := time.Now()
	for i := 0; i < dashboardEventCap+5; i++ {
		out, _ := m.handleEventWatchEvent(service.WatchEventMsg[service.Event]{
			Event: service.WatchEvent[service.Event]{
				Kind: service.WatchAdded,
				Item: service.Event{
					UID:           strconv.Itoa(i),
					Reason:        "n",
					Object:        "Pod/x",
					LastTimestamp: now.Add(time.Duration(i) * time.Second),
				},
			},
		})
		m = out
	}
	if len(m.events) != dashboardEventCap {
		t.Errorf("event slice should be capped at %d, got %d", dashboardEventCap, len(m.events))
	}
}

func TestDashboard_HandleEventWatchEvent_ErrorSetsErrorWithoutDroppingCache(t *testing.T) {
	m := NewDashboardModel("ns")
	m.events = []service.Event{{Reason: "existing"}}

	streamErr := &service.WatchStreamError{Resource: "events", Err: errFake("kaboom")}
	out, _ := m.handleEventWatchEvent(service.WatchEventMsg[service.Event]{
		Event: service.WatchEvent[service.Event]{Kind: service.WatchErrored, Err: streamErr},
	})

	if out.err == nil {
		t.Error("ERROR event should set m.err")
	}
	if len(out.events) != 1 {
		t.Errorf("ERROR event should not drop existing cache, got %d", len(out.events))
	}
}

func TestDashboard_HandleEventWatchEvent_BookmarkPullsWithoutPatching(t *testing.T) {
	m := NewDashboardModel("ns")
	m.events = []service.Event{{Reason: "existing"}}
	out, _ := m.handleEventWatchEvent(service.WatchEventMsg[service.Event]{
		Event: service.WatchEvent[service.Event]{Kind: service.WatchBookmark},
	})
	if len(out.events) != 1 {
		t.Errorf("BOOKMARK should not change cache, got %+v", out.events)
	}
}

type errFake string

func (e errFake) Error() string { return string(e) }

func TestDashboard_Activate_StartsAllThreeWatchers(t *testing.T) {
	installFakeKubectl(t, `#!/bin/sh
sleep 5
`)
	m := NewDashboardModel("ns")
	cmd := m.Activate()
	if cmd == nil {
		t.Fatal("Activate should return a non-nil cmd")
	}
	if !m.active {
		t.Error("Activate should set active=true")
	}
	if m.podWatcher.Current() == nil {
		t.Error("Activate should start the pod watcher")
	}
	if m.deploymentWatcher.Current() == nil {
		t.Error("Activate should start the deployment watcher")
	}
	if m.eventWatcher.Current() == nil {
		t.Error("Activate should start the event watcher")
	}
	m.Deactivate()
}

func TestDashboard_Deactivate_StopsAllWatchers(t *testing.T) {
	installFakeKubectl(t, `#!/bin/sh
sleep 5
`)
	m := NewDashboardModel("ns")
	m.Activate()
	m.Deactivate()
	if m.active {
		t.Error("Deactivate should set active=false")
	}
	if m.podWatcher.Current() != nil ||
		m.deploymentWatcher.Current() != nil ||
		m.eventWatcher.Current() != nil {
		t.Error("Deactivate should null out every watcher")
	}
}

func TestDashboard_SetNamespace_RestartsWatchersWhenActive(t *testing.T) {
	installFakeKubectl(t, `#!/bin/sh
sleep 5
`)
	m := NewDashboardModel("ns")
	m.Activate()
	prev := m.podWatcher.Current()
	if prev == nil {
		t.Fatal("pod watcher should be running")
	}
	m.SetNamespace("kube-system")
	cur := m.podWatcher.Current()
	if cur == nil {
		t.Fatal("SetNamespace on active dashboard should restart the pod watcher")
	}
	if cur == prev {
		t.Error("SetNamespace should install a new watcher, not reuse the old handle")
	}
	m.Deactivate()
}

func TestDashboard_SetNamespace_DoesNotStartWatchersWhenInactive(t *testing.T) {
	installFakeKubectl(t, `#!/bin/sh
sleep 5
`)
	m := NewDashboardModel("ns")
	m.SetNamespace("kube-system")
	if m.podWatcher.Current() != nil {
		t.Error("SetNamespace on inactive dashboard should not start watchers")
	}
}

func TestDashboard_HandlePodWatchClosed_NoOpWhenInactive(t *testing.T) {
	m := NewDashboardModel("ns")
	m.active = false
	if _, cmd := m.handlePodWatchClosed(); cmd != nil {
		t.Error("inactive dashboard should not schedule a reconnect")
	}
}

func TestDashboard_HandlePodWatchClosed_SchedulesReconnect(t *testing.T) {
	m := NewDashboardModel("ns")
	m.active = true
	if _, cmd := m.handlePodWatchClosed(); cmd == nil {
		t.Error("active dashboard should schedule a pod reconnect")
	}
}

func TestDashboard_HandleDeploymentWatchClosed_SchedulesReconnect(t *testing.T) {
	m := NewDashboardModel("ns")
	m.active = true
	if _, cmd := m.handleDeploymentWatchClosed(); cmd == nil {
		t.Error("active dashboard should schedule a deployment reconnect")
	}
}

func TestDashboard_HandleDeploymentWatchClosed_NoOpWhenInactive(t *testing.T) {
	m := NewDashboardModel("ns")
	m.active = false
	if _, cmd := m.handleDeploymentWatchClosed(); cmd != nil {
		t.Error("inactive dashboard should not schedule a deployment reconnect")
	}
}

func TestDashboard_HandleEventWatchClosed_SchedulesReconnect(t *testing.T) {
	m := NewDashboardModel("ns")
	m.active = true
	if _, cmd := m.handleEventWatchClosed(); cmd == nil {
		t.Error("active dashboard should schedule an event reconnect")
	}
}

func TestDashboard_HandleEventWatchClosed_NoOpWhenInactive(t *testing.T) {
	m := NewDashboardModel("ns")
	m.active = false
	if _, cmd := m.handleEventWatchClosed(); cmd != nil {
		t.Error("inactive dashboard should not schedule an event reconnect")
	}
}

func TestEventKey_DistinctOnReasonAndObject(t *testing.T) {
	now := time.Now()
	a := service.Event{Reason: "BackOff", Object: "Pod/x", LastTimestamp: now}
	b := service.Event{Reason: "BackOff", Object: "Pod/y", LastTimestamp: now}
	if eventKey(a) == eventKey(b) {
		t.Error("events with different objects should have different keys")
	}
}

func TestEventKey_StableForSameTimestampReasonObject(t *testing.T) {
	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	a := service.Event{Reason: "BackOff", Object: "Pod/x", LastTimestamp: now}
	b := service.Event{Reason: "BackOff", Object: "Pod/x", LastTimestamp: now, Count: 5}
	if eventKey(a) != eventKey(b) {
		t.Errorf("same key inputs should yield same key; %q vs %q", eventKey(a), eventKey(b))
	}
}

func TestRemoveEvent_DropsMatchingEntry(t *testing.T) {
	now := time.Now()
	target := service.Event{Reason: "BackOff", Object: "Pod/x", LastTimestamp: now}
	other := service.Event{Reason: "Pulled", Object: "Pod/y", LastTimestamp: now}
	got := removeEvent([]service.Event{target, other}, target)
	if len(got) != 1 || got[0].Reason != "Pulled" {
		t.Errorf("removeEvent should drop the matching event; got %+v", got)
	}
}

func TestRemoveEvent_NoOpWhenAbsent(t *testing.T) {
	now := time.Now()
	existing := service.Event{Reason: "Pulled", Object: "Pod/x", LastTimestamp: now}
	missing := service.Event{Reason: "Killed", Object: "Pod/y", LastTimestamp: now}
	got := removeEvent([]service.Event{existing}, missing)
	if len(got) != 1 {
		t.Errorf("removeEvent should be a no-op when target is absent; got %+v", got)
	}
}

func TestTrimEventsToRecent_PreservesShorterSlice(t *testing.T) {
	now := time.Now()
	events := []service.Event{
		{Reason: "a", LastTimestamp: now.Add(-time.Minute)},
		{Reason: "b", LastTimestamp: now},
	}
	got := trimEventsToRecent(events, 5)
	if len(got) != 2 {
		t.Errorf("trim with cap > len should preserve all events, got %d", len(got))
	}
	if got[0].Reason != "b" {
		t.Errorf("trim should sort newest-first, got first reason %q", got[0].Reason)
	}
}

func TestDashboard_HandleEventWatchEvent_DeletedRemoves(t *testing.T) {
	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	m := NewDashboardModel("ns")
	m.events = []service.Event{
		{Reason: "BackOff", Object: "Pod/x", LastTimestamp: now},
	}
	out, _ := m.handleEventWatchEvent(service.WatchEventMsg[service.Event]{
		Event: service.WatchEvent[service.Event]{
			Kind: service.WatchDeleted,
			Item: service.Event{Reason: "BackOff", Object: "Pod/x", LastTimestamp: now},
		},
	})
	if len(out.events) != 0 {
		t.Errorf("DELETED should remove matching event; got %+v", out.events)
	}
}

func TestDashboard_HandlePodWatchEvent_BookmarkPullsWithoutPatching(t *testing.T) {
	m := NewDashboardModel("ns")
	m.SetSize(120, 40)
	m.pods = []service.Pod{{Name: "x", Namespace: "ns"}}
	out, _ := m.handlePodWatchEvent(service.WatchEventMsg[service.Pod]{
		Event: service.WatchEvent[service.Pod]{Kind: service.WatchBookmark},
	})
	if len(out.pods) != 1 {
		t.Errorf("BOOKMARK should not patch cache; pods = %+v", out.pods)
	}
}

func TestDashboard_HandlePodWatchEvent_ErrorSetsErrAndKeepsCache(t *testing.T) {
	m := NewDashboardModel("ns")
	m.SetSize(120, 40)
	m.pods = []service.Pod{{Name: "x", Namespace: "ns"}}
	streamErr := &service.WatchStreamError{Resource: "pods", Err: errFake("boom")}
	out, _ := m.handlePodWatchEvent(service.WatchEventMsg[service.Pod]{
		Event: service.WatchEvent[service.Pod]{Kind: service.WatchErrored, Err: streamErr},
	})
	if out.err == nil {
		t.Error("ERROR event should set m.err")
	}
	if len(out.pods) != 1 {
		t.Errorf("ERROR event should preserve cache; got %+v", out.pods)
	}
}

func TestDashboard_HandleDeploymentWatchEvent_BookmarkAndError(t *testing.T) {
	m := NewDashboardModel("ns")
	m.deployments = []service.Deployment{{Name: "web", Namespace: "ns"}}

	out, _ := m.handleDeploymentWatchEvent(service.WatchEventMsg[service.Deployment]{
		Event: service.WatchEvent[service.Deployment]{Kind: service.WatchBookmark},
	})
	if len(out.deployments) != 1 {
		t.Errorf("BOOKMARK should preserve cache; deployments = %+v", out.deployments)
	}

	streamErr := &service.WatchStreamError{Resource: "deployments", Err: errFake("boom")}
	out, _ = m.handleDeploymentWatchEvent(service.WatchEventMsg[service.Deployment]{
		Event: service.WatchEvent[service.Deployment]{Kind: service.WatchErrored, Err: streamErr},
	})
	if out.err == nil {
		t.Error("ERROR event should set m.err when no prior error")
	}
}
