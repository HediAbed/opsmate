package ui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/HediAbed/opsmate/internal/cluster"
)

func TestBrowserConfirmedSingleActionsPreserveSelectedIdentity(t *testing.T) {
	tests := []struct {
		name   string
		action string
		kind   string
		status string
	}{
		{name: "delete", action: "delete", kind: "pod", status: "Deleting pod/worker"},
		{name: "restart", action: "restart", kind: "deployment", status: "Restarting deployment/worker"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := newTestBrowserModel("team-a")
			model.confirmAction = test.action
			model.confirmTarget = "deployment/worker"
			model.confirmIdentity = resourceIdentity{Kind: test.kind, Namespace: "team-a", Name: "worker"}
			model.showConfirm = true
			model.state = stateDeleteConfirm

			command := model.executeConfirmedResourceAction()

			if command == nil {
				t.Fatal("confirmed action must return a command")
			}
			if model.showConfirm || model.state != stateBrowsing || !model.loading {
				t.Fatalf("confirmation state not cleared: show=%v state=%v loading=%v", model.showConfirm, model.state, model.loading)
			}
			if !strings.Contains(stripAnsiForTest(model.statusMsg), test.status) {
				t.Errorf("status = %q, want text containing %q", stripAnsiForTest(model.statusMsg), test.status)
			}
		})
	}
}

func TestBrowserConfirmedBatchActionsClearSelection(t *testing.T) {
	tests := []struct {
		name   string
		action string
		status string
	}{
		{name: "delete", action: "delete", status: "Deleting 2 deployments"},
		{name: "restart", action: "restart", status: "Restarting 2 deployments"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := newTestBrowserModel("team-a")
			model.resourceType = "deployments"
			model.confirmAction = test.action
			model.confirmTarget = "2 deployments"
			first := resourceIdentity{Kind: "deployment", Namespace: "team-a", Name: "api"}
			second := resourceIdentity{Kind: "deployment", Namespace: "team-a", Name: "worker"}
			model.toggleResourceSelection(first)
			model.toggleResourceSelection(second)

			command := model.executeConfirmedResourceAction()

			if command == nil {
				t.Fatal("confirmed batch action must return a command")
			}
			if len(model.selected) != 0 || len(model.selectedIdentities) != 0 {
				t.Fatalf("selection was not cleared: selected=%v identities=%v", model.selected, model.selectedIdentities)
			}
			if !strings.Contains(stripAnsiForTest(model.statusMsg), test.status) {
				t.Errorf("status = %q, want text containing %q", stripAnsiForTest(model.statusMsg), test.status)
			}
		})
	}
}

func TestBrowserMouseRoutesByInteractionState(t *testing.T) {
	model := newScrollableBrowserForTest()
	model.state = stateDetail
	updated, _ := model.handleBrowserMouseWheel(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if updated.detailView.YOffset() <= model.detailView.YOffset() {
		t.Errorf("detail wheel did not scroll: before=%d after=%d", model.detailView.YOffset(), updated.detailView.YOffset())
	}

	model.state = stateShell
	updated, _ = model.handleBrowserMouseWheel(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if updated.shellView.YOffset() <= model.shellView.YOffset() {
		t.Errorf("shell wheel did not scroll: before=%d after=%d", model.shellView.YOffset(), updated.shellView.YOffset())
	}

	model = newBrowserWithMarkerPod(t, "team-a")
	model.pods = append(model.pods, cluster.Pod{Name: "second", Namespace: "team-a", Status: "Running"})
	model.rebuildTable()
	updated, _ = model.handleBrowserMouseWheel(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if updated.resourceTable.Cursor() == model.resourceTable.Cursor() {
		t.Error("browser wheel did not move the resource selection")
	}
	updated, _ = updated.handleBrowserMouseWheel(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	if updated.resourceTable.Cursor() != 0 {
		t.Errorf("wheel up cursor = %d, want 0", updated.resourceTable.Cursor())
	}
}

func TestBrowserMouseClickAndMotionRespectDetailState(t *testing.T) {
	model := newScrollableBrowserForTest()
	model.state = stateDetail
	updated, _ := model.handleBrowserMouseClick(tea.MouseClickMsg{Button: tea.MouseLeft, X: 1, Y: 1})
	if updated.state != stateDetail {
		t.Errorf("detail click changed state to %v", updated.state)
	}
	updated, _ = model.handleBrowserMouseMotion(tea.MouseMotionMsg{X: 1, Y: 1})
	if updated.state != stateDetail {
		t.Errorf("detail motion changed state to %v", updated.state)
	}
	if command := updated.forwardBrowserDetailMessage(struct{}{}); command != nil {
		t.Error("unknown viewport message unexpectedly returned a command")
	}

	model.state = stateBrowsing
	updated, command := model.handleBrowserMouseMotion(tea.MouseMotionMsg{X: 1, Y: 1})
	if command != nil || updated.state != stateBrowsing {
		t.Fatalf("browsing motion should be ignored: state=%v command=%v", updated.state, command)
	}
	if command = updated.forwardBrowserDetailMessage(struct{}{}); command != nil {
		t.Error("non-detail message unexpectedly returned a command")
	}
}

func TestBrowserMouseClickSelectsRenderedRowThroughUpdate(t *testing.T) {
	model := newBrowserWithMarkerPod(t, "team-a")
	model.pods = append(model.pods, cluster.Pod{Name: "second", Namespace: "team-a", Status: "Running"})
	model.rebuildTable()
	positions := renderedRowPositions(model.View(), []string{"marker", "second"})
	rowY, found := positions["second"]
	if !found {
		t.Fatal("second row was not rendered")
	}

	updated, _ := model.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 10, Y: rowY})
	if selected := updated.resourceTable.SelectedRow(); len(selected) == 0 || !strings.HasPrefix(selected[0], "second") {
		t.Fatalf("selected row = %v, want second", selected)
	}

	unchanged, command := updated.handleBrowserMouseClick(tea.MouseClickMsg{Button: tea.MouseRight, X: 10, Y: rowY})
	if command != nil || unchanged.resourceTable.Cursor() != updated.resourceTable.Cursor() {
		t.Error("right click should not alter browser selection")
	}
}

func TestBrowserHorizontalDetailLayoutFitsBothPanes(t *testing.T) {
	model := newTestBrowserModel("team-a")
	model.width = 160
	model.height = 36
	model.showDetail = true
	model.splitHorizontal = true
	model.state = stateDetail
	model.syncBrowserLayout()

	leftWidth, rightWidth := browserHorizontalPaneWidths(model.width)
	if model.resourceTable.Width() > leftWidth {
		t.Errorf("table width %d exceeds left pane %d", model.resourceTable.Width(), leftWidth)
	}
	if model.detailView.Width() >= rightWidth {
		t.Errorf("detail width %d must leave room for pane chrome inside %d", model.detailView.Width(), rightWidth)
	}
	if model.detailView.Height() <= 0 || model.resourceTable.Height() <= 0 {
		t.Fatalf("invalid pane heights: table=%d detail=%d", model.resourceTable.Height(), model.detailView.Height())
	}
}

func TestBrowserMainContentRendersEveryMode(t *testing.T) {
	base := newBrowserWithMarkerPod(t, "team-a")
	base.detailContent = "detail"
	base.detailView.SetContent("detail")
	base.shellView = viewport.New(viewport.WithWidth(60), viewport.WithHeight(8))
	base.shellView.SetContent("shell output")

	tests := []struct {
		name  string
		setup func(*BrowserModel)
		want  string
	}{
		{name: "confirmation", setup: func(model *BrowserModel) {
			model.showConfirm = true
			model.confirmAction = "delete"
			model.confirmTarget = "pod/marker"
		}, want: "delete pod/marker"},
		{name: "scale", setup: func(model *BrowserModel) { model.state = stateScaleInput; model.scaleName = "marker" }, want: "SCALE"},
		{name: "shell", setup: func(model *BrowserModel) { model.state = stateShell; model.shellPod = "marker" }, want: "shell"},
		{name: "horizontal detail", setup: func(model *BrowserModel) {
			model.showDetail = true
			model.splitHorizontal = true
			model.state = stateDetail
		}, want: "detail"},
		{name: "vertical detail", setup: func(model *BrowserModel) { model.showDetail = true; model.state = stateDetail }, want: "detail"},
		{name: "table", setup: func(*BrowserModel) {}, want: "marker"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := base
			test.setup(&model)
			rendered := strings.ToLower(stripAnsiForTest(model.renderBrowserMainContent(24)))
			if !strings.Contains(rendered, strings.ToLower(test.want)) {
				t.Errorf("rendered content missing %q: %q", test.want, rendered)
			}
		})
	}
}

func newScrollableBrowserForTest() BrowserModel {
	model := newTestBrowserModel("team-a")
	model.detailView = viewport.New(viewport.WithWidth(40), viewport.WithHeight(2))
	model.detailView.SetContent("one\ntwo\nthree\nfour\nfive\nsix")
	model.shellView = viewport.New(viewport.WithWidth(40), viewport.WithHeight(2))
	model.shellView.SetContent("one\ntwo\nthree\nfour\nfive\nsix")
	return model
}
