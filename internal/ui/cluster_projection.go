package ui

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"

	"github.com/HediAbed/opsmate/internal/cluster"
)

const (
	maximumProjectedEvents = 50
	projectedDay           = 24 * time.Hour
	projectedUnknown       = "<unknown>"
	projectedNone          = "<none>"
	projectedNever         = "<none>"
	defaultHPAReplicaCount = 1
	nodeRoleLabelPrefix    = "node-role.kubernetes.io/"
	ingressHTTPPorts       = "80"
	ingressHTTPSPorts      = "80, 443"
	metadataOnlyLabel      = "<metadata only>"
)

func projectPods(items []corev1.Pod, now time.Time) []cluster.Pod {
	return projectSlice(items, now, projectPod)
}

func projectPod(item corev1.Pod, now time.Time) cluster.Pod {
	ready := 0
	restarts := 0
	for _, status := range item.Status.ContainerStatuses {
		if status.Ready {
			ready++
		}
		restarts += int(status.RestartCount)
	}
	containers := make([]string, 0, len(item.Spec.Containers))
	for _, container := range item.Spec.Containers {
		containers = append(containers, container.Name)
	}
	return cluster.Pod{
		Name:       item.Name,
		Namespace:  item.Namespace,
		Status:     projectedPodStatus(item),
		Ready:      fmt.Sprintf("%d/%d", ready, len(item.Status.ContainerStatuses)),
		Restarts:   restarts,
		Age:        projectedAge(now, item.CreationTimestamp.Time),
		Node:       item.Spec.NodeName,
		IP:         item.Status.PodIP,
		Containers: containers,
	}
}

func projectedPodStatus(item corev1.Pod) string {
	if item.DeletionTimestamp != nil {
		return "Terminating"
	}
	if reason := projectedInitStatus(item.Status.InitContainerStatuses); reason != "" {
		return reason
	}
	for _, status := range item.Status.ContainerStatuses {
		if status.State.Waiting != nil && status.State.Waiting.Reason != "" {
			return status.State.Waiting.Reason
		}
	}
	if item.Status.Phase == "" {
		return "Unknown"
	}
	return string(item.Status.Phase)
}

func projectedInitStatus(statuses []corev1.ContainerStatus) string {
	for index, status := range statuses {
		if status.State.Waiting != nil && status.State.Waiting.Reason != "" {
			return "Init:" + status.State.Waiting.Reason
		}
		if status.State.Terminated != nil && status.State.Terminated.ExitCode != 0 {
			reason := status.State.Terminated.Reason
			if reason == "" {
				reason = fmt.Sprintf("ExitCode%d", status.State.Terminated.ExitCode)
			}
			return "Init:" + reason
		}
		if status.State.Terminated == nil {
			return fmt.Sprintf("Init:%d/%d", index, len(statuses))
		}
	}
	return ""
}

func projectDeployments(items []appsv1.Deployment, now time.Time) []cluster.Deployment {
	return projectSlice(items, now, projectDeployment)
}

func projectDeployment(item appsv1.Deployment, now time.Time) cluster.Deployment {
	containers := make([]string, 0, len(item.Spec.Template.Spec.Containers))
	images := make([]string, 0, len(item.Spec.Template.Spec.Containers))
	for _, container := range item.Spec.Template.Spec.Containers {
		containers = append(containers, container.Name)
		images = append(images, container.Image)
	}
	var selectorLabels map[string]string
	if item.Spec.Selector != nil {
		selectorLabels = item.Spec.Selector.MatchLabels
	}
	return cluster.Deployment{
		Name:       item.Name,
		Namespace:  item.Namespace,
		Ready:      fmt.Sprintf("%d/%d", item.Status.ReadyReplicas, item.Status.Replicas),
		UpToDate:   int(item.Status.UpdatedReplicas),
		Available:  int(item.Status.AvailableReplicas),
		Age:        projectedAge(now, item.CreationTimestamp.Time),
		Containers: containers,
		Images:     images,
		Selector:   projectedLabelMap(selectorLabels),
	}
}

func projectEvents(items []corev1.Event, now time.Time) []cluster.Event {
	events := projectSlice(items, now, projectEvent)
	slices.SortStableFunc(events, func(left, right cluster.Event) int {
		return cmp.Compare(right.LastTimestamp.UnixNano(), left.LastTimestamp.UnixNano())
	})
	if len(events) > maximumProjectedEvents {
		return events[:maximumProjectedEvents]
	}
	return events
}

func projectEvent(item corev1.Event, now time.Time) cluster.Event {
	observedAt := eventObservedAt(item)
	count := int(item.Count)
	if item.Series != nil && int(item.Series.Count) > count {
		count = int(item.Series.Count)
	}
	return cluster.Event{
		Name:          item.Name,
		UID:           string(item.UID),
		Namespace:     item.Namespace,
		Type:          item.Type,
		Reason:        item.Reason,
		Object:        item.InvolvedObject.Kind + "/" + item.InvolvedObject.Name,
		Message:       item.Message,
		Age:           projectedAge(now, observedAt),
		Count:         count,
		LastTimestamp: observedAt,
	}
}

func eventObservedAt(item corev1.Event) time.Time {
	switch {
	case item.Series != nil && !item.Series.LastObservedTime.IsZero():
		return item.Series.LastObservedTime.Time
	case !item.EventTime.IsZero():
		return item.EventTime.Time
	case !item.LastTimestamp.IsZero():
		return item.LastTimestamp.Time
	case !item.FirstTimestamp.IsZero():
		return item.FirstTimestamp.Time
	default:
		return item.CreationTimestamp.Time
	}
}

func projectPodMetrics(items []metricsv1beta1.PodMetrics, _ time.Time) []cluster.PodMetric {
	metrics := make([]cluster.PodMetric, 0, len(items))
	for _, item := range items {
		cpu, memory := podMetricUsage(item)
		metrics = append(metrics, cluster.PodMetric{
			Name:      item.Name,
			Namespace: item.Namespace,
			CPU:       cpu,
			Memory:    memory,
		})
	}
	return metrics
}

func podMetricUsage(item metricsv1beta1.PodMetrics) (string, string) {
	var totalCPU, totalMemory int64
	for _, container := range item.Containers {
		totalCPU += container.Usage.Cpu().MilliValue()
		totalMemory += container.Usage.Memory().Value()
	}
	return strconv.FormatInt(totalCPU, 10) + "m", formatBinaryBytes(totalMemory)
}

func formatBinaryBytes(bytes int64) string {
	const bytesPerMebibyte = 1024 * 1024
	return strconv.FormatInt(bytes/bytesPerMebibyte, 10) + "Mi"
}

func projectServices(items []corev1.Service, now time.Time) []cluster.Service {
	return projectSlice(items, now, projectService)
}

func projectService(item corev1.Service, now time.Time) cluster.Service {
	ports := make([]string, 0, len(item.Spec.Ports))
	for _, port := range item.Spec.Ports {
		ports = append(ports, fmt.Sprintf("%d/%s", port.Port, port.Protocol))
	}
	externalAddresses := append([]string(nil), item.Spec.ExternalIPs...)
	for _, ingress := range item.Status.LoadBalancer.Ingress {
		switch {
		case ingress.IP != "":
			externalAddresses = appendUnique(externalAddresses, ingress.IP)
		case ingress.Hostname != "":
			externalAddresses = appendUnique(externalAddresses, ingress.Hostname)
		}
	}
	return cluster.Service{
		Name:       item.Name,
		Namespace:  item.Namespace,
		Type:       string(item.Spec.Type),
		ClusterIP:  item.Spec.ClusterIP,
		ExternalIP: joinProjectedValues(externalAddresses),
		Ports:      strings.Join(ports, ","),
		Age:        projectedAge(now, item.CreationTimestamp.Time),
		Selector:   projectedLabelMap(item.Spec.Selector),
	}
}
