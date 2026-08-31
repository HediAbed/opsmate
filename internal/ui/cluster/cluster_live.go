package cluster

import (
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"

	model "github.com/HediAbed/opsmate/internal/cluster"
	"github.com/HediAbed/opsmate/internal/kube"
)

type LiveState[T interface{}] struct {
	Items []T
	Ready bool
	Err   error
}

type LiveSet[T interface{}] interface {
	Changes() <-chan struct{}
	State() LiveState[T]
	Stop()
}

type projectedLiveSet[Source, Target interface{}] struct {
	source  kube.LiveSet[Source]
	project func([]Source, time.Time) []Target
	now     func() time.Time
}

func (c ResourceCommands) ObservePods(namespace string) (LiveSet[model.Pod], error) {
	return observeProjectedResource(c, func() (kube.LiveSet[corev1.Pod], error) {
		return c.observer.ObservePods(c.parent, namespace)
	}, projectPods)
}

func (c ResourceCommands) ObserveDeployments(namespace string) (LiveSet[model.Deployment], error) {
	return observeProjectedResource(c, func() (kube.LiveSet[appsv1.Deployment], error) {
		return c.observer.ObserveDeployments(c.parent, namespace)
	}, projectDeployments)
}

func (c ResourceCommands) ObserveEvents(namespace string) (LiveSet[model.Event], error) {
	return observeProjectedResource(c, func() (kube.LiveSet[corev1.Event], error) {
		return c.observer.ObserveEvents(c.parent, namespace)
	}, projectEvents)
}

func (c ResourceCommands) ObserveIngresses(namespace string) (LiveSet[model.Ingress], error) {
	return observeProjectedResource(c, func() (kube.LiveSet[networkingv1.Ingress], error) {
		return c.observer.ObserveIngresses(c.parent, namespace)
	}, projectIngresses)
}

func (c ResourceCommands) ObserveNetworkPolicies(namespace string) (LiveSet[model.NetworkPolicy], error) {
	return observeProjectedResource(c, func() (kube.LiveSet[networkingv1.NetworkPolicy], error) {
		return c.observer.ObserveNetworkPolicies(c.parent, namespace)
	}, projectNetworkPolicies)
}

func (c ResourceCommands) ObservePersistentVolumeClaims(namespace string) (LiveSet[model.PersistentVolumeClaim], error) {
	return observeProjectedResource(c, func() (kube.LiveSet[corev1.PersistentVolumeClaim], error) {
		return c.observer.ObservePersistentVolumeClaims(c.parent, namespace)
	}, projectPVCs)
}

func (c ResourceCommands) ObserveCronJobs(namespace string) (LiveSet[model.CronJob], error) {
	return observeProjectedResource(c, func() (kube.LiveSet[batchv1.CronJob], error) {
		return c.observer.ObserveCronJobs(c.parent, namespace)
	}, projectCronJobs)
}

func (c ResourceCommands) ObserveHorizontalPodAutoscalers(namespace string) (LiveSet[model.HPA], error) {
	return observeProjectedResource(c, func() (kube.LiveSet[autoscalingv2.HorizontalPodAutoscaler], error) {
		return c.observer.ObserveHorizontalPodAutoscalers(c.parent, namespace)
	}, projectHPAs)
}

func (c ResourceCommands) ObserveSecrets(namespace string) (LiveSet[model.Secret], error) {
	return observeProjectedResource(c, func() (kube.LiveSet[kube.ResourceMetadata], error) {
		return c.observer.ObserveSecrets(c.parent, namespace)
	}, projectSecrets)
}

func (c ResourceCommands) ObserveReplicaSets(namespace string) (LiveSet[model.ReplicaSet], error) {
	return observeProjectedResource(c, func() (kube.LiveSet[appsv1.ReplicaSet], error) {
		return c.observer.ObserveReplicaSets(c.parent, namespace)
	}, projectReplicaSets)
}

func observeProjectedResource[Source, Target interface{}](
	commands ResourceCommands,
	observe func() (kube.LiveSet[Source], error),
	project func([]Source, time.Time) []Target,
) (LiveSet[Target], error) {
	source, err := observe()
	if err != nil {
		return nil, err
	}
	return &projectedLiveSet[Source, Target]{source: source, project: project, now: commands.now}, nil
}

func (s *projectedLiveSet[Source, Target]) Changes() <-chan struct{} {
	return s.source.Changes()
}

func (s *projectedLiveSet[Source, Target]) State() LiveState[Target] {
	state := s.source.State()
	return LiveState[Target]{
		Items: s.project(state.Items, s.now()),
		Ready: state.Ready,
		Err:   state.Err,
	}
}

func (s *projectedLiveSet[Source, Target]) Stop() {
	s.source.Stop()
}
