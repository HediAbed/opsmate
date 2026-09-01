package helm

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/HediAbed/opsmate/internal/kube"
	"github.com/HediAbed/opsmate/internal/terminal"
	clusterui "github.com/HediAbed/opsmate/internal/ui/cluster"
	"github.com/HediAbed/opsmate/internal/ui/component"
	"github.com/HediAbed/opsmate/internal/ui/theme"
)

type HelmModel struct {
	width     int
	height    int
	namespace string
	releases  []kube.HelmRelease

	releaseTable table.Model
	spinner      spinner.Model

	loading   bool
	err       error
	statusMsg string

	valuesPopupVisible bool
	valuesPopupRelease string
	valuesPopupNS      string
	valuesPopupLoading bool
	valuesPopupErr     error
	valuesPopupView    viewport.Model
	releasesRequestID  uint64
	valuesRequestID    uint64
	commands           clusterui.HelmCommands
}

type helmResultKind uint8

const (
	helmReleasesResult helmResultKind = iota
	helmValuesResult
)

type helmResultMsg struct {
	kind      helmResultKind
	requestID uint64
	namespace string
	release   string
	payload   tea.Msg
}

const (
	valuesPopupSideMargin      = 4
	valuesPopupTopBottomMargin = 2
	valuesPopupMinWidth        = 40
	valuesPopupMinHeight       = 10
	popupChromeW               = 2
	popupChromeH               = 4
	loadingValuesText          = "loading values…"
	helmUpdatedTimeLayout      = "2006-01-02 15:04 MST"
	initialTableWidth          = 80
	pairedSides                = 2
	popupHardGutter            = 1
	popupMinimumContentSize    = 1
)

var helmColSpecs = []component.ColumnSpec{
	{Title: "NAME", Flex: component.ColumnFlexSecondary, Min: component.ColumnMinimumWide},
	{Title: "NAMESPACE", Flex: component.ColumnFlexModest, Min: component.ColumnMinimumStandard},
	{Title: "REVISION", Width: component.ColumnWidthStandard},
	{Title: "STATUS", Width: component.ColumnWidthWide},
	{Title: "CHART", Flex: component.ColumnFlexSecondary, Min: component.ColumnMinimumWide},
	{Title: "APP VERSION", Flex: component.ColumnFlexMinimal, Min: component.ColumnMinimumCompact},
	{Title: "UPDATED", Flex: component.ColumnFlexMinimal, Min: component.ColumnMinimumReadable},
}

func NewHelmModel(namespace string, commands clusterui.HelmCommands) HelmModel {
	loadingSpinner := spinner.New()
	loadingSpinner.Spinner = spinner.Dot
	loadingSpinner.Style = theme.SpinnerStyle

	return HelmModel{
		namespace:       namespace,
		releaseTable:    component.NewTable(initialTableWidth, helmColSpecs),
		spinner:         loadingSpinner,
		loading:         true,
		valuesPopupView: viewport.New(),
		commands:        commands,
	}
}

func (m *HelmModel) Init() tea.Cmd {
	return tea.Batch(m.fetchReleases(), m.spinner.Tick)
}

func (m *HelmModel) Activate() tea.Cmd {
	m.loading = true
	return tea.Batch(m.fetchReleases(), m.spinner.Tick)
}

func (m *HelmModel) Deactivate() {
	m.loading = false
	m.valuesPopupVisible = false
	m.valuesRequestID++
}

func (m *HelmModel) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.releaseTable.SetWidth(width)
	m.releaseTable.SetColumns(component.Columns(width, helmColSpecs))
	m.syncValuesPopupLayout()
}

func (m *HelmModel) SetNamespace(namespace string) tea.Cmd {
	if namespace == m.namespace {
		return nil
	}
	m.namespace = namespace
	m.releases = nil
	m.loading = true
	m.releasesRequestID++
	m.valuesRequestID++
	return m.fetchReleases()
}

func (HelmModel) HasInputFocus() bool { return false }

func (m HelmModel) Size() (int, int) {
	return m.width, m.height
}

func (m HelmModel) Loading() bool {
	return m.loading
}

func (HelmModel) Accepts(msg tea.Msg) bool {
	_, accepted := msg.(helmResultMsg)
	return accepted
}

func (m HelmModel) AnalysisOverlayBounds(totalHeight int) (topOffset, panelHeight, bottomOffset int) {
	top := lipgloss.Height(m.renderTitleBar())
	bottom := lipgloss.Height(m.renderHelpBar())
	if banner := m.renderErrBanner(); banner != "" {
		bottom += lipgloss.Height(banner)
	}
	return component.AnalysisOverlayBounds(totalHeight, top, bottom)
}

func (m HelmModel) tableFirstRowY() int {
	firstRow := lipgloss.Height(m.renderTitleBar())
	if banner := m.renderErrBanner(); banner != "" {
		firstRow += lipgloss.Height(banner)
	}
	return firstRow + component.TableHeaderRows
}

func (m HelmModel) Update(msg tea.Msg) (HelmModel, tea.Cmd) {
	switch msg := msg.(type) {
	case helmResultMsg:
		return m.handleHelmResult(msg)

	case clusterui.HelmReleasesMsg:
		m.applyHelmReleases(msg)
		return m, nil

	case clusterui.HelmValuesMsg:
		return m.applyHelmValues(msg), nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case tea.MouseClickMsg:
		m.handleHelmMouseClick(msg)
		return m, nil

	case tea.MouseWheelMsg:
		return m.handleHelmMouseWheel(msg)
	}
	return m, nil
}

func (m HelmModel) handleHelmResult(msg helmResultMsg) (HelmModel, tea.Cmd) {
	if !m.acceptsResult(msg) {
		return m, nil
	}
	return m.Update(msg.payload)
}

func (m *HelmModel) applyHelmReleases(msg clusterui.HelmReleasesMsg) {
	m.loading = false
	if msg.Err != nil {
		m.err = msg.Err
		m.releases = nil
		m.releaseTable.SetRows(nil)
		return
	}
	m.err = nil
	m.releases = msg.Releases
	m.releaseTable.SetRows(m.currentRows())
	m.statusMsg = theme.Success.Render(fmt.Sprintf("Loaded %d %s", len(msg.Releases), component.NounForCount("release", "releases", len(msg.Releases))))
}

func (m *HelmModel) handleHelmMouseClick(msg tea.MouseClickMsg) {
	if m.valuesPopupVisible || msg.Button != tea.MouseLeft {
		return
	}
	rowIndex := msg.Y - m.tableFirstRowY()
	if rowIndex >= 0 && rowIndex < len(m.releaseTable.Rows()) {
		m.releaseTable.SetCursor(rowIndex)
	}
}

func (m HelmModel) handleHelmMouseWheel(msg tea.MouseWheelMsg) (HelmModel, tea.Cmd) {
	if m.valuesPopupVisible {
		var command tea.Cmd
		m.valuesPopupView, command = m.valuesPopupView.Update(msg)
		return m, command
	}
	switch msg.Button {
	case tea.MouseWheelUp:
		m.releaseTable.MoveUp(component.TableWheelStep)
	case tea.MouseWheelDown:
		m.releaseTable.MoveDown(component.TableWheelStep)
	}
	return m, nil
}

func (m HelmModel) handleKey(msg tea.KeyPressMsg) (HelmModel, tea.Cmd) {
	key := msg.String()
	if m.valuesPopupVisible {
		if key == "esc" || key == "q" {
			m.valuesPopupVisible = false
			return m, nil
		}
		var cmd tea.Cmd
		m.valuesPopupView, cmd = m.valuesPopupView.Update(msg)
		return m, cmd
	}
	switch key {
	case "r":
		m.loading = true
		return m, tea.Batch(m.fetchReleases(), m.spinner.Tick)
	case "v":
		return m.openValuesPopup()
	}
	var cmd tea.Cmd
	m.releaseTable, cmd = m.releaseTable.Update(msg)
	return m, cmd
}

func (m HelmModel) openValuesPopup() (HelmModel, tea.Cmd) {
	rel := m.SelectedRelease()
	if rel.Name == "" {
		return m, nil
	}
	m.valuesPopupVisible = true
	m.valuesPopupRelease = rel.Name
	m.valuesPopupNS = rel.Namespace
	m.valuesPopupLoading = true
	m.valuesPopupErr = nil
	m.valuesPopupView.SetContent(loadingValuesText)
	m.syncValuesPopupLayout()
	return m, m.fetchValues(rel.Name, rel.Namespace)
}

func (m HelmModel) applyHelmValues(msg clusterui.HelmValuesMsg) HelmModel {
	if !m.valuesPopupVisible {
		return m
	}
	if msg.Release != m.valuesPopupRelease || msg.Namespace != m.valuesPopupNS {
		return m
	}
	m.valuesPopupLoading = false
	if msg.Err != nil {
		m.valuesPopupErr = msg.Err
		m.valuesPopupView.SetContent(theme.ErrorBanner.Render(" " + terminal.SanitizeLine(msg.Err.Error()) + " "))
		return m
	}
	content := strings.TrimSpace(terminal.SanitizeText(msg.Values))
	if content == "" {
		content = theme.Dim.Render("(no user-supplied values; this release uses chart defaults)")
	}
	m.valuesPopupView.SetContent(content)
	m.valuesPopupView.GotoTop()
	return m
}

func (m *HelmModel) fetchReleases() tea.Cmd {
	m.releasesRequestID++
	requestID := m.releasesRequestID
	namespace := m.namespace
	command := m.commands.ListReleases(namespace)
	return func() tea.Msg {
		return helmResultMsg{kind: helmReleasesResult, requestID: requestID, namespace: namespace, payload: command()}
	}
}

func (m *HelmModel) fetchValues(release, namespace string) tea.Cmd {
	m.valuesRequestID++
	requestID := m.valuesRequestID
	command := m.commands.GetValues(kube.HelmReleaseReference{Namespace: namespace, Name: release})
	return func() tea.Msg {
		return helmResultMsg{
			kind:      helmValuesResult,
			requestID: requestID,
			namespace: namespace,
			release:   release,
			payload:   command(),
		}
	}
}

func (m HelmModel) acceptsResult(msg helmResultMsg) bool {
	switch msg.kind {
	case helmReleasesResult:
		return msg.requestID == m.releasesRequestID && msg.namespace == m.namespace
	case helmValuesResult:
		return msg.requestID == m.valuesRequestID &&
			msg.namespace == m.valuesPopupNS &&
			msg.release == m.valuesPopupRelease
	default:
		return false
	}
}

func (m HelmModel) View() string {
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
	base := lipgloss.JoinVertical(lipgloss.Left, sections...)

	if m.valuesPopupVisible {
		popup := m.renderValuesPopup()
		return overlayValuesPopup(base, popup, m.width, m.height)
	}
	return base
}

func (m HelmModel) renderValuesPopup() string {
	title := theme.Title.Render(fmt.Sprintf(" VALUES · %s ", m.valuesPopupRelease))
	if m.valuesPopupLoading {
		title += theme.Dim.Render("  loading…")
	}
	footer := theme.Dim.Render(" ↑↓ scroll · esc close ") + component.PopupScrollIndicator(m.valuesPopupView)

	body := m.valuesPopupView.View()
	stacked := lipgloss.JoinVertical(lipgloss.Left, title, body, footer)
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.HotPink).
		Padding(0, 1).
		Render(stacked)
}

func (m *HelmModel) syncValuesPopupLayout() {
	popupW := m.width - valuesPopupSideMargin*pairedSides
	if popupW < valuesPopupMinWidth {
		popupW = valuesPopupMinWidth
	}
	if popupW > m.width-pairedSides*popupHardGutter {
		popupW = m.width - pairedSides*popupHardGutter
	}
	popupH := m.height - valuesPopupTopBottomMargin*pairedSides
	if popupH < valuesPopupMinHeight {
		popupH = valuesPopupMinHeight
	}
	if popupH > m.height-pairedSides*popupHardGutter {
		popupH = m.height - pairedSides*popupHardGutter
	}

	innerW := popupW - popupChromeW
	innerH := popupH - popupChromeH
	if innerW < popupMinimumContentSize {
		innerW = popupMinimumContentSize
	}
	if innerH < popupMinimumContentSize {
		innerH = popupMinimumContentSize
	}
	m.valuesPopupView.SetWidth(innerW)
	m.valuesPopupView.SetHeight(innerH)
}

func overlayValuesPopup(base, popup string, width, height int) string {
	popupW := lipgloss.Width(popup)
	popupH := lipgloss.Height(popup)
	horizontalOffset := (width - popupW) / pairedSides
	if horizontalOffset < 0 {
		horizontalOffset = 0
	}
	verticalOffset := (height - popupH) / pairedSides
	if verticalOffset < 0 {
		verticalOffset = 0
	}
	root := lipgloss.NewLayer(base).Z(0)
	top := lipgloss.NewLayer(popup).X(horizontalOffset).Y(verticalOffset).Z(1)
	root.AddLayers(top)
	return lipgloss.NewCompositor(root).Render()
}

func (m HelmModel) renderTitleBar() string {
	title := theme.Title.Render("HELM RELEASES")
	scope := m.namespace
	if scope == "" {
		scope = "all namespaces"
	}
	bar := " " + title + "  ns:" + scope + " "
	if m.statusMsg != "" {
		bar += "  " + m.statusMsg
	}
	if cursor := component.TableCursorLabel(m.releaseTable.Cursor(), len(m.releases)); cursor != "" {
		bar += "  " + theme.Dim.Render(cursor)
	}
	return theme.StatusBarItem.Width(m.width).MaxWidth(m.width).Render(bar)
}

func (m HelmModel) renderHelpBar() string {
	hints := " r:refresh  ↑/↓:navigate  v:values  q:quit "
	if m.valuesPopupVisible {
		hints = " ↑/↓:scroll  esc:close  q:close "
	}
	return lipgloss.NewStyle().Foreground(theme.NeonCyan).Width(m.width).MaxWidth(m.width).Render(hints)
}

func (m HelmModel) renderErrBanner() string {
	if m.err == nil {
		return ""
	}
	return theme.ErrorBanner.Width(m.width).MaxWidth(m.width).Render(" " + terminal.SanitizeLine(m.err.Error()) + " ")
}

func (m HelmModel) renderBody() string {
	if m.loading {
		return m.spinner.View() + " loading helm releases..."
	}
	if m.err != nil && len(m.releases) == 0 {
		return ""
	}
	if len(m.releases) == 0 {
		return theme.Dim.Render("No helm releases found.")
	}
	return m.releaseTable.View()
}

func (m HelmModel) currentRows() []table.Row {
	rows := make([]table.Row, 0, len(m.releases))
	for _, release := range m.releases {
		rows = append(rows, table.Row{
			release.Name,
			release.Namespace,
			strconv.Itoa(release.Revision),
			release.Status,
			release.ChartLabel(),
			release.AppVersion,
			formatHelmReleaseTime(release.UpdatedAt),
		})
	}
	return rows
}

func (m HelmModel) SelectedRelease() kube.HelmRelease {
	row := m.releaseTable.SelectedRow()
	if len(row) == 0 || len(m.releases) == 0 {
		return kube.HelmRelease{}
	}
	for _, rel := range m.releases {
		if rel.Name == row[0] && rel.Namespace == row[1] {
			return rel
		}
	}
	return kube.HelmRelease{}
}

func formatHelmReleaseTime(updatedAt time.Time) string {
	if updatedAt.IsZero() {
		return "-"
	}
	return updatedAt.Format(helmUpdatedTimeLayout)
}
