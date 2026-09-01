package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/kube"
	screenmodel "github.com/HediAbed/opsmate/internal/ui/screen"
	browserscreen "github.com/HediAbed/opsmate/internal/ui/screen/browser"
)

func TestRootPrimaryOverlaysOwnKeyboardInput(t *testing.T) {
	t.Run("command palette", func(t *testing.T) {
		model := freshRoot(t)
		model.showCmdPalette = true
		model.cmdInput.Focus()
		updated, _ := model.handleRootKey(tea.KeyPressMsg{Code: 'p', Text: "p"})
		root := updated.(RootModel)
		if root.cmdInput.Value() != "p" || !root.showCmdPalette {
			t.Errorf("palette input = %q, visible=%v", root.cmdInput.Value(), root.showCmdPalette)
		}
	})

	t.Run("search", func(t *testing.T) {
		model := freshRoot(t)
		model.showSearch = true
		model.searchInput.Focus()
		model.searchCorpus = []screenmodel.SearchItem{{Kind: screenmodel.ResourceKindPod, Name: "worker"}}
		updated, _ := model.handleRootKey(tea.KeyPressMsg{Code: 'w', Text: "w"})
		root := updated.(RootModel)
		if root.searchInput.Value() != "w" || len(root.searchResults) != 1 {
			t.Errorf("search input = %q, results=%v", root.searchInput.Value(), root.searchResults)
		}
	})

	t.Run("port forwards", func(t *testing.T) {
		model := freshRoot(t)
		model.showPFModal = true
		model.pfSessions = []kube.PortForwardSession{{ID: "one"}, {ID: "two"}}
		updated, _ := model.handleRootKey(tea.KeyPressMsg{Code: tea.KeyDown})
		if cursor := updated.(RootModel).pfCursor; cursor != 1 {
			t.Errorf("port-forward cursor = %d, want 1", cursor)
		}
	})
}

func TestRootSecondaryOverlaysOwnKeyboardInput(t *testing.T) {
	t.Run("context picker", func(t *testing.T) {
		model := freshRoot(t)
		model.showCtxPicker = true
		model.contexts = []cluster.KubeContext{{Name: "one"}, {Name: "two"}}
		updated, _ := model.handleRootKey(tea.KeyPressMsg{Code: tea.KeyDown})
		if cursor := updated.(RootModel).ctxCursor; cursor != 1 {
			t.Errorf("context cursor = %d, want 1", cursor)
		}
	})

	t.Run("error dismissal", func(t *testing.T) {
		model := freshRoot(t)
		model.err = errStub("cluster unavailable")
		updated, _ := model.handleRootKey(tea.KeyPressMsg{Code: tea.KeyEsc})
		if updated.(RootModel).err != nil {
			t.Error("escape did not dismiss root error")
		}
	})

	t.Run("analysis input", func(t *testing.T) {
		model := freshRoot(t)
		model.analysisPanel.SetVisible(true)
		model.analysisPanel.Focus()
		updated, _ := model.handleRootKey(tea.KeyPressMsg{Code: 'x', Text: "x"})
		if value := updated.(RootModel).analysisPanel.InputValue(); value != "x" {
			t.Errorf("analysis input = %q", value)
		}
	})

	t.Run("full analysis closes to dashboard", func(t *testing.T) {
		model := freshRoot(t)
		model.screen = ScreenAnalysis
		model.analysisPanel.SetVisible(true)
		updated, _ := model.handleRootKey(tea.KeyPressMsg{Code: tea.KeyEsc})
		root := updated.(RootModel)
		if root.screen != ScreenDashboard || root.analysisPanel.IsVisible() {
			t.Errorf("escape left screen=%d visible=%v", root.screen, root.analysisPanel.IsVisible())
		}
	})
}

func TestRootFocusedScreenInterruptPolicy(t *testing.T) {
	model := freshRoot(t)
	model.screen = ScreenBrowser
	model.browser, _ = model.browser.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	_, command := model.handleFocusedScreenKey(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}, "ctrl+c")
	if command == nil {
		t.Fatal("focused non-shell screen did not return quit command")
	}
	rawMessage := command()
	if _, ok := rawMessage.(tea.QuitMsg); !ok {
		t.Fatalf("interrupt command returned %T", rawMessage)
	}

	session := newTestShellSession()
	operations := &testClusterOperations{shellSession: session}
	adapter := newNativeClusterOperations(model.runtime.Context, operations, operations, operations, operations)
	commands := newNativeClusterCommands(model.runtime.Context, &testResourceReader{}, &testResourceObserver{})
	model.browser = browserscreen.NewBrowserModel(model.namespace, commands, adapter)
	model.browser.SetSize(model.width, model.height)
	model.browser, _ = model.browser.Update(cluster.PodsMsg{Pods: []cluster.Pod{{Name: "worker", Namespace: "default", Status: "Running"}}})
	model.browser, _ = model.browser.Update(tea.KeyPressMsg{Code: 'X', Text: "X"})
	if !model.browser.InShell() {
		t.Fatal("browser did not open the shell")
	}
	updated, command := model.handleFocusedScreenKey(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}, "ctrl+c")
	root := updated.(RootModel)
	if command != nil {
		t.Errorf("shell interrupt returned command %v", command)
	}
	if root.screen != ScreenBrowser || root.browser.InShell() {
		t.Errorf("shell interrupt left screen=%d, inShell=%v", root.screen, root.browser.InShell())
	}
	if !session.interrupted || !session.closed {
		t.Fatalf("shell interrupt lifecycle = interrupted:%v closed:%v", session.interrupted, session.closed)
	}
}

func TestRootGlobalSearchAndPortForwardShortcuts(t *testing.T) {
	model := freshRoot(t)
	updated, command := model.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	root := updated.(RootModel)
	if command == nil || !root.showSearch {
		t.Fatalf("ctrl+p did not open search: command=%v visible=%v", command, root.showSearch)
	}

	model = freshRoot(t)
	updated, command = model.Update(tea.KeyPressMsg{Code: 'F', Text: "F"})
	root = updated.(RootModel)
	if command != nil || !root.showPFModal {
		t.Fatalf("F did not open port-forward modal: command=%v visible=%v", command, root.showPFModal)
	}
}

func TestRootAnalysisToggleCanCloseVisiblePanel(t *testing.T) {
	model := freshRoot(t)
	model.analysisPanel.SetVisible(true)
	model.toggleAnalysisPanel()
	if model.analysisPanel.IsVisible() {
		t.Error("analysis panel remained visible")
	}
}

func TestRootMouseIsBlockedByPrimaryOverlays(t *testing.T) {
	baseline := rootWithSelectableBrowserRows(t)
	rowY := browserRowY(t, baseline, "overlay-second")
	updated, command := baseline.handleMouse(tea.MouseClickMsg{X: 10, Y: rowY, Button: tea.MouseLeft})
	if command != nil {
		t.Fatalf("unblocked browser click returned command %v", command)
	}
	assertRootBrowserSelection(t, updated.(RootModel), "overlay-second")

	setups := []struct {
		name  string
		apply func(*RootModel)
	}{
		{name: "help", apply: func(model *RootModel) { model.showHelp = true }},
		{name: "search", apply: func(model *RootModel) { model.showSearch = true }},
		{name: "port forwards", apply: func(model *RootModel) { model.showPFModal = true }},
		{name: "command palette", apply: func(model *RootModel) { model.showCmdPalette = true }},
	}
	for _, setup := range setups {
		t.Run(setup.name, func(t *testing.T) {
			model := rootWithSelectableBrowserRows(t)
			setup.apply(&model)
			updated, command := model.handleMouse(tea.MouseClickMsg{X: 10, Y: rowY, Button: tea.MouseLeft})
			if command != nil {
				t.Fatalf("blocked browser click returned command %v", command)
			}
			root := updated.(RootModel)
			if root.screen != ScreenBrowser {
				t.Fatalf("blocked browser click changed screen to %d", root.screen)
			}
			assertRootBrowserSelection(t, root, "overlay-first")
		})
	}
}

func rootWithSelectableBrowserRows(t *testing.T) RootModel {
	t.Helper()
	model := freshRoot(t)
	model.screen = ScreenBrowser
	model.resizeChildren()
	model.browser, _ = model.browser.Update(cluster.PodsMsg{Pods: []cluster.Pod{
		{Name: "overlay-first", Namespace: "default", Status: "Running"},
		{Name: "overlay-second", Namespace: "default", Status: "Running"},
	}})
	assertRootBrowserSelection(t, model, "overlay-first")
	return model
}

func browserRowY(t *testing.T, model RootModel, name string) int {
	t.Helper()
	for row, line := range strings.Split(stripAnsiForTest(model.browser.View()), "\n") {
		if strings.Contains(line, name) {
			return row
		}
	}
	t.Fatalf("browser row %q was not rendered", name)
	return 0
}

func assertRootBrowserSelection(t *testing.T, model RootModel, wantName string) {
	t.Helper()
	kind, name := model.browser.SelectedResource()
	if kind != string(screenmodel.ResourceKindPod) || name != wantName {
		t.Fatalf("browser selection = %s/%s, want pod/%s", kind, name, wantName)
	}
}

func TestRootRoutesFocusedInputToActiveScreen(t *testing.T) {
	model := freshRoot(t)
	model.screen = ScreenBrowser
	model.browser, _ = model.browser.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	if !model.browser.HasInputFocus() {
		t.Fatal("browser filter did not take input focus")
	}
	updated, command := model.handleRootKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if command != nil || updated.(RootModel).browser.HasInputFocus() {
		t.Fatalf("focused browser did not consume escape: command=%v", command)
	}
}

func TestRootUpdateRoutesMouseAndUnknownMessages(t *testing.T) {
	model := freshRoot(t)
	updated, command := model.Update(struct{}{})
	if command != nil || updated.(RootModel).screen != model.screen {
		t.Errorf("unknown update changed root: command=%v", command)
	}

	model.screen = ScreenBrowser
	model.browser, _ = model.browser.Update(cluster.PodsMsg{Pods: []cluster.Pod{
		{Name: "first", Namespace: model.namespace, Status: "Running"},
		{Name: "second", Namespace: model.namespace, Status: "Running"},
	}})
	_, beforeName := model.browser.SelectedResource()
	updated, command = model.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	_, afterName := updated.(RootModel).browser.SelectedResource()
	if command != nil || beforeName != "first" || afterName != "second" {
		t.Fatalf("mouse wheel routing = before:%q after:%q command:%v", beforeName, afterName, command)
	}
}

func TestRootAnalysisPanelMouseRejectsMainPane(t *testing.T) {
	model := freshRoot(t)
	model.analysisPanel.SetVisible(true)
	updated, command, handled := model.handleAnalysisPanelMouse(tea.MouseClickMsg{X: 1, Y: 1, Button: tea.MouseLeft})
	if handled || command != nil || updated.(RootModel).screen != model.screen {
		t.Fatalf("main-pane mouse claimed by analysis panel: handled=%v command=%v", handled, command)
	}
	model.screen = ScreenAnalysis
	updated, command, handled = model.handleAnalysisPanelMouse(tea.MouseClickMsg{X: model.width - 1, Y: 1, Button: tea.MouseLeft})
	if handled || command != nil || updated.(RootModel).screen != ScreenAnalysis {
		t.Fatalf("analysis screen mouse claimed by side panel: handled=%v command=%v", handled, command)
	}
}

type syntheticMouseMessage struct {
	event tea.Mouse
}

func (message syntheticMouseMessage) Mouse() tea.Mouse { return message.event }
func (message syntheticMouseMessage) String() string   { return message.event.String() }

func TestShiftMouseXLeavesUnknownMouseTypeUntouched(t *testing.T) {
	message := syntheticMouseMessage{event: tea.Mouse{X: 9, Y: 2}}
	shifted := shiftMouseX(message, -4)
	if shifted != message {
		t.Errorf("unknown mouse type changed from %+v to %+v", message, shifted)
	}
}

func TestRootStatusBarNamespaceClickUsesCacheOrFetch(t *testing.T) {
	model := freshRoot(t)
	clickX := rootNamespaceBreadcrumbX(model)
	model.namespaces = []string{"default"}
	updated, command := model.handleStatusBarClick(clickX)
	if command != nil || !updated.(RootModel).showNSPicker {
		t.Fatalf("cached namespace click: command=%v visible=%v", command, updated.(RootModel).showNSPicker)
	}

	model.namespaces = nil
	updated, command = model.handleStatusBarClick(clickX)
	root := updated.(RootModel)
	if command == nil || !root.showNSPicker || !root.nsLoading {
		t.Fatalf("uncached namespace click: command=%v visible=%v loading=%v", command, root.showNSPicker, root.nsLoading)
	}

	updated, command = model.handleStatusBarClick(model.width + 20)
	if command != nil || updated.(RootModel).showNSPicker {
		t.Error("outside status click opened namespace picker")
	}
}

func rootNamespaceBreadcrumbX(model RootModel) int {
	width := rootBreadcrumbSpacing
	for _, tab := range rootScreenTabs {
		width += len(stripAnsiForTest(model.renderScreenTab(tab)))
	}
	return width
}

func TestRootSearchResetsCursorWhenFilterShrinks(t *testing.T) {
	model := freshRoot(t)
	model.showSearch = true
	model.searchInput.Focus()
	model.searchCorpus = []screenmodel.SearchItem{{Name: "alpha"}, {Name: "beta"}}
	model.searchResults = model.searchCorpus
	model.searchCursor = 1
	updated, _ := model.handleSearch("z", tea.KeyPressMsg{Code: 'z', Text: "z"})
	root := updated.(RootModel)
	if root.searchCursor != 0 || len(root.searchResults) != 0 {
		t.Errorf("filtered search cursor=%d results=%v", root.searchCursor, root.searchResults)
	}
}

func TestRootSearchOverlayScrollsAndMarksSelection(t *testing.T) {
	model := freshRoot(t)
	for index := range 9 {
		model.searchResults = append(model.searchResults, screenmodel.SearchItem{Kind: screenmodel.ResourceKindPod, Name: string(rune('a' + index)), Namespace: "default"})
	}
	model.searchCursor = 8
	rendered := stripAnsiForTest(model.renderSearchOverlay(4))
	if !strings.Contains(rendered, "pod          i") || strings.Contains(rendered, "pod          a") {
		t.Errorf("search window did not follow cursor: %q", rendered)
	}
	if !strings.Contains(rendered, "▸") {
		t.Errorf("selected row marker missing: %q", rendered)
	}
}
