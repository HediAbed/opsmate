package cluster

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

	model "github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/kube"
)

type Commands interface {
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
	FetchCRDInstances(model.CRD, string) tea.Cmd
	ObservePods(string) (LiveSet[model.Pod], error)
	ObserveDeployments(string) (LiveSet[model.Deployment], error)
	ObserveEvents(string) (LiveSet[model.Event], error)
	ObserveIngresses(string) (LiveSet[model.Ingress], error)
	ObserveNetworkPolicies(string) (LiveSet[model.NetworkPolicy], error)
	ObservePersistentVolumeClaims(string) (LiveSet[model.PersistentVolumeClaim], error)
	ObserveCronJobs(string) (LiveSet[model.CronJob], error)
	ObserveHorizontalPodAutoscalers(string) (LiveSet[model.HPA], error)
	ObserveSecrets(string) (LiveSet[model.Secret], error)
	ObserveReplicaSets(string) (LiveSet[model.ReplicaSet], error)
}

type ResourceCommands struct {
	parent   context.Context
	reader   kube.ResourceReader
	observer kube.ResourceObserver
	now      func() time.Time
}

func NewCommands(parent context.Context, reader kube.ResourceReader, observer kube.ResourceObserver) ResourceCommands {
	return ResourceCommands{parent: parent, reader: reader, observer: observer, now: time.Now}
}

func (c ResourceCommands) FetchPods(namespace string) tea.Cmd {
	return listCommand(c, func(ctx context.Context) ([]corev1.Pod, error) {
		return c.reader.ListPods(ctx, namespace)
	}, projectPods, func(items []model.Pod, err error) tea.Msg {
		return model.PodsMsg{Pods: items, Err: err}
	})
}

func (c ResourceCommands) FetchDeployments(namespace string) tea.Cmd {
	return listCommand(c, func(ctx context.Context) ([]appsv1.Deployment, error) {
		return c.reader.ListDeployments(ctx, namespace)
	}, projectDeployments, func(items []model.Deployment, err error) tea.Msg {
		return model.DeploymentsMsg{Deployments: items, Err: err}
	})
}

func (c ResourceCommands) FetchEvents(namespace string) tea.Cmd {
	return listCommand(c, func(ctx context.Context) ([]corev1.Event, error) {
		return c.reader.ListEvents(ctx, namespace)
	}, projectEvents, func(items []model.Event, err error) tea.Msg {
		return model.EventsMsg{Events: items, Err: err}
	})
}

func (c ResourceCommands) FetchPodMetrics(namespace string) tea.Cmd {
	return listCommand(c, func(ctx context.Context) ([]metricsv1beta1.PodMetrics, error) {
		return c.reader.ListPodMetrics(ctx, namespace)
	}, projectPodMetrics, func(items []model.PodMetric, err error) tea.Msg {
		return model.MetricsMsg{PodMetrics: items, Err: err}
	})
}

func (c ResourceCommands) FetchServices(namespace string) tea.Cmd {
	return listCommand(c, func(ctx context.Context) ([]corev1.Service, error) {
		return c.reader.ListServices(ctx, namespace)
	}, projectServices, func(items []model.Service, err error) tea.Msg {
		return model.ServicesMsg{Services: items, Err: err}
	})
}

func (c ResourceCommands) FetchStatefulSets(namespace string) tea.Cmd {
	return listCommand(c, func(ctx context.Context) ([]appsv1.StatefulSet, error) {
		return c.reader.ListStatefulSets(ctx, namespace)
	}, projectStatefulSets, func(items []model.StatefulSet, err error) tea.Msg {
		return model.StatefulSetsMsg{StatefulSets: items, Err: err}
	})
}

func (c ResourceCommands) FetchDaemonSets(namespace string) tea.Cmd {
	return listCommand(c, func(ctx context.Context) ([]appsv1.DaemonSet, error) {
		return c.reader.ListDaemonSets(ctx, namespace)
	}, projectDaemonSets, func(items []model.DaemonSet, err error) tea.Msg {
		return model.DaemonSetsMsg{DaemonSets: items, Err: err}
	})
}

func (c ResourceCommands) FetchConfigMaps(namespace string) tea.Cmd {
	return listCommand(c, func(ctx context.Context) ([]corev1.ConfigMap, error) {
		return c.reader.ListConfigMaps(ctx, namespace)
	}, projectConfigMaps, func(items []model.ConfigMap, err error) tea.Msg {
		return model.ConfigMapsMsg{ConfigMaps: items, Err: err}
	})
}

func (c ResourceCommands) FetchNodes() tea.Cmd {
	return listCommand(c, c.reader.ListNodes, projectNodes, func(items []model.Node, err error) tea.Msg {
		return model.NodesMsg{Nodes: items, Err: err}
	})
}

func (c ResourceCommands) FetchJobs(namespace string) tea.Cmd {
	return listCommand(c, func(ctx context.Context) ([]batchv1.Job, error) {
		return c.reader.ListJobs(ctx, namespace)
	}, projectJobs, func(items []model.Job, err error) tea.Msg {
		return model.JobsMsg{Jobs: items, Err: err}
	})
}

func (c ResourceCommands) FetchIngresses(namespace string) tea.Cmd {
	return listCommand(c, func(ctx context.Context) ([]networkingv1.Ingress, error) {
		return c.reader.ListIngresses(ctx, namespace)
	}, projectIngresses, func(items []model.Ingress, err error) tea.Msg {
		return model.IngressesMsg{Ingresses: items, Err: err}
	})
}

func (c ResourceCommands) FetchNetworkPolicies(namespace string) tea.Cmd {
	return listCommand(c, func(ctx context.Context) ([]networkingv1.NetworkPolicy, error) {
		return c.reader.ListNetworkPolicies(ctx, namespace)
	}, projectNetworkPolicies, func(items []model.NetworkPolicy, err error) tea.Msg {
		return model.NetworkPoliciesMsg{NetworkPolicies: items, Err: err}
	})
}

func (c ResourceCommands) FetchPVCs(namespace string) tea.Cmd {
	return listCommand(c, func(ctx context.Context) ([]corev1.PersistentVolumeClaim, error) {
		return c.reader.ListPVCs(ctx, namespace)
	}, projectPVCs, func(items []model.PersistentVolumeClaim, err error) tea.Msg {
		return model.PVCsMsg{PVCs: items, Err: err}
	})
}

func (c ResourceCommands) FetchCronJobs(namespace string) tea.Cmd {
	return listCommand(c, func(ctx context.Context) ([]batchv1.CronJob, error) {
		return c.reader.ListCronJobs(ctx, namespace)
	}, projectCronJobs, func(items []model.CronJob, err error) tea.Msg {
		return model.CronJobsMsg{CronJobs: items, Err: err}
	})
}

func (c ResourceCommands) FetchHPAs(namespace string) tea.Cmd {
	return listCommand(c, func(ctx context.Context) ([]autoscalingv2.HorizontalPodAutoscaler, error) {
		return c.reader.ListHPAs(ctx, namespace)
	}, projectHPAs, func(items []model.HPA, err error) tea.Msg {
		return model.HPAsMsg{HPAs: items, Err: err}
	})
}

func (c ResourceCommands) FetchSecrets(namespace string) tea.Cmd {
	return listCommand(c, func(ctx context.Context) ([]kube.ResourceMetadata, error) {
		return c.reader.ListSecrets(ctx, namespace)
	}, projectSecrets, func(items []model.Secret, err error) tea.Msg {
		return model.SecretsMsg{Secrets: items, Err: err}
	})
}

func (c ResourceCommands) FetchReplicaSets(namespace string) tea.Cmd {
	return listCommand(c, func(ctx context.Context) ([]appsv1.ReplicaSet, error) {
		return c.reader.ListReplicaSets(ctx, namespace)
	}, projectReplicaSets, func(items []model.ReplicaSet, err error) tea.Msg {
		return model.ReplicaSetsMsg{ReplicaSets: items, Err: err}
	})
}

func (c ResourceCommands) FetchRBAC(namespace string) tea.Cmd {
	parent := c.parent
	reader := c.reader
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, ReadTimeout)
		defer cancel()
		resources, err := reader.ListRBAC(ctx, namespace)
		return model.RBACMsg{RBAC: projectRBAC(resources, c.now()), Err: err}
	}
}

func (c ResourceCommands) FetchCRDs() tea.Cmd {
	return listCommand(c, c.reader.ListCRDs, projectCRDs, func(items []model.CRD, err error) tea.Msg {
		return model.CRDsMsg{CRDs: items, Err: err}
	})
}

func (c ResourceCommands) FetchCRDInstances(crd model.CRD, namespace string) tea.Cmd {
	resource := schema.GroupVersionResource{Group: crd.Group, Version: crd.PreferredVersion, Resource: crd.Plural}
	requestNamespace := namespace
	if crd.Scope == string(apiextensionsv1.ClusterScoped) {
		namespace = ""
	}
	return listCommand(c, func(ctx context.Context) ([]kube.ResourceMetadata, error) {
		return c.reader.ListResourceMetadata(ctx, resource, namespace)
	}, projectCRDInstances, func(items []model.CRDInstance, err error) tea.Msg {
		return model.CRDInstancesMsg{Resource: crd.Resource, Namespace: requestNamespace, Instances: items, Err: err}
	})
}

func listCommand[Source, Target interface{}](
	commands ResourceCommands,
	load func(context.Context) ([]Source, error),
	project func([]Source, time.Time) []Target,
	message func([]Target, error) tea.Msg,
) tea.Cmd {
	parent := commands.parent
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, ReadTimeout)
		defer cancel()
		items, err := load(ctx)
		if err != nil {
			return message(nil, err)
		}
		return message(project(items, commands.now()), nil)
	}
}

var _ Commands = ResourceCommands{}
