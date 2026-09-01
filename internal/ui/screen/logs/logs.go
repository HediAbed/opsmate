package logs

import (
	"slices"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
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

const (
	logRefreshInterval           = 3 * time.Second
	defaultLogTailLines          = 200
	minimumLogTailLines          = 50
	maximumLogTailLines          = 1000
	logTailLineStep              = 100
	inspectContextLines          = 5
	estimatedLogLineBytes        = 100
	logsPanelGutter              = 2
	logsEmptyStateVerticalChrome = 4
	podPopupDesiredWidth         = 50
	containerPopupDesiredWidth   = 40
	logsPopupHorizontalChrome    = 4
	logsPopupItemChrome          = 4
	logsPopupItemTopOffset       = 3
	logsCursorCenterDivisor      = 2
	podPopupListChrome           = 8
	podPopupMinimumVisibleItems  = 5
	logsFilterCharacterLimit     = 128
	logsFilterInitialWidth       = 40
	logsInitialViewportWidth     = 80
	logsInitialViewportHeight    = 20
	centerDivisor                = 2
	podKind                      = "pod"
)

type tickMsg struct{}

func doTick() tea.Cmd {
	return tea.Tick(logRefreshInterval, newLogTickMessage)
}

func newLogTickMessage(time.Time) tea.Msg {
	return tickMsg{}
}

type LogsModel struct {
	width      int
	height     int
	namespace  string
	cluster    clusterui.Commands
	operations clusterui.Operations
	analysis   analysis.Service

	pods                 []cluster.Pod
	podCursor            int
	showPodPopup         bool
	selectedPod          string
	selectedPodNamespace string

	logView     viewport.Model
	filterInput textinput.Model
	spinner     spinner.Model

	allLines      []string
	filteredLines []string
	filter        string

	loading    bool
	paused     bool
	autoScroll bool
	active     bool

	err       error
	statusMsg string
	tailLines int

	inspectMode            bool
	lineCursor             int
	lineExplanation        string
	lineExplanationLoading bool
	lineExplanationErr     error
	explainRequestID       uint64

	containers         []string
	selectedContainer  string
	showContainerPopup bool
	containerCursor    int

	colorizeCache      map[string]renderedLogLine
	podListRequestID   uint64
	logRequestID       uint64
	containerRequestID uint64
}

type logsResultMsg struct {
	requestID uint64
	pod       podIdentity
	container string
	payload   tea.Msg
}

type logPodsResultMsg struct {
	requestID uint64
	namespace string
	payload   tea.Msg
}

type containersResultMsg struct {
	requestID uint64
	pod       podIdentity
	payload   tea.Msg
}

type logExplainResultMsg struct {
	requestID uint64
	pod       podIdentity
	container string
	line      string
	payload   tea.Msg
}

type renderedLogLine struct {
	severity lineSeverity
	rendered string
}

type podIdentity struct {
	Kind      string
	Namespace string
	Name      string
}

const maxColorizeCacheSize = 4000

func NewLogsModel(namespace string, commands clusterui.Commands, operations clusterui.Operations) LogsModel {
	return NewWithAnalysis(namespace, commands, operations, analysis.NewService(nil))
}

func NewWithAnalysis(
	namespace string,
	commands clusterui.Commands,
	operations clusterui.Operations,
	analysisService analysis.Service,
) LogsModel {
	logView := component.NewViewport(logsInitialViewportWidth, logsInitialViewportHeight)
	logView.SoftWrap = true

	filterInput := component.NewTextInput(component.TextInputOptions{
		Placeholder: "type to filter...",
		CharLimit:   logsFilterCharacterLimit,
		Width:       logsFilterInitialWidth,
		PromptStyle: lipgloss.NewStyle().Foreground(theme.NeonCyan).Bold(true),
		TextStyle:   lipgloss.NewStyle().Foreground(theme.White),
	})

	loadingSpinner := spinner.New(
		spinner.WithSpinner(spinner.Dot),
		spinner.WithStyle(theme.SpinnerStyle),
	)

	return LogsModel{
		namespace:   namespace,
		cluster:     commands,
		operations:  operations,
		analysis:    analysisService,
		logView:     logView,
		filterInput: filterInput,
		spinner:     loadingSpinner,
		autoScroll:  true,
		tailLines:   defaultLogTailLines,
	}
}

func (m LogsModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (LogsModel) Accepts(msg tea.Msg) bool {
	switch msg.(type) {
	case logPodsResultMsg, logsResultMsg, containersResultMsg, logExplainResultMsg:
		return true
	default:
		return false
	}
}

func (m LogsModel) ContextChangedBy(msg tea.Msg) bool {
	switch message := msg.(type) {
	case logPodsResultMsg:
		return m.acceptsPodListResult(message)
	case logsResultMsg:
		return m.acceptsLogResult(message)
	case containersResultMsg:
		return m.acceptsContainersResult(message)
	default:
		return false
	}
}

func (m LogsModel) Size() (int, int) {
	return m.width, m.height
}

func (m LogsModel) Active() bool {
	return m.active
}

func (m LogsModel) Loading() bool {
	return m.loading
}

func (m *LogsModel) Activate() tea.Cmd {
	m.active = true
	cmds := []tea.Cmd{m.fetchPods()}
	if m.selectedPod != "" {
		m.loading = true
		cmds = append(cmds, m.fetchSelectedLogs())
	}
	return tea.Batch(cmds...)
}

func (m *LogsModel) Deactivate() {
	m.active = false
	m.podListRequestID++
	m.logRequestID++
	m.containerRequestID++
	m.loading = false
}

func (m LogsModel) Update(msg tea.Msg) (next LogsModel, command tea.Cmd) {
	defer func() {
		next.syncLogViewport()
	}()
	if updated, cmd, handled := m.updateLogEnvelopeMessage(msg); handled {
		return updated, cmd
	}
	if updated, cmd, handled := m.updateLogDataMessage(msg); handled {
		return updated, cmd
	}
	return m.updateLogInputMessage(msg)
}

func (m LogsModel) updateLogEnvelopeMessage(msg tea.Msg) (LogsModel, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case logPodsResultMsg:
		updated, command := m.handleLogPodsResult(msg)
		return updated, command, true
	case logsResultMsg:
		updated, command := m.handleLogsResult(msg)
		return updated, command, true
	case containersResultMsg:
		updated, command := m.handleContainersResult(msg)
		return updated, command, true
	case logExplainResultMsg:
		updated, command := m.handleLogExplanationResult(msg)
		return updated, command, true
	case screen.ClearStatusMsg:
		m.statusMsg = ""
		return m, nil, true
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.recalcLayout()
		return m, nil, true
	default:
		return m, nil, false
	}
}

func (m LogsModel) updateLogDataMessage(msg tea.Msg) (LogsModel, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case cluster.PodsMsg:
		return m, m.applyLogPods(msg), true
	case cluster.LogsMsg:
		return m, m.applyLogs(msg), true
	case analysis.LogExplanationMsg:
		m.applyLogExplanation(msg)
		return m, nil, true
	case cluster.ContainersMsg:
		return m, m.applyContainers(msg), true
	case tickMsg:
		return m, m.handleLogRefreshTick(), true
	case spinner.TickMsg:
		return m, m.handleLogSpinnerTick(msg), true
	default:
		return m, nil, false
	}
}

func (m LogsModel) updateLogInputMessage(msg tea.Msg) (LogsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.MouseClickMsg:
		if m.showPodPopup {
			return m.handlePopupMouse(msg)
		}
		return m, nil
	case tea.MouseWheelMsg:
		return m.handleLogMouseWheel(msg)
	case tea.KeyPressMsg:
		return m.handleLogKey(msg)
	default:
		return m, nil
	}
}

func (m LogsModel) handleLogPodsResult(msg logPodsResultMsg) (LogsModel, tea.Cmd) {
	if !m.acceptsPodListResult(msg) {
		return m, nil
	}
	return m.Update(msg.payload)
}

func (m LogsModel) handleLogsResult(msg logsResultMsg) (LogsModel, tea.Cmd) {
	if !m.acceptsLogResult(msg) {
		return m, nil
	}
	return m.Update(msg.payload)
}

func (m LogsModel) handleContainersResult(msg containersResultMsg) (LogsModel, tea.Cmd) {
	if !m.acceptsContainersResult(msg) {
		return m, nil
	}
	return m.Update(msg.payload)
}

func (m LogsModel) handleLogExplanationResult(msg logExplainResultMsg) (LogsModel, tea.Cmd) {
	if !m.acceptsExplanation(msg) {
		return m, nil
	}
	return m.Update(msg.payload)
}

func (m *LogsModel) applyLogPods(msg cluster.PodsMsg) tea.Cmd {
	if msg.Err != nil {
		m.err = msg.Err
		m.loading = false
		return nil
	}
	m.pods = msg.Pods
	if m.selectedPod != "" || len(m.pods) == 0 {
		return nil
	}
	m.selectPod(m.pods[0])
	m.loading = true
	return m.fetchSelectedLogs()
}

func (m *LogsModel) applyLogs(msg cluster.LogsMsg) tea.Cmd {
	m.loading = false
	if msg.Err != nil {
		m.err = msg.Err
		return doTick()
	}
	m.err = nil
	m.resetExplanation()
	m.allLines = terminal.SanitizeLines(msg.Lines)
	m.applyFilter()
	m.logView.SetContent(m.colorizeLines(m.filteredLines))
	if m.autoScroll {
		m.logView.GotoBottom()
	}
	if m.paused {
		return nil
	}
	return doTick()
}

func (m *LogsModel) applyLogExplanation(msg analysis.LogExplanationMsg) {
	m.lineExplanationLoading = false
	if msg.Err != nil {
		m.lineExplanationErr = msg.Err
		m.lineExplanation = ""
		return
	}
	m.lineExplanationErr = nil
	m.lineExplanation = terminal.SanitizeText(msg.Explanation)
}

func (m *LogsModel) applyContainers(msg cluster.ContainersMsg) tea.Cmd {
	if msg.Err != nil {
		m.statusMsg = theme.Error.Render("Container list error: " + terminal.SanitizeLine(msg.Err.Error()))
		return screen.ClearStatusAfter(logRefreshInterval)
	}
	m.containers = msg.Containers
	m.showContainerPopup = false
	m.reconcileContainerSelection()
	switch len(m.containers) {
	case 0:
		return nil
	case 1:
		m.statusMsg = theme.Dim.Render("Only one container: " + m.containers[0])
		return screen.ClearStatusAfter(logRefreshInterval)
	default:
		m.showContainerPopup = true
		return nil
	}
}

func (m *LogsModel) reconcileContainerSelection() {
	previous := m.selectedContainer
	index := slices.Index(m.containers, previous)
	if len(m.containers) == 1 {
		m.selectedContainer = m.containers[0]
		index = 0
	} else if index < 0 {
		m.selectedContainer = ""
		index = 0
	}
	m.containerCursor = index
	if m.selectedContainer != previous {
		m.resetExplanation()
	}
}

func (m *LogsModel) handleLogRefreshTick() tea.Cmd {
	if !m.active || m.paused || m.selectedPod == "" {
		return nil
	}
	m.loading = true
	return m.fetchSelectedLogs()
}

func (m *LogsModel) handleLogSpinnerTick(msg spinner.TickMsg) tea.Cmd {
	if !m.loading && !m.lineExplanationLoading {
		return nil
	}
	var command tea.Cmd
	m.spinner, command = m.spinner.Update(msg)
	return command
}

func (m LogsModel) handleLogMouseWheel(msg tea.MouseWheelMsg) (LogsModel, tea.Cmd) {
	if m.showPodPopup {
		return m.handlePopupMouse(msg)
	}
	var command tea.Cmd
	m.logView, command = m.logView.Update(msg)
	switch msg.Button {
	case tea.MouseWheelUp:
		m.autoScroll = false
	case tea.MouseWheelDown:
		m.autoScroll = m.logView.AtBottom()
	}
	return m, command
}
