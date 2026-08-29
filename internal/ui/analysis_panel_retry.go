package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

func (m *AnalysisPanelModel) retryLastQuery() tea.Cmd {
	if m.streaming || m.loading {
		return nil
	}
	last := m.lastFailedEntry()
	if last == nil {
		return nil
	}
	query := last.Query
	if query == "" {
		return nil
	}
	m.input.SetValue(query)
	m.input.Focus()
	return nil
}

func (m *AnalysisPanelModel) lastFailedEntry() *historyEntry {
	for index := len(m.history) - 1; index >= 0; index-- {
		entry := &m.history[index]
		if strings.HasPrefix(entry.Response, "Error:") || strings.HasPrefix(entry.Response, "Command failed:") {
			return entry
		}
	}
	return nil
}
