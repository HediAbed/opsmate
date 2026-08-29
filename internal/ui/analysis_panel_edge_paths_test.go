package ui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

type panelTestMarkdownRenderer struct {
	err   error
	panic bool
}

func (renderer panelTestMarkdownRenderer) Render(string) (string, error) {
	if renderer.panic {
		panic("renderer failed")
	}
	return "", renderer.err
}

func TestWrapAnalysisRequestHandlesNilAndPreservesRequestIdentity(t *testing.T) {
	if wrapAnalysisRequest(1, nil) != nil {
		t.Fatal("nil command produced a wrapper")
	}

	command := wrapAnalysisRequest(42, func() tea.Msg { return "result" })
	message, ok := command().(analysisRequestResultMsg)
	if !ok || message.requestID != 42 || message.payload != "result" {
		t.Fatalf("wrapped message = %#v", message)
	}
}

func TestAnalysisPanelInputRoutingHandlesUnknownMessagesAndFocusedClick(t *testing.T) {
	panel := NewAnalysisPanelModel()
	panel.SetSize(80, 30)
	panel.input.Focus()

	updated, command := panel.handlePanelInput(struct{}{})
	if command != nil || !updated.input.Focused() {
		t.Fatal("unknown input message changed panel state")
	}
	if command := panel.handleMouseClick(tea.MouseClickMsg{Y: panel.innerHeight() - 1}); command != nil {
		t.Fatal("clicking an already focused input returned a command")
	}
}

func TestAnalysisPanelClearKeysResetActiveConversation(t *testing.T) {
	for _, focused := range []bool{false, true} {
		panel := NewAnalysisPanelModel()
		panel.SetSize(80, 30)
		panel.history = []historyEntry{{Query: "question", Response: "answer"}}
		panel.loading = true
		panel.streaming = true
		panel.streamCancel = func() {}
		if focused {
			panel.input.Focus()
		} else {
			panel.input.Blur()
		}

		command := panel.handleKey(tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl})
		if command != nil || len(panel.history) != 0 || panel.loading || panel.streaming {
			t.Fatalf("focused=%t clear result = history:%d loading:%t streaming:%t", focused, len(panel.history), panel.loading, panel.streaming)
		}
	}
}

func TestAnalysisPanelSetLatestResponseIgnoresEmptyHistory(t *testing.T) {
	panel := NewAnalysisPanelModel()
	panel.setLatestResponse("unused")
	if len(panel.history) != 0 {
		t.Fatal("setting a response created a history entry")
	}
}

func TestAnalysisPanelViewRendersProviderAndActivityStates(t *testing.T) {
	states := []struct {
		name      string
		configure func(*AnalysisPanelModel)
		want      string
	}{
		{name: "provider", configure: func(panel *AnalysisPanelModel) { panel.providerName = "configured" }, want: "[configured]"},
		{name: "streaming", configure: func(panel *AnalysisPanelModel) { panel.streaming = true }, want: "streaming"},
		{name: "loading", configure: func(panel *AnalysisPanelModel) { panel.loading = true }, want: "thinking"},
	}
	for _, state := range states {
		t.Run(state.name, func(t *testing.T) {
			panel := NewAnalysisPanelModel()
			panel.SetVisible(true)
			panel.SetSize(80, 30)
			state.configure(&panel)
			if rendered := stripANSI(panel.View()); !strings.Contains(rendered, state.want) {
				t.Fatalf("view does not contain %q: %q", state.want, rendered)
			}
		})
	}
}

func TestMarkdownAtFallsBackAfterRendererFailures(t *testing.T) {
	panel := NewAnalysisPanelModel()
	sentinel := errors.New("render failed")

	got := panel.markdownAtWithFactory("original", 60, func(int) (markdownRenderer, error) {
		return nil, sentinel
	})
	if got != "original" {
		t.Fatalf("factory failure fallback = %q", got)
	}

	panel.glamourRenderer = nil
	got = panel.markdownAtWithFactory("original", 60, func(int) (markdownRenderer, error) {
		return panelTestMarkdownRenderer{err: sentinel}, nil
	})
	if got != "original" {
		t.Fatalf("render failure fallback = %q", got)
	}

	panel.glamourRenderer = nil
	got = panel.markdownAtWithFactory("original", 60, func(int) (markdownRenderer, error) {
		return panelTestMarkdownRenderer{panic: true}, nil
	})
	if got != "original" {
		t.Fatalf("panic fallback = %q", got)
	}
}

func TestConversationMemorySkipsIncompleteTurnsAndHonorsTotalLimit(t *testing.T) {
	panel := NewAnalysisPanelModel()
	panel.history = []historyEntry{
		{Query: "", Response: "missing query"},
		{Query: "missing response", Response: ""},
	}
	for range maxMemoryTurns - 2 {
		panel.history = append(panel.history, historyEntry{
			Query:    strings.Repeat("q", memoryQueryCharacterLimit),
			Response: strings.Repeat("r", memoryResponseCharacterLimit),
		})
	}
	panel.history = append(panel.history, historyEntry{Query: "current"})

	memory := panel.recentConversationMemory()
	if strings.Contains(memory, "missing query") || strings.Contains(memory, "missing response") {
		t.Fatalf("incomplete turn reached memory: %q", memory)
	}
	if !strings.Contains(memory, "truncated") {
		t.Fatalf("bounded memory did not report truncation: length=%d", len(memory))
	}
}

func TestTruncateRunesHandlesNonPositiveLimit(t *testing.T) {
	if got := truncateRunes("value", 0, "suffix"); got != "suffix" {
		t.Fatalf("truncated value = %q", got)
	}
}

func TestRetryLastQueryIgnoresFailedEntryWithoutQuery(t *testing.T) {
	panel := NewAnalysisPanelModel()
	panel.history = []historyEntry{{Response: "Error: failed"}}
	if command := panel.retryLastQuery(); command != nil || panel.input.Value() != "" {
		t.Fatal("empty failed query changed the input")
	}
}
