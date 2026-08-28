package service

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestDescribeResource_NamespaceRequired(t *testing.T) {
	msg := DescribeResource("", "pod", "x")().(DescribeMsg)
	if !errors.Is(msg.Err, ErrNamespaceRequired) {
		t.Errorf("empty ns should produce ErrNamespaceRequired; got %v", msg.Err)
	}
}

func TestDescribeResource_HappyPath(t *testing.T) {
	withFakePathKubectl(t, `printf 'describe output here'`)
	msg := DescribeResource("ns", "pod", "x")().(DescribeMsg)
	if msg.Err != nil {
		t.Fatalf("err: %v", msg.Err)
	}
	if msg.Output != "describe output here" {
		t.Errorf("output = %q", msg.Output)
	}
}

func TestDescribeResource_KubectlError(t *testing.T) {
	withFakePathKubectl(t, `printf 'denied' 1>&2; exit 1`)
	msg := DescribeResource("ns", "pod", "x")().(DescribeMsg)
	if msg.Err == nil {
		t.Error("expected error")
	}
}

func TestFetchContainerLogs_NamespaceRequired(t *testing.T) {
	msg := FetchContainerLogs("", "p", "c", 100)().(LogsMsg)
	if !errors.Is(msg.Err, ErrNamespaceRequired) {
		t.Error("empty ns should error")
	}
}

func TestFetchContainerLogs_HappyPath(t *testing.T) {
	withFakePathKubectl(t, `printf 'line 1\nline 2\nline 3'`)
	msg := FetchContainerLogs("ns", "p", "c", 100)().(LogsMsg)
	if msg.Err != nil {
		t.Fatalf("err: %v", msg.Err)
	}
	if len(msg.Lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(msg.Lines))
	}
}

func TestFetchContainers_NamespaceRequired(t *testing.T) {
	msg := FetchContainers("", "p")().(ContainersMsg)
	if !errors.Is(msg.Err, ErrNamespaceRequired) {
		t.Error("empty ns should error")
	}
}

func TestFetchContainers_HappyPath(t *testing.T) {
	withFakePathKubectl(t, `printf 'main sidecar'`)
	msg := FetchContainers("ns", "p")().(ContainersMsg)
	if msg.Err != nil {
		t.Fatalf("err: %v", msg.Err)
	}
	if len(msg.Containers) != 2 {
		t.Errorf("expected 2 containers, got %v", msg.Containers)
	}
}

func TestScaleResource_NamespaceRequired(t *testing.T) {
	msg := ScaleResource("", "deploy", "x", 3)().(CommandResultMsg)
	if !errors.Is(msg.Err, ErrNamespaceRequired) {
		t.Error("empty ns should error")
	}
}

func TestScaleResource_HappyPath(t *testing.T) {
	withFakePathKubectl(t, `printf 'deployment.apps/x scaled'`)
	msg := ScaleResource("ns", "deploy", "x", 3)().(CommandResultMsg)
	if msg.Err != nil {
		t.Fatalf("err: %v", msg.Err)
	}
	if !strings.Contains(msg.Output, "scaled") {
		t.Error("output should contain 'scaled'")
	}
}

func TestDeleteResource_NamespaceRequired(t *testing.T) {
	msg := DeleteResource("", "pod", "x")().(CommandResultMsg)
	if !errors.Is(msg.Err, ErrNamespaceRequired) {
		t.Error("empty ns should error")
	}
}

func TestDeleteResource_HappyPath(t *testing.T) {
	withFakePathKubectl(t, `printf 'pod/x deleted'`)
	msg := DeleteResource("ns", "pod", "x")().(CommandResultMsg)
	if msg.Err != nil {
		t.Errorf("err: %v", msg.Err)
	}
}

func TestDeleteResources_EmptyNamesIsError(t *testing.T) {
	msg := DeleteResources("ns", "pod", nil)().(CommandResultMsg)
	if msg.Err == nil {
		t.Error("empty names slice should error")
	}
}

func TestDeleteResources_NamespaceRequired(t *testing.T) {
	msg := DeleteResources("", "pod", []string{"x"})().(CommandResultMsg)
	if !errors.Is(msg.Err, ErrNamespaceRequired) {
		t.Error("empty ns should error")
	}
}

func TestDeleteResources_HappyPath(t *testing.T) {
	withFakePathKubectl(t, `printf 'pod/a deleted\npod/b deleted'`)
	msg := DeleteResources("ns", "pod", []string{"a", "b"})().(CommandResultMsg)
	if msg.Err != nil {
		t.Errorf("err: %v", msg.Err)
	}
}

func TestGetYAML_NamespaceRequired(t *testing.T) {
	msg := GetYAML("", "pod", "x")().(YAMLMsg)
	if !errors.Is(msg.Err, ErrNamespaceRequired) {
		t.Error("empty ns should error")
	}
}

func TestGetYAML_HappyPath(t *testing.T) {
	withFakePathKubectl(t, `printf 'apiVersion: v1\nkind: Pod'`)
	msg := GetYAML("ns", "pod", "x")().(YAMLMsg)
	if msg.Err != nil || !strings.Contains(msg.Output, "apiVersion") {
		t.Errorf("yaml fetch wrong: %+v", msg)
	}
}

func TestGetYAML_RejectsSecretResources(t *testing.T) {
	for _, resource := range []string{"secret", "Secrets"} {
		t.Run(resource, func(t *testing.T) {
			msg := GetYAML("ns", resource, "database")().(YAMLMsg)
			if !errors.Is(msg.Err, ErrSensitiveDataCommand) {
				t.Fatalf("error = %v, want sensitive-data policy error", msg.Err)
			}
		})
	}
}

func TestRestartRollout_NamespaceRequired(t *testing.T) {
	msg := RestartRollout("", "deploy", "x")().(CommandResultMsg)
	if !errors.Is(msg.Err, ErrNamespaceRequired) {
		t.Error("empty ns should error")
	}
}

func TestRestartRollout_HappyPath(t *testing.T) {
	withFakePathKubectl(t, `printf 'deployment.apps/x restarted'`)
	msg := RestartRollout("ns", "deploy", "x")().(CommandResultMsg)
	if msg.Err != nil {
		t.Errorf("err: %v", msg.Err)
	}
}

func TestRestartRollouts_EmptyNamesIsError(t *testing.T) {
	msg := RestartRollouts("ns", "deploy", nil)().(CommandResultMsg)
	if msg.Err == nil {
		t.Error("empty names should error")
	}
}

func TestRestartRollouts_NamespaceRequired(t *testing.T) {
	msg := RestartRollouts("", "deploy", []string{"x"})().(CommandResultMsg)
	if !errors.Is(msg.Err, ErrNamespaceRequired) {
		t.Error("empty ns should error")
	}
}

func TestRestartRollouts_HappyPath(t *testing.T) {
	withFakePathKubectl(t, `printf 'restarted'`)
	msg := RestartRollouts("ns", "deploy", []string{"a", "b"})().(CommandResultMsg)
	if msg.Err != nil {
		t.Errorf("err: %v", msg.Err)
	}
}

func TestStartPortForward_NegativePortsRejected(t *testing.T) {
	msg := StartPortForward("ns", "p", -1, 80)().(PortForwardStartedMsg)
	if msg.Err == nil {
		t.Error("negative local port should error")
	}
}

func TestStartPortForward_NamespaceRequired(t *testing.T) {
	msg := StartPortForward("", "p", 8080, 80)().(PortForwardStartedMsg)
	if msg.Err == nil {
		t.Error("empty namespace should error")
	}
}

func TestStartPortForward_HappyPath_RegistersSessionThenStop(t *testing.T) {
	withFakePathKubectl(t, `printf 'Forwarding from 127.0.0.1:8080 -> 80\n'; exec sleep 5`)
	msg := StartPortForward("ns", "alpha", 8080, 80)().(PortForwardStartedMsg)
	if msg.Err != nil {
		t.Fatalf("start err: %v", msg.Err)
	}
	if msg.Session == nil {
		t.Fatal("session should be returned")
	}
	if msg.Session.LocalPort != 8080 || msg.Session.RemotePort != 80 {
		t.Errorf("session ports wrong: %+v", msg.Session)
	}

	found := false
	for _, s := range ListPortForwards() {
		if s.ID == msg.Session.ID {
			found = true
		}
	}
	if !found {
		t.Error("session should be registered")
	}

	stopMsg := StopPortForward(msg.Session.ID)().(PortForwardStoppedMsg)
	if stopMsg.ID != msg.Session.ID {
		t.Errorf("stop msg ID = %q, want %q", stopMsg.ID, msg.Session.ID)
	}
	if stopMsg.Err != nil {
		t.Fatalf("stop err = %v, want nil", stopMsg.Err)
	}
}

func TestStartPortForward_EmptyPodRejected(t *testing.T) {
	msg := StartPortForward("ns", "", 8080, 80)().(PortForwardStartedMsg)
	if msg.Err == nil {
		t.Error("empty pod should error")
	}
}

func TestStartPortForward_PortAboveNetworkRangeRejected(t *testing.T) {
	msg := StartPortForward("ns", "pod", maxNetworkPort+1, 80)().(PortForwardStartedMsg)
	if msg.Err == nil {
		t.Fatal("out-of-range local port must be rejected")
	}
	var portForwardErr *PortForwardError
	if !errors.As(msg.Err, &portForwardErr) || portForwardErr.Stage != "validate" {
		t.Fatalf("error = %#v, want validation PortForwardError", msg.Err)
	}
}

func TestStartPortForward_EarlyExitIncludesDiagnostic(t *testing.T) {
	withFakePathKubectl(t, `printf 'unable to bind requested port\n' >&2; exit 2`)
	msg := StartPortForward("ns", "pod", 8080, 80)().(PortForwardStartedMsg)
	if msg.Err == nil || !strings.Contains(msg.Err.Error(), "unable to bind requested port") {
		t.Fatalf("error = %v, want kubectl diagnostic", msg.Err)
	}
	var portForwardErr *PortForwardError
	if !errors.As(msg.Err, &portForwardErr) || portForwardErr.Stage != "readiness" {
		t.Fatalf("error = %#v, want readiness PortForwardError", msg.Err)
	}
}

func TestStartPortForward_ReadinessTimeoutKillsProcess(t *testing.T) {
	withFakePathKubectl(t, `exec sleep 5`)
	msg := startPortForwardWithTimeout("ns", "pod", 8080, 80, 25*time.Millisecond)().(PortForwardStartedMsg)
	if !errors.Is(msg.Err, ErrPortForwardReadinessTimeout) {
		t.Fatalf("error = %v, want ErrPortForwardReadinessTimeout", msg.Err)
	}
	if msg.Session != nil {
		t.Fatal("timed-out process must not return a session")
	}
}

func TestWaitForPortForwardExit_ReportsNaturalFailureAndRemovesSession(t *testing.T) {
	withFakePathKubectl(t, `printf 'Forwarding from 127.0.0.1:8080 -> 80\n'; sleep 0.05; printf 'pod disappeared\n' >&2; exit 3`)
	started := StartPortForward("ns", "pod", 8080, 80)().(PortForwardStartedMsg)
	if started.Err != nil {
		t.Fatalf("start: %v", started.Err)
	}
	wait := WaitForPortForwardExit(started.Session)
	if wait == nil {
		t.Fatal("live session must return an exit waiter")
	}
	stopped := wait().(PortForwardStoppedMsg)
	if stopped.ID != started.Session.ID {
		t.Fatalf("stopped ID = %q, want %q", stopped.ID, started.Session.ID)
	}
	if stopped.Err == nil || !strings.Contains(stopped.Err.Error(), "pod disappeared") {
		t.Fatalf("exit error = %v, want kubectl diagnostic", stopped.Err)
	}
	for _, session := range ListPortForwards() {
		if session.ID == stopped.ID {
			t.Fatal("naturally exited session must leave registry")
		}
	}
}
