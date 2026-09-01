package browser

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/HediAbed/opsmate/internal/cluster"
	clusterui "github.com/HediAbed/opsmate/internal/ui/cluster"
	"github.com/HediAbed/opsmate/internal/ui/screen"
)

type browserLiveExpectation struct {
	resourceType string
	observerCall string
	resourceName string
}

func TestBrowserStartsAndAppliesEveryLiveResource(t *testing.T) {
	expectations := []browserLiveExpectation{
		{resourceTypePods, "pods", "observed-pod"},
		{resourceTypeDeployments, "deployments", "observed-deployment"},
		{resourceTypeIngresses, "ingresses", "observed-ingress"},
		{resourceTypeNetworkPolicies, "network policies", "observed-network-policy"},
		{resourceTypePVCs, "persistent volume claims", "observed-pvc"},
		{resourceTypeCronJobs, "cron jobs", "observed-cron-job"},
		{resourceTypeHPAs, "horizontal pod autoscalers", "observed-hpa"},
		{resourceTypeSecrets, "secrets", "observed-secret"},
		{resourceTypeReplicaSets, "replica sets", "observed-replica-set"},
	}
	if len(liveResourceKinds) != len(expectations) {
		t.Fatalf("live resource contract lengths = kinds:%d expectations:%d", len(liveResourceKinds), len(expectations))
	}
	commands := &testCommands{}
	model := NewBrowserModel("payments", commands, newTestClusterOperations())
	model.SetSize(100, 24)
	model.active = true
	expectedCalls := make([]string, 0, len(expectations))

	for index, expectation := range expectations {
		if liveResourceKinds[index] != expectation.resourceType {
			t.Fatalf("live resource %d = %q, want %q", index, liveResourceKinds[index], expectation.resourceType)
		}
		expectedCalls = append(expectedCalls, expectation.observerCall)
		updated, message := applyBrowserLiveExpectation(t, model, expectation)
		assertBrowserLiveExpectation(t, updated, message, expectation, commands.calls, expectedCalls)
		model = updated
	}
	model.stopAllLiveSets()
}

func applyBrowserLiveExpectation(
	t *testing.T,
	model BrowserModel,
	expectation browserLiveExpectation,
) (BrowserModel, screen.LiveMessage) {
	t.Helper()
	model.SetResourceType(expectation.resourceType)
	model.loading = true
	command := model.startResourceLiveSet()
	if command == nil {
		t.Fatalf("startResourceLiveSet(%q) returned nil", expectation.resourceType)
	}
	message, ok := command().(screen.LiveMessage)
	if !ok {
		t.Fatalf("startResourceLiveSet(%q) message = %#v", expectation.resourceType, message)
	}
	if !model.OwnsLiveMessage(message) {
		t.Fatalf("browser does not own %q live message", expectation.resourceType)
	}
	updated, next := model.Update(message)
	if next == nil {
		t.Fatalf("live update %q returned no continuation", expectation.resourceType)
	}
	if updated.errBanner != "" {
		t.Fatalf("live update %q error = %q", expectation.resourceType, updated.errBanner)
	}
	if updated.loading {
		t.Fatalf("live update %q remained loading", expectation.resourceType)
	}
	if !strings.Contains(updated.statusMsg, "Loaded 1") {
		t.Fatalf("live update %q status = %q", expectation.resourceType, updated.statusMsg)
	}
	return updated, message
}

func assertBrowserLiveExpectation(
	t *testing.T,
	model BrowserModel,
	message screen.LiveMessage,
	expectation browserLiveExpectation,
	observerCalls []string,
	expectedCalls []string,
) {
	t.Helper()
	if model.podLive.Owns(message) != (expectation.resourceType == resourceTypePods) {
		t.Fatalf("pod live ownership mismatch for %q", expectation.resourceType)
	}
	identity, selected := model.selectedIdentity()
	if !selected {
		t.Fatalf("live update %q selected no resource", expectation.resourceType)
	}
	if identity.Name != expectation.resourceName {
		t.Fatalf("live update %q selected %q", expectation.resourceType, identity.Name)
	}
	if identity.Namespace != "payments" {
		t.Fatalf("live update %q namespace = %q", expectation.resourceType, identity.Namespace)
	}
	if !slices.Equal(observerCalls, expectedCalls) {
		t.Fatalf("observe calls after %q = %v", expectation.resourceType, observerCalls)
	}
}

func TestBrowserLiveStartupHandlesErrorsAndStaticResources(t *testing.T) {
	startupError := errors.New("observe failed\nunsafe detail")
	commands := &testCommands{observeErr: startupError}
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
	set := newTestResourceLiveSet(clusterui.LiveState[cluster.Pod]{Ready: true})
	model.podLive.Set(set)
	closed := screen.LiveMessage{Generation: model.podLive.Generation(), Closed: true}

	updated, command := model.handleSupervisedLiveMessage(closed)
	if command != nil || !errors.Is(updated.err, screen.ErrLiveUpdatesStopped) || updated.loading {
		t.Fatalf("closed live set state = command:%v loading:%v error:%v", command != nil, updated.loading, updated.err)
	}
	if set.stops.Load() != 1 || updated.podLive.Current() != nil {
		t.Fatalf("closed live set remained active: stops=%d", set.stops.Load())
	}
}

func TestBrowserIgnoresStaleLiveMessages(t *testing.T) {
	model := newTestBrowserModel("payments")
	model.active = true
	model.resourceType = resourceTypePods
	staleSet := newTestResourceLiveSet(clusterui.LiveState[cluster.Pod]{Ready: true})
	currentSet := newTestResourceLiveSet(clusterui.LiveState[cluster.Pod]{Ready: true})
	model.podLive.Set(staleSet)
	staleGeneration := model.podLive.Generation()
	model.podLive.Set(currentSet)
	defer model.podLive.Stop()
	model.errBanner = "keep"

	stale := screen.LiveMessage{
		Generation: staleGeneration,
		Payload: screen.LiveSnapshot[cluster.Pod]{
			State: clusterui.LiveState[cluster.Pod]{Items: []cluster.Pod{{Name: "stale", Namespace: "payments"}}},
		},
	}
	updated, command := model.Update(stale)
	if command != nil || updated.errBanner != "keep" || len(updated.pods) != 0 {
		t.Fatalf("stale live message changed model: err=%q pods=%v command=%v", updated.errBanner, updated.pods, command)
	}
	if updated.podLive.Current() != currentSet || staleSet.stops.Load() != 1 {
		t.Fatalf("stale generation replaced current handle: current=%v stale stops=%d", updated.podLive.Current(), staleSet.stops.Load())
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
	set := newTestResourceLiveSet(clusterui.LiveState[cluster.Pod]{})
	model.podLive.Set(set)
	if command := applyBrowserLiveState(&model, &model.podLive, &model.pods, "pods", clusterui.LiveState[cluster.Pod]{}); command != nil {
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
	model.podLive.Set(newTestResourceLiveSet(clusterui.LiveState[cluster.Pod]{}))
	streamError := errors.New("watch failed\nunsafe detail")
	command := applyBrowserLiveState(&model, &model.podLive, &model.pods, "pods", clusterui.LiveState[cluster.Pod]{
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
	model.podLive.Set(newTestResourceLiveSet(clusterui.LiveState[cluster.Pod]{}))
	items := []cluster.Pod{{Name: "api", Namespace: "payments"}}
	command := applyBrowserLiveState(&model, &model.podLive, &model.pods, "pods", clusterui.LiveState[cluster.Pod]{Items: items, Ready: true})
	items[0].Name = "changed"
	if command == nil || model.err != nil || model.errBanner != "" || len(model.pods) != 1 || model.pods[0].Name != "api" {
		t.Fatalf("recovered live state = command:%v error:%v banner:%q pods:%+v", command != nil, model.err, model.errBanner, model.pods)
	}
	model.stopAllLiveSets()
}

func TestResourceNounFallsBackToTypeForUnsupportedResource(t *testing.T) {
	if got := resourceNoun("unsupported", 3); got != "unsupported" {
		t.Fatalf("resourceNoun(unsupported) = %q, want unsupported", got)
	}
}

func TestResourceNounUsesCatalogSingularAndPlural(t *testing.T) {
	if got := resourceNoun(resourceTypePods, 1); got != resourceKindPod {
		t.Fatalf("resourceNoun(pods, 1) = %q, want %q", got, resourceKindPod)
	}
	if got := resourceNoun(resourceTypePods, 2); got != resourceTypePods {
		t.Fatalf("resourceNoun(pods, 2) = %q, want %q", got, resourceTypePods)
	}
}
