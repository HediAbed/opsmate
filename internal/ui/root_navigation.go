package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/HediAbed/opsmate/internal/ui/component"
	screenmodel "github.com/HediAbed/opsmate/internal/ui/screen"
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
		return m, searchDrillDownCommand(chosen)
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

func searchDrillDownCommand(result screenmodel.SearchItem) tea.Cmd {
	return func() tea.Msg {
		return DrillDownMsg{
			Screen:       ScreenBrowser,
			ResourceType: string(result.Kind),
			ResourceName: result.Name,
			ResourceNS:   result.Namespace,
		}
	}
}

func (m RootModel) collectSearchCorpus() []screenmodel.SearchItem {
	candidates := append(m.dashboard.SearchItems(), m.browser.SearchItems()...)
	candidates = append(candidates, m.logs.SearchItems()...)
	return uniqueSearchResults(candidates)
}

func uniqueSearchResults(candidates []screenmodel.SearchItem) []screenmodel.SearchItem {
	seen := make(map[screenmodel.SearchItem]struct{})
	results := make([]screenmodel.SearchItem, 0, len(candidates))
	for _, candidate := range candidates {
		if !candidate.Valid() {
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

func (m RootModel) filterSearchResults(query string) []screenmodel.SearchItem {
	if query == "" {
		return m.searchCorpus
	}
	normalizedQuery := strings.ToLower(query)
	var matches []screenmodel.SearchItem
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
