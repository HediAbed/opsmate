package browser

import (
	"strings"

	"charm.land/bubbles/v2/table"
	"charm.land/lipgloss/v2"
	"github.com/HediAbed/opsmate/internal/ui/component"
	"github.com/HediAbed/opsmate/internal/ui/theme"
)

func (m *BrowserModel) recalcLayout() {
	if m.width == 0 {
		return
	}
	m.rebuildTable()
	m.syncBrowserLayout()
}

func (m BrowserModel) browserContentHeight() int {
	chromeHeight := lipgloss.Height(m.renderTitleBar()) +
		lipgloss.Height(m.renderHelpBar()) +
		lipgloss.Height(m.renderStatusLine())
	if filterBar := m.renderFilterBar(); filterBar != "" {
		chromeHeight += lipgloss.Height(filterBar)
	}
	if errBanner := m.renderErrBanner(); errBanner != "" {
		chromeHeight += lipgloss.Height(errBanner)
	}
	return max(1, m.height-chromeHeight)
}

func (m *BrowserModel) syncBrowserLayout() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	height := m.browserContentHeight()
	switch {
	case m.state == stateShell:
		m.syncShellLayout(height)
	case m.showDetail && m.splitHorizontal && m.width >= narrowHorizontalSplitMinimum:
		m.syncHorizontalDetailLayout(height)
	case m.showDetail:
		m.syncVerticalDetailLayout(height)
	default:
		m.syncFullTableLayout(height)
	}
}

func (m *BrowserModel) syncFullTableLayout(height int) {
	contentWidth := max(1, m.width-browserContentHorizontalChrome)
	if specs, ok := selectColSpecs(m.resourceType, m.wide); ok {
		m.resourceTable.SetColumns(computeColumns(contentWidth, specs))
	}
	m.resourceTable.SetWidth(contentWidth)
	m.resourceTable.SetHeight(max(browserMinimumTableHeight, height-browserPanelGutter))
}

func (m *BrowserModel) syncVerticalDetailLayout(height int) {
	topHeight := max(browserVerticalTableMinHeight, height/pairedSides)
	m.syncFullTableLayout(topHeight)
	detailHeight := max(browserDetailMinimumHeight, height-topHeight-browserVerticalDetailChrome)
	m.detailView.SetWidth(max(1, m.width-browserContentHorizontalChrome))
	m.detailView.SetHeight(detailHeight)
}

func (m *BrowserModel) syncHorizontalDetailLayout(height int) {
	leftWidth, rightWidth := browserHorizontalPaneWidths(m.width)
	tableArea := component.NewPanel(theme.BoxStyle).ContentSize(component.Size{Width: leftWidth - browserPanelGutter, Height: height})
	tableWidth := max(1, tableArea.Width)
	if specs, ok := selectColSpecs(m.resourceType, m.wide); ok {
		m.resourceTable.SetColumns(computeColumns(tableWidth, specs))
	}
	m.resourceTable.SetWidth(tableWidth)
	m.resourceTable.SetHeight(max(browserMinimumTableHeight, height-browserPanelGutter))
	m.detailView.SetWidth(max(1, rightWidth-browserContentHorizontalChrome))
	m.detailView.SetHeight(max(browserDetailMinimumHeight, height-browserVerticalDetailChrome))
}

func browserHorizontalPaneWidths(totalWidth int) (int, int) {
	leftWidth := max(browserHorizontalTableMinWidth, totalWidth*browserHorizontalTablePercent/percentageScale)
	return leftWidth, totalWidth - leftWidth - browserHorizontalPaneGap
}

func (m *BrowserModel) rebuildDetailContent() {
	var parts []string

	if m.analysisSummaryLoading {
		parts = append(parts, theme.SpinnerStyle.Render("Analyzing..."))
		parts = append(parts, "")
	} else if m.analysisSummaryErr != nil {
		parts = append(parts, theme.Error.Render(analysisErrorText(m.analysisSummaryErr)))
		parts = append(parts, "")
	} else if m.analysisSummary != "" {
		header := theme.AnalysisAccent.Render("ANALYSIS ") + theme.Accent.Render("SUMMARY")
		summary := lipgloss.NewStyle().Foreground(theme.LightText).Render(m.analysisSummary)
		border := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).BorderForeground(theme.ElectricPurp).
			Padding(0, 1).Render(header + "\n" + summary)
		parts = append(parts, border)
		parts = append(parts, "")
	}

	body := m.detailContent
	if m.detailKind == "yaml" {
		body = component.HighlightYAML(body)
	}
	parts = append(parts, body)
	m.detailView.SetContent(strings.Join(parts, "\n"))
	m.detailView.GotoTop()
	m.statusMsg = theme.Dim.Render("esc: back | a: analysis | c: copy | v: split")
}

func (m *BrowserModel) rebuildTable() {
	tableWidth := m.width - browserContentHorizontalChrome
	if tableWidth < browserMinimumResourceTableWidth {
		tableWidth = browserMinimumResourceTableWidth
	}

	specs, ok := selectColSpecs(m.resourceType, m.wide)
	if !ok {
		return
	}
	prevCursor := m.resourceTable.Cursor()
	m.resourceTable = buildResourceTable(tableWidth, specs)
	m.detailView.SetWidth(m.width - browserContentHorizontalChrome)
	m.refreshRows(prevCursor)
}

func (m *BrowserModel) refreshRows(cursorHint int) {
	rows := m.currentResourceRows()
	identities := m.currentResourceIdentities()
	rows, identities = filterRowsWithIdentities(rows, identities, m.filterText)
	m.visibleResources = identities
	m.resourceTable.SetRows(rows)

	if cursorHint >= len(rows) && len(rows) > 0 {
		cursorHint = len(rows) - 1
	}
	if len(rows) > 0 {
		m.resourceTable.SetCursor(cursorHint)
	}
	m.resourceTable.Focus()
}

func filterRowsWithIdentities(
	rows []table.Row,
	identities []resourceIdentity,
	filterText string,
) ([]table.Row, []resourceIdentity) {
	if filterText == "" {
		return rows, identities
	}
	filteredRows := make([]table.Row, 0, len(rows))
	filteredIdentities := make([]resourceIdentity, 0, len(rows))
	for index, row := range rows {
		if !rowMatchesFilter(row, filterText) {
			continue
		}
		filteredRows = append(filteredRows, row)
		if index < len(identities) {
			filteredIdentities = append(filteredIdentities, identities[index])
		}
	}
	return filteredRows, filteredIdentities
}

func rowMatchesFilter(row table.Row, filterText string) bool {
	lowerFilter := strings.ToLower(filterText)
	for _, cell := range row {
		if strings.Contains(strings.ToLower(cell), lowerFilter) {
			return true
		}
	}
	return false
}

func (m *BrowserModel) currentResourceRows() []table.Row {
	b, ok := resourceCatalog[m.resourceType]
	if !ok {
		return nil
	}
	if m.wide && b.WideRowsOf != nil {
		return b.WideRowsOf(m)
	}
	return b.RowsOf(m)
}

func (m *BrowserModel) currentResourceIdentities() []resourceIdentity {
	binding, ok := resourceCatalog[m.resourceType]
	if !ok || binding.IdentitiesOf == nil {
		return nil
	}
	identities := binding.IdentitiesOf(m)
	for index := range identities {
		if identities[index].Namespace == "" && identities[index].Kind != resourceKindNode {
			identities[index].Namespace = m.namespace
		}
	}
	return identities
}
