package ui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/HediAbed/opsmate/failure"
	"github.com/HediAbed/opsmate/internal/analysis"
	"github.com/HediAbed/opsmate/internal/kube"
)

type snapshotCollectorStub struct {
	snapshot  kube.ClusterSnapshot
	err       error
	context   context.Context
	namespace string
}

func (c *snapshotCollectorStub) Collect(ctx context.Context, namespace string) (kube.ClusterSnapshot, error) {
	c.context = ctx
	c.namespace = namespace
	return c.snapshot, c.err
}

func TestNativeClusterAnalyzerCollectsAndAnalyzesSnapshot(t *testing.T) {
	collector := &snapshotCollectorStub{snapshot: completeClusterSnapshot()}
	analyzer := newNativeClusterAnalyzer(context.Background(), collector)
	var received struct {
		systemPrompt       string
		question           string
		conversationMemory string
		clusterContext     string
	}
	analyzer.analyze = func(
		ctx context.Context,
		systemPrompt string,
		question string,
		conversationMemory string,
		clusterContext string,
	) analysis.AnalysisMsg {
		if _, hasDeadline := ctx.Deadline(); !hasDeadline {
			t.Fatal("analysis context has no deadline")
		}
		received.systemPrompt = systemPrompt
		received.question = question
		received.conversationMemory = conversationMemory
		received.clusterContext = clusterContext
		return analysis.AnalysisMsg{Response: "answer"}
	}

	message := analyzer.Analyze("system", "question", "memory", "team-a")().(analysis.AnalysisMsg)
	if message.Err != nil || message.Response != "answer" {
		t.Fatalf("Analyze() = %+v", message)
	}
	if collector.namespace != "team-a" || collector.context == nil {
		t.Fatalf("collector request = (%q, %v)", collector.namespace, collector.context)
	}
	if received.systemPrompt != "system" || received.question != "question" || received.conversationMemory != "memory" {
		t.Fatalf("analysis request = %+v", received)
	}
	requireCompleteClusterContext(t, received.clusterContext)
}

func requireCompleteClusterContext(t *testing.T, clusterContext string) {
	t.Helper()
	for _, expected := range []string{
		"Context: work",
		"Namespace: team-a",
		"team-a/api-0 status=Running ready=1/1 restarts=2 node=node-a",
		"team-a/api ready=2/3 updated=2 available=2",
		"http:8080/TCP,8443/TCP",
		`type=Warning reason=BackOff object=Pod/api-0 count=4 message="restart"`,
		"node-a ready=true unschedulable=false version=v1.36.1",
		"events code=permission_denied",
	} {
		if !strings.Contains(clusterContext, expected) {
			t.Errorf("cluster context does not contain %q: %q", expected, clusterContext)
		}
	}
}

func TestNativeClusterAnalyzerPropagatesCollectionFailure(t *testing.T) {
	sentinel := errors.New("snapshot failed")
	collector := &snapshotCollectorStub{err: sentinel}
	analyzer := newNativeClusterAnalyzer(context.Background(), collector)
	analyzer.analyze = func(context.Context, string, string, string, string) analysis.AnalysisMsg {
		t.Fatal("provider analysis ran after collection failure")
		return analysis.AnalysisMsg{}
	}

	message := analyzer.Analyze("system", "question", "", "team-a")().(analysis.AnalysisMsg)
	if !errors.Is(message.Err, sentinel) {
		t.Fatalf("Analyze() error = %v, want snapshot cause", message.Err)
	}
}

func TestNativeClusterAnalyzerRejectsMissingDependencies(t *testing.T) {
	collector := &snapshotCollectorStub{}
	tests := []struct {
		name     string
		analyzer nativeClusterAnalyzer
	}{
		{name: "parent", analyzer: nativeClusterAnalyzer{snapshots: collector, analyze: analysis.AnalyzeClusterContext}},
		{name: "collector", analyzer: nativeClusterAnalyzer{parent: context.Background(), analyze: analysis.AnalyzeClusterContext}},
		{name: "analysis", analyzer: nativeClusterAnalyzer{parent: context.Background(), snapshots: collector}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			message := test.analyzer.Analyze("system", "question", "", "")().(analysis.AnalysisMsg)
			if !errors.Is(message.Err, ErrClusterAnalysisUnavailable) {
				t.Fatalf("Analyze() error = %v, want unavailable", message.Err)
			}
		})
	}
}

func TestUnavailableClusterAnalysisReturnsTypedMessage(t *testing.T) {
	message := unavailableClusterAnalysis("system", "question", "", "")().(analysis.AnalysisMsg)
	if !errors.Is(message.Err, ErrClusterAnalysisUnavailable) {
		t.Fatalf("unavailableClusterAnalysis() error = %v", message.Err)
	}
}

func TestRenderClusterSnapshotHandlesClusterScopeAndEmptySections(t *testing.T) {
	rendered := renderClusterSnapshot(kube.ClusterSnapshot{})
	if !strings.Contains(rendered, "Namespace: all namespaces") {
		t.Fatalf("renderClusterSnapshot() = %q", rendered)
	}
	if strings.Contains(rendered, "Collection warnings") {
		t.Fatalf("empty snapshot rendered warnings: %q", rendered)
	}
	if got := namespacedSnapshotName("", "node-a"); got != "node-a" {
		t.Fatalf("namespacedSnapshotName() = %q", got)
	}
}

func completeClusterSnapshot() kube.ClusterSnapshot {
	lastSeen := time.Date(2026, time.August, 29, 1, 2, 3, 0, time.FixedZone("test", 2*60*60))
	return kube.ClusterSnapshot{
		ContextName: "work",
		Namespace:   "team-a",
		Pods: []kube.PodSnapshot{{
			Name: "api-0", Namespace: "team-a", Status: "Running", Ready: 1, Desired: 1, Restarts: 2, Node: "node-a",
		}},
		Deployments: []kube.DeploymentSnapshot{{
			Name: "api", Namespace: "team-a", Ready: 2, Desired: 3, Updated: 2, Available: 2,
		}},
		Services: []kube.ServiceSnapshot{{
			Name: "api", Namespace: "team-a", Type: "ClusterIP", ClusterIP: "10.0.0.8",
			Ports: []kube.ServicePortSnapshot{
				{Name: "http", Protocol: "TCP", Port: 8080},
				{Protocol: "TCP", Port: 8443},
			},
		}},
		Events: []kube.EventSnapshot{{
			Namespace: "team-a", Type: "Warning", Reason: "BackOff", Object: "Pod/api-0", Message: "restart", Count: 4, LastSeen: lastSeen,
		}},
		Nodes:  []kube.NodeSnapshot{{Name: "node-a", Ready: true, Version: "v1.36.1"}},
		Totals: kube.SnapshotTotals{Pods: 1, Deployments: 1, Services: 1, Events: 1, Nodes: 1},
		Warnings: []kube.SnapshotWarning{{
			Section: kube.SnapshotEvents,
			Err: &kube.Error{
				Operation: kube.OperationList,
				Subject:   kube.SubjectEvents,
				Code:      failure.CodePermissionDenied,
				Err:       errors.New("access denied"),
			},
		}},
	}
}
