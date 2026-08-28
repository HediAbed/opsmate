package model

import (
	"strings"
	"testing"

	"github.com/HediAbed/opsmate/internal/service"
)

func TestSetVisible_CancelsActiveStream(t *testing.T) {
	m := NewAIPanelModel()
	m.SetVisible(true)
	m.streaming = true
	cancelCalls := 0
	m.streamCancel = func() { cancelCalls++ }

	m.SetVisible(false)

	if cancelCalls != 1 {
		t.Errorf("hiding the panel must cancel the active stream exactly once, got %d", cancelCalls)
	}
	if m.streamCancel != nil {
		t.Error("streamCancel should be nil after end")
	}
	if m.streaming {
		t.Error("streaming flag should clear after end")
	}
}

func TestRetryLastQuery_RestoresQueryToInput(t *testing.T) {
	m := NewAIPanelModel()
	m.SetVisible(true)
	m.history = []historyEntry{
		{Query: "why is my pod crashing?", Response: "Error: rate limited\n\n_(press R to retry)_"},
	}
	m.retryLastQuery()
	if got := m.input.Value(); got != "why is my pod crashing?" {
		t.Errorf("retryLastQuery should restore query into input; got %q", got)
	}
}

func TestRetryLastQuery_NoFailedEntryDoesNothing(t *testing.T) {
	m := NewAIPanelModel()
	m.history = []historyEntry{
		{Query: "list pods", Response: "Sure, here you go..."},
	}
	m.input.SetValue("")
	m.retryLastQuery()
	if got := m.input.Value(); got != "" {
		t.Errorf("no failed entry should leave input empty, got %q", got)
	}
}

func TestRetryLastQuery_DoesNotRestoreWhileStreaming(t *testing.T) {
	m := NewAIPanelModel()
	m.streaming = true
	m.history = []historyEntry{
		{Query: "x", Response: "Error: foo"},
	}
	m.retryLastQuery()
	if got := m.input.Value(); got != "" {
		t.Errorf("retryLastQuery must be a no-op while streaming; got %q", got)
	}
}

func TestClassifyKubectlCommand(t *testing.T) {
	cases := []struct {
		cmd      string
		wantKind service.CommandRisk
	}{
		{"kubectl get pods", service.RiskReadOnly},
		{"kubectl describe pod web", service.RiskReadOnly},
		{"kubectl logs web -f", service.RiskReadOnly},
		{"kubectl scale deployment web --replicas=3", service.RiskMutating},
		{"kubectl rollout restart deployment web", service.RiskMutating},
		{"kubectl delete pod web", service.RiskDestructive},
		{"kubectl drain node-1", service.RiskDestructive},
		{"kubectl banana pods", service.RiskUnknown},
		{"", service.RiskUnknown},
	}
	for _, tc := range cases {
		got, _ := service.ClassifyKubectlCommand(tc.cmd)
		if got != tc.wantKind {
			t.Errorf("ClassifyKubectlCommand(%q) = %v; want %v", tc.cmd, got, tc.wantKind)
		}
	}
}

func TestHelpView_ShowsRetryOnlyWhenFailedEntryExists(t *testing.T) {
	m := NewAIPanelModel()
	m.SetVisible(true)
	m.SetSize(120, 30)

	if strings.Contains(m.helpView(), "retry") {
		t.Error("helpView must NOT show retry hint when there is no failed entry")
	}

	m.history = []historyEntry{
		{Query: "q", Response: "Error: rate limit"},
	}
	if !strings.Contains(m.helpView(), "retry") {
		t.Error("helpView must show retry hint after a failed entry")
	}
}
