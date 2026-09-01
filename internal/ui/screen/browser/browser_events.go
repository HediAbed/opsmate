package browser

import (
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/HediAbed/opsmate/internal/analysis"
	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/terminal"
	"github.com/HediAbed/opsmate/internal/ui/component"
	"github.com/HediAbed/opsmate/internal/ui/theme"
)

func (m BrowserModel) updateBrowserInputMessage(msg tea.Msg) (BrowserModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.MouseClickMsg:
		return m.handleBrowserMouseClick(msg)

	case tea.MouseWheelMsg:
		return m.handleBrowserMouseWheel(msg)

	case tea.MouseMotionMsg:
		return m.handleBrowserMouseMotion(msg)

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	default:
		return m, m.forwardBrowserDetailMessage(msg)
	}
}

func (m *BrowserModel) handleDescribeResult(msg cluster.DescribeMsg) {
	m.loading = false
	if msg.Err != nil {
		m.errBanner = operationErrorText("describe", msg.Err)
		return
	}
	output := terminal.SanitizeText(msg.Output)
	m.openBrowserDetail("describe", output, output, m.detailHelp())
}

func (m *BrowserModel) handleDescribeSummaryResult(msg analysis.DescribeSummaryMsg) {
	m.analysisSummaryLoading = false
	if msg.Err != nil {
		m.analysisSummaryErr = msg.Err
		m.analysisSummary = ""
	} else {
		m.analysisSummaryErr = nil
		m.analysisSummary = terminal.SanitizeText(msg.Summary)
	}
	m.rebuildDetailContent()
}

func (m *BrowserModel) handleEventsResult(msg cluster.EventsMsg) {
	m.loading = false
	if msg.Err != nil {
		m.errBanner = operationErrorText("events", msg.Err)
		return
	}
	content := formatEventsOutput(msg.Events)
	m.openBrowserDetail(
		"events",
		content,
		content,
		theme.Dim.Render("esc: back | scroll with j/k or mouse wheel"),
	)
}

func (m *BrowserModel) handleYAMLResult(msg cluster.YAMLMsg) {
	m.loading = false
	if msg.Err != nil {
		m.errBanner = operationErrorText("yaml", msg.Err)
		return
	}
	output := terminal.SanitizeText(msg.Output)
	m.openBrowserDetail(
		"yaml",
		output,
		component.HighlightYAML(output),
		theme.Dim.Render("esc: back | c: copy | v: split"),
	)
}

func (m *BrowserModel) openBrowserDetail(kind, content, renderedContent, status string) {
	m.errBanner = ""
	m.detailRequestID++
	m.detailKind = kind
	m.detailContent = content
	m.detailView.SetContent(renderedContent)
	m.detailView.GotoTop()
	m.showDetail = true
	m.state = stateDetail
	m.analysisSummary = ""
	m.analysisSummaryErr = nil
	m.analysisSummaryLoading = false
	m.statusMsg = status
}

func (m *BrowserModel) handleBrowserCommandResult(msg cluster.MutationResultMsg) tea.Cmd {
	m.loading = false
	m.showConfirm = false
	m.state = stateBrowsing
	if msg.Err != nil {
		m.statusMsg = theme.Error.Render("Command failed: " + terminal.SanitizeLine(msg.Err.Error()))
		return nil
	}
	m.statusMsg = theme.Success.Render(strings.TrimSpace(terminal.SanitizeText(msg.Output)))
	return m.loadCurrentResource()
}

func (m *BrowserModel) handleBrowserSpinnerTick(msg spinner.TickMsg) tea.Cmd {
	if !m.loading && !m.analysisSummaryLoading {
		return nil
	}
	var command tea.Cmd
	m.spinner, command = m.spinner.Update(msg)
	return command
}

func (m BrowserModel) handleBrowserMouseClick(msg tea.MouseClickMsg) (BrowserModel, tea.Cmd) {
	if m.state == stateDetail {
		var command tea.Cmd
		m.detailView, command = m.detailView.Update(msg)
		return m, command
	}
	if m.state == stateBrowsing && msg.Button == tea.MouseLeft {
		return m.handleBrowseClick(msg.X, msg.Y)
	}
	return m, nil
}

func (m BrowserModel) handleBrowserMouseWheel(msg tea.MouseWheelMsg) (BrowserModel, tea.Cmd) {
	switch m.state {
	case stateDetail:
		var command tea.Cmd
		m.detailView, command = m.detailView.Update(msg)
		return m, command
	case stateShell:
		var command tea.Cmd
		m.shellView, command = m.shellView.Update(msg)
		return m, command
	case stateBrowsing:
		m.moveBrowserSelectionWithWheel(msg.Button)
	case stateScaleInput, stateScaleConfirm, stateDeleteConfirm, stateFilter:
	}
	return m, nil
}

func (m *BrowserModel) moveBrowserSelectionWithWheel(button tea.MouseButton) {
	switch button {
	case tea.MouseWheelUp:
		m.resourceTable.MoveUp(component.TableWheelStep)
	case tea.MouseWheelDown:
		m.resourceTable.MoveDown(component.TableWheelStep)
	}
}

func (m BrowserModel) handleBrowserMouseMotion(msg tea.MouseMotionMsg) (BrowserModel, tea.Cmd) {
	if m.state != stateDetail {
		return m, nil
	}
	var command tea.Cmd
	m.detailView, command = m.detailView.Update(msg)
	return m, command
}

func (m *BrowserModel) forwardBrowserDetailMessage(msg tea.Msg) tea.Cmd {
	if m.state != stateDetail {
		return nil
	}
	var command tea.Cmd
	m.detailView, command = m.detailView.Update(msg)
	return command
}

func (m BrowserModel) acceptsFetchResult(msg browserResultMsg) bool {
	return msg.requestID == m.fetchRequestID &&
		msg.namespace == m.namespace &&
		msg.resourceType == m.resourceType
}

func (m BrowserModel) handleBrowseClick(column, row int) (BrowserModel, tea.Cmd) {
	titleHeight := lipgloss.Height(m.renderTitleBar())
	if row < titleHeight {
		return m.handleTitleBarClick(column)
	}
	rowIndex := row - m.tableFirstRowY()
	if rowIndex >= 0 && rowIndex < len(m.resourceTable.Rows()) {
		m.resourceTable.SetCursor(rowIndex)
	}
	return m, nil
}

func (m BrowserModel) tableFirstRowY() int {
	firstRow := lipgloss.Height(m.renderTitleBar()) + tableHeaderChromeRows
	if filter := m.renderFilterBar(); filter != "" {
		firstRow += lipgloss.Height(filter)
	}
	if errBanner := m.renderErrBanner(); errBanner != "" {
		firstRow += lipgloss.Height(errBanner)
	}
	return firstRow
}

func (m BrowserModel) handleTitleBarClick(column int) (BrowserModel, tea.Cmd) {
	titleRendered := theme.Title.Render("KUBERNETES BROWSER")
	relativeColumn := column - (lipgloss.Width(titleRendered) + titleBarSidePadding)
	if relativeColumn < 0 {
		return m, nil
	}
	resourceType, found := m.resourceTypeAtColumn(relativeColumn)
	if !found || resourceType == m.resourceType {
		return m, nil
	}
	m.SetResourceType(resourceType)
	return m, m.loadCurrentResource()
}

func (m BrowserModel) resourceTypeAtColumn(column int) (string, bool) {
	currentColumn := 0
	for _, resourceType := range allResourceTypes {
		tabWidth := m.resourceTabWidth(resourceType)
		if column < currentColumn+tabWidth {
			return resourceType, true
		}
		currentColumn += tabWidth
	}
	return "", false
}

func (m BrowserModel) resourceTabWidth(resourceType string) int {
	label := " " + strings.ToUpper(resourceType) + " "
	if resourceType == m.resourceType {
		return lipgloss.Width(theme.StatusBarActive.Render(label))
	}
	return lipgloss.Width(theme.StatusBarItem.Render(label))
}
