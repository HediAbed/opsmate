package model

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/service"
)

func TestRootRoutesBrowserResultToOwnerWhenDashboardIsVisible(t *testing.T) {
	m := freshRoot(t)
	m.screen = ScreenDashboard
	m.browser.namespace = "payments"
	m.browser.resourceType = "pods"
	m.browser.fetchRequestID = 4
	result := browserResultMsg{
		requestID:    4,
		namespace:    "payments",
		resourceType: "pods",
		payload: service.PodsMsg{Pods: []service.Pod{
			{Name: "api", Namespace: "payments"},
		}},
	}

	updated, _ := m.Update(result)
	root := updated.(RootModel)
	if len(root.browser.pods) != 1 || root.browser.pods[0].Name != "api" {
		t.Fatalf("browser did not receive its result: %+v", root.browser.pods)
	}
	if len(root.dashboard.pods) != 0 {
		t.Fatalf("browser result leaked into dashboard: %+v", root.dashboard.pods)
	}
}

type routingWatcher[T service.WatchResource] struct {
	events chan service.WatchEvent[T]
}

func newRoutingWatcher[T service.WatchResource]() *routingWatcher[T] {
	return &routingWatcher[T]{events: make(chan service.WatchEvent[T], 1)}
}

func (w *routingWatcher[T]) Events() <-chan service.WatchEvent[T] { return w.events }
func (*routingWatcher[T]) Cancel()                                {}

type assistantContextRoutingCase struct {
	name    string
	root    RootModel
	message tea.Msg
	want    string
}

func TestRootRefreshesVisibleAssistantContextAfterAcceptedOwnerResults(t *testing.T) {
	for _, test := range assistantContextRoutingCases() {
		t.Run(test.name, func(t *testing.T) {
			assertAssistantContextRefresh(t, test)
		})
	}
}

func assistantContextRoutingCases() []assistantContextRoutingCase {
	return []assistantContextRoutingCase{
		{
			name: "dashboard",
			root: dashboardRoutingRoot(),
			message: dashboardResultMsg{
				kind: dashboardPods, requestID: 2, namespace: "payments",
				payload: service.PodsMsg{Pods: []service.Pod{{Name: "dashboard-live", Namespace: "payments"}}},
			},
			want: "dashboard-live",
		},
		{
			name: "browser",
			root: browserRoutingRoot(),
			message: browserResultMsg{
				requestID: 3, namespace: "payments", resourceType: "pods",
				payload: service.PodsMsg{Pods: []service.Pod{{Name: "browser-live", Namespace: "payments"}}},
			},
			want: "browser-live",
		},
		{
			name: "logs",
			root: logsRoutingRoot(),
			message: logsResultMsg{
				requestID: 4,
				pod:       resourceIdentity{Kind: "pod", Namespace: "payments", Name: "api"},
				payload:   service.LogsMsg{Lines: []string{"logs-live"}},
			},
			want: "logs-live",
		},
	}
}

func dashboardRoutingRoot() RootModel {
	m := NewRootModel("payments")
	m.screen = ScreenDashboard
	m.dashboard.requestIDs[dashboardPods] = 2
	return m
}

func browserRoutingRoot() RootModel {
	m := NewRootModel("payments")
	m.screen = ScreenBrowser
	m.browser.fetchRequestID = 3
	return m
}

func logsRoutingRoot() RootModel {
	m := NewRootModel("payments")
	m.screen = ScreenLogs
	m.logs.setPodIdentity("api", "payments")
	m.logs.logRequestID = 4
	return m
}

func assertAssistantContextRefresh(t *testing.T, test assistantContextRoutingCase) {
	t.Helper()
	m := test.root
	m.aiPanel.SetVisible(true)
	m.aiPanel.SetScreenContext("before")

	updated, _ := m.Update(test.message)
	root := updated.(RootModel)
	if !strings.Contains(root.aiPanel.screenContext, test.want) {
		t.Fatalf("assistant context was not refreshed: %q", root.aiPanel.screenContext)
	}
}

func TestRootRefreshesVisibleAssistantContextAfterOwnedWatchEvent(t *testing.T) {
	m := NewRootModel("payments")
	m.screen = ScreenDashboard
	m.aiPanel.SetVisible(true)
	m.aiPanel.SetScreenContext("before")
	watcher := newRoutingWatcher[service.Pod]()
	m.dashboard.podWatcher.Set(watcher)
	message := supervisedWatchMsg{
		generation: m.dashboard.podWatcher.Generation(),
		payload: service.WatchEventMsg[service.Pod]{Event: service.WatchEvent[service.Pod]{
			Kind: service.WatchAdded,
			Item: service.Pod{Name: "watch-live", Namespace: "payments"},
		}},
	}

	updated, _ := m.Update(message)
	root := updated.(RootModel)
	if !strings.Contains(root.aiPanel.screenContext, "watch-live") {
		t.Fatalf("watch result did not refresh assistant context: %q", root.aiPanel.screenContext)
	}
}

func TestRootDoesNotRefreshAssistantContextForStaleOwnerResult(t *testing.T) {
	m := NewRootModel("payments")
	m.screen = ScreenBrowser
	m.aiPanel.SetVisible(true)
	m.aiPanel.SetScreenContext("retained-context")
	m.browser.fetchRequestID = 4

	updated, _ := m.Update(browserResultMsg{
		requestID: 3, namespace: "payments", resourceType: "pods",
		payload: service.PodsMsg{Pods: []service.Pod{{Name: "stale", Namespace: "payments"}}},
	})
	root := updated.(RootModel)
	if root.aiPanel.screenContext != "retained-context" {
		t.Fatalf("stale result refreshed assistant context: %q", root.aiPanel.screenContext)
	}
}

func TestProviderSummariesRequireCurrentTarget(t *testing.T) {
	t.Run("browser detail", testBrowserSummaryRequiresCurrentTarget)
	t.Run("dashboard namespace", testDashboardSummaryRequiresCurrentNamespace)
	t.Run("log line and pod", testLogExplanationRequiresCurrentTarget)
}

func testBrowserSummaryRequiresCurrentTarget(t *testing.T) {
	m := NewBrowserModel("payments")
	m.pods = []service.Pod{{Name: "api", Namespace: "payments"}}
	m.rebuildTable()
	m.state = stateDetail
	m.showDetail = true
	m.detailKind = "describe"
	m.detailContent = "current detail"
	m.detailRequestID = 3
	identity, _ := m.selectedIdentity()

	m, _ = m.Update(browserDetailSummaryResultMsg{
		requestID: 2, identity: identity, content: "old detail",
		payload: service.DescribeSummaryMsg{Summary: "stale summary"},
	})
	if m.aiSummary != "" {
		t.Fatalf("stale browser summary was applied: %q", m.aiSummary)
	}

	m, _ = m.Update(browserDetailSummaryResultMsg{
		requestID: 3, identity: identity, content: "current detail",
		payload: service.DescribeSummaryMsg{Summary: "current\x1b]0;bad\a summary"},
	})
	if m.aiSummary != "current summary" {
		t.Fatalf("current browser summary = %q", m.aiSummary)
	}
}

func testDashboardSummaryRequiresCurrentNamespace(t *testing.T) {
	m := NewDashboardModel("payments")
	m.healthRequestID = 2
	_ = m.SetNamespace("platform")
	m, _ = m.Update(dashboardHealthResultMsg{
		requestID: 2,
		namespace: "payments",
		payload:   service.DashHealthMsg{Summary: "stale health"},
	})
	if m.aiHealthSummary != "" {
		t.Fatalf("stale dashboard health was applied: %q", m.aiHealthSummary)
	}
}

func testLogExplanationRequiresCurrentTarget(t *testing.T) {
	m := NewLogsModel("payments")
	m.setPodIdentity("api", "payments")
	m.inspectMode = true
	m.filteredLines = []string{"current line"}
	m.explainRequestID = 5
	m, _ = m.Update(logExplainResultMsg{
		requestID: 4,
		pod:       resourceIdentity{Kind: "pod", Namespace: "payments", Name: "old-api"},
		line:      "old line",
		payload:   service.LogExplainMsg{Explanation: "stale explanation"},
	})
	if m.aiExplanation != "" {
		t.Fatalf("stale log explanation was applied: %q", m.aiExplanation)
	}

	m, _ = m.Update(logExplainResultMsg{
		requestID: 5,
		pod:       resourceIdentity{Kind: "pod", Namespace: "payments", Name: "api"},
		line:      "current line",
		payload:   service.LogExplainMsg{Explanation: "current\x1b]0;bad\a explanation"},
	})
	if m.aiExplanation != "current explanation" {
		t.Fatalf("current log explanation = %q", m.aiExplanation)
	}
}

func TestBrowserRejectsOlderRequestInSameScope(t *testing.T) {
	m := NewBrowserModel("payments")
	m.fetchRequestID = 2
	stale := browserResultMsg{
		requestID:    1,
		namespace:    "payments",
		resourceType: "pods",
		payload:      service.PodsMsg{Pods: []service.Pod{{Name: "stale", Namespace: "payments"}}},
	}

	m, _ = m.Update(stale)
	if len(m.pods) != 0 {
		t.Fatalf("stale browser result changed the cache: %+v", m.pods)
	}
	live := stale
	live.requestID = 2
	live.payload = service.PodsMsg{Pods: []service.Pod{{Name: "live", Namespace: "payments"}}}
	m, _ = m.Update(live)
	if len(m.pods) != 1 || m.pods[0].Name != "live" {
		t.Fatalf("current browser result was not applied: %+v", m.pods)
	}
}

func TestDashboardRejectsResultFromPriorNamespace(t *testing.T) {
	m := NewDashboardModel("platform")
	m.requestIDs[dashboardPods] = 3
	result := dashboardResultMsg{
		kind:      dashboardPods,
		requestID: 3,
		namespace: "payments",
		payload:   service.PodsMsg{Pods: []service.Pod{{Name: "api", Namespace: "payments"}}},
	}

	m, _ = m.Update(result)
	if len(m.pods) != 0 {
		t.Fatalf("prior-namespace result changed dashboard pods: %+v", m.pods)
	}
}

func TestLogsRejectsStalePodAndLogResults(t *testing.T) {
	m := NewLogsModel("payments")
	m.podListRequestID = 2
	m.setPodIdentity("api", "payments")
	m.logRequestID = 5

	m, _ = m.Update(logPodsResultMsg{
		requestID: 1,
		namespace: "payments",
		payload:   service.PodsMsg{Pods: []service.Pod{{Name: "stale", Namespace: "payments"}}},
	})
	m, _ = m.Update(logsResultMsg{
		requestID: 4,
		pod:       resourceIdentity{Kind: "pod", Namespace: "payments", Name: "api"},
		payload:   service.LogsMsg{Lines: []string{"stale"}},
	})
	if len(m.pods) != 0 || len(m.allLines) != 0 {
		t.Fatalf("stale log data was applied: pods=%+v lines=%+v", m.pods, m.allLines)
	}
}

func TestHelmAndCRDResultsRequireCurrentScope(t *testing.T) {
	helm := NewHelmModel("platform")
	helm.releasesRequestID = 2
	helm, _ = helm.Update(helmResultMsg{
		kind:      helmReleasesResult,
		requestID: 2,
		namespace: "payments",
		payload:   service.HelmReleasesMsg{Releases: []service.HelmRelease{{Name: "api"}}},
	})
	if len(helm.releases) != 0 {
		t.Fatalf("stale Helm result was applied: %+v", helm.releases)
	}

	crds := NewCRDsModel("platform")
	crds.listRequestID = 2
	crds, _ = crds.Update(crdResultMsg{
		kind:      crdListResult,
		requestID: 2,
		namespace: "payments",
		payload:   service.CRDsMsg{CRDs: []service.CRD{{Name: "widgets.example.com"}}},
	})
	if len(crds.crds) != 0 {
		t.Fatalf("stale CRD result was applied: %+v", crds.crds)
	}
}

func TestAIPanelRejectsStaleRequestAndSanitizesCurrentResponse(t *testing.T) {
	m := NewAIPanelModel()
	m.SetSize(80, 24)
	m.requestID = 7
	m.loading = true
	m.setLastQuery("status")

	m, _ = m.Update(aiRequestResultMsg{
		requestID: 6,
		payload:   service.AnalysisMsg{Response: "stale"},
	})
	if !m.loading || m.response != "" {
		t.Fatalf("stale assistant result changed state: loading=%v response=%q", m.loading, m.response)
	}
	m, _ = m.Update(aiRequestResultMsg{
		requestID: 7,
		payload:   service.AnalysisMsg{Response: "safe\x1b]52;c;payload\a response"},
	})
	if m.loading || m.response != "safe response" {
		t.Fatalf("current assistant result = loading %v, response %q", m.loading, m.response)
	}
}

func TestAIPanelDoesNotStartOverlappingRequest(t *testing.T) {
	m := NewAIPanelModel()
	m.input.Focus()
	m.input.SetValue("second request")
	m.loading = true
	m.requestID = 3

	command := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	if command != nil {
		t.Fatal("overlapping request returned a command")
	}
	if m.requestID != 3 || m.input.Value() != "second request" || len(m.history) != 0 {
		t.Fatalf("overlapping request changed state: id=%d input=%q history=%+v", m.requestID, m.input.Value(), m.history)
	}
}
