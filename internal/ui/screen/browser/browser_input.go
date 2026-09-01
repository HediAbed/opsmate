package browser

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/HediAbed/opsmate/internal/kube"
	"github.com/HediAbed/opsmate/internal/terminal"
	"github.com/HediAbed/opsmate/internal/ui/screen"
	"github.com/HediAbed/opsmate/internal/ui/theme"
)

const (
	replicaCountBase    = 10
	replicaCountBitSize = 32
)

func (m BrowserModel) handleKey(msg tea.KeyPressMsg) (BrowserModel, tea.Cmd) {
	key := msg.String()
	switch m.state {
	case stateShell:
		return m.handleShellKey(msg)
	case stateFilter:
		return m.handleFilterKey(key, msg)
	case stateScaleInput:
		return m.handleScaleInputKey(key, msg)
	case stateScaleConfirm:
		return m.handleScaleConfirmationKey(key)
	case stateDeleteConfirm:
		return m.handleResourceConfirmationKey(key)
	case stateDetail:
		return m.handleDetailKey(key, msg)
	case stateBrowsing:
		return m.handleBrowsingKey(key, msg)
	default:
		m.state = stateBrowsing
		return m, nil
	}
}

func (m BrowserModel) handleFilterKey(key string, msg tea.KeyPressMsg) (BrowserModel, tea.Cmd) {
	switch key {
	case "esc":
		m.filterActive = false
		m.filterText = ""
		m.filterInput.SetValue("")
		m.filterInput.Blur()
		m.state = stateBrowsing
		m.rebuildTable()
		return m, nil
	case "enter":
		m.filterText = m.filterInput.Value()
		m.filterInput.Blur()
		m.state = stateBrowsing
		m.rebuildTable()
		m.filterActive = m.filterText != ""
		return m, nil
	default:
		var command tea.Cmd
		m.filterInput, command = m.filterInput.Update(msg)
		m.filterText = m.filterInput.Value()
		m.refreshRows(m.resourceTable.Cursor())
		return m, command
	}
}

func (m BrowserModel) handleScaleInputKey(key string, msg tea.KeyPressMsg) (BrowserModel, tea.Cmd) {
	switch key {
	case "esc":
		m.state = stateBrowsing
		m.textInput.Blur()
		m.statusMsg = ""
		return m, nil
	case "enter":
		return m, m.beginScaleConfirmation()
	default:
		var command tea.Cmd
		m.textInput, command = m.textInput.Update(msg)
		return m, command
	}
}

func (m *BrowserModel) beginScaleConfirmation() tea.Cmd {
	value := strings.TrimSpace(m.textInput.Value())
	replicas, err := strconv.ParseInt(value, replicaCountBase, replicaCountBitSize)
	if err != nil || replicas < 0 {
		m.statusMsg = theme.Error.Render("Invalid replica count: " + value)
		return nil
	}
	m.scaleReplicas = int32(replicas)
	m.confirmAction = "scale"
	m.confirmTarget = fmt.Sprintf("%s to %d replicas", m.scaleIdentity.Name, replicas)
	m.showConfirm = true
	m.state = stateScaleConfirm
	return nil
}

func (m BrowserModel) handleScaleConfirmationKey(key string) (BrowserModel, tea.Cmd) {
	switch key {
	case "y", "Y":
		return m.confirmScale()
	case "n", "N", "esc":
		m.showConfirm = false
		m.state = stateBrowsing
		m.textInput.Blur()
		m.statusMsg = theme.Dim.Render("Scale cancelled")
	}
	return m, nil
}

func (m BrowserModel) confirmScale() (BrowserModel, tea.Cmd) {
	identity := m.scaleIdentity
	m.showConfirm = false
	m.loading = true
	m.state = stateBrowsing
	m.textInput.Blur()
	m.statusMsg = theme.SpinnerStyle.Render("Scaling " + identity.Kind + "/" + identity.Name + "...")
	workloadKind, err := workloadKindForResource(identity.Kind)
	if err != nil {
		m.loading = false
		m.statusMsg = theme.Error.Render(terminal.SanitizeLine(err.Error()))
		return m, nil
	}
	request := kube.ScaleRequest{
		Workload: kube.WorkloadReference{
			Kind:      workloadKind,
			Namespace: identity.Namespace,
			Name:      identity.Name,
		},
		Replicas: m.scaleReplicas,
	}
	return m, m.operations.ScaleWorkload(request)
}

func (m BrowserModel) handleResourceConfirmationKey(key string) (BrowserModel, tea.Cmd) {
	switch key {
	case "y", "Y":
		return m, m.executeConfirmedResourceAction()
	case "n", "N", "esc":
		m.showConfirm = false
		m.state = stateBrowsing
		m.statusMsg = theme.Dim.Render(m.confirmAction + " cancelled")
	}
	return m, nil
}

func (m BrowserModel) handleDetailKey(key string, msg tea.KeyPressMsg) (BrowserModel, tea.Cmd) {
	switch key {
	case "esc":
		m.closeDetail()
		return m, nil
	case "a":
		return m, m.analyzeDetail()
	case "v":
		m.splitHorizontal = !m.splitHorizontal
		return m, nil
	case "c":
		return m.copyDetailContent()
	default:
		var command tea.Cmd
		m.detailView, command = m.detailView.Update(msg)
		return m, command
	}
}

func (m *BrowserModel) closeDetail() {
	m.detailRequestID++
	m.showDetail = false
	m.state = stateBrowsing
	m.statusMsg = ""
	m.analysisSummary = ""
	m.analysisSummaryLoading = false
	m.analysisSummaryErr = nil
	m.detailKind = ""
}

func (m *BrowserModel) analyzeDetail() tea.Cmd {
	if !m.canAnalyzeDetail() {
		m.statusMsg = theme.Warning.Render("Analysis is only available for non-secret describe output")
		return nil
	}
	if m.analysisSummaryLoading {
		return nil
	}
	identity, found := m.selectedIdentity()
	if !found {
		m.statusMsg = theme.Warning.Render("No resource selected")
		return nil
	}
	m.analysisSummaryLoading = true
	m.analysisSummary = ""
	m.analysisSummaryErr = nil
	m.statusMsg = theme.SpinnerStyle.Render("Analyzing " + identity.Kind + "/" + identity.Name + "...")
	return m.fetchDetailSummary(identity)
}

func (m BrowserModel) copyDetailContent() (BrowserModel, tea.Cmd) {
	lineCount := strings.Count(m.detailContent, "\n") + 1
	status, command := screen.CopyToClipboard(m.detailContent, fmt.Sprintf("%d lines", lineCount))
	m.statusMsg = status
	return m, command
}

func (m BrowserModel) handleBrowsingKey(key string, msg tea.KeyPressMsg) (BrowserModel, tea.Cmd) {
	m.statusMsg = ""
	if key == "esc" {
		return m, m.handleBrowsingEscape()
	}
	if key == "space" {
		m.toggleSelectedResource()
		return m, nil
	}

	switch key {
	case "left", "shift+tab":
		return m.cycleResourceType(-1)
	case "right", "tab":
		return m.cycleResourceType(1)
	case "p", "d":
		return m.selectResourceTypeShortcut(key)
	case "enter", "e", "l", "s", "y":
		return m.handleResourceReadKey(key)
	case "R", "x":
		return m.handleResourceMutationKey(key)
	case "X":
		return m.openShell()
	default:
		return m.handleBrowserUtilityKey(key, msg)
	}
}

func (m *BrowserModel) handleBrowsingEscape() tea.Cmd {
	if len(m.selected) > 0 {
		m.clearSelection()
		m.rebuildTable()
		return nil
	}
	if m.errBanner != "" {
		m.errBanner = ""
		return nil
	}
	return func() tea.Msg { return screen.GoBackMsg{} }
}

func (m *BrowserModel) toggleSelectedResource() {
	identity, found := m.selectedIdentity()
	if !found {
		return
	}
	m.toggleResourceSelection(identity)
	m.rebuildTable()
}

func (m BrowserModel) selectResourceTypeShortcut(key string) (BrowserModel, tea.Cmd) {
	if key == "p" {
		return m.selectResourceType(resourceTypePods)
	}
	return m.selectResourceType(resourceTypeDeployments)
}

func (m BrowserModel) selectResourceType(resourceType string) (BrowserModel, tea.Cmd) {
	if m.resourceType == resourceType {
		return m, nil
	}
	m.SetResourceType(resourceType)
	return m, m.loadCurrentResource()
}

func (m BrowserModel) handleResourceReadKey(key string) (BrowserModel, tea.Cmd) {
	switch key {
	case "enter":
		return m, m.describeSelectedResource()
	case "e":
		return m, m.fetchSelectedResourceEvents()
	case "l":
		return m, m.openSelectedResourceLogs()
	case "s":
		return m, m.openScaleInput()
	case "y":
		return m, m.fetchSelectedResourceYAML()
	default:
		return m, nil
	}
}

func (m *BrowserModel) describeSelectedResource() tea.Cmd {
	identity, found := m.selectedIdentity()
	if !found {
		return nil
	}
	reference, err := resourceReferenceForIdentity(identity)
	if err != nil {
		m.showOperationSetupError(err)
		return nil
	}
	m.loading = true
	m.statusMsg = theme.SpinnerStyle.Render("Inspecting " + identity.Kind + "/" + identity.Name + "...")
	return tea.Batch(
		m.operations.InspectResource(reference),
		m.spinner.Tick,
	)
}

func (m *BrowserModel) fetchSelectedResourceEvents() tea.Cmd {
	_, name := m.selectedResourceKindAndName()
	if name == "" {
		return nil
	}
	m.loading = true
	m.statusMsg = theme.SpinnerStyle.Render("Fetching events...")
	return tea.Batch(m.cluster.FetchEvents(m.selectedResourceNS()), m.spinner.Tick)
}

func (m *BrowserModel) openSelectedResourceLogs() tea.Cmd {
	if m.resourceType != resourceTypePods {
		m.statusMsg = theme.Warning.Render("Logs are only available for pods")
		return nil
	}
	_, name := m.selectedResourceKindAndName()
	if name == "" {
		return nil
	}
	namespace := m.selectedResourceNS()
	return func() tea.Msg {
		return screen.DrillDownMsg{Screen: screen.Logs, ResourceName: name, ResourceNS: namespace}
	}
}

func (m *BrowserModel) openScaleInput() tea.Cmd {
	if !m.resourceSupportsScaling() {
		m.statusMsg = theme.Warning.Render("Scale is only available for deployments and statefulsets")
		return nil
	}
	identity, found := m.selectedIdentity()
	if !found {
		return nil
	}
	m.textInput.Reset()
	m.textInput.Focus()
	m.state = stateScaleInput
	m.scaleName = identity.Name
	m.scaleIdentity = identity
	m.scaleCurrentInfo = m.lookupReplicaInfoFor(identity)
	m.statusMsg = theme.Accent.Render("Scale " + identity.Name + " -- enter replica count, esc to cancel")
	return textinput.Blink
}

func (m *BrowserModel) fetchSelectedResourceYAML() tea.Cmd {
	if m.resourceType == resourceTypeSecrets {
		m.statusMsg = theme.Warning.Render(restrictedResourceYAMLMessage)
		return nil
	}
	identity, found := m.selectedIdentity()
	if !found {
		return nil
	}
	reference, err := resourceReferenceForIdentity(identity)
	if err != nil {
		m.showOperationSetupError(err)
		return nil
	}
	m.loading = true
	m.statusMsg = theme.SpinnerStyle.Render("Fetching YAML for " + identity.Kind + "/" + identity.Name + "...")
	return tea.Batch(m.operations.ResourceYAML(reference), m.spinner.Tick)
}

func (m BrowserModel) resourceSupportsScaling() bool {
	return m.resourceType == resourceTypeDeployments || m.resourceType == resourceTypeStatefulSets
}
