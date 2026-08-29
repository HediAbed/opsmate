package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/HediAbed/opsmate/internal/cluster"
)

func TestHandleBrowsingKey_SlashEntersFilterState(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	out, _ := m.handleKey(tea.KeyPressMsg{Code: '/', Text: "/"})
	if out.state != stateFilter {
		t.Errorf("/ should enter filter state; got %v", out.state)
	}
	if !out.filterActive {
		t.Error("/ should set filterActive=true")
	}
}

func TestHandleBrowsingKey_EscOnSelectionClearsIt(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	toggleNamedResource(&m, "pod-a")
	out, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEsc, Text: "esc"})
	if len(out.selected) != 0 {
		t.Error("esc should clear selection when one exists")
	}
}

func TestHandleBrowsingKey_EscOnErrBannerClearsIt(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.errBanner = "kubectl: connection refused"
	out, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEsc, Text: "esc"})
	if out.errBanner != "" {
		t.Error("esc should clear errBanner when set")
	}
}

func TestHandleBrowsingKey_EscWithNoStateSendsGoBack(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	_, cmd := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEsc, Text: "esc"})
	if cmd == nil {
		t.Fatal("esc with no state to clear must return a GoBack cmd")
	}
	msg := cmd()
	if _, ok := msg.(GoBackMsg); !ok {
		t.Errorf("expected GoBackMsg, got %T", msg)
	}
}

func TestHandleBrowsingKey_SpaceTogglesSelectionOfCurrentRow(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.SetResourceType("pods")
	m.pods = []cluster.Pod{{Name: "alpha"}}
	m.rebuildTable()
	m.resourceTable.SetCursor(0)

	out, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	if !out.selected["alpha"] {
		t.Errorf("space should toggle selection ON; selected = %+v", out.selected)
	}

	out, _ = out.handleKey(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	if out.selected["alpha"] {
		t.Error("second space should toggle selection OFF")
	}
}

func TestHandleBrowsingKey_PSwitchesToPodsTab(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.SetResourceType("services")

	out, cmd := m.handleKey(tea.KeyPressMsg{Code: 'p', Text: "p"})
	if out.resourceType != "pods" {
		t.Errorf("p should switch to pods; got %q", out.resourceType)
	}
	if cmd == nil {
		t.Error("p should return a fetch+watch batch cmd")
	}
}

func TestHandleBrowsingKey_PWhilePodsActiveIsNoOp(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.SetResourceType("pods")

	out, cmd := m.handleKey(tea.KeyPressMsg{Code: 'p', Text: "p"})
	if out.resourceType != "pods" {
		t.Errorf("p when already on pods should be a no-op; got %q", out.resourceType)
	}
	if cmd != nil {
		t.Error("p when already on pods must not refetch")
	}
}

func TestHandleBrowsingKey_DSwitchesToDeployments(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.SetResourceType("services")
	out, cmd := m.handleKey(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if out.resourceType != "deployments" || cmd == nil {
		t.Errorf("d should switch to deployments + return cmd; got %q cmd=%v", out.resourceType, cmd != nil)
	}
}

func TestHandleBrowsingKey_LeftCyclesBackward(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.SetResourceType("deployments")

	out, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyLeft, Text: "left"})
	if out.resourceType != "pods" {
		t.Errorf("left from deployments should go to pods; got %q", out.resourceType)
	}
}

func TestHandleBrowsingKey_RightCyclesForward(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.SetResourceType("pods")

	out, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyRight, Text: "right"})
	if out.resourceType != "deployments" {
		t.Errorf("right from pods should go to deployments; got %q", out.resourceType)
	}
}

func TestHandleBrowsingKey_EnterDescribesSelected(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.SetResourceType("pods")
	m.pods = []cluster.Pod{{Name: "alpha", Namespace: "ns"}}
	m.rebuildTable()
	m.resourceTable.SetCursor(0)

	out, cmd := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	if !out.loading {
		t.Error("enter on a selected row should set loading=true")
	}
	if cmd == nil {
		t.Error("enter should return a describe cmd")
	}
}

func TestHandleBrowsingKey_EnterWithoutSelectionIsNoOp(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.SetResourceType("pods")
	m.loading = false
	out, cmd := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	if out.loading {
		t.Error("enter with no selection must not flip loading from false to true")
	}
	if cmd != nil {
		t.Error("enter with no selection must not return a cmd")
	}
}

func TestHandleBrowsingKey_LOnNonPodsShowsWarning(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.SetResourceType("services")

	out, cmd := m.handleKey(tea.KeyPressMsg{Code: 'l', Text: "l"})
	if cmd != nil {
		t.Error("l on non-pods should be a no-op cmd-wise")
	}
	if out.statusMsg == "" {
		t.Error("l on non-pods should set a warning status message")
	}
}

func TestHandleFilterState_EscResetsToBrowsing(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.state = stateFilter
	m.filterActive = true
	m.filterText = "abc"
	m.filterInput.SetValue("abc")

	out, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEsc, Text: "esc"})
	if out.state != stateBrowsing {
		t.Error("esc in filter state should return to browsing")
	}
	if out.filterText != "" {
		t.Error("esc should clear filterText")
	}
	if out.filterActive {
		t.Error("esc should clear filterActive")
	}
}

func TestHandleFilterState_EnterAppliesFilter(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.state = stateFilter
	m.filterActive = true
	m.filterInput.SetValue("alpha")

	out, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	if out.state != stateBrowsing {
		t.Error("enter in filter should return to browsing")
	}
	if out.filterText != "alpha" {
		t.Errorf("enter should commit filterText; got %q", out.filterText)
	}
}

func TestHandleFilterState_EnterEmptyDeactivatesFilter(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.state = stateFilter
	m.filterActive = true

	out, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	if out.filterActive {
		t.Error("enter with empty filter should deactivate filterActive")
	}
}

func TestHandleBrowsingKey_YFetchesYAML(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.SetResourceType("pods")
	m.pods = []cluster.Pod{{Name: "alpha", Namespace: "ns"}}
	m.rebuildTable()

	out, cmd := m.handleKey(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if !out.loading || cmd == nil {
		t.Error("y on selected row should set loading + return YAML cmd")
	}
}

func TestHandleBrowsingKey_YOnEmptyTableIsNoOp(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.SetResourceType("pods")
	m.loading = false
	out, cmd := m.handleKey(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if out.loading || cmd != nil {
		t.Error("y with no selection must be a no-op")
	}
}

func TestHandleBrowsingKey_RestartOnNonRolloutKindShowsWarning(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.SetResourceType("services")
	out, _ := m.handleKey(tea.KeyPressMsg{Code: 'R', Text: "R"})
	if out.statusMsg == "" {
		t.Error("R on non-deployment/statefulset should set a warning")
	}
}

func TestHandleBrowsingKey_RestartOnDeploymentEnterConfirmDialog(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.SetResourceType("deployments")
	m.deployments = []cluster.Deployment{{Name: "web", Namespace: "ns"}}
	m.rebuildTable()
	out, _ := m.handleKey(tea.KeyPressMsg{Code: 'R', Text: "R"})
	if !out.showConfirm {
		t.Error("R on deployment should show confirm dialog")
	}
	if out.confirmAction != "restart" {
		t.Errorf("confirmAction = %q, want restart", out.confirmAction)
	}
}

func TestHandleBrowsingKey_DeleteEnterConfirmDialog(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.SetResourceType("pods")
	m.pods = []cluster.Pod{{Name: "alpha", Namespace: "ns"}}
	m.rebuildTable()
	out, _ := m.handleKey(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if !out.showConfirm {
		t.Error("x on selected row should show confirm dialog")
	}
	if out.confirmAction != "delete" {
		t.Errorf("confirmAction = %q, want delete", out.confirmAction)
	}
}

func TestHandleBrowsingKey_BatchDeleteRequiresNamespace(t *testing.T) {
	m := newTestBrowserModel("")
	m.SetSize(200, 40)
	m.SetResourceType("pods")
	toggleNamedResource(&m, "alpha")
	out, _ := m.handleKey(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if out.errBanner == "" {
		t.Error("batch delete in all-namespaces mode must show err banner")
	}
}

func TestHandleBrowsingKey_CCopiesRow(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.SetResourceType("pods")
	m.pods = []cluster.Pod{{Name: "alpha", Namespace: "ns", Status: "Running"}}
	m.rebuildTable()
	out, _ := m.handleKey(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if out.statusMsg == "" {
		t.Error("c on selected row should set a status message (copied indicator)")
	}
}

func TestHandleBrowsingKey_CInDetailViewCopiesContent(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.showDetail = true
	m.detailContent = "describe output\nline 2"
	out, _ := m.handleKey(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if out.statusMsg == "" {
		t.Error("c in detail view should copy and set status")
	}
}

func TestHandleBrowsingKey_RTriggersRefresh(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.SetResourceType("pods")
	m.loading = false
	out, cmd := m.handleKey(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if !out.loading || cmd == nil {
		t.Error("r should set loading + return refetch cmd")
	}
}

func TestHandleBrowsingKey_LOnPodsDrillsToLogs(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.SetResourceType("pods")
	m.pods = []cluster.Pod{{Name: "alpha", Namespace: "ns"}}
	m.rebuildTable()
	_, cmd := m.handleKey(tea.KeyPressMsg{Code: 'l', Text: "l"})
	if cmd == nil {
		t.Fatal("l on selected pod should return a DrillDown cmd")
	}
	msg := cmd()
	if d, ok := msg.(DrillDownMsg); !ok || d.Screen != ScreenLogs {
		t.Errorf("expected DrillDownMsg{Screen: ScreenLogs}; got %T %+v", msg, msg)
	}
}

func TestHandleBrowsingKey_LWithNoSelectionIsNoOp(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.SetResourceType("pods")
	_, cmd := m.handleKey(tea.KeyPressMsg{Code: 'l', Text: "l"})
	if cmd != nil {
		t.Error("l with empty selection must be a no-op")
	}
}

func TestHandleBrowsingKey_SOnDeploymentEntersScaleInput(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.SetResourceType("deployments")
	m.deployments = []cluster.Deployment{{Name: "web", Ready: "1/1", UpToDate: 1}}
	m.rebuildTable()
	out, _ := m.handleKey(tea.KeyPressMsg{Code: 's', Text: "s"})
	if out.state != stateScaleInput {
		t.Errorf("s on deployment should enter scale-input state; got %v", out.state)
	}
	if out.scaleName != "web" {
		t.Errorf("scaleName = %q, want web", out.scaleName)
	}
}

func TestHandleBrowsingKey_SOnPodsShowsWarning(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.SetResourceType("pods")
	out, _ := m.handleKey(tea.KeyPressMsg{Code: 's', Text: "s"})
	if out.statusMsg == "" {
		t.Error("s on pods should set warning")
	}
}

func TestHandleBrowsingKey_EOnSelectionFetchesEvents(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.SetResourceType("pods")
	m.pods = []cluster.Pod{{Name: "alpha"}}
	m.rebuildTable()
	out, cmd := m.handleKey(tea.KeyPressMsg{Code: 'e', Text: "e"})
	if !out.loading || cmd == nil {
		t.Error("e on selection should set loading + return events fetch")
	}
}

func TestHandleScaleInputState_EscReturnsToBrowsing(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.state = stateScaleInput
	out, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEsc, Text: "esc"})
	if out.state != stateBrowsing {
		t.Error("esc in scale-input should return to browsing")
	}
}

func TestHandleScaleInputState_EnterValidNumberShowsConfirm(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.SetResourceType("deployments")
	m.deployments = []cluster.Deployment{{Name: "web"}}
	m.rebuildTable()
	m.state = stateScaleInput
	m.textInput.SetValue("3")

	out, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	if !out.showConfirm {
		t.Error("enter with valid replicas should show confirm dialog")
	}
	if out.state != stateScaleConfirm {
		t.Errorf("state should be stateScaleConfirm; got %v", out.state)
	}
}

func TestHandleScaleInputState_EnterInvalidNumberSetsError(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.state = stateScaleInput
	m.textInput.SetValue("not-a-number")

	out, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	if out.statusMsg == "" {
		t.Error("invalid replicas should set an error status")
	}
}

func TestHandleScaleConfirmState_NCancels(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.state = stateScaleConfirm
	m.showConfirm = true
	out, _ := m.handleKey(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if out.showConfirm {
		t.Error("n should hide confirm dialog")
	}
}

func TestHandleDeleteConfirmState_NCancels(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.state = stateDeleteConfirm
	m.showConfirm = true
	m.confirmAction = "delete"
	out, _ := m.handleKey(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if out.showConfirm {
		t.Error("n should cancel delete confirm")
	}
}

func TestHandleDetailState_EscClosesDetail(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.state = stateDetail
	m.showDetail = true
	out, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEsc, Text: "esc"})
	if out.showDetail {
		t.Error("esc in detail state should close detail view")
	}
}

func TestHandleDetailState_AKeyTriggersAnalysis(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.state = stateDetail
	m.showDetail = true
	m.detailKind = "describe"
	m.detailContent = "describe output"
	m.SetResourceType("pods")
	m.pods = []cluster.Pod{{Name: "alpha"}}
	m.rebuildTable()
	out, cmd := m.handleKey(tea.KeyPressMsg{Code: 'a', Text: "a"})
	if !out.analysisSummaryLoading {
		t.Error("a should set aiSummaryLoad")
	}
	if cmd == nil {
		t.Error("a should return analysis command")
	}
}

func TestHandleDetailState_AWhileLoadingIsNoOp(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.state = stateDetail
	m.detailKind = "describe"
	m.SetResourceType("pods")
	m.analysisSummaryLoading = true
	out, cmd := m.handleKey(tea.KeyPressMsg{Code: 'a', Text: "a"})
	_ = out
	if cmd != nil {
		t.Error("a while analysis is loading must be a no-op")
	}
}

func TestHandleDetailState_VTogglesSplitLayout(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.state = stateDetail
	prev := m.splitHorizontal
	out, _ := m.handleKey(tea.KeyPressMsg{Code: 'v', Text: "v"})
	if out.splitHorizontal == prev {
		t.Error("v should toggle splitHorizontal")
	}
}

func TestHandleDetailState_CCopiesDetailContent(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.state = stateDetail
	m.detailContent = "describe output\nline 2"
	out, _ := m.handleKey(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if out.statusMsg == "" {
		t.Error("c should set status message about copy")
	}
}

func TestHandleDetailState_DefaultForwardsToViewport(_ *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.state = stateDetail
	m.detailContent = strings.Repeat("line\n", 100)
	m.detailView.SetContent(m.detailContent)
	out, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyDown, Text: "down"})
	_ = out
}

func TestHandleBrowsingKey_DefaultForwardsToTable(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(200, 40)
	m.SetResourceType("pods")
	m.pods = []cluster.Pod{{Name: "a"}, {Name: "b"}}
	m.rebuildTable()

	out, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyDown, Text: "down"})
	if out.resourceTable.Cursor() != 1 {
		t.Errorf("down arrow should advance cursor; got %d", out.resourceTable.Cursor())
	}
}
