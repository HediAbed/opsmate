package model

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/HediAbed/opsmate/internal/service"
	"github.com/HediAbed/opsmate/internal/theme"
)

type screenID uint8

const (
	ScreenDashboard screenID = iota
	ScreenBrowser
	ScreenLogs
	ScreenAI
	ScreenHelm
	ScreenCRDs
)

const (
	rootTextInputCharacterLimit     = 128
	rootTextInputInitialWidth       = 60
	rootSpinnerCommandCapacity      = 3
	rootBreadcrumbSpacing           = 2
	searchOverlayChromeHeight       = 8
	searchMinimumVisibleItems       = 5
	portForwardPodDisplayWidth      = 30
	hoursPerDay                     = 24
	portForwardMinimumArgumentCount = 2
	portSpecPartCount               = 2
)

type rootScreenTab struct {
	key  string
	name string
	id   screenID
}

var rootScreenTabs = []rootScreenTab{
	{key: "1", name: "Dashboard", id: ScreenDashboard},
	{key: "2", name: "Browser", id: ScreenBrowser},
	{key: "3", name: "Logs", id: ScreenLogs},
	{key: "4", name: "AI", id: ScreenAI},
	{key: "5", name: "Helm", id: ScreenHelm},
	{key: "6", name: "CRDs", id: ScreenCRDs},
}

// GoBackMsg is sent by child screens to signal "return to previous screen".
type GoBackMsg struct{}

// ClearStatusMsg signals that a transient status message should be cleared.
type ClearStatusMsg struct{}

type initializeRootMsg struct{}

// DrillDownMsg is sent to navigate across screens with context.
type DrillDownMsg struct {
	Screen       screenID
	ResourceType string
	ResourceName string
	ResourceNS   string // namespace of the resource (needed for all-namespaces mode)
}

type RootModel struct {
	width     int
	height    int
	screen    screenID
	namespace string

	dashboard DashboardModel
	browser   BrowserModel
	logs      LogsModel
	helm      HelmModel
	crds      CRDsModel
	aiPanel   AIPanelModel

	namespaces   []string
	showNSPicker bool
	showHelp     bool
	nsCursor     int
	nsSpinner    spinner.Model
	nsLoading    bool

	contexts       []service.KubeContext
	showCtxPicker  bool
	ctxCursor      int
	ctxLoading     bool
	currentContext string

	browserInited   bool
	dashboardInited bool
	logsInited      bool
	helmInited      bool
	crdsInited      bool
	prevScreen      screenID

	showCmdPalette bool
	cmdInput       textinput.Model

	showSearch    bool
	searchInput   textinput.Model
	searchCursor  int
	searchResults []searchResult
	searchCorpus  []searchResult

	showPFModal     bool
	pfCursor        int
	pfSessions      []service.PortForwardSession
	pfConfirmKillID string
	pfConfirmKillOf string

	ready       bool
	initialized bool
	err         error
}

type searchResult struct {
	Kind      string
	Name      string
	Namespace string
}

// NewRootModel creates the application model for namespace.
func NewRootModel(namespace string) RootModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = theme.SpinnerStyle

	ai := NewAIPanelModel()
	ai.SetNamespace(namespace)

	ci := newTextInput(textInputOpts{
		Prompt:      ":",
		Placeholder: "pod, deploy, svc, ns <name>, logs <pod>, q",
		CharLimit:   rootTextInputCharacterLimit,
		Width:       rootTextInputInitialWidth,
		PromptStyle: theme.Accent,
		TextStyle:   lipgloss.NewStyle().Foreground(theme.White),
	})

	si := newTextInput(textInputOpts{
		Prompt:      "find: ",
		Placeholder: "pod/deploy/svc name...",
		CharLimit:   rootTextInputCharacterLimit,
		Width:       rootTextInputInitialWidth,
		PromptStyle: theme.Accent,
		TextStyle:   lipgloss.NewStyle().Foreground(theme.White),
	})

	return RootModel{
		screen:      ScreenDashboard,
		namespace:   namespace,
		dashboard:   NewDashboardModel(namespace),
		browser:     NewBrowserModel(namespace),
		logs:        NewLogsModel(namespace),
		helm:        NewHelmModel(namespace),
		crds:        NewCRDsModel(namespace),
		aiPanel:     ai,
		nsSpinner:   s,
		cmdInput:    ci,
		searchInput: si,
	}
}

// RestoreSession applies persisted screen and resource state.
func (m *RootModel) RestoreSession(s service.SessionState) {
	if restoredScreen, ok := screenIDFromPersisted(s.Screen); ok {
		m.screen = restoredScreen
	}
	if _, supported := resourceCatalog[s.ResourceType]; supported {
		m.browser.resourceType = s.ResourceType
	}
	m.browser.SetWide(s.Wide)
}

func (m RootModel) saveSession() error {
	state := service.SessionState{
		Namespace:    m.namespace,
		Screen:       int(m.screen),
		ResourceType: m.browser.ResourceType(),
		Wide:         m.browser.Wide(),
	}
	return service.SaveSession(state)
}

func (m RootModel) SaveOnExit() error {
	return m.saveSession()
}

func (m *RootModel) persistSession() {
	if err := m.saveSession(); err != nil {
		m.setError(fmt.Errorf("save session: %w", err))
	}
}

func (m *RootModel) setError(err error) {
	m.err = err
	m.resizeChildren()
}

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
			service.FetchNamespaces(),
			service.FetchCurrentContext(),
		}
		cmds = append(cmds, m.activateScreen(m.screen)...)
		return m, tea.Batch(cmds...)
	default:
		return m.updateRootAsyncMessage(msg)
	}
}

func (m RootModel) updateRootAsyncMessage(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case aiRequestResultMsg:
		var cmd tea.Cmd
		m.aiPanel, cmd = m.aiPanel.Update(msg)
		return m, cmd

	case logsResultMsg:
		accepted := m.logs.acceptsLogResult(msg)
		var cmd tea.Cmd
		m.logs, cmd = m.logs.Update(msg)
		m.refreshAssistantContext(ScreenLogs, accepted)
		return m, cmd

	case logPodsResultMsg:
		accepted := m.logs.acceptsPodListResult(msg)
		var cmd tea.Cmd
		m.logs, cmd = m.logs.Update(msg)
		m.refreshAssistantContext(ScreenLogs, accepted)
		return m, cmd

	case containersResultMsg:
		accepted := m.logs.acceptsContainersResult(msg)
		var cmd tea.Cmd
		m.logs, cmd = m.logs.Update(msg)
		m.refreshAssistantContext(ScreenLogs, accepted)
		return m, cmd

	case logExplainResultMsg:
		var cmd tea.Cmd
		m.logs, cmd = m.logs.Update(msg)
		return m, cmd

	case browserResultMsg:
		accepted := m.browser.acceptsFetchResult(msg)
		var cmd tea.Cmd
		m.browser, cmd = m.browser.Update(msg)
		m.refreshAssistantContext(ScreenBrowser, accepted)
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
		m.refreshAssistantContext(ScreenDashboard, accepted)
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
		return m.updateRootWatchMessage(msg)
	}
}

func (m RootModel) updateRootWatchMessage(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case supervisedWatchMsg:
		dashboardOwns := m.dashboard.ownsSupervisedWatchMessage(msg)
		browserOwns := m.browser.ownsSupervisedWatchMessage(msg)
		var dashboardCmd, browserCmd tea.Cmd
		m.dashboard, dashboardCmd = m.dashboard.Update(msg)
		m.browser, browserCmd = m.browser.Update(msg)
		m.refreshAssistantContext(ScreenDashboard, dashboardOwns)
		m.refreshAssistantContext(ScreenBrowser, browserOwns)
		return m, tea.Batch(dashboardCmd, browserCmd)

	case browserReconnectMsg:
		var cmd tea.Cmd
		m.browser, cmd = m.browser.Update(msg)
		return m, cmd

	case dashboardPodReconnectMsg, dashboardDeploymentReconnectMsg, dashboardEventReconnectMsg, dashMetricsTickMsg:
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

	case service.NamespacesMsg:
		m.applyNamespaces(msg)
		return m, nil

	case service.CurrentContextMsg:
		m.applyCurrentContext(msg)
		return m, nil

	case service.ContextsMsg:
		m.applyContexts(msg)
		return m, nil
	default:
		return m.updateRootOperationMessage(msg)
	}
}

func (m *RootModel) applyNamespaces(msg service.NamespacesMsg) {
	m.nsLoading = false
	if msg.Err != nil {
		m.setError(msg.Err)
		return
	}
	m.namespaces = msg.Namespaces
}

func (m *RootModel) applyCurrentContext(msg service.CurrentContextMsg) {
	if msg.Err == nil {
		m.currentContext = msg.Name
	}
}

func (m *RootModel) applyContexts(msg service.ContextsMsg) {
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
	case service.PortForwardStartedMsg:
		return m, m.handlePortForwardStarted(msg)

	case service.PortForwardStoppedMsg:
		m.handlePortForwardStopped(msg)
		return m, nil

	case PortForwardFeedbackMsg:
		m.applyPortForwardFeedback(msg)
		return m, nil

	case service.ContextSwitchedMsg:
		return m, m.handleContextSwitched(msg)
	default:
		return m.updateRootNavigationMessage(msg)
	}
}

func (m *RootModel) handlePortForwardStarted(msg service.PortForwardStartedMsg) tea.Cmd {
	if msg.Err != nil {
		m.setError(msg.Err)
		return nil
	}
	m.pfSessions = service.ListPortForwards()
	return service.WaitForPortForwardExit(msg.Session)
}

func (m *RootModel) handlePortForwardStopped(msg service.PortForwardStoppedMsg) {
	m.pfSessions = service.ListPortForwards()
	if msg.Err != nil {
		m.setError(msg.Err)
	}
	if m.pfCursor >= len(m.pfSessions) && len(m.pfSessions) > 0 {
		m.pfCursor = len(m.pfSessions) - 1
	}
}

func (m *RootModel) applyPortForwardFeedback(msg PortForwardFeedbackMsg) {
	if msg.Err != nil {
		m.setError(msg.Err)
	}
}

func (m *RootModel) handleContextSwitched(msg service.ContextSwitchedMsg) tea.Cmd {
	if msg.Err != nil {
		m.setError(msg.Err)
		return nil
	}
	m.currentContext = msg.Name
	m.namespaces = nil
	m.namespace = ""
	return tea.Batch(m.switchNamespace(), service.FetchNamespaces())
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
		return m.handleMouse(msg)
	default:
		return m.broadcastRootMessage(msg)
	}
}

func (m RootModel) handleRootKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
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
	case m.aiPanel.IsVisible():
		model, command := m.handleVisibleAIPanelKey(msg, key)
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
		m.toggleAssistantPanel()
		return m, nil
	case "n":
		return m, m.openNamespacePicker()
	case "k":
		m.showCtxPicker = true
		m.ctxCursor = 0
		m.ctxLoading = true
		return m, service.FetchContexts()
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

func (m *RootModel) toggleAssistantPanel() {
	if m.aiPanel.IsVisible() {
		m.aiPanel.SetVisible(false)
	} else {
		m.aiPanel.SetVisible(true)
		m.aiPanel.Focus()
		m.updateAIScreenContext()
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
	return service.FetchNamespaces()
}

func (m RootModel) broadcastRootMessage(msg tea.Msg) (tea.Model, tea.Cmd) {
	if spinnerMessage, ok := msg.(spinner.TickMsg); ok {
		return m.broadcastRootSpinnerTick(spinnerMessage)
	}
	updated, command := m.updateActiveScreenValue(msg)
	if updated.aiPanel.IsVisible() && isAssistantContextDataMessage(msg) {
		updated.updateAIScreenContext()
	}
	return updated, command
}

func (m RootModel) broadcastRootSpinnerTick(msg spinner.TickMsg) (tea.Model, tea.Cmd) {
	commands := make([]tea.Cmd, 0, rootSpinnerCommandCapacity)
	var command tea.Cmd
	m.nsSpinner, command = m.nsSpinner.Update(msg)
	commands = append(commands, command)
	if m.aiPanel.IsVisible() {
		m.aiPanel, command = m.aiPanel.Update(msg)
		commands = append(commands, command)
	}
	if m.screen != ScreenAI {
		m, command = m.updateActiveScreenValue(msg)
		commands = append(commands, command)
	}
	return m, tea.Batch(commands...)
}

func isAssistantContextDataMessage(msg tea.Msg) bool {
	switch msg.(type) {
	case service.PodsMsg, service.DeploymentsMsg, service.EventsMsg,
		service.MetricsMsg, service.ServicesMsg, service.StatefulSetsMsg,
		service.DaemonSetsMsg, service.ConfigMapsMsg, service.NodesMsg,
		service.JobsMsg, service.LogsMsg, service.DescribeMsg:
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
	case ScreenAI:
		m.aiPanel, command = m.aiPanel.Update(msg)
	case ScreenHelm:
		m.helm, command = m.helm.Update(msg)
	case ScreenCRDs:
		m.crds, command = m.crds.Update(msg)
	}
	return m, command
}

func (m RootModel) handleVisibleAIPanelKey(msg tea.KeyMsg, key string) (tea.Model, tea.Cmd) {
	if key != "esc" {
		var command tea.Cmd
		m.aiPanel, command = m.aiPanel.Update(msg)
		return m, command
	}
	m.aiPanel.SetVisible(false)
	if m.screen == ScreenAI {
		return m, m.transitionTo(ScreenDashboard, false)
	}
	m.resizeChildren()
	return m, nil
}

func (m RootModel) View() tea.View {
	return tea.View{
		Content:   m.renderContent(),
		AltScreen: true,
		MouseMode: tea.MouseModeCellMotion,
	}
}

func (m RootModel) renderContent() string {
	if !m.ready {
		return "\n  Initializing OpsMate..."
	}

	footer := m.renderRootFooter()
	contentHeight := max(rootMinimumContentHeight, m.height-lipgloss.Height(footer))
	palette := m.renderCommandPalette()
	contentHeight = max(rootMinimumContentHeight, contentHeight-lipgloss.Height(palette))

	if overlay, visible := m.renderActiveRootOverlay(contentHeight); visible {
		return lipgloss.JoinVertical(lipgloss.Left, overlay, footer)
	}

	content := m.renderActiveScreen(contentHeight)
	sections := make([]string, 0, rootViewSectionCapacity)
	if palette != "" {
		sections = append(sections, palette)
	}
	sections = append(sections, content, footer)
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m RootModel) renderRootFooter() string {
	statusBar := m.renderStatusBar()
	errorBar := m.renderRootError()
	if errorBar == "" {
		return statusBar
	}
	return lipgloss.JoinVertical(lipgloss.Left, errorBar, statusBar)
}

func (m RootModel) renderCommandPalette() string {
	if !m.showCmdPalette {
		return ""
	}
	return lipgloss.NewStyle().
		Width(m.width).
		Background(theme.DarkerBg).
		Padding(0, rootHorizontalPadding).
		Render(m.cmdInput.View())
}

func (m RootModel) renderActiveRootOverlay(height int) (string, bool) {
	var overlay string
	switch {
	case m.showHelp:
		overlay = m.renderHelpOverlay(height)
	case m.showNSPicker:
		overlay = m.renderNSPicker(height)
	case m.showCtxPicker:
		overlay = m.renderCtxPicker(height)
	case m.showSearch:
		overlay = m.renderSearchOverlay(height)
	case m.showPFModal:
		overlay = m.renderPFModal(height)
	default:
		return "", false
	}
	return m.fitRootContent(overlay, height), true
}

func (m RootModel) renderActiveScreen(height int) string {
	if m.screen == ScreenAI {
		return m.fitRootContent(m.aiPanel.View(), height)
	}

	screenView := m.activeScreenView()
	if m.aiPanel.IsVisible() {
		topOffset, bottomOffset := m.assistantOverlayOffsets(height)
		assistantView := strings.Repeat("\n", topOffset) + m.aiPanel.View() + strings.Repeat("\n", bottomOffset)
		screenView = lipgloss.JoinHorizontal(lipgloss.Top, screenView, assistantView)
	}
	return m.fitRootContent(screenView, height)
}

func (m RootModel) activeScreenView() string {
	switch m.screen {
	case ScreenDashboard:
		return m.dashboard.View()
	case ScreenBrowser:
		return m.browser.View()
	case ScreenLogs:
		return m.logs.View()
	case ScreenHelm:
		return m.helm.View()
	case ScreenCRDs:
		return m.crds.View()
	case ScreenAI:
		return m.aiPanel.View()
	default:
		return ""
	}
}

func (m RootModel) assistantOverlayOffsets(height int) (int, int) {
	var topOffset, bottomOffset int
	switch m.screen {
	case ScreenBrowser:
		topOffset, _, bottomOffset = m.browser.AIOverlayBounds(height)
	case ScreenLogs:
		topOffset, _, bottomOffset = m.logs.AIOverlayBounds(height)
	case ScreenHelm:
		topOffset, _, bottomOffset = m.helm.AIOverlayBounds(height)
	case ScreenCRDs:
		topOffset, _, bottomOffset = m.crds.AIOverlayBounds(height)
	case ScreenDashboard, ScreenAI:
	}
	return topOffset, bottomOffset
}

func (m RootModel) fitRootContent(content string, height int) string {
	return lipgloss.NewStyle().
		Width(m.width).
		Height(height).
		MaxHeight(height).
		Render(content)
}

func (m RootModel) renderRootError() string {
	if m.err == nil {
		return ""
	}
	message := sanitizeTerminalLine(service.SanitizeKubectlStderr(m.err.Error()))
	return theme.ErrorBanner.Width(m.width).MaxWidth(m.width).Render("ERROR: " + message + "   (esc dismiss)")
}

func (m RootModel) renderStatusBar() string {
	left := m.renderRootTabs()
	middle := "  " + m.renderBreadcrumb() + "  "
	right := m.renderStatusHints()
	gap := max(0, m.width-lipgloss.Width(left)-lipgloss.Width(middle)-lipgloss.Width(right))
	filler := lipgloss.NewStyle().
		Background(theme.DarkerBg).
		Foreground(theme.NeonCyan).
		Width(gap).
		Render("")

	return lipgloss.NewStyle().
		Width(m.width).
		Background(theme.DarkerBg).
		Render(left + middle + filler + right)
}

func (m RootModel) renderRootTabs() string {
	var tabs []string
	for _, tab := range rootScreenTabs {
		tabs = append(tabs, m.renderScreenTab(tab))
	}
	return lipgloss.JoinHorizontal(lipgloss.Center, tabs...)
}

func (m RootModel) renderBreadcrumb() string {
	sep := theme.BreadcrumbSep.Render(" > ")
	nsLabel := m.namespace
	if nsLabel == "" {
		nsLabel = "all-ns"
	}
	breadcrumb := theme.BreadcrumbStyle.Render(nsLabel)
	if m.currentContext != "" {
		breadcrumb = theme.Dim.Render(m.currentContext) + sep + theme.BreadcrumbStyle.Render(nsLabel)
	}
	return breadcrumb + m.renderScreenBreadcrumb(sep)
}

func (m RootModel) renderScreenBreadcrumb(separator string) string {
	switch m.screen {
	case ScreenBrowser:
		return m.renderBrowserBreadcrumb(separator)
	case ScreenLogs:
		return renderSelectedBreadcrumb(separator, "logs", m.logs.SelectedPod())
	case ScreenDashboard:
		return renderOptionalBreadcrumb(separator, m.dashboard.SelectedPod())
	case ScreenHelm:
		return renderSelectedBreadcrumb(separator, "helm", m.helm.SelectedRelease().Name)
	case ScreenCRDs:
		return renderSelectedBreadcrumb(separator, "crds", m.crds.SelectedCRDName())
	case ScreenAI:
		return separator + theme.BreadcrumbStyle.Render("assistant")
	default:
		return ""
	}
}

func (m RootModel) renderBrowserBreadcrumb(separator string) string {
	resourceType := separator + theme.BreadcrumbStyle.Render(m.browser.ResourceType())
	_, selectedName := m.browser.SelectedResource()
	return resourceType + renderOptionalBreadcrumb(separator, selectedName)
}

func renderSelectedBreadcrumb(separator, section, selected string) string {
	return separator + theme.BreadcrumbStyle.Render(section) + renderOptionalBreadcrumb(separator, selected)
}

func renderOptionalBreadcrumb(separator, value string) string {
	if value == "" {
		return ""
	}
	return separator + theme.BreadcrumbStyle.Render(value)
}

func (m RootModel) renderStatusHints() string {
	var text string
	switch m.screen {
	case ScreenDashboard, ScreenHelm:
		text = " r:refresh  ?:help  q:quit "
	case ScreenBrowser:
		text = " /:filter  ?:help  q:quit "
	case ScreenLogs:
		text = " p:pod  f:filter  ?:help  q:quit "
	case ScreenAI:
		text = " !:command  ?:help  q:quit "
	case ScreenCRDs:
		text = " enter:open  esc:back  ?:help  q:quit "
	default:
		return ""
	}
	return theme.StatusBarItem.Render(text)
}

func (m RootModel) renderScreenTab(tab rootScreenTab) string {
	label := fmt.Sprintf(" %s:%s ", tab.key, tab.name)
	if tab.id == m.screen {
		return theme.StatusBarActive.Render(label)
	}
	return theme.StatusBarItem.Render(label)
}

func (m RootModel) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if model, command, handled := m.handlePickerMouse(msg); handled {
		return model, command
	}
	if m.showHelp || m.showSearch || m.showPFModal || m.showCmdPalette {
		return m, nil
	}
	if model, command, handled := m.handleStatusBarMouse(msg); handled {
		return model, command
	}
	if model, command, handled := m.handleAssistantMouse(msg); handled {
		return model, command
	}
	return m.updateActiveScreen(msg)
}

func (m RootModel) handlePickerMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd, bool) {
	switch {
	case m.showNSPicker:
		model, command := m.handleNSPickerMouse(msg)
		return model, command, true
	case m.showCtxPicker:
		model, command := m.handleCtxPickerMouse(msg)
		return model, command, true
	default:
		return m, nil, false
	}
}

func (m RootModel) handleStatusBarMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd, bool) {
	if msg.Mouse().Y < m.height-rootStatusBarHeight {
		return m, nil, false
	}
	click, clickable := msg.(tea.MouseClickMsg)
	if !clickable || click.Button != tea.MouseLeft {
		return m, nil, true
	}
	model, command := m.handleStatusBarClick(click.X)
	return model, command, true
}

func (m RootModel) handleAssistantMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd, bool) {
	if !m.aiPanel.IsVisible() || m.screen == ScreenAI {
		return m, nil, false
	}
	mainWidth := m.width - assistantPanelWidth(m.width)
	if msg.Mouse().X < mainWidth {
		return m, nil, false
	}
	var command tea.Cmd
	m.aiPanel, command = m.aiPanel.Update(shiftMouseX(msg, -mainWidth))
	return m, command, true
}

func assistantPanelWidth(totalWidth int) int {
	preferred := totalWidth / assistantPanelWidthRatio
	minimum := min(assistantPanelMinimumWidth, totalWidth/assistantPanelMaximumRatio)
	maximum := totalWidth / assistantPanelMaximumRatio
	return min(max(preferred, minimum), maximum)
}

// shiftMouseX translates a typed mouse event horizontally.
func shiftMouseX(msg tea.MouseMsg, dx int) tea.Msg {
	switch ev := msg.(type) {
	case tea.MouseClickMsg:
		ev.X += dx
		return ev
	case tea.MouseReleaseMsg:
		ev.X += dx
		return ev
	case tea.MouseWheelMsg:
		ev.X += dx
		return ev
	case tea.MouseMotionMsg:
		ev.X += dx
		return ev
	}
	return msg
}

func (m RootModel) handleStatusBarClick(x int) (tea.Model, tea.Cmd) {
	cumX := 0
	for _, tab := range rootScreenTabs {
		rendered := m.renderScreenTab(tab)
		tabW := lipgloss.Width(rendered)
		if x >= cumX && x < cumX+tabW {
			return m.switchScreen(tab.id)
		}
		cumX += tabW
	}

	breadcrumbStart := cumX + rootBreadcrumbSpacing
	nsRendered := theme.BreadcrumbStyle.Render(m.namespace)
	nsEnd := breadcrumbStart + lipgloss.Width(nsRendered)

	if x >= breadcrumbStart && x < nsEnd {
		m.showNSPicker = true
		m.nsCursor = 0
		if len(m.namespaces) == 0 {
			m.nsLoading = true
			return m, service.FetchNamespaces()
		}
		return m, nil
	}

	return m, nil
}

func (m RootModel) renderHelpOverlay(height int) string {
	title := theme.Title.Render("KEYBINDINGS")
	global := strings.Join(globalHelpBindings(), "\n")
	contextual := strings.Join(m.contextualHelpBindings(), "\n")
	columns := lipgloss.JoinHorizontal(lipgloss.Top, global, "    ", contextual)
	content := title + "\n\n" + columns + "\n\n" + theme.Dim.Render("Press ? or esc to close")

	box := lipgloss.NewStyle().
		Border(lipgloss.ThickBorder()).
		BorderForeground(theme.NeonCyan).
		Padding(0, rootHorizontalPadding).
		Width(clampModalWidth(helpModalDesiredWidth, m.width)).
		MaxWidth(m.width).
		Render(content)
	return lipgloss.Place(m.width, height, lipgloss.Center, lipgloss.Center, box)
}

type rootHelpBinding struct {
	key         string
	description string
}

func renderHelpBindings(section string, bindings ...rootHelpBinding) []string {
	lines := make([]string, 1, len(bindings)+1)
	lines[0] = theme.Accent.Render(section)
	for _, binding := range bindings {
		key := theme.HelpKey.Render(fmt.Sprintf("  %-12s", binding.key))
		lines = append(lines, key+theme.HelpDesc.Render(binding.description))
	}
	return lines
}

func globalHelpBindings() []string {
	return renderHelpBindings("Global",
		rootHelpBinding{key: "1-6", description: "Switch screen"},
		rootHelpBinding{key: "n", description: "Namespace picker"},
		rootHelpBinding{key: "k", description: "Context picker"},
		rootHelpBinding{key: "tab", description: "Toggle assistant panel"},
		rootHelpBinding{key: ":", description: "Command palette"},
		rootHelpBinding{key: "ctrl+p", description: "Find resource"},
		rootHelpBinding{key: "F", description: "Port-forwards"},
		rootHelpBinding{key: "?", description: "Toggle help"},
		rootHelpBinding{key: "q", description: "Quit"},
	)
}

func (m RootModel) contextualHelpBindings() []string {
	switch m.screen {
	case ScreenDashboard:
		return renderHelpBindings("Dashboard",
			rootHelpBinding{key: "enter", description: "Describe pod in Browser"},
			rootHelpBinding{key: "l", description: "Open pod logs"},
			rootHelpBinding{key: "r", description: "Refresh"},
			rootHelpBinding{key: "up/down", description: "Navigate pods"},
		)
	case ScreenBrowser:
		return browserHelpBindings()
	case ScreenLogs:
		return logsHelpBindings()
	case ScreenAI:
		return renderHelpBindings("Assistant",
			rootHelpBinding{key: "enter", description: "Send query"},
			rootHelpBinding{key: "!cmd", description: "Generate kubectl"},
			rootHelpBinding{key: "i / /", description: "Focus input"},
			rootHelpBinding{key: "esc", description: "Close panel"},
		)
	case ScreenHelm:
		return renderHelpBindings("Helm",
			rootHelpBinding{key: "up/down", description: "Navigate releases"},
			rootHelpBinding{key: "r", description: "Refresh"},
		)
	case ScreenCRDs:
		return renderHelpBindings("CRDs",
			rootHelpBinding{key: "up/down", description: "Navigate"},
			rootHelpBinding{key: "enter", description: "Open instances"},
			rootHelpBinding{key: "esc", description: "Back to list"},
			rootHelpBinding{key: "r", description: "Refresh"},
		)
	default:
		return nil
	}
}

func browserHelpBindings() []string {
	return renderHelpBindings("Browser",
		rootHelpBinding{key: "enter", description: "Describe resource"},
		rootHelpBinding{key: "y", description: "View YAML"},
		rootHelpBinding{key: "e", description: "Events"},
		rootHelpBinding{key: "l", description: "Logs (pods only)"},
		rootHelpBinding{key: "s", description: "Scale"},
		rootHelpBinding{key: "R", description: "Rollout restart"},
		rootHelpBinding{key: "X", description: "Shell exec (in-pane)"},
		rootHelpBinding{key: "x", description: "Delete"},
		rootHelpBinding{key: "space", description: "Mark / multi-select"},
		rootHelpBinding{key: "←/→", description: "Cycle resource type tabs"},
		rootHelpBinding{key: "p/d", description: "Pods / Deploys"},
		rootHelpBinding{key: "/", description: "Filter"},
		rootHelpBinding{key: "c", description: "Copy selection"},
		rootHelpBinding{key: "v", description: "Toggle split layout"},
		rootHelpBinding{key: "a", description: "Summarize detail"},
		rootHelpBinding{key: "w", description: "Toggle wide columns"},
		rootHelpBinding{key: "r", description: "Refresh"},
	)
}

func logsHelpBindings() []string {
	return renderHelpBindings("Logs",
		rootHelpBinding{key: "p", description: "Select pod"},
		rootHelpBinding{key: "o", description: "Select container"},
		rootHelpBinding{key: "i", description: "Inspect mode"},
		rootHelpBinding{key: "n / N", description: "Next / previous issue"},
		rootHelpBinding{key: "/", description: "Filter"},
		rootHelpBinding{key: "space", description: "Pause or resume"},
		rootHelpBinding{key: "r", description: "Refresh"},
		rootHelpBinding{key: "g / G", description: "Top / bottom"},
		rootHelpBinding{key: "+ / -", description: "Tail lines"},
		rootHelpBinding{key: "c / C", description: "Copy visible / all"},
		rootHelpBinding{key: "esc", description: "Back"},
	)
}

// switchScreen updates lifecycle state before changing the active screen.
func (m RootModel) switchScreen(screen screenID) (tea.Model, tea.Cmd) {
	cmd := m.transitionTo(screen, false)
	m.persistSession()
	return m, cmd
}

func (m *RootModel) transitionTo(screen screenID, rememberCurrent bool) tea.Cmd {
	if !validScreen(screen) || screen == m.screen {
		return nil
	}
	current := m.screen
	m.deactivateScreen(current)
	if rememberCurrent {
		m.prevScreen = current
	}
	m.screen = screen
	cmds := m.activateScreen(screen)
	m.resizeChildren()
	return tea.Batch(cmds...)
}

func validScreen(screen screenID) bool {
	return screen >= ScreenDashboard && screen <= ScreenCRDs
}

func screenIDFromPersisted(value int) (screenID, bool) {
	if value < int(ScreenDashboard) || value > int(ScreenCRDs) {
		return ScreenDashboard, false
	}
	return screenID(value), true
}

func (m *RootModel) activateScreen(screen screenID) []tea.Cmd {
	switch screen {
	case ScreenBrowser:
		return m.activateBrowser()
	case ScreenDashboard:
		return m.activateDashboard()
	case ScreenLogs:
		return m.activateLogs()
	case ScreenAI:
		m.aiPanel.SetVisible(true)
		m.aiPanel.Focus()
		m.updateAIScreenContext()
		m.resizeChildren()
		return nil
	case ScreenHelm:
		return m.activateHelm()
	case ScreenCRDs:
		return m.activateCRDs()
	default:
		return nil
	}
}

func (m *RootModel) activateBrowser() []tea.Cmd {
	commands := make([]tea.Cmd, 0, rootActivationCommandCapacity)
	if !m.browserInited {
		m.browserInited = true
		commands = append(commands, m.browser.Init())
	}
	return append(commands, m.browser.Activate())
}

func (m *RootModel) activateDashboard() []tea.Cmd {
	commands := make([]tea.Cmd, 0, rootActivationCommandCapacity)
	if !m.dashboardInited {
		m.dashboardInited = true
		commands = append(commands, m.dashboard.Init())
	}
	return append(commands, m.dashboard.Activate())
}

func (m *RootModel) activateLogs() []tea.Cmd {
	commands := make([]tea.Cmd, 0, rootActivationCommandCapacity)
	if !m.logsInited {
		m.logsInited = true
		commands = append(commands, m.logs.Init())
	}
	return append(commands, m.logs.Activate())
}

func (m *RootModel) activateHelm() []tea.Cmd {
	if !m.helmInited {
		m.helmInited = true
		return []tea.Cmd{m.helm.Init()}
	}
	return []tea.Cmd{m.helm.Activate()}
}

func (m *RootModel) activateCRDs() []tea.Cmd {
	if !m.crdsInited {
		m.crdsInited = true
		return []tea.Cmd{m.crds.Init()}
	}
	return []tea.Cmd{m.crds.Activate()}
}

func (m *RootModel) deactivateScreen(screen screenID) {
	switch screen {
	case ScreenBrowser:
		m.browser.Deactivate()
	case ScreenDashboard:
		m.dashboard.Deactivate()
	case ScreenLogs:
		m.logs.Deactivate()
	case ScreenAI:
		m.aiPanel.SetVisible(false)
	case ScreenHelm:
		m.helm.Deactivate()
	case ScreenCRDs:
		m.crds.Deactivate()
	}
}

func (m *RootModel) openSearch() tea.Cmd {
	m.showSearch = true
	m.searchCursor = 0
	m.searchInput.SetValue("")
	m.searchCorpus = m.collectSearchCorpus()
	m.searchResults = m.searchCorpus
	m.searchInput.Focus()
	return textinput.Blink
}

func (m RootModel) handleSearch(key string, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.showSearch = false
		m.searchInput.Blur()
		return m, nil
	case "up", "ctrl+p":
		if m.searchCursor > 0 {
			m.searchCursor--
		}
		return m, nil
	case "down", "ctrl+n":
		if m.searchCursor < len(m.searchResults)-1 {
			m.searchCursor++
		}
		return m, nil
	case "enter":
		if len(m.searchResults) == 0 {
			return m, nil
		}
		chosen := m.searchResults[m.searchCursor]
		m.showSearch = false
		m.searchInput.Blur()
		return m, func() tea.Msg {
			return DrillDownMsg{
				Screen:       ScreenBrowser,
				ResourceType: chosen.Kind,
				ResourceName: chosen.Name,
				ResourceNS:   chosen.Namespace,
			}
		}
	default:
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		m.searchResults = m.filterSearchResults(m.searchInput.Value())
		if m.searchCursor >= len(m.searchResults) {
			m.searchCursor = 0
		}
		return m, cmd
	}
}

func (m RootModel) collectSearchCorpus() []searchResult {
	candidates := append(m.dashboardSearchResults(), m.browserSearchResults()...)
	candidates = append(candidates, m.logSearchResults()...)
	return uniqueSearchResults(candidates)
}

func (m RootModel) dashboardSearchResults() []searchResult {
	results := make([]searchResult, 0, len(m.dashboard.pods)+len(m.dashboard.deployments))
	for _, pod := range m.dashboard.pods {
		results = append(results, searchResult{Kind: "pod", Name: pod.Name, Namespace: pod.Namespace})
	}
	for _, deployment := range m.dashboard.deployments {
		results = append(results, searchResult{Kind: "deployment", Name: deployment.Name, Namespace: deployment.Namespace})
	}
	return results
}

func (m RootModel) browserSearchResults() []searchResult {
	capacity := len(m.browser.pods) + len(m.browser.deployments) + len(m.browser.services) +
		len(m.browser.statefulsets) + len(m.browser.daemonsets) + len(m.browser.configmaps) + len(m.browser.jobs)
	results := make([]searchResult, 0, capacity)
	for _, pod := range m.browser.pods {
		results = append(results, searchResult{Kind: "pod", Name: pod.Name, Namespace: pod.Namespace})
	}
	for _, deployment := range m.browser.deployments {
		results = append(results, searchResult{Kind: "deployment", Name: deployment.Name, Namespace: deployment.Namespace})
	}
	for _, svc := range m.browser.services {
		results = append(results, searchResult{Kind: "service", Name: svc.Name, Namespace: svc.Namespace})
	}
	for _, statefulSet := range m.browser.statefulsets {
		results = append(results, searchResult{Kind: "statefulset", Name: statefulSet.Name, Namespace: statefulSet.Namespace})
	}
	for _, daemonSet := range m.browser.daemonsets {
		results = append(results, searchResult{Kind: "daemonset", Name: daemonSet.Name, Namespace: daemonSet.Namespace})
	}
	for _, configMap := range m.browser.configmaps {
		results = append(results, searchResult{Kind: "configmap", Name: configMap.Name, Namespace: configMap.Namespace})
	}
	for _, job := range m.browser.jobs {
		results = append(results, searchResult{Kind: "job", Name: job.Name, Namespace: job.Namespace})
	}
	return results
}

func (m RootModel) logSearchResults() []searchResult {
	results := make([]searchResult, 0, len(m.logs.pods))
	for _, pod := range m.logs.pods {
		results = append(results, searchResult{Kind: "pod", Name: pod.Name, Namespace: pod.Namespace})
	}
	return results
}

func uniqueSearchResults(candidates []searchResult) []searchResult {
	seen := make(map[searchResult]struct{})
	results := make([]searchResult, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Name == "" {
			continue
		}
		if _, duplicate := seen[candidate]; duplicate {
			continue
		}
		seen[candidate] = struct{}{}
		results = append(results, candidate)
	}
	return results
}

func (m RootModel) filterSearchResults(query string) []searchResult {
	if query == "" {
		return m.searchCorpus
	}
	q := strings.ToLower(query)
	var out []searchResult
	for _, r := range m.searchCorpus {
		if strings.Contains(strings.ToLower(r.Name), q) {
			out = append(out, r)
		}
	}
	return out
}

func (m RootModel) renderSearchOverlay(height int) string {
	title := theme.Title.Render("FIND RESOURCE")

	inputLine := m.searchInput.View()

	visibleMax := height - searchOverlayChromeHeight
	if visibleMax < searchMinimumVisibleItems {
		visibleMax = searchMinimumVisibleItems
	}
	start := 0
	if m.searchCursor >= visibleMax {
		start = m.searchCursor - visibleMax + 1
	}
	end := start + visibleMax
	if end > len(m.searchResults) {
		end = len(m.searchResults)
	}

	var lines []string
	if len(m.searchResults) == 0 {
		lines = append(lines, theme.Dim.Render("no matches (try refreshing screens first — search is limited to cached data)"))
	}
	for i := start; i < end; i++ {
		r := m.searchResults[i]
		label := fmt.Sprintf("%-12s %-32s %s", r.Kind, r.Name, r.Namespace)
		if i == m.searchCursor {
			lines = append(lines, theme.TableSelected.Render(" ▸ "+label+" "))
		} else {
			lines = append(lines, theme.Dim.Render("   "+label))
		}
	}

	help := theme.HelpKey.Render("↑/↓") + theme.HelpDesc.Render(" move  ") +
		theme.HelpKey.Render("enter") + theme.HelpDesc.Render(" open  ") +
		theme.HelpKey.Render("esc") + theme.HelpDesc.Render(" cancel")

	content := title + "\n\n" + inputLine + "\n\n" + strings.Join(lines, "\n") + "\n\n" + help

	box := lipgloss.NewStyle().
		Border(lipgloss.ThickBorder()).
		BorderForeground(theme.NeonCyan).
		Padding(0, 1).
		Width(clampModalWidth(searchModalDesiredWidth, m.width)).
		MaxWidth(m.width).
		Render(content)

	return lipgloss.Place(m.width, height, lipgloss.Center, lipgloss.Center, box)
}

func (m *RootModel) openPFModal() {
	m.showPFModal = true
	m.pfCursor = 0
	m.pfSessions = service.ListPortForwards()
}

func (m RootModel) handlePFModalKey(key string) (tea.Model, tea.Cmd) {
	if m.pfConfirmKillID != "" {
		return m.handlePFKillConfirmation(key)
	}
	switch key {
	case "esc", "F":
		m.showPFModal = false
		return m, nil
	case "up", "k":
		if m.pfCursor > 0 {
			m.pfCursor--
		}
	case "down", "j":
		if m.pfCursor < len(m.pfSessions)-1 {
			m.pfCursor++
		}
	case "r":
		m.refreshPFSessions()
	case "x":
		m.beginPFKillConfirmation()
	}
	return m, nil
}

func (m RootModel) handlePFKillConfirmation(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "y", "Y":
		portForwardID := m.pfConfirmKillID
		m.clearPFKillConfirmation()
		return m, service.StopPortForward(portForwardID)
	case "n", "N", "esc":
		m.clearPFKillConfirmation()
	}
	return m, nil
}

func (m *RootModel) clearPFKillConfirmation() {
	m.pfConfirmKillID = ""
	m.pfConfirmKillOf = ""
}

func (m *RootModel) refreshPFSessions() {
	m.pfSessions = service.ListPortForwards()
	if len(m.pfSessions) == 0 {
		m.pfCursor = 0
		return
	}
	m.pfCursor = min(m.pfCursor, len(m.pfSessions)-1)
}

func (m *RootModel) beginPFKillConfirmation() {
	if m.pfCursor < 0 || m.pfCursor >= len(m.pfSessions) {
		return
	}
	session := m.pfSessions[m.pfCursor]
	m.pfConfirmKillID = session.ID
	m.pfConfirmKillOf = fmt.Sprintf("%s (%d:%d)", session.Pod, session.LocalPort, session.RemotePort)
}

func (m RootModel) renderPFModal(height int) string {
	title := theme.Title.Render("PORT FORWARDS")

	var lines []string
	if len(m.pfSessions) == 0 {
		lines = append(lines, theme.Dim.Render("No active port-forwards."))
		lines = append(lines, "")
		lines = append(lines, theme.Dim.Render("Start one with: "+
			theme.HelpKey.Render(":pf <pod> <local>:<remote>")))
	}
	for i, s := range m.pfSessions {
		uptime := formatUptime(time.Since(s.Started))
		label := fmt.Sprintf("%-30s %5d:%-5d  %-10s  %s",
			truncatePF(s.Pod, portForwardPodDisplayWidth), s.LocalPort, s.RemotePort, s.Status, uptime)
		if i == m.pfCursor {
			lines = append(lines, theme.TableSelected.Render(" ▸ "+label+" "))
		} else {
			lines = append(lines, theme.Dim.Render("   "+label))
		}
	}

	help := theme.HelpKey.Render("j/k") + theme.HelpDesc.Render(" move  ") +
		theme.HelpKey.Render("x") + theme.HelpDesc.Render(" kill  ") +
		theme.HelpKey.Render("r") + theme.HelpDesc.Render(" refresh  ") +
		theme.HelpKey.Render("esc") + theme.HelpDesc.Render(" close")

	var confirmBlock string
	if m.pfConfirmKillID != "" {
		warn := lipgloss.NewStyle().Foreground(theme.Red).Bold(true).Render("KILL " + m.pfConfirmKillOf + "?")
		prompt := lipgloss.NewStyle().Foreground(theme.Red).Bold(true).Render("[y]es / [n]o")
		confirmBlock = "\n\n" + warn + "\n" + prompt
	}

	content := title + "\n\n" + strings.Join(lines, "\n") + "\n\n" + help + confirmBlock

	box := lipgloss.NewStyle().
		Border(lipgloss.ThickBorder()).
		BorderForeground(theme.ElectricPurp).
		Padding(0, 1).
		Width(clampModalWidth(portForwardModalDesiredWidth, m.width)).
		MaxWidth(m.width).
		Render(content)

	return lipgloss.Place(m.width, height, lipgloss.Center, lipgloss.Center, box)
}

func formatUptime(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/hoursPerDay))
	}
}

func truncatePF(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit-1] + "~"
}

func (m RootModel) handleCmdPalette(key string, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.showCmdPalette = false
		m.cmdInput.Blur()
		return m, nil
	case "enter":
		cmd := strings.TrimSpace(m.cmdInput.Value())
		m.showCmdPalette = false
		m.cmdInput.Blur()
		return m.executePaletteCommand(cmd)
	default:
		var cmd tea.Cmd
		m.cmdInput, cmd = m.cmdInput.Update(msg)
		return m, cmd
	}
}

func (m RootModel) executePaletteCommand(input string) (tea.Model, tea.Cmd) {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return m, nil
	}
	command := strings.ToLower(parts[0])
	if resourceType, resourceCommand := paletteResourceType(command); resourceCommand {
		return m.openPaletteResource(resourceType)
	}

	switch command {
	case "q", "quit":
		return m, tea.Quit
	case "pf", "port-forward":
		return m.startPortForwardFromPalette(parts[1:])
	case "ns":
		return m.executeNamespacePaletteCommand(parts[1:])
	case "logs", "log":
		return m.openPaletteLogs(parts[1:])
	default:
		return m, nil
	}
}

func paletteResourceType(command string) (string, bool) {
	switch command {
	case "pod", "pods":
		return "pods", true
	case "deploy", "dep":
		return "deployments", true
	case "svc":
		return "services", true
	case "sts":
		return "statefulsets", true
	case "ds":
		return "daemonsets", true
	case "cm":
		return "configmaps", true
	case "node", "nodes":
		return "nodes", true
	case "job", "jobs":
		return "jobs", true
	default:
		return "", false
	}
}

func (m RootModel) openPaletteResource(resourceType string) (tea.Model, tea.Cmd) {
	m.browser.SetResourceType(resourceType)
	m.browser.loading = true
	var command tea.Cmd
	if m.screen == ScreenBrowser {
		command = m.browser.Activate()
	} else {
		command = m.transitionTo(ScreenBrowser, true)
	}
	m.persistSession()
	return m, command
}

func (m RootModel) executeNamespacePaletteCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 {
		return m, m.openNamespacePicker()
	}
	m.namespace = args[0]
	return m, m.switchNamespace()
}

func (m RootModel) openPaletteLogs(args []string) (tea.Model, tea.Cmd) {
	podName := ""
	if len(args) > 0 {
		podName = args[0]
	}
	return m, func() tea.Msg {
		return DrillDownMsg{Screen: ScreenLogs, ResourceName: podName}
	}
}

// startPortForwardFromPalette validates and starts a port forward.
func (m RootModel) startPortForwardFromPalette(args []string) (tea.Model, tea.Cmd) {
	if len(args) < portForwardMinimumArgumentCount {
		return m, func() tea.Msg {
			return PortForwardFeedbackMsg{
				Err: errors.New("usage: :pf <pod> <local>:<remote>"),
			}
		}
	}
	pod := args[0]
	local, remote, err := parsePortSpec(args[1])
	if err != nil {
		return m, func() tea.Msg {
			return PortForwardFeedbackMsg{Err: err}
		}
	}
	ns := m.namespace
	if ns == "" {
		return m, func() tea.Msg {
			return PortForwardFeedbackMsg{
				Err: errors.New(":pf requires a single namespace (switch out of all-namespaces mode first)"),
			}
		}
	}
	return m, service.StartPortForward(ns, pod, local, remote)
}

// parsePortSpec parses a positive local:remote port pair.
func parsePortSpec(spec string) (local, remote int, err error) {
	parts := strings.SplitN(spec, ":", portSpecPartCount)
	if len(parts) != portSpecPartCount {
		return 0, 0, fmt.Errorf("port spec %q: expected <local>:<remote>", spec)
	}
	local, err = strconv.Atoi(parts[0])
	if err != nil || local <= 0 {
		return 0, 0, fmt.Errorf("port spec %q: invalid local port", spec)
	}
	remote, err = strconv.Atoi(parts[1])
	if err != nil || remote <= 0 {
		return 0, 0, fmt.Errorf("port spec %q: invalid remote port", spec)
	}
	return local, remote, nil
}

// PortForwardFeedbackMsg carries command-palette validation failures.
type PortForwardFeedbackMsg struct {
	Err error
}

func (m RootModel) handleDrillDown(msg DrillDownMsg) (tea.Model, tea.Cmd) {
	m.prepareDrillDown(msg)
	commands := make([]tea.Cmd, 0, rootActivationCommandCapacity)
	commands = appendCommand(commands, m.transitionTo(msg.Screen, true))
	commands = appendCommand(commands, m.drillDownCommand(msg))
	m.persistSession()
	return m, tea.Batch(commands...)
}

func (m *RootModel) prepareDrillDown(msg DrillDownMsg) {
	switch msg.Screen {
	case ScreenBrowser:
		if resourceType := browserResourceType(msg.ResourceType); resourceType != "" {
			m.browser.SetResourceType(resourceType)
		}
	case ScreenLogs:
		m.prepareLogDrillDown(msg)
	case ScreenDashboard, ScreenAI, ScreenHelm, ScreenCRDs:
	}
}

func (m *RootModel) prepareLogDrillDown(msg DrillDownMsg) {
	if msg.ResourceName == "" {
		return
	}
	m.logs.SetPodInNamespace(msg.ResourceName, defaultNamespace(msg.ResourceNS, m.namespace))
}

func (m RootModel) drillDownCommand(msg DrillDownMsg) tea.Cmd {
	if msg.ResourceName == "" {
		return nil
	}
	switch msg.Screen {
	case ScreenBrowser:
		return service.DescribeResource(defaultNamespace(msg.ResourceNS, m.namespace), msg.ResourceType, msg.ResourceName)
	case ScreenLogs:
		return m.logs.SetPodCmd()
	case ScreenDashboard, ScreenAI, ScreenHelm, ScreenCRDs:
		return nil
	}
	return nil
}

func appendCommand(commands []tea.Cmd, command tea.Cmd) []tea.Cmd {
	if command == nil {
		return commands
	}
	return append(commands, command)
}

func defaultNamespace(resourceNamespace, activeNamespace string) string {
	if resourceNamespace != "" {
		return resourceNamespace
	}
	return activeNamespace
}

func browserResourceType(kind string) string {
	if _, ok := resourceCatalog[kind]; ok {
		return kind
	}
	for resourceType, binding := range resourceCatalog {
		if binding.Singular == kind {
			return resourceType
		}
	}
	return ""
}

func (m RootModel) handleNSPicker(key string) (tea.Model, tea.Cmd) {
	totalItems := len(m.namespaces) + allNamespacesPickerItemCount
	switch key {
	case "esc", "n":
		m.showNSPicker = false
		return m, nil
	case "up", "k":
		m.nsCursor = max(0, m.nsCursor-1)
	case "down", "j":
		m.nsCursor = min(totalItems-1, m.nsCursor+1)
	case "enter":
		return m, m.selectNamespace(m.nsCursor)
	}
	return m, nil
}

func (m RootModel) handleNSPickerMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch event := msg.(type) {
	case tea.MouseClickMsg:
		return m.handleNamespacePickerClick(event)
	case tea.MouseWheelMsg:
		totalItems := len(m.namespaces) + allNamespacesPickerItemCount
		m.nsCursor = pickerCursorAfterWheel(m.nsCursor, totalItems, event.Button)
	}
	return m, nil
}

func (m RootModel) handleCtxPickerMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch event := msg.(type) {
	case tea.MouseClickMsg:
		return m.handleContextPickerClick(event)
	case tea.MouseWheelMsg:
		m.ctxCursor = pickerCursorAfterWheel(m.ctxCursor, len(m.contexts), event.Button)
	}
	return m, nil
}

type rootPickerWindow struct {
	start   int
	end     int
	itemTop int
}

type rootPickerItem struct {
	label   string
	current bool
}

func (m RootModel) handleNamespacePickerClick(click tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	totalItems := len(m.namespaces) + allNamespacesPickerItemCount
	window := calculatePickerWindow(m.height-rootStatusBarHeight, totalItems, m.nsCursor)
	selectedIndex, selected := pickerClickedIndex(click, window)
	if !selected {
		return m, nil
	}
	m.nsCursor = selectedIndex
	return m, m.selectNamespace(selectedIndex)
}

func (m RootModel) handleContextPickerClick(click tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	window := calculatePickerWindow(m.height-rootStatusBarHeight, len(m.contexts), m.ctxCursor)
	selectedIndex, selected := pickerClickedIndex(click, window)
	if !selected {
		return m, nil
	}
	m.ctxCursor = selectedIndex
	return m, m.selectContext(selectedIndex)
}

func calculatePickerWindow(contentHeight, totalItems, cursor int) rootPickerWindow {
	maximumVisible := max(pickerMinimumVisibleItems, contentHeight-pickerChromeHeight)
	visibleCount := min(maximumVisible, totalItems)
	normalizedCursor := min(max(0, totalItems-1), max(0, cursor))
	start := max(0, normalizedCursor-maximumVisible+1)
	return rootPickerWindow{
		start:   start,
		end:     min(start+maximumVisible, totalItems),
		itemTop: (contentHeight-(visibleCount+pickerChromeHeight))/2 + pickerItemTopOffset,
	}
}

func pickerClickedIndex(click tea.MouseClickMsg, window rootPickerWindow) (int, bool) {
	visibleCount := window.end - window.start
	if click.Button != tea.MouseLeft || click.Y < window.itemTop || click.Y >= window.itemTop+visibleCount {
		return 0, false
	}
	return window.start + click.Y - window.itemTop, true
}

func pickerCursorAfterWheel(cursor, totalItems int, button tea.MouseButton) int {
	switch button {
	case tea.MouseWheelUp:
		return max(0, cursor-1)
	case tea.MouseWheelDown:
		return min(max(0, totalItems-1), cursor+1)
	default:
		return cursor
	}
}

func (m *RootModel) selectNamespace(index int) tea.Cmd {
	if index < allNamespacesPickerIndex || index > len(m.namespaces) {
		return nil
	}
	m.showNSPicker = false
	if index == allNamespacesPickerIndex {
		m.namespace = ""
	} else if namespaceIndex := index - allNamespacesPickerItemCount; namespaceIndex < len(m.namespaces) {
		m.namespace = m.namespaces[namespaceIndex]
	}
	return m.switchNamespace()
}

func (m RootModel) renderNSPicker(height int) string {
	title := theme.Title.Render("SELECT NAMESPACE")
	if m.nsLoading {
		return m.renderPickerState(height, title, m.nsSpinner.View()+" Loading...")
	}
	if len(m.namespaces) == 0 {
		return m.renderPickerState(height, title, "No namespaces found.")
	}

	items := renderPickerItems(namespacePickerItems(m.namespaces, m.namespace), m.nsCursor, height)
	content := title + "\n\n" + strings.Join(items, "\n") + "\n\n" +
		theme.HelpKey.Render("enter") + theme.HelpDesc.Render(": select  ") +
		theme.HelpKey.Render("esc") + theme.HelpDesc.Render(": cancel")
	box := lipgloss.NewStyle().
		Border(lipgloss.ThickBorder()).
		BorderForeground(theme.NeonCyan).
		Padding(0, rootHorizontalPadding).
		Width(clampModalWidth(nsPickerModalDesiredWidth, m.width)).
		MaxWidth(m.width).
		Render(content)
	return lipgloss.Place(m.width, height, lipgloss.Center, lipgloss.Center, box)
}

func (m RootModel) renderPickerState(height int, title, message string) string {
	content := theme.BoxStyle.Render(title + "\n\n" + message)
	return lipgloss.Place(m.width, height, lipgloss.Center, lipgloss.Center, content)
}

func namespacePickerItems(namespaces []string, currentNamespace string) []rootPickerItem {
	items := make([]rootPickerItem, 0, len(namespaces)+allNamespacesPickerItemCount)
	items = append(items, rootPickerItem{label: "All Namespaces", current: currentNamespace == ""})
	for _, namespace := range namespaces {
		items = append(items, rootPickerItem{label: namespace, current: namespace == currentNamespace})
	}
	return items
}

func renderPickerItems(items []rootPickerItem, cursor, height int) []string {
	window := calculatePickerWindow(height, len(items), cursor)
	lines := make([]string, 0, window.end-window.start)
	for index := window.start; index < window.end; index++ {
		lines = append(lines, renderPickerItem(items[index], index == cursor))
	}
	return lines
}

func renderPickerItem(item rootPickerItem, selected bool) string {
	switch {
	case selected:
		return theme.TableSelected.Render(fmt.Sprintf(" ▸ %s ", item.label))
	case item.current:
		return theme.Accent.Render(fmt.Sprintf("   %s ●", item.label))
	default:
		return theme.Dim.Render("   " + item.label)
	}
}

func (m RootModel) handleCtxPicker(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.showCtxPicker = false
		return m, nil
	case "up", "k":
		m.ctxCursor = max(0, m.ctxCursor-1)
	case "down", "j":
		m.ctxCursor = min(max(0, len(m.contexts)-1), m.ctxCursor+1)
	case "enter":
		return m, m.selectContext(m.ctxCursor)
	}
	return m, nil
}

func (m *RootModel) selectContext(index int) tea.Cmd {
	if index < 0 || index >= len(m.contexts) {
		return nil
	}
	selected := m.contexts[index]
	m.showCtxPicker = false
	if selected.Current {
		return nil
	}
	return service.SwitchContext(selected.Name)
}

func (m RootModel) renderCtxPicker(height int) string {
	title := theme.Title.Render("SELECT CONTEXT")
	if m.ctxLoading {
		return m.renderPickerState(height, title, m.nsSpinner.View()+" Loading contexts...")
	}
	if len(m.contexts) == 0 {
		return m.renderPickerState(height, title, "No contexts found.")
	}

	items := renderPickerItems(contextPickerItems(m.contexts), m.ctxCursor, height)
	content := title + "\n\n" + strings.Join(items, "\n") + "\n\n" +
		theme.HelpKey.Render("enter") + theme.HelpDesc.Render(": switch  ") +
		theme.HelpKey.Render("esc") + theme.HelpDesc.Render(": cancel")
	box := lipgloss.NewStyle().
		Border(lipgloss.ThickBorder()).
		BorderForeground(theme.ElectricPurp).
		Padding(0, rootHorizontalPadding).
		Width(clampModalWidth(ctxPickerModalDesiredWidth, m.width)).
		MaxWidth(m.width).
		Render(content)
	return lipgloss.Place(m.width, height, lipgloss.Center, lipgloss.Center, box)
}

func contextPickerItems(contexts []service.KubeContext) []rootPickerItem {
	items := make([]rootPickerItem, 0, len(contexts))
	for _, context := range contexts {
		items = append(items, rootPickerItem{label: context.Name, current: context.Current})
	}
	return items
}

// switchNamespace updates every screen and persists the selection.
func (m *RootModel) switchNamespace() tea.Cmd {
	m.deactivateScreen(m.screen)
	m.dashboard.SetNamespace(m.namespace)
	m.browser.SetNamespace(m.namespace)
	m.logs.SetNamespace(m.namespace)
	m.helm.SetNamespace(m.namespace)
	m.crds.SetNamespace(m.namespace)
	m.aiPanel.SetNamespace(m.namespace)
	m.persistSession()
	return tea.Batch(m.activateScreen(m.screen)...)
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
	m.resizeAssistantPanel(mainWidth, contentHeight)
}

func (m RootModel) rootMainWidth() int {
	if !m.aiPanel.IsVisible() || m.screen == ScreenAI {
		return m.width
	}
	return m.width - assistantPanelWidth(m.width)
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

func (m *RootModel) resizeAssistantPanel(mainWidth, contentHeight int) {
	if !m.aiPanel.IsVisible() {
		return
	}
	if m.screen == ScreenAI {
		m.aiPanel.SetSize(m.width, contentHeight)
		return
	}
	aiWidth := m.width - mainWidth
	m.aiPanel.SetSize(aiWidth, m.assistantPanelHeight(contentHeight))
}

func (m RootModel) assistantPanelHeight(contentHeight int) int {
	switch m.screen {
	case ScreenBrowser:
		_, panelHeight, _ := m.browser.AIOverlayBounds(contentHeight)
		return panelHeight
	case ScreenLogs:
		_, panelHeight, _ := m.logs.AIOverlayBounds(contentHeight)
		return panelHeight
	case ScreenHelm:
		_, panelHeight, _ := m.helm.AIOverlayBounds(contentHeight)
		return panelHeight
	case ScreenCRDs:
		_, panelHeight, _ := m.crds.AIOverlayBounds(contentHeight)
		return panelHeight
	case ScreenDashboard, ScreenAI:
		return contentHeight
	}
	return contentHeight
}

func (m *RootModel) refreshAssistantContext(owner screenID, accepted bool) {
	if accepted && m.screen == owner && m.aiPanel.IsVisible() {
		m.updateAIScreenContext()
	}
}

func (m *RootModel) updateAIScreenContext() {
	context, err := m.currentScreenContext()
	if err != nil {
		m.aiPanel.SetScreenContext("")
		m.setError(fmt.Errorf("build screen context: %w", err))
		return
	}
	m.aiPanel.SetScreenContext(context)
}

func (m RootModel) currentScreenContext() (string, error) {
	switch m.screen {
	case ScreenDashboard:
		return service.BuildDashboardContext(service.DashboardContextInput{
			Namespace:   m.namespace,
			Pods:        m.dashboard.pods,
			Deployments: m.dashboard.deployments,
			Events:      m.dashboard.events,
		}), nil
	case ScreenBrowser:
		return m.browserScreenContext()
	case ScreenLogs:
		return service.BuildLogsContext(
			m.namespace, m.logs.selectedPod,
			m.logs.allLines, m.logs.filter,
		)
	case ScreenAI, ScreenHelm, ScreenCRDs:
		return "", nil
	default:
		return "", fmt.Errorf("build screen context: unknown screen %d", m.screen)
	}
}

func (m RootModel) browserScreenContext() (string, error) {
	selectedIdentity, _ := m.browser.selectedIdentity()
	input, err := service.NewBrowserContextInput(
		service.BrowserContextSelection{
			Namespace:         m.namespace,
			Resource:          service.BrowserResourceKind(m.browser.resourceType),
			SelectedName:      selectedIdentity.Name,
			SelectedNamespace: selectedIdentity.Namespace,
			DetailContent:     m.browser.providerDetailContext(),
		},
		service.BrowserSnapshot{
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
	return service.BuildBrowserContext(input)
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
	case ScreenDashboard, ScreenAI:
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
