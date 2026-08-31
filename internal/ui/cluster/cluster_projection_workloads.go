package cluster

import (
	"fmt"
	"slices"
	"strings"
	"time"

	model "github.com/HediAbed/opsmate/internal/cluster"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
)

func projectStatefulSets(items []appsv1.StatefulSet, now time.Time) []model.StatefulSet {
	return projectSlice(items, now, func(item appsv1.StatefulSet, now time.Time) model.StatefulSet {
		return model.StatefulSet{
			Name:      item.Name,
			Namespace: item.Namespace,
			Ready:     fmt.Sprintf("%d/%d", item.Status.ReadyReplicas, item.Status.Replicas),
			Replicas:  int(item.Status.Replicas),
			Age:       projectedAge(now, item.CreationTimestamp.Time),
		}
	})
}

func projectDaemonSets(items []appsv1.DaemonSet, now time.Time) []model.DaemonSet {
	return projectSlice(items, now, func(item appsv1.DaemonSet, now time.Time) model.DaemonSet {
		return model.DaemonSet{
			Name:      item.Name,
			Namespace: item.Namespace,
			Desired:   int(item.Status.DesiredNumberScheduled),
			Current:   int(item.Status.CurrentNumberScheduled),
			Ready:     int(item.Status.NumberReady),
			Available: int(item.Status.NumberAvailable),
			Age:       projectedAge(now, item.CreationTimestamp.Time),
		}
	})
}

func projectConfigMaps(items []corev1.ConfigMap, now time.Time) []model.ConfigMap {
	return projectSlice(items, now, func(item corev1.ConfigMap, now time.Time) model.ConfigMap {
		return model.ConfigMap{
			Name:      item.Name,
			Namespace: item.Namespace,
			Data:      len(item.Data) + len(item.BinaryData),
			Age:       projectedAge(now, item.CreationTimestamp.Time),
		}
	})
}

func projectNodes(items []corev1.Node, now time.Time) []model.Node {
	return projectSlice(items, now, projectNode)
}

func projectNode(item corev1.Node, now time.Time) model.Node {
	status := projectedNodeStatus(item.Status.Conditions)
	if item.Spec.Unschedulable {
		status += ",SchedulingDisabled"
	}
	return model.Node{
		Name:       item.Name,
		Status:     status,
		Roles:      projectedNodeRoles(item.Labels),
		Version:    item.Status.NodeInfo.KubeletVersion,
		Age:        projectedAge(now, item.CreationTimestamp.Time),
		InternalIP: projectedNodeInternalIP(item.Status.Addresses),
		OSImage:    item.Status.NodeInfo.OSImage,
		Kernel:     item.Status.NodeInfo.KernelVersion,
		Runtime:    item.Status.NodeInfo.ContainerRuntimeVersion,
	}
}

func projectedNodeStatus(conditions []corev1.NodeCondition) string {
	for _, condition := range conditions {
		if condition.Type != corev1.NodeReady {
			continue
		}
		if condition.Status == corev1.ConditionTrue {
			return "Ready"
		}
		return "NotReady"
	}
	return "Unknown"
}

func projectedNodeRoles(labels map[string]string) string {
	roles := make([]string, 0, len(labels))
	for label := range labels {
		if role := strings.TrimPrefix(label, nodeRoleLabelPrefix); role != label && role != "" {
			roles = append(roles, role)
		}
	}
	if len(roles) == 0 {
		return projectedNone
	}
	slices.Sort(roles)
	return strings.Join(roles, ",")
}

func projectedNodeInternalIP(addresses []corev1.NodeAddress) string {
	for _, address := range addresses {
		if address.Type == corev1.NodeInternalIP {
			return address.Address
		}
	}
	return ""
}

func projectJobs(items []batchv1.Job, now time.Time) []model.Job {
	return projectSlice(items, now, projectJob)
}

func projectJob(item batchv1.Job, now time.Time) model.Job {
	completions := int32(1)
	if item.Spec.Completions != nil {
		completions = *item.Spec.Completions
	}
	return model.Job{
		Name:        item.Name,
		Namespace:   item.Namespace,
		Completions: fmt.Sprintf("%d/%d", item.Status.Succeeded, completions),
		Duration:    projectedJobDuration(item, now),
		Status:      projectedJobStatus(item, completions),
		Age:         projectedAge(now, item.CreationTimestamp.Time),
	}
}

func projectedJobStatus(item batchv1.Job, completions int32) string {
	switch {
	case item.Status.Succeeded >= completions:
		return "Complete"
	case item.Status.Failed > 0 && item.Status.Active == 0:
		return "Failed"
	default:
		return "Running"
	}
}

func projectedJobDuration(item batchv1.Job, now time.Time) string {
	if item.Status.StartTime == nil {
		return "-"
	}
	end := now
	if item.Status.CompletionTime != nil {
		end = item.Status.CompletionTime.Time
	}
	return projectedDuration(end.Sub(item.Status.StartTime.Time))
}
