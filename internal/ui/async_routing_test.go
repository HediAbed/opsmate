package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/ui/screen"
)

func TestRootRoutesBrowserResultWhileDashboardIsVisible(t *testing.T) {
	const sentinel = "background-service"
	model := newTestRootModelWithObserver(t, "default", &testResourceObserver{
		resourceName:      sentinel,
		resourceNamespace: "default",
	})
	model.screen = ScreenDashboard
	model.browser.SetResourceType("services")
	message := firstCommandMessage(t, model.browser.Refresh())

	updated, _ := model.Update(message)
	root := updated.(RootModel)
	items := root.browser.SearchItems()
	if root.browser.Loading() || len(items) != 1 {
		t.Fatalf("browser result was not applied: loading=%v items=%+v", root.browser.Loading(), items)
	}
	if items[0].Kind != screen.ResourceKindService || items[0].Name != sentinel || items[0].Namespace != "default" {
		t.Fatalf("browser owner cache = %+v", items[0])
	}
	if len(root.dashboard.SearchItems()) != 0 {
		t.Fatal("browser result changed dashboard data")
	}
}

func TestRootRefreshesDashboardAnalysisContextAfterAcceptedResult(t *testing.T) {
	observer := &testResourceObserver{
		resourceName:      "dashboard-sentinel",
		resourceNamespace: "default",
	}
	model := newTestRootModelWithObserver(t, "default", observer)
	model.width = 200
	model.height = 50
	model.ready = true
	model.screen = ScreenDashboard
	model.resizeChildren()
	model.analysisPanel.SetVisible(true)
	model.analysisPanel.SetScreenContext("before")
	command := model.dashboard.Activate()
	t.Cleanup(func() { model.dashboard.Deactivate() })

	for _, message := range commandMessages(t, command) {
		updated, _ := model.Update(message)
		model = updated.(RootModel)
	}
	context := model.analysisPanel.ScreenContext()
	if context == "before" || !strings.Contains(context, "dashboard-sentinel") {
		t.Fatalf("dashboard analysis context = %q", context)
	}
	view := stripAnsiForTest(model.dashboard.View())
	if !strings.Contains(view, "0m") || !strings.Contains(view, "0Mi") {
		t.Fatalf("dashboard did not render routed metrics: %q", view)
	}
}

func TestRootRefreshesBrowserAnalysisContextAfterAcceptedResult(t *testing.T) {
	const sentinel = "analysis-service"
	model := newTestRootModelWithObserver(t, "default", &testResourceObserver{
		resourceName:      sentinel,
		resourceNamespace: "default",
	})
	model.screen = ScreenBrowser
	model.analysisPanel.SetVisible(true)
	model.analysisPanel.SetScreenContext("before")
	model.browser.SetResourceType("services")
	message := firstCommandMessage(t, model.browser.Refresh())

	updated, _ := model.Update(message)
	context := updated.(RootModel).analysisPanel.ScreenContext()
	if context == "before" || !strings.Contains(context, sentinel) {
		t.Fatalf("browser analysis context = %q", context)
	}
}

func TestRootRefreshesLogAnalysisContextAfterAcceptedResult(t *testing.T) {
	const sentinel = "analysis-pod"
	model := newTestRootModelWithObserver(t, "default", &testResourceObserver{
		resourceName:      sentinel,
		resourceNamespace: "default",
	})
	model.screen = ScreenLogs
	model.resizeChildren()
	model.analysisPanel.SetVisible(true)
	model.analysisPanel.SetScreenContext("before")
	command := model.logs.Activate()
	t.Cleanup(func() { model.logs.Deactivate() })

	for _, message := range commandMessages(t, command) {
		updated, _ := model.Update(message)
		model = updated.(RootModel)
	}
	context := model.analysisPanel.ScreenContext()
	if context == "before" || !strings.Contains(context, sentinel) {
		t.Fatalf("logs analysis context = %q", context)
	}
}

func firstCommandMessage(t *testing.T, command tea.Cmd) tea.Msg {
	t.Helper()
	if command == nil {
		t.Fatal("command is nil")
	}
	message := command()
	batch, batched := message.(tea.BatchMsg)
	if !batched {
		return message
	}
	if len(batch) == 0 || batch[0] == nil {
		t.Fatal("command batch is empty")
	}
	return batch[0]()
}

func commandMessages(t *testing.T, command tea.Cmd) []tea.Msg {
	t.Helper()
	if command == nil {
		t.Fatal("command is nil")
	}
	message := command()
	batch, batched := message.(tea.BatchMsg)
	if !batched {
		return []tea.Msg{message}
	}
	if len(batch) == 0 {
		t.Fatal("command batch is empty")
	}
	messages := make([]tea.Msg, 0, len(batch))
	for _, batchedCommand := range batch {
		if batchedCommand == nil {
			t.Fatal("command batch contains nil")
		}
		messages = append(messages, batchedCommand())
	}
	return messages
}
