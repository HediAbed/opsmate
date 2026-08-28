package model

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/HediAbed/opsmate/internal/service"
	"github.com/HediAbed/opsmate/internal/theme"
)

// HelmModel lists releases and displays configured values.
type HelmModel struct {
	width     int
	height    int
	namespace string
	releases  []service.HelmRelease

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
)

var helmColSpecs = []colSpec{
	{Title: "NAME", Flex: columnFlexSecondary, Min: columnMinimumWide},
	{Title: "NAMESPACE", Flex: columnFlexModest, Min: columnMinimumStandard},
	{Title: "REVISION", Width: columnWidthStandard},
	{Title: "STATUS", Width: columnWidthWide},
	{Title: "CHART", Flex: columnFlexSecondary, Min: columnMinimumWide},
	{Title: "APP VERSION", Flex: columnFlexMinimal, Min: columnMinimumCompact},
	{Title: "UPDATED", Flex: columnFlexMinimal, Min: columnMinimumReadable},
}

func NewHelmModel(namespace string) HelmModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = theme.SpinnerStyle

	return HelmModel{
		namespace:       namespace,
		releaseTable:    buildResourceTable(initialScreenWidth, helmColSpecs),
		spinner:         s,
		loading:         true,
		valuesPopupView: viewport.New(),
	}
}

func (m *HelmModel) Init() tea.Cmd {
	return tea.Batch(m.fetchReleases(), m.spinner.Tick)
}

// Activate refreshes the release list.
func (m *HelmModel) Activate() tea.Cmd {
	m.loading = true
	return tea.Batch(m.fetchReleases(), m.spinner.Tick)
}

// Deactivate implements the screen lifecycle contract.
func (m *HelmModel) Deactivate() {
	m.loading = false
	m.valuesPopupVisible = false
	m.valuesRequestID++
}

func (m *HelmModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.releaseTable.SetWidth(w)
	m.releaseTable.SetColumns(computeColumns(w, helmColSpecs))
	m.syncValuesPopupLayout()
}

// SetNamespace changes scope and refreshes the release list.
func (m *HelmModel) SetNamespace(ns string) tea.Cmd {
	if ns == m.namespace {
		return nil
	}
	m.namespace = ns
	m.releases = nil
	m.loading = true
	m.releasesRequestID++
	m.valuesRequestID++
	return m.fetchReleases()
}

// HasInputFocus reports whether Helm owns keyboard input.
func (HelmModel) HasInputFocus() bool { return false }

// AIOverlayBounds returns the table region available to the side panel.
func (m HelmModel) AIOverlayBounds(totalHeight int) (topOffset, panelHeight, bottomOffset int) {
	top := lipgloss.Height(m.renderTitleBar())
	bottom := lipgloss.Height(m.renderHelpBar())
	if banner := m.renderErrBanner(); banner != "" {
		bottom += lipgloss.Height(banner)
	}
	return aiOverlayBounds(totalHeight, top, bottom)
}

func (m HelmModel) tableFirstRowY() int {
	y := lipgloss.Height(m.renderTitleBar())
	if banner := m.renderErrBanner(); banner != "" {
		y += lipgloss.Height(banner)
	}
	return y + tableHeaderChromeRows
}

func (m HelmModel) Update(msg tea.Msg) (HelmModel, tea.Cmd) {
	switch msg := msg.(type) {
	case helmResultMsg:
		return m.handleHelmResult(msg)

	case service.HelmReleasesMsg:
		m.applyHelmReleases(msg)
		return m, nil

	case service.HelmValuesMsg:
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

func (m *HelmModel) applyHelmReleases(msg service.HelmReleasesMsg) {
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
	m.statusMsg = theme.Success.Render(fmt.Sprintf("Loaded %d %s", len(msg.Releases), displayKind("releases", len(msg.Releases))))
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
		m.releaseTable.MoveUp(tableWheelStep)
	case tea.MouseWheelDown:
		m.releaseTable.MoveDown(tableWheelStep)
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

func (m HelmModel) applyHelmValues(msg service.HelmValuesMsg) HelmModel {
	if !m.valuesPopupVisible {
		return m
	}
	if msg.Release != m.valuesPopupRelease || msg.Namespace != m.valuesPopupNS {
		return m
	}
	m.valuesPopupLoading = false
	if msg.Err != nil {
		m.valuesPopupErr = msg.Err
		m.valuesPopupView.SetContent(theme.ErrorBanner.Render(" " + sanitizeTerminalLine(msg.Err.Error()) + " "))
		return m
	}
	content := strings.TrimSpace(sanitizeTerminalText(msg.Values))
	if content == "" {
		content = theme.Dim.Render("(no user-supplied values — this release uses chart defaults)")
	}
	m.valuesPopupView.SetContent(content)
	m.valuesPopupView.GotoTop()
	return m
}

func (m *HelmModel) fetchReleases() tea.Cmd {
	m.releasesRequestID++
	requestID := m.releasesRequestID
	namespace := m.namespace
	command := service.ListHelmReleases(namespace)
	return func() tea.Msg {
		return helmResultMsg{kind: helmReleasesResult, requestID: requestID, namespace: namespace, payload: command()}
	}
}

func (m *HelmModel) fetchValues(release, namespace string) tea.Cmd {
	m.valuesRequestID++
	requestID := m.valuesRequestID
	command := service.GetHelmValues(release, namespace)
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
	footer := theme.Dim.Render(" ↑↓ scroll · esc close ") + popupScrollIndicator(m.valuesPopupView)

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
	if popupW > m.width-2*popupHardGutter {
		popupW = m.width - pairedSides*popupHardGutter
	}
	popupH := m.height - valuesPopupTopBottomMargin*pairedSides
	if popupH < valuesPopupMinHeight {
		popupH = valuesPopupMinHeight
	}
	if popupH > m.height-2*popupHardGutter {
		popupH = m.height - pairedSides*popupHardGutter
	}

	innerW := popupW - popupChromeW
	innerH := popupH - popupChromeH
	if innerH < 1 {
		innerH = 1
	}
	m.valuesPopupView.SetWidth(innerW)
	m.valuesPopupView.SetHeight(innerH)
}

func overlayValuesPopup(base, popup string, width, height int) string {
	popupW := lipgloss.Width(popup)
	popupH := lipgloss.Height(popup)
	x := (width - popupW) / pairedSides
	if x < 0 {
		x = 0
	}
	y := (height - popupH) / pairedSides
	if y < 0 {
		y = 0
	}
	root := lipgloss.NewLayer(base).Z(0)
	top := lipgloss.NewLayer(popup).X(x).Y(y).Z(1)
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
	if cursor := tableCursorLabel(m.releaseTable.Cursor(), len(m.releases)); cursor != "" {
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
	if errors.Is(m.err, service.ErrHelmBinaryMissing) {
		return helmBinaryMissingBanner(m.width)
	}
	return theme.ErrorBanner.Width(m.width).MaxWidth(m.width).Render(" " + sanitizeTerminalLine(m.err.Error()) + " ")
}

func helmBinaryMissingBanner(width int) string {
	msg := "helm CLI not found on PATH — install via your package manager (e.g. `pacman -S helm`) or https://helm.sh/docs/intro/install"
	return theme.ErrorBanner.Width(width).MaxWidth(width).Render(" " + msg + " ")
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
	for _, r := range m.releases {
		rows = append(rows, table.Row{
			r.Name,
			r.Namespace,
			strconv.Itoa(r.Revision),
			r.Status,
			r.Chart,
			r.AppVersion,
			r.Updated,
		})
	}
	return rows
}

// SelectedRelease returns the release under the cursor.
func (m HelmModel) SelectedRelease() service.HelmRelease {
	row := m.releaseTable.SelectedRow()
	if len(row) == 0 || len(m.releases) == 0 {
		return service.HelmRelease{}
	}
	for _, rel := range m.releases {
		if rel.Name == row[0] && rel.Namespace == row[1] {
			return rel
		}
	}
	return service.HelmRelease{}
}
