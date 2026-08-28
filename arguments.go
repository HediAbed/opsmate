package main

import (
	"fmt"
	"strings"
)

const usageText = `Usage: opsmate [namespace]
       opsmate --help
       opsmate --version

With no namespace, OpsMate restores the saved namespace or uses all namespaces.
`

type command interface {
	execute(application) int
}

type interactiveCommand struct {
	namespace string
}

func (request interactiveCommand) execute(app application) int {
	return app.runInteractive(request.namespace)
}

type helpCommand struct{}

func (helpCommand) execute(app application) int {
	return app.writeOutput(usageText)
}

type versionCommand struct{}

func (versionCommand) execute(app application) int {
	return app.writeOutput(fmt.Sprintf("opsmate %s\n", currentVersion()))
}

type argumentError struct {
	reason string
}

func (e *argumentError) Error() string {
	return e.reason
}

func parseArguments(arguments []string) (command, error) {
	if len(arguments) == 0 {
		return interactiveCommand{}, nil
	}
	if len(arguments) > 1 {
		return nil, &argumentError{reason: "expected at most one argument"}
	}

	argument := arguments[0]
	switch argument {
	case "-h", "--help":
		return helpCommand{}, nil
	case "-v", "--version":
		return versionCommand{}, nil
	case "":
		return nil, &argumentError{reason: "namespace cannot be empty"}
	default:
		if strings.HasPrefix(argument, "-") {
			return nil, &argumentError{reason: fmt.Sprintf("unknown option %q", argument)}
		}
		return interactiveCommand{namespace: argument}, nil
	}
}
