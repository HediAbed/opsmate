package kube

import (
	"context"
	"strconv"
	"time"

	"golang.org/x/sync/errgroup"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
)

const (
	maximumConcurrentMutations = 4
	restartAnnotation          = "opsmate.io/restarted-at"
	restartPatchPrefix         = `{"spec":{"template":{"metadata":{"annotations":{"` + restartAnnotation + `":`
	restartPatchSuffix         = `}}}}}`
	jsonStringQuoteCount       = 2
)

type workloadScaleClient interface {
	GetScale(context.Context, string, metav1.GetOptions) (*autoscalingv1.Scale, error)
	UpdateScale(context.Context, string, *autoscalingv1.Scale, metav1.UpdateOptions) (*autoscalingv1.Scale, error)
}

func (m *Manager) Delete(parent context.Context, reference ResourceReference) error {
	identifier := reference.Identifier()
	if parent == nil {
		return newResourceError(OperationDelete, SubjectResource, identifier, ErrContextRequired)
	}
	if err := validateResourceReference(reference); err != nil {
		return newResourceError(OperationDelete, SubjectResource, identifier, err)
	}
	clients, ctx, cancel, err := m.clientSession(parent)
	if err != nil {
		return newResourceError(OperationDelete, SubjectResource, identifier, err)
	}
	defer cancel()
	mapping, err := resolveResourceMapping(clients, reference.Resource)
	if err != nil {
		return newResourceError(OperationResolve, SubjectResource, identifier, err)
	}
	resource, err := resourceClient(clients, mapping, reference.Namespace)
	if err != nil {
		return newResourceError(OperationDelete, SubjectResource, identifier, err)
	}
	propagation := metav1.DeletePropagationBackground
	if err := resource.Delete(ctx, reference.Name, metav1.DeleteOptions{PropagationPolicy: &propagation}); err != nil {
		return newResourceError(OperationDelete, SubjectResource, identifier, err)
	}
	return nil
}

func (m *Manager) DeleteBatch(parent context.Context, batch ResourceBatch) (BatchOutcome, error) {
	if parent == nil {
		return BatchOutcome{}, newResourceError(OperationDelete, SubjectResource, batch.Resource.String(), ErrContextRequired)
	}
	if err := validateBatch(batch.Resource, batch.Names); err != nil {
		return BatchOutcome{}, newResourceError(OperationDelete, SubjectResource, batch.Resource.String(), err)
	}
	clients, ctx, cancel, err := m.clientSession(parent)
	if err != nil {
		return BatchOutcome{}, newResourceError(OperationDelete, SubjectResource, batch.Resource.String(), err)
	}
	defer cancel()
	mapping, err := resolveResourceMapping(clients, batch.Resource)
	if err != nil {
		return BatchOutcome{}, newResourceError(OperationResolve, SubjectResource, batch.Resource.String(), err)
	}
	resource, err := resourceClient(clients, mapping, batch.Namespace)
	if err != nil {
		return BatchOutcome{}, newResourceError(OperationDelete, SubjectResource, batch.Resource.String(), err)
	}
	propagation := metav1.DeletePropagationBackground
	outcome := runBounded(batch.Names, func(name string) error {
		if deleteErr := resource.Delete(ctx, name, metav1.DeleteOptions{PropagationPolicy: &propagation}); deleteErr != nil {
			reference := ResourceReference{Resource: batch.Resource, Namespace: batch.Namespace, Name: name}
			return newResourceError(OperationDelete, SubjectResource, reference.Identifier(), deleteErr)
		}
		return nil
	})
	return outcome, outcome.Failure()
}

func (m *Manager) Scale(parent context.Context, request ScaleRequest) error {
	identifier := request.Workload.Identifier()
	if parent == nil {
		return newResourceError(OperationUpdate, SubjectWorkload, identifier, ErrContextRequired)
	}
	if err := validateWorkloadReference(request.Workload); err != nil {
		return newResourceError(OperationUpdate, SubjectWorkload, identifier, err)
	}
	if request.Replicas < 0 {
		return newResourceError(OperationUpdate, SubjectWorkload, identifier, ErrReplicaCountInvalid)
	}
	clients, ctx, cancel, err := m.clientSession(parent)
	if err != nil {
		return newResourceError(OperationUpdate, SubjectWorkload, identifier, err)
	}
	defer cancel()
	typedClient := clients.Kubernetes()
	if typedClient == nil {
		return newResourceError(OperationUpdate, SubjectWorkload, identifier, ErrTypedClientUnavailable)
	}
	if err := scaleWorkload(ctx, typedClient, request); err != nil {
		return newResourceError(OperationUpdate, SubjectWorkload, identifier, err)
	}
	return nil
}

func scaleWorkload(ctx context.Context, client kubernetes.Interface, request ScaleRequest) error {
	workload := request.Workload
	var scaleClient workloadScaleClient
	switch workload.Kind {
	case WorkloadDeployment:
		scaleClient = client.AppsV1().Deployments(workload.Namespace)
	case WorkloadStatefulSet:
		scaleClient = client.AppsV1().StatefulSets(workload.Namespace)
	default:
		return ErrUnsupportedWorkloadKind
	}
	return updateScaleWithRetry(ctx, scaleClient, workload.Name, request.Replicas)
}

func updateScaleWithRetry(ctx context.Context, client workloadScaleClient, name string, replicas int32) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		scale, err := client.GetScale(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		scale.Spec.Replicas = replicas
		_, err = client.UpdateScale(ctx, name, scale, metav1.UpdateOptions{})
		return err
	})
}

func (m *Manager) Restart(parent context.Context, reference WorkloadReference) error {
	identifier := reference.Identifier()
	if parent == nil {
		return newResourceError(OperationUpdate, SubjectWorkload, identifier, ErrContextRequired)
	}
	if err := validateWorkloadReference(reference); err != nil {
		return newResourceError(OperationUpdate, SubjectWorkload, identifier, err)
	}
	clients, ctx, cancel, err := m.clientSession(parent)
	if err != nil {
		return newResourceError(OperationUpdate, SubjectWorkload, identifier, err)
	}
	defer cancel()
	typedClient := clients.Kubernetes()
	if typedClient == nil {
		return newResourceError(OperationUpdate, SubjectWorkload, identifier, ErrTypedClientUnavailable)
	}
	if err := restartWorkload(ctx, typedClient, reference, m.clock()); err != nil {
		return newResourceError(OperationUpdate, SubjectWorkload, identifier, err)
	}
	return nil
}

func (m *Manager) RestartBatch(parent context.Context, batch WorkloadBatch) (BatchOutcome, error) {
	identifier := batch.Namespace + "/" + batch.Kind.String()
	if parent == nil {
		return BatchOutcome{}, newResourceError(OperationUpdate, SubjectWorkload, identifier, ErrContextRequired)
	}
	if err := validateWorkloadBatch(batch); err != nil {
		return BatchOutcome{}, newResourceError(OperationUpdate, SubjectWorkload, identifier, err)
	}
	clients, ctx, cancel, err := m.clientSession(parent)
	if err != nil {
		return BatchOutcome{}, newResourceError(OperationUpdate, SubjectWorkload, identifier, err)
	}
	defer cancel()
	typedClient := clients.Kubernetes()
	if typedClient == nil {
		return BatchOutcome{}, newResourceError(OperationUpdate, SubjectWorkload, identifier, ErrTypedClientUnavailable)
	}
	restartedAt := m.clock()
	outcome := runBounded(batch.Names, func(name string) error {
		reference := WorkloadReference{Kind: batch.Kind, Namespace: batch.Namespace, Name: name}
		if restartErr := restartWorkload(ctx, typedClient, reference, restartedAt); restartErr != nil {
			return newResourceError(OperationUpdate, SubjectWorkload, reference.Identifier(), restartErr)
		}
		return nil
	})
	return outcome, outcome.Failure()
}

func restartWorkload(ctx context.Context, client kubernetes.Interface, reference WorkloadReference, restartedAt time.Time) error {
	patch := newRestartPatch(restartedAt)
	switch reference.Kind {
	case WorkloadDeployment:
		_, err := client.AppsV1().Deployments(reference.Namespace).Patch(
			ctx, reference.Name, types.StrategicMergePatchType, patch, metav1.PatchOptions{},
		)
		return err
	case WorkloadStatefulSet:
		_, err := client.AppsV1().StatefulSets(reference.Namespace).Patch(
			ctx, reference.Name, types.StrategicMergePatchType, patch, metav1.PatchOptions{},
		)
		return err
	default:
		return ErrUnsupportedWorkloadKind
	}
}

func newRestartPatch(restartedAt time.Time) []byte {
	annotationValue := restartedAt.UTC().Format(time.RFC3339Nano)
	capacity := len(restartPatchPrefix) + len(annotationValue) + len(restartPatchSuffix) + jsonStringQuoteCount
	patch := make([]byte, 0, capacity)
	patch = append(patch, restartPatchPrefix...)
	patch = strconv.AppendQuote(patch, annotationValue)
	return append(patch, restartPatchSuffix...)
}

func runBounded(names []string, mutate func(string) error) BatchOutcome {
	results := make([]error, len(names))
	group := errgroup.Group{}
	group.SetLimit(maximumConcurrentMutations)
	for index, name := range names {
		index, name := index, name
		group.Go(func() error {
			results[index] = mutate(name)
			return nil
		})
	}
	_ = group.Wait()
	outcome := BatchOutcome{
		Succeeded: make([]string, 0, len(names)),
		Failed:    make([]BatchFailure, 0),
	}
	for index, name := range names {
		if results[index] == nil {
			outcome.Succeeded = append(outcome.Succeeded, name)
			continue
		}
		outcome.Failed = append(outcome.Failed, BatchFailure{Name: name, Err: results[index]})
	}
	return outcome
}

var _ ResourceWriter = (*Manager)(nil)
