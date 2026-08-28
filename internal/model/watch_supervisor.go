package model

import (
	"context"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/service"
)

// reconnectBackoff limits retry pressure during cluster outages.
var reconnectBackoff = []time.Duration{
	0,
	1 * time.Second,
	5 * time.Second,
	30 * time.Second,
}

var watchGenerationSequence atomic.Uint64

func reconnectCappedDelay(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	if attempt >= len(reconnectBackoff) {
		return reconnectBackoff[len(reconnectBackoff)-1]
	}
	return reconnectBackoff[attempt]
}

// reconnectAfter schedules without blocking the event loop.
func reconnectAfter[Msg tea.Msg](delay time.Duration, msg Msg) tea.Cmd {
	if delay <= 0 {
		return func() tea.Msg { return msg }
	}
	return tea.Tick(delay, func(time.Time) tea.Msg { return msg })
}

// watchSupervisor owns one typed watcher and its reconnect state.
type watchSupervisor[T service.WatchResource] struct {
	current    service.Watcher[T]
	attempts   int
	generation uint64
	active     bool
	onClose    tea.Msg
}

type supervisedWatchMsg struct {
	generation uint64
	payload    tea.Msg
}

// Stop clears the watcher and reconnect state.
func (s *watchSupervisor[T]) Stop() {
	s.cancelCurrent()
	s.attempts = 0
	s.active = false
}

// Set replaces the current watcher and requests its next event.
func (s *watchSupervisor[T]) Set(w service.Watcher[T]) tea.Cmd {
	s.cancelCurrent()
	s.current = w
	s.generation = nextWatchGeneration()
	s.active = true
	return s.nextEventCmd(w)
}

func nextWatchGeneration() uint64 {
	for {
		generation := watchGenerationSequence.Add(1)
		if generation != 0 {
			return generation
		}
	}
}

// SetWithClose replaces the generic close event with onClose.
func (s *watchSupervisor[T]) SetWithClose(w service.Watcher[T], onClose tea.Msg) tea.Cmd {
	s.onClose = onClose
	return s.Set(w)
}

// Current returns the active watcher.
func (s *watchSupervisor[T]) Current() service.Watcher[T] {
	return s.current
}

func (s *watchSupervisor[T]) nextDelay() time.Duration {
	d := reconnectCappedDelay(s.attempts)
	s.attempts++
	return d
}

func (s *watchSupervisor[T]) MarkHealthy() {
	s.attempts = 0
}

func (s *watchSupervisor[T]) Owns(msg supervisedWatchMsg) bool {
	return s.OwnsGeneration(msg.generation)
}

func (s *watchSupervisor[T]) OwnsGeneration(generation uint64) bool {
	return s.active && generation != 0 && generation == s.generation
}

func (s *watchSupervisor[T]) Generation() uint64 {
	if !s.active {
		return 0
	}
	return s.generation
}

// Pull requests the next event from the active watcher.
func (s *watchSupervisor[T]) Pull() tea.Cmd {
	if s.current == nil {
		return nil
	}
	return s.nextEventCmd(s.current)
}

func (s *watchSupervisor[T]) nextEventCmd(w service.Watcher[T]) tea.Cmd {
	base := service.NextWatchEvent(w)
	customClose := s.onClose
	generation := s.generation
	return func() tea.Msg {
		payload := base()
		if _, ok := payload.(service.WatchClosedMsg); ok && customClose != nil {
			payload = customClose
		}
		return supervisedWatchMsg{generation: generation, payload: payload}
	}
}

func (s *watchSupervisor[T]) cancelCurrent() {
	if s.current == nil {
		return
	}
	s.current.Cancel()
	s.current = nil
}

func freshContext() context.Context {
	return context.Background()
}
