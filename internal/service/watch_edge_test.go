//go:build !windows

package service

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"
)

func TestWatcherRunCommandReportsStdoutPipeFailure(t *testing.T) {
	watcher := &watcher[Pod]{events: make(chan WatchEvent[Pod], 2), stop: make(chan struct{})}
	command := exec.Command("sh")
	command.Stdout = io.Discard

	watcher.runCommand(context.Background(), command, "pods", decodePodObject)

	first := <-watcher.events
	second := <-watcher.events
	var streamErr *WatchStreamError
	if first.Kind != WatchErrored || !errors.As(first.Err, &streamErr) || streamErr.Stage != "stdout pipe" {
		t.Fatalf("first event = %#v", first)
	}
	if second.Kind != WatchClosed {
		t.Fatalf("second event = %#v, want closed", second)
	}
}

func TestWatcherRunStopsWhenDispatchIsCanceled(t *testing.T) {
	command := exec.Command("sh", "-c", "sleep 5")
	if err := command.Start(); err != nil {
		t.Fatalf("start process: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	watcher := &watcher[Pod]{events: make(chan WatchEvent[Pod]), stop: make(chan struct{})}
	line := `{"type":"BOOKMARK","object":{}}` + "\n"

	watcher.run(
		ctx,
		command,
		io.NopCloser(strings.NewReader(line)),
		newLimitedBuffer(100),
		"pods",
		decodePodObject,
	)
}

func TestWatcherRunReportsReaderFailure(t *testing.T) {
	command := exec.Command("sh", "-c", "sleep 5")
	if err := command.Start(); err != nil {
		t.Fatalf("start process: %v", err)
	}
	watcher := &watcher[Pod]{events: make(chan WatchEvent[Pod], 2), stop: make(chan struct{})}
	sentinel := errors.New("read failed")

	watcher.run(
		context.Background(),
		command,
		&failingProviderBody{readErr: sentinel},
		newLimitedBuffer(100),
		"pods",
		decodePodObject,
	)

	first := <-watcher.events
	second := <-watcher.events
	if first.Kind != WatchErrored || !errors.Is(first.Err, sentinel) {
		t.Fatalf("first event = %#v, want read failure", first)
	}
	if second.Kind != WatchClosed {
		t.Fatalf("second event = %#v, want closed", second)
	}
}

func TestWatcherErrorReportingStopsAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	watcher := &watcher[Pod]{events: make(chan WatchEvent[Pod]), stop: make(chan struct{})}
	sentinel := errors.New("failed")

	if watcher.reportReadError(ctx, "pods", sentinel) {
		t.Fatal("read error reported after cancellation")
	}
	if watcher.reportExitError(ctx, "pods", "diagnostic", sentinel, nil) {
		t.Fatal("exit error reported after cancellation")
	}

	watcher.reportTermination(ctx, "pods", "diagnostic", nil, sentinel)
	watcher.reportTermination(ctx, "pods", "diagnostic", sentinel, nil)
}

func TestWatcherDispatchLineHandlesCanceledAndEmptyFrames(t *testing.T) {
	watcher := &watcher[Pod]{events: make(chan WatchEvent[Pod], 1), stop: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if watcher.dispatchLine(ctx, []byte(`{"type":"BOOKMARK"}`), "pods", decodePodObject) {
		t.Fatal("canceled dispatch continued")
	}
	if !watcher.dispatchLine(context.Background(), nil, "pods", decodePodObject) {
		t.Fatal("empty frame stopped the watcher")
	}
}

func TestParseWatchLineRejectsInternalEventKinds(t *testing.T) {
	for _, kind := range []WatchEventKind{WatchClosed, WatchErrored} {
		line := []byte(`{"type":"` + string(kind) + `","object":{}}`)
		if _, err := parseWatchLine(line, "pods", decodePodObject); err == nil {
			t.Fatalf("event kind %q did not fail", kind)
		}
	}
}

func TestDecodeDeploymentObjectProjectsContainers(t *testing.T) {
	deployment, err := decodeDeploymentObject([]byte(`{
		"metadata":{"name":"web","namespace":"ns"},
		"spec":{"template":{"spec":{"containers":[{"name":"main","image":"web:v1"}]}}}
	}`))
	if err != nil {
		t.Fatalf("decode deployment: %v", err)
	}
	if len(deployment.Containers) != 1 || deployment.Containers[0] != "main" || deployment.Images[0] != "web:v1" {
		t.Fatalf("deployment = %#v", deployment)
	}
}
