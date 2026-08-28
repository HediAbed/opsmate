package model

import (
	"strings"
	"testing"

	"github.com/HediAbed/opsmate/internal/service"
)

func TestSelectedPodNS_SingleNamespace(t *testing.T) {
	m := NewLogsModel("production")
	m.selectedPod = "web-abc123"
	m.pods = []service.Pod{
		{Name: "web-abc123", Namespace: "production"},
	}

	got := m.selectedPodNS()
	if got != "production" {
		t.Errorf("selectedPodNS() = %q; want %q", got, "production")
	}
}

func TestSelectedPodNS_AllNamespaces_LooksUpPod(t *testing.T) {
	m := NewLogsModel("")
	m.selectedPod = "api-xyz789"
	m.pods = []service.Pod{
		{Name: "web-abc123", Namespace: "frontend"},
		{Name: "api-xyz789", Namespace: "backend"},
		{Name: "db-000111", Namespace: "data"},
	}

	got := m.selectedPodNS()
	if got != "backend" {
		t.Errorf("selectedPodNS() = %q; want %q", got, "backend")
	}
}

func TestSelectedPodNS_AllNamespaces_PodNotFound(t *testing.T) {
	m := NewLogsModel("")
	m.selectedPod = "ghost-pod"
	m.pods = []service.Pod{
		{Name: "web-abc123", Namespace: "frontend"},
	}

	got := m.selectedPodNS()
	if got != "" {
		t.Errorf("selectedPodNS() = %q; want empty string", got)
	}
}

func TestSelectedPodNS_AllNamespaces_NoPods(t *testing.T) {
	m := NewLogsModel("")
	m.selectedPod = "some-pod"

	got := m.selectedPodNS()
	if got != "" {
		t.Errorf("selectedPodNS() = %q; want empty string", got)
	}
}

func TestSelectedPodNS_AllNamespaces_NoSelectedPod(t *testing.T) {
	m := NewLogsModel("")
	m.pods = []service.Pod{
		{Name: "web-abc123", Namespace: "frontend"},
	}

	got := m.selectedPodNS()
	if got != "" {
		t.Errorf("selectedPodNS() = %q; want empty string", got)
	}
}

func TestColorizeLines_Empty(t *testing.T) {
	m := NewLogsModel("default")
	got := m.colorizeLines(nil)
	if got != "" {
		t.Errorf("colorizeLines(nil) = %q; want empty", got)
	}

	got = m.colorizeLines([]string{})
	if got != "" {
		t.Errorf("colorizeLines([]) = %q; want empty", got)
	}
}

func TestColorizeLines_NormalLines(t *testing.T) {
	m := NewLogsModel("default")
	lines := []string{"INFO: server started", "INFO: listening on :8080"}
	got := m.colorizeLines(lines)

	if !strings.Contains(got, "server started") {
		t.Error("colorizeLines should contain 'server started'")
	}
	if !strings.Contains(got, "listening on :8080") {
		t.Error("colorizeLines should contain 'listening on :8080'")
	}
}

func TestColorizeLines_ErrorLines(t *testing.T) {
	m := NewLogsModel("default")
	lines := []string{"ERROR: connection refused", "normal line", "FATAL: out of memory"}
	got := m.colorizeLines(lines)

	if !strings.Contains(got, "connection refused") {
		t.Error("colorizeLines should contain error line content")
	}
	if !strings.Contains(got, "out of memory") {
		t.Error("colorizeLines should contain fatal line content")
	}
}

func TestColorizeLines_WarnLines(t *testing.T) {
	m := NewLogsModel("default")
	lines := []string{"WARN: deprecated API call"}
	got := m.colorizeLines(lines)

	if !strings.Contains(got, "deprecated API call") {
		t.Error("colorizeLines should contain warn line content")
	}
}

func TestColorizeLines_DebugLines(t *testing.T) {
	m := NewLogsModel("default")
	lines := []string{"DEBUG: entering function X", "TRACE: variable y = 42"}
	got := m.colorizeLines(lines)

	if !strings.Contains(got, "entering function X") {
		t.Error("colorizeLines should contain debug line content")
	}
	if !strings.Contains(got, "variable y = 42") {
		t.Error("colorizeLines should contain trace line content")
	}
}

func TestColorizeLines_MixedSeverity(t *testing.T) {
	m := NewLogsModel("default")
	lines := []string{
		"INFO: starting",
		"WARN: slow query",
		"ERROR: timeout",
		"DEBUG: retry attempt",
	}
	got := m.colorizeLines(lines)

	for _, keyword := range []string{"starting", "slow query", "timeout", "retry attempt"} {
		if !strings.Contains(got, keyword) {
			t.Errorf("colorizeLines should contain %q", keyword)
		}
	}

	const expectedNewlineCount = 3
	lineCount := strings.Count(got, "\n")
	if lineCount != expectedNewlineCount {
		t.Errorf("colorizeLines newline count = %d; want %d", lineCount, expectedNewlineCount)
	}
}

func TestColorizeLines_CaseInsensitive(t *testing.T) {
	m := NewLogsModel("default")
	lines := []string{"error: lowercase", "Error: mixed", "ERROR: upper"}
	got := m.colorizeLines(lines)

	for _, keyword := range []string{"lowercase", "mixed", "upper"} {
		if !strings.Contains(got, keyword) {
			t.Errorf("colorizeLines should contain %q", keyword)
		}
	}
}

func TestColorizeLines_PanicDetection(t *testing.T) {
	m := NewLogsModel("default")
	lines := []string{"panic: runtime error: index out of range"}
	got := m.colorizeLines(lines)

	if !strings.Contains(got, "runtime error") {
		t.Error("colorizeLines should contain panic line content")
	}
}

func TestApplyFilter_NoFilter(t *testing.T) {
	m := NewLogsModel("default")
	m.allLines = []string{"line1", "line2", "line3"}
	m.filter = ""
	m.applyFilter()

	if len(m.filteredLines) != 3 {
		t.Errorf("applyFilter with no filter: len = %d; want 3", len(m.filteredLines))
	}
}

func TestApplyFilter_MatchingFilter(t *testing.T) {
	m := NewLogsModel("default")
	m.allLines = []string{
		"INFO: server started",
		"ERROR: connection refused",
		"INFO: request handled",
		"ERROR: timeout exceeded",
	}
	m.filter = "error"
	m.applyFilter()

	if len(m.filteredLines) != 2 {
		t.Errorf("applyFilter('error'): len = %d; want 2", len(m.filteredLines))
	}
	for _, line := range m.filteredLines {
		if !strings.Contains(strings.ToLower(line), "error") {
			t.Errorf("filtered line %q should contain 'error'", line)
		}
	}
}

func TestApplyFilter_CaseInsensitive(t *testing.T) {
	m := NewLogsModel("default")
	m.allLines = []string{"ERROR: big problem", "error: small issue", "Info: normal"}
	m.filter = "ERROR"
	m.applyFilter()

	if len(m.filteredLines) != 2 {
		t.Errorf("applyFilter('ERROR') case-insensitive: len = %d; want 2", len(m.filteredLines))
	}
}

func TestApplyFilter_NoMatch(t *testing.T) {
	m := NewLogsModel("default")
	m.allLines = []string{"INFO: all good", "DEBUG: trace"}
	m.filter = "FATAL"
	m.applyFilter()

	if len(m.filteredLines) != 0 {
		t.Errorf("applyFilter('FATAL') no match: len = %d; want 0", len(m.filteredLines))
	}
}

func TestApplyFilter_EmptyLines(t *testing.T) {
	m := NewLogsModel("default")
	m.allLines = nil
	m.filter = "test"
	m.applyFilter()

	if len(m.filteredLines) != 0 {
		t.Errorf("applyFilter on nil allLines: len = %d; want 0", len(m.filteredLines))
	}
}

func TestApplyFilter_PreservesOriginal(t *testing.T) {
	m := NewLogsModel("default")
	m.allLines = []string{"line1", "line2", "line3"}
	m.filter = "line1"
	m.applyFilter()

	if len(m.allLines) != 3 {
		t.Errorf("applyFilter should not modify allLines: len = %d; want 3", len(m.allLines))
	}
	if len(m.filteredLines) != 1 {
		t.Errorf("applyFilter: filteredLines len = %d; want 1", len(m.filteredLines))
	}
}

func TestFindPodIndex_Found(t *testing.T) {
	m := NewLogsModel("default")
	m.pods = []service.Pod{
		{Name: "pod-a", Namespace: "default"},
		{Name: "pod-b", Namespace: "default"},
		{Name: "pod-c", Namespace: "default"},
	}

	tests := []struct {
		name string
		want int
	}{
		{"pod-a", 0},
		{"pod-b", 1},
		{"pod-c", 2},
	}
	for _, tt := range tests {
		got := m.findPodIndex(tt.name)
		if got != tt.want {
			t.Errorf("findPodIndex(%q) = %d; want %d", tt.name, got, tt.want)
		}
	}
}

func TestFindPodIndex_NotFound(t *testing.T) {
	m := NewLogsModel("default")
	m.pods = []service.Pod{
		{Name: "pod-a", Namespace: "default"},
		{Name: "pod-b", Namespace: "default"},
	}

	got := m.findPodIndex("nonexistent")
	if got != 0 {
		t.Errorf("findPodIndex('nonexistent') = %d; want 0", got)
	}
}

func TestFindPodIndex_EmptyPods(t *testing.T) {
	m := NewLogsModel("default")

	got := m.findPodIndex("anything")
	if got != 0 {
		t.Errorf("findPodIndex on empty pods = %d; want 0", got)
	}
}

func TestNewLogsModel_Defaults(t *testing.T) {
	m := NewLogsModel("kube-system")

	if m.namespace != "kube-system" {
		t.Errorf("namespace = %q; want %q", m.namespace, "kube-system")
	}
	if m.tailLines != 200 {
		t.Errorf("tailLines = %d; want 200", m.tailLines)
	}
	if !m.autoScroll {
		t.Error("autoScroll should be true by default")
	}
	if m.paused {
		t.Error("paused should be false by default")
	}
	if m.selectedPod != "" {
		t.Errorf("selectedPod = %q; want empty", m.selectedPod)
	}
	if m.loading {
		t.Error("loading should be false by default")
	}
}

func TestSetPod(t *testing.T) {
	m := NewLogsModel("default")
	m.allLines = []string{"old log line"}
	m.filteredLines = []string{"old log line"}

	result := m.SetPod("new-pod")

	if result.selectedPod != "new-pod" {
		t.Errorf("SetPod: selectedPod = %q; want %q", result.selectedPod, "new-pod")
	}
	if !result.loading {
		t.Error("SetPod should set loading = true")
	}
	if result.allLines != nil {
		t.Error("SetPod should clear allLines")
	}
	if result.filteredLines != nil {
		t.Error("SetPod should clear filteredLines")
	}
}

func TestSetNamespace(t *testing.T) {
	m := NewLogsModel("default")
	m.selectedPod = "old-pod"
	m.allLines = []string{"old log"}
	m.filteredLines = []string{"old log"}

	m.SetNamespace("staging")

	if m.namespace != "staging" {
		t.Errorf("SetNamespace: namespace = %q; want %q", m.namespace, "staging")
	}
	if m.selectedPod != "" {
		t.Errorf("SetNamespace should clear selectedPod, got %q", m.selectedPod)
	}
	if m.allLines != nil {
		t.Error("SetNamespace should clear allLines")
	}
	if m.filteredLines != nil {
		t.Error("SetNamespace should clear filteredLines")
	}
}

func TestSelectedPod(t *testing.T) {
	m := NewLogsModel("default")
	if m.SelectedPod() != "" {
		t.Errorf("SelectedPod() = %q; want empty", m.SelectedPod())
	}

	m.selectedPod = "my-pod"
	if m.SelectedPod() != "my-pod" {
		t.Errorf("SelectedPod() = %q; want %q", m.SelectedPod(), "my-pod")
	}
}

func TestSetPodCmd_NoPod(t *testing.T) {
	m := NewLogsModel("default")
	cmd := m.SetPodCmd()
	if cmd != nil {
		t.Error("SetPodCmd with no selectedPod should return nil")
	}
}

func TestSetPodCmd_WithPod(t *testing.T) {
	m := NewLogsModel("default")
	m.selectedPod = "web-pod"
	cmd := m.SetPodCmd()
	if cmd == nil {
		t.Error("SetPodCmd with selectedPod should return non-nil cmd")
	}
}

func TestClassifyLine_Severities(t *testing.T) {
	tests := []struct {
		line string
		want lineSeverity
	}{
		{"FATAL: out of memory", sevCritical},
		{"panic: runtime error", sevCritical},
		{"ERROR: connection refused", sevError},
		{"error: something failed", sevError},
		{"exception in thread main", sevError},
		{"WARN: deprecated API", sevWarn},
		{"warning: slow query", sevWarn},
		{"DEBUG: entering function", sevDebug},
		{"TRACE: variable x = 5", sevDebug},
		{"  at com.example.Main.run(Main.java:42)", sevStack},
		{"goroutine 1 [running]:", sevStack},
		{"Traceback (most recent call last):", sevStack},
		{"INFO: server started", sevNone},
		{"just a normal line", sevNone},
	}
	for _, tt := range tests {
		got := classifyLine(tt.line)
		if got != tt.want {
			t.Errorf("classifyLine(%q) = %d; want %d", tt.line, got, tt.want)
		}
	}
}

func TestClassifyLine_CaseInsensitive(t *testing.T) {
	if classifyLine("Error: mixed case") != sevError {
		t.Error("classifyLine should be case-insensitive for 'Error'")
	}
	if classifyLine("PANIC: uppercase") != sevCritical {
		t.Error("classifyLine should be case-insensitive for 'PANIC'")
	}
}

func TestGetSurroundingContext_Middle(t *testing.T) {
	m := NewLogsModel("default")
	m.filteredLines = []string{"line0", "line1", "line2", "line3", "line4", "line5", "line6"}

	got := m.getSurroundingContext(3, 2)
	if !strings.Contains(got, "line1") || !strings.Contains(got, "line5") {
		t.Errorf("getSurroundingContext(3, 2) should contain line1..line5, got: %q", got)
	}
	if strings.Contains(got, "line0") || strings.Contains(got, "line6") {
		t.Errorf("getSurroundingContext(3, 2) should NOT contain line0 or line6, got: %q", got)
	}
}

func TestGetSurroundingContext_Start(t *testing.T) {
	m := NewLogsModel("default")
	m.filteredLines = []string{"line0", "line1", "line2"}

	got := m.getSurroundingContext(0, 5)
	if !strings.Contains(got, "line0") || !strings.Contains(got, "line2") {
		t.Errorf("getSurroundingContext(0, 5) should contain all lines, got: %q", got)
	}
}

func TestGetSurroundingContext_End(t *testing.T) {
	m := NewLogsModel("default")
	m.filteredLines = []string{"line0", "line1", "line2"}

	got := m.getSurroundingContext(2, 5)
	if !strings.Contains(got, "line0") || !strings.Contains(got, "line2") {
		t.Errorf("getSurroundingContext(2, 5) should contain all lines, got: %q", got)
	}
}

func TestLogsHasInputFocus_Default(t *testing.T) {
	m := NewLogsModel("default")
	if m.HasInputFocus() {
		t.Error("HasInputFocus should be false by default")
	}
}

func TestLogsHasInputFocus_InspectMode(t *testing.T) {
	m := NewLogsModel("default")
	m.inspectMode = true
	if !m.HasInputFocus() {
		t.Error("HasInputFocus should be true in inspect mode")
	}
}

func TestLogsHasInputFocus_PodPopup(t *testing.T) {
	m := NewLogsModel("default")
	m.showPodPopup = true
	if !m.HasInputFocus() {
		t.Error("HasInputFocus should be true when pod popup is shown")
	}
}

func TestNewLogsModel_InspectDefaults(t *testing.T) {
	m := NewLogsModel("default")
	if m.inspectMode {
		t.Error("inspectMode should be false by default")
	}
	if m.aiExplainLoading {
		t.Error("aiExplainLoading should be false by default")
	}
	if m.aiExplanation != "" {
		t.Errorf("aiExplanation should be empty by default, got %q", m.aiExplanation)
	}
	if m.lineCursor != 0 {
		t.Errorf("lineCursor should be 0 by default, got %d", m.lineCursor)
	}
}
