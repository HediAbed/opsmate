package model

import (
	"fmt"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/HediAbed/opsmate/internal/service"
	"github.com/HediAbed/opsmate/internal/theme"
)

// CRDsModel provides list and instance views for custom resources.
type CRDsModel struct {
	width     int
	height    int
	namespace string

	view crdsViewState

	crds        []service.CRD
	crdsTable   table.Model
	selectedCRD service.CRD

	instances     []service.CRDInstance
	instanceTable table.Model

	spinner   spinner.Model
	loading   bool
	err       error
	statusMsg string

	listRequestID     uint64
	instanceRequestID uint64
}

type crdResultKind uint8

const (
	crdListResult crdResultKind = iota
	crdInstancesResult
)

type crdResultMsg struct {
	kind      crdResultKind
	requestID uint64
	namespace string
	resource  string
	payload   tea.Msg
}

type crdsViewState int

const (
	crdsViewList crdsViewState = iota
	crdsViewInstances
)

var crdsListColSpecs = []colSpec{
	{Title: "NAME", Flex: columnFlexHalf, Min: columnMinimumName},
	{Title: "GROUP", Flex: columnFlexQuarter, Min: columnMinimumReadable},
	{Title: "SCOPE", Width: columnWidthMedium},
	{Title: "VERSIONS", Flex: columnFlexSmall, Min: columnMinimumCompact},
	{Title: "AGE", Width: columnWidthCompact},
}

var crdInstanceColSpecs = []colSpec{
	{Title: "NAME", Flex: columnFlexPrimary, Min: columnMinimumName},
	{Title: "NAMESPACE", Flex: columnFlexQuarter, Min: columnMinimumStandard},
	{Title: "AGE", Width: columnWidthCompact},
}

func NewCRDsModel(namespace string) CRDsModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = theme.SpinnerStyle

	return CRDsModel{
		namespace:     namespace,
		view:          crdsViewList,
		crdsTable:     buildResourceTable(initialScreenWidth, crdsListColSpecs),
		instanceTable: buildResourceTable(initialScreenWidth, crdInstanceColSpecs),
		spinner:       s,
		loading:       true,
	}
}

func (m *CRDsModel) Init() tea.Cmd {
	return tea.Batch(m.fetchCRDList(), m.spinner.Tick)
}

// Activate refreshes the visible data.
func (m *CRDsModel) Activate() tea.Cmd {
	m.loading = true
	return tea.Batch(m.fetchCurrentView(), m.spinner.Tick)
}

func (m *CRDsModel) Deactivate() {
	m.loading = false
}

func (m *CRDsModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.crdsTable.SetWidth(w)
	m.crdsTable.SetColumns(computeColumns(w, crdsListColSpecs))
	m.instanceTable.SetWidth(w)
	m.instanceTable.SetColumns(computeColumns(w, crdInstanceColSpecs))
}

// SetNamespace changes scope and refreshes the visible data.
func (m *CRDsModel) SetNamespace(ns string) tea.Cmd {
	if ns == m.namespace {
		return nil
	}
	m.namespace = ns
	m.instances = nil
	m.loading = true
	m.listRequestID++
	m.instanceRequestID++
	return m.fetchCurrentView()
}

func (CRDsModel) HasInputFocus() bool { return false }

func (m CRDsModel) AIOverlayBounds(totalHeight int) (topOffset, panelHeight, bottomOffset int) {
	top := lipgloss.Height(m.renderTitleBar())
	bottom := lipgloss.Height(m.renderHelpBar())
	if banner := m.renderErrBanner(); banner != "" {
		bottom += lipgloss.Height(banner)
	}
	return aiOverlayBounds(totalHeight, top, bottom)
}

func (m CRDsModel) Update(msg tea.Msg) (CRDsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case crdResultMsg:
		return m.handleCRDResult(msg)

	case service.CRDsMsg:
		m.applyCRDsResult(msg)
		return m, nil

	case service.CRDInstancesMsg:
		return m.applyCRDInstancesResult(msg), nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case tea.MouseClickMsg:
		m.handleCRDMouseClick(msg)
		return m, nil

	case tea.MouseWheelMsg:
		m.handleCRDMouseWheel(msg)
		return m, nil
	}
	return m, nil
}

func (m CRDsModel) handleCRDResult(msg crdResultMsg) (CRDsModel, tea.Cmd) {
	if !m.acceptsResult(msg) {
		return m, nil
	}
	return m.Update(msg.payload)
}

func (m *CRDsModel) applyCRDsResult(msg service.CRDsMsg) {
	m.loading = false
	if msg.Err != nil {
		m.err = msg.Err
		m.crds = nil
		m.crdsTable.SetRows(nil)
		return
	}
	m.err = nil
	m.crds = msg.CRDs
	m.crdsTable.SetRows(m.crdsRows())
	m.statusMsg = theme.Success.Render(fmt.Sprintf("Loaded %d %s", len(msg.CRDs), displayKind("crds", len(msg.CRDs))))
}

func (m CRDsModel) applyCRDInstancesResult(msg service.CRDInstancesMsg) CRDsModel {
	if msg.Resource != m.selectedCRD.Resource || msg.Namespace != m.namespace {
		return m
	}
	m.loading = false
	if msg.Err != nil {
		m.err = msg.Err
		m.instances = nil
		m.instanceTable.SetRows(nil)
		return m
	}
	m.err = nil
	m.instances = msg.Instances
	m.instanceTable.SetRows(m.instanceRows())
	m.statusMsg = theme.Success.Render(fmt.Sprintf("Loaded %d %s of %s", len(msg.Instances), displayKind("instances", len(msg.Instances)), m.selectedCRD.Kind))
	return m
}

func (m *CRDsModel) handleCRDMouseClick(msg tea.MouseClickMsg) {
	if msg.Button != tea.MouseLeft {
		return
	}
	rowIndex := msg.Y - m.tableFirstRowY()
	activeTable := m.activeTable()
	if rowIndex >= 0 && rowIndex < len(activeTable.Rows()) {
		activeTable.SetCursor(rowIndex)
	}
}

func (m *CRDsModel) handleCRDMouseWheel(msg tea.MouseWheelMsg) {
	activeTable := m.activeTable()
	switch msg.Button {
	case tea.MouseWheelUp:
		activeTable.MoveUp(tableWheelStep)
	case tea.MouseWheelDown:
		activeTable.MoveDown(tableWheelStep)
	}
}

func (m *CRDsModel) activeTable() *table.Model {
	if m.view == crdsViewInstances {
		return &m.instanceTable
	}
	return &m.crdsTable
}

func (m CRDsModel) tableFirstRowY() int {
	y := lipgloss.Height(m.renderTitleBar())
	if banner := m.renderErrBanner(); banner != "" {
		y += lipgloss.Height(banner)
	}
	return y + tableHeaderChromeRows
}

func (m CRDsModel) handleKey(msg tea.KeyPressMsg) (CRDsModel, tea.Cmd) {
	key := msg.String()
	switch key {
	case "r":
		m.loading = true
		return m, tea.Batch(m.fetchCurrentView(), m.spinner.Tick)
	case "enter":
		return m.drillIntoSelected()
	case "esc":
		return m.backToList()
	}
	var cmd tea.Cmd
	switch m.view {
	case crdsViewList:
		m.crdsTable, cmd = m.crdsTable.Update(msg)
	case crdsViewInstances:
		m.instanceTable, cmd = m.instanceTable.Update(msg)
	}
	return m, cmd
}

func (m CRDsModel) drillIntoSelected() (CRDsModel, tea.Cmd) {
	if m.view != crdsViewList {
		return m, nil
	}
	row := m.crdsTable.SelectedRow()
	if len(row) == 0 {
		return m, nil
	}
	for _, c := range m.crds {
		if c.Name == row[0] {
			m.selectedCRD = c
			m.view = crdsViewInstances
			m.instances = nil
			m.instanceTable.SetRows(nil)
			m.loading = true
			return m, tea.Batch(m.fetchCRDInstances(c.Resource), m.spinner.Tick)
		}
	}
	return m, nil
}

func (m CRDsModel) backToList() (CRDsModel, tea.Cmd) {
	if m.view != crdsViewInstances {
		return m, nil
	}
	m.view = crdsViewList
	m.selectedCRD = service.CRD{}
	m.instances = nil
	m.instanceTable.SetRows(nil)
	m.statusMsg = ""
	m.instanceRequestID++
	return m, nil
}

func (m *CRDsModel) fetchCurrentView() tea.Cmd {
	switch m.view {
	case crdsViewList:
		return m.fetchCRDList()
	case crdsViewInstances:
		if m.selectedCRD.Resource == "" {
			return nil
		}
		return m.fetchCRDInstances(m.selectedCRD.Resource)
	}
	return nil
}

func (m *CRDsModel) fetchCRDList() tea.Cmd {
	m.listRequestID++
	requestID := m.listRequestID
	namespace := m.namespace
	command := service.FetchCRDs()
	return func() tea.Msg {
		return crdResultMsg{kind: crdListResult, requestID: requestID, namespace: namespace, payload: command()}
	}
}

func (m *CRDsModel) fetchCRDInstances(resource string) tea.Cmd {
	m.instanceRequestID++
	requestID := m.instanceRequestID
	namespace := m.namespace
	command := service.ListCRDInstances(resource, namespace)
	return func() tea.Msg {
		return crdResultMsg{
			kind:      crdInstancesResult,
			requestID: requestID,
			namespace: namespace,
			resource:  resource,
			payload:   command(),
		}
	}
}

func (m CRDsModel) acceptsResult(msg crdResultMsg) bool {
	if msg.namespace != m.namespace {
		return false
	}
	switch msg.kind {
	case crdListResult:
		return msg.requestID == m.listRequestID && m.view == crdsViewList
	case crdInstancesResult:
		return msg.requestID == m.instanceRequestID &&
			m.view == crdsViewInstances &&
			msg.resource == m.selectedCRD.Resource
	default:
		return false
	}
}

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
	if cursor := tableCursorLabel(m.activeTableCursor()); cursor != "" {
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
	for _, c := range m.crds {
		rows = append(rows, table.Row{
			c.Name,
			c.Group,
			c.Scope,
			service.JoinVersions(c.Versions),
			c.Age,
		})
	}
	return rows
}

func (m CRDsModel) instanceRows() []table.Row {
	rows := make([]table.Row, 0, len(m.instances))
	for _, i := range m.instances {
		rows = append(rows, table.Row{
			i.Name,
			i.Namespace,
			i.Age,
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
