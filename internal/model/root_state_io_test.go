//go:build !windows

package model

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/service"
)

func TestRootPersistSessionSurfacesStorageFailure(t *testing.T) {
	configBlocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(configBlocker, []byte("block"), 0o600); err != nil {
		t.Fatalf("create config blocker: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", configBlocker)

	model := freshRoot(t)
	model.persistSession()
	if model.err == nil || !strings.Contains(model.err.Error(), "save session") {
		t.Fatalf("persistence error = %v", model.err)
	}
}

func TestRootHandlesLivePortForwardLifecycle(t *testing.T) {
	installFakeKubectl(t, "#!/bin/sh\nprintf 'Forwarding from 127.0.0.1:18080 -> 80\\n'\nexec sleep 30\n")
	service.StopAllPortForwards()
	t.Cleanup(service.StopAllPortForwards)

	started := requirePortForwardStarted(t, service.StartPortForward("default", "worker", 18080, 80)())

	model := freshRoot(t)
	waitCommand := model.handlePortForwardStarted(started)
	assertRootTracksPortForward(t, model, waitCommand != nil)

	model.pfCursor = 99
	model.refreshPFSessions()
	if model.pfCursor != 0 {
		t.Errorf("refresh cursor = %d, want 0", model.pfCursor)
	}

	model.pfCursor = 99
	model.handlePortForwardStopped(service.PortForwardStoppedMsg{ID: model.pfSessions[0].ID, Err: errStub("unexpected exit")})
	assertRootHandledStoppedPortForward(t, model)

	service.StopAllPortForwards()
	requirePortForwardStopped(t, waitCommand(), started.Session.ID)
}

func requirePortForwardStarted(t *testing.T, message tea.Msg) service.PortForwardStartedMsg {
	t.Helper()
	started, ok := message.(service.PortForwardStartedMsg)
	if !ok {
		t.Fatalf("start response type = %T", message)
	}
	if started.Err != nil || started.Session == nil {
		t.Fatalf("start response = %#v", started)
	}
	return started
}

func assertRootTracksPortForward(t *testing.T, model RootModel, hasWaitCommand bool) {
	t.Helper()
	if !hasWaitCommand || len(model.pfSessions) != 1 {
		t.Fatalf("root start handling: has command=%t sessions=%v", hasWaitCommand, model.pfSessions)
	}
}

func assertRootHandledStoppedPortForward(t *testing.T, model RootModel) {
	t.Helper()
	if model.err == nil || model.pfCursor != 0 {
		t.Errorf("stopped handling: err=%v cursor=%d", model.err, model.pfCursor)
	}
}

func requirePortForwardStopped(t *testing.T, message tea.Msg, expectedID string) {
	t.Helper()
	stopped, ok := message.(service.PortForwardStoppedMsg)
	if !ok {
		t.Fatalf("wait response type = %T", message)
	}
	if stopped.ID != expectedID {
		t.Errorf("stopped ID = %q, want %q", stopped.ID, expectedID)
	}
}

func TestRootContextListFailureClearsLoadingAndSetsError(t *testing.T) {
	model := freshRoot(t)
	model.ctxLoading = true
	updated, _ := model.Update(service.ContextsMsg{Err: errStub("context list unavailable")})
	root := updated.(RootModel)
	if root.ctxLoading || root.err == nil {
		t.Errorf("context failure: loading=%v err=%v", root.ctxLoading, root.err)
	}
}
