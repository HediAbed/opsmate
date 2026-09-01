package dashboard

import (
	"errors"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/ui/screen"
)

const dashboardLiveSetCount = 3

func (m *DashboardModel) Activate() tea.Cmd {
	defer m.syncDashboardLayout()
	m.active = true
	return m.refreshAll()
}

func (m *DashboardModel) Deactivate() {
	m.active = false
	m.stopDashboardLiveSets()
}

func (m *DashboardModel) stopDashboardLiveSets() {
	m.podLive.Stop()
	m.deploymentLive.Stop()
	m.eventLive.Stop()
}

func (m *DashboardModel) startDashboardLiveSets() []tea.Cmd {
	commands := make([]tea.Cmd, 0, dashboardLiveSetCount)
	if command := m.startDashboardPodLiveSet(); command != nil {
		commands = append(commands, command)
	}
	if command := m.startDashboardDeploymentLiveSet(); command != nil {
		commands = append(commands, command)
	}
	if command := m.startDashboardEventLiveSet(); command != nil {
		commands = append(commands, command)
	}
	return commands
}

func (m *DashboardModel) startDashboardPodLiveSet() tea.Cmd {
	set, err := m.cluster.ObservePods(m.namespace)
	if err != nil {
		m.podLiveError = err
		m.loading = false
		m.syncDashboardLiveError()
		return nil
	}
	return m.podLive.Set(set)
}

func (m *DashboardModel) startDashboardDeploymentLiveSet() tea.Cmd {
	set, err := m.cluster.ObserveDeployments(m.namespace)
	if err != nil {
		m.deploymentLiveError = err
		m.syncDashboardLiveError()
		return nil
	}
	return m.deploymentLive.Set(set)
}

func (m *DashboardModel) startDashboardEventLiveSet() tea.Cmd {
	set, err := m.cluster.ObserveEvents(m.namespace)
	if err != nil {
		m.eventLiveError = err
		m.syncDashboardLiveError()
		return nil
	}
	return m.eventLive.Set(set)
}

func (m *DashboardModel) syncDashboardLiveError() {
	m.err = errors.Join(m.podLiveError, m.deploymentLiveError, m.eventLiveError)
}

func (m DashboardModel) handleSupervisedLiveMessage(message screen.LiveMessage) (DashboardModel, tea.Cmd) {
	switch {
	case m.podLive.Owns(message):
		if message.Closed {
			return m.handleDashboardLiveSetClosed(dashboardPods)
		}
		return m.handlePodLivePayload(message.Payload)
	case m.deploymentLive.Owns(message):
		if message.Closed {
			return m.handleDashboardLiveSetClosed(dashboardDeployments)
		}
		return m.handleDeploymentLivePayload(message.Payload)
	case m.eventLive.Owns(message):
		if message.Closed {
			return m.handleDashboardLiveSetClosed(dashboardEvents)
		}
		return m.handleEventLivePayload(message.Payload)
	default:
		return m, nil
	}
}

func (m DashboardModel) OwnsLiveMessage(message screen.LiveMessage) bool {
	return m.podLive.Owns(message) || m.deploymentLive.Owns(message) || m.eventLive.Owns(message)
}

func (m DashboardModel) handlePodLivePayload(payload tea.Msg) (DashboardModel, tea.Cmd) {
	message, ok := payload.(screen.LiveSnapshot[cluster.Pod])
	if !ok {
		return m, nil
	}
	m.podLiveError = message.State.Err
	m.syncDashboardLiveError()
	if message.State.Err != nil {
		m.loading = false
		return m, m.podLive.Pull()
	}
	if message.State.Ready {
		m.pods = slices.Clone(message.State.Items)
		m.mergeMetrics()
		m.rebuildTableRows()
		m.loading = false
		m.lastRefresh = time.Now()
	}
	return m, m.podLive.Pull()
}

func (m DashboardModel) handleDeploymentLivePayload(payload tea.Msg) (DashboardModel, tea.Cmd) {
	message, ok := payload.(screen.LiveSnapshot[cluster.Deployment])
	if !ok {
		return m, nil
	}
	m.deploymentLiveError = message.State.Err
	m.syncDashboardLiveError()
	if message.State.Err != nil {
		return m, m.deploymentLive.Pull()
	}
	if message.State.Ready {
		m.deployments = slices.Clone(message.State.Items)
	}
	return m, m.deploymentLive.Pull()
}

func (m DashboardModel) handleEventLivePayload(payload tea.Msg) (DashboardModel, tea.Cmd) {
	message, ok := payload.(screen.LiveSnapshot[cluster.Event])
	if !ok {
		return m, nil
	}
	m.eventLiveError = message.State.Err
	m.syncDashboardLiveError()
	if message.State.Err != nil {
		return m, m.eventLive.Pull()
	}
	if message.State.Ready {
		m.events = trimEventsToRecent(message.State.Items, dashboardEventCap)
	}
	return m, m.eventLive.Pull()
}

func (m DashboardModel) handleDashboardLiveSetClosed(kind dashboardDataKind) (DashboardModel, tea.Cmd) {
	switch kind {
	case dashboardPods:
		m.podLive.Stop()
		m.podLiveError = screen.ErrLiveUpdatesStopped
		m.loading = false
	case dashboardDeployments:
		m.deploymentLive.Stop()
		m.deploymentLiveError = screen.ErrLiveUpdatesStopped
	case dashboardEvents:
		m.eventLive.Stop()
		m.eventLiveError = screen.ErrLiveUpdatesStopped
	case dashboardMetrics, dashboardDataKindCount:
		return m, nil
	}
	m.syncDashboardLiveError()
	return m, nil
}

const dashboardEventCap = 50

func trimEventsToRecent(events []cluster.Event, limit int) []cluster.Event {
	if limit <= 0 {
		return nil
	}
	trimmed := slices.Clone(events)
	slices.SortFunc(trimmed, func(left, right cluster.Event) int {
		return right.LastTimestamp.Compare(left.LastTimestamp)
	})
	return trimmed[:min(len(trimmed), limit)]
}
