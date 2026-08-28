package main

import (
	"io"
	"log/slog"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/model"
	"github.com/HediAbed/opsmate/internal/service"
)

func main() {
	application := newApplication(os.Stdout, os.Stderr)
	os.Exit(application.run(os.Args[1:]))
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
			loadEnvironment:    service.LoadDotEnvFromExecutableDir,
			initializeProvider: service.InitAIProvider,
			loadSession:        service.LoadSession,
			newProgram:         newTerminalProgram,
			saveSession:        model.RootModel.SaveOnExit,
			stopPortForwards:   service.StopAllPortForwards,
		},
	}
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
