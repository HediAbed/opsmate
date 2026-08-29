package ui

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"

	"github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/kube"
)

type resourceLiveState[T any] struct {
	Items []T
	Ready bool
	Err   error
}

type resourceLiveSet[T any] interface {
	Changes() <-chan struct{}
	State() resourceLiveState[T]
	Stop()
}

type projectedLiveSet[Source, Target any] struct {
	source  kube.LiveSet[Source]
	project func([]Source, time.Time) []Target
	now     func() time.Time
}

func (c nativeClusterCommands) ObservePods(namespace string) (resourceLiveSet[cluster.Pod], error) {
	return observeProjectedResource(c, func() (kube.LiveSet[corev1.Pod], error) {
		return c.observer.ObservePods(c.parent, namespace)
	}, projectPods)
}

func (c nativeClusterCommands) ObserveDeployments(namespace string) (resourceLiveSet[cluster.Deployment], error) {
	return observeProjectedResource(c, func() (kube.LiveSet[appsv1.Deployment], error) {
		return c.observer.ObserveDeployments(c.parent, namespace)
	}, projectDeployments)
}

func (c nativeClusterCommands) ObserveEvents(namespace string) (resourceLiveSet[cluster.Event], error) {
	return observeProjectedResource(c, func() (kube.LiveSet[corev1.Event], error) {
		return c.observer.ObserveEvents(c.parent, namespace)
	}, projectEvents)
}

func (c nativeClusterCommands) ObserveIngresses(namespace string) (resourceLiveSet[cluster.Ingress], error) {
	return observeProjectedResource(c, func() (kube.LiveSet[networkingv1.Ingress], error) {
		return c.observer.ObserveIngresses(c.parent, namespace)
	}, projectIngresses)
}

func (c nativeClusterCommands) ObserveNetworkPolicies(namespace string) (resourceLiveSet[cluster.NetworkPolicy], error) {
	return observeProjectedResource(c, func() (kube.LiveSet[networkingv1.NetworkPolicy], error) {
		return c.observer.ObserveNetworkPolicies(c.parent, namespace)
	}, projectNetworkPolicies)
}

func (c nativeClusterCommands) ObservePersistentVolumeClaims(namespace string) (resourceLiveSet[cluster.PersistentVolumeClaim], error) {
	return observeProjectedResource(c, func() (kube.LiveSet[corev1.PersistentVolumeClaim], error) {
		return c.observer.ObservePersistentVolumeClaims(c.parent, namespace)
	}, projectPVCs)
}

func (c nativeClusterCommands) ObserveCronJobs(namespace string) (resourceLiveSet[cluster.CronJob], error) {
	return observeProjectedResource(c, func() (kube.LiveSet[batchv1.CronJob], error) {
		return c.observer.ObserveCronJobs(c.parent, namespace)
	}, projectCronJobs)
}

func (c nativeClusterCommands) ObserveHorizontalPodAutoscalers(namespace string) (resourceLiveSet[cluster.HPA], error) {
	return observeProjectedResource(c, func() (kube.LiveSet[autoscalingv2.HorizontalPodAutoscaler], error) {
		return c.observer.ObserveHorizontalPodAutoscalers(c.parent, namespace)
	}, projectHPAs)
}

func (c nativeClusterCommands) ObserveSecrets(namespace string) (resourceLiveSet[cluster.Secret], error) {
	return observeProjectedResource(c, func() (kube.LiveSet[kube.ResourceMetadata], error) {
		return c.observer.ObserveSecrets(c.parent, namespace)
	}, projectSecrets)
}

func (c nativeClusterCommands) ObserveReplicaSets(namespace string) (resourceLiveSet[cluster.ReplicaSet], error) {
	return observeProjectedResource(c, func() (kube.LiveSet[appsv1.ReplicaSet], error) {
		return c.observer.ObserveReplicaSets(c.parent, namespace)
	}, projectReplicaSets)
}

func observeProjectedResource[Source, Target any](
	commands nativeClusterCommands,
	observe func() (kube.LiveSet[Source], error),
	project func([]Source, time.Time) []Target,
) (resourceLiveSet[Target], error) {
	source, err := observe()
	if err != nil {
		return nil, err
	}
	return &projectedLiveSet[Source, Target]{source: source, project: project, now: commands.now}, nil
}

func (s *projectedLiveSet[Source, Target]) Changes() <-chan struct{} {
	return s.source.Changes()
}

func (s *projectedLiveSet[Source, Target]) State() resourceLiveState[Target] {
	state := s.source.State()
	return resourceLiveState[Target]{
		Items: s.project(state.Items, s.now()),
		Ready: state.Ready,
		Err:   state.Err,
	}
}

func (s *projectedLiveSet[Source, Target]) Stop() {
	s.source.Stop()
}
