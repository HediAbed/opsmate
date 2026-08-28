package model

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/service"
)

func newLogsForTest(t *testing.T) LogsModel {
	t.Helper()
	m := NewLogsModel("ns")
	m.SetSize(120, 30)
	return m
}

func TestLogsModel_HandlePopupKey_EscClosesPopup(t *testing.T) {
	m := newLogsForTest(t)
	m.showPodPopup = true
	out, _ := m.handlePopupKey(tea.KeyPressMsg{Code: tea.KeyEsc, Text: "esc"})
	if out.showPodPopup {
		t.Error("esc should close the pod popup")
	}
}

func TestLogsModel_HandlePopupKey_UpDownNavigate(t *testing.T) {
	m := newLogsForTest(t)
	m.pods = []service.Pod{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	m.podCursor = 0
	out, _ := m.handlePopupKey(tea.KeyPressMsg{Code: tea.KeyDown, Text: "down"})
	if out.podCursor != 1 {
		t.Errorf("down should advance cursor; got %d", out.podCursor)
	}
	out, _ = out.handlePopupKey(tea.KeyPressMsg{Code: tea.KeyUp, Text: "up"})
	if out.podCursor != 0 {
		t.Error("up should walk cursor back")
	}
}

func TestLogsModel_HandlePopupKey_EnterSelectsPodAndStartsFetch(t *testing.T) {
	m := newLogsForTest(t)
	m.pods = []service.Pod{{Name: "alpha", Namespace: "ns"}}
	m.podCursor = 0
	m.showPodPopup = true
	out, cmd := m.handlePopupKey(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	if out.showPodPopup {
		t.Error("enter should close popup")
	}
	if out.selectedPod != "alpha" {
		t.Errorf("selectedPod = %q, want alpha", out.selectedPod)
	}
	if cmd == nil {
		t.Error("enter should return a fetch logs cmd")
	}
}

func TestLogsModel_HandlePopupMouse_LeftClickInsideSelectsPod(t *testing.T) {
	m := newLogsForTest(t)
	m.pods = []service.Pod{{Name: "alpha", Namespace: "ns"}}
	m.showPodPopup = true
	popupLeft := (m.width - 50) / 2
	popupTop := (m.height - min(len(m.pods)+4, m.height-4)) / 2
	out, _ := m.handlePopupMouse(tea.MouseClickMsg{
		X: popupLeft + 5, Y: popupTop + 3, Button: tea.MouseLeft,
	})
	if out.showPodPopup {
		t.Error("click on row should close popup")
	}
}

func TestLogsModel_HandlePopupMouse_ClickOutsideDismisses(t *testing.T) {
	m := newLogsForTest(t)
	m.showPodPopup = true
	out, _ := m.handlePopupMouse(tea.MouseClickMsg{X: 0, Y: 0, Button: tea.MouseLeft})
	if out.showPodPopup {
		t.Error("click outside popup should dismiss it")
	}
}

func TestLogsModel_HandlePopupMouse_RightClickIgnored(t *testing.T) {
	m := newLogsForTest(t)
	m.showPodPopup = true
	out, _ := m.handlePopupMouse(tea.MouseClickMsg{X: 100, Y: 10, Button: tea.MouseRight})
	if !out.showPodPopup {
		t.Error("right click should not affect the popup")
	}
}

func TestLogsModel_HandlePopupMouse_WheelScrollsCursor(t *testing.T) {
	m := newLogsForTest(t)
	m.pods = []service.Pod{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	m.podCursor = 0
	out, _ := m.handlePopupMouse(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if out.podCursor != 1 {
		t.Errorf("wheel down should advance cursor; got %d", out.podCursor)
	}
	out, _ = out.handlePopupMouse(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	if out.podCursor != 0 {
		t.Error("wheel up should retract cursor")
	}
}

func TestLogsModel_HandleContainerPopupKey_EscCloses(t *testing.T) {
	m := newLogsForTest(t)
	m.showContainerPopup = true
	out, _ := m.handleContainerPopupKey(tea.KeyPressMsg{Code: tea.KeyEsc, Text: "esc"})
	if out.showContainerPopup {
		t.Error("esc should close container popup")
	}
}

func TestLogsModel_HandleContainerPopupKey_UpDownNavigate(t *testing.T) {
	m := newLogsForTest(t)
	m.containers = []string{"main", "sidecar"}
	m.containerCursor = 0
	out, _ := m.handleContainerPopupKey(tea.KeyPressMsg{Code: tea.KeyDown, Text: "down"})
	if out.containerCursor != 1 {
		t.Errorf("down should advance container cursor; got %d", out.containerCursor)
	}
}

func TestLogsModel_HandleContainerPopupKey_EnterSelectsAndFetches(t *testing.T) {
	m := newLogsForTest(t)
	m.containers = []string{"main", "sidecar"}
	m.containerCursor = 1
	m.selectedPod = "alpha"
	m.showContainerPopup = true
	out, cmd := m.handleContainerPopupKey(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	if out.selectedContainer != "sidecar" {
		t.Errorf("selectedContainer = %q", out.selectedContainer)
	}
	if cmd == nil {
		t.Error("enter should return a fetch cmd")
	}
}

func TestLogsModel_HandleInspectKey_EscExitsInspect(t *testing.T) {
	m := newLogsForTest(t)
	m.inspectMode = true
	m.aiExplanation = "previous"
	out, _ := m.handleInspectKey(tea.KeyPressMsg{Code: tea.KeyEsc, Text: "esc"})
	if out.inspectMode {
		t.Error("esc should exit inspect mode")
	}
	if out.aiExplanation != "" {
		t.Error("esc should clear stale aiExplanation")
	}
}

func TestLogsModel_DoTick_ReturnsNonNilCmd(t *testing.T) {
	if doTick() == nil {
		t.Error("doTick should return a non-nil tea.Cmd")
	}
}

func TestLogsModel_Init_ReturnsCmd(t *testing.T) {
	m := NewLogsModel("ns")
	if m.Init() == nil {
		t.Error("Init should return a non-nil cmd")
	}
}

func TestLogsModel_RenderExplainPanel_ShowsExplanationOrLoadingOrError(t *testing.T) {
	m := newLogsForTest(t)
	m.inspectMode = true

	m.aiExplanation = "this means the pod ran out of memory"
	out := stripAnsiForTest(m.renderExplainPanel())
	if out == "" {
		t.Error("renderExplainPanel with explanation should produce content")
	}

	m.aiExplanation = ""
	m.aiExplainLoading = true
	if stripAnsiForTest(m.renderExplainPanel()) == "" {
		t.Error("loading state should render something")
	}

	m.aiExplainLoading = false
	m.aiExplainErr = errStub("boom")
	if stripAnsiForTest(m.renderExplainPanel()) == "" {
		t.Error("error state should render something")
	}
}

func TestLogsModel_RenderContainerPopupOverlay_NotEmpty(t *testing.T) {
	m := newLogsForTest(t)
	m.containers = []string{"main", "sidecar"}
	m.containerCursor = 0
	if m.renderContainerPopupOverlay(120, 30) == "" {
		t.Error("container popup should render non-empty content")
	}
}

func TestLogsModel_RenderPodPopupOverlay_NotEmpty(t *testing.T) {
	m := newLogsForTest(t)
	m.pods = []service.Pod{{Name: "alpha"}, {Name: "beta"}}
	m.podCursor = 0
	if m.renderPodPopupOverlay(120, 30) == "" {
		t.Error("pod popup should render non-empty content")
	}
}

func TestLogsModel_HandleInspectKey_UpDownNavigatesCursor(t *testing.T) {
	m := newLogsForTest(t)
	m.inspectMode = true
	m.filteredLines = []string{"a", "b", "c"}
	m.lineCursor = 0
	out, _ := m.handleInspectKey(tea.KeyPressMsg{Code: tea.KeyDown, Text: "down"})
	if out.lineCursor != 1 {
		t.Errorf("down should advance cursor; got %d", out.lineCursor)
	}
	out, _ = out.handleInspectKey(tea.KeyPressMsg{Code: tea.KeyUp, Text: "up"})
	if out.lineCursor != 0 {
		t.Errorf("up should retract cursor; got %d", out.lineCursor)
	}
}

func TestLogsModel_HandleInspectKey_EnterTriggersAIExplain(t *testing.T) {
	m := newLogsForTest(t)
	m.inspectMode = true
	m.filteredLines = []string{"oom-killer triggered"}
	m.lineCursor = 0
	m.selectedPod = "alpha"
	out, cmd := m.handleInspectKey(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	if !out.aiExplainLoading {
		t.Error("enter should set aiExplainLoading")
	}
	if cmd == nil {
		t.Error("enter should return AIExplainLogLine cmd")
	}
}

func TestLogsModel_HandleInspectKey_NJumpsToNextSeverityLine(t *testing.T) {
	m := newLogsForTest(t)
	m.inspectMode = true
	m.filteredLines = []string{
		"info line",
		"another info",
		"WARN something happened",
		"more info",
	}
	m.lineCursor = 0
	out, _ := m.handleInspectKey(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if out.lineCursor != 2 {
		t.Errorf("n should jump to next warn/error line at index 2; got %d", out.lineCursor)
	}
}

func TestLogsModel_HandleInspectKey_BigNJumpsToPrevSeverity(t *testing.T) {
	m := newLogsForTest(t)
	m.inspectMode = true
	m.filteredLines = []string{
		"info",
		"WARN earlier issue",
		"more info",
		"current line",
	}
	m.lineCursor = 3
	out, _ := m.handleInspectKey(tea.KeyPressMsg{Code: 'N', Text: "N"})
	if out.lineCursor != 1 {
		t.Errorf("N should jump back to warn line at 1; got %d", out.lineCursor)
	}
}

func TestLogsModel_HandleInspectKey_IExitsInspect(t *testing.T) {
	m := newLogsForTest(t)
	m.inspectMode = true
	out, _ := m.handleInspectKey(tea.KeyPressMsg{Code: 'i', Text: "i"})
	if out.inspectMode {
		t.Error("i should exit inspect mode")
	}
}

func TestLogsRenderFilterBar_FocusedShowsPrompt(t *testing.T) {
	m := newLogsForTest(t)
	m.filterInput.Focus()
	out := stripAnsiForTest(m.renderFilterBar())
	if out == "" {
		t.Error("focused filter input should render prompt")
	}
}

func TestLogsRenderFilterBar_FilterAppliedShowsValueAndMatchInfo(t *testing.T) {
	m := newLogsForTest(t)
	m.filter = "ERROR"
	m.filteredLines = []string{"line"}
	m.allLines = []string{"line", "other"}
	out := stripAnsiForTest(m.renderFilterBar())
	if out == "" {
		t.Error("active filter should render bar")
	}
}

func TestLogsRenderFilterBar_NoneEmpty(t *testing.T) {
	m := newLogsForTest(t)
	if got := m.renderFilterBar(); got != "" {
		t.Error("no filter should return empty bar")
	}
}

func TestLogsModel_RebuildInspectView_PreservesViewportWhenNoLines(t *testing.T) {
	m := newLogsForTest(t)
	m.inspectMode = true
	m.logView.SetContent("existing content")
	m.rebuildInspectView()
	if got := m.logView.View(); !strings.Contains(got, "existing content") {
		t.Fatalf("empty inspect rebuild changed viewport: %q", got)
	}
}
