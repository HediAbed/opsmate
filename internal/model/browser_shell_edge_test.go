//go:build !windows

package model

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/service"
)

func TestOpenShellRejectsDuplicateMissingNamespaceAndStartFailure(t *testing.T) {
	model := NewBrowserModel("team-a")
	model.state = stateShell
	updated, command := model.openShell()
	if command != nil || updated.state != stateShell {
		t.Fatal("opening an active shell changed its state")
	}

	model = NewBrowserModel("")
	model.SetSize(100, 24)
	model.pods = []service.Pod{{Name: "web", Status: "Running"}}
	model.rebuildTable()
	updated, command = model.openShell()
	if command != nil || updated.errBanner != uiErrShellNamespaceRequired {
		t.Fatalf("missing namespace result = command:%v banner:%q", command != nil, updated.errBanner)
	}

	t.Setenv("PATH", t.TempDir())
	model = NewBrowserModel("team-a")
	model.SetSize(100, 24)
	model.pods = []service.Pod{{Name: "web", Namespace: "team-a", Status: "Running"}}
	model.rebuildTable()
	updated, command = model.openShell()
	if command != nil || updated.state == stateShell || !strings.Contains(stripAnsiForTest(updated.errBanner), "shell") {
		t.Fatalf("start failure result = state:%v banner:%q", updated.state, updated.errBanner)
	}
}

func TestShellWaitCommandsReportClosedChannels(t *testing.T) {
	installFakeKubectl(t, "#!/bin/sh\nexit 0\n")
	session, err := service.StartShellSession("team-a", "web", "")
	if err != nil {
		t.Fatalf("start shell: %v", err)
	}
	defer session.Close()

	if message := waitForShellOutput(session)(); message.(shellExitMsg).SessionID != session.Identity().ID {
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

func TestShellKeyReportsInterruptFailure(t *testing.T) {
	installFakeKubectl(t, "#!/bin/sh\nexit 0\n")
	session, err := service.StartShellSession("team-a", "web", "")
	if err != nil {
		t.Fatalf("start shell: %v", err)
	}
	defer session.Close()
	_ = waitForShellExit(session)()

	model := NewBrowserModel("team-a")
	model.state = stateShell
	model.shellSession = session
	updated, command := model.handleShellKey(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if command != nil || !strings.Contains(stripAnsiForTest(updated.errBanner), "interrupt") {
		t.Fatalf("interrupt failure banner = %q", updated.errBanner)
	}
}

func TestSubmitShellCommandHandlesEmptyMissingAndClosedSessions(t *testing.T) {
	model := NewBrowserModel("team-a")
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

	installFakeKubectl(t, "#!/bin/sh\nexit 0\n")
	session, err := service.StartShellSession("team-a", "web", "")
	if err != nil {
		t.Fatalf("start shell: %v", err)
	}
	defer session.Close()
	_ = waitForShellExit(session)()
	model.shellSession = session
	model.shellView = newShellViewport(80, 24)
	model.shellInput.SetValue("pwd")
	updated, _ = model.submitShellCommand()
	if len(updated.shellLines) < 2 || !strings.Contains(stripAnsiForTest(updated.shellLines[len(updated.shellLines)-1]), "error") {
		t.Fatalf("closed-session lines = %v", updated.shellLines)
	}
}

func TestShellHistoryHandlesEmptyAndUnderflow(t *testing.T) {
	model := NewBrowserModel("team-a")
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
	installFakeKubectl(t, "#!/bin/sh\ncat\n")
	session, err := service.StartShellSession("team-a", "web", "")
	if err != nil {
		t.Fatalf("start shell: %v", err)
	}
	model := NewBrowserModel("team-a")
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
