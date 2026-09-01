package analysispanel

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/HediAbed/opsmate/internal/analysis"
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
	message, ok := command().(ResultMsg)
	if !ok || message.RequestID != 42 || message.Payload != "result" {
		t.Fatalf("wrapped message = %#v", message)
	}
}

func TestAnalysisPanelRejectsStaleRequestAndSanitizesCurrentResponse(t *testing.T) {
	model := NewAnalysisPanelModel()
	model.SetSize(80, 24)
	model.requestID = 7
	model.loading = true
	model.setLastQuery("status")

	model, _ = model.Update(ResultMsg{
		RequestID: 6,
		Payload:   analysis.AnalysisMsg{Response: "stale"},
	})
	if !model.loading || model.response != "" {
		t.Fatalf("stale analysis result changed state: loading=%v response=%q", model.loading, model.response)
	}
	model, _ = model.Update(ResultMsg{
		RequestID: 7,
		Payload:   analysis.AnalysisMsg{Response: "safe\x1b]52;c;payload\a response"},
	})
	if model.loading || model.response != "safe response" {
		t.Fatalf("current analysis result = loading %v, response %q", model.loading, model.response)
	}
}

func TestAnalysisPanelRetryResubmissionRejectsEarlierResult(t *testing.T) {
	panel := newRetryContractPanel()
	panel, first := completeFailedAnalysisRequest(t, panel)
	second := resubmitFailedAnalysisRequest(t, &panel, first.RequestID)
	panel = applyStaleAnalysisResult(t, panel, first)
	assertCurrentAnalysisResult(t, panel, second)
}

func newRetryContractPanel() AnalysisPanelModel {
	panel := NewAnalysisPanelModel()
	panel.SetSize(80, 24)
	panel.hasProvider = func() bool { return true }
	panel.supportsStreaming = func() bool { return false }
	panel.screenContext = "resource context"
	analysisCalls := 0
	panel.analyze = func(string, string) tea.Cmd {
		analysisCalls++
		if analysisCalls == 1 {
			return func() tea.Msg { return analysis.AnalysisMsg{Err: errors.New("unavailable")} }
		}
		return func() tea.Msg { return analysis.AnalysisMsg{Response: "fresh"} }
	}
	return panel
}

func completeFailedAnalysisRequest(
	t *testing.T,
	panel AnalysisPanelModel,
) (AnalysisPanelModel, ResultMsg) {
	t.Helper()
	panel.input.SetValue("why is this failing?")
	result := analysisResultFromBatch(t, panel.submitInput())
	panel, _ = panel.Update(result)
	if panel.loading {
		t.Fatal("failed analysis request remained loading")
	}
	if len(panel.history) != 1 {
		t.Fatalf("failed analysis history = %+v", panel.history)
	}
	if !strings.HasPrefix(panel.history[0].Response, "Error:") {
		t.Fatalf("failed analysis response = %q", panel.history[0].Response)
	}
	return panel, result
}

func resubmitFailedAnalysisRequest(
	t *testing.T,
	panel *AnalysisPanelModel,
	previousRequestID uint64,
) ResultMsg {
	t.Helper()
	if command := panel.retryLastQuery(); command != nil {
		t.Fatalf("retry returned command %v", command)
	}
	if panel.input.Value() != "why is this failing?" {
		t.Fatalf("retry restored input %q", panel.input.Value())
	}
	result := analysisResultFromBatch(t, panel.submitInput())
	if result.RequestID <= previousRequestID {
		t.Fatalf("retry request ID = %d, first = %d", result.RequestID, previousRequestID)
	}
	return result
}

func applyStaleAnalysisResult(
	t *testing.T,
	panel AnalysisPanelModel,
	first ResultMsg,
) AnalysisPanelModel {
	t.Helper()
	stale := first
	stale.Payload = analysis.AnalysisMsg{Response: "stale"}
	if !panel.Accepts(stale) {
		t.Fatal("analysis panel did not claim its routed result type")
	}
	panel, _ = panel.Update(stale)
	if !panel.loading {
		t.Fatal("stale result ended the current request")
	}
	if panel.response == "stale" {
		t.Fatal("stale result replaced the current response")
	}
	if len(panel.history) != 2 {
		t.Fatalf("stale result changed history: %+v", panel.history)
	}
	if panel.history[1].Response != "" {
		t.Fatalf("stale result completed retry history: %+v", panel.history)
	}
	return panel
}

func assertCurrentAnalysisResult(t *testing.T, panel AnalysisPanelModel, current ResultMsg) {
	t.Helper()
	panel, _ = panel.Update(current)
	if panel.loading {
		t.Fatal("current retry result remained loading")
	}
	if panel.response != "fresh" {
		t.Fatalf("current retry response = %q", panel.response)
	}
	if panel.history[1].Response != "fresh" {
		t.Fatalf("current retry history = %+v", panel.history)
	}
}

func analysisResultFromBatch(t *testing.T, command tea.Cmd) ResultMsg {
	t.Helper()
	if command == nil {
		t.Fatal("analysis request command is nil")
	}
	batch, ok := command().(tea.BatchMsg)
	if !ok {
		t.Fatal("analysis request command did not return a batch")
	}
	for _, batchedCommand := range batch {
		if batchedCommand == nil {
			continue
		}
		if result, ok := batchedCommand().(ResultMsg); ok {
			return result
		}
	}
	t.Fatal("analysis request batch did not contain a result")
	return ResultMsg{}
}

func TestAnalysisPanelDoesNotStartOverlappingRequest(t *testing.T) {
	model := NewAnalysisPanelModel()
	model.input.Focus()
	model.input.SetValue("second request")
	model.loading = true
	model.requestID = 3

	command := model.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	if command != nil {
		t.Fatal("overlapping request returned a command")
	}
	if model.requestID != 3 || model.input.Value() != "second request" || len(model.history) != 0 {
		t.Fatalf("overlapping request changed state: id=%d input=%q history=%+v", model.requestID, model.input.Value(), model.history)
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

func TestRetryLastQueryIgnoresFailedEntryWithoutQuery(t *testing.T) {
	panel := NewAnalysisPanelModel()
	panel.history = []historyEntry{{Response: "Error: failed"}}
	if command := panel.retryLastQuery(); command != nil || panel.input.Value() != "" {
		t.Fatal("empty failed query changed the input")
	}
}
