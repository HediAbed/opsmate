package screen

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/HediAbed/opsmate/internal/cluster"
	clusterui "github.com/HediAbed/opsmate/internal/ui/cluster"
)

type testResourceLiveSet[T interface{}] struct {
	changes  chan struct{}
	state    clusterui.LiveState[T]
	stopOnce sync.Once
	stops    atomic.Int32
}

func newTestResourceLiveSet[T interface{}](state clusterui.LiveState[T]) *testResourceLiveSet[T] {
	return &testResourceLiveSet[T]{changes: make(chan struct{}, 1), state: state}
}

func (s *testResourceLiveSet[T]) Changes() <-chan struct{} {
	return s.changes
}

func (s *testResourceLiveSet[T]) State() clusterui.LiveState[T] {
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
	set := newTestResourceLiveSet(clusterui.LiveState[cluster.Pod]{
		Ready: true,
		Items: []cluster.Pod{{Name: "api"}},
	})
	var supervisor LiveSupervisor[cluster.Pod]
	command := supervisor.Set(set)
	if command == nil || supervisor.Current() != set || supervisor.Generation() == 0 {
		t.Fatal("Set() did not activate the live set")
	}
	set.signal()
	message := command().(LiveMessage)
	if !supervisor.Owns(message) || message.Closed {
		t.Fatalf("snapshot message = %+v", message)
	}
	snapshot := message.Payload.(LiveSnapshot[cluster.Pod])
	if len(snapshot.State.Items) != 1 || snapshot.State.Items[0].Name != "api" {
		t.Fatalf("snapshot = %+v", snapshot.State)
	}
	supervisor.Stop()
}

func TestLiveSupervisorReportsClosureAndStops(t *testing.T) {
	set := newTestResourceLiveSet(clusterui.LiveState[cluster.Pod]{})
	var supervisor LiveSupervisor[cluster.Pod]
	if command := supervisor.Set(set); command == nil {
		t.Fatal("Set() did not activate the live set")
	}
	closeCommand := supervisor.Pull()
	set.Stop()
	closed := closeCommand().(LiveMessage)
	if !closed.Closed || !supervisor.Owns(closed) {
		t.Fatalf("closed message = %+v", closed)
	}
	supervisor.Stop()
	if supervisor.Current() != nil || supervisor.Generation() != 0 {
		t.Fatal("Stop() retained the live set")
	}
}

func TestLiveSupervisorReplacementRejectsStaleMessages(t *testing.T) {
	first := newTestResourceLiveSet(clusterui.LiveState[cluster.Pod]{})
	second := newTestResourceLiveSet(clusterui.LiveState[cluster.Pod]{})
	var supervisor LiveSupervisor[cluster.Pod]
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
	var supervisor LiveSupervisor[cluster.Pod]
	if command := supervisor.Pull(); command != nil {
		t.Fatal("idle Pull() returned a command")
	}
	if command := supervisor.Set(nil); command != nil || supervisor.Current() != nil {
		t.Fatal("Set(nil) activated a live set")
	}
	if supervisor.OwnsGeneration(0) || supervisor.Owns(LiveMessage{Generation: 1}) {
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

	pods := newTestResourceLiveSet(clusterui.LiveState[cluster.Pod]{})
	deployments := newTestResourceLiveSet(clusterui.LiveState[cluster.Deployment]{})
	var podSupervisor LiveSupervisor[cluster.Pod]
	var deploymentSupervisor LiveSupervisor[cluster.Deployment]
	podSupervisor.Set(pods)
	deploymentSupervisor.Set(deployments)
	if podSupervisor.Generation() == deploymentSupervisor.Generation() {
		t.Fatal("different supervisors received the same generation")
	}
	podSupervisor.Stop()
	deploymentSupervisor.Stop()
}
