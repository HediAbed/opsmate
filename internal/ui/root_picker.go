package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/theme"
	"github.com/HediAbed/opsmate/tui"
)

func (m RootModel) handleNSPicker(key string) (tea.Model, tea.Cmd) {
	totalItems := len(m.namespaces) + allNamespacesPickerItemCount
	switch key {
	case "esc", "n":
		m.showNSPicker = false
		return m, nil
	case "up", "k":
		m.nsCursor = max(0, m.nsCursor-1)
	case "down", "j":
		m.nsCursor = min(totalItems-1, m.nsCursor+1)
	case "enter":
		return m, m.selectNamespace(m.nsCursor)
	}
	return m, nil
}

func (m RootModel) handleNSPickerMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch event := msg.(type) {
	case tea.MouseClickMsg:
		return m.handleNamespacePickerClick(event)
	case tea.MouseWheelMsg:
		totalItems := len(m.namespaces) + allNamespacesPickerItemCount
		m.nsCursor = pickerCursorAfterWheel(m.nsCursor, totalItems, event.Button)
	}
	return m, nil
}

func (m RootModel) handleCtxPickerMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch event := msg.(type) {
	case tea.MouseClickMsg:
		return m.handleContextPickerClick(event)
	case tea.MouseWheelMsg:
		m.ctxCursor = pickerCursorAfterWheel(m.ctxCursor, len(m.contexts), event.Button)
	}
	return m, nil
}

type rootPickerWindow struct {
	start   int
	end     int
	itemTop int
}

type rootPickerItem struct {
	label   string
	current bool
}

func (m RootModel) handleNamespacePickerClick(click tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	totalItems := len(m.namespaces) + allNamespacesPickerItemCount
	window := calculatePickerWindow(m.height-rootStatusBarHeight, totalItems, m.nsCursor)
	selectedIndex, selected := pickerClickedIndex(click, window)
	if !selected {
		return m, nil
	}
	m.nsCursor = selectedIndex
	return m, m.selectNamespace(selectedIndex)
}

func (m RootModel) handleContextPickerClick(click tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	window := calculatePickerWindow(m.height-rootStatusBarHeight, len(m.contexts), m.ctxCursor)
	selectedIndex, selected := pickerClickedIndex(click, window)
	if !selected {
		return m, nil
	}
	m.ctxCursor = selectedIndex
	return m, m.selectContext(selectedIndex)
}

func calculatePickerWindow(contentHeight, totalItems, cursor int) rootPickerWindow {
	maximumVisible := max(pickerMinimumVisibleItems, contentHeight-pickerChromeHeight)
	visibleCount := min(maximumVisible, totalItems)
	normalizedCursor := min(max(0, totalItems-1), max(0, cursor))
	start := max(0, normalizedCursor-maximumVisible+1)
	return rootPickerWindow{
		start:   start,
		end:     min(start+maximumVisible, totalItems),
		itemTop: (contentHeight-(visibleCount+pickerChromeHeight))/2 + pickerItemTopOffset,
	}
}

func pickerClickedIndex(click tea.MouseClickMsg, window rootPickerWindow) (int, bool) {
	visibleCount := window.end - window.start
	if click.Button != tea.MouseLeft || click.Y < window.itemTop || click.Y >= window.itemTop+visibleCount {
		return 0, false
	}
	return window.start + click.Y - window.itemTop, true
}

func pickerCursorAfterWheel(cursor, totalItems int, button tea.MouseButton) int {
	switch button {
	case tea.MouseWheelUp:
		return max(0, cursor-1)
	case tea.MouseWheelDown:
		return min(max(0, totalItems-1), cursor+1)
	default:
		return cursor
	}
}

func (m *RootModel) selectNamespace(index int) tea.Cmd {
	if index < allNamespacesPickerIndex || index > len(m.namespaces) {
		return nil
	}
	m.showNSPicker = false
	if index == allNamespacesPickerIndex {
		m.namespace = ""
	} else if namespaceIndex := index - allNamespacesPickerItemCount; namespaceIndex < len(m.namespaces) {
		m.namespace = m.namespaces[namespaceIndex]
	}
	return m.switchNamespace()
}

func (m RootModel) renderNSPicker(height int) string {
	title := theme.Title.Render("SELECT NAMESPACE")
	if m.nsLoading {
		return m.renderPickerState(height, title, m.nsSpinner.View()+" Loading...")
	}
	if len(m.namespaces) == 0 {
		return m.renderPickerState(height, title, "No namespaces found.")
	}

	items := renderPickerItems(namespacePickerItems(m.namespaces, m.namespace), m.nsCursor, height)
	content := title + "\n\n" + strings.Join(items, "\n") + "\n\n" +
		theme.HelpKey.Render("enter") + theme.HelpDesc.Render(": select  ") +
		theme.HelpKey.Render("esc") + theme.HelpDesc.Render(": cancel")
	box := lipgloss.NewStyle().
		Border(lipgloss.ThickBorder()).
		BorderForeground(theme.NeonCyan).
		Padding(0, rootHorizontalPadding).
		Width(tui.FitModalWidth(nsPickerModalDesiredWidth, m.width)).
		MaxWidth(m.width).
		Render(content)
	return lipgloss.Place(m.width, height, lipgloss.Center, lipgloss.Center, box)
}

func (m RootModel) renderPickerState(height int, title, message string) string {
	content := theme.BoxStyle.Render(title + "\n\n" + message)
	return lipgloss.Place(m.width, height, lipgloss.Center, lipgloss.Center, content)
}

func namespacePickerItems(namespaces []string, currentNamespace string) []rootPickerItem {
	items := make([]rootPickerItem, 0, len(namespaces)+allNamespacesPickerItemCount)
	items = append(items, rootPickerItem{label: "All Namespaces", current: currentNamespace == ""})
	for _, namespace := range namespaces {
		items = append(items, rootPickerItem{label: namespace, current: namespace == currentNamespace})
	}
	return items
}

func renderPickerItems(items []rootPickerItem, cursor, height int) []string {
	window := calculatePickerWindow(height, len(items), cursor)
	lines := make([]string, 0, window.end-window.start)
	for index := window.start; index < window.end; index++ {
		lines = append(lines, renderPickerItem(items[index], index == cursor))
	}
	return lines
}

func renderPickerItem(item rootPickerItem, selected bool) string {
	switch {
	case selected:
		return theme.TableSelected.Render(fmt.Sprintf(" ▸ %s ", item.label))
	case item.current:
		return theme.Accent.Render(fmt.Sprintf("   %s ●", item.label))
	default:
		return theme.Dim.Render("   " + item.label)
	}
}

func (m RootModel) handleCtxPicker(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.showCtxPicker = false
		return m, nil
	case "up", "k":
		m.ctxCursor = max(0, m.ctxCursor-1)
	case "down", "j":
		m.ctxCursor = min(max(0, len(m.contexts)-1), m.ctxCursor+1)
	case "enter":
		return m, m.selectContext(m.ctxCursor)
	}
	return m, nil
}

func (m *RootModel) selectContext(index int) tea.Cmd {
	if index < 0 || index >= len(m.contexts) {
		return nil
	}
	selected := m.contexts[index]
	m.showCtxPicker = false
	if selected.Current {
		return nil
	}
	m.contextSwitching = true
	m.deactivateScreen(m.screen)
	return m.switchContext(selected.Name)
}

func (m RootModel) renderCtxPicker(height int) string {
	title := theme.Title.Render("SELECT CONTEXT")
	if m.ctxLoading {
		return m.renderPickerState(height, title, m.nsSpinner.View()+" Loading contexts...")
	}
	if len(m.contexts) == 0 {
		return m.renderPickerState(height, title, "No contexts found.")
	}

	items := renderPickerItems(contextPickerItems(m.contexts), m.ctxCursor, height)
	content := title + "\n\n" + strings.Join(items, "\n") + "\n\n" +
		theme.HelpKey.Render("enter") + theme.HelpDesc.Render(": switch  ") +
		theme.HelpKey.Render("esc") + theme.HelpDesc.Render(": cancel")
	box := lipgloss.NewStyle().
		Border(lipgloss.ThickBorder()).
		BorderForeground(theme.ElectricPurp).
		Padding(0, rootHorizontalPadding).
		Width(tui.FitModalWidth(ctxPickerModalDesiredWidth, m.width)).
		MaxWidth(m.width).
		Render(content)
	return lipgloss.Place(m.width, height, lipgloss.Center, lipgloss.Center, box)
}

func contextPickerItems(contexts []cluster.KubeContext) []rootPickerItem {
	items := make([]rootPickerItem, 0, len(contexts))
	for _, context := range contexts {
		items = append(items, rootPickerItem{label: context.Name, current: context.Current})
	}
	return items
}

// switchNamespace updates every screen and persists the selection.
func (m *RootModel) switchNamespace() tea.Cmd {
	m.deactivateScreen(m.screen)
	m.dashboard.SetNamespace(m.namespace)
	m.browser.SetNamespace(m.namespace)
	m.logs.SetNamespace(m.namespace)
	m.helm.SetNamespace(m.namespace)
	m.crds.SetNamespace(m.namespace)
	m.analysisPanel.SetNamespace(m.namespace)
	m.persistSession()
	return tea.Batch(m.activateScreen(m.screen)...)
}
