package model

import (
	"context"
	"testing"
	"time"

	"github.com/HediAbed/opsmate/internal/service"
)

type fakeWatcher[T service.WatchResource] struct {
	ch     chan service.WatchEvent[T]
	cancel func()
}

func newFakeWatcher[T service.WatchResource]() *fakeWatcher[T] {
	return &fakeWatcher[T]{
		ch:     make(chan service.WatchEvent[T], 4),
		cancel: func() {},
	}
}

func (f *fakeWatcher[T]) Events() <-chan service.WatchEvent[T] { return f.ch }
func (f *fakeWatcher[T]) Cancel() {
	if f.cancel != nil {
		f.cancel()
	}
}

func TestWatchSupervisor_SetCancelsPrior(t *testing.T) {
	var sup watchSupervisor[service.Pod]

	cancelled := 0
	first := newFakeWatcher[service.Pod]()
	first.cancel = func() { cancelled++ }
	second := newFakeWatcher[service.Pod]()

	sup.Set(first)
	sup.Set(second)

	if cancelled != 1 {
		t.Errorf("Set should cancel prior watcher; cancelled = %d", cancelled)
	}
	if sup.Current() != second {
		t.Error("Set should swap Current to the new watcher")
	}
}

func TestWatchSupervisor_RejectsStaleGenerationAfterRepeatedReplacement(t *testing.T) {
	var sup watchSupervisor[service.Pod]
	sup.Set(newFakeWatcher[service.Pod]())
	staleGeneration := sup.Generation()

	for range 10_000 {
		sup.Set(newFakeWatcher[service.Pod]())
	}

	if sup.Generation() == staleGeneration {
		t.Fatalf("watch generation did not advance from %d", staleGeneration)
	}
	if sup.Owns(supervisedWatchMsg{generation: staleGeneration}) {
		t.Fatal("supervisor accepted a stale watch generation")
	}
	if !sup.Owns(supervisedWatchMsg{generation: sup.Generation()}) {
		t.Fatal("supervisor rejected its current watch generation")
	}
}

func TestWatchSupervisor_GenerationsAreUniqueAcrossOwners(t *testing.T) {
	var pods watchSupervisor[service.Pod]
	var deployments watchSupervisor[service.Deployment]
	pods.Set(newFakeWatcher[service.Pod]())
	deployments.Set(newFakeWatcher[service.Deployment]())

	if pods.Generation() == deployments.Generation() {
		t.Fatalf("independent supervisors share generation %d", pods.Generation())
	}
	deploymentMessage := supervisedWatchMsg{generation: deployments.Generation()}
	if pods.Owns(deploymentMessage) {
		t.Fatal("pod supervisor claimed a deployment supervisor message")
	}
	if !deployments.Owns(deploymentMessage) {
		t.Fatal("deployment supervisor rejected its own message")
	}
}

func TestWatchSupervisor_StopIsIdempotent(t *testing.T) {
	var sup watchSupervisor[service.Pod]
	cancelled := 0
	w := newFakeWatcher[service.Pod]()
	w.cancel = func() { cancelled++ }

	sup.Set(w)
	sup.Stop()
	sup.Stop()

	if cancelled != 1 {
		t.Errorf("Cancel should fire exactly once across Stop/Stop, got %d", cancelled)
	}
	if sup.Current() != nil {
		t.Error("Stop must clear Current")
	}
}

func TestWatchSupervisor_NextDelayMonotonic(t *testing.T) {
	var sup watchSupervisor[service.Pod]
	a := sup.nextDelay()
	b := sup.nextDelay()
	c := sup.nextDelay()
	if a > b || b > c {
		t.Errorf("backoff should be non-decreasing across calls, got %v %v %v", a, b, c)
	}
}

func TestWatchSupervisor_SetPreservesAttemptsUntilEventArrives(t *testing.T) {
	var sup watchSupervisor[service.Pod]
	w := newFakeWatcher[service.Pod]()
	sup.Set(w)
	sup.nextDelay()
	sup.nextDelay()

	w2 := newFakeWatcher[service.Pod]()
	sup.Set(w2)

	if got := sup.nextDelay(); got != reconnectBackoff[2] {
		t.Errorf("replacement reset retry state: delay = %v, want %v", got, reconnectBackoff[2])
	}
	sup.MarkHealthy()
	if got := sup.nextDelay(); got != reconnectBackoff[0] {
		t.Errorf("healthy event did not reset retry state: delay = %v, want %v", got, reconnectBackoff[0])
	}
}

func TestWatchSupervisor_FailedReconnectsAdvanceBackoffUntilHealthy(t *testing.T) {
	var sup watchSupervisor[service.Pod]
	sup.Set(newFakeWatcher[service.Pod]())

	for attempt, want := range reconnectBackoff[:3] {
		if got := sup.nextDelay(); got != want {
			t.Fatalf("failed reconnect %d delay = %v, want %v", attempt, got, want)
		}
		sup.Set(newFakeWatcher[service.Pod]())
	}

	sup.MarkHealthy()
	if got := sup.nextDelay(); got != reconnectBackoff[0] {
		t.Errorf("healthy activity did not reset delay: got %v, want %v", got, reconnectBackoff[0])
	}
}

type closeMsg struct{}

func TestWatchSupervisor_SetWithClose_EmitsCustomMsgOnClose(t *testing.T) {
	var sup watchSupervisor[service.Pod]
	w := newFakeWatcher[service.Pod]()
	cmd := sup.SetWithClose(w, closeMsg{})

	close(w.ch)

	got, ok := cmd().(supervisedWatchMsg)
	if !ok {
		t.Fatalf("expected supervised watch message; got %T", got)
	}
	if _, ok := got.payload.(closeMsg); !ok {
		t.Errorf("after channel close, expected closeMsg payload; got %T", got.payload)
	}
	if !sup.Owns(got) {
		t.Error("supervisor should own the message emitted by its current watcher")
	}
}

func TestWatchSupervisor_SetWithClose_PullThroughForEvents(t *testing.T) {
	var sup watchSupervisor[service.Pod]
	w := newFakeWatcher[service.Pod]()
	cmd := sup.SetWithClose(w, closeMsg{})

	w.ch <- service.WatchEvent[service.Pod]{Kind: service.WatchAdded, Item: service.Pod{Name: "p"}}

	got, ok := cmd().(supervisedWatchMsg)
	if !ok {
		t.Fatalf("expected supervised watch message, got %T", got)
	}
	wrapped, ok := got.payload.(service.WatchEventMsg[service.Pod])
	if !ok {
		t.Fatalf("expected WatchEventMsg[Pod] payload, got %T", got.payload)
	}
	if wrapped.Event.Item.Name != "p" {
		t.Errorf("unexpected pod name: %q", wrapped.Event.Item.Name)
	}
}

func TestWatchSupervisor_PullReturnsNilWhenIdle(t *testing.T) {
	var sup watchSupervisor[service.Pod]
	if cmd := sup.Pull(); cmd != nil {
		t.Error("Pull on idle supervisor should return nil")
	}
}

func TestWatchSupervisor_PullReturnsCmdWhenActive(t *testing.T) {
	var sup watchSupervisor[service.Pod]
	w := newFakeWatcher[service.Pod]()
	sup.Set(w)

	cmd := sup.Pull()
	if cmd == nil {
		t.Fatal("Pull with an active watcher should return a non-nil cmd")
	}
	w.ch <- service.WatchEvent[service.Pod]{Kind: service.WatchAdded}
	if msg := cmd(); msg == nil {
		t.Error("Pull cmd should produce a non-nil message")
	}
}

func TestReconnectAfter_ZeroDelayFiresImmediately(t *testing.T) {
	cmd := reconnectAfter(0, closeMsg{})
	got := cmd()
	if _, ok := got.(closeMsg); !ok {
		t.Errorf("zero-delay reconnect should emit msg directly; got %T", got)
	}
}

func TestReconnectAfter_NonZeroDelayUsesTeaTick(t *testing.T) {
	start := time.Now()
	cmd := reconnectAfter(50*time.Millisecond, closeMsg{})
	got := cmd()
	if got == nil {
		t.Fatal("reconnectAfter cmd returned nil msg")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("reconnectAfter blocked too long: %v", elapsed)
	}
}

func TestFreshContext_NotCancelled(t *testing.T) {
	ctx := freshContext()
	if ctx == nil {
		t.Fatal("freshContext returned nil")
	}
	select {
	case <-ctx.Done():
		t.Error("freshContext returned a cancelled context")
	default:
	}
	derived, cancel := context.WithCancel(ctx)
	cancel()
	<-derived.Done()
}
