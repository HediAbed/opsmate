package ui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/analysis"
	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/kube"
)

func TestRootInitializationRunsOnce(t *testing.T) {
	model := freshRoot(t)
	model.screen = ScreenAnalysis
	initialization := model.Init()
	if initialization == nil {
		t.Fatal("root initialization command is nil")
	}
	if _, ok := initialization().(initializeRootMsg); !ok {
		t.Fatalf("initialization command returned %T", initialization())
	}

	initializedModel, command := model.Update(initializeRootMsg{})
	initialized := initializedModel.(RootModel)
	if !initialized.initialized || command == nil {
		t.Fatalf("first initialization = initialized %v, command %v", initialized.initialized, command)
	}
	secondModel, command := initialized.Update(initializeRootMsg{})
	if command != nil || !secondModel.(RootModel).initialized {
		t.Fatalf("second initialization should be ignored: command=%v initialized=%v", command, secondModel.(RootModel).initialized)
	}
}

func TestRootRoutesAnalysisAndLogEnvelopes(t *testing.T) {
	t.Run("analysis response", func(t *testing.T) {
		model := freshRoot(t)
		model.analysisPanel.requestID = 4
		updated, _ := model.Update(analysisRequestResultMsg{requestID: 4, payload: analysis.AnalysisMsg{Response: "healthy"}})
		if response := updated.(RootModel).analysisPanel.response; response != "healthy" {
			t.Errorf("analysis response = %q", response)
		}
	})

	t.Run("pod list", func(t *testing.T) {
		model := freshRoot(t)
		model.logs.podListRequestID = 3
		model.logs.selectedPod = "current"
		message := logPodsResultMsg{requestID: 3, namespace: model.logs.namespace, payload: cluster.PodsMsg{Pods: []cluster.Pod{{Name: "worker", Namespace: "default"}}}}
		updated, _ := model.Update(message)
		if pods := updated.(RootModel).logs.pods; len(pods) != 1 || pods[0].Name != "worker" {
			t.Errorf("log pod list = %+v", pods)
		}
	})

	t.Run("container list", func(t *testing.T) {
		model := freshRoot(t)
		model.logs.SetPodInNamespace("worker", "default")
		model.logs.containerRequestID = 5
		message := containersResultMsg{
			requestID: 5,
			pod:       resourceIdentity{Kind: "pod", Namespace: "default", Name: "worker"},
			payload:   cluster.ContainersMsg{Containers: []string{"main", "sidecar"}},
		}
		updated, _ := model.Update(message)
		logs := updated.(RootModel).logs
		if !logs.showContainerPopup || len(logs.containers) != 2 {
			t.Errorf("container response not applied: popup=%v containers=%v", logs.showContainerPopup, logs.containers)
		}
	})

	t.Run("line explanation", func(t *testing.T) {
		model := freshRoot(t)
		model.logs.SetPodInNamespace("worker", "default")
		model.logs.inspectMode = true
		model.logs.filteredLines = []string{"failure"}
		model.logs.explainRequestID = 2
		message := logExplainResultMsg{
			requestID: 2,
			pod:       resourceIdentity{Kind: "pod", Namespace: "default", Name: "worker"},
			line:      "failure",
			payload:   analysis.LogExplanationMsg{Explanation: "actionable cause"},
		}
		updated, _ := model.Update(message)
		if explanation := updated.(RootModel).logs.lineExplanation; explanation != "actionable cause" {
			t.Errorf("log explanation = %q", explanation)
		}
	})
}

func TestRootRoutesScreenSpecificResultEnvelopes(t *testing.T) {
	t.Run("browser detail", func(t *testing.T) {
		model := freshRoot(t)
		model.browser.pods = []cluster.Pod{{Name: "worker", Namespace: "default"}}
		model.browser.rebuildTable()
		model.browser.state = stateDetail
		model.browser.showDetail = true
		model.browser.detailKind = "describe"
		model.browser.detailContent = "current details"
		model.browser.detailRequestID = 6
		identity, _ := model.browser.selectedIdentity()
		message := browserDetailSummaryResultMsg{requestID: 6, identity: identity, content: "current details", payload: analysis.DescribeSummaryMsg{Summary: "stable"}}
		updated, _ := model.Update(message)
		if summary := updated.(RootModel).browser.analysisSummary; summary != "stable" {
			t.Errorf("browser summary = %q", summary)
		}
	})

	t.Run("dashboard health", func(t *testing.T) {
		model := freshRoot(t)
		model.dashboard.healthRequestID = 2
		message := dashboardHealthResultMsg{requestID: 2, namespace: model.dashboard.namespace, payload: analysis.DashboardHealthMsg{Summary: "stable"}}
		updated, _ := model.Update(message)
		if summary := updated.(RootModel).dashboard.healthAnalysisSummary; summary != "stable" {
			t.Errorf("dashboard summary = %q", summary)
		}
	})

	t.Run("helm releases", func(t *testing.T) {
		model := freshRoot(t)
		model.helm.releasesRequestID = 8
		message := helmResultMsg{kind: helmReleasesResult, requestID: 8, namespace: model.helm.namespace, payload: helmReleasesMsg{Releases: []kube.HelmRelease{{Name: "gateway", Namespace: "default"}}}}
		updated, _ := model.Update(message)
		if releases := updated.(RootModel).helm.releases; len(releases) != 1 || releases[0].Name != "gateway" {
			t.Errorf("helm releases = %+v", releases)
		}
	})

	t.Run("custom resources", func(t *testing.T) {
		model := freshRoot(t)
		model.crds.listRequestID = 9
		message := crdResultMsg{kind: crdListResult, requestID: 9, namespace: model.crds.namespace, payload: cluster.CRDsMsg{CRDs: []cluster.CRD{{Name: "widgets.example.test", Resource: "widgets"}}}}
		updated, _ := model.Update(message)
		if resources := updated.(RootModel).crds.crds; len(resources) != 1 || resources[0].Resource != "widgets" {
			t.Errorf("custom resources = %+v", resources)
		}
	})
}

func TestRootRoutesStaleShellAndLiveMessagesWithoutCrossScreenEffects(t *testing.T) {
	model := freshRoot(t)
	model.browser.statusMsg = "retained"
	for _, message := range []tea.Msg{
		shellOutputMsg{SessionID: "stale", Line: "ignored"},
		shellExitMsg{SessionID: "stale"},
		supervisedLiveMsg{generation: 1, payload: liveSnapshotMsg[cluster.Pod]{}},
	} {
		updated, _ := model.Update(message)
		root := updated.(RootModel)
		if root.browser.statusMsg != "retained" || root.screen != model.screen {
			t.Errorf("%T changed unrelated root state", message)
		}
	}

	updated, command := model.Update(dashMetricsTickMsg{})
	if command == nil || updated.(RootModel).screen != model.screen {
		t.Errorf("dashboard tick was not routed: command=%v", command)
	}
}

func TestRootBroadcastsSpinnerAndContextData(t *testing.T) {
	model := freshRoot(t)
	model.screen = ScreenDashboard
	model.analysisPanel.SetVisible(true)
	model.analysisPanel.SetScreenContext("before")
	model.dashboard.pods = []cluster.Pod{{Name: "worker", Namespace: "default"}}
	updatedModel, _ := model.broadcastRootMessage(cluster.MetricsMsg{})
	updated := updatedModel.(RootModel)
	if !strings.Contains(updated.analysisPanel.screenContext, "worker") {
		t.Errorf("analysis context was not refreshed: %q", updated.analysisPanel.screenContext)
	}

	spinnerCommand := model.nsSpinner.Tick
	rawSpinnerMessage := spinnerCommand()
	spinnerMessage, ok := rawSpinnerMessage.(spinner.TickMsg)
	if !ok {
		t.Fatalf("spinner command returned %T", rawSpinnerMessage)
	}
	spunModel, command := model.broadcastRootMessage(spinnerMessage)
	if command == nil || spunModel.(RootModel).screen != ScreenDashboard {
		t.Errorf("spinner broadcast failed: command=%v", command)
	}

	model.screen = ScreenAnalysis
	model.analysisPanel.SetVisible(true)
	spunModel, command = model.broadcastRootSpinnerTick(spinnerMessage)
	if command == nil || spunModel.(RootModel).screen != ScreenAnalysis {
		t.Errorf("analysis-screen spinner failed: command=%v", command)
	}
}

func TestAnalysisContextDataMessageClassification(t *testing.T) {
	dataMessages := []tea.Msg{
		cluster.PodsMsg{}, cluster.DeploymentsMsg{}, cluster.EventsMsg{}, cluster.MetricsMsg{},
		cluster.ServicesMsg{}, cluster.StatefulSetsMsg{}, cluster.DaemonSetsMsg{},
		cluster.ConfigMapsMsg{}, cluster.NodesMsg{}, cluster.JobsMsg{}, cluster.LogsMsg{},
		cluster.DescribeMsg{},
	}
	for _, message := range dataMessages {
		if !isAnalysisContextDataMessage(message) {
			t.Errorf("%T should refresh analysis context", message)
		}
	}
	if isAnalysisContextDataMessage(struct{}{}) {
		t.Error("unrelated message classified as analysis context data")
	}
}
