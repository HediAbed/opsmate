//go:build !windows

package model

import (
	"testing"
	"time"

	"github.com/HediAbed/opsmate/internal/service"
)

func TestUpsertByName_AppendsWhenAbsent(t *testing.T) {
	pods := []service.Pod{{Name: "a", Namespace: "ns"}}
	added := service.Pod{Name: "b", Namespace: "ns"}

	got := upsertByName(pods, added, podKey)
	if len(got) != 2 {
		t.Fatalf("len = %d; want 2", len(got))
	}
	if got[1].Name != "b" {
		t.Errorf("appended pod = %q; want b", got[1].Name)
	}
}

func TestUpsertByName_ReplacesInPlace(t *testing.T) {
	pods := []service.Pod{
		{Name: "a", Namespace: "ns", Status: "Pending"},
		{Name: "b", Namespace: "ns", Status: "Running"},
	}
	updated := service.Pod{Name: "a", Namespace: "ns", Status: "Running"}

	got := upsertByName(pods, updated, podKey)
	if len(got) != 2 {
		t.Fatalf("len = %d; want 2", len(got))
	}
	if got[0].Status != "Running" {
		t.Errorf("got[0].Status = %q; want Running", got[0].Status)
	}
}

func TestUpsertByName_SameNameDifferentNamespacesAreDistinct(t *testing.T) {
	pods := []service.Pod{{Name: "x", Namespace: "ns-a"}}
	added := service.Pod{Name: "x", Namespace: "ns-b"}

	got := upsertByName(pods, added, podKey)
	if len(got) != 2 {
		t.Errorf("len = %d; want 2 (different namespaces)", len(got))
	}
}

func TestRemoveByName_DropsMatchingItem(t *testing.T) {
	pods := []service.Pod{
		{Name: "a", Namespace: "ns"},
		{Name: "b", Namespace: "ns"},
		{Name: "c", Namespace: "ns"},
	}
	got := removeByName(pods, service.Pod{Name: "b", Namespace: "ns"}, podKey)
	if len(got) != 2 {
		t.Fatalf("len = %d; want 2", len(got))
	}
	for _, p := range got {
		if p.Name == "b" {
			t.Error("pod 'b' should have been removed, still present")
		}
	}
}

func TestRemoveByName_NoOpWhenAbsent(t *testing.T) {
	pods := []service.Pod{{Name: "a", Namespace: "ns"}}
	got := removeByName(pods, service.Pod{Name: "missing", Namespace: "ns"}, podKey)
	if len(got) != 1 {
		t.Errorf("len = %d; want 1 (missing item should not affect slice)", len(got))
	}
}

func TestApplyTypedWatchEvent_AddedAppendsToSlice(t *testing.T) {
	pods := []service.Pod{}
	applyTypedWatchEvent(&pods, service.WatchEvent[service.Pod]{
		Kind: service.WatchAdded,
		Item: service.Pod{Name: "added", Namespace: "ns", Status: "Pending"},
	}, podKey)
	if len(pods) != 1 || pods[0].Name != "added" {
		t.Errorf("ADDED did not append; pods = %+v", pods)
	}
}

func TestApplyTypedWatchEvent_ModifiedReplacesInPlace(t *testing.T) {
	pods := []service.Pod{{Name: "x", Namespace: "ns", Status: "Pending"}}
	applyTypedWatchEvent(&pods, service.WatchEvent[service.Pod]{
		Kind: service.WatchModified,
		Item: service.Pod{Name: "x", Namespace: "ns", Status: "Running"},
	}, podKey)
	if len(pods) != 1 || pods[0].Status != "Running" {
		t.Errorf("MODIFIED did not patch in place; pods = %+v", pods)
	}
}

func TestApplyTypedWatchEvent_DeletedRemoves(t *testing.T) {
	pods := []service.Pod{
		{Name: "a", Namespace: "ns"},
		{Name: "b", Namespace: "ns"},
	}
	applyTypedWatchEvent(&pods, service.WatchEvent[service.Pod]{
		Kind: service.WatchDeleted,
		Item: service.Pod{Name: "a", Namespace: "ns"},
	}, podKey)
	if len(pods) != 1 || pods[0].Name != "b" {
		t.Errorf("DELETED did not drop; pods = %+v", pods)
	}
}

func TestApplyTypedWatchEvent_WorksOverDifferentTypes(t *testing.T) {
	deps := []service.Deployment{}
	applyTypedWatchEvent(&deps, service.WatchEvent[service.Deployment]{
		Kind: service.WatchAdded,
		Item: service.Deployment{Name: "web", Namespace: "ns", Ready: "1/1"},
	}, deploymentKey)
	if len(deps) != 1 {
		t.Errorf("expected 1 deployment after ADDED; got %d", len(deps))
	}
}

func TestBrowser_StopAllWatchers_LeavesEverySupervisorIdle(t *testing.T) {
	m := NewBrowserModel("ns")
	m.stopAllWatchers()
	for _, kind := range liveResourceKinds {
		if watcher := m.closableForKind(kind); watcher != nil && watcher.Generation() != 0 {
			t.Fatalf("watcher %q retained an active token", kind)
		}
	}
}

func TestBrowser_StartResourceWatch_ReturnsNilForUnwatchedTypes(t *testing.T) {
	m := NewBrowserModel("ns")
	for _, rt := range []string{"services", "statefulsets", "daemonsets", "configmaps", "nodes", "jobs"} {
		m.SetResourceType(rt)
		if cmd := m.startResourceWatch(); cmd != nil {
			t.Errorf("startResourceWatch for %q should return nil; got non-nil cmd", rt)
		}
	}
}

func TestBrowser_StartResourceWatch_DeploymentsStartsDeploymentWatcher(t *testing.T) {
	installFakeKubectl(t, `#!/bin/sh
sleep 5
`)
	m := NewBrowserModel("ns")
	m.SetResourceType("deployments")
	cmd := m.startResourceWatch()
	if cmd == nil {
		t.Fatal("startResourceWatch should return a non-nil cmd for deployments")
	}
	if m.deploymentWatcher.Current() == nil {
		t.Error("deployment watcher should be installed in supervisor")
	}
	m.stopAllWatchers()
}

func TestBrowser_SetNamespace_StopsWatchers(t *testing.T) {
	installFakeKubectl(t, `#!/bin/sh
echo '{"type":"ADDED","object":{"metadata":{"name":"a","namespace":"ns","creationTimestamp":"2024-01-01T00:00:00Z"},"status":{"phase":"Running"},"spec":{"nodeName":""}}}'
sleep 5
`)
	m := NewBrowserModel("ns")
	cmd := m.startResourceWatch()
	if cmd == nil {
		t.Fatal("startResourceWatch should return a non-nil cmd for pods")
	}
	if m.podWatcher.Current() == nil {
		t.Fatal("watchSupervisor should hold the new watcher")
	}

	m.SetNamespace("kube-system")
	if m.podWatcher.Current() != nil {
		t.Error("SetNamespace should cancel watchers (Current must be nil)")
	}
}

func TestBrowser_SetResourceType_CancelsPriorWatcher(t *testing.T) {
	installFakeKubectl(t, `#!/bin/sh
sleep 5
`)
	m := NewBrowserModel("ns")
	m.startResourceWatch()
	if m.podWatcher.Current() == nil {
		t.Fatal("pod watcher should be active before tab switch")
	}
	m.SetResourceType("services")
	if m.podWatcher.Current() != nil {
		t.Error("after switching away from pods, podWatcher must be cleared")
	}
}

func TestBrowser_Activate_SetsActiveAndStartsWatcher(t *testing.T) {
	installFakeKubectl(t, `#!/bin/sh
sleep 5
`)
	m := NewBrowserModel("ns")
	cmd := m.Activate()
	if cmd == nil {
		t.Error("Activate should return a non-nil command")
	}
	if !m.active {
		t.Error("Activate should set active=true")
	}
	if m.podWatcher.Current() == nil {
		t.Error("Activate on default resource (pods) should start a pod watcher")
	}
	m.Deactivate()
}

func TestBrowser_Deactivate_StopsWatchers(t *testing.T) {
	installFakeKubectl(t, `#!/bin/sh
sleep 5
`)
	m := NewBrowserModel("ns")
	m.Activate()
	if m.podWatcher.Current() == nil {
		t.Fatal("pod watcher should be running after Activate")
	}
	m.Deactivate()
	if m.active {
		t.Error("Deactivate should set active=false")
	}
	if m.podWatcher.Current() != nil {
		t.Error("Deactivate should null out the watcher")
	}
}

func TestReconnectCappedDelay_StartsWithImmediateThenRamps(t *testing.T) {
	want := []time.Duration{0, 1 * time.Second, 5 * time.Second, 30 * time.Second, 30 * time.Second, 30 * time.Second}
	for i, w := range want {
		got := reconnectCappedDelay(i)
		if got != w {
			t.Errorf("attempt %d: got %v; want %v", i, got, w)
		}
	}
}

func TestReconnectCappedDelay_NegativeAttemptClampsToZero(t *testing.T) {
	got := reconnectCappedDelay(-1)
	if got != 0 {
		t.Errorf("negative attempt should clamp to 0, got %v", got)
	}
}

func TestHandleTypedWatchEvent_DroppedWhenResourceTypeChanged(t *testing.T) {
	m := NewBrowserModel("ns")
	m.SetResourceType("services")
	_ = handleTypedWatchEvent(&m, &m.podWatcher, &m.pods, podKey, "pods",
		service.WatchEventMsg[service.Pod]{
			Event: service.WatchEvent[service.Pod]{
				Kind: service.WatchAdded,
				Item: service.Pod{Name: "stale", Namespace: "ns"},
			},
		})
	if len(m.pods) != 0 {
		t.Errorf("stale pod event should be ignored when resource type is %q; pods = %+v",
			m.resourceType, m.pods)
	}
}

func TestHandleSharedWatchClosed_NoOpWhenInactive(t *testing.T) {
	m := NewBrowserModel("ns")
	m.active = false
	_, cmd := m.handleSharedWatchClosed("pods")
	if cmd != nil {
		t.Error("handleSharedWatchClosed on inactive model should return nil cmd")
	}
}

func TestHandleSharedWatchClosed_SchedulesReconnectWhenActive(t *testing.T) {
	m := NewBrowserModel("ns")
	m.active = true
	m.resourceType = "pods"
	_, cmd := m.handleSharedWatchClosed("pods")
	if cmd == nil {
		t.Error("handleSharedWatchClosed on active model should schedule a reconnect")
	}
}

func TestHandleSharedWatchClosed_NoReconnectAfterTabSwitch(t *testing.T) {
	m := NewBrowserModel("ns")
	m.active = true
	m.resourceType = "configmaps"
	_, cmd := m.handleSharedWatchClosed("pods")
	if cmd != nil {
		t.Error("handleSharedWatchClosed must not reconnect when the active resource is different")
	}
}

func TestHandleSharedWatchClosed_OnlyReconnectsForMatchingKind(t *testing.T) {
	m := NewBrowserModel("ns")
	m.active = true
	m.resourceType = "deployments"
	if _, cmd := m.handleSharedWatchClosed("deployments"); cmd == nil {
		t.Error("active deployments view should schedule deployment reconnect")
	}
	if _, cmd := m.handleSharedWatchClosed("pods"); cmd != nil {
		t.Error("close for pods while viewing deployments must not reconnect")
	}
}

func TestHandleTypedWatchEvent_AddedRebuildsTable(t *testing.T) {
	m := NewBrowserModel("ns")
	m.SetSize(120, 40)
	_ = handleTypedWatchEvent(&m, &m.podWatcher, &m.pods, podKey, "pods",
		service.WatchEventMsg[service.Pod]{
			Event: service.WatchEvent[service.Pod]{
				Kind: service.WatchAdded,
				Item: service.Pod{Name: "live", Namespace: "ns", Status: "Running", Ready: "1/1"},
			},
		})
	if len(m.pods) != 1 {
		t.Fatalf("ADDED should append; pods = %+v", m.pods)
	}
	if m.loading {
		t.Error("watcher event should clear loading")
	}
}

func TestHandleTypedWatchEvent_BookmarkPullsNextWithoutPatching(t *testing.T) {
	m := NewBrowserModel("ns")
	m.pods = []service.Pod{{Name: "x", Namespace: "ns"}}
	_ = handleTypedWatchEvent(&m, &m.podWatcher, &m.pods, podKey, "pods",
		service.WatchEventMsg[service.Pod]{
			Event: service.WatchEvent[service.Pod]{Kind: service.WatchBookmark},
		})
	if len(m.pods) != 1 {
		t.Errorf("BOOKMARK should not change cache; pods = %+v", m.pods)
	}
}

func TestHandleTypedWatchEvent_ErrorSetsBannerWithoutDroppingCache(t *testing.T) {
	m := NewBrowserModel("ns")
	m.pods = []service.Pod{{Name: "existing", Namespace: "ns"}}
	streamErr := &service.WatchStreamError{Resource: "pods", Err: testErr("kaboom")}
	_ = handleTypedWatchEvent(&m, &m.podWatcher, &m.pods, podKey, "pods",
		service.WatchEventMsg[service.Pod]{
			Event: service.WatchEvent[service.Pod]{Kind: service.WatchErrored, Err: streamErr},
		})
	if m.errBanner == "" {
		t.Error("ERROR event should populate errBanner")
	}
	if len(m.pods) != 1 {
		t.Errorf("ERROR event should not drop cache; pods = %+v", m.pods)
	}
}

func TestHandleTypedWatchEvent_DeploymentLifecycle(t *testing.T) {
	m := NewBrowserModel("ns")
	m.SetSize(120, 40)
	m.SetResourceType("deployments")
	_ = handleTypedWatchEvent(&m, &m.deploymentWatcher, &m.deployments, deploymentKey, "deployments",
		service.WatchEventMsg[service.Deployment]{
			Event: service.WatchEvent[service.Deployment]{
				Kind: service.WatchAdded,
				Item: service.Deployment{Name: "web", Namespace: "ns", Ready: "0/3"},
			},
		})
	if len(m.deployments) != 1 {
		t.Fatalf("ADDED expected 1 deployment; got %d", len(m.deployments))
	}

	_ = handleTypedWatchEvent(&m, &m.deploymentWatcher, &m.deployments, deploymentKey, "deployments",
		service.WatchEventMsg[service.Deployment]{
			Event: service.WatchEvent[service.Deployment]{
				Kind: service.WatchModified,
				Item: service.Deployment{Name: "web", Namespace: "ns", Ready: "3/3"},
			},
		})
	if m.deployments[0].Ready != "3/3" {
		t.Errorf("MODIFIED did not patch; deployment = %+v", m.deployments[0])
	}

	_ = handleTypedWatchEvent(&m, &m.deploymentWatcher, &m.deployments, deploymentKey, "deployments",
		service.WatchEventMsg[service.Deployment]{
			Event: service.WatchEvent[service.Deployment]{
				Kind: service.WatchDeleted,
				Item: service.Deployment{Name: "web", Namespace: "ns"},
			},
		})
	if len(m.deployments) != 0 {
		t.Errorf("DELETED did not drop; deployments = %+v", m.deployments)
	}
}

func TestHandleSharedReconnect_HappyPathInstallsWatcher(t *testing.T) {
	installFakeKubectl(t, `#!/bin/sh
sleep 5
`)
	m := NewBrowserModel("ns")
	m.active = true
	m.resourceType = "pods"
	m.podWatcher.Set(newFakeWatcher[service.Pod]())

	out, cmd := m.handleSharedReconnect(browserReconnectMsg{
		Kind:       "pods",
		Namespace:  "ns",
		Generation: m.podWatcher.Generation(),
	})
	if cmd == nil {
		t.Fatal("active + matching kind + matching namespace must return a non-nil reconnect cmd")
	}
	if out.podWatcher.Current() == nil {
		t.Error("reconnect must install a fresh pod watcher in the supervisor")
	}
	out.stopAllWatchers()
}

func TestBrowserWatchClosedFramePreservesReconnectBackoff(t *testing.T) {
	m := NewBrowserModel("ns")
	m.SetResourceType("pods")
	m.podWatcher.Set(newFakeWatcher[service.Pod]())
	m.podWatcher.attempts = 2

	_ = handleTypedWatchEvent(
		&m,
		&m.podWatcher,
		&m.pods,
		podKey,
		"pods",
		service.WatchEventMsg[service.Pod]{Event: service.WatchEvent[service.Pod]{Kind: service.WatchClosed}},
	)

	if m.podWatcher.attempts != 2 {
		t.Errorf("closed frame reset reconnect attempts to %d", m.podWatcher.attempts)
	}
}

func TestHandleSharedReconnect_NoOpWhenNamespaceChanged(t *testing.T) {
	m := NewBrowserModel("ns-a")
	m.active = true
	m.resourceType = "pods"
	_, cmd := m.handleSharedReconnect(browserReconnectMsg{Kind: "pods", Namespace: "ns-old"})
	if cmd != nil {
		t.Error("reconnect after namespace switch must be a no-op")
	}
}

func TestHandleSharedReconnect_UnknownKindIsNoOp(t *testing.T) {
	m := NewBrowserModel("ns")
	m.active = true
	m.resourceType = "made-up"
	_, cmd := m.handleSharedReconnect(browserReconnectMsg{Kind: "made-up", Namespace: "ns"})
	if cmd != nil {
		t.Error("unknown kind must produce no command")
	}
}

type testErr string

func (e testErr) Error() string { return string(e) }
