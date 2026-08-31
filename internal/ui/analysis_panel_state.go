package ui

func (m *AnalysisPanelModel) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.recalcViewport()
}

func (m AnalysisPanelModel) IsVisible() bool {
	return m.visible
}

func (m *AnalysisPanelModel) SetVisible(visible bool) {
	m.visible = visible
	if visible {
		m.input.Focus()
		return
	}
	if m.loading || m.streaming {
		m.requestID++
	}
	m.endStream()
}

func (m *AnalysisPanelModel) endStream() {
	if m.streamCancel != nil {
		m.streamCancel()
		m.streamCancel = nil
	}
	m.streaming = false
	m.loading = false
	m.streamChan = nil
	m.streamRaw = ""
	m.streamBuffer = ""
}

func (m *AnalysisPanelModel) SetNamespace(namespace string) {
	m.namespace = namespace
}

func (m *AnalysisPanelModel) Focus() {
	m.input.Focus()
}

func (m *AnalysisPanelModel) Blur() {
	m.input.Blur()
}

func (m *AnalysisPanelModel) SetContext(screenContext string) {
	m.response = sanitizeTerminalText(screenContext)
	m.renderedResponse = m.renderMarkdown(m.response)
	m.responseView.SetContent(m.renderedResponse)
	m.responseView.GotoTop()
}

func (m *AnalysisPanelModel) SetScreenContext(ctx string) {
	m.screenContext = sanitizeTerminalText(ctx)
}

func (m *AnalysisPanelModel) RefreshProviderName() {
	m.providerName = sanitizeTerminalText(m.providerNameOf())
}
