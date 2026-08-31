package app

import (
	"context"
	"io"
	"log/slog"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/analysis"
	"github.com/HediAbed/opsmate/internal/config"
	"github.com/HediAbed/opsmate/internal/kube"
	"github.com/HediAbed/opsmate/internal/session"
	"github.com/HediAbed/opsmate/internal/ui"
)

func Run(arguments []string, output, errorOutput io.Writer) int {
	return newApplication(output, errorOutput).run(arguments)
}

func newApplication(output, errorOutput io.Writer) application {
	logger := slog.New(slog.NewTextHandler(errorOutput, &slog.HandlerOptions{
		ReplaceAttr: removeLogTimestamp,
	}))
	return application{
		output:      output,
		errorOutput: errorOutput,
		logger:      logger,
		dependencies: applicationDependencies{
			loadEnvironment:      config.LoadDotEnvFromExecutableDir,
			configureAnalysis:    analysis.NewServiceFromEnvironment,
			connectCluster:       connectCluster,
			newSnapshotCollector: kube.NewSnapshotCollector,
			loadSession:          session.LoadSession,
			newRootModel:         ui.NewRootModel,
			newProgram:           newTerminalProgram,
			saveSession:          ui.RootModel.SaveOnExit,
			stopPortForwards:     stopClusterPortForwards,
		},
	}
}

func connectCluster(ctx context.Context) (kube.Cluster, error) {
	return connectClusterWith(ctx, kube.NewDeferredConfigSource(), kube.DefaultClientBuilder{})
}

func connectClusterWith(ctx context.Context, source kube.ConfigSource, builder kube.ClientBuilder) (kube.Cluster, error) {
	manager, err := kube.NewManager(source, builder)
	if err != nil {
		return nil, err
	}
	if err := manager.Connect(ctx, ""); err != nil {
		return nil, err
	}
	return manager, nil
}

func newTerminalProgram(root tea.Model) programRunner {
	return tea.NewProgram(root)
}

func removeLogTimestamp(_ []string, attribute slog.Attr) slog.Attr {
	if attribute.Key == slog.TimeKey {
		return slog.Attr{}
	}
	return attribute
}
