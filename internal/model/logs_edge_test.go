package model

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/service"
)

func TestLogTickMessageHasExpectedType(t *testing.T) {
	if _, ok := newLogTickMessage(time.Time{}).(tickMsg); !ok {
		t.Fatal("log timer callback returned the wrong message")
	}
}

func TestLogInputRoutingHandlesPopupPlainClickAndUnknownMessage(t *testing.T) {
	model := newLogsForTest(t)
	model.showPodPopup = true
	updated, command := model.updateLogInputMessage(tea.MouseClickMsg{X: 0, Y: 0, Button: tea.MouseLeft})
	if command != nil || updated.showPodPopup {
		t.Fatal("outside popup click did not dismiss the popup")
	}

	updated, command = updated.updateLogInputMessage(tea.MouseClickMsg{X: 0, Y: 0, Button: tea.MouseLeft})
	if command != nil || updated.showPodPopup {
		t.Fatal("plain click changed log state")
	}
	updated, command = updated.updateLogInputMessage(struct{}{})
	if command != nil || updated.namespace != model.namespace {
		t.Fatal("unknown input message changed log state")
	}
}

func TestApplyLogsStopsRefreshWhilePaused(t *testing.T) {
	model := newLogsForTest(t)
	model.paused = true
	command := model.applyLogs(service.LogsMsg{Lines: []string{"line"}})
	if command != nil || len(model.allLines) != 1 || model.loading {
		t.Fatalf("paused log result = command:%v lines:%d loading:%t", command != nil, len(model.allLines), model.loading)
	}
}

func TestApplyContainersHandlesPodWithoutContainers(t *testing.T) {
	model := newLogsForTest(t)
	if command := model.applyContainers(service.ContainersMsg{}); command != nil || len(model.containers) != 0 {
		t.Fatal("empty container result changed selection state")
	}
}

func TestLogSpinnerAdvancesForActiveLoad(t *testing.T) {
	model := newLogsForTest(t)
	model.loading = true
	if command := model.handleLogSpinnerTick(spinner.TickMsg{}); command == nil {
		t.Fatal("active spinner did not schedule its next frame")
	}
}

func TestLogMouseWheelRoutesPopupAndDisablesFollowOnUp(t *testing.T) {
	model := newLogsForTest(t)
	model.pods = []service.Pod{{Name: "one"}, {Name: "two"}}
	model.showPodPopup = true
	updated, _ := model.handleLogMouseWheel(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if updated.podCursor != 1 {
		t.Fatalf("popup wheel cursor = %d", updated.podCursor)
	}

	model.showPodPopup = false
	model.autoScroll = true
	model.logView.SetContent(strings.Repeat("line\n", 50))
	model.logView.GotoBottom()
	updated, _ = model.handleLogMouseWheel(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	if updated.autoScroll {
		t.Fatal("wheel up did not disable follow mode")
	}
}

func TestLogKeyRoutingPrioritizesModalStates(t *testing.T) {
	model := newLogsForTest(t)
	model.showContainerPopup = true
	updated, _ := model.handleLogKey(tea.KeyPressMsg{Code: tea.KeyEsc, Text: "esc"})
	if updated.showContainerPopup {
		t.Fatal("container popup did not receive escape")
	}

	model = updated
	model.showPodPopup = true
	updated, _ = model.handleLogKey(tea.KeyPressMsg{Code: tea.KeyEsc, Text: "esc"})
	if updated.showPodPopup {
		t.Fatal("pod popup did not receive escape")
	}

	model.showPodPopup = false
	model.inspectMode = true
	updated, _ = model.handleLogKey(tea.KeyPressMsg{Code: tea.KeyEsc, Text: "esc"})
	if updated.inspectMode {
		t.Fatal("inspect mode did not receive escape")
	}
}

func TestLogFilterForwardsTyping(t *testing.T) {
	model := newLogsForTest(t)
	model.filterInput.Focus()
	updated, _ := model.handleLogFilterKey(tea.KeyPressMsg{Code: 'e', Text: "e"})
	if updated.filterInput.Value() != "e" {
		t.Fatalf("filter input = %q", updated.filterInput.Value())
	}
}

func TestResumingSelectedLogSchedulesRefresh(t *testing.T) {
	model := newLogsForTest(t)
	model.paused = true
	model.selectedPod = "web"
	updated, command := model.handleLogModeKey("space")
	if command == nil || updated.paused {
		t.Fatal("resuming a selected pod did not schedule refresh")
	}
}

func TestLogFetchKeysRequirePodAndOpenContainerSelector(t *testing.T) {
	model := newLogsForTest(t)
	updated, command := model.handleLogFetchKey("r")
	if command != nil || updated.loading {
		t.Fatal("refresh without a pod changed loading state")
	}
	model.selectedPod = "web"
	updated, command = model.handleLogFetchKey("o")
	if command == nil || updated.loading {
		t.Fatal("container selector did not start its request")
	}
}

func TestForwardLogViewportKeyDisablesFollowWhenMovingUp(t *testing.T) {
	model := newLogsForTest(t)
	model.logView.SetContent(strings.Repeat("line\n", 50))
	model.logView.GotoBottom()
	model.autoScroll = true
	updated, _ := model.forwardLogViewportKey(tea.KeyPressMsg{Code: tea.KeyUp, Text: "up"})
	if updated.autoScroll || updated.logView.YOffset() >= model.logView.YOffset() {
		t.Fatalf("viewport move = follow:%t before:%d after:%d", updated.autoScroll, model.logView.YOffset(), updated.logView.YOffset())
	}
}

func TestLogsViewRendersOptionalSectionsAndPopupModes(t *testing.T) {
	model := newLogsForTest(t)
	model.inspectMode = true
	model.aiExplanation = "explanation"
	model.err = errors.New("fetch failed")
	model.statusMsg = "copied"
	model.filter = "error"
	model.allLines = []string{"error line"}
	model.filteredLines = []string{"error line"}
	rendered := stripAnsiForTest(model.View())
	for _, expected := range []string{"explanation", "fetch failed", "copied", "Filter"} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("view does not contain %q: %q", expected, rendered)
		}
	}

	model.inspectMode = false
	model.showContainerPopup = true
	model.containers = []string{"main"}
	if rendered = stripAnsiForTest(model.renderLogMainContent(20)); !strings.Contains(rendered, "SELECT CONTAINER") {
		t.Fatalf("container popup content = %q", rendered)
	}
	model.showContainerPopup = false
	model.showPodPopup = true
	model.pods = []service.Pod{{Name: "web"}}
	if rendered = stripAnsiForTest(model.renderLogMainContent(20)); !strings.Contains(rendered, "SELECT POD") {
		t.Fatalf("pod popup content = %q", rendered)
	}
}

func TestLogPanelAndOptionalRenderersHandleEmptyAndErrorStates(t *testing.T) {
	model := newLogsForTest(t)
	model.loading = false
	if rendered := stripAnsiForTest(model.renderLogPanel(15)); !strings.Contains(rendered, "No pod selected") {
		t.Fatalf("empty log panel = %q", rendered)
	}
	model.inspectMode = true
	if rendered := stripAnsiForTest(model.renderOptionalExplainPanel()); !strings.Contains(rendered, "Press") {
		t.Fatalf("optional explanation panel = %q", rendered)
	}
	model.err = errors.New("request denied")
	if rendered := stripAnsiForTest(model.renderLogError()); !strings.Contains(rendered, "request denied") {
		t.Fatalf("log error = %q", rendered)
	}
}

func TestLogsOverlayBoundsIncludeEveryVisibleFooter(t *testing.T) {
	model := newLogsForTest(t)
	model.filterInput.Focus()
	model.statusMsg = "copied"
	model.err = errors.New("failed")
	model.inspectMode = true
	top, height, bottom := model.AIOverlayBounds(30)
	if top <= 0 || height <= 0 || bottom <= 4 {
		t.Fatalf("overlay bounds = top:%d height:%d bottom:%d", top, height, bottom)
	}
}

func newLogsWithMissingKubectl(t *testing.T) LogsModel {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
	return NewLogsModel("team-a")
}

func TestLogPodCommandPreservesRequestScope(t *testing.T) {
	model := newLogsWithMissingKubectl(t)

	podsMessage, ok := model.fetchPods()().(logPodsResultMsg)
	if !ok || podsMessage.requestID != model.podListRequestID || podsMessage.namespace != "team-a" {
		t.Fatalf("pod result scope = %#v", podsMessage)
	}
	if payload, ok := podsMessage.payload.(service.PodsMsg); !ok || payload.Err == nil {
		t.Fatalf("pod failure payload = %#v", podsMessage.payload)
	}
}

func TestSelectedLogsCommandPreservesRequestScope(t *testing.T) {
	model := newLogsWithMissingKubectl(t)

	model.SetPodInNamespace("web", "team-a")
	logsMessage, ok := model.fetchSelectedLogs()().(logsResultMsg)
	if !ok || logsMessage.requestID != model.logRequestID || logsMessage.pod.Name != "web" || logsMessage.container != "" {
		t.Fatalf("log result scope = %#v", logsMessage)
	}
	if payload, ok := logsMessage.payload.(service.LogsMsg); !ok || payload.Err == nil {
		t.Fatalf("log failure payload = %#v", logsMessage.payload)
	}
}

func TestLogExplanationCommandPreservesRequestScope(t *testing.T) {
	model := newLogsWithMissingKubectl(t)
	model.SetPodInNamespace("web", "team-a")

	explainMessage, ok := model.explainSelectedLine("failure", "context")().(logExplainResultMsg)
	if !ok || explainMessage.requestID != model.explainRequestID || explainMessage.line != "failure" || explainMessage.pod.Name != "web" {
		t.Fatalf("explanation result scope = %#v", explainMessage)
	}
}

func TestSelectedInspectLineRejectsOutOfRangeCursor(t *testing.T) {
	model := newLogsForTest(t)
	model.filteredLines = []string{"line"}
	model.lineCursor = -1
	if line := model.selectedInspectLine(); line != "" {
		t.Fatalf("negative cursor line = %q", line)
	}
	model.lineCursor = 1
	if line := model.selectedInspectLine(); line != "" {
		t.Fatalf("past-end cursor line = %q", line)
	}
}

func TestPodPopupClickIgnoresChromeRows(t *testing.T) {
	model := newLogsForTest(t)
	model.pods = []service.Pod{{Name: "web"}}
	model.showPodPopup = true
	popupWidth := logsPopupWidth(podPopupDesiredWidth, model.width)
	popupHeight := min(len(model.pods)+logsPopupItemChrome, model.height-logsPopupItemChrome)
	popupLeft := (model.width - popupWidth) / pairedSides
	popupTop := (model.height - popupHeight) / pairedSides
	updated, command := model.handlePopupClick(popupLeft+1, popupTop)
	if command != nil || !updated.showPodPopup || updated.selectedPod != "" {
		t.Fatal("click on popup chrome selected a pod")
	}
}

func TestContainerPopupMovesCursorUp(t *testing.T) {
	model := newLogsForTest(t)
	model.containers = []string{"main", "sidecar"}
	model.containerCursor = 1
	updated, command := model.handleContainerPopupKey(tea.KeyPressMsg{Code: tea.KeyUp, Text: "up"})
	if command != nil || updated.containerCursor != 0 {
		t.Fatalf("container cursor = %d", updated.containerCursor)
	}
}

func TestInspectCursorBoundsAndViewportTracking(t *testing.T) {
	model := newLogsForTest(t)
	model.filteredLines = make([]string, 20)
	for index := range model.filteredLines {
		model.filteredLines[index] = fmt.Sprintf("line %d", index)
	}
	model.logView.SetHeight(3)
	model.rebuildInspectView()
	model.lineCursor = 0
	model.moveInspectCursor(-1)
	if model.lineCursor != 0 {
		t.Fatal("inspect cursor moved before the first line")
	}

	model.logView.SetYOffset(5)
	model.lineCursor = 5
	model.moveInspectCursor(-1)
	if model.lineCursor != 4 || model.logView.YOffset() != 4 {
		t.Fatalf("upper viewport tracking = cursor:%d offset:%d", model.lineCursor, model.logView.YOffset())
	}

	model.logView.SetYOffset(0)
	model.lineCursor = 2
	model.moveInspectCursor(1)
	if model.lineCursor != 3 || model.logView.YOffset() != 1 {
		t.Fatalf("lower viewport tracking = cursor:%d offset:%d", model.lineCursor, model.logView.YOffset())
	}
}

func TestLogExplanationAndIssueNavigationHandleMissingTargets(t *testing.T) {
	model := newLogsForTest(t)
	model.lineCursor = -1
	if command := model.explainInspectedLine(); command != nil {
		t.Fatal("invalid cursor started an explanation")
	}
	model.filteredLines = []string{"info", "debug"}
	model.lineCursor = 0
	if _, found := model.nextImportantLine(); found {
		t.Fatal("non-issue lines produced a next issue")
	}
	model.lineCursor = 1
	if _, found := model.previousImportantLine(); found {
		t.Fatal("non-issue lines produced a previous issue")
	}
	model.aiExplainLoading = true
	if command := model.explainInspectedLine(); command != nil {
		t.Fatal("concurrent explanation request was started")
	}
}

func TestJumpToInspectLineCentersOffscreenTarget(t *testing.T) {
	model := newLogsForTest(t)
	model.filteredLines = make([]string, 20)
	model.logView.SetHeight(4)
	model.logView.SetYOffset(0)
	model.jumpToInspectLine(10)
	if model.lineCursor != 10 || model.logView.YOffset() == 0 {
		t.Fatalf("jump result = cursor:%d offset:%d", model.lineCursor, model.logView.YOffset())
	}
}

func TestLogSeverityStyleFallsBackForInvalidSeverity(t *testing.T) {
	if rendered := applyLogSeverityStyle("line", lineSeverity(255)); rendered != "  line" {
		t.Fatalf("fallback severity rendering = %q", rendered)
	}
}

func TestLogTitleIncludesContainerAndIssueCount(t *testing.T) {
	model := newLogsForTest(t)
	model.selectedPod = "web"
	model.selectedContainer = "sidecar"
	model.filteredLines = []string{"ERROR failed", "healthy"}
	label := stripAnsiForTest(model.renderLogPodLabel())
	if !strings.Contains(label, "web") || !strings.Contains(label, "/sidecar") {
		t.Fatalf("pod label = %q", label)
	}
	indicators := stripAnsiForTest(strings.Join(model.renderLogTitleIndicators(), " "))
	if model.logIssueCount() != 1 || !strings.Contains(indicators, "1 issues") {
		t.Fatalf("title indicators = %q", indicators)
	}
}

func TestContainerPopupRendersEmptyAndCurrentStates(t *testing.T) {
	model := newLogsForTest(t)
	if rendered := stripAnsiForTest(model.renderContainerPopupOverlay(80, 20)); !strings.Contains(rendered, "No containers") {
		t.Fatalf("empty container popup = %q", rendered)
	}
	model.containers = []string{"main", "sidecar"}
	model.containerCursor = 0
	model.selectedContainer = "sidecar"
	if rendered := stripAnsiForTest(model.renderContainerPopupOverlay(80, 20)); !strings.Contains(rendered, "(current)") {
		t.Fatalf("current container popup = %q", rendered)
	}
}

func TestPodPopupRendersEmptySmallScrolledAndCurrentStates(t *testing.T) {
	model := newLogsForTest(t)
	if rendered := stripAnsiForTest(model.renderPodPopupOverlay(80, 8)); !strings.Contains(rendered, "No pods") {
		t.Fatalf("empty pod popup = %q", rendered)
	}
	for index := range 8 {
		model.pods = append(model.pods, service.Pod{
			Name:      fmt.Sprintf("pod-%d", index),
			Namespace: "team-a",
			Status:    "Running",
		})
	}
	model.namespace = "team-a"
	model.podCursor = 6
	model.selectedPod = "pod-4"
	model.selectedPodNamespace = "team-a"
	rendered := stripAnsiForTest(model.renderPodPopupOverlay(80, 8))
	if !strings.Contains(rendered, "pod-6") || !strings.Contains(rendered, "pod-4") || !strings.Contains(rendered, "(current)") {
		t.Fatalf("scrolled pod popup = %q", rendered)
	}
}
