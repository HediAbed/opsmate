package ui

import (
	"strconv"
	"time"

	"github.com/HediAbed/opsmate/internal/cluster"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func projectCronJobs(items []batchv1.CronJob, now time.Time) []cluster.CronJob {
	return projectSlice(items, now, func(item batchv1.CronJob, now time.Time) cluster.CronJob {
		suspended := item.Spec.Suspend != nil && *item.Spec.Suspend
		return cluster.CronJob{
			Name:         item.Name,
			Namespace:    item.Namespace,
			Schedule:     item.Spec.Schedule,
			Suspend:      suspended,
			Active:       len(item.Status.Active),
			LastSchedule: projectedCronJobLastSchedule(item.Status.LastScheduleTime, now),
			Age:          projectedAge(now, item.CreationTimestamp.Time),
		}
	})
}

func projectedCronJobLastSchedule(lastSchedule *metav1.Time, now time.Time) string {
	if lastSchedule == nil || lastSchedule.IsZero() {
		return projectedNever
	}
	return projectedAge(now, lastSchedule.Time)
}

func projectHPAs(items []autoscalingv2.HorizontalPodAutoscaler, now time.Time) []cluster.HPA {
	return projectSlice(items, now, projectHPA)
}

func projectHPA(item autoscalingv2.HorizontalPodAutoscaler, now time.Time) cluster.HPA {
	minimumReplicas := int32(defaultHPAReplicaCount)
	if item.Spec.MinReplicas != nil {
		minimumReplicas = *item.Spec.MinReplicas
	}
	return cluster.HPA{
		Name:      item.Name,
		Namespace: item.Namespace,
		Reference: cluster.ScaleTargetRef{
			Kind: item.Spec.ScaleTargetRef.Kind,
			Name: item.Spec.ScaleTargetRef.Name,
		},
		Targets:     projectedHPATargets(item.Spec.Metrics, item.Status.CurrentMetrics),
		MinReplicas: int(minimumReplicas),
		MaxReplicas: int(item.Spec.MaxReplicas),
		Replicas:    int(item.Status.CurrentReplicas),
		Age:         projectedAge(now, item.CreationTimestamp.Time),
	}
}

func projectedHPATargets(specs []autoscalingv2.MetricSpec, statuses []autoscalingv2.MetricStatus) []cluster.HPAMetricPair {
	if len(specs) == 0 {
		return []cluster.HPAMetricPair{{Target: projectedNone}}
	}
	statusByKey := make(map[string]autoscalingv2.MetricStatus, len(statuses))
	for _, status := range statuses {
		statusByKey[metricStatusKey(status)] = status
	}
	pairs := make([]cluster.HPAMetricPair, 0, len(specs))
	for _, spec := range specs {
		current := projectedUnknown
		if status, found := statusByKey[metricSpecKey(spec)]; found {
			current = projectedMetricStatus(status)
		}
		pairs = append(pairs, cluster.HPAMetricPair{
			Current: current,
			Target:  projectedMetricTarget(spec),
		})
	}
	return pairs
}

func metricSpecKey(metric autoscalingv2.MetricSpec) string {
	if metric.Type == autoscalingv2.ResourceMetricSourceType && metric.Resource != nil {
		return namedMetricKey(metric.Type, string(metric.Resource.Name))
	}
	if metric.Type == autoscalingv2.ContainerResourceMetricSourceType && metric.ContainerResource != nil {
		return containerMetricKey(metric.Type, metric.ContainerResource.Container, string(metric.ContainerResource.Name))
	}
	if metric.Type == autoscalingv2.PodsMetricSourceType && metric.Pods != nil {
		return namedMetricKey(metric.Type, metric.Pods.Metric.Name)
	}
	return externalMetricSpecKey(metric)
}

func externalMetricSpecKey(metric autoscalingv2.MetricSpec) string {
	if metric.Type == autoscalingv2.ObjectMetricSourceType && metric.Object != nil {
		return namedMetricKey(metric.Type, metric.Object.Metric.Name)
	}
	if metric.Type == autoscalingv2.ExternalMetricSourceType && metric.External != nil {
		return namedMetricKey(metric.Type, metric.External.Metric.Name)
	}
	return string(metric.Type)
}

func metricStatusKey(metric autoscalingv2.MetricStatus) string {
	if metric.Type == autoscalingv2.ResourceMetricSourceType && metric.Resource != nil {
		return namedMetricKey(metric.Type, string(metric.Resource.Name))
	}
	if metric.Type == autoscalingv2.ContainerResourceMetricSourceType && metric.ContainerResource != nil {
		return containerMetricKey(metric.Type, metric.ContainerResource.Container, string(metric.ContainerResource.Name))
	}
	if metric.Type == autoscalingv2.PodsMetricSourceType && metric.Pods != nil {
		return namedMetricKey(metric.Type, metric.Pods.Metric.Name)
	}
	return externalMetricStatusKey(metric)
}

func externalMetricStatusKey(metric autoscalingv2.MetricStatus) string {
	if metric.Type == autoscalingv2.ObjectMetricSourceType && metric.Object != nil {
		return namedMetricKey(metric.Type, metric.Object.Metric.Name)
	}
	if metric.Type == autoscalingv2.ExternalMetricSourceType && metric.External != nil {
		return namedMetricKey(metric.Type, metric.External.Metric.Name)
	}
	return string(metric.Type)
}

func namedMetricKey(metricType autoscalingv2.MetricSourceType, name string) string {
	return string(metricType) + ":" + name
}

func containerMetricKey(metricType autoscalingv2.MetricSourceType, container, name string) string {
	return string(metricType) + ":" + container + ":" + name
}

func projectedMetricTarget(metric autoscalingv2.MetricSpec) string {
	if metric.Type == autoscalingv2.ResourceMetricSourceType && metric.Resource != nil {
		return projectedMetricTargetValue(metric.Resource.Target)
	}
	if metric.Type == autoscalingv2.ContainerResourceMetricSourceType && metric.ContainerResource != nil {
		return projectedMetricTargetValue(metric.ContainerResource.Target)
	}
	if metric.Type == autoscalingv2.PodsMetricSourceType && metric.Pods != nil {
		return projectedMetricTargetValue(metric.Pods.Target)
	}
	return projectedExternalMetricTarget(metric)
}

func projectedExternalMetricTarget(metric autoscalingv2.MetricSpec) string {
	if metric.Type == autoscalingv2.ObjectMetricSourceType && metric.Object != nil {
		return projectedMetricTargetValue(metric.Object.Target)
	}
	if metric.Type == autoscalingv2.ExternalMetricSourceType && metric.External != nil {
		return projectedMetricTargetValue(metric.External.Target)
	}
	return projectedUnknown
}

func projectedMetricStatus(metric autoscalingv2.MetricStatus) string {
	if metric.Type == autoscalingv2.ResourceMetricSourceType && metric.Resource != nil {
		return projectedMetricCurrentValue(metric.Resource.Current)
	}
	if metric.Type == autoscalingv2.ContainerResourceMetricSourceType && metric.ContainerResource != nil {
		return projectedMetricCurrentValue(metric.ContainerResource.Current)
	}
	if metric.Type == autoscalingv2.PodsMetricSourceType && metric.Pods != nil {
		return projectedMetricCurrentValue(metric.Pods.Current)
	}
	return projectedExternalMetricStatus(metric)
}

func projectedExternalMetricStatus(metric autoscalingv2.MetricStatus) string {
	if metric.Type == autoscalingv2.ObjectMetricSourceType && metric.Object != nil {
		return projectedMetricCurrentValue(metric.Object.Current)
	}
	if metric.Type == autoscalingv2.ExternalMetricSourceType && metric.External != nil {
		return projectedMetricCurrentValue(metric.External.Current)
	}
	return projectedUnknown
}

func projectedMetricTargetValue(target autoscalingv2.MetricTarget) string {
	switch {
	case target.AverageUtilization != nil:
		return strconv.Itoa(int(*target.AverageUtilization)) + "%"
	case target.AverageValue != nil:
		return target.AverageValue.String()
	case target.Value != nil:
		return target.Value.String()
	default:
		return projectedUnknown
	}
}

func projectedMetricCurrentValue(current autoscalingv2.MetricValueStatus) string {
	switch {
	case current.AverageUtilization != nil:
		return strconv.Itoa(int(*current.AverageUtilization)) + "%"
	case current.AverageValue != nil:
		return current.AverageValue.String()
	case current.Value != nil:
		return current.Value.String()
	default:
		return projectedUnknown
	}
}
