package model

import (
	"errors"
	"strings"
	"testing"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/service"
)

func TestCRDChromeAccountsForErrorsAndAllNamespaceScope(t *testing.T) {
	model := NewCRDsModel("")
	model.SetSize(100, 20)
	model.loading = false
	model.err = errors.New("cluster unavailable")
	model.statusMsg = "refresh failed"

	top, height, bottom := model.AIOverlayBounds(20)
	if top <= 0 || height <= 0 || bottom <= 1 {
		t.Fatalf("overlay bounds = top:%d height:%d bottom:%d", top, height, bottom)
	}
	if firstRow := model.tableFirstRowY(); firstRow <= top+tableHeaderChromeRows {
		t.Fatalf("first row %d did not include the error banner", firstRow)
	}
	rendered := stripAnsiForTest(model.View())
	for _, expected := range []string{"all namespaces", "refresh failed", "cluster unavailable"} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("view does not contain %q: %q", expected, rendered)
		}
	}
	if model.scopeLabel() != "any namespace" {
		t.Fatalf("scope label = %q", model.scopeLabel())
	}
}

func TestCRDUpdateIgnoresUnknownMessage(t *testing.T) {
	model := NewCRDsModel("team-a")
	updated, command := model.Update(struct{}{})
	if command != nil || updated.namespace != "team-a" {
		t.Fatal("unknown message changed CRD state")
	}
}

func TestCRDMouseWheelMovesUp(t *testing.T) {
	model := NewCRDsModel("team-a")
	model.crds = make([]service.CRD, 10)
	model.crdsTable.SetRows(model.crdsRows())
	model.crdsTable.SetCursor(6)
	model.handleCRDMouseWheel(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	if model.crdsTable.Cursor() != 6-tableWheelStep {
		t.Fatalf("wheel-up cursor = %d", model.crdsTable.Cursor())
	}
}

func TestCRDNavigationFallbacksRemainInCurrentView(t *testing.T) {
	model := NewCRDsModel("team-a")
	model.crdsTable.SetRows([]table.Row{{"missing.example.com"}})
	updated, command := model.drillIntoSelected()
	if command != nil || updated.view != crdsViewList {
		t.Fatal("unmatched CRD row opened an instance view")
	}
	updated, command = model.backToList()
	if command != nil || updated.view != crdsViewList {
		t.Fatal("back from list changed the view")
	}
}

func TestCRDFetchCurrentViewRejectsIncompleteAndInvalidStates(t *testing.T) {
	model := NewCRDsModel("team-a")
	model.view = crdsViewInstances
	if command := model.fetchCurrentView(); command != nil {
		t.Fatal("instance fetch started without a selected resource")
	}
	model.view = crdsViewState(255)
	if command := model.fetchCurrentView(); command != nil {
		t.Fatal("invalid CRD view started a fetch")
	}
}

func TestCRDListCommandPreservesRequestScope(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	model := NewCRDsModel("team-a")

	listMessage, ok := model.fetchCRDList()().(crdResultMsg)
	if !ok || listMessage.kind != crdListResult || listMessage.namespace != "team-a" || listMessage.requestID != model.listRequestID {
		t.Fatalf("list result scope = %#v", listMessage)
	}
	if payload, ok := listMessage.payload.(service.CRDsMsg); !ok || payload.Err == nil {
		t.Fatalf("list failure payload = %#v", listMessage.payload)
	}
}

func TestCRDInstancesCommandPreservesRequestScope(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	model := NewCRDsModel("team-a")

	instanceMessage, ok := model.fetchCRDInstances("widgets.example.com")().(crdResultMsg)
	if !ok || instanceMessage.kind != crdInstancesResult || instanceMessage.resource != "widgets.example.com" ||
		instanceMessage.namespace != "team-a" || instanceMessage.requestID != model.instanceRequestID {
		t.Fatalf("instance result scope = %#v", instanceMessage)
	}
	if payload, ok := instanceMessage.payload.(service.CRDInstancesMsg); !ok || payload.Err == nil {
		t.Fatalf("instance failure payload = %#v", instanceMessage.payload)
	}
}

func TestCRDAcceptsCurrentInstanceResultAndRejectsUnknownKind(t *testing.T) {
	model := NewCRDsModel("team-a")
	model.view = crdsViewInstances
	model.selectedCRD = service.CRD{Resource: "widgets.example.com"}
	model.instanceRequestID = 7
	if !model.acceptsResult(crdResultMsg{
		kind:      crdInstancesResult,
		requestID: 7,
		namespace: "team-a",
		resource:  "widgets.example.com",
	}) {
		t.Fatal("current instance result was rejected")
	}
	if model.acceptsResult(crdResultMsg{kind: crdResultKind(255), namespace: "team-a"}) {
		t.Fatal("unknown result kind was accepted")
	}
}

func TestCRDViewHandlesSmallErrorAndInvalidLayouts(t *testing.T) {
	model := NewCRDsModel("team-a")
	model.SetSize(80, 1)
	model.loading = false
	model.err = errors.New("failed")
	if rendered := stripAnsiForTest(model.View()); !strings.Contains(rendered, "failed") {
		t.Fatalf("small error view = %q", rendered)
	}
	model.err = nil
	model.view = crdsViewState(255)
	if body := model.renderBody(); body != "" {
		t.Fatalf("invalid-view body = %q", body)
	}
}

func TestCRDBodiesRenderErrorsEmptyStatesAndTables(t *testing.T) {
	model := NewCRDsModel("team-a")
	model.loading = false
	model.err = errors.New("failed")
	model.view = crdsViewInstances
	if body := model.renderCRDInstancesBody(); body != "" {
		t.Fatalf("instance error body = %q", body)
	}
	model.err = nil
	model.instances = []service.CRDInstance{{Name: "widget", Namespace: "team-a"}}
	model.instanceTable.SetRows(model.instanceRows())
	if body := stripAnsiForTest(model.renderCRDInstancesBody()); !strings.Contains(body, "widget") {
		t.Fatalf("instance table body = %q", body)
	}

	model.view = crdsViewList
	model.err = errors.New("failed")
	model.crds = nil
	if body := model.renderCRDListBody(); body != "" {
		t.Fatalf("list error body = %q", body)
	}
	model.err = nil
	model.crds = []service.CRD{{Name: "widgets.example.com"}}
	model.crdsTable.SetRows(model.crdsRows())
	if body := stripAnsiForTest(model.renderCRDListBody()); !strings.Contains(body, "widgets.example.com") {
		t.Fatalf("CRD table body = %q", body)
	}
}
