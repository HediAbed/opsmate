//go:build !windows

package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestShellSession_IdentityFlowsThroughOutputAndExit(t *testing.T) {
	path := newFakeKubectl(t, `printf '\033[31mready\033[0m\n'`)
	withFakeKubectl(t, path)

	session, err := StartShellSession("ops", "api", "sidecar")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	identity := session.Identity()
	if identity.ID == "" || identity.Namespace != "ops" || identity.Pod != "api" || identity.Container != "sidecar" {
		t.Fatalf("identity = %+v", identity)
	}
	output := <-session.Output()
	if output.SessionID != identity.ID || output.Line != "ready" {
		t.Fatalf("output = %+v, want matching ID and sanitized line", output)
	}
	exit := <-session.Exit()
	if exit.SessionID != identity.ID || exit.Err != nil {
		t.Fatalf("exit = %+v, want clean matching session", exit)
	}
}

func TestShellSession_SendReportsBackpressure(t *testing.T) {
	session := &ShellSession{
		ctx:   context.Background(),
		input: make(chan string, 1),
	}
	session.input <- "queued"
	if err := session.Send("next"); !errors.Is(err, ErrShellInputBackpressure) {
		t.Fatalf("Send error = %v, want ErrShellInputBackpressure", err)
	}
}

func TestShellSession_StreamPipeReportsOversizedLine(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	session := &ShellSession{
		identity: ShellSessionIdentity{ID: "shell-test"},
		output:   make(chan ShellOutput, 1),
		ctx:      ctx,
		cancel:   cancel,
	}
	errorsOut := make(chan error, 1)
	var done sync.WaitGroup
	done.Add(1)
	go session.streamPipe(
		strings.NewReader(strings.Repeat("x", shellScanBufferMax+1)),
		"stdout",
		false,
		&done,
		errorsOut,
	)
	done.Wait()
	err := <-errorsOut
	var streamErr *ShellStreamError
	if !errors.As(err, &streamErr) || streamErr.Stream != "stdout" {
		t.Fatalf("error = %#v, want stdout ShellStreamError", err)
	}
}

func TestShellSession_EmitOutputCountsDroppedLines(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	session := &ShellSession{ctx: ctx, output: make(chan ShellOutput), cancel: cancel}
	if !session.emitOutput(ShellOutput{Line: "dropped"}) {
		t.Fatal("backpressure must not stop pipe draining")
	}
	if got := session.droppedOutput.Load(); got != 1 {
		t.Fatalf("dropped lines = %d, want 1", got)
	}
	cancel()
	if session.emitOutput(ShellOutput{Line: "ignored"}) {
		t.Fatal("canceled session must stop output")
	}
}

func TestShellSession_InterruptRunningProcess(t *testing.T) {
	path := newFakeKubectl(t, `exec sleep 5`)
	withFakeKubectl(t, path)
	session, err := StartShellSession("ns", "pod", "")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := session.Interrupt(); err != nil {
		t.Fatalf("interrupt: %v", err)
	}
	select {
	case <-session.Exit():
	case <-time.After(2 * time.Second):
		t.Fatal("interrupted process did not exit")
	}
}

func TestShellErrorTypes_PreserveDetails(t *testing.T) {
	wantErr := errors.New("stream failed")
	streamErr := &ShellStreamError{Stream: "stderr", Err: wantErr}
	if !errors.Is(streamErr, wantErr) || !strings.Contains(streamErr.Error(), "stderr") {
		t.Fatalf("stream error = %v", streamErr)
	}
	droppedErr := &ShellOutputDroppedError{Count: 3}
	if !strings.Contains(droppedErr.Error(), "3 output lines") {
		t.Fatalf("dropped error = %v", droppedErr)
	}
}
