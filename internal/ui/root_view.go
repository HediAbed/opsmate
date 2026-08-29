package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/HediAbed/opsmate/internal/theme"
	"github.com/HediAbed/opsmate/tui"
)

func (m RootModel) View() tea.View {
	return tea.View{
		Content:   m.renderContent(),
		AltScreen: true,
		MouseMode: tea.MouseModeCellMotion,
	}
}

func (m RootModel) renderContent() string {
	if !m.ready {
		return "\n  Initializing OpsMate..."
	}

	footer := m.renderRootFooter()
	contentHeight := max(rootMinimumContentHeight, m.height-lipgloss.Height(footer))
	palette := m.renderCommandPalette()
	contentHeight = max(rootMinimumContentHeight, contentHeight-lipgloss.Height(palette))

	if overlay, visible := m.renderActiveRootOverlay(contentHeight); visible {
		return lipgloss.JoinVertical(lipgloss.Left, overlay, footer)
	}

	content := m.renderActiveScreen(contentHeight)
	sections := make([]string, 0, rootViewSectionCapacity)
	if palette != "" {
		sections = append(sections, palette)
	}
	sections = append(sections, content, footer)
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m RootModel) renderRootFooter() string {
	statusBar := m.renderStatusBar()
	errorBar := m.renderRootError()
	if errorBar == "" {
		return statusBar
	}
	return lipgloss.JoinVertical(lipgloss.Left, errorBar, statusBar)
}

func (m RootModel) renderCommandPalette() string {
	if !m.showCmdPalette {
		return ""
	}
	return lipgloss.NewStyle().
		Width(m.width).
		Background(theme.DarkerBg).
		Padding(0, rootHorizontalPadding).
		Render(m.cmdInput.View())
}

func (m RootModel) renderActiveRootOverlay(height int) (string, bool) {
	var overlay string
	switch {
	case m.showHelp:
		overlay = m.renderHelpOverlay(height)
	case m.showNSPicker:
		overlay = m.renderNSPicker(height)
	case m.showCtxPicker:
		overlay = m.renderCtxPicker(height)
	case m.showSearch:
		overlay = m.renderSearchOverlay(height)
	case m.showPFModal:
		overlay = m.renderPFModal(height)
	default:
		return "", false
	}
	return m.fitRootContent(overlay, height), true
}

func (m RootModel) renderActiveScreen(height int) string {
	if m.screen == ScreenAnalysis {
		return m.fitRootContent(m.analysisPanel.View(), height)
	}

	screenView := m.activeScreenView()
	if m.analysisPanel.IsVisible() {
		topOffset, bottomOffset := m.analysisPanelOverlayOffsets(height)
		analysisView := strings.Repeat("\n", topOffset) + m.analysisPanel.View() + strings.Repeat("\n", bottomOffset)
		screenView = lipgloss.JoinHorizontal(lipgloss.Top, screenView, analysisView)
	}
	return m.fitRootContent(screenView, height)
}

func (m RootModel) activeScreenView() string {
	switch m.screen {
	case ScreenDashboard:
		return m.dashboard.View()
	case ScreenBrowser:
		return m.browser.View()
	case ScreenLogs:
		return m.logs.View()
	case ScreenHelm:
		return m.helm.View()
	case ScreenCRDs:
		return m.crds.View()
	case ScreenAnalysis:
		return m.analysisPanel.View()
	default:
		return ""
	}
}

func (m RootModel) analysisPanelOverlayOffsets(height int) (int, int) {
	var topOffset, bottomOffset int
	switch m.screen {
	case ScreenBrowser:
		topOffset, _, bottomOffset = m.browser.AnalysisOverlayBounds(height)
	case ScreenLogs:
		topOffset, _, bottomOffset = m.logs.AnalysisOverlayBounds(height)
	case ScreenHelm:
		topOffset, _, bottomOffset = m.helm.AnalysisOverlayBounds(height)
	case ScreenCRDs:
		topOffset, _, bottomOffset = m.crds.AnalysisOverlayBounds(height)
	case ScreenDashboard, ScreenAnalysis:
	}
	return topOffset, bottomOffset
}

func (m RootModel) fitRootContent(content string, height int) string {
	return lipgloss.NewStyle().
		Width(m.width).
		Height(height).
		MaxHeight(height).
		Render(content)
}

func (m RootModel) renderRootError() string {
	if m.err == nil {
		return ""
	}
	message := sanitizeTerminalLine(m.err.Error())
	return theme.ErrorBanner.Width(m.width).MaxWidth(m.width).Render("ERROR: " + message + "   (esc dismiss)")
}

func (m RootModel) renderStatusBar() string {
	left := m.renderRootTabs()
	middle := "  " + m.renderBreadcrumb() + "  "
	right := m.renderStatusHints()
	gap := max(0, m.width-lipgloss.Width(left)-lipgloss.Width(middle)-lipgloss.Width(right))
	filler := lipgloss.NewStyle().
		Background(theme.DarkerBg).
		Foreground(theme.NeonCyan).
		Width(gap).
		Render("")

	return lipgloss.NewStyle().
		Width(m.width).
		Background(theme.DarkerBg).
		Render(left + middle + filler + right)
}

func (m RootModel) renderRootTabs() string {
	var tabs []string
	for _, tab := range rootScreenTabs {
		tabs = append(tabs, m.renderScreenTab(tab))
	}
	return lipgloss.JoinHorizontal(lipgloss.Center, tabs...)
}

func (m RootModel) renderBreadcrumb() string {
	sep := theme.BreadcrumbSep.Render(" > ")
	nsLabel := m.namespace
	if nsLabel == "" {
		nsLabel = "all-ns"
	}
	breadcrumb := theme.BreadcrumbStyle.Render(nsLabel)
	if m.currentContext != "" {
		breadcrumb = theme.Dim.Render(m.currentContext) + sep + theme.BreadcrumbStyle.Render(nsLabel)
	}
	return breadcrumb + m.renderScreenBreadcrumb(sep)
}

func (m RootModel) renderScreenBreadcrumb(separator string) string {
	switch m.screen {
	case ScreenBrowser:
		return m.renderBrowserBreadcrumb(separator)
	case ScreenLogs:
		return renderSelectedBreadcrumb(separator, "logs", m.logs.SelectedPod())
	case ScreenDashboard:
		return renderOptionalBreadcrumb(separator, m.dashboard.SelectedPod())
	case ScreenHelm:
		return renderSelectedBreadcrumb(separator, "helm", m.helm.SelectedRelease().Name)
	case ScreenCRDs:
		return renderSelectedBreadcrumb(separator, "crds", m.crds.SelectedCRDName())
	case ScreenAnalysis:
		return separator + theme.BreadcrumbStyle.Render("analysis")
	default:
		return ""
	}
}

func (m RootModel) renderBrowserBreadcrumb(separator string) string {
	resourceType := separator + theme.BreadcrumbStyle.Render(m.browser.ResourceType())
	_, selectedName := m.browser.SelectedResource()
	return resourceType + renderOptionalBreadcrumb(separator, selectedName)
}

func renderSelectedBreadcrumb(separator, section, selected string) string {
	return separator + theme.BreadcrumbStyle.Render(section) + renderOptionalBreadcrumb(separator, selected)
}

func renderOptionalBreadcrumb(separator, value string) string {
	if value == "" {
		return ""
	}
	return separator + theme.BreadcrumbStyle.Render(value)
}

func (m RootModel) renderStatusHints() string {
	var text string
	switch m.screen {
	case ScreenDashboard, ScreenHelm:
		text = " r:refresh  ?:help  q:quit "
	case ScreenBrowser:
		text = " /:filter  ?:help  q:quit "
	case ScreenLogs:
		text = " p:pod  f:filter  ?:help  q:quit "
	case ScreenAnalysis:
		text = " !:command  ?:help  q:quit "
	case ScreenCRDs:
		text = " enter:open  esc:back  ?:help  q:quit "
	default:
		return ""
	}
	return theme.StatusBarItem.Render(text)
}

func (m RootModel) renderScreenTab(tab rootScreenTab) string {
	label := fmt.Sprintf(" %s:%s ", tab.key, tab.name)
	if tab.id == m.screen {
		return theme.StatusBarActive.Render(label)
	}
	return theme.StatusBarItem.Render(label)
}

func (m RootModel) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if model, command, handled := m.handlePickerMouse(msg); handled {
		return model, command
	}
	if m.showHelp || m.showSearch || m.showPFModal || m.showCmdPalette {
		return m, nil
	}
	if model, command, handled := m.handleStatusBarMouse(msg); handled {
		return model, command
	}
	if model, command, handled := m.handleAnalysisPanelMouse(msg); handled {
		return model, command
	}
	return m.updateActiveScreen(msg)
}

func (m RootModel) handlePickerMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd, bool) {
	switch {
	case m.showNSPicker:
		model, command := m.handleNSPickerMouse(msg)
		return model, command, true
	case m.showCtxPicker:
		model, command := m.handleCtxPickerMouse(msg)
		return model, command, true
	default:
		return m, nil, false
	}
}

func (m RootModel) handleStatusBarMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd, bool) {
	if msg.Mouse().Y < m.height-rootStatusBarHeight {
		return m, nil, false
	}
	click, clickable := msg.(tea.MouseClickMsg)
	if !clickable || click.Button != tea.MouseLeft {
		return m, nil, true
	}
	model, command := m.handleStatusBarClick(click.X)
	return model, command, true
}

func (m RootModel) handleAnalysisPanelMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd, bool) {
	if !m.analysisPanel.IsVisible() || m.screen == ScreenAnalysis {
		return m, nil, false
	}
	mainWidth := m.width - analysisPanelWidth(m.width)
	if msg.Mouse().X < mainWidth {
		return m, nil, false
	}
	var command tea.Cmd
	m.analysisPanel, command = m.analysisPanel.Update(shiftMouseX(msg, -mainWidth))
	return m, command, true
}

func analysisPanelWidth(totalWidth int) int {
	preferred := totalWidth / analysisPanelWidthRatio
	minimum := min(analysisPanelMinimumWidth, totalWidth/analysisPanelMaximumRatio)
	maximum := totalWidth / analysisPanelMaximumRatio
	return min(max(preferred, minimum), maximum)
}

// shiftMouseX translates a typed mouse event horizontally.
func shiftMouseX(message tea.MouseMsg, horizontalOffset int) tea.Msg {
	switch event := message.(type) {
	case tea.MouseClickMsg:
		event.X += horizontalOffset
		return event
	case tea.MouseReleaseMsg:
		event.X += horizontalOffset
		return event
	case tea.MouseWheelMsg:
		event.X += horizontalOffset
		return event
	case tea.MouseMotionMsg:
		event.X += horizontalOffset
		return event
	}
	return message
}

func (m RootModel) handleStatusBarClick(column int) (tea.Model, tea.Cmd) {
	currentColumn := 0
	for _, tab := range rootScreenTabs {
		rendered := m.renderScreenTab(tab)
		tabWidth := lipgloss.Width(rendered)
		if column >= currentColumn && column < currentColumn+tabWidth {
			return m.switchScreen(tab.id)
		}
		currentColumn += tabWidth
	}

	breadcrumbStart := currentColumn + rootBreadcrumbSpacing
	namespaceLabel := theme.BreadcrumbStyle.Render(m.namespace)
	namespaceEnd := breadcrumbStart + lipgloss.Width(namespaceLabel)

	if column >= breadcrumbStart && column < namespaceEnd {
		m.showNSPicker = true
		m.nsCursor = 0
		if len(m.namespaces) == 0 {
			m.nsLoading = true
			return m, m.fetchNamespaces()
		}
		return m, nil
	}

	return m, nil
}

func (m RootModel) renderHelpOverlay(height int) string {
	title := theme.Title.Render("KEYBINDINGS")
	global := strings.Join(globalHelpBindings(), "\n")
	contextual := strings.Join(m.contextualHelpBindings(), "\n")
	columns := lipgloss.JoinHorizontal(lipgloss.Top, global, "    ", contextual)
	content := title + "\n\n" + columns + "\n\n" + theme.Dim.Render("Press ? or esc to close")

	box := lipgloss.NewStyle().
		Border(lipgloss.ThickBorder()).
		BorderForeground(theme.NeonCyan).
		Padding(0, rootHorizontalPadding).
		Width(tui.FitModalWidth(helpModalDesiredWidth, m.width)).
		MaxWidth(m.width).
		Render(content)
	return lipgloss.Place(m.width, height, lipgloss.Center, lipgloss.Center, box)
}

type rootHelpBinding struct {
	key         string
	description string
}

func renderHelpBindings(section string, bindings ...rootHelpBinding) []string {
	lines := make([]string, 1, len(bindings)+1)
	lines[0] = theme.Accent.Render(section)
	for _, binding := range bindings {
		key := theme.HelpKey.Render(fmt.Sprintf("  %-12s", binding.key))
		lines = append(lines, key+theme.HelpDesc.Render(binding.description))
	}
	return lines
}

func globalHelpBindings() []string {
	return renderHelpBindings("Global",
		rootHelpBinding{key: "1-6", description: "Switch screen"},
		rootHelpBinding{key: "n", description: "Namespace picker"},
		rootHelpBinding{key: "k", description: "Context picker"},
		rootHelpBinding{key: "tab", description: "Toggle analysis panel"},
		rootHelpBinding{key: ":", description: "Command palette"},
		rootHelpBinding{key: "ctrl+p", description: "Find resource"},
		rootHelpBinding{key: "F", description: "Port-forwards"},
		rootHelpBinding{key: "?", description: "Toggle help"},
		rootHelpBinding{key: "q", description: "Quit"},
	)
}

func (m RootModel) contextualHelpBindings() []string {
	switch m.screen {
	case ScreenDashboard:
		return renderHelpBindings("Dashboard",
			rootHelpBinding{key: "enter", description: "Describe pod in Browser"},
			rootHelpBinding{key: "l", description: "Open pod logs"},
			rootHelpBinding{key: "r", description: "Refresh"},
			rootHelpBinding{key: "up/down", description: "Navigate pods"},
		)
	case ScreenBrowser:
		return browserHelpBindings()
	case ScreenLogs:
		return logsHelpBindings()
	case ScreenAnalysis:
		return renderHelpBindings("Analysis",
			rootHelpBinding{key: "enter", description: "Send query"},
			rootHelpBinding{key: "!cmd", description: "Generate kubectl"},
			rootHelpBinding{key: "i / /", description: "Focus input"},
			rootHelpBinding{key: "esc", description: "Close panel"},
		)
	case ScreenHelm:
		return renderHelpBindings("Helm",
			rootHelpBinding{key: "up/down", description: "Navigate releases"},
			rootHelpBinding{key: "r", description: "Refresh"},
		)
	case ScreenCRDs:
		return renderHelpBindings("CRDs",
			rootHelpBinding{key: "up/down", description: "Navigate"},
			rootHelpBinding{key: "enter", description: "Open instances"},
			rootHelpBinding{key: "esc", description: "Back to list"},
			rootHelpBinding{key: "r", description: "Refresh"},
		)
	default:
		return nil
	}
}

func browserHelpBindings() []string {
	return renderHelpBindings("Browser",
		rootHelpBinding{key: "enter", description: "Describe resource"},
		rootHelpBinding{key: "y", description: "View YAML"},
		rootHelpBinding{key: "e", description: "Events"},
		rootHelpBinding{key: "l", description: "Logs (pods only)"},
		rootHelpBinding{key: "s", description: "Scale"},
		rootHelpBinding{key: "R", description: "Rollout restart"},
		rootHelpBinding{key: "X", description: "Shell exec (in-pane)"},
		rootHelpBinding{key: "x", description: "Delete"},
		rootHelpBinding{key: "space", description: "Mark / multi-select"},
		rootHelpBinding{key: "←/→", description: "Cycle resource type tabs"},
		rootHelpBinding{key: "p/d", description: "Pods / Deploys"},
		rootHelpBinding{key: "/", description: "Filter"},
		rootHelpBinding{key: "c", description: "Copy selection"},
		rootHelpBinding{key: "v", description: "Toggle split layout"},
		rootHelpBinding{key: "a", description: "Summarize detail"},
		rootHelpBinding{key: "w", description: "Toggle wide columns"},
		rootHelpBinding{key: "r", description: "Refresh"},
	)
}

func logsHelpBindings() []string {
	return renderHelpBindings("Logs",
		rootHelpBinding{key: "p", description: "Select pod"},
		rootHelpBinding{key: "o", description: "Select container"},
		rootHelpBinding{key: "i", description: "Inspect mode"},
		rootHelpBinding{key: "n / N", description: "Next / previous issue"},
		rootHelpBinding{key: "/", description: "Filter"},
		rootHelpBinding{key: "space", description: "Pause or resume"},
		rootHelpBinding{key: "r", description: "Refresh"},
		rootHelpBinding{key: "g / G", description: "Top / bottom"},
		rootHelpBinding{key: "+ / -", description: "Tail lines"},
		rootHelpBinding{key: "c / C", description: "Copy visible / all"},
		rootHelpBinding{key: "esc", description: "Back"},
	)
}
