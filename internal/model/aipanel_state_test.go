package model

import (
	"strings"
	"testing"

	"github.com/HediAbed/opsmate/internal/service"
)

func TestAIPanelModel_Init_ReturnsBlinkCmd(t *testing.T) {
	if cmd := NewAIPanelModel().Init(); cmd == nil {
		t.Error("Init must return a non-nil cmd (textinput.Blink)")
	}
}

func TestAIPanelModel_IsVisibleMatchesSetVisible(t *testing.T) {
	m := NewAIPanelModel()
	if m.IsVisible() {
		t.Error("new panel must start invisible")
	}
	m.SetVisible(true)
	if !m.IsVisible() {
		t.Error("SetVisible(true) → IsVisible() == true")
	}
	m.SetVisible(false)
	if m.IsVisible() {
		t.Error("SetVisible(false) → IsVisible() == false")
	}
}

func TestAIPanelModel_FocusBlur_TogglesInputFocus(t *testing.T) {
	m := NewAIPanelModel()
	m.Focus()
	if !m.input.Focused() {
		t.Error("Focus must focus the textinput")
	}
	m.Blur()
	if m.input.Focused() {
		t.Error("Blur must remove focus")
	}
}

func TestAIPanelModel_SetContext_PopulatesViewport(t *testing.T) {
	m := NewAIPanelModel()
	m.SetSize(80, 20)
	m.SetContext("kubectl describe output")
	if m.response != "kubectl describe output" {
		t.Errorf("response = %q, want our context", m.response)
	}
	if m.renderedResponse == "" {
		t.Error("rendered response must be populated for the viewport")
	}
}

func TestAIPanelModel_SetScreenContext_StoredForLaterQueries(t *testing.T) {
	m := NewAIPanelModel()
	m.SetScreenContext("on browser tab, ns=default, resource=pods")
	if !strings.Contains(m.screenContext, "browser tab") {
		t.Errorf("screenContext = %q, want our snapshot", m.screenContext)
	}
}

func TestAIPanelModel_RefreshProviderName_UsesActiveProvider(t *testing.T) {
	m := NewAIPanelModel()
	m.providerName = "stale"
	m.RefreshProviderName()
	if want := sanitizeTerminalText(service.ProviderName()); m.providerName != want {
		t.Fatalf("providerName = %q, want %q", m.providerName, want)
	}
}

func TestAIPanelModel_ClearConfirm_RemovesPendingCommand(t *testing.T) {
	m := NewAIPanelModel()
	m.pendingCommand = "kubectl delete pod x"
	m.pendingExplanation = "would delete the pod"
	m.showConfirm = true

	m.clearConfirm()
	if m.pendingCommand != "" {
		t.Error("clearConfirm should drop pendingCommand")
	}
	if m.pendingExplanation != "" {
		t.Error("clearConfirm should drop pendingExplanation")
	}
	if m.showConfirm {
		t.Error("clearConfirm should hide the confirm dialog")
	}
}
