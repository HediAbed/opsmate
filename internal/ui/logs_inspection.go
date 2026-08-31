package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/HediAbed/opsmate/internal/ui/theme"
)

func (m LogsModel) handlePopupKey(msg tea.KeyPressMsg) (LogsModel, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc", "p":
		m.showPodPopup = false
		return m, nil
	case "up", "k":
		if m.podCursor > 0 {
			m.podCursor--
		}
	case "down", "j":
		if m.podCursor < len(m.pods)-1 {
			m.podCursor++
		}
	case "enter":
		if m.podCursor < len(m.pods) {
			m.selectPod(m.pods[m.podCursor])
			m.showPodPopup = false
			m.loading = true
			m.allLines = nil
			m.filteredLines = nil
			m.logView.SetContent("")
			return m, m.fetchSelectedLogs()
		}
	}
	return m, nil
}

func (m LogsModel) handlePopupMouse(msg tea.MouseMsg) (LogsModel, tea.Cmd) {
	switch ev := msg.(type) {
	case tea.MouseClickMsg:
		if ev.Button != tea.MouseLeft {
			return m, nil
		}
		return m.handlePopupClick(ev.X, ev.Y)
	case tea.MouseWheelMsg:
		switch ev.Button {
		case tea.MouseWheelUp:
			if m.podCursor > 0 {
				m.podCursor--
			}
		case tea.MouseWheelDown:
			if m.podCursor < len(m.pods)-1 {
				m.podCursor++
			}
		}
	}
	return m, nil
}

func (m LogsModel) handlePopupClick(column, row int) (LogsModel, tea.Cmd) {
	popupWidth := logsPopupWidth(podPopupDesiredWidth, m.width)
	popupHeight := min(len(m.pods)+logsPopupItemChrome, m.height-logsPopupItemChrome)
	popupLeft := (m.width - popupWidth) / pairedSides
	popupTop := (m.height - popupHeight) / pairedSides

	inside := column >= popupLeft && column < popupLeft+popupWidth &&
		row >= popupTop && row < popupTop+popupHeight
	if !inside {
		m.showPodPopup = false
		return m, nil
	}
	rowInPopup := row - popupTop - logsPopupItemTopOffset
	if rowInPopup < 0 || rowInPopup >= len(m.pods) {
		return m, nil
	}
	m.podCursor = rowInPopup
	m.selectPod(m.pods[m.podCursor])
	m.showPodPopup = false
	m.loading = true
	m.allLines = nil
	m.filteredLines = nil
	m.logView.SetContent("")
	return m, m.fetchSelectedLogs()
}

func (m LogsModel) handleContainerPopupKey(msg tea.KeyPressMsg) (LogsModel, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc", "o":
		m.showContainerPopup = false
		return m, nil
	case "up", "k":
		if m.containerCursor > 0 {
			m.containerCursor--
		}
	case "down", "j":
		if m.containerCursor < len(m.containers)-1 {
			m.containerCursor++
		}
	case "enter":
		if m.containerCursor < len(m.containers) {
			m.selectedContainer = m.containers[m.containerCursor]
			m.resetExplanation()
			m.showContainerPopup = false
			m.loading = true
			m.allLines = nil
			m.filteredLines = nil
			m.logView.SetContent("")
			return m, m.fetchSelectedLogs()
		}
	}
	return m, nil
}

func (m LogsModel) handleInspectKey(msg tea.KeyPressMsg) (LogsModel, tea.Cmd) {
	switch msg.String() {
	case "esc", "i":
		m.inspectMode = false
		m.resetExplanation()
		m.logView.SetContent(m.colorizeLines(m.filteredLines))
		return m, nil
	case "up", "k":
		m.moveInspectCursor(-1)
	case "down", "j":
		m.moveInspectCursor(1)
	case "enter":
		return m, m.explainInspectedLine()
	case "n":
		if index, found := m.nextImportantLine(); found {
			m.jumpToInspectLine(index)
		}
	case "N":
		if index, found := m.previousImportantLine(); found {
			m.jumpToInspectLine(index)
		}
	}
	return m, nil
}

func (m *LogsModel) moveInspectCursor(offset int) {
	nextCursor := m.lineCursor + offset
	if nextCursor < 0 || nextCursor >= len(m.filteredLines) {
		return
	}
	m.lineCursor = nextCursor
	m.resetExplanation()
	m.rebuildInspectView()
	if m.lineCursor < m.logView.YOffset() {
		m.logView.SetYOffset(m.lineCursor)
	}
	if m.lineCursor >= m.logView.YOffset()+m.logView.Height() {
		m.logView.SetYOffset(m.lineCursor - m.logView.Height() + 1)
	}
}

func (m *LogsModel) explainInspectedLine() tea.Cmd {
	if m.lineCursor < 0 || m.lineCursor >= len(m.filteredLines) || m.lineExplanationLoading {
		return nil
	}
	line := m.filteredLines[m.lineCursor]
	contextLines := m.getSurroundingContext(m.lineCursor, inspectContextLines)
	m.lineExplanationLoading = true
	m.lineExplanation = ""
	m.lineExplanationErr = nil
	return m.explainSelectedLine(line, contextLines)
}

func (m LogsModel) nextImportantLine() (int, bool) {
	for index := m.lineCursor + 1; index < len(m.filteredLines); index++ {
		if classifyLine(m.filteredLines[index]) >= sevWarn {
			return index, true
		}
	}
	return 0, false
}

func (m LogsModel) previousImportantLine() (int, bool) {
	for index := m.lineCursor - 1; index >= 0; index-- {
		if classifyLine(m.filteredLines[index]) >= sevWarn {
			return index, true
		}
	}
	return 0, false
}

func (m *LogsModel) jumpToInspectLine(index int) {
	m.lineCursor = index
	m.resetExplanation()
	m.rebuildInspectView()
	viewportTop := m.logView.YOffset()
	viewportBottom := viewportTop + m.logView.Height()
	if index < viewportTop || index >= viewportBottom {
		m.logView.SetYOffset(max(0, index-m.logView.Height()/pairedSides))
	}
}

func (m *LogsModel) rebuildInspectView() {
	if len(m.filteredLines) == 0 {
		return
	}
	var b strings.Builder
	b.Grow(len(m.filteredLines) * estimatedLogLineBytes)
	for i, line := range m.filteredLines {
		if i == m.lineCursor {
			b.WriteString(theme.LogInspectCursor.Render("▶ " + line))
		} else {
			b.WriteString(m.renderedLine(line).rendered)
		}
		if i < len(m.filteredLines)-1 {
			b.WriteByte('\n')
		}
	}
	m.logView.SetContent(b.String())
}

func (m LogsModel) getSurroundingContext(index, radius int) string {
	start := index - radius
	if start < 0 {
		start = 0
	}
	end := index + radius + 1
	if end > len(m.filteredLines) {
		end = len(m.filteredLines)
	}
	return strings.Join(m.filteredLines[start:end], "\n")
}
