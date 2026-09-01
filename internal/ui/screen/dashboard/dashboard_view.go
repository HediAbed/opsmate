package dashboard

import (
	"fmt"
	"math"
	"strings"

	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
	"github.com/HediAbed/opsmate/internal/terminal"
	"github.com/HediAbed/opsmate/internal/ui/component"
	"github.com/HediAbed/opsmate/internal/ui/theme"
)

func (m DashboardModel) View() string {
	if m.width == 0 {
		return "Initializing dashboard..."
	}
	sections := m.renderDashboardSections()
	body := m.composeDashboardBody(sections)
	availableHeight := max(1, m.height-lipgloss.Height(sections.help))
	if lipgloss.Height(body) > availableHeight {
		bodyView := m.bodyView
		bodyView.SetContent(body)
		help := sections.help
		if hint := dashboardScrollHint(bodyView); hint != "" {
			help = component.AppendHint(help, theme.Dim.Render(hint), m.width)
		}
		return bodyView.View() + "\n" + help
	}
	if gap := availableHeight - lipgloss.Height(body); gap > 0 {
		body += strings.Repeat("\n", gap)
	}
	return body + "\n" + sections.help
}

func (m DashboardModel) renderDashboardSections() dashboardSections {
	sections := dashboardSections{
		title:        m.renderTitleBar(m.width),
		overview:     m.renderOverviewRow(m.width),
		help:         m.renderHelpLine(m.width),
		alerts:       m.renderAlerts(m.innerW()),
		topConsumers: m.renderTopConsumers(m.innerW()),
	}
	if m.err != nil {
		clean := terminal.SanitizeLine(m.err.Error())
		sections.errorBanner = lipgloss.NewStyle().
			Foreground(theme.Red).Bold(true).Width(m.width).Padding(0, 1).
			Render(fmt.Sprintf("⚠ ERROR: %s; press r to retry", clean))
	}
	if m.showHealthAnalysis {
		sections.health = m.renderHealthAnalysis(m.innerW())
	}
	if len(m.deployments) > 0 {
		sections.deployments = m.renderDeploymentHealth(m.innerW())
	}
	if len(m.events) > 0 {
		sections.events = m.renderEvents(m.innerW())
	}
	return sections
}

func (m DashboardModel) composeDashboardBody(sections dashboardSections) string {
	content := make([]string, 0, dashboardSectionCapacity)
	content = append(content, sections.title)
	if sections.errorBanner != "" {
		content = append(content, sections.errorBanner)
	}
	content = append(content, sections.overview)
	if sections.health != "" {
		content = append(content, sections.health)
	}
	if sections.alerts != "" {
		content = append(content, sections.alerts)
	}
	if sections.deployments != "" {
		content = append(content, sections.deployments)
	}
	if sections.topConsumers != "" {
		content = append(content, sections.topConsumers)
	}
	content = append(content, m.renderPodTable(m.innerW()))
	if sections.events != "" {
		content = append(content, sections.events)
	}
	return lipgloss.JoinVertical(lipgloss.Left, content...)
}

func (m *DashboardModel) syncDashboardLayout() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	sections := m.renderDashboardSections()
	chromeHeight := dashboardPanelOuterChrome
	for _, section := range []string{
		sections.title,
		sections.overview,
		sections.help,
		sections.errorBanner,
		sections.health,
		sections.alerts,
		sections.deployments,
		sections.events,
		sections.topConsumers,
	} {
		chromeHeight += lipgloss.Height(section)
	}
	tableWidth := max(1, m.innerW()-dashboardPanelOuterChrome)
	m.podTable.SetWidth(tableWidth)
	m.podTable.SetColumns(dashPodColumns(tableWidth))
	m.podTable.SetHeight(max(dashboardMinimumTableHeight, m.height-chromeHeight))

	body := m.composeDashboardBody(sections)
	availableHeight := max(1, m.height-lipgloss.Height(sections.help))
	m.bodyView.SetWidth(m.width)
	m.bodyView.SetHeight(availableHeight)
	m.bodyView.SetContent(body)
}

func dashboardScrollHint(view viewport.Model) string {
	direction := component.ViewportScrollDirection(view)
	if direction == component.ScrollNone {
		return ""
	}
	return fmt.Sprintf("%d%% %s", component.ViewportScrollPercent(view), direction.Arrows())
}

func (m DashboardModel) renderTitleBar(width int) string {
	title := theme.Title.Render("CLUSTER MONITOR")
	ns := theme.Subtitle.Render(m.namespace)

	right := ""
	if m.loading {
		right = m.spinner.View() + theme.SpinnerStyle.Render(fmt.Sprintf(" Syncing %s…", m.namespace))
	} else if !m.lastRefresh.IsZero() {
		right = theme.Dim.Render(fmt.Sprintf("last %s", m.lastRefresh.Format("15:04:05")))
	}

	left := title + " " + ns
	gap := max(1, width-lipgloss.Width(left)-lipgloss.Width(right)-1)
	bar := left + strings.Repeat(" ", gap) + right

	return lipgloss.NewStyle().Width(width).Background(theme.DarkerBg).Render(bar)
}

func (m DashboardModel) renderOverviewRow(width int) string {
	total := len(m.pods)
	running, pending, failed := 0, 0, 0
	for _, pod := range m.pods {
		switch pod.Status {
		case "Running":
			running++
		case "Pending":
			pending++
		case "Failed", "Error", "CrashLoopBackOff", "ImagePullBackOff":
			failed++
		}
	}

	badge := func(label string, val int, style lipgloss.Style) string {
		return style.Padding(0, 1).Render(fmt.Sprintf("%s:%d", label, val))
	}

	badges := lipgloss.JoinHorizontal(lipgloss.Center,
		badge("Pods", total, theme.Accent), " ",
		badge("Running", running, theme.Success), " ",
		badge("Pending", pending, theme.Warning), " ",
		badge("Failed", failed, theme.Error), " ",
		badge("Deploys", len(m.deployments), overviewBadgeRun),
	)

	var distBar string
	if total > 0 {
		barW := min(dashboardOverviewBarMaximumWidth, width/dashboardOverviewBarWidthDivisor)
		runPct := float64(running) / float64(total)
		pendPct := float64(pending) / float64(total)
		failPct := float64(failed) / float64(total)
		runW := int(math.Round(runPct * float64(barW)))
		pendW := int(math.Round(pendPct * float64(barW)))
		failW := int(math.Round(failPct * float64(barW)))
		otherW := barW - runW - pendW - failW
		if otherW < 0 {
			otherW = 0
		}
		distBar = " " +
			barFillRunning.Render(strings.Repeat("█", runW)) +
			barFillPending.Render(strings.Repeat("█", pendW)) +
			barFillFailed.Render(strings.Repeat("█", failW)) +
			barFillDimmed.Render(strings.Repeat("░", otherW))
	}

	row := badges + distBar
	return lipgloss.NewStyle().Width(width).Padding(0, 1).Render(row)
}
