package ui

import (
	"errors"
	"fmt"
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/theme"
)

var ErrLiveUpdatesStopped = errors.New("cluster resource updates stopped")

var liveResourceKinds = []string{
	resourceTypePods,
	resourceTypeDeployments,
	resourceTypeIngresses,
	resourceTypeNetworkPolicies,
	resourceTypePVCs,
	resourceTypeCronJobs,
	resourceTypeHPAs,
	resourceTypeSecrets,
	resourceTypeReplicaSets,
}

type closableLiveSet interface {
	Stop()
	Owns(supervisedLiveMsg) bool
}

func (m *BrowserModel) liveSetForKind(kind string) closableLiveSet {
	switch kind {
	case resourceTypePods:
		return &m.podLive
	case resourceTypeDeployments:
		return &m.deploymentLive
	case resourceTypeIngresses:
		return &m.ingressLive
	case resourceTypeNetworkPolicies:
		return &m.networkPolicyLive
	case resourceTypePVCs:
		return &m.persistentVolumeClaimLive
	default:
		return m.scheduledLiveSetForKind(kind)
	}
}

func (m *BrowserModel) scheduledLiveSetForKind(kind string) closableLiveSet {
	switch kind {
	case resourceTypeCronJobs:
		return &m.cronJobLive
	case resourceTypeHPAs:
		return &m.horizontalPodAutoscalerLive
	case resourceTypeSecrets:
		return &m.secretLive
	case resourceTypeReplicaSets:
		return &m.replicaSetLive
	default:
		return nil
	}
}

var resourceLiveStarters = map[string]func(*BrowserModel) tea.Cmd{
	resourceTypePods: func(m *BrowserModel) tea.Cmd {
		set, err := m.cluster.ObservePods(m.namespace)
		return startBrowserLiveSet(m, &m.podLive, set, err)
	},
	resourceTypeDeployments: func(m *BrowserModel) tea.Cmd {
		set, err := m.cluster.ObserveDeployments(m.namespace)
		return startBrowserLiveSet(m, &m.deploymentLive, set, err)
	},
	resourceTypeIngresses: func(m *BrowserModel) tea.Cmd {
		set, err := m.cluster.ObserveIngresses(m.namespace)
		return startBrowserLiveSet(m, &m.ingressLive, set, err)
	},
	resourceTypeNetworkPolicies: func(m *BrowserModel) tea.Cmd {
		set, err := m.cluster.ObserveNetworkPolicies(m.namespace)
		return startBrowserLiveSet(m, &m.networkPolicyLive, set, err)
	},
	resourceTypePVCs: func(m *BrowserModel) tea.Cmd {
		set, err := m.cluster.ObservePersistentVolumeClaims(m.namespace)
		return startBrowserLiveSet(m, &m.persistentVolumeClaimLive, set, err)
	},
	resourceTypeCronJobs: func(m *BrowserModel) tea.Cmd {
		set, err := m.cluster.ObserveCronJobs(m.namespace)
		return startBrowserLiveSet(m, &m.cronJobLive, set, err)
	},
	resourceTypeHPAs: func(m *BrowserModel) tea.Cmd {
		set, err := m.cluster.ObserveHorizontalPodAutoscalers(m.namespace)
		return startBrowserLiveSet(m, &m.horizontalPodAutoscalerLive, set, err)
	},
	resourceTypeSecrets: func(m *BrowserModel) tea.Cmd {
		set, err := m.cluster.ObserveSecrets(m.namespace)
		return startBrowserLiveSet(m, &m.secretLive, set, err)
	},
	resourceTypeReplicaSets: func(m *BrowserModel) tea.Cmd {
		set, err := m.cluster.ObserveReplicaSets(m.namespace)
		return startBrowserLiveSet(m, &m.replicaSetLive, set, err)
	},
}

func startBrowserLiveSet[T liveResource](
	model *BrowserModel,
	supervisor *liveSupervisor[T],
	set resourceLiveSet[T],
	err error,
) tea.Cmd {
	if err != nil {
		model.loading = false
		model.err = err
		model.errBanner = sanitizeTerminalLine(err.Error())
		return nil
	}
	return supervisor.Set(set)
}

func (m *BrowserModel) startResourceLiveSet() tea.Cmd {
	start := resourceLiveStarters[m.resourceType]
	if start == nil {
		return nil
	}
	return start(m)
}

func (m *BrowserModel) loadCurrentResource() tea.Cmd {
	m.loading = true
	if _, observed := resourceLiveStarters[m.resourceType]; observed {
		command := m.startResourceLiveSet()
		if !m.loading {
			return command
		}
		return tea.Batch(command, m.spinner.Tick)
	}
	return tea.Batch(m.fetchCurrentResources(), m.spinner.Tick)
}

func (m *BrowserModel) stopAllLiveSets() {
	for _, kind := range liveResourceKinds {
		if set := m.liveSetForKind(kind); set != nil {
			set.Stop()
		}
	}
}

func (m *BrowserModel) Activate() tea.Cmd {
	m.active = true
	return m.loadCurrentResource()
}

func (m *BrowserModel) Deactivate() {
	m.active = false
	m.stopAllLiveSets()
	if m.shellSession != nil {
		closed, _ := m.closeShell()
		*m = closed
	}
}

func (m BrowserModel) handleSupervisedLiveMessage(message supervisedLiveMsg) (BrowserModel, tea.Cmd) {
	for _, kind := range liveResourceKinds {
		set := m.liveSetForKind(kind)
		if set == nil || !set.Owns(message) {
			continue
		}
		if message.closed {
			return m.handleLiveSetClosed(kind)
		}
		return m.Update(message.payload)
	}
	return m, nil
}

func (m BrowserModel) ownsSupervisedLiveMessage(message supervisedLiveMsg) bool {
	for _, kind := range liveResourceKinds {
		set := m.liveSetForKind(kind)
		if set != nil && set.Owns(message) {
			return true
		}
	}
	return false
}

func (m BrowserModel) handleLiveSetClosed(kind string) (BrowserModel, tea.Cmd) {
	set := m.liveSetForKind(kind)
	if set != nil {
		set.Stop()
	}
	if m.active && m.resourceType == kind {
		m.loading = false
		m.err = ErrLiveUpdatesStopped
		m.errBanner = ErrLiveUpdatesStopped.Error()
	}
	return m, nil
}

func (m BrowserModel) updateBrowserLiveMessage(message tea.Msg) (BrowserModel, tea.Cmd) {
	switch message := message.(type) {
	case liveSnapshotMsg[cluster.Pod]:
		return m, applyBrowserLiveState(&m, &m.podLive, &m.pods, resourceTypePods, message.State)
	case liveSnapshotMsg[cluster.Deployment]:
		return m, applyBrowserLiveState(&m, &m.deploymentLive, &m.deployments, resourceTypeDeployments, message.State)
	case liveSnapshotMsg[cluster.Ingress]:
		return m, applyBrowserLiveState(&m, &m.ingressLive, &m.ingresses, resourceTypeIngresses, message.State)
	case liveSnapshotMsg[cluster.NetworkPolicy]:
		return m, applyBrowserLiveState(&m, &m.networkPolicyLive, &m.networkpolicies, resourceTypeNetworkPolicies, message.State)
	case liveSnapshotMsg[cluster.PersistentVolumeClaim]:
		return m, applyBrowserLiveState(&m, &m.persistentVolumeClaimLive, &m.pvcs, resourceTypePVCs, message.State)
	default:
		return m.updateScheduledBrowserLiveMessage(message)
	}
}

func (m BrowserModel) updateScheduledBrowserLiveMessage(message tea.Msg) (BrowserModel, tea.Cmd) {
	switch message := message.(type) {
	case liveSnapshotMsg[cluster.CronJob]:
		return m, applyBrowserLiveState(&m, &m.cronJobLive, &m.cronjobs, resourceTypeCronJobs, message.State)
	case liveSnapshotMsg[cluster.HPA]:
		return m, applyBrowserLiveState(&m, &m.horizontalPodAutoscalerLive, &m.hpas, resourceTypeHPAs, message.State)
	case liveSnapshotMsg[cluster.Secret]:
		return m, applyBrowserLiveState(&m, &m.secretLive, &m.secrets, resourceTypeSecrets, message.State)
	case liveSnapshotMsg[cluster.ReplicaSet]:
		return m, applyBrowserLiveState(&m, &m.replicaSetLive, &m.replicasets, resourceTypeReplicaSets, message.State)
	default:
		return m.updateBrowserLifecycleMessage(message)
	}
}

func applyBrowserLiveState[T liveResource](
	model *BrowserModel,
	supervisor *liveSupervisor[T],
	destination *[]T,
	resourceType string,
	state resourceLiveState[T],
) tea.Cmd {
	if model.resourceType != resourceType {
		supervisor.Stop()
		return nil
	}
	if state.Err != nil {
		model.err = state.Err
		model.errBanner = sanitizeTerminalLine(state.Err.Error())
		model.loading = false
		return supervisor.Pull()
	}
	model.err = nil
	model.errBanner = ""
	if state.Ready {
		*destination = slices.Clone(state.Items)
		model.loading = false
		model.statusMsg = theme.Success.Render(fmt.Sprintf("Loaded %d %s", len(state.Items), displayKind(resourceType, len(state.Items))))
		model.refreshRows(model.resourceTable.Cursor())
	}
	return supervisor.Pull()
}

type browserFetchResource interface {
	cluster.Pod | cluster.Deployment | cluster.Service | cluster.StatefulSet |
		cluster.DaemonSet | cluster.ConfigMap | cluster.Node | cluster.Job |
		cluster.Ingress | cluster.NetworkPolicy | cluster.PersistentVolumeClaim |
		cluster.CronJob | cluster.HPA | cluster.Secret | cluster.ReplicaSet | cluster.RBAC
}

func applyTypedFetchResult[T browserFetchResource](model *BrowserModel, kind string, items *[]T, payload []T, err error) {
	model.loading = false
	if err != nil {
		model.err = err
		model.errBanner = sanitizeTerminalLine(err.Error())
		return
	}
	*items = payload
	model.err = nil
	model.errBanner = ""
	model.statusMsg = theme.Success.Render(fmt.Sprintf("Loaded %d %s", len(payload), displayKind(kind, len(payload))))
	model.rebuildTable()
}

var kindSingulars = map[string]string{
	resourceTypeIngresses:       resourceKindIngress,
	resourceTypeNetworkPolicies: resourceKindNetworkPolicy,
}

func displayKind(kind string, count int) string {
	if count != 1 {
		return kind
	}
	if singular, ok := kindSingulars[kind]; ok {
		return singular
	}
	if len(kind) > 1 && kind[len(kind)-1] == 's' {
		return kind[:len(kind)-1]
	}
	return kind
}
