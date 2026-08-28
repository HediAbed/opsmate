package model

import (
	"errors"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/HediAbed/opsmate/internal/service"
	"github.com/HediAbed/opsmate/internal/theme"
)

func TestDashboardMetricsTickMessageHasExpectedType(t *testing.T) {
	if _, ok := newDashMetricsTickMessage(time.Time{}).(dashMetricsTickMsg); !ok {
		t.Fatal("metrics timer callback returned the wrong message")
	}
}

func TestDashboardRoutesTypedWatchAndClosedMessages(t *testing.T) {
	model := NewDashboardModel("team-a")
	model.SetSize(100, 24)

	updated, command, handled := model.updateDashboardWatchMessage(service.WatchEventMsg[service.Pod]{Event: service.WatchEvent[service.Pod]{
		Kind: service.WatchAdded,
		Item: service.Pod{Name: "web", Namespace: "team-a"},
	}})
	if !handled || command != nil || len(updated.pods) != 1 {
		t.Fatalf("pod watch result = handled:%t command:%v pods:%d", handled, command != nil, len(updated.pods))
	}
	model = updated
	updated, command, handled = model.updateDashboardWatchMessage(service.WatchEventMsg[service.Deployment]{Event: service.WatchEvent[service.Deployment]{
		Kind: service.WatchAdded,
		Item: service.Deployment{Name: "web", Namespace: "team-a"},
	}})
	if !handled || command != nil || len(updated.deployments) != 1 {
		t.Fatalf("deployment watch result = handled:%t command:%v deployments:%d", handled, command != nil, len(updated.deployments))
	}

	for _, message := range []tea.Msg{
		dashboardPodWatchClosedMsg{},
		dashboardDeploymentWatchClosedMsg{},
		dashboardEventWatchClosedMsg{},
	} {
		_, command, handled = updated.updateDashboardWatchMessage(message)
		if !handled || command != nil {
			t.Fatalf("inactive close result for %T = handled:%t command:%v", message, handled, command != nil)
		}
	}
}

func TestDashboardSecondaryFetchErrorsPreserveFirstFailure(t *testing.T) {
	first := errors.New("deployment failed")
	second := errors.New("event failed")
	model := NewDashboardModel("team-a")
	model.applyDashboardDeployments(service.DeploymentsMsg{Err: first})
	if !errors.Is(model.err, first) {
		t.Fatalf("deployment error = %v", model.err)
	}
	model.applyDashboardEvents(service.EventsMsg{Err: second})
	if !errors.Is(model.err, first) {
		t.Fatalf("event error replaced first failure: %v", model.err)
	}

	model.err = nil
	model.applyDashboardEvents(service.EventsMsg{Err: second})
	if !errors.Is(model.err, second) {
		t.Fatalf("event error = %v", model.err)
	}
	model.applyDashboardDeployments(service.DeploymentsMsg{Deployments: []service.Deployment{{Name: "web"}}})
	model.applyDashboardEvents(service.EventsMsg{Events: []service.Event{{Name: "scheduled"}}})
	if len(model.deployments) != 1 || len(model.events) != 1 {
		t.Fatalf("successful secondary fetches = deployments:%d events:%d", len(model.deployments), len(model.events))
	}
}

func TestDashboardMetricsRefreshesVisibleHealthSummary(t *testing.T) {
	model := NewDashboardModel("team-a")
	model.showAIHealth = true
	model.aiHealthLoading = false
	command := model.applyDashboardMetrics(service.MetricsMsg{PodMetrics: []service.PodMetric{{Name: "web", CPU: "10m"}}})
	if command == nil || !model.aiHealthLoading {
		t.Fatal("visible health panel did not refresh after metrics")
	}
}

func TestDashboardScrollKeysPreferOverflowingBody(t *testing.T) {
	model := NewDashboardModel("team-a")
	model.bodyView = viewport.New(viewport.WithWidth(40), viewport.WithHeight(2))
	model.bodyView.SetContent("one\ntwo\nthree\nfour\nfive")
	model.bodyView.GotoTop()
	updated, command := model.handleDashboardKey(tea.KeyPressMsg{Code: tea.KeyDown, Text: "down"})
	if command != nil || updated.bodyView.YOffset() == 0 {
		t.Fatalf("overflow scroll = command:%v offset:%d", command != nil, updated.bodyView.YOffset())
	}
}

func TestDashboardDrillDownRequiresSelectedPod(t *testing.T) {
	model := NewDashboardModel("team-a")
	if command := model.selectedPodDrillDown(ScreenBrowser); command != nil {
		t.Fatal("empty dashboard started a drill-down")
	}
}

func TestDashboardDataCommandPreservesRequestScope(t *testing.T) {
	model := NewDashboardModel("team-a")
	command := model.fetchDashboardData(dashboardEvents, func() tea.Msg { return "payload" })
	message, ok := command().(dashboardResultMsg)
	if !ok || message.kind != dashboardEvents || message.requestID != model.requestIDs[dashboardEvents] ||
		message.namespace != "team-a" || message.payload != "payload" {
		t.Fatalf("dashboard result scope = %#v", message)
	}
}

func TestDashboardHealthCommandPreservesRequestScope(t *testing.T) {
	model := NewDashboardModel("team-a")
	model.pods = []service.Pod{{Name: "web", Namespace: "team-a", Status: "Running"}}
	command := model.fetchHealthSummary()
	message, ok := command().(dashboardHealthResultMsg)
	if !ok || message.requestID != model.healthRequestID || message.namespace != "team-a" {
		t.Fatalf("health result scope = %#v", message)
	}
	if payload, ok := message.payload.(service.DashHealthMsg); !ok || payload.Err == nil {
		t.Fatalf("provider failure payload = %#v", message.payload)
	}
}

func TestDashboardTopConsumersHandlesMissingUsageAndLongNames(t *testing.T) {
	model := NewDashboardModel("team-a")
	model.metrics = []service.PodMetric{{Name: "web"}}
	model.pods = []service.Pod{{Name: "web", CPU: "-"}}
	if rendered := model.renderTopConsumers(40); rendered != "" {
		t.Fatalf("missing usage rendered as %q", rendered)
	}

	model.pods = []service.Pod{{Name: strings.Repeat("w", 80), CPU: "20m", Memory: "10Mi"}}
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
	model := NewDashboardModel("team-a")
	model.pods = []service.Pod{
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
	model := NewDashboardModel("team-a")
	model.deployments = []service.Deployment{{Name: strings.Repeat("d", 60), Ready: "1/2", Age: "1d"}}
	if rendered := stripAnsiForTest(model.renderDeploymentHealth(30)); !strings.Contains(rendered, "~") {
		t.Fatalf("deployment health = %q", rendered)
	}
}

func TestDashboardEmptyPodTableNamesAllNamespaceScope(t *testing.T) {
	model := NewDashboardModel("")
	model.SetSize(80, 20)
	model.loading = false
	if rendered := stripAnsiForTest(model.renderPodTable(60)); !strings.Contains(rendered, "all namespaces") {
		t.Fatalf("empty pod table = %q", rendered)
	}
}

func TestDashboardEventsRenderRepeatedCounts(t *testing.T) {
	model := NewDashboardModel("team-a")
	model.events = []service.Event{{Type: "Other", Reason: "Repeated", Message: "message", Count: 3}}
	if rendered := stripAnsiForTest(model.renderEvents(80)); !strings.Contains(rendered, "(x3)") {
		t.Fatalf("events = %q", rendered)
	}
}

func TestDashboardHealthRendersErrorAndEmptyStates(t *testing.T) {
	model := NewDashboardModel("team-a")
	model.aiHealthErr = errors.New("analysis failed")
	if rendered := stripAnsiForTest(model.renderAIHealth(60)); !strings.Contains(rendered, "analysis failed") {
		t.Fatalf("health error = %q", rendered)
	}
	model.aiHealthErr = nil
	if rendered := stripAnsiForTest(model.renderAIHealth(60)); !strings.Contains(rendered, "No analysis") {
		t.Fatalf("empty health = %q", rendered)
	}
}
