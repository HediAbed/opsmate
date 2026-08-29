package ui

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/HediAbed/opsmate/internal/analysis"
	"github.com/HediAbed/opsmate/internal/theme"
)

func (m *BrowserModel) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.recalcLayout()
}

func (m *BrowserModel) SetNamespace(namespace string) {
	m.fetchRequestID++
	m.detailRequestID++
	m.namespace = namespace
	for _, binding := range resourceCatalog {
		binding.Clear(m)
	}
	m.loading = true
	m.showDetail = false
	m.showConfirm = false
	m.state = stateBrowsing
	m.statusMsg = ""
	m.errBanner = ""
	m.err = nil
	m.analysisSummary = ""
	m.analysisSummaryErr = nil
	m.analysisSummaryLoading = false
	m.clearSelection()
	m.stopAllLiveSets()
}

func (m BrowserModel) SelectedResource() (string, string) {
	return m.selectedResourceKindAndName()
}

func (m BrowserModel) ResourceType() string {
	return m.resourceType
}

func (m BrowserModel) Wide() bool {
	return m.wide
}

func (m *BrowserModel) SetWide(wide bool) {
	m.wide = wide
}

func (m BrowserModel) cycleResourceType(delta int) (BrowserModel, tea.Cmd) {
	idx := -1
	for i, rt := range allResourceTypes {
		if rt == m.resourceType {
			idx = i
			break
		}
	}
	if idx < 0 {
		idx = 0
	}
	next := (idx + delta + len(allResourceTypes)) % len(allResourceTypes)
	target := allResourceTypes[next]
	return m.selectResourceType(target)
}

func (m *BrowserModel) SetResourceType(resourceType string) {
	if m.resourceType == resourceType {
		return
	}
	m.resourceType = resourceType
	m.fetchRequestID++
	m.detailRequestID++
	m.clearSelection()
	m.stopAllLiveSets()
	m.rebuildTable()
}

func (m BrowserModel) canAnalyzeDetail() bool {
	return m.detailKind == "describe" && m.resourceType != resourceTypeSecrets
}

func (m BrowserModel) detailHelp() string {
	keys := "esc: back | c: copy | v: split"
	if m.canAnalyzeDetail() {
		keys = "esc: back | a: summary | c: copy | v: split"
	}
	return theme.Dim.Render(keys)
}

func (m BrowserModel) providerDetailContext() string {
	if m.resourceType == resourceTypeSecrets {
		return ""
	}
	return m.detailContent
}

func (m *BrowserModel) fetchDetailSummary(identity resourceIdentity) tea.Cmd {
	m.detailRequestID++
	requestID := m.detailRequestID
	content := m.detailContent
	command := analysis.DescribeSummary(identity.Kind, identity.Name, content)
	return func() tea.Msg {
		return browserDetailSummaryResultMsg{
			requestID: requestID,
			identity:  identity,
			content:   content,
			payload:   command(),
		}
	}
}

func (m BrowserModel) acceptsDetailSummary(msg browserDetailSummaryResultMsg) bool {
	identity, ok := m.selectedIdentity()
	return ok && m.canAnalyzeDetail() &&
		msg.requestID == m.detailRequestID &&
		msg.identity == identity &&
		msg.content == m.detailContent
}

func (m BrowserModel) selectedResourceKindAndName() (string, string) {
	identity, ok := m.selectedIdentity()
	if !ok {
		return m.resourceKindSingular(), ""
	}
	return identity.Kind, identity.Name
}

func (m BrowserModel) selectedIdentity() (resourceIdentity, bool) {
	index := m.resourceTable.Cursor()
	if index >= 0 && index < len(m.visibleResources) {
		return m.visibleResources[index], true
	}
	row := m.resourceTable.SelectedRow()
	if len(row) == 0 {
		return resourceIdentity{}, false
	}
	nameIndex := 0
	kind := m.resourceKindSingular()
	if m.resourceType == resourceTypeRBAC && len(row) > 1 {
		kind = strings.ToLower(row[0])
		nameIndex = 1
	}
	name := strings.TrimPrefix(row[nameIndex], selectionMark)
	return resourceIdentity{Kind: kind, Namespace: m.namespace, Name: name}, name != ""
}

func (m BrowserModel) resourceKindSingular() string {
	if b, ok := resourceCatalog[m.resourceType]; ok {
		return b.Singular
	}
	return m.resourceType
}

func (m *BrowserModel) toggleResourceSelection(identity resourceIdentity) {
	if m.selected == nil {
		m.selected = make(map[string]bool)
	}
	if m.selectedIdentities == nil {
		m.selectedIdentities = make(map[string]resourceIdentity)
	}
	key := m.selectionKey(identity)
	if m.selected[key] {
		delete(m.selected, key)
		delete(m.selectedIdentities, key)
	} else {
		m.selected[key] = true
		m.selectedIdentities[key] = identity
	}
}

func (m *BrowserModel) clearSelection() {
	m.selected = nil
	m.selectedIdentities = nil
}

func (m BrowserModel) selectedNames() []string {
	names := make([]string, 0, len(m.selected))
	for key := range m.selected {
		if identity, ok := m.selectedIdentities[key]; ok {
			names = append(names, identity.Name)
			continue
		}
		names = append(names, key)
	}
	slices.Sort(names)
	return names
}

func (m BrowserModel) selectedBatchIdentity(fallbackKind string) (resourceIdentity, error) {
	target := resourceIdentity{}
	for key := range m.selected {
		identity, found := m.selectedIdentities[key]
		if !found {
			continue
		}
		if target.Kind == "" {
			target = resourceIdentity{Kind: identity.Kind, Namespace: identity.Namespace}
			continue
		}
		if target.Kind != identity.Kind || target.Namespace != identity.Namespace {
			return resourceIdentity{}, ErrMixedResourceSelection
		}
	}
	if target.Kind == "" {
		return resourceIdentity{Kind: fallbackKind, Namespace: m.namespace}, nil
	}
	return target, nil
}

func (m *BrowserModel) showOperationSetupError(err error) {
	m.loading = false
	m.statusMsg = theme.Error.Render(sanitizeTerminalLine(err.Error()))
}

func (m BrowserModel) displayIdentity(identity resourceIdentity) string {
	name := displayResourceName(identity.Namespace, identity.Name, m.namespace == "")
	if m.selected[m.selectionKey(identity)] {
		return selectionMark + name
	}
	return name
}

func (m BrowserModel) selectionKey(identity resourceIdentity) string {
	if m.namespace != "" && m.resourceType != resourceTypeRBAC {
		return identity.Name
	}
	return identity.key()
}

func (m BrowserModel) podStatusFor(identity resourceIdentity) (string, bool) {
	for _, pod := range m.pods {
		if pod.Name == identity.Name && namespacesMatch(identity.Namespace, pod.Namespace) {
			return pod.Status, true
		}
	}
	return "", false
}

func podSupportsExec(status string) bool {
	return status == "Running"
}

func (m BrowserModel) selectedResourceNS() string {
	identity, ok := m.selectedIdentity()
	if !ok {
		return m.namespace
	}
	return identity.Namespace
}

func (m *BrowserModel) fetchCurrentResources() tea.Cmd {
	m.fetchRequestID++
	requestID := m.fetchRequestID
	namespace := m.namespace
	resourceType := m.resourceType
	command := m.cluster.FetchPods(namespace)
	if binding, ok := resourceCatalog[resourceType]; ok {
		command = binding.Fetch(m.cluster, namespace)
	}
	return func() tea.Msg {
		return browserResultMsg{
			requestID:    requestID,
			namespace:    namespace,
			resourceType: resourceType,
			payload:      command(),
		}
	}
}
