package model

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/HediAbed/opsmate/internal/service"
	"github.com/HediAbed/opsmate/internal/theme"
)

const (
	logRefreshInterval           = 3 * time.Second
	defaultLogTailLines          = 200
	minimumLogTailLines          = 50
	maximumLogTailLines          = 1000
	logTailLineStep              = 100
	inspectContextLines          = 5
	estimatedLogLineBytes        = 100
	logsPanelBorderChrome        = 2
	logsEmptyStateVerticalChrome = 4
	podPopupDesiredWidth         = 50
	containerPopupDesiredWidth   = 40
	logsPopupHorizontalChrome    = 4
	logsPopupItemChrome          = 4
	logsPopupItemTopOffset       = 3
	podPopupListChrome           = 8
	podPopupMinimumVisibleItems  = 5
)

type tickMsg struct{}

func doTick() tea.Cmd {
	return tea.Tick(logRefreshInterval, newLogTickMessage)
}

func newLogTickMessage(time.Time) tea.Msg {
	return tickMsg{}
}

// LogsModel displays, filters, and inspects logs for one pod and container.
type LogsModel struct {
	width     int
	height    int
	namespace string

	pods                 []service.Pod
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

	inspectMode      bool
	lineCursor       int
	aiExplanation    string
	aiExplainLoading bool
	aiExplainErr     error
	explainRequestID uint64

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
	pod       resourceIdentity
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
	pod       resourceIdentity
	payload   tea.Msg
}

type logExplainResultMsg struct {
	requestID uint64
	pod       resourceIdentity
	container string
	line      string
	payload   tea.Msg
}

type renderedLogLine struct {
	severity lineSeverity
	rendered string
}

const maxColorizeCacheSize = 4000

// NewLogsModel creates a log viewer for namespace.
func NewLogsModel(namespace string) LogsModel {
	logView := newViewport(initialScreenWidth, initialViewportHeight)
	logView.SoftWrap = true

	fi := newTextInput(textInputOpts{
		Placeholder: "type to filter...",
		CharLimit:   filterInputCharacterLimit,
		Width:       filterInputInitialWidth,
		PromptStyle: lipgloss.NewStyle().Foreground(theme.NeonCyan).Bold(true),
		TextStyle:   lipgloss.NewStyle().Foreground(theme.White),
	})

	sp := spinner.New(
		spinner.WithSpinner(spinner.Dot),
		spinner.WithStyle(theme.SpinnerStyle),
	)

	return LogsModel{
		namespace:   namespace,
		logView:     logView,
		filterInput: fi,
		spinner:     sp,
		autoScroll:  true,
		tailLines:   defaultLogTailLines,
	}
}

func (m LogsModel) Init() tea.Cmd {
	return m.spinner.Tick
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
	case ClearStatusMsg:
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
	case service.PodsMsg:
		return m, m.applyLogPods(msg), true
	case service.LogsMsg:
		return m, m.applyLogs(msg), true
	case service.LogExplainMsg:
		m.applyLogExplanation(msg)
		return m, nil, true
	case service.ContainersMsg:
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

func (m *LogsModel) applyLogPods(msg service.PodsMsg) tea.Cmd {
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

func (m *LogsModel) applyLogs(msg service.LogsMsg) tea.Cmd {
	m.loading = false
	if msg.Err != nil {
		m.err = msg.Err
		return doTick()
	}
	m.err = nil
	m.resetExplanation()
	m.allLines = sanitizeTerminalLines(msg.Lines)
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

func (m *LogsModel) applyLogExplanation(msg service.LogExplainMsg) {
	m.aiExplainLoading = false
	if msg.Err != nil {
		m.aiExplainErr = msg.Err
		m.aiExplanation = ""
		return
	}
	m.aiExplainErr = nil
	m.aiExplanation = sanitizeTerminalText(msg.Explanation)
}

func (m *LogsModel) applyContainers(msg service.ContainersMsg) tea.Cmd {
	if msg.Err != nil {
		m.statusMsg = theme.Error.Render("Container list error: " + sanitizeTerminalLine(msg.Err.Error()))
		return clearStatusAfter(logRefreshInterval)
	}
	m.containers = msg.Containers
	switch len(m.containers) {
	case 0:
		return nil
	case 1:
		m.statusMsg = theme.Dim.Render("Only one container: " + m.containers[0])
		return clearStatusAfter(logRefreshInterval)
	default:
		m.showContainerPopup = true
		m.containerCursor = m.findContainerIndex(m.selectedContainer)
		return nil
	}
}

func (m LogsModel) findContainerIndex(container string) int {
	for index, candidate := range m.containers {
		if candidate == container {
			return index
		}
	}
	return 0
}

func (m *LogsModel) handleLogRefreshTick() tea.Cmd {
	if !m.active || m.paused || m.selectedPod == "" {
		return nil
	}
	m.loading = true
	return m.fetchSelectedLogs()
}

func (m *LogsModel) handleLogSpinnerTick(msg spinner.TickMsg) tea.Cmd {
	if !m.loading && !m.aiExplainLoading {
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

func (m LogsModel) handleLogKey(msg tea.KeyPressMsg) (LogsModel, tea.Cmd) {
	switch {
	case m.filterInput.Focused():
		return m.handleLogFilterKey(msg)
	case m.showContainerPopup:
		return m.handleContainerPopupKey(msg)
	case m.showPodPopup:
		return m.handlePopupKey(msg)
	case m.inspectMode:
		return m.handleInspectKey(msg)
	default:
		return m.handleGlobalLogKey(msg)
	}
}

func (m LogsModel) handleLogFilterKey(msg tea.KeyPressMsg) (LogsModel, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.filter = m.filterInput.Value()
		m.applyFilter()
		m.logView.SetContent(m.colorizeLines(m.filteredLines))
		if m.autoScroll {
			m.logView.GotoBottom()
		}
		m.filterInput.Blur()
		return m, nil
	case "esc":
		m.filterInput.Blur()
		return m, nil
	default:
		var command tea.Cmd
		m.filterInput, command = m.filterInput.Update(msg)
		return m, command
	}
}

func (m LogsModel) handleGlobalLogKey(msg tea.KeyPressMsg) (LogsModel, tea.Cmd) {
	key := msg.String()
	switch key {
	case "i", "p", "f", "/", "space":
		return m.handleLogModeKey(key)
	case "o", "r":
		return m.handleLogFetchKey(key)
	case "g", "G", "+", "-":
		m.handleLogNavigationKey(key)
	case "c":
		return m.copyVisibleLogs()
	case "C":
		return m.copyAllLogs()
	case "esc", "escape":
		return m, func() tea.Msg { return GoBackMsg{} }
	default:
		return m.forwardLogViewportKey(msg)
	}
	return m, nil
}

func (m LogsModel) handleLogModeKey(key string) (LogsModel, tea.Cmd) {
	switch key {
	case "i":
		m.startLogInspection()
	case "p":
		m.showPodPopup = true
		m.podCursor = m.findPodIndex(m.selectedPod)
	case "f", "/":
		return m, m.filterInput.Focus()
	case "space":
		m.paused = !m.paused
		if !m.paused && m.selectedPod != "" {
			return m, doTick()
		}
	}
	return m, nil
}

func (m LogsModel) handleLogFetchKey(key string) (LogsModel, tea.Cmd) {
	if m.selectedPod == "" {
		return m, nil
	}
	if key == "o" {
		return m, m.fetchContainers()
	}
	m.loading = true
	return m, m.fetchSelectedLogs()
}

func (m *LogsModel) handleLogNavigationKey(key string) {
	switch key {
	case "g":
		m.logView.GotoTop()
		m.autoScroll = false
	case "G":
		m.logView.GotoBottom()
		m.autoScroll = true
	case "+":
		m.tailLines = min(maximumLogTailLines, m.tailLines+logTailLineStep)
	case "-":
		m.tailLines = max(minimumLogTailLines, m.tailLines-logTailLineStep)
	}
}

func (m *LogsModel) startLogInspection() {
	if len(m.filteredLines) == 0 {
		return
	}
	m.resetExplanation()
	m.inspectMode = true
	m.paused = true
	m.lineCursor = min(m.logView.YOffset()+m.logView.Height()/2, len(m.filteredLines)-1)
	m.rebuildInspectView()
}

func (m LogsModel) copyVisibleLogs() (LogsModel, tea.Cmd) {
	content := strings.Join(m.filteredLines, "\n")
	status, command := copyToClipboard(content, fmt.Sprintf("%d lines", len(m.filteredLines)))
	m.statusMsg = status
	return m, command
}

func (m LogsModel) copyAllLogs() (LogsModel, tea.Cmd) {
	content := strings.Join(m.allLines, "\n")
	status, command := copyToClipboard(content, fmt.Sprintf("%d lines", len(m.allLines)))
	m.statusMsg = status
	return m, command
}

func (m LogsModel) forwardLogViewportKey(msg tea.KeyPressMsg) (LogsModel, tea.Cmd) {
	previousOffset := m.logView.YOffset()
	var command tea.Cmd
	m.logView, command = m.logView.Update(msg)
	if m.logView.YOffset() < previousOffset {
		m.autoScroll = false
	} else if m.logView.AtBottom() {
		m.autoScroll = true
	}
	return m, command
}

func (m LogsModel) acceptsPodListResult(msg logPodsResultMsg) bool {
	return msg.requestID == m.podListRequestID && msg.namespace == m.namespace
}

func (m LogsModel) View() string {
	if m.width == 0 {
		return ""
	}
	titleBar := m.renderTitleBar()
	helpLine := m.renderHelpBar()
	filterBar := m.renderFilterBar()
	contentHeight := m.logContentHeight()
	sections := []string{titleBar, m.renderLogMainContent(contentHeight)}
	for _, optionalSection := range []string{
		m.renderOptionalExplainPanel(),
		m.renderLogError(),
		m.statusMsg,
		filterBar,
	} {
		if optionalSection != "" {
			sections = append(sections, optionalSection)
		}
	}
	sections = append(sections, helpLine)
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m LogsModel) renderLogMainContent(contentHeight int) string {
	switch {
	case m.showContainerPopup:
		return m.renderContainerPopupOverlay(m.width, contentHeight)
	case m.showPodPopup:
		return m.renderPodPopupOverlay(m.width, contentHeight)
	default:
		return m.renderLogPanel(contentHeight)
	}
}

func (m LogsModel) renderLogPanel(contentHeight int) string {
	content := m.logView.View()
	if m.selectedPod == "" && !m.loading {
		emptyMessage := theme.Dim.Render("No pod selected") + "\n\n" +
			theme.HelpKey.Render("[p]") + theme.HelpDesc.Render(" to select a pod")
		content = lipgloss.Place(
			m.logView.Width(),
			max(1, contentHeight-logsEmptyStateVerticalChrome),
			lipgloss.Center,
			lipgloss.Center,
			emptyMessage,
		)
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.NeonCyan).
		Padding(0, 1).
		Width(m.width - logsPanelBorderChrome).
		Height(max(1, contentHeight-logsPanelBorderChrome)).
		Render(content)
}

func (m LogsModel) renderOptionalExplainPanel() string {
	if !m.inspectMode {
		return ""
	}
	return m.renderExplainPanel()
}

func (m LogsModel) renderLogError() string {
	if m.err == nil {
		return ""
	}
	message := sanitizeTerminalLine(service.SanitizeKubectlStderr(m.err.Error()))
	return theme.Error.Render("Error: " + message + " — press r to retry")
}

// SetSize updates the terminal dimensions.
func (m *LogsModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.recalcLayout()
}

func (m LogsModel) AIOverlayBounds(totalHeight int) (topOffset, panelHeight, bottomOffset int) {
	topOffset = lipgloss.Height(m.renderTitleBar())
	bottomOffset = lipgloss.Height(m.renderHelpBar())
	if filter := m.renderFilterBar(); filter != "" {
		bottomOffset += lipgloss.Height(filter)
	}
	if m.statusMsg != "" {
		bottomOffset += lipgloss.Height(m.statusMsg)
	}
	if m.err != nil {
		bottomOffset++
	}
	if m.inspectMode {
		bottomOffset += lipgloss.Height(m.renderExplainPanel())
	}
	return aiOverlayBounds(totalHeight, topOffset, bottomOffset)
}

// SetNamespace changes scope and refreshes the pod list.
func (m *LogsModel) SetNamespace(ns string) {
	m.namespace = ns
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
	command := service.FetchPods(namespace)
	return func() tea.Msg {
		return logPodsResultMsg{requestID: requestID, namespace: namespace, payload: command()}
	}
}

// SelectedPod returns the name of the currently selected pod.
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
	for _, p := range m.pods {
		if p.Name == m.selectedPod {
			return p.Namespace
		}
	}
	return m.namespace
}

// SetPod selects a specific pod and fetches its logs.
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

// SetPodCmd fetches logs for the selected pod.
func (m *LogsModel) SetPodCmd() tea.Cmd {
	if m.selectedPod == "" {
		return nil
	}
	return m.fetchSelectedLogs()
}

func (m *LogsModel) selectPod(pod service.Pod) {
	m.setPodIdentity(pod.Name, pod.Namespace)
}

func (m *LogsModel) resetContainerSelection() {
	m.containers = nil
	m.selectedContainer = ""
	m.showContainerPopup = false
	m.containerCursor = 0
}

func (m LogsModel) selectedPodIdentity() resourceIdentity {
	return resourceIdentity{Kind: "pod", Namespace: m.selectedPodNS(), Name: m.selectedPod}
}

func (m *LogsModel) fetchSelectedLogs() tea.Cmd {
	m.logRequestID++
	requestID := m.logRequestID
	pod := m.selectedPodIdentity()
	container := m.selectedContainer
	command := service.FetchContainerLogs(pod.Namespace, pod.Name, container, m.tailLines)
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
	command := service.FetchContainers(pod.Namespace, pod.Name)
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
	command := service.AIExplainLogLine(line, context, pod.Name)
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
	m.aiExplanation = ""
	m.aiExplainErr = nil
	m.aiExplainLoading = false
}

func (m LogsModel) handlePopupKey(msg tea.KeyPressMsg) (LogsModel, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc", "p":
		m.showPodPopup = false
		return m, nil
	case "up", "k":
		if m.podCursor > 0 {
			m.podCursor--
		}
	case "down", "j":
		if m.podCursor < len(m.pods)-1 {
			m.podCursor++
		}
	case "enter":
		if m.podCursor < len(m.pods) {
			m.selectPod(m.pods[m.podCursor])
			m.showPodPopup = false
			m.loading = true
			m.allLines = nil
			m.filteredLines = nil
			m.logView.SetContent("")
			return m, m.fetchSelectedLogs()
		}
	}
	return m, nil
}

func (m LogsModel) handlePopupMouse(msg tea.MouseMsg) (LogsModel, tea.Cmd) {
	switch ev := msg.(type) {
	case tea.MouseClickMsg:
		if ev.Button != tea.MouseLeft {
			return m, nil
		}
		return m.handlePopupClick(ev.X, ev.Y)
	case tea.MouseWheelMsg:
		switch ev.Button {
		case tea.MouseWheelUp:
			if m.podCursor > 0 {
				m.podCursor--
			}
		case tea.MouseWheelDown:
			if m.podCursor < len(m.pods)-1 {
				m.podCursor++
			}
		}
	}
	return m, nil
}

func (m LogsModel) handlePopupClick(x, y int) (LogsModel, tea.Cmd) {
	popupWidth := logsPopupWidth(podPopupDesiredWidth, m.width)
	popupHeight := min(len(m.pods)+logsPopupItemChrome, m.height-logsPopupItemChrome)
	popupLeft := (m.width - popupWidth) / pairedSides
	popupTop := (m.height - popupHeight) / pairedSides

	inside := x >= popupLeft && x < popupLeft+popupWidth &&
		y >= popupTop && y < popupTop+popupHeight
	if !inside {
		m.showPodPopup = false
		return m, nil
	}
	rowInPopup := y - popupTop - logsPopupItemTopOffset
	if rowInPopup < 0 || rowInPopup >= len(m.pods) {
		return m, nil
	}
	m.podCursor = rowInPopup
	m.selectPod(m.pods[m.podCursor])
	m.showPodPopup = false
	m.loading = true
	m.allLines = nil
	m.filteredLines = nil
	m.logView.SetContent("")
	return m, m.fetchSelectedLogs()
}

func (m LogsModel) handleContainerPopupKey(msg tea.KeyPressMsg) (LogsModel, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc", "o":
		m.showContainerPopup = false
		return m, nil
	case "up", "k":
		if m.containerCursor > 0 {
			m.containerCursor--
		}
	case "down", "j":
		if m.containerCursor < len(m.containers)-1 {
			m.containerCursor++
		}
	case "enter":
		if m.containerCursor < len(m.containers) {
			m.selectedContainer = m.containers[m.containerCursor]
			m.resetExplanation()
			m.showContainerPopup = false
			m.loading = true
			m.allLines = nil
			m.filteredLines = nil
			m.logView.SetContent("")
			return m, m.fetchSelectedLogs()
		}
	}
	return m, nil
}

func (m LogsModel) handleInspectKey(msg tea.KeyPressMsg) (LogsModel, tea.Cmd) {
	switch msg.String() {
	case "esc", "i":
		m.inspectMode = false
		m.resetExplanation()
		m.logView.SetContent(m.colorizeLines(m.filteredLines))
		return m, nil
	case "up", "k":
		m.moveInspectCursor(-1)
	case "down", "j":
		m.moveInspectCursor(1)
	case "enter":
		return m, m.explainInspectedLine()
	case "n":
		if index, found := m.nextImportantLine(); found {
			m.jumpToInspectLine(index)
		}
	case "N":
		if index, found := m.previousImportantLine(); found {
			m.jumpToInspectLine(index)
		}
	}
	return m, nil
}

func (m *LogsModel) moveInspectCursor(offset int) {
	nextCursor := m.lineCursor + offset
	if nextCursor < 0 || nextCursor >= len(m.filteredLines) {
		return
	}
	m.lineCursor = nextCursor
	m.resetExplanation()
	m.rebuildInspectView()
	if m.lineCursor < m.logView.YOffset() {
		m.logView.SetYOffset(m.lineCursor)
	}
	if m.lineCursor >= m.logView.YOffset()+m.logView.Height() {
		m.logView.SetYOffset(m.lineCursor - m.logView.Height() + 1)
	}
}

func (m *LogsModel) explainInspectedLine() tea.Cmd {
	if m.lineCursor < 0 || m.lineCursor >= len(m.filteredLines) || m.aiExplainLoading {
		return nil
	}
	line := m.filteredLines[m.lineCursor]
	contextLines := m.getSurroundingContext(m.lineCursor, inspectContextLines)
	m.aiExplainLoading = true
	m.aiExplanation = ""
	m.aiExplainErr = nil
	return m.explainSelectedLine(line, contextLines)
}

func (m LogsModel) nextImportantLine() (int, bool) {
	for index := m.lineCursor + 1; index < len(m.filteredLines); index++ {
		if classifyLine(m.filteredLines[index]) >= sevWarn {
			return index, true
		}
	}
	return 0, false
}

func (m LogsModel) previousImportantLine() (int, bool) {
	for index := m.lineCursor - 1; index >= 0; index-- {
		if classifyLine(m.filteredLines[index]) >= sevWarn {
			return index, true
		}
	}
	return 0, false
}

func (m *LogsModel) jumpToInspectLine(index int) {
	m.lineCursor = index
	m.resetExplanation()
	m.rebuildInspectView()
	viewportTop := m.logView.YOffset()
	viewportBottom := viewportTop + m.logView.Height()
	if index < viewportTop || index >= viewportBottom {
		m.logView.SetYOffset(max(0, index-m.logView.Height()/pairedSides))
	}
}

func (m *LogsModel) rebuildInspectView() {
	if len(m.filteredLines) == 0 {
		return
	}
	var b strings.Builder
	b.Grow(len(m.filteredLines) * estimatedLogLineBytes)
	for i, line := range m.filteredLines {
		if i == m.lineCursor {
			b.WriteString(theme.LogInspectCursor.Render("▶ " + line))
		} else {
			b.WriteString(m.renderedLine(line).rendered)
		}
		if i < len(m.filteredLines)-1 {
			b.WriteByte('\n')
		}
	}
	m.logView.SetContent(b.String())
}

func (m LogsModel) getSurroundingContext(idx, n int) string {
	start := idx - n
	if start < 0 {
		start = 0
	}
	end := idx + n + 1
	if end > len(m.filteredLines) {
		end = len(m.filteredLines)
	}
	return strings.Join(m.filteredLines[start:end], "\n")
}

// HasInputFocus reports whether the log viewer owns keyboard input.
func (m LogsModel) HasInputFocus() bool {
	return m.filterInput.Focused() || m.showPodPopup || m.showContainerPopup || m.inspectMode
}

func (m *LogsModel) recalcLayout() {
	m.syncLogViewport()
}

func (m *LogsModel) syncLogViewport() {
	contentWidth := max(1, theme.BoxContentWidth(m.width-logsPanelBorderChrome))
	viewportHeight := max(1, m.logContentHeight()-logsPanelBorderChrome)
	m.logView.SetWidth(contentWidth)
	m.logView.SetHeight(viewportHeight)
}

func (m LogsModel) logContentHeight() int {
	usedLines := lipgloss.Height(m.renderTitleBar()) + lipgloss.Height(m.renderHelpBar())
	if filterBar := m.renderFilterBar(); filterBar != "" {
		usedLines += lipgloss.Height(filterBar)
	}
	if m.err != nil {
		usedLines++
	}
	if m.statusMsg != "" {
		usedLines += lipgloss.Height(m.statusMsg)
	}
	if m.inspectMode {
		usedLines += lipgloss.Height(m.renderExplainPanel())
	}
	return max(1, m.height-usedLines)
}

func (m *LogsModel) applyFilter() {
	if m.filter == "" {
		m.filteredLines = make([]string, len(m.allLines))
		copy(m.filteredLines, m.allLines)
		return
	}

	lowerFilter := strings.ToLower(m.filter)
	m.filteredLines = m.filteredLines[:0]
	for _, line := range m.allLines {
		if strings.Contains(strings.ToLower(line), lowerFilter) {
			m.filteredLines = append(m.filteredLines, line)
		}
	}
}

type lineSeverity int

const (
	sevNone lineSeverity = iota
	sevDebug
	sevStack
	sevWarn
	sevError
	sevCritical
)

func classifyLine(line string) lineSeverity {
	lower := strings.ToLower(line)
	switch {
	case containsEither(lower, "fatal", "panic"):
		return sevCritical
	case containsEither(lower, "error", "exception"):
		return sevError
	case strings.Contains(lower, "warn"):
		return sevWarn
	case isStackTraceLine(lower):
		return sevStack
	case containsEither(lower, "debug", "trace"):
		return sevDebug
	default:
		return sevNone
	}
}

func containsEither(value, first, second string) bool {
	return strings.Contains(value, first) || strings.Contains(value, second)
}

func isStackTraceLine(lowercaseLine string) bool {
	return strings.HasPrefix(strings.TrimSpace(lowercaseLine), "at ") ||
		strings.Contains(lowercaseLine, "goroutine ") ||
		strings.Contains(lowercaseLine, "stacktrace") ||
		strings.Contains(lowercaseLine, "traceback")
}

func (m *LogsModel) colorizeLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	b.Grow(len(lines) * estimatedLogLineBytes)
	for i, line := range lines {
		entry := m.renderedLine(line)
		b.WriteString(entry.rendered)
		if i < len(lines)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (m *LogsModel) renderedLine(line string) renderedLogLine {
	if m.colorizeCache == nil {
		m.colorizeCache = make(map[string]renderedLogLine)
	}
	if cached, ok := m.colorizeCache[line]; ok {
		return cached
	}
	if len(m.colorizeCache) >= maxColorizeCacheSize {
		m.colorizeCache = make(map[string]renderedLogLine)
	}
	sev := classifyLine(line)
	entry := renderedLogLine{severity: sev, rendered: applyLogSeverityStyle(line, sev)}
	m.colorizeCache[line] = entry
	return entry
}

func applyLogSeverityStyle(line string, sev lineSeverity) string {
	switch sev {
	case sevCritical:
		return theme.LogGutterError.Render("▌") + theme.LogCritical.Render(line)
	case sevError:
		return theme.LogGutterError.Render("▌") + theme.LogError.Render(line)
	case sevWarn:
		return theme.LogGutterWarn.Render("▌") + theme.LogWarn.Render(line)
	case sevStack:
		return "  " + theme.LogStack.Render(line)
	case sevDebug:
		return "  " + theme.LogDebug.Render(line)
	case sevNone:
		return "  " + line
	}
	return "  " + line
}

func (m *LogsModel) resetColorizeCache() {
	m.colorizeCache = nil
}

func (m LogsModel) findPodIndex(name string) int {
	namespace := m.selectedPodNS()
	for i, p := range m.pods {
		if p.Name == name && namespacesMatch(namespace, p.Namespace) {
			return i
		}
	}
	return 0
}

func (m LogsModel) renderTitleBar() string {
	titleText := theme.Title.Render("LOG VIEWER")
	podLabel := m.renderLogPodLabel()
	indicators := m.renderLogTitleIndicators()
	titleLeft := lipgloss.JoinHorizontal(lipgloss.Center, titleText, "  ", podLabel)
	titleRight := strings.Join(indicators, "  ")
	titleBarStyle := lipgloss.NewStyle().
		Width(m.width).
		MaxWidth(m.width).
		Background(theme.DarkerBg).
		Padding(0, 1)
	gap := max(1, m.width-lipgloss.Width(titleLeft)-lipgloss.Width(titleRight)-logsPanelBorderChrome)
	return titleBarStyle.Render(titleLeft + strings.Repeat(" ", gap) + titleRight)
}

func (m LogsModel) renderLogPodLabel() string {
	if m.selectedPod != "" {
		podLabel := theme.Subtitle.Render(m.selectedPod)
		if m.selectedContainer != "" {
			podLabel += theme.Dim.Render("/") + theme.Accent.Render(m.selectedContainer)
		}
		podLabel += "  " + theme.HelpKey.Render("[p]") + theme.HelpDesc.Render(" pod") +
			"  " + theme.HelpKey.Render("[o]") + theme.HelpDesc.Render(" container")
		return podLabel
	}
	return theme.Warning.Render("No pod selected") + "  " +
		theme.HelpKey.Render("[p]") + theme.HelpDesc.Render(" to choose")
}

func (m LogsModel) renderLogTitleIndicators() []string {
	indicators := []string{m.renderLogModeIndicator()}
	if issueCount := m.logIssueCount(); issueCount > 0 {
		indicators = append(indicators, theme.Warning.Render(fmt.Sprintf("⚠ %d issues", issueCount)))
	}
	if m.loading {
		indicators = append(indicators, m.spinner.View()+" "+theme.Dim.Render("fetching..."))
	}
	lineCount := theme.Dim.Render(fmt.Sprintf("[%d lines | tail %d]", len(m.filteredLines), m.tailLines))
	indicators = append(indicators, lineCount)
	if percentage := m.scrollPctLabel(); percentage != "" {
		indicators = append(indicators, percentage)
	}
	return indicators
}

func (m LogsModel) renderLogModeIndicator() string {
	if m.inspectMode {
		return theme.LogGutterError.Render(" INSPECT ")
	}
	if m.paused {
		return theme.Warning.Render(" PAUSED ")
	}
	if m.autoScroll {
		return theme.IndicatorOn.Render(" FOLLOW ")
	}
	return theme.IndicatorOff.Render(" SCROLL ")
}

func (m LogsModel) logIssueCount() int {
	issueCount := 0
	for _, line := range m.filteredLines {
		if classifyLine(line) >= sevWarn {
			issueCount++
		}
	}
	return issueCount
}

func (m LogsModel) scrollPctLabel() string {
	if m.autoScroll && m.logView.AtBottom() {
		return ""
	}
	return theme.Dim.Render(fmt.Sprintf("%d%%", viewportScrollPct(m.logView)))
}

func (m LogsModel) renderHelpBar() string {
	var helpParts []string
	if m.inspectMode {
		helpParts = []string{
			theme.HelpKey.Render("j/k") + theme.HelpDesc.Render(": move"),
			theme.HelpKey.Render("enter") + theme.HelpDesc.Render(": AI explain"),
			theme.HelpKey.Render("n/N") + theme.HelpDesc.Render(": next/prev issue"),
			theme.HelpKey.Render("esc") + theme.HelpDesc.Render(": exit inspect"),
		}
	} else {
		helpParts = []string{
			theme.HelpKey.Render("p") + theme.HelpDesc.Render(": pods"),
			theme.HelpKey.Render("i") + theme.HelpDesc.Render(": inspect"),
			theme.HelpKey.Render("/") + theme.HelpDesc.Render(": filter"),
			theme.HelpKey.Render("space") + theme.HelpDesc.Render(": pause"),
			theme.HelpKey.Render("r") + theme.HelpDesc.Render(": refresh"),
			theme.HelpKey.Render("g/G") + theme.HelpDesc.Render(": top/bottom"),
			theme.HelpKey.Render("+/-") + theme.HelpDesc.Render(": tail lines"),
			theme.HelpKey.Render("c/C") + theme.HelpDesc.Render(": copy"),
		}
	}
	return lipgloss.NewStyle().
		Width(m.width).
		MaxWidth(m.width).
		Background(theme.DarkerBg).
		Foreground(theme.DimText).
		Padding(0, 1).
		Render(strings.Join(helpParts, "  |  "))
}

func (m LogsModel) renderFilterBar() string {
	if m.filterInput.Focused() {
		filterPrompt := theme.Accent.Render("Filter: ")
		return lipgloss.NewStyle().
			Width(m.width).
			MaxWidth(m.width).
			Background(theme.DarkerBg).
			Padding(0, 1).
			Render(filterPrompt + m.filterInput.View())
	}
	if m.filter != "" {
		filterPrompt := theme.Accent.Render("Filter: ")
		filterValue := theme.Subtitle.Render(m.filter)
		matchInfo := theme.Dim.Render(fmt.Sprintf(" (%d/%d)", len(m.filteredLines), len(m.allLines)))
		return lipgloss.NewStyle().
			Width(m.width).
			Background(theme.DarkerBg).
			Padding(0, 1).
			Render(filterPrompt + filterValue + matchInfo)
	}
	return ""
}

func (m LogsModel) renderExplainPanel() string {
	var content string
	if m.aiExplainLoading {
		content = m.spinner.View() + " " + theme.Dim.Render("AI is analyzing this line...")
	} else if m.aiExplainErr != nil {
		content = theme.Error.Render("AI Error: " + sanitizeTerminalLine(m.aiExplainErr.Error()))
	} else if m.aiExplanation != "" {
		content = theme.LogGutterError.Render("AI ") + theme.Subtitle.Render("Explanation") + "\n" +
			lipgloss.NewStyle().Foreground(theme.LightText).Render(m.aiExplanation)
	} else {
		content = theme.Dim.Render("Press ") + theme.HelpKey.Render("enter") + theme.Dim.Render(" on a line to get AI explanation")
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.ElectricPurp).
		Width(m.width-logsPanelBorderChrome).
		Padding(0, 1).
		Render(content)
}

func (m LogsModel) renderContainerPopupOverlay(width, height int) string {
	title := theme.Title.Render("SELECT CONTAINER")

	if len(m.containers) == 0 {
		box := lipgloss.NewStyle().Border(lipgloss.ThickBorder()).BorderForeground(theme.ElectricPurp).Padding(0, 1).
			Width(logsPopupWidth(containerPopupDesiredWidth, width)).
			Render(title + "\n\n" + theme.Dim.Render("No containers found."))
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
	}

	var items []string
	for i, c := range m.containers {
		if i == m.containerCursor {
			items = append(items, theme.TableSelected.Render(fmt.Sprintf(" > %-30s ", c)))
		} else if c == m.selectedContainer {
			items = append(items, theme.Accent.Render(fmt.Sprintf("   %-30s (current)", c)))
		} else {
			items = append(items, theme.Dim.Render(fmt.Sprintf("   %-30s", c)))
		}
	}

	content := title + "\n\n" + strings.Join(items, "\n") + "\n\n" +
		theme.HelpKey.Render("enter") + theme.HelpDesc.Render(": select  ") +
		theme.HelpKey.Render("esc") + theme.HelpDesc.Render(": cancel")

	popupWidth := logsPopupWidth(containerPopupDesiredWidth, width)
	box := lipgloss.NewStyle().Border(lipgloss.ThickBorder()).BorderForeground(theme.ElectricPurp).Padding(0, 1).
		Width(popupWidth).
		Render(content)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

func (m LogsModel) renderPodPopupOverlay(width, height int) string {
	title := theme.Title.Render("SELECT POD")

	if len(m.pods) == 0 {
		box := lipgloss.NewStyle().Border(lipgloss.ThickBorder()).BorderForeground(theme.NeonCyan).Padding(0, 1).
			Width(logsPopupWidth(podPopupDesiredWidth, width)).
			Render(title + "\n\n" + theme.Dim.Render("No pods found."))
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
	}

	maxVisible := height - podPopupListChrome
	if maxVisible < podPopupMinimumVisibleItems {
		maxVisible = podPopupMinimumVisibleItems
	}

	start := 0
	if m.podCursor >= maxVisible {
		start = m.podCursor - maxVisible + 1
	}
	end := start + maxVisible
	if end > len(m.pods) {
		end = len(m.pods)
	}

	var items []string
	for i := start; i < end; i++ {
		p := m.pods[i]
		name := displayResourceName(p.Namespace, p.Name, m.namespace == "")
		statusStr := theme.PodStatusStyle(p.Status).Render(p.Status)
		if i == m.podCursor {
			items = append(items, theme.TableSelected.Render(fmt.Sprintf(" > %-36s ", name))+" "+statusStr)
		} else if p.Name == m.selectedPod && namespacesMatch(m.selectedPodNS(), p.Namespace) {
			items = append(items, theme.Accent.Render(fmt.Sprintf("   %-36s (current)", name))+" "+statusStr)
		} else {
			items = append(items, theme.Dim.Render(fmt.Sprintf("   %-36s", name))+" "+statusStr)
		}
	}

	content := title + "\n\n" + strings.Join(items, "\n") + "\n\n" +
		theme.HelpKey.Render("enter") + theme.HelpDesc.Render(": select  ") +
		theme.HelpKey.Render("esc") + theme.HelpDesc.Render(": cancel")

	popupWidth := logsPopupWidth(podPopupDesiredWidth, width)
	box := lipgloss.NewStyle().Border(lipgloss.ThickBorder()).BorderForeground(theme.NeonCyan).Padding(0, 1).
		Width(popupWidth).
		Render(content)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

func logsPopupWidth(desired, terminalWidth int) int {
	return min(desired, max(1, terminalWidth-logsPopupHorizontalChrome))
}
