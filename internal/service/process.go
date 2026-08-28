package service

import (
	"context"
	"os/exec"
)

func newExternalCommandContext(ctx context.Context, executable string, arguments ...string) *exec.Cmd {
	// #nosec G204 -- executable selection is package-controlled and arguments are passed directly without a shell.
	return exec.CommandContext(ctx, executable, arguments...)
}

func newExternalCommand(executable string, arguments ...string) *exec.Cmd {
	// #nosec G204 -- executable selection is package-controlled and arguments are passed directly without a shell.
	return exec.Command(executable, arguments...)
}
