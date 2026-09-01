package browser

import (
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/ui/component"
)

func TestComputeColumns_FitsBoxAtEveryWidth(t *testing.T) {
	const minimumFillWidth = 120
	widths := []int{40, 60, 80, minimumFillWidth, 160, 200, 240, 320}

	for _, resourceType := range allResourceTypes {
		specs, ok := resourceColSpecs[resourceType]
		if !ok {
			t.Fatalf("missing column specification for %q", resourceType)
		}
		for _, tableWidth := range widths {
			name := resourceType + "_w" + strconv.Itoa(tableWidth)
			t.Run(name, func(t *testing.T) {
				assertColumnsFit(t, resourceType, tableWidth, minimumFillWidth, specs)
			})
		}
	}
}

func assertColumnsFit(t *testing.T, resourceType string, tableWidth, minimumFillWidth int, specs []component.ColumnSpec) {
	t.Helper()
	columns := computeColumns(tableWidth, specs)
	columnWidth := 0
	for _, column := range columns {
		columnWidth += column.Width
	}
	padding := component.TableCellPadding * len(specs)
	totalWidth := columnWidth + padding
	if totalWidth > tableWidth {
		t.Errorf("columns overflow: resource=%s width=%d columns=%d padding=%d total=%d",
			resourceType, tableWidth, columnWidth, padding, totalWidth)
	}
	if tableWidth >= minimumFillWidth && totalWidth != tableWidth {
		t.Errorf("columns underfill: resource=%s width=%d columns=%d padding=%d total=%d",
			resourceType, tableWidth, columnWidth, padding, totalWidth)
	}
}

func TestFormatEventsOutput_Empty(t *testing.T) {
	got := formatEventsOutput(nil)
	if got != "No events found." {
		t.Errorf("formatEventsOutput(nil) = %q; want %q", got, "No events found.")
	}

	got = formatEventsOutput([]cluster.Event{})
	if got != "No events found." {
		t.Errorf("formatEventsOutput([]) = %q; want %q", got, "No events found.")
	}
}

func TestFormatEventsOutput_NormalEvents(t *testing.T) {
	events := []cluster.Event{
		{Type: "Normal", Reason: "Scheduled", Object: "Pod/test-pod", Message: "Successfully assigned default/test-pod to node1"},
		{Type: "Warning", Reason: "BackOff", Object: "Pod/bad-pod", Message: "Back-off restarting failed container"},
	}

	got := formatEventsOutput(events)

	if !strings.Contains(got, "TYPE") || !strings.Contains(got, "REASON") || !strings.Contains(got, "OBJECT") || !strings.Contains(got, "MESSAGE") {
		t.Errorf("formatEventsOutput missing header columns, got:\n%s", got)
	}

	if !strings.Contains(got, strings.Repeat("-", 90)) {
		t.Error("formatEventsOutput missing separator line")
	}

	if !strings.Contains(got, "Normal") {
		t.Error("formatEventsOutput missing Normal event type")
	}
	if !strings.Contains(got, "Warning") {
		t.Error("formatEventsOutput missing Warning event type")
	}
	if !strings.Contains(got, "Scheduled") {
		t.Error("formatEventsOutput missing Scheduled reason")
	}
	if !strings.Contains(got, "Pod/test-pod") {
		t.Error("formatEventsOutput missing Pod/test-pod object")
	}
}

func TestFormatEventsOutput_TruncatedMessages(t *testing.T) {
	longMsg := strings.Repeat("x", 100)
	events := []cluster.Event{
		{Type: "Normal", Reason: "Test", Object: "Pod/foo", Message: longMsg},
	}

	got := formatEventsOutput(events)

	if !strings.Contains(got, "...") {
		t.Error("formatEventsOutput should truncate long messages with '...'")
	}

	if strings.Contains(got, longMsg) {
		t.Error("formatEventsOutput should not contain full long message")
	}
}

func TestFormatEventsOutput_ShortMessage(t *testing.T) {
	events := []cluster.Event{
		{Type: "Normal", Reason: "Pulled", Object: "Pod/app", Message: "Short msg"},
	}

	got := formatEventsOutput(events)

	if !strings.Contains(got, "Short msg") {
		t.Error("formatEventsOutput should show short messages in full")
	}
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 3 {
		t.Errorf("formatEventsOutput line count = %d; want 3", len(lines))
	}
}

func TestFormatEventsOutput_ExactlyAtLimit(t *testing.T) {
	msg60 := strings.Repeat("a", 60)
	events := []cluster.Event{
		{Type: "Normal", Reason: "Test", Object: "Pod/bar", Message: msg60},
	}

	got := formatEventsOutput(events)

	if !strings.Contains(got, msg60) {
		t.Error("formatEventsOutput should not truncate 60-char message")
	}
}

func TestFormatEventsOutput_JustOverLimit(t *testing.T) {
	msg61 := strings.Repeat("b", 61)
	events := []cluster.Event{
		{Type: "Normal", Reason: "Test", Object: "Pod/baz", Message: msg61},
	}

	got := formatEventsOutput(events)

	if strings.Contains(got, msg61) {
		t.Error("formatEventsOutput should truncate 61-char message")
	}
	if !strings.Contains(got, "...") {
		t.Error("formatEventsOutput should add ... to truncated message")
	}
}

func TestBrowserHasInputFocus_Default(t *testing.T) {
	m := newTestBrowserModel("default")
	if m.HasInputFocus() {
		t.Error("HasInputFocus should be false in default browsing state")
	}
}

func TestBrowserHasInputFocus_ScaleInput(t *testing.T) {
	m := newTestBrowserModel("default")
	m.state = stateScaleInput
	if !m.HasInputFocus() {
		t.Error("HasInputFocus should be true in stateScaleInput")
	}
}

func TestBrowserHasInputFocus_ScaleConfirm(t *testing.T) {
	m := newTestBrowserModel("default")
	m.state = stateScaleConfirm
	if !m.HasInputFocus() {
		t.Error("HasInputFocus should be true in stateScaleConfirm")
	}
}

func TestBrowserHasInputFocus_DeleteConfirm(t *testing.T) {
	m := newTestBrowserModel("default")
	m.state = stateDeleteConfirm
	if !m.HasInputFocus() {
		t.Error("HasInputFocus should be true in stateDeleteConfirm")
	}
}

func TestBrowserHasInputFocus_Filter(t *testing.T) {
	m := newTestBrowserModel("default")
	m.state = stateFilter
	if !m.HasInputFocus() {
		t.Error("HasInputFocus should be true in stateFilter")
	}
}

func TestBrowserHasInputFocus_AppliedFilterDoesNotCaptureGlobalKeys(t *testing.T) {
	m := newTestBrowserModel("default")
	m.state = stateFilter
	m.filterActive = true
	m.filterInput.SetValue("web")
	m, _ = m.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	if m.filterText != "web" || m.state != stateBrowsing {
		t.Fatalf("filter was not applied: text=%q state=%v", m.filterText, m.state)
	}
	if m.HasInputFocus() {
		t.Error("an applied, unfocused filter must not capture global shortcuts")
	}
}

func TestBrowserHasInputFocus_Detail(t *testing.T) {
	m := newTestBrowserModel("default")
	m.state = stateDetail
	if m.HasInputFocus() {
		t.Error("HasInputFocus should be false in stateDetail (not an input state)")
	}
}

func TestBrowser_AnalysisSummary_Defaults(t *testing.T) {
	m := newTestBrowserModel("default")
	if m.analysisSummary != "" {
		t.Errorf("aiSummary should be empty by default, got %q", m.analysisSummary)
	}
	if m.analysisSummaryLoading {
		t.Error("aiSummaryLoad should be false by default")
	}
	if m.analysisSummaryErr != nil {
		t.Errorf("aiSummaryErr should be nil by default, got %v", m.analysisSummaryErr)
	}
}

func TestBrowser_RebuildDetailContent_NoSummary(t *testing.T) {
	m := newTestBrowserModel("default")
	m.SetSize(120, 40)
	m.detailContent = "Name: test-pod\nNamespace: default"
	m.rebuildDetailContent()
	got := m.detailView.View()
	if !strings.Contains(got, "test-pod") {
		t.Error("rebuildDetailContent should contain detail content")
	}
}

func TestBrowser_RebuildDetailContent_WithSummary(t *testing.T) {
	m := newTestBrowserModel("default")
	m.SetSize(120, 40)
	m.detailContent = "Name: test-pod\nNamespace: default"
	m.analysisSummary = "Pod is running normally with no issues."
	m.rebuildDetailContent()
	got := m.detailView.View()
	if !strings.Contains(got, "running normally") {
		t.Error("rebuildDetailContent should contain analysis summary text")
	}
}
