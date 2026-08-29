package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/session"
)

func freshRoot(t *testing.T) RootModel {
	t.Helper()
	m := newTestRootModel(t, "default")
	m.width = 200
	m.height = 50
	m.ready = true
	return m
}

func TestRootModel_Init_ReturnsCmd(t *testing.T) {
	if newTestRootModel(t, "ns").Init() == nil {
		t.Error("Init must return a non-nil cmd")
	}
}

func TestRootModel_RestoreSession_AppliesPersistedScreen(t *testing.T) {
	m := newTestRootModel(t, "ns")
	m.RestoreSession(session.SessionState{Screen: int(ScreenBrowser), ResourceType: "pods"})
	if m.screen != ScreenBrowser {
		t.Errorf("screen = %d, want ScreenBrowser", m.screen)
	}
}

func TestRootModel_RestoreSession_RejectsOutOfRange(t *testing.T) {
	m := newTestRootModel(t, "ns")
	m.screen = ScreenDashboard
	m.RestoreSession(session.SessionState{Screen: 99})
	if m.screen != ScreenDashboard {
		t.Errorf("out-of-range screen should not overwrite; got %d", m.screen)
	}
}

func TestRootModel_View_RendersContent(t *testing.T) {
	m := freshRoot(t)
	v := m.View()
	if v.Content == "" {
		t.Error("View should render non-empty content")
	}
	if !v.AltScreen {
		t.Error("View should request AltScreen")
	}
}

func TestRootModel_View_NotReadyShowsInitializing(t *testing.T) {
	m := newTestRootModel(t, "ns")
	m.width = 100
	m.height = 30
	v := m.View()
	if !strings.Contains(v.Content, "Initializing") {
		t.Errorf("not-ready view should show Initializing; got %q", v.Content)
	}
}

func TestRootModel_RenderStatusBar_ShowsActiveTab(t *testing.T) {
	m := freshRoot(t)
	bar := m.renderStatusBar()
	if !strings.Contains(stripAnsiForTest(bar), "Dashboard") {
		t.Errorf("status bar should mention current tab name; got %q", bar)
	}
}

func TestRootModel_HandleStatusBarClick_SwitchesToTab(t *testing.T) {
	m := freshRoot(t)
	model, _ := m.handleStatusBarClick(20)
	r := model.(RootModel)
	if r.screen != ScreenBrowser {
		t.Fatalf("clicking the Browser tab selected screen %d", r.screen)
	}
}

func TestRootModel_SwitchScreen_ChangesActive(t *testing.T) {
	m := freshRoot(t)
	model, _ := m.switchScreen(ScreenBrowser)
	r := model.(RootModel)
	if r.screen != ScreenBrowser {
		t.Errorf("switchScreen → %d, want ScreenBrowser", r.screen)
	}
}

func TestRootModel_SwitchScreen_SameScreenIsNoOp(t *testing.T) {
	m := freshRoot(t)
	m.screen = ScreenDashboard
	model, _ := m.switchScreen(ScreenDashboard)
	r := model.(RootModel)
	if r.screen != ScreenDashboard {
		t.Errorf("same-screen switch should leave state alone; got %d", r.screen)
	}
}

func TestRootModel_HandleMouse_StatusBarClickSwitchesScreen(t *testing.T) {
	m := freshRoot(t)
	x := lipgloss.Width(m.renderScreenTab(rootScreenTabs[0]))
	click := tea.MouseClickMsg{
		X: x, Y: m.height - 1, Button: tea.MouseLeft,
	}
	model, _ := m.handleMouse(click)
	if got := model.(RootModel).screen; got != ScreenBrowser {
		t.Fatalf("status click selected screen %d, want Browser", got)
	}
}

func TestRootModel_ShiftMouseX_TranslatesAllMouseTypes(t *testing.T) {
	cases := []tea.MouseMsg{
		tea.MouseClickMsg{X: 10, Button: tea.MouseLeft},
		tea.MouseReleaseMsg{X: 10, Button: tea.MouseLeft},
		tea.MouseWheelMsg{X: 10},
		tea.MouseMotionMsg{X: 10},
	}
	for _, c := range cases {
		got := shiftMouseX(c, -3)
		if mm, ok := got.(tea.MouseMsg); ok {
			if mm.Mouse().X != 7 {
				t.Errorf("%T: X = %d, want 7", c, mm.Mouse().X)
			}
		}
	}
}

func TestRootModel_FormatUptime_VariousDurations(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{5 * time.Second, "5s"},
		{90 * time.Second, "1m"},
		{3700 * time.Second, "1h"},
		{2 * 24 * time.Hour, "2d"},
	}
	for _, c := range cases {
		got := formatUptime(c.d)
		if got != c.want {
			t.Errorf("formatUptime(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestRootModel_TruncatePF_RespectsBudget(t *testing.T) {
	if got := truncatePF("short", 10); got != "short" {
		t.Errorf("short string should pass through; got %q", got)
	}
	if got := truncatePF("this-is-longer-than-the-budget", 10); len(got) > 10 {
		t.Errorf("truncated length = %d, want ≤ 10", len(got))
	}
}

func TestRootModel_ActiveScreenHasInputFocus_BrowserDelegates(t *testing.T) {
	m := freshRoot(t)
	m.screen = ScreenBrowser
	if m.activeScreenHasInputFocus() {
		t.Error("default browser should not have input focus")
	}
}

func TestRootModel_UpdateActiveScreen_RoutesToCurrent(t *testing.T) {
	m := freshRoot(t)
	m.screen = ScreenDashboard
	dummy := struct{}{}
	model, _ := m.updateActiveScreen(dummy)
	if _, ok := model.(RootModel); !ok {
		t.Errorf("updateActiveScreen should return a RootModel; got %T", model)
	}
}

func TestRootModel_ResizeChildren_PropagatesGeometry(t *testing.T) {
	m := freshRoot(t)
	m.resizeChildren()
	for name, size := range map[string][2]int{
		"dashboard": {m.dashboard.width, m.dashboard.height},
		"browser":   {m.browser.width, m.browser.height},
		"logs":      {m.logs.width, m.logs.height},
		"helm":      {m.helm.width, m.helm.height},
		"crds":      {m.crds.width, m.crds.height},
	} {
		if size[0] != m.width || size[1] <= 0 || size[1] >= m.height {
			t.Errorf("%s geometry = %dx%d", name, size[0], size[1])
		}
	}
}

func TestRootModel_DeactivateScreen_ClearsScreenActivity(t *testing.T) {
	m := freshRoot(t)
	m.dashboard.active = true
	m.browser.active = true
	m.logs.active = true
	m.analysisPanel.SetVisible(true)
	m.helm.loading = true
	m.crds.loading = true
	for _, s := range []screenID{ScreenDashboard, ScreenBrowser, ScreenLogs, ScreenAnalysis, ScreenHelm, ScreenCRDs} {
		m.deactivateScreen(s)
	}
	if m.dashboard.active || m.browser.active || m.logs.active || m.analysisPanel.IsVisible() || m.helm.loading || m.crds.loading {
		t.Fatalf("deactivation left active state: dashboard=%v browser=%v logs=%v analysis=%v helm=%v crds=%v",
			m.dashboard.active, m.browser.active, m.logs.active, m.analysisPanel.IsVisible(), m.helm.loading, m.crds.loading)
	}
}

func TestRootModel_RenderHelpOverlay_ContainsKeyHints(t *testing.T) {
	m := freshRoot(t)
	overlay := stripAnsiForTest(m.renderHelpOverlay(40))
	if !strings.Contains(overlay, "KEYBINDINGS") {
		t.Errorf("help overlay should have KEYBINDINGS title; got:\n%s", overlay)
	}
	if !strings.Contains(overlay, "Quit") {
		t.Error("help overlay should mention Quit")
	}
}

func TestRootModel_RenderHelpOverlay_ContextualPerScreen(t *testing.T) {
	for _, s := range []screenID{ScreenDashboard, ScreenBrowser, ScreenLogs, ScreenAnalysis, ScreenHelm, ScreenCRDs} {
		m := freshRoot(t)
		m.screen = s
		overlay := m.renderHelpOverlay(40)
		if overlay == "" {
			t.Errorf("help overlay for screen %d should not be empty", s)
		}
	}
}

func TestRootModel_OpenSearch_PopulatesCorpusAndCursor(t *testing.T) {
	m := freshRoot(t)
	cmd := m.openSearch()
	if cmd == nil {
		t.Error("openSearch should return a Blink cmd")
	}
}

func TestRootModel_RenderContent_HelpOverlay(t *testing.T) {
	m := freshRoot(t)
	m.showHelp = true
	out := stripAnsiForTest(m.renderContent())
	if !strings.Contains(out, "KEYBINDINGS") {
		t.Errorf("help overlay should render via renderContent; got:\n%s", out)
	}
}

func TestRootModel_RenderContent_NSPickerOverlay(t *testing.T) {
	m := freshRoot(t)
	m.showNSPicker = true
	m.namespaces = []string{"default", "kube-system"}
	out := m.renderContent()
	if out == "" {
		t.Error("ns picker overlay should render")
	}
}

func TestRootModel_RenderContent_CtxPickerOverlay(t *testing.T) {
	m := freshRoot(t)
	m.showCtxPicker = true
	m.contexts = []cluster.KubeContext{{Name: "ctx1"}}
	out := m.renderContent()
	if out == "" {
		t.Error("ctx picker overlay should render")
	}
}

func TestRootModel_RenderContent_SearchOverlay(t *testing.T) {
	m := freshRoot(t)
	m.showSearch = true
	out := m.renderContent()
	if out == "" {
		t.Error("search overlay should render")
	}
}

func TestRootModel_RenderContent_PFOverlay(t *testing.T) {
	m := freshRoot(t)
	m.showPFModal = true
	out := m.renderContent()
	if out == "" {
		t.Error("port-forward modal should render")
	}
}

func TestRootModel_HandleNSPicker_NavigatesAndSelects(t *testing.T) {
	m := freshRoot(t)
	m.showNSPicker = true
	m.namespaces = []string{"default", "kube-system"}
	m.nsCursor = 0

	model, _ := m.handleNSPicker("down")
	r := model.(RootModel)
	if r.nsCursor != 1 {
		t.Errorf("down should increment cursor; got %d", r.nsCursor)
	}

	model, _ = r.handleNSPicker("up")
	r = model.(RootModel)
	if r.nsCursor != 0 {
		t.Error("up should decrement cursor")
	}

	model, _ = r.handleNSPicker("esc")
	r = model.(RootModel)
	if r.showNSPicker {
		t.Error("esc should close picker")
	}
}

func TestRootModel_HandleCtxPicker_EscClosesPicker(t *testing.T) {
	m := freshRoot(t)
	m.showCtxPicker = true
	model, _ := m.handleCtxPicker("esc")
	r := model.(RootModel)
	if r.showCtxPicker {
		t.Error("esc should close ctx picker")
	}
}

func TestRootModel_OpenPFModal_TogglesVisibility(t *testing.T) {
	m := freshRoot(t)
	m.openPFModal()
	if !m.showPFModal {
		t.Error("openPFModal should set showPFModal=true")
	}
}

func TestRootModel_UpdateAnalysisScreenContext_UsesActiveScreenData(t *testing.T) {
	m := freshRoot(t)
	m.dashboard.pods = []cluster.Pod{{Name: "dashboard-api", Namespace: "default"}}
	m.screen = ScreenDashboard
	m.updateAnalysisScreenContext()
	if !strings.Contains(m.analysisPanel.screenContext, "dashboard-api") {
		t.Fatalf("dashboard context = %q", m.analysisPanel.screenContext)
	}

	m.browser.pods = []cluster.Pod{{Name: "browser-api", Namespace: "default"}}
	m.screen = ScreenBrowser
	m.updateAnalysisScreenContext()
	if !strings.Contains(m.analysisPanel.screenContext, "browser-api") {
		t.Fatalf("browser context = %q", m.analysisPanel.screenContext)
	}

	m.logs.selectedPod = "logs-api"
	m.logs.allLines = []string{"ready"}
	m.screen = ScreenLogs
	m.updateAnalysisScreenContext()
	if !strings.Contains(m.analysisPanel.screenContext, "logs-api") {
		t.Fatalf("logs context = %q", m.analysisPanel.screenContext)
	}
}

func TestRootModel_HandleSearch_EscClosesAndKeepsModel(t *testing.T) {
	m := freshRoot(t)
	m.showSearch = true
	model, _ := m.handleSearch("esc", tea.KeyPressMsg{Code: tea.KeyEsc, Text: "esc"})
	r := model.(RootModel)
	if r.showSearch {
		t.Error("esc should close search")
	}
}

func TestRootModel_HandleCmdPalette_EscClosesIt(t *testing.T) {
	m := freshRoot(t)
	m.showCmdPalette = true
	model, _ := m.handleCmdPalette("esc", tea.KeyPressMsg{Code: tea.KeyEsc, Text: "esc"})
	r := model.(RootModel)
	if r.showCmdPalette {
		t.Error("esc should close cmd palette")
	}
}

func TestRootModel_HandleDrillDown_RoutesToTargetScreen(t *testing.T) {
	m := freshRoot(t)
	model, _ := m.handleDrillDown(DrillDownMsg{Screen: ScreenBrowser, ResourceType: "pods", ResourceName: "p1", ResourceNS: "ns"})
	r := model.(RootModel)
	if r.screen != ScreenBrowser {
		t.Errorf("drill-down to Browser should switch screen; got %d", r.screen)
	}
}
