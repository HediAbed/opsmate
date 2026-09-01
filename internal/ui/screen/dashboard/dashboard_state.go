package dashboard

import (
	tea "charm.land/bubbletea/v2"
	"github.com/HediAbed/opsmate/internal/analysis"
	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/ui/screen"
)

func (m *DashboardModel) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.recalcLayout()
}

func (m DashboardModel) Namespace() string { return m.namespace }

func (m DashboardModel) Size() (int, int) {
	return m.width, m.height
}

func (m DashboardModel) Active() bool {
	return m.active
}

func (m DashboardModel) Loading() bool {
	return m.loading
}

func (m DashboardModel) Error() error {
	return m.err
}

func (m *DashboardModel) Refresh() tea.Cmd {
	return m.refreshAll()
}

func (m DashboardModel) SearchItems() []screen.SearchItem {
	items := make([]screen.SearchItem, 0, len(m.pods)+len(m.deployments))
	for _, pod := range m.pods {
		items = append(items, screen.SearchItem{Kind: screen.ResourceKindPod, Name: pod.Name, Namespace: pod.Namespace})
	}
	for _, deployment := range m.deployments {
		items = append(items, screen.SearchItem{Kind: screen.ResourceKindDeployment, Name: deployment.Name, Namespace: deployment.Namespace})
	}
	return items
}

func (m DashboardModel) AnalysisContext() string {
	return analysis.BuildDashboardContext(analysis.DashboardContextInput{
		Namespace:   m.namespace,
		Pods:        m.pods,
		Deployments: m.deployments,
		Events:      m.events,
	})
}

func (m DashboardModel) Accepts(message tea.Msg) bool {
	switch message := message.(type) {
	case dashboardResultMsg:
		return m.acceptsResult(message)
	case dashboardHealthResultMsg:
		return message.requestID == m.healthRequestID && message.namespace == m.namespace
	case screen.LiveMessage:
		return m.OwnsLiveMessage(message)
	case dashMetricsTickMsg:
		return true
	default:
		return false
	}
}

func (m DashboardModel) ContextChangedBy(message tea.Msg) bool {
	switch message := message.(type) {
	case dashboardResultMsg:
		return m.acceptsResult(message)
	case screen.LiveMessage:
		return m.OwnsLiveMessage(message)
	default:
		return false
	}
}

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

func (m DashboardModel) selectedPodIdentity() (podIdentity, bool) {
	index := m.podTable.Cursor()
	if index < 0 || index >= len(m.podRows) {
		return podIdentity{}, false
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
	command := m.analysis.ClusterHealth(context)
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
