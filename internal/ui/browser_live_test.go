package ui

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/HediAbed/opsmate/internal/cluster"
)

func TestBrowserStartsAndAppliesEveryLiveResource(t *testing.T) {
	wantCalls := []string{
		"pods",
		"deployments",
		"ingresses",
		"network policies",
		"persistent volume claims",
		"cron jobs",
		"horizontal pod autoscalers",
		"secrets",
		"replica sets",
	}
	observer := &testResourceObserver{}
	commands := newNativeClusterCommands(context.Background(), &testResourceReader{}, observer)

	for index, kind := range liveResourceKinds {
		model := NewBrowserModel("payments", commands, newTestClusterOperations())
		model.SetResourceType(kind)
		model.loading = true
		command := model.startResourceLiveSet()
		if command == nil {
			t.Fatalf("startResourceLiveSet(%q) returned no command", kind)
		}

		message, ok := command().(supervisedLiveMsg)
		if !ok || !model.ownsSupervisedLiveMessage(message) {
			t.Fatalf("startResourceLiveSet(%q) message = %#v", kind, message)
		}
		updated, next := model.handleSupervisedLiveMessage(message)
		if next == nil || updated.loading || !strings.Contains(updated.statusMsg, "Loaded 1") {
			t.Fatalf("live %q state was not applied: loading=%v status=%q", kind, updated.loading, updated.statusMsg)
		}
		if !slices.Equal(observer.calls, wantCalls[:index+1]) {
			t.Fatalf("observer calls = %v, want %v", observer.calls, wantCalls[:index+1])
		}
		updated.stopAllLiveSets()
	}
}

func TestBrowserLiveStartupHandlesErrorsAndStaticResources(t *testing.T) {
	startupError := errors.New("observe failed\nunsafe detail")
	observer := &testResourceObserver{err: startupError}
	commands := newNativeClusterCommands(context.Background(), &testResourceReader{}, observer)
	model := NewBrowserModel("payments", commands, newTestClusterOperations())
	model.resourceType = "pods"

	if command := model.loadCurrentResource(); command != nil {
		t.Fatal("failed live startup returned a command")
	}
	if model.loading || !errors.Is(model.err, startupError) || strings.Contains(model.errBanner, "\n") {
		t.Fatalf("startup error state = loading:%v error:%v banner:%q", model.loading, model.err, model.errBanner)
	}

	model.resourceType = "services"
	if command := model.startResourceLiveSet(); command != nil {
		t.Fatal("static resource started a live set")
	}
	if command := model.loadCurrentResource(); command == nil || !model.loading {
		t.Fatalf("static resource load = command:%v loading:%v", command != nil, model.loading)
	}
	if model.liveSetForKind("services") != nil {
		t.Fatal("static resource returned a live supervisor")
	}
}

func TestBrowserLiveClosureStopsCurrentLiveSet(t *testing.T) {
	model := newTestBrowserModel("payments")
	model.active = true
	model.resourceType = "pods"
	set := newTestResourceLiveSet(resourceLiveState[cluster.Pod]{Ready: true})
	model.podLive.Set(set)
	closed := supervisedLiveMsg{generation: model.podLive.Generation(), closed: true}

	updated, command := model.handleSupervisedLiveMessage(closed)
	if command != nil || !errors.Is(updated.err, ErrLiveUpdatesStopped) || updated.loading {
		t.Fatalf("closed live set state = command:%v loading:%v error:%v", command != nil, updated.loading, updated.err)
	}
	if set.stops.Load() != 1 || updated.podLive.Current() != nil {
		t.Fatalf("closed live set remained active: stops=%d", set.stops.Load())
	}
}

func TestBrowserIgnoresStaleLiveMessages(t *testing.T) {
	model := newTestBrowserModel("payments")
	model.active = true
	model.resourceType = "pods"
	model.errBanner = "retained"
	stale := supervisedLiveMsg{generation: model.podLive.Generation() + 1, closed: true}

	updated, command := model.handleSupervisedLiveMessage(stale)
	if command != nil || updated.errBanner != "retained" || updated.ownsSupervisedLiveMessage(stale) {
		t.Fatalf("stale message changed browser state: %#v", updated)
	}
}

func TestBrowserIgnoresClosureWhenInactiveOrStatic(t *testing.T) {
	inactive := newTestBrowserModel("payments")
	inactive, command := inactive.handleLiveSetClosed("deployments")
	if command != nil || inactive.err != nil {
		t.Fatalf("inactive closure changed visible state: command=%v error=%v", command != nil, inactive.err)
	}
	inactive, command = inactive.handleLiveSetClosed("services")
	if command != nil || inactive.err != nil {
		t.Fatalf("unknown closure changed visible state: command=%v error=%v", command != nil, inactive.err)
	}
}

func TestBrowserLiveStateOutOfScopeStopsLiveSet(t *testing.T) {
	model := newTestBrowserModel("payments")
	model.resourceType = "deployments"
	set := newTestResourceLiveSet(resourceLiveState[cluster.Pod]{})
	model.podLive.Set(set)
	if command := applyBrowserLiveState(&model, &model.podLive, &model.pods, "pods", resourceLiveState[cluster.Pod]{}); command != nil {
		t.Fatal("out-of-scope live state requested another snapshot")
	}
	if set.stops.Load() != 1 {
		t.Fatalf("out-of-scope live set stop count = %d", set.stops.Load())
	}
}

func TestBrowserLiveStreamErrorRetainsLastGoodState(t *testing.T) {
	model := newTestBrowserModel("payments")
	model.resourceType = "pods"
	model.loading = true
	model.pods = []cluster.Pod{{Name: "retained", Namespace: "payments"}}
	model.podLive.Set(newTestResourceLiveSet(resourceLiveState[cluster.Pod]{}))
	streamError := errors.New("watch failed\nunsafe detail")
	command := applyBrowserLiveState(&model, &model.podLive, &model.pods, "pods", resourceLiveState[cluster.Pod]{
		Items: []cluster.Pod{{Name: "discarded", Namespace: "payments"}},
		Ready: true,
		Err:   streamError,
	})
	if command == nil || model.loading || !errors.Is(model.err, streamError) || strings.Contains(model.errBanner, "\n") {
		t.Fatalf("live error state = command:%v loading:%v error:%v banner:%q", command != nil, model.loading, model.err, model.errBanner)
	}
	if len(model.pods) != 1 || model.pods[0].Name != "retained" {
		t.Fatalf("live error replaced the last good state: %+v", model.pods)
	}
	model.stopAllLiveSets()
}

func TestBrowserLiveRecoveryClearsErrorAndCopiesItems(t *testing.T) {
	model := newTestBrowserModel("payments")
	model.resourceType = "pods"
	model.err = errors.New("watch failed")
	model.errBanner = "watch failed"
	model.podLive.Set(newTestResourceLiveSet(resourceLiveState[cluster.Pod]{}))
	items := []cluster.Pod{{Name: "api", Namespace: "payments"}}
	command := applyBrowserLiveState(&model, &model.podLive, &model.pods, "pods", resourceLiveState[cluster.Pod]{Items: items, Ready: true})
	items[0].Name = "changed"
	if command == nil || model.err != nil || model.errBanner != "" || len(model.pods) != 1 || model.pods[0].Name != "api" {
		t.Fatalf("recovered live state = command:%v error:%v banner:%q pods:%+v", command != nil, model.err, model.errBanner, model.pods)
	}
	model.stopAllLiveSets()
}
