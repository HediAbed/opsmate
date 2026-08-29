package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/HediAbed/opsmate/internal/theme"
	"github.com/HediAbed/opsmate/tui"
)

func (m *RootModel) openPFModal() {
	m.showPFModal = true
	m.pfCursor = 0
	m.pfSessions = m.operations.PortForwards()
}

func (m RootModel) handlePFModalKey(key string) (tea.Model, tea.Cmd) {
	if m.pfConfirmKillID != "" {
		return m.handlePFKillConfirmation(key)
	}
	switch key {
	case "esc", "F":
		m.showPFModal = false
		return m, nil
	case "up", "k":
		if m.pfCursor > 0 {
			m.pfCursor--
		}
	case "down", "j":
		if m.pfCursor < len(m.pfSessions)-1 {
			m.pfCursor++
		}
	case "r":
		m.refreshPFSessions()
	case "x":
		m.beginPFKillConfirmation()
	}
	return m, nil
}

func (m RootModel) handlePFKillConfirmation(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "y", "Y":
		portForwardID := m.pfConfirmKillID
		m.clearPFKillConfirmation()
		return m, m.operations.StopPortForward(portForwardID)
	case "n", "N", "esc":
		m.clearPFKillConfirmation()
	}
	return m, nil
}

func (m *RootModel) clearPFKillConfirmation() {
	m.pfConfirmKillID = ""
	m.pfConfirmKillOf = ""
}

func (m *RootModel) refreshPFSessions() {
	m.pfSessions = m.operations.PortForwards()
	if len(m.pfSessions) == 0 {
		m.pfCursor = 0
		return
	}
	m.pfCursor = min(m.pfCursor, len(m.pfSessions)-1)
}

func (m *RootModel) beginPFKillConfirmation() {
	if m.pfCursor < 0 || m.pfCursor >= len(m.pfSessions) {
		return
	}
	selectedSession := m.pfSessions[m.pfCursor]
	m.pfConfirmKillID = selectedSession.ID
	m.pfConfirmKillOf = fmt.Sprintf(
		"%s (%d:%d)",
		selectedSession.Pod.Name,
		selectedSession.LocalPort.Int(),
		selectedSession.RemotePort.Int(),
	)
}

func (m RootModel) renderPFModal(height int) string {
	title := theme.Title.Render("PORT FORWARDS")

	var lines []string
	if len(m.pfSessions) == 0 {
		lines = append(lines, theme.Dim.Render("No active port-forwards."))
		lines = append(lines, "")
		lines = append(lines, theme.Dim.Render("Start one with: "+
			theme.HelpKey.Render(":pf <pod> <local>:<remote>")))
	}
	for index, session := range m.pfSessions {
		uptime := formatUptime(time.Since(session.StartedAt))
		label := fmt.Sprintf("%-30s %5d:%-5d  %-10s  %s",
			truncatePF(session.Pod.Name, portForwardPodDisplayWidth),
			session.LocalPort.Int(),
			session.RemotePort.Int(),
			session.Status.String(),
			uptime,
		)
		if index == m.pfCursor {
			lines = append(lines, theme.TableSelected.Render(" ▸ "+label+" "))
		} else {
			lines = append(lines, theme.Dim.Render("   "+label))
		}
	}

	help := theme.HelpKey.Render("j/k") + theme.HelpDesc.Render(" move  ") +
		theme.HelpKey.Render("x") + theme.HelpDesc.Render(" kill  ") +
		theme.HelpKey.Render("r") + theme.HelpDesc.Render(" refresh  ") +
		theme.HelpKey.Render("esc") + theme.HelpDesc.Render(" close")

	var confirmBlock string
	if m.pfConfirmKillID != "" {
		warn := lipgloss.NewStyle().Foreground(theme.Red).Bold(true).Render("KILL " + m.pfConfirmKillOf + "?")
		prompt := lipgloss.NewStyle().Foreground(theme.Red).Bold(true).Render("[y]es / [n]o")
		confirmBlock = "\n\n" + warn + "\n" + prompt
	}

	content := title + "\n\n" + strings.Join(lines, "\n") + "\n\n" + help + confirmBlock

	box := lipgloss.NewStyle().
		Border(lipgloss.ThickBorder()).
		BorderForeground(theme.ElectricPurp).
		Padding(0, 1).
		Width(tui.FitModalWidth(portForwardModalDesiredWidth, m.width)).
		MaxWidth(m.width).
		Render(content)

	return lipgloss.Place(m.width, height, lipgloss.Center, lipgloss.Center, box)
}

func formatUptime(duration time.Duration) string {
	switch {
	case duration < time.Minute:
		return fmt.Sprintf("%ds", int(duration.Seconds()))
	case duration < time.Hour:
		return fmt.Sprintf("%dm", int(duration.Minutes()))
	case duration < 24*time.Hour:
		return fmt.Sprintf("%dh", int(duration.Hours()))
	default:
		return fmt.Sprintf("%dd", int(duration.Hours()/hoursPerDay))
	}
}

func truncatePF(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit-1] + "~"
}
