package ui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/kube"
)

func TestHelmValuesPopupOpensForSelectedRelease(t *testing.T) {
	model := helmModelWithSelectedRelease()
	updated, command := model.openValuesPopup()

	if command == nil {
		t.Fatal("opening values must issue a values request")
	}
	if !updated.valuesPopupVisible || !updated.valuesPopupLoading {
		t.Fatalf("popup state = visible %v, loading %v", updated.valuesPopupVisible, updated.valuesPopupLoading)
	}
	if updated.valuesPopupRelease != "gateway" || updated.valuesPopupNS != "edge" {
		t.Errorf("popup target = %s/%s, want edge/gateway", updated.valuesPopupNS, updated.valuesPopupRelease)
	}
	if !strings.Contains(updated.valuesPopupView.View(), loadingValuesText) {
		t.Errorf("popup did not render loading text: %q", updated.valuesPopupView.View())
	}
}

func TestHelmValuesPopupOpensFromKeyboard(t *testing.T) {
	model := helmModelWithSelectedRelease()
	updated, command := model.Update(tea.KeyPressMsg{Code: 'v', Text: "v"})
	if command == nil || !updated.valuesPopupVisible {
		t.Fatalf("v did not open popup: command=%v visible=%v", command, updated.valuesPopupVisible)
	}
}

func TestHelmValuesPopupRequiresSelection(t *testing.T) {
	model := newTestHelmModel("edge")
	updated, command := model.openValuesPopup()
	if command != nil || updated.valuesPopupVisible {
		t.Fatalf("empty selection opened popup: visible=%v command=%v", updated.valuesPopupVisible, command)
	}
}

func TestHelmValuesResponseUpdatesMatchingPopup(t *testing.T) {
	tests := []struct {
		name       string
		message    helmValuesMsg
		wantText   string
		wantErr    bool
		wantLoaded bool
	}{
		{
			name:       "values",
			message:    helmValuesMsg{Release: "gateway", Namespace: "edge", Values: "replicas: 3"},
			wantText:   "replicas: 3",
			wantLoaded: true,
		},
		{
			name:       "chart defaults",
			message:    helmValuesMsg{Release: "gateway", Namespace: "edge", Values: "  \n"},
			wantText:   "chart defaults",
			wantLoaded: true,
		},
		{
			name:       "request error",
			message:    helmValuesMsg{Release: "gateway", Namespace: "edge", Err: errors.New("access denied\x1b[31m")},
			wantText:   "access denied",
			wantErr:    true,
			wantLoaded: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model, _ := helmModelWithSelectedRelease().openValuesPopup()
			updated := model.applyHelmValues(test.message)
			if updated.valuesPopupLoading == test.wantLoaded {
				t.Errorf("loading = %v, want false", updated.valuesPopupLoading)
			}
			if (updated.valuesPopupErr != nil) != test.wantErr {
				t.Errorf("error = %v, wantErr %v", updated.valuesPopupErr, test.wantErr)
			}
			if rendered := stripAnsiForTest(updated.valuesPopupView.View()); !strings.Contains(rendered, test.wantText) {
				t.Errorf("popup = %q, want text containing %q", rendered, test.wantText)
			}
		})
	}
}

func TestHelmValuesResponseRejectsInactiveOrDifferentPopup(t *testing.T) {
	message := helmValuesMsg{Release: "gateway", Namespace: "edge", Values: "new"}
	inactive := newTestHelmModel("edge")
	if updated := inactive.applyHelmValues(message); updated.valuesPopupView.View() != inactive.valuesPopupView.View() {
		t.Error("inactive popup accepted a values response")
	}

	active, _ := helmModelWithSelectedRelease().openValuesPopup()
	original := active.valuesPopupView.View()
	wrongRelease := message
	wrongRelease.Release = "other"
	if updated := active.applyHelmValues(wrongRelease); updated.valuesPopupView.View() != original {
		t.Error("popup accepted a response for another release")
	}
	wrongNamespace := message
	wrongNamespace.Namespace = "other"
	if updated := active.applyHelmValues(wrongNamespace); updated.valuesPopupView.View() != original {
		t.Error("popup accepted a response for another namespace")
	}
}

func TestHelmValuesResultEnvelopeRejectsStaleAndAcceptsCurrent(t *testing.T) {
	model, _ := helmModelWithSelectedRelease().openValuesPopup()
	current := helmResultMsg{
		kind:      helmValuesResult,
		requestID: model.valuesRequestID,
		namespace: "edge",
		release:   "gateway",
		payload:   helmValuesMsg{Release: "gateway", Namespace: "edge", Values: "enabled: true"},
	}
	stale := current
	stale.requestID--

	unchanged, command := model.handleHelmResult(stale)
	if command != nil || unchanged.valuesPopupView.View() != model.valuesPopupView.View() {
		t.Error("stale values envelope changed the popup")
	}
	updated, command := model.handleHelmResult(current)
	if command != nil || updated.valuesPopupLoading {
		t.Fatalf("current values envelope not applied: loading=%v command=%v", updated.valuesPopupLoading, command)
	}
	if !strings.Contains(updated.valuesPopupView.View(), "enabled: true") {
		t.Errorf("current values missing from popup: %q", updated.valuesPopupView.View())
	}
}

func TestHelmValuesPopupConsumesKeysAndMouseWheel(t *testing.T) {
	model, _ := helmModelWithSelectedRelease().openValuesPopup()
	model.valuesPopupView.SetContent(strings.Repeat("line\n", 40))

	scrolled, _ := model.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if scrolled.valuesPopupView.YOffset() <= model.valuesPopupView.YOffset() {
		t.Errorf("popup did not scroll: before=%d after=%d", model.valuesPopupView.YOffset(), scrolled.valuesPopupView.YOffset())
	}
	scrolled, _ = scrolled.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if scrolled.valuesPopupView.YOffset() == 0 {
		t.Error("popup did not consume navigation key")
	}
	closed, _ := scrolled.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if closed.valuesPopupVisible {
		t.Error("escape did not close values popup")
	}

	model, _ = helmModelWithSelectedRelease().openValuesPopup()
	closed, _ = model.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if closed.valuesPopupVisible {
		t.Error("q did not close values popup")
	}
}

func TestHelmValuesPopupLayoutClampsToTinyTerminal(t *testing.T) {
	model := newTestHelmModel("edge")
	model.SetSize(3, 2)
	if model.valuesPopupView.Width() >= model.width {
		t.Errorf("popup width %d must fit terminal width %d", model.valuesPopupView.Width(), model.width)
	}
	if model.valuesPopupView.Height() != 1 {
		t.Errorf("tiny popup height = %d, want 1", model.valuesPopupView.Height())
	}

	model.SetSize(120, 40)
	if model.valuesPopupView.Width() <= 0 || model.valuesPopupView.Height() <= 1 {
		t.Fatalf("normal popup dimensions invalid: %dx%d", model.valuesPopupView.Width(), model.valuesPopupView.Height())
	}
}

func TestHelmValuesPopupRendersAboveReleaseList(t *testing.T) {
	model, _ := helmModelWithSelectedRelease().openValuesPopup()
	model.valuesPopupView.SetContent("replicas: 3")
	rendered := stripAnsiForTest(model.View())
	for _, expected := range []string{"HELM RELEASES", "VALUES · gateway", "loading", "replicas: 3", "esc close"} {
		if !strings.Contains(rendered, expected) {
			t.Errorf("view missing %q", expected)
		}
	}
}

func TestHelmFetchReleasesWrapsSuccessfulResponse(t *testing.T) {
	model := newTestHelmModel("edge")
	model.commands = &testHelmCommands{listReleases: func(namespace string) tea.Cmd {
		if namespace != "edge" {
			t.Errorf("release namespace = %q, want edge", namespace)
		}
		return func() tea.Msg {
			return helmReleasesMsg{Releases: []kube.HelmRelease{{
				Name: "gateway", Namespace: "edge", Revision: 2, Status: "deployed",
			}}}
		}
	}}
	releasesEnvelope := model.fetchReleases()().(helmResultMsg)
	releases, ok := releasesEnvelope.payload.(helmReleasesMsg)
	if !ok || releases.Err != nil || len(releases.Releases) != 1 {
		t.Fatalf("release response = %#v, error = %v", releasesEnvelope.payload, releases.Err)
	}
	if releasesEnvelope.requestID != model.releasesRequestID || releasesEnvelope.namespace != "edge" {
		t.Errorf("release envelope metadata = %+v", releasesEnvelope)
	}
}

func TestHelmFetchValuesWrapsSuccessfulResponse(t *testing.T) {
	model := newTestHelmModel("edge")
	model.commands = &testHelmCommands{getValues: func(reference kube.HelmReleaseReference) tea.Cmd {
		if reference.Name != "gateway" || reference.Namespace != "edge" {
			t.Errorf("values target = %s/%s, want edge/gateway", reference.Namespace, reference.Name)
		}
		return func() tea.Msg {
			return helmValuesMsg{
				Release: reference.Name, Namespace: reference.Namespace, Values: "replicas: 3\n",
			}
		}
	}}
	valuesEnvelope := model.fetchValues("gateway", "edge")().(helmResultMsg)
	values, ok := valuesEnvelope.payload.(helmValuesMsg)
	if !ok || values.Err != nil || !strings.Contains(values.Values, "replicas: 3") {
		t.Fatalf("values response = %#v", valuesEnvelope.payload)
	}
	if valuesEnvelope.requestID != model.valuesRequestID || valuesEnvelope.release != "gateway" {
		t.Errorf("values envelope metadata = %+v", valuesEnvelope)
	}
}

func TestHelmLayoutAndFallbackRenderingPaths(t *testing.T) {
	model := helmModelWithSelectedRelease()
	plainRowY := model.tableFirstRowY()
	model.err = errors.New("cluster unavailable")
	if withBanner := model.tableFirstRowY(); withBanner <= plainRowY {
		t.Errorf("error banner did not move first row: plain=%d banner=%d", plainRowY, withBanner)
	}

	model.namespace = ""
	if title := stripAnsiForTest(model.renderTitleBar()); !strings.Contains(title, "all namespaces") {
		t.Errorf("all-namespace title = %q", title)
	}

	oversized := overlayValuesPopup("base", "popup\ncontent\nthat\nis tall", 2, 1)
	if !strings.Contains(oversized, "popup") {
		t.Errorf("oversized popup was lost: %q", oversized)
	}

	model.releases = []kube.HelmRelease{{Name: "different", Namespace: "edge"}}
	if selected := model.SelectedRelease(); selected.Name != "" {
		t.Errorf("unmatched table row resolved to %+v", selected)
	}

	updated, command := model.Update(struct{}{})
	if command != nil || updated.namespace != model.namespace {
		t.Errorf("unknown message changed model: command=%v namespace=%q", command, updated.namespace)
	}
}

func TestHelmResultKindRejectsUnknownValue(t *testing.T) {
	model := newTestHelmModel("edge")
	if model.acceptsResult(helmResultMsg{kind: helmResultKind(255)}) {
		t.Error("unknown result kind was accepted")
	}
}

func helmModelWithSelectedRelease() HelmModel {
	model := newTestHelmModel("edge")
	model.SetSize(120, 40)
	model.releases = []kube.HelmRelease{{Name: "gateway", Namespace: "edge", Revision: 2}}
	model.releaseTable.SetRows(model.currentRows())
	model.releaseTable.SetCursor(0)
	return model
}
