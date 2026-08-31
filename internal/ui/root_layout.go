package ui

import (
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/HediAbed/opsmate/internal/analysis"
	"github.com/HediAbed/opsmate/internal/failure"
)

var ErrUnsupportedScreen = errors.New("screen is not supported")

type ScreenStateError struct {
	Screen screenID
}

func (e ScreenStateError) Error() string {
	return fmt.Sprintf("screen %d is not supported", e.Screen)
}

func (ScreenStateError) Unwrap() error {
	return ErrUnsupportedScreen
}

func (ScreenStateError) FailureCode() failure.Code {
	return failure.CodeInternal
}

func (m *RootModel) resizeChildren() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	contentHeight := m.height - lipgloss.Height(m.renderStatusBar()) - lipgloss.Height(m.renderRootError())
	contentHeight = max(rootMinimumContentHeight, contentHeight)
	mainWidth := m.rootMainWidth()
	m.resizePrimaryScreens(mainWidth, contentHeight)
	m.resizeRootInputs()
	m.resizeAnalysisPanel(mainWidth, contentHeight)
}

func (m RootModel) rootMainWidth() int {
	if !m.analysisPanel.IsVisible() || m.screen == ScreenAnalysis {
		return m.width
	}
	return m.width - analysisPanelWidth(m.width)
}

func (m *RootModel) resizePrimaryScreens(width, height int) {
	m.dashboard.SetSize(width, height)
	m.browser.SetSize(width, height)
	m.logs.SetSize(width, height)
	m.helm.SetSize(width, height)
	m.crds.SetSize(width, height)
}

func (m *RootModel) resizeRootInputs() {
	availableWidth := max(rootInputMinimumWidth, m.width-rootInputHorizontalMargin)
	m.cmdInput.SetWidth(availableWidth)
	m.searchInput.SetWidth(min(rootSearchInputMaximumWidth, availableWidth))
}

func (m *RootModel) resizeAnalysisPanel(mainWidth, contentHeight int) {
	if !m.analysisPanel.IsVisible() {
		return
	}
	if m.screen == ScreenAnalysis {
		m.analysisPanel.SetSize(m.width, contentHeight)
		return
	}
	analysisPanelWidth := m.width - mainWidth
	m.analysisPanel.SetSize(analysisPanelWidth, m.analysisPanelHeight(contentHeight))
}

func (m RootModel) analysisPanelHeight(contentHeight int) int {
	switch m.screen {
	case ScreenBrowser:
		_, panelHeight, _ := m.browser.AnalysisOverlayBounds(contentHeight)
		return panelHeight
	case ScreenLogs:
		_, panelHeight, _ := m.logs.AnalysisOverlayBounds(contentHeight)
		return panelHeight
	case ScreenHelm:
		_, panelHeight, _ := m.helm.AnalysisOverlayBounds(contentHeight)
		return panelHeight
	case ScreenCRDs:
		_, panelHeight, _ := m.crds.AnalysisOverlayBounds(contentHeight)
		return panelHeight
	case ScreenDashboard, ScreenAnalysis:
		return contentHeight
	}
	return contentHeight
}

func (m *RootModel) refreshAnalysisContext(owner screenID, accepted bool) {
	if accepted && m.screen == owner && m.analysisPanel.IsVisible() {
		m.updateAnalysisScreenContext()
	}
}

func (m *RootModel) updateAnalysisScreenContext() {
	context, err := m.currentScreenContext()
	if err != nil {
		m.analysisPanel.SetScreenContext("")
		m.setError(fmt.Errorf("build screen context: %w", err))
		return
	}
	m.analysisPanel.SetScreenContext(context)
}

func (m RootModel) currentScreenContext() (string, error) {
	switch m.screen {
	case ScreenDashboard:
		return analysis.BuildDashboardContext(analysis.DashboardContextInput{
			Namespace:   m.namespace,
			Pods:        m.dashboard.pods,
			Deployments: m.dashboard.deployments,
			Events:      m.dashboard.events,
		}), nil
	case ScreenBrowser:
		return m.browserScreenContext()
	case ScreenLogs:
		return analysis.BuildLogsContext(
			m.namespace, m.logs.selectedPod,
			m.logs.allLines, m.logs.filter,
		)
	case ScreenAnalysis, ScreenHelm, ScreenCRDs:
		return "", nil
	default:
		return "", ScreenStateError{Screen: m.screen}
	}
}

func (m RootModel) browserScreenContext() (string, error) {
	selectedIdentity, _ := m.browser.selectedIdentity()
	input, err := analysis.NewBrowserContextInput(
		analysis.BrowserContextSelection{
			Namespace:         m.namespace,
			Resource:          analysis.BrowserResourceKind(m.browser.resourceType),
			SelectedName:      selectedIdentity.Name,
			SelectedNamespace: selectedIdentity.Namespace,
			DetailContent:     m.browser.providerDetailContext(),
		},
		analysis.BrowserSnapshot{
			Pods:            m.browser.pods,
			Deployments:     m.browser.deployments,
			Services:        m.browser.services,
			StatefulSets:    m.browser.statefulsets,
			DaemonSets:      m.browser.daemonsets,
			ConfigMaps:      m.browser.configmaps,
			Nodes:           m.browser.nodes,
			Jobs:            m.browser.jobs,
			Ingresses:       m.browser.ingresses,
			NetworkPolicies: m.browser.networkpolicies,
			PVCs:            m.browser.pvcs,
			CronJobs:        m.browser.cronjobs,
			HPAs:            m.browser.hpas,
			Secrets:         m.browser.secrets,
			ReplicaSets:     m.browser.replicasets,
			RBAC:            m.browser.rbac,
		},
	)
	if err != nil {
		return "", err
	}
	return analysis.BuildBrowserContext(input)
}

func (m RootModel) activeScreenHasInputFocus() bool {
	switch m.screen {
	case ScreenBrowser:
		return m.browser.HasInputFocus()
	case ScreenLogs:
		return m.logs.HasInputFocus()
	case ScreenHelm:
		return m.helm.HasInputFocus()
	case ScreenCRDs:
		return m.crds.HasInputFocus()
	case ScreenDashboard, ScreenAnalysis:
		return false
	}
	return false
}

func (m RootModel) activeScreenHandlesInterrupt() bool {
	return m.screen == ScreenBrowser && m.browser.state == stateShell
}

func (m RootModel) updateActiveScreen(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, command := m.updateActiveScreenValue(msg)
	return updated, command
}
