package ui

import (
	"fmt"
	"image/color"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/ui/component"
	"github.com/HediAbed/opsmate/internal/ui/theme"
)

func (m BrowserModel) View() string {
	if m.width == 0 {
		return ""
	}

	titleBar := m.renderTitleBar()
	filterBar := m.renderFilterBar()
	errBanner := m.renderErrBanner()
	helpBar := m.renderHelpBar()
	statusLine := m.renderStatusLine()
	content := m.renderBrowserMainContent(m.browserContentHeight())
	sections := []string{titleBar}
	if filterBar != "" {
		sections = append(sections, filterBar)
	}
	if errBanner != "" {
		sections = append(sections, errBanner)
	}
	sections = append(sections, content, statusLine, helpBar)

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m BrowserModel) renderBrowserMainContent(height int) string {
	switch {
	case m.showConfirm:
		return lipgloss.Place(
			m.width,
			height,
			lipgloss.Center,
			lipgloss.Center,
			m.renderConfirmBox(),
		)
	case m.state == stateScaleInput:
		return lipgloss.Place(
			m.width,
			height,
			lipgloss.Center,
			lipgloss.Center,
			m.renderScaleBox(),
		)
	case m.state == stateShell:
		return m.renderShellSplit(height)
	case m.showDetail && m.splitHorizontal && m.width >= narrowHsplitMinWidth:
		return m.renderHSplitContent(height)
	case m.showDetail:
		return m.renderSplitContent(height)
	default:
		return m.renderTableContent(height)
	}
}

func (m BrowserModel) HasInputFocus() bool {
	return m.state == stateScaleInput || m.state == stateScaleConfirm ||
		m.state == stateDeleteConfirm || m.state == stateFilter ||
		m.state == stateShell
}

func (m BrowserModel) AnalysisOverlayBounds(totalHeight int) (topOffset, panelHeight, bottomOffset int) {
	topOffset = lipgloss.Height(m.renderTitleBar())
	if filter := m.renderFilterBar(); filter != "" {
		topOffset += lipgloss.Height(filter)
	}
	if errBan := m.renderErrBanner(); errBan != "" {
		topOffset += lipgloss.Height(errBan)
	}
	bottomOffset = lipgloss.Height(m.renderStatusLine()) + lipgloss.Height(m.renderHelpBar())
	return analysisOverlayBounds(totalHeight, topOffset, bottomOffset)
}

func (m BrowserModel) renderTitleBar() string {
	title := theme.Title.Render("KUBERNETES BROWSER")
	rightLabel := theme.Subtitle.Render("ns:" + m.namespace)
	if m.wide {
		rightLabel = theme.Accent.Render("WIDE ") + rightLabel
	}

	chromeWidth := lipgloss.Width(title) + titleBarSidePadding + lipgloss.Width(rightLabel) + titleBarSidePadding
	tabsWidth := m.width - chromeWidth
	if tabsWidth < browserTabStripMinWidth {
		tabsWidth = browserTabStripMinWidth
	}
	indicator := renderBrowserTabStrip(m.resourceType, tabsWidth)

	left := lipgloss.JoinHorizontal(lipgloss.Center, title, "  ", indicator)
	gap := max(1, m.width-lipgloss.Width(left)-lipgloss.Width(rightLabel)-titleBarSidePadding)
	bar := left + strings.Repeat(" ", gap) + rightLabel

	return lipgloss.NewStyle().MaxWidth(m.width).Background(theme.DarkerBg).Render(bar)
}

func renderBrowserTabStrip(active string, maxWidth int) string {
	if maxWidth < browserTabStripMinViable {
		return ""
	}
	rendered, widths, activeIdx := buildBrowserTabs(active)
	leftHint := theme.StatusBarItem.Render(" ‹ ")
	rightHint := theme.StatusBarItem.Render(" › ")

	start, end := windowAroundActive(activeIdx, widths, maxWidth, lipgloss.Width(leftHint), lipgloss.Width(rightHint))

	parts := make([]string, 0, end-start+browserTabHintCapacity)
	if start > 0 {
		parts = append(parts, leftHint)
	}
	parts = append(parts, rendered[start:end+1]...)
	if end < len(allResourceTypes)-1 {
		parts = append(parts, rightHint)
	}
	return lipgloss.JoinHorizontal(lipgloss.Center, parts...)
}

func buildBrowserTabs(active string) ([]string, []int, int) {
	rendered := make([]string, len(allResourceTypes))
	widths := make([]int, len(allResourceTypes))
	activeIdx := 0
	for i, rt := range allResourceTypes {
		label := " " + strings.ToUpper(rt) + " "
		if rt == active {
			rendered[i] = theme.StatusBarActive.Render(label)
			activeIdx = i
		} else {
			rendered[i] = theme.StatusBarItem.Render(label)
		}
		widths[i] = lipgloss.Width(rendered[i])
	}
	return rendered, widths, activeIdx
}

func windowAroundActive(activeIdx int, widths []int, maxWidth, leftHintW, rightHintW int) (start, end int) {
	start, end = activeIdx, activeIdx
	used := widths[activeIdx]
	for {
		extendedLeft := tryExtendLeft(&start, &used, widths, end, maxWidth, leftHintW)
		extendedRight := tryExtendRight(&end, &used, widths, start, maxWidth, rightHintW)
		if !extendedLeft && !extendedRight {
			return start, end
		}
	}
}

func tryExtendLeft(start, used *int, widths []int, end, maxWidth, leftHintW int) bool {
	if *start == 0 {
		return false
	}
	candidate := *start - 1
	need := widths[candidate]
	if candidate == 0 && end == len(widths)-1 {
		if *used+need > maxWidth {
			return false
		}
	} else if *used+need+leftHintW > maxWidth {
		return false
	}
	*start = candidate
	*used += need
	return true
}

func tryExtendRight(end, used *int, widths []int, start, maxWidth, rightHintW int) bool {
	if *end == len(widths)-1 {
		return false
	}
	candidate := *end + 1
	need := widths[candidate]
	if start == 0 && candidate == len(widths)-1 {
		if *used+need > maxWidth {
			return false
		}
	} else if *used+need+rightHintW > maxWidth {
		return false
	}
	*end = candidate
	*used += need
	return true
}

func (m BrowserModel) renderTableContent(height int) string {
	if m.loading && len(m.resourceTable.Rows()) == 0 {
		loadingMsg := m.spinner.View() + " " + theme.Dim.Render(
			fmt.Sprintf("Loading %s in %s...", m.resourceType, m.namespace))
		return lipgloss.Place(m.width, height, lipgloss.Center, lipgloss.Center, loadingMsg)
	}

	if len(m.resourceTable.Rows()) == 0 {
		emptyMsg := theme.Dim.Render("No "+m.resourceType+" found in "+m.namespace) + "\n\n" +
			theme.HelpKey.Render("[r]") + theme.HelpDesc.Render(" refresh  ") +
			theme.HelpKey.Render("[p]") + theme.HelpDesc.Render(" pods  ") +
			theme.HelpKey.Render("[d]") + theme.HelpDesc.Render(" deploys")
		return lipgloss.Place(m.width, height, lipgloss.Center, lipgloss.Center, emptyMsg)
	}

	return component.NewPanel(theme.BoxStyle).Render(
		component.Size{Width: m.width - browserPanelGutter, Height: height - browserPanelGutter},
		m.resourceTable.View(),
	)
}

func (m BrowserModel) renderSplitContent(height int) string {
	tableView := theme.BoxStyle.Width(m.width - browserPanelGutter).Render(m.resourceTable.View())

	detailTitle := theme.Accent.Render(" Detail ") + "  " +
		theme.HelpKey.Render("[v]") + theme.HelpDesc.Render(" toggle layout") +
		"  " + popupScrollIndicator(m.detailView)
	detailBody := m.detailView.View()
	detailBox := theme.ActiveBoxStyle.
		Width(m.width - browserPanelGutter).MaxWidth(m.width).
		Render(lipgloss.JoinVertical(lipgloss.Left, detailTitle, detailBody))

	combined := lipgloss.JoinVertical(lipgloss.Left, tableView, detailBox)
	return lipgloss.Place(m.width, height, lipgloss.Left, lipgloss.Top, combined)
}

func (m BrowserModel) renderHSplitContent(height int) string {
	leftW, rightW := browserHorizontalPaneWidths(m.width)

	tableView := component.NewPanel(theme.BoxStyle).Render(
		component.Size{Width: leftW - browserPanelGutter, Height: height - browserPanelGutter},
		m.resourceTable.View(),
	)

	detailTitle := theme.Accent.Render(" Detail ") + "  " +
		theme.HelpKey.Render("[v]") + theme.HelpDesc.Render(" toggle layout") +
		"  " + popupScrollIndicator(m.detailView)
	detailBody := m.detailView.View()
	detailBox := theme.ActiveBoxStyle.
		Width(rightW - browserPanelGutter).
		Height(height - browserPanelGutter).
		MaxWidth(m.width).
		Render(lipgloss.JoinVertical(lipgloss.Left, detailTitle, detailBody))

	combined := lipgloss.JoinHorizontal(lipgloss.Top, tableView, detailBox)
	return lipgloss.Place(m.width, height, lipgloss.Left, lipgloss.Top, combined)
}

func (m BrowserModel) renderConfirmBox() string {
	label, warning, borderColor := confirmDialogStyle(m.confirmAction)

	header := label + " " + theme.Accent.Render(m.confirmTarget)

	rows := []string{header}
	if warning != "" {
		rows = append(rows, "", warning)
	}
	rows = append(rows,
		"",
		"Proceed?",
		"",
		lipgloss.JoinHorizontal(lipgloss.Center,
			theme.HelpKey.Render("y")+" "+theme.HelpDesc.Render("yes")+"   ",
			theme.HelpKey.Render("n")+" "+theme.HelpDesc.Render("no"),
		),
	)

	body := lipgloss.JoinVertical(lipgloss.Center, rows...)

	return lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(borderColor).
		Padding(1, confirmHorizontalPadding).
		Foreground(theme.White).
		Width(component.FitModalWidth(confirmModalDesiredWidth, m.width)).
		MaxWidth(m.width).
		Align(lipgloss.Center).
		Render(body)
}

func confirmDialogStyle(action string) (label, warning string, border color.Color) {
	switch action {
	case "delete":
		return theme.Error.Render("DELETE"),
			theme.Error.Render("⚠ IRREVERSIBLE: the resource will be destroyed"),
			theme.Red
	case "restart":
		return theme.Warning.Render("RESTART"),
			theme.Warning.Render("Rolling restart: brief service disruption"),
			theme.Orange
	default:
		return theme.Warning.Render("SCALE"),
			"",
			theme.Yellow
	}
}

func (m BrowserModel) renderScaleBox() string {
	title := theme.Accent.Render("Scale " + m.scaleName)
	var info string
	if m.scaleCurrentInfo != "" {
		info = theme.Dim.Render(m.scaleCurrentInfo)
	}

	parts := []string{title}
	if info != "" {
		parts = append(parts, info)
	}
	parts = append(parts, "", m.textInput.View(), "", theme.Dim.Render("enter: confirm | esc: cancel"))

	return lipgloss.NewStyle().Border(lipgloss.ThickBorder()).BorderForeground(theme.NeonCyan).Padding(0, 1).
		Width(component.FitModalWidth(scaleModalDesiredWidth, m.width)).MaxWidth(m.width).Render(
		lipgloss.JoinVertical(lipgloss.Left, parts...),
	)
}

func (m BrowserModel) lookupReplicaInfoFor(identity resourceIdentity) string {
	switch m.resourceType {
	case resourceTypeDeployments:
		for _, deployment := range m.deployments {
			if deployment.Name == identity.Name && namespacesMatch(identity.Namespace, deployment.Namespace) {
				return fmt.Sprintf("currently %s ready, %d up-to-date", deployment.Ready, deployment.UpToDate)
			}
		}
	case resourceTypeStatefulSets:
		for _, statefulSet := range m.statefulsets {
			if statefulSet.Name == identity.Name && namespacesMatch(identity.Namespace, statefulSet.Namespace) {
				return fmt.Sprintf("currently %s ready, %d replicas", statefulSet.Ready, statefulSet.Replicas)
			}
		}
	}
	return ""
}

func (m BrowserModel) renderErrBanner() string {
	if m.errBanner == "" {
		return ""
	}
	msg := "ERROR: " + m.errBanner + "   (esc dismiss · r retry)"
	return theme.ErrorBanner.Width(m.width).MaxWidth(m.width).Render(msg)
}

func (m BrowserModel) renderFilterBar() string {
	if m.state == stateFilter {
		matchCount := len(m.resourceTable.Rows())
		filterPrompt := theme.Accent.Render("Filter: ")
		matchInfo := theme.Dim.Render(fmt.Sprintf(" (%d matching)", matchCount))
		return lipgloss.NewStyle().
			Width(m.width).
			Background(theme.DarkerBg).
			Padding(0, 1).
			Render(filterPrompt + m.filterInput.View() + matchInfo)
	}
	if m.filterActive && m.filterText != "" {
		matchCount := len(m.resourceTable.Rows())
		badge := theme.FilterBadge.Render(fmt.Sprintf("FILTER: %s", m.filterText))
		matchInfo := theme.Dim.Render(fmt.Sprintf(" (%d matching)", matchCount))
		clearHint := "  " + theme.HelpKey.Render("/") + theme.HelpDesc.Render(": edit  ")
		return lipgloss.NewStyle().
			Width(m.width).
			Background(theme.DarkerBg).
			Padding(0, 1).
			Render(badge + matchInfo + clearHint)
	}
	return ""
}

func (m BrowserModel) renderStatusLine() string {
	if m.loading {
		return m.spinner.View() + " " + m.statusMsg
	}
	if selectedCount := len(m.selected); selectedCount > 0 {
		badge := theme.FilterBadge.Render(fmt.Sprintf("%d SELECTED", selectedCount))
		hint := theme.Dim.Render("  x: delete all · R: restart all · esc: clear")
		return badge + hint
	}
	if m.statusMsg != "" {
		return m.statusMsg
	}
	count := m.currentResourceCount()
	return theme.Dim.Render(fmt.Sprintf("%d %s in %s", count, m.resourceType, m.namespace))
}

func (m BrowserModel) currentResourceCount() int {
	if b, ok := resourceCatalog[m.resourceType]; ok {
		mc := m
		return b.Count(&mc)
	}
	return 0
}

func (m BrowserModel) renderHelpBar() string {
	return browserHelpBarStyle.Width(m.width).MaxWidth(m.width).Render(browserHelpBarText)
}

func formatEventsOutput(events []cluster.Event) string {
	if len(events) == 0 {
		return "No events found."
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%-8s %-20s %-30s %s\n", "TYPE", "REASON", "OBJECT", "MESSAGE")
	b.WriteString(strings.Repeat("-", eventSeparatorWidth) + "\n")
	for _, ev := range events {
		eventType := sanitizeTerminalText(ev.Type)
		reason := sanitizeTerminalText(ev.Reason)
		object := sanitizeTerminalText(ev.Object)
		msg := sanitizeTerminalText(ev.Message)
		if utf8.RuneCountInString(msg) > maxEventMessageRunes {
			messageRunes := []rune(msg)
			msg = string(messageRunes[:maxEventMessageRunes-len(eventMessageSuffix)]) + eventMessageSuffix
		}
		fmt.Fprintf(&b, "%-8s %-20s %-30s %s\n", eventType, reason, object, msg)
	}
	return b.String()
}

func clearStatusAfter(delay time.Duration) tea.Cmd {
	return tea.Tick(delay, func(_ time.Time) tea.Msg {
		return ClearStatusMsg{}
	})
}
