package model

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/service"
)

// resourceBinding groups browser behavior for one resource kind.
type resourceBinding struct {
	Singular     string
	RowsOf       func(m *BrowserModel) []table.Row
	IdentitiesOf func(m *BrowserModel) []resourceIdentity
	// WideRowsOf is optional; nil falls back to RowsOf.
	WideRowsOf func(m *BrowserModel) []table.Row
	Fetch      func(namespace string) tea.Cmd
	Clear      func(m *BrowserModel)
	Count      func(m *BrowserModel) int
}

var resourceCatalog = map[string]resourceBinding{
	"pods": {
		Singular:     "pod",
		RowsOf:       podRows,
		WideRowsOf:   podRowsWide,
		IdentitiesOf: func(m *BrowserModel) []resourceIdentity { return namespacedIdentities(m.pods, "pod", podNamespacePair) },
		Fetch:        service.FetchPods,
		Clear:        func(m *BrowserModel) { m.pods = nil },
		Count:        func(m *BrowserModel) int { return len(m.pods) },
	},
	"deployments": {
		Singular:   "deployment",
		RowsOf:     deploymentRows,
		WideRowsOf: deploymentRowsWide,
		IdentitiesOf: func(m *BrowserModel) []resourceIdentity {
			return namespacedIdentities(m.deployments, "deployment", deploymentNamespacePair)
		},
		Fetch: service.FetchDeployments,
		Clear: func(m *BrowserModel) { m.deployments = nil },
		Count: func(m *BrowserModel) int { return len(m.deployments) },
	},
	"services": {
		Singular:   "service",
		RowsOf:     serviceRows,
		WideRowsOf: serviceRowsWide,
		IdentitiesOf: func(m *BrowserModel) []resourceIdentity {
			return namespacedIdentities(m.services, "service", serviceNamespacePair)
		},
		Fetch: service.FetchServices,
		Clear: func(m *BrowserModel) { m.services = nil },
		Count: func(m *BrowserModel) int { return len(m.services) },
	},
	"statefulsets": {
		Singular: "statefulset",
		RowsOf:   statefulSetRows,
		IdentitiesOf: func(m *BrowserModel) []resourceIdentity {
			return namespacedIdentities(m.statefulsets, "statefulset", statefulSetNamespacePair)
		},
		Fetch: service.FetchStatefulSets,
		Clear: func(m *BrowserModel) { m.statefulsets = nil },
		Count: func(m *BrowserModel) int { return len(m.statefulsets) },
	},
	"daemonsets": {
		Singular: "daemonset",
		RowsOf:   daemonSetRows,
		IdentitiesOf: func(m *BrowserModel) []resourceIdentity {
			return namespacedIdentities(m.daemonsets, "daemonset", daemonSetNamespacePair)
		},
		Fetch: service.FetchDaemonSets,
		Clear: func(m *BrowserModel) { m.daemonsets = nil },
		Count: func(m *BrowserModel) int { return len(m.daemonsets) },
	},
	"configmaps": {
		Singular: "configmap",
		RowsOf:   configMapRows,
		IdentitiesOf: func(m *BrowserModel) []resourceIdentity {
			return namespacedIdentities(m.configmaps, "configmap", configMapNamespacePair)
		},
		Fetch: service.FetchConfigMaps,
		Clear: func(m *BrowserModel) { m.configmaps = nil },
		Count: func(m *BrowserModel) int { return len(m.configmaps) },
	},
	"nodes": {
		Singular:     "node",
		RowsOf:       nodeRows,
		WideRowsOf:   nodeRowsWide,
		IdentitiesOf: nodeIdentities,
		Fetch:        func(string) tea.Cmd { return service.FetchNodes() },
		Clear:        func(m *BrowserModel) { m.nodes = nil },
		Count:        func(m *BrowserModel) int { return len(m.nodes) },
	},
	"jobs": {
		Singular:     "job",
		RowsOf:       jobRows,
		IdentitiesOf: func(m *BrowserModel) []resourceIdentity { return namespacedIdentities(m.jobs, "job", jobNamespacePair) },
		Fetch:        service.FetchJobs,
		Clear:        func(m *BrowserModel) { m.jobs = nil },
		Count:        func(m *BrowserModel) int { return len(m.jobs) },
	},
	"ingresses": {
		Singular: "ingress",
		RowsOf:   ingressRows,
		IdentitiesOf: func(m *BrowserModel) []resourceIdentity {
			return namespacedIdentities(m.ingresses, "ingress", ingressNamespacePair)
		},
		Fetch: service.FetchIngresses,
		Clear: func(m *BrowserModel) { m.ingresses = nil },
		Count: func(m *BrowserModel) int { return len(m.ingresses) },
	},
	"networkpolicies": {
		Singular: "networkpolicy",
		RowsOf:   networkPolicyRows,
		IdentitiesOf: func(m *BrowserModel) []resourceIdentity {
			return namespacedIdentities(m.networkpolicies, "networkpolicy", networkPolicyNamespacePair)
		},
		Fetch: service.FetchNetworkPolicies,
		Clear: func(m *BrowserModel) { m.networkpolicies = nil },
		Count: func(m *BrowserModel) int { return len(m.networkpolicies) },
	},
	"pvcs": {
		Singular:     "pvc",
		RowsOf:       pvcRows,
		IdentitiesOf: func(m *BrowserModel) []resourceIdentity { return namespacedIdentities(m.pvcs, "pvc", pvcNamespacePair) },
		Fetch:        service.FetchPVCs,
		Clear:        func(m *BrowserModel) { m.pvcs = nil },
		Count:        func(m *BrowserModel) int { return len(m.pvcs) },
	},
	"cronjobs": {
		Singular: "cronjob",
		RowsOf:   cronJobRows,
		IdentitiesOf: func(m *BrowserModel) []resourceIdentity {
			return namespacedIdentities(m.cronjobs, "cronjob", cronJobNamespacePair)
		},
		Fetch: service.FetchCronJobs,
		Clear: func(m *BrowserModel) { m.cronjobs = nil },
		Count: func(m *BrowserModel) int { return len(m.cronjobs) },
	},
	"hpas": {
		Singular:     "hpa",
		RowsOf:       hpaRows,
		IdentitiesOf: func(m *BrowserModel) []resourceIdentity { return namespacedIdentities(m.hpas, "hpa", hpaNamespacePair) },
		Fetch:        service.FetchHPAs,
		Clear:        func(m *BrowserModel) { m.hpas = nil },
		Count:        func(m *BrowserModel) int { return len(m.hpas) },
	},
	"secrets": {
		Singular: "secret",
		RowsOf:   secretRows,
		IdentitiesOf: func(m *BrowserModel) []resourceIdentity {
			return namespacedIdentities(m.secrets, "secret", secretNamespacePair)
		},
		Fetch: service.FetchSecrets,
		Clear: func(m *BrowserModel) { m.secrets = nil },
		Count: func(m *BrowserModel) int { return len(m.secrets) },
	},
	"replicasets": {
		Singular: "replicaset",
		RowsOf:   replicaSetRows,
		IdentitiesOf: func(m *BrowserModel) []resourceIdentity {
			return namespacedIdentities(m.replicasets, "replicaset", replicaSetNamespacePair)
		},
		Fetch: service.FetchReplicaSets,
		Clear: func(m *BrowserModel) { m.replicasets = nil },
		Count: func(m *BrowserModel) int { return len(m.replicasets) },
	},
	"rbac": {
		Singular:     "rbac",
		RowsOf:       rbacRows,
		IdentitiesOf: rbacIdentities,
		Fetch:        service.FetchRBAC,
		Clear:        func(m *BrowserModel) { m.rbac = nil },
		Count:        func(m *BrowserModel) int { return len(m.rbac) },
	},
}

func podNamespacePair(p service.Pod) (string, string)                 { return p.Name, p.Namespace }
func deploymentNamespacePair(d service.Deployment) (string, string)   { return d.Name, d.Namespace }
func serviceNamespacePair(s service.Service) (string, string)         { return s.Name, s.Namespace }
func statefulSetNamespacePair(s service.StatefulSet) (string, string) { return s.Name, s.Namespace }
func daemonSetNamespacePair(d service.DaemonSet) (string, string)     { return d.Name, d.Namespace }
func configMapNamespacePair(c service.ConfigMap) (string, string)     { return c.Name, c.Namespace }
func jobNamespacePair(j service.Job) (string, string)                 { return j.Name, j.Namespace }
func ingressNamespacePair(i service.Ingress) (string, string)         { return i.Name, i.Namespace }
func networkPolicyNamespacePair(np service.NetworkPolicy) (string, string) {
	return np.Name, np.Namespace
}
func pvcNamespacePair(p service.PersistentVolumeClaim) (string, string) { return p.Name, p.Namespace }
func cronJobNamespacePair(c service.CronJob) (string, string)           { return c.Name, c.Namespace }
func hpaNamespacePair(h service.HPA) (string, string)                   { return h.Name, h.Namespace }
func secretNamespacePair(s service.Secret) (string, string)             { return s.Name, s.Namespace }
func replicaSetNamespacePair(r service.ReplicaSet) (string, string)     { return r.Name, r.Namespace }

type namespacedBrowserResource interface {
	service.Pod |
		service.Deployment |
		service.Service |
		service.StatefulSet |
		service.DaemonSet |
		service.ConfigMap |
		service.Job |
		service.Ingress |
		service.NetworkPolicy |
		service.PersistentVolumeClaim |
		service.CronJob |
		service.HPA |
		service.Secret |
		service.ReplicaSet
}

func namespacedIdentities[T namespacedBrowserResource](
	items []T,
	kind string,
	pair func(T) (string, string),
) []resourceIdentity {
	identities := make([]resourceIdentity, 0, len(items))
	for _, item := range items {
		name, namespace := pair(item)
		identities = append(identities, resourceIdentity{Kind: kind, Namespace: namespace, Name: name})
	}
	return identities
}

func nodeIdentities(m *BrowserModel) []resourceIdentity {
	identities := make([]resourceIdentity, 0, len(m.nodes))
	for _, node := range m.nodes {
		identities = append(identities, resourceIdentity{Kind: "node", Name: node.Name})
	}
	return identities
}

func rbacIdentities(m *BrowserModel) []resourceIdentity {
	identities := make([]resourceIdentity, 0, len(m.rbac))
	for _, resource := range m.rbac {
		identities = append(identities, resourceIdentity{
			Kind:      strings.ToLower(resource.Kind),
			Namespace: resource.Namespace,
			Name:      resource.Name,
		})
	}
	return identities
}

func podRows(m *BrowserModel) []table.Row {
	rows := make([]table.Row, 0, len(m.pods))
	for _, p := range m.pods {
		rows = append(rows, table.Row{
			m.displayIdentity(resourceIdentity{Kind: "pod", Namespace: p.Namespace, Name: p.Name}), p.Status, p.Ready, strconv.Itoa(p.Restarts), p.Age, p.Node,
		})
	}
	return rows
}

func deploymentRows(m *BrowserModel) []table.Row {
	rows := make([]table.Row, 0, len(m.deployments))
	for _, d := range m.deployments {
		rows = append(rows, table.Row{
			m.displayIdentity(resourceIdentity{Kind: "deployment", Namespace: d.Namespace, Name: d.Name}), d.Ready, strconv.Itoa(d.UpToDate), strconv.Itoa(d.Available), d.Age,
		})
	}
	return rows
}

func serviceRows(m *BrowserModel) []table.Row {
	rows := make([]table.Row, 0, len(m.services))
	for _, s := range m.services {
		rows = append(rows, table.Row{
			m.displayIdentity(resourceIdentity{Kind: "service", Namespace: s.Namespace, Name: s.Name}), s.Type, s.ClusterIP, s.Ports, s.Age,
		})
	}
	return rows
}

func statefulSetRows(m *BrowserModel) []table.Row {
	rows := make([]table.Row, 0, len(m.statefulsets))
	for _, s := range m.statefulsets {
		rows = append(rows, table.Row{
			m.displayIdentity(resourceIdentity{Kind: "statefulset", Namespace: s.Namespace, Name: s.Name}), s.Ready, strconv.Itoa(s.Replicas), s.Age,
		})
	}
	return rows
}

func daemonSetRows(m *BrowserModel) []table.Row {
	rows := make([]table.Row, 0, len(m.daemonsets))
	for _, d := range m.daemonsets {
		rows = append(rows, table.Row{
			m.displayIdentity(resourceIdentity{Kind: "daemonset", Namespace: d.Namespace, Name: d.Name}),
			strconv.Itoa(d.Desired),
			strconv.Itoa(d.Current),
			strconv.Itoa(d.Ready),
			strconv.Itoa(d.Available),
			d.Age,
		})
	}
	return rows
}

func configMapRows(m *BrowserModel) []table.Row {
	rows := make([]table.Row, 0, len(m.configmaps))
	for _, c := range m.configmaps {
		rows = append(rows, table.Row{
			m.displayIdentity(resourceIdentity{Kind: "configmap", Namespace: c.Namespace, Name: c.Name}), strconv.Itoa(c.Data), c.Age,
		})
	}
	return rows
}

func nodeRows(m *BrowserModel) []table.Row {
	rows := make([]table.Row, 0, len(m.nodes))
	for _, n := range m.nodes {
		rows = append(rows, table.Row{
			m.displayIdentity(resourceIdentity{Kind: "node", Name: n.Name}), n.Status, n.Roles, n.Version, n.Age,
		})
	}
	return rows
}

func jobRows(m *BrowserModel) []table.Row {
	rows := make([]table.Row, 0, len(m.jobs))
	for _, j := range m.jobs {
		rows = append(rows, table.Row{
			m.displayIdentity(resourceIdentity{Kind: "job", Namespace: j.Namespace, Name: j.Name}), j.Completions, j.Duration, j.Status, j.Age,
		})
	}
	return rows
}

func ingressRows(m *BrowserModel) []table.Row {
	rows := make([]table.Row, 0, len(m.ingresses))
	for _, ing := range m.ingresses {
		rows = append(rows, table.Row{
			m.displayIdentity(resourceIdentity{Kind: "ingress", Namespace: ing.Namespace, Name: ing.Name}), ing.Class, ing.Hosts, ing.Address, ing.Ports, ing.Age,
		})
	}
	return rows
}

func networkPolicyRows(m *BrowserModel) []table.Row {
	rows := make([]table.Row, 0, len(m.networkpolicies))
	for _, np := range m.networkpolicies {
		rows = append(rows, table.Row{
			m.displayIdentity(resourceIdentity{Kind: "networkpolicy", Namespace: np.Namespace, Name: np.Name}),
			formatLabelSelector(np.PodSelector),
			strings.Join(np.PolicyTypes, ","),
			np.Age,
		})
	}
	return rows
}

func pvcRows(m *BrowserModel) []table.Row {
	rows := make([]table.Row, 0, len(m.pvcs))
	for _, p := range m.pvcs {
		rows = append(rows, table.Row{
			m.displayIdentity(resourceIdentity{Kind: "pvc", Namespace: p.Namespace, Name: p.Name}),
			p.Status,
			p.Volume,
			p.Capacity,
			strings.Join(formatPVCAccessModes(p.AccessModes), ","),
			p.StorageClass,
			p.Age,
		})
	}
	return rows
}

func cronJobRows(m *BrowserModel) []table.Row {
	rows := make([]table.Row, 0, len(m.cronjobs))
	for _, c := range m.cronjobs {
		rows = append(rows, table.Row{
			m.displayIdentity(resourceIdentity{Kind: "cronjob", Namespace: c.Namespace, Name: c.Name}),
			c.Schedule,
			formatBoolColumn(c.Suspend),
			strconv.Itoa(c.Active),
			c.LastSchedule,
			c.Age,
		})
	}
	return rows
}

func hpaRows(m *BrowserModel) []table.Row {
	rows := make([]table.Row, 0, len(m.hpas))
	for _, h := range m.hpas {
		targets := make([]string, 0, len(h.Targets))
		for _, t := range h.Targets {
			targets = append(targets, t.String())
		}
		rows = append(rows, table.Row{
			m.displayIdentity(resourceIdentity{Kind: "hpa", Namespace: h.Namespace, Name: h.Name}),
			h.Reference.String(),
			strings.Join(targets, ","),
			strconv.Itoa(h.MinReplicas),
			strconv.Itoa(h.MaxReplicas),
			strconv.Itoa(h.Replicas),
			h.Age,
		})
	}
	return rows
}

func secretRows(m *BrowserModel) []table.Row {
	rows := make([]table.Row, 0, len(m.secrets))
	for _, s := range m.secrets {
		rows = append(rows, table.Row{
			m.displayIdentity(resourceIdentity{Kind: "secret", Namespace: s.Namespace, Name: s.Name}), s.Type, strconv.Itoa(s.Data), s.Age,
		})
	}
	return rows
}

func replicaSetRows(m *BrowserModel) []table.Row {
	rows := make([]table.Row, 0, len(m.replicasets))
	for _, r := range m.replicasets {
		rows = append(rows, table.Row{
			m.displayIdentity(resourceIdentity{Kind: "replicaset", Namespace: r.Namespace, Name: r.Name}),
			strconv.Itoa(r.Desired),
			strconv.Itoa(r.Current),
			strconv.Itoa(r.Ready),
			r.Age,
		})
	}
	return rows
}

func rbacRows(m *BrowserModel) []table.Row {
	rows := make([]table.Row, 0, len(m.rbac))
	for _, r := range m.rbac {
		rows = append(rows, table.Row{
			r.Kind, m.displayIdentity(resourceIdentity{Kind: strings.ToLower(r.Kind), Namespace: r.Namespace, Name: r.Name}), strconv.Itoa(r.Count), r.Scope, r.Age,
		})
	}
	return rows
}

func podRowsWide(m *BrowserModel) []table.Row {
	rows := make([]table.Row, 0, len(m.pods))
	for _, p := range m.pods {
		rows = append(rows, table.Row{
			m.displayIdentity(resourceIdentity{Kind: "pod", Namespace: p.Namespace, Name: p.Name}), p.Status, p.Ready, strconv.Itoa(p.Restarts), p.Age, p.IP, p.Node,
		})
	}
	return rows
}

func deploymentRowsWide(m *BrowserModel) []table.Row {
	rows := make([]table.Row, 0, len(m.deployments))
	for _, d := range m.deployments {
		rows = append(rows, table.Row{
			m.displayIdentity(resourceIdentity{Kind: "deployment", Namespace: d.Namespace, Name: d.Name}),
			d.Ready,
			strconv.Itoa(d.UpToDate),
			strconv.Itoa(d.Available),
			d.Age,
			strings.Join(d.Containers, ","),
			strings.Join(d.Images, ","),
			d.Selector,
		})
	}
	return rows
}

func serviceRowsWide(m *BrowserModel) []table.Row {
	rows := make([]table.Row, 0, len(m.services))
	for _, s := range m.services {
		rows = append(rows, table.Row{
			m.displayIdentity(resourceIdentity{Kind: "service", Namespace: s.Namespace, Name: s.Name}), s.Type, s.ClusterIP, s.ExternalIP, s.Ports, s.Age, s.Selector,
		})
	}
	return rows
}

func nodeRowsWide(m *BrowserModel) []table.Row {
	rows := make([]table.Row, 0, len(m.nodes))
	for _, n := range m.nodes {
		rows = append(rows, table.Row{
			m.displayIdentity(resourceIdentity{Kind: "node", Name: n.Name}),
			n.Status,
			n.Roles,
			n.Version,
			n.Age,
			n.InternalIP,
			n.OSImage,
			n.Kernel,
			n.Runtime,
		})
	}
	return rows
}
