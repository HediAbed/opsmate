package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/analysis"
	"github.com/HediAbed/opsmate/internal/failure"
	"github.com/HediAbed/opsmate/internal/kube"
	"github.com/HediAbed/opsmate/internal/session"
	"github.com/HediAbed/opsmate/internal/ui"
)

const (
	exitSuccess = iota
	exitFailure
	exitUsage

	portForwardShutdownTimeout = 2 * time.Second
)

type programRunner interface {
	Run() (tea.Model, error)
}

type applicationDependencies struct {
	loadEnvironment      func() error
	configureAnalysis    func() (analysis.Service, error)
	connectCluster       func(context.Context) (kube.Cluster, error)
	newSnapshotCollector func(
		kube.ContextManager,
		kube.ResourceReader,
	) (*kube.SnapshotCollector, error)
	loadSession      func() (session.SessionState, error)
	newRootModel     func(string, ui.RuntimeDependencies) (ui.RootModel, error)
	newProgram       func(tea.Model) programRunner
	saveSession      func(ui.RootModel) error
	stopPortForwards func(context.Context, kube.PortForwardManager) error
}

type application struct {
	output       io.Writer
	errorOutput  io.Writer
	logger       *slog.Logger
	dependencies applicationDependencies
}

type finalModelError struct {
	modelType string
}

func (e *finalModelError) Error() string {
	if e == nil || e.modelType == "" {
		return "terminal returned an invalid model"
	}
	return fmt.Sprintf("terminal returned %s, want ui.RootModel", e.modelType)
}

func (*finalModelError) FailureCode() failure.Code {
	return failure.CodeInternal
}

func (a application) logFailure(message string, err error) {
	a.logger.Error(
		message,
		slog.String("code", string(failure.CodeOf(err))),
		slog.Any("error", err),
	)
}

func (a application) run(arguments []string) int {
	command, err := parseArguments(arguments)
	if err != nil {
		a.logFailure("invalid arguments", err)
		if _, writeErr := io.WriteString(a.errorOutput, usageText); writeErr != nil {
			a.logFailure("write usage", writeErr)
		}
		return exitUsage
	}
	return command.execute(a)
}

func (a application) runInteractive(namespaceOverride string) (exitCode int) {
	runtimeContext, cancelRuntime := context.WithCancel(context.Background())
	defer cancelRuntime()

	if err := a.dependencies.loadEnvironment(); err != nil {
		a.logFailure("load environment configuration", err)
		return exitFailure
	}
	analysisService, err := a.dependencies.configureAnalysis()
	if err != nil {
		a.logFailure("configure analysis", err)
		return exitFailure
	}
	cluster, err := a.dependencies.connectCluster(runtimeContext)
	if err != nil {
		a.logFailure("connect cluster", err)
		return exitFailure
	}
	snapshots, err := a.dependencies.newSnapshotCollector(cluster, cluster)
	if err != nil {
		a.logFailure("create cluster snapshot collector", err)
		return exitFailure
	}

	savedSession, namespace, err := a.loadStartupState(namespaceOverride)
	if err != nil {
		a.logFailure("load session state", err)
		return exitFailure
	}
	root, err := a.dependencies.newRootModel(namespace, clusterRuntimeDependencies(
		runtimeContext,
		cluster,
		snapshots,
		analysisService,
	))
	if err != nil {
		a.logFailure("create terminal model", err)
		return exitFailure
	}
	root.RestoreSession(savedSession)

	defer func() {
		exitCode = a.shutdownCluster(cluster, exitCode)
	}()
	finalModel, err := a.dependencies.newProgram(root).Run()
	if err != nil {
		a.logFailure("terminal failed", err)
		return exitFailure
	}
	finalRoot, err := rootFromFinalModel(finalModel)
	if err != nil {
		a.logFailure("terminal returned invalid state", err)
		return exitFailure
	}
	if err := a.dependencies.saveSession(finalRoot); err != nil {
		a.logFailure("save session state", err)
		return exitFailure
	}
	return exitSuccess
}

func clusterRuntimeDependencies(
	runtimeContext context.Context,
	cluster kube.Cluster,
	snapshots ui.ClusterSnapshotCollector,
	analysisService analysis.Service,
) ui.RuntimeDependencies {
	return ui.RuntimeDependencies{
		Context:           runtimeContext,
		ClusterContext:    cluster,
		ClusterResources:  cluster,
		ClusterSnapshots:  snapshots,
		ClusterObserver:   cluster,
		ClusterOperations: cluster,
		HelmReleases:      cluster,
		Analysis:          analysisService,
	}
}

func (a application) shutdownCluster(cluster kube.PortForwardManager, currentExitCode int) int {
	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), portForwardShutdownTimeout)
	defer cancelShutdown()
	if err := a.dependencies.stopPortForwards(shutdownContext, cluster); err != nil {
		a.logFailure("stop port forwards", err)
		return exitFailure
	}
	return currentExitCode
}

func stopClusterPortForwards(ctx context.Context, manager kube.PortForwardManager) error {
	return manager.StopAllPortForwards(ctx)
}

func (a application) loadSession() (session.SessionState, error) {
	savedSession, err := a.dependencies.loadSession()
	if err == nil {
		return savedSession, nil
	}
	if errors.Is(err, session.ErrNoSession) {
		return session.SessionState{}, nil
	}
	return session.SessionState{}, err
}

func (a application) loadStartupState(namespaceOverride string) (session.SessionState, string, error) {
	savedSession, err := a.loadSession()
	if err != nil {
		return session.SessionState{}, "", err
	}
	return savedSession, resolveStartupNamespace(namespaceOverride, savedSession.Namespace), nil
}

func (a application) writeOutput(content string) int {
	if _, err := io.WriteString(a.output, content); err != nil {
		a.logFailure("write output", err)
		return exitFailure
	}
	return exitSuccess
}

func resolveStartupNamespace(namespaceOverride, sessionNamespace string) string {
	if namespaceOverride != "" {
		return namespaceOverride
	}
	return sessionNamespace
}

func rootFromFinalModel(finalModel tea.Model) (ui.RootModel, error) {
	switch finalRoot := finalModel.(type) {
	case ui.RootModel:
		return finalRoot, nil
	case *ui.RootModel:
		if finalRoot != nil {
			return *finalRoot, nil
		}
	}
	return ui.RootModel{}, &finalModelError{modelType: fmt.Sprintf("%T", finalModel)}
}
