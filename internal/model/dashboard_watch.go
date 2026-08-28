package model

import (
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/service"
)

// Activate refreshes cached data and starts dashboard watchers.
func (m *DashboardModel) Activate() tea.Cmd {
	defer m.syncDashboardLayout()
	m.active = true
	cmds := []tea.Cmd{
		m.refreshAll(),
		m.podWatcher.SetWithClose(
			service.WatchPods(freshContext(), m.namespace),
			dashboardPodWatchClosedMsg{},
		),
		m.deploymentWatcher.SetWithClose(
			service.WatchDeployments(freshContext(), m.namespace),
			dashboardDeploymentWatchClosedMsg{},
		),
		m.eventWatcher.SetWithClose(
			service.WatchEvents(freshContext(), m.namespace),
			dashboardEventWatchClosedMsg{},
		),
	}
	return tea.Batch(cmds...)
}

// Deactivate stops dashboard watchers.
func (m *DashboardModel) Deactivate() {
	m.active = false
	m.podWatcher.Stop()
	m.deploymentWatcher.Stop()
	m.eventWatcher.Stop()
}

func (m DashboardModel) handleSupervisedWatchMessage(msg supervisedWatchMsg) (DashboardModel, tea.Cmd) {
	switch {
	case m.podWatcher.Owns(msg):
		return m.handlePodWatchPayload(msg.payload)
	case m.deploymentWatcher.Owns(msg):
		return m.handleDeploymentWatchPayload(msg.payload)
	case m.eventWatcher.Owns(msg):
		return m.handleEventWatchPayload(msg.payload)
	default:
		return m, nil
	}
}

func (m DashboardModel) ownsSupervisedWatchMessage(msg supervisedWatchMsg) bool {
	return m.podWatcher.Owns(msg) || m.deploymentWatcher.Owns(msg) || m.eventWatcher.Owns(msg)
}

func (m DashboardModel) handlePodWatchPayload(payload tea.Msg) (DashboardModel, tea.Cmd) {
	switch msg := payload.(type) {
	case service.WatchEventMsg[service.Pod]:
		return m.handlePodWatchEvent(msg)
	case dashboardPodWatchClosedMsg:
		return m.handlePodWatchClosed()
	default:
		return m, nil
	}
}

func (m DashboardModel) handleDeploymentWatchPayload(payload tea.Msg) (DashboardModel, tea.Cmd) {
	switch msg := payload.(type) {
	case service.WatchEventMsg[service.Deployment]:
		return m.handleDeploymentWatchEvent(msg)
	case dashboardDeploymentWatchClosedMsg:
		return m.handleDeploymentWatchClosed()
	default:
		return m, nil
	}
}

func (m DashboardModel) handleEventWatchPayload(payload tea.Msg) (DashboardModel, tea.Cmd) {
	switch msg := payload.(type) {
	case service.WatchEventMsg[service.Event]:
		return m.handleEventWatchEvent(msg)
	case dashboardEventWatchClosedMsg:
		return m.handleEventWatchClosed()
	default:
		return m, nil
	}
}

func (m DashboardModel) handlePodWatchEvent(msg service.WatchEventMsg[service.Pod]) (DashboardModel, tea.Cmd) {
	switch msg.Event.Kind {
	case service.WatchErrored:
		if msg.Event.Err != nil {
			m.err = msg.Event.Err
		}
		return m, m.podWatcher.Pull()
	case service.WatchClosed:
		return m, m.podWatcher.Pull()
	case service.WatchBookmark:
		m.podWatcher.MarkHealthy()
		return m, m.podWatcher.Pull()
	case service.WatchAdded, service.WatchModified:
		m.pods = upsertByName(m.pods, msg.Event.Item, podKey)
	case service.WatchDeleted:
		m.pods = removeByName(m.pods, msg.Event.Item, podKey)
	}
	m.podWatcher.MarkHealthy()
	m.mergeMetrics()
	m.rebuildTableRows()
	m.loading = false
	return m, m.podWatcher.Pull()
}

func (m DashboardModel) handleDeploymentWatchEvent(msg service.WatchEventMsg[service.Deployment]) (DashboardModel, tea.Cmd) {
	switch msg.Event.Kind {
	case service.WatchErrored:
		if msg.Event.Err != nil && m.err == nil {
			m.err = msg.Event.Err
		}
		return m, m.deploymentWatcher.Pull()
	case service.WatchClosed:
		return m, m.deploymentWatcher.Pull()
	case service.WatchBookmark:
		m.deploymentWatcher.MarkHealthy()
		return m, m.deploymentWatcher.Pull()
	case service.WatchAdded, service.WatchModified:
		m.deployments = upsertByName(m.deployments, msg.Event.Item, deploymentKey)
	case service.WatchDeleted:
		m.deployments = removeByName(m.deployments, msg.Event.Item, deploymentKey)
	}
	m.deploymentWatcher.MarkHealthy()
	return m, m.deploymentWatcher.Pull()
}

func (m DashboardModel) handleEventWatchEvent(msg service.WatchEventMsg[service.Event]) (DashboardModel, tea.Cmd) {
	switch msg.Event.Kind {
	case service.WatchErrored:
		if msg.Event.Err != nil && m.err == nil {
			m.err = msg.Event.Err
		}
		return m, m.eventWatcher.Pull()
	case service.WatchClosed:
		return m, m.eventWatcher.Pull()
	case service.WatchBookmark:
		m.eventWatcher.MarkHealthy()
		return m, m.eventWatcher.Pull()
	case service.WatchAdded, service.WatchModified:
		m.events = upsertEvent(m.events, msg.Event.Item)
	case service.WatchDeleted:
		m.events = removeEvent(m.events, msg.Event.Item)
	}
	m.eventWatcher.MarkHealthy()
	m.events = trimEventsToRecent(m.events, dashboardEventCap)
	return m, m.eventWatcher.Pull()
}

const dashboardEventCap = 50

func eventKey(e service.Event) string {
	identifier := e.UID
	if identifier == "" {
		identifier = e.Name
	}
	if identifier == "" {
		identifier = e.Type + "\x00" + e.Reason + "\x00" + e.Object
	}
	return e.Namespace + "\x00" + identifier
}

func upsertEvent(events []service.Event, item service.Event) []service.Event {
	target := eventKey(item)
	for i := range events {
		if eventKey(events[i]) == target {
			updated := slices.Clone(events)
			updated[i] = item
			return updated
		}
	}
	return append(slices.Clone(events), item)
}

func removeEvent(events []service.Event, item service.Event) []service.Event {
	target := eventKey(item)
	for i := range events {
		if eventKey(events[i]) == target {
			updated := slices.Clone(events)
			copy(updated[i:], updated[i+1:])
			return updated[:len(updated)-1]
		}
	}
	return events
}

func trimEventsToRecent(events []service.Event, limit int) []service.Event {
	if limit <= 0 {
		return nil
	}
	trimmed := slices.Clone(events)
	slices.SortFunc(trimmed, func(left, right service.Event) int {
		return right.LastTimestamp.Compare(left.LastTimestamp)
	})
	return trimmed[:min(len(trimmed), limit)]
}

func (m DashboardModel) handlePodWatchClosed() (DashboardModel, tea.Cmd) {
	if !m.active {
		return m, nil
	}
	return m, reconnectAfter(m.podWatcher.nextDelay(), dashboardPodReconnectMsg{
		namespace:  m.namespace,
		generation: m.podWatcher.Generation(),
	})
}

func (m DashboardModel) handleDeploymentWatchClosed() (DashboardModel, tea.Cmd) {
	if !m.active {
		return m, nil
	}
	return m, reconnectAfter(m.deploymentWatcher.nextDelay(), dashboardDeploymentReconnectMsg{
		namespace:  m.namespace,
		generation: m.deploymentWatcher.Generation(),
	})
}

func (m DashboardModel) handleEventWatchClosed() (DashboardModel, tea.Cmd) {
	if !m.active {
		return m, nil
	}
	return m, reconnectAfter(m.eventWatcher.nextDelay(), dashboardEventReconnectMsg{
		namespace:  m.namespace,
		generation: m.eventWatcher.Generation(),
	})
}
