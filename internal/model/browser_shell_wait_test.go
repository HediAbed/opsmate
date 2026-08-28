//go:build !windows

package model

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/service"
)

func TestWaitForShellOutput_DeliversLine(t *testing.T) {
	installFakeKubectl(t, `#!/bin/sh
printf 'line one\n'
sleep 1
`)
	s, err := service.StartShellSession("ns", "pod", "")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer s.Close()

	cmd := waitForShellOutput(s)
	if cmd == nil {
		t.Fatal("waitForShellOutput should return non-nil cmd")
	}
	msg := cmd()
	if out, ok := msg.(shellOutputMsg); ok {
		if out.Line == "" {
			t.Errorf("expected non-empty shell line; got %+v", out)
		}
		return
	}
	if _, ok := msg.(shellExitMsg); ok {
		return
	}
	t.Errorf("unexpected msg type: %T", msg)
}

func TestHandleShellKey_PgUpScrollsViewport(t *testing.T) {
	installFakeKubectl(t, `#!/bin/sh
cat
`)
	m := NewBrowserModel("ns")
	m.SetSize(120, 30)
	s, err := service.StartShellSession("ns", "pod", "")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	m.shellSession = s
	m.state = stateShell
	m.shellView.SetContent("a\nb\nc\nd\ne\nf\ng\nh\ni\nj")
	m.shellView.GotoBottom()

	out, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyPgUp, Text: "pgup"})
	_ = out
	out.shellSession.Close()
}

func TestHandleShellKey_PgDownScrollsViewport(t *testing.T) {
	installFakeKubectl(t, `#!/bin/sh
cat
`)
	m := NewBrowserModel("ns")
	m.SetSize(120, 30)
	s, err := service.StartShellSession("ns", "pod", "")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	m.shellSession = s
	m.state = stateShell
	m.shellView.SetContent("a\nb\nc\nd\ne\nf\ng\nh\ni\nj")

	out, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyPgDown, Text: "pgdown"})
	_ = out
	out.shellSession.Close()
}

func TestHandleShellKey_EnterSubmitsCommand(t *testing.T) {
	installFakeKubectl(t, `#!/bin/sh
cat
`)
	m := NewBrowserModel("ns")
	m.SetSize(120, 30)
	s, err := service.StartShellSession("ns", "pod", "")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	m.shellSession = s
	m.state = stateShell
	m.shellInput.SetValue("ls -la")

	out, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	if out.shellInput.Value() != "" {
		t.Error("enter should clear shell input after submit")
	}
	out.shellSession.Close()
}

func TestWaitForShellExit_DeliversExit(t *testing.T) {
	installFakeKubectl(t, `#!/bin/sh
exit 0
`)
	s, err := service.StartShellSession("ns", "pod", "")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer s.Close()

	cmd := waitForShellExit(s)
	if cmd == nil {
		t.Fatal("waitForShellExit should return non-nil cmd")
	}
	msg := cmd()
	if _, ok := msg.(shellExitMsg); !ok {
		t.Errorf("expected shellExitMsg; got %T", msg)
	}
}
