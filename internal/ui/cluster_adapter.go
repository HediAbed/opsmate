package ui

import (
	"context"

	"github.com/HediAbed/opsmate/internal/analysis"
	"github.com/HediAbed/opsmate/internal/kube"
	clusterui "github.com/HediAbed/opsmate/internal/ui/cluster"
)

type clusterCommands = clusterui.Commands
type clusterOperations = clusterui.Operations
type clusterSessionOperations = clusterui.SessionOperations
type helmCommands = clusterui.HelmCommands
type clusterAnalyzer = clusterui.Analyzer
type portForwardStartedMsg = clusterui.PortForwardStartedMsg
type portForwardStoppedMsg = clusterui.PortForwardStoppedMsg

func newNativeClusterCommands(
	parent context.Context,
	reader kube.ResourceReader,
	observer kube.ResourceObserver,
) clusterCommands {
	return clusterui.NewCommands(parent, reader, observer)
}

func newNativeClusterOperations(
	parent context.Context,
	inspector kube.ResourceInspector,
	pods kube.PodReader,
	writer kube.ResourceWriter,
	sessions ...clusterSessionOperations,
) clusterOperations {
	return clusterui.NewOperations(parent, inspector, pods, writer, sessions...)
}

func newNativeHelmCommands(parent context.Context, reader kube.HelmReader) helmCommands {
	return clusterui.NewHelmCommands(parent, reader)
}

func newNativeClusterAnalyzer(
	parent context.Context,
	snapshots clusterui.SnapshotCollector,
	service analysis.Service,
) clusterAnalyzer {
	return clusterui.NewAnalyzer(parent, snapshots, service)
}
