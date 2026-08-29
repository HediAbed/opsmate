package ui

import (
	"context"
	"errors"
	"testing"

	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/kube"
)

func TestLogsContainerRequestCarriesTargetIdentity(t *testing.T) {
	reader := &testPodReader{containers: []string{"main", "sidecar"}}
	operations := newNativeClusterOperations(context.Background(), &testResourceInspector{}, reader, &testResourceWriter{})
	model := NewLogsModel("team-a", newTestClusterCommands(), operations)
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
	if message.pod != (resourceIdentity{Kind: "pod", Namespace: "team-a", Name: "worker"}) {
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
		pod:       resourceIdentity{Kind: "pod", Namespace: "team-a", Name: "worker"},
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
			pod:       resourceIdentity{Kind: "pod", Namespace: "team-a", Name: "worker"},
			payload:   cluster.ContainersMsg{Containers: []string{"stale"}},
		},
		{
			requestID: 7,
			pod:       resourceIdentity{Kind: "pod", Namespace: "team-a", Name: "other"},
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
	reader := &testPodReader{containerErr: kube.ErrNamespaceRequired}
	operations := newNativeClusterOperations(context.Background(), &testResourceInspector{}, reader, &testResourceWriter{})
	model := NewLogsModel("", newTestClusterCommands(), operations)
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
