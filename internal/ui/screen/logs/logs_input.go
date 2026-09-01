package logs

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/HediAbed/opsmate/internal/ui/screen"
)

func (m LogsModel) handleLogKey(msg tea.KeyPressMsg) (LogsModel, tea.Cmd) {
	switch {
	case m.filterInput.Focused():
		return m.handleLogFilterKey(msg)
	case m.showContainerPopup:
		return m.handleContainerPopupKey(msg)
	case m.showPodPopup:
		return m.handlePopupKey(msg)
	case m.inspectMode:
		return m.handleInspectKey(msg)
	default:
		return m.handleGlobalLogKey(msg)
	}
}

func (m LogsModel) handleLogFilterKey(msg tea.KeyPressMsg) (LogsModel, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.filter = m.filterInput.Value()
		m.applyFilter()
		m.logView.SetContent(m.colorizeLines(m.filteredLines))
		if m.autoScroll {
			m.logView.GotoBottom()
		}
		m.filterInput.Blur()
		return m, nil
	case "esc":
		m.filterInput.Blur()
		return m, nil
	default:
		var command tea.Cmd
		m.filterInput, command = m.filterInput.Update(msg)
		return m, command
	}
}

func (m LogsModel) handleGlobalLogKey(msg tea.KeyPressMsg) (LogsModel, tea.Cmd) {
	key := msg.String()
	switch key {
	case "i", "p", "f", "/", "space":
		return m.handleLogModeKey(key)
	case "o", "r":
		return m.handleLogFetchKey(key)
	case "g", "G", "+", "-":
		m.handleLogNavigationKey(key)
	case "c":
		return m.copyVisibleLogs()
	case "C":
		return m.copyAllLogs()
	case "esc", "escape":
		return m, func() tea.Msg { return screen.GoBackMsg{} }
	default:
		return m.forwardLogViewportKey(msg)
	}
	return m, nil
}

func (m LogsModel) handleLogModeKey(key string) (LogsModel, tea.Cmd) {
	switch key {
	case "i":
		m.startLogInspection()
	case "p":
		m.showPodPopup = true
		m.podCursor = m.findPodIndex(m.selectedPod)
	case "f", "/":
		return m, m.filterInput.Focus()
	case "space":
		m.paused = !m.paused
		if !m.paused && m.selectedPod != "" {
			return m, doTick()
		}
	}
	return m, nil
}

func (m LogsModel) handleLogFetchKey(key string) (LogsModel, tea.Cmd) {
	if m.selectedPod == "" {
		return m, nil
	}
	if key == "o" {
		return m, m.fetchContainers()
	}
	m.loading = true
	return m, m.fetchSelectedLogs()
}

func (m *LogsModel) handleLogNavigationKey(key string) {
	switch key {
	case "g":
		m.logView.GotoTop()
		m.autoScroll = false
	case "G":
		m.logView.GotoBottom()
		m.autoScroll = true
	case "+":
		m.tailLines = min(maximumLogTailLines, m.tailLines+logTailLineStep)
	case "-":
		m.tailLines = max(minimumLogTailLines, m.tailLines-logTailLineStep)
	}
}

func (m *LogsModel) startLogInspection() {
	if len(m.filteredLines) == 0 {
		return
	}
	m.resetExplanation()
	m.inspectMode = true
	m.paused = true
	m.lineCursor = min(m.logView.YOffset()+m.logView.Height()/logsCursorCenterDivisor, len(m.filteredLines)-1)
	m.rebuildInspectView()
}

func (m LogsModel) copyVisibleLogs() (LogsModel, tea.Cmd) {
	content := strings.Join(m.filteredLines, "\n")
	status, command := screen.CopyToClipboard(content, fmt.Sprintf("%d lines", len(m.filteredLines)))
	m.statusMsg = status
	return m, command
}

func (m LogsModel) copyAllLogs() (LogsModel, tea.Cmd) {
	content := strings.Join(m.allLines, "\n")
	status, command := screen.CopyToClipboard(content, fmt.Sprintf("%d lines", len(m.allLines)))
	m.statusMsg = status
	return m, command
}

func (m LogsModel) forwardLogViewportKey(msg tea.KeyPressMsg) (LogsModel, tea.Cmd) {
	previousOffset := m.logView.YOffset()
	var command tea.Cmd
	m.logView, command = m.logView.Update(msg)
	if m.logView.YOffset() < previousOffset {
		m.autoScroll = false
	} else if m.logView.AtBottom() {
		m.autoScroll = true
	}
	return m, command
}

func (m LogsModel) acceptsPodListResult(msg logPodsResultMsg) bool {
	return msg.requestID == m.podListRequestID && msg.namespace == m.namespace
}
