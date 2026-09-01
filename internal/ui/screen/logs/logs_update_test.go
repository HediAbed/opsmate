package logs

import (
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/HediAbed/opsmate/internal/analysis"
	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/ui/screen"
)

func TestLogsUpdate_WindowSize(t *testing.T) {
	m := newTestLogsModel("ns")
	out, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 40})
	if out.width != 200 || out.height != 40 {
		t.Errorf("WindowSize not applied; got %dx%d", out.width, out.height)
	}
}

func TestLogsUpdate_ClearStatusMsg(t *testing.T) {
	m := newTestLogsModel("ns")
	m.statusMsg = "stale"
	out, _ := m.Update(screen.ClearStatusMsg{})
	if out.statusMsg != "" {
		t.Errorf("statusMsg should clear; got %q", out.statusMsg)
	}
}

func TestLogsUpdate_PodsMsg_PicksFirstPod(t *testing.T) {
	m := newTestLogsModel("ns")
	m.SetSize(120, 30)
	out, cmd := m.Update(cluster.PodsMsg{Pods: []cluster.Pod{{Name: "alpha", Namespace: "ns"}}})
	if out.selectedPod != "alpha" {
		t.Errorf("first pod should be selected; got %q", out.selectedPod)
	}
	if cmd == nil {
		t.Error("first-pod selection should kick off log fetch")
	}
}

func TestLogsUpdate_PodsMsg_Error(t *testing.T) {
	m := newTestLogsModel("ns")
	out, _ := m.Update(cluster.PodsMsg{Err: errStub("denied")})
	if out.err == nil {
		t.Error("err should propagate to model")
	}
}

func TestLogsUpdate_LogsMsg_HappyPath(t *testing.T) {
	m := newTestLogsModel("ns")
	m.SetSize(120, 30)
	m.selectedPod = "alpha"
	out, _ := m.Update(cluster.LogsMsg{Lines: []string{"line 1", "line 2"}})
	if out.loading {
		t.Error("LogsMsg should clear loading")
	}
	if len(out.allLines) != 2 {
		t.Errorf("allLines should be populated; got %d", len(out.allLines))
	}
}

func TestLogsUpdate_LogsMsg_Error(t *testing.T) {
	m := newTestLogsModel("ns")
	out, cmd := m.Update(cluster.LogsMsg{Err: errStub("boom")})
	if out.err == nil {
		t.Error("err should propagate")
	}
	if cmd == nil {
		t.Error("err path should still tick the next refresh")
	}
}

func TestLogsUpdateLogExplanationMsgSuccess(t *testing.T) {
	m := newTestLogsModel("ns")
	m.lineExplanationLoading = true
	out, _ := m.Update(analysis.LogExplanationMsg{Explanation: "this is the OOM line"})
	if out.lineExplanationLoading {
		t.Error("explain msg should clear loading")
	}
	if out.lineExplanation == "" {
		t.Error("explanation should be set")
	}
}

func TestLogsUpdateLogExplanationMsgError(t *testing.T) {
	m := newTestLogsModel("ns")
	m.lineExplanationLoading = true
	out, _ := m.Update(analysis.LogExplanationMsg{Err: errStub("rate limited")})
	if out.lineExplanationErr == nil {
		t.Error("explain err should be set")
	}
}

func TestLogsUpdate_ContainersMsg_MultiplePopsContainerPopup(t *testing.T) {
	m := newTestLogsModel("ns")
	m.SetSize(120, 30)
	m.selectedContainer = "main"
	out, _ := m.Update(cluster.ContainersMsg{Containers: []string{"main", "sidecar"}})
	if !out.showContainerPopup {
		t.Error("multi-container should show popup")
	}
	if out.containerCursor != 0 {
		t.Errorf("cursor should be at the selected container; got %d", out.containerCursor)
	}
}

func TestLogsUpdate_ContainersMsg_SingleContainerSetsStatus(t *testing.T) {
	m := newTestLogsModel("ns")
	m.SetSize(120, 30)
	out, _ := m.Update(cluster.ContainersMsg{Containers: []string{"main"}})
	if out.statusMsg == "" {
		t.Error("single container should set a status hint")
	}
}

func TestLogsUpdate_ContainersMsg_Error(t *testing.T) {
	m := newTestLogsModel("ns")
	out, _ := m.Update(cluster.ContainersMsg{Err: errStub("denied")})
	if out.statusMsg == "" {
		t.Error("err should produce a banner-style status")
	}
}

func TestLogsUpdate_ContainersMsg_DropsRemovedSelection(t *testing.T) {
	cases := []struct {
		name       string
		containers []string
		want       string
	}{
		{name: "pod has no containers left", containers: nil, want: ""},
		{name: "pod has a single replacement container", containers: []string{"main"}, want: "main"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			assertRemovedContainerSelection(t, testCase.containers, testCase.want)
		})
	}
}

func assertRemovedContainerSelection(t *testing.T, containers []string, want string) {
	t.Helper()
	m := newTestLogsModel("ns")
	m.SetSize(120, 30)
	m.selectedPod = "web"
	m.selectedContainer = "removed"
	out, _ := m.Update(cluster.ContainersMsg{Containers: containers})
	if out.selectedContainer != want {
		t.Errorf("selectedContainer = %q, want %q; logs would stream from a container that no longer exists",
			out.selectedContainer, want)
	}
}

func TestLogsUpdate_ContainersMsg_RemovedSelectionNeverSurvivesMultiContainerRefresh(t *testing.T) {
	m := newTestLogsModel("ns")
	m.SetSize(120, 30)
	m.selectedPod = "web"
	m.selectedContainer = "removed"
	out, _ := m.Update(cluster.ContainersMsg{Containers: []string{"main", "sidecar"}})
	if !out.showContainerPopup {
		t.Fatal("a multi-container refresh should open the container picker")
	}
	if out.selectedContainer != "" && !slices.Contains(out.containers, out.selectedContainer) {
		t.Errorf("selectedContainer = %q is absent from the refreshed list %v", out.selectedContainer, out.containers)
	}
}

func TestLogsUpdate_ContainersMsg_KeepsSurvivingSelection(t *testing.T) {
	m := newTestLogsModel("ns")
	m.SetSize(120, 30)
	m.selectedPod = "web"
	m.selectedContainer = "sidecar"
	out, _ := m.Update(cluster.ContainersMsg{Containers: []string{"main", "sidecar"}})
	if out.selectedContainer != "sidecar" {
		t.Errorf("selectedContainer = %q, want the still-present sidecar", out.selectedContainer)
	}
	if out.containerCursor != 1 {
		t.Errorf("containerCursor = %d, want the row of the surviving container", out.containerCursor)
	}
}

func TestLogsUpdate_TickMsgRefreshesLogsWhenNotPaused(t *testing.T) {
	m := newTestLogsModel("ns")
	m.SetSize(120, 30)
	m.selectedPod = "alpha"
	m.active = true
	out, cmd := m.Update(tickMsg{})
	if !out.loading || cmd == nil {
		t.Error("tick when not paused + selected pod should refetch")
	}
}

func TestLogsUpdate_TickMsgDoesNotFetchWhileInactive(t *testing.T) {
	m := newTestLogsModel("ns")
	m.selectedPod = "alpha"

	out, cmd := m.Update(tickMsg{})
	if out.loading {
		t.Fatal("an inactive log screen must not enter loading state")
	}
	if cmd != nil {
		t.Fatal("an inactive log screen must not start a log request")
	}
}

func TestLogsUpdate_KeyI_EntersInspectMode(t *testing.T) {
	m := newTestLogsModel("ns")
	m.SetSize(120, 30)
	m.filteredLines = []string{"line1", "line2"}
	out, _ := m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	if !out.inspectMode {
		t.Error("i should enter inspect mode when there are filtered lines")
	}
}

func TestLogsUpdate_KeyI_EmptyFilteredLines_NoOp(t *testing.T) {
	m := newTestLogsModel("ns")
	m.SetSize(120, 30)
	out, _ := m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	if out.inspectMode {
		t.Error("i with no lines should not enter inspect mode")
	}
}

func TestLogsUpdate_KeyP_OpensPodPopup(t *testing.T) {
	m := newTestLogsModel("ns")
	m.SetSize(120, 30)
	out, _ := m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	if !out.showPodPopup {
		t.Error("p should open pod popup")
	}
}

func TestLogsUpdate_KeyF_FocusesFilterInput(t *testing.T) {
	m := newTestLogsModel("ns")
	m.SetSize(120, 30)
	out, _ := m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	if !out.filterInput.Focused() {
		t.Error("f should focus filter input")
	}
}

func TestLogsUpdate_Space_TogglesPause(t *testing.T) {
	m := newTestLogsModel("ns")
	m.SetSize(120, 30)
	prev := m.paused
	out, _ := m.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	if out.paused == prev {
		t.Error("space should toggle paused")
	}
}

func TestLogsUpdate_R_TriggersRefresh(t *testing.T) {
	m := newTestLogsModel("ns")
	m.SetSize(120, 30)
	m.selectedPod = "alpha"
	out, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if !out.loading || cmd == nil {
		t.Error("r with selected pod should set loading + return cmd")
	}
}

func TestLogsUpdate_G_GoToTopDisablesAutoScroll(t *testing.T) {
	m := newTestLogsModel("ns")
	m.SetSize(120, 30)
	m.autoScroll = true
	out, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	if out.autoScroll {
		t.Error("g should disable autoScroll")
	}
}

func TestLogsUpdate_BigG_GoToBottomEnablesAutoScroll(t *testing.T) {
	m := newTestLogsModel("ns")
	m.SetSize(120, 30)
	m.autoScroll = false
	out, _ := m.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	if !out.autoScroll {
		t.Error("G should enable autoScroll")
	}
}

func TestLogsUpdate_Plus_IncrementsTail(t *testing.T) {
	m := newTestLogsModel("ns")
	m.SetSize(120, 30)
	prev := m.tailLines
	out, _ := m.Update(tea.KeyPressMsg{Code: '+', Text: "+"})
	if out.tailLines <= prev {
		t.Errorf("+ should increase tailLines; got %d (was %d)", out.tailLines, prev)
	}
}

func TestLogsUpdate_Minus_DecrementsTailButFloors(t *testing.T) {
	m := newTestLogsModel("ns")
	m.SetSize(120, 30)
	m.tailLines = 60
	out, _ := m.Update(tea.KeyPressMsg{Code: '-', Text: "-"})
	if out.tailLines < 50 {
		t.Errorf("- must floor at 50; got %d", out.tailLines)
	}
}

func TestLogsUpdate_C_CopiesFilteredLines(t *testing.T) {
	m := newTestLogsModel("ns")
	m.SetSize(120, 30)
	m.filteredLines = []string{"a", "b"}
	out, _ := m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if out.statusMsg == "" {
		t.Error("c should set status (copied indicator)")
	}
}

func TestLogsUpdate_BigC_CopiesAllLines(t *testing.T) {
	m := newTestLogsModel("ns")
	m.SetSize(120, 30)
	m.allLines = []string{"a", "b", "c"}
	out, _ := m.Update(tea.KeyPressMsg{Code: 'C', Text: "C"})
	if out.statusMsg == "" {
		t.Error("C should set status")
	}
}

func TestLogsUpdate_Esc_SendsGoBack(t *testing.T) {
	m := newTestLogsModel("ns")
	m.SetSize(120, 30)
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc, Text: "esc"})
	if cmd == nil {
		t.Fatal("esc should return GoBack cmd")
	}
	if _, ok := cmd().(screen.GoBackMsg); !ok {
		t.Errorf("expected GoBackMsg; got %T", cmd())
	}
}

func TestLogsUpdate_FilterInputFocused_EnterApplies(t *testing.T) {
	m := newTestLogsModel("ns")
	m.SetSize(120, 30)
	m.filterInput.Focus()
	m.filterInput.SetValue("ERROR")
	out, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	if out.filter != "ERROR" {
		t.Errorf("filter should be applied; got %q", out.filter)
	}
}

func TestLogsUpdate_FilterInputFocused_EscBlurs(t *testing.T) {
	m := newTestLogsModel("ns")
	m.SetSize(120, 30)
	m.filterInput.Focus()
	out, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc, Text: "esc"})
	if out.filterInput.Focused() {
		t.Error("esc should blur filter input")
	}
}

func TestLogsUpdate_TickMsgPausedNoFetch(t *testing.T) {
	m := newTestLogsModel("ns")
	m.SetSize(120, 30)
	m.selectedPod = "alpha"
	m.paused = true
	_, cmd := m.Update(tickMsg{})
	if cmd != nil {
		t.Error("paused tick must not refetch")
	}
}
