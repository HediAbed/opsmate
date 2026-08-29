package ui

import (
	"fmt"

	"charm.land/bubbles/v2/table"
	"charm.land/lipgloss/v2"
	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/theme"
	"github.com/HediAbed/opsmate/tui"
)

func (m CRDsModel) View() string {
	titleBar := m.renderTitleBar()
	errBanner := m.renderErrBanner()
	helpBar := m.renderHelpBar()

	chromeH := lipgloss.Height(titleBar) + lipgloss.Height(helpBar)
	if errBanner != "" {
		chromeH += lipgloss.Height(errBanner)
	}
	contentH := m.height - chromeH
	if contentH < 1 {
		contentH = 1
	}

	body := m.renderBody()
	body = lipgloss.NewStyle().Width(m.width).Height(contentH).MaxHeight(contentH).Render(body)

	sections := []string{titleBar}
	if errBanner != "" {
		sections = append(sections, errBanner)
	}
	sections = append(sections, body, helpBar)
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m CRDsModel) renderTitleBar() string {
	var bar string
	if m.view == crdsViewInstances {
		bar = " " + theme.Title.Render("CRD INSTANCES") + "  " + m.selectedCRD.Resource
	} else {
		bar = " " + theme.Title.Render("CUSTOM RESOURCE DEFINITIONS")
	}
	scope := m.namespace
	if scope == "" {
		scope = "all namespaces"
	}
	bar += "  ns:" + scope + " "
	if m.statusMsg != "" {
		bar += "  " + m.statusMsg
	}
	if cursor := tui.TableCursorLabel(m.activeTableCursor()); cursor != "" {
		bar += "  " + theme.Dim.Render(cursor)
	}
	return theme.StatusBarItem.Width(m.width).MaxWidth(m.width).Render(bar)
}

func (m CRDsModel) activeTableCursor() (int, int) {
	if m.view == crdsViewInstances {
		return m.instanceTable.Cursor(), len(m.instances)
	}
	return m.crdsTable.Cursor(), len(m.crds)
}

func (m CRDsModel) renderHelpBar() string {
	hints := " r:refresh  ↑/↓:navigate  enter:open  q:quit "
	if m.view == crdsViewInstances {
		hints = " r:refresh  ↑/↓:navigate  esc:back  q:quit "
	}
	return lipgloss.NewStyle().Foreground(theme.NeonCyan).Width(m.width).MaxWidth(m.width).Render(hints)
}

func (m CRDsModel) renderErrBanner() string {
	if m.err == nil {
		return ""
	}
	return theme.ErrorBanner.Width(m.width).MaxWidth(m.width).Render(" " + sanitizeTerminalLine(m.err.Error()) + " ")
}

func (m CRDsModel) renderBody() string {
	if m.loading {
		return m.spinner.View() + " loading..."
	}
	switch m.view {
	case crdsViewInstances:
		return m.renderCRDInstancesBody()
	case crdsViewList:
		return m.renderCRDListBody()
	default:
		return ""
	}
}

func (m CRDsModel) renderCRDInstancesBody() string {
	if m.err != nil && len(m.instances) == 0 {
		return ""
	}
	if len(m.instances) == 0 {
		return theme.Dim.Render(fmt.Sprintf("No %s instances in %s.", m.selectedCRD.Kind, m.scopeLabel()))
	}
	return m.instanceTable.View()
}

func (m CRDsModel) renderCRDListBody() string {
	if m.err != nil && len(m.crds) == 0 {
		return ""
	}
	if len(m.crds) == 0 {
		return theme.Dim.Render("No CRDs installed in this cluster.")
	}
	return m.crdsTable.View()
}

// scopeLabel formats the namespace for the empty-instances message.
func (m CRDsModel) scopeLabel() string {
	if m.namespace == "" {
		return "any namespace"
	}
	return "ns/" + m.namespace
}

func (m CRDsModel) crdsRows() []table.Row {
	rows := make([]table.Row, 0, len(m.crds))
	for _, definition := range m.crds {
		rows = append(rows, table.Row{
			definition.Name,
			definition.Group,
			definition.Scope,
			cluster.JoinVersions(definition.Versions),
			definition.Age,
		})
	}
	return rows
}

func (m CRDsModel) instanceRows() []table.Row {
	rows := make([]table.Row, 0, len(m.instances))
	for _, instance := range m.instances {
		rows = append(rows, table.Row{
			instance.Name,
			instance.Namespace,
			instance.Age,
		})
	}
	return rows
}

// SelectedCRDName returns the CRD currently under the cursor (or the
// drilled-into one when in instance view) for the breadcrumb.
func (m CRDsModel) SelectedCRDName() string {
	if m.view == crdsViewInstances {
		return m.selectedCRD.Name
	}
	row := m.crdsTable.SelectedRow()
	if len(row) == 0 {
		return ""
	}
	return row[0]
}
