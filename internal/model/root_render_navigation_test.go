package model

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/service"
)

func TestRootFooterIncludesDismissibleError(t *testing.T) {
	model := freshRoot(t)
	model.err = errStub("cluster unavailable")
	rendered := stripAnsiForTest(model.renderRootFooter())
	for _, expected := range []string{"cluster unavailable", "esc dismiss", "Dashboard"} {
		if !strings.Contains(rendered, expected) {
			t.Errorf("root footer missing %q: %q", expected, rendered)
		}
	}
}

func TestRootActiveScreenViewHandlesAssistantAndInvalidScreen(t *testing.T) {
	model := freshRoot(t)
	model.screen = ScreenAI
	model.aiPanel.SetVisible(true)
	model.aiPanel.SetSize(80, 20)
	if rendered := model.activeScreenView(); rendered == "" {
		t.Error("assistant screen rendered empty content")
	}
	model.screen = screenID(255)
	if rendered := model.activeScreenView(); rendered != "" {
		t.Errorf("invalid screen rendered %q", rendered)
	}
}

func TestRootAssistantOverlayOffsetsFollowScreenChrome(t *testing.T) {
	for _, screen := range []screenID{ScreenBrowser, ScreenLogs, ScreenHelm, ScreenCRDs, ScreenDashboard, ScreenAI} {
		model := freshRoot(t)
		model.screen = screen
		top, bottom := model.assistantOverlayOffsets(30)
		if top < 0 || bottom < 0 || top+bottom > 30 {
			t.Errorf("screen %d offsets = %d/%d", screen, top, bottom)
		}
	}
}

func TestRootBreadcrumbUsesContextAndAllNamespaceLabels(t *testing.T) {
	model := freshRoot(t)
	model.namespace = ""
	model.currentContext = "staging"
	rendered := stripAnsiForTest(model.renderBreadcrumb())
	if !strings.Contains(rendered, "staging") || !strings.Contains(rendered, "all-ns") {
		t.Errorf("breadcrumb = %q", rendered)
	}

	model.screen = screenID(255)
	if child := model.renderScreenBreadcrumb(" > "); child != "" {
		t.Errorf("invalid screen breadcrumb = %q", child)
	}
	if hints := model.renderStatusHints(); hints != "" {
		t.Errorf("invalid screen hints = %q", hints)
	}
	if bindings := model.contextualHelpBindings(); bindings != nil {
		t.Errorf("invalid screen help = %v", bindings)
	}
}

func TestRootActivationHandlesRepeatedAndInvalidScreens(t *testing.T) {
	model := freshRoot(t)
	model.helmInited = true
	if commands := model.activateScreen(ScreenHelm); len(commands) != 1 || commands[0] == nil {
		t.Errorf("repeated Helm activation commands = %v", commands)
	}
	model.crdsInited = true
	if commands := model.activateScreen(ScreenCRDs); len(commands) != 1 || commands[0] == nil {
		t.Errorf("repeated CRD activation commands = %v", commands)
	}
	if commands := model.activateScreen(screenID(255)); commands != nil {
		t.Errorf("invalid screen activation commands = %v", commands)
	}
}

func TestRootPaletteCoversPortForwardAndActiveBrowserPaths(t *testing.T) {
	model := freshRoot(t)
	updated, command := model.executePaletteCommand("pf worker 8080:80")
	if command == nil || updated.(RootModel).screen != model.screen {
		t.Fatalf("port-forward palette command=%v screen=%d", command, updated.(RootModel).screen)
	}

	model.namespace = ""
	_, command = model.startPortForwardFromPalette([]string{"worker", "8080:80"})
	if command == nil {
		t.Fatal("all-namespace port-forward did not return feedback command")
	}
	feedback, ok := command().(PortForwardFeedbackMsg)
	if !ok || feedback.Err == nil || !strings.Contains(feedback.Err.Error(), "single namespace") {
		t.Errorf("all-namespace feedback = %#v", feedback)
	}

	model = freshRoot(t)
	model.screen = ScreenBrowser
	updated, command = model.openPaletteResource("services")
	root := updated.(RootModel)
	if command == nil || root.screen != ScreenBrowser || root.browser.ResourceType() != "services" {
		t.Errorf("active browser palette result: command=%v screen=%d resource=%q", command, root.screen, root.browser.ResourceType())
	}
}

func TestRootDrillDownSkipsMissingOrUnsupportedTargets(t *testing.T) {
	model := freshRoot(t)
	model.logs.selectedPod = "retained"
	model.prepareLogDrillDown(DrillDownMsg{Screen: ScreenLogs})
	if model.logs.selectedPod != "retained" {
		t.Errorf("empty log target changed selection to %q", model.logs.selectedPod)
	}

	for _, screen := range []screenID{ScreenDashboard, ScreenAI, ScreenHelm, ScreenCRDs, screenID(255)} {
		command := model.drillDownCommand(DrillDownMsg{Screen: screen, ResourceName: "worker"})
		if command != nil {
			t.Errorf("screen %d unexpectedly returned drill-down command", screen)
		}
	}
}

func TestRootPickerBoundaryStates(t *testing.T) {
	model := freshRoot(t)
	if cursor := pickerCursorAfterWheel(2, 5, tea.MouseLeft); cursor != 2 {
		t.Errorf("non-wheel cursor = %d, want 2", cursor)
	}
	if command := model.selectNamespace(-1); command != nil {
		t.Error("negative namespace index returned a command")
	}
	if command := model.selectNamespace(1); command != nil {
		t.Error("index beyond an empty namespace list returned a command")
	}

	model.contexts = []service.KubeContext{{Name: "current", Current: true}}
	model.showCtxPicker = true
	if command := model.selectContext(-1); command != nil {
		t.Error("negative context index returned a command")
	}
	if command := model.selectContext(1); command != nil {
		t.Error("context index beyond list returned a command")
	}
	if command := model.selectContext(0); command != nil {
		t.Error("selecting current context returned a switch command")
	}
	if model.showCtxPicker {
		t.Error("selecting current context did not close picker")
	}
}

func TestRootPickerRendersLoadingAndEmptyStates(t *testing.T) {
	model := freshRoot(t)
	model.nsLoading = true
	if rendered := stripAnsiForTest(model.renderNSPicker(20)); !strings.Contains(rendered, "Loading") {
		t.Errorf("namespace loading view = %q", rendered)
	}
	model.nsLoading = false
	if rendered := stripAnsiForTest(model.renderNSPicker(20)); !strings.Contains(rendered, "No namespaces") {
		t.Errorf("empty namespace view = %q", rendered)
	}

	model.ctxLoading = true
	if rendered := stripAnsiForTest(model.renderCtxPicker(20)); !strings.Contains(rendered, "Loading contexts") {
		t.Errorf("context loading view = %q", rendered)
	}
	model.ctxLoading = false
	if rendered := stripAnsiForTest(model.renderCtxPicker(20)); !strings.Contains(rendered, "No contexts") {
		t.Errorf("empty context view = %q", rendered)
	}
}

func TestRootPortForwardModalRendersUnselectedAndConfirmationRows(t *testing.T) {
	model := freshRoot(t)
	model.pfSessions = []service.PortForwardSession{
		{ID: "one", Pod: "api", LocalPort: 8080, RemotePort: 80, Status: service.PortForwardRunning, Started: time.Now()},
		{ID: "two", Pod: "worker", LocalPort: 9090, RemotePort: 90, Status: service.PortForwardRunning, Started: time.Now()},
	}
	model.pfCursor = 1
	model.pfConfirmKillID = "two"
	model.pfConfirmKillOf = "worker (9090:90)"
	rendered := stripAnsiForTest(model.renderPFModal(25))
	for _, expected := range []string{"api", "worker", "KILL worker", "[y]es"} {
		if !strings.Contains(rendered, expected) {
			t.Errorf("port-forward modal missing %q", expected)
		}
	}

	model.clearPFKillConfirmation()
	updated, _ := model.handlePFModalKey("up")
	if cursor := updated.(RootModel).pfCursor; cursor != 0 {
		t.Errorf("up cursor = %d, want 0", cursor)
	}
	model.pfCursor = -1
	model.beginPFKillConfirmation()
	if model.pfConfirmKillID != "" {
		t.Error("invalid cursor created kill confirmation")
	}
}

func TestRootResizeAndAssistantHeightBoundaryPaths(t *testing.T) {
	model := NewRootModel("default")
	model.resizeChildren()
	if model.dashboard.width != 0 || model.browser.width != 0 {
		t.Error("zero-sized root resized child screens")
	}

	model = freshRoot(t)
	for _, screen := range []screenID{ScreenBrowser, ScreenLogs, ScreenHelm, ScreenCRDs, ScreenDashboard, ScreenAI, screenID(255)} {
		model.screen = screen
		height := model.assistantPanelHeight(30)
		if height <= 0 || height > 30 {
			t.Errorf("screen %d assistant height = %d", screen, height)
		}
	}
}

func TestRootContextErrorsClearAssistantEvidence(t *testing.T) {
	model := freshRoot(t)
	model.aiPanel.SetScreenContext("stale")
	model.screen = screenID(255)
	model.updateAIScreenContext()
	if model.err == nil || model.aiPanel.screenContext != "" {
		t.Fatalf("invalid screen context: err=%v context=%q", model.err, model.aiPanel.screenContext)
	}

	model = freshRoot(t)
	model.browser.resourceType = "unsupported"
	if _, err := model.browserScreenContext(); err == nil {
		t.Error("unsupported browser resource did not return an error")
	}
	model.screen = screenID(255)
	if model.activeScreenHasInputFocus() {
		t.Error("invalid screen reported input focus")
	}
}
