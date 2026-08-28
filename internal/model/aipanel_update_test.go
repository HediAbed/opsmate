package model

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/service"
)

func TestAIPanelUpdate_WindowSize(t *testing.T) {
	m := NewAIPanelModel()
	out, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 30})
	if out.width != 60 || out.height != 30 {
		t.Errorf("WindowSize not applied; got %dx%d", out.width, out.height)
	}
}

func TestAIPanelUpdate_AnalysisMsg_SuccessClearsLoading(t *testing.T) {
	m := NewAIPanelModel()
	m.SetSize(80, 30)
	m.loading = true
	m.setLastQuery("explain this")
	out, _ := m.Update(service.AnalysisMsg{Response: "OOM detected"})
	if out.loading {
		t.Error("AnalysisMsg should clear loading")
	}
	if out.response == "" {
		t.Error("response should be populated")
	}
}

func TestAIPanelUpdate_AnalysisMsg_ErrorPath(t *testing.T) {
	m := NewAIPanelModel()
	m.SetSize(80, 30)
	m.loading = true
	m.setLastQuery("q")
	out, _ := m.Update(service.AnalysisMsg{Err: errStub("boom")})
	if out.err == nil {
		t.Error("err should propagate")
	}
}

func TestAIPanelUpdate_GeneratedCommandMsg_SuccessOpensConfirm(t *testing.T) {
	m := NewAIPanelModel()
	m.SetSize(80, 30)
	m.loading = true
	out, _ := m.Update(service.GeneratedCommandMsg{Command: "kubectl get pods", Explanation: "lists pods"})
	if !out.showConfirm {
		t.Error("GeneratedCommandMsg should show confirm")
	}
	if out.pendingCommand != "kubectl get pods" {
		t.Errorf("pendingCommand = %q", out.pendingCommand)
	}
}

func TestAIPanelUpdate_GeneratedCommandMsg_ErrorPath(t *testing.T) {
	m := NewAIPanelModel()
	m.SetSize(80, 30)
	m.loading = true
	out, _ := m.Update(service.GeneratedCommandMsg{Err: errStub("rate limited")})
	if out.err == nil {
		t.Error("GeneratedCommandMsg err should propagate")
	}
}

func TestAIPanelUpdate_CommandResultMsg_Success(t *testing.T) {
	m := NewAIPanelModel()
	m.SetSize(80, 30)
	m.setLastQuery("get pods")
	out, _ := m.Update(service.CommandResultMsg{Output: "pod1\npod2"})
	if out.loading {
		t.Error("CommandResultMsg should clear loading")
	}
	if len(out.history) == 0 || out.history[len(out.history)-1].Response == "" {
		t.Error("CommandResultMsg should populate the last history entry")
	}
	if !out.history[len(out.history)-1].localOnly {
		t.Error("local command output must be excluded from later provider requests")
	}
}

func TestAIPanelUpdate_CommandResultMsg_Error(t *testing.T) {
	m := NewAIPanelModel()
	m.SetSize(80, 30)
	m.setLastQuery("get pods")
	out, _ := m.Update(service.CommandResultMsg{Err: errStub("failed"), Output: "stderr"})
	if out.err == nil {
		t.Error("CommandResultMsg err should propagate")
	}
}

func TestAIPanelUpdate_SpinnerTick_LoadingProgresses(_ *testing.T) {
	m := NewAIPanelModel()
	m.SetSize(80, 30)
	m.loading = true
	_, _ = m.Update(m.spinner.Tick())
}

func TestAIPanelUpdate_MouseClickFocusesInputWhenAtBottom(t *testing.T) {
	m := NewAIPanelModel()
	m.SetSize(80, 30)
	m.input.Blur()
	innerH := m.innerHeight()
	out, _ := m.Update(tea.MouseClickMsg{X: 5, Y: innerH - 2, Button: tea.MouseLeft})
	if !out.input.Focused() {
		t.Error("click near bottom should focus input")
	}
}

func TestAIPanelUpdate_MouseClickHigherUpBlursInput(t *testing.T) {
	m := NewAIPanelModel()
	m.SetSize(80, 30)
	m.input.Focus()
	out, _ := m.Update(tea.MouseClickMsg{X: 5, Y: 1, Button: tea.MouseLeft})
	if out.input.Focused() {
		t.Error("click higher up should blur input")
	}
}

func TestAIPanelUpdate_MouseWheelForwardsToViewport(t *testing.T) {
	m := NewAIPanelModel()
	m.SetSize(80, 10)
	m.input.Blur()
	m.responseView.SetContent(strings.Repeat("line\n", 100))
	m.responseView.GotoTop()
	out, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if out.responseView.YOffset() == 0 {
		t.Fatal("mouse wheel did not scroll the response viewport")
	}
}
