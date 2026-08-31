package ui

import (
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/HediAbed/opsmate/internal/analysis"
	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/kube"
	"github.com/HediAbed/opsmate/internal/ui/theme"
)

var allResourceTypes = []string{resourceTypePods, resourceTypeDeployments, resourceTypeServices, resourceTypeStatefulSets, resourceTypeDaemonSets, resourceTypeConfigMaps, resourceTypeNodes, resourceTypeJobs, resourceTypeIngresses, resourceTypeNetworkPolicies, resourceTypePVCs, resourceTypeCronJobs, resourceTypeHPAs, resourceTypeSecrets, resourceTypeReplicaSets, resourceTypeRBAC}

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
	for _, pair := range pairs {
		parts = append(parts, theme.HelpKey.Render(pair.key)+theme.HelpDesc.Render(":"+pair.desc))
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
	browserPanelGutter               = 2
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
	cluster      clusterCommands
	operations   clusterOperations
	analysis     analysis.Service

	pods            []cluster.Pod
	deployments     []cluster.Deployment
	services        []cluster.Service
	statefulsets    []cluster.StatefulSet
	daemonsets      []cluster.DaemonSet
	configmaps      []cluster.ConfigMap
	nodes           []cluster.Node
	jobs            []cluster.Job
	ingresses       []cluster.Ingress
	networkpolicies []cluster.NetworkPolicy
	pvcs            []cluster.PersistentVolumeClaim
	cronjobs        []cluster.CronJob
	hpas            []cluster.HPA
	secrets         []cluster.Secret
	replicasets     []cluster.ReplicaSet
	rbac            []cluster.RBAC

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
	scaleReplicas    int32

	splitHorizontal bool

	wide bool

	detailKind string

	selected           map[string]bool
	selectedIdentities map[string]resourceIdentity
	visibleResources   []resourceIdentity
	confirmIdentity    resourceIdentity
	scaleIdentity      resourceIdentity

	analysisSummary        string
	analysisSummaryLoading bool
	analysisSummaryErr     error

	podLive                     liveSupervisor[cluster.Pod]
	deploymentLive              liveSupervisor[cluster.Deployment]
	ingressLive                 liveSupervisor[cluster.Ingress]
	networkPolicyLive           liveSupervisor[cluster.NetworkPolicy]
	persistentVolumeClaimLive   liveSupervisor[cluster.PersistentVolumeClaim]
	cronJobLive                 liveSupervisor[cluster.CronJob]
	horizontalPodAutoscalerLive liveSupervisor[cluster.HPA]
	secretLive                  liveSupervisor[cluster.Secret]
	replicaSetLive              liveSupervisor[cluster.ReplicaSet]

	active bool

	state browserState
	err   error

	statusMsg string
	errBanner string

	shellSession kube.ShellSession
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

func NewBrowserModel(namespace string, commands clusterCommands, operations clusterOperations) BrowserModel {
	return newBrowserWithAnalysis(namespace, commands, operations, analysis.NewService(nil))
}

func newBrowserWithAnalysis(
	namespace string,
	commands clusterCommands,
	operations clusterOperations,
	analysisService analysis.Service,
) BrowserModel {
	loadingSpinner := spinner.New(
		spinner.WithSpinner(spinner.Dot),
		spinner.WithStyle(theme.SpinnerStyle),
	)

	scaleInput := newTextInput(textInputOptions{
		Prompt:      "Replicas: ",
		Placeholder: "replica count",
		CharLimit:   scaleInputCharacterLimit,
		Width:       scaleInputInitialWidth,
		PromptStyle: lipgloss.NewStyle().Foreground(theme.NeonCyan).Bold(true),
		TextStyle:   lipgloss.NewStyle().Foreground(theme.White),
	})

	filterInput := newTextInput(textInputOptions{
		Prompt:      "/ ",
		Placeholder: "type to filter...",
		CharLimit:   filterInputCharacterLimit,
		Width:       filterInputInitialWidth,
		PromptStyle: lipgloss.NewStyle().Foreground(theme.NeonCyan).Bold(true),
		TextStyle:   lipgloss.NewStyle().Foreground(theme.White),
	})

	resourceTable := buildResourceTable(initialScreenWidth, resourceColSpecs[resourceTypePods])
	detailView := newViewport(initialScreenWidth, initialViewportHeight)

	return BrowserModel{
		namespace:     namespace,
		resourceType:  resourceTypePods,
		cluster:       commands,
		operations:    operations,
		analysis:      analysisService,
		resourceTable: resourceTable,
		detailView:    detailView,
		spinner:       loadingSpinner,
		textInput:     scaleInput,
		filterInput:   filterInput,
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

	case supervisedLiveMsg:
		return m.handleSupervisedLiveMessage(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.recalcLayout()
		return m, nil
	default:
		return m.updateBrowserLiveMessage(msg)
	}
}

func (m BrowserModel) updateBrowserLifecycleMessage(msg tea.Msg) (BrowserModel, tea.Cmd) {
	switch msg := msg.(type) {
	case shellOutputMsg:
		return m.handleShellOutput(msg)

	case shellExitMsg:
		return m.handleShellExit(msg)

	case shellOutputClosedMsg:
		return m, nil

	default:
		return m.updateWorkloadFetchMessage(msg)
	}
}

func (m BrowserModel) updateWorkloadFetchMessage(msg tea.Msg) (BrowserModel, tea.Cmd) {
	switch msg := msg.(type) {
	case cluster.PodsMsg:
		return applyBrowserFetch(&m, resourceTypePods, &m.pods, msg.Pods, msg.Err)

	case cluster.DeploymentsMsg:
		return applyBrowserFetch(&m, resourceTypeDeployments, &m.deployments, msg.Deployments, msg.Err)

	case cluster.StatefulSetsMsg:
		return applyBrowserFetch(&m, resourceTypeStatefulSets, &m.statefulsets, msg.StatefulSets, msg.Err)

	case cluster.DaemonSetsMsg:
		return applyBrowserFetch(&m, resourceTypeDaemonSets, &m.daemonsets, msg.DaemonSets, msg.Err)
	default:
		return m.updateCoreBrowserFetchMessage(msg)
	}
}

func (m BrowserModel) updateCoreBrowserFetchMessage(msg tea.Msg) (BrowserModel, tea.Cmd) {
	switch msg := msg.(type) {
	case cluster.ServicesMsg:
		return applyBrowserFetch(&m, resourceTypeServices, &m.services, msg.Services, msg.Err)

	case cluster.ConfigMapsMsg:
		return applyBrowserFetch(&m, resourceTypeConfigMaps, &m.configmaps, msg.ConfigMaps, msg.Err)

	case cluster.NodesMsg:
		return applyBrowserFetch(&m, resourceTypeNodes, &m.nodes, msg.Nodes, msg.Err)

	case cluster.JobsMsg:
		return applyBrowserFetch(&m, resourceTypeJobs, &m.jobs, msg.Jobs, msg.Err)
	default:
		return m.updateNetworkStorageFetchMessage(msg)
	}
}

func (m BrowserModel) updateNetworkStorageFetchMessage(msg tea.Msg) (BrowserModel, tea.Cmd) {
	switch msg := msg.(type) {
	case cluster.IngressesMsg:
		return applyBrowserFetch(&m, resourceTypeIngresses, &m.ingresses, msg.Ingresses, msg.Err)

	case cluster.NetworkPoliciesMsg:
		return applyBrowserFetch(&m, resourceTypeNetworkPolicies, &m.networkpolicies, msg.NetworkPolicies, msg.Err)

	case cluster.PVCsMsg:
		return applyBrowserFetch(&m, resourceTypePVCs, &m.pvcs, msg.PVCs, msg.Err)
	default:
		return m.updateScheduledSecurityFetchMessage(msg)
	}
}

func (m BrowserModel) updateScheduledSecurityFetchMessage(msg tea.Msg) (BrowserModel, tea.Cmd) {
	switch msg := msg.(type) {
	case cluster.CronJobsMsg:
		return applyBrowserFetch(&m, resourceTypeCronJobs, &m.cronjobs, msg.CronJobs, msg.Err)

	case cluster.HPAsMsg:
		return applyBrowserFetch(&m, resourceTypeHPAs, &m.hpas, msg.HPAs, msg.Err)

	case cluster.SecretsMsg:
		return applyBrowserFetch(&m, resourceTypeSecrets, &m.secrets, msg.Secrets, msg.Err)

	case cluster.ReplicaSetsMsg:
		return applyBrowserFetch(&m, resourceTypeReplicaSets, &m.replicasets, msg.ReplicaSets, msg.Err)

	case cluster.RBACMsg:
		return applyBrowserFetch(&m, resourceTypeRBAC, &m.rbac, msg.RBAC, msg.Err)
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
	case cluster.DescribeMsg:
		m.handleDescribeResult(msg)
		return m, nil

	case analysis.DescribeSummaryMsg:
		m.handleDescribeSummaryResult(msg)
		return m, nil

	case cluster.EventsMsg:
		m.handleEventsResult(msg)
		return m, nil

	case cluster.YAMLMsg:
		m.handleYAMLResult(msg)
		return m, nil

	case cluster.MutationResultMsg:
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
