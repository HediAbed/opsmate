package screen

import (
	"errors"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"

	clusterui "github.com/HediAbed/opsmate/internal/ui/cluster"
)

var ErrLiveUpdatesStopped = errors.New("cluster resource updates stopped")

type LiveSnapshot[T interface{}] struct {
	State clusterui.LiveState[T]
}

type LiveMessage struct {
	Generation uint64
	Payload    tea.Msg
	Closed     bool
}

type LiveSupervisor[T interface{}] struct {
	current    clusterui.LiveSet[T]
	generation uint64
}

var liveGenerationSequence atomic.Uint64

func (s *LiveSupervisor[T]) Set(set clusterui.LiveSet[T]) tea.Cmd {
	s.Stop()
	if set == nil {
		return nil
	}
	s.current = set
	s.generation = nextLiveGeneration()
	return s.nextSnapshotCommand(set)
}

func (s *LiveSupervisor[T]) Stop() {
	if s.current != nil {
		s.current.Stop()
	}
	s.current = nil
	s.generation = 0
}

func (s *LiveSupervisor[T]) Current() clusterui.LiveSet[T] {
	return s.current
}

func (s *LiveSupervisor[T]) Owns(message LiveMessage) bool {
	return s.OwnsGeneration(message.Generation)
}

func (s *LiveSupervisor[T]) OwnsGeneration(generation uint64) bool {
	return s.current != nil && generation != 0 && generation == s.generation
}

func (s *LiveSupervisor[T]) Generation() uint64 {
	return s.generation
}

func (s *LiveSupervisor[T]) Pull() tea.Cmd {
	if s.current == nil {
		return nil
	}
	return s.nextSnapshotCommand(s.current)
}

func (s *LiveSupervisor[T]) nextSnapshotCommand(set clusterui.LiveSet[T]) tea.Cmd {
	generation := s.generation
	return func() tea.Msg {
		_, open := <-set.Changes()
		if !open {
			return LiveMessage{Generation: generation, Closed: true}
		}
		return LiveMessage{
			Generation: generation,
			Payload:    LiveSnapshot[T]{State: set.State()},
		}
	}
}

func nextLiveGeneration() uint64 {
	for {
		generation := liveGenerationSequence.Add(1)
		if generation != 0 {
			return generation
		}
	}
}
