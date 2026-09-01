package logs

import (
	"errors"
	"testing"

	"github.com/HediAbed/opsmate/internal/analysis"
	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/kube"
)

func TestLogsContainerRequestCarriesTargetIdentity(t *testing.T) {
	operations := &testOperations{containers: []string{"main", "sidecar"}}
	model := NewLogsModel("team-a", testCommands{}, operations)
	model.SetPodInNamespace("worker", "team-a")
	command := model.fetchContainers()
	if command == nil {
		t.Fatal("container request command is nil")
	}

	rawMessage := command()
	message, ok := rawMessage.(containersResultMsg)
	if !ok {
		t.Fatalf("container request returned %T", rawMessage)
	}
	if message.pod != (podIdentity{Kind: "pod", Namespace: "team-a", Name: "worker"}) {
		t.Errorf("request identity = %+v", message.pod)
	}
	payload, ok := message.payload.(cluster.ContainersMsg)
	if !ok {
		t.Fatalf("container payload = %T", message.payload)
	}
	if len(payload.Containers) != 2 || payload.Containers[1] != "sidecar" {
		t.Errorf("containers = %v, want [main sidecar]", payload.Containers)
	}
}

func TestLogsContainerEnvelopeAcceptsCurrentRequest(t *testing.T) {
	model := newTestLogsModel("team-a")
	model.SetPodInNamespace("worker", "team-a")
	model.containerRequestID = 7
	message := containersResultMsg{
		requestID: 7,
		pod:       podIdentity{Kind: "pod", Namespace: "team-a", Name: "worker"},
		payload:   cluster.ContainersMsg{Containers: []string{"main", "sidecar"}},
	}

	updated, command := model.handleContainersResult(message)
	if command != nil {
		t.Errorf("multiple containers should not schedule a command: %v", command)
	}
	if !updated.showContainerPopup || len(updated.containers) != 2 {
		t.Fatalf("current response not applied: popup=%v containers=%v", updated.showContainerPopup, updated.containers)
	}
}

func TestLogsContainerEnvelopeRejectsStaleTarget(t *testing.T) {
	model := newTestLogsModel("team-a")
	model.SetPodInNamespace("worker", "team-a")
	model.containerRequestID = 7
	tests := []containersResultMsg{
		{
			requestID: 6,
			pod:       podIdentity{Kind: "pod", Namespace: "team-a", Name: "worker"},
			payload:   cluster.ContainersMsg{Containers: []string{"stale"}},
		},
		{
			requestID: 7,
			pod:       podIdentity{Kind: "pod", Namespace: "team-a", Name: "other"},
			payload:   cluster.ContainersMsg{Containers: []string{"wrong-pod"}},
		},
	}
	for _, message := range tests {
		updated, command := model.handleContainersResult(message)
		if command != nil || len(updated.containers) != 0 || updated.showContainerPopup {
			t.Errorf("stale response changed model: command=%v containers=%v popup=%v", command, updated.containers, updated.showContainerPopup)
		}
	}
}

func TestLogsContainerRequestReportsNamespaceRequirement(t *testing.T) {
	operations := &testOperations{containerErr: kube.ErrNamespaceRequired}
	model := NewLogsModel("", testCommands{}, operations)
	model.SetPod("worker")
	message := model.fetchContainers()().(containersResultMsg)
	payload := message.payload.(cluster.ContainersMsg)
	if !errors.Is(payload.Err, kube.ErrNamespaceRequired) {
		t.Error("all-namespace container request should return a namespace error")
	}
	if !model.acceptsContainersResult(message) {
		t.Error("matching failed request should still be accepted by the model")
	}
}

func TestLogExplanationRequiresCurrentTarget(t *testing.T) {
	model := newTestLogsModel("payments")
	model.setPodIdentity("api", "payments")
	model.inspectMode = true
	model.filteredLines = []string{"current line"}
	model.explainRequestID = 5
	model, _ = model.Update(logExplainResultMsg{
		requestID: 4,
		pod:       podIdentity{Kind: podKind, Namespace: "payments", Name: "old-api"},
		line:      "old line",
		payload:   analysis.LogExplanationMsg{Explanation: "stale explanation"},
	})
	if model.lineExplanation != "" {
		t.Fatalf("stale log explanation was applied: %q", model.lineExplanation)
	}

	model, _ = model.Update(logExplainResultMsg{
		requestID: 5,
		pod:       podIdentity{Kind: podKind, Namespace: "payments", Name: "api"},
		line:      "current line",
		payload:   analysis.LogExplanationMsg{Explanation: "current\x1b]0;bad\a explanation"},
	})
	if model.lineExplanation != "current explanation" {
		t.Fatalf("current log explanation = %q", model.lineExplanation)
	}
}

func TestLogsRejectStalePodAndLogResults(t *testing.T) {
	model := newTestLogsModel("payments")
	model.podListRequestID = 2
	model.setPodIdentity("api", "payments")
	model.logRequestID = 5

	model, _ = model.Update(logPodsResultMsg{
		requestID: 1,
		namespace: "payments",
		payload:   cluster.PodsMsg{Pods: []cluster.Pod{{Name: "stale", Namespace: "payments"}}},
	})
	model, _ = model.Update(logsResultMsg{
		requestID: 4,
		pod:       podIdentity{Kind: podKind, Namespace: "payments", Name: "api"},
		payload:   cluster.LogsMsg{Lines: []string{"stale"}},
	})
	if len(model.pods) != 0 || len(model.allLines) != 0 {
		t.Fatalf("stale log data was applied: pods=%+v lines=%+v", model.pods, model.allLines)
	}
}
