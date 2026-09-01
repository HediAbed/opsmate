package dashboard

import (
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/HediAbed/opsmate/internal/analysis"
	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/terminal"
	clusterui "github.com/HediAbed/opsmate/internal/ui/cluster"
	"github.com/HediAbed/opsmate/internal/ui/component"
	"github.com/HediAbed/opsmate/internal/ui/screen"
	"github.com/HediAbed/opsmate/internal/ui/theme"
)

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

type DashboardModel struct {
	width  int
	height int

	namespace string
	cluster   clusterui.Commands
	analysis  analysis.Service

	pods        []cluster.Pod
	deployments []cluster.Deployment
	events      []cluster.Event
	metrics     []cluster.PodMetric
	podTable    table.Model
	podRows     []podIdentity
	spinner     spinner.Model
	bodyView    viewport.Model

	loading     bool
	err         error
	lastRefresh time.Time

	healthAnalysisSummary string
	healthAnalysisLoading bool
	healthAnalysisErr     error
	showHealthAnalysis    bool
	healthRequestID       uint64

	podLive        screen.LiveSupervisor[cluster.Pod]
	deploymentLive screen.LiveSupervisor[cluster.Deployment]
	eventLive      screen.LiveSupervisor[cluster.Event]

	podLiveError        error
	deploymentLiveError error
	eventLiveError      error

	active             bool
	metricsTickPending bool

	requestIDs [dashboardDataKindCount]uint64
}

func NewDashboardModel(namespace string, commands clusterui.Commands) DashboardModel {
	return newDashboardWithAnalysis(namespace, commands, analysis.NewService(nil))
}

func NewWithAnalysis(namespace string, commands clusterui.Commands, analysisService analysis.Service) DashboardModel {
	return newDashboardWithAnalysis(namespace, commands, analysisService)
}

func newDashboardWithAnalysis(namespace string, commands clusterui.Commands, analysisService analysis.Service) DashboardModel {
	loadingSpinner := spinner.New(
		spinner.WithSpinner(spinner.Dot),
		spinner.WithStyle(theme.SpinnerStyle),
	)

	podTable := table.New(
		table.WithColumns(dashPodColumns(dashboardInitialWidth)),
		table.WithRows([]table.Row{}),
		table.WithFocused(true),
		table.WithHeight(dashboardInitialTableHeight),
		table.WithWidth(dashboardInitialWidth),
	)
	podTable.SetStyles(dashTableStyles())

	return DashboardModel{
		namespace:          namespace,
		cluster:            commands,
		analysis:           analysisService,
		podTable:           podTable,
		spinner:            loadingSpinner,
		loading:            true,
		bodyView:           viewport.New(),
		metricsTickPending: true,
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
	case screen.LiveMessage:
		updated, command := m.handleSupervisedLiveMessage(msg)
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
	m.metricsTickPending = false
	if !m.active {
		return nil
	}
	m.metricsTickPending = true
	return tea.Batch(
		scheduleMetricsTick(),
		m.fetchDashboardData(dashboardMetrics, m.cluster.FetchPodMetrics(m.namespace)),
	)
}

func (m DashboardModel) updateDashboardDataMessage(msg tea.Msg) (DashboardModel, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case cluster.MetricsMsg:
		return m, m.applyDashboardMetrics(msg), true
	case analysis.DashboardHealthMsg:
		m.applyDashboardHealth(msg)
		return m, nil, true
	case spinner.TickMsg:
		return m, m.handleDashboardSpinnerTick(msg), true
	default:
		return m, nil, false
	}
}

func (m *DashboardModel) applyDashboardMetrics(msg cluster.MetricsMsg) tea.Cmd {
	if msg.Err != nil {
		m.metrics = nil
	} else {
		m.metrics = msg.PodMetrics
		m.mergeMetrics()
		m.rebuildTableRows()
	}
	if !m.showHealthAnalysis || m.healthAnalysisLoading {
		return nil
	}
	m.healthAnalysisLoading = true
	return m.fetchHealthSummary()
}

func (m *DashboardModel) applyDashboardHealth(msg analysis.DashboardHealthMsg) {
	m.healthAnalysisLoading = false
	if msg.Err != nil {
		m.healthAnalysisErr = msg.Err
		m.healthAnalysisSummary = ""
		return
	}
	m.healthAnalysisErr = nil
	m.healthAnalysisSummary = terminal.SanitizeText(msg.Summary)
}

func (m *DashboardModel) handleDashboardSpinnerTick(msg spinner.TickMsg) tea.Cmd {
	if !m.loading && !m.healthAnalysisLoading {
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
		m.podTable.MoveUp(component.TableWheelStep)
	case tea.MouseWheelDown:
		m.podTable.MoveDown(component.TableWheelStep)
	}
	return m, nil
}

func (m *DashboardModel) handleDashboardMouseClick(msg tea.MouseClickMsg) {
	if msg.Button != tea.MouseLeft {
		return
	}
	rowIndex := msg.Y + m.bodyView.YOffset() - m.podTableTopBoundary() - dashTableHeaderRows
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
		return m, m.selectedPodDrillDown(screen.Browser)
	case "l":
		return m, m.selectedPodDrillDown(screen.Logs)
	case "esc":
		m.showHealthAnalysis = false
		return m, nil
	case "j", "down", "pgdown", "k", "up", "pgup":
		if updated, command, handled := m.routeScrollToBody(msg); handled {
			return updated, command
		}
	}
	return m.forwardDashboardTableMessage(msg)
}

func (m DashboardModel) toggleDashboardHealth() (DashboardModel, tea.Cmd) {
	m.showHealthAnalysis = !m.showHealthAnalysis
	if !m.showHealthAnalysis || m.healthAnalysisSummary != "" || m.healthAnalysisLoading {
		return m, nil
	}
	m.healthAnalysisLoading = true
	return m, m.fetchHealthSummary()
}

func (m DashboardModel) selectedPodDrillDown(target screen.ID) tea.Cmd {
	pod := m.SelectedPod()
	if pod == "" {
		return nil
	}
	namespace := m.SelectedPodNS()
	return func() tea.Msg {
		return screen.DrillDownMsg{Screen: target, ResourceType: podResourceType, ResourceName: pod, ResourceNS: namespace}
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
	dashTableHeaderRows   = 2
	podPanelTopBorderRows = 1
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
	sections := m.renderDashboardSections()
	above := []string{
		sections.title,
		sections.errorBanner,
		sections.overview,
		sections.health,
		sections.alerts,
		sections.deployments,
		sections.topConsumers,
	}
	aboveHeight := 0
	for _, section := range above {
		if section != "" {
			aboveHeight += lipgloss.Height(section)
		}
	}
	return aboveHeight + podPanelTopBorderRows
}

func (m DashboardModel) innerW() int {
	width := m.width - dashboardHorizontalChrome
	if width < dashboardMinimumInnerWidth {
		width = dashboardMinimumInnerWidth
	}
	return width
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
