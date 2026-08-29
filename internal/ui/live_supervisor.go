package ui

import (
	"sync/atomic"

	tea "charm.land/bubbletea/v2"
	"github.com/HediAbed/opsmate/internal/cluster"
)

type liveResource interface {
	cluster.Pod | cluster.Deployment | cluster.Event | cluster.Ingress |
		cluster.NetworkPolicy | cluster.PersistentVolumeClaim | cluster.CronJob |
		cluster.HPA | cluster.Secret | cluster.ReplicaSet
}

type liveSnapshotMsg[T liveResource] struct {
	State resourceLiveState[T]
}

type supervisedLiveMsg struct {
	generation uint64
	payload    tea.Msg
	closed     bool
}

type liveSupervisor[T liveResource] struct {
	current    resourceLiveSet[T]
	generation uint64
}

var liveGenerationSequence atomic.Uint64

func (s *liveSupervisor[T]) Set(set resourceLiveSet[T]) tea.Cmd {
	s.Stop()
	if set == nil {
		return nil
	}
	s.current = set
	s.generation = nextLiveGeneration()
	return s.nextSnapshotCommand(set)
}

func (s *liveSupervisor[T]) Stop() {
	if s.current != nil {
		s.current.Stop()
	}
	s.current = nil
	s.generation = 0
}

func (s *liveSupervisor[T]) Current() resourceLiveSet[T] {
	return s.current
}

func (s *liveSupervisor[T]) Owns(message supervisedLiveMsg) bool {
	return s.OwnsGeneration(message.generation)
}

func (s *liveSupervisor[T]) OwnsGeneration(generation uint64) bool {
	return s.current != nil && generation != 0 && generation == s.generation
}

func (s *liveSupervisor[T]) Generation() uint64 {
	return s.generation
}

func (s *liveSupervisor[T]) Pull() tea.Cmd {
	if s.current == nil {
		return nil
	}
	return s.nextSnapshotCommand(s.current)
}

func (s *liveSupervisor[T]) nextSnapshotCommand(set resourceLiveSet[T]) tea.Cmd {
	generation := s.generation
	return func() tea.Msg {
		_, open := <-set.Changes()
		if !open {
			return supervisedLiveMsg{generation: generation, closed: true}
		}
		return supervisedLiveMsg{
			generation: generation,
			payload:    liveSnapshotMsg[T]{State: set.State()},
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
