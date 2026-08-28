//go:build !windows

package service

import (
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func isolatePortForwardRegistry(t *testing.T) {
	t.Helper()
	previous := pfRegistry
	pfRegistry = &portForwardRegistry{sessions: map[string]*portForwardProcess{}}
	t.Cleanup(func() { pfRegistry = previous })
}

func TestPortForwardErrorFormatsUnknownCauseAndUnwraps(t *testing.T) {
	sentinel := errors.New("failed")
	unknown := (&PortForwardError{Stage: "start"}).Error()
	if unknown != "port-forward (start): unknown error" {
		t.Fatalf("error = %q", unknown)
	}
	err := &PortForwardError{Namespace: "ns", Pod: "pod", Err: sentinel}
	if !errors.Is(err, sentinel) || !strings.Contains(err.Error(), "ns/pod") {
		t.Fatalf("error = %v, want identity and cause", err)
	}
}

func TestValidatePortForwardRejectsRemotePortOutsideRange(t *testing.T) {
	if err := validatePortForward("ns", "pod", 8080, maxNetworkPort+1); err == nil {
		t.Fatal("invalid remote port was accepted")
	}
}

func TestStartPortForwardReportsProcessStartFailure(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	message := StartPortForward("ns", "pod", 8080, 80)().(PortForwardStartedMsg)

	var portForwardErr *PortForwardError
	if !errors.As(message.Err, &portForwardErr) || portForwardErr.Stage != "start" {
		t.Fatalf("error = %#v, want start-stage failure", message.Err)
	}
}

func TestMergePortForwardKillErrorHandlesExpectedAndUnexpectedFailures(t *testing.T) {
	waitFailure := errors.New("wait failed")
	if got := mergePortForwardKillError(waitFailure, nil); !errors.Is(got, waitFailure) {
		t.Fatalf("nil kill result = %v", got)
	}
	if got := mergePortForwardKillError(waitFailure, os.ErrProcessDone); !errors.Is(got, waitFailure) {
		t.Fatalf("completed-process result = %v", got)
	}
	killFailure := errors.New("kill failed")
	got := mergePortForwardKillError(waitFailure, killFailure)
	if !errors.Is(got, waitFailure) || !errors.Is(got, killFailure) {
		t.Fatalf("joined result = %v", got)
	}
}

func TestWaitForPortForwardExitRejectsIncompleteHandles(t *testing.T) {
	for _, handle := range []*PortForwardHandle{nil, {}, {process: &portForwardProcess{}}} {
		if WaitForPortForwardExit(handle) != nil {
			t.Fatalf("handle %#v returned an exit command", handle)
		}
	}
}

func TestStopPortForwardReportsKillFailureAndTimeout(t *testing.T) {
	isolatePortForwardRegistry(t)
	killFailure := errors.New("kill failed")
	completed := testPortForwardProcess(PortForwardSession{ID: "completed", Namespace: "ns", Pod: "pod"})
	completed.kill = func() error { return killFailure }
	close(completed.done)
	pfRegistry.add(completed)

	message := stopPortForwardWithTimeout(completed.info.ID, time.Millisecond)().(PortForwardStoppedMsg)
	if !errors.Is(message.Err, killFailure) {
		t.Fatalf("kill error = %v, want sentinel", message.Err)
	}

	stalled := testPortForwardProcess(PortForwardSession{ID: "stalled", Namespace: "ns", Pod: "pod"})
	stalled.kill = func() error { return nil }
	pfRegistry.add(stalled)
	message = stopPortForwardWithTimeout(stalled.info.ID, time.Millisecond)().(PortForwardStoppedMsg)
	if message.Err == nil || !strings.Contains(message.Err.Error(), "did not exit") {
		t.Fatalf("timeout error = %v", message.Err)
	}
	close(stalled.done)
}

func TestStopAllPortForwardsStopsActiveProcessesAndHonorsTimeout(t *testing.T) {
	isolatePortForwardRegistry(t)
	var stops atomic.Int32
	completed := testPortForwardProcess(PortForwardSession{ID: "completed"})
	completed.kill = func() error {
		stops.Add(1)
		return nil
	}
	close(completed.done)
	pfRegistry.add(completed)
	stopAllPortForwardsWithTimeout(time.Millisecond)
	if stops.Load() != 1 {
		t.Fatalf("stop calls = %d, want 1", stops.Load())
	}

	stalled := testPortForwardProcess(PortForwardSession{ID: "stalled"})
	stalled.kill = func() error { return nil }
	pfRegistry.add(stalled)
	stopAllPortForwardsWithTimeout(time.Millisecond)
	close(stalled.done)
}

func TestPortForwardRegistryHandlesMissingFinishAndReturnsActiveProcesses(t *testing.T) {
	registry := &portForwardRegistry{sessions: map[string]*portForwardProcess{}}
	registry.finish("missing", errors.New("ignored"))
	process := testPortForwardProcess(PortForwardSession{ID: "active"})
	registry.add(process)
	active := registry.active()
	if len(active) != 1 || active[0] != process {
		t.Fatalf("active processes = %#v", active)
	}
}

func TestPortForwardProcessErrorSuppliesMissingExitCause(t *testing.T) {
	err := portForwardProcessError("readiness", "ns", "pod", nil, "")
	if err == nil || !strings.Contains(err.Error(), "exited before reporting readiness") {
		t.Fatalf("error = %v", err)
	}
}
