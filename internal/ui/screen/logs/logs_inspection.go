package logs

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
		m.movePodPopupCursor(ev.Button)
	}
	return m, nil
}

func (m *LogsModel) movePodPopupCursor(button tea.MouseButton) {
	if button == tea.MouseWheelUp && m.podCursor > 0 {
		m.podCursor--
		return
	}
	if button == tea.MouseWheelDown && m.podCursor < len(m.pods)-1 {
		m.podCursor++
	}
}

func (m LogsModel) handlePopupClick(column, row int) (LogsModel, tea.Cmd) {
	popupWidth := logsPopupWidth(podPopupDesiredWidth, m.width)
	popupHeight := min(len(m.pods)+logsPopupItemChrome, m.height-logsPopupItemChrome)
	popupLeft := (m.width - popupWidth) / centerDivisor
	popupTop := (m.height - popupHeight) / centerDivisor

	inside := column >= popupLeft && column < popupLeft+popupWidth &&
		row >= popupTop && row < popupTop+popupHeight
	if !inside {
		m.showPodPopup = false
		return m, nil
	}
	rowInPopup := row - popupTop - logsPopupItemTopOffset
	visibleStart, visibleEnd := m.visiblePodRange(m.height)
	podIndex := visibleStart + rowInPopup
	if rowInPopup < 0 || podIndex >= visibleEnd {
		return m, nil
	}
	m.podCursor = podIndex
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
		m.selectingWithMouse = false
		m.selectionAnchor = noLineSelected
		m.resetExplanation()
		m.syncLogContent()
		return m, nil
	case "up", "k":
		m.moveInspectCursor(-1)
	case "down", "j":
		m.moveInspectCursor(1)
	case "enter":
		return m, m.explainInspectedLine()
	case "c":
		return m.copySelectedLine()
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
	m.selectionAnchor = nextCursor
	m.resetExplanation()
	m.rebuildInspectView()
	m.revealInspectCursor()
}

func (m *LogsModel) revealInspectCursor() {
	index := m.logRowIndex()
	cursorTopRow := index.rowOf(m.lineCursor)
	cursorBottomRow := cursorTopRow + index.rowsIn(m.lineCursor)
	if cursorTopRow < m.logView.YOffset() {
		m.logView.SetYOffset(cursorTopRow)
		return
	}
	if cursorBottomRow > m.logView.YOffset()+m.logView.Height() {
		m.logView.SetYOffset(cursorBottomRow - m.logView.Height())
	}
}

func (m *LogsModel) logRowIndex() lineRowIndex {
	return newLineRowIndex(m.viewportLines(), m.logView.Width())
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
	m.selectionAnchor = index
	m.resetExplanation()
	m.rebuildInspectView()
	rows := m.logRowIndex()
	cursorRow := rows.rowOf(index)
	viewportTop := m.logView.YOffset()
	viewportBottom := viewportTop + m.logView.Height()
	if cursorRow < viewportTop || cursorRow >= viewportBottom {
		m.logView.SetYOffset(max(0, cursorRow-m.logView.Height()/centerDivisor))
	}
}

func (m *LogsModel) viewportLines() []string {
	rendered := m.renderLogLines(m.filteredLines)
	if !m.inspectMode {
		return rendered
	}
	first, last := m.selectionRange()
	for position := first; position <= last && position < len(rendered); position++ {
		prefix := selectionLinePrefix
		if position == m.lineCursor {
			prefix = inspectCursorPrefix
		}
		rendered[position] = theme.LogInspectCursor.Render(prefix + m.filteredLines[position])
	}
	return rendered
}

func (m LogsModel) selectionRange() (int, int) {
	if m.selectionAnchor < 0 || m.selectionAnchor >= len(m.filteredLines) {
		return m.lineCursor, m.lineCursor
	}
	return min(m.selectionAnchor, m.lineCursor), max(m.selectionAnchor, m.lineCursor)
}

func (m *LogsModel) selectedLines() []string {
	first, last := m.selectionRange()
	if first < 0 || last >= len(m.filteredLines) {
		return nil
	}
	return m.filteredLines[first : last+1]
}

func (m *LogsModel) syncLogContent() {
	m.logView.SetContent(strings.Join(m.viewportLines(), "\n"))
}

func (m *LogsModel) rebuildInspectView() {
	if len(m.filteredLines) == 0 {
		return
	}
	m.syncLogContent()
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
