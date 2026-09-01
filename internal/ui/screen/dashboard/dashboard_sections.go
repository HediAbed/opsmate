package dashboard

import (
	"cmp"
	"fmt"
	"image/color"
	"math"
	"slices"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/terminal"
	"github.com/HediAbed/opsmate/internal/ui/theme"
)

const eventMessageSuffix = "..."

func (m DashboardModel) renderTopConsumers(innerW int) string {
	if len(m.metrics) == 0 {
		return ""
	}

	type podUsage struct {
		name, cpu, memory string
	}
	var usage []podUsage
	for _, pod := range m.pods {
		if pod.CPU != "" && pod.CPU != "-" {
			name := displayPodName(pod.Namespace, pod.Name, m.namespace == "")
			usage = append(usage, podUsage{name: name, cpu: pod.CPU, memory: pod.Memory})
		}
	}
	if len(usage) == 0 {
		return ""
	}

	slices.SortFunc(usage, func(left, right podUsage) int {
		return cmp.Compare(parseMilli(right.cpu), parseMilli(left.cpu))
	})

	maxShow := min(dashboardTopConsumerLimit, len(usage))
	header := theme.Accent.Render("TOP RESOURCE CONSUMERS")
	lines := []string{header}
	nameW := max(dashboardNameMinimumWidth, innerW-dashboardInfoColumnsWidth)
	for index := 0; index < maxShow; index++ {
		consumer := usage[index]
		name := consumer.name
		if len(name) > nameW {
			name = name[:nameW-1] + "~"
		}
		lines = append(lines, fmt.Sprintf("  %-*s  CPU:%-8s  Mem:%s", nameW, name, consumer.cpu, consumer.memory))
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(theme.BorderColor).
		Padding(0, 1).Width(innerW + dashboardPanelOuterChrome).Render(strings.Join(lines, "\n"))
}

func sortedEventsNewestFirst(events []cluster.Event) []cluster.Event {
	warnings := make([]cluster.Event, 0, len(events))
	others := make([]cluster.Event, 0, len(events))
	for _, ev := range events {
		if ev.Type == "Warning" {
			warnings = append(warnings, ev)
		} else {
			others = append(others, ev)
		}
	}
	slices.SortFunc(warnings, func(left, right cluster.Event) int {
		return right.LastTimestamp.Compare(left.LastTimestamp)
	})
	slices.SortFunc(others, func(left, right cluster.Event) int {
		return right.LastTimestamp.Compare(left.LastTimestamp)
	})
	return append(warnings, others...)
}

func parseMilli(value string) int {
	value = strings.TrimSpace(value)
	if value == "" || value == "-" {
		return 0
	}
	if strings.HasSuffix(value, "m") {
		millicores, err := strconv.Atoi(strings.TrimSuffix(value, "m"))
		if err != nil {
			return 0
		}
		return millicores
	}
	cores, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return cores * milliCPUPerCore
}

func renderBar(pct float64, width int, fillColor, emptyColor color.Color) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 1 {
		pct = 1
	}
	filled := int(math.Round(pct * float64(width)))
	empty := width - filled

	fillStyle := lipgloss.NewStyle().Foreground(fillColor)
	emptyStyle := lipgloss.NewStyle().Foreground(emptyColor)

	if pct > dashboardCriticalUsageThreshold {
		fillStyle = barFillCritical
	} else if pct > dashboardWarningUsageThreshold {
		fillStyle = barFillWarning
	}

	return fillStyle.Render(strings.Repeat("█", filled)) +
		emptyStyle.Render(strings.Repeat("░", empty))
}

type dashboardAlert struct {
	icon   string
	pod    string
	reason string
	style  lipgloss.Style
}

func (m DashboardModel) renderAlerts(innerW int) string {
	alerts := m.collectDashboardAlerts()
	if len(alerts) == 0 {
		return ""
	}

	header := theme.Error.Render("ALERTS") + theme.Dim.Render(fmt.Sprintf(" (%d)", len(alerts)))
	maxAlerts := min(dashboardAlertLimit, len(alerts))
	lines := []string{header}
	nameW := max(dashboardNameMinimumWidth, innerW-dashboardInfoColumnsWidth)
	for index := 0; index < maxAlerts; index++ {
		alert := alerts[index]
		name := alert.pod
		if len(name) > nameW {
			name = name[:nameW-1] + "~"
		}
		lines = append(lines, alert.style.Render(fmt.Sprintf("  %s %-*s %s", alert.icon, nameW, name, alert.reason)))
	}
	if len(alerts) > maxAlerts {
		lines = append(lines, theme.Dim.Render(fmt.Sprintf("  +%d more", len(alerts)-maxAlerts)))
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(theme.Red).
		Padding(0, 1).Width(innerW + dashboardPanelOuterChrome).Render(strings.Join(lines, "\n"))
}

func (m DashboardModel) collectDashboardAlerts() []dashboardAlert {
	alerts := make([]dashboardAlert, 0)
	for _, pod := range m.pods {
		if alert, found := m.alertForPod(pod); found {
			alerts = append(alerts, alert)
		}
	}
	return alerts
}

func (m DashboardModel) alertForPod(pod cluster.Pod) (dashboardAlert, bool) {
	name := displayPodName(pod.Namespace, pod.Name, m.namespace == "")
	switch pod.Status {
	case "CrashLoopBackOff":
		return dashboardAlert{icon: "⚠", pod: name, reason: "CrashLoopBackOff", style: alertCritical}, true
	case "ImagePullBackOff", "ErrImagePull":
		return dashboardAlert{icon: "⚠", pod: name, reason: "ImagePullBackOff", style: alertCritical}, true
	case "Error", "Failed":
		return dashboardAlert{icon: "✗", pod: name, reason: pod.Status, style: alertCritical}, true
	case "Pending":
		return dashboardAlert{icon: "◷", pod: name, reason: "Pending", style: alertWarning}, true
	default:
		if pod.Restarts > dashboardRestartAlertThreshold {
			return dashboardAlert{icon: "↻", pod: name, reason: fmt.Sprintf("Restarts:%d", pod.Restarts), style: alertInfo}, true
		}
		return dashboardAlert{}, false
	}
}

func (m DashboardModel) renderDeploymentHealth(innerW int) string {
	header := theme.Accent.Render("DEPLOYMENT HEALTH")
	barW := max(dashboardDeploymentBarMinimumWidth, innerW/dashboardDeploymentBarWidthDivisor)
	lines := []string{header}
	maxDeploys := min(dashboardDeploymentLimit, len(m.deployments))

	for index := 0; index < maxDeploys; index++ {
		deployment := m.deployments[index]
		name := displayPodName(deployment.Namespace, deployment.Name, m.namespace == "")
		nameW := max(dashboardNameMinimumWidth, innerW-barW-dashboardInfoColumnsWidth)
		if len(name) > nameW {
			name = name[:nameW-1] + "~"
		}

		ready, desired := parseReadyReplicas(deployment.Ready)
		pct := 0.0
		if desired > 0 {
			pct = float64(ready) / float64(desired)
		}
		barColor := theme.Green
		if pct < 1.0 {
			barColor = theme.Yellow
		}
		if pct == 0 {
			barColor = theme.Red
		}

		bar := renderBar(pct, barW, barColor, theme.DimText)
		lines = append(lines, fmt.Sprintf("  %-*s %s %s  Age:%s", nameW, name, bar, deployment.Ready, deployment.Age))
	}
	if len(m.deployments) > maxDeploys {
		lines = append(lines, theme.Dim.Render(fmt.Sprintf("  +%d more", len(m.deployments)-maxDeploys)))
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(theme.ElectricPurp).
		Padding(0, 1).Width(innerW + dashboardPanelOuterChrome).Render(strings.Join(lines, "\n"))
}

func parseReadyReplicas(value string) (int, int) {
	parts := strings.SplitN(value, "/", deploymentReadyPartCount)
	if len(parts) != deploymentReadyPartCount {
		return 0, 0
	}
	ready, readyErr := strconv.Atoi(parts[0])
	desired, desiredErr := strconv.Atoi(parts[1])
	if readyErr != nil || desiredErr != nil {
		return 0, 0
	}
	return ready, desired
}

func (m DashboardModel) renderPodTable(innerW int) string {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(theme.BorderColor).
		Padding(0, 1).Width(innerW + dashboardPanelOuterChrome)

	if m.loading && len(m.pods) == 0 {
		ns := m.namespace
		if ns == "" {
			ns = "all namespaces"
		}
		body := m.spinner.View() + " " + theme.Dim.Render("Loading pods in "+ns+"...")
		centered := lipgloss.Place(innerW, m.podTable.Height(), lipgloss.Center, lipgloss.Center, body)
		return style.Render(centered)
	}
	if !m.loading && len(m.pods) == 0 {
		ns := m.namespace
		if ns == "" {
			ns = "all namespaces"
		}
		hint := theme.Dim.Render("No pods in "+ns+".") + "\n\n" +
			theme.HelpKey.Render("[n]") + theme.HelpDesc.Render(" namespace  ") +
			theme.HelpKey.Render("[k]") + theme.HelpDesc.Render(" context  ") +
			theme.HelpKey.Render("[:]") + theme.HelpDesc.Render(" command palette")
		centered := lipgloss.Place(innerW, m.podTable.Height(), lipgloss.Center, lipgloss.Center, hint)
		return style.Render(centered)
	}
	return style.Render(m.podTable.View())
}

func (m DashboardModel) renderEvents(innerW int) string {
	header := theme.Accent.Render("RECENT EVENTS")
	maxEvents := min(dashboardEventLimit, len(m.events))
	lines := []string{header}

	sorted := sortedEventsNewestFirst(m.events)

	for index := 0; index < maxEvents && index < len(sorted); index++ {
		event := sorted[index]
		var typeStyle lipgloss.Style
		switch event.Type {
		case "Warning":
			typeStyle = theme.Warning
		case "Normal":
			typeStyle = lipgloss.NewStyle().Foreground(theme.Green)
		default:
			typeStyle = theme.Dim
		}
		message := terminal.SanitizeText(event.Message)
		maxMsg := max(dashboardEventMessageMinimumWidth, innerW-dashboardEventMessageReservedWidth)
		message = terminal.TruncateRunes(message, maxMsg, eventMessageSuffix)
		line := fmt.Sprintf("  %s %-12s %s",
			typeStyle.Render(fmt.Sprintf("%-8s", terminal.SanitizeText(event.Type))),
			terminal.SanitizeText(event.Reason),
			message,
		)
		if event.Count > 1 {
			line += theme.Dim.Render(fmt.Sprintf(" (x%d)", event.Count))
		}
		lines = append(lines, line)
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(theme.BorderColor).
		Padding(0, 1).Width(innerW + dashboardPanelOuterChrome).Render(strings.Join(lines, "\n"))
}

func (m DashboardModel) renderHealthAnalysis(innerW int) string {
	header := theme.AnalysisAccent.Render("ANALYSIS ") + theme.Accent.Render("CLUSTER HEALTH")

	var content string
	if m.healthAnalysisLoading {
		content = m.spinner.View() + " " + theme.Dim.Render("Analyzing cluster health...")
	} else if m.healthAnalysisErr != nil {
		content = theme.Error.Render(dashboardAnalysisErrorText(m.healthAnalysisErr))
	} else if m.healthAnalysisSummary != "" {
		content = lipgloss.NewStyle().Foreground(theme.LightText).Render(m.healthAnalysisSummary)
	} else {
		content = theme.Dim.Render("No analysis available yet.")
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(theme.ElectricPurp).
		Padding(0, 1).Width(innerW + dashboardPanelOuterChrome).Render(header + "\n" + content)
}

func dashboardAnalysisErrorText(err error) string {
	return "Analysis error: " + terminal.SanitizeLine(err.Error())
}

func (DashboardModel) renderHelpLine(width int) string {
	keys := []struct{ key, desc string }{
		{"enter", "describe"}, {"l", "logs"}, {"a", "health analysis"},
		{"r", "refresh"}, {"n", "namespace"}, {"k", "context"}, {"tab", "analysis"},
	}
	parts := make([]string, 0, len(keys))
	for _, binding := range keys {
		parts = append(parts, theme.HelpKey.Render(binding.key)+theme.HelpDesc.Render(":"+binding.desc))
	}
	helpText := strings.Join(parts, theme.Dim.Render(" │ "))
	return lipgloss.NewStyle().Background(theme.DarkerBg).Foreground(theme.NeonCyan).Padding(0, 1).Width(width).Render(helpText)
}
