package ui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"

	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/kube"
)

type clusterCommands interface {
	FetchPods(string) tea.Cmd
	FetchDeployments(string) tea.Cmd
	FetchEvents(string) tea.Cmd
	FetchPodMetrics(string) tea.Cmd
	FetchServices(string) tea.Cmd
	FetchStatefulSets(string) tea.Cmd
	FetchDaemonSets(string) tea.Cmd
	FetchConfigMaps(string) tea.Cmd
	FetchNodes() tea.Cmd
	FetchJobs(string) tea.Cmd
	FetchIngresses(string) tea.Cmd
	FetchNetworkPolicies(string) tea.Cmd
	FetchPVCs(string) tea.Cmd
	FetchCronJobs(string) tea.Cmd
	FetchHPAs(string) tea.Cmd
	FetchSecrets(string) tea.Cmd
	FetchReplicaSets(string) tea.Cmd
	FetchRBAC(string) tea.Cmd
	FetchCRDs() tea.Cmd
	FetchCRDInstances(cluster.CRD, string) tea.Cmd
	ObservePods(string) (resourceLiveSet[cluster.Pod], error)
	ObserveDeployments(string) (resourceLiveSet[cluster.Deployment], error)
	ObserveEvents(string) (resourceLiveSet[cluster.Event], error)
	ObserveIngresses(string) (resourceLiveSet[cluster.Ingress], error)
	ObserveNetworkPolicies(string) (resourceLiveSet[cluster.NetworkPolicy], error)
	ObservePersistentVolumeClaims(string) (resourceLiveSet[cluster.PersistentVolumeClaim], error)
	ObserveCronJobs(string) (resourceLiveSet[cluster.CronJob], error)
	ObserveHorizontalPodAutoscalers(string) (resourceLiveSet[cluster.HPA], error)
	ObserveSecrets(string) (resourceLiveSet[cluster.Secret], error)
	ObserveReplicaSets(string) (resourceLiveSet[cluster.ReplicaSet], error)
}

type nativeClusterCommands struct {
	parent   context.Context
	reader   kube.ResourceReader
	observer kube.ResourceObserver
	now      func() time.Time
}

func newNativeClusterCommands(parent context.Context, reader kube.ResourceReader, observer kube.ResourceObserver) nativeClusterCommands {
	return nativeClusterCommands{parent: parent, reader: reader, observer: observer, now: time.Now}
}

func (c nativeClusterCommands) FetchPods(namespace string) tea.Cmd {
	return listCommand(c, func(ctx context.Context) ([]corev1.Pod, error) {
		return c.reader.ListPods(ctx, namespace)
	}, projectPods, func(items []cluster.Pod, err error) tea.Msg {
		return cluster.PodsMsg{Pods: items, Err: err}
	})
}

func (c nativeClusterCommands) FetchDeployments(namespace string) tea.Cmd {
	return listCommand(c, func(ctx context.Context) ([]appsv1.Deployment, error) {
		return c.reader.ListDeployments(ctx, namespace)
	}, projectDeployments, func(items []cluster.Deployment, err error) tea.Msg {
		return cluster.DeploymentsMsg{Deployments: items, Err: err}
	})
}

func (c nativeClusterCommands) FetchEvents(namespace string) tea.Cmd {
	return listCommand(c, func(ctx context.Context) ([]corev1.Event, error) {
		return c.reader.ListEvents(ctx, namespace)
	}, projectEvents, func(items []cluster.Event, err error) tea.Msg {
		return cluster.EventsMsg{Events: items, Err: err}
	})
}

func (c nativeClusterCommands) FetchPodMetrics(namespace string) tea.Cmd {
	return listCommand(c, func(ctx context.Context) ([]metricsv1beta1.PodMetrics, error) {
		return c.reader.ListPodMetrics(ctx, namespace)
	}, projectPodMetrics, func(items []cluster.PodMetric, err error) tea.Msg {
		return cluster.MetricsMsg{PodMetrics: items, Err: err}
	})
}

func (c nativeClusterCommands) FetchServices(namespace string) tea.Cmd {
	return listCommand(c, func(ctx context.Context) ([]corev1.Service, error) {
		return c.reader.ListServices(ctx, namespace)
	}, projectServices, func(items []cluster.Service, err error) tea.Msg {
		return cluster.ServicesMsg{Services: items, Err: err}
	})
}

func (c nativeClusterCommands) FetchStatefulSets(namespace string) tea.Cmd {
	return listCommand(c, func(ctx context.Context) ([]appsv1.StatefulSet, error) {
		return c.reader.ListStatefulSets(ctx, namespace)
	}, projectStatefulSets, func(items []cluster.StatefulSet, err error) tea.Msg {
		return cluster.StatefulSetsMsg{StatefulSets: items, Err: err}
	})
}

func (c nativeClusterCommands) FetchDaemonSets(namespace string) tea.Cmd {
	return listCommand(c, func(ctx context.Context) ([]appsv1.DaemonSet, error) {
		return c.reader.ListDaemonSets(ctx, namespace)
	}, projectDaemonSets, func(items []cluster.DaemonSet, err error) tea.Msg {
		return cluster.DaemonSetsMsg{DaemonSets: items, Err: err}
	})
}

func (c nativeClusterCommands) FetchConfigMaps(namespace string) tea.Cmd {
	return listCommand(c, func(ctx context.Context) ([]corev1.ConfigMap, error) {
		return c.reader.ListConfigMaps(ctx, namespace)
	}, projectConfigMaps, func(items []cluster.ConfigMap, err error) tea.Msg {
		return cluster.ConfigMapsMsg{ConfigMaps: items, Err: err}
	})
}

func (c nativeClusterCommands) FetchNodes() tea.Cmd {
	return listCommand(c, c.reader.ListNodes, projectNodes, func(items []cluster.Node, err error) tea.Msg {
		return cluster.NodesMsg{Nodes: items, Err: err}
	})
}

func (c nativeClusterCommands) FetchJobs(namespace string) tea.Cmd {
	return listCommand(c, func(ctx context.Context) ([]batchv1.Job, error) {
		return c.reader.ListJobs(ctx, namespace)
	}, projectJobs, func(items []cluster.Job, err error) tea.Msg {
		return cluster.JobsMsg{Jobs: items, Err: err}
	})
}

func (c nativeClusterCommands) FetchIngresses(namespace string) tea.Cmd {
	return listCommand(c, func(ctx context.Context) ([]networkingv1.Ingress, error) {
		return c.reader.ListIngresses(ctx, namespace)
	}, projectIngresses, func(items []cluster.Ingress, err error) tea.Msg {
		return cluster.IngressesMsg{Ingresses: items, Err: err}
	})
}

func (c nativeClusterCommands) FetchNetworkPolicies(namespace string) tea.Cmd {
	return listCommand(c, func(ctx context.Context) ([]networkingv1.NetworkPolicy, error) {
		return c.reader.ListNetworkPolicies(ctx, namespace)
	}, projectNetworkPolicies, func(items []cluster.NetworkPolicy, err error) tea.Msg {
		return cluster.NetworkPoliciesMsg{NetworkPolicies: items, Err: err}
	})
}

func (c nativeClusterCommands) FetchPVCs(namespace string) tea.Cmd {
	return listCommand(c, func(ctx context.Context) ([]corev1.PersistentVolumeClaim, error) {
		return c.reader.ListPVCs(ctx, namespace)
	}, projectPVCs, func(items []cluster.PersistentVolumeClaim, err error) tea.Msg {
		return cluster.PVCsMsg{PVCs: items, Err: err}
	})
}

func (c nativeClusterCommands) FetchCronJobs(namespace string) tea.Cmd {
	return listCommand(c, func(ctx context.Context) ([]batchv1.CronJob, error) {
		return c.reader.ListCronJobs(ctx, namespace)
	}, projectCronJobs, func(items []cluster.CronJob, err error) tea.Msg {
		return cluster.CronJobsMsg{CronJobs: items, Err: err}
	})
}

func (c nativeClusterCommands) FetchHPAs(namespace string) tea.Cmd {
	return listCommand(c, func(ctx context.Context) ([]autoscalingv2.HorizontalPodAutoscaler, error) {
		return c.reader.ListHPAs(ctx, namespace)
	}, projectHPAs, func(items []cluster.HPA, err error) tea.Msg {
		return cluster.HPAsMsg{HPAs: items, Err: err}
	})
}

func (c nativeClusterCommands) FetchSecrets(namespace string) tea.Cmd {
	return listCommand(c, func(ctx context.Context) ([]kube.ResourceMetadata, error) {
		return c.reader.ListSecrets(ctx, namespace)
	}, projectSecrets, func(items []cluster.Secret, err error) tea.Msg {
		return cluster.SecretsMsg{Secrets: items, Err: err}
	})
}

func (c nativeClusterCommands) FetchReplicaSets(namespace string) tea.Cmd {
	return listCommand(c, func(ctx context.Context) ([]appsv1.ReplicaSet, error) {
		return c.reader.ListReplicaSets(ctx, namespace)
	}, projectReplicaSets, func(items []cluster.ReplicaSet, err error) tea.Msg {
		return cluster.ReplicaSetsMsg{ReplicaSets: items, Err: err}
	})
}

func (c nativeClusterCommands) FetchRBAC(namespace string) tea.Cmd {
	parent := c.parent
	reader := c.reader
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, clusterReadTimeout)
		defer cancel()
		resources, err := reader.ListRBAC(ctx, namespace)
		return cluster.RBACMsg{RBAC: projectRBAC(resources, c.now()), Err: err}
	}
}

func (c nativeClusterCommands) FetchCRDs() tea.Cmd {
	return listCommand(c, c.reader.ListCRDs, projectCRDs, func(items []cluster.CRD, err error) tea.Msg {
		return cluster.CRDsMsg{CRDs: items, Err: err}
	})
}

func (c nativeClusterCommands) FetchCRDInstances(crd cluster.CRD, namespace string) tea.Cmd {
	resource := schema.GroupVersionResource{Group: crd.Group, Version: crd.PreferredVersion, Resource: crd.Plural}
	requestNamespace := namespace
	if crd.Scope == string(apiextensionsv1.ClusterScoped) {
		namespace = ""
	}
	return listCommand(c, func(ctx context.Context) ([]kube.ResourceMetadata, error) {
		return c.reader.ListResourceMetadata(ctx, resource, namespace)
	}, projectCRDInstances, func(items []cluster.CRDInstance, err error) tea.Msg {
		return cluster.CRDInstancesMsg{Resource: crd.Resource, Namespace: requestNamespace, Instances: items, Err: err}
	})
}

func listCommand[Source, Target any](
	commands nativeClusterCommands,
	load func(context.Context) ([]Source, error),
	project func([]Source, time.Time) []Target,
	message func([]Target, error) tea.Msg,
) tea.Cmd {
	parent := commands.parent
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, clusterReadTimeout)
		defer cancel()
		items, err := load(ctx)
		if err != nil {
			return message(nil, err)
		}
		return message(project(items, commands.now()), nil)
	}
}

var _ clusterCommands = nativeClusterCommands{}
