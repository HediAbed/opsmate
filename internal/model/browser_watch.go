package model

import (
	"fmt"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/service"
	"github.com/HediAbed/opsmate/internal/theme"
)

type browserWatchClosedMsg struct {
	Kind string
}

type browserReconnectMsg struct {
	Kind       string
	Namespace  string
	Generation uint64
}

// liveResourceKinds lists resources backed by watch streams.
var liveResourceKinds = []string{
	"pods",
	"deployments",
	"ingresses",
	"networkpolicies",
	"pvcs",
	"cronjobs",
	"hpas",
	"secrets",
	"replicasets",
}

type closableWatcher interface {
	Stop()
	nextDelay() time.Duration
	Owns(supervisedWatchMsg) bool
	OwnsGeneration(uint64) bool
	Generation() uint64
}

func (m *BrowserModel) closableForKind(kind string) closableWatcher {
	switch kind {
	case "pods":
		return &m.podWatcher
	case "deployments":
		return &m.deploymentWatcher
	case "ingresses":
		return &m.ingressWatcher
	case "networkpolicies":
		return &m.networkPolicyWatcher
	case "pvcs":
		return &m.pvcWatcher
	case "cronjobs":
		return &m.cronJobWatcher
	case "hpas":
		return &m.hpaWatcher
	case "secrets":
		return &m.secretWatcher
	case "replicasets":
		return &m.replicaSetWatcher
	}
	return nil
}

var resourceWatchStarters = map[string]func(m *BrowserModel) tea.Cmd{
	"pods": func(m *BrowserModel) tea.Cmd {
		return m.podWatcher.SetWithClose(
			service.WatchPods(freshContext(), m.namespace),
			browserWatchClosedMsg{Kind: "pods"},
		)
	},
	"deployments": func(m *BrowserModel) tea.Cmd {
		return m.deploymentWatcher.SetWithClose(
			service.WatchDeployments(freshContext(), m.namespace),
			browserWatchClosedMsg{Kind: "deployments"},
		)
	},
	"ingresses": func(m *BrowserModel) tea.Cmd {
		return m.ingressWatcher.SetWithClose(
			service.WatchIngresses(freshContext(), m.namespace),
			browserWatchClosedMsg{Kind: "ingresses"},
		)
	},
	"networkpolicies": func(m *BrowserModel) tea.Cmd {
		return m.networkPolicyWatcher.SetWithClose(
			service.WatchNetworkPolicies(freshContext(), m.namespace),
			browserWatchClosedMsg{Kind: "networkpolicies"},
		)
	},
	"pvcs": func(m *BrowserModel) tea.Cmd {
		return m.pvcWatcher.SetWithClose(
			service.WatchPVCs(freshContext(), m.namespace),
			browserWatchClosedMsg{Kind: "pvcs"},
		)
	},
	"cronjobs": func(m *BrowserModel) tea.Cmd {
		return m.cronJobWatcher.SetWithClose(
			service.WatchCronJobs(freshContext(), m.namespace),
			browserWatchClosedMsg{Kind: "cronjobs"},
		)
	},
	"hpas": func(m *BrowserModel) tea.Cmd {
		return m.hpaWatcher.SetWithClose(
			service.WatchHPAs(freshContext(), m.namespace),
			browserWatchClosedMsg{Kind: "hpas"},
		)
	},
	"secrets": func(m *BrowserModel) tea.Cmd {
		return m.secretWatcher.SetWithClose(
			service.WatchSecrets(freshContext(), m.namespace),
			browserWatchClosedMsg{Kind: "secrets"},
		)
	},
	"replicasets": func(m *BrowserModel) tea.Cmd {
		return m.replicaSetWatcher.SetWithClose(
			service.WatchReplicaSets(freshContext(), m.namespace),
			browserWatchClosedMsg{Kind: "replicasets"},
		)
	},
}

func (m *BrowserModel) startResourceWatch() tea.Cmd {
	if start := resourceWatchStarters[m.resourceType]; start != nil {
		return start(m)
	}
	return nil
}

func (m *BrowserModel) stopAllWatchers() {
	for _, kind := range liveResourceKinds {
		if w := m.closableForKind(kind); w != nil {
			w.Stop()
		}
	}
}

// Activate refreshes the visible resource and starts its watcher.
func (m *BrowserModel) Activate() tea.Cmd {
	m.active = true
	cmds := []tea.Cmd{m.fetchCurrentResources()}
	if watchCmd := m.startResourceWatch(); watchCmd != nil {
		cmds = append(cmds, watchCmd)
	}
	return tea.Batch(cmds...)
}

// Deactivate stops browser-owned processes.
func (m *BrowserModel) Deactivate() {
	m.active = false
	m.stopAllWatchers()
	if m.shellSession != nil {
		closed, _ := m.closeShell()
		*m = closed
	}
}

func (m BrowserModel) handleSupervisedWatchMessage(msg supervisedWatchMsg) (BrowserModel, tea.Cmd) {
	for _, kind := range liveResourceKinds {
		watcher := m.closableForKind(kind)
		if watcher != nil && watcher.Owns(msg) {
			return m.Update(msg.payload)
		}
	}
	return m, nil
}

func (m BrowserModel) ownsSupervisedWatchMessage(msg supervisedWatchMsg) bool {
	for _, kind := range liveResourceKinds {
		watcher := m.closableForKind(kind)
		if watcher != nil && watcher.Owns(msg) {
			return true
		}
	}
	return false
}

func podKey(p service.Pod) string { return p.Namespace + "/" + p.Name }

func deploymentKey(d service.Deployment) string { return d.Namespace + "/" + d.Name }

func ingressKey(i service.Ingress) string { return i.Namespace + "/" + i.Name }

func networkPolicyKey(np service.NetworkPolicy) string { return np.Namespace + "/" + np.Name }

func pvcKey(p service.PersistentVolumeClaim) string { return p.Namespace + "/" + p.Name }

func cronJobKey(c service.CronJob) string { return c.Namespace + "/" + c.Name }

func hpaKey(h service.HPA) string { return h.Namespace + "/" + h.Name }

func secretKey(s service.Secret) string { return s.Namespace + "/" + s.Name }

func replicaSetKey(r service.ReplicaSet) string { return r.Namespace + "/" + r.Name }

func upsertByName[T service.WatchResource](items []T, item T, key func(T) string) []T {
	target := key(item)
	for i := range items {
		if key(items[i]) == target {
			updated := slices.Clone(items)
			updated[i] = item
			return updated
		}
	}
	return append(slices.Clone(items), item)
}

func removeByName[T service.WatchResource](items []T, item T, key func(T) string) []T {
	target := key(item)
	for i := range items {
		if key(items[i]) == target {
			updated := slices.Clone(items)
			copy(updated[i:], updated[i+1:])
			return updated[:len(updated)-1]
		}
	}
	return items
}

func applyTypedWatchEvent[T service.WatchResource](items *[]T, ev service.WatchEvent[T], keyFn func(T) string) {
	switch ev.Kind {
	case service.WatchAdded, service.WatchModified:
		*items = upsertByName(*items, ev.Item, keyFn)
	case service.WatchDeleted:
		*items = removeByName(*items, ev.Item, keyFn)
	case service.WatchBookmark, service.WatchClosed, service.WatchErrored:
	}
}

// items must point to a field on m so the returned model contains the merge.
func handleTypedWatchEvent[T service.WatchResource](
	m *BrowserModel,
	watcher *watchSupervisor[T],
	items *[]T,
	keyFn func(T) string,
	expectedKind string,
	msg service.WatchEventMsg[T],
) tea.Cmd {
	if m.resourceType != expectedKind {
		return nil
	}
	switch msg.Event.Kind {
	case service.WatchErrored:
		if msg.Event.Err != nil {
			m.errBanner = sanitizeTerminalLine(msg.Event.Err.Error())
		}
		return watcher.Pull()
	case service.WatchClosed:
		return watcher.Pull()
	case service.WatchBookmark:
		watcher.MarkHealthy()
		return watcher.Pull()
	case service.WatchAdded, service.WatchModified, service.WatchDeleted:
	}
	watcher.MarkHealthy()
	applyTypedWatchEvent(items, msg.Event, keyFn)
	m.loading = false
	m.refreshRows(m.resourceTable.Cursor())
	return watcher.Pull()
}

func (m BrowserModel) handleSharedWatchClosed(kind string) (BrowserModel, tea.Cmd) {
	if !m.active || m.resourceType != kind {
		return m, nil
	}
	w := m.closableForKind(kind)
	if w == nil {
		return m, nil
	}
	return m, reconnectAfter(w.nextDelay(), browserReconnectMsg{
		Kind:       kind,
		Namespace:  m.namespace,
		Generation: w.Generation(),
	})
}

type browserFetchResource interface {
	service.Pod | service.Deployment | service.Service | service.StatefulSet |
		service.DaemonSet | service.ConfigMap | service.Node | service.Job |
		service.Ingress | service.NetworkPolicy | service.PersistentVolumeClaim |
		service.CronJob | service.HPA | service.Secret | service.ReplicaSet | service.RBAC
}

func applyTypedFetchResult[T browserFetchResource](m *BrowserModel, kind string, items *[]T, payload []T, err error) {
	m.loading = false
	if err != nil {
		m.err = err
		m.errBanner = sanitizeTerminalLine(service.SanitizeKubectlStderr(err.Error()))
		return
	}
	*items = payload
	m.err = nil
	m.errBanner = ""
	m.statusMsg = theme.Success.Render(fmt.Sprintf("Loaded %d %s", len(payload), displayKind(kind, len(payload))))
	m.rebuildTable()
}

// kindSingulars contains plurals that cannot be reduced by trimming "s".
var kindSingulars = map[string]string{
	"ingresses":       "ingress",
	"networkpolicies": "networkpolicy",
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

// handleSharedReconnect rejects stale messages before replacing a watcher.
func (m BrowserModel) handleSharedReconnect(msg browserReconnectMsg) (BrowserModel, tea.Cmd) {
	if !m.active || m.resourceType != msg.Kind || m.namespace != msg.Namespace {
		return m, nil
	}
	start, ok := resourceWatchStarters[msg.Kind]
	if !ok {
		return m, nil
	}
	watcher := m.closableForKind(msg.Kind)
	if watcher == nil || !watcher.OwnsGeneration(msg.Generation) {
		return m, nil
	}
	return m, start(&m)
}
