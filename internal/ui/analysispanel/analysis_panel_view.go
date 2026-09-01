package analysispanel

import (
	"encoding/json"
	"strings"

	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"
	"github.com/HediAbed/opsmate/internal/terminal"
	"github.com/HediAbed/opsmate/internal/ui/component"
	"github.com/HediAbed/opsmate/internal/ui/theme"
)

func (m AnalysisPanelModel) View() string {
	if !m.visible || m.width == 0 || m.height == 0 {
		return ""
	}

	innerWidth := m.innerWidth()
	innerHeight := m.innerHeight()
	body := lipgloss.JoinVertical(lipgloss.Left,
		m.titleLine(innerWidth),
		m.responseContent(innerWidth, innerHeight),
		m.inputBar(innerWidth),
		m.helpView(),
	)
	style := analysisPanelBoxStyle()
	return component.NewPanel(style).Render(
		component.Size{
			Width:  innerWidth + style.GetHorizontalFrameSize(),
			Height: innerHeight + style.GetVerticalFrameSize(),
		},
		body,
	)
}

func (m AnalysisPanelModel) titleLine(width int) string {
	var providerTag string
	if m.providerName != "" && m.providerName != "None" {
		providerTag = theme.Subtitle.Render("[" + m.providerName + "]")
	} else {
		providerTag = theme.Error.Render("[No provider]")
	}
	titleText := theme.AnalysisAccent.Render(" Cluster Analysis ") + " " + providerTag
	if m.streaming {
		titleText += " " + m.spinner.View() + " " + theme.Accent.Render("streaming…")
	} else if m.loading {
		titleText += " " + m.spinner.View() + " " + theme.Dim.Render("thinking…")
	}
	padLen := max(0, width-lipgloss.Width(titleText))
	titleLine := titleText + theme.Dim.Render(strings.Repeat("─", padLen))
	return titleLine
}

func (m AnalysisPanelModel) inputBar(width int) string {
	input := m.input
	input.SetWidth(max(analysisPanelMinimumContentWidth, width-analysisPanelInputChromeWidth))
	return input.View()
}

func (m AnalysisPanelModel) responseContent(width, height int) string {
	contentHeight := max(1, height-analysisPanelFixedContentRows)
	responseView := m.responseView
	responseView.SetWidth(width)
	responseView.SetHeight(contentHeight)
	return responseView.View()
}

func analysisPanelBoxStyle() lipgloss.Style {
	return theme.BoxStyle.BorderForeground(theme.ElectricPurp)
}

func (m AnalysisPanelModel) helpView() string {
	separator := theme.Dim.Render(" | ")
	parts := []string{
		theme.HelpKey.Render("enter") + theme.HelpDesc.Render(": send"),
		theme.HelpKey.Render("!cmd") + theme.HelpDesc.Render(": suggest command"),
		theme.HelpKey.Render("esc") + theme.HelpDesc.Render(": close"),
	}
	if m.innerWidth() >= analysisPanelExpandedHelpMinimumWidth {
		parts = append(parts[:analysisPanelHelpInsertionIndex], append([]string{theme.HelpKey.Render("ctrl+l") + theme.HelpDesc.Render(": clear")}, parts[analysisPanelHelpInsertionIndex:]...)...)
	}
	if m.lastFailedEntry() != nil {
		retryPart := theme.HelpKey.Render("R") + theme.HelpDesc.Render(": retry")
		parts = append([]string{parts[0], retryPart}, parts[1:]...)
	}
	return strings.Join(parts, separator)
}

func (m *AnalysisPanelModel) renderMarkdown(markdown string) string {
	markdown = terminal.SanitizeText(markdown)
	if markdown == "" {
		return ""
	}
	width := m.innerWidth()
	if width < analysisPanelMarkdownFallbackThreshold {
		width = analysisPanelMarkdownFallbackWidth
	}
	return m.markdownAt(markdown, width-analysisPanelMarkdownHorizontalChrome)
}

func (m *AnalysisPanelModel) markdownAt(markdown string, wrapWidth int) string {
	return m.markdownAtWithFactory(markdown, wrapWidth, newMarkdownRenderer)
}

func newMarkdownRenderer(wrapWidth int) (markdownRenderer, error) {
	return glamour.NewTermRenderer(
		glamour.WithStylePath("dark"),
		glamour.WithWordWrap(wrapWidth),
	)
}

func (m *AnalysisPanelModel) markdownAtWithFactory(
	markdown string,
	wrapWidth int,
	createRenderer markdownRendererFactory,
) (output string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			output = markdown
		}
	}()
	if wrapWidth < analysisPanelMinimumMarkdownWrap {
		wrapWidth = analysisPanelMinimumMarkdownWrap
	}
	if m.glamourRenderer == nil || m.glamourWrap != wrapWidth {
		renderer, err := createRenderer(wrapWidth)
		if err != nil {
			return markdown
		}
		m.glamourRenderer = renderer
		m.glamourWrap = wrapWidth
	}
	rendered, err := m.glamourRenderer.Render(markdown)
	if err != nil {
		return markdown
	}
	return sanitizeRendered(strings.TrimSpace(rendered))
}

func sanitizeRendered(value string) string {
	var sanitized strings.Builder
	sanitized.Grow(len(value))
	for _, character := range value {
		switch {
		case character == '\n' || character == '\t' || character == 0x1b:
			sanitized.WriteRune(character)
		case character < 0x20 || (character >= 0x7f && character <= 0x9f):
			continue
		default:
			sanitized.WriteRune(character)
		}
	}
	return sanitized.String()
}

func (m *AnalysisPanelModel) recalcViewport() {
	iw := m.innerWidth()
	vpHeight := max(analysisPanelMinimumViewportHeight, m.innerHeight()-analysisPanelFixedContentRows)

	m.responseView.SetWidth(max(analysisPanelMinimumContentWidth, iw))
	m.responseView.SetHeight(vpHeight)

	if iw != m.lastRenderWidth {
		m.lastRenderWidth = iw
		m.rebuildChatContent()
	}
}

func (m *AnalysisPanelModel) rebuildChatContent() {
	innerWidth := max(analysisPanelMinimumContentWidth, m.innerWidth())
	if len(m.history) == 0 && !m.loading {
		m.responseView.SetContent(renderChatWelcome(innerWidth))
		return
	}
	sections := m.renderChatSections(innerWidth)
	if m.loading {
		sections = append(sections, m.renderThinkingIndicator())
	}
	m.responseView.SetContent(strings.Join(sections, "\n"))
}

func renderChatWelcome(width int) string {
	return lipgloss.NewStyle().
		Foreground(theme.DimText).
		Width(width).
		Align(lipgloss.Center).
		Render("\n\nCluster Analysis\n\n" +
			"Ask questions about your K8s resources\n" +
			"or use !command to generate kubectl commands.\n\n" +
			"Examples:\n" +
			"  \"Why is my pod crashing?\"\n" +
			"  \"!show recent warning events\"\n" +
			"  \"Explain the events for this pod\"\n")
}

func (m *AnalysisPanelModel) renderChatSections(innerWidth int) []string {
	userBubble := lipgloss.NewStyle().
		Foreground(theme.White).
		Background(theme.DeepViolet).
		Padding(0, 1).
		MarginTop(1).
		Bold(true)

	providerResponseLabel := lipgloss.NewStyle().
		Foreground(theme.NeonCyan).
		Bold(true)

	errStyle := lipgloss.NewStyle().
		Foreground(theme.Red)

	maxBubbleW := max(analysisPanelMinimumContentWidth, innerWidth-analysisPanelBubbleHorizontalChrome)
	sections := make([]string, 0, len(m.history)*analysisPanelHistorySectionCapacity)
	wrap := maxBubbleW - analysisPanelBubbleTextChrome
	lastIdx := len(m.history) - 1
	providerResponseHeader := providerResponseLabel.Render("ANALYSIS")

	for index := range m.history {
		entry := &m.history[index]
		sections = append(sections, renderQueryBubble(entry.Query, userBubble, innerWidth, maxBubbleW))
		if entry.Response == "" {
			continue
		}
		inFlight := m.streaming && index == lastIdx
		body := m.renderResponseBubble(entry, wrap, maxBubbleW, inFlight, errStyle)
		sections = append(sections, providerResponseHeader, body)
	}
	return sections
}

func (m *AnalysisPanelModel) renderThinkingIndicator() string {
	thinkingQuery := ""
	if len(m.history) > 0 {
		thinkingQuery = m.history[len(m.history)-1].Query
		thinkingQuery = terminal.TruncateRunes(thinkingQuery, analysisPanelThinkingQueryRunes, "…")
		thinkingQuery = ": " + theme.Dim.Render("\""+thinkingQuery+"\"")
	}
	return "\n" + m.spinner.View() + " " + theme.AnalysisAccent.Render("Thinking...") + thinkingQuery
}

func renderQueryBubble(query string, bubble lipgloss.Style, iw, maxBubbleW int) string {
	query = terminal.TruncateRunes(query, maxBubbleW, "…")
	msg := bubble.MaxWidth(maxBubbleW).Render("▸ " + query)
	return lipgloss.NewStyle().Width(iw).Align(lipgloss.Right).Render(msg)
}

func (m *AnalysisPanelModel) renderResponseBubble(entry *historyEntry, wrap, maxBubbleW int, inFlight bool, errStyle lipgloss.Style) string {
	if strings.HasPrefix(entry.Response, "Error:") || strings.HasPrefix(entry.Response, "Command failed:") {
		return errStyle.Width(maxBubbleW).Render(entry.Response)
	}
	if entry.rendered != "" && entry.renderedWrap == wrap && entry.renderedFrom == entry.Response {
		return entry.rendered
	}
	rendered := m.markdownAt(entry.Response, wrap)
	if !inFlight {
		entry.rendered = rendered
		entry.renderedFrom = entry.Response
		entry.renderedWrap = wrap
	}
	return rendered
}

func (m AnalysisPanelModel) innerWidth() int {
	inner := component.NewPanel(analysisPanelBoxStyle()).ContentSize(component.Size{
		Width:  m.width - analysisPanelOuterBorderChrome,
		Height: m.height,
	})
	return max(analysisPanelMinimumContentWidth, inner.Width)
}

func (m AnalysisPanelModel) innerHeight() int {
	return max(analysisPanelMinimumInnerHeight, m.height-analysisPanelOuterBorderChrome)
}

func (m *AnalysisPanelModel) setLastQuery(query string) {
	m.history = append(m.history, historyEntry{Query: terminal.SanitizeText(query)})
}

func (m *AnalysisPanelModel) clearChat() {
	if m.loading || m.streaming {
		m.requestID++
	}
	m.endStream()
	m.history = m.history[:0]
	m.response = ""
	m.renderedResponse = ""
	m.streamBuffer = ""
	m.err = nil
	m.input.SetValue("")
	m.rebuildChatContent()
	m.responseView.GotoTop()
}

func (AnalysisPanelModel) analysisSystemPrompt() string {
	return analysisSystemInstructions
}

func (m AnalysisPanelModel) analysisUserMessage(question string) string {
	payload, _ := json.Marshal(analysisPayload{
		Question:           question,
		ScreenContext:      m.screenContext,
		ConversationMemory: m.recentConversationMemory(),
	})
	return analysisPayloadNotice + string(payload)
}

func (m AnalysisPanelModel) recentConversationMemory() string {
	lastCompleted := len(m.history) - 1
	if lastCompleted <= 0 {
		return ""
	}
	start := max(0, lastCompleted-maxMemoryTurns)
	var b strings.Builder
	for index := start; index < lastCompleted; index++ {
		entry := m.history[index]
		query := strings.TrimSpace(entry.Query)
		response := strings.TrimSpace(entry.Response)
		if query == "" || response == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("User: ")
		b.WriteString(limitMemoryText(query, memoryQueryCharacterLimit))
		b.WriteString("\nAnalysis: ")
		b.WriteString(limitMemoryText(response, memoryResponseCharacterLimit))
		if b.Len() >= maxMemoryChars {
			return limitMemoryText(b.String(), maxMemoryChars)
		}
	}
	return b.String()
}

func limitMemoryText(value string, maximumLength int) string {
	value = strings.TrimSpace(value)
	return terminal.TruncateRunes(value, maximumLength, "\n... (truncated)")
}
