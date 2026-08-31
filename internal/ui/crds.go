package ui

import (
	"fmt"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/ui/theme"
)

type CRDsModel struct {
	width     int
	height    int
	namespace string
	cluster   clusterCommands

	view crdsViewState

	crds        []cluster.CRD
	crdsTable   table.Model
	selectedCRD cluster.CRD

	instances     []cluster.CRDInstance
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

func NewCRDsModel(namespace string, commands clusterCommands) CRDsModel {
	loadingSpinner := spinner.New()
	loadingSpinner.Spinner = spinner.Dot
	loadingSpinner.Style = theme.SpinnerStyle

	return CRDsModel{
		namespace:     namespace,
		cluster:       commands,
		view:          crdsViewList,
		crdsTable:     buildResourceTable(initialScreenWidth, crdsListColSpecs),
		instanceTable: buildResourceTable(initialScreenWidth, crdInstanceColSpecs),
		spinner:       loadingSpinner,
		loading:       true,
	}
}

func (m *CRDsModel) Init() tea.Cmd {
	return tea.Batch(m.fetchCRDList(), m.spinner.Tick)
}

func (m *CRDsModel) Activate() tea.Cmd {
	m.loading = true
	return tea.Batch(m.fetchCurrentView(), m.spinner.Tick)
}

func (m *CRDsModel) Deactivate() {
	m.loading = false
}

func (m *CRDsModel) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.crdsTable.SetWidth(width)
	m.crdsTable.SetColumns(computeColumns(width, crdsListColSpecs))
	m.instanceTable.SetWidth(width)
	m.instanceTable.SetColumns(computeColumns(width, crdInstanceColSpecs))
}

func (m *CRDsModel) SetNamespace(namespace string) tea.Cmd {
	if namespace == m.namespace {
		return nil
	}
	m.namespace = namespace
	m.instances = nil
	m.loading = true
	m.listRequestID++
	m.instanceRequestID++
	return m.fetchCurrentView()
}

func (CRDsModel) HasInputFocus() bool { return false }

func (m CRDsModel) AnalysisOverlayBounds(totalHeight int) (topOffset, panelHeight, bottomOffset int) {
	top := lipgloss.Height(m.renderTitleBar())
	bottom := lipgloss.Height(m.renderHelpBar())
	if banner := m.renderErrBanner(); banner != "" {
		bottom += lipgloss.Height(banner)
	}
	return analysisOverlayBounds(totalHeight, top, bottom)
}

func (m CRDsModel) Update(msg tea.Msg) (CRDsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case crdResultMsg:
		return m.handleCRDResult(msg)

	case cluster.CRDsMsg:
		m.applyCRDsResult(msg)
		return m, nil

	case cluster.CRDInstancesMsg:
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

func (m *CRDsModel) applyCRDsResult(msg cluster.CRDsMsg) {
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

func (m CRDsModel) applyCRDInstancesResult(msg cluster.CRDInstancesMsg) CRDsModel {
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
	firstRow := lipgloss.Height(m.renderTitleBar())
	if banner := m.renderErrBanner(); banner != "" {
		firstRow += lipgloss.Height(banner)
	}
	return firstRow + tableHeaderChromeRows
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
	for _, definition := range m.crds {
		if definition.Name == row[0] {
			m.selectedCRD = definition
			m.view = crdsViewInstances
			m.instances = nil
			m.instanceTable.SetRows(nil)
			m.loading = true
			return m, tea.Batch(m.fetchCRDInstances(definition), m.spinner.Tick)
		}
	}
	return m, nil
}

func (m CRDsModel) backToList() (CRDsModel, tea.Cmd) {
	if m.view != crdsViewInstances {
		return m, nil
	}
	m.view = crdsViewList
	m.selectedCRD = cluster.CRD{}
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
		return m.fetchCRDInstances(m.selectedCRD)
	}
	return nil
}

func (m *CRDsModel) fetchCRDList() tea.Cmd {
	m.listRequestID++
	requestID := m.listRequestID
	namespace := m.namespace
	command := m.cluster.FetchCRDs()
	return func() tea.Msg {
		return crdResultMsg{kind: crdListResult, requestID: requestID, namespace: namespace, payload: command()}
	}
}

func (m *CRDsModel) fetchCRDInstances(crd cluster.CRD) tea.Cmd {
	m.instanceRequestID++
	requestID := m.instanceRequestID
	namespace := m.namespace
	command := m.cluster.FetchCRDInstances(crd, namespace)
	return func() tea.Msg {
		return crdResultMsg{
			kind:      crdInstancesResult,
			requestID: requestID,
			namespace: namespace,
			resource:  crd.Resource,
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
