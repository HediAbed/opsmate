package ui

import (
	"strings"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"

	"github.com/HediAbed/opsmate/internal/cluster"
)

// resourceBinding groups browser behavior for one resource kind.
type resourceBinding struct {
	Singular     string
	RowsOf       func(model *BrowserModel) []table.Row
	IdentitiesOf func(model *BrowserModel) []resourceIdentity
	// WideRowsOf is optional; nil falls back to RowsOf.
	WideRowsOf func(model *BrowserModel) []table.Row
	Fetch      func(clusterCommands, string) tea.Cmd
	Clear      func(model *BrowserModel)
	Count      func(model *BrowserModel) int
}

var resourceCatalog = map[string]resourceBinding{
	resourceTypePods: {
		Singular:   resourceKindPod,
		RowsOf:     podRows,
		WideRowsOf: podRowsWide,
		IdentitiesOf: func(model *BrowserModel) []resourceIdentity {
			return namespacedIdentities(model.pods, resourceKindPod, podNamespacePair)
		},
		Fetch: clusterCommands.FetchPods,
		Clear: func(model *BrowserModel) { model.pods = nil },
		Count: func(model *BrowserModel) int { return len(model.pods) },
	},
	resourceTypeDeployments: {
		Singular:   resourceKindDeployment,
		RowsOf:     deploymentRows,
		WideRowsOf: deploymentRowsWide,
		IdentitiesOf: func(model *BrowserModel) []resourceIdentity {
			return namespacedIdentities(model.deployments, resourceKindDeployment, deploymentNamespacePair)
		},
		Fetch: clusterCommands.FetchDeployments,
		Clear: func(model *BrowserModel) { model.deployments = nil },
		Count: func(model *BrowserModel) int { return len(model.deployments) },
	},
	resourceTypeServices: {
		Singular:   resourceKindService,
		RowsOf:     serviceRows,
		WideRowsOf: serviceRowsWide,
		IdentitiesOf: func(model *BrowserModel) []resourceIdentity {
			return namespacedIdentities(model.services, resourceKindService, serviceNamespacePair)
		},
		Fetch: clusterCommands.FetchServices,
		Clear: func(model *BrowserModel) { model.services = nil },
		Count: func(model *BrowserModel) int { return len(model.services) },
	},
	resourceTypeStatefulSets: {
		Singular: resourceKindStatefulSet,
		RowsOf:   statefulSetRows,
		IdentitiesOf: func(model *BrowserModel) []resourceIdentity {
			return namespacedIdentities(model.statefulsets, resourceKindStatefulSet, statefulSetNamespacePair)
		},
		Fetch: clusterCommands.FetchStatefulSets,
		Clear: func(model *BrowserModel) { model.statefulsets = nil },
		Count: func(model *BrowserModel) int { return len(model.statefulsets) },
	},
	resourceTypeDaemonSets: {
		Singular: resourceKindDaemonSet,
		RowsOf:   daemonSetRows,
		IdentitiesOf: func(model *BrowserModel) []resourceIdentity {
			return namespacedIdentities(model.daemonsets, resourceKindDaemonSet, daemonSetNamespacePair)
		},
		Fetch: clusterCommands.FetchDaemonSets,
		Clear: func(model *BrowserModel) { model.daemonsets = nil },
		Count: func(model *BrowserModel) int { return len(model.daemonsets) },
	},
	resourceTypeConfigMaps: {
		Singular: resourceKindConfigMap,
		RowsOf:   configMapRows,
		IdentitiesOf: func(model *BrowserModel) []resourceIdentity {
			return namespacedIdentities(model.configmaps, resourceKindConfigMap, configMapNamespacePair)
		},
		Fetch: clusterCommands.FetchConfigMaps,
		Clear: func(model *BrowserModel) { model.configmaps = nil },
		Count: func(model *BrowserModel) int { return len(model.configmaps) },
	},
	resourceTypeNodes: {
		Singular:     resourceKindNode,
		RowsOf:       nodeRows,
		WideRowsOf:   nodeRowsWide,
		IdentitiesOf: nodeIdentities,
		Fetch:        func(commands clusterCommands, _ string) tea.Cmd { return commands.FetchNodes() },
		Clear:        func(model *BrowserModel) { model.nodes = nil },
		Count:        func(model *BrowserModel) int { return len(model.nodes) },
	},
	resourceTypeJobs: {
		Singular: resourceKindJob,
		RowsOf:   jobRows,
		IdentitiesOf: func(model *BrowserModel) []resourceIdentity {
			return namespacedIdentities(model.jobs, resourceKindJob, jobNamespacePair)
		},
		Fetch: clusterCommands.FetchJobs,
		Clear: func(model *BrowserModel) { model.jobs = nil },
		Count: func(model *BrowserModel) int { return len(model.jobs) },
	},
	resourceTypeIngresses: {
		Singular: resourceKindIngress,
		RowsOf:   ingressRows,
		IdentitiesOf: func(model *BrowserModel) []resourceIdentity {
			return namespacedIdentities(model.ingresses, resourceKindIngress, ingressNamespacePair)
		},
		Fetch: clusterCommands.FetchIngresses,
		Clear: func(model *BrowserModel) { model.ingresses = nil },
		Count: func(model *BrowserModel) int { return len(model.ingresses) },
	},
	resourceTypeNetworkPolicies: {
		Singular: resourceKindNetworkPolicy,
		RowsOf:   networkPolicyRows,
		IdentitiesOf: func(model *BrowserModel) []resourceIdentity {
			return namespacedIdentities(model.networkpolicies, resourceKindNetworkPolicy, networkPolicyNamespacePair)
		},
		Fetch: clusterCommands.FetchNetworkPolicies,
		Clear: func(model *BrowserModel) { model.networkpolicies = nil },
		Count: func(model *BrowserModel) int { return len(model.networkpolicies) },
	},
	resourceTypePVCs: {
		Singular: resourceKindPVC,
		RowsOf:   pvcRows,
		IdentitiesOf: func(model *BrowserModel) []resourceIdentity {
			return namespacedIdentities(model.pvcs, resourceKindPVC, pvcNamespacePair)
		},
		Fetch: clusterCommands.FetchPVCs,
		Clear: func(model *BrowserModel) { model.pvcs = nil },
		Count: func(model *BrowserModel) int { return len(model.pvcs) },
	},
	resourceTypeCronJobs: {
		Singular: resourceKindCronJob,
		RowsOf:   cronJobRows,
		IdentitiesOf: func(model *BrowserModel) []resourceIdentity {
			return namespacedIdentities(model.cronjobs, resourceKindCronJob, cronJobNamespacePair)
		},
		Fetch: clusterCommands.FetchCronJobs,
		Clear: func(model *BrowserModel) { model.cronjobs = nil },
		Count: func(model *BrowserModel) int { return len(model.cronjobs) },
	},
	resourceTypeHPAs: {
		Singular: resourceKindHPA,
		RowsOf:   hpaRows,
		IdentitiesOf: func(model *BrowserModel) []resourceIdentity {
			return namespacedIdentities(model.hpas, resourceKindHPA, hpaNamespacePair)
		},
		Fetch: clusterCommands.FetchHPAs,
		Clear: func(model *BrowserModel) { model.hpas = nil },
		Count: func(model *BrowserModel) int { return len(model.hpas) },
	},
	resourceTypeSecrets: {
		Singular: resourceKindSecret,
		RowsOf:   secretRows,
		IdentitiesOf: func(model *BrowserModel) []resourceIdentity {
			return namespacedIdentities(model.secrets, resourceKindSecret, secretNamespacePair)
		},
		Fetch: clusterCommands.FetchSecrets,
		Clear: func(model *BrowserModel) { model.secrets = nil },
		Count: func(model *BrowserModel) int { return len(model.secrets) },
	},
	resourceTypeReplicaSets: {
		Singular: resourceKindReplicaSet,
		RowsOf:   replicaSetRows,
		IdentitiesOf: func(model *BrowserModel) []resourceIdentity {
			return namespacedIdentities(model.replicasets, resourceKindReplicaSet, replicaSetNamespacePair)
		},
		Fetch: clusterCommands.FetchReplicaSets,
		Clear: func(model *BrowserModel) { model.replicasets = nil },
		Count: func(model *BrowserModel) int { return len(model.replicasets) },
	},
	resourceTypeRBAC: {
		Singular:     resourceKindRBAC,
		RowsOf:       rbacRows,
		IdentitiesOf: rbacIdentities,
		Fetch:        clusterCommands.FetchRBAC,
		Clear:        func(model *BrowserModel) { model.rbac = nil },
		Count:        func(model *BrowserModel) int { return len(model.rbac) },
	},
}

func podNamespacePair(pod cluster.Pod) (string, string) { return pod.Name, pod.Namespace }
func deploymentNamespacePair(deployment cluster.Deployment) (string, string) {
	return deployment.Name, deployment.Namespace
}
func serviceNamespacePair(service cluster.Service) (string, string) {
	return service.Name, service.Namespace
}
func statefulSetNamespacePair(statefulSet cluster.StatefulSet) (string, string) {
	return statefulSet.Name, statefulSet.Namespace
}
func daemonSetNamespacePair(daemonSet cluster.DaemonSet) (string, string) {
	return daemonSet.Name, daemonSet.Namespace
}
func configMapNamespacePair(configMap cluster.ConfigMap) (string, string) {
	return configMap.Name, configMap.Namespace
}
func jobNamespacePair(job cluster.Job) (string, string) { return job.Name, job.Namespace }
func ingressNamespacePair(ingress cluster.Ingress) (string, string) {
	return ingress.Name, ingress.Namespace
}
func networkPolicyNamespacePair(networkPolicy cluster.NetworkPolicy) (string, string) {
	return networkPolicy.Name, networkPolicy.Namespace
}
func pvcNamespacePair(claim cluster.PersistentVolumeClaim) (string, string) {
	return claim.Name, claim.Namespace
}
func cronJobNamespacePair(cronJob cluster.CronJob) (string, string) {
	return cronJob.Name, cronJob.Namespace
}
func hpaNamespacePair(autoscaler cluster.HPA) (string, string) {
	return autoscaler.Name, autoscaler.Namespace
}
func secretNamespacePair(secret cluster.Secret) (string, string) {
	return secret.Name, secret.Namespace
}
func replicaSetNamespacePair(replicaSet cluster.ReplicaSet) (string, string) {
	return replicaSet.Name, replicaSet.Namespace
}

type namespacedBrowserResource interface {
	cluster.Pod |
		cluster.Deployment |
		cluster.Service |
		cluster.StatefulSet |
		cluster.DaemonSet |
		cluster.ConfigMap |
		cluster.Job |
		cluster.Ingress |
		cluster.NetworkPolicy |
		cluster.PersistentVolumeClaim |
		cluster.CronJob |
		cluster.HPA |
		cluster.Secret |
		cluster.ReplicaSet
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

func nodeIdentities(model *BrowserModel) []resourceIdentity {
	identities := make([]resourceIdentity, 0, len(model.nodes))
	for _, node := range model.nodes {
		identities = append(identities, resourceIdentity{Kind: resourceKindNode, Name: node.Name})
	}
	return identities
}

func rbacIdentities(model *BrowserModel) []resourceIdentity {
	identities := make([]resourceIdentity, 0, len(model.rbac))
	for _, resource := range model.rbac {
		identities = append(identities, resourceIdentity{
			Kind:      strings.ToLower(resource.Kind),
			Namespace: resource.Namespace,
			Name:      resource.Name,
		})
	}
	return identities
}
