package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/HediAbed/opsmate/internal/ui/component"
	"github.com/HediAbed/opsmate/internal/ui/theme"
)

func (m LogsModel) HasInputFocus() bool {
	return m.filterInput.Focused() || m.showPodPopup || m.showContainerPopup || m.inspectMode
}

func (m *LogsModel) recalcLayout() {
	m.syncLogViewport()
}

func (m *LogsModel) syncLogViewport() {
	inner := logsPanel().ContentSize(component.Size{
		Width:  m.width - logsPanelGutter,
		Height: m.logContentHeight(),
	})
	m.logView.SetWidth(max(1, inner.Width))
	m.logView.SetHeight(max(1, inner.Height))
}

func (m LogsModel) logContentHeight() int {
	usedLines := lipgloss.Height(m.renderTitleBar()) + lipgloss.Height(m.renderHelpBar())
	if filterBar := m.renderFilterBar(); filterBar != "" {
		usedLines += lipgloss.Height(filterBar)
	}
	if m.err != nil {
		usedLines++
	}
	if m.statusMsg != "" {
		usedLines += lipgloss.Height(m.statusMsg)
	}
	if m.inspectMode {
		usedLines += lipgloss.Height(m.renderExplainPanel())
	}
	return max(1, m.height-usedLines)
}

func (m *LogsModel) applyFilter() {
	if m.filter == "" {
		m.filteredLines = make([]string, len(m.allLines))
		copy(m.filteredLines, m.allLines)
		return
	}

	lowerFilter := strings.ToLower(m.filter)
	m.filteredLines = m.filteredLines[:0]
	for _, line := range m.allLines {
		if strings.Contains(strings.ToLower(line), lowerFilter) {
			m.filteredLines = append(m.filteredLines, line)
		}
	}
}

type lineSeverity int

const (
	sevNone lineSeverity = iota
	sevDebug
	sevStack
	sevWarn
	sevError
	sevCritical
)

func classifyLine(line string) lineSeverity {
	lower := strings.ToLower(line)
	switch {
	case containsEither(lower, "fatal", "panic"):
		return sevCritical
	case containsEither(lower, "error", "exception"):
		return sevError
	case strings.Contains(lower, "warn"):
		return sevWarn
	case isStackTraceLine(lower):
		return sevStack
	case containsEither(lower, "debug", "trace"):
		return sevDebug
	default:
		return sevNone
	}
}

func containsEither(value, first, second string) bool {
	return strings.Contains(value, first) || strings.Contains(value, second)
}

func isStackTraceLine(lowercaseLine string) bool {
	return strings.HasPrefix(strings.TrimSpace(lowercaseLine), "at ") ||
		strings.Contains(lowercaseLine, "goroutine ") ||
		strings.Contains(lowercaseLine, "stacktrace") ||
		strings.Contains(lowercaseLine, "traceback")
}

func (m *LogsModel) colorizeLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	b.Grow(len(lines) * estimatedLogLineBytes)
	for i, line := range lines {
		entry := m.renderedLine(line)
		b.WriteString(entry.rendered)
		if i < len(lines)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (m *LogsModel) renderedLine(line string) renderedLogLine {
	if m.colorizeCache == nil {
		m.colorizeCache = make(map[string]renderedLogLine)
	}
	if cached, ok := m.colorizeCache[line]; ok {
		return cached
	}
	if len(m.colorizeCache) >= maxColorizeCacheSize {
		m.colorizeCache = make(map[string]renderedLogLine)
	}
	sev := classifyLine(line)
	entry := renderedLogLine{severity: sev, rendered: applyLogSeverityStyle(line, sev)}
	m.colorizeCache[line] = entry
	return entry
}

func applyLogSeverityStyle(line string, sev lineSeverity) string {
	switch sev {
	case sevCritical:
		return theme.LogGutterError.Render("▌") + theme.LogCritical.Render(line)
	case sevError:
		return theme.LogGutterError.Render("▌") + theme.LogError.Render(line)
	case sevWarn:
		return theme.LogGutterWarn.Render("▌") + theme.LogWarn.Render(line)
	case sevStack:
		return "  " + theme.LogStack.Render(line)
	case sevDebug:
		return "  " + theme.LogDebug.Render(line)
	case sevNone:
		return "  " + line
	}
	return "  " + line
}

func (m *LogsModel) resetColorizeCache() {
	m.colorizeCache = nil
}

func (m LogsModel) findPodIndex(name string) int {
	namespace := m.selectedPodNS()
	for index, pod := range m.pods {
		if pod.Name == name && namespacesMatch(namespace, pod.Namespace) {
			return index
		}
	}
	return 0
}
