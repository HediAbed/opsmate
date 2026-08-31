package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/kube"
)

func TestRootHandlesLivePortForwardLifecycle(t *testing.T) {
	model := freshRoot(t)
	operations := &testClusterOperations{}
	installTestRootOperations(&model, operations)
	sessionInfo := testModelPortForwardSession(t, "forward-1", "worker", 18080, 80)
	session := &testPortForward{session: sessionInfo, exit: make(chan kube.PortForwardExit, 1)}
	operations.portForwards = []kube.PortForwardSession{sessionInfo}
	started := portForwardStartedMsg{Session: session}
	waitCommand := model.handlePortForwardStarted(started)
	assertRootTracksPortForward(t, model, waitCommand != nil)

	model.pfCursor = 99
	model.refreshPFSessions()
	if model.pfCursor != 0 {
		t.Errorf("refresh cursor = %d, want 0", model.pfCursor)
	}

	model.pfCursor = 99
	operations.portForwards = nil
	model.handlePortForwardStopped(portForwardStoppedMsg{SessionID: model.pfSessions[0].ID, Err: errStub("unexpected exit")})
	assertRootHandledStoppedPortForward(t, model)

	operations.portForwards = []kube.PortForwardSession{sessionInfo}
	model.pfCursor = 99
	model.handlePortForwardStopped(portForwardStoppedMsg{SessionID: "another"})
	if model.pfCursor != 0 {
		t.Fatalf("non-empty stopped cursor = %d, want 0", model.pfCursor)
	}

	session.exit <- kube.PortForwardExit{SessionID: sessionInfo.ID}
	requirePortForwardStopped(t, waitCommand(), sessionInfo.ID)
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
	stopped, ok := message.(portForwardStoppedMsg)
	if !ok {
		t.Fatalf("wait response type = %T", message)
	}
	if stopped.SessionID != expectedID {
		t.Errorf("stopped ID = %q, want %q", stopped.SessionID, expectedID)
	}
}

func TestRootContextListFailureClearsLoadingAndSetsError(t *testing.T) {
	model := freshRoot(t)
	model.ctxLoading = true
	updated, _ := model.Update(cluster.ContextsMsg{Err: errStub("context list unavailable")})
	root := updated.(RootModel)
	if root.ctxLoading || root.err == nil {
		t.Errorf("context failure: loading=%v err=%v", root.ctxLoading, root.err)
	}
}
