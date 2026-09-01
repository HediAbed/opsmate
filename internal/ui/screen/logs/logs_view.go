package logs

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/HediAbed/opsmate/internal/terminal"
	"github.com/HediAbed/opsmate/internal/ui/component"
	"github.com/HediAbed/opsmate/internal/ui/theme"
)

func (m LogsModel) View() string {
	if m.width == 0 {
		return ""
	}
	titleBar := m.renderTitleBar()
	helpLine := m.renderHelpBar()
	filterBar := m.renderFilterBar()
	contentHeight := m.logContentHeight()
	sections := []string{titleBar, m.renderLogMainContent(contentHeight)}
	for _, optionalSection := range []string{
		m.renderOptionalExplainPanel(),
		m.renderLogError(),
		m.statusMsg,
		filterBar,
	} {
		if optionalSection != "" {
			sections = append(sections, optionalSection)
		}
	}
	sections = append(sections, helpLine)
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m LogsModel) renderLogMainContent(contentHeight int) string {
	switch {
	case m.showContainerPopup:
		return m.renderContainerPopupOverlay(m.width, contentHeight)
	case m.showPodPopup:
		return m.renderPodPopupOverlay(m.width, contentHeight)
	default:
		return m.renderLogPanel(contentHeight)
	}
}

func (m LogsModel) renderLogPanel(contentHeight int) string {
	content := m.logView.View()
	if m.selectedPod == "" && !m.loading {
		emptyMessage := theme.Dim.Render("No pod selected") + "\n\n" +
			theme.HelpKey.Render("[p]") + theme.HelpDesc.Render(" to select a pod")
		content = lipgloss.Place(
			m.logView.Width(),
			max(1, contentHeight-logsEmptyStateVerticalChrome),
			lipgloss.Center,
			lipgloss.Center,
			emptyMessage,
		)
	}
	return logsPanel().Render(
		component.Size{
			Width:  m.width - logsPanelGutter,
			Height: max(1, contentHeight-logsPanelGutter),
		},
		content,
	)
}

func logsPanel() component.Panel {
	return component.NewPanel(theme.BoxStyle.BorderForeground(theme.NeonCyan))
}

func (m LogsModel) renderOptionalExplainPanel() string {
	if !m.inspectMode {
		return ""
	}
	return m.renderExplainPanel()
}

func (m LogsModel) renderLogError() string {
	if m.err == nil {
		return ""
	}
	message := terminal.SanitizeLine(m.err.Error())
	return theme.Error.Render("Error: " + message + "; press r to retry")
}

func (m *LogsModel) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.recalcLayout()
}

func (m LogsModel) AnalysisOverlayBounds(totalHeight int) (topOffset, panelHeight, bottomOffset int) {
	topOffset = lipgloss.Height(m.renderTitleBar())
	bottomOffset = lipgloss.Height(m.renderHelpBar())
	if filter := m.renderFilterBar(); filter != "" {
		bottomOffset += lipgloss.Height(filter)
	}
	if m.statusMsg != "" {
		bottomOffset += lipgloss.Height(m.statusMsg)
	}
	if m.err != nil {
		bottomOffset++
	}
	if m.inspectMode {
		bottomOffset += lipgloss.Height(m.renderExplainPanel())
	}
	return component.AnalysisOverlayBounds(totalHeight, topOffset, bottomOffset)
}

func (m LogsModel) renderTitleBar() string {
	titleText := theme.Title.Render("LOG VIEWER")
	podLabel := m.renderLogPodLabel()
	indicators := m.renderLogTitleIndicators()
	titleLeft := lipgloss.JoinHorizontal(lipgloss.Center, titleText, "  ", podLabel)
	titleRight := strings.Join(indicators, "  ")
	titleBarStyle := lipgloss.NewStyle().
		Width(m.width).
		MaxWidth(m.width).
		Background(theme.DarkerBg).
		Padding(0, 1)
	gap := max(1, m.width-lipgloss.Width(titleLeft)-lipgloss.Width(titleRight)-logsPanelGutter)
	return titleBarStyle.Render(titleLeft + strings.Repeat(" ", gap) + titleRight)
}

func (m LogsModel) renderLogPodLabel() string {
	if m.selectedPod != "" {
		podLabel := theme.Subtitle.Render(m.selectedPod)
		if m.selectedContainer != "" {
			podLabel += theme.Dim.Render("/") + theme.Accent.Render(m.selectedContainer)
		}
		podLabel += "  " + theme.HelpKey.Render("[p]") + theme.HelpDesc.Render(" pod") +
			"  " + theme.HelpKey.Render("[o]") + theme.HelpDesc.Render(" container")
		return podLabel
	}
	return theme.Warning.Render("No pod selected") + "  " +
		theme.HelpKey.Render("[p]") + theme.HelpDesc.Render(" to choose")
}

func (m LogsModel) renderLogTitleIndicators() []string {
	indicators := []string{m.renderLogModeIndicator()}
	if issueCount := m.logIssueCount(); issueCount > 0 {
		indicators = append(indicators, theme.Warning.Render(fmt.Sprintf("⚠ %d issues", issueCount)))
	}
	if m.loading {
		indicators = append(indicators, m.spinner.View()+" "+theme.Dim.Render("fetching..."))
	}
	lineCount := theme.Dim.Render(fmt.Sprintf("[%d lines | tail %d]", len(m.filteredLines), m.tailLines))
	indicators = append(indicators, lineCount)
	if percentage := m.scrollPctLabel(); percentage != "" {
		indicators = append(indicators, percentage)
	}
	return indicators
}

func (m LogsModel) renderLogModeIndicator() string {
	if m.inspectMode {
		return theme.LogGutterError.Render(" INSPECT ")
	}
	if m.paused {
		return theme.Warning.Render(" PAUSED ")
	}
	if m.autoScroll {
		return theme.IndicatorOn.Render(" FOLLOW ")
	}
	return theme.IndicatorOff.Render(" SCROLL ")
}

func (m LogsModel) logIssueCount() int {
	issueCount := 0
	for _, line := range m.filteredLines {
		if classifyLine(line) >= sevWarn {
			issueCount++
		}
	}
	return issueCount
}

func (m LogsModel) scrollPctLabel() string {
	if m.autoScroll && m.logView.AtBottom() {
		return ""
	}
	return theme.Dim.Render(fmt.Sprintf("%d%%", component.ViewportScrollPercent(m.logView)))
}

func (m LogsModel) renderHelpBar() string {
	var helpParts []string
	if m.inspectMode {
		helpParts = []string{
			theme.HelpKey.Render("j/k") + theme.HelpDesc.Render(": move"),
			theme.HelpKey.Render("enter") + theme.HelpDesc.Render(": explain"),
			theme.HelpKey.Render("n/N") + theme.HelpDesc.Render(": next/prev issue"),
			theme.HelpKey.Render("esc") + theme.HelpDesc.Render(": exit inspect"),
		}
	} else {
		helpParts = []string{
			theme.HelpKey.Render("p") + theme.HelpDesc.Render(": pods"),
			theme.HelpKey.Render("i") + theme.HelpDesc.Render(": inspect"),
			theme.HelpKey.Render("/") + theme.HelpDesc.Render(": filter"),
			theme.HelpKey.Render("space") + theme.HelpDesc.Render(": pause"),
			theme.HelpKey.Render("r") + theme.HelpDesc.Render(": refresh"),
			theme.HelpKey.Render("g/G") + theme.HelpDesc.Render(": top/bottom"),
			theme.HelpKey.Render("+/-") + theme.HelpDesc.Render(": tail lines"),
			theme.HelpKey.Render("c/C") + theme.HelpDesc.Render(": copy"),
		}
	}
	return lipgloss.NewStyle().
		Width(m.width).
		MaxWidth(m.width).
		Background(theme.DarkerBg).
		Foreground(theme.DimText).
		Padding(0, 1).
		Render(strings.Join(helpParts, "  |  "))
}

func (m LogsModel) renderFilterBar() string {
	if m.filterInput.Focused() {
		filterPrompt := theme.Accent.Render("Filter: ")
		return lipgloss.NewStyle().
			Width(m.width).
			MaxWidth(m.width).
			Background(theme.DarkerBg).
			Padding(0, 1).
			Render(filterPrompt + m.filterInput.View())
	}
	if m.filter != "" {
		filterPrompt := theme.Accent.Render("Filter: ")
		filterValue := theme.Subtitle.Render(m.filter)
		matchInfo := theme.Dim.Render(fmt.Sprintf(" (%d/%d)", len(m.filteredLines), len(m.allLines)))
		return lipgloss.NewStyle().
			Width(m.width).
			Background(theme.DarkerBg).
			Padding(0, 1).
			Render(filterPrompt + filterValue + matchInfo)
	}
	return ""
}

func (m LogsModel) renderExplainPanel() string {
	var content string
	if m.lineExplanationLoading {
		content = m.spinner.View() + " " + theme.Dim.Render("Analyzing this line...")
	} else if m.lineExplanationErr != nil {
		content = theme.Error.Render("Analysis error: " + terminal.SanitizeLine(m.lineExplanationErr.Error()))
	} else if m.lineExplanation != "" {
		content = theme.LogGutterError.Render("ANALYSIS ") + theme.Subtitle.Render("Explanation") + "\n" +
			lipgloss.NewStyle().Foreground(theme.LightText).Render(m.lineExplanation)
	} else {
		content = theme.Dim.Render("Press ") + theme.HelpKey.Render("enter") + theme.Dim.Render(" on a line to request an explanation")
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.ElectricPurp).
		Width(m.width-logsPanelGutter).
		Padding(0, 1).
		Render(content)
}

func (m LogsModel) renderContainerPopupOverlay(width, height int) string {
	title := theme.Title.Render("SELECT CONTAINER")

	if len(m.containers) == 0 {
		box := lipgloss.NewStyle().Border(lipgloss.ThickBorder()).BorderForeground(theme.ElectricPurp).Padding(0, 1).
			Width(logsPopupWidth(containerPopupDesiredWidth, width)).
			Render(title + "\n\n" + theme.Dim.Render("No containers found."))
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
	}

	var items []string
	for index, container := range m.containers {
		if index == m.containerCursor {
			items = append(items, theme.TableSelected.Render(fmt.Sprintf(" > %-30s ", container)))
		} else if container == m.selectedContainer {
			items = append(items, theme.Accent.Render(fmt.Sprintf("   %-30s (current)", container)))
		} else {
			items = append(items, theme.Dim.Render(fmt.Sprintf("   %-30s", container)))
		}
	}

	content := title + "\n\n" + strings.Join(items, "\n") + "\n\n" +
		theme.HelpKey.Render("enter") + theme.HelpDesc.Render(": select  ") +
		theme.HelpKey.Render("esc") + theme.HelpDesc.Render(": cancel")

	popupWidth := logsPopupWidth(containerPopupDesiredWidth, width)
	box := lipgloss.NewStyle().Border(lipgloss.ThickBorder()).BorderForeground(theme.ElectricPurp).Padding(0, 1).
		Width(popupWidth).
		Render(content)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

func (m LogsModel) renderPodPopupOverlay(width, height int) string {
	title := theme.Title.Render("SELECT POD")
	if len(m.pods) == 0 {
		return renderEmptyPodPopup(title, width, height)
	}
	start, end := m.visiblePodRange(height)
	content := title + "\n\n" + strings.Join(m.renderPodPopupItems(start, end), "\n") + "\n\n" +
		theme.HelpKey.Render("enter") + theme.HelpDesc.Render(": select  ") +
		theme.HelpKey.Render("esc") + theme.HelpDesc.Render(": cancel")
	box := lipgloss.NewStyle().Border(lipgloss.ThickBorder()).BorderForeground(theme.NeonCyan).Padding(0, 1).
		Width(logsPopupWidth(podPopupDesiredWidth, width)).
		Render(content)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

func renderEmptyPodPopup(title string, width, height int) string {
	box := lipgloss.NewStyle().Border(lipgloss.ThickBorder()).BorderForeground(theme.NeonCyan).Padding(0, 1).
		Width(logsPopupWidth(podPopupDesiredWidth, width)).
		Render(title + "\n\n" + theme.Dim.Render("No pods found."))
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

func (m LogsModel) visiblePodRange(height int) (int, int) {
	maximumVisible := max(podPopupMinimumVisibleItems, height-podPopupListChrome)
	start := 0
	if m.podCursor >= maximumVisible {
		start = m.podCursor - maximumVisible + 1
	}
	return start, min(len(m.pods), start+maximumVisible)
}

func (m LogsModel) renderPodPopupItems(start, end int) []string {
	items := make([]string, 0, end-start)
	for index := start; index < end; index++ {
		pod := m.pods[index]
		name := pod.Name
		if m.namespace == "" && pod.Namespace != "" {
			name = pod.Namespace + "/" + pod.Name
		}
		statusStr := theme.PodStatusStyle(pod.Status).Render(pod.Status)
		if index == m.podCursor {
			items = append(items, theme.TableSelected.Render(fmt.Sprintf(" > %-36s ", name))+" "+statusStr)
		} else if pod.Name == m.selectedPod && m.selectedPodNS() == pod.Namespace {
			items = append(items, theme.Accent.Render(fmt.Sprintf("   %-36s (current)", name))+" "+statusStr)
		} else {
			items = append(items, theme.Dim.Render(fmt.Sprintf("   %-36s", name))+" "+statusStr)
		}
	}
	return items
}

func logsPopupWidth(desired, terminalWidth int) int {
	return min(desired, max(1, terminalWidth-logsPopupHorizontalChrome))
}
