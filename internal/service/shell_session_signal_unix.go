//go:build !windows

package service

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func configureShellCommand(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		if command.Process == nil {
			return os.ErrProcessDone
		}
		err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
}

func interruptShellProcess(process *os.Process) error {
	err := syscall.Kill(-process.Pid, syscall.SIGINT)
	if errors.Is(err, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	return err
}
