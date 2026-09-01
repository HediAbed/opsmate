package logs

import (
	"strings"
	"testing"

	"github.com/HediAbed/opsmate/internal/cluster"
)

func TestLogsContextChangedByRequiresCurrentPodList(t *testing.T) {
	model := newTestLogsModel("team-a")
	current := model.fetchPods()().(logPodsResultMsg)
	if !model.ContextChangedBy(current) {
		t.Fatal("current pod list was not classified as a context change")
	}
	stale := current
	current = model.fetchPods()().(logPodsResultMsg)
	if model.ContextChangedBy(stale) {
		t.Fatal("stale pod list was classified as a context change")
	}
	wrongNamespace := current
	wrongNamespace.namespace = "team-b"
	if model.ContextChangedBy(wrongNamespace) {
		t.Fatal("foreign pod list was classified as a context change")
	}
}

func TestLogsContextChangedByRequiresCurrentContainerList(t *testing.T) {
	model := newTestLogsModel("team-a")
	model.SetPodInNamespace("web", "team-a")
	current := model.fetchContainers()().(containersResultMsg)
	if !model.ContextChangedBy(current) {
		t.Fatal("current container list was not classified as a context change")
	}
	stale := current
	current = model.fetchContainers()().(containersResultMsg)
	if model.ContextChangedBy(stale) {
		t.Fatal("stale container list was classified as a context change")
	}
	wrongPod := current
	wrongPod.pod.Name = "other"
	if model.ContextChangedBy(wrongPod) {
		t.Fatal("wrong-pod container list was classified as a context change")
	}
}

func TestLogsContextChangedByRequiresCurrentLogResult(t *testing.T) {
	model := newTestLogsModel("team-a")
	model.SetPodInNamespace("web", "team-a")
	current := model.fetchSelectedLogs()().(logsResultMsg)
	if !model.ContextChangedBy(current) {
		t.Fatal("current logs were not classified as a context change")
	}
	stale := current
	current = model.fetchSelectedLogs()().(logsResultMsg)
	if model.ContextChangedBy(stale) {
		t.Fatal("stale logs were classified as a context change")
	}
	wrongContainer := current
	wrongContainer.container = "sidecar"
	if model.ContextChangedBy(wrongContainer) {
		t.Fatal("wrong-container logs were classified as a context change")
	}
	if model.ContextChangedBy(struct{}{}) {
		t.Fatal("unrelated message was classified as a context change")
	}
}

func TestLogsUpdateRoutesIssuedPodAndContainerResults(t *testing.T) {
	model := newTestLogsModel("team-a")
	pods := model.fetchPods()().(logPodsResultMsg)
	pods.payload = cluster.PodsMsg{Pods: []cluster.Pod{{
		Name: "web", Namespace: "team-a", Status: "Running",
	}}}
	if !model.Accepts(pods) {
		t.Fatal("logs screen rejected its issued pod result")
	}
	updated, command := model.Update(pods)
	if command == nil || updated.selectedPod != "web" {
		t.Fatalf("pod result = selected:%q command:%v", updated.selectedPod, command)
	}

	containers := updated.fetchContainers()().(containersResultMsg)
	containers.payload = cluster.ContainersMsg{Containers: []string{"app", "sidecar"}}
	if !updated.Accepts(containers) {
		t.Fatal("logs screen rejected its issued container result")
	}
	updated, command = updated.Update(containers)
	if command != nil || len(updated.containers) != 2 || !updated.showContainerPopup {
		t.Fatalf("container result = containers:%v popup:%v command:%v", updated.containers, updated.showContainerPopup, command)
	}
}

func TestLogsApplyPodListPreservesExistingSelection(t *testing.T) {
	model := newTestLogsModel("team-a")
	model.SetPodInNamespace("selected", "team-a")
	command := model.applyLogPods(cluster.PodsMsg{Pods: []cluster.Pod{{Name: "other", Namespace: "team-a"}}})
	if command != nil || model.selectedPod != "selected" {
		t.Fatalf("pod refresh replaced selection: selected=%q command=%v", model.selectedPod, command)
	}

	empty := newTestLogsModel("team-a")
	if command := empty.applyLogPods(cluster.PodsMsg{}); command != nil || empty.selectedPod != "" {
		t.Fatalf("empty pod refresh selected a pod: selected=%q command=%v", empty.selectedPod, command)
	}
}

func TestLogsPodPopupShowsNamespaceInAllNamespaceMode(t *testing.T) {
	model := newTestLogsModel("")
	model.pods = []cluster.Pod{{Name: "web", Namespace: "team-a", Status: "Running"}}
	items := model.renderPodPopupItems(0, len(model.pods))
	if len(items) != 1 || !strings.Contains(stripAnsiForTest(items[0]), "team-a/web") {
		t.Fatalf("all-namespace popup items = %v", items)
	}
}
