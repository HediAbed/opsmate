package model

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/HediAbed/opsmate/internal/service"
	"github.com/HediAbed/opsmate/internal/theme"
)

const (
	shellPromptText      = "$ "
	shellMaxHistoryLines = 2000

	shellSplitLeftPercent          = 50
	shellSplitMinPaneWidth         = 30
	shellHeaderRows                = 1
	shellBorderRows                = 2
	shellMinViewportHeight         = 3
	shellMinInputWidth             = 10
	shellInputCharLimit            = 2048
	shellInputWidthChromeCols      = 12
	shellUnavailableStatusDuration = 3 * time.Second
	shellInitialMinimumWidth       = 20
	shellViewportWidthChrome       = 6
	shellViewportHeightDivisor     = 2
	shellViewportHeightChrome      = 4
	shellPageScrollDivisor         = 2
	shellPaneChromeWidth           = 4
)

type shellOutputMsg struct {
	SessionID string
	Line      string
	Stderr    bool
}

type shellExitMsg struct {
	SessionID string
	Err       error
}

func (m BrowserModel) openShell() (BrowserModel, tea.Cmd) {
	if m.state == stateShell {
		return m, nil
	}
	if m.resourceType != "pods" {
		m.statusMsg = theme.Warning.Render("Shell is only available for pods")
		return m, clearStatusAfter(shellUnavailableStatusDuration)
	}
	identity, ok := m.selectedIdentity()
	if !ok {
		return m, nil
	}
	if identity.Namespace == "" {
		m.errBanner = uiErrShellNamespaceRequired
		return m, nil
	}
	if status, found := m.podStatusFor(identity); found && !podSupportsExec(status) {
		m.errBanner = shellPodPhaseErr(identity.Name, status)
		return m, nil
	}

	session, err := service.StartShellSession(identity.Namespace, identity.Name, "")
	if err != nil {
		m.errBanner = kubectlActionErr("shell", err)
		return m, nil
	}

	input := newShellInput(m.width)
	input.Focus()
	view := newShellViewport(m.width, m.height)
	greeting := fmt.Sprintf("Connected to %s/%s — type a command, ↑/↓ for history, ctrl+x to close", identity.Namespace, identity.Name)
	lines := []string{theme.Dim.Render(greeting)}
	view.SetContent(strings.Join(lines, "\n"))

	m.shellSession = session
	m.shellPod = identity.Name
	m.shellNS = identity.Namespace
	m.shellInput = input
	m.shellView = view
	m.shellLines = lines
	m.shellHistory = nil
	m.shellHistIdx = -1
	m.state = stateShell
	m.syncBrowserLayout()

	return m, tea.Batch(textinput.Blink, waitForShellOutput(session), waitForShellExit(session))
}

func newShellInput(width int) textinput.Model {
	prompt := theme.AIPrompt.Render(shellPromptText)
	return newTextInput(textInputOpts{
		Prompt:      prompt,
		Placeholder: "ls /etc",
		CharLimit:   shellInputCharLimit,
		Width:       max(shellMinInputWidth, width-shellInputWidthChromeCols),
		PromptStyle: theme.AIPrompt,
		TextStyle:   lipgloss.NewStyle().Foreground(theme.LightText),
	})
}

func newShellViewport(width, height int) viewport.Model {
	w := max(shellInitialMinimumWidth, width-shellViewportWidthChrome)
	h := max(shellMinViewportHeight, height/shellViewportHeightDivisor-shellViewportHeightChrome)
	return newViewport(w, h)
}

func waitForShellOutput(s *service.ShellSession) tea.Cmd {
	sessionID := s.Identity().ID
	return func() tea.Msg {
		out, ok := <-s.Output()
		if !ok {
			return shellExitMsg{SessionID: sessionID}
		}
		return shellOutputMsg{SessionID: out.SessionID, Line: out.Line, Stderr: out.Stderr}
	}
}

func waitForShellExit(s *service.ShellSession) tea.Cmd {
	sessionID := s.Identity().ID
	return func() tea.Msg {
		exit, ok := <-s.Exit()
		if !ok {
			return shellExitMsg{SessionID: sessionID}
		}
		return shellExitMsg{SessionID: exit.SessionID, Err: exit.Err}
	}
}

func (m BrowserModel) handleShellKey(msg tea.KeyPressMsg) (BrowserModel, tea.Cmd) {
	switch msg.String() {
	case "ctrl+x", "esc":
		return m.closeShell()
	case "ctrl+c":
		if m.shellSession != nil {
			if err := m.shellSession.Interrupt(); err != nil {
				m.errBanner = kubectlActionErr("shell interrupt", err)
			}
		}
		return m, nil
	case "enter":
		return m.submitShellCommand()
	case "up":
		m.shellInput.SetValue(m.shellHistoryStep(1))
		m.refreshShellViewport()
		return m, nil
	case "down":
		m.shellInput.SetValue(m.shellHistoryStep(-1))
		m.refreshShellViewport()
		return m, nil
	case "pgup":
		m.shellView.ScrollUp(m.shellView.Height() / shellPageScrollDivisor)
		return m, nil
	case "pgdown":
		m.shellView.ScrollDown(m.shellView.Height() / shellPageScrollDivisor)
		return m, nil
	}
	var cmd tea.Cmd
	m.shellInput, cmd = m.shellInput.Update(msg)
	m.refreshShellViewport()
	return m, cmd
}

func (m BrowserModel) submitShellCommand() (BrowserModel, tea.Cmd) {
	cmd := strings.TrimRight(m.shellInput.Value(), "\n")
	m.shellInput.SetValue("")
	if cmd == "" {
		return m, nil
	}
	if m.shellSession == nil {
		return m, nil
	}
	m.shellHistory = appendBoundedHistory(m.shellHistory, cmd, shellMaxHistoryLines)
	m.shellHistIdx = -1
	m.appendShellLine(theme.HelpKey.Render(shellPromptText) + cmd)
	if err := m.shellSession.Send(cmd); err != nil {
		m.appendShellLine(theme.Error.Render("error: " + sanitizeTerminalLine(err.Error())))
	}
	return m, nil
}

func (m *BrowserModel) shellHistoryStep(delta int) string {
	if len(m.shellHistory) == 0 {
		return m.shellInput.Value()
	}
	idx := m.shellHistIdx + delta
	if idx < -1 {
		idx = -1
	}
	if idx >= len(m.shellHistory) {
		idx = len(m.shellHistory) - 1
	}
	m.shellHistIdx = idx
	if idx == -1 {
		return ""
	}
	return m.shellHistory[len(m.shellHistory)-1-idx]
}

func (m *BrowserModel) appendShellLine(line string) {
	wasAtBottom := m.shellView.AtBottom()
	m.shellLines = appendBoundedHistory(m.shellLines, line, shellMaxHistoryLines)
	m.refreshShellViewport()
	if wasAtBottom {
		m.shellView.GotoBottom()
	}
}

func (m *BrowserModel) refreshShellViewport() {
	if m.shellSession == nil {
		return
	}
	parts := make([]string, 0, len(m.shellLines)+1)
	parts = append(parts, m.shellLines...)
	parts = append(parts, m.shellInput.View())
	m.shellView.SetContent(strings.Join(parts, "\n"))
}

func appendBoundedHistory(history []string, item string, limit int) []string {
	history = append(history, item)
	if len(history) > limit {
		history = history[len(history)-limit:]
	}
	return history
}

func (m BrowserModel) handleShellOutput(msg shellOutputMsg) (BrowserModel, tea.Cmd) {
	if m.state != stateShell || !m.acceptsShellMessage(msg.SessionID) {
		return m, nil
	}
	rendered := sanitizeTerminalText(msg.Line)
	if msg.Stderr {
		rendered = theme.Error.Render(rendered)
	}
	m.appendShellLine(rendered)
	return m, waitForShellOutput(m.shellSession)
}

func (m BrowserModel) handleShellExit(msg shellExitMsg) (BrowserModel, tea.Cmd) {
	if !m.acceptsShellMessage(msg.SessionID) {
		return m, nil
	}
	if msg.Err != nil {
		m.errBanner = kubectlActionErr("shell", msg.Err)
	} else {
		m.statusMsg = theme.Dim.Render("shell session ended")
	}
	return m.closeShell()
}

func (m BrowserModel) acceptsShellMessage(sessionID string) bool {
	if m.shellSession == nil {
		return false
	}
	return m.shellSession.Identity().ID == sessionID
}

func (m BrowserModel) closeShell() (BrowserModel, tea.Cmd) {
	if m.shellSession != nil {
		m.shellSession.Close()
		m.shellSession = nil
	}
	m.shellPod = ""
	m.shellNS = ""
	m.shellHistory = nil
	m.shellHistIdx = -1
	m.shellLines = nil
	m.state = stateBrowsing
	return m, nil
}

func (m BrowserModel) renderShellSplit(height int) string {
	leftW, rightW := shellPaneWidths(m.width)

	tableView := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.BorderColor).
		Padding(0, 1).
		Width(leftW).
		Height(height).
		Render(m.resourceTable.View())

	header := theme.AIPrompt.Render(" SHELL ") + "  " +
		theme.Dim.Render(m.shellNS+"/"+m.shellPod) + "   " +
		theme.HelpKey.Render("[ctrl+c]") + theme.HelpDesc.Render(" interrupt ") +
		theme.HelpKey.Render("[ctrl+x]") + theme.HelpDesc.Render(" close")

	body := lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		m.shellView.View(),
	)

	shellView := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.NeonCyan).
		Padding(0, 1).
		Width(rightW).
		Height(height).
		Render(body)

	return lipgloss.JoinHorizontal(lipgloss.Top, tableView, shellView)
}

func (m *BrowserModel) syncShellLayout(height int) {
	leftWidth, rightWidth := shellPaneWidths(m.width)
	tableWidth := max(1, theme.BoxContentWidth(leftWidth))
	if specs, ok := selectColSpecs(m.resourceType, m.wide); ok {
		m.resourceTable.SetColumns(computeColumns(tableWidth, specs))
	}
	m.resourceTable.SetHeight(max(shellMinViewportHeight, height-shellBorderRows))
	m.resourceTable.SetWidth(tableWidth)

	shellWidth := max(1, theme.BoxContentWidth(rightWidth))
	inputWidth := min(shellWidth, max(shellMinInputWidth, shellWidth-len(shellPromptText)))
	m.shellInput.SetWidth(max(1, inputWidth))
	m.shellView.SetWidth(shellWidth)
	m.shellView.SetHeight(max(shellMinViewportHeight, height-(shellHeaderRows+shellBorderRows)))
	m.refreshShellViewport()
}

func shellPaneWidths(width int) (left, right int) {
	availableWidth := max(pairedSides, width-shellPaneChromeWidth)
	left = max(1, availableWidth*shellSplitLeftPercent/percentageScale)
	if availableWidth >= shellSplitMinPaneWidth*2 {
		left = max(shellSplitMinPaneWidth, left)
	}
	left = min(left, availableWidth-1)
	return left, availableWidth - left
}
