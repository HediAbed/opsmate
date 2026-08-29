package ui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/HediAbed/opsmate/internal/analysis"
	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/kube"
)

func (m *LogsModel) SetNamespace(namespace string) {
	m.namespace = namespace
	m.selectedPod = ""
	m.selectedPodNamespace = ""
	m.resetContainerSelection()
	m.podListRequestID++
	m.logRequestID++
	m.containerRequestID++
	m.resetExplanation()
	m.allLines = nil
	m.filteredLines = nil
	m.resetColorizeCache()
	m.logView.SetContent("")
}

func (m *LogsModel) fetchPods() tea.Cmd {
	m.podListRequestID++
	requestID := m.podListRequestID
	namespace := m.namespace
	command := m.cluster.FetchPods(namespace)
	return func() tea.Msg {
		return logPodsResultMsg{requestID: requestID, namespace: namespace, payload: command()}
	}
}

func (m LogsModel) SelectedPod() string {
	return m.selectedPod
}

// selectedPodNS resolves namespace in scoped and cluster-wide views.
func (m LogsModel) selectedPodNS() string {
	if m.selectedPodNamespace != "" {
		return m.selectedPodNamespace
	}
	if m.namespace != "" {
		return m.namespace
	}
	for _, pod := range m.pods {
		if pod.Name == m.selectedPod {
			return pod.Namespace
		}
	}
	return m.namespace
}

func (m *LogsModel) SetPod(name string) LogsModel {
	m.setPodIdentity(name, m.namespace)
	return *m
}

func (m *LogsModel) SetPodInNamespace(name, namespace string) {
	m.setPodIdentity(name, namespace)
}

func (m *LogsModel) setPodIdentity(name, namespace string) {
	changed := m.selectedPod != name || m.selectedPodNS() != namespace
	m.selectedPod = name
	m.selectedPodNamespace = namespace
	if changed {
		m.resetContainerSelection()
	}
	m.logRequestID++
	m.containerRequestID++
	m.resetExplanation()
	m.loading = true
	m.allLines = nil
	m.filteredLines = nil
	m.resetColorizeCache()
	m.logView.SetContent("")
}

func (m *LogsModel) SetPodCmd() tea.Cmd {
	if m.selectedPod == "" {
		return nil
	}
	return m.fetchSelectedLogs()
}

func (m *LogsModel) selectPod(pod cluster.Pod) {
	m.setPodIdentity(pod.Name, pod.Namespace)
}

func (m *LogsModel) resetContainerSelection() {
	m.containers = nil
	m.selectedContainer = ""
	m.showContainerPopup = false
	m.containerCursor = 0
}

func (m LogsModel) selectedPodIdentity() resourceIdentity {
	return resourceIdentity{Kind: resourceKindPod, Namespace: m.selectedPodNS(), Name: m.selectedPod}
}

func (m *LogsModel) fetchSelectedLogs() tea.Cmd {
	m.logRequestID++
	requestID := m.logRequestID
	pod := m.selectedPodIdentity()
	container := m.selectedContainer
	command := m.operations.FetchPodLogs(kube.PodLogRequest{
		Pod:       kube.PodReference{Namespace: pod.Namespace, Name: pod.Name},
		Container: container,
		TailLines: int64(m.tailLines),
	})
	return func() tea.Msg {
		return logsResultMsg{
			requestID: requestID,
			pod:       pod,
			container: container,
			payload:   command(),
		}
	}
}

func (m LogsModel) acceptsLogResult(msg logsResultMsg) bool {
	return msg.requestID == m.logRequestID &&
		msg.pod == m.selectedPodIdentity() &&
		msg.container == m.selectedContainer
}

func (m *LogsModel) fetchContainers() tea.Cmd {
	m.containerRequestID++
	requestID := m.containerRequestID
	pod := m.selectedPodIdentity()
	command := m.operations.FetchPodContainers(kube.PodReference{Namespace: pod.Namespace, Name: pod.Name})
	return func() tea.Msg {
		return containersResultMsg{requestID: requestID, pod: pod, payload: command()}
	}
}

func (m LogsModel) acceptsContainersResult(msg containersResultMsg) bool {
	return msg.requestID == m.containerRequestID && msg.pod == m.selectedPodIdentity()
}

func (m *LogsModel) explainSelectedLine(line, context string) tea.Cmd {
	m.explainRequestID++
	requestID := m.explainRequestID
	pod := m.selectedPodIdentity()
	container := m.selectedContainer
	command := analysis.ExplainLogLine(line, context, pod.Name)
	return func() tea.Msg {
		return logExplainResultMsg{
			requestID: requestID,
			pod:       pod,
			container: container,
			line:      line,
			payload:   command(),
		}
	}
}

func (m LogsModel) acceptsExplanation(msg logExplainResultMsg) bool {
	return m.inspectMode &&
		msg.requestID == m.explainRequestID &&
		msg.pod == m.selectedPodIdentity() &&
		msg.container == m.selectedContainer &&
		m.selectedInspectLine() == msg.line
}

func (m LogsModel) selectedInspectLine() string {
	if m.lineCursor < 0 || m.lineCursor >= len(m.filteredLines) {
		return ""
	}
	return m.filteredLines[m.lineCursor]
}

func (m *LogsModel) resetExplanation() {
	m.explainRequestID++
	m.lineExplanation = ""
	m.lineExplanationErr = nil
	m.lineExplanationLoading = false
}
