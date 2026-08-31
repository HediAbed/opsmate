package ui

import (
	"strconv"

	"charm.land/bubbles/v2/table"
	"charm.land/lipgloss/v2"
	"github.com/HediAbed/opsmate/internal/ui/theme"
)

func (m *DashboardModel) recalcLayout() {
	m.syncDashboardLayout()
}

func dashPodColumns(innerW int) []table.Column {
	cellPad := dashboardPodColumnCount * tableCellPadding
	minW := dashboardPodColumnBaseWidth + cellPad
	if innerW < minW {
		innerW = minW
	}
	available := innerW - cellPad
	statusW := max(dashboardStatusMinimumWidth, available/dashboardStatusWidthDivisor)
	readyW := max(dashboardReadyMinimumWidth, available/dashboardReadyWidthDivisor)
	rstW := max(dashboardRestartMinimumWidth, available/dashboardRestartWidthDivisor)
	ageW := max(dashboardAgeMinimumWidth, available/dashboardReadyWidthDivisor)
	cpuW := max(dashboardMetricMinimumWidth, available/dashboardMetricWidthDivisor)
	memW := max(dashboardMetricMinimumWidth, available/dashboardMetricWidthDivisor)
	fixed := statusW + readyW + rstW + ageW + cpuW + memW
	nameW := available - fixed
	return []table.Column{
		{Title: "NAME", Width: nameW},
		{Title: "STATUS", Width: statusW},
		{Title: "READY", Width: readyW},
		{Title: "RST", Width: rstW},
		{Title: "AGE", Width: ageW},
		{Title: "CPU", Width: cpuW},
		{Title: "MEM", Width: memW},
	}
}

func dashTableStyles() table.Styles {
	styles := table.DefaultStyles()
	styles.Header = styles.Header.Bold(true).Foreground(theme.NeonCyan).
		BorderStyle(lipgloss.NormalBorder()).BorderForeground(theme.DimText).BorderBottom(true)
	styles.Selected = styles.Selected.Foreground(theme.White).Background(theme.DeepViolet).Bold(true)
	styles.Cell = styles.Cell.Foreground(theme.LightText)
	return styles
}

func (m *DashboardModel) rebuildTableRows() {
	rows := make([]table.Row, 0, len(m.pods))
	identities := make([]resourceIdentity, 0, len(m.pods))
	for _, pod := range m.pods {
		cpu, memory := pod.CPU, pod.Memory
		if cpu == "" {
			cpu = "-"
		}
		if memory == "" {
			memory = "-"
		}
		rows = append(rows, table.Row{
			displayResourceName(pod.Namespace, pod.Name, m.namespace == ""),
			pod.Status,
			pod.Ready,
			strconv.Itoa(pod.Restarts),
			pod.Age,
			cpu,
			memory,
		})
		identities = append(identities, resourceIdentity{Kind: resourceKindPod, Namespace: pod.Namespace, Name: pod.Name})
	}
	m.podRows = identities
	m.podTable.SetRows(rows)
}
