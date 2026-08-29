package kube

import (
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func stripManagedFields(object any) (any, error) {
	metadata, err := apiMeta.Accessor(object)
	if err != nil {
		return nil, err
	}
	metadata.SetManagedFields(nil)
	return object, nil
}

func stripSensitiveMetadata(object any) (any, error) {
	metadata, err := apiMeta.Accessor(object)
	if err != nil {
		return nil, err
	}
	metadata.SetManagedFields(nil)
	metadata.SetAnnotations(nil)
	return object, nil
}

func decodeLivePod(object any) (corev1.Pod, error) {
	item, ok := object.(*corev1.Pod)
	if !ok || item == nil {
		return corev1.Pod{}, ErrUnexpectedResourceObject
	}
	return *item.DeepCopy(), nil
}

func decodeLiveDeployment(object any) (appsv1.Deployment, error) {
	item, ok := object.(*appsv1.Deployment)
	if !ok || item == nil {
		return appsv1.Deployment{}, ErrUnexpectedResourceObject
	}
	return *item.DeepCopy(), nil
}

func decodeLiveEvent(object any) (corev1.Event, error) {
	item, ok := object.(*corev1.Event)
	if !ok || item == nil {
		return corev1.Event{}, ErrUnexpectedResourceObject
	}
	return *item.DeepCopy(), nil
}

func decodeLiveIngress(object any) (networkingv1.Ingress, error) {
	item, ok := object.(*networkingv1.Ingress)
	if !ok || item == nil {
		return networkingv1.Ingress{}, ErrUnexpectedResourceObject
	}
	return *item.DeepCopy(), nil
}

func decodeLiveNetworkPolicy(object any) (networkingv1.NetworkPolicy, error) {
	item, ok := object.(*networkingv1.NetworkPolicy)
	if !ok || item == nil {
		return networkingv1.NetworkPolicy{}, ErrUnexpectedResourceObject
	}
	return *item.DeepCopy(), nil
}

func decodeLivePersistentVolumeClaim(object any) (corev1.PersistentVolumeClaim, error) {
	item, ok := object.(*corev1.PersistentVolumeClaim)
	if !ok || item == nil {
		return corev1.PersistentVolumeClaim{}, ErrUnexpectedResourceObject
	}
	return *item.DeepCopy(), nil
}

func decodeLiveCronJob(object any) (batchv1.CronJob, error) {
	item, ok := object.(*batchv1.CronJob)
	if !ok || item == nil {
		return batchv1.CronJob{}, ErrUnexpectedResourceObject
	}
	return *item.DeepCopy(), nil
}

func decodeLiveHorizontalPodAutoscaler(object any) (autoscalingv2.HorizontalPodAutoscaler, error) {
	item, ok := object.(*autoscalingv2.HorizontalPodAutoscaler)
	if !ok || item == nil {
		return autoscalingv2.HorizontalPodAutoscaler{}, ErrUnexpectedResourceObject
	}
	return *item.DeepCopy(), nil
}

func decodeLiveReplicaSet(object any) (appsv1.ReplicaSet, error) {
	item, ok := object.(*appsv1.ReplicaSet)
	if !ok || item == nil {
		return appsv1.ReplicaSet{}, ErrUnexpectedResourceObject
	}
	return *item.DeepCopy(), nil
}

func decodeLiveMetadata(object any) (ResourceMetadata, error) {
	item, ok := object.(*metav1.PartialObjectMetadata)
	if !ok || item == nil {
		return ResourceMetadata{}, ErrUnexpectedResourceObject
	}
	return ResourceMetadata{
		Name:      item.Name,
		Namespace: item.Namespace,
		CreatedAt: item.CreationTimestamp.Time,
	}, nil
}

var _ ResourceObserver = (*Manager)(nil)
