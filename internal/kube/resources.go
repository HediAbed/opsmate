package kube

import (
	"context"
	"errors"
	"sync"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

type ResourceReader interface {
	ListPods(context.Context, string) ([]corev1.Pod, error)
	ListDeployments(context.Context, string) ([]appsv1.Deployment, error)
	ListEvents(context.Context, string) ([]corev1.Event, error)
	ListServices(context.Context, string) ([]corev1.Service, error)
	ListStatefulSets(context.Context, string) ([]appsv1.StatefulSet, error)
	ListDaemonSets(context.Context, string) ([]appsv1.DaemonSet, error)
	ListConfigMaps(context.Context, string) ([]corev1.ConfigMap, error)
	ListNodes(context.Context) ([]corev1.Node, error)
	ListJobs(context.Context, string) ([]batchv1.Job, error)
	ListIngresses(context.Context, string) ([]networkingv1.Ingress, error)
	ListNetworkPolicies(context.Context, string) ([]networkingv1.NetworkPolicy, error)
	ListPVCs(context.Context, string) ([]corev1.PersistentVolumeClaim, error)
	ListCronJobs(context.Context, string) ([]batchv1.CronJob, error)
	ListHPAs(context.Context, string) ([]autoscalingv2.HorizontalPodAutoscaler, error)
	ListSecrets(context.Context, string) ([]ResourceMetadata, error)
	ListReplicaSets(context.Context, string) ([]appsv1.ReplicaSet, error)
	ListRBAC(context.Context, string) (RBACResources, error)
	ListCRDs(context.Context) ([]apiextensionsv1.CustomResourceDefinition, error)
	ListPodMetrics(context.Context, string) ([]metricsv1beta1.PodMetrics, error)
	ListResourceMetadata(context.Context, schema.GroupVersionResource, string) ([]ResourceMetadata, error)
}

type Cluster interface {
	ContextManager
	ResourceReader
	ResourceObserver
	HelmReader
	ClusterOperations
}

type RBACResources struct {
	ServiceAccounts     []corev1.ServiceAccount
	Roles               []rbacv1.Role
	RoleBindings        []rbacv1.RoleBinding
	ClusterRoles        []rbacv1.ClusterRole
	ClusterRoleBindings []rbacv1.ClusterRoleBinding
}

type ResourceMetadata struct {
	Name      string
	Namespace string
	CreatedAt time.Time
}

func (m *Manager) ListPods(ctx context.Context, namespace string) ([]corev1.Pod, error) {
	return listResources(ctx, m, SubjectPods, func(ctx context.Context, clients *Clients) ([]corev1.Pod, error) {
		list, err := clients.Kubernetes().CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		return list.Items, nil
	})
}

func (m *Manager) ListDeployments(ctx context.Context, namespace string) ([]appsv1.Deployment, error) {
	return listResources(ctx, m, SubjectDeployments, func(ctx context.Context, clients *Clients) ([]appsv1.Deployment, error) {
		list, err := clients.Kubernetes().AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		return list.Items, nil
	})
}

func (m *Manager) ListEvents(ctx context.Context, namespace string) ([]corev1.Event, error) {
	return listResources(ctx, m, SubjectEvents, func(ctx context.Context, clients *Clients) ([]corev1.Event, error) {
		list, err := clients.Kubernetes().CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		return list.Items, nil
	})
}

func (m *Manager) ListServices(ctx context.Context, namespace string) ([]corev1.Service, error) {
	return listResources(ctx, m, SubjectServices, func(ctx context.Context, clients *Clients) ([]corev1.Service, error) {
		list, err := clients.Kubernetes().CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		return list.Items, nil
	})
}

func (m *Manager) ListStatefulSets(ctx context.Context, namespace string) ([]appsv1.StatefulSet, error) {
	return listResources(ctx, m, SubjectStatefulSets, func(ctx context.Context, clients *Clients) ([]appsv1.StatefulSet, error) {
		list, err := clients.Kubernetes().AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		return list.Items, nil
	})
}

func (m *Manager) ListDaemonSets(ctx context.Context, namespace string) ([]appsv1.DaemonSet, error) {
	return listResources(ctx, m, SubjectDaemonSets, func(ctx context.Context, clients *Clients) ([]appsv1.DaemonSet, error) {
		list, err := clients.Kubernetes().AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		return list.Items, nil
	})
}

func (m *Manager) ListConfigMaps(ctx context.Context, namespace string) ([]corev1.ConfigMap, error) {
	return listResources(ctx, m, SubjectConfigMaps, func(ctx context.Context, clients *Clients) ([]corev1.ConfigMap, error) {
		list, err := clients.Kubernetes().CoreV1().ConfigMaps(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		return list.Items, nil
	})
}

func (m *Manager) ListNodes(ctx context.Context) ([]corev1.Node, error) {
	return listResources(ctx, m, SubjectNodes, func(ctx context.Context, clients *Clients) ([]corev1.Node, error) {
		list, err := clients.Kubernetes().CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		return list.Items, nil
	})
}

func (m *Manager) ListJobs(ctx context.Context, namespace string) ([]batchv1.Job, error) {
	return listResources(ctx, m, SubjectJobs, func(ctx context.Context, clients *Clients) ([]batchv1.Job, error) {
		list, err := clients.Kubernetes().BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		return list.Items, nil
	})
}

func (m *Manager) ListIngresses(ctx context.Context, namespace string) ([]networkingv1.Ingress, error) {
	return listResources(ctx, m, SubjectIngresses, func(ctx context.Context, clients *Clients) ([]networkingv1.Ingress, error) {
		list, err := clients.Kubernetes().NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		return list.Items, nil
	})
}

func (m *Manager) ListNetworkPolicies(ctx context.Context, namespace string) ([]networkingv1.NetworkPolicy, error) {
	return listResources(ctx, m, SubjectNetworkPolicies, func(ctx context.Context, clients *Clients) ([]networkingv1.NetworkPolicy, error) {
		list, err := clients.Kubernetes().NetworkingV1().NetworkPolicies(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		return list.Items, nil
	})
}

func (m *Manager) ListPVCs(ctx context.Context, namespace string) ([]corev1.PersistentVolumeClaim, error) {
	return listResources(ctx, m, SubjectPVCs, func(ctx context.Context, clients *Clients) ([]corev1.PersistentVolumeClaim, error) {
		list, err := clients.Kubernetes().CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		return list.Items, nil
	})
}

func (m *Manager) ListCronJobs(ctx context.Context, namespace string) ([]batchv1.CronJob, error) {
	return listResources(ctx, m, SubjectCronJobs, func(ctx context.Context, clients *Clients) ([]batchv1.CronJob, error) {
		list, err := clients.Kubernetes().BatchV1().CronJobs(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		return list.Items, nil
	})
}

func (m *Manager) ListHPAs(ctx context.Context, namespace string) ([]autoscalingv2.HorizontalPodAutoscaler, error) {
	return listResources(ctx, m, SubjectHPAs, func(ctx context.Context, clients *Clients) ([]autoscalingv2.HorizontalPodAutoscaler, error) {
		list, err := clients.Kubernetes().AutoscalingV2().HorizontalPodAutoscalers(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		return list.Items, nil
	})
}

func (m *Manager) ListSecrets(ctx context.Context, namespace string) ([]ResourceMetadata, error) {
	return listMetadata(ctx, m, SubjectSecrets, corev1.SchemeGroupVersion.WithResource("secrets"), namespace)
}

func (m *Manager) ListReplicaSets(ctx context.Context, namespace string) ([]appsv1.ReplicaSet, error) {
	return listResources(ctx, m, SubjectReplicaSets, func(ctx context.Context, clients *Clients) ([]appsv1.ReplicaSet, error) {
		list, err := clients.Kubernetes().AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		return list.Items, nil
	})
}

func (m *Manager) ListCRDs(ctx context.Context) ([]apiextensionsv1.CustomResourceDefinition, error) {
	return listResources(ctx, m, SubjectCRDs, func(ctx context.Context, clients *Clients) ([]apiextensionsv1.CustomResourceDefinition, error) {
		list, err := clients.Extensions().ApiextensionsV1().CustomResourceDefinitions().List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		return list.Items, nil
	})
}

func (m *Manager) ListPodMetrics(ctx context.Context, namespace string) ([]metricsv1beta1.PodMetrics, error) {
	return listResources(ctx, m, SubjectPodMetrics, func(ctx context.Context, clients *Clients) ([]metricsv1beta1.PodMetrics, error) {
		list, err := clients.Metrics().MetricsV1beta1().PodMetricses(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		return list.Items, nil
	})
}

func (m *Manager) ListResourceMetadata(ctx context.Context, resource schema.GroupVersionResource, namespace string) ([]ResourceMetadata, error) {
	if err := ctx.Err(); err != nil {
		return nil, newError(OperationList, SubjectResourceMetadata, "", err)
	}
	if resource.Version == "" || resource.Resource == "" {
		return nil, newError(OperationList, SubjectResourceMetadata, "", ErrResourceIdentifierRequired)
	}
	return listMetadata(ctx, m, SubjectResourceMetadata, resource, namespace)
}

func listMetadata(ctx context.Context, manager *Manager, subject Subject, resource schema.GroupVersionResource, namespace string) ([]ResourceMetadata, error) {
	return listResources(ctx, manager, subject, func(ctx context.Context, clients *Clients) ([]ResourceMetadata, error) {
		list, err := clients.Metadata().Resource(resource).Namespace(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		metadata := make([]ResourceMetadata, 0, len(list.Items))
		for _, item := range list.Items {
			metadata = append(metadata, ResourceMetadata{
				Name:      item.Name,
				Namespace: item.Namespace,
				CreatedAt: item.CreationTimestamp.Time,
			})
		}
		return metadata, nil
	})
}

func (m *Manager) ListRBAC(ctx context.Context, namespace string) (RBACResources, error) {
	if err := ctx.Err(); err != nil {
		return RBACResources{}, newError(OperationList, SubjectRBAC, "", err)
	}
	clients, err := m.Clients()
	if err != nil {
		return RBACResources{}, newError(OperationList, SubjectRBAC, "", err)
	}
	var reads sync.WaitGroup
	serviceAccounts := collectRBAC(&reads, SubjectServiceAccounts, func() ([]corev1.ServiceAccount, error) {
		list, listErr := clients.Kubernetes().CoreV1().ServiceAccounts(namespace).List(ctx, metav1.ListOptions{})
		if listErr != nil {
			return nil, listErr
		}
		return list.Items, listErr
	})
	roles := collectRBAC(&reads, SubjectRoles, func() ([]rbacv1.Role, error) {
		list, listErr := clients.Kubernetes().RbacV1().Roles(namespace).List(ctx, metav1.ListOptions{})
		if listErr != nil {
			return nil, listErr
		}
		return list.Items, listErr
	})
	roleBindings := collectRBAC(&reads, SubjectRoleBindings, func() ([]rbacv1.RoleBinding, error) {
		list, listErr := clients.Kubernetes().RbacV1().RoleBindings(namespace).List(ctx, metav1.ListOptions{})
		if listErr != nil {
			return nil, listErr
		}
		return list.Items, listErr
	})
	clusterRoles := collectRBAC(&reads, SubjectClusterRoles, func() ([]rbacv1.ClusterRole, error) {
		list, listErr := clients.Kubernetes().RbacV1().ClusterRoles().List(ctx, metav1.ListOptions{})
		if listErr != nil {
			return nil, listErr
		}
		return list.Items, listErr
	})
	clusterRoleBindings := collectRBAC(&reads, SubjectClusterRoleBindings, func() ([]rbacv1.ClusterRoleBinding, error) {
		list, listErr := clients.Kubernetes().RbacV1().ClusterRoleBindings().List(ctx, metav1.ListOptions{})
		if listErr != nil {
			return nil, listErr
		}
		return list.Items, listErr
	})
	reads.Wait()
	resources := RBACResources{
		ServiceAccounts:     serviceAccounts.items,
		Roles:               roles.items,
		RoleBindings:        roleBindings.items,
		ClusterRoles:        clusterRoles.items,
		ClusterRoleBindings: clusterRoleBindings.items,
	}
	return resources, errors.Join(serviceAccounts.err, roles.err, roleBindings.err, clusterRoles.err, clusterRoleBindings.err)
}

type rbacCollection[T any] struct {
	items []T
	err   error
}

func collectRBAC[T any](reads *sync.WaitGroup, subject Subject, list func() ([]T, error)) *rbacCollection[T] {
	result := &rbacCollection[T]{}
	reads.Go(func() {
		items, err := list()
		if err == nil {
			result.items = normalizedItems(items)
		}
		result.err = resourceListError(subject, err)
	})
	return result
}

func listResources[T any](ctx context.Context, manager *Manager, subject Subject, load func(context.Context, *Clients) ([]T, error)) ([]T, error) {
	if err := ctx.Err(); err != nil {
		return nil, newError(OperationList, subject, "", err)
	}
	clients, err := manager.Clients()
	if err != nil {
		return nil, newError(OperationList, subject, "", err)
	}
	items, err := load(ctx, clients)
	if err != nil {
		return nil, newError(OperationList, subject, "", err)
	}
	return normalizedItems(items), nil
}

func resourceListError(subject Subject, err error) error {
	if err == nil {
		return nil
	}
	return newError(OperationList, subject, "", err)
}

func normalizedItems[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}

var _ Cluster = (*Manager)(nil)
