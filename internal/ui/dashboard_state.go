package ui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/HediAbed/opsmate/internal/analysis"
	"github.com/HediAbed/opsmate/internal/cluster"
)

func (m *DashboardModel) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.recalcLayout()
}

func (m DashboardModel) Namespace() string { return m.namespace }

func (m *DashboardModel) SetNamespace(namespace string) tea.Cmd {
	defer m.syncDashboardLayout()
	wasActive := m.active
	m.Deactivate()

	m.namespace = namespace
	m.pods = nil
	m.deployments = nil
	m.events = nil
	m.metrics = nil
	m.loading = true
	m.err = nil
	m.healthRequestID++
	m.healthAnalysisLoading = false
	m.healthAnalysisSummary = ""
	m.healthAnalysisErr = nil
	m.rebuildTableRows()

	if wasActive {
		return m.Activate()
	}
	return m.refreshAll()
}

func (m DashboardModel) SelectedPod() string {
	identity, ok := m.selectedPodIdentity()
	if !ok {
		return ""
	}
	return identity.Name
}

func (m DashboardModel) SelectedPodNS() string {
	identity, ok := m.selectedPodIdentity()
	if !ok {
		return m.namespace
	}
	return identity.Namespace
}

func (m DashboardModel) selectedPodIdentity() (resourceIdentity, bool) {
	index := m.podTable.Cursor()
	if index < 0 || index >= len(m.podRows) {
		return resourceIdentity{}, false
	}
	return m.podRows[index], true
}

func (m *DashboardModel) refreshAll() tea.Cmd {
	m.loading = true
	commands := []tea.Cmd{m.fetchDashboardData(dashboardMetrics, m.cluster.FetchPodMetrics(m.namespace))}
	if m.active {
		m.stopDashboardLiveSets()
		commands = append(commands, m.startDashboardLiveSets()...)
	}
	return tea.Batch(commands...)
}

func (m *DashboardModel) fetchDashboardData(kind dashboardDataKind, command tea.Cmd) tea.Cmd {
	m.requestIDs[kind]++
	requestID := m.requestIDs[kind]
	namespace := m.namespace
	return func() tea.Msg {
		return dashboardResultMsg{kind: kind, requestID: requestID, namespace: namespace, payload: command()}
	}
}

func (m *DashboardModel) fetchHealthSummary() tea.Cmd {
	m.healthRequestID++
	requestID := m.healthRequestID
	namespace := m.namespace
	context := analysis.BuildDashboardContext(analysis.DashboardContextInput{
		Namespace:   namespace,
		Pods:        m.pods,
		Deployments: m.deployments,
		Events:      m.events,
	})
	command := analysis.ClusterHealth(context)
	return func() tea.Msg {
		return dashboardHealthResultMsg{requestID: requestID, namespace: namespace, payload: command()}
	}
}

func (m DashboardModel) acceptsResult(msg dashboardResultMsg) bool {
	return msg.kind < dashboardDataKindCount &&
		msg.namespace == m.namespace &&
		msg.requestID == m.requestIDs[msg.kind]
}

func (m *DashboardModel) mergeMetrics() {
	lookup := make(map[string]cluster.PodMetric, len(m.metrics))
	for _, pm := range m.metrics {
		namespace := pm.Namespace
		if namespace == "" {
			namespace = m.namespace
		}
		lookup[namespacedResourceKey(namespace, pm.Name)] = pm
	}
	for index := range m.pods {
		namespace := m.pods[index].Namespace
		if namespace == "" {
			namespace = m.namespace
		}
		key := namespacedResourceKey(namespace, m.pods[index].Name)
		if pm, ok := lookup[key]; ok {
			m.pods[index].CPU = pm.CPU
			m.pods[index].Memory = pm.Memory
		}
	}
}
