package ui

import (
	"fmt"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"

	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/kube"
	"github.com/HediAbed/opsmate/internal/session"
	"github.com/HediAbed/opsmate/internal/theme"
)

type sessionStateSaver func(session.SessionState) error

type screenID uint8

const (
	ScreenDashboard screenID = iota
	ScreenBrowser
	ScreenLogs
	ScreenAnalysis
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

const (
	clusterReadTimeout   = 15 * time.Second
	clusterActionTimeout = 30 * time.Second
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
	{key: "4", name: "Analysis", id: ScreenAnalysis},
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
	width      int
	height     int
	screen     screenID
	namespace  string
	runtime    RuntimeDependencies
	operations clusterOperations

	dashboard     DashboardModel
	browser       BrowserModel
	logs          LogsModel
	helm          HelmModel
	crds          CRDsModel
	analysisPanel AnalysisPanelModel

	namespaces   []string
	showNSPicker bool
	showHelp     bool
	nsCursor     int
	nsSpinner    spinner.Model
	nsLoading    bool

	contexts         []cluster.KubeContext
	showCtxPicker    bool
	ctxCursor        int
	ctxLoading       bool
	currentContext   string
	contextSwitching bool

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
	pfSessions      []kube.PortForwardSession
	pfConfirmKillID string
	pfConfirmKillOf string

	ready            bool
	initialized      bool
	err              error
	saveSessionState sessionStateSaver
}

type searchResult struct {
	Kind      string
	Name      string
	Namespace string
}

func NewRootModel(namespace string, runtime RuntimeDependencies) (RootModel, error) {
	if err := runtime.validate(); err != nil {
		return RootModel{}, err
	}
	commands := newNativeClusterCommands(runtime.Context, runtime.ClusterResources, runtime.ClusterObserver)
	operations := newNativeClusterOperations(
		runtime.Context,
		runtime.ClusterOperations,
		runtime.ClusterOperations,
		runtime.ClusterOperations,
		runtime.ClusterOperations,
	)
	helm := newNativeHelmCommands(runtime.Context, runtime.HelmReleases)
	clusterAnalysis := newNativeClusterAnalyzer(runtime.Context, runtime.ClusterSnapshots)
	namespaceSpinner := spinner.New()
	namespaceSpinner.Spinner = spinner.Dot
	namespaceSpinner.Style = theme.SpinnerStyle

	analysisPanel := NewAnalysisPanelModel()
	analysisPanel.analyzeCluster = clusterAnalysis.Analyze
	analysisPanel.SetNamespace(namespace)

	commandInput := newTextInput(textInputOptions{
		Prompt:      ":",
		Placeholder: "pod, deploy, svc, ns <name>, logs <pod>, q",
		CharLimit:   rootTextInputCharacterLimit,
		Width:       rootTextInputInitialWidth,
		PromptStyle: theme.Accent,
		TextStyle:   lipgloss.NewStyle().Foreground(theme.White),
	})

	searchInput := newTextInput(textInputOptions{
		Prompt:      "find: ",
		Placeholder: "pod/deploy/svc name...",
		CharLimit:   rootTextInputCharacterLimit,
		Width:       rootTextInputInitialWidth,
		PromptStyle: theme.Accent,
		TextStyle:   lipgloss.NewStyle().Foreground(theme.White),
	})

	return RootModel{
		screen:           ScreenDashboard,
		namespace:        namespace,
		runtime:          runtime,
		operations:       operations,
		dashboard:        NewDashboardModel(namespace, commands),
		browser:          NewBrowserModel(namespace, commands, operations),
		logs:             NewLogsModel(namespace, commands, operations),
		helm:             NewHelmModel(namespace, helm),
		crds:             NewCRDsModel(namespace, commands),
		analysisPanel:    analysisPanel,
		nsSpinner:        namespaceSpinner,
		cmdInput:         commandInput,
		searchInput:      searchInput,
		saveSessionState: session.SaveSession,
	}, nil
}

func (m *RootModel) RestoreSession(s session.SessionState) {
	if restoredScreen, ok := screenIDFromPersisted(s.Screen); ok {
		m.screen = restoredScreen
	}
	if _, supported := resourceCatalog[s.ResourceType]; supported {
		m.browser.resourceType = s.ResourceType
	}
	m.browser.SetWide(s.Wide)
}

func (m RootModel) saveSession() error {
	state := session.SessionState{
		Namespace:    m.namespace,
		Screen:       int(m.screen),
		ResourceType: m.browser.ResourceType(),
		Wide:         m.browser.Wide(),
	}
	return m.saveSessionState(state)
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
