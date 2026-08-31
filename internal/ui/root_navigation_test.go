package ui

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/kube"
	"github.com/HediAbed/opsmate/internal/session"
)

func TestRootModel_SaveOnExit_PersistsCurrentState(t *testing.T) {
	m := freshRoot(t)
	m.screen = ScreenLogs
	m.namespace = "payments"
	var state session.SessionState
	m.saveSessionState = func(saved session.SessionState) error {
		state = saved
		return nil
	}
	if err := m.SaveOnExit(); err != nil {
		t.Fatalf("SaveOnExit returned an error: %v", err)
	}
	if state.Screen != int(ScreenLogs) || state.Namespace != "payments" {
		t.Fatalf("saved state = %+v", state)
	}
}

func TestRootModel_Update_WindowSize(t *testing.T) {
	m := freshRoot(t)
	model, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	r := model.(RootModel)
	if r.width != 100 || r.height != 30 {
		t.Errorf("WindowSize not applied; got width=%d height=%d", r.width, r.height)
	}
}

func TestRootModel_Update_Quit(t *testing.T) {
	m := freshRoot(t)
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Error("q should produce a Quit cmd")
	}
}

func TestRootModel_Update_ScreenSwitchKeys(t *testing.T) {
	for _, key := range []rune{'1', '2', '3', '4', '5', '6', '7'} {
		m := freshRoot(t)
		model, _ := m.Update(tea.KeyPressMsg{Code: key, Text: string(key)})
		_ = model
	}
}

func TestRootModel_Update_HelpToggle(t *testing.T) {
	m := freshRoot(t)
	model, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	r := model.(RootModel)
	if !r.showHelp {
		t.Error("? should toggle help on")
	}
}

func TestRootModel_ExecutePaletteCommand_QuitReturnsQuitCmd(t *testing.T) {
	m := freshRoot(t)
	_, cmd := m.executePaletteCommand("q")
	if cmd == nil {
		t.Error("q palette command should return a quit cmd")
	}
}

func TestRootModel_ExecutePaletteCommand_ResourceAliasSwitchesScreen(t *testing.T) {
	m := freshRoot(t)
	model, _ := m.executePaletteCommand("pod")
	r := model.(RootModel)
	if r.screen != ScreenBrowser {
		t.Errorf("'pod' alias should switch to ScreenBrowser; got %d", r.screen)
	}
	if r.browser.ResourceType() != "pods" {
		t.Errorf("'pod' alias should set browser to pods; got %q", r.browser.ResourceType())
	}
}

func TestRootModel_ExecutePaletteCommand_NamespaceSwitches(t *testing.T) {
	m := freshRoot(t)
	model, _ := m.executePaletteCommand("ns kube-system")
	r := model.(RootModel)
	if r.namespace != "kube-system" {
		t.Errorf("ns command should switch namespace; got %q", r.namespace)
	}
}

func TestRootModel_ExecutePaletteCommand_LogsDrillsDown(t *testing.T) {
	m := freshRoot(t)
	_, cmd := m.executePaletteCommand("logs my-pod")
	if cmd == nil {
		t.Fatal("logs palette should return a DrillDownMsg cmd")
	}
	msg := cmd()
	if d, ok := msg.(DrillDownMsg); !ok || d.Screen != ScreenLogs {
		t.Errorf("expected DrillDownMsg{Screen: ScreenLogs}; got %T %+v", msg, msg)
	}
}

func TestRootModel_ExecutePaletteCommand_EmptyIsNoOp(t *testing.T) {
	m := freshRoot(t)
	model, cmd := m.executePaletteCommand("")
	r := model.(RootModel)
	if cmd != nil {
		t.Error("empty palette command must be a no-op")
	}
	if r.screen != m.screen {
		t.Error("empty command should not change screen")
	}
}

func TestRootModel_ExecutePaletteCommand_UnknownIsNoOp(t *testing.T) {
	m := freshRoot(t)
	model, cmd := m.executePaletteCommand("bogus")
	r := model.(RootModel)
	if cmd != nil || r.screen != m.screen {
		t.Error("unknown palette command must be a no-op")
	}
}

func TestRootModel_StartPortForwardFromPalette_RequiresArgs(t *testing.T) {
	m := freshRoot(t)
	_, cmd := m.startPortForwardFromPalette([]string{})
	if cmd == nil {
		t.Fatal("missing args should still return a feedback cmd")
	}
	msg := cmd()
	if pf, ok := msg.(PortForwardFeedbackMsg); !ok || pf.Err == nil {
		t.Errorf("expected error feedback; got %+v", msg)
	}
}

func TestRootModel_SwitchNamespace_TouchesEveryChild(t *testing.T) {
	m := freshRoot(t)
	m.namespace = "test"
	cmd := m.switchNamespace()
	if cmd == nil {
		t.Error("switchNamespace should return a batch cmd of dashboard+helm+crds+network refetches")
	}
	if m.browserInited || m.logsInited || m.helmInited || m.crdsInited {
		t.Error("switchNamespace must invalidate every per-screen inited flag")
	}
}

func TestRootModel_HandleNSPickerMouse_WheelScrollsCursor(t *testing.T) {
	m := freshRoot(t)
	m.showNSPicker = true
	m.namespaces = []string{"a", "b", "c"}
	model, command := m.handleNSPickerMouse(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	updated := model.(RootModel)
	if command != nil || updated.nsCursor != 1 {
		t.Fatalf("wheel result = cursor %d, command %v", updated.nsCursor, command)
	}
}

func TestRootModel_Update_TabTogglesAnalysisPanelOn(t *testing.T) {
	m := freshRoot(t)
	model, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Text: "tab"})
	r := model.(RootModel)
	if !r.analysisPanel.IsVisible() {
		t.Error("tab should toggle analysis panel on")
	}
}

func TestRootModel_Update_NOpensNSPicker(t *testing.T) {
	m := freshRoot(t)
	m.namespaces = []string{"default"}
	model, _ := m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	r := model.(RootModel)
	if !r.showNSPicker {
		t.Error("n should open NS picker")
	}
}

func TestRootModel_Update_KOpensCtxPicker(t *testing.T) {
	m := freshRoot(t)
	model, _ := m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	r := model.(RootModel)
	if !r.showCtxPicker {
		t.Error("k should open ctx picker")
	}
}

func TestRootModel_Update_ColonOpensCmdPalette(t *testing.T) {
	m := freshRoot(t)
	model, _ := m.Update(tea.KeyPressMsg{Code: ':', Text: ":"})
	r := model.(RootModel)
	if !r.showCmdPalette {
		t.Error(": should open command palette")
	}
}

func TestRootModel_Update_QuestionOpensHelp(t *testing.T) {
	m := freshRoot(t)
	model, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	r := model.(RootModel)
	if !r.showHelp {
		t.Error("? should open help")
	}
}

func TestRootModel_Update_HelpVisibleEscClosesIt(t *testing.T) {
	m := freshRoot(t)
	m.showHelp = true
	model, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc, Text: "esc"})
	r := model.(RootModel)
	if r.showHelp {
		t.Error("esc should close help when visible")
	}
}

func TestRootModel_Update_NSPickerKeyForwarded(t *testing.T) {
	m := freshRoot(t)
	m.showNSPicker = true
	m.namespaces = []string{"default"}
	model, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown, Text: "down"})
	_ = model
}

func TestRootModel_Update_AnalysisVisibleEscHidesIt(t *testing.T) {
	m := freshRoot(t)
	m.analysisPanel.SetVisible(true)
	model, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc, Text: "esc"})
	r := model.(RootModel)
	if r.analysisPanel.IsVisible() {
		t.Error("esc should hide analysis panel")
	}
}

func TestRootHandleCmdPalette_EnterRunsCommand(t *testing.T) {
	m := freshRoot(t)
	m.showCmdPalette = true
	m.cmdInput.SetValue("q")
	model, cmd := m.handleCmdPalette("enter", tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	r := model.(RootModel)
	if r.showCmdPalette {
		t.Error("enter should close palette")
	}
	if cmd == nil {
		t.Error("q palette → quit cmd")
	}
}

func TestRootHandleCmdPalette_DefaultForwardsToInput(t *testing.T) {
	m := freshRoot(t)
	m.showCmdPalette = true
	model, _ := m.handleCmdPalette("a", tea.KeyPressMsg{Code: 'a', Text: "a"})
	r := model.(RootModel)
	if !r.showCmdPalette {
		t.Error("typing should not close the palette")
	}
}

func TestRootExecutePaletteCommand_NSWithoutArgOpensPicker(t *testing.T) {
	m := freshRoot(t)
	m.namespaces = []string{"default"}
	model, _ := m.executePaletteCommand("ns")
	r := model.(RootModel)
	if !r.showNSPicker {
		t.Error(":ns without arg should open picker")
	}
}

func TestRootExecutePaletteCommand_NSWithoutNamespacesFetches(t *testing.T) {
	m := freshRoot(t)
	m.namespaces = nil
	_, cmd := m.executePaletteCommand("ns")
	if cmd == nil {
		t.Error(":ns with empty cache should return fetch cmd")
	}
}

func TestRootExecutePaletteCommand_AllResourceAliases(t *testing.T) {
	aliases := map[string]string{
		"pod": "pods", "pods": "pods", "deploy": "deployments", "dep": "deployments",
		"svc": "services", "sts": "statefulsets", "ds": "daemonsets", "cm": "configmaps",
		"node": "nodes", "nodes": "nodes", "job": "jobs", "jobs": "jobs",
	}
	for alias, want := range aliases {
		m := freshRoot(t)
		model, _ := m.executePaletteCommand(alias)
		r := model.(RootModel)
		if r.browser.ResourceType() != want {
			t.Errorf("alias %q should map to %q; got %q", alias, want, r.browser.ResourceType())
		}
	}
}

func TestRootStartPortForwardFromPalette_ValidArgs(t *testing.T) {
	m := freshRoot(t)
	_, cmd := m.startPortForwardFromPalette([]string{"my-pod", "8080:80"})
	if cmd == nil {
		t.Error("valid pf args should return a cmd")
	}
}

func TestRootUpdate_NamespacesMsg_PopulatesOrErr(t *testing.T) {
	m := freshRoot(t)
	model, _ := m.Update(cluster.NamespacesMsg{Namespaces: []string{"default"}})
	r := model.(RootModel)
	if len(r.namespaces) != 1 {
		t.Errorf("namespaces should populate; got %d", len(r.namespaces))
	}

	model, _ = m.Update(cluster.NamespacesMsg{Err: errStub("denied")})
	r = model.(RootModel)
	if r.err == nil {
		t.Error("err should propagate")
	}
}

func TestRootUpdate_CurrentContextMsg(t *testing.T) {
	m := freshRoot(t)
	model, _ := m.Update(cluster.CurrentContextMsg{Name: "my-cluster"})
	r := model.(RootModel)
	if r.currentContext != "my-cluster" {
		t.Errorf("currentContext = %q", r.currentContext)
	}
}

func TestRootUpdate_ContextsMsg_PicksCurrentMarker(t *testing.T) {
	m := freshRoot(t)
	model, _ := m.Update(cluster.ContextsMsg{Contexts: []cluster.KubeContext{
		{Name: "a"},
		{Name: "b", Current: true},
	}})
	r := model.(RootModel)
	if r.currentContext != "b" {
		t.Errorf("currentContext from list = %q, want b", r.currentContext)
	}
}

func TestRootUpdate_PortForwardStartedMsg_ReshresPFList(t *testing.T) {
	m := freshRoot(t)
	model, _ := m.Update(portForwardStartedMsg{Err: errStub("port in use")})
	r := model.(RootModel)
	if r.err == nil {
		t.Error("err should propagate")
	}
}

func TestRootUpdate_PortForwardStoppedMsg_ResetsPFCursor(t *testing.T) {
	m := freshRoot(t)
	m.pfCursor = 99
	model, _ := m.Update(portForwardStoppedMsg{SessionID: "x"})
	_ = model
}

func TestRootUpdate_PortForwardFeedbackMsg_PropagatesErr(t *testing.T) {
	m := freshRoot(t)
	model, _ := m.Update(PortForwardFeedbackMsg{Err: errStub("bind failed")})
	r := model.(RootModel)
	if r.err == nil {
		t.Error("err should propagate")
	}
}

func TestRootUpdate_ContextSwitchedMsg_ResetsState(t *testing.T) {
	m := freshRoot(t)
	m.contextSwitching = true
	model, cmd := m.Update(cluster.ContextSwitchedMsg{Name: "new-ctx"})
	r := model.(RootModel)
	if r.currentContext != "new-ctx" {
		t.Error("ctx switch should set currentContext")
	}
	if r.namespace != "" {
		t.Errorf("ns should reset to all-namespaces (empty); got %q", r.namespace)
	}
	if cmd == nil {
		t.Error("ctx switch should return batch cmd")
	}
	if r.contextSwitching {
		t.Error("completed context switch retained the switching state")
	}
}

func TestRootUpdate_ContextSwitchedMsg_ErrorPath(t *testing.T) {
	m := freshRoot(t)
	model, _ := m.Update(cluster.ContextSwitchedMsg{Err: errStub("denied")})
	r := model.(RootModel)
	if r.err == nil {
		t.Error("err should propagate")
	}
}

func rootAmidContextSwitch(t *testing.T) (RootModel, *testResourceLiveSet[cluster.Pod]) {
	t.Helper()
	m := freshRoot(t)
	m.dashboard.active = true
	set := newTestResourceLiveSet(resourceLiveState[cluster.Pod]{Ready: true})
	m.dashboard.podLive.Set(set)
	m.contexts = []cluster.KubeContext{{Name: "secondary"}}
	m.showCtxPicker = true
	if command := m.selectContext(0); command == nil {
		t.Fatal("selectContext() returned no command")
	}
	return m, set
}

func TestContextSwitchStopsLiveSetsBeforeConnecting(t *testing.T) {
	m, set := rootAmidContextSwitch(t)
	if !m.contextSwitching || m.dashboard.active || m.dashboard.podLive.Current() != nil {
		t.Fatalf("context switch start = switching:%v dashboard active:%v live set:%v", m.contextSwitching, m.dashboard.active, m.dashboard.podLive.Current() != nil)
	}
	if set.stops.Load() != 1 {
		t.Fatalf("live set stop count = %d, want one", set.stops.Load())
	}
	staleClosure := supervisedLiveMsg{generation: 1, closed: true}
	updated, _ := m.Update(staleClosure)
	m = updated.(RootModel)
	if m.dashboard.err != nil {
		t.Fatalf("stale closure surfaced during context switch: %v", m.dashboard.err)
	}
}

func TestContextSwitchFailureRestartsDashboard(t *testing.T) {
	m, _ := rootAmidContextSwitch(t)
	updated, command := m.Update(cluster.ContextSwitchedMsg{Err: errStub("denied")})
	m = updated.(RootModel)
	if command == nil || m.contextSwitching || !m.dashboard.active || m.err == nil {
		t.Fatalf("failed context switch recovery = command:%v switching:%v dashboard active:%v error:%v", command != nil, m.contextSwitching, m.dashboard.active, m.err)
	}
	m.dashboard.Deactivate()
}

func TestContextSwitchBlocksNavigationButAllowsExit(t *testing.T) {
	m := freshRoot(t)
	m.contextSwitching = true
	updated, command := m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	root := updated.(RootModel)
	if command != nil || root.screen != ScreenDashboard {
		t.Fatalf("navigation during context switch = command:%v screen:%d", command != nil, root.screen)
	}
	updated, command = root.Update(tea.MouseClickMsg{X: 1, Y: 1, Button: tea.MouseLeft})
	if command != nil || updated.(RootModel).screen != ScreenDashboard {
		t.Fatal("mouse input was handled during context switch")
	}
	_, command = root.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if command == nil {
		t.Fatal("quit was blocked during context switch")
	}
}

func TestRootUpdate_GoBackMsg_RestoresPrevScreen(t *testing.T) {
	m := freshRoot(t)
	m.prevScreen = ScreenLogs
	model, _ := m.Update(GoBackMsg{})
	r := model.(RootModel)
	if r.screen != ScreenLogs {
		t.Errorf("GoBack should restore prevScreen; got %d", r.screen)
	}
}

func TestRootHandleDrillDown_BrowserWithName_FetchesDescribe(t *testing.T) {
	m := freshRoot(t)
	model, cmd := m.handleDrillDown(DrillDownMsg{
		Screen: ScreenBrowser, ResourceType: "pod", ResourceName: "alpha", ResourceNS: "ns",
	})
	r := model.(RootModel)
	if r.screen != ScreenBrowser {
		t.Errorf("screen = %d, want Browser", r.screen)
	}
	if cmd == nil {
		t.Error("drill into browser with name should return describe cmd")
	}
}

func TestRootHandleDrillDown_BrowserNoNameNoFetch(t *testing.T) {
	m := freshRoot(t)
	model, _ := m.handleDrillDown(DrillDownMsg{Screen: ScreenBrowser})
	r := model.(RootModel)
	if r.screen != ScreenBrowser {
		t.Error("drill should still switch screen")
	}
}

func TestRootHandleDrillDown_BrowserNamespaceFallsBackToCurrent(t *testing.T) {
	m := freshRoot(t)
	m.namespace = "default-ns"
	_, cmd := m.handleDrillDown(DrillDownMsg{
		Screen: ScreenBrowser, ResourceType: "pod", ResourceName: "alpha",
	})
	if cmd == nil {
		t.Error("describe cmd should still fire with implicit ns")
	}
}

func TestRootHandleDrillDown_LogsWithName(t *testing.T) {
	m := freshRoot(t)
	model, cmd := m.handleDrillDown(DrillDownMsg{
		Screen: ScreenLogs, ResourceName: "alpha", ResourceNS: "ns",
	})
	r := model.(RootModel)
	if r.screen != ScreenLogs {
		t.Error("screen should be Logs")
	}
	if cmd == nil {
		t.Error("drill into logs should return cmd")
	}
}

func TestRootHandleDrillDown_DefaultJustSwitchesScreen(t *testing.T) {
	m := freshRoot(t)
	model, _ := m.handleDrillDown(DrillDownMsg{Screen: ScreenAnalysis})
	r := model.(RootModel)
	if r.screen != ScreenAnalysis {
		t.Errorf("screen = %d, want analysis", r.screen)
	}
}

func TestRootHandleStatusBarClick_EachTabSwitchesScreen(t *testing.T) {
	x := 0
	for _, tab := range rootScreenTabs {
		m := freshRoot(t)
		model, _ := m.handleStatusBarClick(x)
		if got := model.(RootModel).screen; got != tab.id {
			t.Errorf("click at tab %q selected screen %d, want %d", tab.name, got, tab.id)
		}
		x += lipgloss.Width(m.renderScreenTab(tab))
	}
}

func TestRootRenderStatusBar_EachScreenShowsContextualHints(t *testing.T) {
	for _, s := range []screenID{ScreenDashboard, ScreenBrowser, ScreenLogs, ScreenAnalysis, ScreenHelm, ScreenCRDs} {
		m := freshRoot(t)
		m.screen = s
		bar := m.renderStatusBar()
		if bar == "" {
			t.Errorf("status bar for screen %d should render content", s)
		}
	}
}

func TestRootUpdate_DrillDownMsg(t *testing.T) {
	m := freshRoot(t)
	model, _ := m.Update(DrillDownMsg{Screen: ScreenBrowser, ResourceType: "pods", ResourceName: "alpha", ResourceNS: "ns"})
	r := model.(RootModel)
	if r.screen != ScreenBrowser {
		t.Errorf("DrillDown should switch screen; got %d", r.screen)
	}
}

func TestRootHandleMouse_NSPickerInterceptsAllEvents(t *testing.T) {
	m := freshRoot(t)
	m.showNSPicker = true
	m.namespaces = []string{"a"}
	model, cmd := m.handleMouse(tea.MouseClickMsg{X: 5, Y: 5, Button: tea.MouseLeft})
	r := model.(RootModel)
	if !r.showNSPicker || r.namespace != "default" || cmd != nil {
		t.Fatalf("outside click leaked through namespace overlay: picker=%v namespace=%q cmd=%v", r.showNSPicker, r.namespace, cmd)
	}
}

func TestRootHandleMouse_CtxPickerInterceptsAllEvents(t *testing.T) {
	m := freshRoot(t)
	m.showCtxPicker = true
	m.contexts = []cluster.KubeContext{{Name: "a"}}
	model, cmd := m.handleMouse(tea.MouseClickMsg{X: 5, Y: 5, Button: tea.MouseLeft})
	r := model.(RootModel)
	if !r.showCtxPicker || r.screen != ScreenDashboard || cmd != nil {
		t.Fatalf("outside click leaked through context overlay: picker=%v screen=%d cmd=%v", r.showCtxPicker, r.screen, cmd)
	}
}

func TestRootHandleMouse_RightClickOnStatusBarIgnored(t *testing.T) {
	m := freshRoot(t)
	model, cmd := m.handleMouse(tea.MouseClickMsg{X: 1, Y: m.height - 1, Button: tea.MouseRight})
	_ = model
	if cmd != nil {
		t.Error("right click on status bar should be ignored")
	}
}

func TestRootHandleMouse_AnalysisVisibleAdjustsXForRightSideClick(t *testing.T) {
	m := freshRoot(t)
	m.analysisPanel.SetVisible(true)
	m.resizeChildren()
	model, _ := m.handleMouse(tea.MouseClickMsg{X: m.width - 5, Y: 5, Button: tea.MouseLeft})
	r := model.(RootModel)
	if r.analysisPanel.input.Focused() {
		t.Fatal("clicking the response area should blur the analysis input")
	}
}

func TestRootHandleMouse_LeftClickOnContentForwardsToScreen(t *testing.T) {
	m := freshRoot(t)
	m.dashboard.SetSize(m.width, m.height-1)
	m.dashboard.pods = []cluster.Pod{{Name: "first"}, {Name: "second"}}
	m.dashboard.rebuildTableRows()
	y := m.dashboard.podTableTopBoundary() + dashTableHeaderRows + 1
	model, _ := m.handleMouse(tea.MouseClickMsg{X: 5, Y: y, Button: tea.MouseLeft})
	r := model.(RootModel)
	if got := r.dashboard.SelectedPod(); got != "second" {
		t.Fatalf("content click selected pod %q, want second", got)
	}
}

func TestRootStartPortForwardFromPalette_BadPortMapping(t *testing.T) {
	m := freshRoot(t)
	_, cmd := m.startPortForwardFromPalette([]string{"my-pod", "not-a-mapping"})
	if cmd == nil {
		t.Fatal("should still return feedback cmd")
	}
	msg := cmd()
	if pf, ok := msg.(PortForwardFeedbackMsg); !ok || pf.Err == nil {
		t.Errorf("bad mapping should error; got %+v", msg)
	}
}

func TestRootHandlePFModalKey_EscClosesModal(t *testing.T) {
	m := freshRoot(t)
	m.showPFModal = true
	model, _ := m.handlePFModalKey("esc")
	r := model.(RootModel)
	if r.showPFModal {
		t.Error("esc should close PF modal")
	}
}

func TestRootHandlePFModalKey_BigFClosesModal(t *testing.T) {
	m := freshRoot(t)
	m.showPFModal = true
	model, _ := m.handlePFModalKey("F")
	r := model.(RootModel)
	if r.showPFModal {
		t.Error("F should close PF modal")
	}
}

func TestRootHandlePFModalKey_DownNavigates(t *testing.T) {
	m := freshRoot(t)
	m.showPFModal = true
	m.pfSessions = []kube.PortForwardSession{{ID: "a"}, {ID: "b"}}
	m.pfCursor = 0
	model, _ := m.handlePFModalKey("down")
	r := model.(RootModel)
	if r.pfCursor != 1 {
		t.Error("down should advance pf cursor")
	}
}

func TestRootHandlePFModalKey_XSetsConfirmKill(t *testing.T) {
	m := freshRoot(t)
	m.showPFModal = true
	m.pfSessions = []kube.PortForwardSession{testModelPortForwardSession(t, "abc", "alpha", 8080, 80)}
	m.pfCursor = 0
	model, _ := m.handlePFModalKey("x")
	r := model.(RootModel)
	if r.pfConfirmKillID != "abc" {
		t.Errorf("x should set pfConfirmKillID; got %q", r.pfConfirmKillID)
	}
}

func TestRootHandlePFModalKey_ConfirmYExecutesKill(t *testing.T) {
	m := freshRoot(t)
	m.showPFModal = true
	m.pfConfirmKillID = "abc"
	m.pfConfirmKillOf = "alpha (8080:80)"
	model, cmd := m.handlePFModalKey("y")
	r := model.(RootModel)
	if r.pfConfirmKillID != "" {
		t.Error("y should clear pfConfirmKillID")
	}
	if cmd == nil {
		t.Error("y should return StopPortForward cmd")
	}
}

func TestRootHandlePFModalKey_ConfirmNCancels(t *testing.T) {
	m := freshRoot(t)
	m.showPFModal = true
	m.pfConfirmKillID = "abc"
	model, _ := m.handlePFModalKey("n")
	r := model.(RootModel)
	if r.pfConfirmKillID != "" {
		t.Error("n should clear pfConfirmKillID")
	}
}

func TestRootHandlePFModalKey_R_RefreshesSessions(t *testing.T) {
	m := freshRoot(t)
	m.showPFModal = true
	m.pfCursor = 999
	model, _ := m.handlePFModalKey("r")
	r := model.(RootModel)
	if r.pfCursor >= 999 && len(r.pfSessions) > 0 {
		t.Error("r should clamp pfCursor to len(pfSessions)-1")
	}
}

func TestRootRenderPFModal_NoActiveShowsHelpHint(t *testing.T) {
	m := freshRoot(t)
	out := stripAnsiForTest(m.renderPFModal(20))
	if out == "" {
		t.Error("PF modal should render even with no sessions")
	}
}

func TestRootRenderPFModal_WithSessionsHighlightsCursor(t *testing.T) {
	m := freshRoot(t)
	m.pfSessions = []kube.PortForwardSession{testModelPortForwardSession(t, "a", "alpha", 8080, 80)}
	out := stripAnsiForTest(m.renderPFModal(20))
	if out == "" {
		t.Error("PF modal with session should render")
	}
}

func TestRootHandleCtxPickerMouse_ClickInsideSelects(t *testing.T) {
	m := freshRoot(t)
	m.showCtxPicker = true
	m.contexts = []cluster.KubeContext{{Name: "a"}, {Name: "b"}}
	contentHeight := m.height - 1
	maxVisible := contentHeight - 6
	if maxVisible < 5 {
		maxVisible = 5
	}
	visibleCount := minInt(maxVisible, len(m.contexts))
	itemStart := (contentHeight-(visibleCount+6))/2 + 3

	model, command := m.handleCtxPickerMouse(tea.MouseClickMsg{
		X: 5, Y: itemStart, Button: tea.MouseLeft,
	})
	updated := model.(RootModel)
	if updated.showCtxPicker || updated.ctxCursor != 0 || command == nil {
		t.Fatalf("click result = visible %t, cursor %d, command %v", updated.showCtxPicker, updated.ctxCursor, command)
	}
}

func TestRootHandleCtxPickerMouse_RightClickIgnored(t *testing.T) {
	m := freshRoot(t)
	m.showCtxPicker = true
	m.contexts = []cluster.KubeContext{{Name: "a"}}
	model, cmd := m.handleCtxPickerMouse(tea.MouseClickMsg{X: 5, Y: 10, Button: tea.MouseRight})
	updated := model.(RootModel)
	if !updated.showCtxPicker || updated.ctxCursor != m.ctxCursor || cmd != nil {
		t.Fatalf("right click changed picker: visible %t, cursor %d, command %v", updated.showCtxPicker, updated.ctxCursor, cmd)
	}
}

func TestRootActiveScreenHasInputFocus_DefaultScreensAreUnfocused(t *testing.T) {
	for _, s := range []screenID{ScreenDashboard, ScreenBrowser, ScreenLogs, ScreenAnalysis, ScreenHelm, ScreenCRDs} {
		m := freshRoot(t)
		m.screen = s
		if m.activeScreenHasInputFocus() {
			t.Errorf("default screen %d unexpectedly owns input focus", s)
		}
	}
}

func TestRootUpdateActiveScreen_AllScreens(t *testing.T) {
	for _, s := range []screenID{ScreenDashboard, ScreenBrowser, ScreenLogs, ScreenAnalysis, ScreenHelm, ScreenCRDs} {
		m := freshRoot(t)
		m.screen = s
		model, _ := m.updateActiveScreen(tea.KeyPressMsg{Code: 'x', Text: "x"})
		if _, ok := model.(RootModel); !ok {
			t.Errorf("updateActiveScreen for %d should return RootModel", s)
		}
	}
}

func TestRootSaveSessionReturnsStorageFailure(t *testing.T) {
	storageFailure := errors.New("storage failed")
	m := freshRoot(t)
	m.saveSessionState = func(session.SessionState) error { return storageFailure }
	if err := m.saveSession(); !errors.Is(err, storageFailure) {
		t.Fatalf("saveSession error = %v, want storage failure", err)
	}
}

func TestRootModel_HandleCtxPickerMouse_WheelDoesNotPanic(t *testing.T) {
	m := freshRoot(t)
	m.showCtxPicker = true
	model, _ := m.handleCtxPickerMouse(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	_ = model
}
