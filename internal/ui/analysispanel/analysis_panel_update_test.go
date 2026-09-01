package analysispanel

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/HediAbed/opsmate/internal/analysis"
)

func TestAnalysisPanelUpdate_WindowSize(t *testing.T) {
	m := NewAnalysisPanelModel()
	out, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 30})
	if out.width != 60 || out.height != 30 {
		t.Errorf("WindowSize not applied; got %dx%d", out.width, out.height)
	}
}

func TestAnalysisPanelUpdate_AnalysisMsg_SuccessClearsLoading(t *testing.T) {
	m := NewAnalysisPanelModel()
	m.SetSize(80, 30)
	m.loading = true
	m.setLastQuery("explain this")
	out, _ := m.Update(analysis.AnalysisMsg{Response: "OOM detected"})
	if out.loading {
		t.Error("AnalysisMsg should clear loading")
	}
	if out.response == "" {
		t.Error("response should be populated")
	}
}

func TestAnalysisPanelUpdate_AnalysisMsg_ErrorPath(t *testing.T) {
	m := NewAnalysisPanelModel()
	m.SetSize(80, 30)
	m.loading = true
	m.setLastQuery("q")
	out, _ := m.Update(analysis.AnalysisMsg{Err: errStub("boom")})
	if out.err == nil {
		t.Error("err should propagate")
	}
}

func TestAnalysisPanelUpdateGeneratedCommandShowsSuggestion(t *testing.T) {
	m := NewAnalysisPanelModel()
	m.SetSize(80, 30)
	m.loading = true
	m.setLastQuery("list pods")
	out, _ := m.Update(analysis.GeneratedCommandMsg{Command: "kubectl get pods", Explanation: "lists pods"})
	if out.loading {
		t.Error("GeneratedCommandMsg should clear loading")
	}
	if !strings.Contains(out.response, "Suggested command") || !strings.Contains(out.response, "kubectl get pods") || !strings.Contains(out.response, "lists pods") {
		t.Fatalf("generated response = %q", out.response)
	}
}

func TestAnalysisPanelUpdate_GeneratedCommandMsg_ErrorPath(t *testing.T) {
	m := NewAnalysisPanelModel()
	m.SetSize(80, 30)
	m.loading = true
	out, _ := m.Update(analysis.GeneratedCommandMsg{Err: errStub("rate limited")})
	if out.err == nil {
		t.Error("GeneratedCommandMsg err should propagate")
	}
}

func TestAnalysisPanelGeneratedCommandFailureStaysRetryable(t *testing.T) {
	m := NewAnalysisPanelModel()
	m.SetSize(80, 30)
	m.loading = true
	m.setLastQuery("scale my deployment")

	out, _ := m.Update(analysis.GeneratedCommandMsg{Err: errStub("rate limited")})
	if out.lastFailedEntry() == nil {
		t.Fatalf("failed command wrote no retryable history: %+v", out.history)
	}
	if !strings.Contains(out.helpView(), "retry") {
		t.Error("failed command must offer the retry hint")
	}
	if command := out.retryLastQuery(); command != nil {
		t.Fatalf("retry returned command %v, want an input restore only", command)
	}
	if got := out.input.Value(); got != "scale my deployment" {
		t.Errorf("retry restored %q, want the failed query", got)
	}
}

func TestAnalysisPanelUpdate_SpinnerTick_LoadingProgresses(_ *testing.T) {
	m := NewAnalysisPanelModel()
	m.SetSize(80, 30)
	m.loading = true
	_, _ = m.Update(m.spinner.Tick())
}

func TestAnalysisPanelUpdate_MouseClickFocusesInputWhenAtBottom(t *testing.T) {
	m := NewAnalysisPanelModel()
	m.SetSize(80, 30)
	m.input.Blur()
	innerH := m.innerHeight()
	out, _ := m.Update(tea.MouseClickMsg{X: 5, Y: innerH - 2, Button: tea.MouseLeft})
	if !out.input.Focused() {
		t.Error("click near bottom should focus input")
	}
}

func TestAnalysisPanelUpdate_MouseClickHigherUpBlursInput(t *testing.T) {
	m := NewAnalysisPanelModel()
	m.SetSize(80, 30)
	m.input.Focus()
	out, _ := m.Update(tea.MouseClickMsg{X: 5, Y: 1, Button: tea.MouseLeft})
	if out.input.Focused() {
		t.Error("click higher up should blur input")
	}
}

func TestAnalysisPanelUpdate_MouseWheelForwardsToViewport(t *testing.T) {
	m := NewAnalysisPanelModel()
	m.SetSize(80, 10)
	m.input.Blur()
	m.responseView.SetContent(strings.Repeat("line\n", 100))
	m.responseView.GotoTop()
	out, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if out.responseView.YOffset() == 0 {
		t.Fatal("mouse wheel did not scroll the response viewport")
	}
}
