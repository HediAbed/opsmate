package ui

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/HediAbed/opsmate/internal/cluster"
)

type testResourceLiveSet[T interface{}] struct {
	changes  chan struct{}
	state    resourceLiveState[T]
	stopOnce sync.Once
	stops    atomic.Int32
}

func newTestResourceLiveSet[T interface{}](state resourceLiveState[T]) *testResourceLiveSet[T] {
	return &testResourceLiveSet[T]{changes: make(chan struct{}, 1), state: state}
}

func (s *testResourceLiveSet[T]) Changes() <-chan struct{} {
	return s.changes
}

func (s *testResourceLiveSet[T]) State() resourceLiveState[T] {
	return s.state
}

func (s *testResourceLiveSet[T]) Stop() {
	s.stopOnce.Do(func() {
		s.stops.Add(1)
		close(s.changes)
	})
}

func (s *testResourceLiveSet[T]) signal() {
	s.changes <- struct{}{}
}

func TestLiveSupervisorDeliversSnapshots(t *testing.T) {
	set := newTestResourceLiveSet(resourceLiveState[cluster.Pod]{
		Ready: true,
		Items: []cluster.Pod{{Name: "api"}},
	})
	var supervisor liveSupervisor[cluster.Pod]
	command := supervisor.Set(set)
	if command == nil || supervisor.Current() != set || supervisor.Generation() == 0 {
		t.Fatal("Set() did not activate the live set")
	}
	set.signal()
	message := command().(supervisedLiveMsg)
	if !supervisor.Owns(message) || message.closed {
		t.Fatalf("snapshot message = %+v", message)
	}
	snapshot := message.payload.(liveSnapshotMsg[cluster.Pod])
	if len(snapshot.State.Items) != 1 || snapshot.State.Items[0].Name != "api" {
		t.Fatalf("snapshot = %+v", snapshot.State)
	}
	supervisor.Stop()
}

func TestLiveSupervisorReportsClosureAndStops(t *testing.T) {
	set := newTestResourceLiveSet(resourceLiveState[cluster.Pod]{})
	var supervisor liveSupervisor[cluster.Pod]
	if command := supervisor.Set(set); command == nil {
		t.Fatal("Set() did not activate the live set")
	}
	closeCommand := supervisor.Pull()
	set.Stop()
	closed := closeCommand().(supervisedLiveMsg)
	if !closed.closed || !supervisor.Owns(closed) {
		t.Fatalf("closed message = %+v", closed)
	}
	supervisor.Stop()
	if supervisor.Current() != nil || supervisor.Generation() != 0 {
		t.Fatal("Stop() retained the live set")
	}
}

func TestLiveSupervisorReplacementRejectsStaleMessages(t *testing.T) {
	first := newTestResourceLiveSet(resourceLiveState[cluster.Pod]{})
	second := newTestResourceLiveSet(resourceLiveState[cluster.Pod]{})
	var supervisor liveSupervisor[cluster.Pod]
	supervisor.Set(first)
	firstGeneration := supervisor.Generation()
	supervisor.Set(second)
	if first.stops.Load() != 1 {
		t.Fatalf("first stop count = %d, want one", first.stops.Load())
	}
	if supervisor.OwnsGeneration(firstGeneration) {
		t.Fatal("replacement retained the old generation")
	}
	if !supervisor.OwnsGeneration(supervisor.Generation()) {
		t.Fatal("replacement does not own its current generation")
	}
	supervisor.Stop()
	supervisor.Stop()
	if second.stops.Load() != 1 {
		t.Fatalf("second stop count = %d, want one", second.stops.Load())
	}
}

func TestLiveSupervisorHandlesIdleAndNilSets(t *testing.T) {
	var supervisor liveSupervisor[cluster.Pod]
	if command := supervisor.Pull(); command != nil {
		t.Fatal("idle Pull() returned a command")
	}
	if command := supervisor.Set(nil); command != nil || supervisor.Current() != nil {
		t.Fatal("Set(nil) activated a live set")
	}
	if supervisor.OwnsGeneration(0) || supervisor.Owns(supervisedLiveMsg{generation: 1}) {
		t.Fatal("idle supervisor claimed a generation")
	}
}

func TestLiveGenerationsAreUniqueAndSkipZero(t *testing.T) {
	original := liveGenerationSequence.Load()
	defer liveGenerationSequence.Store(original)
	liveGenerationSequence.Store(^uint64(0))
	if generation := nextLiveGeneration(); generation == 0 {
		t.Fatal("nextLiveGeneration() returned zero after overflow")
	}

	pods := newTestResourceLiveSet(resourceLiveState[cluster.Pod]{})
	deployments := newTestResourceLiveSet(resourceLiveState[cluster.Deployment]{})
	var podSupervisor liveSupervisor[cluster.Pod]
	var deploymentSupervisor liveSupervisor[cluster.Deployment]
	podSupervisor.Set(pods)
	deploymentSupervisor.Set(deployments)
	if podSupervisor.Generation() == deploymentSupervisor.Generation() {
		t.Fatal("different supervisors received the same generation")
	}
	podSupervisor.Stop()
	deploymentSupervisor.Stop()
}
