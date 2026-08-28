//go:build windows

package service

import (
	"os"
	"os/exec"
)

func configureShellCommand(_ *exec.Cmd) {}

func interruptShellProcess(_ *os.Process) error {
	return ErrShellInterruptUnsupported
}
