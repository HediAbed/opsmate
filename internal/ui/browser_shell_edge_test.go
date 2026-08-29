package ui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/kube"
)

func TestOpenShellRejectsDuplicateMissingNamespaceAndStartFailure(t *testing.T) {
	model := newTestBrowserModel("team-a")
	model.state = stateShell
	updated, command := model.openShell()
	if command != nil || updated.state != stateShell {
		t.Fatal("opening an active shell changed its state")
	}

	model = newTestBrowserModel("")
	model.SetSize(100, 24)
	model.pods = []cluster.Pod{{Name: "web", Status: "Running"}}
	model.rebuildTable()
	updated, command = model.openShell()
	if command != nil || updated.errBanner != shellNamespaceRequiredMessage {
		t.Fatalf("missing namespace result = command:%v banner:%q", command != nil, updated.errBanner)
	}

	model = newTestBrowserWithClusterOperations("team-a", &testClusterOperations{shellErr: errors.New("start failed")})
	model.SetSize(100, 24)
	model.pods = []cluster.Pod{{Name: "web", Namespace: "team-a", Status: "Running"}}
	model.rebuildTable()
	updated, command = model.openShell()
	if command != nil || updated.state == stateShell || !strings.Contains(stripAnsiForTest(updated.errBanner), "shell") {
		t.Fatalf("start failure result = state:%v banner:%q", updated.state, updated.errBanner)
	}
}

func TestShellWaitCommandsReportClosedChannels(t *testing.T) {
	session := makeFakeShellSession(t)
	close(session.output)
	session.exit <- kube.ShellExit{SessionID: session.Identity().ID}
	close(session.exit)

	if message := waitForShellOutput(session)(); message.(shellOutputClosedMsg).SessionID != session.Identity().ID {
		t.Fatalf("closed output message = %#v", message)
	}
	firstExit := waitForShellExit(session)()
	if _, ok := firstExit.(shellExitMsg); !ok {
		t.Fatalf("first exit message = %T", firstExit)
	}
	secondExit, ok := waitForShellExit(session)().(shellExitMsg)
	if !ok || secondExit.SessionID != session.Identity().ID {
		t.Fatalf("closed exit message = %#v", secondExit)
	}
}

func TestBrowserIgnoresClosedShellOutputChannel(t *testing.T) {
	model := newTestBrowserModel("team-a")
	model.state = stateShell
	updated, command := model.Update(shellOutputClosedMsg{SessionID: "shell-1"})
	if command != nil || updated.state != stateShell {
		t.Fatalf("closed output channel changed browser state: command=%t state=%v", command != nil, updated.state)
	}
}

func TestShellKeyReportsInterruptFailure(t *testing.T) {
	session := makeFakeShellSession(t)
	session.interruptErr = errors.New("interrupt failed")

	model := newTestBrowserModel("team-a")
	model.state = stateShell
	model.shellSession = session
	updated, command := model.handleShellKey(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if command != nil || !strings.Contains(stripAnsiForTest(updated.errBanner), "interrupt") {
		t.Fatalf("interrupt failure banner = %q", updated.errBanner)
	}
}

func TestSubmitShellCommandHandlesEmptyMissingAndClosedSessions(t *testing.T) {
	model := newTestBrowserModel("team-a")
	model.shellInput = newShellInput(80)
	model.shellInput.SetValue("")
	updated, command := model.submitShellCommand()
	if command != nil || len(updated.shellHistory) != 0 {
		t.Fatal("empty command changed shell history")
	}

	model.shellInput.SetValue("pwd")
	updated, command = model.submitShellCommand()
	if command != nil || len(updated.shellHistory) != 0 {
		t.Fatal("command without a session changed shell history")
	}

	session := makeFakeShellSession(t)
	session.sendErr = kube.ErrShellSessionClosed
	model.shellSession = session
	model.shellView = newShellViewport(80, 24)
	model.shellInput.SetValue("pwd")
	updated, _ = model.submitShellCommand()
	if len(updated.shellLines) < 2 || !strings.Contains(stripAnsiForTest(updated.shellLines[len(updated.shellLines)-1]), "error") {
		t.Fatalf("closed-session lines = %v", updated.shellLines)
	}
}

func TestShellHistoryHandlesEmptyAndUnderflow(t *testing.T) {
	model := newTestBrowserModel("team-a")
	model.shellInput = newShellInput(80)
	model.shellInput.SetValue("draft")
	if got := model.shellHistoryStep(1); got != "draft" {
		t.Fatalf("empty history value = %q", got)
	}

	model.shellHistory = []string{"first"}
	model.shellHistIdx = -1
	if got := model.shellHistoryStep(-1); got != "" || model.shellHistIdx != -1 {
		t.Fatalf("history underflow = value:%q index:%d", got, model.shellHistIdx)
	}
}

func TestHandleShellExitIncludesProcessFailure(t *testing.T) {
	session := makeFakeShellSession(t)
	model := newTestBrowserModel("team-a")
	model.SetSize(100, 24)
	model.state = stateShell
	model.shellSession = session
	model.shellView = newShellViewport(100, 24)
	sentinel := errors.New("process failed")

	updated, command := model.handleShellExit(shellExitMsg{SessionID: session.Identity().ID, Err: sentinel})
	if command != nil || updated.state != stateBrowsing {
		t.Fatalf("exit result = state:%v command:%v", updated.state, command != nil)
	}
	if !strings.Contains(stripAnsiForTest(updated.errBanner), "process failed") {
		t.Fatalf("exit failure banner = %q", updated.errBanner)
	}
}
