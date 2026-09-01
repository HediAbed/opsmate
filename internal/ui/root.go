package ui

import (
	"fmt"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"

	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/kube"
	"github.com/HediAbed/opsmate/internal/session"
	"github.com/HediAbed/opsmate/internal/ui/analysispanel"
	"github.com/HediAbed/opsmate/internal/ui/component"
	"github.com/HediAbed/opsmate/internal/ui/screen"
	browserscreen "github.com/HediAbed/opsmate/internal/ui/screen/browser"
	crdsscreen "github.com/HediAbed/opsmate/internal/ui/screen/crds"
	dashboardscreen "github.com/HediAbed/opsmate/internal/ui/screen/dashboard"
	helmscreen "github.com/HediAbed/opsmate/internal/ui/screen/helm"
	logscreen "github.com/HediAbed/opsmate/internal/ui/screen/logs"
	"github.com/HediAbed/opsmate/internal/ui/theme"
)

type sessionStateSaver func(session.SessionState) error

type screenID = screen.ID

const (
	ScreenDashboard = screen.Dashboard
	ScreenBrowser   = screen.Browser
	ScreenLogs      = screen.Logs
	ScreenAnalysis  = screen.Analysis
	ScreenHelm      = screen.Helm
	ScreenCRDs      = screen.CRDs
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
	{key: "4", name: "Analysis", id: ScreenAnalysis},
	{key: "5", name: "Helm", id: ScreenHelm},
	{key: "6", name: "CRDs", id: ScreenCRDs},
}

type GoBackMsg = screen.GoBackMsg

type ClearStatusMsg = screen.ClearStatusMsg

type initializeRootMsg struct{}

type DrillDownMsg = screen.DrillDownMsg

type RootModel struct {
	width      int
	height     int
	screen     screenID
	namespace  string
	runtime    RuntimeDependencies
	operations clusterOperations

	dashboard     dashboardscreen.DashboardModel
	browser       browserscreen.BrowserModel
	logs          logscreen.LogsModel
	helm          helmscreen.HelmModel
	crds          crdsscreen.CRDsModel
	analysisPanel analysispanel.AnalysisPanelModel

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
	searchResults []screen.SearchItem
	searchCorpus  []screen.SearchItem

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

func NewRootModel(namespace string, runtime RuntimeDependencies) (RootModel, error) {
	if err := runtime.validate(); err != nil {
		return RootModel{}, err
	}
	commands := newNativeClusterCommands(runtime.Context, runtime.ClusterResources, runtime.ClusterObserver)
	operations := newRootClusterOperations(runtime)
	helm := newNativeHelmCommands(runtime.Context, runtime.HelmReleases)
	analysisPanel := newRootAnalysisPanel(namespace, runtime)
	namespaceSpinner := spinner.New()
	namespaceSpinner.Spinner = spinner.Dot
	namespaceSpinner.Style = theme.SpinnerStyle

	return RootModel{
		screen:           ScreenDashboard,
		namespace:        namespace,
		runtime:          runtime,
		operations:       operations,
		dashboard:        dashboardscreen.NewWithAnalysis(namespace, commands, runtime.Analysis),
		browser:          browserscreen.NewWithAnalysis(namespace, commands, operations, runtime.Analysis),
		logs:             logscreen.NewWithAnalysis(namespace, commands, operations, runtime.Analysis),
		helm:             helmscreen.NewHelmModel(namespace, helm),
		crds:             crdsscreen.NewCRDsModel(namespace, commands),
		analysisPanel:    analysisPanel,
		nsSpinner:        namespaceSpinner,
		cmdInput:         newRootInput(":", "pod, deploy, svc, ns <name>, logs <pod>, q"),
		searchInput:      newRootInput("find: ", "pod/deploy/svc name..."),
		saveSessionState: session.SaveSession,
	}, nil
}

func newRootClusterOperations(runtime RuntimeDependencies) clusterOperations {
	return newNativeClusterOperations(
		runtime.Context,
		runtime.ClusterOperations,
		runtime.ClusterOperations,
		runtime.ClusterOperations,
		runtime.ClusterOperations,
	)
}

func newRootAnalysisPanel(namespace string, runtime RuntimeDependencies) analysispanel.AnalysisPanelModel {
	panel := analysispanel.NewWithService(runtime.Analysis)
	clusterAnalysis := newNativeClusterAnalyzer(runtime.Context, runtime.ClusterSnapshots, runtime.Analysis)
	panel.SetClusterAnalyzer(clusterAnalysis.Analyze)
	panel.SetNamespace(namespace)
	return panel
}

func newRootInput(prompt, placeholder string) textinput.Model {
	return component.NewTextInput(component.TextInputOptions{
		Prompt:      prompt,
		Placeholder: placeholder,
		CharLimit:   rootTextInputCharacterLimit,
		Width:       rootTextInputInitialWidth,
		PromptStyle: theme.Accent,
		TextStyle:   lipgloss.NewStyle().Foreground(theme.White),
	})
}

func (m *RootModel) RestoreSession(s session.SessionState) {
	if restoredScreen, ok := screenIDFromPersisted(s.Screen); ok {
		m.screen = restoredScreen
	}
	if resourceType := browserscreen.ResourceTypeForKind(s.ResourceType); resourceType != "" {
		m.browser.SetResourceType(resourceType)
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
