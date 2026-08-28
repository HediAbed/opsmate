package model

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/HediAbed/opsmate/internal/service"
	"github.com/HediAbed/opsmate/internal/theme"
)

var allResourceTypes = []string{"pods", "deployments", "services", "statefulsets", "daemonsets", "configmaps", "nodes", "jobs", "ingresses", "networkpolicies", "pvcs", "cronjobs", "hpas", "secrets", "replicasets", "rbac"}

var (
	browserHelpBarStyle = lipgloss.NewStyle().
				Background(theme.DarkerBg).
				Foreground(theme.NeonCyan).
				Padding(0, 1)
	browserHelpBarText = buildBrowserHelpBarText()
)

func buildBrowserHelpBarText() string {
	pairs := []struct{ key, desc string }{
		{"enter", "describe"},
		{"y", "yaml"},
		{"e", "events"},
		{"l", "logs"},
		{"s", "scale"},
		{"R", "restart"},
		{"x", "delete"},
		{"X", "shell"},
		{"space", "mark"},
		{"←/→", "tabs"},
		{"p/d", "pods/deploy"},
		{"/", "filter"},
		{"c", "copy"},
		{"r", "refresh"},
	}
	parts := make([]string, 0, len(pairs))
	for _, p := range pairs {
		parts = append(parts, theme.HelpKey.Render(p.key)+theme.HelpDesc.Render(":"+p.desc))
	}
	return strings.Join(parts, theme.Dim.Render(" | "))
}

const selectionMark = "● "

const restrictedResourceYAMLMessage = "Secret YAML is hidden to protect sensitive data"

const (
	maxEventMessageRunes = 60
	eventMessageSuffix   = "..."
	eventSeparatorWidth  = 90

	scaleInputCharacterLimit         = 5
	scaleInputInitialWidth           = 12
	filterInputCharacterLimit        = 128
	filterInputInitialWidth          = 40
	browserContentHorizontalChrome   = 6
	browserMinimumTableHeight        = 3
	browserVerticalTableMinHeight    = 5
	browserDetailMinimumHeight       = 2
	browserVerticalDetailChrome      = 3
	browserHorizontalTablePercent    = 60
	browserHorizontalTableMinWidth   = 30
	browserHorizontalPaneGap         = 2
	browserBoxChrome                 = 2
	browserMinimumResourceTableWidth = 40
	browserTabHintCapacity           = 3
	confirmHorizontalPadding         = 2
)

type browserState int

const (
	stateBrowsing browserState = iota
	stateDetail
	stateScaleInput
	stateScaleConfirm
	stateDeleteConfirm
	stateFilter
	stateShell
)

type BrowserModel struct {
	width  int
	height int

	namespace    string
	resourceType string

	pods            []service.Pod
	deployments     []service.Deployment
	services        []service.Service
	statefulsets    []service.StatefulSet
	daemonsets      []service.DaemonSet
	configmaps      []service.ConfigMap
	nodes           []service.Node
	jobs            []service.Job
	ingresses       []service.Ingress
	networkpolicies []service.NetworkPolicy
	pvcs            []service.PersistentVolumeClaim
	cronjobs        []service.CronJob
	hpas            []service.HPA
	secrets         []service.Secret
	replicasets     []service.ReplicaSet
	rbac            []service.RBAC

	resourceTable table.Model
	detailView    viewport.Model
	spinner       spinner.Model
	textInput     textinput.Model

	loading       bool
	showDetail    bool
	detailContent string

	filterInput  textinput.Model
	filterActive bool
	filterText   string

	confirmAction    string
	confirmTarget    string
	showConfirm      bool
	scaleName        string
	scaleCurrentInfo string
	scaleReplicas    int

	splitHorizontal bool

	wide bool

	detailKind string

	selected           map[string]bool
	selectedIdentities map[string]resourceIdentity
	visibleResources   []resourceIdentity
	confirmIdentity    resourceIdentity
	scaleIdentity      resourceIdentity

	aiSummary     string
	aiSummaryLoad bool
	aiSummaryErr  error

	podWatcher           watchSupervisor[service.Pod]
	deploymentWatcher    watchSupervisor[service.Deployment]
	ingressWatcher       watchSupervisor[service.Ingress]
	networkPolicyWatcher watchSupervisor[service.NetworkPolicy]
	pvcWatcher           watchSupervisor[service.PersistentVolumeClaim]
	cronJobWatcher       watchSupervisor[service.CronJob]
	hpaWatcher           watchSupervisor[service.HPA]
	secretWatcher        watchSupervisor[service.Secret]
	replicaSetWatcher    watchSupervisor[service.ReplicaSet]

	active bool

	state browserState
	err   error

	statusMsg string
	errBanner string

	shellSession *service.ShellSession
	shellPod     string
	shellNS      string
	shellInput   textinput.Model
	shellView    viewport.Model
	shellLines   []string
	shellHistory []string
	shellHistIdx int

	fetchRequestID  uint64
	detailRequestID uint64
}

type browserResultMsg struct {
	requestID    uint64
	namespace    string
	resourceType string
	payload      tea.Msg
}

type browserDetailSummaryResultMsg struct {
	requestID uint64
	identity  resourceIdentity
	content   string
	payload   tea.Msg
}

func NewBrowserModel(namespace string) BrowserModel {
	sp := spinner.New(
		spinner.WithSpinner(spinner.Dot),
		spinner.WithStyle(theme.SpinnerStyle),
	)

	ti := newTextInput(textInputOpts{
		Prompt:      "Replicas: ",
		Placeholder: "replica count",
		CharLimit:   scaleInputCharacterLimit,
		Width:       scaleInputInitialWidth,
		PromptStyle: lipgloss.NewStyle().Foreground(theme.NeonCyan).Bold(true),
		TextStyle:   lipgloss.NewStyle().Foreground(theme.White),
	})

	fi := newTextInput(textInputOpts{
		Prompt:      "/ ",
		Placeholder: "type to filter...",
		CharLimit:   filterInputCharacterLimit,
		Width:       filterInputInitialWidth,
		PromptStyle: lipgloss.NewStyle().Foreground(theme.NeonCyan).Bold(true),
		TextStyle:   lipgloss.NewStyle().Foreground(theme.White),
	})

	t := buildResourceTable(initialScreenWidth, resourceColSpecs["pods"])
	vp := newViewport(initialScreenWidth, initialViewportHeight)

	return BrowserModel{
		namespace:     namespace,
		resourceType:  "pods",
		resourceTable: t,
		detailView:    vp,
		spinner:       sp,
		textInput:     ti,
		filterInput:   fi,
		loading:       true,
		state:         stateBrowsing,
	}
}

func (m BrowserModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m BrowserModel) Update(msg tea.Msg) (next BrowserModel, command tea.Cmd) {
	defer func() {
		next.syncBrowserLayout()
	}()

	switch msg := msg.(type) {
	case browserResultMsg:
		if !m.acceptsFetchResult(msg) {
			return m, nil
		}
		return m.Update(msg.payload)

	case browserDetailSummaryResultMsg:
		if !m.acceptsDetailSummary(msg) {
			return m, nil
		}
		return m.Update(msg.payload)

	case supervisedWatchMsg:
		return m.handleSupervisedWatchMessage(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.recalcLayout()
		return m, nil
	default:
		return m.updateBrowserWatchMessage(msg)
	}
}

func (m BrowserModel) updateBrowserWatchMessage(msg tea.Msg) (BrowserModel, tea.Cmd) {
	switch msg := msg.(type) {
	case service.WatchEventMsg[service.Pod]:
		return m, handleTypedWatchEvent(&m, &m.podWatcher, &m.pods, podKey, "pods", msg)

	case service.WatchEventMsg[service.Deployment]:
		return m, handleTypedWatchEvent(&m, &m.deploymentWatcher, &m.deployments, deploymentKey, "deployments", msg)

	case service.WatchEventMsg[service.Ingress]:
		return m, handleTypedWatchEvent(&m, &m.ingressWatcher, &m.ingresses, ingressKey, "ingresses", msg)

	case service.WatchEventMsg[service.NetworkPolicy]:
		return m, handleTypedWatchEvent(&m, &m.networkPolicyWatcher, &m.networkpolicies, networkPolicyKey, "networkpolicies", msg)
	default:
		return m.updateBrowserExtendedWatchMessage(msg)
	}
}

func (m BrowserModel) updateBrowserExtendedWatchMessage(msg tea.Msg) (BrowserModel, tea.Cmd) {
	switch msg := msg.(type) {
	case service.WatchEventMsg[service.PersistentVolumeClaim]:
		return m, handleTypedWatchEvent(&m, &m.pvcWatcher, &m.pvcs, pvcKey, "pvcs", msg)

	case service.WatchEventMsg[service.CronJob]:
		return m, handleTypedWatchEvent(&m, &m.cronJobWatcher, &m.cronjobs, cronJobKey, "cronjobs", msg)

	case service.WatchEventMsg[service.HPA]:
		return m, handleTypedWatchEvent(&m, &m.hpaWatcher, &m.hpas, hpaKey, "hpas", msg)

	case service.WatchEventMsg[service.Secret]:
		return m, handleTypedWatchEvent(&m, &m.secretWatcher, &m.secrets, secretKey, "secrets", msg)

	case service.WatchEventMsg[service.ReplicaSet]:
		return m, handleTypedWatchEvent(&m, &m.replicaSetWatcher, &m.replicasets, replicaSetKey, "replicasets", msg)
	default:
		return m.updateBrowserLifecycleMessage(msg)
	}
}

func (m BrowserModel) updateBrowserLifecycleMessage(msg tea.Msg) (BrowserModel, tea.Cmd) {
	switch msg := msg.(type) {
	case shellOutputMsg:
		return m.handleShellOutput(msg)

	case shellExitMsg:
		return m.handleShellExit(msg)

	case browserWatchClosedMsg:
		return m.handleSharedWatchClosed(msg.Kind)

	case browserReconnectMsg:
		return m.handleSharedReconnect(msg)
	default:
		return m.updateWorkloadFetchMessage(msg)
	}
}

func (m BrowserModel) updateWorkloadFetchMessage(msg tea.Msg) (BrowserModel, tea.Cmd) {
	switch msg := msg.(type) {
	case service.PodsMsg:
		return applyBrowserFetch(&m, "pods", &m.pods, msg.Pods, msg.Err)

	case service.DeploymentsMsg:
		return applyBrowserFetch(&m, "deployments", &m.deployments, msg.Deployments, msg.Err)

	case service.StatefulSetsMsg:
		return applyBrowserFetch(&m, "statefulsets", &m.statefulsets, msg.StatefulSets, msg.Err)

	case service.DaemonSetsMsg:
		return applyBrowserFetch(&m, "daemonsets", &m.daemonsets, msg.DaemonSets, msg.Err)
	default:
		return m.updateCoreBrowserFetchMessage(msg)
	}
}

func (m BrowserModel) updateCoreBrowserFetchMessage(msg tea.Msg) (BrowserModel, tea.Cmd) {
	switch msg := msg.(type) {
	case service.ServicesMsg:
		return applyBrowserFetch(&m, "services", &m.services, msg.Services, msg.Err)

	case service.ConfigMapsMsg:
		return applyBrowserFetch(&m, "configmaps", &m.configmaps, msg.ConfigMaps, msg.Err)

	case service.NodesMsg:
		return applyBrowserFetch(&m, "nodes", &m.nodes, msg.Nodes, msg.Err)

	case service.JobsMsg:
		return applyBrowserFetch(&m, "jobs", &m.jobs, msg.Jobs, msg.Err)
	default:
		return m.updateNetworkStorageFetchMessage(msg)
	}
}

func (m BrowserModel) updateNetworkStorageFetchMessage(msg tea.Msg) (BrowserModel, tea.Cmd) {
	switch msg := msg.(type) {
	case service.IngressesMsg:
		return applyBrowserFetch(&m, "ingresses", &m.ingresses, msg.Ingresses, msg.Err)

	case service.NetworkPoliciesMsg:
		return applyBrowserFetch(&m, "networkpolicies", &m.networkpolicies, msg.NetworkPolicies, msg.Err)

	case service.PVCsMsg:
		return applyBrowserFetch(&m, "pvcs", &m.pvcs, msg.PVCs, msg.Err)
	default:
		return m.updateScheduledSecurityFetchMessage(msg)
	}
}

func (m BrowserModel) updateScheduledSecurityFetchMessage(msg tea.Msg) (BrowserModel, tea.Cmd) {
	switch msg := msg.(type) {
	case service.CronJobsMsg:
		return applyBrowserFetch(&m, "cronjobs", &m.cronjobs, msg.CronJobs, msg.Err)

	case service.HPAsMsg:
		return applyBrowserFetch(&m, "hpas", &m.hpas, msg.HPAs, msg.Err)

	case service.SecretsMsg:
		return applyBrowserFetch(&m, "secrets", &m.secrets, msg.Secrets, msg.Err)

	case service.ReplicaSetsMsg:
		return applyBrowserFetch(&m, "replicasets", &m.replicasets, msg.ReplicaSets, msg.Err)

	case service.RBACMsg:
		return applyBrowserFetch(&m, "rbac", &m.rbac, msg.RBAC, msg.Err)
	default:
		return m.updateBrowserResultMessage(msg)
	}
}

func applyBrowserFetch[T browserFetchResource](
	model *BrowserModel,
	resourceType string,
	destination *[]T,
	items []T,
	err error,
) (BrowserModel, tea.Cmd) {
	applyTypedFetchResult(model, resourceType, destination, items, err)
	return *model, nil
}

func (m BrowserModel) updateBrowserResultMessage(msg tea.Msg) (BrowserModel, tea.Cmd) {
	switch msg := msg.(type) {
	case service.DescribeMsg:
		m.handleDescribeResult(msg)
		return m, nil

	case service.DescribeSummaryMsg:
		m.handleDescribeSummaryResult(msg)
		return m, nil

	case service.EventsMsg:
		m.handleEventsResult(msg)
		return m, nil

	case service.YAMLMsg:
		m.handleYAMLResult(msg)
		return m, nil

	case service.CommandResultMsg:
		return m, m.handleBrowserCommandResult(msg)

	case ClearStatusMsg:
		m.statusMsg = ""
		return m, nil

	case spinner.TickMsg:
		return m, m.handleBrowserSpinnerTick(msg)
	default:
		return m.updateBrowserInputMessage(msg)
	}
}

func (m BrowserModel) updateBrowserInputMessage(msg tea.Msg) (BrowserModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.MouseClickMsg:
		return m.handleBrowserMouseClick(msg)

	case tea.MouseWheelMsg:
		return m.handleBrowserMouseWheel(msg)

	case tea.MouseMotionMsg:
		return m.handleBrowserMouseMotion(msg)

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	default:
		return m, m.forwardBrowserDetailMessage(msg)
	}
}

func (m *BrowserModel) handleDescribeResult(msg service.DescribeMsg) {
	m.loading = false
	if msg.Err != nil {
		m.errBanner = kubectlActionErr("describe", msg.Err)
		return
	}
	output := sanitizeTerminalText(msg.Output)
	m.openBrowserDetail("describe", output, output, m.detailHelp())
}

func (m *BrowserModel) handleDescribeSummaryResult(msg service.DescribeSummaryMsg) {
	m.aiSummaryLoad = false
	if msg.Err != nil {
		m.aiSummaryErr = msg.Err
		m.aiSummary = ""
	} else {
		m.aiSummaryErr = nil
		m.aiSummary = sanitizeTerminalText(msg.Summary)
	}
	m.rebuildDetailContent()
}

func (m *BrowserModel) handleEventsResult(msg service.EventsMsg) {
	m.loading = false
	if msg.Err != nil {
		m.errBanner = kubectlActionErr("events", msg.Err)
		return
	}
	content := formatEventsOutput(msg.Events)
	m.openBrowserDetail(
		"events",
		content,
		content,
		theme.Dim.Render("esc: back | scroll with j/k or mouse wheel"),
	)
}

func (m *BrowserModel) handleYAMLResult(msg service.YAMLMsg) {
	m.loading = false
	if msg.Err != nil {
		m.errBanner = kubectlActionErr("yaml", msg.Err)
		return
	}
	output := sanitizeTerminalText(msg.Output)
	m.openBrowserDetail(
		"yaml",
		output,
		service.HighlightYAML(output),
		theme.Dim.Render("esc: back | c: copy | v: split"),
	)
}

func (m *BrowserModel) openBrowserDetail(kind, content, renderedContent, status string) {
	m.errBanner = ""
	m.detailRequestID++
	m.detailKind = kind
	m.detailContent = content
	m.detailView.SetContent(renderedContent)
	m.detailView.GotoTop()
	m.showDetail = true
	m.state = stateDetail
	m.aiSummary = ""
	m.aiSummaryErr = nil
	m.aiSummaryLoad = false
	m.statusMsg = status
}

func (m *BrowserModel) handleBrowserCommandResult(msg service.CommandResultMsg) tea.Cmd {
	m.loading = false
	m.showConfirm = false
	m.state = stateBrowsing
	if msg.Err != nil {
		m.statusMsg = theme.Error.Render("Command failed: " + sanitizeTerminalLine(msg.Err.Error()))
		return nil
	}
	m.statusMsg = theme.Success.Render(strings.TrimSpace(sanitizeTerminalText(msg.Output)))
	return m.fetchCurrentResources()
}

func (m *BrowserModel) handleBrowserSpinnerTick(msg spinner.TickMsg) tea.Cmd {
	if !m.loading && !m.aiSummaryLoad {
		return nil
	}
	var command tea.Cmd
	m.spinner, command = m.spinner.Update(msg)
	return command
}

func (m BrowserModel) handleBrowserMouseClick(msg tea.MouseClickMsg) (BrowserModel, tea.Cmd) {
	if m.state == stateDetail {
		var command tea.Cmd
		m.detailView, command = m.detailView.Update(msg)
		return m, command
	}
	if m.state == stateBrowsing && msg.Button == tea.MouseLeft {
		return m.handleBrowseClick(msg.X, msg.Y)
	}
	return m, nil
}

func (m BrowserModel) handleBrowserMouseWheel(msg tea.MouseWheelMsg) (BrowserModel, tea.Cmd) {
	switch m.state {
	case stateDetail:
		var command tea.Cmd
		m.detailView, command = m.detailView.Update(msg)
		return m, command
	case stateShell:
		var command tea.Cmd
		m.shellView, command = m.shellView.Update(msg)
		return m, command
	case stateBrowsing:
		m.moveBrowserSelectionWithWheel(msg.Button)
	case stateScaleInput, stateScaleConfirm, stateDeleteConfirm, stateFilter:
	}
	return m, nil
}

func (m *BrowserModel) moveBrowserSelectionWithWheel(button tea.MouseButton) {
	switch button {
	case tea.MouseWheelUp:
		m.resourceTable.MoveUp(tableWheelStep)
	case tea.MouseWheelDown:
		m.resourceTable.MoveDown(tableWheelStep)
	}
}

func (m BrowserModel) handleBrowserMouseMotion(msg tea.MouseMotionMsg) (BrowserModel, tea.Cmd) {
	if m.state != stateDetail {
		return m, nil
	}
	var command tea.Cmd
	m.detailView, command = m.detailView.Update(msg)
	return m, command
}

func (m *BrowserModel) forwardBrowserDetailMessage(msg tea.Msg) tea.Cmd {
	if m.state != stateDetail {
		return nil
	}
	var command tea.Cmd
	m.detailView, command = m.detailView.Update(msg)
	return command
}

func (m BrowserModel) acceptsFetchResult(msg browserResultMsg) bool {
	return msg.requestID == m.fetchRequestID &&
		msg.namespace == m.namespace &&
		msg.resourceType == m.resourceType
}

func (m BrowserModel) handleBrowseClick(x, y int) (BrowserModel, tea.Cmd) {
	titleH := lipgloss.Height(m.renderTitleBar())
	if y < titleH {
		return m.handleTitleBarClick(x)
	}
	rowIndex := y - m.tableFirstRowY()
	if rowIndex >= 0 && rowIndex < len(m.resourceTable.Rows()) {
		m.resourceTable.SetCursor(rowIndex)
	}
	return m, nil
}

// tableFirstRowY derives the first data-row position from rendered chrome.
func (m BrowserModel) tableFirstRowY() int {
	y := lipgloss.Height(m.renderTitleBar()) + tableHeaderChromeRows
	if filter := m.renderFilterBar(); filter != "" {
		y += lipgloss.Height(filter)
	}
	if errBanner := m.renderErrBanner(); errBanner != "" {
		y += lipgloss.Height(errBanner)
	}
	return y
}

func (m BrowserModel) handleTitleBarClick(x int) (BrowserModel, tea.Cmd) {
	titleRendered := theme.Title.Render("KUBERNETES BROWSER")
	relativeX := x - (lipgloss.Width(titleRendered) + titleBarSidePadding)
	if relativeX < 0 {
		return m, nil
	}
	cumX := 0
	for _, rt := range allResourceTypes {
		label := " " + strings.ToUpper(rt) + " "
		var tabW int
		if rt == m.resourceType {
			tabW = lipgloss.Width(theme.StatusBarActive.Render(label))
		} else {
			tabW = lipgloss.Width(theme.StatusBarItem.Render(label))
		}
		if relativeX >= cumX && relativeX < cumX+tabW {
			if rt != m.resourceType {
				m.SetResourceType(rt)
				m.loading = true
				cmds := []tea.Cmd{m.fetchCurrentResources(), m.spinner.Tick}
				if watchCmd := m.startResourceWatch(); watchCmd != nil {
					cmds = append(cmds, watchCmd)
				}
				return m, tea.Batch(cmds...)
			}
			return m, nil
		}
		cumX += tabW
	}
	return m, nil
}

func (m BrowserModel) handleKey(msg tea.KeyPressMsg) (BrowserModel, tea.Cmd) {
	key := msg.String()
	switch m.state {
	case stateShell:
		return m.handleShellKey(msg)
	case stateFilter:
		return m.handleFilterKey(key, msg)
	case stateScaleInput:
		return m.handleScaleInputKey(key, msg)
	case stateScaleConfirm:
		return m.handleScaleConfirmationKey(key)
	case stateDeleteConfirm:
		return m.handleResourceConfirmationKey(key)
	case stateDetail:
		return m.handleDetailKey(key, msg)
	case stateBrowsing:
		return m.handleBrowsingKey(key, msg)
	default:
		m.state = stateBrowsing
		return m, nil
	}
}

func (m BrowserModel) handleFilterKey(key string, msg tea.KeyPressMsg) (BrowserModel, tea.Cmd) {
	switch key {
	case "esc":
		m.filterActive = false
		m.filterText = ""
		m.filterInput.SetValue("")
		m.filterInput.Blur()
		m.state = stateBrowsing
		m.rebuildTable()
		return m, nil
	case "enter":
		m.filterText = m.filterInput.Value()
		m.filterInput.Blur()
		m.state = stateBrowsing
		m.rebuildTable()
		m.filterActive = m.filterText != ""
		return m, nil
	default:
		var command tea.Cmd
		m.filterInput, command = m.filterInput.Update(msg)
		m.filterText = m.filterInput.Value()
		m.refreshRows(m.resourceTable.Cursor())
		return m, command
	}
}

func (m BrowserModel) handleScaleInputKey(key string, msg tea.KeyPressMsg) (BrowserModel, tea.Cmd) {
	switch key {
	case "esc":
		m.state = stateBrowsing
		m.textInput.Blur()
		m.statusMsg = ""
		return m, nil
	case "enter":
		return m, m.beginScaleConfirmation()
	default:
		var command tea.Cmd
		m.textInput, command = m.textInput.Update(msg)
		return m, command
	}
}

func (m *BrowserModel) beginScaleConfirmation() tea.Cmd {
	value := strings.TrimSpace(m.textInput.Value())
	replicas, err := strconv.Atoi(value)
	if err != nil || replicas < 0 {
		m.statusMsg = theme.Error.Render("Invalid replica count: " + value)
		return nil
	}
	m.scaleReplicas = replicas
	m.confirmAction = "scale"
	m.confirmTarget = fmt.Sprintf("%s to %d replicas", m.scaleIdentity.Name, replicas)
	m.showConfirm = true
	m.state = stateScaleConfirm
	return nil
}

func (m BrowserModel) handleScaleConfirmationKey(key string) (BrowserModel, tea.Cmd) {
	switch key {
	case "y", "Y":
		identity := m.scaleIdentity
		m.showConfirm = false
		m.loading = true
		m.state = stateBrowsing
		m.textInput.Blur()
		m.statusMsg = theme.SpinnerStyle.Render("Scaling " + identity.Kind + "/" + identity.Name + "...")
		return m, service.ScaleResource(identity.Namespace, identity.Kind, identity.Name, m.scaleReplicas)
	case "n", "N", "esc":
		m.showConfirm = false
		m.state = stateBrowsing
		m.textInput.Blur()
		m.statusMsg = theme.Dim.Render("Scale cancelled")
	}
	return m, nil
}

func (m BrowserModel) handleResourceConfirmationKey(key string) (BrowserModel, tea.Cmd) {
	switch key {
	case "y", "Y":
		return m, m.executeConfirmedResourceAction()
	case "n", "N", "esc":
		m.showConfirm = false
		m.state = stateBrowsing
		m.statusMsg = theme.Dim.Render(m.confirmAction + " cancelled")
	}
	return m, nil
}

func (m BrowserModel) handleDetailKey(key string, msg tea.KeyPressMsg) (BrowserModel, tea.Cmd) {
	switch key {
	case "esc":
		m.closeDetail()
		return m, nil
	case "a":
		return m, m.analyzeDetail()
	case "v":
		m.splitHorizontal = !m.splitHorizontal
		return m, nil
	case "c":
		return m.copyDetailContent()
	default:
		var command tea.Cmd
		m.detailView, command = m.detailView.Update(msg)
		return m, command
	}
}

func (m *BrowserModel) closeDetail() {
	m.detailRequestID++
	m.showDetail = false
	m.state = stateBrowsing
	m.statusMsg = ""
	m.aiSummary = ""
	m.aiSummaryErr = nil
	m.detailKind = ""
}

func (m *BrowserModel) analyzeDetail() tea.Cmd {
	if !m.canAnalyzeDetail() {
		m.statusMsg = theme.Warning.Render("Analysis is only available for non-secret describe output")
		return nil
	}
	if m.aiSummaryLoad {
		return nil
	}
	identity, found := m.selectedIdentity()
	if !found {
		m.statusMsg = theme.Warning.Render("No resource selected")
		return nil
	}
	m.aiSummaryLoad = true
	m.aiSummary = ""
	m.aiSummaryErr = nil
	m.statusMsg = theme.SpinnerStyle.Render("Analyzing " + identity.Kind + "/" + identity.Name + "...")
	return m.fetchDetailSummary(identity)
}

func (m BrowserModel) copyDetailContent() (BrowserModel, tea.Cmd) {
	lineCount := strings.Count(m.detailContent, "\n") + 1
	status, command := copyToClipboard(m.detailContent, fmt.Sprintf("%d lines", lineCount))
	m.statusMsg = status
	return m, command
}

func (m BrowserModel) handleBrowsingKey(key string, msg tea.KeyPressMsg) (BrowserModel, tea.Cmd) {
	m.statusMsg = ""
	if key == "esc" {
		return m, m.handleBrowsingEscape()
	}
	if key == "space" {
		m.toggleSelectedResource()
		return m, nil
	}

	switch key {
	case "left", "shift+tab":
		return m.cycleResourceType(-1)
	case "right", "tab":
		return m.cycleResourceType(1)
	case "p", "d":
		return m.selectResourceTypeShortcut(key)
	case "enter", "e", "l", "s", "y":
		return m.handleResourceReadKey(key)
	case "R", "x":
		return m.handleResourceMutationKey(key)
	case "X":
		return m.openShell()
	default:
		return m.handleBrowserUtilityKey(key, msg)
	}
}

func (m *BrowserModel) handleBrowsingEscape() tea.Cmd {
	if len(m.selected) > 0 {
		m.clearSelection()
		m.rebuildTable()
		return nil
	}
	if m.errBanner != "" {
		m.errBanner = ""
		return nil
	}
	return func() tea.Msg { return GoBackMsg{} }
}

func (m *BrowserModel) toggleSelectedResource() {
	identity, found := m.selectedIdentity()
	if !found {
		return
	}
	m.toggleResourceSelection(identity)
	m.rebuildTable()
}

func (m BrowserModel) selectResourceTypeShortcut(key string) (BrowserModel, tea.Cmd) {
	if key == "p" {
		return m.selectResourceType("pods")
	}
	return m.selectResourceType("deployments")
}

func (m BrowserModel) selectResourceType(resourceType string) (BrowserModel, tea.Cmd) {
	if m.resourceType == resourceType {
		return m, nil
	}
	m.SetResourceType(resourceType)
	m.loading = true
	commands := []tea.Cmd{m.fetchCurrentResources(), m.spinner.Tick}
	if watchCommand := m.startResourceWatch(); watchCommand != nil {
		commands = append(commands, watchCommand)
	}
	return m, tea.Batch(commands...)
}

func (m BrowserModel) handleResourceReadKey(key string) (BrowserModel, tea.Cmd) {
	switch key {
	case "enter":
		return m, m.describeSelectedResource()
	case "e":
		return m, m.fetchSelectedResourceEvents()
	case "l":
		return m, m.openSelectedResourceLogs()
	case "s":
		return m, m.openScaleInput()
	case "y":
		return m, m.fetchSelectedResourceYAML()
	default:
		return m, nil
	}
}

func (m *BrowserModel) describeSelectedResource() tea.Cmd {
	resourceType, name := m.selectedResourceKindAndName()
	if name == "" {
		return nil
	}
	m.loading = true
	m.statusMsg = theme.SpinnerStyle.Render("Describing " + resourceType + "/" + name + "...")
	return tea.Batch(
		service.DescribeResource(m.selectedResourceNS(), resourceType, name),
		m.spinner.Tick,
	)
}

func (m *BrowserModel) fetchSelectedResourceEvents() tea.Cmd {
	_, name := m.selectedResourceKindAndName()
	if name == "" {
		return nil
	}
	m.loading = true
	m.statusMsg = theme.SpinnerStyle.Render("Fetching events...")
	return tea.Batch(service.FetchEvents(m.selectedResourceNS()), m.spinner.Tick)
}

func (m *BrowserModel) openSelectedResourceLogs() tea.Cmd {
	if m.resourceType != "pods" {
		m.statusMsg = theme.Warning.Render("Logs are only available for pods")
		return nil
	}
	_, name := m.selectedResourceKindAndName()
	if name == "" {
		return nil
	}
	namespace := m.selectedResourceNS()
	return func() tea.Msg {
		return DrillDownMsg{Screen: ScreenLogs, ResourceName: name, ResourceNS: namespace}
	}
}

func (m *BrowserModel) openScaleInput() tea.Cmd {
	if !m.resourceSupportsScaling() {
		m.statusMsg = theme.Warning.Render("Scale is only available for deployments and statefulsets")
		return nil
	}
	identity, found := m.selectedIdentity()
	if !found {
		return nil
	}
	m.textInput.Reset()
	m.textInput.Focus()
	m.state = stateScaleInput
	m.scaleName = identity.Name
	m.scaleIdentity = identity
	m.scaleCurrentInfo = m.lookupReplicaInfoFor(identity)
	m.statusMsg = theme.Accent.Render("Scale " + identity.Name + " -- enter replica count, esc to cancel")
	return textinput.Blink
}

func (m *BrowserModel) fetchSelectedResourceYAML() tea.Cmd {
	if m.resourceType == "secrets" {
		m.statusMsg = theme.Warning.Render(restrictedResourceYAMLMessage)
		return nil
	}
	resourceType, name := m.selectedResourceKindAndName()
	if name == "" {
		return nil
	}
	m.loading = true
	m.statusMsg = theme.SpinnerStyle.Render("Fetching YAML for " + resourceType + "/" + name + "...")
	return tea.Batch(service.GetYAML(m.selectedResourceNS(), resourceType, name), m.spinner.Tick)
}

func (m BrowserModel) resourceSupportsScaling() bool {
	return m.resourceType == "deployments" || m.resourceType == "statefulsets"
}

func (m BrowserModel) handleResourceMutationKey(key string) (BrowserModel, tea.Cmd) {
	action := "delete"
	if key == "R" {
		if !m.resourceSupportsScaling() {
			m.statusMsg = theme.Warning.Render("Restart rollout is only available for deployments and statefulsets")
			return m, nil
		}
		action = "restart"
	}
	return m, m.beginResourceConfirmation(action)
}

func (m *BrowserModel) beginResourceConfirmation(action string) tea.Cmd {
	if len(m.selected) > 0 {
		return m.beginBatchResourceConfirmation(action)
	}
	identity, found := m.selectedIdentity()
	if !found {
		return nil
	}
	m.confirmAction = action
	m.confirmTarget = identity.Kind + "/" + identity.Name
	if action == "restart" {
		m.confirmTarget = m.resourceType + "/" + identity.Name
	}
	m.confirmIdentity = identity
	m.showConfirm = true
	m.state = stateDeleteConfirm
	return nil
}

func (m *BrowserModel) beginBatchResourceConfirmation(action string) tea.Cmd {
	if m.namespace == "" {
		m.errBanner = batchAllNamespacesErr(action)
		return nil
	}
	m.confirmAction = action
	m.confirmTarget = fmt.Sprintf("%d %s", len(m.selected), m.resourceType)
	m.showConfirm = true
	m.state = stateDeleteConfirm
	return nil
}

func (m BrowserModel) handleBrowserUtilityKey(key string, msg tea.KeyPressMsg) (BrowserModel, tea.Cmd) {
	switch key {
	case "c":
		return m.copyBrowserSelection()
	case "/":
		m.filterActive = true
		m.filterInput.SetValue(m.filterText)
		m.filterInput.Focus()
		m.state = stateFilter
		return m, textinput.Blink
	case "r":
		m.loading = true
		return m, tea.Batch(m.fetchCurrentResources(), m.spinner.Tick)
	case "w":
		m.wide = !m.wide
		m.rebuildTable()
		return m, nil
	default:
		var command tea.Cmd
		m.resourceTable, command = m.resourceTable.Update(msg)
		return m, command
	}
}

func (m BrowserModel) copyBrowserSelection() (BrowserModel, tea.Cmd) {
	if m.showDetail {
		return m.copyDetailContent()
	}
	row := m.resourceTable.SelectedRow()
	if row == nil {
		return m, nil
	}
	text := strings.Join(row, "\t")
	status, command := copyToClipboard(text, "row")
	m.statusMsg = status
	return m, command
}

func (m *BrowserModel) executeConfirmedResourceAction() tea.Cmd {
	m.showConfirm = false
	m.loading = true
	m.state = stateBrowsing

	resourceKind := m.resourceKindSingular()
	if len(m.selected) > 0 {
		resourceNames := m.selectedNames()
		m.clearSelection()
		return m.executeConfirmedBatchAction(resourceKind, resourceNames)
	}
	return m.executeConfirmedSingleAction()
}

func (m *BrowserModel) executeConfirmedBatchAction(resourceKind string, resourceNames []string) tea.Cmd {
	if m.confirmAction == "restart" {
		m.statusMsg = theme.SpinnerStyle.Render("Restarting " + m.confirmTarget + "...")
		return service.RestartRollouts(m.namespace, resourceKind, resourceNames)
	}
	m.statusMsg = theme.SpinnerStyle.Render("Deleting " + m.confirmTarget + "...")
	return service.DeleteResources(m.namespace, resourceKind, resourceNames)
}

func (m *BrowserModel) executeConfirmedSingleAction() tea.Cmd {
	identity := m.confirmIdentity
	if m.confirmAction == "restart" {
		m.statusMsg = theme.SpinnerStyle.Render("Restarting " + m.confirmTarget + "...")
		return service.RestartRollout(identity.Namespace, identity.Kind, identity.Name)
	}
	m.statusMsg = theme.SpinnerStyle.Render("Deleting " + identity.Kind + "/" + identity.Name + "...")
	return service.DeleteResource(identity.Namespace, identity.Kind, identity.Name)
}

func (m BrowserModel) View() string {
	if m.width == 0 {
		return ""
	}

	titleBar := m.renderTitleBar()
	filterBar := m.renderFilterBar()
	errBanner := m.renderErrBanner()
	helpBar := m.renderHelpBar()
	statusLine := m.renderStatusLine()
	content := m.renderBrowserMainContent(m.browserContentHeight())
	sections := []string{titleBar}
	if filterBar != "" {
		sections = append(sections, filterBar)
	}
	if errBanner != "" {
		sections = append(sections, errBanner)
	}
	sections = append(sections, content, statusLine, helpBar)

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m BrowserModel) renderBrowserMainContent(height int) string {
	switch {
	case m.showConfirm:
		return lipgloss.Place(
			m.width,
			height,
			lipgloss.Center,
			lipgloss.Center,
			m.renderConfirmBox(),
		)
	case m.state == stateScaleInput:
		return lipgloss.Place(
			m.width,
			height,
			lipgloss.Center,
			lipgloss.Center,
			m.renderScaleBox(),
		)
	case m.state == stateShell:
		return m.renderShellSplit(height)
	case m.showDetail && m.splitHorizontal && m.width >= narrowHsplitMinWidth:
		return m.renderHSplitContent(height)
	case m.showDetail:
		return m.renderSplitContent(height)
	default:
		return m.renderTableContent(height)
	}
}

func (m BrowserModel) HasInputFocus() bool {
	return m.state == stateScaleInput || m.state == stateScaleConfirm ||
		m.state == stateDeleteConfirm || m.state == stateFilter ||
		m.state == stateShell
}

func (m BrowserModel) AIOverlayBounds(totalHeight int) (topOffset, panelHeight, bottomOffset int) {
	topOffset = lipgloss.Height(m.renderTitleBar())
	if filter := m.renderFilterBar(); filter != "" {
		topOffset += lipgloss.Height(filter)
	}
	if errBan := m.renderErrBanner(); errBan != "" {
		topOffset += lipgloss.Height(errBan)
	}
	bottomOffset = lipgloss.Height(m.renderStatusLine()) + lipgloss.Height(m.renderHelpBar())
	return aiOverlayBounds(totalHeight, topOffset, bottomOffset)
}

func (m *BrowserModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.recalcLayout()
}

func (m *BrowserModel) SetNamespace(ns string) {
	m.fetchRequestID++
	m.detailRequestID++
	m.namespace = ns
	for _, b := range resourceCatalog {
		b.Clear(m)
	}
	m.loading = true
	m.showDetail = false
	m.showConfirm = false
	m.state = stateBrowsing
	m.statusMsg = ""
	m.errBanner = ""
	m.err = nil
	m.aiSummary = ""
	m.aiSummaryErr = nil
	m.aiSummaryLoad = false
	m.clearSelection()
	m.stopAllWatchers()
}

func (m BrowserModel) SelectedResource() (string, string) {
	return m.selectedResourceKindAndName()
}

func (m BrowserModel) ResourceType() string {
	return m.resourceType
}

// Wide reports whether wide columns are active.
func (m BrowserModel) Wide() bool {
	return m.wide
}

// SetWide restores the persisted wide-column setting.
func (m *BrowserModel) SetWide(w bool) {
	m.wide = w
}

func (m BrowserModel) cycleResourceType(delta int) (BrowserModel, tea.Cmd) {
	idx := -1
	for i, rt := range allResourceTypes {
		if rt == m.resourceType {
			idx = i
			break
		}
	}
	if idx < 0 {
		idx = 0
	}
	next := (idx + delta + len(allResourceTypes)) % len(allResourceTypes)
	target := allResourceTypes[next]
	return m.selectResourceType(target)
}

func (m *BrowserModel) SetResourceType(rt string) {
	if m.resourceType == rt {
		return
	}
	m.resourceType = rt
	m.fetchRequestID++
	m.detailRequestID++
	m.clearSelection()
	m.stopAllWatchers()
	m.rebuildTable()
}

func (m BrowserModel) canAnalyzeDetail() bool {
	return m.detailKind == "describe" && m.resourceType != "secrets"
}

func (m BrowserModel) detailHelp() string {
	keys := "esc: back | c: copy | v: split"
	if m.canAnalyzeDetail() {
		keys = "esc: back | a: summary | c: copy | v: split"
	}
	return theme.Dim.Render(keys)
}

func (m BrowserModel) providerDetailContext() string {
	if m.resourceType == "secrets" {
		return ""
	}
	return m.detailContent
}

func (m *BrowserModel) fetchDetailSummary(identity resourceIdentity) tea.Cmd {
	m.detailRequestID++
	requestID := m.detailRequestID
	content := m.detailContent
	command := service.AIDescribeSummary(identity.Kind, identity.Name, content)
	return func() tea.Msg {
		return browserDetailSummaryResultMsg{
			requestID: requestID,
			identity:  identity,
			content:   content,
			payload:   command(),
		}
	}
}

func (m BrowserModel) acceptsDetailSummary(msg browserDetailSummaryResultMsg) bool {
	identity, ok := m.selectedIdentity()
	return ok && m.canAnalyzeDetail() &&
		msg.requestID == m.detailRequestID &&
		msg.identity == identity &&
		msg.content == m.detailContent
}

func (m BrowserModel) selectedResourceKindAndName() (string, string) {
	identity, ok := m.selectedIdentity()
	if !ok {
		return m.resourceKindSingular(), ""
	}
	return identity.Kind, identity.Name
}

func (m BrowserModel) selectedIdentity() (resourceIdentity, bool) {
	index := m.resourceTable.Cursor()
	if index >= 0 && index < len(m.visibleResources) {
		return m.visibleResources[index], true
	}
	row := m.resourceTable.SelectedRow()
	if len(row) == 0 {
		return resourceIdentity{}, false
	}
	nameIndex := 0
	kind := m.resourceKindSingular()
	if m.resourceType == "rbac" && len(row) > 1 {
		kind = strings.ToLower(row[0])
		nameIndex = 1
	}
	name := strings.TrimPrefix(row[nameIndex], selectionMark)
	return resourceIdentity{Kind: kind, Namespace: m.namespace, Name: name}, name != ""
}

func (m BrowserModel) resourceKindSingular() string {
	if b, ok := resourceCatalog[m.resourceType]; ok {
		return b.Singular
	}
	return m.resourceType
}

func (m *BrowserModel) toggleResourceSelection(identity resourceIdentity) {
	if m.selected == nil {
		m.selected = make(map[string]bool)
	}
	if m.selectedIdentities == nil {
		m.selectedIdentities = make(map[string]resourceIdentity)
	}
	key := m.selectionKey(identity)
	if m.selected[key] {
		delete(m.selected, key)
		delete(m.selectedIdentities, key)
	} else {
		m.selected[key] = true
		m.selectedIdentities[key] = identity
	}
}

func (m *BrowserModel) clearSelection() {
	m.selected = nil
	m.selectedIdentities = nil
}

func (m BrowserModel) selectedNames() []string {
	names := make([]string, 0, len(m.selected))
	for key := range m.selected {
		if identity, ok := m.selectedIdentities[key]; ok {
			names = append(names, identity.Name)
			continue
		}
		names = append(names, key)
	}
	return names
}

func (m BrowserModel) displayIdentity(identity resourceIdentity) string {
	name := displayResourceName(identity.Namespace, identity.Name, m.namespace == "")
	if m.selected[m.selectionKey(identity)] {
		return selectionMark + name
	}
	return name
}

func (m BrowserModel) selectionKey(identity resourceIdentity) string {
	if m.namespace != "" && m.resourceType != "rbac" {
		return identity.Name
	}
	return identity.key()
}

func (m BrowserModel) podStatusFor(identity resourceIdentity) (string, bool) {
	for _, p := range m.pods {
		if p.Name == identity.Name && namespacesMatch(identity.Namespace, p.Namespace) {
			return p.Status, true
		}
	}
	return "", false
}

func podSupportsExec(status string) bool {
	return status == "Running"
}

func (m BrowserModel) selectedResourceNS() string {
	identity, ok := m.selectedIdentity()
	if !ok {
		return m.namespace
	}
	return identity.Namespace
}

func (m *BrowserModel) fetchCurrentResources() tea.Cmd {
	m.fetchRequestID++
	requestID := m.fetchRequestID
	namespace := m.namespace
	resourceType := m.resourceType
	command := service.FetchPods(namespace)
	if binding, ok := resourceCatalog[resourceType]; ok {
		command = binding.Fetch(namespace)
	}
	return func() tea.Msg {
		return browserResultMsg{
			requestID:    requestID,
			namespace:    namespace,
			resourceType: resourceType,
			payload:      command(),
		}
	}
}

func (m *BrowserModel) recalcLayout() {
	if m.width == 0 {
		return
	}
	m.rebuildTable()
	m.syncBrowserLayout()
}

func (m BrowserModel) browserContentHeight() int {
	chromeHeight := lipgloss.Height(m.renderTitleBar()) +
		lipgloss.Height(m.renderHelpBar()) +
		lipgloss.Height(m.renderStatusLine())
	if filterBar := m.renderFilterBar(); filterBar != "" {
		chromeHeight += lipgloss.Height(filterBar)
	}
	if errBanner := m.renderErrBanner(); errBanner != "" {
		chromeHeight += lipgloss.Height(errBanner)
	}
	return max(1, m.height-chromeHeight)
}

func (m *BrowserModel) syncBrowserLayout() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	height := m.browserContentHeight()
	switch {
	case m.state == stateShell:
		m.syncShellLayout(height)
	case m.showDetail && m.splitHorizontal && m.width >= narrowHsplitMinWidth:
		m.syncHorizontalDetailLayout(height)
	case m.showDetail:
		m.syncVerticalDetailLayout(height)
	default:
		m.syncFullTableLayout(height)
	}
}

func (m *BrowserModel) syncFullTableLayout(height int) {
	contentWidth := max(1, m.width-browserContentHorizontalChrome)
	if specs, ok := selectColSpecs(m.resourceType, m.wide); ok {
		m.resourceTable.SetColumns(computeColumns(contentWidth, specs))
	}
	m.resourceTable.SetWidth(contentWidth)
	m.resourceTable.SetHeight(max(browserMinimumTableHeight, height-browserBoxChrome))
}

func (m *BrowserModel) syncVerticalDetailLayout(height int) {
	topHeight := max(browserVerticalTableMinHeight, height/pairedSides)
	m.syncFullTableLayout(topHeight)
	detailHeight := max(browserDetailMinimumHeight, height-topHeight-browserVerticalDetailChrome)
	m.detailView.SetWidth(max(1, m.width-browserContentHorizontalChrome))
	m.detailView.SetHeight(detailHeight)
}

func (m *BrowserModel) syncHorizontalDetailLayout(height int) {
	leftWidth, rightWidth := browserHorizontalPaneWidths(m.width)
	tableWidth := max(1, theme.BoxContentWidth(leftWidth-browserBoxChrome))
	if specs, ok := selectColSpecs(m.resourceType, m.wide); ok {
		m.resourceTable.SetColumns(computeColumns(tableWidth, specs))
	}
	m.resourceTable.SetWidth(tableWidth)
	m.resourceTable.SetHeight(max(browserMinimumTableHeight, height-browserBoxChrome))
	m.detailView.SetWidth(max(1, rightWidth-browserContentHorizontalChrome))
	m.detailView.SetHeight(max(browserDetailMinimumHeight, height-browserVerticalDetailChrome))
}

func browserHorizontalPaneWidths(totalWidth int) (int, int) {
	leftWidth := max(browserHorizontalTableMinWidth, totalWidth*browserHorizontalTablePercent/percentageScale)
	return leftWidth, totalWidth - leftWidth - browserHorizontalPaneGap
}

func (m *BrowserModel) rebuildDetailContent() {
	var parts []string

	if m.aiSummaryLoad {
		parts = append(parts, theme.SpinnerStyle.Render("AI analyzing..."))
		parts = append(parts, "")
	} else if m.aiSummaryErr != nil {
		parts = append(parts, theme.Error.Render(aiErr(m.aiSummaryErr)))
		parts = append(parts, "")
	} else if m.aiSummary != "" {
		header := theme.AIPrompt.Render("AI ") + theme.Accent.Render("SUMMARY")
		summary := lipgloss.NewStyle().Foreground(theme.LightText).Render(m.aiSummary)
		border := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).BorderForeground(theme.ElectricPurp).
			Padding(0, 1).Render(header + "\n" + summary)
		parts = append(parts, border)
		parts = append(parts, "")
	}

	body := m.detailContent
	if m.detailKind == "yaml" {
		body = service.HighlightYAML(body)
	}
	parts = append(parts, body)
	m.detailView.SetContent(strings.Join(parts, "\n"))
	m.detailView.GotoTop()
	m.statusMsg = theme.Dim.Render("esc: back | a: AI summary | c: copy | v: split")
}

func (m *BrowserModel) rebuildTable() {
	tableWidth := m.width - browserContentHorizontalChrome
	if tableWidth < browserMinimumResourceTableWidth {
		tableWidth = browserMinimumResourceTableWidth
	}

	specs, ok := selectColSpecs(m.resourceType, m.wide)
	if !ok {
		return
	}
	prevCursor := m.resourceTable.Cursor()
	m.resourceTable = buildResourceTable(tableWidth, specs)
	m.detailView.SetWidth(m.width - browserContentHorizontalChrome)
	m.refreshRows(prevCursor)
}

func (m *BrowserModel) refreshRows(cursorHint int) {
	rows := m.currentResourceRows()
	identities := m.currentResourceIdentities()
	rows, identities = filterRowsWithIdentities(rows, identities, m.filterText)
	m.visibleResources = identities
	m.resourceTable.SetRows(rows)

	if cursorHint >= len(rows) && len(rows) > 0 {
		cursorHint = len(rows) - 1
	}
	if len(rows) > 0 {
		m.resourceTable.SetCursor(cursorHint)
	}
	m.resourceTable.Focus()
}

func filterRowsWithIdentities(
	rows []table.Row,
	identities []resourceIdentity,
	filterText string,
) ([]table.Row, []resourceIdentity) {
	if filterText == "" {
		return rows, identities
	}
	filteredRows := make([]table.Row, 0, len(rows))
	filteredIdentities := make([]resourceIdentity, 0, len(rows))
	for index, row := range rows {
		if !rowMatchesFilter(row, filterText) {
			continue
		}
		filteredRows = append(filteredRows, row)
		if index < len(identities) {
			filteredIdentities = append(filteredIdentities, identities[index])
		}
	}
	return filteredRows, filteredIdentities
}

func rowMatchesFilter(row table.Row, filterText string) bool {
	lowerFilter := strings.ToLower(filterText)
	for _, cell := range row {
		if strings.Contains(strings.ToLower(cell), lowerFilter) {
			return true
		}
	}
	return false
}

func (m *BrowserModel) currentResourceRows() []table.Row {
	b, ok := resourceCatalog[m.resourceType]
	if !ok {
		return nil
	}
	if m.wide && b.WideRowsOf != nil {
		return b.WideRowsOf(m)
	}
	return b.RowsOf(m)
}

func (m *BrowserModel) currentResourceIdentities() []resourceIdentity {
	binding, ok := resourceCatalog[m.resourceType]
	if !ok || binding.IdentitiesOf == nil {
		return nil
	}
	identities := binding.IdentitiesOf(m)
	for index := range identities {
		if identities[index].Namespace == "" && identities[index].Kind != "node" {
			identities[index].Namespace = m.namespace
		}
	}
	return identities
}

func browserTableStyles() table.Styles {
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(theme.DimText).
		BorderBottom(true).
		Bold(true).
		Foreground(theme.NeonCyan)
	s.Selected = s.Selected.
		Foreground(theme.White).
		Background(theme.DeepViolet).
		Bold(true)
	return s
}

func (m BrowserModel) renderTitleBar() string {
	title := theme.Title.Render("KUBERNETES BROWSER")
	rightLabel := theme.Subtitle.Render("ns:" + m.namespace)
	if m.wide {
		rightLabel = theme.Accent.Render("WIDE ") + rightLabel
	}

	chromeWidth := lipgloss.Width(title) + titleBarSidePadding + lipgloss.Width(rightLabel) + titleBarSidePadding
	tabsWidth := m.width - chromeWidth
	if tabsWidth < browserTabStripMinWidth {
		tabsWidth = browserTabStripMinWidth
	}
	indicator := renderBrowserTabStrip(m.resourceType, tabsWidth)

	left := lipgloss.JoinHorizontal(lipgloss.Center, title, "  ", indicator)
	gap := max(1, m.width-lipgloss.Width(left)-lipgloss.Width(rightLabel)-titleBarSidePadding)
	bar := left + strings.Repeat(" ", gap) + rightLabel

	return lipgloss.NewStyle().MaxWidth(m.width).Background(theme.DarkerBg).Render(bar)
}

// renderBrowserTabStrip windows tabs around the active resource.
func renderBrowserTabStrip(active string, maxWidth int) string {
	if maxWidth < browserTabStripMinViable {
		return ""
	}
	rendered, widths, activeIdx := buildBrowserTabs(active)
	leftHint := theme.StatusBarItem.Render(" ‹ ")
	rightHint := theme.StatusBarItem.Render(" › ")

	start, end := windowAroundActive(activeIdx, widths, maxWidth, lipgloss.Width(leftHint), lipgloss.Width(rightHint))

	parts := make([]string, 0, end-start+browserTabHintCapacity)
	if start > 0 {
		parts = append(parts, leftHint)
	}
	parts = append(parts, rendered[start:end+1]...)
	if end < len(allResourceTypes)-1 {
		parts = append(parts, rightHint)
	}
	return lipgloss.JoinHorizontal(lipgloss.Center, parts...)
}

func buildBrowserTabs(active string) ([]string, []int, int) {
	rendered := make([]string, len(allResourceTypes))
	widths := make([]int, len(allResourceTypes))
	activeIdx := 0
	for i, rt := range allResourceTypes {
		label := " " + strings.ToUpper(rt) + " "
		if rt == active {
			rendered[i] = theme.StatusBarActive.Render(label)
			activeIdx = i
		} else {
			rendered[i] = theme.StatusBarItem.Render(label)
		}
		widths[i] = lipgloss.Width(rendered[i])
	}
	return rendered, widths, activeIdx
}

func windowAroundActive(activeIdx int, widths []int, maxWidth, leftHintW, rightHintW int) (start, end int) {
	start, end = activeIdx, activeIdx
	used := widths[activeIdx]
	for {
		extendedLeft := tryExtendLeft(&start, &used, widths, end, maxWidth, leftHintW)
		extendedRight := tryExtendRight(&end, &used, widths, start, maxWidth, rightHintW)
		if !extendedLeft && !extendedRight {
			return start, end
		}
	}
}

func tryExtendLeft(start, used *int, widths []int, end, maxWidth, leftHintW int) bool {
	if *start == 0 {
		return false
	}
	candidate := *start - 1
	need := widths[candidate]
	if candidate == 0 && end == len(widths)-1 {
		if *used+need > maxWidth {
			return false
		}
	} else if *used+need+leftHintW > maxWidth {
		return false
	}
	*start = candidate
	*used += need
	return true
}

func tryExtendRight(end, used *int, widths []int, start, maxWidth, rightHintW int) bool {
	if *end == len(widths)-1 {
		return false
	}
	candidate := *end + 1
	need := widths[candidate]
	if start == 0 && candidate == len(widths)-1 {
		if *used+need > maxWidth {
			return false
		}
	} else if *used+need+rightHintW > maxWidth {
		return false
	}
	*end = candidate
	*used += need
	return true
}

func (m BrowserModel) renderTableContent(height int) string {
	if m.loading && len(m.resourceTable.Rows()) == 0 {
		loadingMsg := m.spinner.View() + " " + theme.Dim.Render(
			fmt.Sprintf("Loading %s in %s...", m.resourceType, m.namespace))
		return lipgloss.Place(m.width, height, lipgloss.Center, lipgloss.Center, loadingMsg)
	}

	if len(m.resourceTable.Rows()) == 0 {
		emptyMsg := theme.Dim.Render("No "+m.resourceType+" found in "+m.namespace) + "\n\n" +
			theme.HelpKey.Render("[r]") + theme.HelpDesc.Render(" refresh  ") +
			theme.HelpKey.Render("[p]") + theme.HelpDesc.Render(" pods  ") +
			theme.HelpKey.Render("[d]") + theme.HelpDesc.Render(" deploys")
		return lipgloss.Place(m.width, height, lipgloss.Center, lipgloss.Center, emptyMsg)
	}

	tableView := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(theme.BorderColor).
		Padding(0, 1).Width(m.width - browserBoxChrome).Height(height - browserBoxChrome).
		Render(m.resourceTable.View())
	return tableView
}

func (m BrowserModel) renderSplitContent(height int) string {
	tableView := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(theme.BorderColor).Padding(0, 1).Width(m.width - browserBoxChrome).Render(m.resourceTable.View())

	detailTitle := theme.Accent.Render(" Detail ") + "  " +
		theme.HelpKey.Render("[v]") + theme.HelpDesc.Render(" toggle layout") +
		"  " + popupScrollIndicator(m.detailView)
	detailBody := m.detailView.View()
	detailBox := lipgloss.NewStyle().Border(lipgloss.ThickBorder()).BorderForeground(theme.NeonCyan).Padding(0, 1).
		Width(m.width - browserBoxChrome).MaxWidth(m.width).
		Render(lipgloss.JoinVertical(lipgloss.Left, detailTitle, detailBody))

	combined := lipgloss.JoinVertical(lipgloss.Left, tableView, detailBox)
	return lipgloss.Place(m.width, height, lipgloss.Left, lipgloss.Top, combined)
}

func (m BrowserModel) renderHSplitContent(height int) string {
	leftW, rightW := browserHorizontalPaneWidths(m.width)

	tableView := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(theme.BorderColor).Padding(0, 1).Width(leftW - browserBoxChrome).Height(height - browserBoxChrome).Render(m.resourceTable.View())

	detailTitle := theme.Accent.Render(" Detail ") + "  " +
		theme.HelpKey.Render("[v]") + theme.HelpDesc.Render(" toggle layout") +
		"  " + popupScrollIndicator(m.detailView)
	detailBody := m.detailView.View()
	detailBox := lipgloss.NewStyle().Border(lipgloss.ThickBorder()).BorderForeground(theme.NeonCyan).Padding(0, 1).
		Width(rightW - browserBoxChrome).
		Height(height - browserBoxChrome).
		MaxWidth(m.width).
		Render(lipgloss.JoinVertical(lipgloss.Left, detailTitle, detailBody))

	combined := lipgloss.JoinHorizontal(lipgloss.Top, tableView, detailBox)
	return lipgloss.Place(m.width, height, lipgloss.Left, lipgloss.Top, combined)
}

func (m BrowserModel) renderConfirmBox() string {
	label, warning, borderColor := confirmDialogStyle(m.confirmAction)

	header := label + " " + theme.Accent.Render(m.confirmTarget)

	rows := []string{header}
	if warning != "" {
		rows = append(rows, "", warning)
	}
	rows = append(rows,
		"",
		"Proceed?",
		"",
		lipgloss.JoinHorizontal(lipgloss.Center,
			theme.HelpKey.Render("y")+" "+theme.HelpDesc.Render("yes")+"   ",
			theme.HelpKey.Render("n")+" "+theme.HelpDesc.Render("no"),
		),
	)

	body := lipgloss.JoinVertical(lipgloss.Center, rows...)

	return lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(borderColor).
		Padding(1, confirmHorizontalPadding).
		Foreground(theme.White).
		Width(clampModalWidth(confirmModalDesiredWidth, m.width)).
		MaxWidth(m.width).
		Align(lipgloss.Center).
		Render(body)
}

func confirmDialogStyle(action string) (label, warning string, border color.Color) {
	switch action {
	case "delete":
		return theme.Error.Render("DELETE"),
			theme.Error.Render("⚠ IRREVERSIBLE — the resource will be destroyed"),
			theme.Red
	case "restart":
		return theme.Warning.Render("RESTART"),
			theme.Warning.Render("Rolling restart — brief service disruption"),
			theme.Orange
	default:
		return theme.Warning.Render("SCALE"),
			"",
			theme.Yellow
	}
}

func (m BrowserModel) renderScaleBox() string {
	title := theme.Accent.Render("Scale " + m.scaleName)
	var info string
	if m.scaleCurrentInfo != "" {
		info = theme.Dim.Render(m.scaleCurrentInfo)
	}

	parts := []string{title}
	if info != "" {
		parts = append(parts, info)
	}
	parts = append(parts, "", m.textInput.View(), "", theme.Dim.Render("enter: confirm | esc: cancel"))

	return lipgloss.NewStyle().Border(lipgloss.ThickBorder()).BorderForeground(theme.NeonCyan).Padding(0, 1).
		Width(clampModalWidth(scaleModalDesiredWidth, m.width)).MaxWidth(m.width).Render(
		lipgloss.JoinVertical(lipgloss.Left, parts...),
	)
}

func (m BrowserModel) lookupReplicaInfoFor(identity resourceIdentity) string {
	switch m.resourceType {
	case "deployments":
		for _, d := range m.deployments {
			if d.Name == identity.Name && namespacesMatch(identity.Namespace, d.Namespace) {
				return fmt.Sprintf("currently %s ready, %d up-to-date", d.Ready, d.UpToDate)
			}
		}
	case "statefulsets":
		for _, s := range m.statefulsets {
			if s.Name == identity.Name && namespacesMatch(identity.Namespace, s.Namespace) {
				return fmt.Sprintf("currently %s ready, %d replicas", s.Ready, s.Replicas)
			}
		}
	}
	return ""
}

func (m BrowserModel) renderErrBanner() string {
	if m.errBanner == "" {
		return ""
	}
	msg := "ERROR: " + m.errBanner + "   (esc dismiss · r retry)"
	return theme.ErrorBanner.Width(m.width).MaxWidth(m.width).Render(msg)
}

func (m BrowserModel) renderFilterBar() string {
	if m.state == stateFilter {
		matchCount := len(m.resourceTable.Rows())
		filterPrompt := theme.Accent.Render("Filter: ")
		matchInfo := theme.Dim.Render(fmt.Sprintf(" (%d matching)", matchCount))
		return lipgloss.NewStyle().
			Width(m.width).
			Background(theme.DarkerBg).
			Padding(0, 1).
			Render(filterPrompt + m.filterInput.View() + matchInfo)
	}
	if m.filterActive && m.filterText != "" {
		matchCount := len(m.resourceTable.Rows())
		badge := theme.FilterBadge.Render(fmt.Sprintf("FILTER: %s", m.filterText))
		matchInfo := theme.Dim.Render(fmt.Sprintf(" (%d matching)", matchCount))
		clearHint := "  " + theme.HelpKey.Render("/") + theme.HelpDesc.Render(": edit  ")
		return lipgloss.NewStyle().
			Width(m.width).
			Background(theme.DarkerBg).
			Padding(0, 1).
			Render(badge + matchInfo + clearHint)
	}
	return ""
}

func (m BrowserModel) renderStatusLine() string {
	if m.loading {
		return m.spinner.View() + " " + m.statusMsg
	}
	if n := len(m.selected); n > 0 {
		badge := theme.FilterBadge.Render(fmt.Sprintf("%d SELECTED", n))
		hint := theme.Dim.Render("  x: delete all · R: restart all · esc: clear")
		return badge + hint
	}
	if m.statusMsg != "" {
		return m.statusMsg
	}
	count := m.currentResourceCount()
	return theme.Dim.Render(fmt.Sprintf("%d %s in %s", count, m.resourceType, m.namespace))
}

func (m BrowserModel) currentResourceCount() int {
	if b, ok := resourceCatalog[m.resourceType]; ok {
		mc := m
		return b.Count(&mc)
	}
	return 0
}

func (m BrowserModel) renderHelpBar() string {
	return browserHelpBarStyle.Width(m.width).MaxWidth(m.width).Render(browserHelpBarText)
}

func formatEventsOutput(events []service.Event) string {
	if len(events) == 0 {
		return "No events found."
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%-8s %-20s %-30s %s\n", "TYPE", "REASON", "OBJECT", "MESSAGE")
	b.WriteString(strings.Repeat("-", eventSeparatorWidth) + "\n")
	for _, ev := range events {
		eventType := sanitizeTerminalText(ev.Type)
		reason := sanitizeTerminalText(ev.Reason)
		object := sanitizeTerminalText(ev.Object)
		msg := sanitizeTerminalText(ev.Message)
		if utf8.RuneCountInString(msg) > maxEventMessageRunes {
			messageRunes := []rune(msg)
			msg = string(messageRunes[:maxEventMessageRunes-len(eventMessageSuffix)]) + eventMessageSuffix
		}
		fmt.Fprintf(&b, "%-8s %-20s %-30s %s\n", eventType, reason, object, msg)
	}
	return b.String()
}

func clearStatusAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(_ time.Time) tea.Msg {
		return ClearStatusMsg{}
	})
}
