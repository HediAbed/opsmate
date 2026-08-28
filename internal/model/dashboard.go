package model

import (
	"cmp"
	"fmt"
	"image/color"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/HediAbed/opsmate/internal/service"
	"github.com/HediAbed/opsmate/internal/theme"
)

// Metrics require polling because Kubernetes does not expose a watch stream for them.
const dashMetricsRefreshInterval = 30 * time.Second

const (
	dashboardInitialWidth              = 60
	dashboardInitialTableHeight        = 6
	dashboardPanelOuterChrome          = 2
	dashboardHorizontalChrome          = 4
	dashboardMinimumInnerWidth         = 20
	dashboardSectionCapacity           = 10
	dashboardMinimumTableHeight        = 3
	dashboardPodColumnCount            = 7
	dashboardPodColumnBaseWidth        = 35
	dashboardStatusWidthDivisor        = 12
	dashboardReadyWidthDivisor         = 18
	dashboardRestartWidthDivisor       = 22
	dashboardMetricWidthDivisor        = 16
	dashboardStatusMinimumWidth        = 6
	dashboardReadyMinimumWidth         = 4
	dashboardRestartMinimumWidth       = 3
	dashboardAgeMinimumWidth           = 4
	dashboardMetricMinimumWidth        = 5
	dashboardOverviewBarMaximumWidth   = 30
	dashboardOverviewBarWidthDivisor   = 3
	dashboardTopConsumerLimit          = 5
	dashboardNameMinimumWidth          = 10
	dashboardInfoColumnsWidth          = 30
	milliCPUPerCore                    = 1000
	dashboardCriticalUsageThreshold    = 0.9
	dashboardWarningUsageThreshold     = 0.7
	dashboardAlertLimit                = 4
	dashboardRestartAlertThreshold     = 10
	dashboardDeploymentBarMinimumWidth = 8
	dashboardDeploymentBarWidthDivisor = 4
	dashboardDeploymentLimit           = 6
	deploymentReadyPartCount           = 2
	dashboardEventLimit                = 4
	dashboardEventMessageMinimumWidth  = 20
	dashboardEventMessageReservedWidth = 45
)

type dashMetricsTickMsg struct{}

type dashboardDataKind uint8

const (
	dashboardPods dashboardDataKind = iota
	dashboardDeployments
	dashboardEvents
	dashboardMetrics
	dashboardDataKindCount
)

type dashboardResultMsg struct {
	kind      dashboardDataKind
	requestID uint64
	namespace string
	payload   tea.Msg
}

type dashboardHealthResultMsg struct {
	requestID uint64
	namespace string
	payload   tea.Msg
}

type dashboardPodReconnectMsg struct {
	namespace  string
	generation uint64
}

type dashboardDeploymentReconnectMsg struct {
	namespace  string
	generation uint64
}

type dashboardEventReconnectMsg struct {
	namespace  string
	generation uint64
}

type dashboardPodWatchClosedMsg struct{}
type dashboardDeploymentWatchClosedMsg struct{}
type dashboardEventWatchClosedMsg struct{}

type DashboardModel struct {
	width  int
	height int

	namespace string

	pods        []service.Pod
	deployments []service.Deployment
	events      []service.Event
	metrics     []service.PodMetric
	podTable    table.Model
	podRows     []resourceIdentity
	spinner     spinner.Model
	bodyView    viewport.Model

	loading     bool
	err         error
	lastRefresh time.Time

	aiHealthSummary string
	aiHealthLoading bool
	aiHealthErr     error
	showAIHealth    bool
	healthRequestID uint64

	podWatcher        watchSupervisor[service.Pod]
	deploymentWatcher watchSupervisor[service.Deployment]
	eventWatcher      watchSupervisor[service.Event]

	active bool

	requestIDs [dashboardDataKindCount]uint64
}

func NewDashboardModel(namespace string) DashboardModel {
	s := spinner.New(
		spinner.WithSpinner(spinner.Dot),
		spinner.WithStyle(theme.SpinnerStyle),
	)

	t := table.New(
		table.WithColumns(dashPodColumns(dashboardInitialWidth)),
		table.WithRows([]table.Row{}),
		table.WithFocused(true),
		table.WithHeight(dashboardInitialTableHeight),
		table.WithWidth(dashboardInitialWidth),
	)
	t.SetStyles(dashTableStyles())

	return DashboardModel{
		namespace: namespace,
		podTable:  t,
		spinner:   s,
		loading:   true,
		bodyView:  viewport.New(),
	}
}

func (m DashboardModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, scheduleMetricsTick())
}

func scheduleMetricsTick() tea.Cmd {
	return tea.Tick(dashMetricsRefreshInterval, newDashMetricsTickMessage)
}

func newDashMetricsTickMessage(time.Time) tea.Msg {
	return dashMetricsTickMsg{}
}

func (m DashboardModel) Update(msg tea.Msg) (next DashboardModel, command tea.Cmd) {
	defer func() {
		next.syncDashboardLayout()
	}()
	if updated, cmd, handled := m.updateDashboardLifecycleMessage(msg); handled {
		return updated, cmd
	}
	if updated, cmd, handled := m.updateDashboardWatchMessage(msg); handled {
		return updated, cmd
	}
	if updated, cmd, handled := m.updateDashboardDataMessage(msg); handled {
		return updated, cmd
	}
	return m.updateDashboardInputMessage(msg)
}

func (m DashboardModel) updateDashboardLifecycleMessage(msg tea.Msg) (DashboardModel, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case dashboardResultMsg:
		updated, command := m.handleDashboardResult(msg)
		return updated, command, true
	case dashboardHealthResultMsg:
		updated, command := m.handleDashboardHealthResult(msg)
		return updated, command, true
	case supervisedWatchMsg:
		updated, command := m.handleSupervisedWatchMessage(msg)
		return updated, command, true
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.recalcLayout()
		return m, nil, true
	case dashMetricsTickMsg:
		return m, m.handleDashboardMetricsTick(), true
	default:
		return m, nil, false
	}
}

func (m DashboardModel) handleDashboardResult(msg dashboardResultMsg) (DashboardModel, tea.Cmd) {
	if !m.acceptsResult(msg) {
		return m, nil
	}
	return m.Update(msg.payload)
}

func (m DashboardModel) handleDashboardHealthResult(msg dashboardHealthResultMsg) (DashboardModel, tea.Cmd) {
	if msg.requestID != m.healthRequestID || msg.namespace != m.namespace {
		return m, nil
	}
	return m.Update(msg.payload)
}

func (m *DashboardModel) handleDashboardMetricsTick() tea.Cmd {
	commands := []tea.Cmd{scheduleMetricsTick()}
	if m.active {
		commands = append(commands, m.fetchDashboardData(dashboardMetrics, service.FetchPodMetrics(m.namespace)))
	}
	return tea.Batch(commands...)
}

func (m DashboardModel) updateDashboardWatchMessage(msg tea.Msg) (DashboardModel, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case service.WatchEventMsg[service.Pod]:
		updated, command := m.handlePodWatchEvent(msg)
		return updated, command, true
	case service.WatchEventMsg[service.Deployment]:
		updated, command := m.handleDeploymentWatchEvent(msg)
		return updated, command, true
	case service.WatchEventMsg[service.Event]:
		updated, command := m.handleEventWatchEvent(msg)
		return updated, command, true
	case dashboardPodWatchClosedMsg:
		updated, command := m.handlePodWatchClosed()
		return updated, command, true
	case dashboardDeploymentWatchClosedMsg:
		updated, command := m.handleDeploymentWatchClosed()
		return updated, command, true
	case dashboardEventWatchClosedMsg:
		updated, command := m.handleEventWatchClosed()
		return updated, command, true
	default:
		return m.updateDashboardReconnectMessage(msg)
	}
}

func (m DashboardModel) updateDashboardReconnectMessage(msg tea.Msg) (DashboardModel, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case dashboardPodReconnectMsg:
		return m.handlePodReconnect(msg)
	case dashboardDeploymentReconnectMsg:
		return m.handleDeploymentReconnect(msg)
	case dashboardEventReconnectMsg:
		return m.handleEventReconnect(msg)
	default:
		return m, nil, false
	}
}

func (m DashboardModel) handlePodReconnect(msg dashboardPodReconnectMsg) (DashboardModel, tea.Cmd, bool) {
	if !m.active || m.namespace != msg.namespace || !m.podWatcher.OwnsGeneration(msg.generation) {
		return m, nil, true
	}
	command := m.podWatcher.SetWithClose(
		service.WatchPods(freshContext(), m.namespace),
		dashboardPodWatchClosedMsg{},
	)
	return m, command, true
}

func (m DashboardModel) handleDeploymentReconnect(msg dashboardDeploymentReconnectMsg) (DashboardModel, tea.Cmd, bool) {
	if !m.active || m.namespace != msg.namespace || !m.deploymentWatcher.OwnsGeneration(msg.generation) {
		return m, nil, true
	}
	command := m.deploymentWatcher.SetWithClose(
		service.WatchDeployments(freshContext(), m.namespace),
		dashboardDeploymentWatchClosedMsg{},
	)
	return m, command, true
}

func (m DashboardModel) handleEventReconnect(msg dashboardEventReconnectMsg) (DashboardModel, tea.Cmd, bool) {
	if !m.active || m.namespace != msg.namespace || !m.eventWatcher.OwnsGeneration(msg.generation) {
		return m, nil, true
	}
	command := m.eventWatcher.SetWithClose(
		service.WatchEvents(freshContext(), m.namespace),
		dashboardEventWatchClosedMsg{},
	)
	return m, command, true
}

func (m DashboardModel) updateDashboardDataMessage(msg tea.Msg) (DashboardModel, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case service.PodsMsg:
		m.applyDashboardPods(msg)
		return m, nil, true
	case service.DeploymentsMsg:
		m.applyDashboardDeployments(msg)
		return m, nil, true
	case service.EventsMsg:
		m.applyDashboardEvents(msg)
		return m, nil, true
	case service.MetricsMsg:
		return m, m.applyDashboardMetrics(msg), true
	case service.DashHealthMsg:
		m.applyDashboardHealth(msg)
		return m, nil, true
	case spinner.TickMsg:
		return m, m.handleDashboardSpinnerTick(msg), true
	default:
		return m, nil, false
	}
}

func (m *DashboardModel) applyDashboardPods(msg service.PodsMsg) {
	if msg.Err != nil {
		m.err = msg.Err
	} else {
		m.pods = msg.Pods
		m.err = nil
	}
	m.loading = false
	m.lastRefresh = time.Now()
	m.rebuildTableRows()
}

func (m *DashboardModel) applyDashboardDeployments(msg service.DeploymentsMsg) {
	if msg.Err != nil {
		if m.err == nil {
			m.err = msg.Err
		}
		return
	}
	m.deployments = msg.Deployments
}

func (m *DashboardModel) applyDashboardEvents(msg service.EventsMsg) {
	if msg.Err != nil {
		if m.err == nil {
			m.err = msg.Err
		}
		return
	}
	m.events = msg.Events
}

func (m *DashboardModel) applyDashboardMetrics(msg service.MetricsMsg) tea.Cmd {
	if msg.Err != nil {
		m.metrics = nil
	} else {
		m.metrics = msg.PodMetrics
		m.mergeMetrics()
		m.rebuildTableRows()
	}
	if !m.showAIHealth || m.aiHealthLoading {
		return nil
	}
	m.aiHealthLoading = true
	return m.fetchHealthSummary()
}

func (m *DashboardModel) applyDashboardHealth(msg service.DashHealthMsg) {
	m.aiHealthLoading = false
	if msg.Err != nil {
		m.aiHealthErr = msg.Err
		m.aiHealthSummary = ""
		return
	}
	m.aiHealthErr = nil
	m.aiHealthSummary = sanitizeTerminalText(msg.Summary)
}

func (m *DashboardModel) handleDashboardSpinnerTick(msg spinner.TickMsg) tea.Cmd {
	if !m.loading && !m.aiHealthLoading {
		return nil
	}
	var command tea.Cmd
	m.spinner, command = m.spinner.Update(msg)
	return command
}

func (m DashboardModel) updateDashboardInputMessage(msg tea.Msg) (DashboardModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.MouseWheelMsg:
		return m.handleDashboardMouseWheel(msg)
	case tea.MouseClickMsg:
		m.handleDashboardMouseClick(msg)
		return m, nil
	case tea.KeyPressMsg:
		return m.handleDashboardKey(msg)
	default:
		return m.forwardDashboardTableMessage(msg)
	}
}

func (m DashboardModel) handleDashboardMouseWheel(msg tea.MouseWheelMsg) (DashboardModel, tea.Cmd) {
	if updated, command, handled := m.routeScrollToBody(msg); handled {
		return updated, command
	}
	switch msg.Button {
	case tea.MouseWheelUp:
		m.podTable.MoveUp(tableWheelStep)
	case tea.MouseWheelDown:
		m.podTable.MoveDown(tableWheelStep)
	}
	return m, nil
}

func (m *DashboardModel) handleDashboardMouseClick(msg tea.MouseClickMsg) {
	if msg.Button != tea.MouseLeft {
		return
	}
	rowIndex := msg.Y - m.podTableTopBoundary() - dashTableHeaderRows
	if rowIndex >= 0 && rowIndex < len(m.podTable.Rows()) {
		m.podTable.SetCursor(rowIndex)
	}
}

func (m DashboardModel) handleDashboardKey(msg tea.KeyPressMsg) (DashboardModel, tea.Cmd) {
	switch msg.String() {
	case "a":
		return m.toggleDashboardHealth()
	case "r":
		return m, m.refreshAll()
	case "enter":
		return m, m.selectedPodDrillDown(ScreenBrowser)
	case "l":
		return m, m.selectedPodDrillDown(ScreenLogs)
	case "esc":
		m.showAIHealth = false
		return m, nil
	case "j", "down", "pgdown", "k", "up", "pgup":
		if updated, command, handled := m.routeScrollToBody(msg); handled {
			return updated, command
		}
	}
	return m.forwardDashboardTableMessage(msg)
}

func (m DashboardModel) toggleDashboardHealth() (DashboardModel, tea.Cmd) {
	m.showAIHealth = !m.showAIHealth
	if !m.showAIHealth || m.aiHealthSummary != "" || m.aiHealthLoading {
		return m, nil
	}
	m.aiHealthLoading = true
	return m, m.fetchHealthSummary()
}

func (m DashboardModel) selectedPodDrillDown(screen screenID) tea.Cmd {
	pod := m.SelectedPod()
	if pod == "" {
		return nil
	}
	namespace := m.SelectedPodNS()
	return func() tea.Msg {
		return DrillDownMsg{Screen: screen, ResourceType: "pod", ResourceName: pod, ResourceNS: namespace}
	}
}

func (m DashboardModel) forwardDashboardTableMessage(msg tea.Msg) (DashboardModel, tea.Cmd) {
	var command tea.Cmd
	m.podTable, command = m.podTable.Update(msg)
	return m, command
}

func (m DashboardModel) bodyOverflows() bool {
	return m.bodyView.TotalLineCount() > m.bodyView.Height() && m.bodyView.Height() > 0
}

func (m DashboardModel) routeScrollToBody(msg tea.Msg) (DashboardModel, tea.Cmd, bool) {
	if !m.bodyOverflows() {
		return m, nil, false
	}
	var cmd tea.Cmd
	m.bodyView, cmd = m.bodyView.Update(msg)
	return m, cmd, true
}

const (
	dashHelpBarRows     = 1
	dashTableHeaderRows = 2
)

var (
	barFillRunning   = lipgloss.NewStyle().Foreground(theme.Green)
	barFillPending   = lipgloss.NewStyle().Foreground(theme.Yellow)
	barFillFailed    = lipgloss.NewStyle().Foreground(theme.Red)
	barFillCritical  = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF4444"))
	barFillWarning   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD700"))
	barFillDimmed    = lipgloss.NewStyle().Foreground(theme.DimText)
	overviewBadgeRun = lipgloss.NewStyle().Foreground(theme.Purple).Bold(true)
	alertCritical    = lipgloss.NewStyle().Foreground(theme.Red).Bold(true)
	alertWarning     = lipgloss.NewStyle().Foreground(theme.Yellow).Bold(true)
	alertInfo        = lipgloss.NewStyle().Foreground(theme.Yellow)
)

func (m DashboardModel) podTableTopBoundary() int {
	return m.height - dashHelpBarRows - (m.podTable.Height() + dashboardPanelOuterChrome)
}

func (m DashboardModel) innerW() int {
	w := m.width - dashboardHorizontalChrome
	if w < dashboardMinimumInnerWidth {
		w = dashboardMinimumInnerWidth
	}
	return w
}

type dashboardSections struct {
	title        string
	overview     string
	help         string
	errorBanner  string
	health       string
	alerts       string
	deployments  string
	events       string
	topConsumers string
}

func (m DashboardModel) View() string {
	if m.width == 0 {
		return "Initializing dashboard..."
	}
	sections := m.renderDashboardSections()
	body := m.composeDashboardBody(sections)
	availableHeight := max(1, m.height-lipgloss.Height(sections.help))
	if lipgloss.Height(body) > availableHeight {
		bodyView := m.bodyView
		bodyView.SetContent(body)
		help := sections.help
		if hint := dashboardScrollHint(bodyView); hint != "" {
			help = appendHelpHint(help, hint, m.width)
		}
		return bodyView.View() + "\n" + help
	}
	if gap := availableHeight - lipgloss.Height(body); gap > 0 {
		body += strings.Repeat("\n", gap)
	}
	return body + "\n" + sections.help
}

func (m DashboardModel) renderDashboardSections() dashboardSections {
	sections := dashboardSections{
		title:        m.renderTitleBar(m.width),
		overview:     m.renderOverviewRow(m.width),
		help:         m.renderHelpLine(m.width),
		alerts:       m.renderAlerts(m.innerW()),
		topConsumers: m.renderTopConsumers(m.innerW()),
	}
	if m.err != nil {
		clean := service.SanitizeKubectlStderr(m.err.Error())
		sections.errorBanner = lipgloss.NewStyle().
			Foreground(theme.Red).Bold(true).Width(m.width).Padding(0, 1).
			Render(fmt.Sprintf("⚠ ERROR: %s — press r to retry", clean))
	}
	if m.showAIHealth {
		sections.health = m.renderAIHealth(m.innerW())
	}
	if len(m.deployments) > 0 {
		sections.deployments = m.renderDeploymentHealth(m.innerW())
	}
	if len(m.events) > 0 {
		sections.events = m.renderEvents(m.innerW())
	}
	return sections
}

func (m DashboardModel) composeDashboardBody(sections dashboardSections) string {
	content := make([]string, 0, dashboardSectionCapacity)
	content = append(content, sections.title)
	if sections.errorBanner != "" {
		content = append(content, sections.errorBanner)
	}
	content = append(content, sections.overview)
	if sections.health != "" {
		content = append(content, sections.health)
	}
	if sections.alerts != "" {
		content = append(content, sections.alerts)
	}
	if sections.deployments != "" {
		content = append(content, sections.deployments)
	}
	if sections.topConsumers != "" {
		content = append(content, sections.topConsumers)
	}
	content = append(content, m.renderPodTable(m.innerW()))
	if sections.events != "" {
		content = append(content, sections.events)
	}
	return lipgloss.JoinVertical(lipgloss.Left, content...)
}

func (m *DashboardModel) syncDashboardLayout() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	sections := m.renderDashboardSections()
	chromeHeight := dashboardPanelOuterChrome
	for _, section := range []string{
		sections.title,
		sections.overview,
		sections.help,
		sections.errorBanner,
		sections.health,
		sections.alerts,
		sections.deployments,
		sections.events,
		sections.topConsumers,
	} {
		chromeHeight += lipgloss.Height(section)
	}
	tableWidth := max(1, m.innerW()-dashboardPanelOuterChrome)
	m.podTable.SetWidth(tableWidth)
	m.podTable.SetColumns(dashPodColumns(tableWidth))
	m.podTable.SetHeight(max(dashboardMinimumTableHeight, m.height-chromeHeight))

	body := m.composeDashboardBody(sections)
	availableHeight := max(1, m.height-lipgloss.Height(sections.help))
	m.bodyView.SetWidth(m.width)
	m.bodyView.SetHeight(availableHeight)
	m.bodyView.SetContent(body)
}

func dashboardScrollHint(v viewport.Model) string {
	dir := viewportScrollDirection(v)
	if dir == "" {
		return ""
	}
	return fmt.Sprintf("%d%% %s", viewportScrollPct(v), dir)
}

func (m *DashboardModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.recalcLayout()
}

func (m DashboardModel) Namespace() string { return m.namespace }

func (m *DashboardModel) SetNamespace(ns string) tea.Cmd {
	defer m.syncDashboardLayout()
	wasActive := m.active
	m.Deactivate()

	m.namespace = ns
	m.pods = nil
	m.deployments = nil
	m.events = nil
	m.metrics = nil
	m.loading = true
	m.err = nil
	m.healthRequestID++
	m.aiHealthLoading = false
	m.aiHealthSummary = ""
	m.aiHealthErr = nil
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
	return tea.Batch(
		m.fetchDashboardData(dashboardPods, service.FetchPods(m.namespace)),
		m.fetchDashboardData(dashboardDeployments, service.FetchDeployments(m.namespace)),
		m.fetchDashboardData(dashboardEvents, service.FetchEvents(m.namespace)),
		m.fetchDashboardData(dashboardMetrics, service.FetchPodMetrics(m.namespace)),
	)
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
	context := service.BuildDashboardContext(service.DashboardContextInput{
		Namespace:   namespace,
		Pods:        m.pods,
		Deployments: m.deployments,
		Events:      m.events,
	})
	command := service.AIClusterHealth(context)
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
	lookup := make(map[string]service.PodMetric, len(m.metrics))
	for _, pm := range m.metrics {
		namespace := pm.Namespace
		if namespace == "" {
			namespace = m.namespace
		}
		lookup[namespacedResourceKey(namespace, pm.Name)] = pm
	}
	for i := range m.pods {
		namespace := m.pods[i].Namespace
		if namespace == "" {
			namespace = m.namespace
		}
		key := namespacedResourceKey(namespace, m.pods[i].Name)
		if pm, ok := lookup[key]; ok {
			m.pods[i].CPU = pm.CPU
			m.pods[i].Memory = pm.Memory
		}
	}
}

func (m *DashboardModel) recalcLayout() {
	m.syncDashboardLayout()
}

func dashPodColumns(innerW int) []table.Column {
	cellPad := dashboardPodColumnCount * tableCellPadding
	minW := dashboardPodColumnBaseWidth + cellPad
	if innerW < minW {
		innerW = minW
	}
	available := innerW - cellPad
	statusW := max(dashboardStatusMinimumWidth, available/dashboardStatusWidthDivisor)
	readyW := max(dashboardReadyMinimumWidth, available/dashboardReadyWidthDivisor)
	rstW := max(dashboardRestartMinimumWidth, available/dashboardRestartWidthDivisor)
	ageW := max(dashboardAgeMinimumWidth, available/dashboardReadyWidthDivisor)
	cpuW := max(dashboardMetricMinimumWidth, available/dashboardMetricWidthDivisor)
	memW := max(dashboardMetricMinimumWidth, available/dashboardMetricWidthDivisor)
	fixed := statusW + readyW + rstW + ageW + cpuW + memW
	nameW := available - fixed
	return []table.Column{
		{Title: "NAME", Width: nameW},
		{Title: "STATUS", Width: statusW},
		{Title: "READY", Width: readyW},
		{Title: "RST", Width: rstW},
		{Title: "AGE", Width: ageW},
		{Title: "CPU", Width: cpuW},
		{Title: "MEM", Width: memW},
	}
}

func dashTableStyles() table.Styles {
	s := table.DefaultStyles()
	s.Header = s.Header.Bold(true).Foreground(theme.NeonCyan).
		BorderStyle(lipgloss.NormalBorder()).BorderForeground(theme.DimText).BorderBottom(true)
	s.Selected = s.Selected.Foreground(theme.White).Background(theme.DeepViolet).Bold(true)
	s.Cell = s.Cell.Foreground(theme.LightText)
	return s
}

func (m *DashboardModel) rebuildTableRows() {
	rows := make([]table.Row, 0, len(m.pods))
	identities := make([]resourceIdentity, 0, len(m.pods))
	for _, p := range m.pods {
		cpu, mem := p.CPU, p.Memory
		if cpu == "" {
			cpu = "-"
		}
		if mem == "" {
			mem = "-"
		}
		rows = append(rows, table.Row{
			displayResourceName(p.Namespace, p.Name, m.namespace == ""),
			p.Status,
			p.Ready,
			strconv.Itoa(p.Restarts),
			p.Age,
			cpu,
			mem,
		})
		identities = append(identities, resourceIdentity{Kind: "pod", Namespace: p.Namespace, Name: p.Name})
	}
	m.podRows = identities
	m.podTable.SetRows(rows)
}

func (m DashboardModel) renderTitleBar(w int) string {
	title := theme.Title.Render("CLUSTER MONITOR")
	ns := theme.Subtitle.Render(m.namespace)

	right := ""
	if m.loading {
		right = m.spinner.View() + theme.SpinnerStyle.Render(fmt.Sprintf(" Syncing %s…", m.namespace))
	} else if !m.lastRefresh.IsZero() {
		right = theme.Dim.Render(fmt.Sprintf("last %s", m.lastRefresh.Format("15:04:05")))
	}

	left := title + " " + ns
	gap := max(1, w-lipgloss.Width(left)-lipgloss.Width(right)-1)
	bar := left + strings.Repeat(" ", gap) + right

	return lipgloss.NewStyle().Width(w).Background(theme.DarkerBg).Render(bar)
}

func (m DashboardModel) renderOverviewRow(w int) string {
	total := len(m.pods)
	running, pending, failed := 0, 0, 0
	for _, p := range m.pods {
		switch p.Status {
		case "Running":
			running++
		case "Pending":
			pending++
		case "Failed", "Error", "CrashLoopBackOff", "ImagePullBackOff":
			failed++
		}
	}

	badge := func(label string, val int, style lipgloss.Style) string {
		return style.Padding(0, 1).Render(fmt.Sprintf("%s:%d", label, val))
	}

	badges := lipgloss.JoinHorizontal(lipgloss.Center,
		badge("Pods", total, theme.Accent), " ",
		badge("Running", running, theme.Success), " ",
		badge("Pending", pending, theme.Warning), " ",
		badge("Failed", failed, theme.Error), " ",
		badge("Deploys", len(m.deployments), overviewBadgeRun),
	)

	var distBar string
	if total > 0 {
		barW := min(dashboardOverviewBarMaximumWidth, w/dashboardOverviewBarWidthDivisor)
		runPct := float64(running) / float64(total)
		pendPct := float64(pending) / float64(total)
		failPct := float64(failed) / float64(total)
		runW := int(math.Round(runPct * float64(barW)))
		pendW := int(math.Round(pendPct * float64(barW)))
		failW := int(math.Round(failPct * float64(barW)))
		otherW := barW - runW - pendW - failW
		if otherW < 0 {
			otherW = 0
		}
		distBar = " " +
			barFillRunning.Render(strings.Repeat("█", runW)) +
			barFillPending.Render(strings.Repeat("█", pendW)) +
			barFillFailed.Render(strings.Repeat("█", failW)) +
			barFillDimmed.Render(strings.Repeat("░", otherW))
	}

	row := badges + distBar
	return lipgloss.NewStyle().Width(w).Padding(0, 1).Render(row)
}

func (m DashboardModel) renderTopConsumers(innerW int) string {
	if len(m.metrics) == 0 {
		return ""
	}

	type podUsage struct {
		name, cpu, mem string
	}
	var usage []podUsage
	for _, p := range m.pods {
		if p.CPU != "" && p.CPU != "-" {
			name := displayResourceName(p.Namespace, p.Name, m.namespace == "")
			usage = append(usage, podUsage{name, p.CPU, p.Memory})
		}
	}
	if len(usage) == 0 {
		return ""
	}

	slices.SortFunc(usage, func(left, right podUsage) int {
		return cmp.Compare(parseMilli(right.cpu), parseMilli(left.cpu))
	})

	maxShow := min(dashboardTopConsumerLimit, len(usage))
	header := theme.Accent.Render("TOP RESOURCE CONSUMERS")
	lines := []string{header}
	nameW := max(dashboardNameMinimumWidth, innerW-dashboardInfoColumnsWidth)
	for i := 0; i < maxShow; i++ {
		u := usage[i]
		name := u.name
		if len(name) > nameW {
			name = name[:nameW-1] + "~"
		}
		lines = append(lines, fmt.Sprintf("  %-*s  CPU:%-8s  Mem:%s", nameW, name, u.cpu, u.mem))
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(theme.BorderColor).
		Padding(0, 1).Width(innerW + dashboardPanelOuterChrome).Render(strings.Join(lines, "\n"))
}

func sortedEventsNewestFirst(events []service.Event) []service.Event {
	warnings := make([]service.Event, 0, len(events))
	others := make([]service.Event, 0, len(events))
	for _, ev := range events {
		if ev.Type == "Warning" {
			warnings = append(warnings, ev)
		} else {
			others = append(others, ev)
		}
	}
	slices.SortFunc(warnings, func(left, right service.Event) int {
		return right.LastTimestamp.Compare(left.LastTimestamp)
	})
	slices.SortFunc(others, func(left, right service.Event) int {
		return right.LastTimestamp.Compare(left.LastTimestamp)
	})
	return append(warnings, others...)
}

// parseMilli parses a Kubernetes CPU value like "100m" or "2" into millicores.
func parseMilli(s string) int {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return 0
	}
	if strings.HasSuffix(s, "m") {
		value, err := strconv.Atoi(strings.TrimSuffix(s, "m"))
		if err != nil {
			return 0
		}
		return value
	}
	value, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return value * milliCPUPerCore
}

func renderBar(pct float64, width int, fillColor, emptyColor color.Color) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 1 {
		pct = 1
	}
	filled := int(math.Round(pct * float64(width)))
	empty := width - filled

	fillStyle := lipgloss.NewStyle().Foreground(fillColor)
	emptyStyle := lipgloss.NewStyle().Foreground(emptyColor)

	if pct > dashboardCriticalUsageThreshold {
		fillStyle = barFillCritical
	} else if pct > dashboardWarningUsageThreshold {
		fillStyle = barFillWarning
	}

	return fillStyle.Render(strings.Repeat("█", filled)) +
		emptyStyle.Render(strings.Repeat("░", empty))
}

type dashboardAlert struct {
	icon   string
	pod    string
	reason string
	style  lipgloss.Style
}

func (m DashboardModel) renderAlerts(innerW int) string {
	alerts := m.collectDashboardAlerts()
	if len(alerts) == 0 {
		return ""
	}

	header := theme.Error.Render("ALERTS") + theme.Dim.Render(fmt.Sprintf(" (%d)", len(alerts)))
	maxAlerts := min(dashboardAlertLimit, len(alerts))
	lines := []string{header}
	nameW := max(dashboardNameMinimumWidth, innerW-dashboardInfoColumnsWidth)
	for i := 0; i < maxAlerts; i++ {
		a := alerts[i]
		name := a.pod
		if len(name) > nameW {
			name = name[:nameW-1] + "~"
		}
		lines = append(lines, a.style.Render(fmt.Sprintf("  %s %-*s %s", a.icon, nameW, name, a.reason)))
	}
	if len(alerts) > maxAlerts {
		lines = append(lines, theme.Dim.Render(fmt.Sprintf("  +%d more", len(alerts)-maxAlerts)))
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(theme.Red).
		Padding(0, 1).Width(innerW + dashboardPanelOuterChrome).Render(strings.Join(lines, "\n"))
}

func (m DashboardModel) collectDashboardAlerts() []dashboardAlert {
	alerts := make([]dashboardAlert, 0)
	for _, pod := range m.pods {
		if alert, found := m.alertForPod(pod); found {
			alerts = append(alerts, alert)
		}
	}
	return alerts
}

func (m DashboardModel) alertForPod(pod service.Pod) (dashboardAlert, bool) {
	name := displayResourceName(pod.Namespace, pod.Name, m.namespace == "")
	switch pod.Status {
	case "CrashLoopBackOff":
		return dashboardAlert{icon: "⚠", pod: name, reason: "CrashLoopBackOff", style: alertCritical}, true
	case "ImagePullBackOff", "ErrImagePull":
		return dashboardAlert{icon: "⚠", pod: name, reason: "ImagePullBackOff", style: alertCritical}, true
	case "Error", "Failed":
		return dashboardAlert{icon: "✗", pod: name, reason: pod.Status, style: alertCritical}, true
	case "Pending":
		return dashboardAlert{icon: "◷", pod: name, reason: "Pending", style: alertWarning}, true
	default:
		if pod.Restarts > dashboardRestartAlertThreshold {
			return dashboardAlert{icon: "↻", pod: name, reason: fmt.Sprintf("Restarts:%d", pod.Restarts), style: alertInfo}, true
		}
		return dashboardAlert{}, false
	}
}

func (m DashboardModel) renderDeploymentHealth(innerW int) string {
	header := theme.Accent.Render("DEPLOYMENT HEALTH")
	barW := max(dashboardDeploymentBarMinimumWidth, innerW/dashboardDeploymentBarWidthDivisor)
	lines := []string{header}
	maxDeploys := min(dashboardDeploymentLimit, len(m.deployments))

	for i := 0; i < maxDeploys; i++ {
		d := m.deployments[i]
		name := displayResourceName(d.Namespace, d.Name, m.namespace == "")
		nameW := max(dashboardNameMinimumWidth, innerW-barW-dashboardInfoColumnsWidth)
		if len(name) > nameW {
			name = name[:nameW-1] + "~"
		}

		ready, desired := parseReadyReplicas(d.Ready)
		pct := 0.0
		if desired > 0 {
			pct = float64(ready) / float64(desired)
		}
		barColor := theme.Green
		if pct < 1.0 {
			barColor = theme.Yellow
		}
		if pct == 0 {
			barColor = theme.Red
		}

		bar := renderBar(pct, barW, barColor, theme.DimText)
		lines = append(lines, fmt.Sprintf("  %-*s %s %s  Age:%s", nameW, name, bar, d.Ready, d.Age))
	}
	if len(m.deployments) > maxDeploys {
		lines = append(lines, theme.Dim.Render(fmt.Sprintf("  +%d more", len(m.deployments)-maxDeploys)))
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(theme.ElectricPurp).
		Padding(0, 1).Width(innerW + dashboardPanelOuterChrome).Render(strings.Join(lines, "\n"))
}

func parseReadyReplicas(value string) (int, int) {
	parts := strings.SplitN(value, "/", deploymentReadyPartCount)
	if len(parts) != deploymentReadyPartCount {
		return 0, 0
	}
	ready, readyErr := strconv.Atoi(parts[0])
	desired, desiredErr := strconv.Atoi(parts[1])
	if readyErr != nil || desiredErr != nil {
		return 0, 0
	}
	return ready, desired
}

func (m DashboardModel) renderPodTable(innerW int) string {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(theme.BorderColor).
		Padding(0, 1).Width(innerW + dashboardPanelOuterChrome)

	if m.loading && len(m.pods) == 0 {
		ns := m.namespace
		if ns == "" {
			ns = "all namespaces"
		}
		body := m.spinner.View() + " " + theme.Dim.Render("Loading pods in "+ns+"...")
		centered := lipgloss.Place(innerW, m.podTable.Height(), lipgloss.Center, lipgloss.Center, body)
		return style.Render(centered)
	}
	if !m.loading && len(m.pods) == 0 {
		ns := m.namespace
		if ns == "" {
			ns = "all namespaces"
		}
		hint := theme.Dim.Render("No pods in "+ns+".") + "\n\n" +
			theme.HelpKey.Render("[n]") + theme.HelpDesc.Render(" namespace  ") +
			theme.HelpKey.Render("[k]") + theme.HelpDesc.Render(" context  ") +
			theme.HelpKey.Render("[:]") + theme.HelpDesc.Render(" command palette")
		centered := lipgloss.Place(innerW, m.podTable.Height(), lipgloss.Center, lipgloss.Center, hint)
		return style.Render(centered)
	}
	return style.Render(m.podTable.View())
}

func (m DashboardModel) renderEvents(innerW int) string {
	header := theme.Accent.Render("RECENT EVENTS")
	maxEvents := min(dashboardEventLimit, len(m.events))
	lines := []string{header}

	sorted := sortedEventsNewestFirst(m.events)

	for i := 0; i < maxEvents && i < len(sorted); i++ {
		ev := sorted[i]
		var typeStyle lipgloss.Style
		switch ev.Type {
		case "Warning":
			typeStyle = theme.Warning
		case "Normal":
			typeStyle = lipgloss.NewStyle().Foreground(theme.Green)
		default:
			typeStyle = theme.Dim
		}
		message := sanitizeTerminalText(ev.Message)
		maxMsg := max(dashboardEventMessageMinimumWidth, innerW-dashboardEventMessageReservedWidth)
		message = truncateRunes(message, maxMsg, eventMessageSuffix)
		line := fmt.Sprintf("  %s %-12s %s",
			typeStyle.Render(fmt.Sprintf("%-8s", sanitizeTerminalText(ev.Type))),
			sanitizeTerminalText(ev.Reason),
			message,
		)
		if ev.Count > 1 {
			line += theme.Dim.Render(fmt.Sprintf(" (x%d)", ev.Count))
		}
		lines = append(lines, line)
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(theme.BorderColor).
		Padding(0, 1).Width(innerW + dashboardPanelOuterChrome).Render(strings.Join(lines, "\n"))
}

func (m DashboardModel) renderAIHealth(innerW int) string {
	header := theme.AIPrompt.Render("AI ") + theme.Accent.Render("CLUSTER HEALTH")

	var content string
	if m.aiHealthLoading {
		content = m.spinner.View() + " " + theme.Dim.Render("Analyzing cluster health...")
	} else if m.aiHealthErr != nil {
		content = theme.Error.Render(aiErr(m.aiHealthErr))
	} else if m.aiHealthSummary != "" {
		content = lipgloss.NewStyle().Foreground(theme.LightText).Render(m.aiHealthSummary)
	} else {
		content = theme.Dim.Render("No analysis available yet.")
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(theme.ElectricPurp).
		Padding(0, 1).Width(innerW + dashboardPanelOuterChrome).Render(header + "\n" + content)
}

func (DashboardModel) renderHelpLine(w int) string {
	keys := []struct{ key, desc string }{
		{"enter", "describe"}, {"l", "logs"}, {"a", "AI health"},
		{"r", "refresh"}, {"n", "namespace"}, {"k", "context"}, {"tab", "AI"},
	}
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, theme.HelpKey.Render(k.key)+theme.HelpDesc.Render(":"+k.desc))
	}
	helpText := strings.Join(parts, theme.Dim.Render(" │ "))
	return lipgloss.NewStyle().Background(theme.DarkerBg).Foreground(theme.NeonCyan).Padding(0, 1).Width(w).Render(helpText)
}
