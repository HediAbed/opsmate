package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/HediAbed/opsmate/internal/kube"
	"github.com/HediAbed/opsmate/internal/ui/component"
	"github.com/HediAbed/opsmate/internal/ui/theme"
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

type shellOutputClosedMsg struct {
	SessionID string
}

func (m BrowserModel) openShell() (BrowserModel, tea.Cmd) {
	if m.state == stateShell {
		return m, nil
	}
	if m.resourceType != resourceTypePods {
		m.statusMsg = theme.Warning.Render("Shell is only available for pods")
		return m, clearStatusAfter(shellUnavailableStatusDuration)
	}
	identity, ok := m.selectedIdentity()
	if !ok {
		return m, nil
	}
	if identity.Namespace == "" {
		m.errBanner = shellNamespaceRequiredMessage
		return m, nil
	}
	if status, found := m.podStatusFor(identity); found && !podSupportsExec(status) {
		m.errBanner = shellPodPhaseErrorText(identity.Name, status)
		return m, nil
	}

	session, err := m.operations.StartShell(kube.ShellRequest{
		Pod: kube.PodReference{Namespace: identity.Namespace, Name: identity.Name},
	})
	if err != nil {
		m.errBanner = operationErrorText("shell", err)
		return m, nil
	}

	input := newShellInput(m.width)
	input.Focus()
	view := newShellViewport(m.width, m.height)
	greeting := fmt.Sprintf("Connected to %s/%s. Type a command, ↑/↓ for history, ctrl+x to close", identity.Namespace, identity.Name)
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
	prompt := theme.AnalysisAccent.Render(shellPromptText)
	return newTextInput(textInputOptions{
		Prompt:      prompt,
		Placeholder: "ls /etc",
		CharLimit:   shellInputCharLimit,
		Width:       max(shellMinInputWidth, width-shellInputWidthChromeCols),
		PromptStyle: theme.AnalysisAccent,
		TextStyle:   lipgloss.NewStyle().Foreground(theme.LightText),
	})
}

func newShellViewport(width, height int) viewport.Model {
	viewportWidth := max(shellInitialMinimumWidth, width-shellViewportWidthChrome)
	viewportHeight := max(shellMinViewportHeight, height/shellViewportHeightDivisor-shellViewportHeightChrome)
	return newViewport(viewportWidth, viewportHeight)
}

func waitForShellOutput(session kube.ShellSession) tea.Cmd {
	sessionID := session.Identity().ID
	return func() tea.Msg {
		output, open := <-session.Output()
		if !open {
			return shellOutputClosedMsg{SessionID: sessionID}
		}
		return shellOutputMsg{SessionID: output.SessionID, Line: output.Line, Stderr: output.Stderr}
	}
}

func waitForShellExit(session kube.ShellSession) tea.Cmd {
	sessionID := session.Identity().ID
	return func() tea.Msg {
		exit, open := <-session.Exit()
		if !open {
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
				m.errBanner = operationErrorText("shell interrupt", err)
				return m, nil
			}
		}
		m.statusMsg = theme.Dim.Render("shell session interrupted")
		return m.closeShell()
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
		m.errBanner = operationErrorText("shell", msg.Err)
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

	tableView := component.NewPanel(theme.BoxStyle).Render(
		component.Size{Width: leftW, Height: height},
		m.resourceTable.View(),
	)

	header := theme.AnalysisAccent.Render(" SHELL ") + "  " +
		theme.Dim.Render(m.shellNS+"/"+m.shellPod) + "   " +
		theme.HelpKey.Render("[ctrl+c]") + theme.HelpDesc.Render(" end ") +
		theme.HelpKey.Render("[ctrl+x]") + theme.HelpDesc.Render(" close")

	body := lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		m.shellView.View(),
	)

	shellView := component.NewPanel(theme.BoxStyle.BorderForeground(theme.NeonCyan)).Render(
		component.Size{Width: rightW, Height: height},
		body,
	)

	return lipgloss.JoinHorizontal(lipgloss.Top, tableView, shellView)
}

func (m *BrowserModel) syncShellLayout(height int) {
	leftWidth, rightWidth := shellPaneWidths(m.width)
	pane := component.NewPanel(theme.BoxStyle)
	tableWidth := max(1, pane.ContentSize(component.Size{Width: leftWidth, Height: height}).Width)
	if specs, ok := selectColSpecs(m.resourceType, m.wide); ok {
		m.resourceTable.SetColumns(computeColumns(tableWidth, specs))
	}
	m.resourceTable.SetHeight(max(shellMinViewportHeight, height-shellBorderRows))
	m.resourceTable.SetWidth(tableWidth)

	shellWidth := max(1, pane.ContentSize(component.Size{Width: rightWidth, Height: height}).Width)
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
