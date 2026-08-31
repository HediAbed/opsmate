package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/HediAbed/opsmate/internal/analysis"
	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/ui/theme"
)

func TestDashboardMetricsTickMessageHasExpectedType(t *testing.T) {
	if _, ok := newDashMetricsTickMessage(time.Time{}).(dashMetricsTickMsg); !ok {
		t.Fatal("metrics timer callback returned the wrong message")
	}
}

func TestDashboardCombinesLiveResourceErrors(t *testing.T) {
	first := errors.New("deployment failed")
	second := errors.New("event failed")
	model := newTestDashboardModel("team-a")
	model.deploymentLiveError = first
	model.eventLiveError = second
	model.syncDashboardLiveError()
	if !errors.Is(model.err, first) || !errors.Is(model.err, second) {
		t.Fatalf("combined live error = %v", model.err)
	}
}

func TestDashboardMetricsRefreshesVisibleHealthSummary(t *testing.T) {
	model := newTestDashboardModel("team-a")
	model.showHealthAnalysis = true
	model.healthAnalysisLoading = false
	command := model.applyDashboardMetrics(cluster.MetricsMsg{PodMetrics: []cluster.PodMetric{{Name: "web", CPU: "10m"}}})
	if command == nil || !model.healthAnalysisLoading {
		t.Fatal("visible health panel did not refresh after metrics")
	}
}

func TestDashboardScrollKeysPreferOverflowingBody(t *testing.T) {
	model := newTestDashboardModel("team-a")
	model.bodyView = viewport.New(viewport.WithWidth(40), viewport.WithHeight(2))
	model.bodyView.SetContent("one\ntwo\nthree\nfour\nfive")
	model.bodyView.GotoTop()
	updated, command := model.handleDashboardKey(tea.KeyPressMsg{Code: tea.KeyDown, Text: "down"})
	if command != nil || updated.bodyView.YOffset() == 0 {
		t.Fatalf("overflow scroll = command:%v offset:%d", command != nil, updated.bodyView.YOffset())
	}
}

func TestDashboardDrillDownRequiresSelectedPod(t *testing.T) {
	model := newTestDashboardModel("team-a")
	if command := model.selectedPodDrillDown(ScreenBrowser); command != nil {
		t.Fatal("empty dashboard started a drill-down")
	}
}

func TestDashboardDataCommandPreservesRequestScope(t *testing.T) {
	model := newTestDashboardModel("team-a")
	command := model.fetchDashboardData(dashboardEvents, func() tea.Msg { return "payload" })
	message, ok := command().(dashboardResultMsg)
	if !ok || message.kind != dashboardEvents || message.requestID != model.requestIDs[dashboardEvents] ||
		message.namespace != "team-a" || message.payload != "payload" {
		t.Fatalf("dashboard result scope = %#v", message)
	}
}

func TestDashboardHealthCommandPreservesRequestScope(t *testing.T) {
	model := newTestDashboardModel("team-a")
	model.pods = []cluster.Pod{{Name: "web", Namespace: "team-a", Status: "Running"}}
	command := model.fetchHealthSummary()
	message, ok := command().(dashboardHealthResultMsg)
	if !ok || message.requestID != model.healthRequestID || message.namespace != "team-a" {
		t.Fatalf("health result scope = %#v", message)
	}
	if payload, ok := message.payload.(analysis.DashboardHealthMsg); !ok || payload.Err == nil {
		t.Fatalf("provider failure payload = %#v", message.payload)
	}
}

func TestDashboardTopConsumersHandlesMissingUsageAndLongNames(t *testing.T) {
	model := newTestDashboardModel("team-a")
	model.metrics = []cluster.PodMetric{{Name: "web"}}
	model.pods = []cluster.Pod{{Name: "web", CPU: "-"}}
	if rendered := model.renderTopConsumers(40); rendered != "" {
		t.Fatalf("missing usage rendered as %q", rendered)
	}

	model.pods = []cluster.Pod{{Name: strings.Repeat("w", 80), CPU: "20m", Memory: "10Mi"}}
	rendered := stripAnsiForTest(model.renderTopConsumers(40))
	if !strings.Contains(rendered, "~") || !strings.Contains(rendered, "20m") {
		t.Fatalf("top consumers = %q", rendered)
	}
}

func TestDashboardMetricParsersRejectMalformedValues(t *testing.T) {
	for _, value := range []string{"badm", "bad"} {
		if parsed := parseMilli(value); parsed != 0 {
			t.Fatalf("parseMilli(%q) = %d", value, parsed)
		}
	}
	if ready, desired := parseReadyReplicas("ready/desired"); ready != 0 || desired != 0 {
		t.Fatalf("invalid ready replicas = %d/%d", ready, desired)
	}
}

func TestDashboardWarningBarUsesBoundedWidth(t *testing.T) {
	rendered := renderBar(0.8, 10, theme.Green, theme.DimText)
	if lipgloss.Width(rendered) != 10 {
		t.Fatalf("warning bar width = %d", lipgloss.Width(rendered))
	}
}

func TestDashboardAlertsCoverStatusesTruncationAndOverflow(t *testing.T) {
	model := newTestDashboardModel("team-a")
	model.pods = []cluster.Pod{
		{Name: strings.Repeat("a", 40), Status: "ImagePullBackOff"},
		{Name: "failed", Status: "Failed"},
		{Name: "pending", Status: "Pending"},
		{Name: "restarting", Status: "Running", Restarts: dashboardRestartAlertThreshold + 1},
		{Name: "error", Status: "Error"},
	}
	rendered := stripAnsiForTest(model.renderAlerts(30))
	for _, expected := range []string{"ImagePullBackOff", "Failed", "Pending", "Restarts", "+1 more", "~"} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("alerts do not contain %q: %q", expected, rendered)
		}
	}
}

func TestDashboardDeploymentHealthTruncatesLongNames(t *testing.T) {
	model := newTestDashboardModel("team-a")
	model.deployments = []cluster.Deployment{{Name: strings.Repeat("d", 60), Ready: "1/2", Age: "1d"}}
	if rendered := stripAnsiForTest(model.renderDeploymentHealth(30)); !strings.Contains(rendered, "~") {
		t.Fatalf("deployment health = %q", rendered)
	}
}

func TestDashboardEmptyPodTableNamesAllNamespaceScope(t *testing.T) {
	model := newTestDashboardModel("")
	model.SetSize(80, 20)
	model.loading = false
	if rendered := stripAnsiForTest(model.renderPodTable(60)); !strings.Contains(rendered, "all namespaces") {
		t.Fatalf("empty pod table = %q", rendered)
	}
}

func TestDashboardEventsRenderRepeatedCounts(t *testing.T) {
	model := newTestDashboardModel("team-a")
	model.events = []cluster.Event{{Type: "Other", Reason: "Repeated", Message: "message", Count: 3}}
	if rendered := stripAnsiForTest(model.renderEvents(80)); !strings.Contains(rendered, "(x3)") {
		t.Fatalf("events = %q", rendered)
	}
}

func TestDashboardHealthRendersErrorAndEmptyStates(t *testing.T) {
	model := newTestDashboardModel("team-a")
	model.healthAnalysisErr = errors.New("analysis failed")
	if rendered := stripAnsiForTest(model.renderHealthAnalysis(60)); !strings.Contains(rendered, "analysis failed") {
		t.Fatalf("health error = %q", rendered)
	}
	model.healthAnalysisErr = nil
	if rendered := stripAnsiForTest(model.renderHealthAnalysis(60)); !strings.Contains(rendered, "No analysis") {
		t.Fatalf("empty health = %q", rendered)
	}
}
