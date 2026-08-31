package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/HediAbed/opsmate/internal/ui/component"
	"github.com/HediAbed/opsmate/internal/ui/theme"
)

func (m RootModel) switchScreen(screen screenID) (tea.Model, tea.Cmd) {
	cmd := m.transitionTo(screen, false)
	m.persistSession()
	return m, cmd
}

func (m *RootModel) transitionTo(screen screenID, rememberCurrent bool) tea.Cmd {
	if !validScreen(screen) || screen == m.screen {
		return nil
	}
	current := m.screen
	m.deactivateScreen(current)
	if rememberCurrent {
		m.prevScreen = current
	}
	m.screen = screen
	cmds := m.activateScreen(screen)
	m.resizeChildren()
	return tea.Batch(cmds...)
}

func validScreen(screen screenID) bool {
	return screen >= ScreenDashboard && screen <= ScreenCRDs
}

func screenIDFromPersisted(value int) (screenID, bool) {
	if value < int(ScreenDashboard) || value > int(ScreenCRDs) {
		return ScreenDashboard, false
	}
	return screenID(value), true
}

func (m *RootModel) activateScreen(screen screenID) []tea.Cmd {
	switch screen {
	case ScreenBrowser:
		return m.activateBrowser()
	case ScreenDashboard:
		return m.activateDashboard()
	case ScreenLogs:
		return m.activateLogs()
	case ScreenAnalysis:
		m.analysisPanel.SetVisible(true)
		m.analysisPanel.Focus()
		m.updateAnalysisScreenContext()
		m.resizeChildren()
		return nil
	case ScreenHelm:
		return m.activateHelm()
	case ScreenCRDs:
		return m.activateCRDs()
	default:
		return nil
	}
}

func (m *RootModel) activateBrowser() []tea.Cmd {
	commands := make([]tea.Cmd, 0, rootActivationCommandCapacity)
	if !m.browserInited {
		m.browserInited = true
		commands = append(commands, m.browser.Init())
	}
	return append(commands, m.browser.Activate())
}

func (m *RootModel) activateDashboard() []tea.Cmd {
	commands := make([]tea.Cmd, 0, rootActivationCommandCapacity)
	if !m.dashboardInited {
		m.dashboardInited = true
		commands = append(commands, m.dashboard.Init())
	}
	return append(commands, m.dashboard.Activate())
}

func (m *RootModel) activateLogs() []tea.Cmd {
	commands := make([]tea.Cmd, 0, rootActivationCommandCapacity)
	if !m.logsInited {
		m.logsInited = true
		commands = append(commands, m.logs.Init())
	}
	return append(commands, m.logs.Activate())
}

func (m *RootModel) activateHelm() []tea.Cmd {
	if !m.helmInited {
		m.helmInited = true
		return []tea.Cmd{m.helm.Init()}
	}
	return []tea.Cmd{m.helm.Activate()}
}

func (m *RootModel) activateCRDs() []tea.Cmd {
	if !m.crdsInited {
		m.crdsInited = true
		return []tea.Cmd{m.crds.Init()}
	}
	return []tea.Cmd{m.crds.Activate()}
}

func (m *RootModel) deactivateScreen(screen screenID) {
	switch screen {
	case ScreenBrowser:
		m.browser.Deactivate()
	case ScreenDashboard:
		m.dashboard.Deactivate()
	case ScreenLogs:
		m.logs.Deactivate()
	case ScreenAnalysis:
		m.analysisPanel.SetVisible(false)
	case ScreenHelm:
		m.helm.Deactivate()
	case ScreenCRDs:
		m.crds.Deactivate()
	}
}

func (m *RootModel) openSearch() tea.Cmd {
	m.showSearch = true
	m.searchCursor = 0
	m.searchInput.SetValue("")
	m.searchCorpus = m.collectSearchCorpus()
	m.searchResults = m.searchCorpus
	m.searchInput.Focus()
	return textinput.Blink
}

func (m RootModel) handleSearch(key string, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.showSearch = false
		m.searchInput.Blur()
		return m, nil
	case "up", "ctrl+p":
		if m.searchCursor > 0 {
			m.searchCursor--
		}
		return m, nil
	case "down", "ctrl+n":
		if m.searchCursor < len(m.searchResults)-1 {
			m.searchCursor++
		}
		return m, nil
	case "enter":
		if len(m.searchResults) == 0 {
			return m, nil
		}
		chosen := m.searchResults[m.searchCursor]
		m.showSearch = false
		m.searchInput.Blur()
		return m, func() tea.Msg {
			return DrillDownMsg{
				Screen:       ScreenBrowser,
				ResourceType: chosen.Kind,
				ResourceName: chosen.Name,
				ResourceNS:   chosen.Namespace,
			}
		}
	default:
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		m.searchResults = m.filterSearchResults(m.searchInput.Value())
		if m.searchCursor >= len(m.searchResults) {
			m.searchCursor = 0
		}
		return m, cmd
	}
}

func (m RootModel) collectSearchCorpus() []searchResult {
	candidates := append(m.dashboardSearchResults(), m.browserSearchResults()...)
	candidates = append(candidates, m.logSearchResults()...)
	return uniqueSearchResults(candidates)
}

func (m RootModel) dashboardSearchResults() []searchResult {
	results := make([]searchResult, 0, len(m.dashboard.pods)+len(m.dashboard.deployments))
	for _, pod := range m.dashboard.pods {
		results = append(results, searchResult{Kind: resourceKindPod, Name: pod.Name, Namespace: pod.Namespace})
	}
	for _, deployment := range m.dashboard.deployments {
		results = append(results, searchResult{Kind: resourceKindDeployment, Name: deployment.Name, Namespace: deployment.Namespace})
	}
	return results
}

func (m RootModel) browserSearchResults() []searchResult {
	capacity := len(m.browser.pods) + len(m.browser.deployments) + len(m.browser.services) +
		len(m.browser.statefulsets) + len(m.browser.daemonsets) + len(m.browser.configmaps) + len(m.browser.jobs)
	results := make([]searchResult, 0, capacity)
	for _, pod := range m.browser.pods {
		results = append(results, searchResult{Kind: resourceKindPod, Name: pod.Name, Namespace: pod.Namespace})
	}
	for _, deployment := range m.browser.deployments {
		results = append(results, searchResult{Kind: resourceKindDeployment, Name: deployment.Name, Namespace: deployment.Namespace})
	}
	for _, svc := range m.browser.services {
		results = append(results, searchResult{Kind: resourceKindService, Name: svc.Name, Namespace: svc.Namespace})
	}
	for _, statefulSet := range m.browser.statefulsets {
		results = append(results, searchResult{Kind: resourceKindStatefulSet, Name: statefulSet.Name, Namespace: statefulSet.Namespace})
	}
	for _, daemonSet := range m.browser.daemonsets {
		results = append(results, searchResult{Kind: resourceKindDaemonSet, Name: daemonSet.Name, Namespace: daemonSet.Namespace})
	}
	for _, configMap := range m.browser.configmaps {
		results = append(results, searchResult{Kind: resourceKindConfigMap, Name: configMap.Name, Namespace: configMap.Namespace})
	}
	for _, job := range m.browser.jobs {
		results = append(results, searchResult{Kind: resourceKindJob, Name: job.Name, Namespace: job.Namespace})
	}
	return results
}

func (m RootModel) logSearchResults() []searchResult {
	results := make([]searchResult, 0, len(m.logs.pods))
	for _, pod := range m.logs.pods {
		results = append(results, searchResult{Kind: resourceKindPod, Name: pod.Name, Namespace: pod.Namespace})
	}
	return results
}

func uniqueSearchResults(candidates []searchResult) []searchResult {
	seen := make(map[searchResult]struct{})
	results := make([]searchResult, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Name == "" {
			continue
		}
		if _, duplicate := seen[candidate]; duplicate {
			continue
		}
		seen[candidate] = struct{}{}
		results = append(results, candidate)
	}
	return results
}

func (m RootModel) filterSearchResults(query string) []searchResult {
	if query == "" {
		return m.searchCorpus
	}
	normalizedQuery := strings.ToLower(query)
	var matches []searchResult
	for _, result := range m.searchCorpus {
		if strings.Contains(strings.ToLower(result.Name), normalizedQuery) {
			matches = append(matches, result)
		}
	}
	return matches
}

func (m RootModel) renderSearchOverlay(height int) string {
	title := theme.Title.Render("FIND RESOURCE")

	inputLine := m.searchInput.View()

	visibleMax := height - searchOverlayChromeHeight
	if visibleMax < searchMinimumVisibleItems {
		visibleMax = searchMinimumVisibleItems
	}
	start := 0
	if m.searchCursor >= visibleMax {
		start = m.searchCursor - visibleMax + 1
	}
	end := start + visibleMax
	if end > len(m.searchResults) {
		end = len(m.searchResults)
	}

	var lines []string
	if len(m.searchResults) == 0 {
		lines = append(lines, theme.Dim.Render("no matches (try refreshing screens first; search is limited to cached data)"))
	}
	for index := start; index < end; index++ {
		result := m.searchResults[index]
		label := fmt.Sprintf("%-12s %-32s %s", result.Kind, result.Name, result.Namespace)
		if index == m.searchCursor {
			lines = append(lines, theme.TableSelected.Render(" ▸ "+label+" "))
		} else {
			lines = append(lines, theme.Dim.Render("   "+label))
		}
	}

	help := theme.HelpKey.Render("↑/↓") + theme.HelpDesc.Render(" move  ") +
		theme.HelpKey.Render("enter") + theme.HelpDesc.Render(" open  ") +
		theme.HelpKey.Render("esc") + theme.HelpDesc.Render(" cancel")

	content := title + "\n\n" + inputLine + "\n\n" + strings.Join(lines, "\n") + "\n\n" + help

	box := lipgloss.NewStyle().
		Border(lipgloss.ThickBorder()).
		BorderForeground(theme.NeonCyan).
		Padding(0, 1).
		Width(component.FitModalWidth(searchModalDesiredWidth, m.width)).
		MaxWidth(m.width).
		Render(content)

	return lipgloss.Place(m.width, height, lipgloss.Center, lipgloss.Center, box)
}
