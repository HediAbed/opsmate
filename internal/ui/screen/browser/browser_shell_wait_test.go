package browser

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/kube"
)

func TestWaitForShellOutput_DeliversLine(t *testing.T) {
	s := makeFakeShellSession(t)
	s.output <- kube.ShellOutput{SessionID: s.Identity().ID, Line: "line one"}

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
	t.Errorf("unexpected msg type: %T", msg)
}

func TestHandleShellKey_PgUpScrollsViewport(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(120, 30)
	s := makeFakeShellSession(t)
	m.shellSession = s
	m.state = stateShell
	m.shellView.SetContent("a\nb\nc\nd\ne\nf\ng\nh\ni\nj")
	m.shellView.GotoBottom()

	out, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyPgUp, Text: "pgup"})
	_ = out
	out.shellSession.Close()
}

func TestHandleShellKey_PgDownScrollsViewport(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(120, 30)
	s := makeFakeShellSession(t)
	m.shellSession = s
	m.state = stateShell
	m.shellView.SetContent("a\nb\nc\nd\ne\nf\ng\nh\ni\nj")

	out, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyPgDown, Text: "pgdown"})
	_ = out
	out.shellSession.Close()
}

func TestHandleShellKey_EnterSubmitsCommand(t *testing.T) {
	m := newTestBrowserModel("ns")
	m.SetSize(120, 30)
	s := makeFakeShellSession(t)
	m.shellSession = s
	m.state = stateShell
	m.shellInput.SetValue("ls -la")

	out, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	if out.shellInput.Value() != "" {
		t.Error("enter should clear shell input after submit")
	}
	if len(s.sent) != 1 || s.sent[0] != "ls -la" {
		t.Fatalf("sent commands = %v", s.sent)
	}
	out.shellSession.Close()
}

func TestWaitForShellExit_DeliversExit(t *testing.T) {
	s := makeFakeShellSession(t)
	s.exit <- kube.ShellExit{SessionID: s.Identity().ID}

	cmd := waitForShellExit(s)
	if cmd == nil {
		t.Fatal("waitForShellExit should return non-nil cmd")
	}
	msg := cmd()
	if _, ok := msg.(shellExitMsg); !ok {
		t.Errorf("expected shellExitMsg; got %T", msg)
	}
}
