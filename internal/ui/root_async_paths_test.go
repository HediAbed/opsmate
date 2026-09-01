package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/analysis"
	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/ui/analysispanel"
	clusterui "github.com/HediAbed/opsmate/internal/ui/cluster"
	"github.com/HediAbed/opsmate/internal/ui/screen"
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

func newTestRootModelWithAnalysis(t *testing.T, namespace, responseBody string) RootModel {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(responseBody))
	}))
	t.Cleanup(server.Close)
	t.Setenv("OPSMATE_PROVIDER_URL", server.URL)
	t.Setenv("OPSMATE_PROVIDER_MODEL", "test-model")
	t.Setenv("OPSMATE_PROVIDER_API_KEY", "test-key")
	service, err := analysis.NewServiceFromEnvironment()
	if err != nil {
		t.Fatalf("NewServiceFromEnvironment() error = %v", err)
	}
	operations := &testClusterOperations{}
	root, err := NewRootModel(namespace, RuntimeDependencies{
		Context:           context.Background(),
		ClusterContext:    &testContextManager{},
		ClusterResources:  &testResourceReader{},
		ClusterSnapshots:  &snapshotCollectorStub{},
		ClusterObserver:   &testResourceObserver{},
		ClusterOperations: operations,
		HelmReleases:      operations,
		Analysis:          service,
	})
	if err != nil {
		t.Fatalf("NewRootModel() error = %v", err)
	}
	return root
}

func TestRootRoutesAnalysisAndLogEnvelopes(t *testing.T) {
	t.Run("analysis response", func(t *testing.T) {
		model := newTestRootModelWithAnalysis(t, "default", `{"choices":[{"message":{"content":"cluster looks healthy"}}]}`)
		model.analysisPanel.SetVisible(true)
		model.analysisPanel.Focus()
		for _, key := range "status" {
			updated, _ := model.Update(tea.KeyPressMsg{Code: key, Text: string(key)})
			model = updated.(RootModel)
		}
		updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
		model = updated.(RootModel)
		if command == nil {
			t.Fatal("analysis submit returned no command")
		}
		var result analysispanel.ResultMsg
		found := false
		for _, message := range commandMessages(t, command) {
			if candidate, ok := message.(analysispanel.ResultMsg); ok {
				result, found = candidate, true
			}
		}
		if !found || result.RequestID == 0 {
			t.Fatalf("analysis request result = %#v", result)
		}
		updated, _ = model.Update(result)
		if response := updated.(RootModel).analysisPanel.Response(); !strings.Contains(response, "cluster looks healthy") {
			t.Fatalf("analysis response = %q", response)
		}
	})

	t.Run("log response", func(t *testing.T) {
		model := freshRoot(t)
		model.logs.SetPodInNamespace("worker", "default")
		command := model.logs.SetPodCmd()
		if command == nil {
			t.Fatal("log request command is nil")
		}
		updated, _ := model.Update(command())
		if updated.(RootModel).logs.Loading() {
			t.Error("log response was not routed")
		}
	})
}

func TestRootRoutesScreenSpecificResultEnvelopes(t *testing.T) {
	t.Run("helm releases", func(t *testing.T) {
		model := freshRoot(t)
		command := model.helm.SetNamespace("routing-test")
		updated, _ := model.Update(command())
		if updated.(RootModel).helm.Loading() {
			t.Error("Helm result was not routed")
		}
	})

	t.Run("custom resources", func(t *testing.T) {
		model := freshRoot(t)
		command := model.crds.SetNamespace("routing-test")
		updated, _ := model.Update(command())
		if updated.(RootModel).crds.Loading() {
			t.Error("CRD result was not routed")
		}
	})
}

func TestRootIgnoresUnownedLiveMessages(t *testing.T) {
	t.Run("browser data preserved", func(t *testing.T) {
		model := newSeededLiveRootModel(t)
		root, command := sendUnownedLiveMessage(t, model)
		if command != nil {
			t.Fatalf("unowned live message returned command %v", command)
		}
		browserItems := root.browser.SearchItems()
		if len(browserItems) != 1 || browserItems[0].Name != "browser-pod" {
			t.Fatalf("unowned live message changed browser data: %+v", browserItems)
		}
	})

	t.Run("dashboard data preserved", func(t *testing.T) {
		model := newSeededLiveRootModel(t)
		root, _ := sendUnownedLiveMessage(t, model)
		dashboardItems := root.dashboard.SearchItems()
		if len(dashboardItems) == 0 {
			t.Fatal("unowned live message cleared dashboard data")
		}
		for _, item := range dashboardItems {
			if item.Name == "intruder" {
				t.Fatalf("unowned live message injected dashboard data: %+v", dashboardItems)
			}
		}
	})

	t.Run("root state preserved", func(t *testing.T) {
		model := newSeededLiveRootModel(t)
		root, _ := sendUnownedLiveMessage(t, model)
		if root.screen != model.screen || root.browser.Error() != nil || root.dashboard.Error() != nil {
			t.Fatalf("unowned live message changed root state: screen=%d", root.screen)
		}
	})
}

func newSeededLiveRootModel(t *testing.T) RootModel {
	t.Helper()
	model := newTestRootModelWithObserver(t, "default", &testResourceObserver{
		resourceName:      "dashboard-pod",
		resourceNamespace: "default",
	})
	for _, message := range commandMessages(t, model.dashboard.Activate()) {
		model.dashboard, _ = model.dashboard.Update(message)
	}
	t.Cleanup(func() { model.dashboard.Deactivate() })
	model.browser, _ = model.browser.Update(cluster.PodsMsg{Pods: []cluster.Pod{{Name: "browser-pod", Namespace: "default"}}})
	return model
}

func sendUnownedLiveMessage(t *testing.T, model RootModel) (RootModel, tea.Cmd) {
	t.Helper()
	intruder := screen.LiveMessage{
		Generation: ^uint64(0),
		Payload: screen.LiveSnapshot[cluster.Pod]{State: clusterui.LiveState[cluster.Pod]{
			Ready: true,
			Items: []cluster.Pod{{Name: "intruder", Namespace: "default"}},
		}},
	}
	updated, command := model.Update(intruder)
	return updated.(RootModel), command
}

func TestRootBroadcastsSpinnerAndContextData(t *testing.T) {
	model := freshRoot(t)
	model.screen = ScreenDashboard
	model.analysisPanel.SetVisible(true)
	model.analysisPanel.SetScreenContext("before")
	updatedModel, _ := model.broadcastRootMessage(cluster.MetricsMsg{})
	updated := updatedModel.(RootModel)
	if !strings.Contains(updated.analysisPanel.ScreenContext(), model.namespace) {
		t.Errorf("analysis context was not refreshed: %q", updated.analysisPanel.ScreenContext())
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
