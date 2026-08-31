package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/HediAbed/opsmate/internal/kube"
	"github.com/HediAbed/opsmate/internal/ui/theme"
)

func (m BrowserModel) handleResourceMutationKey(key string) (BrowserModel, tea.Cmd) {
	action := "delete"
	if key == "R" {
		if !m.resourceSupportsScaling() {
			m.statusMsg = theme.Warning.Render("Restart rollout is only available for deployments and statefulsets")
			return m, nil
		}
		action = "restart"
	}
	return m, m.beginResourceConfirmation(action)
}

func (m *BrowserModel) beginResourceConfirmation(action string) tea.Cmd {
	if len(m.selected) > 0 {
		return m.beginBatchResourceConfirmation(action)
	}
	identity, found := m.selectedIdentity()
	if !found {
		return nil
	}
	m.confirmAction = action
	m.confirmTarget = identity.Kind + "/" + identity.Name
	if action == "restart" {
		m.confirmTarget = m.resourceType + "/" + identity.Name
	}
	m.confirmIdentity = identity
	m.showConfirm = true
	m.state = stateDeleteConfirm
	return nil
}

func (m *BrowserModel) beginBatchResourceConfirmation(action string) tea.Cmd {
	if m.namespace == "" {
		m.errBanner = batchAllNamespacesErrorText(action)
		return nil
	}
	m.confirmAction = action
	m.confirmTarget = fmt.Sprintf("%d %s", len(m.selected), m.resourceType)
	m.showConfirm = true
	m.state = stateDeleteConfirm
	return nil
}

func (m BrowserModel) handleBrowserUtilityKey(key string, msg tea.KeyPressMsg) (BrowserModel, tea.Cmd) {
	switch key {
	case "c":
		return m.copyBrowserSelection()
	case "/":
		m.filterActive = true
		m.filterInput.SetValue(m.filterText)
		m.filterInput.Focus()
		m.state = stateFilter
		return m, textinput.Blink
	case "r":
		return m, m.loadCurrentResource()
	case "w":
		m.wide = !m.wide
		m.rebuildTable()
		return m, nil
	default:
		var command tea.Cmd
		m.resourceTable, command = m.resourceTable.Update(msg)
		return m, command
	}
}

func (m BrowserModel) copyBrowserSelection() (BrowserModel, tea.Cmd) {
	if m.showDetail {
		return m.copyDetailContent()
	}
	row := m.resourceTable.SelectedRow()
	if row == nil {
		return m, nil
	}
	text := strings.Join(row, "\t")
	status, command := copyToClipboard(text, "row")
	m.statusMsg = status
	return m, command
}

func (m *BrowserModel) executeConfirmedResourceAction() tea.Cmd {
	m.showConfirm = false
	m.loading = true
	m.state = stateBrowsing

	if len(m.selected) > 0 {
		resourceNames := m.selectedNames()
		batchIdentity, err := m.selectedBatchIdentity(m.resourceKindSingular())
		m.clearSelection()
		if err != nil {
			m.showOperationSetupError(err)
			return nil
		}
		return m.executeConfirmedBatchAction(batchIdentity, resourceNames)
	}
	return m.executeConfirmedSingleAction()
}

func (m *BrowserModel) executeConfirmedBatchAction(identity resourceIdentity, resourceNames []string) tea.Cmd {
	if m.confirmAction == "restart" {
		workloadKind, err := workloadKindForResource(identity.Kind)
		if err != nil {
			m.showOperationSetupError(err)
			return nil
		}
		m.statusMsg = theme.SpinnerStyle.Render("Restarting " + m.confirmTarget + "...")
		return m.operations.RestartWorkloads(kube.WorkloadBatch{
			Kind:      workloadKind,
			Namespace: identity.Namespace,
			Names:     resourceNames,
		})
	}
	resource, err := groupResourceForKind(identity.Kind)
	if err != nil {
		m.showOperationSetupError(err)
		return nil
	}
	m.statusMsg = theme.SpinnerStyle.Render("Deleting " + m.confirmTarget + "...")
	return m.operations.DeleteResources(kube.ResourceBatch{
		Resource:  resource,
		Namespace: identity.Namespace,
		Names:     resourceNames,
	})
}

func (m *BrowserModel) executeConfirmedSingleAction() tea.Cmd {
	identity := m.confirmIdentity
	if m.confirmAction == "restart" {
		reference, err := workloadReferenceForIdentity(identity)
		if err != nil {
			m.showOperationSetupError(err)
			return nil
		}
		m.statusMsg = theme.SpinnerStyle.Render("Restarting " + m.confirmTarget + "...")
		return m.operations.RestartWorkload(reference)
	}
	reference, err := resourceReferenceForIdentity(identity)
	if err != nil {
		m.showOperationSetupError(err)
		return nil
	}
	m.statusMsg = theme.SpinnerStyle.Render("Deleting " + identity.Kind + "/" + identity.Name + "...")
	return m.operations.DeleteResource(reference)
}
