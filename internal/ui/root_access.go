package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/ui/component"
	"github.com/HediAbed/opsmate/internal/ui/screen"
	"github.com/HediAbed/opsmate/internal/ui/theme"
)

const (
	defaultClusterNamespace   = "default"
	namespaceListDeniedNotice = "Namespace list not permitted"
	namespaceTypePrompt       = "Type a namespace name:"
)

func (m *RootModel) applyNamespaces(msg cluster.NamespacesMsg) tea.Cmd {
	m.nsLoading = false
	if msg.Err == nil {
		m.namespaces = msg.Namespaces
		m.namespaceListDenied = false
		return nil
	}
	if !screen.AccessDenied(msg.Err) {
		m.setError(msg.Err)
		return nil
	}
	m.namespaces = nil
	m.namespaceListDenied = true
	if !m.showNSPicker {
		return m.leaveClusterScope()
	}
	m.nsInput.SetValue("")
	return tea.Batch(m.leaveClusterScope(), m.nsInput.Focus())
}

func (m *RootModel) applyContexts(msg cluster.ContextsMsg) tea.Cmd {
	m.ctxLoading = false
	m.contextsLoaded = true
	if msg.Err != nil {
		m.setError(msg.Err)
		return m.leaveClusterScope()
	}
	m.contexts = msg.Contexts
	m.contextNamespace = ""
	for _, clusterContext := range m.contexts {
		if clusterContext.Current {
			m.currentContext = clusterContext.Name
			m.contextNamespace = clusterContext.Namespace
			break
		}
	}
	return m.leaveClusterScope()
}

func (m *RootModel) leaveClusterScope() tea.Cmd {
	if !m.namespaceListDenied || !m.contextsLoaded || m.namespace != "" {
		return nil
	}
	m.namespace = m.fallbackNamespace()
	m.setNotice(fmt.Sprintf("Cluster-wide access is not permitted; showing namespace %s", m.namespace))
	return m.switchNamespace()
}

func (m RootModel) fallbackNamespace() string {
	if m.contextNamespace != "" {
		return m.contextNamespace
	}
	return defaultClusterNamespace
}

func (m RootModel) namespaceForContext(name string) string {
	for _, clusterContext := range m.contexts {
		if clusterContext.Name == name {
			return clusterContext.Namespace
		}
	}
	return ""
}

func (m *RootModel) openNamespacePicker() tea.Cmd {
	m.showNSPicker = true
	m.nsCursor = 0
	if m.namespaceListDenied {
		m.nsInput.SetValue("")
		return tea.Batch(m.fetchNamespaces(), m.nsInput.Focus())
	}
	if len(m.namespaces) > 0 {
		return nil
	}
	m.nsLoading = true
	return m.fetchNamespaces()
}

func (m RootModel) handleTypedNamespaceKey(key string, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.showNSPicker = false
		m.nsInput.Blur()
		return m, nil
	case "enter":
		return m, m.selectTypedNamespace()
	default:
		var command tea.Cmd
		m.nsInput, command = m.nsInput.Update(msg)
		return m, command
	}
}

func (m *RootModel) selectTypedNamespace() tea.Cmd {
	typed := strings.TrimSpace(m.nsInput.Value())
	if typed == "" {
		return nil
	}
	m.showNSPicker = false
	m.nsInput.Blur()
	m.namespace = typed
	m.notice = ""
	return m.switchNamespace()
}

func (m RootModel) renderTypedNamespacePicker(height int, title string) string {
	content := title + "\n\n" +
		theme.Notice.Render(namespaceListDeniedNotice) + "\n" +
		theme.Dim.Render(namespaceTypePrompt) + "\n\n" +
		m.nsInput.View() + "\n\n" +
		theme.HelpKey.Render("enter") + theme.HelpDesc.Render(": switch  ") +
		theme.HelpKey.Render("esc") + theme.HelpDesc.Render(": cancel")
	box := lipgloss.NewStyle().
		Border(lipgloss.ThickBorder()).
		BorderForeground(theme.NeonCyan).
		Padding(0, rootHorizontalPadding).
		Width(component.FitModalWidth(nsPickerModalDesiredWidth, m.width)).
		MaxWidth(m.width).
		Render(content)
	return lipgloss.Place(m.width, height, lipgloss.Center, lipgloss.Center, box)
}

func (m RootModel) renderRootNotice() string {
	if m.notice == "" {
		return ""
	}
	return theme.NoticeBanner.Width(m.width).MaxWidth(m.width).Render(m.notice + "   (esc dismiss)")
}
