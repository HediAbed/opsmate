//go:build !windows

package service

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

type shellTestWriteCloser struct {
	writeErr error
	closeErr error
}

func (writer *shellTestWriteCloser) Write(data []byte) (int, error) {
	if writer.writeErr != nil {
		return 0, writer.writeErr
	}
	return len(data), nil
}

func (writer *shellTestWriteCloser) Close() error { return writer.closeErr }

func TestShellStreamErrorHandlesMissingCause(t *testing.T) {
	err := &ShellStreamError{Stream: "stdout"}
	if got := err.Error(); got != "shell: stdout stream failed" {
		t.Fatalf("error = %q", got)
	}
}

func TestStartShellSessionReportsProcessStartFailure(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := StartShellSession("ns", "pod", ""); err == nil || !strings.Contains(err.Error(), "start") {
		t.Fatalf("error = %v, want process start failure", err)
	}
}

func TestOpenShellProcessPipesReportsEveryPipeFailure(t *testing.T) {
	stdinFailure := exec.Command("sh")
	stdinFailure.Stdin = strings.NewReader("")
	if _, err := openShellProcessPipes(stdinFailure); err == nil || !strings.Contains(err.Error(), "stdin") {
		t.Fatalf("stdin error = %v", err)
	}

	stdoutFailure := exec.Command("sh")
	stdoutFailure.Stdout = io.Discard
	if _, err := openShellProcessPipes(stdoutFailure); err == nil || !strings.Contains(err.Error(), "stdout") {
		t.Fatalf("stdout error = %v", err)
	}

	stderrFailure := exec.Command("sh")
	stderrFailure.Stderr = io.Discard
	if _, err := openShellProcessPipes(stderrFailure); err == nil || !strings.Contains(err.Error(), "stderr") {
		t.Fatalf("stderr error = %v", err)
	}
}

func TestStartShellProcessCancelsAfterPipeFailure(t *testing.T) {
	command := exec.Command("sh")
	command.Stdin = strings.NewReader("")
	ctx, cancelContext := context.WithCancel(context.Background())
	var cancellations atomic.Int32
	cancel := func() {
		cancellations.Add(1)
		cancelContext()
	}

	if _, err := startShellProcess(ctx, cancel, command, "ns", "pod", ""); err == nil {
		t.Fatal("pipe failure was not returned")
	}
	if cancellations.Load() != 1 {
		t.Fatalf("cancellations = %d, want 1", cancellations.Load())
	}
}

func TestShellProcessPipesCloseJoinsStreamFailures(t *testing.T) {
	sentinel := errors.New("close failed")
	pipes := shellProcessPipes{
		stdin:  &shellTestWriteCloser{closeErr: sentinel},
		stdout: io.NopCloser(strings.NewReader("")),
	}
	err := pipes.close()
	if !errors.Is(err, sentinel) || !strings.Contains(err.Error(), "stdin") {
		t.Fatalf("error = %v, want named close failure", err)
	}
}

func TestShellSessionNilAndClosedOperations(t *testing.T) {
	var session *ShellSession
	if !errors.Is(session.Send("line"), ErrShellNilSession) {
		t.Fatal("nil send did not return ErrShellNilSession")
	}
	if identity := session.Identity(); identity != (ShellSessionIdentity{}) {
		t.Fatalf("nil identity = %#v", identity)
	}
	session.Close()

	var cancellations atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	session = &ShellSession{
		ctx:    ctx,
		cancel: func() { cancellations.Add(1) },
		input:  make(chan string),
	}
	if !errors.Is(session.Send("line"), ErrShellSessionClosed) {
		t.Fatal("canceled send did not return ErrShellSessionClosed")
	}
	session.Close()
	session.Close()
	if cancellations.Load() != 1 {
		t.Fatalf("cancellations = %d, want 1", cancellations.Load())
	}
}

func TestShellSessionWriteInputReportsWriteAndCloseFailures(t *testing.T) {
	closeFailure := errors.New("close failed")
	closedContext, cancelClosed := context.WithCancel(context.Background())
	cancelClosed()
	closeSession := &ShellSession{
		ctx:       closedContext,
		cancel:    func() {},
		stdin:     &shellTestWriteCloser{closeErr: closeFailure},
		input:     make(chan string),
		inputDone: make(chan struct{}),
	}
	closeSession.writeInput()
	if !errors.Is(closeSession.getAsyncError(), closeFailure) {
		t.Fatalf("close error = %v, want sentinel", closeSession.getAsyncError())
	}

	writeFailure := errors.New("write failed")
	ctx, cancel := context.WithCancel(context.Background())
	writeSession := &ShellSession{
		ctx:       ctx,
		cancel:    cancel,
		stdin:     &shellTestWriteCloser{writeErr: writeFailure},
		input:     make(chan string, 1),
		inputDone: make(chan struct{}),
	}
	writeSession.input <- "command\n"
	writeSession.writeInput()
	if !errors.Is(writeSession.getAsyncError(), writeFailure) {
		t.Fatalf("write error = %v, want sentinel", writeSession.getAsyncError())
	}
}

func TestShellSessionStreamPipeStopsAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	session := &ShellSession{ctx: ctx, output: make(chan ShellOutput)}
	var done sync.WaitGroup
	done.Add(1)
	errorsOut := make(chan error, 1)

	session.streamPipe(strings.NewReader("ignored\n"), "stdout", false, &done, errorsOut)

	done.Wait()
	if err := <-errorsOut; err != nil {
		t.Fatalf("stream error = %v", err)
	}
}

func TestShellSessionWaitForExitCollectsAllFailureSources(t *testing.T) {
	command := exec.Command("sh", "-c", "exit 2")
	if err := command.Start(); err != nil {
		t.Fatalf("start command: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	inputDone := make(chan struct{})
	close(inputDone)
	session := &ShellSession{
		identity:  ShellSessionIdentity{ID: "shell-test"},
		cmd:       command,
		ctx:       ctx,
		cancel:    cancel,
		inputDone: inputDone,
		output:    make(chan ShellOutput),
		exit:      make(chan ShellExitError, 1),
	}
	asyncFailure := errors.New("input failed")
	session.setAsyncError(asyncFailure)
	session.setAsyncError(errors.New("later failure"))
	session.droppedOutput.Store(2)
	pipeFailure := errors.New("pipe failed")
	pipeErrors := make(chan error, shellStreamCount)
	pipeErrors <- pipeFailure
	pipeErrors <- nil
	var pipes sync.WaitGroup

	session.waitForExit(&pipes, pipeErrors)

	exit := <-session.exit
	if !errors.Is(exit.Err, pipeFailure) || !errors.Is(exit.Err, asyncFailure) {
		t.Fatalf("exit error = %v, want pipe and input failures", exit.Err)
	}
	var dropped *ShellOutputDroppedError
	if !errors.As(exit.Err, &dropped) || dropped.Count != 2 {
		t.Fatalf("exit error = %v, want dropped-output count", exit.Err)
	}
	if !strings.Contains(exit.Err.Error(), "exit status 2") {
		t.Fatalf("exit error = %v, want process failure", exit.Err)
	}
	if !errors.Is(session.getAsyncError(), asyncFailure) {
		t.Fatal("later asynchronous error replaced the first one")
	}
}

func TestShellSignalHelpersHandleMissingProcesses(t *testing.T) {
	command := &exec.Cmd{}
	configureShellCommand(command)
	if !errors.Is(command.Cancel(), os.ErrProcessDone) {
		t.Fatal("cancel before start did not report completed process")
	}
	command.Process = &os.Process{Pid: 99_999_999}
	if !errors.Is(command.Cancel(), os.ErrProcessDone) {
		t.Fatal("cancel for missing process did not report completed process")
	}
	if !errors.Is(interruptShellProcess(command.Process), os.ErrProcessDone) {
		t.Fatal("interrupt for missing process did not report completed process")
	}
}
