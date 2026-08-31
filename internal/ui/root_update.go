package ui

import (
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/HediAbed/opsmate/internal/cluster"
)

func (RootModel) Init() tea.Cmd {
	return func() tea.Msg { return initializeRootMsg{} }
}

func (m RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case initializeRootMsg:
		if m.initialized {
			return m, nil
		}
		m.initialized = true
		cmds := []tea.Cmd{
			m.nsSpinner.Tick,
			m.fetchNamespaces(),
			m.fetchCurrentContext(),
		}
		cmds = append(cmds, m.activateScreen(m.screen)...)
		return m, tea.Batch(cmds...)
	default:
		return m.updateRootAsyncMessage(msg)
	}
}

func (m RootModel) updateRootAsyncMessage(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case analysisRequestResultMsg:
		var cmd tea.Cmd
		m.analysisPanel, cmd = m.analysisPanel.Update(msg)
		return m, cmd

	case logsResultMsg:
		accepted := m.logs.acceptsLogResult(msg)
		var cmd tea.Cmd
		m.logs, cmd = m.logs.Update(msg)
		m.refreshAnalysisContext(ScreenLogs, accepted)
		return m, cmd

	case logPodsResultMsg:
		accepted := m.logs.acceptsPodListResult(msg)
		var cmd tea.Cmd
		m.logs, cmd = m.logs.Update(msg)
		m.refreshAnalysisContext(ScreenLogs, accepted)
		return m, cmd

	case containersResultMsg:
		accepted := m.logs.acceptsContainersResult(msg)
		var cmd tea.Cmd
		m.logs, cmd = m.logs.Update(msg)
		m.refreshAnalysisContext(ScreenLogs, accepted)
		return m, cmd

	case logExplainResultMsg:
		var cmd tea.Cmd
		m.logs, cmd = m.logs.Update(msg)
		return m, cmd

	case browserResultMsg:
		accepted := m.browser.acceptsFetchResult(msg)
		var cmd tea.Cmd
		m.browser, cmd = m.browser.Update(msg)
		m.refreshAnalysisContext(ScreenBrowser, accepted)
		return m, cmd

	case browserDetailSummaryResultMsg:
		var cmd tea.Cmd
		m.browser, cmd = m.browser.Update(msg)
		return m, cmd
	default:
		return m.updateRootScreenResultMessage(msg)
	}
}

func (m RootModel) updateRootScreenResultMessage(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case dashboardResultMsg:
		accepted := m.dashboard.acceptsResult(msg)
		var cmd tea.Cmd
		m.dashboard, cmd = m.dashboard.Update(msg)
		m.refreshAnalysisContext(ScreenDashboard, accepted)
		return m, cmd

	case dashboardHealthResultMsg:
		var cmd tea.Cmd
		m.dashboard, cmd = m.dashboard.Update(msg)
		return m, cmd

	case helmResultMsg:
		var cmd tea.Cmd
		m.helm, cmd = m.helm.Update(msg)
		return m, cmd

	case crdResultMsg:
		var cmd tea.Cmd
		m.crds, cmd = m.crds.Update(msg)
		return m, cmd

	case shellOutputMsg:
		var cmd tea.Cmd
		m.browser, cmd = m.browser.Update(msg)
		return m, cmd

	case shellExitMsg:
		var cmd tea.Cmd
		m.browser, cmd = m.browser.Update(msg)
		return m, cmd
	default:
		return m.updateRootLiveMessage(msg)
	}
}

func (m RootModel) updateRootLiveMessage(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case supervisedLiveMsg:
		dashboardOwns := m.dashboard.ownsSupervisedLiveMessage(msg)
		browserOwns := m.browser.ownsSupervisedLiveMessage(msg)
		var dashboardCmd, browserCmd tea.Cmd
		m.dashboard, dashboardCmd = m.dashboard.Update(msg)
		m.browser, browserCmd = m.browser.Update(msg)
		m.refreshAnalysisContext(ScreenDashboard, dashboardOwns)
		m.refreshAnalysisContext(ScreenBrowser, browserOwns)
		return m, tea.Batch(dashboardCmd, browserCmd)

	case dashMetricsTickMsg:
		var cmd tea.Cmd
		m.dashboard, cmd = m.dashboard.Update(msg)
		return m, cmd
	default:
		return m.updateRootClusterMessage(msg)
	}
}

func (m RootModel) updateRootClusterMessage(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.resizeChildren()
		return m, nil

	case cluster.NamespacesMsg:
		m.applyNamespaces(msg)
		return m, nil

	case cluster.CurrentContextMsg:
		m.applyCurrentContext(msg)
		return m, nil

	case cluster.ContextsMsg:
		m.applyContexts(msg)
		return m, nil
	default:
		return m.updateRootOperationMessage(msg)
	}
}

func (m *RootModel) applyNamespaces(msg cluster.NamespacesMsg) {
	m.nsLoading = false
	if msg.Err != nil {
		m.setError(msg.Err)
		return
	}
	m.namespaces = msg.Namespaces
}

func (m *RootModel) applyCurrentContext(msg cluster.CurrentContextMsg) {
	if msg.Err == nil {
		m.currentContext = msg.Name
	}
}

func (m *RootModel) applyContexts(msg cluster.ContextsMsg) {
	m.ctxLoading = false
	if msg.Err != nil {
		m.setError(msg.Err)
		return
	}
	m.contexts = msg.Contexts
	for _, clusterContext := range m.contexts {
		if clusterContext.Current {
			m.currentContext = clusterContext.Name
			return
		}
	}
}

func (m RootModel) updateRootOperationMessage(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case portForwardStartedMsg:
		return m, m.handlePortForwardStarted(msg)

	case portForwardStoppedMsg:
		m.handlePortForwardStopped(msg)
		return m, nil

	case PortForwardFeedbackMsg:
		m.applyPortForwardFeedback(msg)
		return m, nil

	case cluster.ContextSwitchedMsg:
		return m, m.handleContextSwitched(msg)
	default:
		return m.updateRootNavigationMessage(msg)
	}
}

func (m *RootModel) handlePortForwardStarted(msg portForwardStartedMsg) tea.Cmd {
	if msg.Err != nil {
		m.setError(msg.Err)
		return nil
	}
	m.pfSessions = m.operations.PortForwards()
	return m.operations.WaitForPortForwardExit(msg.Session)
}

func (m *RootModel) handlePortForwardStopped(msg portForwardStoppedMsg) {
	m.pfSessions = m.operations.PortForwards()
	if msg.Err != nil {
		m.setError(msg.Err)
	}
	if len(m.pfSessions) == 0 {
		m.pfCursor = 0
		return
	}
	if m.pfCursor >= len(m.pfSessions) {
		m.pfCursor = len(m.pfSessions) - 1
	}
}

func (m *RootModel) applyPortForwardFeedback(msg PortForwardFeedbackMsg) {
	if msg.Err != nil {
		m.setError(msg.Err)
	}
}

func (m *RootModel) handleContextSwitched(msg cluster.ContextSwitchedMsg) tea.Cmd {
	wasSwitching := m.contextSwitching
	m.contextSwitching = false
	if msg.Err != nil {
		m.setError(msg.Err)
		if wasSwitching {
			return tea.Batch(m.activateScreen(m.screen)...)
		}
		return nil
	}
	m.currentContext = msg.Name
	m.namespaces = nil
	m.namespace = ""
	return tea.Batch(m.switchNamespace(), m.fetchNamespaces())
}

func (m RootModel) updateRootNavigationMessage(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case GoBackMsg:
		return m, m.transitionTo(m.prevScreen, false)

	case DrillDownMsg:
		return m.handleDrillDown(msg)

	case tea.KeyMsg:
		return m.handleRootKey(msg)

	case tea.MouseMsg:
		if m.contextSwitching {
			return m, nil
		}
		return m.handleMouse(msg)
	default:
		return m.broadcastRootMessage(msg)
	}
}

func (m RootModel) handleRootKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if m.contextSwitching {
		if key == "q" || key == "ctrl+c" {
			return m, tea.Quit
		}
		return m, nil
	}
	if model, command, handled := m.handlePrimaryRootOverlayKey(msg, key); handled {
		return model, command
	}
	if model, command, handled := m.handleSecondaryRootOverlayKey(msg, key); handled {
		return model, command
	}
	if m.activeScreenHasInputFocus() {
		return m.handleFocusedScreenKey(msg, key)
	}
	if screen, found := rootScreenForKey(key); found {
		return m.switchScreen(screen)
	}
	return m.handleGlobalRootKey(key, msg)
}

func (m RootModel) handlePrimaryRootOverlayKey(
	msg tea.KeyMsg,
	key string,
) (tea.Model, tea.Cmd, bool) {
	switch {
	case m.showCmdPalette:
		model, command := m.handleCmdPalette(key, msg)
		return model, command, true
	case m.showSearch:
		model, command := m.handleSearch(key, msg)
		return model, command, true
	case m.showPFModal:
		model, command := m.handlePFModalKey(key)
		return model, command, true
	case m.showHelp:
		if key == "?" || key == "esc" {
			m.showHelp = false
		}
		return m, nil, true
	default:
		return m, nil, false
	}
}

func (m RootModel) handleSecondaryRootOverlayKey(
	msg tea.KeyMsg,
	key string,
) (tea.Model, tea.Cmd, bool) {
	switch {
	case m.showNSPicker:
		model, command := m.handleNSPicker(key)
		return model, command, true
	case m.showCtxPicker:
		model, command := m.handleCtxPicker(key)
		return model, command, true
	case m.err != nil && key == "esc":
		m.err = nil
		m.resizeChildren()
		return m, nil, true
	case m.analysisPanel.IsVisible():
		model, command := m.handleVisibleAnalysisPanelKey(msg, key)
		return model, command, true
	default:
		return m, nil, false
	}
}

func (m RootModel) handleFocusedScreenKey(msg tea.KeyMsg, key string) (tea.Model, tea.Cmd) {
	if key == "ctrl+c" && !m.activeScreenHandlesInterrupt() {
		return m, tea.Quit
	}
	return m.updateActiveScreen(msg)
}

func rootScreenForKey(key string) (screenID, bool) {
	for _, tab := range rootScreenTabs {
		if tab.key == key {
			return tab.id, true
		}
	}
	return 0, false
}

func (m RootModel) handleGlobalRootKey(key string, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "tab":
		m.toggleAnalysisPanel()
		return m, nil
	case "n":
		return m, m.openNamespacePicker()
	case "k":
		m.showCtxPicker = true
		m.ctxCursor = 0
		m.ctxLoading = true
		return m, m.fetchContexts()
	case "?":
		m.showHelp = !m.showHelp
		return m, nil
	case ":":
		m.showCmdPalette = true
		m.cmdInput.SetValue("")
		m.cmdInput.Focus()
		return m, textinput.Blink
	case "ctrl+p":
		return m, m.openSearch()
	case "F":
		m.openPFModal()
		return m, nil
	default:
		return m.updateActiveScreen(msg)
	}
}

func (m *RootModel) toggleAnalysisPanel() {
	if m.analysisPanel.IsVisible() {
		m.analysisPanel.SetVisible(false)
	} else {
		m.analysisPanel.SetVisible(true)
		m.analysisPanel.Focus()
		m.updateAnalysisScreenContext()
	}
	m.resizeChildren()
}

func (m *RootModel) openNamespacePicker() tea.Cmd {
	m.showNSPicker = true
	m.nsCursor = 0
	if len(m.namespaces) > 0 {
		return nil
	}
	m.nsLoading = true
	return m.fetchNamespaces()
}

func (m RootModel) broadcastRootMessage(msg tea.Msg) (tea.Model, tea.Cmd) {
	if spinnerMessage, ok := msg.(spinner.TickMsg); ok {
		return m.broadcastRootSpinnerTick(spinnerMessage)
	}
	updated, command := m.updateActiveScreenValue(msg)
	if updated.analysisPanel.IsVisible() && isAnalysisContextDataMessage(msg) {
		updated.updateAnalysisScreenContext()
	}
	return updated, command
}

func (m RootModel) broadcastRootSpinnerTick(msg spinner.TickMsg) (tea.Model, tea.Cmd) {
	commands := make([]tea.Cmd, 0, rootSpinnerCommandCapacity)
	var command tea.Cmd
	m.nsSpinner, command = m.nsSpinner.Update(msg)
	commands = append(commands, command)
	if m.analysisPanel.IsVisible() {
		m.analysisPanel, command = m.analysisPanel.Update(msg)
		commands = append(commands, command)
	}
	if m.screen != ScreenAnalysis {
		m, command = m.updateActiveScreenValue(msg)
		commands = append(commands, command)
	}
	return m, tea.Batch(commands...)
}

func isAnalysisContextDataMessage(msg tea.Msg) bool {
	switch msg.(type) {
	case cluster.PodsMsg, cluster.DeploymentsMsg, cluster.EventsMsg,
		cluster.MetricsMsg, cluster.ServicesMsg, cluster.StatefulSetsMsg,
		cluster.DaemonSetsMsg, cluster.ConfigMapsMsg, cluster.NodesMsg,
		cluster.JobsMsg, cluster.LogsMsg, cluster.DescribeMsg:
		return true
	default:
		return false
	}
}

func (m RootModel) updateActiveScreenValue(msg tea.Msg) (RootModel, tea.Cmd) {
	var command tea.Cmd
	switch m.screen {
	case ScreenDashboard:
		m.dashboard, command = m.dashboard.Update(msg)
	case ScreenBrowser:
		m.browser, command = m.browser.Update(msg)
	case ScreenLogs:
		m.logs, command = m.logs.Update(msg)
	case ScreenAnalysis:
		m.analysisPanel, command = m.analysisPanel.Update(msg)
	case ScreenHelm:
		m.helm, command = m.helm.Update(msg)
	case ScreenCRDs:
		m.crds, command = m.crds.Update(msg)
	}
	return m, command
}

func (m RootModel) handleVisibleAnalysisPanelKey(msg tea.KeyMsg, key string) (tea.Model, tea.Cmd) {
	if key != "esc" {
		var command tea.Cmd
		m.analysisPanel, command = m.analysisPanel.Update(msg)
		return m, command
	}
	m.analysisPanel.SetVisible(false)
	if m.screen == ScreenAnalysis {
		return m, m.transitionTo(ScreenDashboard, false)
	}
	m.resizeChildren()
	return m, nil
}
