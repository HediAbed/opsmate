package main

import (
	"errors"
	"fmt"
	"io"
	"log/slog"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/model"
	"github.com/HediAbed/opsmate/internal/service"
)

const (
	exitSuccess = iota
	exitFailure
	exitUsage
)

type programRunner interface {
	Run() (tea.Model, error)
}

type applicationDependencies struct {
	loadEnvironment    func() error
	initializeProvider func() error
	loadSession        func() (service.SessionState, error)
	newProgram         func(tea.Model) programRunner
	saveSession        func(model.RootModel) error
	stopPortForwards   func()
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
	return fmt.Sprintf("terminal returned %s, want model.RootModel", e.modelType)
}

func (a application) run(arguments []string) int {
	command, err := parseArguments(arguments)
	if err != nil {
		a.logger.Error("invalid arguments", "error", err)
		if _, writeErr := io.WriteString(a.errorOutput, usageText); writeErr != nil {
			a.logger.Error("write usage", "error", writeErr)
		}
		return exitUsage
	}
	return command.execute(a)
}

func (a application) runInteractive(namespaceOverride string) int {
	if err := a.dependencies.loadEnvironment(); err != nil {
		a.logger.Error("load environment configuration", "error", err)
		return exitFailure
	}
	if err := a.dependencies.initializeProvider(); err != nil {
		a.logger.Error("initialize analysis provider", "error", err)
		return exitFailure
	}

	session, err := a.loadSession()
	if err != nil {
		a.logger.Error("load session state", "error", err)
		return exitFailure
	}
	namespace := resolveStartupNamespace(namespaceOverride, session.Namespace)
	root := model.NewRootModel(namespace)
	root.RestoreSession(session)

	defer a.dependencies.stopPortForwards()
	finalModel, err := a.dependencies.newProgram(root).Run()
	if err != nil {
		a.logger.Error("terminal failed", "error", err)
		return exitFailure
	}
	finalRoot, err := rootFromFinalModel(finalModel)
	if err != nil {
		a.logger.Error("terminal returned invalid state", "error", err)
		return exitFailure
	}
	if err := a.dependencies.saveSession(finalRoot); err != nil {
		a.logger.Error("save session state", "error", err)
		return exitFailure
	}
	return exitSuccess
}

func (a application) loadSession() (service.SessionState, error) {
	session, err := a.dependencies.loadSession()
	if err == nil {
		return session, nil
	}
	if errors.Is(err, service.ErrNoSession) {
		return service.SessionState{}, nil
	}
	return service.SessionState{}, err
}

func (a application) writeOutput(content string) int {
	if _, err := io.WriteString(a.output, content); err != nil {
		a.logger.Error("write output", "error", err)
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

func rootFromFinalModel(finalModel tea.Model) (model.RootModel, error) {
	switch finalRoot := finalModel.(type) {
	case model.RootModel:
		return finalRoot, nil
	case *model.RootModel:
		if finalRoot != nil {
			return *finalRoot, nil
		}
	}
	return model.RootModel{}, &finalModelError{modelType: fmt.Sprintf("%T", finalModel)}
}
