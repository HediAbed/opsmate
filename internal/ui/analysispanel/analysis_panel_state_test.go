package analysispanel

import (
	"strings"
	"testing"
)

func TestAnalysisPanelModel_Init_ReturnsBlinkCmd(t *testing.T) {
	if cmd := NewAnalysisPanelModel().Init(); cmd == nil {
		t.Error("Init must return a non-nil cmd (textinput.Blink)")
	}
}

func TestAnalysisPanelModel_IsVisibleMatchesSetVisible(t *testing.T) {
	m := NewAnalysisPanelModel()
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

func TestAnalysisPanelModel_FocusBlur_TogglesInputFocus(t *testing.T) {
	m := NewAnalysisPanelModel()
	m.Focus()
	if !m.input.Focused() {
		t.Error("Focus must focus the textinput")
	}
	m.Blur()
	if m.input.Focused() {
		t.Error("Blur must remove focus")
	}
}

func TestAnalysisPanelModel_SetContext_PopulatesViewport(t *testing.T) {
	m := NewAnalysisPanelModel()
	m.SetSize(80, 20)
	m.SetContext("kubectl describe output")
	if m.response != "kubectl describe output" {
		t.Errorf("response = %q, want our context", m.response)
	}
	if m.renderedResponse == "" {
		t.Error("rendered response must be populated for the viewport")
	}
}

func TestAnalysisPanelModel_SetScreenContext_StoredForLaterQueries(t *testing.T) {
	m := NewAnalysisPanelModel()
	m.SetScreenContext("on browser tab, ns=default, resource=pods")
	if !strings.Contains(m.screenContext, "browser tab") {
		t.Errorf("screenContext = %q, want our snapshot", m.screenContext)
	}
}

func TestAnalysisPanelModel_RefreshProviderName_UsesActiveProvider(t *testing.T) {
	m := NewAnalysisPanelModel()
	m.providerName = "stale"
	m.providerNameOf = func() string { return "configured" }
	m.RefreshProviderName()
	if want := "configured"; m.providerName != want {
		t.Fatalf("providerName = %q, want %q", m.providerName, want)
	}
}

func TestSetLatestResponseIfEmptySkipsEmptyHistory(t *testing.T) {
	m := NewAnalysisPanelModel()
	m.setLatestResponseIfEmpty("response")
	if len(m.history) != 0 {
		t.Fatalf("history = %+v, want empty", m.history)
	}
}

func TestSetLatestResponseIfEmptyKeepsExistingResponse(t *testing.T) {
	m := NewAnalysisPanelModel()
	m.setLastQuery("query")
	m.history[len(m.history)-1].Response = "existing"
	m.setLatestResponseIfEmpty("replacement")
	if got := m.history[len(m.history)-1].Response; got != "existing" {
		t.Fatalf("response = %q, want existing", got)
	}
}

func TestSetLatestResponseIfEmptyFillsEmptyResponse(t *testing.T) {
	m := NewAnalysisPanelModel()
	m.setLastQuery("query")
	m.setLatestResponseIfEmpty("filled")
	if got := m.history[len(m.history)-1].Response; got != "filled" {
		t.Fatalf("response = %q, want filled", got)
	}
}
