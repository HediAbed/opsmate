//go:build !windows

package model

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/service"
)

func TestOpenShell_NonPodResourceShowsWarning(t *testing.T) {
	m := NewBrowserModel("default")
	m.SetSize(200, 40)
	m.resourceType = "deployments"

	m2, _ := m.openShell()
	if m2.state == stateShell {
		t.Error("openShell on non-pod resource must NOT enter stateShell")
	}
	if m2.statusMsg == "" {
		t.Error("openShell on non-pod must surface a warning")
	}
}

func TestOpenShell_RejectsCompletedPods(t *testing.T) {
	m := NewBrowserModel("default")
	m.SetSize(200, 40)
	m.resourceType = "pods"
	m.pods = []service.Pod{{Name: "completed", Namespace: "default", Status: "Succeeded"}}
	m.rebuildTable()

	m2, _ := m.openShell()
	if m2.state == stateShell {
		t.Error("openShell into a completed pod must not start a session")
	}
	if m2.errBanner == "" {
		t.Error("openShell into completed pod must show an error banner")
	}
}

func TestCloseShell_ResetsState(t *testing.T) {
	installShellKubectl(t)

	session, err := service.StartShellSession("ns", "pod", "")
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	m := NewBrowserModel("default")
	m.state = stateShell
	m.shellSession = session
	m.shellPod = "pod"
	m.shellNS = "ns"
	m.shellLines = []string{"a", "b"}
	m.shellHistory = []string{"ls"}
	m.shellHistIdx = 0

	m2, _ := m.closeShell()
	if m2.state != stateBrowsing {
		t.Errorf("closeShell must restore stateBrowsing, got %v", m2.state)
	}
	if m2.shellSession != nil {
		t.Error("closeShell must clear shellSession")
	}
	if m2.shellPod != "" || m2.shellNS != "" {
		t.Error("closeShell must clear pod / namespace")
	}
	if m2.shellHistIdx != -1 || len(m2.shellHistory) != 0 || len(m2.shellLines) != 0 {
		t.Error("closeShell must clear history + lines")
	}
}

func TestShellHistoryStep_NavigatesAndClamps(t *testing.T) {
	m := NewBrowserModel("default")
	m.shellHistory = []string{"first", "second", "third"}
	m.shellHistIdx = -1

	if got := m.shellHistoryStep(1); got != "third" {
		t.Errorf("up once should return last entry; got %q", got)
	}
	if m.shellHistIdx != 0 {
		t.Errorf("after one up, idx=0; got %d", m.shellHistIdx)
	}
	m.shellHistIdx = 0
	if got := m.shellHistoryStep(1); got != "second" {
		t.Errorf("up twice should walk back; got %q", got)
	}
	m.shellHistIdx = 2
	if got := m.shellHistoryStep(1); got != "first" {
		t.Errorf("idx must clamp to len-1; got %q", got)
	}
	m.shellHistIdx = 0
	if got := m.shellHistoryStep(-1); got != "" {
		t.Errorf("down past newest must return empty; got %q", got)
	}
}

func TestAppendBoundedHistory_RespectsCap(t *testing.T) {
	got := appendBoundedHistory(nil, "a", 2)
	got = appendBoundedHistory(got, "b", 2)
	got = appendBoundedHistory(got, "c", 2)
	if len(got) != 2 || got[0] != "b" || got[1] != "c" {
		t.Errorf("expected [b c], got %v", got)
	}
}

func TestHandleShellOutput_AppendsLine(t *testing.T) {
	m := NewBrowserModel("default")
	m.SetSize(200, 40)
	m.state = stateShell
	m.shellSession = makeFakeShellSession(t)
	m.shellLines = []string{"$ ls"}

	m2, _ := m.handleShellOutput(shellOutputMsg{SessionID: m.shellSession.Identity().ID, Line: "etc"})
	if got := len(m2.shellLines); got != 2 {
		t.Errorf("output should append a line; got %d", got)
	}
	if !strings.Contains(m2.shellLines[1], "etc") {
		t.Error("appended line should contain the output text")
	}
}

func TestHandleShellOutput_StyledStderr(t *testing.T) {
	m := NewBrowserModel("default")
	m.SetSize(200, 40)
	m.state = stateShell
	m.shellSession = makeFakeShellSession(t)

	m2, _ := m.handleShellOutput(shellOutputMsg{SessionID: m.shellSession.Identity().ID, Line: "boom", Stderr: true})
	if !strings.Contains(m2.shellLines[0], "boom") {
		t.Errorf("stderr line should be appended; got %q", m2.shellLines[0])
	}
}

func TestSubmitShellCommand_PushesHistoryAndEchoes(t *testing.T) {
	installShellKubectl(t)

	m := NewBrowserModel("default")
	m.SetSize(200, 40)
	m.resourceType = "pods"
	m.pods = []service.Pod{{Name: "running", Namespace: "default", Status: "Running"}}
	m.rebuildTable()

	m, _ = m.openShell()
	if m.state != stateShell {
		t.Fatalf("expected stateShell, got %v", m.state)
	}
	defer m.shellSession.Close()

	m.shellInput.SetValue("ls /etc")
	m, _ = m.submitShellCommand()
	if got := len(m.shellHistory); got != 1 || m.shellHistory[0] != "ls /etc" {
		t.Errorf("history should record the command; got %v", m.shellHistory)
	}
	echoed := false
	for _, line := range m.shellLines {
		if strings.Contains(line, "ls /etc") {
			echoed = true
			break
		}
	}
	if !echoed {
		t.Error("submitted command should be echoed locally to the viewport")
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		select {
		case <-m.shellSession.Output():
		default:
		}
	}
}

func makeFakeShellSession(t *testing.T) *service.ShellSession {
	t.Helper()
	installShellKubectl(t)

	session, err := service.StartShellSession("ns", "pod", "")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(session.Close)
	return session
}

func installShellKubectl(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "kubectl")
	writeTestExecutable(t, path, "#!/bin/sh\ncat\n")
	t.Setenv("PATH", tmp+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func keyPress(s string) tea.KeyPressMsg {
	if len(s) != 1 {
		return tea.KeyPressMsg{Text: s}
	}
	return tea.KeyPressMsg{Code: rune(s[0]), Text: s}
}

func TestHandleShellKey_EscClosesSession(t *testing.T) {
	m := NewBrowserModel("default")
	m.SetSize(200, 40)
	m.state = stateShell
	m.shellSession = makeFakeShellSession(t)

	m2, _ := m.handleShellKey(tea.KeyPressMsg{Text: "esc"})
	if m2.state != stateBrowsing {
		t.Errorf("esc must restore stateBrowsing; got %v", m2.state)
	}
}

func TestHandleShellKey_CtrlXClosesSession(t *testing.T) {
	m := NewBrowserModel("default")
	m.SetSize(200, 40)
	m.state = stateShell
	m.shellSession = makeFakeShellSession(t)

	m2, _ := m.handleShellKey(tea.KeyPressMsg{Text: "ctrl+x"})
	if m2.state != stateBrowsing {
		t.Errorf("ctrl+x must restore stateBrowsing; got %v", m2.state)
	}
}

func TestHandleShellKey_UpDownWalkHistory(t *testing.T) {
	m := NewBrowserModel("default")
	m.SetSize(200, 40)
	m.state = stateShell
	m.shellSession = makeFakeShellSession(t)
	m.shellInput = newShellInput(200)
	m.shellHistory = []string{"first", "second", "third"}
	m.shellHistIdx = -1

	m, _ = m.handleShellKey(tea.KeyPressMsg{Text: "up"})
	if got := m.shellInput.Value(); got != "third" {
		t.Errorf("up must restore newest history; got %q", got)
	}
	m, _ = m.handleShellKey(tea.KeyPressMsg{Text: "down"})
	if got := m.shellInput.Value(); got != "" {
		t.Errorf("down past newest must clear input; got %q", got)
	}
}

func TestShellState_ClaimsInputFocus(t *testing.T) {
	m := NewBrowserModel("default")
	m.state = stateShell
	if !m.HasInputFocus() {
		t.Error("stateShell must claim input focus so root.go does not intercept letter keys")
	}
}

func TestRootCtrlCInterruptsFocusedShellWithoutQuitting(t *testing.T) {
	installShellKubectl(t)
	session, err := service.StartShellSession("ns", "pod", "")
	if err != nil {
		t.Fatalf("start shell: %v", err)
	}
	t.Cleanup(session.Close)

	m := NewRootModel("ns")
	m.screen = ScreenBrowser
	m.browser.state = stateShell
	m.browser.shellSession = session

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd != nil {
		t.Fatal("focused shell Ctrl+C returned an application command")
	}
	root := updated.(RootModel)
	if root.screen != ScreenBrowser || root.browser.state != stateShell {
		t.Fatalf("Ctrl+C left the shell screen: screen=%d state=%d", root.screen, root.browser.state)
	}

	select {
	case <-session.Exit():
	case <-time.After(2 * time.Second):
		t.Fatal("shell process did not receive the interrupt")
	}
}

func TestOpenShell_FocusesTheInput(t *testing.T) {
	installShellKubectl(t)

	m := NewBrowserModel("default")
	m.SetSize(200, 40)
	m.resourceType = "pods"
	m.pods = []service.Pod{{Name: "running", Namespace: "default", Status: "Running"}}
	m.rebuildTable()

	m, _ = m.openShell()
	if !m.shellInput.Focused() {
		t.Error("openShell must focus the input — without focus, textinput drops keystrokes")
	}
	t.Cleanup(func() { m.shellSession.Close() })
}

func TestHandleShellKey_UnknownForwardsToInput(t *testing.T) {
	m := NewBrowserModel("default")
	m.SetSize(200, 40)
	m.state = stateShell
	m.shellSession = makeFakeShellSession(t)
	m.shellInput = newShellInput(200)
	m.shellInput.Focus()
	m.shellInput.SetValue("ls /e")

	m, _ = m.handleShellKey(keyPress("t"))
	m, _ = m.handleShellKey(keyPress("c"))
	if got := m.shellInput.Value(); got != "ls /etc" {
		t.Errorf("unknown keypresses must extend the input value; got %q", got)
	}
	if m.state != stateShell {
		t.Error("plain keypresses must not change state")
	}
}

func TestRefreshShellViewport_IncludesLiveInputAsLastLine(t *testing.T) {
	installShellKubectl(t)

	m := NewBrowserModel("default")
	m.SetSize(200, 40)
	m.resourceType = "pods"
	m.pods = []service.Pod{{Name: "running", Namespace: "default", Status: "Running"}}
	m.rebuildTable()
	m, _ = m.openShell()
	t.Cleanup(func() { m.shellSession.Close() })

	m.shellLines = []string{"hostname", "/etc"}
	m.shellInput.SetValue("ls /var")
	m.refreshShellViewport()

	content := m.shellView.View()
	if !strings.Contains(content, "ls /var") {
		t.Error("viewport content must include the live input value as the last line")
	}
	if !strings.Contains(content, "hostname") {
		t.Error("viewport content must include prior history lines above the input")
	}
}

func TestHandleShellKey_TypingRefreshesViewportInline(t *testing.T) {
	installShellKubectl(t)

	m := NewBrowserModel("default")
	m.SetSize(200, 40)
	m.resourceType = "pods"
	m.pods = []service.Pod{{Name: "running", Namespace: "default", Status: "Running"}}
	m.rebuildTable()
	m, _ = m.openShell()
	t.Cleanup(func() { m.shellSession.Close() })

	m, _ = m.handleShellKey(keyPress("x"))
	m, _ = m.handleShellKey(keyPress("y"))
	m, _ = m.handleShellKey(keyPress("z"))
	m, _ = m.handleShellKey(keyPress("z"))
	m, _ = m.handleShellKey(keyPress("y"))
	if !strings.Contains(m.shellView.View(), "xyzzy") {
		t.Error("typing must refresh the viewport so the inline prompt shows the new value")
	}
}

func TestHandleShellKey_HistoryStepRefreshesViewportInline(t *testing.T) {
	installShellKubectl(t)

	m := NewBrowserModel("default")
	m.SetSize(200, 40)
	m.resourceType = "pods"
	m.pods = []service.Pod{{Name: "running", Namespace: "default", Status: "Running"}}
	m.rebuildTable()
	m, _ = m.openShell()
	t.Cleanup(func() { m.shellSession.Close() })

	m.shellHistory = []string{"echo hi"}
	m.shellHistIdx = -1

	m, _ = m.handleShellKey(tea.KeyPressMsg{Text: "up"})
	if !strings.Contains(m.shellView.View(), "echo hi") {
		t.Error("history navigation must refresh the viewport so the recalled command shows inline")
	}
}

func TestAppendShellLine_PreservesUserScrollPosition(t *testing.T) {
	m := NewBrowserModel("default")
	m.SetSize(200, 40)
	m.shellView = newShellViewport(200, 40)
	m.shellLines = make([]string, 100)
	for i := range m.shellLines {
		m.shellLines[i] = "line"
	}
	m.shellView.SetContent(strings.Join(m.shellLines, "\n"))
	m.shellView.SetYOffset(0)

	m.appendShellLine("new line")
	if m.shellView.AtBottom() {
		t.Error("scrolled-up viewport must NOT auto-scroll on new output")
	}

	m.shellView.GotoBottom()
	m.appendShellLine("another line")
	if !m.shellView.AtBottom() {
		t.Error("at-bottom viewport must follow new output to bottom")
	}
}

func TestHandleShellExit_ClosesSession(t *testing.T) {
	m := NewBrowserModel("default")
	m.SetSize(200, 40)
	m.state = stateShell
	m.shellSession = makeFakeShellSession(t)

	m2, _ := m.handleShellExit(shellExitMsg{SessionID: m.shellSession.Identity().ID})
	if m2.state != stateBrowsing {
		t.Errorf("shell exit must restore stateBrowsing; got %v", m2.state)
	}
	if m2.shellSession != nil {
		t.Error("shell exit must clear session")
	}
}

func TestHandleShellMessages_IgnoreStaleSession(t *testing.T) {
	m := NewBrowserModel("default")
	m.SetSize(200, 40)
	m.state = stateShell
	m.shellSession = makeFakeShellSession(t)
	m.shellLines = []string{"connected"}

	withOutput, outputCmd := m.handleShellOutput(shellOutputMsg{SessionID: "expired", Line: "stale"})
	if outputCmd != nil {
		t.Fatal("stale output must not schedule another read")
	}
	if len(withOutput.shellLines) != 1 || withOutput.shellLines[0] != "connected" {
		t.Fatalf("stale output changed shell history: %v", withOutput.shellLines)
	}

	withExit, exitCmd := m.handleShellExit(shellExitMsg{SessionID: "expired"})
	if exitCmd != nil {
		t.Fatal("stale exit must not schedule work")
	}
	if withExit.state != stateShell || withExit.shellSession == nil {
		t.Fatal("stale exit closed the active shell session")
	}
}
