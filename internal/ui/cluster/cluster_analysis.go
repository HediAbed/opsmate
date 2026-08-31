package cluster

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/analysis"
	"github.com/HediAbed/opsmate/internal/failure"
	"github.com/HediAbed/opsmate/internal/kube"
)

const clusterAnalysisTimeout = time.Minute

var ErrClusterAnalysisUnavailable = errors.New("cluster analysis is unavailable")

type Analyzer interface {
	Analyze(string, string, string, string) tea.Cmd
}

type SnapshotCollector interface {
	Collect(context.Context, string) (kube.ClusterSnapshot, error)
}

type SnapshotAnalyzer struct {
	parent    context.Context
	snapshots SnapshotCollector
	analyze   func(context.Context, string, string, string, string) analysis.AnalysisMsg
}

func NewAnalyzer(
	parent context.Context,
	snapshots SnapshotCollector,
	service analysis.Service,
) SnapshotAnalyzer {
	return SnapshotAnalyzer{
		parent:    parent,
		snapshots: snapshots,
		analyze:   service.AnalyzeClusterContext,
	}
}

func (a SnapshotAnalyzer) Analyze(
	systemPrompt string,
	question string,
	conversationMemory string,
	namespace string,
) tea.Cmd {
	return func() tea.Msg {
		if a.parent == nil || a.snapshots == nil || a.analyze == nil {
			return analysis.AnalysisMsg{Err: ErrClusterAnalysisUnavailable}
		}
		ctx, cancel := context.WithTimeout(a.parent, clusterAnalysisTimeout)
		defer cancel()
		snapshot, err := a.snapshots.Collect(ctx, namespace)
		if err != nil {
			return analysis.AnalysisMsg{Err: err}
		}
		return a.analyze(
			ctx,
			systemPrompt,
			question,
			conversationMemory,
			renderClusterSnapshot(snapshot),
		)
	}
}

func UnavailableAnalysis(string, string, string, string) tea.Cmd {
	return func() tea.Msg {
		return analysis.AnalysisMsg{Err: ErrClusterAnalysisUnavailable}
	}
}

func renderClusterSnapshot(snapshot kube.ClusterSnapshot) string {
	var output strings.Builder
	writeSnapshotHeader(&output, snapshot)
	writeSnapshotPods(&output, snapshot.Pods)
	writeSnapshotDeployments(&output, snapshot.Deployments)
	writeSnapshotServices(&output, snapshot.Services)
	writeSnapshotEvents(&output, snapshot.Events)
	writeSnapshotNodes(&output, snapshot.Nodes)
	writeSnapshotWarnings(&output, snapshot.Warnings)
	return strings.TrimSpace(output.String())
}

func writeSnapshotHeader(output *strings.Builder, snapshot kube.ClusterSnapshot) {
	namespace := snapshot.Namespace
	if namespace == "" {
		namespace = "all namespaces"
	}
	fmt.Fprintf(output, "Context: %s\nNamespace: %s\n", snapshot.ContextName, namespace)
	fmt.Fprintf(
		output,
		"Totals: pods=%d deployments=%d services=%d events=%d nodes=%d\n",
		snapshot.Totals.Pods,
		snapshot.Totals.Deployments,
		snapshot.Totals.Services,
		snapshot.Totals.Events,
		snapshot.Totals.Nodes,
	)
}

func writeSnapshotPods(output *strings.Builder, pods []kube.PodSnapshot) {
	output.WriteString("\nPods:\n")
	for _, pod := range pods {
		fmt.Fprintf(
			output,
			"- %s status=%s ready=%d/%d restarts=%d node=%s\n",
			namespacedSnapshotName(pod.Namespace, pod.Name),
			pod.Status,
			pod.Ready,
			pod.Desired,
			pod.Restarts,
			pod.Node,
		)
	}
}

func writeSnapshotDeployments(output *strings.Builder, deployments []kube.DeploymentSnapshot) {
	output.WriteString("\nDeployments:\n")
	for _, deployment := range deployments {
		fmt.Fprintf(
			output,
			"- %s ready=%d/%d updated=%d available=%d\n",
			namespacedSnapshotName(deployment.Namespace, deployment.Name),
			deployment.Ready,
			deployment.Desired,
			deployment.Updated,
			deployment.Available,
		)
	}
}

func writeSnapshotServices(output *strings.Builder, services []kube.ServiceSnapshot) {
	output.WriteString("\nServices:\n")
	for _, clusterService := range services {
		fmt.Fprintf(
			output,
			"- %s type=%s cluster_ip=%s ports=%s\n",
			namespacedSnapshotName(clusterService.Namespace, clusterService.Name),
			clusterService.Type,
			clusterService.ClusterIP,
			snapshotServicePorts(clusterService.Ports),
		)
	}
}

func snapshotServicePorts(ports []kube.ServicePortSnapshot) string {
	formatted := make([]string, 0, len(ports))
	for _, port := range ports {
		label := fmt.Sprintf("%d/%s", port.Port, port.Protocol)
		if port.Name != "" {
			label = port.Name + ":" + label
		}
		formatted = append(formatted, label)
	}
	return strings.Join(formatted, ",")
}

func writeSnapshotEvents(output *strings.Builder, events []kube.EventSnapshot) {
	output.WriteString("\nRecent events:\n")
	for _, event := range events {
		fmt.Fprintf(
			output,
			"- %s type=%s reason=%s object=%s count=%d message=%q last_seen=%s\n",
			event.Namespace,
			event.Type,
			event.Reason,
			event.Object,
			event.Count,
			event.Message,
			event.LastSeen.UTC().Format(time.RFC3339),
		)
	}
}

func writeSnapshotNodes(output *strings.Builder, nodes []kube.NodeSnapshot) {
	output.WriteString("\nNodes:\n")
	for _, node := range nodes {
		fmt.Fprintf(
			output,
			"- %s ready=%t unschedulable=%t version=%s\n",
			node.Name,
			node.Ready,
			node.Unschedulable,
			node.Version,
		)
	}
}

func writeSnapshotWarnings(output *strings.Builder, warnings []kube.SnapshotWarning) {
	if len(warnings) == 0 {
		return
	}
	output.WriteString("\nCollection warnings:\n")
	for _, warning := range warnings {
		fmt.Fprintf(
			output,
			"- %s code=%s error=%q\n",
			warning.Section,
			failure.CodeOf(warning.Err),
			warning.Err,
		)
	}
}

func namespacedSnapshotName(namespace, name string) string {
	if namespace == "" {
		return name
	}
	return namespace + "/" + name
}

var _ Analyzer = SnapshotAnalyzer{}
