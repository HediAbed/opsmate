package model

import (
	"errors"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/service"
)

func TestBrowserRoutesEveryTypedWatchResource(t *testing.T) {
	assertBrowserWatchAdds(t, "deployments", service.WatchEventMsg[service.Deployment]{Event: service.WatchEvent[service.Deployment]{
		Kind: service.WatchAdded, Item: service.Deployment{Name: "web", Namespace: "team-a"},
	}}, func(model BrowserModel) int { return len(model.deployments) })
	assertBrowserWatchAdds(t, "ingresses", service.WatchEventMsg[service.Ingress]{Event: service.WatchEvent[service.Ingress]{
		Kind: service.WatchAdded, Item: service.Ingress{Name: "web", Namespace: "team-a"},
	}}, func(model BrowserModel) int { return len(model.ingresses) })
	assertBrowserWatchAdds(t, "networkpolicies", service.WatchEventMsg[service.NetworkPolicy]{Event: service.WatchEvent[service.NetworkPolicy]{
		Kind: service.WatchAdded, Item: service.NetworkPolicy{Name: "default-deny", Namespace: "team-a"},
	}}, func(model BrowserModel) int { return len(model.networkpolicies) })
	assertBrowserWatchAdds(t, "pvcs", service.WatchEventMsg[service.PersistentVolumeClaim]{Event: service.WatchEvent[service.PersistentVolumeClaim]{
		Kind: service.WatchAdded, Item: service.PersistentVolumeClaim{Name: "data", Namespace: "team-a"},
	}}, func(model BrowserModel) int { return len(model.pvcs) })
	assertBrowserWatchAdds(t, "cronjobs", service.WatchEventMsg[service.CronJob]{Event: service.WatchEvent[service.CronJob]{
		Kind: service.WatchAdded, Item: service.CronJob{Name: "cleanup", Namespace: "team-a"},
	}}, func(model BrowserModel) int { return len(model.cronjobs) })
	assertBrowserWatchAdds(t, "hpas", service.WatchEventMsg[service.HPA]{Event: service.WatchEvent[service.HPA]{
		Kind: service.WatchAdded, Item: service.HPA{Name: "web", Namespace: "team-a"},
	}}, func(model BrowserModel) int { return len(model.hpas) })
	assertBrowserWatchAdds(t, "secrets", service.WatchEventMsg[service.Secret]{Event: service.WatchEvent[service.Secret]{
		Kind: service.WatchAdded, Item: service.Secret{Name: "credentials", Namespace: "team-a"},
	}}, func(model BrowserModel) int { return len(model.secrets) })
	assertBrowserWatchAdds(t, "replicasets", service.WatchEventMsg[service.ReplicaSet]{Event: service.WatchEvent[service.ReplicaSet]{
		Kind: service.WatchAdded, Item: service.ReplicaSet{Name: "web-123", Namespace: "team-a"},
	}}, func(model BrowserModel) int { return len(model.replicasets) })
}

func assertBrowserWatchAdds(t *testing.T, resource string, message tea.Msg, itemCount func(BrowserModel) int) {
	t.Helper()
	model := NewBrowserModel("team-a")
	model.SetSize(140, 30)
	model.SetResourceType(resource)
	model, _ = model.updateBrowserWatchMessage(message)
	if itemCount(model) != 1 {
		t.Fatalf("%s watch event was not applied", resource)
	}
}

func TestBrowserResultAndInputRoutingCoverTransientMessages(t *testing.T) {
	model := NewBrowserModel("team-a")
	model.SetSize(100, 20)
	model.statusMsg = "finished"
	model, command := model.updateBrowserResultMessage(ClearStatusMsg{})
	if command != nil || model.statusMsg != "" {
		t.Fatal("clear-status message did not clear browser status")
	}

	model.state = stateDetail
	model.detailView.SetContent(strings.Repeat("line\n", 40))
	model, _ = model.updateBrowserInputMessage(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if model.detailView.YOffset() == 0 {
		t.Fatal("routed mouse wheel did not scroll detail content")
	}
	model, _ = model.updateBrowserInputMessage(tea.MouseMotionMsg{X: 2, Y: 2})
	if model.state != stateDetail {
		t.Fatal("routed mouse motion changed detail state")
	}
	model, command = model.updateBrowserInputMessage(struct{}{})
	if command != nil || model.state != stateDetail {
		t.Fatal("unknown detail message changed browser state")
	}
}

func TestBrowserClickAndStateFallbacksAreSafe(t *testing.T) {
	model := NewBrowserModel("team-a")
	model.SetSize(120, 24)
	updated, command := model.handleBrowseClick(0, 0)
	if command != nil || updated.resourceType != model.resourceType {
		t.Fatal("title padding click changed the active resource")
	}
	updated, command = model.handleTitleBarClick(100_000)
	if command != nil || updated.resourceType != model.resourceType {
		t.Fatal("click beyond the tabs changed the active resource")
	}

	model.state = browserState(255)
	updated, command = model.handleKey(tea.KeyPressMsg{Code: 'z', Text: "z"})
	if command != nil || updated.state != stateBrowsing {
		t.Fatalf("invalid state recovered as %v", updated.state)
	}
}

func TestBrowserTextEntryAndConfirmationsCoverAcceptedPaths(t *testing.T) {
	model := NewBrowserModel("team-a")
	model.SetSize(120, 24)
	model.state = stateFilter
	model.filterInput.Focus()
	model, _ = model.handleFilterKey("a", tea.KeyPressMsg{Code: 'a', Text: "a"})
	if model.filterText != "a" {
		t.Fatalf("live filter = %q, want a", model.filterText)
	}

	model.state = stateScaleInput
	model.textInput.Focus()
	model, _ = model.handleScaleInputKey("3", tea.KeyPressMsg{Code: '3', Text: "3"})
	if model.textInput.Value() != "3" {
		t.Fatalf("replica input = %q, want 3", model.textInput.Value())
	}

	model.scaleIdentity = resourceIdentity{Kind: "deployment", Namespace: "team-a", Name: "web"}
	model.scaleReplicas = 3
	model.state = stateScaleConfirm
	model.showConfirm = true
	model, command := model.handleScaleConfirmationKey("y")
	if command == nil || model.state != stateBrowsing || model.showConfirm || !model.loading {
		t.Fatal("accepted scale confirmation did not begin execution")
	}

	model.confirmIdentity = resourceIdentity{Kind: "pod", Namespace: "team-a", Name: "worker"}
	model.confirmAction = "delete"
	model.confirmTarget = "pod/worker"
	model.state = stateDeleteConfirm
	model.showConfirm = true
	model, command = model.handleResourceConfirmationKey("Y")
	if command == nil || model.state != stateBrowsing || model.showConfirm {
		t.Fatal("accepted resource confirmation did not begin execution")
	}
}

func TestBrowserMissingDetailSelectionDoesNotStartRequests(t *testing.T) {
	model := NewBrowserModel("team-a")
	model.SetSize(120, 24)
	model.loading = false
	model.detailKind = "describe"
	if command := model.analyzeDetail(); command != nil || !strings.Contains(stripAnsiForTest(model.statusMsg), "No resource") {
		t.Fatal("detail analysis without a selection did not explain the issue")
	}
	if command := model.fetchSelectedResourceEvents(); command != nil || model.loading {
		t.Fatal("event fetch without a selection changed loading state")
	}
}

func TestBrowserMissingTableSelectionDoesNotStartActions(t *testing.T) {
	model := NewBrowserModel("team-a")
	model.SetSize(120, 24)
	model.resourceType = "deployments"
	if command := model.openScaleInput(); command != nil || model.state == stateScaleInput {
		t.Fatal("scale input opened without a selected deployment")
	}
	model.resourceType = "pods"
	model.toggleSelectedResource()
	if len(model.selected) != 0 {
		t.Fatal("empty selection was toggled")
	}
}

func TestBrowserUnsupportedActionsPreserveBrowsingState(t *testing.T) {
	model := NewBrowserModel("team-a")
	model.SetSize(120, 24)
	updated, command := model.handleResourceReadKey("unknown")
	if command != nil || updated.resourceType != model.resourceType {
		t.Fatal("unknown resource action changed browser state")
	}
	updated, command = model.handleBrowsingKey("X", tea.KeyPressMsg{Code: 'X', Text: "X"})
	if command != nil || updated.shellSession != nil || updated.state != stateBrowsing {
		t.Fatal("shell action without a pod changed browser state")
	}
}

func TestBrowserBatchWideAndCopyPaths(t *testing.T) {
	model := NewBrowserModel("team-a")
	model.SetSize(120, 24)
	model.resourceType = "deployments"
	model.selected = map[string]bool{"web": true, "worker": true}
	if command := model.beginBatchResourceConfirmation("restart"); command != nil {
		t.Fatal("opening batch confirmation returned a command")
	}
	if !model.showConfirm || model.confirmTarget != "2 deployments" || model.state != stateDeleteConfirm {
		t.Fatalf("batch confirmation state = show:%t target:%q state:%v", model.showConfirm, model.confirmTarget, model.state)
	}

	model.state = stateBrowsing
	model.showConfirm = false
	updated, command := model.handleBrowserUtilityKey("w", tea.KeyPressMsg{Code: 'w', Text: "w"})
	if command != nil || !updated.wide {
		t.Fatal("wide-mode key did not enable wide mode")
	}

	empty := NewBrowserModel("team-a")
	empty.SetSize(120, 24)
	empty.resourceTable.SetRows(nil)
	empty.visibleResources = nil
	updated, command = empty.copyBrowserSelection()
	if command != nil || updated.statusMsg != "" {
		t.Fatal("copy on an empty table changed browser state")
	}
}

func TestBrowserOverlayAndDetailHelpersIncludeVisibleChrome(t *testing.T) {
	model := NewBrowserModel("team-a")
	model.SetSize(120, 30)
	model.loading = false
	model.state = stateFilter
	model.filterActive = true
	model.filterText = "web"
	model.errBanner = "failed"
	top, height, bottom := model.AIOverlayBounds(30)
	if top <= 1 || height <= 0 || bottom <= 0 {
		t.Fatalf("overlay bounds = top:%d height:%d bottom:%d", top, height, bottom)
	}

	model.detailKind = "describe"
	model.resourceType = "pods"
	if help := stripAnsiForTest(model.detailHelp()); !strings.Contains(help, "summary") {
		t.Fatalf("detail help = %q, want summary action", help)
	}
}

func TestBrowserDetailSummaryCommandPreservesRequestScope(t *testing.T) {
	model := NewBrowserModel("team-a")
	model.detailContent = "pod details"
	identity := resourceIdentity{Kind: "pod", Namespace: "team-a", Name: "web"}
	command := model.fetchDetailSummary(identity)
	message, ok := command().(browserDetailSummaryResultMsg)
	if !ok {
		t.Fatalf("summary command returned %T", command())
	}
	if message.requestID != model.detailRequestID || message.identity != identity || message.content != "pod details" {
		t.Fatalf("summary result scope = %#v", message)
	}
}

func TestBrowserSelectedIdentityFallsBackToRenderedRows(t *testing.T) {
	model := NewBrowserModel("team-a")
	model.visibleResources = nil
	model.resourceTable.SetRows([]table.Row{{selectionMark + "web", "Running"}})
	identity, found := model.selectedIdentity()
	if !found || identity.Kind != "pod" || identity.Name != "web" || identity.Namespace != "team-a" {
		t.Fatalf("pod identity = (%#v, %t)", identity, found)
	}

	model.resourceType = "rbac"
	model.visibleResources = nil
	model.resourceTable.SetRows([]table.Row{{"ClusterRole", selectionMark + "reader"}})
	identity, found = model.selectedIdentity()
	if !found || identity.Kind != "clusterrole" || identity.Name != "reader" {
		t.Fatalf("RBAC identity = (%#v, %t)", identity, found)
	}

	model.selected = map[string]bool{"fallback": true}
	model.selectedIdentities = nil
	if names := model.selectedNames(); len(names) != 1 || names[0] != "fallback" {
		t.Fatalf("selected names = %v", names)
	}
}

func TestBrowserUnknownResourceFallbacksRemainSafe(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	model := NewBrowserModel("team-a")
	model.resourceType = "unknown"
	model.width = 0
	model.recalcLayout()
	if model.resourceTable.Width() == 0 {
		t.Fatal("zero-width layout damaged the initialized table")
	}
	if rows := model.currentResourceRows(); rows != nil {
		t.Fatalf("unknown resource rows = %v", rows)
	}
	if identities := model.currentResourceIdentities(); identities != nil {
		t.Fatalf("unknown resource identities = %v", identities)
	}
	if count := model.currentResourceCount(); count != 0 {
		t.Fatalf("unknown resource count = %d", count)
	}
	model.rebuildTable()

	command := model.fetchCurrentResources()
	message, ok := command().(browserResultMsg)
	if !ok || message.resourceType != "unknown" || message.namespace != "team-a" {
		t.Fatalf("fallback fetch result = %#v", message)
	}

	cycled, _ := model.cycleResourceType(0)
	cycled.Deactivate()
	if cycled.resourceType != "pods" {
		t.Fatalf("unknown resource cycled to %q, want pods", cycled.resourceType)
	}
}

func TestBrowserDetailAndRowRebuildsCoverBoundaryStates(t *testing.T) {
	model := NewBrowserModel("team-a")
	model.SetSize(100, 24)
	model.aiSummaryLoad = true
	model.detailContent = "apiVersion: v1\nkind: Pod"
	model.detailKind = "yaml"
	model.rebuildDetailContent()
	if rendered := stripAnsiForTest(model.detailView.View()); !strings.Contains(rendered, "analyzing") || !strings.Contains(rendered, "apiVersion") {
		t.Fatalf("detail view = %q", rendered)
	}

	model.resourceType = "pods"
	model.pods = []service.Pod{
		{Name: "one", Namespace: "team-a"},
		{Name: "two", Namespace: "team-a"},
	}
	model.rebuildTable()
	model.filterText = "two"
	model.refreshRows(10)
	if model.resourceTable.Cursor() != 0 || len(model.resourceTable.Rows()) != 1 {
		t.Fatalf("filtered table = cursor:%d rows:%d", model.resourceTable.Cursor(), len(model.resourceTable.Rows()))
	}
}

func TestBrowserTitleTableAndStatusRenderBoundaryStates(t *testing.T) {
	model := NewBrowserModel("team-a")
	model.SetSize(100, 24)
	model.wide = true
	if title := stripAnsiForTest(model.renderTitleBar()); !strings.Contains(title, "WIDE") {
		t.Fatalf("wide title = %q", title)
	}

	model.loading = true
	model.resourceTable.SetRows(nil)
	if rendered := stripAnsiForTest(model.renderTableContent(10)); !strings.Contains(rendered, "Loading") {
		t.Fatalf("loading table = %q", rendered)
	}
	model.loading = false
	if rendered := stripAnsiForTest(model.renderTableContent(10)); !strings.Contains(rendered, "No pods") {
		t.Fatalf("empty table = %q", rendered)
	}

	model.selected = map[string]bool{"web": true}
	if rendered := stripAnsiForTest(model.renderStatusLine()); !strings.Contains(rendered, "1 SELECTED") {
		t.Fatalf("selection status = %q", rendered)
	}
}

func TestTryExtendLeftHandlesCompleteWindowBoundary(t *testing.T) {
	widths := []int{1, 1}
	start := 1
	used := 1
	if tryExtendLeft(&start, &used, widths, 1, 1, 1) {
		t.Fatal("left extension exceeded the complete-window width")
	}
	if start != 1 || used != 1 {
		t.Fatalf("rejected extension changed window: start=%d used=%d", start, used)
	}
	if !tryExtendLeft(&start, &used, widths, 1, 2, 1) || start != 0 || used != 2 {
		t.Fatalf("fitting extension = start:%d used:%d", start, used)
	}
}

func TestClearStatusAfterProducesClearMessage(t *testing.T) {
	message := clearStatusAfter(0)()
	if _, ok := message.(ClearStatusMsg); !ok {
		t.Fatalf("clear timer returned %T", message)
	}
}

func TestBrowserAnalyzeDetailPreservesProviderFailure(t *testing.T) {
	model := NewBrowserModel("team-a")
	model.SetSize(100, 24)
	model.pods = []service.Pod{{Name: "web", Namespace: "team-a"}}
	model.rebuildTable()
	model.detailKind = "describe"
	model.detailContent = "details"
	model.aiSummaryLoad = false
	command := model.analyzeDetail()
	if command == nil || !model.aiSummaryLoad {
		t.Fatal("valid detail analysis did not start")
	}
	message := command()
	result, ok := message.(browserDetailSummaryResultMsg)
	if !ok {
		t.Fatalf("analysis returned %T", message)
	}
	if payload, ok := result.payload.(service.DescribeSummaryMsg); !ok || payload.Err == nil {
		t.Fatalf("provider failure payload = %#v", result.payload)
	}
}

func TestBrowserResultRoutingKeepsCommandErrorsTyped(t *testing.T) {
	model := NewBrowserModel("team-a")
	sentinel := errors.New("failed")
	updated, command := model.updateBrowserResultMessage(service.CommandResultMsg{Err: sentinel})
	if command != nil || !strings.Contains(stripAnsiForTest(updated.statusMsg), "failed") {
		t.Fatalf("command failure status = %q", stripAnsiForTest(updated.statusMsg))
	}
}

func TestBrowserClearStatusDelayIsNonNegative(t *testing.T) {
	if command := clearStatusAfter(time.Nanosecond); command == nil {
		t.Fatal("positive clear delay returned nil command")
	}
}
