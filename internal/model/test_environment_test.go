package model

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestMain(testSuite *testing.M) {
	commandDirectory, err := prepareModelTestEnvironment()
	if err != nil {
		panic(err)
	}
	defer removeModelTestDirectory(commandDirectory)
	testSuite.Run()
}

func prepareModelTestEnvironment() (string, error) {
	commandDirectory, err := os.MkdirTemp("", "opsmate-model-tests-")
	if err != nil {
		return "", fmt.Errorf("create model test command directory: %w", err)
	}
	if err := publishTestExecutable(filepath.Join(commandDirectory, "kubectl"), "#!/bin/sh\nexit 0\n"); err != nil {
		removeModelTestDirectory(commandDirectory)
		return "", fmt.Errorf("install model test command: %w", err)
	}
	if err := os.Setenv("PATH", commandDirectory+string(os.PathListSeparator)+os.Getenv("PATH")); err != nil {
		removeModelTestDirectory(commandDirectory)
		return "", fmt.Errorf("configure model test command path: %w", err)
	}
	return commandDirectory, nil
}

func removeModelTestDirectory(path string) {
	if err := os.RemoveAll(path); err != nil {
		panic(fmt.Errorf("remove model test command directory: %w", err))
	}
}
