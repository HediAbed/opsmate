package kube

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

const (
	maximumSnapshotItems  = 100
	maximumSnapshotEvents = 50
	defaultReplicaCount   = 1
	unknownSnapshotState  = "Unknown"
)

type SnapshotSection string

const (
	SnapshotContext     SnapshotSection = "context"
	SnapshotPods        SnapshotSection = "pods"
	SnapshotDeployments SnapshotSection = "deployments"
	SnapshotServices    SnapshotSection = "services"
	SnapshotEvents      SnapshotSection = "events"
	SnapshotNodes       SnapshotSection = "nodes"
)

type SnapshotWarning struct {
	Section SnapshotSection
	Err     error
}

type PodSnapshot struct {
	Name      string
	Namespace string
	Status    string
	Ready     int
	Desired   int
	Restarts  int32
	Node      string
}

type DeploymentSnapshot struct {
	Name      string
	Namespace string
	Ready     int32
	Desired   int32
	Updated   int32
	Available int32
}

type ServicePortSnapshot struct {
	Name     string
	Protocol string
	Port     int32
}

type ServiceSnapshot struct {
	Name      string
	Namespace string
	Type      string
	ClusterIP string
	Ports     []ServicePortSnapshot
}

type EventSnapshot struct {
	Namespace string
	Type      string
	Reason    string
	Object    string
	Message   string
	Count     int32
	LastSeen  time.Time
}

type NodeSnapshot struct {
	Name          string
	Ready         bool
	Unschedulable bool
	Version       string
}

type SnapshotTotals struct {
	Pods        int
	Deployments int
	Services    int
	Events      int
	Nodes       int
}

type ClusterSnapshot struct {
	ContextName string
	Namespace   string
	Pods        []PodSnapshot
	Deployments []DeploymentSnapshot
	Services    []ServiceSnapshot
	Events      []EventSnapshot
	Nodes       []NodeSnapshot
	Totals      SnapshotTotals
	Warnings    []SnapshotWarning
}

type SnapshotCollector struct {
	contexts  ContextManager
	resources ResourceReader
}

type snapshotResult[T any] struct {
	items []T
	err   error
}

const snapshotResourceReadCount = 5

func NewSnapshotCollector(contexts ContextManager, resources ResourceReader) (*SnapshotCollector, error) {
	if contexts == nil {
		return nil, newError(OperationCreate, SubjectClusterSnapshot, "", ErrContextManagerRequired)
	}
	if resources == nil {
		return nil, newError(OperationCreate, SubjectClusterSnapshot, "", ErrResourceReaderRequired)
	}
	return &SnapshotCollector{contexts: contexts, resources: resources}, nil
}

func (c *SnapshotCollector) Collect(ctx context.Context, namespace string) (ClusterSnapshot, error) {
	if ctx == nil {
		return ClusterSnapshot{}, newError(OperationCollect, SubjectClusterSnapshot, "", ErrContextRequired)
	}
	if c == nil {
		return ClusterSnapshot{}, newError(OperationCollect, SubjectClusterSnapshot, "", ErrSnapshotCollectorRequired)
	}
	if err := ctx.Err(); err != nil {
		return ClusterSnapshot{}, newError(OperationCollect, SubjectClusterSnapshot, "", err)
	}

	contextName, contextErr := c.contexts.CurrentContext(ctx)
	pods, deployments, services, events, nodes := c.collectResources(ctx, namespace)
	if err := ctx.Err(); err != nil {
		return ClusterSnapshot{}, newError(OperationCollect, SubjectClusterSnapshot, contextName, err)
	}

	snapshot := buildClusterSnapshot(contextName, namespace, pods, deployments, services, events, nodes)
	if allSnapshotReadsFailed(pods.err, deployments.err, services.err, events.err, nodes.err) {
		return snapshot, newError(OperationCollect, SubjectClusterSnapshot, contextName, errors.Join(
			pods.err,
			deployments.err,
			services.err,
			events.err,
			nodes.err,
		))
	}
	snapshot.Warnings = snapshotWarnings(contextErr, pods.err, deployments.err, services.err, events.err, nodes.err)
	return snapshot, nil
}

func (c *SnapshotCollector) collectResources(
	ctx context.Context,
	namespace string,
) (
	snapshotResult[corev1.Pod],
	snapshotResult[appsv1.Deployment],
	snapshotResult[corev1.Service],
	snapshotResult[corev1.Event],
	snapshotResult[corev1.Node],
) {
	var pods snapshotResult[corev1.Pod]
	var deployments snapshotResult[appsv1.Deployment]
	var services snapshotResult[corev1.Service]
	var events snapshotResult[corev1.Event]
	var nodes snapshotResult[corev1.Node]
	var reads sync.WaitGroup
	reads.Add(snapshotResourceReadCount)
	go collectSnapshotItems(&reads, &pods, func() ([]corev1.Pod, error) {
		return c.resources.ListPods(ctx, namespace)
	})
	go collectSnapshotItems(&reads, &deployments, func() ([]appsv1.Deployment, error) {
		return c.resources.ListDeployments(ctx, namespace)
	})
	go collectSnapshotItems(&reads, &services, func() ([]corev1.Service, error) {
		return c.resources.ListServices(ctx, namespace)
	})
	go collectSnapshotItems(&reads, &events, func() ([]corev1.Event, error) {
		return c.resources.ListEvents(ctx, namespace)
	})
	go collectSnapshotItems(&reads, &nodes, func() ([]corev1.Node, error) {
		return c.resources.ListNodes(ctx)
	})
	reads.Wait()
	return pods, deployments, services, events, nodes
}

func collectSnapshotItems[T any](
	reads *sync.WaitGroup,
	result *snapshotResult[T],
	read func() ([]T, error),
) {
	defer reads.Done()
	result.items, result.err = read()
}

func buildClusterSnapshot(
	contextName string,
	namespace string,
	pods snapshotResult[corev1.Pod],
	deployments snapshotResult[appsv1.Deployment],
	services snapshotResult[corev1.Service],
	events snapshotResult[corev1.Event],
	nodes snapshotResult[corev1.Node],
) ClusterSnapshot {
	return ClusterSnapshot{
		ContextName: contextName,
		Namespace:   namespace,
		Pods:        projectSnapshotPods(pods.items),
		Deployments: projectSnapshotDeployments(deployments.items),
		Services:    projectSnapshotServices(services.items),
		Events:      projectSnapshotEvents(events.items),
		Nodes:       projectSnapshotNodes(nodes.items),
		Totals: SnapshotTotals{
			Pods:        len(pods.items),
			Deployments: len(deployments.items),
			Services:    len(services.items),
			Events:      len(events.items),
			Nodes:       len(nodes.items),
		},
	}
}

func allSnapshotReadsFailed(readErrors ...error) bool {
	for _, err := range readErrors {
		if err == nil {
			return false
		}
	}
	return true
}

func snapshotWarnings(
	contextErr error,
	podsErr error,
	deploymentsErr error,
	servicesErr error,
	eventsErr error,
	nodesErr error,
) []SnapshotWarning {
	sections := []struct {
		section SnapshotSection
		err     error
	}{
		{SnapshotContext, contextErr},
		{SnapshotPods, podsErr},
		{SnapshotDeployments, deploymentsErr},
		{SnapshotServices, servicesErr},
		{SnapshotEvents, eventsErr},
		{SnapshotNodes, nodesErr},
	}
	warnings := make([]SnapshotWarning, 0, len(sections))
	for _, section := range sections {
		if section.err != nil {
			warnings = append(warnings, SnapshotWarning{Section: section.section, Err: section.err})
		}
	}
	return warnings
}

func projectSnapshotPods(items []corev1.Pod) []PodSnapshot {
	sortSnapshotObjects(items, func(item corev1.Pod) (string, string) {
		return item.Namespace, item.Name
	})
	items = limitSnapshotItems(items, maximumSnapshotItems)
	pods := make([]PodSnapshot, 0, len(items))
	for _, item := range items {
		ready, restarts := snapshotContainerState(item.Status.ContainerStatuses)
		pods = append(pods, PodSnapshot{
			Name:      item.Name,
			Namespace: item.Namespace,
			Status:    snapshotPodStatus(item),
			Ready:     ready,
			Desired:   len(item.Spec.Containers),
			Restarts:  restarts,
			Node:      item.Spec.NodeName,
		})
	}
	return pods
}

func snapshotContainerState(statuses []corev1.ContainerStatus) (int, int32) {
	ready := 0
	var restarts int32
	for _, status := range statuses {
		if status.Ready {
			ready++
		}
		restarts += status.RestartCount
	}
	return ready, restarts
}

func snapshotPodStatus(pod corev1.Pod) string {
	if pod.DeletionTimestamp != nil {
		return "Terminating"
	}
	for _, status := range pod.Status.InitContainerStatuses {
		if status.State.Waiting != nil && status.State.Waiting.Reason != "" {
			return "Init:" + status.State.Waiting.Reason
		}
	}
	for _, status := range pod.Status.ContainerStatuses {
		if status.State.Waiting != nil && status.State.Waiting.Reason != "" {
			return status.State.Waiting.Reason
		}
	}
	if pod.Status.Phase == "" {
		return unknownSnapshotState
	}
	return string(pod.Status.Phase)
}

func projectSnapshotDeployments(items []appsv1.Deployment) []DeploymentSnapshot {
	sortSnapshotObjects(items, func(item appsv1.Deployment) (string, string) {
		return item.Namespace, item.Name
	})
	items = limitSnapshotItems(items, maximumSnapshotItems)
	deployments := make([]DeploymentSnapshot, 0, len(items))
	for _, item := range items {
		deployments = append(deployments, DeploymentSnapshot{
			Name:      item.Name,
			Namespace: item.Namespace,
			Ready:     item.Status.ReadyReplicas,
			Desired:   snapshotDesiredReplicas(item.Spec.Replicas),
			Updated:   item.Status.UpdatedReplicas,
			Available: item.Status.AvailableReplicas,
		})
	}
	return deployments
}

func snapshotDesiredReplicas(replicas *int32) int32 {
	if replicas == nil {
		return defaultReplicaCount
	}
	return *replicas
}

func projectSnapshotServices(items []corev1.Service) []ServiceSnapshot {
	sortSnapshotObjects(items, func(item corev1.Service) (string, string) {
		return item.Namespace, item.Name
	})
	items = limitSnapshotItems(items, maximumSnapshotItems)
	services := make([]ServiceSnapshot, 0, len(items))
	for _, item := range items {
		ports := make([]ServicePortSnapshot, 0, len(item.Spec.Ports))
		for _, port := range item.Spec.Ports {
			ports = append(ports, ServicePortSnapshot{
				Name:     port.Name,
				Protocol: string(port.Protocol),
				Port:     port.Port,
			})
		}
		services = append(services, ServiceSnapshot{
			Name:      item.Name,
			Namespace: item.Namespace,
			Type:      string(item.Spec.Type),
			ClusterIP: item.Spec.ClusterIP,
			Ports:     ports,
		})
	}
	return services
}

func projectSnapshotEvents(items []corev1.Event) []EventSnapshot {
	slices.SortFunc(items, func(left, right corev1.Event) int {
		return snapshotEventTime(right).Compare(snapshotEventTime(left))
	})
	items = limitSnapshotItems(items, maximumSnapshotEvents)
	events := make([]EventSnapshot, 0, len(items))
	for _, item := range items {
		events = append(events, EventSnapshot{
			Namespace: item.Namespace,
			Type:      item.Type,
			Reason:    item.Reason,
			Object:    item.InvolvedObject.Kind + "/" + item.InvolvedObject.Name,
			Message:   item.Message,
			Count:     item.Count,
			LastSeen:  snapshotEventTime(item),
		})
	}
	return events
}

func snapshotEventTime(event corev1.Event) time.Time {
	if !event.EventTime.IsZero() {
		return event.EventTime.Time
	}
	if event.Series != nil && !event.Series.LastObservedTime.IsZero() {
		return event.Series.LastObservedTime.Time
	}
	if !event.LastTimestamp.IsZero() {
		return event.LastTimestamp.Time
	}
	return event.CreationTimestamp.Time
}

func projectSnapshotNodes(items []corev1.Node) []NodeSnapshot {
	sortSnapshotObjects(items, func(item corev1.Node) (string, string) {
		return item.Namespace, item.Name
	})
	items = limitSnapshotItems(items, maximumSnapshotItems)
	nodes := make([]NodeSnapshot, 0, len(items))
	for _, item := range items {
		nodes = append(nodes, NodeSnapshot{
			Name:          item.Name,
			Ready:         snapshotNodeReady(item.Status.Conditions),
			Unschedulable: item.Spec.Unschedulable,
			Version:       item.Status.NodeInfo.KubeletVersion,
		})
	}
	return nodes
}

func snapshotNodeReady(conditions []corev1.NodeCondition) bool {
	for _, condition := range conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func sortSnapshotObjects[T any](items []T, identity func(T) (string, string)) {
	slices.SortFunc(items, func(left, right T) int {
		leftNamespace, leftName := identity(left)
		rightNamespace, rightName := identity(right)
		return cmp.Or(
			cmp.Compare(leftNamespace, rightNamespace),
			cmp.Compare(leftName, rightName),
		)
	})
}

func limitSnapshotItems[T any](items []T, limit int) []T {
	if len(items) <= limit {
		return items
	}
	return items[:limit]
}
