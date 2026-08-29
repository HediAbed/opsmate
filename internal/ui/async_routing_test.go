package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/analysis"
	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/kube"
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
		payload: cluster.PodsMsg{Pods: []cluster.Pod{
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

type analysisContextRoutingCase struct {
	name    string
	root    RootModel
	message tea.Msg
	want    string
}

func TestRootRefreshesVisibleAnalysisContextAfterAcceptedOwnerResults(t *testing.T) {
	for _, test := range analysisContextRoutingCases(t) {
		t.Run(test.name, func(t *testing.T) {
			assertAnalysisContextRefresh(t, test)
		})
	}
}

func analysisContextRoutingCases(t *testing.T) []analysisContextRoutingCase {
	t.Helper()
	dashboard := dashboardRoutingRoot(t)
	dashboard.dashboard.podLive.Set(newTestResourceLiveSet(resourceLiveState[cluster.Pod]{}))
	return []analysisContextRoutingCase{
		{
			name: "dashboard",
			root: dashboard,
			message: supervisedLiveMsg{
				generation: dashboard.dashboard.podLive.Generation(),
				payload: liveSnapshotMsg[cluster.Pod]{State: resourceLiveState[cluster.Pod]{
					Ready: true,
					Items: []cluster.Pod{{Name: "dashboard-live", Namespace: "payments"}},
				}},
			},
			want: "dashboard-live",
		},
		{
			name: "browser",
			root: browserRoutingRoot(t),
			message: browserResultMsg{
				requestID: 3, namespace: "payments", resourceType: "pods",
				payload: cluster.PodsMsg{Pods: []cluster.Pod{{Name: "browser-live", Namespace: "payments"}}},
			},
			want: "browser-live",
		},
		{
			name: "logs",
			root: logsRoutingRoot(t),
			message: logsResultMsg{
				requestID: 4,
				pod:       resourceIdentity{Kind: "pod", Namespace: "payments", Name: "api"},
				payload:   cluster.LogsMsg{Lines: []string{"logs-live"}},
			},
			want: "logs-live",
		},
	}
}

func dashboardRoutingRoot(t *testing.T) RootModel {
	t.Helper()
	m := newTestRootModel(t, "payments")
	m.screen = ScreenDashboard
	return m
}

func browserRoutingRoot(t *testing.T) RootModel {
	t.Helper()
	m := newTestRootModel(t, "payments")
	m.screen = ScreenBrowser
	m.browser.fetchRequestID = 3
	return m
}

func logsRoutingRoot(t *testing.T) RootModel {
	t.Helper()
	m := newTestRootModel(t, "payments")
	m.screen = ScreenLogs
	m.logs.setPodIdentity("api", "payments")
	m.logs.logRequestID = 4
	return m
}

func assertAnalysisContextRefresh(t *testing.T, test analysisContextRoutingCase) {
	t.Helper()
	m := test.root
	m.analysisPanel.SetVisible(true)
	m.analysisPanel.SetScreenContext("before")

	updated, _ := m.Update(test.message)
	root := updated.(RootModel)
	if !strings.Contains(root.analysisPanel.screenContext, test.want) {
		t.Fatalf("analysis context was not refreshed: %q", root.analysisPanel.screenContext)
	}
}

func TestRootRefreshesVisibleAnalysisContextAfterOwnedLiveSnapshot(t *testing.T) {
	m := newTestRootModel(t, "payments")
	m.screen = ScreenDashboard
	m.analysisPanel.SetVisible(true)
	m.analysisPanel.SetScreenContext("before")
	set := newTestResourceLiveSet(resourceLiveState[cluster.Pod]{Ready: true})
	m.dashboard.podLive.Set(set)
	message := supervisedLiveMsg{
		generation: m.dashboard.podLive.Generation(),
		payload: liveSnapshotMsg[cluster.Pod]{State: resourceLiveState[cluster.Pod]{
			Ready: true,
			Items: []cluster.Pod{{Name: "live-pod", Namespace: "payments"}},
		}},
	}

	updated, _ := m.Update(message)
	root := updated.(RootModel)
	if !strings.Contains(root.analysisPanel.screenContext, "live-pod") {
		t.Fatalf("live snapshot did not refresh analysis context: %q", root.analysisPanel.screenContext)
	}
}

func TestRootDoesNotRefreshAnalysisContextForStaleOwnerResult(t *testing.T) {
	m := newTestRootModel(t, "payments")
	m.screen = ScreenBrowser
	m.analysisPanel.SetVisible(true)
	m.analysisPanel.SetScreenContext("retained-context")
	m.browser.fetchRequestID = 4

	updated, _ := m.Update(browserResultMsg{
		requestID: 3, namespace: "payments", resourceType: "pods",
		payload: cluster.PodsMsg{Pods: []cluster.Pod{{Name: "stale", Namespace: "payments"}}},
	})
	root := updated.(RootModel)
	if root.analysisPanel.screenContext != "retained-context" {
		t.Fatalf("stale result refreshed analysis context: %q", root.analysisPanel.screenContext)
	}
}

func TestProviderSummariesRequireCurrentTarget(t *testing.T) {
	t.Run("browser detail", testBrowserSummaryRequiresCurrentTarget)
	t.Run("dashboard namespace", testDashboardSummaryRequiresCurrentNamespace)
	t.Run("log line and pod", testLogExplanationRequiresCurrentTarget)
}

func testBrowserSummaryRequiresCurrentTarget(t *testing.T) {
	m := newTestBrowserModel("payments")
	m.pods = []cluster.Pod{{Name: "api", Namespace: "payments"}}
	m.rebuildTable()
	m.state = stateDetail
	m.showDetail = true
	m.detailKind = "describe"
	m.detailContent = "current detail"
	m.detailRequestID = 3
	identity, _ := m.selectedIdentity()

	m, _ = m.Update(browserDetailSummaryResultMsg{
		requestID: 2, identity: identity, content: "old detail",
		payload: analysis.DescribeSummaryMsg{Summary: "stale summary"},
	})
	if m.analysisSummary != "" {
		t.Fatalf("stale browser summary was applied: %q", m.analysisSummary)
	}

	m, _ = m.Update(browserDetailSummaryResultMsg{
		requestID: 3, identity: identity, content: "current detail",
		payload: analysis.DescribeSummaryMsg{Summary: "current\x1b]0;bad\a summary"},
	})
	if m.analysisSummary != "current summary" {
		t.Fatalf("current browser summary = %q", m.analysisSummary)
	}
}

func testDashboardSummaryRequiresCurrentNamespace(t *testing.T) {
	m := newTestDashboardModel("payments")
	m.healthRequestID = 2
	_ = m.SetNamespace("platform")
	m, _ = m.Update(dashboardHealthResultMsg{
		requestID: 2,
		namespace: "payments",
		payload:   analysis.DashboardHealthMsg{Summary: "stale health"},
	})
	if m.healthAnalysisSummary != "" {
		t.Fatalf("stale dashboard health was applied: %q", m.healthAnalysisSummary)
	}
}

func testLogExplanationRequiresCurrentTarget(t *testing.T) {
	m := newTestLogsModel("payments")
	m.setPodIdentity("api", "payments")
	m.inspectMode = true
	m.filteredLines = []string{"current line"}
	m.explainRequestID = 5
	m, _ = m.Update(logExplainResultMsg{
		requestID: 4,
		pod:       resourceIdentity{Kind: "pod", Namespace: "payments", Name: "old-api"},
		line:      "old line",
		payload:   analysis.LogExplanationMsg{Explanation: "stale explanation"},
	})
	if m.lineExplanation != "" {
		t.Fatalf("stale log explanation was applied: %q", m.lineExplanation)
	}

	m, _ = m.Update(logExplainResultMsg{
		requestID: 5,
		pod:       resourceIdentity{Kind: "pod", Namespace: "payments", Name: "api"},
		line:      "current line",
		payload:   analysis.LogExplanationMsg{Explanation: "current\x1b]0;bad\a explanation"},
	})
	if m.lineExplanation != "current explanation" {
		t.Fatalf("current log explanation = %q", m.lineExplanation)
	}
}

func TestBrowserRejectsOlderRequestInSameScope(t *testing.T) {
	m := newTestBrowserModel("payments")
	m.fetchRequestID = 2
	stale := browserResultMsg{
		requestID:    1,
		namespace:    "payments",
		resourceType: "pods",
		payload:      cluster.PodsMsg{Pods: []cluster.Pod{{Name: "stale", Namespace: "payments"}}},
	}

	m, _ = m.Update(stale)
	if len(m.pods) != 0 {
		t.Fatalf("stale browser result changed the cache: %+v", m.pods)
	}
	live := stale
	live.requestID = 2
	live.payload = cluster.PodsMsg{Pods: []cluster.Pod{{Name: "live", Namespace: "payments"}}}
	m, _ = m.Update(live)
	if len(m.pods) != 1 || m.pods[0].Name != "live" {
		t.Fatalf("current browser result was not applied: %+v", m.pods)
	}
}

func TestDashboardRejectsResultFromPriorNamespace(t *testing.T) {
	m := newTestDashboardModel("platform")
	m.requestIDs[dashboardPods] = 3
	result := dashboardResultMsg{
		kind:      dashboardPods,
		requestID: 3,
		namespace: "payments",
		payload:   cluster.PodsMsg{Pods: []cluster.Pod{{Name: "api", Namespace: "payments"}}},
	}

	m, _ = m.Update(result)
	if len(m.pods) != 0 {
		t.Fatalf("prior-namespace result changed dashboard pods: %+v", m.pods)
	}
}

func TestLogsRejectsStalePodAndLogResults(t *testing.T) {
	m := newTestLogsModel("payments")
	m.podListRequestID = 2
	m.setPodIdentity("api", "payments")
	m.logRequestID = 5

	m, _ = m.Update(logPodsResultMsg{
		requestID: 1,
		namespace: "payments",
		payload:   cluster.PodsMsg{Pods: []cluster.Pod{{Name: "stale", Namespace: "payments"}}},
	})
	m, _ = m.Update(logsResultMsg{
		requestID: 4,
		pod:       resourceIdentity{Kind: "pod", Namespace: "payments", Name: "api"},
		payload:   cluster.LogsMsg{Lines: []string{"stale"}},
	})
	if len(m.pods) != 0 || len(m.allLines) != 0 {
		t.Fatalf("stale log data was applied: pods=%+v lines=%+v", m.pods, m.allLines)
	}
}

func TestHelmAndCRDResultsRequireCurrentScope(t *testing.T) {
	helm := newTestHelmModel("platform")
	helm.releasesRequestID = 2
	helm, _ = helm.Update(helmResultMsg{
		kind:      helmReleasesResult,
		requestID: 2,
		namespace: "payments",
		payload:   helmReleasesMsg{Releases: []kube.HelmRelease{{Name: "api"}}},
	})
	if len(helm.releases) != 0 {
		t.Fatalf("stale Helm result was applied: %+v", helm.releases)
	}

	crds := newTestCRDsModel("platform")
	crds.listRequestID = 2
	crds, _ = crds.Update(crdResultMsg{
		kind:      crdListResult,
		requestID: 2,
		namespace: "payments",
		payload:   cluster.CRDsMsg{CRDs: []cluster.CRD{{Name: "widgets.example.com"}}},
	})
	if len(crds.crds) != 0 {
		t.Fatalf("stale CRD result was applied: %+v", crds.crds)
	}
}

func TestAnalysisPanelRejectsStaleRequestAndSanitizesCurrentResponse(t *testing.T) {
	m := NewAnalysisPanelModel()
	m.SetSize(80, 24)
	m.requestID = 7
	m.loading = true
	m.setLastQuery("status")

	m, _ = m.Update(analysisRequestResultMsg{
		requestID: 6,
		payload:   analysis.AnalysisMsg{Response: "stale"},
	})
	if !m.loading || m.response != "" {
		t.Fatalf("stale analysis result changed state: loading=%v response=%q", m.loading, m.response)
	}
	m, _ = m.Update(analysisRequestResultMsg{
		requestID: 7,
		payload:   analysis.AnalysisMsg{Response: "safe\x1b]52;c;payload\a response"},
	})
	if m.loading || m.response != "safe response" {
		t.Fatalf("current analysis result = loading %v, response %q", m.loading, m.response)
	}
}

func TestAnalysisPanelDoesNotStartOverlappingRequest(t *testing.T) {
	m := NewAnalysisPanelModel()
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
